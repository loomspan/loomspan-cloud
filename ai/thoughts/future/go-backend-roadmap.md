# Go backend roadmap

## Go backend components

V1 runs one Go backend deployment. Its four internal components share the Console proxy's in-memory observations.

| Name | Responsibility |
| --- | --- |
| **Go backend** | The deployable service that contains the Cloud API, Console proxy, placement controller, and router. |
| **Cloud API** | Handles caller-facing requests and authorization, including administration of application settings and roles, uploads, validation, and publication requests. |
| **Console proxy** | Maintains the Framework Console connections and the observed state of engine pods, JVMs, namespaces, and publication versions. |
| **Placement controller** | Reconciles intended placements with the Console proxy's observations, publishes or restores applications, and confirms the intended publications are serving. |
| **Router** | Selects an eligible engine for an authorized invocation using the intended placement and the Console proxy's observations. |

The [execution engine API](execution-engine-api.md) defines the interface to Java.

## Upload and staged validation

The Cloud API authorizes the upload and enforces the [archive rules and limits](publication-package-format.md#archive-rules) across the whole ZIP. It limits ZIP bytes at receipt, counts all entries including explicit directories, and counts actual expanded bytes as it reads every file, including companion assets. It checks entry types, path spelling and ambiguity, and case-insensitive path uniqueness. For an invalid entry it reports the offending path, or both paths for a collision.

Before staging, the Cloud API checks the single destination namespace directory, the [companion entry point](publication-package-format.md#required-companion-entry-point), and the [execution subtree layout](publication-package-format.md#execution-inputs). It reads the authored execution configuration to reject reserved administrative sections and any executor configuration owner key that differs from the target namespace. A pack that passes these checks can be staged while the engine is offline. Staged validation can still report errors in the skill definitions, their references, or the execution configuration.

The Cloud API computes SHA-256 over the exact ZIP bytes and compares the digest with revisions in the target application. A match with the current revision returns it and its validation results. A match only with an older revision returns a warning naming the matched and current revisions. Confirmation applies to that pair. If staging changes before confirmation, the Cloud API reevaluates the upload and refreshes the warning if needed.

A confirmed restaging creates a new current revision referencing the same stored ZIP. Validation results stay with their original revision. Previously unseen ZIP bytes create a new revision after upload checks pass. The Cloud API makes the comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Through Console or MCP, an authorized caller can request validation of a specific revision. The Cloud API associates the engine's validation result and diagnostics with the requested revision, even if another upload arrived during validation. A validation error leaves the pack available for inspection and the current publication serving. Corrections go through the same upload checks.

Validation describes the combination of staged revision and administrative settings used for the check. If settings change before publication, the engine checks the combination it actually installs. The Go backend keeps the settings sent for a publication distinguishable from subsequent edits so Console can show the confirmed active settings even if an edit arrives during installation. [Administrative runtime settings](java-execution-engine-roadmap.md#administrative-runtime-settings)

### Publishing a reviewed revision

During staged validation and before each install, the Go backend discovers references in the selected ZIP using the [package variable syntax](publication-package-format.md#application-variables). Report missing or blank variables with their pack-relative source and authored YAML path.

For each installation, including restart and recovery, the Go backend reads the destination application’s current values for those names, computes its effective runtime settings from current saved selections and Cloud defaults, and uses the [publish endpoint](execution-engine-api.md#internal-api-requests). The ZIP is the durable record of its requirements.

The caller names the staging revision to publish. When accepting the publish request, the Cloud API compares it with the application's current staged revision. If they differ, the Cloud API returns a warning identifying both revisions and waits for explicit confirmation before dispatching the installation. Confirmation authorizes publication of the named, reviewed revision. The newer revision stays current in staging for continued work. Console and MCP use this same Cloud API operation.

Go can publish as soon as the Console proxy reads the first authenticated instance snapshot from the intended engine JVM. If the proxy lacks a snapshot from the current connection, the Cloud API reports the publish attempt as failed. The revision stays staged, the current publication pointer stays in place, and the owner can retry. Go can route invocations once the proxy confirms the requested namespace and publication version on an eligible engine.

On engine recovery, the placement controller loads the version recorded as the application's current published version. Once a new version is durably marked published, it is the recovery target even if the engine goes down immediately afterward.

### Unconfirmed publications

If the placement controller loses the publish response after dispatch, it requests an immediate snapshot refresh from the Console proxy and watches for the requested change for up to 30 seconds. For a new publication, confirmation means observing the requested namespace and version on the intended engine. For a restart, use the [generation check](#restarting-an-application). If the window expires, report the version as unconfirmed, explain that it may still complete, and offer an explicit retry if that version is still intended.

The placement controller serializes the retry with other publish operations for the application. Before dispatch, it checks that the requested version is still the application's current publication target. If a newer target has superseded it, the Cloud API rejects the retry and identifies the current target to the owner. Restoring the older version requires a separate, deliberate publish as a downgrade. If the target still matches, the placement controller checks the Console proxy's latest snapshot and reports success if the original change is now visible. Otherwise it sends the publish request again.

### Restarting an application

Saving variables or administrative settings leaves the running generation unchanged. The next successful publication applies both. An owner can choose **Restart application** to republish the current ZIP and version with the saved edits. Other applications keep serving.

The placement controller serializes restart and publish operations per application. Before dispatching a restart, it records the connected JVM's `instanceId`, the application's current `generationId`, and its `publicationVersion`. After Java reports success, it asks the Console proxy for a fresh snapshot. It reports the restart complete once the Console proxy observes the same namespace and publication version with a different generation ID on the same JVM.

If preparation fails, the Cloud API reports the diagnostic and the previous generation keeps serving. If the response is lost, the placement controller follows the [publish confirmation rule](#unconfirmed-publications). Generation IDs are process-local. A changed instance ID triggers engine recovery and fresh placement confirmation.

## Publication promotion

An ordinary upload must match the selected application's namespace. The Cloud API reports a mismatch so the author can correct the pack. For promotion through Console or MCP, the caller selects a source publication and a destination application. the Cloud API checks access to the source and permission to stage changes at the destination.

The Cloud API stages a destination pack derived from the selected publication. When the namespaces differ, it renames the ZIP's top-level directory to the destination namespace, updates the `name` in the companion `SKILL.md` frontmatter to match, and renames the source namespace's owner key under `loomspan.executor-config` in `loomspan/execution.yaml` if present. It preserves the companion instructions, Framework skill definitions, nested settings, and explicit references to shared executors and other external dependencies. If no edit is needed, the Cloud API can reuse the original ZIP. The staged pack identifies its source publication and exposes the namespace change for review. Publishing it uses the destination application's normal validation and publication flow.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, the Cloud API could generate a client-only skill pack for that user's companion. It would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. The Cloud API would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response loss

The Cloud API defines what its caller-facing interfaces expose from the [engine's invoke response](execution-engine-api.md#invoke-responses).

The router removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. The router does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

## Console proxy

### Observe live engines

The Console proxy maintains its in-memory observations using the [pod and JVM identity contract](execution-engine-api.md#pod-and-jvm-identity). After an install succeeds or its response is lost, the placement controller asks the Console proxy to refresh that engine's snapshot. On disconnect, the Console proxy keeps the last observation available for inspection and marks it disconnected. Routing requires a connected instance with a confirmed version.

On Go backend startup or Console reconnection, the placement controller compares each intended placement with the fresh snapshot for that engine instance. If the namespace and publication version match, routing can resume on the existing generation with the variables and settings it already loaded. The placement controller installs only missing or out-of-date publications and waits for the Console proxy to confirm the requested version before routing those applications.

Recovery proceeds per namespace. As soon as the Console proxy confirms an application's intended publication on a ready engine, the router can send it new requests, even while other namespaces are still restoring. If one restore fails, the Cloud API reports that application's failure while the placement controller continues restoring the remaining namespaces. Already-confirmed applications keep serving.

After a failed restore, the owner can select **Restart application** to retry the current publication. [Validate and publish responses](execution-engine-api.md#validate-and-publish-responses)

In V1 the router sends work only to intended placements. If the Console proxy observes an additional namespace, the placement controller leaves it loaded and the router excludes it from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The Console proxy consumes streams from engines that can host several applications. The Cloud API authorizes each user's or companion's access and the Console proxy exposes live data scoped to the authorized application namespace.

### Look up retained traces

Engines write retained traces to a persistent filesystem. The Console proxy reads those files for authorized historical lookups, including after the producing engine has been replaced. Users can browse runs with retained traces and open the trace for a particular invocation.

**Proposed V1 design.** Scan finalized files on startup and build an in-memory index using the entry namespace, root skill, trace ID, start time, elapsed time, outcome, and file location. Index the trace under the initiating application even when child calls appear in it. Load the trace payload when an authorized caller opens it. A trace-completion notification could prompt a refresh, with periodic scans to discover missed files and reconcile index entries for files that have disappeared.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify application access before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

**Open integration question.** Framework's optional execution view exposes a session ID, and retained trace records also carry it. Verify exact-run lookup for concurrent invocations and decide how the playground obtains the lookup identifier when the view is omitted or requested only on error. The existing trace start and completion records already carry the entry identity and timestamps needed for the proposed listing.
