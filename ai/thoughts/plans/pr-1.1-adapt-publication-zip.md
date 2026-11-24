# Cloud Publication ZIP Adapter Implementation Plan

## Overview

- Ticket — `ai/thoughts/tickets/pr-1.1-adapt-publication-zip.md`
- Research — `ai/thoughts/research/pr-1.1-adapt-publication-zip.md`
- Outcome — A bounded adapter takes a multipart publication pack and returns its portable configuration and exact Framework skill document sources for the requested namespace.

## Current State

`execution-engine/pom.xml` builds the Boot application on Java 21 with the Framework starter snapshot. `ExecutionEngineApplication` is the only Cloud Java source at the baseline. Boot and Tomcat parse multipart data before MVC dispatch. Framework's `SkillDocument` carries authored YAML and an exact source label. Framework's `AuthoredYamlVariables` and `YamlSkillCatalog` parse and validate the supplied skill documents, with a single YAML document expected per source. Framework's `ExecutionConfiguration` applies its own configuration limit when later created.

## Desired End State

A shared adapter accepts the original named `pack` part and returns authored portable YAML plus an immutable list of `SkillDocument` records. Both ZIP and expanded byte ceilings are inclusive and apply before retaining excess bytes. The archive reader establishes the 1,000-entry ceiling before creating an index, checks names and execution types, reads only execution payloads, and reports the pack-relative paths used for diagnostics. Zero-skill archives produce an empty document list.

## Scope

### In scope

- Multipart ingress configuration, archive preflight, execution extraction, reserved field checks, and focused tests under the Java module.
- Counting companion entries and accounting for their declared uncompressed lengths while leaving their payloads compressed. The Go staging contract checks actual whole-pack expansion separately.

### Out of scope

- Validate and publish handlers, Framework generation construction, runtime settings composition, and publication state.
- Go staging checks for companion file types, companion collisions, companion entry point, and actual companion contents.

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Impact and Risk Analysis

The servlet layer can buffer uploads before a controller sees a part, so limiting only the adapter stream cannot establish the transfer boundary. Central-directory counts and expanded lengths are untrusted, and a ZIP library can allocate an index before giving its caller access to entries. The adapter also cannot rely on a library's normalized name when detecting slash, case, and traversal ambiguities. YAML object conversion could change alias and non-string-key behavior before Framework receives authored content.

## Implementation Approach

Use Boot's standard multipart support with a disk threshold of zero and `spring.servlet.multipart.max-file-size=1000000B`. That limit is applied by the servlet parser during transfer, inclusive of all ZIP bytes inside the part and independent of multipart framing. Keep Boot's existing request envelope limit. Accept the original MIME part through `jakarta.servlet.http.Part` and use an incrementally bounded read of its input stream without relying only on the part's reported size. The request parser can return a multipart failure before the adapter runs. PR 1.2 will map that rejection to the endpoint error response.

Use Apache Commons Compress 1.28.0 for seekable ZIP entry metadata and execution payload streams. Introduce a small central-directory preflight that locates the end record, verifies directory bounds and a single-disk layout, and traverses actual central file headers up to the first excessive entry. Handle the ZIP64 end structures when present, under the same archive bounds. Keep preflight to envelope validation and counting. Commons Compress owns payload decompression and detailed entry metadata after the cap is established. Verify that the indexed set agrees with preflight. Because seekable access leaves companion data compressed, count the companion entries' declared expanded byte lengths as part of the package limit without opening them. For execution content, enforce the remaining byte allowance from actual incrementally expanded chunks and check agreement with entry metadata. No expanded bytes are generated to skip companions in this reader.

Use the SnakeYAML Engine low-level composer with core schema and a code-point bound at least as large as the entire expanded ZIP allowance. Inspect only the authored top-level `loomspan` mapping for reserved string keys, including aliased mappings and keys. The returned configuration remains the original text. Preserve each skill's raw UTF-8 YAML and leave document syntax, shape, definitions, ownership, and references to Framework's existing single-document candidate parsing. This also permits the next ticket to report independent Framework findings for different skill sources.

## Phase 1 — Bounded multipart and archive envelope

### Changes

- [x] `execution-engine/pom.xml` — declare Commons Compress 1.28.0 and SnakeYAML Engine 3.0.1 for direct use and add the matching Boot test starter.
- [x] `execution-engine/src/main/resources/application.properties` — configure the inclusive decimal multipart part size and disk threshold.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java` — expose `adapt(String namespace, Part pack)`, check the part name and ZIP content type, and read into bounded seekable storage using fixed-size chunks. Check the remaining allowance before retaining bytes. Close all streams and any backing storage on rejection.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/ZipDirectoryPreflight.java` — verify an end record at the end of the archive, with its declared comment considered, match the directory envelope and counts for a single archive, and walk fixed-size central headers plus bounded variable fields without allocating an index. Count an explicit directory the same way as a file. Stop at entry 1,001. Check positions and arithmetic before advancing past any record. A supported ZIP64 envelope must apply the same checks to its long-valued directory coordinates and counts. Match the preflight's envelope interpretation to the subsequent Commons index so a forged alternate directory cannot bypass it.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationMultipartTest.java` and `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationZipAdapterTest.java` — prove the first parser boundary and the adapter's independent ZIP and actual-entry boundaries.

