# Go backend phase 3 — Access, companions, and integrations

**Proposed boundary for review.** A companion can take a user from sign-in through a real invocation using the operations established in [Phase 2](go-backend-phase-2-publication-lifecycle.md). A second user and an integration can invoke the same application with their own identities and permissions.

## What belongs here

- Extend the Console code moved in Phase 1 with Full Cloud browser and MCP identity and application scoping. Register Cloud management modules through the profile selection so their menu entries and routes appear together. Console-only retains its local credentials and diagnostic modules. [Console profiles and navigation](../future/roadmap.md#console-navigation-and-modules)
- Complete owner-managed memberships, Cloud roles, application-scoped skill roles, API key issuance and rotation, and logging of access changes. Apply current permissions across UI-backed operations, REST, and MCP. Forward the appropriate identity and skill roles to Framework on each invocation. [Permissions](../future/go-backend-roadmap.md#application-permissions) · [Application API keys](../future/go-backend-roadmap.md#application-api-keys)
- Implement companion browser sign-in, token verification, and application discovery. Expose pack transfer, validation, reviewed publication, status, restart, and promotion through the shared operations. Promotion checks both applications and imports through ordinary destination staging. [Companion sign-in](../future/go-backend-roadmap.md#companion-sign-in) · [Pack transfer acceptance](../future/go-backend-roadmap.md#pack-history) · [Promotion](../future/go-backend-roadmap.md#publication-promotion)
- Serve authenticated invocation documentation from the generation-identified namespace catalog, including pre-publication and offline behavior. Complete REST and MCP invocation, file mappings, shared limits, caller response visibility, and application invocation logging. [Invocation documentation](../future/go-backend-roadmap.md#invocation-documentation) · [Catalog](../future/go-backend-roadmap.md#namespace-skill-catalog) · [Response visibility](../future/go-backend-roadmap.md#invocation-response-visibility)

## Completion evidence

Use the documented signed-in and signed-out companion flows. Exercise the required byte-for-byte pack round trip and promote a retained publication between applications. Give a second user Caller access, invoke through their companion, and verify management access is denied. Rotate an integration key and verify the backend identity stays stable. Exercise a multipart call at the shared limits and verify caller-visible details follow the administrator's switch.

## Handoff

The session ID returned here is the lookup key for [Phase 4 diagnostics](go-backend-phase-4-diagnostics-and-deletion.md). Phase 3 covers the invoker's own response. Phase 4 applies diagnostic permissions to live subscriptions, retained history, and trace imports. Full administration screen design remains with the UI workstream.

Draft Phase 3 tickets after Phase 2 lands. Use the resulting identities, authorization checks, catalog observations, and real invocation results when planning Phase 4.
