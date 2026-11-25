---
date: 2026-11-24
repository: loomspan-cloud
branch: main
commit: bc217016f62528855c70cbf505dd092657cabe18
ticket: ai/thoughts/tickets/pr-1.5-invoke-a-restored-skill.md
tags: [invocation, framework, authorization, diagnostics, k3s, recovery]
---

# Restored skill invocation research

## Research Question

Where do Cloud and Framework establish invocation identity, placement eligibility, generation capture, execution admission, and result delivery? What executable evidence is available for the replacement-JVM milestone?

## Summary

The current Cloud host has Spring Boot and the publication ZIP adapter in progress. The Go and Java invocation paths, publication durability, and K3s workload manifests are absent from the inspected source. Framework already exposes a synchronous text API with a session-scoped execution view, dependency-generation capture, and Spring Security integration. Its current closure signal is a wrapped generic exception, and cross-namespace executor visibility is private at host startup.

## Repository State

Inspected November 24, 2026 at 15:21 Pacific. Cloud `main` and local `origin/main` were at `bc217016f62528855c70cbf505dd092657cabe18`. The PR 1.1 research artifact was staged, with PR 1.1 adapter, POM, application properties, test work, and preceding tickets' artifacts in the working tree. The adjacent Framework checkout at `2ca9caf4b3d0e84540fbc975f0c7509face16ed5` was read-only for this research; unrelated Markdown deletions existed there.

## Current Behavior and Data Flow

### Cloud contract and preceding handoffs

- `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6` starts Spring Boot. `execution-engine/pom.xml:16` selects Java 21 and the Framework starter. There are no working-tree Go sources or Cloud invoke controllers in the inspected checkout. `ai/thoughts/plans/pr-1.2-validate-and-publish-engine-generations.md:51–73` describes Java publication paths. `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md:57` and `:75` describe a Go direct Pod IP client that verifies the shared engine service identity, plus observation APIs `Current()`, `Changes()`, and `Refresh(ctx)`. These are planned interfaces. `ai/thoughts/research/pr-1.4-record-and-restore-publications.md:41–43` traces the durable pointer and reconciliation handoff.
- `ai/thoughts/future/go-backend-roadmap.md:92–104` defines the inclusive 20,971,520-byte caller body ceiling, the 25-file contract for future multipart and the per-call administrator switch. `ai/thoughts/future/execution-engine-api.md:61–67` and `:98–105` separate invocation arguments from the authenticated caller, and distinguish Cloud-issued direct identity from backend-issued end-user identity. `ai/thoughts/future/roadmap.md:45–47` and `:71–82` describe Supabase Auth and application membership, signed-in roles, and owner authorization of calling backends. The inspected Cloud checkout contains no membership, backend identity, or settings API implementation.
- `ai/thoughts/future/execution-engine-api.md:107–135` defines JSON result and failure envelopes, pre-execution `503` from the authenticated engine, unknown outcome after dispatch, exact result/message strings, and no caller cancellation of admitted work. Its example keeps a Framework result that contains JSON as a string. The Go caller's switch independently controls response exposure and does not change persisted trace behavior. PR 1.3's planned client refuses redirects and disables proxy discovery. Its direct mTLS path is intended to give provenance to a Java engine status independently of a hypothetical intermediary response.

### Framework identity, selection, and admission

