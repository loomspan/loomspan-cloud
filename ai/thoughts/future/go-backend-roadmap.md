# Go backend roadmap

## Go backend components

Full Cloud runs one Go backend deployment in V1. Its four internal components share the Console proxy's in-memory observations.

| Name | Responsibility |
| --- | --- |
| **Go backend** | The deployable service that contains the Cloud API, Console proxy, placement controller, and router. |
| **Cloud API** | Handles caller-facing requests and authorization, including administration of application settings and roles, uploads, validation, and publication requests. |
| **Console proxy** | Maintains the Framework Console connections and the observed state of engine pods, JVMs, namespaces, and publication versions. |
| **Placement controller** | Reconciles intended placements with the Console proxy's observations, publishes or restores applications, and confirms the intended publications are serving. |
| **Router** | Selects an eligible engine for an authorized invocation using the intended placement and the Console proxy's observations. Framework handles [invocation cutover](java-execution-engine-roadmap.md#publication-and-cutover). |

The [execution engine API](execution-engine-api.md) defines the interface to Java.

## Go backend configuration

In V1 the operator supplies one Go backend configuration file through a mounted Kubernetes ConfigMap. The file has two top-level sections. Go reads `startup` when the backend starts and applies edits to it on the next restart.

At startup, Go validates both sections before it becomes ready to serve requests. If the file cannot be read or parsed, or an installation-wide setting fails validation, Go stays unready, logs the error with the offending setting when identifiable, and periodically rereads the mounted file. Once installation-wide validation succeeds, Go applies the configuration and starts serving automatically.

After startup, Go periodically rereads `reloadable` and validates it independently of `startup`. After Kubernetes propagates an edit to the mount, Go applies all validated reloadable settings at once. If Go cannot read or parse the file or an installation-wide reloadable setting fails validation, Go logs the error and keeps the last valid reloadable settings.

At startup and on reload, if any part of a namespace's configuration fails to load or validate, Go logs the application ID and error, skips the entire namespace for quota processing, and continues with the other namespaces. Skipping the namespace protects its retained history from deletion under an unintended fallback quota. The namespace participates again once its configuration loads successfully.

