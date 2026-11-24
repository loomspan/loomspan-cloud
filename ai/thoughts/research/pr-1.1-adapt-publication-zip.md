---
date: 2026-11-24
repository: loomspan-cloud
branch: main
commit: bc217016f62528855c70cbf505dd092657cabe18
ticket: ai/thoughts/tickets/pr-1.1-adapt-publication-zip.md
tags: [execution-engine, publication, zip, multipart]
---

# Cloud publication ZIP adapter research

## Research Question

What receives and interprets a publication pack today, and where are the multipart, ZIP, YAML, and Framework boundaries for the new Java adapter?

## Summary

Cloud currently has a Spring Boot bootstrap, a standalone Maven build, and published design contracts. There is no Cloud multipart handler, archive reader, or package test suite. The neighboring Framework checkout exposes immutable YAML inputs for host-managed publication, with an exact source label on each skill document.

## Repository State

Read on 2026-11-24 at 15:07 Pacific. `main` matched the local `origin/main` tracking ref at `bc217016f62528855c70cbf505dd092657cabe18`. The Cloud checkout had no tracked or untracked changes. Framework source evidence comes from the adjacent `C:/opendev/code/loomspan-framework` checkout at `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`, which has unrelated working-tree deletions in its process documents. Its source checkout and the local Maven snapshot are independent evidence.

## Current Behavior and Data Flow

- `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6` starts Spring Boot. There is no Cloud-owned package ingestion route, multipart configuration, or application resources folder at the baseline. `execution-engine/pom.xml:16` targets JDK 21, embeds Framework `1.0.0-beta.10-SNAPSHOT`, and adds Boot Web MVC. The Boot parent is `4.2.0-RC2`.
- The API contract describes two future multipart routes sharing the original `application/zip` `pack` bytes. The `application/json` `metadata` part carries the target namespace and the endpoint-specific metadata. PR 1.2 owns the endpoints and links both to this adapter. See `ai/thoughts/future/execution-engine-api.md:25` and `ai/thoughts/tickets/pr-1.1-adapt-publication-zip.md:22`.
- The package format counts the whole ZIP including metadata, all expanded file contents, and explicit directory entries. Its inclusive ceilings are 1,000,000 ZIP bytes, 10,000,000 expanded bytes, and 1,000 entries, with no separate per-file package ceiling. It describes stronger whole-pack rules for staging. The Java roadmap explicitly calls for checking every path for ambiguity, checking the top-level namespace, and enforcing execution-specific layout, spelling, types, and case-insensitive uniqueness. It permits readers to leave companion payloads compressed and counts any companion bytes the reader expands to skip them. See `ai/thoughts/future/publication-package-format.md:68` and `ai/thoughts/future/java-execution-engine-roadmap.md:13`.
- The format defines exactly one `loomspan/execution.yaml` and zero or more lowercase `loomspan/skills/**/*.yaml` documents. Configuration supports the minimal `loomspan: {}` and forbids authored `loomspan.session` and `loomspan.execution-trace`, even if empty. The Java roadmap assigns Framework the validation of skill definitions, references, and the executor configuration owner key. See `ai/thoughts/future/publication-package-format.md:39` and `ai/thoughts/future/java-execution-engine-roadmap.md:21`.

## Key Components

- `../loomspan-framework/src/main/java/ai/loomspan/api/SkillDocument.java:3` defines `SkillDocument(sourceName, yaml)`. The name is the exact label shown in diagnostics, independent of the declared skill name. The host checks definition paths before creating these records.
- `../loomspan-framework/src/main/java/ai/loomspan/api/ExecutionConfiguration.java:6` defines an authored YAML string with a constructor ceiling of 1,000,000 UTF-16 code units. This is an existing Framework validation boundary distinct from Cloud's archive limits. The later endpoint ticket composes the final configuration before calling Framework.
- `../loomspan-framework/src/main/java/ai/loomspan/api/SkillReloader.java:14` accepts `Collection<SkillDocument>` and `ExecutionConfiguration` for complete candidate validation and preparation. Preparation and publication are separate operations.
- `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/YamlValues.java:21` shows Framework uses SnakeYAML Engine with `CoreSchema`, duplicate keys disallowed, and a separate expanded YAML value count. Authored variable processing uses SnakeYAML's node composer in `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:3`.

