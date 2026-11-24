# Private Framework Generation Implementation Plan

## Overview

- Ticket — `ai/thoughts/tickets/pr-1.2-validate-and-publish-engine-generations.md`
- Research — `ai/thoughts/research/pr-1.2-validate-and-publish-engine-generations.md`
- Outcome — Validate authored publications or atomically install versioned private generations through the two Java multipart endpoints.

## Current State

`PublicationZipAdapter.adapt(String, Part)` currently returns exact `SkillDocument` sources and the portable YAML through `AdaptedPublication`. The module has Servlet multipart limits, SnakeYAML Engine, Spring Boot's Web MVC, and the Framework starter. Framework's supported `SkillReloader` exposes `validate`, versioned explicit-map `prepare`, and `publish`. `DefaultSkillReloader` acquires admission for validation and guards activation on the prepared base generation. Its runtime factory eagerly creates provider clients during `prepare`. Framework permits a Vertex connection with omitted credentials and can fall back to ADC.

## Desired End State

Both endpoints validate the runtime metadata before calling Framework and compose it into one in-memory candidate. Staged validation returns a completed report with independent Cloud policy and Framework findings. Publication checks effective connection policy before prepare and exposes the supplied version only after activation. Repeated versions still create new generations. Failures keep the existing generation serving and preserve the HTTP failure message.

## Scope

### In scope

- The Java multipart validate and publish handlers, Cloud metadata checks, YAML composition, policy checks, and HTTP and Framework integration tests.
- All installation origins share the same Java publish code. The Go-side restore and restart workflows are covered in their owning tickets.

### Out of scope

- Internal API mTLS, Go, Kubernetes, placement, invocation REST transport, and live cloud verification.
- Widening Framework's supported SPI or modifying the sibling Framework checkout. Changes use the checked starter snapshot and isolate the reliance on Framework internals.

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Impact and Risk Analysis

- Presence and JSON wire types are part of the endpoint contract. Framework's YAML binder applies defaults and coerces values, so the wrapper must check types and valid domains before composition.
- The same YAML mapping key semantics, alias structure, and literal variable strings must reach policy preflight and the real generation. Converting portable YAML to a JSON-like object would change supported complex keys and aliases.
- A missed effective Vertex connection could give tenant code an engine credential. Examine each typed connection after Framework's own substitution and binding. Framework currently gives Cloud no supported pre-client callback. Keep the two internal helper dependencies narrow, backed by regression tests that exercise the real starter, and fail the install when effective policy cannot be established.
- Provider construction must occur only after the whole effective connection set passes. Validation must never build a provider, consult host variables, or prepare a generation.
- Publication can fail both during preparation and at the final base-generation/lifecycle check. Ensure a rejected prepared update is closed and leave pointer ownership to Framework.

## Implementation Approach

`PublicationController` owns HTTP and delegates typed publication work to `PublicationService`. `PublicationMetadata` owns presence, wire types, and value domains. `PublicationConfiguration` owns structural YAML composition. `TenantVertexPolicy` contains the Cloud rule and Framework internal parser calls. This keeps the normal publish path on Framework's supported reloader and avoids a separate Cloud implementation of candidate substitution or a replacement runtime factory.

The Go backend owns the numeric Cloud policy values and selected trace persistence. Java accepts complete, well-typed, valid settings in the permitted domains, including an explicit size unit, and never derives defaults for omitted fields. Validate exact trace choices `NEVER`, `ONERROR`, `ALWAYS`, a positive explicit-unit or ISO duration accepted by Spring, a positive signed-32-bit integral depth, nonnegative signed-32-bit integral quotas, and a nonnegative Spring size with an explicit unit. The V1 values are the cross-language example and the default Go payload. The trace directory stays host-configured. Keep namespace metadata exact and validate the namespace grammar in the package contract with a path-bearing error. Treat publication versions as opaque nonblank strings and pass them verbatim.