### Automated verification

- [x] From `execution-engine/`, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationMultipartTest,PublicationZipAdapterTest' test` — accepted and rejected edges and malformed metadata.

### Optional developer checks

None. The local parser integration exercises multipart before either production handler exists.

## Phase 2 — Execution inputs and diagnostics

### Changes

- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java` — validate each authored path from the actual entry name, including companion names, before skipping or opening any payload. Check raw names for silent separator replacement or decoding substitution, and reject conflicting local and central names rather than treating an unsafe header as another spelling. Reject absolute paths, control characters, backslashes, repeated separators, and dot or dot-dot components. Require an exact namespace as the first component, with at least one descendant and no root file.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java` — classify the execution subtree by its first pack-relative component, catching a differently cased spelling as an invalid layout. Track explicit execution entries and implied parent directories case-insensitively. Allow an explicit directory to match the same implied directory's spelling, but reject duplicate explicit entries, incompatible types, and file/directory or case-folded conflicts. Allow the exact execution configuration file once and any lowercase `*.yaml` file beneath `loomspan/skills/`, at any depth. Report the path and first offending lowercase component. Allow directory entries for the execution root and skills hierarchy. Reject nonregular execution file types using external attributes, while allowing regular files whose valid ZIP format carries no Unix type.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java` — check declared lengths for the package as an early bound. Keep companion expansion accounting separate so actual chunks from execution entries are counted once. Bound the execution file chunks before retaining them, verify their sizes and CRC, and confirm explicit execution directories contain no expanded data. Decode execution YAML as strict UTF-8 and identify the path on malformed content or stream failure.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java` — inspect `loomspan/execution.yaml` for authored `loomspan.session` and `loomspan.execution-trace` keys and report each present path. Return the immutable `AdaptedPublication` record with `executionYaml` and `skills`, directly consumable by both PR 1.2 handlers.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationZipAdapterTest.java` — cover execution paths, application limits, unchanged YAML, skipped companions, reserved keys, and zero skills.

### Automated verification

- [x] From `execution-engine/`, `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify` — all local checks pass and Boot repackages the module.

### Optional developer checks

None. PR 1.2 owns the production endpoint wiring and PR 1.5 owns cumulative live-stack verification.

## Test Strategy

Build generated ZIP fixtures without unpacking them to the filesystem. Exercise central claims and actual execution content independently. Use the embedded servlet container with a test-only controller to demonstrate where multipart rejection happens. Exercise only Java and local loopback.

## Acceptance-Criteria Traceability

| Acceptance criterion | Planned code evidence | Planned test evidence |
| --- | --- | --- |
| Valid pack with companions, nested skill sources and portable configuration, plus a zero-skill pack | `PublicationZipAdapter.adapt`, immutable `AdaptedPublication`, exact source-name construction | `extractsOnlyExecutionInputsWithOriginalSourceNames` and `acceptsPortablePackWithoutSkills` |
| Inclusive ZIP, expansion and entry limits, enforced during reading, with explicit directories and skipped companions accounted for | Boot/Tomcat multipart configuration, `ZipDirectoryPreflight`, bounded payload read and compressed companion accounting | Multipart parser boundary plus adapter raw, actual central-count, aggregate expansion and false-size fixtures |
| Reject malformed paths, layout, spelling, types, execution collisions and reserved sections | Path and layout inventory before payload reads, ZIP entry type checks, authored YAML key checks | Parameterized invalid archives with full path and component or both conflicting paths, plus reserved key cases |

## Risks and Rollback/Recovery

All changes are local to the Java module, and the adapter extracts no files from the archive to paths supplied by the pack. Reject invalid data before constructing a result. The adapter installs no generations and introduces no persisted format, so reverting this ticket requires no data migration. Cross-ticket integration starts in PR 1.2.

## Implementation Decisions

- Present the preflight-validated directory to Commons Compress in a private in-memory view. Remove the opaque archive comment from that view. Where a standard directory ends with bytes that Commons Compress would misread as a ZIP64 locator, give the reader an explicit ZIP64 footer for the same directory. The transport limit counts the original metadata, and the directory entries and file payloads retain their original bytes.
- Compare sorted paths component by component. This checks explicit and implied execution paths without retaining a separate index of all implied ancestors.
- Pin Surefire to 3.5.6. The Boot parent's 3.6.1 provider is absent from the local Maven cache. The pinned provider runs the tests offline with the JUnit version managed by Boot.

## References

- `ai/thoughts/tickets/pr-1.1-adapt-publication-zip.md`
- `ai/thoughts/research/pr-1.1-adapt-publication-zip.md`
- `ai/thoughts/future/publication-package-format.md`
- `ai/thoughts/future/java-execution-engine-roadmap.md`
- `ai/thoughts/future/execution-engine-api.md`
- Framework `SkillDocument`, `AuthoredYamlVariables`, and `YamlSkillCatalog`
