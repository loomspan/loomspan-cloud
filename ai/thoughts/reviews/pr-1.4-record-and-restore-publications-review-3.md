# PR 1.4 Publication and Recovery Code Review — Cycle 3

## Scope and Repository State

Compared the working tree against `1b945b8`. Reviewed the staged selection and authorization boundary, runtime settings, mounted ZIP and authored variable discovery, authenticated dispatch, SQL migration and transactions, restore and routing lifecycle, backend startup, deployment changes, and focused tests. The PR 1.4 package and migration are untracked in this checkout. PR 1.5 Java work and planning documents and the unrelated `.vscode` directory are outside this review and were preserved. The index was empty.

## Findings

No actionable findings remain after the fix below.

## Findings Resolved in This Context

### [P2] Read ZIP64 packs that mark only the directory size

- Location. `management-backend/internal/publication/zip_discovery.go:19`.
- Scenario. A valid ZIP64 end record carries the central directory size. Its classic end record marks that field with `0xffffffff` and retains the ordinary entry counts and central directory offset. Java follows the locator and accepts these consistent directory fields. Go 1.26's `archive/zip` recognizes `0xffff` in the directory size field. It skips the locator for the valid size-only marker and rejects the archive as `invalid-zip`.
- Impact. A staged pack accepted by the Java adapter loses Cloud variable findings and cannot publish or restore through Go.
- Evidence. `TestMountedPackDiscoversVariablesWithZIP64DirectorySize` reproduced the false rejection and now verifies exact variable discovery and byte-for-byte preservation. The Java structural preflight in `execution-engine/src/main/java/ai/loomspan/cloud/engine/ZipDirectoryPreflight.java` checks the corresponding offsets, count, and wide size before accepting this envelope.
- Fix. After verifying the ZIP64 directory, set the classic record count markers on the private Go reader view when its directory size alone has the ZIP64 marker. The mounted and dispatched bytes retain their original end records.

## Acceptance-Criteria and Plan Conformance

| Criterion | Code evidence | Executable evidence | Result |
| --- | --- | --- | --- |
| Selected revision and captured settings, completed Java and Cloud findings, exact variable blockers, no partial result after interruption, and direct publication after filling values | `Validate`, `OpenSelection`, `discoverVariables`, `variableValues` | `TestValidate`, `TestVariableCorpus`, `TestVariableDiscovery`, `TestEngineClient`, `TestPublicationPostgres` | Implemented |
| Explicit confirmation identifies both revision IDs and preserves newer staging, while an already-current revision creates no version or install | `Publish` confirmation and already-current branches, `OpenSelection` | `TestPublish`, `TestPublicationPostgres` | Implemented |
| First authenticated snapshot permits installation, missing input and known shutdown prevent dispatch, Java rejection text stays intact, and response loss stays unconfirmed | `PublicationTarget`, `installInputs`, `EngineClient.send` and `install` | `TestPublish`, `TestEngineClient`, `TestEngineClientAdmissionAndAuthenticatedReceipt` | Implemented |
| Acknowledgment persists before caller success, owner and restore serialize, refresh follows acknowledgment, and routing waits for observed confirmation | `CompleteInstall`, application gate, `requestRefresh`, `Eligible` | `TestPublicationPostgres`, `TestRecoverySerializesWithPublish`, `TestEligibility` | Implemented |
| Recovery reloads original bytes and acknowledged version, uses current saved inputs, reuses matching generations, isolates failures, and keeps saved, sent, and active policies distinct | `Run`, `restore`, `alreadyInstalled`, install attempts, `Eligible` | `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, `TestReconcileRetriesOnlyOnFreshAssociation`, `TestPublicationPostgres` | Implemented |
| Deployment starts recovery from the production store and preserves the single writer | `cmd/management-backend/main.go`, migration, mounted storage and Deployment `Recreate` strategy | `go build ./...`, `kubectl kustomize deploy/k3s`, disposable PostgreSQL test | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

The publication gate is process-local and the V1 deployment uses one management replica with a `Recreate` strategy, as planned. The real K3s skill invocation after replacement is the PR 1.5 milestone check.

## Verification Results

- FAIL — `go test ./internal/publication -run '^TestMountedPackDiscoversVariablesWithZIP64DirectorySize$' -count=1` before the fix, with `invalid-zip` returned for the fixture.
- PASS — `go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack|Validate|Publish|EngineClient|Reconcile|Eligibility|Recovery)' -count=1`.
- PASS — `go test ./...`, `go test -race ./...`, `go build ./...`, and `go vet ./...`.
- PASS — `& ../execution-engine/target/verify-pr14-postgres.ps1` from `management-backend` before and after the fix. Each run used a fresh loopback PostgreSQL container with the production migration and scoped cleanup.
- PASS — `kubectl kustomize deploy/k3s` and `git diff --check 1b945b8 -- management-backend deploy/k3s`. The two edited untracked Go files were checked with `git diff --no-index --check`.

## Residual Risks and Optional Developer Checks

None. PostgreSQL verification used a disposable loopback database. The cumulative K3s scenario remains assigned to PR 1.5.

## Disposition

- `fixes-applied`. Run a fresh independent Step 5 review on the revised PR 1.4 scope.

## Step Report: 5_code_review

STATUS: complete
ARTIFACTS:
  - ai/thoughts/reviews/pr-1.4-record-and-restore-publications-review-3.md
  - management-backend/internal/publication/zip_discovery.go
  - management-backend/internal/publication/pack_test.go
SUMMARY: Fixed variable discovery for ZIP64 archives whose classic record marks only the central directory size. The final Go checks, manifest rendering, and disposable PostgreSQL integration passed.
DECISIONS:
  - Normalize the already validated private reader view while preserving the mounted and dispatched archive bytes.
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
VERIFICATION:
  - FAIL — The focused ZIP64 regression reproduced the rejection before the fix.
  - PASS — Focused and full Go tests, race, build, vet, Kustomize rendering, disposable PostgreSQL integration, and whitespace checks.
OPTIONAL_DEVELOPER_CHECKS: none
REVIEW_RESULT: fixes-applied
NEXT: Run a fresh independent Step 5 review.
