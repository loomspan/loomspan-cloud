# Go backend phase 2 — Publication lifecycle and recovery

**Proposed boundary for review.** Starting from [Phase 1’s working publication](go-backend-phase-1-first-application.md), an administrator can change what an application serves and determine what happened after an interruption. The owner can exercise these Manager operations before Phase 3 adds membership administration.

## What belongs here

- Complete the staging and publication workflows: identical uploads, validation results per revision, explicit revalidation, reviewed-revision confirmation, an already-published selection, and retained history and download by selector. Preserve the current publication across a failed candidate. [Publication workflow](../future/go-backend-roadmap.md#publishing-a-reviewed-revision) · [Pack history](../future/go-backend-roadmap.md#pack-history)
- Apply encrypted, write-only application variables and saved runtime settings to new publications, restarts, and restores. Exercise discovery against the shared fixtures, distinguish active settings from saved edits, and apply the enable/disable requests setting. [Application variables](../future/go-backend-roadmap.md#application-variables) · [Conformance cases](../future/application-variable-conformance.md) · [Application status](../future/go-backend-roadmap.md#application-status)
- Use the accepted request record for progress, outcomes, last confirmation, and recovery. Complete ordered dispatch, shutdown handling, late confirmation, same-version restart checks, and recovery across Go and engine replacement. Use the operations log for best-effort administrative history. [Unconfirmed publications](../future/go-backend-roadmap.md#unconfirmed-publications) · [Console recovery](../future/go-backend-roadmap.md#observe-live-engines)

## Completion evidence

Stage B while someone reviews A, publish A with confirmation, and verify B stays staged. Restart the published version after a settings edit and confirm the generation change. Exercise a lost publish reply, a late confirmation after a newer acceptance, and a Go restart with a request known to be unsent. Check the original request ID, the publication used for recovery, and what a new invocation can run. On engine replacement, verify one failed restore leaves other applications able to recover.

## Handoff

[Phase 3](go-backend-phase-3-access-and-companions.md) exposes these operations to companions and additional principals, and uses download and staging for promotion. Phase 4 applies byte quotas to history. Phase 2 establishes the protected ZIP references and application row locking that trimming will consume.

Draft Phase 2 tickets against the merged Phase 1 code. Use the resulting publication, download, and status interfaces to scope Phase 3 tickets.
