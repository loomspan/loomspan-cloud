# K3s engine authentication and Console observation implementation plan

## Overview

- Ticket `ai/thoughts/tickets/pr-1.3-authenticate-and-observe-engine-pods.md`
- Research `ai/thoughts/research/pr-1.3-authenticate-and-observe-engine-pods.md`
- Go discovers the single engine pod, authenticates its Java service, and makes the authenticated JVM observation available to placement immediately after the first snapshot. The router can use the same projection once its additional conditions hold.

## Current State

`execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java` is the Spring Boot entry point. PR 1.1 adds its first Cloud adapter and application properties. PR 1.2 owns the validate and publish handlers under `/internal/v1`. There is no Go module, backend daemon, deployment tree, or engine certificate material in the starting checkout.

The Framework starter owns the Console API-key filter, runtime instance UUID, and instance and activity resources. `ObservabilityRestController.instance` requires the operator session independently of installed applications. The Framework `NamespaceStatus` record contains per-generation trace policy at sibling commit `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`. The preexisting local artifact at the same Maven snapshot coordinates lacks that field. Use the refreshed starter to verify the namespace wire contract. The isolated Maven repository under `execution-engine/target/pr1-framework/repository` provides it in this checkout.

## Desired End State

1. The engine's only deployed connector uses HTTPS and requires a trusted client certificate. A servlet filter admits only the Go workload identity. The Framework Console still verifies its API key independently.
2. Go uses trusted roots and the designated engine DNS identity for every internal API and Console request. Its connection goes directly to the Pod IP learned from a restricted Kubernetes list/watch.
3. A connected snapshot binds the watched pod namespace, name, UID, address, readiness, and termination state to an authenticated JVM UUID and per-namespace generation data. A valid snapshot can contain no published applications and can be used for first publication before pod readiness. Console activity follows the snapshot. A fresh observation confirms routing eligibility only on a ready, non-terminating pod.
4. The previous observation stays inspectable during a disconnect. Fresh snapshots are required for reconnect and explicit refresh. An old watch, pod, or stream cannot overwrite current state. JVM restarts and pod replacements remain distinguishable when names or addresses repeat.

## Scope

### In scope

- Engine and backend certificate authentication, environment configuration and rotation, and the Framework Console key on the deployed path.
- One engine Deployment, backend observer bootstrap, pod watch and scoped RBAC, network policy, and in-memory Console projection.
- Local verification with real TLS and streaming plus a disposable K3s probe for the actual manifests.

### Out of scope

- Publication persistence, recovery dispatch, caller-facing Console events, invocation, and retained trace browsing.
- Planned shutdown drain and completed trace recovery acceptance suites. Configure the 60-second Framework shutdown timeout and pod grace period for the engine deployment.
- Multi-engine selection, public routing and ingress, certificate authority rollover, shared trace storage selection, and ClusterIssuer provisioning beyond its environment-owned CA prerequisite.

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Impact and Risk Analysis

- The ingress policy covers pod-to-pod traffic and pairs namespace and pod selectors in one peer. The TLS connector and leaf check cover same-pod traffic and situations in which network policy does not authenticate the sender. CA trust alone admits any leaf issued by that CA at the TLS layer, so the servlet check is required for every protected request.
- A trusted engine certificate is common to all engine pods. Authenticate the service with mTLS and identify the JVM using the bound pod watch and Console UUID. Connect directly to the Pod IP and refuse redirects, using the engine DNS SAN for TLS verification.
- Kubernetes readiness and stream liveness are independent. Preserve the last snapshot during loss, expose current readiness and termination, and close routing eligibility on uncertainty. Suppress late list, stream, and refresh results from superseded targets.
- A refresh can race with stream disconnect or JVM replacement. Stage network results outside the state lock and install them only after checking the current watch target and observation epoch. Use one nonblocking update notification for consumers to reread the full projection.

## Implementation Approach

