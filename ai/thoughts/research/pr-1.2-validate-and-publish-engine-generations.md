---
date: 2026-11-24
repository: loomspan-cloud
branch: main
commit: bc217016f62528855c70cbf505dd092657cabe18
ticket: ai/thoughts/tickets/pr-1.2-validate-and-publish-engine-generations.md
tags: [execution-engine, staged-validation, publication, variables, vertex]
---

# Private Framework generation validation and publication research

## Research Question

Which Cloud and Framework entry points, data flows, and diagnostics exist for validating an authored candidate and installing its resolved private generation? Where can a host see effective connection settings before Framework constructs a provider client?

## Summary

The Cloud baseline is the Spring Boot application and embedded Framework starter. Framework's supported reloader provides the required validation, exact-map preparation, and atomic publication entry points. The starter exposes no supported effective-connection policy callback. Its internal execution parser can bind typed effective execution settings without building clients and uses the same staging, substitution, and binding code as preparation.

## Repository State

Cloud was inspected on 2026-11-24 at 15:14 Pacific. `main` matched the local `origin/main` ref at the commit above. The staged research document and untracked planning artifacts belonged to PR 1.1, whose implementation was running in a separate step. This research identifies the checked-in Cloud baseline separately from that work's planned integration. The adjacent Framework checkout is `C:/opendev/code/loomspan-framework` at `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`. Existing Framework working-tree deletions are process documents and are outside this ticket.

## Current Behavior and Data Flow

### HTTP and YAML boundaries

- `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6` starts Boot. At the baseline there is no Cloud controller, deployment configuration, tenant extension bean, or HTTP test. `execution-engine/pom.xml:15` selects JDK 21, Boot 4.2.0-RC2, and Framework starter 1.0.0-beta.10-SNAPSHOT.
- The PR 1.1 plan names `PublicationZipAdapter.adapt(String namespace, Part pack)` and an immutable `AdaptedPublication` with the portable execution YAML and `SkillDocument` values. The planned execution YAML is authored text. The skills retain the precise pack-relative source labels. The plan's adapter checks the reserved keys and pack media type and defers Framework interpretation. See `ai/thoughts/plans/pr-1.1-adapt-publication-zip.md:51` and `:67–70`. This is planned handoff evidence as of this scan.
- The wire contract defines two named MIME parts and all mandatory runtime settings. Metadata is a separate JSON object. `runtimeSettings.session` includes a duration string, integer depth, a size string with an explicit unit, and six integer quotas. The trace object contains a closed persistence choice. Framework's execution parser is a YAML configuration binder that turns several scalar types into strings for Spring binding and supplies defaults for omitted configuration. Its behavior alone cannot establish the required JSON types or presence. See `ai/thoughts/future/execution-engine-api.md:25–57` and `../loomspan-framework/src/main/java/ai/loomspan/internal/skill/ExecutionConfigurationParser.java:122–152,265–271`.
- Framework's `ExecutionConfiguration` record accepts one YAML string, with a ceiling of 1,000,000 UTF-16 code units. Framework's authored parser selects field boundaries on YAML nodes and recognizes complex mapping keys, aliases, and selected whole-node variable markers. The cached SnakeYAML Engine 3.0.1 library exposes low-level `Compose.composeString`, `MappingNode.getValue/setValue`, `Serialize.serializeOne`, and `Present.emitToString`; these operate on YAML nodes. Ordinary JSON object keys cannot represent the allowed mapping-key markers. See Framework `src/main/java/ai/loomspan/api/ExecutionConfiguration.java:6–11` and `src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:54–75,134–159,186–207`.

### Framework validation and generation lifecycle

