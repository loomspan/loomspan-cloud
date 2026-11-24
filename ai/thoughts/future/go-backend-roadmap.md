# Go backend roadmap

## Go backend components

V1 runs one Go backend deployment. Its four internal components share the Console proxy's in-memory observations.

| Name | Responsibility |
| --- | --- |
| **Go backend** | The deployable service that contains the Cloud API, Console proxy, placement controller, and router. |
| **Cloud API** | Handles caller-facing requests and authorization, including administration of application settings and roles, uploads, validation, and publication requests. |
| **Console proxy** | Maintains the Framework Console connections and the observed state of engine pods, JVMs, namespaces, and publication versions. |
| **Placement controller** | Reconciles intended placements with the Console proxy's observations, publishes or restores applications, and confirms the intended publications are serving. |
| **Router** | Selects an eligible engine for an authorized invocation using the intended placement and the Console proxy's observations. Framework handles [invocation cutover](java-execution-engine-roadmap.md#publication-and-cutover). |

The [execution engine API](execution-engine-api.md) defines the interface to Java.

## Upload and staged validation

### ZIP checks before staging

Go authorizes the upload and checks the entire archive against the [package limits](publication-package-format.md#archive-rules). It caps the incoming ZIP bytes, counts every entry including explicit directories, and counts actual expanded bytes while reading every file, including companion assets. It rejects nonregular files, entry types other than files and directories, ambiguous paths, and paths that collide without regard to case. Report the offending path, or both paths for a collision.

Go requires one top-level directory matching the destination namespace, the [companion entry point](publication-package-format.md#required-companion-entry-point), and the [execution subtree layout](publication-package-format.md#execution-inputs), including lowercase skill paths. In `loomspan/execution.yaml`, reject reserved administrative sections and any executor configuration owner key that differs from the destination namespace. Report the offending field and its pack-relative source. A pack that passes these checks can be staged while the engine is offline.

### Staging revisions and validation

The Cloud API computes SHA-256 over the exact ZIP bytes and compares the digest with revisions in the target application. A match with the current revision returns it and its validation results. A match only with an older revision returns a warning naming the matched and current revisions. Confirmation applies to that pair. If staging changes before confirmation, the Cloud API reevaluates the upload and refreshes the warning if needed.

A confirmed restaging creates a new current revision referencing the same stored ZIP. Validation results stay with their original revision. Previously unseen ZIP bytes create a new revision after upload checks pass. The Cloud API makes the comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Through Console or MCP, an authorized caller can validate a specific staged revision. Go runs all independent checks it can complete, including engine validation of the authored pack even when application variables are missing or blank. It returns the engine diagnostics, all missing or blank variable references (with pack-relative source and authored YAML path), and any other pack issues found by Cloud in one result. The result belongs to the requested revision even if another upload arrives. The pack stays staged and the current publication keeps serving.

If engine validation is unavailable, interrupted, or blocked by an unreadable request, Go includes that outcome and every available finding in the same caller-facing result. The result identifies any checks that could not complete.

If the pack is otherwise valid, report that it passed validation and identify the variables blocking publication. Once the owner supplies them, they can publish the same revision directly. Publication checks the current values.

Validation describes the combination of staged revision and administrative settings used for the check. If settings change before publication, the engine checks the combination it actually installs. The Go backend keeps the settings sent for a publication distinguishable from subsequent edits so Console can show the confirmed active settings even if an edit arrives during installation. [Administrative runtime settings](java-execution-engine-roadmap.md#administrative-runtime-settings)

Go reads the application's active trace persistence policy from `tracePersistencePolicy` on the matching namespace entry in a fresh authenticated instance snapshot. Console compares that observed policy with the owner's saved selection. A fresh snapshot supplies the active policy after a Go restart or a lost publish response. To confirm that a particular restart completed, Go still applies the [JVM and generation checks](#restarting-an-application).

### Publishing a reviewed revision

Before each install, Go checks the selected ZIP’s [variable references](publication-package-format.md#application-variables) and reports any missing or blank values with their pack-relative source and authored YAML path.

When starting a publication, restart, or recovery, Go reads the destination application’s current values for those names, computes its effective runtime settings from saved selections and Cloud defaults, and uses the [publish endpoint](execution-engine-api.md#internal-api-requests). The ZIP is the durable record of its requirements.

The caller names the staging revision to publish. When accepting the publish request, the Cloud API compares it with the application's current staged revision. If they differ, the Cloud API returns a warning identifying both revisions and waits for explicit confirmation before dispatching the installation. Confirmation authorizes publication of the named, reviewed revision. The newer revision stays current in staging for continued work. Console and MCP use this same Cloud API operation.

If the owner selects **Publish** for the staged revision that is already the application's current publication, the Cloud API reports “This revision is already published.” It returns without creating a publication version or dispatching an installation. To apply saved variables or runtime settings to that revision, the owner selects [Restart application](#restarting-an-application).

Go can publish as soon as the Console proxy reads the first authenticated instance snapshot from the intended engine JVM. If the proxy lacks a snapshot from the current connection, the Cloud API reports the publish attempt as failed. The revision stays staged, the current publication pointer stays in place, and the owner can retry. Go can route invocations once the proxy confirms the requested namespace and publication version on an eligible engine.

On receiving `204 No Content` from Java, Go durably records the acknowledged version as the application's current published version before reporting publish success to the caller. If the pod dies before Console observes that version, the placement controller restores it on the replacement. Routing resumes only after a fresh Console snapshot confirms the namespace and version on the eligible replacement JVM.

### Unconfirmed publications

**Retry** means “finish the publication I requested.” Go records the application’s retry revision when it accepts a publish or restart. Saving a variable or runtime setting, or accepting another publish or restart (even for the same version), advances that revision. An edit followed by a reversal still closes the earlier retry. Staging another ZIP or changing roles or unrelated metadata leaves the revision unchanged.

If the placement controller loses the publish response after dispatch, it requests an immediate snapshot refresh from the Console proxy and watches for the requested change for the configured confirmation window (30 seconds in V1). For a new publication, confirmation means observing the requested namespace and version on the intended engine. For a restart, use the [restart confirmation rule](#restarting-an-application). If the window expires, report the version as unconfirmed, explain that it may still complete, and offer an explicit retry if the request’s retry revision is still current.

Go reconciles durably recorded publish and restart requests after a backend restart and after the caller's confirmation window expires. Before restoring the previous current publication, Go checks a fresh authenticated snapshot from the intended engine. If it confirms the requested installation, Go records the confirmed publication as current without requiring the caller to retry. Seeing a distinct new publication version in the requested namespace confirms a new publication. For a same-version restart, apply the generation and JVM checks below.

In the same Postgres transaction that records the confirmed publication as current, Go verifies that no newer publish or restart request has been accepted for that application. Invocations can proceed under the normal routing rules and do not supersede the publication request.

If Go accepts a newer publish or restart for the same application while the earlier installation is unconfirmed, it holds the newer request until the earlier installation is confirmed or 30 seconds past the end of its configured confirmation window, whichever comes first. Go measures that deadline from the start of the earlier request's confirmation wait (60 seconds total with the V1 setting). Once the deadline passes, Go can dispatch the newer request even if the earlier installation is still unconfirmed.

Framework rejects an earlier candidate as stale if the newer generation becomes active after the earlier candidate captured its base generation. V1 accepts the remaining risk that a delayed earlier request can first enter preparation after the newer generation is active and replace it on the engine. Go still applies the current-publication transaction check above.

On retry, the Cloud API checks the caller’s current permission. Go serializes the revision check and dispatch with changes that advance the application’s retry revision. If the revision has changed, Go sends nothing to the engine and tells the caller why the retry is closed. Otherwise, the placement controller checks the latest Console snapshot. If it confirms the original request, Go reports success. If confirmation is still missing, Go resends the original ZIP and publication version with the unchanged variable values and effective runtime settings.

### Restarting an application

Saving variables or administrative settings leaves the running generation unchanged. An owner can choose **Restart application** to republish the current ZIP and version with the saved edits. Other applications keep serving.

The placement controller serializes restarts and publishes per application. Before dispatch, it reads a fresh Console snapshot and records the JVM’s `instanceId` and the namespace’s `generationId` if present. After Java reports success, Go requests another snapshot from the same JVM to confirm the current published version:

- If the namespace was present, confirm that it serves the requested version with a different `generationId`.
- If the namespace was absent, confirm that it now appears with the requested version.

If preparation fails, the Cloud API reports the diagnostic and any previously active generation keeps serving. If the response is lost, the placement controller follows the [publish confirmation rule](#unconfirmed-publications). Generation IDs are process-local. A changed instance ID triggers engine recovery and fresh placement confirmation.

## Publication promotion

An ordinary upload must match the selected application’s namespace. For promotion through Console or MCP, the caller selects a source publication and destination application. The Cloud API checks access to the source and permission to stage at the destination.

When the namespaces differ, Go renames the ZIP’s top-level directory, the companion’s `SKILL.md` frontmatter `name`, and the `loomspan.executor-config` owner key if present. It preserves the companion instructions, skill definitions, nested settings, and explicit external references. Check every local skill name against the [combined length limit](publication-package-format.md#namespace-and-skill-names) using the destination namespace before staging. If no edit is needed, Go can reuse the original ZIP. The staged revision identifies its source publication and exposes the namespace change for review. The destination’s normal validation and publication flow applies.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, the Cloud API could generate a client-only skill pack for that user's companion. It would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. The Cloud API would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response visibility

Go alone enforces the V1 request limits for REST and MCP invocations before dispatch to Java. The caller's invoke request body may contain up to 20 MiB (20,971,520 bytes) in total and up to 25 uploaded files. Both limits are inclusive. Count the entire JSON or multipart body, including metadata, file contents, part headers, and boundaries. Each uploaded file part counts once, including an empty file. The JSON metadata part counts only toward the byte limit. If either limit is exceeded, Go rejects the request before dispatch.

The Cloud API applies the same V1 rule to REST and MCP. With permission to invoke the application, callers receive the exact `result` on success or the engine’s top-level `message` verbatim and any top-level input-validation `issues` on failure. Caller-friendly error categories and rewritten messages are deferred to V2. [Engine response fields](execution-engine-api.md#invoke-responses)

One application administrator setting controls `executionView`, `debug`, and `diagnosis` in REST and MCP invoke responses. It defaults to off for new applications. When enabled, Go automatically returns the execution view for the invoker's own call on success or failure and includes the available `debug` and `diagnosis` fields on failure. When disabled, Go omits all three fields even if the invoker asks for them. The switch applies independently of trace persistence.

## Invocation response loss

The router removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. The router does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

## Console proxy

### Observe live engines

The Console proxy maintains its in-memory observations using the [pod and JVM identity contract](execution-engine-api.md#pod-and-jvm-identity). After an install succeeds or its response is lost, the placement controller asks the Console proxy to refresh that engine's snapshot. On disconnect, the Console proxy keeps the last observation available for inspection and marks it disconnected. Routing requires a connected instance with a confirmed version.

On Go backend startup or Console reconnection, the placement controller compares each intended placement with the fresh snapshot for that engine instance. If the namespace and publication version match, routing can resume on the existing generation with the variables and settings it already loaded. The placement controller installs only missing or out-of-date publications and waits for the Console proxy to confirm the requested version before routing those applications.

Recovery proceeds per namespace. As soon as the Console proxy confirms an application's intended publication on a ready engine, the router can send it new requests, even while other namespaces are still restoring. If one restore fails, the Cloud API reports that application's failure while the placement controller continues restoring the remaining namespaces. Already-confirmed applications keep serving.

After a failed restore, the owner can select [Restart application](#restarting-an-application) to install the current publication with the latest saved variables and settings.

In V1 the router sends work only to intended placements. If the Console proxy observes an additional namespace, the placement controller leaves it loaded and the router excludes it from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The Console proxy consumes streams from engines that can host several applications. Only application administrators and companions acting with their permissions can inspect live system events. The Cloud API enforces that permission and the Console proxy scopes the stream to the authorized application namespace.

### Look up retained traces

Engines write retained traces to a persistent filesystem. The Console proxy reads those files for authorized historical lookups, including after the producing engine has been replaced. Only application administrators and companions acting with their permissions can browse retained runs or look up, review, and download traces for that application.

**Proposed V1 design.** Scan finalized files on startup and build an in-memory index using the entry namespace, root skill, trace ID, start time, elapsed time, outcome, and file location. Index the trace under the initiating application even when child calls appear in it. Load the trace payload when an authorized caller opens it. A trace-completion notification could prompt a refresh, with periodic scans to discover missed files and reconcile index entries for files that have disappeared.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify administrator permission for the application before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

Verify that an authorized lookup identifies the correct retained trace when several callers invoke the same skill concurrently.
