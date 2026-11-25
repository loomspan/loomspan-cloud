# PR 1.3 code review — cycle 2

## Scope and Repository State

- Full pipeline review of the current checkout against `3d1f785`. The ticket changes were untracked at the review boundary. The staged and unstaged tracked diffs were empty.
- Scope included the Java transport profile, servlet filter and tests, both container builds and their Docker ignore files, the Go observer, transport, watch and command, deployment and observation fixtures, and the ticket's research and plans. Unrelated IDE settings, publication work and later ticket artifacts were excluded.
- Traced the authenticated dispatch through the Boot TLS connector and Framework key filter. Compared Go snapshot and stream handling to the Framework DTOs, authenticated headers, cursor, and frame format. Traced candidate selection, termination, disconnect and refresh through the observer and its callers.

## Findings

No actionable findings.

## Findings Resolved in This Context

None. This review changed only this review document.

## Acceptance-Criteria and Plan Conformance

| Criterion | Code evidence | Test and deployed evidence | Result |
| --- | --- | --- | --- |
| Mutual identity and first authenticated snapshot | `application-k3s.properties:5` requires TLS client authentication. `EngineClientIdentityConfiguration.java:24` checks the trusted leaf URI SAN before the Framework key filter. `transport.go:76` dials the selected Pod IP using explicit roots and the engine DNS SAN. `observer.go:147` exposes the first snapshot before waiting for activity admission. | Java loopback rejects missing and untrusted certificates, trusted wrong identities, and bad Console keys. Go loopback rejects wrong engine credentials on both route families without dispatch. `TestObserverRefreshProjectsNamespacePolicy` holds stream admission and marks the pod unready while still obtaining a publication target. In K3s, the initial snapshot had no Cloud publications and the Go fixture publish reached the actual Java handler. | Implemented |
| Certificate lifecycle, matching Console Secrets, and ingress | `certificates.yaml` issues the two correctly scoped identities with rotation enabled. Both workload definitions mount the full Secret and reference their namespace's `loomspan-console` key. `network-policy.yaml:14` combines the namespace and workload label selectors in the same peer. `rbac.yaml:15` limits Go to pod list and watch in the engine namespace. | Render and deployed certificate checks confirmed ownership annotations and readiness. Authenticated publish and the key checks exercised both workloads. Go and Java loopback independently verify renewed leaf use. The K3s allow control and both deny controls passed, as did rejection without a client certificate from within the engine pod. | Implemented |
| Pod and JVM identity, refresh, and per-generation policy | `pod_watch.go:111` processes resource-versioned list and watch events. `console.go:78` binds the body UUID to the authenticated response header and preserves each namespace tuple. `observer.go:195` serializes refreshes and checks the current session before applying results. `observer.go:79` returns a copy of the last observation. | Go tests cover empty state, opaque versions, distinct policy, readiness and termination, serialized refresh, and mutation of returned data. The K3s explicit refresh confirmed `probe-version-1`, namespace policy `NEVER`, and default `ONERROR` on the original JVM. | Implemented |
| Disconnect, reconnect, JVM restart, and replacement | `observer.go:104` cancels superseded watch targets even with the same name and address, and `observer.go:243` removes eligibility while retaining the snapshot. Refresh and stream handling require the same current JVM identity. | `TestObserverRetainsDisconnectedObservation` requires a fresh snapshot before routing resumes on reconnect. `TestObserverRejectsSupersededWork` covers a changed JVM ID under the same pod UID and a delayed old result across pod replacement. The deployed probe checked original eligibility before deleting the designated pod, then verified stale routing was invalidated and both identities changed. | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

- The K3s profile uses the environment's dedicated transport CA. A coordinated trust update for CA rollover is documented and is outside this ticket.
- The refreshed Framework starter includes the namespace trace policy field. Both the reviewed Java build and the deployed image used that version of the starter.

## Verification Results

- PASS — `mvn -o -B "-Dmaven.repo.local=$repo" "-Dmaven.repo.local.tail=$tail" '-Dtest=EngineClientIdentityTest' test` from `execution-engine`, using `C:/hamdev/maven39/bin/mvn.cmd`, JDK 21.0.2, the isolated `target/pr1-framework/repository` head and user Maven cache tail. All 3 focused tests passed.
- PASS — `mvn -o -B "-Dmaven.repo.local=$repo" "-Dmaven.repo.local.tail=$tail" verify` with the same executable and repositories. All 55 tests passed and Maven repackaged the executable JAR.
- PASS — `go test ./internal/engine -run 'Test(Transport|PodWatch|Console|Observer)' -count=1`, `go test ./... -count=1`, `go test -race ./... -count=1`, `go build ./...`, `go vet ./...`, and `go test -tags=k3s_integration ./internal/engine -run '^$'` from `management-backend`.
- PASS — `kubectl kustomize deploy/k3s`. The rendered resources preserved the certificate usages, two namespace-local Secret references, scoped RBAC and policy peer.
- PASS — `docker image inspect` for the three existing `:local` images verified they were built after their applicable reviewed source changes. The probe image build followed the current replacement checkpoint source edit.
- PASS — `& execution-engine/target/pr13-draft/verify-observation.ps1` with the explicit disposable `k3d-loomspan-cloud-pr1-cg1` context and `execution-engine/target/pr1-k3s/kubeconfig.yaml`. Both managed certificates became Ready. The allowed management peer opened port 8443, the unlabeled and wrong-namespace peers remained blocked, and the engine-local clientless TLS connection was rejected. The observation Job passed all checks.
- PASS — `git diff --check 3d1f785` and `git diff --cached --check`.

| Observation | K3s pod UID | Authenticated JVM ID |
| --- | --- | --- |
| First snapshot and confirmed fixture routing | `43c3148b-cb41-4755-9438-3a0ac35526ef` | `4cdc57b1-80f5-4b83-a3ee-c9086a2e2e29` |
| Replacement snapshot | `fc87805d-e27f-4483-84aa58f20090` | `07ce3d6a-6a5b-44e7-8d5c-3c06bb04e281` |

## Residual Risks and Optional Developer Checks

The first deployed snapshot arrived from a Ready pod. Pre-readiness dispatch eligibility is verified separately in the observer test with a held stream handshake, and the deployment declares no readiness probe. Durable restoration and invocation belong to PR 1.4 and PR 1.5. No optional developer checks remain for PR 1.3.

## Disposition

`clean`
