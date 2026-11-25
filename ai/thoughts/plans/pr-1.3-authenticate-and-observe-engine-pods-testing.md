# K3s engine authentication and Console observation testing plan

## Change Summary

Exercise mutual identity, independent Console authentication, namespaced pod discovery, and a usable early snapshot before the engine contains any Cloud publications. Verify the connection, pod, JVM, and per-generation fields survive or expire at the correct lifecycle boundaries.

## Impacted Areas and Risks

| Category | Risk | Planned evidence |
| --- | --- | --- |
| Java transport | A client with a trusted but unrelated leaf reaches a handler, an alternate path permits plaintext, or the key filter becomes optional | Embedded Boot connector tests with distinct temporary credentials, real Framework routes, and an internal route side-effect counter |
| Go transport | HTTP client connects to an unintended workload or follows redirects with credentials | Direct loopback TLS and wrong DNS/CA tests for both route families, no-redirect and renewal tests |
| Watch and projection | Default namespace policy is used for an application, stale network work reactivates routing, or first publication waits for readiness | Deterministic watch, Console, and observer integration tests with explicit event ordering and barriers |
| Deployment | Cert-manager files or RBAC are linked incorrectly, or the network selector admits unrelated pods | Rendered manifests followed by a separate, disposable K3s probe |

## Existing Coverage and Environment Constraints

The starting execution-engine module uses Maven and Boot with prior ticket testing under `execution-engine/src/test/`. No Cloud test covered these transport or observation boundaries before PR 1.3. Go 1.26.1, JDK 21, Maven 3.9.11, and the `kubectl` client are available.

The cached Framework snapshot from before `2ca9caf4` lacks `NamespaceStatus.tracePersistencePolicy`. The newer Framework starter has been installed in the isolated local Maven head at `execution-engine/target/pr1-framework/repository` for this checkout. With that repository under the module target, avoid Maven `clean` and use the existing local Maven cache as the read-only tail.

Normal tests are loopback-only. The deployed opt-in probe requires a functioning K3s NetworkPolicy controller, cert-manager, and test image tags. The disposable `loomspan-cloud-pr1-cg1` K3s v1.34.13 cluster used cert-manager v1.21.3 and a dedicated test transport issuer. No environment credentials belong in test fixtures or command output.

## Failing Test First

- Name `authenticatedLeafAndConsoleKeyAreRequiredIndependently`.
- Type. Embedded servlet integration with real HTTPS plus a protected test handler.
- Location `execution-engine/src/test/java/ai/loomspan/cloud/engine/EngineClientIdentityTest.java`.
- Arrange and act. Start the Cloud application with the Framework observability key and required client authentication. Generate one fixture CA, a server leaf with the engine DNS identity, a Go client leaf with the expected URI SAN, and a client leaf with a different URI signed by the same CA. Start with zero Cloud publications. Use each leaf on the internal endpoint and Console instance endpoint. On the Console endpoint use both the valid key and an invalid one.
- Expected pre-fix failure. The trusted wrong Go identity reaches the test handler or receives the successful Framework Console response with the correct key. The connector rejects clients without a certificate at the TLS layer.

## Tests to Add or Update

### 1. `authenticatedLeafAndConsoleKeyAreRequiredIndependently` and `enforcesCertificateIdentityAcrossProtectedDispatches`

- Type and location. Embedded Boot TLS plus servlet boundary checks in `execution-engine/src/test/java/ai/loomspan/cloud/engine/EngineClientIdentityTest.java`.
- Proves. Authorized Go reaches internal processing and receives the first real Console snapshot before Cloud installations. Missing or untrusted certificates fail negotiation. A trusted wrong leaf, a matching CN without the URI SAN, and headers simulating a forwarded certificate never grant access. With valid mTLS, missing, malformed, or wrong Console keys still fail. The valid key exposes the Framework instance header and matching UUID.
- Inputs. Use generated or test-only short-lifetime chains with explicit extended key usages. Connect directly to loopback when testing an engine-pod-local equivalent. Assert the internal endpoint handler hit count and positive Console shape. Check the stream async handshake with valid credentials. Start the test app with Cloud k3s profile properties and a random port while supplying independent fixture mounting paths. Verify Java picks up a renewed server certificate at the same Secret path on a fresh handshake without restarting the JVM.
- Boundary. No live identity provider or actual Cloud credential. Any prior publication handler integration tests exercising the now-protected paths must set a trusted verified servlet identity or use the real fixture connector.

