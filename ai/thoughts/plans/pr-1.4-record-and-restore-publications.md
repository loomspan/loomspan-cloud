# Acknowledged publication and replacement recovery implementation plan

## Overview

- Ticket `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md`.
- Research `ai/thoughts/research/pr-1.4-record-and-restore-publications.md`.
- Outcome. An authorized owner can validate and publish a selected staged ZIP. The Go backend records an installation acknowledged by Java before returning success and restores the acknowledged version after engine replacement.

## Current State

The Cloud Java ZIP adapter exists in the working tree. `PublicationZipAdapter` supplies raw portable execution and skill YAML to Java while carrying the pack-relative names. PR 1.2 owns the Java validate and publish endpoints. At planning time the Go observer and authenticated Pod IP transport are specified under `management-backend/internal/engine/` in `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md`, with `Current()`, `Changes()`, and `Refresh(ctx)` as the placement handoff. The implementation step must verify and use the actual signatures. There is no pre-existing Go publication workflow, authorization API, database schema, or mounted storage adapter to change. `AuthoredYamlVariables` and the Framework corpus supply the language-neutral discovery behavior.

## Desired End State

Go validates the named revision with the captured effective settings and returns both completed Java findings and Cloud findings for that attempt. Missing values block publication independently of validation validity. Go checks reviewed revision confirmation and saved inputs, publishes the original ZIP to the current authenticated observation, and durably advances the application pointer only on Java `204`. On startup and a fresh connection or JVM association, Go compares that pointer with the Console version, restores missing or obsolete versions, and exposes routes only for confirmed intended placements.

## Scope

### In scope

- Authorization inside shared Go validation and publish operations, original staged ZIP lookup, authored variable discovery and value checks, and complete wire settings.
- A migration and PostgreSQL implementation for minimal seeded application, staging, saved input, publication, and install state. Mounted immutable ZIP lookup and backend startup recovery wiring.
- Shared authenticated dispatch, publication outcome classification, restore ordering, and an observed placement interface for PR 1.5.

### Out of scope

- Staging/upload creation workflow, full REST/MCP/UI plumbing, billing, promotion, manual restart, and follow-up confirmation or retry of an unconfirmed publish.
- Production Supabase provisioning, Supabase Auth integration, durable filesystem provider setup, or end-user invocation. The workflow accepts authenticated inputs at its shared service boundary, and the milestone can seed application and staging state beforehand.

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Impact and Risk Analysis

- Java activation, the durable current pointer, and Console confirmation occur at separate boundaries. A lost response can leave an install uncertain. A checked or acknowledged engine response must be tied to the admitted direct authenticated connection. A lost response or failed local acknowledgment transaction leaves the old durable pointer in place and yields no success.
- A restore decision based on an old pointer can overwrite newer work unless it enters the same dispatch gate as owner publication and rereads both the pointer and current observer state. Use one gate per application in the V1 single Go backend process, and configure the deployment as one replica with a non-overlapping replacement strategy. Stage and settings edits remain independent of that gate while an install waits on Java.
- Parsed Go YAML is used for discovery only. Preserve authored node ordering, tags, alias use locations, and traversal boundaries. Framework resolves the literal supplied strings and detects effective mapping-key collisions. Go carries original bytes, so traversal must avoid materializing or rewriting the publication.
- Keep raw connector/service errors and variable strings away from logs. Capture effective settings in SQL per attempt without persisting a duplicate variable value map. Bind selections to the already authorized application before revealing revision, variable, or engine response details.

## Implementation Approach

Use `management-backend/internal/publication` for the domain service, persisted state, and discovery. Reuse `internal/engine` for authenticated requests and observer state. Keep the engine's completed validation result distinct from Cloud-side blockers, and keep durable intended state distinct from Console-observed serving state. The management entry point runs the observer and the reconciler with one derived lifetime context. Postgres operations remain short and separate from engine I/O. A single in-process keyed dispatch gate covers owner and recovery installs from preflight reread through acknowledgment processing, consistent with the V1 deployment constraint. Installation attempts carry runtime settings and target context, letting later saved selections and active Console policy remain separately observable.

## Phase 1 — Persist seeded applications and capture authored inputs

### Changes

