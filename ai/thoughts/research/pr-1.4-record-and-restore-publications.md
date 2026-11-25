---
date: 2026-11-24
repository: loomspan-cloud
branch: main
commit: bc217016f62528855c70cbf505dd092657cabe18
ticket: ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md
tags: [go-backend, publication, postgres, recovery, variables]
---

# Acknowledged publication and engine recovery research

## Research Question

What does the checkout provide for Go-side staged validation, variable discovery, durable acknowledgment, and replacement-engine reconciliation? Where do Java, the Console observation, and the database each establish a result?

## Summary

The Cloud baseline contains the Java bootstrap and the V1 contracts. It has no Go backend, database schema, mounted pack reader, authorization implementation, or recovery tests. The Framework checkout supplies executable variable-discovery conformance fixtures and the namespace status fields. Earlier Cloud stages are implementing the Java candidate endpoints and planning the Go observer and transport that publication and recovery consume.

## Repository State

Inspected on November 24, 2026 at 15:19 Pacific. Cloud `main` and the cached `origin/main` were at `bc217016f62528855c70cbf505dd092657cabe18`. The index held PR 1.1's staged research artifact. The working tree also contained that ticket's adapter, Maven, resource, and test work and the earlier tickets' artifacts. This step treats those changes as owned by their respective tickets. The adjacent `loomspan-framework` checkout was inspected read-only at `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`. `ai/thoughts/design-lens.md:5` records no active project guardrails.

## Current Behavior and Data Flow

### Cloud baseline and adjacent changes

- The tracked Java host starts Spring Boot through `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6`. The `execution-engine/pom.xml` declares the Framework starter and Boot Web MVC. At this research snapshot there is no tracked or working-tree Go source, Go module, SQL migration, storage implementation, or backend authorization flow. There is also no pre-existing application or staging table to extend.
- The work in progress for PR 1.1 reads a servlet `Part` and returns `AdaptedPublication` with execution YAML and Framework `SkillDocument` records. The implementation sets each skill's diagnostic label from the adapter's pack-relative entry path. The original bytes remain the Go-to-Java wire representation in the API contract. Source `execution-engine/src/main/java/ai/loomspan/cloud/engine/PublicationZipAdapter.java:54`, `:86`, `:293` and `execution-engine/src/main/java/ai/loomspan/cloud/engine/AdaptedPublication.java:7`.
- PR 1.2 owns the Java validate and publish routes. Its implementation plan exists. The PR 1.3 implementation plan describes a Go module under `management-backend/`, with authenticated Pod IP transport in `internal/engine/transport.go` and the pod/Console projection in `internal/engine/observer.go`. It specifies `Current()`, `Changes()`, and `Refresh(ctx)` plus namespace lookup. These are planned APIs at the Cloud checkout inspected in this step. Source `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md:57`, `:73`, `:75`.

### Staged validation and owner publish contracts

- The Cloud contract treats staged revisions as immutable selections whose IDs distinguish successive staging events. Validation uses the selected ZIP and effective settings captured for that attempt. A completed engine report is a valid outcome even if the pack fails. It is also an opportunity to return Cloud's authored source/path findings, including missing or blank saved values. Missing values alone are publication blockers and an otherwise valid revision can pass validation. An incomplete attempt yields an unavailable result and a retry invitation without partial findings. Upload and promotion have their own revision and digest rules beyond PR 1.4. Source `ai/thoughts/future/go-backend-roadmap.md:25–37` and `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md:9`.
- The publish operation accepts a named staged revision for the authorized application. For a stale reviewed revision the API identifies that revision and current staging before dispatch. Explicit confirmation authorizes the reviewed revision and leaves newer staging intact. The already-current revision reports exactly “This revision is already published.” without creating a version or dispatching an install. Manual restart is a separate operation for reapplying settings to the existing publication. Source `ai/thoughts/future/go-backend-roadmap.md:41–53`, `:71–80`.
- Go sends multipart `pack` as `application/zip` with the original bytes and `metadata` as `application/json`. Validate supplies namespace and effective runtime settings. Publish and restore additionally supply the opaque `publicationVersion` and a non-null `variableValues` map of exactly the distinct names, with `{}` for a reference-free pack. Runtime settings include the session and all quotas, the 60-second timeout, maximum depth 32, attachment size `20971520B`, and trace persistence (`ONERROR` default, `NEVER` and `ALWAYS` choices). Source `ai/thoughts/future/execution-engine-api.md:24–59` and `ai/thoughts/future/java-execution-engine-roadmap.md:43–65`.
- Validate's completed wire result is `200` JSON containing `valid`, `issues`, and `skills`. A non-success text response means the check could not be completed. For publish, Java emits `204` only after Framework returns from activation. A received Java publication failure supplies verbatim `text/plain` for the authorized caller. A missing authenticated snapshot or known termination prevents dispatch. If dispatch occurred and the response is lost, the acknowledgment is unconfirmed. The ticket includes classifying the loss and excludes the follow-up confirmation window, retry revision, and resend procedure. Source `ai/thoughts/future/execution-engine-api.md:88–96`, `ai/thoughts/future/go-backend-roadmap.md:55–69`, and `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md:13`.