The ZIP adapter's reserved-field scan and configuration composition share the same SnakeYAML settings. Non-scalar keys are enabled because a documented variable marker can be an authored mapping key. Framework still checks the semantics of those keys. The real-Framework fixture registers its executor in a separate, public host namespace, so the same fixed executor can read each private tenant's captured configuration and an empty tenant pack can remove the YAML skills.

## Phase 1 — Multipart request, metadata, and composition

### Changes

- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationController.java` — map the two exact POST routes under `/internal/v1`, accept precisely one MIME part of each required name, check the metadata media type, and pass the `pack` part directly to `PublicationZipAdapter`. Check duplicates rather than selecting one of multiple same-named parts. Use Boot's Jackson 3 object mapper or an immutable configured reader to consume one JSON object without coercion, ambiguous duplicate keys, or trailing documents. Enforce the same ZIP boundary and parser properties introduced in PR 1.1.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationMetadata.java` — validate the paths `namespace`, `runtimeSettings.session.mission-timeout`, `max-depth`, `attachments.max-size`, all six leaves under `quotas`, and `runtimeSettings.execution-trace.persistence`. Require `publicationVersion` and an object `variableValues` for publish, with all values nonblank strings and `{}` permitted. Validation reads neither a value map nor a host value source. Copy incoming accepted values unchanged and immutable. Enforce exact map membership in Framework's explicit-map candidate path.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationConfiguration.java` — use SnakeYAML Engine's `Compose`, `MappingNode`, `Serialize` and `Present` on the authored node graph to insert only `session` and `execution-trace` under the `loomspan` mapping. Build the added nodes from the already validated metadata scalars without reparsing unquoted arbitrary text. Preserve complex keys, mapping order, alias relationships, and exact variable scalar values. Keep one `ExecutionConfiguration` alongside the original exact skill sources. When a structurally authored document cannot take settings, preserve its invalid shape for Framework diagnostics or return a prevalidation packaging error if it cannot be parsed at all. Never generate a valid replacement for invalid authored content.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationController.java` and narrowly scoped MVC resolver where needed — on parse, pre-dispatch multipart, metadata, ZIP, or other operational failure, return non-success `text/plain` with the exception's message intact. Include oversized multipart and unsupported media type caught before a handler runs. Scope handling to the two publication routes so later invoke responses can have their own contract. Do not serialize an exception as a `200` validation report.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationControllerTest.java` — exercise real HTTP multipart parsing and runtime setting checks on both routes.

### Automated verification

- [x] From `execution-engine/`, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest' test` — parser and wire contract pass.

### Optional developer checks

None.

## Phase 2 — Independent authored validation and pre-client publication

### Changes

- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/TenantVertexPolicy.java` — use the Framework-authored connection slice with its staged `Unresolved` markers to check known authored connections without a lookup. For a known Gemini Vertex connection, report missing, blank, non-string, or invalid `gemini.credentials-json` when the key is known. Defer decisions that depend on an unresolved key to publication. Recognize string and boolean modes through the same Spring conversions used by binding. Report explicitly authored file and URI credential options under Gemini at the offending field paths. When the connection slice is readable, check it independently of materialization or binding failures elsewhere in the document. Give Framework the real authored candidate and combine findings when both checks can complete.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/TenantVertexPolicy.java` — for publish use `AuthoredYamlVariables.execution(...).discover()` to take the execution-only projection of the same supplied immutable value map. Pass that projection to `ExecutionConfigurationParser.parse(namespace, composedConfiguration, executionValues)`. Framework's parser prepares no clients and uses the same staged and effective binding as the eventual `prepare`. Check every entry in `Parsed.properties().getConnections()` by typed driver and Vertex boolean. Reject missing or blank credential text and verify explicit service-account JSON in memory with the same `ServiceAccountCredentials.fromStream` class as the provider integration. Preserve path and safe reason, never include the credential in a policy failure. Framework's strict typed parser rejects file or URI credential fields, including substituted keys. Parse failure stops before any `prepare`; where Framework supplies an authored occurrence or connection path preserve it. Never consult ADC, use file credentials, resolve placeholders a second time, or bypass preflight for same-version installs.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationService.java` — run authored policy and `SkillReloader.validate(namespace, skills, configuration, PRIVATE)` independently. Return stable Cloud response records for `valid`, `issues`, and `skills`, with enum wire names and source location fields. Translate Framework's generic `<configuration>` source to `loomspan/execution.yaml`, preserve exact skill sources, and omit unavailable optional fields. An `ERROR` from any check sets `valid` false and empties skills. Keep warnings on successful validation. A thrown lifecycle/operational validation failure propagates to the HTTP failure handler.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationService.java` — gate every publication on effective policy and call `SkillReloader.prepare(namespace, publicationVersion, skills, configuration, PRIVATE, fullVariableMap)`. Pass the complete immutable map, including names used only in skills. Use try-with-resources for the `PreparedSkillUpdate` and call `publish` inside it. Only acknowledge 204 after Framework's successful activation. Preserve preparation and publication exception messages unchanged. Load extensions from normal host registration only and leave lifecycle, visibility, and generation identity to Framework.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/TenantVertexPolicyTest.java` and `PublicationGenerationTest.java` — establish authored and effective credential cases, exact map and generated release semantics, and real generation retention when preparation or publication fails.