### 2. `TestTransport`

- Type and location. Go real loopback TLS tests in `management-backend/internal/engine/transport_test.go`.
- Proves. The same transport dials a Pod IP while validating the service DNS SAN and correct root on both internal and Console paths. Wrong DNS from the configured issuer, a wrong issuer, and incomplete client material fail with zero receiver dispatches. Refuse alternate hosts and redirects and verify the Console key stays off internal requests. Exercise IPv6 URL construction if the Pod IP supports it.
- Inputs. Generate independent issuer and identities with Go `crypto/x509`. Force new connections when replacing fixture files and assert the renewed Go leaf serial is observed by the receiver. Use short local contexts and an explicit request counter to distinguish handshake failures from endpoint errors.

### 3. `TestPodWatchRelistsOnExpiredResourceVersion` and `TestPodWatchDerivesCurrentEligibilityFields`

- Type and location. Namespaced Kubernetes REST protocol tests in `management-backend/internal/engine/pod_watch_test.go`.
- Proves. The watch begins at the resource version returned by the labeled list, consumes changes and bookmarks, relists after expiry or transport loss, and reloads the projected bearer token. Snapshots map namespace, name, UID, address, readiness, and deletion correctly. Reject empty or ambiguous candidates conservatively.
- Inputs. An HTTP test server serves list and watch responses, including readiness-only changes, a deletion timestamp, status 410 and an `ERROR` watch event, a malformed or truncated event, and replacement records with the same name and IP. Confirm cancellation and token reload deterministically.

### 4. `TestConsoleReadsSnapshotBeforeStream` and `TestConsoleRejectsMismatchedIdentityOrGeneration`

- Type and location. Go HTTP and SSE protocol integration in `management-backend/internal/engine/console_test.go`.
- Proves. Authenticated requests use the exact routes, matching instance headers, cursor query, and SSE Accept header. Snapshot parsing permits empty Cloud state and null default publication, retains all legitimate namespace tuples, and rejects malformed or inconsistent results. Stream startup validates the handshake UUID and cursor. Cursor eviction, EOF, and malformed or oversized frames produce observable failure.
- Inputs. Supply deterministic UUIDs and a per-namespace policy differing from the top-level default. Return no stream admission until the test has seen the first authenticated snapshot, then emit a handshake and one activity frame before closing. Polling tests use channels, timeouts as upper bounds, and no sleep-based ordering.

### 5. `TestObserverRefreshProjectsNamespacePolicy`, `TestObserverRetainsDisconnectedObservation`, and `TestObserverRejectsSupersededWork`

- Type and location. Go state-loop integration in `management-backend/internal/engine/observer_test.go`.
- Proves. A valid empty snapshot while unready may dispatch the first publish. Routing is closed until the current connected, ready, non-terminating pod confirms the exact intended namespace and publication version. Termination removes administrative eligibility immediately. The explicit refresh retrieves the new publication and its same-generation policy without waiting for activity. After stream or watch loss, the last observation stays inspectable but cannot route. A fresh snapshot is required after reconnection, including when the version previously matched.
- Inputs. Cover same UID with a different JVM ID, different UIDs on the original name and address, a stale HTTP response delivered after cancellation, two simultaneous refreshes, a quiet current SSE stream, and callers modifying the returned snapshot copy. Release barriers after checking intermediate state. Run tests under the Go race detector where supported.

### 6. `TestK3sEngineObservation`

