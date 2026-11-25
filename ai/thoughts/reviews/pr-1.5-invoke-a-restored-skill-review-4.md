# PR 1.5 restored skill invocation review, cycle 4

## Scope and Repository State

Full 5-Step Pipeline, Step 5, reviewed against cumulative base `6cf3d4c`. The tracked changes update the deployment guide and reuse the existing Java mTLS test credentials. The untracked implementation adds the Java invocation endpoint and Framework boundary, Go invocation and response handling, SQL migration, Java and Go tests, and the scoped K3s milestone harness. The index was empty. The unrelated `.vscode/settings.json` was excluded. The review used the ticket, its research and both plans, the current diff and sources, and the adjacent Framework lifecycle implementation.

## Findings

No actionable findings.

## Findings Resolved in This Context

None. This cycle added only this review artifact.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Executable evidence | Result |
| --- | --- | --- | --- |
| Authorized JSON invocation, application-scoped identity and roles, exact result, and inclusive 20 MiB body limit | `management-backend/internal/invocation/service.go:95`, `postgres.go:30`, `management-backend/migrations/002_invocation.sql`, `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationController.java:44`, and `InvocationService.java:27` | `TestInvokeAuthorization`, `TestInvokeJSONBodyLimit`, `InvocationControllerTest`, and the live probe's caller and grant assertions | Implemented |
| Per-call diagnostics override applies to both outcomes, with null view and complete available validation details | `response.go:27`, `InvocationController.java:44`, `InvocationService.java:27`, and the administrator setting in `002_invocation.sql` | `TestInvokeDiagnosticProjection`, Java observer and failure tests, and all four live combinations plus pre-session null view under `NEVER` | Implemented |
| Unavailable before dispatch, unknown after loss or an intermediary response, and engine-confirmed pre-session rejection | `service.go:201`, direct authenticated transport in `management-backend/internal/engine/transport.go:76`, `response.go:27`, and `InvocationAdmissionRejection.java:11` | `TestInvokeOutcome` and real Java admission closure, provider failure, and caller-disconnect tests | Implemented |
| First publication, denied replacement, later durable acknowledgment, replacement JVM restoration, and zero premature dispatches | `management-backend/internal/invocation/k3s_test.go:33`, test transport barriers, and `deploy/k3s/milestone/run.ps1` | Live `TestK3sRestoredSkill`, plus the separate `originalPackReturnsFirstPing` test | Implemented |
| Admitted root and child retain their generation across activation, while in-transit and fresh invocations pick the replacement | `InvocationController.java:44` invokes Framework once via `InvocationService.java:27` without a generation token | `InvocationCutoverTest#onlyAdmittedRequestRetainsItsReachableChildAcrossPublication` | Implemented |
| Executable Boot JAR and cumulative Go and K3s verification | Module build and reviewed milestone runner | Maven, JAR inspection, Go checks, and the live dedicated-cluster run below | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

None.

## Verification Results

- PASS — In `execution-engine/`, `$env:JAVA_HOME='C:/hamdev/jbrsdk21'; $cfg=(Resolve-Path 'target/pr1-framework/repository').Path; $tail=Join-Path $env:USERPROFILE '.m2/repository'` selected JDK 21.0.2 and the isolated starter. `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$cfg" "-Dmaven.repo.local.tail=$tail" '-Dtest=InvocationControllerTest,InvocationCutoverTest' test` passed 8 tests.
- PASS — With that same setup, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$cfg" "-Dmaven.repo.local.tail=$tail" verify` passed 63 tests. Inspection of the generated `loomspan-cloud-execution-engine-0.1.0-SNAPSHOT.jar` confirmed the Boot launcher, application, invocation controller, and executable manifest. The nested Framework starter SHA-256 matched the isolated snapshot.
- PASS — In `management-backend/`, `go test ./internal/invocation -run TestInvoke -count=1`, `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`, and `go test -tags=k3s_integration ./internal/invocation -run '^$'`.
- PASS — `kubectl kustomize deploy/k3s`, `git diff --check 6cf3d4c`, and `git diff --cached --check`. The invocation Go files passed `gofmt -l` inspection and the PowerShell runner parsed without errors.
- PASS — `& ./deploy/k3s/milestone/run.ps1 -Kubeconfig execution-engine/target/pr1-k3s/kubeconfig.yaml -Context k3d-loomspan-cloud-pr1-cg1`. The live probe passed in 43.58 seconds. The first acknowledgment refreshed publication `fcf28ddb881e5d20cd52d54d5c5eb8b4` on the original JVM `2f985557-3753-4b51-9ded-88414c1997c3`. The replacement JVM `586e29b2-c1bc-456d-84b8-a6d451cb4e73` restored durable publication `04d5647a7f15d1d1744986fbe663f013`. The probe recorded zero invoke dispatches before confirmation. The first response was exactly `{"result":"restored/ping"}`. All four diagnostic combinations and the pre-session null view passed under `NEVER`.

## Residual Risks and Optional Developer Checks

The confirmed pre-session 503 classifier depends on the pinned Framework starter's rejection sites. The Java loopback test exercises the actual shutdown path. No optional developer checks remain.

## Disposition

`clean`
