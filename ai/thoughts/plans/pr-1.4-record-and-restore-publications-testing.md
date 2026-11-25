# Acknowledged publication and replacement recovery testing plan

## Change Summary

Exercise authored variable discovery, owner validation and publish, durable acknowledgment, and restoration from Go. Check the transport and SQL at their real boundaries where substitutes would hide a contract failure. Hold dispatch, database acknowledgment, and Console observation independently to verify routing does not treat one as another.

## Impacted Areas and Risks

| Category | Risk | Planned evidence |
| --- | --- | --- |
| Authored inputs and visibility | YAML tags, ordering, aliases or paths drift from Framework. An unauthorized caller receives a staged or engine diagnostic | Shared corpus, parser edge cases, authorization-denial counters, and wire payload assertions |
| Dispatch and cancellation | Known no-send becomes a lost acknowledgment, or a caller sees a partial validation result or a rewritten Java failure | Authenticated loopback multipart tests with closure and response barriers |
| Persistence and state | Install status and current pointer are committed separately, settings changes overwrite the actual sent values, or an app crosses an ownership boundary | Production SQL migration and store on disposable PostgreSQL and content-addressed ZIPs on disk |
| Observation and ordering | A stale observation routes, a restore uses stale durable intent, or one failure prevents all other namespaces from recovering | Deterministic observer and engine event barriers, multiple applications, replacement association and restarted store |

## Existing Coverage and Environment Constraints

No Cloud publication tests or Go SQL schema exist at planning time. PR 1.3 plans a Go observer, authenticated direct-pod HTTP transport, and local protocol tests under `management-backend/internal/engine`. Use its testable boundaries after confirming actual signatures. The Framework `loomspan-variable-fixtures/` corpus supplies authored cases and expected discovery records. Extend locally with alias and resolver-sensitive cases; keep the checked-in Go test fixture set independent of a sibling checkout and document Framework revision and provenance. The actual Framework fixture data is small and available locally. No provider, Supabase project, operational credential, or cluster DNS is needed for default tests.

Go 1.26.1 is available. A disposable Docker engine has PostgreSQL `postgres:17-alpine` available, and the image was imported into the disposable K3s context `k3d-loomspan-cloud-pr1-cg1`. Use an explicit opt-in database DSN for the SQL integration suite. Provision a dedicated database and its throwaway user in a named test container and keep the DSN out of output. PR 1.5 exercises a production-shaped engine/recovery loop on K3s, using disposable state.

## Failing Test First

- Name `TestVariableCorpus`, subtest `worked`.
- Type and location. Pure Go unit in `management-backend/internal/publication/variables_test.go`.
- Arrange and act. Load the exact Framework `worked` YAML documents and `expected.json`, execute the scanner, and compare the ordered expected six occurrences, four names, and the skill-only `config.path` occurrence.
- Expected pre-fix failure. The scanner is missing or cannot produce the authored occurrence records. A value-only or map-based approximation fails this assertion.

## Tests to Add or Update

### 1. `TestVariableCorpus`, `TestVariableDiscovery`, `TestRuntimeSettings`, and `TestMountedPack`

- Type and location. Units in `management-backend/internal/publication/variables_test.go`, `runtime_test.go`, and `pack_test.go`. Store fixture data under `management-backend/internal/publication/testdata/framework-variables/`.
- Proves. Read `expected.json` for each portable case in its document order. Successful discovery compares ordered names and source/path pairs. Negative discovery cases compare the exact reason and location. `collision` succeeds at discovery and retains both paths for Java. Include complex keys and their paired values, `excluded`, `header-key`, and `paths`. A staged ZIP assembled from `worked` produces identical discovery while companion data is skipped and the original stored bytes are returned as-is. Check deployed whole-archive byte and selected expanded limits at exactly the boundary and over it.
- Edge cases. Add authored alias use paths and required marker types; recursive aliases and logical expansion above the Framework budget stop with a deterministic diagnostic. Verify Java whitespace code points that Go `strings.TrimSpace` would classify differently. Check exact quoted path output for brackets, dot, ASCII controls and HTML characters. Verify plain and quoted dates, ordinary and signed numerics, explicit tags, `${…}`, `~`, and literal `<<` semantics with expected cases confirmed against the locally cached Framework starter. Exercise malformed marker, ordinary multi-key maps, executor absent, and non-mapping skill `config`. A named value containing spaces around nonblank content or `${another}` is sent unchanged. Check `NEVER`, missing-selection `ONERROR`, and `ALWAYS`, with all policy fields and exact JSON types. Reject mounted traversal, inconsistent digest or changed content, and oversized stored packs.

### 2. `TestValidate`, `TestEngineClient`, and `TestPublish`

