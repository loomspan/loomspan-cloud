# Private Framework Generation Code Review — Cycle 3

## Scope and Repository State

Reviewed the Java publication changes against `d8f128d`. The scope includes the tracked ZIP adapter change and the untracked publication controller, MVC resolver, metadata parser, configuration composition, service, tenant Vertex policy, and their tests. The staging area had no changes. Concurrent Go, deployment, future-ticket and IDE changes are outside this review.

## Findings

No actionable findings.

The controller reads the original pack through the existing bounded adapter. The metadata parser rejects missing or mistyped fields before validation or preparation. YAML composition keeps the authored node graph and adds only administrator-supplied settings. The authored credential check reads relevant field slices without resolving variables. The effective check binds the substituted connection set without constructing clients and rejects unsafe credentials before preparation, while Framework receives the complete explicit map for skills and configuration. Validation does not activate a candidate. Publication acknowledges only after Framework activation and closes a prepared candidate if publication fails.

## Findings Resolved in This Context

None. No implementation artifacts changed in this review.

## Acceptance-Criteria and Plan Conformance

Java source anchors below are under `execution-engine/src/main/java/ai/loomspan/cloud/engine/`. The referenced test classes are under the corresponding test package.

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Completed reports preserve source paths, report independent issues, and keep the serving generation active. Operational failures return plain text. | `PublicationController.java:35`, `PublicationService.java:24`, `PublicationExceptionResolver.java:26` | `PublicationControllerTest` checks warnings, skill shape, and failure messages. `PublicationGenerationTest.validationKeepsServingAndCollectsIndependentAuthoredAndSkillIssues` checks real source paths and generation state. | implemented |
| Both endpoints require the complete runtime settings. Publication requires the exact nonblank value map, including an empty map for a reference-free pack. | `PublicationMetadata.java:23`, `PublicationConfiguration.java:26`, `PublicationService.java:35` | `PublicationControllerTest` checks both routes and all required runtime fields and types, plus the empty-map case. `PublicationGenerationTest` shows missing and extra variables fail even with a host value. | implemented |
| Preserve worked variable semantics, tenant isolation, effective collisions and same-version behavior. | `PublicationZipAdapter.java:51`, `PublicationConfiguration.java:26`, `TenantVertexPolicy.java:124`, `PublicationService.java:35` | `PublicationGenerationTest` covers exact worked values, aliases and complex keys, the literal placeholder, collision preservation, and latched work across same-version rotation. | implemented |
| Check authored and effective Vertex credentials, including variable-selected policy, before any provider client is constructed. | `TenantVertexPolicy.java:35` and `:124`; `PublicationService.java:35` | `TenantVertexPolicyTest` covers literal and variable credentials, synthetic host ADC, file and URI options, malformed neighbors, and Gemini API-key mode. `PublicationControllerTest.rejectsIneffectiveCredentialsBeforePreparingEveryInstall` checks the reloader boundary. | implemented |
| Acknowledge successful publication with an empty 204 and preserve private namespace/version semantics, including an installed zero-skill namespace. | `PublicationController.java:41`, `PublicationService.java:35` | `PublicationControllerTest` checks the empty response and publish sequencing. `PublicationGenerationTest` checks private visibility, exact version, fresh generation ID, and zero-skill replacement. | implemented |
| Reject failed preparation and publication with the original failure message and keep the prior generation serving. | `PublicationExceptionResolver.java:26`, `PublicationService.java:35` | Controller tests exercise verbatim exception responses and candidate closure. Real generation tests verify the old generation ID, version, and invocation after rejected replacements. | implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

None affecting this ticket. All Go install origins use the same Java publish route. Their deployed lifecycle verification belongs to the later milestone tickets.

## Verification Results

- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest,TenantVertexPolicyTest,PublicationGenerationTest' test` — 20 tests passed.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — 52 tests passed and Maven packaged the executable Spring Boot JAR with the shared cached snapshot.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$((Get-Location).Path)/target/pr1-framework/repository" "-Dmaven.repo.local.tail=$env:USERPROFILE/.m2/repository" verify` — 52 tests passed and Maven packaged the executable Spring Boot JAR with the isolated rebuilt snapshot.
- PASS — `git diff --check d8f128d -- execution-engine` and `git diff --cached --check`.

The Maven commands ran from `execution-engine/` with JDK 21. The tests used local loopback, synthetic credentials, and the real Framework where appropriate.

## Residual Risks and Optional Developer Checks

Effective credential preflight depends on the Framework snapshot's internal typed parser. Both available snapshots passed the contract and integration tests. None of this ticket's checks requires a configured external service.

## Disposition

`clean`
