# Restored skill invocation testing plan

## Change Summary

Verify the synchronous JSON invocation path through Go and real Framework, diagnostic field visibility, pre-execution rejection, and restoration before first caller dispatch after losing the original engine JVM. This plan uses the implementation, ticket, and research in `ai/thoughts/plans/pr-1.5-invoke-a-restored-skill.md`, `ai/thoughts/tickets/pr-1.5-invoke-a-restored-skill.md`, and `ai/thoughts/research/pr-1.5-invoke-a-restored-skill.md`.

## Impacted Areas and Risks

| Area | Risk | Planned evidence |
| --- | --- | --- |
| Go authorization and parsing | Users override roles or diagnostics, or a backend asserts a caller for another app. A large actual body gets dispatched. | Trusted actor and Postgres grant tests, counted-byte request boundary, captured wire request |
| Java invocation and result | Coercion accepts invalid identity, Spring context leaks, or a caller loses the original result or error fields. | Real loopback HTTP, annotated executor, strict request and exact response assertions |
| Admission and transport | Late failure is marked safe to retry, or caller cancellation stops a skill already running. | Real Framework shutdown-admission test and downstream 503, Go mTLS provenance and loss tests, blocked executor completion |
| Generation cutover | A request in transit pins the old generation, or an admitted parent loses its reachable child. | Loopback planner and HTTP request barriers around real Framework publication |
| Durability and readiness | Console observes the newer version before pod loss, or routing resumes before exact replacement confirmation. | Tagged Kubernetes probe with real SQL and transport, two snapshot barriers, and host-side pod replacement |

## Existing Coverage and Environment Constraints

Cloud's PR 1.1 checkout has ZIP and multipart Maven tests. PR 1.2 through PR 1.4 add the publication, mTLS observation, and durable Go paths required here. The plan uses their actual cumulative APIs during implementation. The adjacent Framework source already covers generation capture, reachability, application roles, and view mapping independently. This ticket requires evidence through Cloud's ordinary invocation API and, for recovery, through watch, transport, SQL, storage, Go and Java together.

JDK 21, Maven 3.9.11 and Go 1.26.1 are available. The isolated Maven repository under `execution-engine/target/pr1-framework/repository` has the starter rebuilt from `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`. Its namespace snapshot carries trace policy. Keep this path and the test kubeconfig available across builds. The disposable K3s node is available via explicit context `k3d-loomspan-cloud-pr1-cg1`, with cert-manager, an isolated transport CA, and PostgreSQL and Java base images. Ordinary suites use local resources and loopback connections. The tagged probe runs separately against the disposable cluster.

## Failing Test First

- Name. `originalPackReturnsFirstPing`.
- Type. Java localhost HTTP integration.
- Location. `execution-engine/src/test/java/ai/loomspan/cloud/engine/InvocationControllerTest.java`.
- Arrange, act, assert. Make the fixture's host executor publicly referenceable and install the reference-free `milestone` pack with literal release `first` through Java publish. With a well-formed caller, POST the namespace, exact skill and `{"text":"ping"}` and require a JSON `200` whose only member is `result` with string `first/ping`.
- Expected pre-implementation failure. The HTTP route is missing before the invocation handler is introduced. This test exercises the original pack separately from the cumulative recovery scenario.

## Tests to Add or Update

### 1. `InvocationControllerTest`

- Type. Embedded Spring Boot HTTP, Framework-backed execution, and narrowly scoped service unit tests.
- Location. `execution-engine/src/test/java/ai/loomspan/cloud/engine/InvocationControllerTest.java`.
- Proves. Ordinary POST passes namespace and exact local name separately, sets the issuer/subject principal and authorities, and restores the previous security context. The no-observer and observer overloads follow wire mode. Verify an exact result that contains JSON remains a string.
- Fixture. Use the annotated test host executor and an executor-backed app skill with `rbac_roles: [reviewer]`. Test a direct verified role including the already prefixed authority path, and check empty roles on a role-free skill. Capture the actual Framework authentication on a session or its observed principal. Exercise regular result, executor failure after entering the skill, and missing required `text` as a real pre-session rejection with validation issue and diagnosis.
- Assertions. Incorrect identity, absent/blank issuer or subject, missing/null or non-array roles, invalid role element, malformed or trailing JSON, duplicate caller keys and a role-denied invocation produce non-success and no executor activity. A disallowed namespace/skill name reaches the intended validation path with exact case intact. Results and errors with requested view preserve the whole event list, including representative details and session ID. An absent view when requested returns explicit JSON null. Compare error message with the original top-level Framework exception and debug output with class, stack, nested cause and a deliberately added suppressed exception. A spy or stub proves the no-observer call uses the no-observer overload without a production seam.
- Shutdown and wait. Use a final isolated Spring context for actual admission closure. Invoke Framework's lifecycle shutdown while keeping the test connector reachable, release the in-transit request, and assert Java 503 and zero executor entries. Use a separate started test executor throwing a provider-style HTTP 503 and confirm an ordinary non-503 invocation failure. Unit-test origin matching with cause chain and same-class downstream rejection negatives. For caller cancellation, let an executor signal it entered and wait on a bounded latch, cancel the HTTP client, then release and require the executor to finish under Framework, with no cancellation induced by the client.

