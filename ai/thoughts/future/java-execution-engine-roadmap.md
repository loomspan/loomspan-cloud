# Java execution engine roadmap

The [execution-engine subproject](../../../execution-engine/pom.xml) embeds Loomspan Framework in a Spring Boot application. Start with one disposable engine in K3s. An engine can serve multiple applications, each installed in its own private Framework namespace with Cloud-vetted extensions. The [platform roadmap](roadmap.md#cloud-managed-engine-lifecycle) describes where Cloud stores publications and how Go decides which engine should serve an application.

## Build and run

The initial subproject is a standalone Maven build. Its Java 21 and Spring Boot versions follow the Framework starter it embeds. With JDK 21, Maven 3.9 or later, and the Framework starter snapshot available to Maven, run `mvn verify` from `execution-engine/` to produce the executable JAR under `target/`. Run `mvn spring-boot:run` there to start the application.

## Engine implementation

Implement the [shared execution engine API](execution-engine-api.md) in the Spring Boot wrapper.

### Cloud ZIP adapter

Validate and publish share this adapter. Restart and restore use publish. The [package format](publication-package-format.md#archive-rules) defines the byte and entry limits.

- Enforce the whole-ZIP byte limit on the incoming multipart `pack` part as it arrives, before buffering the full part in memory.
- Count every entry, including explicit directories. Enforce the entry limit before allocating an index for the entire archive.
- Read in bounded chunks. Before retaining each chunk, check the cumulative bytes expanded by the reader against the expanded-size limit. Skip companion file contents when the reader supports it. Count any bytes it expands to skip them.

Check every entry’s path for ambiguity and require a single top-level directory matching the request’s `namespace`. Under `loomspan/`, enforce the [execution layout](publication-package-format.md#execution-inputs), including lowercase skill paths. Reject nonregular execution inputs and case-insensitive execution path collisions. Independently reject reserved sections in `loomspan/execution.yaml` and report their paths. Pass each skill’s pack-relative path to Framework as its `SkillDocument.sourceName`.

Framework validates the skill definitions, their references, and the composed execution configuration, including the executor configuration owner key.

### Startup and readiness

Go discovers engine pods through the K3s Kubernetes API. When the Console proxy reads its first authenticated instance snapshot from a newly discovered JVM, Go can immediately send that engine its first publication, even with zero Cloud applications installed. The pod watch and the Console connection provide the discovery and observation paths for V1, so engines need no separate event bus or etcd registration. Whether K3s needs a readiness probe for rollouts is a separate deployment decision.

An application becomes eligible for routing on a ready, non-terminating pod once the Console proxy has confirmed the requested namespace and publication version on the connected JVM instance. Placement and routing read observed engine state from the proxy's in-memory projection.

### Planned engine shutdown

V1 deliberately runs a single engine for testing and accepts downtime during engine restarts and replacements. While that engine is unavailable, Go returns a temporarily unavailable response to new invocations until an engine is eligible to serve the requested publication. This response is the expected V1 behavior. Defer work to maintain continuous availability, including overlapping engine pods and failover, to V2. [Go routing during shutdown](go-backend-roadmap.md#invocation-response-loss)

When K3s begins a planned pod shutdown, the engine stops accepting new invocations and becomes unready. It rejects any invocation arriving after admission closes as rejected before execution, including requests already in transit from Go.

Set `loomspan.shutdown.timeout=60s` for V1. When Framework closes admission, all previously admitted invocations share a single 60-second drain window. Each invocation keeps its original mission deadline. Shutdown can proceed as soon as all admitted invocations finish. Configure the K3s pod termination grace period to allow the full drain window plus time to deliver final responses and exit.

Once Go observes that an engine pod is terminating, it stops sending publish and validate requests to that pod. In single-engine V1, Go tells the caller that the engine is shutting down, the request was not sent, and they can retry when the engine is available.

For requests already dispatched, Go handles any engine response normally. If shutdown interrupts the response, Go tells a validation caller that the result is unavailable and validation can be retried. For a publish request, Go follows the [unconfirmed publication procedure](go-backend-roadmap.md#unconfirmed-publications) and tells the caller if it could not confirm installation during shutdown.

### Administrative runtime settings

| Component | Responsibility |
| --- | --- |
| Cloud administration and Go backend | Own the destination application's administrative settings, defaults, allowed choices, and permissions. For validation or publication, compute one complete set of effective settings from Cloud policy and the application's saved selections. Send that set separately from the portable ZIP. Console shows owners which settings are active and which edits are pending. |
| Cloud Java wrapper in the execution engine | Accept the settings from the authenticated Go backend. Reject missing or invalid administrative settings. Construct one in-memory Framework execution configuration from the portable sections and Cloud's supplied session and trace settings. Call `SkillReloader.validate` for a staged check, or the private, versioned, explicit-map `prepare` overload followed by `publish` for publication. Include the offending path in the message for an invalid administrative setting. |
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

Staged validation checks the authored candidate without reading variable values. Apply the [publish request requirements](execution-engine-api.md#internal-api-requests) when resolving application variables. The explicit-map overload resolves names only from that map. Each generation captures its resolved strings as literal data. [Publication variable syntax](publication-package-format.md#application-variables)

### Tenant Vertex AI credentials

The Cloud ZIP adapter requires every tenant Vertex AI connection to supply nonblank service-account JSON through `gemini.credentials-json`, as a literal string or an application variable such as `{ $var: vertex.service-account }`. Framework parses explicit JSON as service-account credentials. If credentials are omitted, the Google client can use the engine's Application Default Credentials.

Staged validation reports credential-policy violations that are decidable from the authored YAML. Every publish request, including restart and recovery, must establish that the effective connection satisfies the policy before allowing Framework to build a client. Account for variable-selected keys, driver, Vertex mode, and credential values. If Cloud cannot establish that a candidate is safe, reject it with the connection path.

Exercise both JSON forms, a missing declaration with engine credentials available, a missing or invalid value, a variable-selected Vertex connection, a file or URI credential field, and an ordinary Gemini API-key connection. A failed replacement leaves the active generation serving.

### Publication and cutover

For an application restart, the wrapper prepares and publishes a fresh Framework generation even when the requested publication version is already installed. Console exposes the resulting generation ID for the [restart confirmation in Go](go-backend-roadmap.md#restarting-an-application).

**Settled decision for invocation cutover.** Go routes to an eligible engine. Framework captures the active generation when it accepts an invocation and retains the captured generations for that invocation and its reachable child skills. An invocation already in transit at cutover uses the replacement if Framework first captures its generation after publication. This requires no additional cutover coordination in Go.

The wrapper adapts the ZIP to Framework skill documents and execution configuration, supplies the value map, private visibility, and the assigned version to `SkillReloader.prepare`, then publishes the prepared update. The current generation keeps serving during preparation and after a failed update. Publishing zero YAML skill documents removes previously published YAML skills and leaves the namespace installed.

### Application diagnostics and retained traces

Set `loomspan.execution-trace.directory` in the engine’s host startup configuration to the shared trace mount. Framework writes `<root>/<canonical entry namespace>/<sessionId>.<traceId>.execution-trace.ndjson`. A call into another namespace stays in the initiating application’s trace. The deployment owns the directory, and Framework rejects it in published configuration. The engine also exposes its live Console stream. [Go trace discovery](go-backend-roadmap.md#look-up-retained-traces)

Framework writes active traces with the suffix `.execution-trace.ndjson.in-progress`. After closing the `TRACE_COMPLETED` append, it atomically renames retained files in the same directory to `.execution-trace.ndjson`. Immediately discarded traces are deleted under the active name. Files retained temporarily for Console’s completion grace period can also receive the completed name. Go scans only completed names and applies the [history indexing rule](go-backend-roadmap.md#look-up-retained-traces) to their terminal records. Verify same-directory atomic rename on the deployed shared volume and verify a replacement pod can read its predecessor’s completed traces.

Framework upgrades, including engine and proxy rollout order and access to retained traces across versions, are outside V1 scope.

## First single-engine milestone

Implement the Cloud ZIP adapter, validate and publish endpoints, mTLS wiring, and Go integration in the Spring Boot host.

Start an engine with zero Cloud applications installed. Have the Go Console proxy read its first `GET /_loomspan/observability/v1/instance` snapshot using mTLS and the environment’s Console API key. Immediately send a pack using Framework defaults through the engine’s publish endpoint with an empty value map and the application’s effective runtime settings. Assert that the first publish request succeeds. Before the first invocation, verify the success response triggers a proxy refresh and routing sees the requested namespace and version on the JVM identified by the first snapshot. Verify a failed replacement leaves the previous version serving. Then publish a new version and terminate the pod after Go receives `204` but before Console observes the update. Verify Go restores that version on the replacement and sends it the first invocation only after Console confirms the namespace and version on the new JVM.

That first invocation must execute a real skill from the restored publication on the replacement JVM. Verify that the caller receives the skill's expected result through Go.

Test the [validate and publish responses](execution-engine-api.md#validate-and-publish-responses) with an invalid staged pack and a publication exception. In the invocation milestone, test the [execution-view modes](execution-engine-api.md#invocation-metadata) on success and skill error with tracing disabled.

## After V1: engine capacity

Defer Cloud-specific concurrency limits and overload behavior until we can profile the working engine with representative workloads. Use measured resource use and latency under concurrent invocations to decide whether admission limits or queuing are needed.