- `../loomspan-framework/src/main/java/ai/loomspan/api/SkillTemplate.java:7–31` exposes separate exact namespace and local-name arguments, `Map<String, Object>` and object inputs, and optional `Consumer<SkillExecutionView>`. Both `DefaultSkillTemplate.invoke` paths capture via `SkillGenerationManager.capture` before input validation, authorization, or session execution. Object conversion happens after capture. `SkillTemplate.validate` captures temporary ownership and releases it without admitting work. Source `../loomspan-framework/src/main/java/ai/loomspan/internal/skillapi/DefaultSkillTemplate.java:80–177` and `:196–212`.
- `../loomspan-framework/src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:340–383` reads the active generations while holding one monitor, traverses declared reachable skills and executor namespaces, and acquires owner leases for that snapshot. `../loomspan-framework/src/main/java/ai/loomspan/internal/core/LoomspanSessionRunner.java:319–346` binds the captured snapshot and authentication to the new session. Its generation supplies trace policy and settings. An already captured invocation can therefore continue to consult its own child definitions after replacement. Cloud's version observation establishes a route to an engine; the Framework invocation API has no publication-version pin.
- `DefaultSkillTemplate.java:220–245` and `:287–294` read `SecurityContextHolderStrategy`, authorize the selected capability, and pass the captured authentication to the session. `../loomspan-framework/src/main/java/ai/loomspan/internal/security/SkillRoleEvaluator.java:18–31` maps authored role names to `ROLE_` authorities by default. Lines 35–53 combine an executor-backed skill's policy with the registered executor policy and throw Spring `AccessDeniedException` on denial. `LoomspanAutoConfiguration.java:294–317` passes an optional strategy bean to the template. `ScopedAuthentication.java:19–25` shows how Framework saves, installs and restores an isolated Spring context at its proxy boundary.
- `FrameworkExecutionLifecycle.java:60–69` throws `RejectedExecutionException` with the closure message from `admitRoot` while holding the lifecycle monitor. `LoomspanSessionRunner.java:264–278` releases the generation owner on this path before creating a session. A root losing its execution claim at cutoff has a distinct pre-session rejection at `:302–308`. `DefaultSkillTemplate.java:278–284` wraps generic runtime failures in `SkillException`. A downstream `RejectedExecutionException` and `FrameworkShutdownException` from a mission cutoff can instead occur after admission. The starter has no public checked or typed pre-admission shutdown exception on this overload. Framework's public `SkillInvocationHandoff` is a different API that separates admission and execution. Its implementation also wraps lifecycle rejection (`DefaultSkillInvocationHandoff.java:29–64`); PR 1.5 requires the ordinary invocation overload.

### Response and fixture evidence

- `../loomspan-framework/src/main/java/ai/loomspan/api/SkillInputValidationException.java:5–25` exposes issues on the exception; `SkillInputValidationIssue.java:3` defines path, code and message. `SkillException.java:20–29` exposes the optional terminal diagnosis, whose skill name, stage, original message and structured issue list are declared at `SkillFailureDiagnosis.java:7–16`. The template constructs the top-level validation exception during input checking before a session, including issues and diagnosis. For generic execution failures it may supply its own wrapper message and preserve the executor exception in the cause chain.
- `DefaultSkillTemplate.java:252–284` reads the terminal diagnosis and maps the view in the session completion callback. `SkillExecutionViewMapper.java:32–56` reads the finalized journal and maps every entry to an event. The public view is a session ID and event list. `LoomspanSessionRunner.java:337–359` retains the primary execution failure and suppresses completion failures. A mapping failure following a normal execution result is surfaced by Framework as an exception at the invocation boundary. `LoomspanSession.java:616–669` shows journal projection during finalization. The resulting in-memory journal is distinct from file-retention policy. A pre-session rejection reaches no completion callback.
- `../loomspan-framework/src/main/java/ai/loomspan/api/SkillExecutor.java:8–27` registers reference-only host behavior. `api/extensions/v1/SkillExecutorInvocation.java:52–59` exposes input and separate skill, owner and consumer configurations. `SkillGenerationManager.java:94–110` starts both the default generation and additional annotated host namespaces with PRIVATE visibility. `SkillDependencyValidator.java:21–24` and `:159–170` disallow foreign references into private namespaces. The preceding Framework integration fixtures explicitly prepare and publish an empty host generation with PUBLIC visibility before publishing a foreign executor leaf (`ExecutorGenerationExecutionIntegrationTest.java:80–85`, `:186–196`, `:389–397`). An installed host generation is separate from an installed Cloud application.
- `ai/thoughts/future/java-execution-engine-roadmap.md:99–107` and the PR 1.5 acceptance criteria distinguish the initial snapshot, initial publish acknowledgment and refresh, second durable acknowledgment, observation of the newer version, and first restored invocation. The PR 1.4 research at lines 41–43 identifies the saved pointer as recovery input after acknowledgment and before observation. No Cloud fixture or harness currently pauses observation at that boundary. The Framework snapshot's JVM ID and namespace version are observable independently of the invocation result.

