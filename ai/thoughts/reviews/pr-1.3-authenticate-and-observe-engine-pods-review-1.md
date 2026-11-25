# PR 1.3 Authentication and Observation Code Review — Cycle 1

## Scope and Repository State

Reviewed the PR 1.3 changes against `3d1f78536ebdc5fd683baeff8e6e714cc63f36d4`. The review included the untracked Java TLS filter, K3s profile, Java tests and image files, the Go observer, pod watch, direct transport, entry point and tests, and the K3s manifests and observation probe. PR 1.4 publication and migration work, subsequent ticket artifacts, and `.vscode` were outside this scope. The publication package used its temporary nested module during the Go verification.

## Findings

No open actionable findings after re-review.

## Findings Resolved in This Context

### [P2] Confirm the original route before testing its invalidation

- Location `management-backend/internal/engine/k3s_test.go:91` and `deploy/k3s/README.md:49`.
- Scenario. The replacement checkpoint previously required activity admission after refresh. A pod that never became ready could reach that checkpoint and pass replacement invalidation, even though its publication never qualified for routing.
- Impact. The deployed probe could miss a readiness or eligibility regression in the K3s watch-to-router path.
- Evidence. The original probe checked that `Confirms` was false during replacement and had no prior assertion that `Confirms` was true. The local observer test supplies simulated readiness and cannot detect a deployed readiness watch regression by itself.
- Fix. The replacement checkpoint now requires `Confirms` for the fixture version while the original pod UID and JVM ID still match. It also checks the generation and trace policy obtained by explicit refresh. The deployment guide documents the stronger checkpoint.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Authenticated internal requests and first Console snapshot in an empty JVM | `EngineClientIdentityConfiguration.java:24` requires the authenticated management URI SAN. `application-k3s.properties:3` requires the client certificate at the connector. The Console keeps its independent key check. Go `transport.go:76` verifies the configured server DNS identity while dialing the watched Pod IP. | Java TLS integration covers absent, untrusted, and wrong URI certificates, forwarded headers, async and forward dispatch, wrong Console key, and a real empty snapshot. `TestTransport` checks wrong CA and DNS on both request paths. The K3s probe performs a real publish from its first observation. | Implemented |
| Cert-manager issuance, projected credential rotation, key Secret, and restricted ingress | `deploy/k3s/certificates.yaml:1` defines the certificate usages and renewal settings. `deploy/k3s/network-policy.yaml:14` combines both namespace and pod selectors on port 8443. The deployments use full read-only Secret mounts and reference the same key entry in environment-local namespace Secrets. | Java reload test verifies a renewed server leaf without a JVM restart. Go transport test verifies a new client handshake uses the changed leaf. K3s checks issued Secrets and positive and negative TCP controls. | Implemented |
| Pod and JVM identity, readiness, termination, and refreshed application policy | `pod_watch.go:111` associates list and watch using the list resource version. `console.go:78` validates the snapshot identity and namespace tuples. `observer.go:147` makes the authenticated snapshot available before SSE admission and `observer.go:195` serializes refresh work. | Pod watch protocol tests cover readiness and termination, and observer tests cover first publication without readiness and application policy independent of the default. The deployed probe checks and refreshes the fixture tuple before routing. | Implemented |
| Disconnect, reconnect, restart, and replacement invalidate stale routing | `observer.go:104` rejects a superseded pod target. `observer.go:195` invalidates the connection on a changed JVM and `observer.go:243` retains the last snapshot while closing eligibility. `observer.go:27` requires the current ready and non-terminating pod to confirm the version. | Observer tests cover pause across disconnect, same-UID restart, stale refresh, reused pod name and address, and serialized refresh. The K3s probe requires an eligible original route and observes a new pod UID and JVM ID after replacement. | Implemented |

## Active Project Guardrails

None recorded. The Java trust check uses the servlet's authenticated certificate identity. The Go client disables redirects, confines requests to the selected direct HTTPS endpoint, and supplies the Console key only through the Console client. Production logs contain connection diagnostics without credential or HTTP body output.

## Open Questions and Assumptions

The review used the isolated Framework snapshot containing per-namespace trace policy, as required by the deployment guide. The dev cluster had a dedicated transport CA and fixture-only credentials. No developer decision remains.

## Verification Results

- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$repoHead" "-Dmaven.repo.local.tail=$repoTail" '-Dtest=EngineClientIdentityTest' test` in `execution-engine/`. Three tests passed.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$repoHead" "-Dmaven.repo.local.tail=$repoTail" verify` in `execution-engine/`. All 55 tests passed and Maven repackaged the executable JAR. `$repoHead` referred to `execution-engine/target/pr1-framework/repository`, and `$repoTail` to the user's normal Maven repository.
- PASS — `go test ./internal/engine -run 'Test(Transport|PodWatch|Console|Observer)'`, `go test ./...`, `go test -race ./...`, `go build ./...`, `go test -tags=k3s_integration ./internal/engine -run '^$'`, and `go vet ./...` from `management-backend/`.
- PASS — `kubectl kustomize deploy/k3s` rendered the expected certificate, Secret mount references, RBAC, deployment, and NetworkPolicy manifests.
- PASS — `docker build -t loomspan-execution-engine:local execution-engine`, `docker build --target backend -t loomspan-management-backend:local management-backend`, and `docker build --target observation-probe -t loomspan-observation-probe:local management-backend`. Imported the verification images with `execution-engine/target/pr1-k3s/k3d.exe image import` and the explicit `--cluster loomspan-cloud-pr1-cg1` option.
- PASS — `& execution-engine/target/pr13-draft/verify-observation.ps1` with only the disposable `k3d-loomspan-cloud-pr1-cg1` context and its dedicated kubeconfig. Both certificates and Secret issuance annotations verified. The management peer reached port 8443, both negative peers were blocked, and engine-local clientless TLS was rejected. The Job exited successfully after checking the independent Console key, the first authenticated snapshot, fixture publish and refresh, and stale route invalidation. It logged `ready=true` for the original routed `probe-version-1` before replacement.
- PASS — `git diff --check`, `git diff --cached --check`, and `gofmt -l management-backend/internal/engine management-backend/cmd/management-backend`.

| Observation | Kubernetes pod UID | Authenticated JVM ID |
| --- | --- | --- |
| Original, with route confirmed | `86e0d6a8-ea6b-4c03-a61b-29731962e783` | `e48f475b-a659-4577-b905-83f2363618a0` |
| Replacement, after invalidation | `5a8a0229-6f7f-49ec-a085-fc9411c353f8` | `f157f967-7750-46ba-980d-d893457b7004` |

## Residual Risks and Optional Developer Checks

No optional checks for PR 1.3. The explicit deployment scope is the disposable K3s configuration. The cumulative milestone has its separate restoration and invocation checks.

## Disposition

`fixes-applied`. The deployed probe and its guide changed. The review found no remaining actionable issues after verifying the strengthened probe. A fresh independent review is required.
