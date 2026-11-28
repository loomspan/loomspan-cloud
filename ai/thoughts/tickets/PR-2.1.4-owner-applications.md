# PR 2.1.4 — Create an application under a verified owner through REST

## Outcome

An authenticated owner can create an application and safely repeat the call after losing the response. Later uploads target the returned immutable application ID and use its namespace as the ZIP root.

## Context

GitHub PR 2, Go backend Phase 1. Use the Full Cloud process from PR 2.1.1 and native Postgres/migrations from PR 2.1.2. Engine observation is independent. Use shared Go authorization operations so future UI and MCP handlers inherit identical decisions.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Supabase authentication](../future/roadmap.md#authentication) · [Creation contract](../future/go-backend-roadmap.md#application-creation) · [Application permissions](../future/go-backend-roadmap.md#application-permissions)

## Requirements

Implement this ticket’s public routes, request bodies, response fields and statuses from the [Phase 1 Cloud REST API](../future/phase-1-rest-api.md).

- Accept the installation’s Supabase user access token through the public REST bearer-token path. Verify the asymmetric signature against the trusted issuer’s keys, the issuer, expiry, and audience locally. For browser and REST user sessions, require `aud` to equal `authenticated` or contain that exact value in an array. This fixed value requires no installer setting. Phase 3 uses the canonical public MCP endpoint URL as the expected audience for MCP requests. Require top-level `is_anonymous` to be the JSON boolean `false` and top-level `email` to be a string. Reject missing or malformed identity claims, anonymous users and empty normalized addresses with `401 Unauthorized` before any application lookup or write. Ignore `user_metadata` for identity and verification. Rely on the installation’s email-verifying issuer and check these claims locally. One exported normalizer trims surrounding whitespace and lowercases the entire address. Cloud verifies current access on each ordinary request.
- Get or create the stable database-generated numeric owner ID by normalized email, including under concurrency. Retain it across application lifetimes. Persist an application row with ID, owner, slug, namespace, creation time and lifecycle state. Enforce owner/slug uniqueness. Different owners may reuse the same slug. Enforce package naming rules and the 61-character maximum on the full u{numericUserId}-{appSlug}; report 400 and the maximum slug length for this prefix when exceeded.
- Return ID and namespace for new and existing applications and identify an existing result. Preserve all existing application state on repeat creation. A Deleting row reserves its slug and yields 409 with its ID and state. New applications have requests enabled, no current publication, execution views off and the default ONERROR persistence policy.
- Implement the shared application permission check using the stored owner in Phase 1. The owner inherits Manager and Caller permissions. After a successful database lookup, return the same 404 for a missing application and for one owned by another user. If Go cannot read the access data, return 503 before any write or dispatch.

## Acceptance criteria

Serve the fixture’s JWKS over HTTPS with a certificate valid for the configured issuer hostname. Configure the isolated test deployment to trust the fixture certificate chain and use normal TLS verification.

Provide a test fixture that signs short-lived user tokens with a local asymmetric test key and serves its public verification keys at the configured test issuer’s JWKS endpoint, reachable from the backend pod. Configure only the isolated test deployment to trust that issuer. Send the token as a REST bearer credential through the ordinary verifier. Include the issuer, expiry, `authenticated` audience and the required identity claims, using top-level `is_anonymous: false` and distinct top-level `email` values for the owner and nonowner cases. Keep the signing key in the fixture and credentials out of evidence. The post-V1 `loomspan-cloud-test-suite` scenario owns the first real email-code sign-in.

- [ ] Start Go with an empty verification-key cache, the configured HTTPS test issuer offline and the other readiness prerequisites satisfied. Verify Go is ready. Submit a Create request for a new slug with a valid, unexpired fixture token. Expect 503 with zero application lookups, writes or engine dispatches. Bring the issuer online and repeat the identical request with the same still-valid token. Verify the same Go process fetches the keys and returns 201.
- [ ] Take the issuer offline while the matching cached key is still usable and verify the token normally. For an otherwise valid token with an unknown key ID, verify that a required refresh which fails returns 503 and a successful refresh with no matching key returns 401. Both responses precede application lookup, writes and engine dispatch.
- [ ] With otherwise valid fixture tokens, verify a nonanonymous owner with a nonblank top-level email succeeds with `user_metadata` omitted. Test missing, null, nonstring, empty and whitespace-only emails. Test `is_anonymous` set to true, missing, null or a string such as `"false"`. Include a missing-email case and an anonymous case with `user_metadata.email_verified: true`. Every rejected token produces 401 before any application lookup or write.

- [ ] Create an application via REST using a locally signed owner token. Exercise the browser/REST verifier with otherwise valid tokens: accept `"authenticated"` and an array containing it; reject a missing `aud` and an audience containing only the MCP endpoint URL before any application read or write. Untrusted or expired tokens cannot create or access an application, and supplying an email cannot select another owner.
- [ ] Repeat and race creates for the same owner/slug and verify one application and stable namespace; another owner can use the slug independently. Existing state survives repeat creation, including the 409 behavior for a seeded Deleting row.
- [ ] Verify namespace boundaries, owner normalization and creation defaults.
- [ ] Exercise the shared check at both Manager and Caller permission levels with the signed-in owner. Both pass. With a second signed-in user, compare the response for the owner’s application with the response for a missing application. Both must have the same 404 status and body. Make the access lookup unavailable and verify 503 with zero writes and zero dispatches.

## Handoff and deferred work

Make the stored lifecycle state available with the authorized application lookup for Stage, Publish and Invoke. Check permission before exposing `Deleting`. Phase 4 owns entering that state and coordinating Delete with admitted work.

Use [PR 2.1.1](PR-2.1.1-console-home.md) and [PR 2.1.2](PR-2.1.2-backend-readiness.md) as the local baseline. Give [PR 2.1.5](PR-2.1.5-stage-and-validate.md), [PR 2.1.6](PR-2.1.6-first-publication.md) and [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) the test issuer and token fixture, creation operation and shared permission-check entry points. [PR 2.1.7](PR-2.1.7-invoke-first-skill.md) repeats creation using a fresh fixture token at phase close. [Phase 3](../phases/go-backend-phase-3-access-and-companions.md) owns the membership schema and lookup, membership administration, the end-to-end 403 check for insufficient role, companion OAuth and integration keys. [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) drives actual deletion; this ticket can exercise the reserved-slug rule using a seeded Deleting row.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Introduces durable identity, access controls and concurrent creation semantics.
- **Reassessment triggers:** Reassess the sign-in integration if the migrated browser assets already provide part of the flow.
