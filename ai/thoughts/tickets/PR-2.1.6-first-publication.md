# PR 2.1.6 — Publish the selected staged revision and durably confirm its installation

## Outcome

A Manager explicitly publishes the chosen current staged revision. Java installs it into the application’s private namespace, and the REST caller receives a stable request ID and confirmed version only after Go has durable evidence of success.

## Context

GitHub PR 2, Go backend Phase 1. Depends on PR 2.1.3–2.1.5 for authenticated observation and Java calls, application authorization, staging, immutable ZIP references, validation and effective settings. This slice establishes the first successful publication. Phase 2 completes historical-revision confirmation, same-version restart and interruption recovery.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Reviewed publication](../future/go-backend-roadmap.md#publishing-a-reviewed-revision) · [Row lock and ZIP protection](../future/go-backend-roadmap.md#pack-history) · [Confirmation and retry semantics](../future/go-backend-roadmap.md#unconfirmed-publications) · [Engine wire contract](../future/execution-engine-api.md#internal-api-requests)

## Requirements

- Check current Manager access and bind the request to the caller-selected staged revision. Never silently substitute newer staging. If staging changed, return the warning identifying both revisions and stop before accepting or dispatching the request. Phase 2 adds the user-confirmed path for a noncurrent selection. Selecting the application's currently published revision returns the existing no-op result before fresh validation, with no new version or request ID.
- For a new installation, use a short native Postgres transaction and SELECT ... FOR UPDATE on the application row, verify the ZIP exists, and persist the accepted request with its ID, operation, actor, target version and protected ZIP reference before dispatch. Release the lock before network calls. A missing trimmed ZIP requires re-upload. Use the same request record for progress, outcome and confirmation time; the current-publication pointer is distinct.
- Run fresh validation and fix the revision, required values and complete effective settings in memory for this attempt. Missing or blank references block installation with source/path feedback. The reference-free first pack uses an explicit empty variableValues map. POST /internal/v1/publish over mTLS with multipart pack and metadata carrying namespace, runtimeSettings and a Cloud-assigned opaque publicationVersion. Require a current authenticated snapshot from the intended engine, and stop new installs to a terminating pod.
- For a new publication Java’s 204 confirms installation. Record success and advance the current pointer in the same transaction only if no newer publish or restart was accepted. Request a fresh observation for routing. Preserve the old pointer on failure and pass through a known engine diagnostic. On a lost reply, refresh and apply the 30-second observation rule; report an unconfirmed request ID if unresolved. Never internally resend. The 60-second hold/reconciliation scenarios, full status views and restart management belong to Phase 2; preserve the request evidence they require.

## Acceptance criteria

- [ ] Publish the current validated pack to the real engine and verify stable request ID, durable confirmed version, exact ZIP reference and a matching fresh namespace observation. Verify repeating Publish for the confirmed revision is a no-op.
- [ ] A validation failure, missing required value, stale selection or absent eligible engine cannot accidentally install another revision or change the prior current pointer. An engine failure returns its diagnostic.
- [ ] Verify the commit precedes success reporting, an older confirmation cannot overwrite a newer accepted operation’s pointer, and a lost reply never produces a second install from Go. The pending outcome remains distinguishable from known failure.

## Handoff and deferred work

Use [PR 2.1.3](PR-2.1.3-engine-observation.md), [PR 2.1.4](PR-2.1.4-owner-applications.md) and [PR 2.1.5](PR-2.1.5-stage-and-validate.md). Hand [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the confirmed publication lookup, snapshot refresh, and reproducible REST Publish. [Phase 2](../phases/go-backend-phase-2-publication-lifecycle.md) owns the historical-revision confirmation round trip, same-version restart, 60-second dispatch hold, backend-restart reconciliation and full status views. Phase 1 records the request evidence and handles a lost reply during the original call.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Couples a remote installation with durable request history, transaction ordering and ambiguous outcomes.
- **Reassessment triggers:** Reassess ticket boundaries if a required Phase 1 guarantee depends on a Phase 2 recovery feature; preserve the published contract.