- `SkillReloader.validate(namespace, documents, configuration, visibility)` captures the active metadata view while lifecycle admission is open. The manager parses an authored staged configuration, checks supplied skills and dependencies, and produces a `SkillValidationResult`. This path reads no application variables and never calls the runtime factory. The result's `valid()` tests for errors, with warnings permitted. Optional issue fields are nullable, and a result with errors cannot carry skills. The implementation uses the opaque `<configuration>` source label and `configuration` field on a thrown configuration check. Passed skill documents retain their source labels. See Framework `src/main/java/ai/loomspan/internal/skill/DefaultSkillReloader.java:37–48,142–152`, `src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:183–209,236–251`, and `src/main/java/ai/loomspan/api/SkillValidationResult.java:10–24`.
- The manager catches runtime exceptions from authored configuration binding into a completed configuration issue before it reaches skill dependency checks. A closed lifecycle before validation reaches the manager is instead a thrown `SkillReloadException`. For a structurally valid Vertex connection that merely omits explicit credentials, Framework's stanza permits ADC, so its skill check remains independently available. Credential fields outside Framework's strict allowlist can stop binding and thus its skill-reference checks. See Framework `src/main/java/ai/loomspan/internal/skill/ExecutionConfigurationParser.java:76–105,273–281` and `src/main/java/ai/loomspan/autoconfigure/LoomspanProperties.java:232–253`.
- `prepare(namespace, publicationVersion, documents, configuration, visibility, variableValues)` copies the supplied map. The manager builds and stages one authored candidate, checks it, resolves all selected occurrences, binds the typed resolved configuration, rechecks effective skills, and creates a detached runtime through its runtime factory. Explicit values use the supplied copied map exclusively. `PreparedCandidateVariables.resolve` rejects unused names and memoizes lookup, while the YAML document checks missing or blank values at the authored occurrence and rejects effective mapping-key collisions with both authored locations. Framework treats resolved strings, including literal placeholders, as terminal. See Framework `src/main/java/ai/loomspan/internal/skill/DefaultSkillReloader.java:73–94`, `src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:168–181,253–286`, `src/main/java/ai/loomspan/internal/skill/PreparedCandidateVariables.java:62–79`, and `src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:226–279`.
- `publish` accepts that same framework instance's detached candidate exactly once, compares the captured base generation ID to the active target, swaps only the target namespace, and retires old state after captured work releases it. Generation IDs are allocated independently of the publication version. An empty candidate stays loaded as a namespace and preserves its fixed host registrations. An unpublished candidate's close releases its runtime, while closing a successfully published candidate is inert. See Framework `src/main/java/ai/loomspan/internal/skill/DefaultSkillReloader.java:98–128,169–192` and `src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:253–282,297–322`.

### Effective Vertex credential boundary

- Framework's connection parser allows only `vertex-ai`, `project-id`, `location`, and `credentials-json` under `gemini`. It drops unresolved keys for staged binding, remembers unresolved scalar fields as unknown, retains the source locations of resolved marker keys, and uses Spring's binder to determine the effective enum and boolean. Framework distinguishes Gemini API-key and Vertex modes. Vertex requires project and location, with no Framework requirement for explicit credentials. See Framework `src/main/java/ai/loomspan/internal/skill/ExecutionConfigurationParser.java:94–105,136–152,249–281` and `src/main/java/ai/loomspan/autoconfigure/LoomspanProperties.java:232–253`.
- The default auto-configuration creates the manager with a fixed runtime factory. The factory constructs a `NamedAiConnectionRegistry` over all typed connections, which eagerly runs `SpringAiProviderIntegration.create` for each one. Its Gemini builder uses an API key when present, otherwise it enables Vertex mode and supplies explicit credentials only when the JSON string has text. The explicit string is parsed by `ServiceAccountCredentials.fromStream` before the Google client is built. With an omitted declaration the client builder has no explicit credentials and can use engine ADC. See Framework `src/main/java/ai/loomspan/autoconfigure/LoomspanAutoConfiguration.java:154–174`, `src/main/java/ai/loomspan/internal/autoconfigure/NamedAiConnectionRegistry.java:19–49` and `src/main/java/ai/loomspan/internal/springai/SpringAiProviderIntegration.java:151–162,456–466`.
- The supported reloader has no pre-client authorization callback or effective-configuration accessor. The `SkillGenerationManager` constructor takes an internal `BiFunction` runtime factory, but the auto-configuration instantiates it directly and provides no host SPI. Framework’s `AGENTS.md` defines its supported API as the public top-level `ai.loomspan.api` types plus two specific v1 extension types. `internal` and `autoconfigure` types carry no compatibility promise.
- A separately callable internal entry point, `ExecutionConfigurationParser.parse(namespace, configuration, Map<String,String>)`, stages the execution-only authored document and binds its effective connections without creating a runtime. This calls the same staged and effective parser as the generation manager. Its `PreparedCandidateVariables` sees zero skill documents, so a map containing a skill-only key is considered to contain an unused entry by this execution-only method. `AuthoredYamlVariables.execution(yaml, source).discover()` returns exact names for the execution document's scan alone. `Document.staged()` explicitly marks unresolved keys and values, which are distinguishable from literal policy violations. See Framework `src/main/java/ai/loomspan/internal/skill/ExecutionConfigurationParser.java:33–55,161`, `src/main/java/ai/loomspan/internal/skill/PreparedCandidateVariables.java:23–39,62–79`, and `src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:54–59,186–207,226–245`. The signatures of these helpers were verified against the actual 11/23 cached starter.

## Affected Areas

