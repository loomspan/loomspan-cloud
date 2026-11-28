# Go backend phase 4 — Diagnostics, retention, and deletion

**Proposed boundary for review.** An application administrator can investigate a real invocation from [Phase 3](go-backend-phase-3-access-and-companions.md), manage retained data, and safely remove an application. This phase closes the backend portion of the [V1 demonstration](../future/v1-prototype-brief.md#proposed-v1-demonstration).

## What belongs here

- Scope live events by application, recheck subscription access at the reloadable interval, and report stream gaps. Rebuild retained history from finalized files and support authorized lookup and download by session ID after engine replacement. [Console proxy](../future/go-backend-roadmap.md#console-proxy) · [Retained traces](../future/go-backend-roadmap.md#look-up-retained-traces)
- Complete trace import, overwrite, expiry, and deletion. Apply the distinct Full Cloud and Console-only destination and retention rules to shared diagnostic operations. Implement reloadable quota defaults and overrides, save-time manual-upload checks, and quota-screen measurements. [Trace import](../future/roadmap.md#opening-a-trace-file) · [Quota visibility](../future/go-backend-roadmap.md#quota-visibility)
- Run the five-minute retention pass. The job handles manual expiry and engine traces and calls Go once per eligible application for pack trimming. Reuse the publication protections and short row locks established in Phase 2. [Retention job](../future/go-backend-roadmap.md#trace-cleanup-job) · [Pack protection](../future/go-backend-roadmap.md#pack-history)
- Complete owner-requested deletion, including management-write admission, durable Deleting state, empty-pack installation, drain or confirmed engine termination, direct file cleanup, and release of the slug after success. Keep Go restart during deletion distinct from engine termination. [Deletion workflow](../future/go-backend-roadmap.md#application-permissions) · [Engine drain signal](../future/execution-engine-api.md#namespace-drain-observation)

## Completion evidence

Open the same trace in the moved Console's Full Cloud and Console-only profiles. Verify the shared diagnostic screens and navigation work in both, each enforces its own access rules, and Console-only exposes only its enabled modules. [Console navigation acceptance](../future/roadmap.md#console-navigation-and-modules)

Find one of several concurrent retained runs using its caller's session ID with execution views off. Revoke a viewer's access during a live subscription and verify other authorized viewers continue. Exercise quota changes, a skipped namespace, and pack trimming racing with Publish. Delete an application with admitted work, verify cleanup waits for the required evidence, then recreate the same slug. Resume a paused retention pass and verify the fresh application's files and measurements survive.

## Handoff

Draft Phase 4 tickets from the merged invocation, permission, publication, and filesystem code. Use the V1 demo and the linked contracts for final backend verification. The private engine namespace, publication version, root session ID, and Cloud application ID each retain their established roles throughout the demonstration.
