# Go backend phase 1 — One application, end to end

**Proposed boundary for review.** An owner signs in, creates an application, stages and publishes a small pack, and invokes its first skill through REST. Use a pack with one local skill, ordinary JSON input, and no variable references. This exercises the real Go-to-Java boundary early. The [Go backend roadmap](../future/go-backend-roadmap.md) and its linked contracts govern behavior across all four phases.

## What belongs here

- Move the existing Loomspan Console from the Framework repository into `loomspan-cloud`, including its Go code, browser UI, MCP and REST handlers, and relevant build and test assets. Establish the Cloud repository as the build home for both Full Cloud and the Console-only executable. Use the existing Console client and diagnostic code as the starting point for the in-process Cloud Console proxy. Framework continues to serve the engine-side Console endpoints. [Console profiles](../future/roadmap.md#console-deployment-profiles)
- Preserve the standalone diagnostic workflow during the move. Select profile-specific modules and access rules at startup. Full Cloud enables shared operations as their Cloud authorization and application scoping are implemented. [Navigation and modules](../future/roadmap.md#console-navigation-and-modules)
- Bring the single Go backend up on K3s with its mounted configuration, native Postgres connection, embedded migrations, readiness handling, and durable pack storage. Exercise startup with Postgres initially unavailable. [Backend configuration](../future/go-backend-roadmap.md#go-backend-configuration)
- Verify the signed-in user's Supabase token and check current application access in the shared Go operations. Establish normalized owner identity, repeatable application creation, and the distinction between a durable application ID and its assigned namespace. [Authentication](../future/roadmap.md#authentication) · [Application creation](../future/go-backend-roadmap.md#application-creation)
- Send the uploaded ZIP through the ordinary pack checks, preserve its exact bytes, create a staging revision, and obtain validation from Java. Publish that revision with the effective defaults and empty variable map. Persist the accepted request and the confirmed publication under the roadmap's transaction rules. [Upload and publication](../future/go-backend-roadmap.md#upload-and-staged-validation) · [Engine requests](../future/execution-engine-api.md#internal-api-requests)
- Discover the single engine, establish the authenticated Console observation, and route the owner's authorized invocation to the eligible namespace. Return the engine result and available session ID with execution views off. [Placement and routing](../future/roadmap.md#starting-point-for-placement-and-routing) · [Engine responses](../future/execution-engine-api.md#invoke-responses)

## Completion evidence

Build and test the moved Console from this repository. Run the Console-only executable, connect to a Framework target with its existing local access controls, and inspect a known run in the browser. In Full Cloud, demonstrate that the Go backend uses the moved Console client to read the engine snapshot for the REST publication and invocation below.

Starting with no application, repeat creation for the same owner and slug and obtain the same application ID. Demonstrate that an inaccessible application and an application awaiting its first publication cannot dispatch a skill. Stage the sample, publish it, and invoke it against the running Java engine. Trace the result back to the selected staging revision, stored ZIP, publication version, and observed namespace.

## Handoff

REST is the first caller of the shared operations. [Phase 2](go-backend-phase-2-publication-lifecycle.md) completes publication management, failure handling, and recovery. Phase 3 adds companion and integration access, and Phase 4 completes administration diagnostics and retention. The first slice establishes the permission, identity, durable-state, and engine boundaries those features depend on.

Review these four outlines before creating Phase 1 tickets. Draft Phase 2 tickets after the Phase 1 implementation lands, using its actual request record, storage layout, and engine integration tests.