### 2. `InvocationCutoverTest`

- Type. Java real HTTP and Framework publication with a loopback-only planner.
- Location. `execution-engine/src/test/java/ai/loomspan/cloud/engine/InvocationCutoverTest.java`.
- Proves. An admitted call retains its parent and reachable child generation. A request started by the caller before activation but completed toward Java afterward uses the replacement, along with a fresh call. No version token travels to Java.
- Fixture. Publish a parent planner skill with one required child binding and `output_from`, using the Framework's planner response shape evidenced by `../loomspan-framework/src/test/java/ai/loomspan/integration/ExecutorGenerationExecutionIntegrationTest.java`. Use the same private namespace for parent and child and an annotated leaf executor whose release label is in skill config. Run a local test response endpoint for the planner. Hold its first complete response after the old parent enters the provider so the child has yet to run. Hold a second Java invoke JSON request in transit with an incomplete body or gated loopback forwarder. Record that the test sender started that request before installing the new generation.
- Assertions. Update both graph definitions via real publish and await 204. Complete the in-transit request and issue a newly started call. Release the old planner response. Read distinct expected results and assert the old parent reached the old child while the latter two reached the replacement. Verify request bodies carry only invocation inputs and caller metadata. Bound waits and release all barriers in cleanup.

### 3. `TestInvokeAuthorization` and `TestInvokeJSONBodyLimit`

- Type. Go shared-service tests with a wire-capturing transport and a separate real Postgres grant test.
- Location. `management-backend/internal/invocation/service_test.go` and the PostgreSQL assertions in `k3s_test.go`.
- Proves. Go chooses `namespace` from the grant and intended placement, sends exact local skill, signed-in subject and application-assigned roles, and derives backend issuer solely from the authenticated backend. Username-like hints, roles and issuer in a direct caller's JSON cannot forge authorization.
- Inputs. Different direct memberships for two apps, allowed backend with asserted role and end-user ID, same backend denied for an unrelated app, incorrect or missing assertion, attempt to use the Cloud issuer as backend identity, explicit empty roles, missing membership, and caller view override while the setting is off. Verify grant and diagnostics default through the production SQL adapter using a disposable database.
- Limits. Build a valid complete request of exactly 20,971,520 bytes using JSON whitespace and/or a string value with metadata included. Exercise a reader without declared length. Count transport calls, assert exact-limit dispatch, and assert limit+1 is rejected before dispatch with bounded reading. Check the configured future multipart file count equals 25 in the shared limits. Reject a second document and missing or non-object `input`.

### 4. `TestInvokeDiagnosticProjection` and `TestInvokeOutcome`

- Type. Go service/transport tests using the PR 1.3 verified engine client and loopback TLS where provenance matters.
- Location. `management-backend/internal/invocation/service_test.go`.
- Proves. Read the switch on every call and observe off/on in actual request bytes. With an engine success or execution error, compare exact result or failure message, top-level issues, and complete diagnostic fields with large or nested JSON values. On includes explicit view null when unavailable. Off drops the diagnostics keys even if returned by the engine. Preserve embedded JSON strings as strings.
- Outcome cases. Count zero requests for no eligible placement, or if a stale association fails the dispatch recheck. Endpoint 503 after verified direct mTLS means confirmed rejection and safe retry advice, even when there is no decodable response body. Plain intermediary 503, post-dispatch EOF, timeout, cancellation, and a truncated success body produce unknown outcome. Preserve the count of one dispatch on each uncertain request. Show an unauthenticated response cannot gain provenance from body or headers that resemble the engine. Confirm the adapter does not replay a result-lost POST or follow redirects. Assert a started execution's non-503 failure is not marked as confirmed rejection.

### 5. `TestK3sRestoredSkill`

