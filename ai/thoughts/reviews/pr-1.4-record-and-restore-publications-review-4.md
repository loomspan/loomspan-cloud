# Record acknowledged publications and restore them from Go. Code review cycle 4

## Scope and Repository State

Reviewed the PR 1.4 working-tree changes against `1b945b8b6e50d95582fe336459c3dab7b15ba5a4`, including the untracked Go publication package, tests and fixture corpus, migration, dependency checksums, and PR 1.4 research and plans. Reviewed the backend entry point, Go module, container build, and K3s configuration changes. Read the existing engine observer and Java adapter at the affected boundaries. PR 1.5's Java changes, other ticket artifacts, and `.vscode` remain outside this scope.

## Findings

No actionable findings in the staged publication and recovery paths.

## Findings Resolved in This Context

None. This review changed only this artifact.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Authorized validation binds the selected revision and effective settings. Incomplete checks return retry guidance without partial results. Completed results include separate variable blockers with authored paths. | `service.go:131`, `service.go:234`, `postgres.go:55`, `engine_client.go:66`, and `variables.go` | `TestValidate`, `TestVariableCorpus`, `TestVariableDiscovery`, and `TestEngineClient` | Implemented |
| A stale selection requires its exact reviewed and current revision pair. Publishing an already-current revision sends nothing and creates no version. | `service.go:166` and `postgres.go:205` | `TestPublish` and `TestPublicationPostgres` | Implemented |
| First authenticated snapshot is sufficient for dispatch. Missing values, absent observations, and known termination block sends. Rejection and an unconfirmed response stay distinct. | `engine_client.go:82`, `engine_client.go:107`, `service.go:270`, and `service.go:285` | `TestPublish`, `TestEngineClient`, and `TestEngineClientAdmissionAndAuthenticatedReceipt` | Implemented |
| Durable acknowledgment precedes caller success, preserves the mounted digest, and requests a fresh snapshot. Owner install and recovery share the application gate. | `service.go:285`, `service.go:365`, `pack.go:45`, `postgres.go:205`, and `reconcile.go:139` | `TestPublicationPostgres`, `TestRecoverySerializesWithPublish`, and `TestMountedPack` | Implemented |
| Reopened durable intent restores with current saved values and settings. Fresh matching installations are preserved. A failed restore is isolated, and only confirmed, ready placements route. | `reconcile.go:29`, `reconcile.go:68`, `reconcile.go:110`, `reconcile.go:176`, and `postgres.go:158` | `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, `TestEligibility`, and disposable PostgreSQL recovery | Implemented |
| Single backend starts recovery with a private Postgres schema and read-only immutable ZIP mount. | `management-backend/cmd/management-backend/main.go:42`, `management-backend/migrations/001_publications.sql:3`, and `deploy/k3s/management-backend.yaml:29` | Go build, real migration and store verification, and client-side Kustomize rendering | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

This ticket consumes ZIPs that have passed Cloud staging. The stability contract for the future upload validator should use the same supported member codecs as shared Go discovery. Go discovery's standard reader handles Store and Deflate. The Java adapter also delegates additional compression methods to Commons Compress. No current Cloud staging operation admits those other methods. The prior Java adapter's broader decoder set does not establish which codecs the separate upload workflow will accept.

## Verification Results

- PASS — `go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack|Validate|Publish|EngineClient|Reconcile|Eligibility|Recovery)' -count=1`.
- PASS — `& ../execution-engine/target/verify-pr14-postgres.ps1`. The inspected helper created a fresh PostgreSQL 17 container on a random loopback port, applied the exact migration, and removed the container after the real acknowledgment, rollback, and reopened-store recovery test.
- PASS — `go test ./...`, `go test -race ./...`, `go build ./...`, and `go vet ./...`.
- PASS — `kubectl kustomize deploy/k3s` with client-side rendering. Confirmed one replica, Recreate, the Secret reference, and read-only pack storage.
- PASS — `git diff --check 1b945b8 -- management-backend deploy/k3s` and `git diff --cached --check`.

## Residual Risks and Optional Developer Checks

Hosted Supabase and cumulative K3s invocation were not part of this review. The backend requires its documented single writer, operator-provisioned database role, and durable ZIP mount. Package discovery assumes the prior staging contract. PR 1.5 owns invocation against the real replacement engine.

## Disposition

`clean`
