# Private Framework Generation Code Review — Cycle 2

## Scope and Repository State

Reviewed PR 1.2 from base `d8f128d`. The tracked adapter diff and untracked Java publication handlers, policy, configuration, and tests were in scope. The ticket, research, and plans supplied the intended behavior. Concurrent Go, deployment, editor settings, and other ticket artifacts were outside scope.

## Findings

No open actionable findings after the fix below.

## Findings Resolved in This Context

### [P2] Keep known Vertex findings when unrelated fields of the same connection are malformed

The authored policy materialized an entire connection before reading its driver and Gemini fields. A repeated header within that connection caused materialization to throw and discard a separate, decidable missing Vertex credential finding. A repeated `project-id` similarly obscured the independent check. The effective publish policy still rejected malformed candidates before constructing a client. The affected response was the completed staged-validation report.

`TenantVertexPolicy.java:59` now reads the driver and Gemini branches through Framework's authored slices, and materializes only policy fields. It defers ambiguous repeated policy fields and variable-dependent decisions to their existing Framework or effective checks. The new regression at `TenantVertexPolicyTest.java:76` failed on the previous code and passes for malformed headers and repeated project IDs on the same connection. The existing unresolved credential and field-name tests still pass.

## Acceptance-Criteria and Plan Conformance

| Criterion | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Completed reports preserve warnings, skill sources, and independently decidable issues. Interrupted checks return non-success text. Validation does not activate a candidate. | `PublicationService.validate`, `TenantVertexPolicy.checkAuthored`, `PublicationExceptionResolver` | Controller report and interruption tests, real validation with missing Vertex credentials and an unrelated skill reference, same-connection policy regression, and the serving-generation assertion | Implemented |
| Both endpoints require runtime metadata with valid types and precise offending paths. Publish requires an exact explicit variable map and allows an empty map. | `PublicationController.readPublication`, `PublicationMetadata.parse`, `PublicationConfiguration.compose`, private explicit-map reloader call | Loopback multipart tests for both endpoints and each runtime field, version and variable metadata tests, real missing, blank, and extra map tests | Implemented |
| Worked substitutions, exact literal values, namespace isolation, key collision, and in-flight same-version rotation obey Framework semantics. | Structural configuration composition, execution-only preflight projection, and full map passed to Framework preparation | Real two-namespace worked corpus with a distinct host value, literal placeholder and alias checks, collision rejection, and captured-versus-new invocation checks | Implemented |
| Authored and effective Vertex policy covers explicit service-account JSON, variable-selected fields, ADC omission, and other Gemini mode. Every install enters pre-client policy. | `TenantVertexPolicy` and `PublicationService.publish` | Synthetic literal, variable, wrong JSON, file and URI, API-key, and host-ADC fixtures. HTTP install tests prove the reloader is untouched on policy failure across same-version and recovery-shaped requests. | Implemented |
| Successful publication returns empty `204`, captures runtime settings, preserves private visibility, and creates distinct generation IDs across versions and same-version replacement. An empty pack leaves the namespace with no YAML skills. | `PublicationController.publish` and private versioned `prepare` in `PublicationService.publish` | Loopback acknowledgement test and real Framework namespace, settings, opaque version, replacement ID, and empty-generation assertions | Implemented |
| Rejected preparation and publication failures preserve the previous generation and return the original message as text. | Detached update lifetime in `PublicationService.publish` and publication-scoped exception resolver | Real invalid replacements and an injected exception after successful detached preparation, together with exact loopback text assertions | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

The review covered the installed and locally refreshed Framework snapshots. The current starter exposes no supported pre-client connection hook, so authored field inspection and effective binding rely on internal Framework interfaces. The tests exercise this seam. Pod authentication and external recovery remain scoped to later tickets.

## Verification Results

- FAIL — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=TenantVertexPolicyTest#reportsKnownVertexViolationAlongsideMalformedFieldsOfTheSameConnection' test` — reproduced the missing issue before the fix.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest,TenantVertexPolicyTest,PublicationGenerationTest' test` — 20 tests passed after the fix.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — 52 tests passed and Maven repackaged the executable JAR.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$((Get-Location).Path)/target/pr1-framework/repository" "-Dmaven.repo.local.tail=$env:USERPROFILE/.m2/repository" verify` — 52 tests passed and Maven repackaged the executable JAR against Framework `2ca9caf`.
- PASS — `git diff --check d8f128d -- execution-engine` and `git diff --cached --check`.

## Residual Risks and Optional Developer Checks

No optional developer checks. Routine tests use loopback connections, synthetic credentials, and a local executor fixture.

## Disposition

`fixes-applied`. A fresh review should verify the final patch.
