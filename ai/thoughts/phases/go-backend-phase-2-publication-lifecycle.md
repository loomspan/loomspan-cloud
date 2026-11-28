# Go backend phase 2 — Publication lifecycle and recovery

**Proposed boundary for review.** Starting from [Phase 1’s working publication](go-backend-phase-1-first-application.md), an administrator can change what an application serves and determine what happened after an interruption. The owner can exercise these Manager operations before Phase 3 adds membership administration.

## What belongs here

- Add the Manager-only latest-operation lookup by application ID and the full application status and history views. Show serving status, the most recently accepted Publish or Restart with its request ID, target version and progress or outcome, and the last confirmation time. Reuse Phase 1’s by-ID GET for individual requests and extend it to Restart. [Application status](../future/go-backend-roadmap.md#application-status)
- When introducing saved mission timeouts, verify the synchronous Invoke transport, Go server and ingress budgets accommodate supported mission durations and response delivery. Phase 1 sizes those budgets for the 60-second mission default.

- Complete the staging and publication workflows: identical uploads, validation results per revision, explicit revalidation, reviewed-revision confirmation, an already-published selection, and retained history and download by selector. Preserve the current publication across a failed candidate. [Publication workflow](../future/go-backend-roadmap.md#publishing-a-reviewed-revision) · [Pack history](../future/go-backend-roadmap.md#pack-history)
- Apply encrypted, write-only application variables and saved runtime settings to new publications, restarts, and restores. Reuse the fixture-verified Go discovery from [PR 2.1.5](../tickets/PR-2.1.5-stage-and-validate.md) to look up the target application’s saved values. Distinguish active settings from saved edits, and apply the enable/disable requests setting. [Application variables](../future/go-backend-roadmap.md#application-variables) · [Conformance cases](../future/application-variable-conformance.md) · [Application status](../future/go-backend-roadmap.md#application-status)
- Use the accepted request record for progress, outcomes, last confirmation, and recovery. Extend Phase 1’s Publish ordering and confirmation rules to same-version application restarts, including overlap with Publish. After Go restarts, close requests known to be unsent and reconcile possibly dispatched requests against fresh observations before restoring durable intent. On engine replacement, restore and confirm each intended publication. Use the operations log for best-effort administrative history. [Unconfirmed publications](../future/go-backend-roadmap.md#unconfirmed-publications) · [Console recovery](../future/go-backend-roadmap.md#observe-live-engines)

## Completion evidence

Lose the caller’s response before it receives a request ID. With Manager access, reload the application status, find the latest accepted operation and use its ID to read that request. Verify that a later confirmation of an older request updates its outcome and the last confirmation time while the latest-operation lookup still returns the newer accepted request.
Publish the worked variable fixture in two private namespaces with different saved values for the same names. Verify the provider and executor receive their own application’s values. Rotate one application’s value and Restart with the same ZIP and publication version. Verify the new generation uses the rotated value, already admitted work retains its original value, and the other application keeps its own values. Restore on a replacement engine and verify each application again.

Stage B while someone reviews A, publish A with confirmation, and verify B stays staged. Restart the published version after a settings edit and confirm the generation change. Exercise a lost restart reply and check the same-JVM generation evidence. Restart Go with a request known to be unsent and, separately, with one that may have reached Java. Check the original request ID, the publication used for recovery, and what a new invocation can run. On engine replacement, verify one failed restore leaves other applications able to recover.

## Handoff

[Phase 3](go-backend-phase-3-access-and-companions.md) exposes these operations to companions and additional principals, and uses download and staging for promotion. Phase 4 applies byte quotas to history. Phase 2 establishes the protected ZIP references and application row locking that trimming will consume.

Draft Phase 2 tickets against the completed Phase 1 code. [PR 2.1.6](../tickets/PR-2.1.6-first-publication.md) supplies same-process Publish-to-Publish ordering, including confirmation after caller timeout and release of a held dispatch at the deadline. Reuse its request records and tests for the cross-restart and replacement scenarios above. Use the resulting publication, download, and status interfaces to scope Phase 3 tickets.
