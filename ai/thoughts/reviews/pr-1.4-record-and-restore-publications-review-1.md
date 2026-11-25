# PR 1.4 Record acknowledged publications and restore them from Go — Code Review, Cycle 1

## Scope and Repository State

Reviewed the staged-validation, owner publication, and restore changes against commit `1b945b8`. The index contained no staged changes. The ticket scope includes the tracked backend startup, dependencies, Dockerfile and K3s edits, and the untracked publication code, migration, fixtures, tests and PR 1.4 process artifacts. Preserved the separate PR 1.5 work, `.vscode/settings.json`, and a concurrent change to `EngineClientIdentityTest.java`. The ZIP reader, authenticated Java transport, Console observer and framework variable grammar were traced at the integration boundaries. The PostgreSQL test used a new disposable loopback container for each run.

## Findings

No actionable findings remain after the fix and re-review.

## Findings Resolved in This Context

### [P2] Discover variables in valid ZIPs with opaque comments

- Location: `management-backend/internal/publication/pack.go:84` and `management-backend/internal/publication/zip_discovery.go:19`.
- Scenario: A valid staged ZIP has opaque archive-comment bytes that resemble an end-of-directory record. The standard Go ZIP reader can choose the lookalike while the Java ZIP adapter selects the actual directory. The same mismatch occurs when an ordinary central directory is exactly 65,535 bytes and an entry comment resembles a ZIP64 locator.
- Impact: Go omitted authored references or blocked publication and restore with an unrelated ZIP finding for a pack Java accepts.
- Evidence: The new archive-comment regression returned `missing-execution-configuration` before the fix. The Java adapter already has regressions for both opaque metadata positions. The Go reader's ZIP64 branch also inspects bytes preceding an ordinary end record when the directory size is exactly 65,535.
- Fix: Check the directory envelope and actual record structure before exposing a private view to Go's ZIP reader. Exclude the opaque archive suffix from that view and provide an unambiguous ZIP64 locator when the ordinary-size lookup could shadow the directory. Keep the mounted original bytes, digest, and multipart wire unchanged. Both new tests assert exact authored source and path plus byte preservation.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Validate the named authorized revision with captured settings, return complete engine and Cloud results, and surface blockers without ending the check | `service.go:131`, `postgres.go:55`, `engine_client.go:65` | `TestValidate`, `TestEngineClient`, `TestVariableCorpus`, and the real SQL selection check | Implemented |
| Require confirmation for stale staging and send nothing for an already published revision | `service.go:166` compares both revision IDs inside the application gate and preserves the staging pointer | `TestPublish/first snapshot and stale staged confirmation` | Implemented |
| Preserve original ZIP and exact value map, classify first-snapshot admission, termination, authenticated rejection and response loss | `pack.go:42`, `variables.go:367`, `engine_client.go:82`, `engine_client.go:107` | `TestMountedPack`, both new ZIP comment cases, `TestPublish`, `TestEngineClient`, and `TestEngineClientAdmissionAndAuthenticatedReceipt` | Implemented |
| Record the acknowledged pointer before returning, serialize with restoration, and keep routing closed until Console confirms | `service.go:285`, `postgres.go:205`, `reconcile.go:29` | `TestPublicationPostgres` holds the application row lock after simulated Java acknowledgment, reopens the store and checks rollback. `TestRecoverySerializesWithPublish` covers both orderings, and `TestEligibility` checks routing state. | Implemented |
| Reopen and restore through fresh observation, skip matching placements, isolate failures and retain active policy separately | `reconcile.go:68`, `reconcile.go:110`, `reconcile.go:139`, and `reconcile.go:176` | `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, `TestReconcileRetriesOnlyOnFreshAssociation`, and `TestPublicationPostgres` | Implemented |
| Start the single-writer reconciliation worker with mounted ZIPs and database configuration | `management-backend/cmd/management-backend/main.go:42`, `deploy/k3s/management-backend.yaml:9` and `:29` | Go build and rendered Deployment with the database Secret, publication PVC and `Recreate` strategy | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

None. The deployment uses the documented single writer. Shared owner operations consume an explicit authorizer, with the backend's standalone recovery process closed to owner operations until an authorized caller is wired.

## Verification Results

- FAIL — `go test ./internal/publication -run '^TestMountedPackDiscoversVariablesWithAnOpaqueZIPComment$' -count=1` reproduced the missing execution configuration before the fix.
- PASS — `go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack|Validate|Publish|EngineClient|Reconcile|Eligibility|Recovery)' -count=1`.
- PASS — `go test ./...`, `go test -race ./...`, `go build ./...`, and `go vet ./...`.
- PASS — `& ../execution-engine/target/verify-pr14-postgres.ps1` before and after the fix, using separate disposable loopback PostgreSQL containers.
- PASS — `kubectl kustomize deploy/k3s` and verified the rendered Secret, volume, mount and single-replica strategy.
- PASS — `git diff --check 1b945b8` and `git diff --cached --check`.

## Residual Risks and Optional Developer Checks

The real skill and K3s replacement milestone belongs to PR 1.5. This review verified the production PostgreSQL migration in disposable PostgreSQL. No optional developer checks remain for PR 1.4.

## Disposition

`fixes-applied`. Run a fresh PR 1.4 review on the updated working tree.