Integrate through a Java servlet filter and profile-specific TLS properties, keeping Framework's Console security in its existing layer. The standard Go HTTP and TLS libraries provide the narrow transport used by both Console and domain requests. Kubernetes namespaced pod list/watch semantics are sufficient for V1. Keep that protocol in a dedicated adapter and the observer independently testable. Consume the first snapshot before waiting for stream admission. Detect wire discontinuities by matching the instance response header, active page, and activity handshake to the snapshot UUID.

The active execution page associates its cursor with the JVM through the authenticated response header. Its JSON body carries the cursor. The observer exposes a broadcast change channel. Each consumer obtains `Changes()` before calling `Current()` and obtains a new channel after closure. `ConnectionID` advances when a fresh connection installs its initial snapshot. `Refresh(ctx)` keeps that identity for a matching JVM and schedules reconnect when the JVM identity changes.

### Phase 1 — Engine and backend authentication

#### Changes

- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/EngineClientIdentityConfiguration.java` registers a servlet filter on `/internal/v1`, `/internal/v1/*`, `/_loomspan/observability/v1`, and `/_loomspan/observability/v1/*`. Run before the Framework filter with async support and cover forwarded and async dispatches. Require the leaf of the servlet `jakarta.servlet.request.X509Certificate` chain on a secure request. Require an exact URI SAN `spiffe://loomspan-cloud/management/backend` (configurable from the engine environment). Reject a missing, invalid, or mismatched certificate without accepting forwarding headers or a CN substitute. Leave Framework API-key authentication and response semantics intact after the certificate check.
- [x] `execution-engine/src/main/resources/application-k3s.properties` configures port 8443, a Boot PEM SSL bundle with `reload-on-update=true`, mandatory trust and client authentication, and paths supplied by the engine Deployment. Enable Framework observability with its existing API-key property from the shared environment key. Configure `loomspan.shutdown.timeout=60s`. Avoid exposing an alternate plaintext or loopback connector. Preserve prior multipart configuration in `application.properties`.
- [x] `management-backend/go.mod` defines module `github.com/loomspan/loomspan-cloud/management-backend` with Go 1.26. `management-backend/internal/engine/transport.go` constructs the transport for an observed Pod IP using `tls.Config.ServerName` for `execution-engine.loomspan-engine.svc` and explicit environment roots. Set minimum TLS 1.2, disable proxy and redirects, and forbid host-changing requests. Reload the client certificate and key for each new TLS connection from the mounted Secret. Snapshot a single Secret projection generation when available, or fail the handshake on inconsistent files and allow the connection loop to retry. The Console key is added only by the Console client. Expose the same authenticated transport to validate, publish, and later invoke callers.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/EngineClientIdentityTest.java` and `management-backend/internal/engine/transport_test.go` cover independent certificate checks, Console authorization, Pod IP dialing, trusted wrong identities, and renewal.

#### Automated verification

- [x] From `execution-engine/`, `mvn -B -o -Dtest=EngineClientIdentityTest test` with JDK 21 and refreshed Framework passes the certificate and Console cases.
- [x] From `management-backend/`, `go test ./internal/engine -run 'Test(Transport|PodWatch|Console|Observer)' -count 1` passes real loopback TLS identity checks on both managed route families and the Console and watch lifecycle tests.

#### Optional developer checks

- None.

### Phase 2 — Pod watch and Console projection

#### Changes

- [x] `management-backend/internal/engine/pod_watch.go` implements a narrow authenticated Kubernetes list/watch client with the projected service account token and API CA. Select the engine pods in `loomspan-engine` using the deployment label. Start each watch from the list resource version, update the candidate on pod events, and accept bookmarks without changing the candidate. Reconnect through a fresh list after interruption or an expired resource version. Read the projected token afresh for new list or watch requests. Track namespace, name, UID, IP, `Ready=True`, and deletion timestamp. Treat absent, multiple, or addressless candidates as unavailable and avoid assuming an unready selected pod cannot accept publication. Treat loss of the authoritative watch as unavailability until it is re-established.
- [x] `management-backend/internal/engine/console.go` queries the exact Framework paths. GET `/instance` returns a complete snapshot with UUID and generation tuples. Preserve nullable publication versions and namespace policies. Reject snapshots with inconsistent identity, malformed or duplicate rows, or missing policy for a published namespace. For each new stream, GET `/active-executions?pageSize=1` and read its opaque decimal `resumeCursor`. Open `/activity?instanceId=…&afterCursor=…` with `Accept: text/event-stream` and the Console key, then validate the authenticated instance header and first handshake. Drain framed activity with bounded parsing and report EOF or protocol failure. After stream loss or cursor rejection, obtain a fresh snapshot and baseline before reopening. Put cancellable deadlines on finite requests and stream setup. Use TCP keepalives. Framework's stream can legitimately remain idle with no application activity.
- [x] `management-backend/internal/engine/observer.go` owns the single selected pod and an immutable-by-copy projection. Keep the last observation for inspection after closing the serving view. Scope each acceptance of HTTP results to the matching watch selection, pod UID/address, and connection epoch. Reconcile watch changes, stream lifecycle, and `Refresh(ctx)` without holding state locks during I/O. Treat a same-UID changed UUID as JVM restart and a changed UID as replacement. Drop old results after either transition. A refresh may register the new JVM only as part of its new observation epoch and stream lifecycle. Notify observers on relevant state transitions, including the first legitimate snapshot. Provide `Current()`, `Changes()`, `Refresh(ctx)`, and projection lookup methods for the later placement and router. The publication candidate requires a current connected snapshot and no deletion timestamp. A routing confirmation additionally requires pod readiness, a namespace entry, and exact intended publication version equality. The namespace's own policy is returned with its generation. The top-level policy describes default only.
- [x] `management-backend/cmd/management-backend/main.go` wires the in-cluster watch, credential files, and Console observer with coordinated cancellation and sanitized errors. Report discovery and liveness through normal logging with pod UID and instance ID. No credential values or HTTP bodies in startup, certificate, or watch logs.
- [x] `management-backend/internal/engine/pod_watch_test.go`, `console_test.go`, and `observer_test.go` exercise startup, shapes, list/watch recovery, refresh, disconnected inspection, JVM replacement, and concurrency.

#### Automated verification

- [x] From `management-backend/`, `go test ./...` and `go build ./...` pass.
- [x] From `management-backend/`, `go test -race ./...` passes on a race-enabled Go platform. Use local Linux when the Windows race runtime or C compiler is unavailable.

#### Optional developer checks

- None.

### Phase 3 — K3s deployment and deployed verification

#### Changes

- [x] `deploy/k3s/kustomization.yaml`, `namespaces.yaml`, `certificates.yaml`, `rbac.yaml`, `network-policy.yaml`, `execution-engine.yaml`, and `management-backend.yaml` define the initial deployment. Use `loomspan-engine` and `loomspan-management` namespaces. Bind a namespaced Role with only pod list/watch to the Go service account across namespaces. Use one engine replica and a replacement strategy consistent with a single engine. Accept engine-port ingress from backend-labeled pods in `loomspan-management`. Disable API token mounting on the Java engine and mount credential Secrets as volumes without subPath.
- [x] In `certificates.yaml`, use two cert-manager `Certificate` resources backed by the environment's dedicated `loomspan-engine-transport` ClusterIssuer. Issue the single Java server certificate with the engine DNS SAN and server-auth usage and the single Go client certificate with its URI SAN and client-auth usage. Set private key rotation to `Always`. Mount the shared Java certificate on every engine replica, including replacement instances. Document the dedicated CA and expected `ca.crt` trust artifact in the issued Secrets. Match the manifests' port and identity values to the code and tests.
- [x] `deploy/k3s/README.md` supplies namespace creation, issuer prerequisites, image build/tag steps, and a safe method to populate a fresh environment key in the two namespace-local copies of the `loomspan-console` Secret from one external value. Kubernetes Secrets are namespaced. Reference both copies under the same key name and avoid tracked Secret contents. Use a different key for development, QA, and production.
- [x] `execution-engine/Dockerfile` packages the executable JDK 21 Boot JAR, and `management-backend/Dockerfile` builds the Go binary. Add the opt-in K3s probe at `management-backend/internal/engine/k3s_test.go` with the `k3s_integration` build tag. Provide `deploy/k3s/verify-observation.yaml` as an opt-in Job using the Go deployment's ServiceAccount, management label, and certificate mounts. Use a test-only pack and avoid adding privileged debug routes to either service.

#### Automated verification

- [x] From repository root, `kubectl kustomize deploy/k3s` renders. Verify the NetworkPolicy direction and scope, RBAC namespace, Certificate SANs and usages, and both Secret references with the rendered artifacts.
- [x] On the dedicated `k3d-loomspan-cloud-pr1-cg1` context, run the scoped commands and acceptance probe in `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods-testing.md`. Capture the watched pod UID, authenticated JVM ID, run with no Cloud apps, stream handshake, route-family credential results, and policy evidence.

#### Optional developer checks

- None. The disposable cluster is available for PR 1.3 deployed checks. PR 1.5 still owns the cumulative recovery and invocation.

## Test Strategy

Use embedded Java tests to check the real TLS connector and Framework key filter, Go loopback TLS tests for peer verification and connection reuse, a deterministic list/watch and Console test server for concurrency and lifecycle, and a disposable K3s test for provisioned certs, RBAC, NetworkPolicy and replacement. Verify the first snapshot without a published namespace and compare a later namespace policy with the default policy after an explicit refresh. The testing plan contains exact evidence and commands.

## Acceptance-Criteria Traceability

| Acceptance criterion | Planned code evidence | Planned test evidence |
| --- | --- | --- |
| Mutual identity on internal and Console paths, independent key, first snapshot with no Cloud apps | Java filter and Boot SSL connector, Go Pod IP transport, watch bootstrap, Console GET | `EngineClientIdentityTest`, `TestTransport` cases, `TestK3sEngineObservation` |
| Cert-manager ownership, environment Secret and scoped ingress | Certificates, Deployment mounts, separate key copies, one NetworkPolicy peer and limited RBAC | Manifest rendering and deployed checks of cert bindings, disallowed pod, allowed Go pod |
| Current projection and refresh with per-generation policy, last snapshot inspectable | `observer.go` and `console.go` tuple and projection methods | `TestObserverRefreshProjectsNamespacePolicy`, `TestObserverRetainsDisconnectedObservation`, deployed refresh after versioned test publish |
| Expire stale observations on reconnect, JVM restart and replacement pod | Watch target identity plus observer epoch, matched stream and snapshot UUIDs | `TestPodWatchRelistsOnExpiredResourceVersion`, `TestObserverRejectsSupersededWork`, deployed replacement smoke check |

## Risks and Rollback/Recovery

Enable the deployed Go and Java transport together with the certs and Console key. The deployment should fail closed on absent or invalid credentials. Restore the known-good credential mounts if a rejected rollout fails, retaining the direct HTTPS trust boundary. Inspect the last snapshot on disconnection but keep it ineligible until a new authenticated observation. Use the subsequently prepared disposable K3s context for deployed acceptance. Use the explicit kubeconfig and context for every command.

## References

- `ai/thoughts/tickets/pr-1.3-authenticate-and-observe-engine-pods.md`
- `ai/thoughts/research/pr-1.3-authenticate-and-observe-engine-pods.md`
- `ai/thoughts/future/execution-engine-api.md`
- `ai/thoughts/future/roadmap.md`
- `ai/thoughts/future/go-backend-roadmap.md`
- `ai/thoughts/future/java-execution-engine-roadmap.md`