### Durable state and reconciliation contracts

- The roadmap specifies one Go backend deployment for V1 with Cloud API, Console proxy, placement controller, and router. The original ZIP is on a durable mounted filesystem. Supabase Postgres owns platform records and the publication history, immutable ZIP reference, status, and application current-publication pointer. The roadmap leaves the durable filesystem provider open. Observed Console status is an in-memory projection for the selected engine. Source `ai/thoughts/future/go-backend-roadmap.md:3–15` and `ai/thoughts/future/roadmap.md:45–49`, `:90–94`, `:112–116`.
- Java's `204`, the database acknowledgment, and a confirming snapshot represent three distinct boundaries. Go records the acknowledged version as current before reporting success and requests an immediate snapshot refresh. A pod loss between durable acknowledgment and observation leaves the new database record as recovery input. The saved settings sent with an install and later edits are distinct, and the namespace snapshot provides the active trace policy. Source `ai/thoughts/future/go-backend-roadmap.md:35–39`, `:51–53`, `:108–118` and `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md:14–15`.
- Placement compares a fresh Console observation against intended application publications on startup, reconnect, or JVM replacement. A matching namespace and publication version keeps its already loaded inputs. Missing or stale namespaces recover from the same original ZIP and publication version using the then-current application values and saved settings. Failed restores are reported per application and reconciliation continues. Unintended observed namespaces remain installed and are outside application routing. Route eligibility requires connection freshness, matching namespace/version, ready pod, and no known termination. First install requires an authenticated snapshot, including one with zero installed applications, and can precede pod readiness. Source `ai/thoughts/future/go-backend-roadmap.md:108–118`, `ai/thoughts/future/execution-engine-api.md:9–20`, `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md:13–15`.

### Framework variable discovery

- Framework parses a YAML node tree using SnakeYAML Engine with the Core schema. It selects each immediate value below `loomspan.connections` and `loomspan.executor-config` in authored order, excluding the selector keys. A skill's actual root must declare `executor` and its actual top-level `config` must be a mapping. Under the selected nodes it traverses keys before paired values, and sequences by zero-based index. Source `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:54–67`, `:132–155`, `:186–209` and `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/YamlValues.java:43–49`.
- A marker is a one-entry mapping with key `$var` tagged as a YAML string. Its value must be tagged as a string, contain nonblank text, and have no surrounding whitespace. Names are opaque and case-sensitive. The Framework whitespace set is the union of Java `Character.isWhitespace` and `Character.isSpaceChar`, enumerated in the shared corpus README. Multi-key mappings retain ordinary traversal. Dot or JSON-quoted bracket segments describe literal string keys. Complex keys and their paired values use `{key#i}` and `{value#i}` at authored entry indices. Each alias use contributes its own authored path. Framework rejects recursive aliases, and its expanded node budget includes repeated alias uses, keys, and values. Source `../loomspan-framework/loomspan-variable-fixtures/README.md:11–25` and `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:306–393`.
- Framework candidate validation uses unresolved structural values to check available definitions. Preparation discovers distinct names, checks extra explicit map keys, memoizes the lookup by name across configuration and skills, and materializes each final tree. Colliding effective YAML keys are a substitution failure with both authored locations. Discovery of the collision fixture succeeds. The Java materializer treats each supplied value as one terminal string. Source `../loomspan-framework/src/main/java/ai/loomspan/internal/skill/PreparedCandidateVariables.java:22–40`, `:63–80` and `../loomspan-framework/src/main/java/ai/loomspan/internal/serialization/AuthoredYamlVariables.java:226–279`.

## Key Components

| Boundary | Current evidence |
| --- | --- |
| Cloud authorization and selected inputs | `ai/thoughts/future/go-backend-roadmap.md:9`, `:31`, `:43–49` describe authorization and application-scoped selections. Go authorization, membership adapters, and callers do not yet exist in Cloud. |
| Authenticated dispatch | `ai/thoughts/future/execution-engine-api.md:14`, `:24–32` define mTLS and multipart. The planned PR 1.3 Pod IP transport is intended for Console and the publication API, with the Console API key confined to the Console client. |
| Observation and JVM identity | `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRestController.java:54–68` reports the runtime instance and a single namespace tuple with generation, publication version, and policy. `ai/thoughts/future/execution-engine-api.md:18–20` describes pairing that observation with the watched pod UID. |
| Persistence and lifecycle | `ai/thoughts/future/roadmap.md:90–94` identifies the immutable ZIP and database rows. `ai/thoughts/tickets/pr-1.4-record-and-restore-publications.md:14–15` couples per-application publish/restore serialization to the acknowledged current pointer and observed routing state. |

## Existing Tests and Fixtures