- Type and location. Build-tagged deployed integration in `management-backend/internal/engine/k3s_test.go`. Use `k3s_integration` so the default local commands neither need nor query a cluster.
- Proves. The actual rendered resources let Go use its ServiceAccount to watch the intended engine pods, authenticate to the engine on the internal and Console paths, take the first snapshot with no Cloud applications, and consume the SSE handshake. Connect before waiting for any engine rollout readiness condition. Confirm the pod UID in the Go projection against Kubernetes and its instance UUID against Framework's authenticated header. With the PR 1.2 endpoints deployed, publish a minimal test-only reference-free pack with the active policy `NEVER`, refresh, and assert the namespace generation, version, and exact policy differ from the default as expected. Exercise a restarted or replacement pod and confirm no old observation routes.
- Edge cases. The authorized management probe has an independent bad-key test. Inside the engine pod, an HTTPS request without a client credential fails. Run a separate probe from an unlabeled pod in the management namespace and from a matching-label pod in an unrelated namespace to verify each selector is necessary. Use raw TCP reachability for policy negatives and a passing control from the authorized management probe to distinguish policy denial from TLS rejection. Assert both certificate objects are Ready, their generated Secrets reference the intended cert-manager certificates, and the rendered deployment uses read-only full mounts without `subPath`.
- Boundary. Run exclusively in the disposable K3s context. The test utility runs with the management labels and bounded contexts and owns only the test publication and verification resources. Use fixture keys for authentication negatives. The routine Maven and Go suites stay local.

## Safe Verification Commands

From `execution-engine/` in PowerShell, with the isolated Framework snapshot already installed:

```powershell
$loomspanFrameworkHead = Join-Path (Get-Location) 'target/pr1-framework/repository'
$loomspanMavenTail = Join-Path $env:USERPROFILE '.m2/repository'
& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$loomspanFrameworkHead" "-Dmaven.repo.local.tail=$loomspanMavenTail" '-Dtest=EngineClientIdentityTest' test
& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$loomspanFrameworkHead" "-Dmaven.repo.local.tail=$loomspanMavenTail" verify
```

From `management-backend/`:

```text
go test ./internal/engine -run 'Test(Transport|PodWatch|Console|Observer)'
go test ./...
go test -race ./...
go build ./...
go test -tags=k3s_integration ./internal/engine -run '^$'
```

From repository root, client-side render only:

```text
kubectl kustomize deploy/k3s
```

### Disposable K3s verification

Use the already prepared disposable context `k3d-loomspan-cloud-pr1-cg1`. Follow `deploy/k3s/README.md` to check cert-manager, create the test-only ClusterIssuer, certificates' CA prerequisites, and namespace-local matching Console Secrets. Import the engine, backend, and probe images to the disposable cluster under its image tags. From the repository root in PowerShell:

```powershell
$loomspanKubeconfig = Join-Path (Get-Location) 'execution-engine/target/pr1-k3s/kubeconfig.yaml'
$loomspanK3sArgs = @('--kubeconfig', $loomspanKubeconfig, '--context', 'k3d-loomspan-cloud-pr1-cg1')
kubectl @loomspanK3sArgs get nodes
kubectl @loomspanK3sArgs -n cert-manager wait --for=condition=Available deployment --all --timeout=120s
kubectl @loomspanK3sArgs apply -k deploy/k3s
kubectl @loomspanK3sArgs -n loomspan-engine wait --for=condition=Ready certificate/engine-server --timeout=120s
kubectl @loomspanK3sArgs -n loomspan-management wait --for=condition=Ready certificate/management-client --timeout=120s
kubectl @loomspanK3sArgs apply -f deploy/k3s/verify-observation.yaml
kubectl @loomspanK3sArgs -n loomspan-management logs -f job/loomspan-observation-probe --pod-running-timeout=120s
```

Compile the Linux probe with `GOOS=linux go test -c -tags=k3s_integration -o k3s-engine-test ./internal/engine` from `management-backend/` in a shell that supports that environment assignment. The Job runs `/app/k3s-engine-test -test.run '^TestK3sEngineObservation$' -test.v` under the backend mounts, configuration, labels, and ServiceAccount. Have it record the initial pod UID, authenticated UUID and SSE handshake, the test publication refresh and namespace policy, and its readiness to observe replacement.

After that checkpoint, use a second terminal with the same explicit kubeconfig and context:

```powershell
kubectl @loomspanK3sArgs -n loomspan-engine get pods -l app.kubernetes.io/name=loomspan-execution-engine -o wide
kubectl @loomspanK3sArgs -n loomspan-engine delete pod -l app.kubernetes.io/name=loomspan-execution-engine --wait=false
kubectl @loomspanK3sArgs -n loomspan-management wait --for=condition=Complete job/loomspan-observation-probe --timeout=180s
```

Delete only the isolated scenario's single engine pod. The same observer must see stale eligibility close and a fresh, changed UID and instance UUID on the replacement. Verify the two negative ingress peers and engine-pod-local clientless handshake in this designated context with bounded probes. The initial deployed proof is complete once the Job passes and the policy and handshake evidence are captured. PR 1.5 still verifies durable recovery and invocation.

## Implementation Verification

The focused Java TLS suite passed all 3 tests. Full Maven verification passed all 55 tests and produced the executable JAR used for the final local engine image. Focused Go tests, the full Go suite, the race detector, the Go build, and opt-in build-tag compilation passed. The deployment rendered with both certificate resources, the namespaced Console Secret references, restricted pod list/watch RBAC, and the engine-port ingress selectors.

The final disposable K3s run used the current module's JAR and explicitly scoped kubeconfig and context. Both certificates were Ready and their generated Secrets carried the corresponding certificate annotations. The authorized probe received its first snapshot before any Cloud publication and consumed the activity handshake. The same authenticated identity with a different Console key received 401. Publish returned 204, and an explicit refresh returned `probe-version-1` with application policy `NEVER` and default policy `ONERROR`. The engine-local clientless TLS handshake failed. A backend-labeled management pod opened TCP to port 8443. An unlabeled management pod and a backend-labeled pod in another namespace both remained blocked during the 12-second controls. The bounded TCP probes allow the network policy controller time to reconcile newly created pods.

| Observation | Kubernetes pod UID | Authenticated JVM UUID |
| --- | --- | --- |
| Initial fixture JVM | `ba0b58c5-77be-492c-8647-cdc54dbce30a` | `c49c9139-125d-41d9-9486-9be08ee4d331` |
| Replacement fixture JVM | `87e18adc-591f-4840-a732-c5757822d2ca` | `34610bcd-29c7-4ea5-ba24-f9b100acb3b8` |

The replacement test invalidated the old route and read a new snapshot with no fixture publication. The observer race suite separately covers a restarted JVM under a reused pod UID, stale refresh responses, and concurrent refreshes.

## Optional Developer Checks

None. The disposable K3s scenario remains required at the cumulative milestone.

## Exit Criteria

- [x] The isolated real TLS red test omitted the servlet identity filter and returned 204 for a trusted wrong leaf where 403 was required.
- [x] Java and Go focused and full safe suites pass with the refreshed Framework starter.
- [x] Go race verification covers refresh, disconnect and replacement, including ten repetitions of the observer suite on Windows.
- [x] The deployment renders and exercises its mTLS, key, watch, and ingress rules in the disposable K3s context.
- [x] All acceptance criteria map to directly executable evidence, including first publish readiness independence and policy selection from the namespace tuple.
- [x] Routine tests use local fixtures without operational credentials, live cluster changes, or external services.

## Step Report: 2_create_plan + 3_testing_plan

STATUS: complete
ARTIFACTS:
  - ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md
  - ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods-testing.md
SUMMARY: Planned mutual peer identity and the independent Console key, an authenticated direct-pod observer, and the K3s deployment. The testing plan covers real TLS, ordering and connection races, and disposable-cluster checks.
DECISIONS:
  - Use a configured engine DNS SAN with direct Pod IP dialing, and a Java leaf URI SAN check before Framework Console authentication.
  - Publish the first authenticated snapshot before stream admission and reject stale updates using the pod target and connection epoch.
  - Use the refreshed Framework starter in an isolated local Maven repository and restrict deployed checks to the explicitly named disposable K3s context.
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Run PR 1.3 implementation against these plans after PR 1.2.