- [x] `management-backend/internal/publication/runtime.go` defines the shared `RuntimeSettings` wire shape and `EffectiveRuntimeSettings`. Emit all required V1 settings even for a reference-free pack. Use `60s`, `32`, `20971520B`, and quotas `64`, `128`, `32`, `64`, `192`, `200000` under the contract's exact names. Accept saved `NEVER`, `ONERROR`, or `ALWAYS`, default to `ONERROR`, and reject unknown saved selections.
- [x] `management-backend/internal/publication/variables.go` and `pack.go` scan the portable execution YAML and the exact skill source names in the already staged ZIP. Read only selected ZIP members, apply the existing whole-ZIP and total expanded input bounds, and preserve member order within skill docs. Scan execution before skills. Use node-aware `go.yaml.in/yaml/v3` parsing. Match Framework's resolved string-tag predicate for plain scalars with the actual SnakeYAML Engine Core resolver associated with the starter, retaining explicit tag/quoted style semantics. Cover differing implicit dates, `~`, signed or separated numerics, and `${…}` ENV-looking values with cross-language evidence. Do not classify a scalar as a string solely because the Go resolver returns `!!str`. Preserve tag and style for keys too. Follow actual alias nodes at each use, reject cycles and count logical expanded nodes through aliases before walking. Retain actual authored mappings and do not synthesize keys from `<<` merges. Walk keys before values and preserve indexes for sequences and complex mapping entries. Match Framework's literal dot/bracket escaping, path segments `{key#i}` and `{value#i}`, and code-point whitespace predicate. Capture ordered occurrences, distinct names in first-occurrence order, and diagnostics with fixed reason and exact source/path. Allow Go discovery to succeed in the Framework collision fixture.
- [x] `management-backend/internal/publication/store.go` defines application-scoped reads, `OpenSelection` read snapshot, `StartInstall`, `CompleteInstall`, `CurrentPublication`, and `ListCurrentPublications` contracts. Capture the stage selection, current staging pointer, current publication, saved settings, and referenced values from one short read-only repeatable-read snapshot. Read the values for distinct referenced names only. A value containing nonblank data remains literal, including surrounding whitespace and placeholder syntax. Distinguish missing and blank with an occurrence per authored path. Read ZIP bytes from an immutable content-addressed ref under the configured mounted root using a safe root-relative file API, verify the stored digest against the original bytes, and keep original ZIP bytes as the outbound `pack`.
- [x] `management-backend/migrations/001_publications.sql` and `management-backend/internal/publication/postgres.go` implement the schema and SQL through `database/sql` with the PGX v5 stdlib driver pinned in `management-backend/go.mod` and `go.sum`. Store application ID, unique namespace, saved policy, and current staging/publication pointers. Stage records carry immutable revision ID and pack ref. Application variable names and values are keyed by application. Publication rows carry app ID, opaque unique version, originating stage, immutable pack ref and status. Install attempts reference the publication and store the actual effective settings for each owner install and restore, including settings sent, target pod/JVM identifiers where available, status, and timestamp. Use composite foreign keys to prevent cross-application pointers. Keep runtime data in an internal schema with access held by the backend database role. Retain historical records when the current pointer advances. Use a migration apply command in deployment instructions and keep all connection credentials outside source.

### Automated verification

- [x] From `management-backend/`, `go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack)'` proves Framework parity, exact values, and immutable ZIP reads.
- [x] Run the disposable PostgreSQL integration command from the testing plan. Reopen the store from a new connection to establish that records and pointers are durable.

### Optional developer checks

- None.

## Phase 2 — Validate and acknowledge owner publications

### Changes