The `reloadable` section holds two independent installation-wide defaults of 100 MB (100,000,000 bytes) per application, one for [manual trace uploads](#manual-trace-upload-quota) and one for [retained engine traces](#trace-cleanup-job). For each application namespace, the operator can override either or both quotas using the application ID as the key. Both defaults and both kinds of override reload while Go is running, so operators can adjust allowances as applications grow or future pricing tiers change. Each namespace inherits the current default for any quota it does not override. Removing an override restores that quota’s current default for that namespace.

An explicit namespace override of `0` sets that quota to zero bytes.

## Application creation

Creating an application for a verified owner and slug returns the application ID. If the application already exists for that owner and slug, Go returns the same ID, indicates that the application already existed, and preserves its settings, memberships, and publications. Concurrent requests for the same owner and slug resolve to one application. A companion that loses the creation response can repeat the request and use the returned ID to continue staging its pack. Different owners can use the same slug for separate applications.

## Application permissions

Each application has exactly one owner, identified by verified email. Ownership transfer is a future feature. In V1 only the owner can manage access. The owner can assign, change, or revoke a user's Cloud role for that application, define and assign skill roles, and authorize a calling backend to assert end-user identities and skill roles.

| Access | What it permits |
| --- | --- |
| Owner | All Manager permissions and exclusive control over access. |
| Manager | All Caller permissions. Upload, download, validate, promote, and publish packs. Manage application variables and runtime settings, restart the application, and inspect live events and retained traces. |
| Caller | Read the application's invocation documentation through the API, invoke published skills, and receive the result and any execution details enabled for that call. This role grants no access to management REST endpoints, Cloud UI screens, or Console for the application. |

Manager and Caller are the two assignable Cloud roles. References to application administrators mean the owner and Managers. The shared Cloud API checks current permissions on each operation, including when a companion acts with its user's permissions through MCP. Framework enforces the user's separate skill roles when executing skills, including for owners and Managers.

### Invocation documentation

The Cloud API serves an authenticated OpenAPI document for each application. Its OpenAPI module reads the [namespace skill catalog](#namespace-skill-catalog) from the Console proxy, identifies the catalog's publication version in the document, and renders the callable entry points with their descriptions and schemas. On every request, the Cloud API checks current permission to invoke the application. For a signed-in user or their companion, it includes only entry points allowed by the user's current skill roles. A role change takes effect on the next documentation request even when the catalog is unchanged.

An integration presenting a valid application API key sees every published invocation entry point for that key's target application. The key alone is sufficient to read the document because the integration is authorized to assert every skill role for that application. When the integration invokes a skill, it supplies the end-user identity and skill roles under the [application API key contract](#application-api-keys).

### Companion approval and inactivity

Cloud enforces a default inactivity period of 30 days for each user-approved MCP companion connection. On the browser approval screen, the user can keep the default or enter a shorter or longer period for that companion. Later, the user can open their account’s connected-companions settings, select the connection, and change its inactivity period or revoke approval.

Go stores the period and last-use time per connection in Postgres. The clock starts at approval. Before allowing an operation, Go checks that the approval is current and the inactivity period has not elapsed. An authorized Cloud operation resets the clock. Token refreshes and keepalives leave it unchanged. An actively used connection stays approved until revoked. After inactivity expiry, the next operation requires fresh browser approval, even if the Supabase access token is still valid.

### Application API keys

Only the owner can issue or revoke application API keys. Each third-party integration can have up to two active keys for REST invocation of one target application. Both keys identify the same backend. Go checks the presented key and its scope on every call.

Cloud shows the secret to the owner only when the key is created. Go stores only a verification hash of the secret. Each key has a visible key ID and creation date so the owner can distinguish the keys and select one for revocation.

To rotate a key, the owner issues a second key, deploys and tests it in the calling application, then revokes the old key when ready. Revocation takes effect on the next call using that key and affects only that key. If both slots are occupied, the owner must revoke one key before issuing another.

The integration authenticates its own users. On each invocation it supplies a stable end-user ID and decides which of the target application's skill roles that user holds. The owner's authorization covers the full set of application skill roles. Go derives the issuer from the authenticated integration and uses the supplied end-user ID as the subject. The asserted roles grant permissions only within skill execution. [Invocation identity and roles](execution-engine-api.md#invocation-identity-and-roles)

### Application variables

In V1 all saved application variable values are write-only through the Cloud API. The owner and Managers can list variable names, see which have a nonblank value configured, and set, replace, or remove a value. Console, REST, and MCP expose only names and configuration status when reading variable settings. Go reads the required values internally for publication, restart, and recovery.

### Encryption at rest

Go encrypts saved application variable values before writing them to Postgres. Each Cloud environment supplies its own variable encryption key to Go through a Kubernetes Secret. Keep a recoverable copy outside the cluster so a replacement cluster can read the saved variables.

Go writes original pack ZIP bytes to the [shared volume](roadmap.md#durable-execution-traces), computes SHA-256 over those bytes, and returns the same bytes on authorized download. The storage provider handles file encryption transparently.

## Upload and staged validation

### ZIP checks before staging

Go authorizes the upload and checks the entire archive against the [package limits](publication-package-format.md#archive-rules). It caps the incoming ZIP bytes, counts every entry including explicit directories, and counts actual expanded bytes while reading every file, including companion assets. It rejects nonregular files, entry types other than files and directories, ambiguous paths, and paths that collide without regard to case. Report the offending path, or both paths for a collision.

Go requires one top-level directory matching the destination namespace, the [companion entry point](publication-package-format.md#required-companion-entry-point), and the [execution subtree layout](publication-package-format.md#execution-inputs), including lowercase skill paths. In `loomspan/execution.yaml`, reject reserved administrative sections and any executor configuration owner key that differs from the destination namespace. Report the offending field and its pack-relative source. A pack that passes these checks can be staged while the engine is offline.

### Staging revisions and validation

The Cloud API computes SHA-256 over the exact ZIP bytes and compares the digest with revisions in the target application. A match with the current revision returns it and its validation results. A match only with an older revision returns a warning naming the matched and current revisions. Confirmation applies to that pair. If staging changes before confirmation, the Cloud API reevaluates the upload and refreshes the warning if needed.

A confirmed restaging creates a new current revision referencing the same stored ZIP. Validation results stay with their original revision. Previously unseen ZIP bytes create a new revision after upload checks pass. The Cloud API makes the comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Through Console or MCP, an authorized caller can validate a specific staged revision. Go runs its Cloud checks and requests engine validation of the authored pack even when application variables are missing or blank. Once validation completes, Go returns the engine diagnostics, all missing or blank variable references (with pack-relative source and authored YAML path), and any other pack issues found by Cloud in one result. A completed result can report an invalid pack. The result belongs to the requested revision even if another upload arrives. The pack stays staged and the current publication keeps serving.

If validation cannot complete, Go reports that the result is unavailable and tells the caller to retry validation. Go withholds partial findings from the caller.

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

Promotion stages a selected publication from a source application as a revision in a destination application. The caller chooses the source and destination for each promotion through Console or MCP. The applications can serve any purpose. Cloud gives their names and the direction of promotion no special meaning.

Go owns promotion. It reuses the internal operations that serve publication downloads and ordinary uploads, with a namespace transformation between them.

1. **Export.** Check permission to read the selected source publication and stage at the destination. Read the immutable portable ZIP through the same operation used to download that publication.
2. **Transform.** When the namespaces differ, open the ZIP and write a new archive with the destination namespace as its top-level directory. Update the companion’s `SKILL.md` frontmatter `name` and the `loomspan.executor-config` owner key if present. Preserve companion instructions, skill definitions, nested settings, and explicit external references. Reuse the original ZIP bytes when no edit is needed.
3. **Import.** Pass the resulting ZIP to the ordinary destination upload and staging operation. It applies the [package checks](#zip-checks-before-staging), including the [combined namespace and skill name length limit](publication-package-format.md#namespace-and-skill-names), and the usual [digest comparison and restaging rules](#staging-revisions-and-validation). The staged revision identifies its source publication and exposes the namespace change for review.

The destination uses its own variables and runtime settings. Its current publication keeps serving until an authorized user publishes the staged revision. Framework sees only ordinary validate and publish requests for the destination namespace.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, the Cloud API could generate a client-only skill pack for that user's companion. It would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. The Cloud API would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response visibility

- [ ] Once the Framework ticket `ai/thoughts/tickets/invocation-session-id.md` lands in `loomspan-framework`, verify the implemented API and tests before marking this Framework follow-up complete. Check that success and post-session failure expose the canonical session ID with execution views off, including when no trace is retained, and that a failure constructing a requested view still carries the ID. Cloud response handling and authorized retained-trace lookup are separate Cloud work.

REST, MCP, and the UI all go through the same Go invocation logic, which enforces the V1 request limits before dispatch to Java. The caller's invoke request body may contain up to 20 MiB (20,971,520 bytes) in total and up to 25 uploaded files. Both limits are inclusive. Count the entire JSON or multipart body, including metadata, file contents, part headers, and boundaries. Each uploaded file part counts once, including an empty file. The JSON metadata part counts only toward the byte limit. If either limit is exceeded, Go rejects the request before dispatch.

The Cloud API applies the same V1 rule to REST and MCP. With permission to invoke the application, callers receive the exact `result` on success or the engine’s top-level `message` verbatim and any top-level input-validation `issues` on failure. Caller-friendly error categories and rewritten messages are deferred to V2. [Engine response fields](execution-engine-api.md#invoke-responses)

One application administrator setting controls `executionView`, `debug`, and `diagnosis` in REST and MCP invoke responses. It defaults to off for new applications. When enabled, Go automatically returns the execution view for the invoker's own call on success or failure and includes the available `debug` and `diagnosis` fields on failure. When disabled, Go omits all three fields even if the invoker asks for them. The switch applies independently of trace persistence.

## Invocation response loss

If the caller disconnects after execution starts, the engine continues the invocation under its normal runtime limits. V1 has no mechanism to cancel a running invocation.

The router removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. The router does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

### Application invocation log

The invocation service owns the application's log of incoming API invocations. Each entry records who made the request, when it arrived, the requested skill entry point, and the incoming parameters. The application's administration section makes this log available to its owner and current Managers. These fields support later manual comparison with traces, including when a caller loses a response. V1 records and displays the invocation log independently of Framework trace capture and lookup.

Within the Go invocation service, one invocation log component owns the layout, JSON Lines encoding, and all reads and writes of the namespace logs on the shared volume. Request handlers call its synchronous append operation, which serializes writes for the same namespace within the Go process and returns success once the entry has been written. Handlers for different namespaces can append independently. The administration view uses the component's application-scoped read operation after the Cloud API checks access. Reads return complete entries even while appends are in progress.

## Console proxy

### Observe live engines

The Console proxy maintains its in-memory observations using the [pod and JVM identity contract](execution-engine-api.md#pod-and-jvm-identity). After an install succeeds or its response is lost, the placement controller asks the Console proxy to refresh that engine's snapshot. On disconnect, the Console proxy keeps the last observation available for inspection and marks it disconnected. Routing requires a connected instance with a confirmed version.

On Go backend startup or Console reconnection, the placement controller compares each intended placement with the fresh snapshot for that engine instance. If the namespace and publication version match, routing can resume on the existing generation with the variables and settings it already loaded. The placement controller installs only missing or out-of-date publications and waits for the Console proxy to confirm the requested version before routing those applications.

Recovery proceeds per namespace. As soon as the Console proxy confirms an application's intended publication on a ready engine, the router can send it new requests, even while other namespaces are still restoring. If one restore fails, the Cloud API reports that application's failure while the placement controller continues restoring the remaining namespaces. Already-confirmed applications keep serving.

After a failed restore, the owner can select [Restart application](#restarting-an-application) to install the current publication with the latest saved variables and settings.

In V1 the router sends work only to intended placements. If the Console proxy observes an additional namespace, the placement controller leaves it loaded and the router excludes it from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The Console proxy consumes streams from engines that can host several applications. Only application administrators and companions acting with their permissions can inspect live system events. The Cloud API enforces that permission and the Console proxy scopes the stream to the authorized application namespace.

When the proxy loses the live event stream, Cloud tells the owner or Manager viewing each affected application “Live events temporarily unavailable.” When the proxy can deliver live events again, Cloud reports “Live events resumed.” Both notices describe event visibility for that application. Engine-wide diagnostic details stay in the platform logs.

On reconnect, the proxy resumes from the current point. The live timeline keeps the gap between the two notices visible and skips replay of events from that interval. If an affected run has a retained trace, the owner or Manager can inspect it in the application's run history.

### Namespace skill catalog

The Full Cloud Console proxy owns catalog retrieval, freshness, and storage. It uses the same authenticated engine observation as routing to identify the application's serving namespace and publication. Staging or validating a revision does not change the catalog. Through the Console connection, one namespace-scoped read returns the complete catalog from one active generation: skill names, descriptions, input schemas, output schemas where available, and effective entry-point access policies. The response identifies the engine instance, generation, and publication it actually describes.

Go keeps the lightweight engine observation in memory and reads the saved catalog from disk as needed. In the dedicated Go-owned `skill-catalogs/` directory on the [shared volume](roadmap.md#durable-execution-traces), the proxy keeps one replaceable file per application containing the complete catalog and its identity. This file is shared across documentation requests regardless of the caller's roles.

When the saved `instanceId` and `generationId` match the proxy's current observation for the serving namespace, the proxy reuses the file without querying Java. A missing file or a change to either ID prompts a full namespace read. Before replacing the file, the proxy checks the response's own identity and publication against the serving observation so a concurrent publish cannot label one generation's skills as another's. It writes the complete replacement to the same directory and atomically renames it into place. An interrupted refresh preserves the previous complete catalog. While a refresh is pending or the engine is offline, the last complete file remains available with its recorded publication version, including after Go restarts.

### Uploaded trace permissions

Full Cloud uses the shared volume's uploads directory, with a subdirectory per destination namespace. Console-only uses a persistent local uploads directory grouped by connected target and then by destination namespace, so two targets can each have a namespace named `default`.

Console lists and opens uploads through scoped API operations. The backend resolves each file from the authorized destination and an upload identifier, and uses the same scope for overwrite and deletion. Clients supply identifiers rather than filesystem paths. The directory comes from the confirmed destination, independently of the namespace recorded inside the file.

Go checks the destination application's diagnostic permissions on import, discovery, and inspection. Store the original bytes with separate destination, upload time, and optional expiry metadata. Keep uploads out of the engine's retained-trace index. Both Console profiles follow the same [import and retention workflow](roadmap.md#opening-a-trace-file).

The destination application's owner or any current Manager can change an upload's expiry or delete it, regardless of who uploaded it. Go checks the user's current role in the destination application on each request, including requests made by their companion.

### Manual trace upload quota

Full Cloud enforces the [configured manual-upload quota](#go-backend-configuration) in public and internal installations. Console-only leaves local disk usage under the user's control and applies no manual-upload quota.

Whenever Go checks manual-upload usage, including at save time and when the quota screen is read, it sums the sizes of the saved manual-upload files present for that application. An expired file counts until deletion. The next check reflects the freed space directly from the filesystem.

At save time, Go checks the upload or overwrite against the active quota. When the quota check passes, the upload proceeds through the normal import workflow. Console omits quota previews and quota-specific success messages. Users can review their usage on the [quota screen](#trace-quota-visibility).

The caller chooses how long to keep a manual upload, so lowering the quota preserves existing uploads and their expiry dates. Owners and Managers can still inspect, download, or delete them. For an overwrite, count the incoming file size minus the size of the upload it replaces. If the application is already over quota, allow changes that leave usage unchanged or reduce it. Reject increases that would put or keep the application over quota, and preserve the existing upload on rejection.

When Go rejects a manual upload or overwrite for exceeding quota, report the destination application's current manual-upload usage, its effective limit, and how many bytes the proposed usage would exceed that limit by. Use the values from the final quota check.

### Trace cleanup job

One Full Cloud cleanup job runs every five minutes with write access to the manual-upload and engine-trace directories. Each pass deletes manual uploads whose saved expiry has passed and trims retained engine traces to their per-application quotas. Only one pass can run at a time. If a pass is still running at a scheduled start, the job skips that start, lets the current pass finish, and resumes at the next scheduled start. At the start of each pass, it fetches a snapshot from Go with the active retained-trace default, accepted overrides, and application IDs skipped during configuration loading. The job uses that snapshot for the entire pass and skips those applications for both expiry and trimming. If Go is unready or the job cannot fetch the complete snapshot, the job skips the pass and tries again at the next scheduled pass. A quota edit first applies on the next pass after it becomes active, including when a lower quota requires deleting existing retained traces.

A zero-byte quota removes all eligible retained traces on the next pass. For each application over its retained-engine-trace quota, the job deletes eligible retained traces in order of completion, oldest first, until usage is at or below the quota or no eligible traces remain. The job skips active writes and files still in Console’s completion grace period. The quota is soft. Usage can exceed it between passes. Monitor free space on the shared volume. Time-based expiry for engine traces and cleanup of abandoned writes remain outside V1. Measure trace count, bytes per application, and growth over time to inform future quota values and whether allowances should vary by pricing tier.

Each cleanup pass is stateless. After an interruption, the next pass sees the files that remain on disk and can resume cleanup without a checkpoint. On completion it sends Go the post-cleanup retained-engine-trace byte count and measurement time for each application it measured. For each application where the job trims retained engine traces, it writes a structured audit log with the application ID, quota applied, number of traces removed, and bytes freed. These logs serve only as an audit record.

If cleanup fails for an application in either trace directory, the job skips the rest of that application's work for the pass, logs the application ID and error, and continues with the next application. It reports usage only for applications whose cleanup completed successfully. If the failure occurred partway through deletion, the audit log records the deletions that succeeded. The next pass scans the directory afresh.

### Trace quota visibility

In Full Cloud, the application owner and current Managers can see bytes used and the current effective limit for each of the application's two trace quotas. Show manual uploads and retained engine traces separately. For retained engine traces, show the most recent successful usage measurement for that application and its original timestamp. The deployment operator manages both limits.

Go keeps only the latest reported retained-trace byte count and measurement time per application in memory. After startup, Console shows the effective limit as soon as Go loads the configuration and displays “Awaiting first measurement” for usage until the cleanup job reports a measurement for that application.

### Look up retained traces

The Console proxy reads the [shared trace filesystem](roadmap.md#durable-execution-traces) for historical lookups, including after the producing engine has been replaced. Only application administrators and companions acting with their permissions can browse retained runs or look up, review, and download traces for that application.

**Open design question.** Review the existing Framework Console trace inventory and analysis services against the shared Console workflow before choosing how to index retained runs. One proposal is to scan finalized files on startup and build an in-memory index using the entry namespace, root skill, trace ID, start time, elapsed time, outcome, and file location. Index the trace under the initiating application even when child calls appear in it. Load the trace payload when an authorized caller opens it. A trace-completion notification could prompt a refresh, with periodic scans to discover missed files and reconcile index entries for files that have disappeared.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify administrator permission for the application before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

Verify that an authorized lookup identifies the correct retained trace when several callers invoke the same skill concurrently.
