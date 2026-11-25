# PR 1.4 publication and recovery review — Cycle 2

## Scope and Repository State

Reviewed the staged, unstaged, and untracked PR 1.4 changes against `1b945b8`. The scope covered the publication service, mounted ZIP discovery, authenticated engine client, PostgreSQL adapter and migration, startup wiring, deployment configuration, and their tests and plans. Concurrent PR 1.5 Java and process artifacts and the unrelated `.vscode` settings were excluded.

## Findings

No actionable findings remain after the fix below.

## Findings Resolved in This Context

### [P2] Decode legacy ZIP names consistently with the Java adapter

- Location `management-backend/internal/publication/pack.go:99`
- A supported ZIP can encode a lowercase non-ASCII skill filename in IBM437 with the UTF-8 language flag unset. Java decodes and validates that name in `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java:144`. Go's `archive/zip` returns the filename bytes unchanged. The Cloud scanner used those bytes as the authored source name, so a missing-value blocker could contain an invalid UTF-8 filename and appear with a replacement character in JSON.
- The focused regression in `management-backend/internal/publication/pack_test.go:155` first returned `loomspan/skills/caf\x82.yaml` for the source of the missing variable. It now returns `loomspan/skills/café.yaml` with the original `config.path`.
- The scanner now decodes selected names using the archive's UTF-8 flag and the same IBM437 fallback as Java. It uses `golang.org/x/text` at the existing version and still dispatches the original ZIP bytes.

## Acceptance-Criteria and Plan Conformance

| Criterion | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Combined validation belongs to the captured revision and settings. Missing and blank values block publish while the Java validation request still runs. Incomplete validation returns no partial findings. | `service.go:131`, `postgres.go:55`, `variables.go:37`, `engine_client.go:65` | `TestValidate`, `TestVariableCorpus`, `TestPublish`, and `TestEngineClient` cover blocked values, six occurrences and four distinct names, revision/settings changes, and incomplete responses. | Implemented |
| A stale reviewed revision requires confirmation of the reviewed/current pair. Publishing the already-current revision sends nothing. | `service.go:166` | `TestPublish` covers stale confirmation, preserved newer staging, and the exact already-published result with no dispatch. | Implemented |
| First authenticated snapshot permits publish. Missing inputs and known shutdown prevent dispatch. A received Java rejection reaches the caller verbatim. | `service.go:215`, `service.go:270`, and `engine_client.go:82` | `TestPublish` and loopback `TestEngineClient` cover dispatch before readiness, no-send cases, authenticated rejection text, and uncertain outcomes. | Implemented |
| Authenticated `204` precedes a durable pointer and caller success. Owner and restore dispatch serialize per application. Routing waits for a matching snapshot and an install requests refresh. | `service.go:285`, `postgres.go:205`, `reconcile.go:29`, and `reconcile.go:139` | Real `TestPublicationPostgres` verifies the transactional boundary and reopening. `TestRecoverySerializesWithPublish` verifies both orderings. `TestEligibility` checks observed route and trace policy. | Implemented |
| Replacement restores the acknowledged ZIP and version with current inputs. Matching applications remain installed, failures are reported independently, and unintended namespaces stay ineligible. | `reconcile.go:74`, `reconcile.go:110`, `reconcile.go:139`, and `management-backend/cmd/management-backend/main.go:41` | `TestRecoveryAfterDurableAcknowledgment`, `TestReconcile`, `TestReconcileRetriesOnlyOnFreshAssociation`, and `TestPublicationPostgres`; deployment manifest renders. | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

- Recovery and owner dispatch share the keyed gate in the V1 single backend. The deployment uses one replica with `Recreate`.
- The actual K3s skill invocation after replacement belongs to PR 1.5 under the ticket's pipeline notes. The PR 1.4 review exercises real PostgreSQL and authenticated loopback transport with deterministic replacement and observation tests.

## Verification Results

- FAIL — `go test ./internal/publication -run '^TestMountedPackPreservesLegacyZIPSourceName$' -count=1` — reproduced the raw IBM437 source bytes before the fix.
- PASS — `go test ./internal/publication -run 'Test(VariableCorpus|VariableDiscovery|RuntimeSettings|MountedPack|Validate|Publish|EngineClient|Reconcile|Eligibility|Recovery)' -count=1`.
- PASS — `go test ./...`, `go test -race ./...`, `go build ./...`, and `go vet ./...` from `management-backend/`.
- PASS — `& ../execution-engine/target/verify-pr14-postgres.ps1` from `management-backend/`, before and after the fix. The disposable PostgreSQL 17 container bound only to loopback and was stopped by the script after each run.
- PASS — `kubectl kustomize deploy/k3s` rendered successfully.
- PASS — `git diff --check 1b945b8 -- management-backend deploy/k3s`, `git diff --cached --check`, and the `gofmt -l` check on the two edited Go files.

## Residual Risks and Optional Developer Checks

None for this ticket. PR 1.5 owns the cumulative deployed skill exercise.

## Disposition

`fixes-applied`. The change requires a fresh Step 5 review.

## Step Report

STATUS: complete
ARTIFACTS:
  - ai/thoughts/reviews/pr-1.4-record-and-restore-publications-review-2.md
  - management-backend/internal/publication/pack.go
  - management-backend/internal/publication/pack_test.go
  - management-backend/go.mod
SUMMARY: Fixed incorrect Cloud blocker source names from valid IBM437 ZIP entries. The complete Go and disposable PostgreSQL checks passed.
DECISIONS:
  - Match Java's UTF-8 flag and IBM437 fallback using the existing x/text version.
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
REVIEW_RESULT: fixes-applied
NEXT: Launch fresh independent review cycle 3.
