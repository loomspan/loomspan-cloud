# PR 2.1.7 — Invoke the first published skill through REST and prove the Phase 1 workflow

## Outcome

An owner starts with no application and ends with a real skill result and its Framework session ID. Trace the result using the same namespace name in the Cloud request and engine observation, together with the publication version. Demonstrate the same path refuses work before dispatch when authorization or placement fails.

## Context

GitHub PR 2, Go backend Phase 1. Depends on PR 2.1.1–2.1.6. Use the moved Console client inside the deployed backend, the owner’s locally signed REST test token, the staged revision and the real Java engine. This ticket closes Phase 1; its reproducible walkthrough becomes the reference for planning Phase 2.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Routing](../future/roadmap.md#starting-point-for-placement-and-routing) · [Invocation responses](../future/execution-engine-api.md#invoke-responses) · [Cloud limits and visibility](../future/go-backend-roadmap.md#invocation-response-visibility) · [Synchronous outcomes](../future/roadmap.md#synchronous-invocation)

## Requirements

Implement this ticket’s public routes, request bodies, response fields and statuses from the [Phase 1 Cloud REST API](../future/phase-1-rest-api.md).

Give the outbound synchronous Invoke call a 75-second total budget, including the wait for response headers. Configure its transport to allow that wait beyond the Console diagnostic client's 10-second header and 30-second request defaults. PR 2.1.2 supplies the 90-second caller-facing budget. These budgets cover Phase 1's 60-second mission default. Publish confirmation and Console streams retain their own timing rules.

- Consume the Java wrapper's top-level `sessionId` from PR 2.1.3. The Framework supplies the canonical ID independently of execution views. Verify that REST preserves the same ID on success and post-session failure and omits it on a pre-session failure.
- Implement the ordinary JSON REST invocation through the shared Cloud operation. Check current Caller access, enabled requests, and a durably current publication for this namespace. Before the first publication reject even if an empty or stale generation shares the namespace. Use PR 2.1.3’s existing in-memory observations: the assigned pod has Kubernetes `Ready=True` and no `metadata.deletionTimestamp`, and its current authenticated Console connection has observed the target namespace. Go needs neither a separate `accepting` flag nor a pre-dispatch engine poll. The engine chooses its active generation at admission; avoid pinning invocations by matching a publication version.
- Build the engine's [caller object](../future/execution-engine-api.md#invocation-identity-and-roles) from the authenticated REST session. Set `issuer` to the literal `"loomspan-cloud"`. Use the PR 2.1.4 email normalizer on the signed-in caller's verified email for `subject` (trim surrounding whitespace and lowercase the entire address). Phase 1 always sends `"roles": []`, including for the owner. Phase 3 introduces application-scoped skill-role assignments.
- Send the JSON invoke request over mTLS with the authorized namespace, caller-supplied local skill name, input object and caller object. Apply the common 20 MiB inclusive body limit before dispatch. Phase 1 uses ordinary JSON with execution views off; omit the internal executionView request field. Return the exact result string, or engine message and available top-level validation issues. Preserve the engine’s top-level `sessionId` under the success and failure rules above. With views off omit executionView, debug and diagnosis. Multipart/MCP, API keys, visibility settings administration and application-visible invocation logging are completed in Phase 3.
- An authorized disabled application receives 503 without Retry-After. Lack of eligible placement means rejected before execution. After dispatch a lost result means outcome unknown: Go must not automatically invoke again. Only an authenticated engine 503 establishes its confirmed pre-start rejection. Caller disconnection does not cancel work already started.
- Deliver repeatable end-to-end evidence against K3s, the normal token verifier configured with the test issuer, native Postgres, the durable volume and running Java engine. Use a portable sample with one skill, ordinary JSON, defaults and no variable references. Follow the migrated Console-only regression evidence from PR 2.1.1 as well as the Full Cloud workflow. The harness may reuse earlier tickets’ deployment and fixtures.

## Acceptance criteria

- [ ] Invoke as an authorized Caller with the application row seeded as `Deleting`, requests enabled, a current publication and an eligible engine hosting the namespace. Return `409 Conflict` with reason `Deleting` before routing. Verify zero engine dispatches. Previously admitted invocations may drain.

- [ ] With an eligible observation already established, verify Invoke goes directly to the engine with zero pre-dispatch probes or snapshot reads. Disconnect Console and verify Invoke is rejected before dispatch until a fresh authenticated observation on reconnect shows the namespace again.

- [ ] Through ingress, obtain a successful engine response delayed more than 30 seconds but within the mission deadline.
- [ ] Through ingress, send valid caller JSON containing a single large ASCII input string. Size the serialized public body to exactly 20,971,520 bytes before Go adds namespace and caller. Invoke a real deterministic skill that accepts that input and returns a small known result. Assert HTTP 200, the expected result and a nonblank Framework session ID. Extend the string by one ASCII byte and repeat with valid JSON of 20,971,521 bytes. Assert Go returns the documented 413 rejection before dispatch, with zero engine calls for the oversized request.

For the deployed walkthrough, reuse PR 2.1.4’s test issuer and fixture to mint a fresh owner token. Send it as a bearer token on ordinary REST requests and capture sanitized results. The live Supabase email-code sign-in belongs to the post-V1 test suite.

- [ ] With a verified owner email of `Owner@Example.com`, assert that the outbound engine JSON contains exactly this caller object: `{"issuer":"loomspan-cloud","subject":"owner@example.com","roles":[]}`.
- [ ] Authenticate with the locally signed owner token, create and repeat creation, stage and validate original bytes, publish, then invoke through the public REST entry point. Verify the Cloud request and engine observation name the same namespace. Link its staged revision and confirmed publication to the returned result and session ID. Capture repeatable steps and expected outcomes without embedding credentials.
- [ ] An inaccessible application and an unpublished application produce zero Java dispatches. Exercise offline/terminating placement, disabled requests and body size rejection. A lost response after dispatch produces one engine invocation and an unknown outcome.
- [ ] With views off, the successful real-engine walkthrough returns a nonblank top-level `sessionId` through REST. Verify that Go forwards the engine’s ID verbatim for the same invocation, including on a post-session failure, and omits the field on a pre-session failure. An absent ID on the successful walkthrough fails the phase gate. Rerun the migrated standalone diagnostic smoke check and preserve the evidence for Phase 2.

- [ ] Use the same real-engine post-session failure with the default `ONERROR` policy. On PR 2.1.3’s shared trace mount, find `<canonical entry namespace>/<sessionId>.<traceId>.execution-trace.ndjson` using the session ID returned by REST. Record the completed file’s relative path and digest. Replace the engine pod, verify its pod UID changed, and verify the same file and digest on its trace mount.

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
