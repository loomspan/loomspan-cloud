# PR 1.1 Publication ZIP Adapter Code Review — Cycle 2

## Scope and Repository State

Compared `main` and its working tree with `bc217016f62528855c70cbf505dd092657cabe18`. The reviewed scope includes the staged PR 1.1 research, both PR 1.1 plans, the Maven changes, and the untracked adapter, preflight, multipart properties, and Java tests. PR 1.2 through PR 1.5 process artifacts belong to their respective tickets. No repository design guardrail is active. The Java bootstrap remains unchanged, and the test controller exercises the adapter through Boot’s embedded Tomcat multipart boundary.

## Findings

No actionable findings.

The review traced the original part from Tomcat through the measured ZIP read, preflight, Commons Compress, path and type checks, execution expansion, YAML inspection, and the Framework document boundary. Streams and the seekable reader close on success and rejection. Archive metadata is checked against bounded offsets before indexing, and preflight and the seekable reader select the same directory. The path and namespace checks also apply to companion entries. Compound case conflicts are checked across explicit paths and their shared ancestors. Execution reads use a single remaining allowance and check sizes and CRCs. Reserved field checks examine the composed YAML mapping while the returned text keeps its authored content.

## Findings Resolved in This Context

None.

## Acceptance-Criteria and Plan Conformance

| Criterion or decision | Code evidence | Test evidence | Result |
| --- | --- | --- | --- |
| A valid pack retains the exact skill source names and authored configuration. A zero-skill pack succeeds. | `PublicationZipAdapter.java:265` and `AdaptedPublication.java:7` | `extractsOnlyExecutionInputsWithOriginalSourceNames`, `acceptsPortablePackWithoutSkills` | Implemented |
| The inclusive 1,000,000-byte ZIP limit applies during transfer and includes metadata inside the ZIP. | `application.properties:2`, `PublicationZipAdapter.java:80` | Embedded Tomcat accepts a chunked exact-size part with separate metadata and rejects the larger part before dispatch. A counting stream proves the adapter stops at the first excess byte. | Implemented |
| The inclusive 1,000-entry limit applies before allocating an index and counts explicit directories. | `PublicationZipAdapter.java:63`, `ZipDirectoryPreflight.java:84` | Exact and excessive actual central directory lengths, explicit directories, underclaimed count, invalid envelope, standard ZIP and ZIP64, and signature-shaped comment bytes | Implemented |
| Execution expansion applies the inclusive 10,000,000-byte limit without a per-file cap. | `PublicationZipAdapter.java:116`, `PublicationZipAdapter.java:265`, `PublicationZipAdapter.java:306` | Exact and excess bytes in a single execution file, a false declared size, companion declaration accounting, an unread corrupted companion payload, and data under an execution directory | Implemented |
| Entry paths, top-level namespace, execution layout and type, and case-insensitive collisions are validated with authored diagnostics. | `PublicationZipAdapter.java:143`, `PublicationZipAdapter.java:157`, `PublicationZipAdapter.java:184`, `PublicationZipAdapter.java:206`, `PublicationZipAdapter.java:217`, `PublicationZipAdapter.java:244` | Parameterized path and layout failures, local/central path disagreement, extra root, invalid requested root, symlink, explicit and implied path conflicts, and Unicode source preservation | Implemented |
| Authored `loomspan.session` and `loomspan.execution-trace` are rejected and reported by field path. Framework owns skill definitions, references, and configuration ownership. | `PublicationZipAdapter.java:284`, `PublicationZipAdapter.java:327` | Direct and aliased reserved fields, a permitted nested `session` data field, and original YAML with a Framework-owned configuration selector and an unresolved executor | Implemented |

## Active Project Guardrails

None recorded.

## Open Questions and Assumptions

The compatible seekable ZIP reader opens only execution payloads. Companion entries are accounted for using declared expansion, and their actual contents and expanded sizes are the responsibility of the whole-pack staging checks.

## Verification Results

- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationMultipartTest,PublicationZipAdapterTest' test` from `execution-engine/` — 32 tests passed.
- PASS — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` from `execution-engine/` — 32 tests passed on JDK 21 and Maven repackaged the executable Boot JAR.
- PASS — `git diff --check -- execution-engine` and `git diff --cached --check` — no whitespace errors.

## Residual Risks and Optional Developer Checks

Production validate and publish wiring is owned by PR 1.2. Cumulative K3s verification is owned by PR 1.5. No PR 1.1 developer checks remain.

## Disposition

`clean`. This context changed only this review artifact.
