# PR 1.1 Cloud Publication ZIP Adapter Code Review — Cycle 1

## Scope and Repository State

Compared staged, unstaged, and untracked PR 1.1 work with `bc217016f62528855c70cbf505dd092657cabe18`. The review covered the Java module, multipart configuration, Maven dependencies, and PR 1.1 research and plans. Uncommitted artifacts for later tickets were outside this review. Traced multipart admission through Tomcat and MVC, the preflight and Commons Compress views, and the Framework single-document consumer for returned skill YAML. Tests used the cached Framework starter with JDK 21.0.2 and Maven 3.9.11.

## Findings

No actionable findings remain after the fixes below.

## Findings Resolved in This Context

### [P2] Distinguish ZIP records from bytes inside opaque comments

- Location — `execution-engine/src/main/java/ai/loomspan/cloud/engine/ZipDirectoryPreflight.java:25` and `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java:65`.
- Scenario — An ordinary archive comment contains an end-record-shaped sequence with a matching comment-length suffix but invalid directory metadata. A central entry comment can also end in bytes resembling a ZIP64 locator, with the ordinary directory still describing the complete central index.
- Impact — Either archive was rejected even though its execution paths, directory records, and payloads met the publication format. Commons Compress independently probes for a locator before the ordinary end record, so preflight alone could not correct both cases.
- Evidence — The strengthened `ignoresEndRecordSignaturesInAnArchiveComment` fixture failed in the original end-record search. The new `ignoresZip64LocatorSignatureInAnEntryComment` fixture failed in preflight until the complete footer fix. Both pass in the final focused suite.
- Fix — Select a structurally valid end record, reject competing valid interpretations, and keep the actual entry cap in preflight. Give Commons Compress a private view of that same directory. Remove the opaque archive comment or, for a misleading ZIP64 locator signature, supply an explicit footer for the same central records. The original upload bytes still determine the transfer size, and original central entries and file data are preserved.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| Valid archive with companions and exact nested source names, including zero skills | `PublicationZipAdapter.extract` and `readExecution` keep payloads and pack-relative names, and `AdaptedPublication` copies the skill list | `extractsOnlyExecutionInputsWithOriginalSourceNames`, `acceptsPortablePackWithoutSkills`, and the Unicode source-name case | Implemented |
| Inclusive limits before buffering or retention, with explicit directories and skipped companions covered | Tomcat multipart properties, entry preflight before `ZipFile.get()`, declared companion accounting, bounded actual byte reads and CRC checks | Loopback chunked multipart test, exact and above-limit byte and entry tests, underreported transfer and execution expansion, and corrupted unopened companion payload | Implemented |
| Reject path and layout problems, nonregular execution inputs, execution collisions, and reserved sections | Central and local header consistency, authored path checks, sorted collision detection, execution type checks, and YAML node inspection | Invalid path and layout parameter tests, local and central name conflicts, nonregular and file-directory collision cases, and reserved keys and aliases | Implemented |
| Agree on the central directory with the reader and keep checking limits on original ZIP bytes | `ZipDirectoryPreflight.inspectDirectory` and `Directory.readerArchive` choose the same index without changing admitted content | Both opaque comment cases, actual ZIP64, and remaining central-directory and size tests | Implemented |
| Leave Framework validation of skill definitions, references, and the execution owner key; wire endpoints in PR 1.2 | Adapter returns the authored configuration and skill documents, while Framework `YamlSkillCatalog` checks supplied YAML | Exact YAML assertion uses an unrelated executor owner and unknown skill executor | Implemented |

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Open Questions and Assumptions

None for this ticket. The seekable reader skips companion payloads and relies on Go staging to check their actual contents, consistent with the ticket boundary.

## Verification Results

- FAIL — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationZipAdapterTest#ignoresEndRecordSignaturesInAnArchiveComment' test` — confirmed the original false rejection.
- FAIL — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationZipAdapterTest#ignoresZip64LocatorSignatureInAnEntryComment' test` — confirmed the original false rejection.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationMultipartTest,PublicationZipAdapterTest' test` — 32 tests passed after both fixes.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — 32 tests passed and Boot repackaged the executable JAR.
- PASS — `git diff --check` and `git diff --cached --check`.

## Residual Risks and Optional Developer Checks

None for PR 1.1. The test-only controller exercises actual servlet ingress. Later tickets cover the production handlers and cumulative K3s checks.

## Disposition

`fixes-applied`. A fresh review context must review and verify the resulting change.
