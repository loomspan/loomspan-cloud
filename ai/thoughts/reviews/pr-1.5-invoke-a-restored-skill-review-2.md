# PR 1.5 Code Review — Cycle 2

## Scope and Repository State

Reviewed the cumulative checkout against `6cf3d4c`. Scope covered the Java invocation endpoint and its Framework integration, the Go invocation service and grant migration, the deployed recovery probe, and accompanying tests and documentation. Reviewed both unstaged tracked changes and untracked implementation files. The index had no staged changes. The untracked `.vscode/settings.json` was outside scope.

## Findings

No actionable findings remain after the fix below.

## Findings Resolved in This Context

### [P2] Reject unpaired surrogate escapes in backend caller assertions

- Location. `management-backend/internal/invocation/service.go:148`.
- Scenario. An application-authorized backend supplies a subject or role with an unpaired escaped UTF-16 surrogate. The complete request contains valid UTF-8 bytes and passes the request syntax check. Go's standard JSON decoder then replaces the unpaired surrogate with U+FFFD before marshaling the authenticated engine request.
- Impact. The engine can execute with an altered backend-supplied identity. A malformed assertion and an assertion containing a legitimate replacement character can yield the same principal.
- Evidence. A TLS `TestInvokeAuthorization` assertion with a lone escaped high surrogate failed before the fix because the service returned success instead of `ErrInvalidRequest`.
- Fix. Check Unicode escapes on the syntactically validated raw backend assertion before decoding. Reject lone high and low surrogates. The focused test also verifies that a valid surrogate pair and literal backslash text arrive unchanged, and checks the exact dispatch count.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Authorized caller, exact identity and result, invalid metadata denied, inclusive body ceiling | Go `InvokeJSON`, application-scoped `PostgresGrants`, Java `InvocationController` and `InvocationService` | `TestInvokeAuthorization`, `TestInvokeJSONBodyLimit`, `InvocationControllerTest`, tagged K3s PostgreSQL grant checks | Implemented |
| Diagnostics off/on with `NEVER`, real skill failure, null view before a session | Java overload and observer selection, Go `projectResponse` | `TestInvokeDiagnosticProjection`, real Java error and validation tests, deployed diagnostic matrix | Implemented |
| Unavailable before dispatch, authenticated 503 distinguished from other outcomes, no replay | Go placement recheck and direct TLS provenance, Java `InvocationAdmissionRejection` | `TestInvokeOutcome`, closed-admission HTTP test, started provider failure and caller-disconnect tests | Implemented |
| First publication, failed replacement preserves it, durable later version before observation, restored version routes only after confirmation | `k3s_test.go`, test-only snapshot barriers in `milestone_transport_test.go`, scoped runner | Actual tagged K3s run and separate Java `originalPackReturnsFirstPing` assertion | Implemented |
| Old root and child keep generation, in-transit and later requests use replacement | Java calls ordinary `SkillTemplate.invoke` with separate namespace and local name | `InvocationCutoverTest` uses real HTTP barriers around publication and a loopback planning response | Implemented |
| Executable boot artifact and cumulative Go build | Maven repackaging, Go packages and tagged probe | Offline Maven `verify`, executable JAR entry and manifest checks, Go build and K3s | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

The pinned Framework starter exposes its pre-session rejection through two identifiable exception origins. Any future starter change needs to preserve this signal or replace it with a supported typed equivalent.

## Verification Results

- FAIL before the fix — `go test ./internal/invocation -run '^TestInvokeAuthorization$' -count=1` reproduced the unpaired-surrogate dispatch.
- PASS after the fix — `go test ./internal/invocation -run TestInvoke -count=1`.
- PASS — `go test ./... -count=1`, `go test -race ./...`, `go build ./...`, `go vet ./...`, and `go test -tags=k3s_integration ./internal/invocation -run '^$'`.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$h" "-Dmaven.repo.local.tail=$t" '-Dtest=InvocationControllerTest,InvocationCutoverTest' test` and the same offline Maven command with `verify` in `execution-engine/`. `$h` points to `target/pr1-framework/repository` and `$t` to the user Maven cache. JDK 21.0.2 and Maven 3.9.11 passed seven focused and 62 cumulative tests. The repackaged JAR contains the launcher, application and invocation classes, and declares `JarLauncher` as its main class.
- PASS — `kubectl kustomize deploy/k3s`, `git diff --check 6cf3d4c`, and `git diff --cached --check`.
- PASS — `& ./deploy/k3s/milestone/run.ps1 -Kubeconfig execution-engine/target/pr1-k3s/kubeconfig.yaml -Context k3d-loomspan-cloud-pr1-cg1`. `TestK3sRestoredSkill` passed in 42.83 seconds. The first acknowledgment refreshed publication `042b7841ceb26d248dbff541cab5daf5` on JVM `59915440-f21b-4689-9556-b55206ac42ac`. After the rejected replacement, the probe held Console observation at the later durable acknowledgment and verified restore of `00d19c7b2e3363dced9de0927ac22714` on JVM `fd1e51df-9f8b-45cc-a1cf-ade01e946b5f`. It recorded zero premature invokes and exact first response `{"result":"restored/ping"}`. All four visibility combinations and the pre-session null view passed with trace retention `NEVER`.

## Residual Risks and Optional Developer Checks

None beyond the pinned starter dependency above.

## Disposition

`fixes-applied`. A fresh independent review is required because this cycle changed the Go assertion guard and its regression test.
