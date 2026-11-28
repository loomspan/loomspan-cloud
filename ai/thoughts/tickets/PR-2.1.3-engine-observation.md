# PR 2.1.3 — Observe the assigned Java engine from inside the Go backend

## Outcome

Go discovers the single Java engine and obtains a trustworthy live view of its JVM and namespaces, including when zero applications are installed. Placement and routing can use that same in-memory observation.

## Context

GitHub PR 2, Go backend Phase 1. Build on PR 2.1.1’s moved Console client and PR 2.1.2’s K3s backend. The existing Java service lives in `execution-engine/`. This is the first real Go-to-engine integration, before publication or routing handlers are built.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Placement and routing](../future/roadmap.md#starting-point-for-placement-and-routing) · [Pod and JVM identity](../future/execution-engine-api.md#pod-and-jvm-identity) · [Shutdown contract](../future/java-execution-engine-roadmap.md#planned-engine-shutdown)

## Requirements

- Deploy the single engine with Recreate rollout and snapshot-based readiness. Keep its API internal and restrict its API port to management-namespace Go backend pods using NetworkPolicy. Configure cert-manager and installation trust for mutual TLS with one Go client credential and the shared engine server credential. Console additionally requires the environment’s shared API key Secret; ordinary pod replacement preserves it.
- Use the K3s pod watch and the moved client to connect directly to the discovered pod. Pair Kubernetes namespace/name/UID/address with the authenticated instanceId. Read the instance snapshot on connect/reconnect, consume the engine stream, and support a requested fresh snapshot after install. Namespace observations carry generationId, publicationVersion and tracePersistencePolicy.
- Expose the observation via Go calls in the backend. Track connected, ready, accepting and terminating state so a stale observation cannot authorize dispatch. Mark a disconnected observation for inspection and obtain a fresh one before using a replacement connection. Engine restoration, per-viewer event subscriptions, catalogs and trace indexing are later phases.

## Acceptance criteria

- [ ] From the deployed backend, read an authenticated snapshot from the real engine before any application is installed. Demonstrate that the engine is ready and observable at that point.
- [ ] Exercise missing/incorrect Console credentials and an untrusted peer; neither becomes an eligible observed engine. Verify the internal network and TLS setup still allows authorized Go access.
- [ ] Disconnect and reconnect, distinguish a JVM restart from a replacement pod, and show that cached namespaces do not become eligible before a fresh authenticated observation. A terminating pod is ineligible for new publish, validate and invoke dispatch.

## Handoff and deferred work

Use [PR 2.1.1](PR-2.1.1-console-home.md) for the client and [PR 2.1.2](PR-2.1.2-backend-readiness.md) for deployment. Hand [PR 2.1.5](PR-2.1.5-stage-and-validate.md), [PR 2.1.6](PR-2.1.6-first-publication.md) and [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the authenticated engine connection, observation/refresh entry points and known-good K3s snapshot check. They own Validate, Publish and Invoke respectively. [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns restoration and restart confirmation; [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns catalogs; [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns viewer subscriptions and trace indexing.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Combines identity, K3s discovery, network isolation and lifecycle transitions across services.
- **Reassessment triggers:** Reassess adapter scope if the moved client already implements parts of snapshot or stream handling.
