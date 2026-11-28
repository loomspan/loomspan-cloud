# PR 2.1.5 — Stage an exact pack ZIP and return real engine validation

## Outcome

A Manager can upload a portable ZIP for one application and obtain a revision ID and advisory validation feedback. Rejected bytes leave staging untouched; an unavailable engine leaves an accepted pack staged.

## Context

GitHub PR 2, Go backend Phase 1. Consumes the durable pack volume and native Postgres from PR 2.1.2, authenticated Java client from PR 2.1.3, and Manager check/application namespace from PR 2.1.4. This is the owner’s first REST upload; the same operation will later serve MCP and promotion.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [ZIP format and layout](../future/publication-package-format.md) · [Staging and validation](../future/go-backend-roadmap.md#staging-revisions-and-validation) · [Variable fixtures](../future/application-variable-conformance.md) · [Engine settings and reports](../future/execution-engine-api.md#runtime-settings)

## Requirements

Use PR 2.1.3’s [dispatch eligibility for Validate and Publish](../future/go-backend-roadmap.md#dispatch-eligibility-for-validate-and-publish). Verify that while a terminating pod and its replacement are both visible, a new valid upload stays staged with validation unavailable and makes zero Java validation calls.

Implement this ticket’s public routes, request bodies, response fields and statuses from the [Phase 1 Cloud REST API](../future/phase-1-rest-api.md).

- Check current Manager access before touching files. Inspect all archive entries, including companion assets, using inclusive limits of 1,000,000 ZIP bytes, 10,000,000 actual expanded bytes, and 1,000 entries including directories. Enforce one namespace-matching root, valid companion SKILL.md, the specified execution subtree, lowercase skill paths, regular files/directories, unambiguous paths and uniqueness ignoring case. Reject reserved administrative sections and mismatched executor owner keys. Apply namespace length + 2 + local skill length <= 64 before staging, even offline; identify offending skill and pack-relative source.
- Store the original ZIP on the shared volume at `packs/<namespace>/<sha256>.zip`. Use the authorized application's assigned namespace and the SHA-256 digest of the exact uploaded bytes. This keeps ZIP reuse, quota accounting and deletion scoped to one application. Keep the format version outside the ZIP and record staging metadata in Postgres. Use the application row lock when establishing a protected ZIP reference. Make digest comparison and current-revision update atomic. An identical current upload preserves the revision, actor and staging time. Reuse a completed report, join a live validation attempt, or start one shared retry if no completed report was saved. Restaging an older digest can reuse that application's retained file while creating a new staging revision. Rejection preserves the prior revision and results.
- A new revision automatically runs the Cloud checks and authenticated POST /internal/v1/validate with the original pack and JSON metadata for namespace and complete effective runtime settings. Centralize Cloud defaults for reuse in Publish: 60s timeout, depth 32, 20971520B attachments, quotas 64 skill/128 tool/32 linter/64 model/192 provider/200000 usage, persistence ONERROR. Java owns Framework reference and configuration validation. Implement full variable-reference discovery in the shared Go pack inspector used by staging and Publish. Follow the Framework `loomspan-variable-fixtures/README.md` selection, marker, traversal and diagnostic rules. Phase 1 uses an empty application value map. Run Java validation even when values are missing. On completion, return all missing or blank references with their pack-relative sources and authored YAML paths alongside engine findings.
- Keep completed feedback and timestamp with the requested revision. Distinguish a completed invalid report from unavailable validation; withhold partial findings when unavailable. The Phase 1 REST message explains that the ZIP remains staged, validation was unavailable, and an explicit Publish runs fresh validation before installation. After observing termination, send no new validation to that engine. Keep a published revision serving throughout staging. Reviewed publication, downloads, history UI and MCP upload tools are outside this ticket.

## Acceptance criteria

- [ ] Apply the [shared 30-second outbound deadline](../future/execution-engine-api.md#validate-and-publish-responses) to Validate. Accept a completed report whose headers arrive after 10 seconds and whose body finishes within 30 seconds. With a silent engine or a stalled report body, return the staged revision and unavailable feedback at the deadline and withhold partial findings. Hand the same deadline value to PR 2.1.6 for Install.

- [ ] Stage a valid new ZIP as Manager against a seeded `Deleting` application. After authorization, return `409 Conflict` with reason `Deleting`. Verify zero pack writes, unchanged staging and validation feedback, and zero Java validation calls.

- [ ] Submit the portable reference-free pack through authorized REST and receive the staged revision plus real Java validation. Compute the digest from the submitted bytes and verify that the file at `packs/<namespace>/<sha256>.zip` contains those exact bytes. A completed invalid report names its revision; a newer upload cannot redirect the result.
- [ ] Verify each documented archive limit accepts its boundary and rejects excess, invalid paths/layouts are rejected with useful diagnostics, and a combined skill name length of 65 fails offline while 64 passes that check. Rejection and denied access leave staging unchanged.
- [ ] While Java validation is in progress, upload identical bytes to the same application. Both requests return 200 with the same revision ID and feedback. Verify one Java call and the original actor and timestamp. Exercise both a completed report and a validation timeout. After completion, a later identical upload returns the saved report with zero new Java calls, including for an invalid report. After a timeout, concurrent identical re-uploads share one new attempt.
- [ ] Stage valid bytes with the engine offline or terminating. Verify the unavailable feedback explains that the ZIP remains staged and an explicit Publish validates again before installation. Restore the engine and upload identical bytes. Verify one Java validation call and a completed report on the same revision with its original actor and staging time.
- [ ] Restart Go after the staging revision commits and before the validation report is saved. Re-upload identical bytes and verify validation completes on the same revision with its original actor and staging time.
- [ ] Use the variable corpus materialized by [PR 2.1.1](PR-2.1.1-console-home.md) from the same Framework commit as the Console fixtures. Run Go discovery against every case. Compare ordered distinct names and every ordered occurrence (name, source and path) with `expected.json`, or the discovery failure reason and ordered locations where specified. For the `collision` case, verify successful discovery. Its `key-collision` arises only when Java substitutes the supplied value during installation.
- [ ] Starting with a staged revision and saved feedback, upload an otherwise acceptable ZIP containing the `invalid-blank` marker. Verify 400 with reason `blank-name` and the marker’s pack-relative source and authored YAML path, zero Java calls, and unchanged revision and feedback. Change the marker to a valid name with no application value and upload again. Verify 200 with a new revision, a Java validation call and, when validation completes, the missing occurrence in `missingVariables`.
- [ ] Stage an otherwise acceptable ZIP with repeated references to unset variables and an independent Java validation finding. Verify Java validation runs, the revision stays staged, and its completed feedback includes every missing occurrence at its authored location together with the Java finding. Keep the reference-free pack for the successful Phase 1 walkthrough.

## Handoff and deferred work

Use the storage, engine and access boundaries from [PR 2.1.2](PR-2.1.2-backend-readiness.md), [PR 2.1.3](PR-2.1.3-engine-observation.md) and [PR 2.1.4](PR-2.1.4-owner-applications.md). Give [PR 2.1.6](PR-2.1.6-first-publication.md) the selected-revision lookup, ZIP/digest, row-lock convention, shared validation routine and effective-settings builder. Give [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the portable sample and repeatable REST upload. [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns saved variables, revalidation, retained history/download and historical-revision publication; [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns MCP pack transfer and promotion; [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns trimming.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Handles untrusted archive input, concurrent storage changes and a two-system validation contract.
- **Reassessment triggers:** Reassess reuse boundaries if existing Console code already handles ZIP inspection or diagnostic mapping.
