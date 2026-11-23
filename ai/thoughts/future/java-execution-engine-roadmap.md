# Java execution engine roadmap

The [execution-engine subproject](../../../execution-engine/pom.xml) embeds Loomspan Framework in a Spring Boot application. Start with one disposable engine in K3s. An engine can serve multiple applications, each installed in its own private Framework namespace with Cloud-vetted extensions. The [platform roadmap](roadmap.md#cloud-managed-engine-lifecycle) describes where Cloud stores publications and how Go decides which engine should serve an application.

## Build and run

The initial subproject is a standalone Maven build. Its Java 21 and Spring Boot versions follow the Framework starter it embeds. With JDK 21, Maven 3.9 or later, and the Framework starter snapshot available to Maven, run `mvn verify` from `execution-engine/` to produce the executable JAR under `target/`. Run `mvn spring-boot:run` there to start the application.

## Integration contracts

| Integration | Caller and input | Receiver and observable outcome |
| --- | --- | --- |
| Validate a staged pack | Go sends the staged ZIP, target namespace, and destination application’s effective runtime settings at the owner or companion's request. | The wrapper uses the same ZIP adapter as installation and calls `SkillReloader.validate` with the skill documents, execution configuration, and private visibility. Framework checks the complete candidate, including skill references, without creating provider clients or activating a generation. Java returns Framework diagnostics, preserving source paths when available. |
| Install or restore a publication | The Go placement controller sends the original ZIP, the target namespace, the Cloud-assigned opaque `publicationVersion`, the required `variableValues` map, and the effective runtime settings to the engine's internal API. | The Java wrapper reads the complete skill pack and runtime configuration and publishes into the supplied namespace. Go can confirm the version through Console before routing work. Restoration reuses the saved ZIP and version with the target application’s current variables and effective runtime settings at restore time. |
| Observe an engine | The Go Console proxy connects to the engine and reads `GET /_loomspan/observability/v1/instance` at connection and reconnection, then consumes the engine stream. | The snapshot identifies the JVM instance and lists active namespaces with their Framework-local `generationId` and Cloud publication version, even before first invocation. A fresh snapshot resolves a lost installation acknowledgment. Go owns the projection used by placement and routing. |
| Invoke a skill | Go authorizes the caller for the application. The router selects a confirmed engine and sends the namespace, skill, and input. Caller identity and application roles travel separately from skill input. | The Java wrapper sets the Framework security context, invokes the namespace-qualified skill, and returns the skill result or reported skill error to Go, including the execution view when requested and available. |

Go and Java authenticate each other using mTLS on the internal API and Console connection. For Console requests, every engine also enforces Framework's existing API-key check. A Kubernetes Secret supplies the same Console API key to the Go proxy and every engine in that environment. Development, QA, and production each use their own key.

In K3s, the wrapper obtains the pod name, namespace, and UID through Downward API environment variables. It reports them alongside Framework's per-JVM instance ID so Go can identify a process restart within the same pod.

### Invocation metadata

V1 does not support caller-requested cancellation. Once an invocation starts, it continues under Framework's normal execution limits if the caller disconnects or times out while waiting for the response.

The caller can omit the execution view (default) or select `always` or `on_error` per invocation. Go forwards the mode to Java. With the default, Java calls `SkillTemplate.invoke` without an observer. For either opt-in mode, Java captures Framework's `SkillExecutionView` through the observer. With `always`, Java returns the available view on success or reported skill error. With `on_error`, Java includes it only if the invocation ends in a reported skill error. A recovered internal error followed by success counts as success.

The projection contains the session ID and execution events. Delivery is independent of trace retention. A pre-session rejection has no execution view. Framework can also omit the callback if session finalization or mapping fails; the response for unavailable metadata in that exceptional case remains open.

### Startup and readiness

Go discovers engine pods through the K3s Kubernetes API. The Java readiness probe succeeds when the engine can receive a publication, including when it has zero applications installed. The pod watch and the Console connection provide the discovery and observation paths for V1, so engines need no separate event bus or etcd registration.

An application becomes eligible for routing on a ready, non-terminating pod once the Console proxy has confirmed the requested namespace and publication version on the connected JVM instance. Placement and routing read observed engine state from the proxy's in-memory projection.

### Planned engine shutdown

When K3s begins a planned pod shutdown, the engine stops accepting new invocations and becomes unready. It rejects any invocation arriving after admission closes as rejected before execution, including requests already in transit from Go. Previously admitted invocations have a grace period to finish and return their results. Set the grace period when configuring the K3s deployment.

### Administrative runtime settings

| Component | Responsibility |
| --- | --- |
| Cloud administration and Go backend | Own the destination application's administrative settings, defaults, allowed choices, and permissions. On import, reject reserved sections with their pack-relative paths. For validation or publication, compute one complete set of effective settings from Cloud policy and the application's saved selections. Send that set separately from the portable ZIP. Console shows owners which settings are active and which edits are pending. |
| Cloud Java wrapper in the execution engine | Accept the settings from the authenticated Go backend. Check the ZIP independently for reserved sections and reject missing or invalid administrative settings. Construct one in-memory Framework execution configuration from the portable sections and Cloud's supplied session and trace settings. Call `SkillReloader.validate` for a staged check, or the private, versioned, explicit-map `prepare` overload followed by `publish` for installation. Return distinct machine-readable errors for a reserved pack field and an invalid administrative setting, with the offending path. |
| Embedded Loomspan Framework | Validate the composed candidate, resolve application variables during preparation, and capture the settings with the generation. Enforce mission timeout, depth, the six session quotas, and attachment size during execution. The entry generation's trace persistence policy determines which completed traces are retained. Cloud owns their storage lifetime. A failed preparation leaves the serving generation in place. |

Trace persistence is the owner-selectable administrative runtime setting in V1. New applications use `ONERROR`. Owners can select `NEVER`, `ONERROR`, or `ALWAYS`. Cloud defers [trace expiry and cleanup](roadmap.md#durable-execution-traces) beyond V1.

Cloud supplies the following session limits explicitly on each installation. Console displays them read-only. These values are Cloud policy, so changing the embedded Framework version alone does not change them.

| Session limit | V1 value |
| --- | ---: |
| Mission timeout | 60 seconds |
| Maximum call depth | 32 |
| Maximum size per attachment | 20 MiB |
| Skill invocations per session | 64 |
| Tool invocations per session | 128 |
| Linter retries per session | 32 |
| Model calls per session | 64 |
| Provider attempts per session, including retries | 192 |
| Usage units per session | 200,000 |

Size the Go response timeout and pod shutdown grace period independently. A caller timeout ends the wait for a result, and the pod grace period bounds planned shutdown.

### Application variables

Staged validation checks the authored candidate without reading variable values. For installation, Java requires a non-null `variableValues` map with exactly one nonblank value per distinct referenced name, including an empty map for a reference-free pack. The explicit-map overload resolves names only from that map. Each generation captures its resolved strings as literal data. Keep supplied values out of logs and diagnostics. [Publication variable syntax](publication-package-format.md#application-variables)

### Tenant Vertex AI credentials

The Cloud ZIP adapter requires every tenant Vertex AI connection to supply nonblank service-account JSON through `gemini.credentials-json`, as a literal string or an application variable such as `{ $var: vertex.service-account }`. Framework parses explicit JSON as service-account credentials. If credentials are omitted, the Google client can use the engine's Application Default Credentials.

Staged validation reports credential-policy violations that are decidable from the authored YAML. Every install, including restart and recovery, must establish that the effective connection satisfies the policy before allowing Framework to build a client. Account for variable-selected keys, driver, Vertex mode, and credential values. If Cloud cannot establish that a candidate is safe, reject it with the connection path. Keep credential contents out of diagnostics.

Exercise both JSON forms, a missing declaration with engine credentials available, a missing or invalid value, a variable-selected Vertex connection, a file or URI credential field, and an ordinary Gemini API-key connection. A failed replacement leaves the active generation serving.

### Publication and cutover

For an application restart, the wrapper prepares and publishes a fresh Framework generation even when the requested publication version is already installed. Console exposes the resulting generation ID for the [restart confirmation in Go](go-backend-roadmap.md#restarting-an-application).

The engine acknowledges installation after publication. Go handles snapshot refresh and lost acknowledgments through the [Console proxy](go-backend-roadmap.md#observe-live-engines).

The wrapper adapts the ZIP to Framework skill documents and execution configuration, supplies the value map, private visibility, and the assigned version to `SkillReloader.prepare`, then publishes the prepared update. Framework keeps the current generation serving during preparation. After publication, fresh invocations use the replacement and already admitted invocations retain their captured generations, including for reachable child skills. A failed update leaves the current publication available. Publishing zero YAML skill documents removes previously published YAML skills and leaves the namespace installed.

When validation or installation fails, the Java wrapper returns a machine-readable error code identifying the cause, together with technical diagnostics. It preserves the affected skill, file, or configuration path when Framework supplies it. Define the codes and response fields in the engine API contract so Go can act on the code independently of the diagnostic wording.

### Application diagnostics and retained traces

Set `loomspan.execution-trace.directory` in the engine’s host startup configuration to the shared trace mount. Framework writes `<root>/<canonical entry namespace>/<sessionId>.<traceId>.execution-trace.ndjson`. A call into another namespace stays in the initiating application’s trace. The deployment owns the directory, and Framework rejects it in published configuration. The engine also exposes its live Console stream. [Go trace discovery](go-backend-roadmap.md#look-up-retained-traces)

Framework writes active traces with the suffix `.execution-trace.ndjson.in-progress`. After closing the `TRACE_COMPLETED` append, it atomically renames retained files in the same directory to `.execution-trace.ndjson`. Immediately discarded traces are deleted under the active name. Files retained temporarily for Console’s completion grace period can also receive the completed name. Go scans only completed names and applies the [history indexing rule](go-backend-roadmap.md#look-up-retained-traces) to their terminal records. Verify same-directory atomic rename on the deployed shared volume and verify a replacement pod can read its predecessor’s completed traces.

## First single-engine milestone

Implement the Cloud ZIP adapter, validation and installation endpoints, K3s readiness probe, mTLS wiring, and Go integration in the Spring Boot host.

Start with an empty, ready engine and a pack using Framework defaults. Install with an empty value map and the application’s effective runtime settings. Before the first invocation, verify the success response triggers a proxy refresh and routing sees the requested namespace and version on the expected JVM. Verify a failed replacement leaves the previous version serving. Restart the engine and confirm the saved publication on the new JVM.

Specify the install wire format, error codes, and response fields. Verify the proxy can fetch the instance snapshot with mTLS and the environment’s Console API key. In the invocation milestone, test the [execution-view modes](#invocation-metadata) on success and skill error with tracing disabled.

## After V1: engine capacity

Defer Cloud-specific concurrency limits and overload behavior until we can profile the working engine with representative workloads. Use measured resource use and latency under concurrent invocations to decide whether admission limits or queuing are needed.
