# PR 1.2 — Validate and publish private Framework generations

## Outcome

Go can ask Java to check an authored publication or install a complete private generation. Validation leaves the serving generation untouched, and a failed installation leaves the prior generation serving.

## Requirements

- Implement the Java side of `POST /internal/v1/validate` and `POST /internal/v1/publish`. Both consume the original `application/zip` `pack` part through the shared adapter and the `application/json` `metadata` part. Both require the destination namespace and complete effective `runtimeSettings`. Reject missing or invalid settings and identify the offending path. Compose a single in-memory Framework configuration with the portable sections and the supplied session and trace settings. Add `session` and `execution-trace` under `loomspan`.
- Require every field and wire type in the [runtime settings contract](../future/execution-engine-api.md#runtime-settings). Go supplies Cloud policy on both requests. V1 sends `mission-timeout: "60s"`, integer `max-depth: 32`, and `attachments.max-size: "20971520B"`. The six integer quotas for skill invocations, tool invocations, linter retries, model calls, provider attempts (including retries), and usage units are 64, 128, 32, 64, 192, and 200000 respectively. The owner selects trace persistence from `NEVER`, `ONERROR` (new application default), and `ALWAYS`. Framework captures these settings with the generation and enforces the session limits. The trace directory is host configuration.
- Validate the authored candidate with private visibility through `SkillReloader.validate`, without variable values, provider clients, or generation activation. Collect the results of independent checks, including authored Vertex policy findings and unrelated skill references when they can be checked. A completed validation returns `200` with `valid`, `issues`, and `skills`. Include `severity`, `sourceName`, and `message` on each issue, plus `skillName` and `fieldPath` when available. Each validated skill carries `namespace`, `localName`, and `kind`. Warnings alone permit `valid: true`. An error yields `valid: false` and empty `skills`. When validation cannot complete or the request is rejected before validation can start, return a non-success status and a `text/plain` failure message.
- For publish, require the assigned opaque publication version and a non-null `variableValues` map with exactly the distinct referenced names and a nonblank string for each, including an empty map for a reference-free pack. Use the private, versioned, explicit-map `SkillReloader.prepare` overload, then `publish`. Resolve values exclusively from that map as literal strings. Every successful publish installs a fresh generation, including a same-version restart. Keep the previous generation serving throughout preparation. Use only Cloud-vetted host extensions and isolate each application’s settings and credentials in its private namespace. A zero-skill update clears previous YAML skills while keeping the namespace installed.
- At staged validation, report tenant Vertex AI credential-policy violations that are decidable from the authored YAML. Before any installation builds a provider client, enforce the policy on the effective connection, including variable-selected keys, driver, Vertex mode, and credentials. Require nonblank service-account JSON in `gemini.credentials-json` for Vertex AI tenant connections, with no engine Application Default Credentials fallback. Reject a candidate whose compliance cannot be established and identify the connection path.
- Return `204 No Content` only after a successful Framework publish. Return a non-success HTTP status with the original failure message as `text/plain` for a rejected request or a preparation or publication exception.

## Acceptance criteria

- [ ] A completed validation reports warnings and errors in the documented shape, preserves source paths, and reports an authored credential-policy violation alongside independent findings. Validation can complete without a value map and leaves the serving generation untouched. An interrupted validation returns a non-success text response instead of a completed report.
- [ ] An install requires complete, valid runtime metadata and an exact variable map. A reference-free pack installs with an empty map. Missing, blank, and extra variable entries fail, even when the host has a matching value. Verify required settings and wire types on both endpoints, including the offending field path on rejection.
- [ ] Exercise the [worked variable corpus and isolation checks](../future/application-variable-conformance.md). Use different values for the same names in two namespaces, a third value on the host, and a literal containing placeholder syntax. Verify Framework rejects an effective mapping-key collision during substitution and leaves the current generation serving. A same-version replacement captures edited values while work already admitted retains its originals.
- [ ] Vertex policy coverage exercises literal and variable service-account JSON, an omitted declaration with engine credentials available, missing or invalid values, a variable-selected Vertex connection, file or URI credential fields, and ordinary Gemini API-key mode. Check authored violations without looking up values. Check effective policy on all installation paths, including restore and same-version restart. Failures occur before client construction, report the connection path, and leave the previous generation serving.
- [ ] A successful installation returns an empty `204` response and a private generation with the supplied version. Reinstalling the version creates a new generation. Installing zero skills leaves the namespace installed with its YAML skills removed.
- [ ] Rejected installation and a publication exception return non-success text responses with their messages and leave the previous generation serving.

## Context

GitHub PR 1, ticket 1.2 of 5. Depends on [PR 1.1](pr-1.1-adapt-publication-zip.md). [PR 1.3](pr-1.3-authenticate-and-observe-engine-pods.md) supplies internal transport authentication and K3s observation. This ticket owns Java endpoint behavior. Go staging, publication records, and reconciliation are separate.

[Request and runtime-settings contract](../future/execution-engine-api.md#internal-api-requests) · [Response contract](../future/execution-engine-api.md#validate-and-publish-responses) · [Variable conformance](../future/application-variable-conformance.md) · [Tenant Vertex policy](../future/java-execution-engine-roadmap.md#tenant-vertex-ai-credentials)

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** This adds a cross-language API, tenant credential enforcement, and generation lifecycle behavior.
- **Reassessment triggers:** Verify that the embedded Framework can enforce the effective connection policy before client construction while preserving its variable semantics and standalone uses.

## Pipeline notes

PR 1 tickets are committed locally in order without syncing between them. Intermediate commits may have incomplete wiring or build failures at the ticket boundary. Verify this ticket’s behavior where possible and record any verification that depends on later tickets. The cumulative milestone must build and pass its end-to-end checks in PR 1.5.
