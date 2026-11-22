# Java execution engine roadmap

The [execution-engine subproject](../../../execution-engine/pom.xml) embeds Loomspan Framework in a Spring Boot application. Start with one disposable engine in K3s. An engine can serve multiple applications, each installed in its own private Framework namespace with Cloud-vetted extensions. The [platform roadmap](roadmap.md#cloud-managed-engine-lifecycle) describes where Cloud stores publications and how Go decides which engine should serve an application.

## Build and run

The initial subproject is a standalone Maven build. Its Java 21 and Spring Boot versions follow the Framework starter it embeds. With JDK 21, Maven 3.9 or later, and the Framework starter snapshot available to Maven, run `mvn verify` from `execution-engine/` to produce the executable JAR under `target/`. Run `mvn spring-boot:run` there to start the application.

## Integration contracts

| Integration | Caller and input | Receiver and observable outcome |
| --- | --- | --- |
| Validate a staged pack | Go sends the staged ZIP and target namespace at the owner or companion's request. | The wrapper uses the same ZIP adapter as installation and calls `SkillReloader.validate` with the skill documents, execution configuration, and private visibility. Framework checks the complete candidate, including skill references, without creating provider clients or activating a generation. Java returns Framework diagnostics to Go, preserving source paths when available. |
| Install or restore a publication | The Go placement controller sends the original ZIP, the target namespace, and the Cloud-assigned opaque `publicationVersion` to the engine's internal API. | The Java wrapper reads the complete skill pack and runtime configuration and publishes into the supplied namespace. Go can confirm the version through Console before routing work. Restoration uses the same request with the same saved ZIP and version. |
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

### Planned Framework feature ticket: explicit variable references

**Blocker for Cloud's pre-publication check for missing variables across provider connections and executor settings.** Implementing this check depends on Framework exposing all references in the pack through the common reference contract.

Published provider connections currently use explicit credential references such as `api-key-ref`. Executor-owned configuration preserves literal data. Define one explicit variable reference syntax for provider connections and executor settings, including settings introduced by future executors. Framework should let Cloud identify the referenced variable names and their source locations before publication. The same reference must resolve from the target application's variable values wherever it appears in supported settings.

An acceptance case is a pack whose provider connection and executor each reference a variable. Cloud can report a missing name and its location before activation. Once both variables are supplied, Framework can prepare the same pack. The exact notation and compatibility with existing provider reference fields belong in the Framework ticket.

### Publication and cutover

For an application restart, the wrapper prepares and publishes a fresh Framework generation even when the requested publication version is already installed. Console exposes the resulting generation ID for the [restart confirmation in Go](go-backend-roadmap.md#restarting-an-application).

Go sends one installation request containing the target namespace, the Cloud-assigned `publicationVersion`, and the original ZIP. That request asks Java to prepare and publish the complete pack. Staging happens in Cloud. The engine reports success after Framework publishes the update.

When Java reports success, the placement controller asks the Console proxy to refresh the target engine's instance snapshot. The proxy updates its projection from the snapshot. The router can then select that ready instance once the projection confirms the target namespace and requested `publicationVersion` on the connected JVM. If Go loses the installation response, the controller requests the same refresh to determine which version is active.

The wrapper adapts the ZIP to Framework skill documents and execution configuration, supplies private visibility and the assigned version to `SkillReloader.prepare`, then publishes the prepared update. Framework keeps the current generation serving during preparation. After publication, fresh invocations use the replacement and already admitted invocations retain their captured generations, including for reachable child skills. A failed update leaves the current publication available. Publishing zero YAML skill documents removes previously published YAML skills and leaves the namespace installed.

When validation or installation fails, the Java wrapper returns a machine-readable error code identifying the cause, together with technical diagnostics. It preserves the affected skill, file, or configuration path when Framework supplies it. Define the codes and response fields in the engine API contract so Go can act on the code independently of the diagnostic wording.

### Application diagnostics and retained traces

The engine exposes its live Console stream and writes retained traces to the shared persistent filesystem when tracing is enabled. A retained trace must remain readable after engine replacement. The [Go Console proxy roadmap](go-backend-roadmap.md#console-proxy) owns namespace-scoped presentation and historical trace discovery.

The engine makes a retained trace discoverable once execution has finished and the saved artifact is complete and parseable in the trace format. A failed skill can produce a valid, discoverable trace.

**Blocker for filesystem trace discovery.** The current Loomspan Framework writer appends records directly to the trace path, so a directory scan can see an unfinished trace. We intend to create a Loomspan Framework feature ticket to have the writer use an in-progress filename and atomically rename a completed, validated, retained trace to its final name on the same filesystem. If writing or validation fails, or the JVM stops mid-write, the file must never appear under the final name. Verify atomic rename support on the chosen Cloud mount during integration.

## First single-engine milestone

**Current implementation:** The standalone Maven project builds and starts the Spring Boot host. Its Cloud ZIP adapter, installation and validation endpoints, K3s probe and mTLS wiring, and Go integration are planned work.

**Next scope:** Use a ready engine with no customer publications. Start with a pack using Framework defaults so install and observation can be exercised independently of the variable-reference and trace-writer blockers. The first integration milestone is to send one application's ZIP, namespace, and version to that process and verify that the success response triggers a proxy refresh. Before the first invocation, confirm that the router sees the installed namespace and version on the expected JVM through the updated projection. Exercise a failed replacement and verify the previous version still serves. Restart the engine, reinstall the saved ZIP with its original version, and confirm it on the new JVM instance.

Use the [publication ZIP format](publication-package-format.md) as the shared archive contract with Go. The installation request shape, machine-readable error codes, and response fields are the first API details to settle. Verify that the proxy can fetch the instance snapshot using mTLS and the environment's Console API key. In the subsequent invocation milestone, verify the default response omits the view. With tracing disabled, verify success and error responses for both opt-in modes, including omission of the view on a successful `on_error` invocation.

## After V1: engine capacity

Defer Cloud-specific concurrency limits and overload behavior until we can profile the working engine with representative workloads. Use measured resource use and latency under concurrent invocations to decide whether admission limits or queuing are needed.