- [x] `management-backend/internal/publication/service.go` exposes `Validate(ctx, actor, applicationID, revisionID)` and `Publish(ctx, actor, applicationID, request)` with result types that identify the requested revision, and the exact settings used, or typed unavailable, confirmation-required, and publish outcomes. Require an explicit `Authorizer` receiving a trusted authenticated actor, application ID, and validate/publish permission. Reject before revision lookup on denial, with no allow-by-default fallback. No publicly accessible adapter accepts an untrusted identity assertion. The milestone test may inject its documented seeded authorization adapter.
- [x] `Validate` captures the named selection and the settings used, checks application variables and pack issues, calls Java `/internal/v1/validate` even with missing or blank references, and returns the completed diagnostics, skills, and Cloud findings. Missing values are separate blockers and preserve otherwise successful validity. Authored Cloud errors contribute to invalidity. Return a typed retryable unavailable outcome without partial findings for a missing connection, known termination, non-success response, transport loss, or malformed completed report. Distinguish known shutdown in the caller-facing explanation.
- [x] `Publish` serializes with restore by application. Inside the gate, reread auth as required by the shared authorizer boundary and capture the current staging/current publication selections and saved inputs. First return exactly `This revision is already published.` for an already-current revision, without allocating a new version. For a stale revision return both IDs and require explicit confirmation of the compared reviewed/current pair. Confirmed publish keeps newer staging intact. Validate the ZIP source, variable discovery, missing/blank occurrence list, and runtime settings before dispatch. Generate a new opaque version only once dispatch prerequisites are met and persist a proposed row/attempt before sending. A definite no-send race marks the attempt accordingly without changing the pointer.
- [x] `management-backend/internal/publication/engine_client.go` writes the unchanged `pack` part as `application/zip` and the JSON `metadata` part as `application/json` with exact field names. Omit `variableValues` for validation and require the exact non-null map for install. Use the PR 1.3 direct authenticated transport with no redirects or automatic workflow retry. Build the request before a final observer-backed admission check. Bind admission to the current pod address, UID, authenticated JVM observation, and connection epoch, and order it against accepted termination/disconnect updates. Do not hold observation locks across network I/O. First publication may use a connected snapshot before pod readiness, even with no Cloud applications. After admission, process responses normally if the observation changes. Classify a proven pre-send failure as retryable not sent, a lost or incomplete response after possible dispatch as unconfirmed, and a received authenticated Java text rejection as rejected, preserving its message byte-for-byte. Treat `204` as the only positive publish acknowledgment. No observation promotes an unconfirmed attempt in this ticket.
- [x] On `204`, `CompleteInstall` transactionally acknowledges the owner publication and attempt and updates the current pointer, verifying the expected previous pointer. After Java accepts the request, use a bounded service-lifetime acknowledgment context so caller cancellation does not discard an already received `204`. Report success only after commit. If Java acknowledged but durability cannot be established, report an explicit acknowledgment-recording error and permit no caller success. Queue an immediate Console refresh after a committed install. Refresh failure does not undo a durable acknowledgment. The observation can lag success.

### Automated verification

- [x] From `management-backend/`, `go test ./internal/publication -run 'Test(Validate|Publish|EngineClient)'` proves combined diagnostics, no dispatch on known unavailable states, exact payloads, and publish outcome handling.
- [x] The PostgreSQL integration confirms no `Publish` success leaks before the pointer transaction commits, and proves the rejected, not-sent and unconfirmed paths preserve the prior pointer.

### Optional developer checks

- None.

## Phase 3 — Restore durable intent and expose routing eligibility

### Changes

- [x] `management-backend/internal/publication/reconcile.go` implements `Run(ctx)`, observation-change processing, and `Eligible(ctx, applicationID)` as the shared PR 1.5 placement handoff. Coalesce observer notifications while always rereading `Current()`. At backend startup and when a fresh connection/JVM association becomes available, enumerate current publications, then acquire the per-application gate before each restore. Reread the application's current pointer and the latest eligible dispatch observation inside the gate. Compare namespace and version. A match preserves loaded settings and values and skips the engine call. For a mismatch use the original ZIP and same durable version with settings and variables read from current application state. Record restore attempt settings and the response outcome while preserving the current pointer. Continue other applications after one fails, report failed application/target/attempt identities with safe diagnostics, and schedule a refresh after an acknowledged restore. Deduplicate recovery by connection association and application/version so failed requests and refresh notifications do not produce a hot retry loop. Fresh associations can try again. Leave unassigned namespace entries alone.
- [x] `Eligible` reads the application's durable current publication and accepts only the observer's fresh connected, ready, non-terminating confirmation for that exact namespace and version. Return a value describing the engine target, observed JVM, generation ID, version and namespace trace policy. Do not return a prepared generation handle or send the version to an invocation endpoint. Reuse observer admission/transport for later invoke to reject a superseded target before dispatch. This function may return an already confirmed application while reconciliation is still restoring other applications.
- [x] `management-backend/cmd/management-backend/main.go` reads `LOOMSPAN_DATABASE_URL` and `LOOMSPAN_PACK_ROOT`, opens and verifies the PostgreSQL store and mounted directory, constructs the publication service with the existing observer, and starts recovery and the refresh worker under the same shutdown context. Use safe error reporting without the connection URL or variables. `deploy/k3s/management-backend.yaml` uses one replica and a `Recreate` rollout, references database credentials in a Secret, and mounts the operator-supplied publication volume read-only for the milestone. Update `deploy/k3s/README.md` with external credentials, migration application, and existing-volume requirements. Test setup may create its own disposable database and volume.

