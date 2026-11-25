# PR 1.5 Code Review — Cycle 3

## Scope and Repository State

Full pipeline Step 5 against cumulative base `6cf3d4c` on `main`. The index was empty. Reviewed tracked and untracked invocation code and tests, application grants migration, the K3s probe and deployment files, documentation, and the governing ticket and plans. Traced dispatch to the existing publication eligibility resolver and direct pod transport, and Java admission to the pinned Framework starter. Preserved untracked `.vscode/settings.json` separately.

## Findings

No actionable findings remain after the fix and internal re-review.

## Findings Resolved in This Context

### [P2] Preserve exact JSON decimal arguments at Framework admission

- Location. `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationController.java:38`.
- Scenario. Go correctly forwards a valid numeric argument, but the Java invocation reader materializes the fraction as a binary double before constructing the Framework input map.
- Impact. The executor receives a different number from the caller's argument. Framework's exact numeric schema validation also sees that altered number.
- Evidence. The new authenticated HTTP regression `InvocationControllerTest#preservesExactDecimalInputBeforeFrameworkAdmission` published a real probe executor. It returned `9.007199254740994E15` instead of the submitted value `9007199254740993.25` before the fix.
- Fix. Enable Jackson's BigDecimal handling on the invocation reader so finite decimal arguments reach Framework at their original precision. Keep the existing Jackson configuration for publication and all other endpoints. The focused regression now returns the exact expected number.

## Acceptance-Criteria and Plan Conformance

| Criterion | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Authorized caller, exact skill and result, role isolation, complete JSON ceiling | `service.go:95`, `postgres.go:30`, `InvocationController.java:44`, `InvocationService.java:27` | `TestInvokeAuthorization`, `TestInvokeJSONBodyLimit`, `InvocationControllerTest`, disposable PostgreSQL grants and restored invocation | Implemented |
| Off/on visibility for real success and failure, pre-session null with issues, exact diagnostics | `response.go:27`, `InvocationController.java:71`, `InvocationService.java:38`, `002_invocation.sql` | `TestInvokeDiagnosticProjection`, `InvocationControllerTest`, deployed diagnostic matrix under `NEVER` | Implemented |
| Temporary unavailability, confirmed authenticated admission rejection, unknown outcomes, no replay and continued admitted work | `service.go:201`, `InvocationAdmissionRejection.java:8`, `InvocationExceptionResolver.java:21` | `TestInvokeOutcome`, Java in-transit shutdown, running-provider failure and cancellation checks | Implemented |
| First authenticated publish and refresh, rejected replacement, durable acknowledgment before observation and first invocation on replacement | `k3s_test.go:33`, `milestone_transport_test.go`, `deploy/k3s/milestone/run.ps1` | Independent deployed scenario. The Java fixture separately proves `first/ping`. | Implemented |
| Original reachable generation for admitted work, replacement for in-transit and new calls | `InvocationService.java:38` and Framework generation capture | `InvocationCutoverTest` uses real mTLS invocation, publication and a loopback planner barrier | Implemented |
| Executable Java JAR, cumulative Go build and focused deployed milestone pass | Maven project, Go module and disposable milestone runner | Verification below | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

None requiring a developer decision. The Java 503 classifier uses the pinned starter’s two specific rejection origins because this starter has no public typed pre-session rejection signal. The real closure test guards the current integration.

## Verification Results

- FAIL before fix — `mvn --offline --batch-mode "-Dmaven.repo.local=$head" "-Dmaven.repo.local.tail=$tail" '-Dtest=InvocationControllerTest#preservesExactDecimalInputBeforeFrameworkAdmission' test` — reproduced numeric rounding through the published executor.
- PASS — `mvn --offline --batch-mode "-Dmaven.repo.local=$head" "-Dmaven.repo.local.tail=$tail" '-Dtest=InvocationControllerTest,InvocationCutoverTest' test` — 8 focused Java tests passed.
- PASS — `mvn --offline --batch-mode "-Dmaven.repo.local=$head" "-Dmaven.repo.local.tail=$tail" verify` from `execution-engine/`, using JDK 21.0.2, Maven 3.9.11 and the isolated Framework starter. The 63 cumulative tests passed. Inspected the Boot JAR entries and executable launcher manifest.
- PASS — `go test ./internal/invocation -run TestInvoke -count=1`, `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`, `go test -tags=k3s_integration ./internal/invocation -run '^$'`.
- PASS — `kubectl kustomize deploy/k3s`, `git diff --check 6cf3d4c` and `git diff --cached --check`.
- PASS — `& ./deploy/k3s/milestone/run.ps1 -Kubeconfig execution-engine/target/pr1-k3s/kubeconfig.yaml -Context k3d-loomspan-cloud-pr1-cg1`. The independently observed probe passed in 44.50 seconds. Its first refresh confirmed publication `28e94538c276a1e0a82fa01a7426807a` on JVM `cf55f91d-65ce-42da-92eb-b8ea03b15e4e` before any invocation. The probe restored durable publication `d1e92a1d4298c038ba2c85ea529d6958` on replacement JVM `8f7af9b4-f65a-4d3a-a3df-3fdc5cb374f9`. It counted zero premature invocation dispatches, received exactly `{"result":"restored/ping"}`, and verified the diagnostic matrix and pre-session null view under `NEVER`.

## Residual Risks and Optional Developer Checks

None for this ticket. A future Framework snapshot change may require retesting the precise shutdown-admission signals.

## Disposition

`fixes-applied`. The targeted decimal regression and cumulative checks passed after the Java reader change. A fresh reviewer is required.
