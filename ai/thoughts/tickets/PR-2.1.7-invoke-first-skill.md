# PR 2.1.7 — Invoke the first published skill through REST and prove the Phase 1 workflow

## Outcome

An owner starts with no application and ends with a real skill result and its Framework session ID. Trace the result from Cloud application ID through publication version to the engine namespace. Demonstrate the same path refuses work before dispatch when authorization or placement fails.

## Context

GitHub PR 2, Go backend Phase 1. Depends on PR 2.1.1–2.1.6. Use the moved Console client inside the deployed backend, the owner’s signed-in REST session, the staged revision and the real Java engine. This ticket closes Phase 1; its reproducible walkthrough becomes the reference for planning Phase 2.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Routing](../future/roadmap.md#starting-point-for-placement-and-routing) · [Invocation responses](../future/execution-engine-api.md#invoke-responses) · [Cloud limits and visibility](../future/go-backend-roadmap.md#invocation-response-visibility) · [Synchronous outcomes](../future/roadmap.md#synchronous-invocation)

## Requirements

- Implement the ordinary JSON REST invocation through the shared Cloud operation. Check current Caller access, enabled requests, and a durably current publication for this application ID. Before the first publication reject even if an empty or stale generation shares the namespace. Afterward use the assigned engine only while it is ready, connected, accepting requests, and observed to host the namespace. The engine chooses its active generation at admission; avoid pinning invocations by matching a publication version.
- Send the JSON invoke request over mTLS with the authorized namespace, caller-supplied local skill name, input object and Cloud-derived caller identity plus application skill roles (empty array if none). Apply the common 20 MiB inclusive body limit before dispatch. Phase 1 uses ordinary JSON with execution views off; omit the internal executionView request field. Return the exact result string, or engine message and available top-level validation issues; forward sessionId when supplied. With views off omit executionView, debug and diagnosis. Multipart/MCP, API keys, visibility settings administration and application-visible invocation logging are completed in Phase 3.
- An authorized disabled application receives 503 without Retry-After. Lack of eligible placement means rejected before execution. After dispatch a lost result means outcome unknown: Go must not automatically invoke again. Only an authenticated engine 503 establishes its confirmed pre-start rejection. Caller disconnection does not cancel work already started.
- Deliver repeatable end-to-end evidence against K3s, Supabase Auth, native Postgres, the durable volume and running Java engine. Use a portable sample with one skill, ordinary JSON, defaults and no variable references. Follow the migrated Console-only regression evidence from PR 2.1.1 as well as the Full Cloud workflow. The harness may reuse earlier tickets’ deployment and fixtures.

## Acceptance criteria

- [ ] Sign in, create and repeat creation, stage and validate original bytes, publish, then invoke through the public REST entry point. Demonstrate the same application ID, staging revision, accepted publication, observed namespace and returned result/session ID line up. Capture repeatable steps and expected outcomes without embedding credentials.
- [ ] An inaccessible application and an unpublished application produce zero Java dispatches. Exercise offline/terminating placement, disabled requests and body size rejection. A lost response after dispatch produces one engine invocation and an unknown outcome.
- [ ] With views off, verify success and failure visibility, including pre-session failure with no sessionId. Confirm real-engine integration and rerun the migrated standalone diagnostic smoke check from the Cloud checkout. Preserve the evidence needed for Phase 2 ticket authors.

## Handoff and deferred work

Consume the local handoffs from [PR 2.1.1](PR-2.1.1-console-home.md), [PR 2.1.2](PR-2.1.2-backend-readiness.md), [PR 2.1.3](PR-2.1.3-engine-observation.md), [PR 2.1.4](PR-2.1.4-owner-applications.md), [PR 2.1.5](PR-2.1.5-stage-and-validate.md), [PR 2.1.6](PR-2.1.6-first-publication.md) and close any outstanding Phase 1 verification. Use a seeded disabled setting for the rejection case if needed; [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns the management switch and full publication recovery. [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns multipart/MCP, integration keys, response-detail controls and the invocation log. [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns retained-trace lookup by the session ID returned here. Record the working walkthrough and fixture locations for Phase 2 ticket authors.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. At phase close, resolve any remaining cross-ticket build breaks, build both supported profiles, and run the accumulated Phase 1 acceptance checks.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Completes an externally visible authenticated execution path across multiple components and proves the phase outcome.
- **Reassessment triggers:** Reassess the integration harness if earlier local tickets already supply an equivalent end-to-end fixture.