- Type. Opt-in K3s end-to-end test with the production Java Boot artifact, real Go pipeline, mTLS, TLS-authenticated Console snapshots, and disposable Postgres and mounted ZIP storage.
- Location. `management-backend/internal/invocation/k3s_test.go`, `execution-engine/src/test/java/ai/loomspan/cloud/engine/fixture/MilestoneExecutorConfiguration.java`, and `deploy/k3s/milestone/`.
- Fixture. One engine replica and unchanged test image across replacement. Maintain the test transport CA, dedicated database, real Go management ServiceAccount and deployed credential mounts. Apply both migrations. Seed the authenticated actor's app grant and publication actor authorization, stage two reference-free `milestone` packs and a third invalid replacement. The ZIPs contain `loomspan: {}` and a role-appropriate executor-backed `loomspan/skills/echo-release.yaml` with required string `text` and an operation label of `first` or `restored`. Publish with empty values and V1 settings, including `NEVER` trace persistence. Invalid replacement has valid staging structure and a bad Framework executor reference, so it actually reaches Java.
- Initial checkpoints. Read the first authenticated snapshot before any app is installed and issue the first publish at that event. Capture a real Java 204. Observe the scheduled snapshot refresh confirm the first version on the same JVM. Exercise the invalid replacement and verify the durable pointer and confirmed original tuple stay current. Keep the invoke count zero in this scenario up to the restored request. `originalPackReturnsFirstPing` proves the first pack's executable result separately.
- Durability checkpoint. Intercept Console requests and response delivery only in the tagged probe, at the real authenticated transport. Gate all old-UID instance snapshots before sending the newer publication, including reads already in flight. After a real Java 204, verify the committed pointer from a separate fresh Postgres handle and assert that the observer has never received the newer namespace version. Wait for the immediate refresh to attempt the gated request and assert no eligible route or invoke dispatch. Emit the old pod name, UID, JVM ID, and acknowledged version as the replacement checkpoint. The scoped host script verifies the sole engine pod's UID, scales the deployment to zero, waits for that pod to leave, and starts its replacement with the same image.
- Replacement checkpoint. Allow the new pod UID's initial authenticated empty snapshot and capture its UUID. As real restore dispatch begins, gate subsequent replacement instance snapshots. Observe successful same-version restore and zero invoke dispatches while the confirm step is held. Release the barrier and wait for connected, ready, non-terminating observation with the correct namespace and restored version. Invoke from Go with diagnostics off and assert the first dispatched user request has input `{"text":"ping"}` and yields exactly `{"result":"restored/ping"}`. Correlate the real Java response with the trusted replacement connection, different pod UID and JVM UUID, and acknowledged durable version. Print only scenario IDs and checkpoint assertions.
- Diagnostics. On the restored generation, leave policy at `NEVER`. After claiming the first restored result, run off/ok, off/`fail`, on/ok, and on/`fail` while toggling the saved setting through the test database. Verify internal JSON requests against outbound traffic captured by the test wrapper. Assert exact result or engine message, off key omission, and on complete view with failure debug and available diagnosis. Exercise a missing input field with the switch on to see explicit null view, intact issues and full diagnosis. Count executor-start evidence so the matrix includes genuine execution failure separately from pre-session rejection.

## Safe Verification Commands

From `execution-engine/` in PowerShell, with the isolated starter installed:

```powershell
$loomspanFrameworkHead = Join-Path (Get-Location) 'target/pr1-framework/repository'
$loomspanMavenTail = Join-Path $env:USERPROFILE '.m2/repository'
& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$loomspanFrameworkHead" "-Dmaven.repo.local.tail=$loomspanMavenTail" '-Dtest=InvocationControllerTest,InvocationCutoverTest' test
& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode "-Dmaven.repo.local=$loomspanFrameworkHead" "-Dmaven.repo.local.tail=$loomspanMavenTail" verify
$loomspanJar = [IO.Compression.ZipFile]::OpenRead((Resolve-Path 'target/loomspan-cloud-execution-engine-0.1.0-SNAPSHOT.jar').Path)
try {
    if (-not $loomspanJar.GetEntry('BOOT-INF/classes/ai/loomspan/cloud/engine/ExecutionEngineApplication.class') -or
        -not $loomspanJar.GetEntry('org/springframework/boot/loader/launch/JarLauncher.class')) {
        throw 'Executable Boot classes missing'
    }
    $loomspanManifest = [IO.StreamReader]::new($loomspanJar.GetEntry('META-INF/MANIFEST.MF').Open())
    try {
        if ($loomspanManifest.ReadToEnd() -notmatch 'Main-Class: org\.springframework\.boot\.loader\.launch\.JarLauncher') {
            throw 'Executable Boot launcher missing from manifest'
        }
    } finally { $loomspanManifest.Dispose() }
} finally { $loomspanJar.Dispose() }
```

