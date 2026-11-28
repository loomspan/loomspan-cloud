# PR 2.1.6 — Publish the selected staged revision and durably confirm its installation

## Outcome

A Manager explicitly publishes the chosen current staged revision. The REST caller receives a stable request ID for every accepted Publish, including a failed attempt. On success, Go also returns the confirmed version after recording installation in the application’s private namespace.

## Context

GitHub PR 2, Go backend Phase 1. Depends on PR 2.1.3–2.1.5 for authenticated observation and Java calls, application authorization, staging, immutable ZIP references, validation and effective settings. This ticket covers initial publication and overlapping Publishes for one application within the lifetime of a Go process. Phase 2 extends that behavior to application restarts and recovery across process or engine replacement.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Reviewed publication](../future/go-backend-roadmap.md#publishing-a-reviewed-revision) · [Row lock and ZIP protection](../future/go-backend-roadmap.md#pack-history) · [Confirmation and retry semantics](../future/go-backend-roadmap.md#unconfirmed-publications) · [Engine wire contract](../future/execution-engine-api.md#internal-api-requests)

## Requirements

- After checking Manager access, compare the caller’s selected revision ID with the application’s current staged revision ID. A mismatch ends the Phase 1 request with a warning naming both IDs, even if the selected revision is already published. Only a match proceeds to the current-publication check: if that revision is already published, return “This revision is already published.” Both early returns precede acceptance, fresh validation and Java dispatch.
- Phase 1 takes only the selected revision ID for this decision. Leave the confirmation parameter and confirmed publication of a noncurrent revision to Phase 2. The comparison uses the current staged revision read by Go; it requires no caller-supplied staging ETag or precondition token.
- For a new installation, lock the application row with `SELECT ... FOR UPDATE`, verify the ZIP exists, and commit the request ID, operation, actor, target version and protected ZIP reference. A missing ZIP requires re-upload and leaves the request unaccepted. Commit before checking engine eligibility, checking required variable values, or running fresh validation, even if Go already knows the engine is down.
- If an accepted attempt fails before install dispatch, persist the terminal failure and specific reason on its request row and return its ID and diagnostic. This includes an invalid pack, missing or blank variable, incomplete validation, missing current snapshot, or observed shutdown. The failed Publish remains visible by ID and as the latest operation until superseded. Release this request’s ZIP protection, preserve staging and the current publication, and exclude the failed target from confirmed pack history.
- Run fresh validation and fix the revision, required values and complete effective settings in memory for this attempt. Missing or blank references block installation with source/path feedback. The reference-free first pack uses an explicit empty variableValues map. POST /internal/v1/publish over mTLS with multipart pack and metadata carrying namespace, runtimeSettings and a Cloud-assigned opaque publicationVersion. Require a current authenticated snapshot from the intended engine, and stop new installs to a terminating pod.
- For a new publication Java’s 204 confirms installation. Record success and advance the current pointer in the same transaction only if no newer publish or restart was accepted. Request a fresh observation for routing. Preserve the old pointer on failure and pass through a known engine diagnostic. On a lost install reply, immediately refresh the intended engine’s authenticated snapshot. A matching namespace and target version confirm the installation. After 30 seconds without confirmation, return the original request ID and explain that installation may still complete. The running Go process continues checking fresh observations after the caller’s wait ends. Never resend the original install.
- If Go accepts Publish B for the same application while Publish A is unconfirmed, persist B with its own request ID. Hold B’s install dispatch until A is confirmed or 60 seconds have elapsed from the start of A’s confirmation wait, whichever comes first. Releasing B at the deadline leaves A unconfirmed. If A later confirms, record success on A’s row and apply the current-pointer transaction rule even if B is still awaiting dispatch.

## Acceptance criteria

- [ ] Publish A while A is current in staging, then select A again: the second call returns the already-published no-op. Stage B and select A again: this time the response warns that A was selected and B is current in staging. Repeating that selection while B remains staged returns the same warning. Verify both early-return paths leave publication and staging unchanged, allocate no request ID or target version, and make zero Java calls.
- [ ] Publish the current validated pack to the real engine and verify stable request ID, durable confirmed version, exact ZIP reference and a matching fresh namespace observation. Verify repeating Publish for the confirmed revision is a no-op.
- [ ] With A still staged and durably current, supply a fresh replacement-JVM snapshot missing A’s application namespace. Selecting A again still returns “This revision is already published.” Verify zero Java calls, no request ID or target version, and the same current-publication pointer. The missing namespace affects routing eligibility. Phase 2 handles restoration.
- [ ] Start Publish with the engine already disconnected, and separately with the pod already marked terminating. Each eligible selection yields an accepted request ID and a recorded failed Publish stating the corresponding reason. Verify the same ID and reason in the response and durable request record, zero install calls, and an unchanged current publication.
- [ ] Exercise invalid, missing-variable and unavailable-validation cases after acceptance. Each leaves a terminal diagnostic on its own request and releases its outstanding ZIP protection. A stale-selection warning, already-published no-op, denied access, or ZIP removed before acceptance creates no accepted request or target version.
- [ ] Lose A’s reply and let its 30-second caller wait expire. With no newer acceptance, deliver a fresh matching snapshot and verify the same request becomes successful and current without a second install. Verify the success transaction commits before any success is reported.
- [ ] Starting from current publication P, accept B while A is unconfirmed. Exercise both release conditions. If A confirms at 45 seconds, record A’s success, keep P current because B was accepted, and release B immediately. If A stays unconfirmed, verify B can dispatch at 60 seconds measured from A’s original wait. In that case confirm B, then deliver late confirmation for A: both request rows show success, B stays current, and A was dispatched only once.

## Handoff and deferred work

Use [PR 2.1.3](PR-2.1.3-engine-observation.md), [PR 2.1.4](PR-2.1.4-owner-applications.md) and [PR 2.1.5](PR-2.1.5-stage-and-validate.md). Hand [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the confirmed publication lookup, snapshot refresh, and reproducible REST Publish. [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns the historical-revision confirmation round trip, same-version application restarts and their ordering with Publish, reconciliation after Go restarts, restoration on engine replacement, and full status/history views. Hand Phase 2 the durable request records and same-process overlap tests from this ticket.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Couples a remote installation with durable request history, transaction ordering and ambiguous outcomes.
- **Reassessment triggers:** Reassess ticket boundaries if a required Phase 1 guarantee depends on a Phase 2 recovery feature; preserve the published contract.
