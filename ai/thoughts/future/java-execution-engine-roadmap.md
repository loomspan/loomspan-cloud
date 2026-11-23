# Java execution engine roadmap

The [execution-engine subproject](../../../execution-engine/pom.xml) embeds Loomspan Framework in a Spring Boot application. Start with one disposable engine in K3s. An engine can serve multiple applications, each installed in its own private Framework namespace with Cloud-vetted extensions. The [platform roadmap](roadmap.md#cloud-managed-engine-lifecycle) describes where Cloud stores publications and how Go decides which engine should serve an application.

## Build and run

The initial subproject is a standalone Maven build. Its Java 21 and Spring Boot versions follow the Framework starter it embeds. With JDK 21, Maven 3.9 or later, and the Framework starter snapshot available to Maven, run `mvn verify` from `execution-engine/` to produce the executable JAR under `target/`. Run `mvn spring-boot:run` there to start the application.

## Engine implementation

Implement the [shared execution engine API](execution-engine-api.md) in the Spring Boot wrapper.

### Startup and readiness

Go discovers engine pods through the K3s Kubernetes API. The Java readiness probe succeeds when the engine can receive a publication, including when it has zero applications installed. The pod watch and the Console connection provide the discovery and observation paths for V1, so engines need no separate event bus or etcd registration.

An application becomes eligible for routing on a ready, non-terminating pod once the Console proxy has confirmed the requested namespace and publication version on the connected JVM instance. Placement and routing read observed engine state from the proxy's in-memory projection.

### Planned engine shutdown

When K3s begins a planned pod shutdown, the engine stops accepting new invocations and becomes unready. It rejects any invocation arriving after admission closes as rejected before execution, including requests already in transit from Go. Previously admitted invocations have a grace period to finish and return their results. Set the grace period when configuring the K3s deployment.

### Administrative runtime settings

| Component | Responsibility |
| --- | --- |
| Cloud administration and Go backend | Own the destination application's administrative settings, defaults, allowed choices, and permissions. On import, reject reserved sections with their pack-relative paths. For validation or publication, compute one complete set of effective settings from Cloud policy and the application's saved selections. Send that set separately from the portable ZIP. Console shows owners which settings are active and which edits are pending. |
| Cloud Java wrapper in the execution engine | Accept the settings from the authenticated Go backend. Check the ZIP independently for reserved sections and reject missing or invalid administrative settings. Construct one in-memory Framework execution configuration from the portable sections and Cloud's supplied session and trace settings. Call `SkillReloader.validate` for a staged check, or the private, versioned, explicit-map `prepare` overload followed by `publish` for publication. Include the offending path in the message when rejecting a reserved pack field or an invalid administrative setting. |
| Embedded Loomspan Framework | Validate the composed candidate, resolve application variables during preparation, and capture the settings with the generation. Enforce mission timeout, depth, the six session quotas, and attachment size during execution. The entry generation's trace persistence policy determines which completed traces are retained. Cloud owns their storage lifetime. A failed preparation leaves the serving generation in place. |

Trace persistence is the owner-selectable administrative runtime setting in V1. New applications use `ONERROR`. Owners can select `NEVER`, `ONERROR`, or `ALWAYS`. Cloud defers [trace expiry and cleanup](roadmap.md#durable-execution-traces) beyond V1.

Cloud supplies the following session limits explicitly on each publish request. Console displays them read-only. These values are Cloud policy, so changing the embedded Framework version alone does not change them.

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

Staged validation checks the authored candidate without reading variable values. Apply the [publish request requirements](execution-engine-api.md#internal-api-requests) when resolving application variables. The explicit-map overload resolves names only from that map. Each generation captures its resolved strings as literal data. Keep supplied values out of logs. [Publication variable syntax](publication-package-format.md#application-variables)

### Tenant Vertex AI credentials

The Cloud ZIP adapter requires every tenant Vertex AI connection to supply nonblank service-account JSON through `gemini.credentials-json`, as a literal string or an application variable such as `{ $var: vertex.service-account }`. Framework parses explicit JSON as service-account credentials. If credentials are omitted, the Google client can use the engine's Application Default Credentials.

Staged validation reports credential-policy violations that are decidable from the authored YAML. Every publish request, including restart and recovery, must establish that the effective connection satisfies the policy before allowing Framework to build a client. Account for variable-selected keys, driver, Vertex mode, and credential values. If Cloud cannot establish that a candidate is safe, reject it with the connection path.

Exercise both JSON forms, a missing declaration with engine credentials available, a missing or invalid value, a variable-selected Vertex connection, a file or URI credential field, and an ordinary Gemini API-key connection. A failed replacement leaves the active generation serving.

### Publication and cutover

For an application restart, the wrapper prepares and publishes a fresh Framework generation even when the requested publication version is already installed. Console exposes the resulting generation ID for the [restart confirmation in Go](go-backend-roadmap.md#restarting-an-application).

The wrapper adapts the ZIP to Framework skill documents and execution configuration, supplies the value map, private visibility, and the assigned version to `SkillReloader.prepare`, then publishes the prepared update. Framework keeps the current generation serving during preparation. After publication, fresh invocations use the replacement and already admitted invocations retain their captured generations, including for reachable child skills. A failed update leaves the current publication available. Publishing zero YAML skill documents removes previously published YAML skills and leaves the namespace installed.

### Application diagnostics and retained traces

Set `loomspan.execution-trace.directory` in the engine’s host startup configuration to the shared trace mount. Framework writes `<root>/<canonical entry namespace>/<sessionId>.<traceId>.execution-trace.ndjson`. A call into another namespace stays in the initiating application’s trace. The deployment owns the directory, and Framework rejects it in published configuration. The engine also exposes its live Console stream. [Go trace discovery](go-backend-roadmap.md#look-up-retained-traces)

Framework writes active traces with the suffix `.execution-trace.ndjson.in-progress`. After closing the `TRACE_COMPLETED` append, it atomically renames retained files in the same directory to `.execution-trace.ndjson`. Immediately discarded traces are deleted under the active name. Files retained temporarily for Console’s completion grace period can also receive the completed name. Go scans only completed names and applies the [history indexing rule](go-backend-roadmap.md#look-up-retained-traces) to their terminal records. Verify same-directory atomic rename on the deployed shared volume and verify a replacement pod can read its predecessor’s completed traces.

Framework upgrades, including engine and proxy rollout order and access to retained traces across versions, are outside V1 scope.

## First single-engine milestone

Implement the Cloud ZIP adapter, validate and publish endpoints, K3s readiness probe, mTLS wiring, and Go integration in the Spring Boot host.

Start with an empty, ready engine and a pack using Framework defaults. Publish the pack with an empty value map and the application’s effective runtime settings. Before the first invocation, verify the success response triggers a proxy refresh and routing sees the requested namespace and version on the expected JVM. Verify a failed replacement leaves the previous version serving. Restart the engine and confirm the saved publication on the new JVM.

Test the [validate and publish responses](execution-engine-api.md#validate-and-publish-responses) with an invalid staged pack and a publication exception. Verify the proxy can fetch the instance snapshot with mTLS and the environment’s Console API key. In the invocation milestone, test the [execution-view modes](execution-engine-api.md#invocation-metadata) on success and skill error with tracing disabled.

## After V1: engine capacity

Defer Cloud-specific concurrency limits and overload behavior until we can profile the working engine with representative workloads. Use measured resource use and latency under concurrent invocations to decide whether admission limits or queuing are needed.