From `management-backend/`:

```text
go test ./internal/invocation -run 'TestInvoke'
go test ./...
go test -race ./...
go build ./...
go test -tags=k3s_integration ./internal/invocation -run '^$'
```

Go race verification requires the supported C toolchain described by the Go build. If only that toolchain is unavailable, retain the actual failure in verification evidence and run the ordinary suite and focused concurrency tests. The Java `verify`, Go `build` and actual K3s probe remain required.

From repository root, with the Java `verify` output available:

```powershell
$loomspanKubeconfig = Join-Path (Get-Location) 'execution-engine/target/pr1-k3s/kubeconfig.yaml'
$loomspanContext = 'k3d-loomspan-cloud-pr1-cg1'
kubectl --kubeconfig $loomspanKubeconfig --context $loomspanContext get nodes
& ./deploy/k3s/milestone/run.ps1 -Kubeconfig $loomspanKubeconfig -Context $loomspanContext
```

The script compiles `go test -c -tags=k3s_integration -o ... ./internal/invocation` for the Linux node with `CGO_ENABLED=0`, stages only generated binaries and selected test class files for image builds, imports them into the named test cluster, applies the dedicated workload manifests, and runs the one recovery test. It checks the checkpoint before replacing the fixture pod and waits for a successful Job. A production cluster or different context is outside this command's scope.

## Verification Evidence

The isolated red run of `InvocationControllerTest#originalPackReturnsFirstPing`
returned HTTP 404 before the endpoint existed. Cumulative Maven verification
passed all 62 tests and produced an executable Boot JAR. The authenticated
loopback tests confirmed `first/ping`, the old parent and child across cutover,
the replacement for an in-transit request, and confirmed pre-session rejection.
The cumulative Go tests, race checks, build, and tagged probe compilation passed.

The cumulative K3s Job passed in 43.59 seconds in
`k3d-loomspan-cloud-pr1-cg1`. Its first Java acknowledgment refreshed
publication `e003a7e9923869c755472bf82bd3363e` on JVM
`39e07b22-3133-4948-8cd7-1d073c0c6cc8`. An invalid replacement left it
serving. Go reopened the newer acknowledged publication
`f9a6bc1c88850e6169e3aaf2deb09eb5` from PostgreSQL while the old Console
snapshot was held. JVM `0af41b7c-298d-4d33-ac2b-098393609950` restored that
version. The first eligible response was exactly `{"result":"restored/ping"}`
with zero premature invoke dispatches. The four diagnostics combinations and
the pre-session null view passed with trace retention `NEVER`.

## Optional Developer Checks

None.

## Exit Criteria

- [x] The first real Java invoke test fails at the missing route before implementation and passes after implementation.
- [x] Focused Java and Go test suites pass. Maven `verify` produces the executable JAR and the cumulative Go build succeeds.
- [x] Role and identity, exact body boundary, diagnostic visibility, null-view, failure fields, unknown outcome, no replay, and real admission and cancellation assertions pass.
- [x] Real cutover test prints or asserts distinct old and new child results with the early-started transport request resolving to the new generation.
- [x] The K3s Job proves both acknowledgment checkpoints and routing barriers, records old and new JVM identities and the restored version, and verifies its first eligible response contains only the exact `restored/ping` result.
- [x] The real deployed test passes all four visibility combinations with trace retention set to `NEVER`.
- [x] Routine tests use local bounded fixtures, and the pod replacement occurs only in the named disposable environment.

## Step Report: 2_create_plan + 3_testing_plan

STATUS: complete

ARTIFACTS:

- `ai/thoughts/plans/pr-1.5-invoke-a-restored-skill.md`
- `ai/thoughts/plans/pr-1.5-invoke-a-restored-skill-testing.md`

SUMMARY: Planned Java and Go invocation, response and authorization coverage, and the required disposable K3s recovery test. The test blocks Console confirmation across both durability checkpoints while retaining real mTLS, PostgreSQL, observation, and execution.

DECISIONS:

- Scope 503 to the verified Framework pre-session rejection sites and the verified direct engine response.
- Put the pod replacement trigger in the integration harness and use ordinary production API calls on the data path.
- Use the original release in a separate fixture test so the recovery scenario first dispatches to the restored version.

DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Run implementation against the cumulative PR 1.4 checkout.