### Automated verification

- [x] From `management-backend/`, `go test ./internal/publication -run 'Test(Reconcile|Eligibility|Recovery)'` verifies reuse, restoration, ordering, and interrupted observation.
- [x] From `management-backend/`, `go test ./...`, `go test -race ./...` on a race-enabled host, and `go build ./...` pass. `kubectl kustomize deploy/k3s` renders the new backend configuration. The testing plan's disposable PostgreSQL run exercises the real migration and durable restart. PR 1.5 extends this proof through a real skill dispatch on a replacement JVM.

### Optional developer checks

- None.

## Test Strategy

Use deterministic YAML corpus tests, authenticated loopback HTTP at the real multipart encoding boundary, and channel barriers for ordering. Exercise the production SQL adapter on disposable PostgreSQL with exact migration and an on-disk ZIP. Run backend recovery against a re-opened store and a new observer association, controlling the acknowledged version and refresh timing independently. Test entrypoint configuration and manifest rendering separately from the later cumulative K3s invocation.

## Acceptance-Criteria Traceability

| Acceptance criterion | Planned code evidence | Planned executable evidence |
| --- | --- | --- |
| Complete and incomplete validation, saved values as blockers, exact worked corpus, original selection under concurrent edit, and direct publication afterwards | `Validate`, `OpenSelection`, authored discovery, and publish preflight | `TestVariableCorpus`, `TestValidate`, `TestPublish/filled-blockers`, `TestPublicationPostgres` |
| Revised staging requires explicit confirmation, newer staging stays intact, already-current revision performs no new install | `Publish` confirmation, current revision comparison and gate | `TestPublish/stale-reviewed-revision`, `TestPublish/already-current`, `TestPublicationPostgres` |
| First authenticated snapshot publishes, missing values and shutdown yield no dispatch, lost response differs from Java rejection, rejection text is preserved | Observer admission, `Publish` preflight, authenticated client outcome types | `TestPublish/first-snapshot`, `TestPublish/known-unavailable`, `TestPublish/engine-outcomes`, `TestEngineClient` |
| Ack durably precedes success, restore racing owner publish finishes on new version, refresh requested and routing waits for confirmation | `CompleteInstall`, per-application gate, refresh trigger, `Eligible` | `TestPublicationPostgres`, `TestRecoverySerializesWithPublish`, `TestEligibility` |
| Restore after ack without observation, match skips reload, failure isolation, extra namespace ignored, and sent/current/active settings independent | `Run`, `Restore`, per-attempt settings, and `Eligible` | `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, `TestPublicationPostgres` |

## Risks and Rollback/Recovery

The single writer is a V1 deployment invariant. A later scale-out must provide a shared fenced dispatch owner before multiple Go instances can publish. After a failed migration or startup configuration check, the backend cannot safely dispatch publications. Keep the SQL history and volume during rollback. If a response was lost or an acknowledgment transaction failed, preserve the previous pointer and attempt record and report uncertainty. Do not automatically resend the uncertain operation. Once a fresh observer connection is usable, ordinary reconciliation uses only durably acknowledged intent. Retain the previous record when an owner or restore installation is rejected.

## References

- `ai/thoughts/research/pr-1.4-record-and-restore-publications.md`
- `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md`
- `ai/thoughts/future/execution-engine-api.md`
- `ai/thoughts/future/go-backend-roadmap.md`
- `ai/thoughts/future/application-variable-conformance.md`
- `../loomspan-framework/loomspan-variable-fixtures/README.md`
- `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java`
