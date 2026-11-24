# PR 1.2 Code Review — Cycle 1

## Scope and Repository State

Compared the checkout with `d8f128d`, the PR 1.1 adapter commit. The review covered the staged, unstaged, and untracked PR 1.2 Java endpoint, policy, YAML adapter and composition, tests, and governing artifacts. Concurrent Go, deployment, and later-ticket process files remained outside this review.

## Findings

No actionable findings remain after the fixes below.

## Findings Resolved in This Context

### [P2] Keep independent authored connection diagnostics

- Location — `execution-engine/src/main/java/ai/loomspan/cloud/engine/TenantVertexPolicy.java:34`
- Scenario — A Gemini Vertex declaration omits its credential while a second, unrelated connection has duplicate mapping keys.
- Impact — Materializing all connections in one operation discarded the independently decidable tenant credential issue. Framework still reported the malformed candidate, but the credential omission was absent from the completed response.
- Evidence — `TenantVertexPolicyTest.reportsKnownVertexViolationAlongsideMalformedIndependentConnection` returned an empty issue list before the fix.
- Fix — Materialize authored connections separately, using the Framework slice's original path for each credential diagnostic. A malformed connection still reaches Framework's structural validation.

### [P2] Reject trailing data after service-account JSON

- Location — `execution-engine/src/main/java/ai/loomspan/cloud/engine/TenantVertexPolicy.java:127`
- Scenario — An otherwise parseable service-account object is followed by a second JSON document in the literal or supplied credential string.
- Impact — The Google credential reader consumed the first object and accepted content that was not a single JSON document. This allowed staged validation or publication preflight to accept an invalid credential field.
- Evidence — `TenantVertexPolicyTest.rejectsCredentialTextWithTrailingJson` produced no credential diagnostic before the fix. The regression checks both the authored literal and the effective variable value.
- Fix — Check that the credential text contains one JSON document before using the same service-account reader as the Framework runtime. Return the credential path and generic reason without including its value.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Completed and interrupted validation, independent findings, exact source paths, and unchanged serving state | `PublicationService.validate`, per-connection authored policy, and the publication exception resolver | Loopback warning and interrupted responses, real Framework callable and Vertex findings, independent-connection regression, and serving snapshot and invocation assertions | Implemented |
| Complete metadata and an exact value map | `PublicationMetadata.parse`, original ZIP adaptation, and versioned explicit-map `prepare` | Loopback path and wire-type matrix for both operations, reference-free `{}`, and real Framework missing, blank, and unused values with a competing host value | Implemented |
| Worked corpus, collision, literal handling, tenant isolation, and same-version replacement | Shared SnakeYAML loader, node composition, and Framework candidate lifecycle | Six occurrences, four distinct names, captured settings, separate private namespaces, literal placeholder, authored collision paths, alias and complex-key tests, and bounded concurrent invocation | Implemented |
| Authored and effective Vertex enforcement before client creation | `TenantVertexPolicy`, typed effective Framework parser, and policy call before `prepare` | Synthetic literal and variable JSON, separate JVM with synthetic ADC, variable-selected keys and mode, API-key mode, file and URI fields, invalid trailing JSON, and observed absence of prepare across version and restart requests | Implemented |
| Empty `204` after private versioned publication, new generation, and zero-skill replacement | `PublicationController.publish` and Framework activation | Loopback exact status and empty body, real private snapshots and distinct generation IDs, and a versioned empty namespace | Implemented |
| Failed preparation and publication return text and preserve the prior generation | `PublicationExceptionResolver`, try-with-resources around the detached candidate, and Framework publication | Exact loopback exception message, closed detached candidate, rejected real preparation and simulated pre-activation failure with unchanged active invocation | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

- Both pinned local starter builds expose the same effective configuration parser semantics. Re-review that internal dependency when changing the Framework starter version.

## Verification Results

Maven commands were run from `execution-engine/` with JDK 21.

- FAIL before the fix — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=TenantVertexPolicyTest#reportsKnownVertexViolationAlongsideMalformedIndependentConnection' test` — reproduced the missing independent issue.
- FAIL before the fix — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=TenantVertexPolicyTest#rejectsCredentialTextWithTrailingJson' test` — reproduced acceptance of a second JSON document.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest,TenantVertexPolicyTest,PublicationGenerationTest' test` — all 19 focused tests passed after both fixes.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — all 51 tests passed with the cached Framework snapshot and the executable Boot JAR was repackaged.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$((Get-Location).Path)/target/pr1-framework/repository" "-Dmaven.repo.local.tail=$env:USERPROFILE/.m2/repository" verify` — all 51 tests passed with the isolated Framework snapshot from `2ca9caf` and the executable Boot JAR was repackaged.
- PASS — `git diff --check -- execution-engine` and `git diff --cached --check`.

## Residual Risks and Optional Developer Checks

K3s transport and execution on a recovered engine are covered by the later milestone tickets. Credential coverage used synthetic data and did not need external provider access. No optional checks for PR 1.2.

## Disposition

`fixes-applied`. A fresh reviewer should verify the updated policy and its regressions.
