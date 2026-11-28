# PR 2.1.3 — Observe the assigned Java engine from inside the Go backend

## Outcome

Go discovers the single Java engine and obtains a trustworthy live view of its JVM and namespaces, including when zero applications are installed. Placement and routing can use that same in-memory observation.

## Context

GitHub PR 2, Go backend Phase 1. Build on PR 2.1.1’s moved Console client and PR 2.1.2’s K3s backend. The existing Java service lives in `execution-engine/`. This is the first real Go-to-engine integration, before publication or routing handlers are built.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Placement and routing](../future/roadmap.md#starting-point-for-placement-and-routing) · [Pod and JVM identity](../future/execution-engine-api.md#pod-and-jvm-identity) · [Shutdown contract](../future/java-execution-engine-roadmap.md#planned-engine-shutdown)

## Requirements

Run the initial pod list and subsequent watch with the PR 2.1.2 service account, engine namespace and label selector. Exercise both operations with the installed namespaced RoleBinding.

- Treat `loomspan-cloud-test-suite/deploy/k3s` and `management-backend/` at Cloud commit `75358f8` as historical PR 1 references. Reuse the pod-watch, certificate identity, RBAC and NetworkPolicy patterns where they meet this ticket. Keep the PR 2.1.2 configuration and storage contract. On reconnection this ticket refreshes the authenticated observation. Verify that reconnecting to the same JVM makes no install call. The milestone’s publication reconciliation worker is outside this ticket; Phase 2 owns reconciliation of outstanding requests and conditional restoration of durable intent under the [current recovery sequence](../future/go-backend-roadmap.md#observe-live-engines).
- In Full Cloud, configure the moved Go transport to present the mounted management client certificate and private key for Console and internal API calls. The source Console transport in `loomspan-console/internal/applicationclient/client.go` configures trusted roots but supplies no client certificate. The engine checks the leaf certificate's URI SAN against `loomspan.cloud.engine.management-client-uri` (default `spiffe://loomspan-cloud/management/backend`). Keep the standalone profile independent of Cloud credentials.
- Dial the watched pod IP and set the TLS server name to the shared engine certificate's DNS identity (`execution-engine.loomspan-engine.svc` in the reference deployment). Verify the chain against the installation CA and verify the DNS name on every connection. Dialing the pod preserves the pod UID/JVM association; the stable DNS identity permits certificate reuse across pod replacements. Reference: Git revision `75358f8`, `deploy/k3s/README.md` and `deploy/k3s/certificates.yaml`.
- Use the existing `consoleCompatibilityVersion` when connecting the moved Console client to the engine. Framework has no clients yet, so the Console migration and Java wrapper update can ship together at that version. Verify that the deployed pair passes the existing exact-match check. [Console compatibility decision](../future/roadmap.md#console-deployment-profiles)
- Build the Java engine image against the Framework snapshot that supplies `SkillInvocationResult`. The Cloud wrapper's `InvocationService` currently expects a String from `SkillTemplate.invoke`. Update the wrapper and affected tests for the result object, preserving its exact text and passing its canonical session ID through the top-level internal invoke response. On failure use the Framework exception's session ID when available and omit the field for pre-session failures. Verify this with execution views off. This compatibility work belongs here because this ticket deploys the real engine. PR 2.1.7 owns Go forwarding and end-to-end verification. [Internal invoke responses](../future/execution-engine-api.md#invoke-responses)
- Deploy the single engine with Recreate rollout and snapshot-based readiness. Keep its API internal and restrict its API port to management-namespace Go backend pods using NetworkPolicy. Configure cert-manager and installation trust for mutual TLS with one Go client credential and the shared engine server credential. Console additionally requires the environment’s shared API key Secret; ordinary pod replacement preserves it.
- Use the K3s pod watch and the moved client to connect directly to the discovered pod. Pair Kubernetes namespace/name/UID/address with the authenticated instanceId. Read the instance snapshot on connect/reconnect, consume the engine stream, and support a requested fresh snapshot after install. Namespace observations carry generationId, publicationVersion and tracePersistencePolicy.
- Expose the current pod and Console observations in memory. For Invoke, the assigned pod must have Kubernetes `Ready=True` and no `metadata.deletionTimestamp`, and the observation from its current authenticated Console connection must contain the target namespace. Keep a disconnected observation available for inspection only. Require a fresh authenticated snapshot on reconnect. Go evaluates Invoke eligibility without a separate `accepting` flag or a pre-dispatch engine poll. The namespace check is specific to Invoke so Validate and first Publish can target an empty engine. Engine restoration, per-viewer event subscriptions, catalogs and trace indexing are later phases.

## Acceptance criteria

- [ ] Compile and test the wrapper against the Framework API returning `SkillInvocationResult`. With views off, exercise `POST /internal/v1/invoke`: on success its JSON contains the exact result text and the nonblank top-level `sessionId` from Framework; a failure after session creation carries the exception’s ID; a pre-session failure omits the field. Exercise the HTTP response body so a change to `InvocationService` alone cannot satisfy this check.
- [ ] Reach a selected pod by IP with an engine certificate valid for the configured DNS name and lacking the pod IP SAN. With the correct Console API key, verify that a missing client certificate fails TLS, a trusted client certificate with the wrong management URI receives 403, and a trusted server certificate with the wrong DNS name fails verification. Only the correctly authenticated snapshot becomes eligible for use.
- [ ] From the deployed backend, read an authenticated snapshot from the real engine before any application is installed. Demonstrate that the engine is ready and observable at that point.
- [ ] Exercise missing/incorrect Console credentials and an untrusted peer; neither becomes an eligible observed engine. Verify the internal network and TLS setup still allows authorized Go access.
- [ ] Disconnect and reconnect, distinguish a JVM restart from a replacement pod, and show that cached namespaces do not become eligible before a fresh authenticated observation. A terminating pod is ineligible for new publish, validate and invoke dispatch.
- [ ] A fresh snapshot from the same JVM can make a still-installed namespace eligible again. A new JVM, including one in the same pod, can report zero Cloud namespaces. Accept that snapshot and make zero install calls. Once Phase 1 supports Publish and Invoke, this means a previously published application whose namespace is absent retains its durable current publication and is temporarily unavailable for invocation. Phase 2 owns restoring it.

## Handoff and deferred work

The same Maven SNAPSHOT coordinate has existed with both invoke signatures. Build against a starter produced from the Framework source revision exposing `SkillInvocationResult`; record that revision with the engine verification. A successful build against a cached String-returning starter can mask the wrapper mismatch.

Use [PR 2.1.1](PR-2.1.1-console-home.md) for the client and [PR 2.1.2](PR-2.1.2-backend-readiness.md) for deployment. Hand [PR 2.1.5](PR-2.1.5-stage-and-validate.md), [PR 2.1.6](PR-2.1.6-first-publication.md) and [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the authenticated engine connection, observation/refresh entry points and known-good K3s snapshot check. They own Validate, Publish and Invoke respectively. [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns restoration and restart confirmation; [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns catalogs; [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns viewer subscriptions and trace indexing.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Combines identity, K3s discovery, network isolation and lifecycle transitions across services.
- **Reassessment triggers:** Reassess adapter scope if the moved client already implements parts of snapshot or stream handling.
