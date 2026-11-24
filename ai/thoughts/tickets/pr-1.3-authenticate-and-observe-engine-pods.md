# PR 1.3 — Authenticate and observe the single engine on K3s

## Outcome

Go can discover the single engine pod and obtain an authenticated observation of the exact JVM it will publish to, including when the JVM has zero Cloud applications installed.

## Requirements

- Use mTLS between Go and Java for the internal API and Framework Console. Java accepts only authenticated Go client identity, including on requests originating inside the engine pod. Console additionally enforces the existing Framework API-key check. Configure an environment-specific Kubernetes Secret to supply the same Console key to Go and the engine in that environment.
- Use the V1 K3s certificate and network boundaries. Cert-manager provisions and rotates the shared Java server credential and the single Go client credential. The engine ingress NetworkPolicy limits pod-to-pod access on the engine API port to Go management backend pods in the management namespace.
- Discover the engine through the K3s pod watch. Connect to that pod and read `GET /_loomspan/observability/v1/instance` on connection, reconnection, and an explicit refresh requested by placement. Associate Kubernetes namespace, name, UID, and address with the authenticated JVM `instanceId`. Keep each namespace’s observed `generationId`, `publicationVersion`, and `tracePersistencePolicy` together in the Go proxy’s in-memory projection. Read the application’s active policy from that namespace entry. The top-level policy describes the default namespace. Consume the Console stream after the snapshot.
- Keep the last snapshot available for inspection on disconnect. Mark disconnected observations and require a fresh snapshot before using them for routing again. Treat a changed instance ID under one pod UID as a JVM restart and a changed pod UID as a replacement, including when name or address is reused. Make the first authenticated snapshot available to placement even with zero Cloud applications installed. Report pod readiness and termination state for the routing eligibility check. The pod watch and Console connection suffice for discovery. The first publication has no dependency on a rollout readiness probe or a populated namespace list.

## Acceptance criteria

- [ ] A single engine on K3s accepts authenticated Go requests on the internal API and allows Go to read the first Console snapshot before any Cloud application is installed. The engine rejects requests lacking the required authenticated Go identity. Go also rejects a server credential that fails engine identity verification, on both the internal API and Console. A shared engine server credential authenticates the service. Use the pod watch and the directly queried JVM snapshot to identify the individual instance. An mTLS-authenticated Console request also needs the correct Console API key.
- [ ] K3s configuration uses the specified certificate and Secret ownership and restricts engine API port ingress to the management Go pods.
- [ ] The projection exposes pod and JVM identity, connection state, pod readiness and termination, and the per-namespace fields from an authenticated snapshot. A refresh sees installed publication data and the namespace’s own active policy before invocation, including when it differs from the default namespace’s policy. On disconnect the last observation remains inspectable.
- [ ] Disconnect, reconnect, JVM restart, and replacement pod observations distinguish the old and new JVMs and prevent stale observations from qualifying for routing.

## Context

GitHub PR 1, ticket 1.3 of 5. Follows [PR 1.2](pr-1.2-validate-and-publish-engine-generations.md). [PR 1.4](pr-1.4-record-and-restore-publications.md) uses discovery and explicit snapshot refresh for placement. Scope is single-engine startup and authenticated observation. Live event delivery to users, retained-trace lookup, and planned drain verification have separate V1 acceptance conditions. The [shutdown contract](../future/java-execution-engine-roadmap.md#planned-engine-shutdown) specifies one shared `loomspan.shutdown.timeout=60s` drain window, original mission deadlines and pod grace time for final responses and exit. The [trace contract](../future/java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces) specifies a shared host-configured directory, same-directory atomic completion rename and a replacement pod reading its predecessor’s completed files. PR 1 verifies discovery and observation during replacement.

[Pod and JVM identity](../future/execution-engine-api.md#pod-and-jvm-identity) · [Startup and readiness](../future/java-execution-engine-roadmap.md#startup-and-readiness) · [K3s network and certificate boundary](../future/roadmap.md#starting-point-for-placement-and-routing)

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** This introduces service authentication, deployment boundaries, and a lifecycle-sensitive observation contract.
- **Reassessment triggers:** Check that pod identity, certificate identity, and observation freshness can be verified together through the deployed connection.

## Pipeline notes

PR 1 tickets are committed locally in order without syncing between them. Intermediate commits may have incomplete wiring or build failures at the ticket boundary. Verify this ticket’s behavior where possible and record any verification that depends on later tickets. The cumulative milestone must build and pass its end-to-end checks in PR 1.5.
