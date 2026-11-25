# Restored skill invocation implementation plan

## Overview

- Ticket `ai/thoughts/tickets/pr-1.5-invoke-a-restored-skill.md`.
- Research `ai/thoughts/research/pr-1.5-invoke-a-restored-skill.md`.
- Outcome. The first caller invocation after replacement returns the exact string from a real skill installed from the durable publication.

## Current State

Cloud's Java host starts embedded Framework from `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java`. PR 1.2 supplies Java publication and PR 1.3 supplies mTLS, the pod watch and Console projection. PR 1.4 plans the Go `internal/publication.Eligible` handoff and durable Postgres state. Implementation must use the signatures in the cumulative checkout. The PR 1.3 handoff exposes `Observer.Current()`, `Observer.Changes()`, and `Observer.Refresh(ctx)`, with `Projection.Confirms(namespace, version)` and `Transport.ForPod(pod)` for a direct bound HTTPS client. The Framework starter has the synchronous `SkillTemplate.invoke(namespace, localName, Map, observer)` overload and captures reachable generation ownership in `SkillGenerationManager.capture`. `DefaultSkillTemplate` uses Framework's configured Spring Security context strategy, and session completion supplies a `SkillExecutionView`.

The adjacent Framework `FrameworkExecutionLifecycle.admitRoot` and `LoomspanSessionRunner.executeAdmittedRoot` expose two known pre-session rejection sites through a wrapped `RejectedExecutionException`. The public starter API does not provide a typed pre-session signal. The isolated starter at Framework commit `2ca9caf4b3d0e84540fbc975f0c7509face16ed5` also provides the namespace trace policy required for deployed verification.

## Desired End State

1. An authenticated direct user or authorized calling backend can invoke an application using the common Go JSON path. Go enforces the complete body ceiling and resolves roles and diagnostics settings at call time. It sends the authorized namespace and exact local name over the observed placement's authenticated transport.
2. Java enters the caller's Spring Security context, calls Framework once, and serializes exact result and failure details. Framework owns permission evaluation, generation selection, admitted work, and trace retention.
3. The caller receives the original string result or available failure message and top-level input issues. The saved diagnostics setting controls the execution view, stack trace, and diagnosis fields. Confirmed admission rejection, temporary unavailability prior to dispatch, and loss after dispatch remain distinguishable.
4. The dedicated K3s run proves initial publication on the first authenticated snapshot, failed replacement preservation, durable newer publication before Console confirmation, recovery on a changed pod and JVM, and the first successful invocation after observed confirmation.

## Scope

### In scope

- Shared Go synchronous JSON invocation, application grants, default-off visibility, and authenticated internal dispatch and projection.
- Java JSON invoke endpoint, Framework principal, view handling, and pre-session rejection classification.
- Local real Framework cutover and response tests, Go request-limit and transport tests, and a deterministic disposable K3s milestone.

### Out of scope

- General REST, MCP, UI, Supabase sign-in flows, membership administration, and multipart transport wiring.
- Caller-requested cancellation, additional retry or durable execution mechanisms, and post-V1 availability.

## Active Project Guardrails

None recorded in `ai/thoughts/design-lens.md`.

## Impact and Risk Analysis

- The body can contain caller-controlled identity and view metadata. The Go service takes an authenticated identity at its trusted adapter boundary and resolves grants for exactly the requested application. Only authorized backend callers can assert an end-user ID and roles. Read the saved view switch for every call.
- A Go observation can change after routing, and an HTTP request can straddle activation. Recheck its routability at the dispatch boundary and give Framework only namespace and local name. Keep publication IDs and generation tokens out of the invoke wire request.
- A provider or executor can fail with an error that resembles an admission error after work has started. Reserve Java 503 for verified Framework pre-session origins and Go retry advice for the direct authenticated engine response. Disable replay and treat ambiguous response loss as unknown outcome.
- Caller-side timeouts must end the wait without interrupting already admitted Framework work. Keep Java invocation synchronous on its server thread and avoid any bridge from request interruption or cancellation to Framework cancellation.
- A refresh normally races with pod deletion after publication. Verify that boundary by holding Console snapshots in the test transport from before the newer dispatch, and likewise hold the replacement's post-restore snapshot before proving routing resumes. The test uses real Kubernetes, TLS, Go publication and PostgreSQL writes.

## Implementation Approach

Compose a Go invocation service with the existing publication placement and engine transport. Its JSON entry reads one bounded caller request and takes trusted route and authentication arguments separately. This is usable by the milestone and later adapters. Use a small database-backed reader for application grants and the saved diagnostics switch, and keep Go response shaping in the shared service.