- Type and location. `management-backend/internal/publication/service_test.go` and `engine_client_test.go`. Use deterministic in-memory store state for the domain suite, a real loopback authenticated TLS responder with the PR 1.3 transport for wire cases, and explicit channels for timing. Do not substitute an unverified client for the success/rejection identity checks.
- Proves. An unauthorized caller receives no stage details, and denial prevents ZIP reads and dispatch. A completed Java report returns diagnostics, skill information where valid, and Cloud findings from the original revision and settings. Missing or blank variables still cause a validate dispatch and keep an otherwise valid pack valid. Prove exactly four requested names for six occurrences and report each missing/blank occurrence source and path, including the skill-only case. Edit stage/current settings while a request waits on Java, then confirm the returned revision/settings remain the captured ones. Fill the values afterwards and publish that same revision with the then-current effective settings. Validation never supplies a value map.
- Edge cases. Java `200` with invalid report shape, non-success or partial body, a connection cut, and known termination return no partial findings and invite retry. Parse both multipart messages and compare exact original ZIP bytes, content types, namespace and settings. A valid first authenticated empty snapshot on an unready pod permits publish. Missing/blank variables block dispatch and version creation. A reference-free publish emits `variableValues: {}`. A stale reviewed revision returns both IDs, exact confirmation sends the reviewed ZIP, and newer staging stays. An already published revision returns the exact defined message with zero new versions or requests. A local pre-dispatch unavailable or terminating state, an authenticated received Java rejection with newline and non-ASCII text, and a lost or truncated response after dispatch produce distinct typed outcomes. Reject or report uncertainty for malformed success statuses. Already admitted replies are still processed after a newer observation or termination notification. Assert no automatic retransmission.

### 3. `TestRecoverySerializesWithPublish`, `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, and `TestEligibility`

- Type and location. Deterministic application/observer integration in `management-backend/internal/publication/reconcile_test.go` with `service.go` and `reconcile.go` running together. Use channels as barriers and timeouts only as guards.
- Proves. Gate restore and owner installs per application, including when restore reads the old pointer before it waits. In both orderings, the acknowledged newer publication ends as the engine version and current pointer. An old restore waiting for the gate rereads and cannot send the stale version after the newer acknowledgment. Queue the explicit refresh and keep routing closed for the newer intended version until a currently connected, ready, non-terminating observation confirms it.
- Recovery sequence. Acknowledge and durably record old version, observe it, then acknowledge and durably record the newer version while blocking observation. Simulate disappearance and restart with a re-opened store or separate SQL-backed counterpart and the new observer association. A restore requests the same durable version and original bytes, using current saved variable values and current settings. No application route is returned before the new JVM confirms the matching version. Assert old and replacement target identities differ. For a correctly matching fresh observation, change saved variables while offline and assert zero dispatch on reuse. Supply other namespaces including one missing, one matching, one failed, and one unintended, and verify reconciliation progress and route eligibility independently. A failed app is individually reported and causes no hot loop within one connection association. Known termination and loss close eligibility; a fresh observation can reopen it. Preserve the observed active policy even if saved settings edited during install differ and the default namespace has a different policy.

### 4. `TestPublicationPostgres`

- Type and location. PostgreSQL integration in `management-backend/internal/publication/postgres_integration_test.go`. Skip if `LOOMSPAN_TEST_POSTGRES_URL` is unset. This variable is set only for a freshly created disposable database. Apply the exact `management-backend/migrations/001_publications.sql` through the production driver and seed two applications, multiple staging revisions, saved variables, and a mounted fixture ZIP via temporary disk. Keep the simulation of Java and authenticated observation independent from the real store. The PR 1.5 suite adds the actual engine and K3s pod replacement.
- Proves. Composite pointers prevent cross-app mutation and reads. Establish an initial acknowledged pointer. For an owner install, after dispatch begins use a second SQL connection to lock the application row with `FOR NO KEY UPDATE`, then release Java's `204`. While the write is held, the previous durable pointer survives and the caller cannot receive success. Release the row lock and assert acknowledged publication and attempt, exact sent settings and immutable ZIP ref, and new current pointer are visible through a fresh connection at success. Independently update saved settings after request capture and before release; observe the attempt values and saved selection separately. Inject a transaction failure and observe no success and no partial transaction. A Java rejection and unconfirmed dispatch leave the old current publication while recording their distinct statuses. Exercise a restore using a fresh instance of the SQL store after the later committed version was never observed, and verify restoration and route gating from the same durable bytes. Verify binding same names across applications, exact selected lookups, and independent progress when another app fails.
- Boundary. No production data or running Supabase project. Never run migrations against a configured database accidentally. Establish the disposable DB and connection explicitly in the same shell with cleanup confined to the returned test container ID. Production startup uses the same PostgreSQL store and migration schema; smoke verify required variables and mount wiring in rendered Kubernetes manifests. PR 1.5 starts the wired backend with real disposable state.