### Automated verification

- [x] From `execution-engine/`, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest,TenantVertexPolicyTest,PublicationGenerationTest' test`.
- [x] From `execution-engine/`, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — local adapter and endpoint suites pass and Boot repackages the module.

### Optional developer checks

None. Credential parsing, provider-bound configuration inspection, and actual executor tests require no external provider or host credential.

## Test Strategy

Use deterministic ZIP/JSON input and HTTP tests for parser and response shape, internal-parser tests for policy preflight, and real Framework tests to see activated metadata, exact values, and concurrent old/new work. Use a stub only where a deterministic operational interruption or publication exception requires one. Anchor the regression corpus in the Framework-owned worked and collision examples with portable checked-in test inputs.

## Acceptance-Criteria Traceability

| Criterion | Planned code evidence | Planned test evidence |
| --- | --- | --- |
| Completed report, warnings, exact sources, independent issues, interrupted failure, no activation | `PublicationService.validate`, metadata/HTTP error boundary | Controller wire tests, Framework independent-issues and serving-generation tests |
| Valid settings and exact map | `PublicationMetadata`, Framework explicit-map prepare | Both endpoints' path/type matrix, `{}` reference-free install, missing/blank/extra with host value |
| Worked variables, literal substitution, collision, isolation and same-version rotation | Node composition, execution-only preflight map, full explicit-map prepare, lifecycle ownership | Worked and collision pack tests, provider and executor state assertions, latch-based captured work |
| Authored and effective Vertex credential decisions | `TenantVertexPolicy` staged and typed checks before prepare | Synthetic credential, host fallback, variable-selected key/mode/value, file/URI, API-key and rejected replacement tests |
| 204, private versioned generation, new ID and zero-skill update | `PublicationService.publish`, controller status | Real active namespace/version/generation assertion and cleared skills after empty publication |
| Installation and publication exceptions preserve previous service and text | Framework guarded activation, scoped HTTP failure mapping | Existing-generation probe after rejected preparation and an injected failure at `publish` with a real detached candidate, exact HTTP text assertions |

## Risks and Rollback/Recovery

The typed preflight relies on Framework internal APIs because the supported reloader has no pre-client hook. Review their semantics when changing the starter snapshot. Verify the same typed binder sees both preflight and prepare inputs and guard regressions at the production entry point. PR 1.2 changes in-memory state only. Failed prepared updates are closed, and a running instance can continue serving its current generation after any candidate failure. A source rollback removes the endpoints and leaves durable data unaffected.

## References

- `ai/thoughts/future/execution-engine-api.md`
- `ai/thoughts/future/java-execution-engine-roadmap.md`
- `ai/thoughts/future/application-variable-conformance.md`
- Framework `SkillReloader`, `DefaultSkillReloader`, `SkillGenerationManager`, `ExecutionConfigurationParser`, `AuthoredYamlVariables`, `LoomspanProperties`, and `SpringAiProviderIntegration`.
