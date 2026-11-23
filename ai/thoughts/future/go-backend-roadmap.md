# Go backend roadmap

Start with one Go backend deployment. The placement controller reconciles intended placements, the router selects an engine for an authorized invocation, and the Console proxy supplies the observed state both use. The [Java engine integration contracts](java-execution-engine-roadmap.md#integration-contracts) describe what Go sends and what the engine returns.

## Upload and staged validation

Go authorizes the upload and checks the ZIP against the [package contract](publication-package-format.md) before accepting it into the target application's staging area. Go counts expanded bytes while reading the entries and reports the offending path, or both paths for a collision. A successful upload can be staged while the engine is offline.

Go computes SHA-256 over the exact ZIP bytes and compares the digest with revisions in the target application. A match with the current revision returns it and its validation results. A match only with an older revision returns a warning naming the matched and current revisions. Confirmation applies to that pair. If staging changes before confirmation, Go reevaluates the upload and refreshes the warning if needed.

A confirmed restaging creates a new current revision referencing the same stored ZIP. Validation results stay with their original revision. Previously unseen ZIP bytes create a new revision after upload checks pass. Go makes the comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Through Console or MCP, an authorized caller can request validation of a specific revision. Go sends that revision's ZIP, the target namespace, and effective runtime settings to the engine and associates the returned result and diagnostics with that revision, even if another upload arrived during validation. A validation error leaves the pack available for inspection and the current publication serving. Corrections go through the same upload checks.

Validation describes the combination of staged revision and administrative settings used for the check. If settings change before publication, the engine checks the combination it actually installs. Go keeps the settings sent for a publication distinguishable from subsequent edits so Console can show the confirmed active settings even if an edit arrives during installation. [Administrative runtime settings](java-execution-engine-roadmap.md#administrative-runtime-settings)

### Publishing a reviewed revision

During staged validation and before each install, Go discovers references in the selected ZIP using the [package variable syntax](publication-package-format.md#application-variables). Report missing or blank variables with their pack-relative source and authored YAML path.

For each installation, including restart and recovery, Go reads the destination application’s current values for those names, computes its effective runtime settings from current saved selections and Cloud defaults, and sends both with the same ZIP under the [engine install contract](java-execution-engine-roadmap.md#integration-contracts). The ZIP is the durable record of its requirements.

The caller names the staging revision to publish. When accepting the publish request, Go compares it with the application's current staged revision. If they differ, Go returns a warning identifying both revisions and waits for explicit confirmation before dispatching the installation. Confirmation authorizes publication of the named, reviewed revision. The newer revision stays current in staging for continued work. Console and MCP use this same Go operation.

Before accepting a publish request, Go checks that the intended engine is ready and its Framework endpoint is reachable. If this check fails, Go reports the publish attempt as failed. The revision stays staged, the current publication pointer stays in place, and the owner can retry. If contact is lost after dispatch, Go uses the existing Console snapshot reconciliation to determine whether Framework installed the requested version.

On engine recovery, Go loads the version recorded as the application's current published version. Once a new version is durably marked published, it is the recovery target even if the engine goes down immediately afterward.

### Restarting an application

Saving variables or administrative settings leaves the running generation unchanged. The next successful publication applies both. An owner can choose **Restart application** to republish the current ZIP and version with the saved edits. Other applications keep serving.

Go serializes restart and publish operations per application. Before dispatching a restart, Go records the connected JVM's `instanceId`, the application's current `generationId`, and its `publicationVersion`. After Java reports success, Go requests a fresh Console snapshot. It reports the restart complete once the proxy observes the same namespace and publication version with a different generation ID on the same JVM.

If preparation fails, Go reports the diagnostic and the previous generation keeps serving. If the response is lost, Go refreshes the snapshot before reporting an outcome. Generation IDs are process-local. A changed instance ID triggers engine recovery and fresh placement confirmation.

## Publication promotion

An ordinary upload must match the selected application's namespace. Go reports a mismatch so the author can correct the pack. For promotion through Console or MCP, the caller selects a source publication and a destination application. Go checks access to the source and permission to stage changes at the destination.

Go stages a destination pack derived from the selected publication. When present, it changes the owner key under `loomspan.executor-config` from the source namespace to the destination namespace. It preserves skill content, nested settings, and explicit references to shared executors and other external dependencies. If no edit is needed, Go can reuse the original ZIP. The staged pack identifies its source publication and exposes the namespace change for review. Publishing it uses the destination application's normal validation and publication flow.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, Go could generate a client-only skill pack for that user's companion. Go would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. Cloud would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response loss

Go removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. Go does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

## Console proxy

### Observe live engines

The proxy authenticates snapshot and stream requests under the [engine integration contract](java-execution-engine-roadmap.md#integration-contracts).

Go discovers ready engine pods through K3s. The proxy owns the authenticated Console connections and maintains an in-memory view of each JVM instance and its installed namespaces and publication versions. It reads a fresh instance snapshot on connection and reconnection. After an install succeeds or its response is lost, the placement controller asks the proxy to refresh that engine's snapshot. On disconnect, the proxy keeps the last observation available for inspection and marks it disconnected. Routing requires a connected instance with a confirmed version.

On Go startup or Console reconnection, the placement controller compares each intended placement with the fresh snapshot for that engine instance. If the namespace and publication version match, routing can resume on the existing generation with the variables and settings it already loaded. The controller installs only missing or out-of-date publications and waits for the proxy to confirm the requested version before routing those applications.

Recovery proceeds per namespace. As soon as the proxy confirms an application's intended publication on a ready engine, the router can send it new requests, even while other namespaces are still restoring. If one restore fails, Go reports that application's failure and continues restoring the remaining namespaces. Already-confirmed applications keep serving.

When Java reports a failure, Go uses the engine's machine-readable error code to decide whether to retry and its technical details to compose the message for the user or companion. Go retries transient restore failures automatically with backoff. If restoration requires an owner action, such as supplying a missing variable or correcting a [Vertex connection](java-execution-engine-roadmap.md#tenant-vertex-ai-credentials), Go pauses automatic retries for that namespace and shows the reason. After correcting the configuration, the owner can select **Restart application** to retry the current publication.

In V1 the router sends work only to intended placements. If Console reports an additional namespace, reconciliation leaves it loaded and excludes that placement from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The proxy consumes streams from engines that can host several applications. The shared Cloud API authorizes each user's or companion's access and the proxy exposes live data scoped to the authorized application namespace.

### Look up retained traces

Engines write retained traces to a persistent filesystem. The Cloud proxy reads those files for authorized historical lookups, including after the producing engine has been replaced. Users can browse runs with retained traces and open the trace for a particular invocation.

**Proposed V1 design.** Scan finalized files on startup and build an in-memory index using the entry namespace, root skill, trace ID, start time, elapsed time, outcome, and file location. Index the trace under the initiating application even when child calls appear in it. Load the trace payload when an authorized caller opens it. A trace-completion notification could prompt a refresh, with periodic scans to discover missed files and reconcile index entries for files that have disappeared.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify application access before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

**Open integration question.** Framework's optional execution view exposes a session ID, and retained trace records also carry it. Verify exact-run lookup for concurrent invocations and decide how the playground obtains the lookup identifier when the view is omitted or requested only on error. The existing trace start and completion records already carry the entry identity and timestamps needed for the proposed listing.