## Safe Verification Commands

Run these from `management-backend/`:

```powershell
go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack)'
go test ./internal/publication -run 'Test(Validate|Publish|EngineClient|Reconcile|Eligibility|Recovery)'
go test ./...
go test -race ./...
go build ./...
```

From the repository root, run `kubectl kustomize deploy/k3s`. This client-side command should render the single backend replica, database Secret reference, and read-only mounted pack path without a cluster connection.

### Disposable PostgreSQL integration

Run from `management-backend/` with a local Docker engine and the prepared image. The script prepares one fresh test database, binds it only to loopback, runs the focused proof, and removes only its own container.

```powershell
$loomspanDbPassword = [Guid]::NewGuid().ToString('N')
$loomspanDbName = 'loomspan_pr14'
$loomspanDbContainer = docker run --detach --rm --publish '127.0.0.1::5432' `
    --env "POSTGRES_PASSWORD=$loomspanDbPassword" --env "POSTGRES_DB=$loomspanDbName" postgres:17-alpine
if ($LASTEXITCODE -ne 0 -or $loomspanDbContainer -notmatch '^[a-f0-9]{64}$') {
    throw 'Could not create the disposable PostgreSQL test container'
}
try {
    $loomspanDbReady = $false
    for ($loomspanCheck = 0; $loomspanCheck -lt 30; $loomspanCheck++) {
        docker exec $loomspanDbContainer pg_isready -h 127.0.0.1 -d $loomspanDbName -U postgres | Out-Null
        if ($LASTEXITCODE -eq 0) { $loomspanDbReady = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $loomspanDbReady) { throw 'Disposable PostgreSQL did not start' }
    $loomspanDbAddress = docker port $loomspanDbContainer '5432/tcp'
    if ($LASTEXITCODE -ne 0 -or $loomspanDbAddress -notmatch '^127\.0\.0\.1:\d+$') {
        throw 'Disposable PostgreSQL did not bind its expected loopback port'
    }
    $env:LOOMSPAN_TEST_POSTGRES_URL = "postgres://postgres:${loomspanDbPassword}@${loomspanDbAddress}/${loomspanDbName}?sslmode=disable"
    go test ./internal/publication -run '^TestPublicationPostgres$' -count=1 -v
    if ($LASTEXITCODE -ne 0) { throw 'Publication PostgreSQL integration failed' }
} finally {
    Remove-Item Env:LOOMSPAN_TEST_POSTGRES_URL -ErrorAction SilentlyContinue
    docker stop $loomspanDbContainer | Out-Null
}
```

Use the local test container even when production settings are present. For PR 1.5, use the isolated kubeconfig `execution-engine/target/pr1-k3s/kubeconfig.yaml` and explicit context `k3d-loomspan-cloud-pr1-cg1` only.

## Optional Developer Checks

- None. The actual K3s skill invocation remains required in PR 1.5, and the transient PostgreSQL integration is an automated gate here.

## Exit Criteria

- [x] The first Framework corpus test fails before authored discovery exists, then passes across all supplied cases and the alias and type edges.
- [x] Shared Go operations and wire tests verify each authorization, validation, and dispatch classification without externally hosted services.
- [x] The separate PostgreSQL integration passes with the real schema, a restarted store, and the acknowledgment transaction and disk reference assertions.
- [x] Restore ordering, failure isolation, stale observation, fresh settings and exact route eligibility pass, including under the race detector where supported.
- [x] The Go module builds, its full safe suite passes, and the Kubernetes manifest renders with the needed state and Secret wiring.
- [x] Acceptance criteria map to executable evidence. Any cumulative K3s skill check remains attributed to PR 1.5.

## Step Report: 2_create_plan + 3_testing_plan

STATUS: complete
ARTIFACTS:
  - ai/thoughts/plans/pr-1.4-record-and-restore-publications.md
  - ai/thoughts/plans/pr-1.4-record-and-restore-publications-testing.md
SUMMARY: Planned authenticated staged validation and publication, durable Postgres acknowledgment, mounted ZIP lookup, and recovery from fresh engine observations. The test plan covers the Framework corpus, dispatch outcomes, and the database and routing boundaries.
DECISIONS:
  - Serialize owner and recovery installs under a per-application gate in the single Go backend and reread durable intent inside that gate.
  - Commit the acknowledged pointer before returning success, preserve settings per attempt, and gate routing on the matching authenticated Console observation.
  - Wire Postgres and mounted ZIP storage into the backend entry point and verify persistence against a dedicated disposable PostgreSQL database.
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Start PR 1.4 implementation after the PR 1.3 transport and observer handoff.