| Area | Current behavior and evidence |
| --- | --- |
| Publication transport | No Cloud controller at baseline. The API defines multipart ingestion and distinct completed validation and publication responses. PR 1.1 provides the shared adapter. |
| Administrative settings | The engine's trace directory belongs to startup. Framework's publishable trace-section allowlist accepts only `persistence`, and its published session belongs to the generation. |
| Tenant/provider lifecycle | Framework performs a candidate-only exact map substitution and uses its typed runtime factory during prepare. The original configuration connection policy lacks an explicit service-account requirement. |
| Host extension boundary | At baseline the Cloud wrapper supplies no domain-specific executor beans and no tenant-controlled extension installation. Framework checks declarations against the boot-time fixed capabilities and executor registrations, with cross-namespace reference visibility rules. |
| Observation dependency for PR 1.3 | The checkout has per-namespace trace policy at Framework commit `2ca9caf`. The installed starter predates it and has `namespaceCatalogs()` with no `NamespaceSnapshot` type. The sibling build's `target` JAR currently has the same SHA-256 as the installed cache. |

## Existing Tests and Fixtures

- The Cloud baseline packages offline as documented in `ai/thoughts/research/pr-1.1-adapt-publication-zip.md`. It had no controller tests at the starting commit. This research performed only reads and local bytecode inspection so it did not collide with PR 1.1's Maven output.
- Framework `src/test/java/ai/loomspan/internal/skill/SkillReloaderTest.java:21–155` checks reference-free and explicit-map preparation, source labels, version metadata, retained snapshots, and target-only replacement. Its cases at lines 240–321 and 353–404 cover competing publication, shutdown, isolated credentials, and failed runtime construction.
- Framework `src/test/java/ai/loomspan/internal/skill/ExecutionConfigurationParserTest.java:53–63,140–177,359–368` checks variable Vertex mode, literal and variable credentials, forbidden reference/URI fields, and local-only validation evidence. Framework `src/test/java/ai/loomspan/internal/serialization/VariableMarkerFixtureTest.java:18` reads the shared corpus in `loomspan-variable-fixtures/`, including the worked six occurrences, aliases, markers as keys, and collision cases. The Framework worked-path test at `src/test/java/ai/loomspan/integration/ExecutorGenerationExecutionIntegrationTest.java:25` checks that the provider and real executor receive their expected values via a loopback stub.
- Framework `src/test/java/ai/loomspan/integration/PublicSkillReloadIntegrationTest.java:302–340,685–710,787–808` checks explicit credentials with host values present, prior work retaining its captured generation, and new work seeing the replacement.

## Dependencies and Operational Constraints

- Maven 3.9.11 and JDK 21.0.2 are available locally. The starter snapshot is present in the Maven cache. `C:/hamdev/jdk25/bin/javap.exe` can inspect its JDK 21 bytecode independently of the build toolchain. `javap` confirmed the supported reloader signatures and the internal execution parser and authored document signatures listed above. The installed and adjacent `target` JARs have SHA-256 `41BA16E2951A17E72895FF6C57C3B8F0269FDC793643A51E86BCFFAE6901C280`.
- The publish path can construct provider SDK objects with an explicit synthetic credential fixture, and existing Framework tests use loopback provider HTTP doubles when actual invocations are needed. Nothing in staged validation requires contacting a provider. An engine that uses ADC may otherwise perform host-specific credential lookup during provider SDK construction.
- Later milestone transport authentication, K3s observation, Go recovery, and end-to-end invocation are separate tickets. The cache's observation gap affects those checks after PR 1.2.

## Historical Context

Cloud added the shared Go-to-Java API in `1b58a16`, updated diagnostic and recovery design in `fa5b2db`, and added the five dependent PR 1 tickets in `bc21701`. Framework's previous `8515d16` changed its execution parser and candidate variable code as part of the Sidecar handoff. Its subsequent `2ca9caf` implements the observation format's per-namespace trace state.

## Open Questions

- How does the Cloud host establish the pre-client check while accounting for the dependency's unsupported internal signatures and preserving Framework's precise effective binding behavior?
- How does the composed configuration retain authored complex keys, aliases, and literal values while injecting host settings and reporting configuration source paths?
- Which malformed authored configurations block Framework's independent dependency checks, and which policy failures can be added alongside them without changing the candidate Framework sees?

## Step Report: 1_research_codebase

STATUS: complete
ARTIFACTS:
  - ai/thoughts/research/pr-1.2-validate-and-publish-engine-generations.md
SUMMARY: Traced Cloud's wire contract and the cached Framework reloader, variable, credential construction, and generation lifecycle boundaries. Verified that the cached starter exposes eager typed connection binding without clients and predates the per-namespace observation fields needed by PR 1.3.
DECISIONS:
  - none
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Run implementation and testing planning after the PR 1.1 adapter handoff is ready.