Use an invocation-specific Java handler with strict JSON validation and a scoped authentication helper. Reuse the strict JSON reader from publication handling where it applies, while each endpoint family has its own media type, error shape and inputs. Invoke the ordinary Framework synchronous API from the HTTP handler on every accepted input shape. Keep the current snapshot-specific pre-session check narrowly encapsulated and covered with real Framework tests so a starter update that changes the signal fails verification.

## Phase 1 — Java invoke and Framework integration

### Changes

- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationController.java` — implement `POST /internal/v1/invoke`. Require JSON object input with textual namespace and exact local skill name, an object `input`, and a `caller` object with nonblank string `issuer`, `subject`, and a required string array `roles`, permitting an empty array. Reject duplicate keys, trailing content and ambiguous or coerced node types. Accept omitted `executionView` or exact `always`. Keep application invocation metadata outside `input`. Do not build an invoke name from namespace and local name or call Framework validation as a separate preflight. Preserve original metadata values after wire validation.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationService.java` — use Framework's configured `SecurityContextHolderStrategy` or the same default selection. Set a fresh context with a principal that retains the case-sensitive issuer and subject as a pair and uses an unambiguous name representation. Build authorities with one `ROLE_` prefix and restore the previous context in `finally`. Invoke the no-observer overload when the mode is omitted and the observer overload when it is `always`. Capture the full observer value and include `executionView` with explicit null if requested and unavailable. Preserve the returned result string without interpreting embedded JSON.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationController.java` and scoped MVC exception handling — use JSON for rejected request and invocation responses that reach the endpoint. Preserve `Throwable.getMessage()` and full `printStackTrace(PrintWriter)`, including causes and suppressed failures. Include the complete `SkillFailureDiagnosis` from the top-level Framework exception and the complete issue array for a top-level `SkillInputValidationException`. Preserve the captured view on exceptions if requested. Map invalid request/input to 400, denied skill access to 403, and other execution exceptions to a non-503 failure. Keep existing mTLS and publication response boundaries.
- [x] `execution-engine/src/main/java/ai/loomspan/cloud/engine/InvocationAdmissionRejection.java` — classify the current starter's pre-session `SkillException` by its exact direct `RejectedExecutionException` origin and known rejection message. Match the first frame in `FrameworkExecutionLifecycle.admitRoot` for closed admission or `LoomspanSessionRunner.executeAdmittedRoot` for an unclaimed root before session creation. Keep the exact matching rule local to this lightweight classifier and cover it with the starter deployed for the milestone. A provider 503, general rejected execution exception, post-start shutdown cutoff or class/message appearing deeper in the chain is an execution failure. The controller's only 503 path uses confirmed admission rejection.
- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/InvocationControllerTest.java` and `InvocationCutoverTest.java` — cover strict wire handling and Spring context restoration; install fixture skills and exercise exact strings, real errors, view fallback, caller disconnection, real closed admission, and live cutover with a captured child.

### Automated verification

- [x] Run JDK 21 Maven focused tests with the isolated starter. Exact invocation and dependencies appear in the testing plan.
- [x] Run `mvn verify` in the Java module against that starter and verify the executable Boot JAR.

### Optional developer checks

None.

## Phase 2 — Shared Go authorization, invocation and response projection

### Changes

- [x] `management-backend/internal/invocation/service.go` — define one `InvokeJSON` entry taking context, authenticated actor, application ID, exact local skill name and request body reader. Enforce the inclusive 20,971,520-byte entire body limit by reading at most the limit plus one byte before parsing or dispatch. Bound by actual readable bytes, including metadata and whitespace, with document and object checks. Put the future multipart ceiling of 25 file parts beside the byte limit. Parse caller `input` and the backend end-user assertion at the shared boundary; keep trusted issuer and identity outside the submitted body. Ignore caller requests for a diagnostic mode. Preserve original JSON numbers and nested values through transport.
- [x] `management-backend/internal/invocation/postgres.go` and `management-backend/migrations/002_invocation.sql` — build on the PR 1.4 internal application schema and DB handle. Add default-false diagnostics state on the application and narrowly scoped invocation-grant data for seeded authenticated subjects with application roles and for owner-authorized backend IDs. A successful direct authorization resolves the configured Cloud issuer, authenticated subject and assigned roles in the target application. A backend authorization uses its authenticated canonical identity as issuer and honors its nonblank asserted end-user ID and roles only for an explicitly authorized target. Reserve the configured Cloud issuer for direct users. Reject unassigned callers and permit explicit empty role arrays. Read the permission and switch in one application-scoped query or transaction. Leave administration and authentication entry points to their separate work.
- [x] `management-backend/internal/invocation/service.go` and `management-backend/internal/engine/` — select the PR 1.4 eligible placement for the authorized app, revalidate its watched pod and JVM association before dispatch using the existing engine entry point, and create one JSON POST containing only `namespace`, `skill`, `input`, `caller` and optional `executionView: "always"`. Use the PR 1.3 direct Pod IP TLS path with verified engine service identity, no redirects and no discovered proxy. Retain authentication provenance at the transport boundary. Disable replay of the request body and do not set automatic retry or idempotency headers. Cancel only the Go wait with its caller context.
- [x] `management-backend/internal/invocation/response.go` — handle pre-dispatch unavailability, confirmed authenticated engine rejection, confirmed ordinary results, and unknown outcome after dispatch. A direct verified engine 503 confirms pre-execution rejection using status alone. An unauthenticated response, including an intermediary 503, conveys no outcome. Preserve the original result or failure message and top-level input issues on a confirmed response. Off omits all three diagnostics fields. On returns the whole available view on either outcome, explicit null for unavailable view, and available debug/diagnosis on failure. Preserve nested framework JSON using `json.RawMessage` or equivalent lossless projection.
- [x] `internal/invocation.NewService` and `deploy/k3s/README.md` — compose the production grant reader, placement resolver, transport, and configured Cloud issuer through the shared service used by the milestone. Document the migration and the contract for authenticated transport adapters. The management daemon continues running publication reconciliation and adapters can use the shared entry point as they are wired.
- [x] `management-backend/internal/invocation/service_test.go` and the PostgreSQL assertions in `k3s_test.go` — cover body limit, grants and role isolation, per-call switches and complete results, authenticated 503 provenance, unconfirmed transport paths and cancellation.

### Automated verification

- [x] `go test ./internal/invocation` and `go test ./...` from `management-backend/`.
- [x] `go test -race ./...` on a supported Go/C toolchain and `go build ./...` from `management-backend/`.
- [x] Run the real Postgres grant/migration assertions as part of the opt-in milestone.

### Optional developer checks

None.

## Phase 3 — Disposable K3s milestone

### Changes

- [x] `execution-engine/src/test/java/ai/loomspan/cloud/engine/fixture/MilestoneExecutorConfiguration.java` — register `@SkillExecutor(namespace = "milestone-host", name = "echo-release")` as a test-only host bean. Obtain the release label from `invocation.skillConfig()` and text from `invocation.input()`. Return their slash-separated result and throw the known error after starting execution for text `fail`. Publish the reference-only host generation PUBLIC with an empty document set and `loomspan: {}` using the supported reloader. Initialize host visibility after Framework generation setup and before the test listener accepts the first snapshot, for example with a test `SmartLifecycle` preceding web-server startup. This host namespace has no Cloud publication version. Keep `milestone` absent on both JVMs until Go publishes or restores it.
- [x] `deploy/k3s/milestone/execution-engine.Dockerfile` — launch the verified production Boot JAR with Spring Boot `PropertiesLauncher` and the fixture configuration as an extra Spring source. Build from a staging directory containing only the JAR and named fixture class files. Run the normal K3s profile, certificates, Console key and observable JVM identity. Use the same image for both JVMs. Make no test route available on the engine.
- [x] `management-backend/internal/invocation/k3s_test.go` — add the `k3s_integration` probe using actual Kubernetes watch, observation, verified per-pod transport, publication and recovery service and Postgres. Seed one authorized application and stages on an actual test volume with the two reference-free ZIPs and an install-time invalid ZIP. Use `loomspan/execution.yaml` with `loomspan: {}` and `loomspan/skills/echo-release.yaml` with required string `text`, qualified executor reference, and a literal `first` or `restored` release label. Select trace persistence `NEVER`. Invoke the shared Go entry as a seeded authorized actor. Test original `first/ping` in a separate Java fixture invocation so the recovery scenario's first dispatch uses the restored version.
- [x] In the K3s probe, wrap the real engine HTTPS client's `RoundTripper` as returned by `Transport.ForPod` through a test constructor or existing injection point. Bind barriers and counters to observed pod UID and request path, keeping mTLS and complete engine responses. Read a new closing broadcast channel from `Observer.Changes()` before each `Current()` predicate evaluation to avoid missing an event. Allow the first snapshot with no Cloud applications and the refresh of the acknowledged first publication. Prove invalid replacement rejected and the original tuple and durable pointer stay current. Arm an old-UID snapshot barrier before the newer publish, including in-flight old snapshot delivery. Capture the actual Java 204 and verify the current pointer with a new DB handle. Observe the requested immediate refresh waiting behind the barrier, an invocation rejected before dispatch, and an invoke count of zero. Print a machine-readable checkpoint with the old pod name, UID, JVM ID, and durable version.
- [x] After pod replacement, the same Go observer and reconciliation read the fresh initially empty snapshot and restore the durably acknowledged version. Starting at restore dispatch, withhold replacement snapshot delivery with a separate barrier even if another read is waiting. Assert no invoke dispatches while neither the new namespace/version tuple nor readiness is confirmed. Release the replacement barrier at the verified restoration checkpoint, then wait for a connected observation of the expected version on a ready replacement with different pod UID and JVM ID. Assert the first dispatched caller gets a JSON object with exactly `{"result":"restored/ping"}`. Keep evidence for old/replacement JVM IDs, recalled durable version and zero premature dispatches. With `NEVER` unchanged, run the four diagnostic visibility combinations and genuine input-validation null-view case only after the first-call assertion.
- [x] `deploy/k3s/milestone/run.ps1`, `deployment.yaml`, `job.yaml`, and `README.md` — document and automate the explicitly scoped scenario. Use the disposable single-node context and a Go probe Job with the existing management label, ServiceAccount, certificate and Console mounts. Use a disposable Postgres database and immutable ZIP fixture volume. Stop the ordinary management deployment so the probe is the single backend writer. From the host, wait for the Job's durable checkpoint and verify that its pod name and UID identify the sole engine. Scale to zero, wait for that pod to exit, then scale to one using the same image. This avoids the pod overlap possible with a direct deletion from a live ReplicaSet. Bound the wait for Job completion. Keep database and credential creation local, preserve redacted evidence, and keep test barriers and pod-replacement permissions outside the production service.

### Automated verification

- [x] Build the ordinary Java executable JAR and Go program and compile the tagged Linux probe.
- [x] Run `deploy/k3s/milestone/run.ps1` with the explicit disposable kubeconfig and context. The Job exits successfully and prints both JVM IDs, the durable restored version, no premature invoke dispatch, and the exact restored result. The separate original-release test passes.

### Optional developer checks

None. The cumulative K3s scenario is an automated milestone gate.

## Test Strategy

Use the smallest boundary that covers each behavior. Real Java HTTP tests exercise strict parse, Framework execution and generation ownership. Go tests exercise the complete body limit, routing and diagnostic policy on the transport boundary. Use loopback-only planner responses for the cutover graph. The scoped K3s probe verifies actual deployed TLS, pod identity, durable SQL, observation gating and the exact caller response, with no provider calls for the recovery skill.

## Acceptance-Criteria Traceability

| Acceptance criterion | Planned code evidence | Planned test evidence |
| --- | --- | --- |
| Authorized caller and exact response, invalid identity and unauthorized denied, inclusive full JSON limit | Go `InvokeJSON` and grant reader, Java caller context and direct Framework invocation | `TestInvokeAuthorization`, `TestInvokeJSONBodyLimit`, `InvocationControllerTest`, `TestK3sRestoredSkill` |
| Switch off/on for real success and failure with `NEVER`, null view fallback, preserved issues and diagnostics | Go response projection, Java observer selection and failure envelope | `TestInvokeDiagnosticProjection`, `InvocationControllerTest`, live probe visibility matrix |
| No eligible engine, lost response, intermediary 503, real pre-execution 503 and started provider failure | PR 1.4 placement, Go authenticated transport and availability classification, Java origin classifier | `TestInvokeOutcome`, `InvocationControllerTest` |
| First authenticated publish, invalid replacement, later durable acknowledgment without observation, new-JVM restoration and sole result | Test-only executor, probe barriers, real PostgreSQL and per-JVM routing | `TestK3sRestoredSkill`, fixture `originalPackReturnsFirstPing` |
| Already-admitted old root and reachable child, in-transit and newly accepted invocations use new generation | Java delegates ordinary invocation and captures nothing in Go | `InvocationCutoverTest` |
| Executable Java JAR, cumulative Go build and K3s pass | Standard module build and test deployment | `mvn verify`, `go build ./...`, scripted disposable K3s e2e |

## Risks and Rollback/Recovery

The Framework starter's pre-session rejection origin currently depends on snapshot internals. A compatible update must preserve verified proof for HTTP 503 or replace the classifier with a supported typed signal. Unknown transport outcomes remain non-retryable automatically. For a failed milestone, retain the checkpoint and probe logs, identify whether acknowledgment, recovery, observation or invocation was last successful, and restrict cleanup to the dedicated disposable environment. Keep production migrations additive and leave publication state and volumes intact.

## References

- `ai/thoughts/tickets/pr-1.5-invoke-a-restored-skill.md`
- `ai/thoughts/research/pr-1.5-invoke-a-restored-skill.md`
- `ai/thoughts/plans/pr-1.3-authenticate-and-observe-engine-pods.md`
- `ai/thoughts/plans/pr-1.4-record-and-restore-publications.md`
- `ai/thoughts/future/execution-engine-api.md`
- `ai/thoughts/future/go-backend-roadmap.md`
- `../loomspan-framework/src/main/java/ai/loomspan/internal/skillapi/DefaultSkillTemplate.java`
- `../loomspan-framework/src/main/java/ai/loomspan/internal/core/LoomspanSessionRunner.java`
