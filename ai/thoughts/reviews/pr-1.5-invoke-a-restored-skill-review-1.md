# PR 1.5 Invoke a Restored Skill Code Review — Cycle 1

## Scope and Repository State

Reviewed the cumulative Cloud checkout against local commit `6cf3d4c`, with an empty Git index. The scope included the tracked deployment and TLS test edits and the untracked Java endpoint, Go service and database migration, and disposable K3s scenario. The unrelated `.vscode/settings.json` was outside the review scope. I traced the production Go transport and publication placement, Java's Framework boundary, and the tagged probe's publication, observation, and invocation barriers.

## Findings

No actionable findings remain after the fix below.

## Findings Resolved in This Context

### [P2] Reject malformed UTF-8 before decoding invocation identity

- Location: `management-backend/internal/invocation/service.go:128`.
- Scenario: An authenticated backend with an application grant submits a JSON body containing an invalid UTF-8 byte inside its asserted end-user ID or role.
- Impact: `encoding/json` replaces the invalid byte with U+FFFD. The service accepts the resulting nonblank string and sends an invocation with an identity or role different from the bytes the backend supplied.
- Evidence: The new loopback TLS authorization case failed before the fix with `malformed backend assertion = <nil>`. Its successful path indicated dispatch rather than `ErrInvalidRequest`.
- Fix: Validate the complete bounded body with `utf8.Valid` before unmarshaling. Regressions now verify both an invalid ID byte and an invalid role byte reject without increasing the TLS dispatch count.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Executable evidence | Result |
| --- | --- | --- | --- |
| Authorized identity and exact application roles, invalid caller rejected, exact Go body ceiling | `service.go:94`, `service.go:128`, `service.go:147`, `postgres.go:30`, `InvocationService.java:27` | `TestInvokeAuthorization`, `TestInvokeJSONBodyLimit`, `InvocationControllerTest`, real PostgreSQL grants in `TestK3sRestoredSkill` | Implemented |
| Diagnostic modes under NEVER, real skill error, and null-view fallback | `InvocationController.java:43`, `InvocationService.java:27`, `response.go:27`, lookup on every call | `TestInvokeDiagnosticProjection`, `InvocationControllerTest`, live K3s four-case matrix and pre-session validation | Implemented |
| Unavailable before dispatch, unknown after loss or unverified response, authenticated pre-execution 503 and no replay | `service.go:171`, `response.go:27`, `InvocationAdmissionRejection.java:11` | `TestInvokeOutcome`; real close-admission, provider 503 and cancellation tests in `InvocationControllerTest` | Implemented |
| First snapshot and publish, invalid replacement leaves original version active, confirmed replacement before first restored invoke | `k3s_test.go:33`, `milestone_transport_test.go`, `deploy/k3s/milestone/run.ps1` | Real K3s run with authenticated per-pod transport and PostgreSQL. `originalPackReturnsFirstPing` separately proves the first pack. | Implemented |
| Already-admitted parent and child retain original generation, in-transit and new requests use current generation | Java passes namespace, local skill and inputs to ordinary `SkillTemplate.invoke`. Go sends no generation token. | `InvocationCutoverTest#onlyAdmittedRequestRetainsItsReachableChildAcrossPublication` | Implemented |
| Executable Maven JAR, cumulative Go build and live K3s | `execution-engine/pom.xml`, Go module, and disposable runner | Commands and live values below | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

The admission-origin check is specific to the Framework starter snapshot whose two pre-session rejection sites use wrapped `RejectedExecutionException`. The authenticated HTTP tests exercise admission closure and rule out a started provider failure as a 503 source.

## Verification Results

- PASS — JDK 21.0.2 / Maven 3.9.11 `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$reviewFramework" "-Dmaven.repo.local.tail=$env:USERPROFILE/.m2/repository" '-Dtest=InvocationControllerTest,InvocationCutoverTest' test` and the same command with `verify` from `execution-engine`, with `$reviewFramework` set to the isolated `target/pr1-framework/repository`. The full Maven run passed all 62 tests. The packaged JAR contains the invocation controller, application, and `JarLauncher`, which is also the manifest main class.
- FAIL before the fix — `go test ./internal/invocation -run '^TestInvokeAuthorization$' -count=1` reproduced acceptance of the malformed assertion.
- PASS after the fix — `go test ./internal/invocation -run TestInvoke -count=1`, `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`, and `go test -tags=k3s_integration ./internal/invocation -run '^$'` from `management-backend`.
- PASS — `kubectl kustomize deploy/k3s`, `git diff --check 6cf3d4c`, and `git diff --cached --check`.
- PASS — `& ./deploy/k3s/milestone/run.ps1 -Kubeconfig execution-engine/target/pr1-k3s/kubeconfig.yaml -Context k3d-loomspan-cloud-pr1-cg1` after the fix. The runner verified the named isolated single-node cluster and the sole old pod, recreated only scenario data there, and completed `TestK3sRestoredSkill` in 43.61 seconds.

The first acknowledged version was `276685ecb44a04fe86b1b13643a3f730` on JVM `2cc54f0f-f301-4eb6-add3-ec393c413483`. The newer version `5ca86e32f4747b2e27b57dfe8e3993b6` was acknowledged over TLS and independently reopened from PostgreSQL with Console confirmation held. Replacement JVM `63aab24f-6feb-4ad3-8b4a-b3236c802d31` restored that version. There were zero invoke dispatches before confirmation. The first response was exactly `{"result":"restored/ping"}`. All four diagnostic cases and the pre-session null view passed with trace persistence `NEVER`.

## Residual Risks and Optional Developer Checks

No optional developer checks. The JSON milestone covers the shared invoke entry point. Transport adapters and multipart remain in their own ticket scope.

## Disposition

`fixes-applied`. A fresh Step 5 context must review the updated implementation.
