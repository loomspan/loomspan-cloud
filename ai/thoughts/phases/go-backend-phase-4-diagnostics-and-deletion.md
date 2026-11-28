# Go backend phase 4 — Diagnostics, retention, and deletion

**Proposed boundary for review.** An application administrator can investigate a real invocation from [Phase 3](go-backend-phase-3-access-and-companions.md), manage retained data, and safely remove an application. This phase closes the backend portion of the [V1 demonstration](../future/v1-prototype-brief.md#proposed-v1-demonstration).

## What belongs here

- Scope live events by application, recheck subscription access at the reloadable interval, and report stream gaps. Rebuild retained history from finalized files and support authorized lookup and download by session ID after engine replacement. [Console proxy](../future/go-backend-roadmap.md#console-proxy) · [Retained traces](../future/go-backend-roadmap.md#look-up-retained-traces)
- Complete trace import, overwrite, expiry, and deletion. Apply the distinct Full Cloud and Console-only destination and retention rules to shared diagnostic operations. Use the effective quotas and per-application skip state from [PR 2.1.2](../tickets/PR-2.1.2-backend-readiness.md) for retention, save-time manual-upload checks and quota-screen reads. [Trace import](../future/roadmap.md#opening-a-trace-file) · [Quota visibility](../future/go-backend-roadmap.md#quota-visibility)
- Run the five-minute retention pass. The job handles manual expiry and engine traces and calls Go once per eligible application for pack trimming. Reuse the publication protections and short row locks established in Phase 2. [Retention job](../future/go-backend-roadmap.md#trace-cleanup-job) · [Pack protection](../future/go-backend-roadmap.md#pack-history)
- Complete owner-requested deletion, including management-write admission, durable Deleting state, empty-pack installation, drain or confirmed engine termination, direct file cleanup, and release of the slug after success. Keep Go restart during deletion distinct from engine termination. [Deletion workflow](../future/go-backend-roadmap.md#application-permissions) · [Engine drain signal](../future/execution-engine-api.md#namespace-drain-observation)

## Completion evidence

Open the same trace in the moved Console's Full Cloud and Console-only profiles. Verify the shared diagnostic screens and navigation work in both, each enforces its own access rules, and Console-only exposes only its enabled modules. [Console navigation acceptance](../future/roadmap.md#console-navigation-and-modules)

Find one of several concurrent retained runs using its caller's session ID with execution views off. Change the permission-check interval and verify open and new subscriptions use it without reconnecting. Revoke a viewer's access and verify other authorized viewers continue. Verify a skipped application receives 503 on manual uploads, overwrites and quota reads, its files survive retention, and other applications continue. Correct the override and verify processing resumes. Exercise quota changes and pack trimming racing with Publish.

Populate two applications, A and B, across every application data root on the shared volume. Include retained pack ZIPs, manually uploaded traces, engine-written traces, invocation and operations logs, and skill catalogs for each. Record B's file paths and contents. With work admitted for A, request its deletion through Go and verify file cleanup waits for confirmed drain or engine termination. After deletion, inspect each root directly. Verify A's pack directory and engine-trace directory are gone, no files belonging to A remain in any root, and every recorded B file is still present with identical bytes.

Recreate A with the same owner and slug and the same namespace as its application ID. Resume a retention pass paused after selecting files for the original A, deliver its usage report, and verify the recreated application's files survive and its quota screen excludes the old measurements. [Slug reuse during retention](../future/go-backend-roadmap.md#trace-cleanup-job)

## Handoff

Draft Phase 4 tickets from the merged invocation, permission, publication, and filesystem code. Use the V1 demo and the linked contracts for final backend verification. Use the same namespace name in Cloud requests and engine observations throughout the demonstration. Identify the publication by its version and the run by its root session ID.