## Affected Areas

| Area | Current behavior and evidence |
| --- | --- |
| First multipart byte buffering | Cloud has no override. Local Boot dependencies pin Tomcat 11.0.27. Bytecode for `org.apache.catalina.connector.Request.parseParts` applies the servlet `MultipartConfigElement` spool threshold, per-file size, and total request size to the multipart parser before creating part objects. A later read from a part is distinct from that parser boundary. |
| ZIP structure and entry type | The Cloud build has no ZIP library declaration. Locally cached Commons Compress 1.28.0 exposes Unix mode and symlink checks on `ZipArchiveEntry`. Its `ZipFile` construction populates from the central directory before entries can be enumerated. `getInputStream(entry)` is separate from that index operation. |
| Raw ZIP names and stream skipping | The same library's bytecode shows that `ZipArchiveEntry.setName` can replace backslashes for DOS-platform names, and it exposes `getRawName`. `ZipArchiveInputStream` reads entries sequentially and may invoke `skip(Long.MAX_VALUE)` when implicitly closing a prior entry. The entry type and path available in a local header do not necessarily carry the central directory's attributes or spelling. |
| Portable configuration | No Cloud code currently checks reserved keys, decodes skill YAML, or preserves pack paths. `SkillDocument` retains its supplied string and label. |

## Existing Tests and Fixtures

The baseline Cloud tree contains no test sources or ZIP fixtures. `mvn -o verify` in `execution-engine/` succeeded and produced a repackaged Boot JAR, with no test sources to compile. In the neighboring Framework checkout, `../loomspan-framework/src/test/java/ai/loomspan/internal/skill/SkillReloaderTest.java:21` covers reference-free publication and shared variables, and its tests beginning at lines 94 and 137 cover source labels and namespace replacement. Those tests exercise Framework inputs and lifecycle, not the Cloud archive or multipart boundaries.

## Dependencies and Operational Constraints

`mvn -version` reports Maven 3.9.6 and Java 21.0.2 from `JAVA_HOME`; `java` is absent from `PATH`. The local Maven cache has the configured Framework starter snapshot and Boot parent, as shown by the successful offline build. It also contains Commons Compress 1.28.0, SnakeYAML Engine 3.0.1, and Boot's matching test starter. Cache presence alone does not establish that any proposed new build dependency will resolve or that the neighboring Framework sources are identical to the cached snapshot.

## Historical Context

The current milestone tickets were added at `bc21701`. The Java roadmap describes the checked-in project as the buildable wrapper foundation. The PR 1.1 pipeline note allows incomplete endpoint wiring at the intermediate commit and places cumulative end-to-end verification in PR 1.5.

## Open Questions

- Which incoming multipart boundary will own the transfer limit once PR 1.2 adds the endpoints, and which part of that behavior can this ticket verify independently?
- How will the implementation establish the actual number of archive entries before an eager reader allocates the complete index, including malformed metadata whose claimed count disagrees with the observed records?
- What structural checks distinguish a single authored YAML document from Framework skill semantics, and how is the existing Framework configuration ceiling handled when forming the composed configuration in PR 1.2?

## Step Report: 1_research_codebase

STATUS: complete
ARTIFACTS:
  - ai/thoughts/research/pr-1.1-adapt-publication-zip.md
SUMMARY: Traced the multipart, ZIP, YAML, and Framework boundaries and established the offline Maven baseline. No Cloud adapter or focused Cloud tests exist at the baseline.
DECISIONS:
  - none
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Start implementation and testing planning in one context.