- `../loomspan-framework/loomspan-variable-fixtures/README.md:3–9` defines the portable case format, input order, and expected discovery and substitution outcomes. Each YAML document carries a source filename. Successful discovery fixtures list ordered occurrences and distinct names in order of first occurrence. Malformed cases give discovery failures. `complex-key`, `excluded`, `grammar`, `header-key`, `paths`, `invalid-*`, and `collision` cover those boundary cases. The README specifies alias semantics, and the alias examples are in the Java tests below.
- `../loomspan-framework/loomspan-variable-fixtures/worked/expected.json:2–14` has six occurrences across `loomspan/execution.yaml` and `loomspan/skills/read-record.yaml`, with four distinct names: `provider.key`, `app.region`, `records.url`, and `records.lookup-path`. The last name occurs only at `config.path` in the skill. `../loomspan-framework/loomspan-variable-fixtures/collision/expected.json:2–13` gives successful ordered discovery followed by `key-collision` during substitution.
- `../loomspan-framework/src/test/java/ai/loomspan/internal/serialization/VariableMarkerFixtureTest.java:21` enumerates the portable cases in Java. `../loomspan-framework/src/test/java/ai/loomspan/internal/serialization/AuthoredYamlVariablesTest.java:136`, `:151`, `:189`, `:198` cover missing/blank values, collisions, alias paths, and alias bounds. `../loomspan-framework/src/test/java/ai/loomspan/integration/ExecutorGenerationExecutionIntegrationTest.java:25` prepares and invokes the worked fixture through Framework consumers. These are existing Framework tests. This research step did not run them or a Go/Postgres/engine integration test.
- Cloud has no baseline Go tests, SQL migration fixtures, or disposable Postgres harness. The PR 1.1 Java tests in the working tree exercise the adapter and multipart ingress. The PR 1.3 plan describes forthcoming scoped K3s observation checks. PR 1.5 specifies the first cumulative real-skill scenario, including JVM replacement after durable acknowledgment and before confirmation.

## Dependencies and Operational Constraints

- The installed Go tool reports `go1.26.1 windows/amd64`. Node-aware YAML modules are present in the local Go cache (`gopkg.in/yaml.v3`, `go.yaml.in/yaml/v3`, and release-candidate `go.yaml.in/yaml/v4`). Cloud does not yet pin a YAML module or a PostgreSQL driver. The inspected cache listing did not show `pgx` or `lib/pq` modules.
- Parser classification matters because a marker value must be a YAML string. The locally cached `go.yaml.in/yaml/v3@v3.0.4/resolve.go:181–187` infers a timestamp tag from an unquoted date. SnakeYAML Engine 3.0.1's `CoreScalarResolver.java:60–69` registers no timestamp resolver. The authored mapping and scalar nodes, including explicit tags and styles, are observable inputs to cross-language discovery conformance.
- The adjacent Framework source and the Maven snapshot at identical coordinates can differ. PR 1.3 research verified the previously cached artifact lacked the namespace trace policy. The PR 1.3 plan records a rebuilt Framework snapshot in the isolated Maven repository under `execution-engine/target/pr1-framework/repository` for observing that field. See `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md:13`.
- The ticket assumes an already authorized application and staged pack. It includes reading their persisted state and excludes upload/review UI and manual restart. Local verification has prospective disposable K3s and PostgreSQL environments being prepared by the orchestrator. This research did not connect to an external Supabase instance or a configured production engine.

## Historical Context

`acd4d58` introduced the Go and Java roadmaps. `1b58a16` introduced the shared API contract. `fa5b2db` updated diagnostics, recovery, and publication behavior, and `bc21701` added the milestone tickets. PR 1.4's ticket scopes acknowledgment and recovery of acknowledged publications. PR 1.5 owns the cumulative invocation scaffold and executable end-to-end milestone. The Cloud checkout contains no earlier Go implementation to migrate.

## Open Questions

- Which concrete observer and transport signatures will PR 1.3 expose after implementation, including observation-change delivery, transport binding to the selected pod, and immediate refresh? The current evidence is a plan and the external wire contract.
- Which application authorization and state interfaces will seed the first owner and staged revision, and which PostgreSQL and mounted filesystem configuration will the first backend deployment consume? The roadmap specifies the providers and the ticket assumes authorized seeded inputs. The checkout has no established implementation or schema.
- What evidence at dispatch can distinguish a locally prevented send from a lost response, and how will a lost acknowledgment coexist with the current pointer before the post-ticket confirmation procedure is available? The contract specifies the three outcomes. The current Go checkout contains no implementation whose transport failures can be traced.
- What is the verification boundary for durable acknowledgment failure after a Java `204`, restore ordering under concurrent owner publish, and the ack-before-snapshot interruption? The ticket specifies durable success and per-application serialization and the cumulative milestone specifies the interruption. Cloud has yet to implement test hooks or persisted state for these transitions.

## Step Report: 1_research_codebase

STATUS: complete
ARTIFACTS:
  - ai/thoughts/research/pr-1.4-record-and-restore-publications.md
SUMMARY: Traced the staged validation, durable acknowledgment, observation, and restore contracts. Documented the absence of existing Go and SQL code, the planned PR 1.3 handoff, and the Framework variable conformance evidence.
DECISIONS:
  - none
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Plan implementation and verification using the PR 1.3 observer and transport handoff.
