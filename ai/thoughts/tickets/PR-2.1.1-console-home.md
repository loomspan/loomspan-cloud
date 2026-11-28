# PR 2.1.1 — Build and run Loomspan Console from loomspan-cloud

## Outcome

A developer builds Console from the Cloud checkout and inspects an existing run locally. The same Console code can live inside Full Cloud. This establishes the implementation that later tickets extend and avoids building a second engine client.

## Context

GitHub PR 2, Go backend Phase 1. First ticket. The source of the existing Go module is `loomspan-console/` in the sibling `loomspan-framework` repository. Use the Console revision available to this work and identify it in the handoff so reviewers can compare behavior. The Java engine and its Framework observability endpoints remain on the engine side.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Deployment profiles](../future/roadmap.md#console-deployment-profiles) · [Navigation and modules](../future/roadmap.md#console-navigation-and-modules) · [Go components](../future/go-backend-roadmap.md#go-backend-components)

## Requirements

- Move the Go sources, browser UI, REST and MCP handlers, and the build, packaging, fixtures and tests they depend on into this repository. Resolve build-time assets and module imports so building the moved application does not require the old Console source tree. Preserve the standalone executable and existing diagnostic workflows. Framework remains an independently usable host.
- Select enabled modules and access rules in one place per profile. Console-only serves its browser, REST and MCP interfaces on loopback; preserve browser pairing, the local MCP credential, and verified authenticated HTTPS to remote targets. Module routes and navigation entries travel together.
- Full Cloud has one Go process containing the Cloud API, proxy, placement controller and router. Establish its in-process composition point using the moved Console code. Expose Cloud operations as subsequent tickets add Cloud authorization and namespace scoping; keep standalone access paths out of the Cloud deployment. Cloud sign-in, publication and new diagnostics belong to later tickets.

## Acceptance criteria

- [ ] Build the standalone executable and its UI from the Cloud checkout; the moved Console tests and build steps run there.
- [ ] Start Console-only, connect to a Framework target and inspect a known run in the browser. Existing local REST and MCP diagnostics still use their respective credentials. Verify loopback binding and certificate verification.
- [ ] Build the Full Cloud entry point from the same checkout; its startup wiring uses the moved Go packages without launching a separate Console server or exposing standalone diagnostic routes. Console-only has no Cloud management routes or empty menus.

## Handoff and deferred work

Start in the sibling module’s README, AGENTS.md and `internal/buildtool`. Its canonical commands are `go run ./internal/buildtool verify` and `go run ./internal/buildtool build`. The build currently reads the Framework root POM for the version used by the exact Console/target compatibility check. Carry the pinned toolchain, locked frontend, asset integrity checks and version provenance into the Cloud build. Verify a compatible target still connects and a mismatched target is rejected without requiring the old checkout to build the executable.

Hand off the moved module path, source revision, build commands and known-run fixture to [PR 2.1.2](PR-2.1.2-backend-readiness.md) and [PR 2.1.3](PR-2.1.3-engine-observation.md). [PR 2.1.2](PR-2.1.2-backend-readiness.md) owns deployable Cloud startup; [PR 2.1.3](PR-2.1.3-engine-observation.md) owns the first authenticated Cloud snapshot; [PR 2.1.4](PR-2.1.4-owner-applications.md) owns Cloud owner authentication. [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) reruns the standalone smoke check. [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns Cloud companion and management integration; [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns the cross-profile trace review.

## Pipeline notes

If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Cross-repository migration affects build, distribution, existing interfaces and trust boundaries.
- **Reassessment triggers:** Reassess the move boundary if a dependency is also used by the Framework engine or another host.
