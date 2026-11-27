# Go backend roadmap

## Go backend components

Full Cloud runs one Go backend deployment in V1. Its four internal components share the Console proxy's in-memory observations.

| Name | Responsibility |
| --- | --- |
| **Go backend** | The deployable service that contains the Cloud API, Console proxy, placement controller, and router. |
| **Cloud API** | Handles caller-facing requests and authorization, including administration of application settings and roles, uploads, validation, and publication requests. |
| **Console proxy** | Maintains the Framework Console connections and the observed state of engine pods, JVMs, namespaces, and publication versions. |
| **Placement controller** | Reconciles intended placements with the Console proxy's observations, publishes or restores applications, and confirms the intended publications are serving. |
| **Router** | Forwards an authorized invocation to the engine assigned to the application when that engine is ready, connected, and accepting requests. Framework handles [invocation cutover](java-execution-engine-roadmap.md#publication-and-cutover). |

The [execution engine API](execution-engine-api.md) defines the interface to Java.

When the Cloud API cannot read data required for an operation, it uses any fallback defined for that operation. If it still cannot answer, REST returns `503 Service Unavailable` and MCP reports the same condition. Go logs the underlying read error. REST uses `500 Internal Server Error` for unexpected internal failures.

## Go backend configuration

At first installation, the installer creates one YAML file in a dedicated configuration directory on the [shared volume](roadmap.md#durable-execution-traces). This file is the sole operator-edited source for Go backend settings. Go mounts the directory read-only. The engines and cleanup job have no mount for it.

The file has two top-level sections. Go reads `startup` when the backend starts and applies edits to it on the next restart. This section holds the trusted Supabase Auth issuer URL for [user access token verification](roadmap.md#authentication). Postgres startup settings include the host, port, database name, username, TLS settings, and path to the password file. A separate Kubernetes Secret supplies the password at that path. On cluster replacement, the operator supplies a valid password for the recovered database before starting Go with the restored YAML file.

At startup, Go validates both sections before it becomes ready to serve requests. If the file cannot be read or parsed, or an installation-wide setting fails validation, Go stays unready, logs the error with the offending setting when identifiable, and periodically rereads the mounted file.

Once installation-wide validation succeeds, Go applies the configuration and connects to Postgres. If Postgres is unavailable, Go stays unready and retries the connection with capped backoff. When Postgres becomes available, the same Go process continues startup without an operator restart.

Go runs pending Postgres migrations using Goose. Each backend release embeds its versioned SQL migration files in the Go binary. Go enables Goose's Postgres migration lock so that only one backend applies migrations at a time. Go starts serving once migrations succeed or Goose finds none pending. If a migration fails, Go logs the error and stays unready.

Verify that when Go starts with Postgres offline and Postgres subsequently comes online, the original Go process completes its migration check and becomes ready.

Handling a backend rollback after a newer release has migrated the database is outside V1 scope.

After startup, Go periodically rereads `reloadable` from the same file and validates it independently of `startup`. On detecting a valid edit, Go applies all reloadable settings at once. If Go cannot read or parse the file or an installation-wide reloadable setting fails validation, Go logs the error and keeps the last valid reloadable settings.

The operator sets the installation-wide interval for [subscription permission checks](#application-permissions) in `reloadable`. It defaults to 60 seconds and accepts a positive duration. Updating the interval applies to open and new subscriptions without reconnecting viewers.

At startup and on reload, if any part of a namespace's configuration fails to load or validate, Go logs the application ID and error, skips the entire namespace for quota processing, and continues with the other namespaces. Skipping the namespace protects its retained history from deletion under an unintended fallback quota. The namespace participates again once its configuration loads successfully.

The `reloadable` section holds three independent installation-wide quota defaults, each initially 100 MB (100,000,000 bytes) per application: [manual trace uploads](#manual-trace-upload-quota), [retained engine traces](#trace-cleanup-job), and [retained skill packs](#pack-history). The operator can override any of the three for an application using its application ID as the key. All defaults and overrides reload while Go is running. Each application inherits the current default for any quota it does not override. Removing an override restores that quota’s current default for the application.

An explicit namespace override of `0` sets that quota to zero bytes.

## Application creation

When a verified owner creates their first application, Go gets or creates a Postgres record mapping the owner's verified email to a database-generated numeric ID. Concurrent creation requests for the same verified email resolve to the same ID. Go uses that ID as the numeric part of the namespace prefix for each of the owner's applications. Go retains the mapping after the owner deletes their last application so future applications use the same prefix.

At creation, Go validates the complete `u{numericUserId}-{appSlug}` namespace against the [publication naming rules](publication-package-format.md#namespace-and-skill-names) and enforces a maximum of 61 ASCII characters, including the owner prefix and hyphen. This leaves room within the 64-character qualified skill name limit for the two-character separator and at least one character of local skill name. If the namespace exceeds 61 characters, Go returns `400 Bad Request` with the maximum slug length for that owner's prefix.

The first record Go creates for an application is its Postgres application row. It records the application ID, verified owner, slug, namespace, creation time, and lifecycle state.

Creating an application for a verified owner and slug returns the application ID and assigned namespace. If the application already exists for that owner and slug and is in the `Deleting` state, Go returns `409 Conflict` with the existing application ID and the `Deleting` state. Once cleanup releases the slug, the same request creates a fresh application with a new ID.

For any other existing application with that owner and slug, Go returns the same application ID and namespace, indicates that the application already existed, and preserves its settings, memberships, and publications. Concurrent requests for the same owner and slug resolve to one application. A companion that loses the creation response can repeat the request, use the returned namespace to name the pack's top-level directory, and upload the pack to the returned application ID. Different owners can use the same slug for separate applications.

New applications start with requests enabled. The owner or a Manager can [disable requests](#application-status).

## Application permissions

The Go backend uses one exported utility to normalize every email address. It trims leading and trailing whitespace, then lowercases the entire address. Go uses the normalized value when storing, looking up, or comparing addresses, including verified sign-in emails and addresses an owner enters before a user's first sign-in.

In V1, only the owner can delete an application. Go first stops routing new requests to run skills in the namespace. It then saves `Deleting` on the application's existing Postgres row before acknowledging deletion. This state keeps requests blocked, reserves the owner's slug, and prevents publication or recovery from reinstalling the application's skills. Go then uses the existing publication path to install an empty pack in the namespace. Deletion continues in the background if the owner disconnects. If a Go restart or pod replacement interrupts deletion after the state is saved, V1 leaves cleanup unfinished. The owner continues to see `Deleting`, and the row keeps the slug reserved.

While the namespace is in `Deleting`, its only permitted activity is draining previously admitted work and carrying out deletion. After cutover to the empty pack, Go waits for the Console proxy to confirm that all work admitted for the namespace has finished. If Console is unavailable at this step, deletion waits here until Console reconnects and confirms completion. Once the namespace's writers have finished, Cloud removes the application's files (including logs and traces) and dependent database records. If deletion fails, Go records the namespace and error in the operator log. The application stays in `Deleting` with its slug reserved until an operator intervenes. Deleting the application row is the final step and releases the slug for the owner to create a fresh application.

Each application has exactly one owner, identified by verified email. Ownership transfer is a future feature. In V1 only the owner can manage access. The owner can assign, change, or revoke a user's Cloud role for that application, define and assign skill roles, and authorize a calling backend to assert end-user identities and skill roles.

| Access | What it permits |
| --- | --- |
| Owner | All Manager permissions and exclusive control over access. |
| Manager | All Caller permissions. Upload, download, validate, promote, and publish packs. Manage application variables and runtime settings, restart the application, and inspect live events and retained traces. |
| Caller | Read the application's invocation documentation through the API, invoke published skills, and receive the result and any execution details enabled for that call. This role grants no access to management REST endpoints, Cloud UI screens, or Console for the application. |

Manager and Caller are the two assignable Cloud roles. References to application administrators mean the owner and Managers. Framework enforces the user's separate skill roles when executing skills, including for owners and Managers.

Each application owns its skill-role definitions and direct-user grants. Once the application exists, its owner can define a skill role for use by any skill in its namespace. A role with the same name in another application has independent grants and an independent lifecycle, even when both applications have the same owner.

A direct user must already be a member of the application before its owner can grant that user a skill role. The owner can add a user by email and then grant skill roles before the user's first sign-in.

Through REST or MCP, a signed-in user can list every application the user currently owns, manages, or holds the Caller role for. Each entry includes the application ID, namespace, the user's Cloud role, and the [invocation documentation](#invocation-documentation) URL.

When the owner removes a direct user from an application, Go clears that user's skill-role assignments for that application. If the owner later adds the user back, earlier assignments do not return. Changing an existing member's Cloud role, including from Manager to Caller, preserves their skill-role assignments.

When the owner deletes a skill role, Go deletes its definition and all of its direct-user assignments for that application in one Postgres transaction. If the owner later creates a role with the same name, it starts with no direct-user assignments.

Each operation declares its required application permission. The shared Cloud API checks the caller's current access when each ordinary request arrives. Once Go authorizes the request, it proceeds to completion under the operation's normal rules, even if the caller's access or roles change while it is in progress. Each subsequent request, including a status poll, lookup, or retry, receives a fresh access check.

After a successful access check, Go gives a signed-in user the same `404 Not Found` response when the application does not exist or the user has no access to it. If the user has access to the application but their Cloud role does not permit the requested action, Go returns `403 Forbidden`.

If Go cannot complete the application access check, it returns `503 Service Unavailable` before dispatching a skill or applying a management change. The response tells the caller that Go could not verify access to the application and that they can retry the request.

For open subscriptions, the shared subscription handler applies the same check at subscription time and repeats it at the [operator-configured interval](#go-backend-configuration). Delivery continues during the check. A successful check keeps the subscription open without interruption. If access ends or Go cannot verify it, the handler closes only the affected subscription.

### Invocation documentation

The Cloud API serves the full OpenAPI document at one stable, authenticated URL per application. It reads the [namespace skill catalog](#namespace-skill-catalog) from the Console proxy, identifies the catalog's publication version in the document, and includes every published invocation entry point with its description and schema.

One read-only MCP tool takes the application ID and returns the same OpenAPI document as the REST endpoint. Both call the same Go operation.

Before the application's first publication, an authorized documentation request returns `200 OK` with a valid OpenAPI document containing an empty `paths` object. Its description says the application has no published skills yet, and it omits the publication version. MCP returns the same document. This also applies when a pack has been staged.

Go forwards the authorized invocation to the engine. Framework runs it on the generation active in the namespace when Framework accepts the invocation. [Framework invocation cutover](java-execution-engine-roadmap.md#publication-and-cutover)

V1 exposes one MCP invocation tool for all published skills. The companion reads the application's invocation documentation to find the skill name and input schema, then calls the tool with the application ID, skill name, and arguments.

The same tool accepts an optional `files` array. For each local file, the companion client supplies its filename, content type, base64-encoded bytes, and destination as a JSON Pointer relative to the skill input (for example, `/invoice`). Go decodes each file into a binary multipart part with its filename and content type, names that part in the request metadata, and maps it to the supplied destination. Go passes the resulting request through the [existing multipart invocation path](execution-engine-api.md#invoke-requests) with its usual file mapping and validation rules. Ordinary arguments and all files travel in one authenticated tool call.

On every documentation request, Go checks the caller's current permission to invoke the target application. A user with that permission through the UI, REST, or MCP, or an integration presenting a valid [application API key](#application-api-keys) for that application receives the same full document regardless of skill roles. The key alone is sufficient to read the document. Framework enforces skill-role requirements when a skill runs.

### Companion sign-in

MCP uses the [same Cloud identity and browser sign-in flow as the web UI](roadmap.md#authentication). On each MCP request, Go verifies the companion's Supabase access token, including its expiry, and checks the signed-in user's current application permissions. Multiple companions can act for the same user at the same time. Companion-specific inactivity expiration and revocation controls are outside V1 scope.

### Application API keys

Only the owner can issue or revoke application API keys. Each third-party integration can have up to two active keys for REST invocation of one target application. Both keys identify the same backend. Go checks the presented key and its scope on every call.

Cloud shows the secret to the owner only when the key is created. Go stores only a verification hash of the secret. Each key has a visible key ID and creation date so the owner can distinguish the keys and select one for revocation.

To rotate a key, the owner issues a second key, deploys and tests it in the calling application, then revokes the old key when ready. Revocation takes effect on the next call using that key and affects only that key. If both slots are occupied, the owner must revoke one key before issuing another.

The integration authenticates its own users. On each invocation it supplies a stable end-user ID and the skill role names it asserts for that user. Go derives the issuer from the authenticated integration and uses the supplied end-user ID as the subject. Go forwards the role names as supplied, including unrecognized or misspelled names, without checking them against the application's defined roles. Framework checks the supplied roles against each skill's requirements at execution. A skill requiring a role permits the call only when its role requirement is satisfied. The asserted roles grant permissions only within skill execution. [Invocation identity and roles](execution-engine-api.md#invocation-identity-and-roles)

### Application variables

In V1 all saved application variable values are write-only through the Cloud API. The owner and Managers can list variable names, see which have a nonblank value configured, and set, replace, or remove a value. Console, REST, and MCP expose only names and configuration status when reading variable settings. Go reads the required values internally for publication, restart, and recovery.

If authorized administrators edit the same variable, the last saved edit takes effect. The Cloud API accepts the edit even if another administrator has changed the variable since the caller listed it.

### Encryption at rest

Go encrypts saved application variable values before writing them to Postgres. Each Cloud environment supplies its own variable encryption key to Go through a Kubernetes Secret. At installation, the operator saves a recovery copy in an operator-only directory on the shared volume. Restrict the directory's filesystem permissions and mount only the required data directories into Cloud workloads so the recovery directory stays outside their mounts. Include the directory in the protected off-cluster volume backup.

On a replacement cluster, the operator first [restores access to the shared volume](roadmap.md#durable-execution-traces), retrieves the saved variable encryption key, and recreates Go's Kubernetes Secret with the same key before starting Go against the recovered Postgres data.

Go writes original pack ZIP bytes to the [shared volume](roadmap.md#durable-execution-traces), computes SHA-256 over those bytes, and returns the same bytes on authorized download. The storage provider handles file encryption transparently.

## Upload and staged validation

Upload stages the pack and returns advisory validation feedback. Publish is a separate action that automatically runs fresh validation on the selected staged revision before installing it. The caller can select Publish directly, including when validation was unavailable at upload time.

### ZIP checks before staging

An MCP companion uploads a local pack in one authenticated `upload_pack(applicationId, zipBase64)` call. The companion client reads the ZIP and base64-encodes its bytes. Go checks the signed-in user's upload permission for the application, decodes the ZIP, and passes the original bytes to the shared upload and staging operation. The tool returns that operation's outcome, including the revision ID and validation feedback for a newly staged pack.

Go authorizes the upload and checks the entire archive against the [package limits](publication-package-format.md#archive-rules). It caps the incoming ZIP bytes, counts every entry including explicit directories, and counts actual expanded bytes while reading every file, including companion assets. It rejects nonregular files, entry types other than files and directories, ambiguous paths, and paths that collide without regard to case. Report the offending path, or both paths for a collision.

Go requires one top-level directory matching the destination namespace, the [companion entry point](publication-package-format.md#required-companion-entry-point), and the [execution subtree layout](publication-package-format.md#execution-inputs), including lowercase skill paths. In `loomspan/execution.yaml`, reject reserved administrative sections and any executor configuration owner key that differs from the destination namespace. Report the offending field and its pack-relative source. A pack that passes these checks can be staged while the engine is offline.

### Staging revisions and validation

Through MCP, an owner or Manager can call `validate_pack(applicationId, stagedRevisionId)` to rerun the shared validation operation and get its feedback for that revision.

The Cloud API computes SHA-256 over the exact ZIP bytes and compares the digest with revisions in the target application. A match with the current revision returns it and its validation results. A match only with an older revision returns a warning naming the matched and current revisions. Confirmation applies to that pair. If staging changes before confirmation, the Cloud API reevaluates the upload and refreshes the warning if needed.

A confirmed restaging creates a new current revision. Go reuses the stored ZIP if it is still present or saves the submitted bytes again if pack cleanup removed it. Validation results stay with their original revision. Previously unseen ZIP bytes create a new revision after upload checks pass. The Cloud API makes the comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Each new staging revision records and shows who staged it and when, including revisions created by promotion. An identical upload that returns the current revision preserves its original actor and time.

Whenever staging creates a revision, Go automatically validates it and returns validation feedback to the caller as part of staging. An authorized caller can also rerun validation for a specific staged revision through Console or MCP. Go runs its Cloud checks and requests engine validation of the authored pack even when application variables are missing or blank. Once validation completes, Go returns the engine diagnostics, all missing or blank variable references (with pack-relative source and authored YAML path), and any other pack issues found by Cloud in one result. A completed result can report an invalid pack. The result belongs to the requested revision even if another upload arrives. The pack stays staged and the current publication keeps serving.

If validation cannot complete, Go reports that the result is unavailable and tells the caller to retry validation. Go withholds partial findings from the caller.

If the pack is otherwise valid, report that it passed validation and identify the variables blocking publication. Once the owner supplies them, they can publish the same revision directly. Publication checks the current values.

For each staged revision, the Cloud API keeps the last completed validation result and its timestamp visible, including feedback from automatic validation on upload. The result describes the revision and settings used at that time. Later edits leave the result and timestamp intact. V1 has no stale validation state. Publish always runs fresh validation before installing the revision.

For a Publish request, Go fixes the selected pack revision, reads the required variable values and effective runtime settings, and holds them in memory. Go uses those values for its checks and sends the same values and settings with that revision for installation. The engine checks the combination it actually installs. V1 leaves the timing of edits that overlap those reads unspecified and requires no configuration version history or edit lock. Users save settings before selecting Publish. If they edit settings during Publish, they can select Restart afterward to apply the latest saved values. Console shows the confirmed active settings separately from subsequent edits. [Administrative runtime settings](java-execution-engine-roadmap.md#administrative-runtime-settings)

Go reads the application's active trace persistence policy from `tracePersistencePolicy` on the matching namespace entry in a fresh authenticated instance snapshot. Console compares that observed policy with the owner's saved selection. A fresh snapshot supplies the active policy after a Go restart or a lost publish response. To confirm that a particular restart completed, Go still applies the [JVM and generation checks](#restarting-an-application).

### Publishing a reviewed revision

The MCP `publish_pack` tool takes the application ID and staged revision ID. It follows the same reviewed-revision confirmation and fresh-validation rules as Console. Once Go accepts the publication, the call waits for confirmation. On success it returns the confirmed publication version and stable request ID. On a known failure it returns the diagnostic. If Go loses the engine reply after dispatch and the outcome is still unconfirmed at the end of the [30-second confirmation window](#unconfirmed-publications), the call returns the stable request ID and reports that the publication may still complete.

When Publish proceeds with a new installation, Go validates the selected revision against the application's current configuration. If validation passes and the required variables are configured, Go sends that same revision to the engine's publish endpoint. If validation fails or cannot complete, Go reports the outcome, keeps the revision staged, and leaves the current publication serving.

Before each install, Go checks the selected ZIP’s [variable references](publication-package-format.md#application-variables) and reports any missing or blank values with their pack-relative source and authored YAML path.

When starting a publication, restart, or recovery, Go reads the destination application’s current values for those names, computes its effective runtime settings from saved selections and Cloud defaults, and uses the [publish endpoint](execution-engine-api.md#internal-api-requests). The ZIP is the durable record of its requirements.

The caller names the staged revision to publish. If it differs from the application's current staged revision, the Cloud API returns a warning identifying both revisions. After the user confirms, Console and MCP repeat the same publish operation with the reviewed revision ID and the current revision ID shown in the warning. Go accepts that confirmation only while the warned-about revision is still current. If staging has changed again, Go returns an updated warning and waits for fresh confirmation. An accepted confirmation authorizes publication of the reviewed revision while its ZIP is retained. If cleanup has removed the ZIP, Go tells the caller to upload it again. The newer revision stays current in staging for continued work.

If the owner selects **Publish** for the staged revision that is already the application's current publication, the Cloud API reports “This revision is already published.” It returns without creating a publication version or dispatching an installation. To apply saved variables or runtime settings to that revision, the owner selects [Restart application](#restarting-an-application).

Go can publish as soon as the Console proxy reads the first authenticated instance snapshot from the intended engine JVM. If the proxy lacks a snapshot from the current connection, the Cloud API reports the publish attempt as failed. The revision stays staged, the current publication pointer stays in place, and the owner can retry.

On receiving `204 No Content` from Java, Go durably records the acknowledged version as the application's current published version before reporting publish success to the caller. If the pod dies before Console observes that version, the placement controller restores it on the replacement.

### Pack history

An owner or current Manager can call the read-only MCP tool `list_packs(applicationId, includeValidation=false)` to find the application's retained, downloadable packs. Each staged entry gives the staged revision ID, who staged it, and when. Each published entry gives the publication version, the user who requested the successful publication, and the time Go first confirmed that version as current. A restart preserves the original publisher and publication time. The list marks the current staged revision and the current publication independently and returns the selector each entry needs for `download_pack`.

By default, staged entries omit the `validation` field. With `includeValidation=true`, each staged entry includes its last completed validation result, diagnostics, and timestamp, or `validation: null` if no check has completed. The listing reads saved data and works while the engine is offline. To run a fresh check, call `validate_pack`.

An MCP companion calls `download_pack` with the application ID and exactly one selector: a staged revision ID or a publication version. Go checks that the signed-in user is the application's owner or a current Manager and reads the retained ZIP for that selection through the shared download operation. The tool returns the original ZIP bytes encoded as base64 and the SHA-256 digest of the original bytes. The companion client decodes the ZIP to a local file.

The backend pack-transfer ticket requires an automated integration test in CI. Using one authenticated test-user session, an MCP client reads a valid 1,000,000-byte ZIP from disk, calls `upload_pack`, downloads the returned staged revision through `download_pack`, writes the decoded bytes to a second file, and compares the two files byte for byte. When first integrating the intended companion, verify the same local-file workflow after one sign-in without copying a token or pasting the archive into chat. Automate that compatibility check if the companion supports it; otherwise perform it once manually.

The owner and current Managers can select a retained publication by version and download its original ZIP. To review a change, they can download two retained versions and compare them locally. The [pack-history quota](#go-backend-configuration) sets a byte allowance for retaining pack history.

V1 handles rollback through the ordinary publishing workflow. An owner or Manager can download an older version’s retained ZIP, stage it, and publish it again.

Go timestamps every accepted pack upload. Each stored ZIP has an upload timestamp on the filesystem. For each application, count the bytes of every eligible ZIP on disk, including superseded staged packs that were never published. Multiple revisions referencing the same file count once. A ZIP needed for the current publication, the current staged revision, or an outstanding publish, restart, or retry stays outside the quota and is protected from trimming. The [cleanup job](#trace-cleanup-job) applies these protections at deletion time. Even with a zero-byte quota, cleanup must preserve the currently published ZIP for download and recovery.

Cleanup scans the application's pack files. When eligible ZIP bytes exceed the quota, it removes eligible files in upload timestamp order, oldest first, until usage is at or below the limit.

Pack cleanup leaves the [application operations log](#application-operations-log) intact. Owners and Managers can still read every logged entry and download any retained ZIP by publication version.

### Application status

The Cloud API exposes two separate application status fields to the owner, current Managers, and companions acting with their permissions. Console and MCP use the same fields.

- **Serving status** reports whether the router can forward a new invocation for this application now. If the engine disconnects, the status becomes “Temporarily unavailable.”
- **Latest publish or restart status** identifies the most recently accepted request, its operation (publish or restart), its target version, and its current progress or outcome under the publication and restart confirmation rules below.

The owner or a Manager can select **Disable requests** or **Enable requests** for an application. While disabled, serving status shows **Requests disabled** and the Go router rejects new requests to run skills before dispatch. Calls already running finish normally. Administrators can still inspect the application and publish a fix. The switch leaves the Java engine and its installed publication unchanged. Enabling requests restores normal routing eligibility checks.

Requests stay disabled across publishes, application restarts, and Go backend restarts until the owner or a Manager explicitly selects **Enable requests**. An authorized HTTP caller whose request is rejected because requests are disabled receives `503 Service Unavailable` before execution. Go omits `Retry-After` because there is no scheduled time for requests to resume.

For example, Console can show “Serving” while a publication awaits confirmation. A later engine disconnect changes serving status independently of the recorded publication outcome.

Alongside the latest request status, Console shows the time and type of the application's last confirmed publish or restart. A confirmed restart advances this time even when the publication version stays the same.

When Go accepts a publish or restart, it generates and returns a stable request ID. If the caller loses the response before receiving the ID, the [ordinary Console reload](roadmap.md#console-navigation-and-modules) shows the latest operation and last confirmation time. If concurrent restarts leave the caller unsure which request was theirs, the user decides whether to try again. The owner, current Managers, and their authorized companions can look up that request's progress or outcome by application and request ID, including after a newer request becomes the latest. The request identifies its target publication version. Two restarts of the same publication have distinct request IDs and refer to the same published pack.

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
3. **Import.** Pass the resulting ZIP to the ordinary destination upload and staging operation. It applies the [package checks](#zip-checks-before-staging), including the [combined namespace and skill name length limit](publication-package-format.md#namespace-and-skill-names), and the usual [digest comparison and restaging rules](#staging-revisions-and-validation). The caller can review the transformed pack in the destination.

The destination uses its own variables and runtime settings. Its current publication keeps serving until an authorized user publishes the staged revision. Framework sees only ordinary validate and publish requests for the destination namespace.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, the Cloud API could generate a client-only skill pack for that user's companion. It would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. The Cloud API would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response visibility

- [x] Verified the Framework invocation session ID enhancement. Both `SkillTemplate.invoke(...)` and `AdmittedSkillInvocation.invoke()` return a `SkillInvocationResult` on success. For example, after `var result = skillTemplate.invoke("support", "summarize", input);`, the Java wrapper can read the original result text from `result.text()` and the canonical root session ID from `result.sessionId()`.

On failure, `SkillException.getSessionId()` returns the root ID if a session was created and `null` for a pre-session failure. A post-session `SkillAccessDeniedException` also exposes the ID. The ID is available with execution views off, when trace retention discards the trace, and when constructing a requested view fails. It matches the ID in any delivered view or retained trace.

When the engine supplies a session ID, Cloud returns it to the caller. A caller can give that ID to an application owner or Manager, who can use it to [look up the corresponding retained trace](#look-up-retained-traces) within that application. This is the agreed V1 workflow. The Cloud response and lookup still need implementation. In the Framework repository, see `ai/thoughts/tickets/invocation-session-id.md` for direct and admitted caller examples and `ai/thoughts/verification/2026-11-25-invocation-session-id.md` for test evidence.

Go enforces both V1 invocation limits once, after the REST and MCP endpoints converge on the shared invocation path and before dispatch to Java. UI invocations use the same path. Measure the common JSON or multipart invocation body, up to 20 MiB (20,971,520 bytes) in total and up to 25 uploaded files, both inclusive. For MCP file calls, measurement happens after Go decodes the base64 file data and constructs the multipart body. Count the entire common body, including metadata, decoded file bytes, part headers, and boundaries. Each uploaded file part counts once, including an empty file. The JSON metadata part counts only toward the byte limit. If either limit is exceeded, Go rejects the request before dispatch.

For a rejection by the shared invocation limit check, REST returns `413 Content Too Large` and MCP returns a tool error with the same message. The message identifies the exceeded limit (20 MiB total or 25 files) and tells the caller that the skill did not start.

The Cloud API applies the same V1 rule to REST and MCP. With permission to invoke the application, callers receive the exact `result` on success or the engine’s top-level `message` verbatim and any top-level input-validation `issues` on failure. Caller-friendly error categories and rewritten messages are deferred to V2. [Engine response fields](execution-engine-api.md#invoke-responses)

One application administrator setting controls `executionView`, `debug`, and `diagnosis` in REST and MCP invoke responses. It defaults to off for new applications. When enabled, Go automatically returns the execution view for the invoker's own call on success or failure and includes the available `debug` and `diagnosis` fields on failure. When disabled, Go omits all three fields even if the invoker asks for them. The switch applies independently of trace persistence.

## Invocation response loss

If the caller disconnects after execution starts, the engine continues the invocation under its normal runtime limits. V1 has no mechanism to cancel a running invocation.

The router removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. The router does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

### Application invocation log

The invocation service owns the application's log of incoming API invocations. Each entry records who made the request, when it arrived, the requested skill entry point, and the incoming parameters. The application's administration section makes this log available to its owner and current Managers. These fields support later manual comparison with traces, including when a caller loses a response. V1 records and displays the invocation log independently of Framework trace capture and lookup.

If Go rejects a request because the application has requests disabled, it returns before logging and writes no per-request entry to any Go backend log.

Log each authorized invocation request that passes Cloud's request checks, including when no eligible engine is available. The entry identifies whether Go dispatched the request to an engine or rejected it before dispatch. Requests that fail authentication or application authorization are excluded from the application-visible log.

For a direct call, record the verified Cloud user. For a companion call, record the verified Cloud user and the companion connection ID. For an application API key call, record the authenticated integration ID together with the end-user ID asserted by that integration.

For ordinary parameters, the entry records their submitted values. For each uploaded file, it records only the parameter name, supplied filename, content type, and byte count. File contents are excluded.

Within the Go invocation service, one invocation log component owns all reads and writes of one JSON Lines file per namespace on the [shared volume](roadmap.md#durable-execution-traces). Its synchronous append operation encodes the record, acquires an exclusive filesystem lock on the namespace file, writes the complete line, and releases the lock before the skill runs. Every Go writer follows this protocol, including during pod replacement. The component tries the lock immediately. If another writer holds it, the component retries for up to one second from the first attempt. This limit applies to lock contention. If the wait expires or lock acquisition otherwise fails, the component reports an append failure without writing. Appends for different namespaces proceed independently. While holding the lock and before appending the record, the writer checks the file's last byte. If a nonempty file lacks a trailing newline, the writer inserts one to separate the previous bytes from the new record. It preserves the existing bytes.

After the Cloud API checks access, the administration view reads the namespace log through the component. The reader processes each newline-terminated record independently, skips malformed records, and continues displaying valid entries that follow. It ignores an unterminated tail for the current read because a writer may still be appending it. Reading leaves the file untouched.

If a namespace log append fails, the invocation service reports the affected namespace and the error in the main Cloud operational logs. An otherwise authorized invocation proceeds if it can run.

### Application operations log

Log a promotion in the destination as an ordinary app push. For a push from another namespace, the same entry can note “Promoted from another namespace.” The destination does not retain a reference to the source application or publication.

Go keeps a separate operations log for each application on the same shared-volume mount as the [invocation log](#application-invocation-log). For a publish or restart, append an entry at acceptance with the actor, time, operation, stable request ID, and target publication version. When the outcome becomes known, append it with the same request ID so the history shows both acceptance and outcome.

For each application access change, record when it took effect, who made it, the affected user, role, or integration, and what changed. For a Cloud role change, record the previous and new roles. Include skill-role grants and revocations.

When the request setting changes, append one entry recording who enabled or disabled requests, which action took effect, and when. Selecting the current setting produces no entry.

When an owner or Manager sets, replaces, or removes a saved application variable, append one entry with the actor, time, variable name, and action. Omit both the old and new values.

Operations-log writes are best effort. If an append fails, Go continues the otherwise authorized operation and attempts to report the affected namespace, operation, and append error in the operator logs. The caller receives the operation's actual outcome.

## Console proxy

### Observe live engines

The Console proxy maintains its in-memory observations using the [pod and JVM identity contract](execution-engine-api.md#pod-and-jvm-identity). After an install succeeds or its response is lost, the placement controller asks the Console proxy to refresh that engine's snapshot. On disconnect, the Console proxy keeps the last observation available for inspection and marks it disconnected. The placement controller uses fresh observations from connected instances to confirm placements.

On Go backend startup or Console reconnection, the placement controller compares each intended publication with the fresh snapshot for its assigned engine. If the namespace and publication version match, the existing generation keeps its loaded variables and settings. The controller installs missing or out-of-date publications and confirms each installation through the Console proxy.

Recovery proceeds per namespace. If one restore fails, the Cloud API reports that application's failure while the placement controller continues restoring the remaining namespaces.

After a failed restore, the owner can select [Restart application](#restarting-an-application) to install the current publication with the latest saved variables and settings.

In V1 the router sends work only to intended placements. If the Console proxy observes an additional namespace, the placement controller leaves it loaded and the router excludes it from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The Console proxy consumes streams from engines that can host several applications. Only application administrators and companions acting with their permissions can inspect live system events. Each viewer's subscription follows the [shared application permission rules](#application-permissions), and the proxy scopes events to the authorized namespace. Closing one viewer's subscription leaves the shared engine connection available to other viewers.

When the proxy loses the live event stream, Cloud tells the owner or Manager viewing each affected application “Live events temporarily unavailable.” When the proxy can deliver live events again, Cloud reports “Live events resumed.” Both notices describe event visibility for that application. Engine-wide diagnostic details stay in the platform logs.

On reconnect, the proxy resumes from the current point. The live timeline keeps the gap between the two notices visible and skips replay of events from that interval. If an affected run has a retained trace, the owner or Manager can inspect it in the application's run history.

### Namespace skill catalog

The Full Cloud Console proxy owns catalog retrieval, freshness, and storage. It uses the authenticated engine snapshot to identify the application's serving namespace and publication. Staging or validating a revision does not change the catalog. Through the Console connection, one namespace-scoped read returns the complete catalog from one active generation: skill names, descriptions, input schemas, output schemas where available, and effective entry-point access policies. The response identifies the engine instance, generation, and publication it actually describes.

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

At save time, Go checks the upload or overwrite against the active quota. When the quota check passes, the upload proceeds through the normal import workflow. Console omits quota previews and quota-specific success messages. Users can review their usage on the [quota screen](#quota-visibility).

The caller chooses how long to keep a manual upload, so lowering the quota preserves existing uploads and their expiry dates. Owners and Managers can still inspect, download, or delete them. For an overwrite, count the incoming file size minus the size of the upload it replaces. If the application is already over quota, allow changes that leave usage unchanged or reduce it. Reject increases that would put or keep the application over quota, and preserve the existing upload on rejection.

When Go rejects a manual upload or overwrite for exceeding quota, report the destination application's current manual-upload usage, its effective limit, and how many bytes the proposed usage would exceed that limit by. Use the values from the final quota check.

### Trace cleanup job

One Full Cloud cleanup job runs every five minutes with write access to the manual-upload, engine-trace, and pack directories. For each application, the same pass expires manual uploads and trims retained engine traces and [eligible retained pack ZIPs](#pack-history) to their respective byte quotas. Only one pass can run at a time. If a pass is still running at a scheduled start, the job skips that start, lets the current pass finish, and resumes at the next scheduled start.

At the start of each pass, the job fetches a snapshot from Go with the active retained-trace and pack-history defaults, accepted overrides, and application IDs skipped during configuration loading. It uses that snapshot for the entire pass and skips those applications for all cleanup steps. If Go is unready or the job cannot fetch the complete snapshot, the job skips the pass and tries again at the next scheduled pass. A quota edit first applies on the next pass after it becomes active.

A zero-byte quota removes all eligible retained traces on the next pass. For each application over its retained-engine-trace quota, the job deletes eligible retained traces in order of completion, oldest first, until usage is at or below the quota or no eligible traces remain. The job skips active writes and files still in Console’s completion grace period. The quota is soft. Usage can exceed it between passes. Monitor free space on the shared volume. Time-based expiry for engine traces and cleanup of abandoned writes remain outside V1. Measure trace count, bytes per application, and growth over time to inform future quota values and whether allowances should vary by pricing tier.

Each cleanup pass is stateless. After an interruption, the next pass sees the files that remain on disk and can resume cleanup without a checkpoint. On completion it sends Go separate post-cleanup byte counts for retained engine traces and [eligible retained pack ZIPs](#pack-history), with the measurement time for each count. For each application where the job trims retained engine traces, it writes a structured audit log with the application ID, quota applied, number of traces removed, and bytes freed. These logs serve only as an audit record.

If any cleanup step fails for an application, including pack trimming, the job logs the application ID and error in the operator logs, skips all remaining work for that namespace in the pass, and continues with the next application. It reports usage only for applications whose cleanup completed successfully. If the failure occurred partway through deletion, the audit log records the deletions that succeeded. The next pass scans the directory afresh.

### Quota visibility

In Full Cloud, the application owner and current Managers see three rows on the same quota screen: manual trace uploads, retained engine traces, and pack history. Each row shows bytes used and the current effective limit for that application. The deployment operator manages all three limits.

Go measures manual uploads when the screen is read. For retained engine traces and pack history, Go keeps the latest successful byte count and its original measurement time per application in memory. Console shows that timestamp alongside usage. After startup, Console shows the effective limits as soon as Go loads the configuration and displays “Awaiting first measurement” in each cleanup-measured row until its first report arrives.

### Look up retained traces

The Console proxy reads the [shared trace filesystem](roadmap.md#durable-execution-traces) for historical lookups, including after the producing engine has been replaced. Only application administrators and companions acting with their permissions can browse retained runs or look up, review, and download traces for that application.

Finalized engine trace files on the shared volume are the source of truth for run history. On Go startup, the Console proxy scans them in the background to rebuild an in-memory index. Console shows that history is still loading while the initial scan is incomplete. Periodic scans discover newly retained files and remove entries for files that have disappeared.

When the Console proxy receives a run-completion event, it checks that run’s finalized file and adds the run to history as soon as the file passes the retention checks below. This check reads only the affected run. If the event arrives before finalization or the proxy misses it during a disconnect, a subsequent periodic scan discovers the eligible file.

Each entry records the entry namespace, trace ID, root skill, start time, elapsed time, outcome, and file location. A run appears under the initiating application even when it contains child calls. The history list uses these summaries. When an authorized caller opens a run, Console loads the selected trace for detailed analysis. Manually uploaded traces remain in the application's separate uploads view.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify administrator permission for the application before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

Verify that an authorized lookup identifies the correct retained trace when several callers invoke the same skill concurrently.
