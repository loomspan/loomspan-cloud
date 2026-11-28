# PR 2.1.4 — Create an application under a verified owner through REST

## Outcome

A real signed-in owner can create an application and safely repeat the call after losing the response. Later uploads target the returned immutable application ID and use its namespace as the ZIP root.

## Context

GitHub PR 2, Go backend Phase 1. Use the Full Cloud process from PR 2.1.1 and native Postgres/migrations from PR 2.1.2. Engine observation is independent. Use shared Go authorization operations so future UI and MCP handlers inherit identical decisions.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Supabase authentication](../future/roadmap.md#authentication) · [Creation contract](../future/go-backend-roadmap.md#application-creation) · [Application permissions](../future/go-backend-roadmap.md#application-permissions)

## Requirements

- Use the installation’s Supabase email-code sign-in and a real user access token for the REST workflow. Verify asymmetric signature against the trusted issuer’s keys, issuer, intended audience and expiry locally. Use verified email for identity. One exported normalizer trims surrounding whitespace and lowercases the entire address. Cloud verifies current access on each ordinary request.
- Get or create the stable database-generated numeric owner ID by normalized email, including under concurrency. Retain it across application lifetimes. Persist an application row with ID, owner, slug, namespace, creation time and lifecycle state. Enforce owner/slug uniqueness. Different owners may reuse the same slug. Enforce package naming rules and the 61-character maximum on the full u{numericUserId}-{appSlug}; report 400 and the maximum slug length for this prefix when exceeded.
- Return ID and namespace for new and existing applications and identify an existing result. Preserve existing settings, memberships and publication. A Deleting row reserves its slug and yields 409 with its ID and state. New applications have requests enabled, no current publication, execution views off and the default ONERROR persistence policy.
- Provide the common application permission check for subsequent tickets. Owner inherits Manager and Caller. Inaccessible and missing applications give the same 404 to an authenticated user, insufficient role gives 403, and unreadable access data gives 503 before any write or dispatch. Membership management, companion OAuth and API keys belong to Phase 3.

## Acceptance criteria

- [ ] Use an email-code session to create an application via REST. Verify untrusted, expired, or wrong-audience tokens cannot create or access it; the caller cannot choose another owner by supplying an email.
- [ ] Repeat and race creates for the same owner/slug and verify one application and stable namespace; another owner can use the slug independently. Existing state survives repeat creation, including the 409 behavior for a seeded Deleting row.
- [ ] Verify namespace boundaries, owner normalization, creation defaults and access-check outcomes, including the unavailable-database case.

## Handoff and deferred work

Use [PR 2.1.1](PR-2.1.1-console-home.md) and [PR 2.1.2](PR-2.1.2-backend-readiness.md) as the local baseline. Give [PR 2.1.5](PR-2.1.5-stage-and-validate.md), [PR 2.1.6](PR-2.1.6-first-publication.md) and [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the REST sign-in exercise, creation operation and shared permission-check entry points. [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) repeats creation with a real session at phase close. [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns membership administration, companion OAuth and integration keys. [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) drives actual deletion; this ticket can exercise the reserved-slug rule using a seeded Deleting row.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Introduces durable identity, access controls and concurrent creation semantics.
- **Reassessment triggers:** Reassess the sign-in integration if the migrated browser assets already provide part of the flow.