## Existing Tests and Fixtures

- `../loomspan-framework/src/test/java/ai/loomspan/internal/skill/SkillReachableAdmissionTest.java:28`, `:60`, `:91`, `:230`, and `:296` cover exact root identity, same-local-name role isolation, fresh overloaded invocation after replacement, retained dependency snapshots, and captured authentication. `PublicSkillReloadIntegrationTest.java:87` exercises a delayed descendant and retry retaining old provider configuration while new work uses the replacement. These tests run through the Framework boundary, independent of Go or an HTTP request in transit.
- `../loomspan-framework/src/test/java/ai/loomspan/internal/skillapi/DefaultSkillTemplateTest.java:387–420` covers observer or mapper failures without masking an execution failure. `ExecutorApplicationIntegrationTest.java:40–44`, `:88–119`, and `:180–198` cover view callbacks and actual executor input and skill-config selection. `FrameworkExecutionLifecycleTest.java:236` covers invalidated pending admission at shutdown cutoff. Cloud has no current invoke-wire, diagnostics visibility, body-size, or cumulative recovery test. This research did not rerun Framework's tests.

## Dependencies and Operational Constraints

- The project build target is JDK 21. Maven 3.9.11 and Go 1.26.1 are available from earlier pipeline checks. The global cached and isolated Framework snapshot JARs both expose the needed synchronous `SkillTemplate.invoke` and diagnosis types. `javap` verified that per-namespace `tracePersistencePolicy()` exists in the isolated starter from Framework commit `2ca9caf` and is absent from the earlier global cache at the same coordinates. The isolated repository is under `execution-engine/target/pr1-framework/repository`.
- The orchestrator reported a ready disposable K3s node at `v1.34.13+k3s1`, cert-manager `v1.21.3` available, and a pulled PostgreSQL image. The workspace kubeconfig exists at `execution-engine/target/pr1-k3s/kubeconfig.yaml` for explicit context `k3d-loomspan-cloud-pr1-cg1`. This step did not connect to or mutate the cluster. The checkout still lacks the cumulative test workloads, authorized app fixture, and staged ZIP pair.

## Historical Context

`1b58a16` introduced the Go-to-Java API contract, `fa5b2db` revised diagnosis and recovery behavior, and `bc21701` supplied the ordered milestone tickets. Prior Cloud ticket notes allow intermediate commits awaiting wiring. The PR 1.5 ticket sets the cumulative executable Maven, Go and K3s verification requirement.

## Open Questions

- Which concrete Go authorization and routing APIs will PR 1.4 expose, and which durable field will hold the administrator switch? The current research and plans define the state transitions without implemented signatures.
- What invocation-boundary evidence can distinguish the starter's known pre-session closure from a same-class rejection after execution begins? The verified starter uses a generic exception in both possible contexts and does not expose a typed public admission rejection.
- How will the cumulative fixture preserve the separation between a durable acknowledgment and Console confirmation while using an actual watched engine pod? No existing Cloud fixture provides a test checkpoint between those events.

## Step Report: 1_research_codebase

STATUS: complete
ARTIFACTS:
  - ai/thoughts/research/pr-1.5-invoke-a-restored-skill.md
SUMMARY: Traced Framework generation capture, caller security, execution admission, structured diagnostics, and host executor visibility. Verified invocation API signatures in both local starter artifacts and documented the existing tests and disposable K3s prerequisites.
DECISIONS:
  - none
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Plan invocation and verification against the implemented publication and observation handoffs.
