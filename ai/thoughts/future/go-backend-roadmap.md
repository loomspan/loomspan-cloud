# Go backend roadmap

## Go backend components

The [Phase 1 Cloud REST API](phase-1-rest-api.md) specifies public routes, request bodies, statuses and response fields for the first application workflow.

Full Cloud runs one Go backend deployment in V1. Its four components run in one process, call each other through Go functions or methods, and share the Console proxy's in-memory observations. REST, MCP, and web UI handlers call the same authorized Cloud API operations. The cleanup job and the Java engine communicate with Go across process boundaries.

| Name | Responsibility |
| --- | --- |
| **Go backend** | The deployable service that contains the Cloud API, Console proxy, placement controller, and router. |
| **Cloud API** | Handles caller-facing requests and authorization, including administration of application settings and roles, uploads, validation, and publication requests. |
| **Console proxy** | Maintains the Framework Console connections and the observed state of engine pods, JVMs, namespaces, and publication versions. |
| **Placement controller** | Reconciles intended placements with the Console proxy's observations, publishes or restores applications, and confirms the intended publications are serving. |
| **Router** | Uses the Console proxy’s live view to forward authorized invocations to an [eligible assigned engine hosting the application’s namespace](roadmap.md#starting-point-for-placement-and-routing). Framework handles [invocation cutover](java-execution-engine-roadmap.md#publication-and-cutover). |

The [execution engine API](execution-engine-api.md) defines the interface to Java.

When the Cloud API cannot read data required for an operation, it uses any fallback defined for that operation. If it still cannot answer, REST returns `503 Service Unavailable` and MCP reports the same condition. Go logs the underlying read error. REST uses `500 Internal Server Error` for unexpected internal failures.

## Go backend configuration

At first installation, the installer creates one YAML file in a dedicated configuration directory on the [shared volume](roadmap.md#durable-execution-traces). This file is the sole operator-edited source for Go backend settings. Go mounts the directory read-only. The engines and cleanup job have no mount for it.

The file has two top-level sections. Go reads `startup` when the backend starts and applies edits to it on the next restart. This section holds the trusted Supabase Auth issuer URL for [user access token verification](roadmap.md#authentication). Postgres startup settings include the host, port, database name, username, TLS settings, and path to the password file. A separate Kubernetes Secret supplies the password at that path. On cluster replacement, the operator supplies a valid password for the recovered database before starting Go with the restored YAML file.

At startup, Go validates both sections before it becomes ready to serve requests. If the file cannot be read or parsed, or an installation-wide setting fails validation, Go stays unready, logs the error with the offending setting when identifiable, and periodically rereads the mounted file.

Once installation-wide validation succeeds, Go connects using the native PostgreSQL protocol. Go uses this connection configuration for migrations and all runtime reads, writes, and transactions. With hosted Supabase, the operator can supply the direct database endpoint or the session pooler endpoint. If Postgres is unavailable, Go stays unready and retries the connection with capped backoff. When Postgres becomes available, the same Go process continues startup without an operator restart.

Go runs pending Postgres migrations using Goose. Each backend release embeds its versioned SQL migration files in the Go binary. Go enables Goose's Postgres migration lock so that only one backend applies migrations at a time. Go starts serving once migrations succeed or Goose finds none pending. If a migration fails, Go logs the error and stays unready.

Verify that when Go starts with Postgres offline and Postgres subsequently comes online, the original Go process completes its migration check and becomes ready.

Handling a backend rollback after a newer release has migrated the database is outside V1 scope.

After startup, Go periodically rereads `reloadable` from the same file and validates it independently of `startup`. On detecting a valid edit, Go applies all reloadable settings at once. If Go cannot read or parse the file or an installation-wide reloadable setting fails validation, Go logs the error and keeps the last valid reloadable settings.

The operator sets the installation-wide interval for [subscription permission checks](#application-permissions) in `reloadable`. It defaults to 60 seconds and accepts a positive duration. Updating the interval applies to open and new subscriptions without reconnecting viewers.

At startup and on reload, if any part of a namespace's configuration fails to load or validate, Go logs the application ID and error, skips the entire namespace for quota processing, and continues with the other namespaces. Skipping the namespace protects its retained history from deletion under an unintended fallback quota. The namespace participates again once its configuration loads successfully.

While a namespace is skipped, [manual uploads and overwrites](#manual-trace-upload-quota) and [quota-screen reads](#quota-visibility) follow the [Cloud API rule for unavailable required data](#go-backend-components). REST returns `503 Service Unavailable`, MCP reports the same condition, and an upload or overwrite leaves the stored files untouched.

The `reloadable` section holds three independent installation-wide quota defaults, each initially 100 MB (100,000,000 bytes) per application: [manual trace uploads](#manual-trace-upload-quota), [retained engine traces](#trace-cleanup-job), and [retained skill packs](#pack-history). The operator can override any of the three for an application using its application ID as the key. All defaults and overrides reload while Go is running. Each application inherits the current default for any quota it does not override. Removing an override restores that quota’s current default for the application.

At startup and on reload, Go validates each override’s application ID syntax and quota values locally, without a database lookup. Go accepts a valid override for a well-formed ID even when the application does not yet exist. The override takes effect when a matching application exists. A malformed entry follows the per-application error handling above.

An explicit namespace override of `0` sets that quota to zero bytes.

## Application creation

When a verified owner creates their first application, Go gets or creates a Postgres record mapping the owner's verified email to a database-generated numeric ID. Concurrent creation requests for the same verified email resolve to the same ID. Go uses that ID as the numeric part of the namespace prefix for each of the owner's applications. Go retains the mapping after the owner deletes their last application so future applications use the same prefix.

At creation, Go validates the complete `u{numericUserId}-{appSlug}` namespace against the [publication naming rules](publication-package-format.md#namespace-and-skill-names) and enforces a maximum of 61 ASCII characters, including the owner prefix and hyphen. This leaves room within the 64-character qualified skill name limit for the two-character separator and at least one character of local skill name. If the namespace exceeds 61 characters, Go returns `400 Bad Request` with the maximum slug length for that owner's prefix.

The first record Go creates for an application is its Postgres application row. It records the application ID, verified owner, slug, namespace, creation time, and lifecycle state.

Creating an application for a verified owner and slug returns the application ID and assigned namespace. If the application already exists for that owner and slug and is in the `Deleting` state, Go returns `409 Conflict` with the existing application ID and the `Deleting` state. Once cleanup releases the slug, the same request creates a fresh application with a new ID.

For any other existing application with that owner and slug, Go returns the same application ID and namespace, indicates that the application already existed, and preserves its settings, memberships, and publications. Concurrent requests for the same owner and slug resolve to one application. A companion that loses the creation response can repeat the request, use the returned namespace to name the pack's top-level directory, and upload the pack to the returned application ID. Different owners can use the same slug for separate applications.

New applications start with requests enabled. A user with Manager access can [disable requests](#application-status).

## Application permissions

The Go backend uses one exported utility to normalize every email address. It trims leading and trailing whitespace, then lowercases the entire address. Go uses the normalized value when storing, looking up, or comparing addresses, including verified sign-in emails and addresses an owner enters before a user's first sign-in.

In V1, only the owner can delete an application. Go accepts Delete only when the application is idle for management changes. If an upload, publish, restart, or other application management write has been admitted and is still pending or in progress, Delete immediately returns `409 Conflict`. The response tells the owner to retry later. Go leaves the application and the admitted operation unchanged. A dispatched installation with an unknown outcome remains a conflict while it could still take effect, including after the caller's confirmation window expires. The [existing reconciliation rules](#unconfirmed-publications) apply. Running skill invocations do not prevent Go from accepting Delete.

For each application, admitting a management write and checking for conflicting writes and entering `Deleting` are mutually exclusive. Once Delete wins admission, Go stops routing new skill requests, saves `Deleting` on the application's existing Postgres row before acknowledging deletion, and rejects subsequent management writes. This state reserves the owner's slug and prevents publication or recovery from reinstalling the application's skills. Go then uses the existing publication path to install an empty pack in the namespace. Deletion continues in the background if the owner disconnects. If a Go backend restart interrupts deletion after the state is saved, V1 leaves cleanup unfinished. The owner continues to see `Deleting`, and the row keeps the slug reserved.

While the namespace is in `Deleting`, its only permitted activity is draining previously admitted work and carrying out deletion. After publishing the empty pack, Go polls the Console proxy for a [fresh namespace drain observation](execution-engine-api.md#namespace-drain-observation). Go can begin cleanup when one authenticated snapshot from the JVM that installed the empty pack identifies that deletion’s namespace and empty-pack publication and reports `priorGenerationsDrained: true`. On Console reconnection to the same JVM, Go obtains a fresh snapshot. Go may also begin cleanup if Kubernetes confirms that the engine process that could write for this application has terminated. This applies both to a JVM restart in the same pod and to pod replacement. A Console disconnect, a terminating pod, or discovery of a replacement alone leaves termination unconfirmed. Go waits until it confirms either drain or termination.

The existing Console active-executions list shows visible runs by entry namespace. An invocation can already be admitted while still absent from that list. Repeated empty reads and elapsed time therefore cannot establish drain completion. The polling interval controls only when Go checks again. Framework owns the drain signal; Go consumes it through Console.

Once Go confirms drain or engine termination, Go directly removes the application's engine-trace directory as part of the deletion routine, along with its other files and dependent database records. An already absent trace directory satisfies this step. Deletion proceeds independently of the five-minute retention pass and of the application's quota configuration. When termination allows cleanup to proceed, Go records in the operator log that the engine exited before drain was confirmed and that in-flight work may have been interrupted. Go records completion there once deletion succeeds. If deletion fails, Go records the namespace and error in the operator log. The application stays in `Deleting` with its slug reserved until an operator intervenes. Deleting the application row is the final step and releases the slug for the owner to create a fresh application.

Verify both termination paths during an active deletion. Hold a trace write open and disconnect Console. Verify Go preserves the files while termination is unconfirmed. Exercise a JVM restart in the same pod and a pod replacement. In each case verify that Go completes cleanup after Kubernetes confirms the old process exited, records the outcome in the operator log, and releases the slug only after cleanup succeeds.

Each application has exactly one owner, identified by verified email. Ownership transfer is a future feature. In V1 only the owner can manage access. The owner can assign, change, or revoke a user's Cloud role for that application, define and assign skill roles, and authorize a calling backend to assert end-user identities and skill roles.

Cloud permissions are inherited within each application. A Manager has every Caller permission, and the owner has every Manager permission. Each operation below names its minimum Cloud role. Companions act with the signed-in user’s access.

| Minimum Cloud role | Operations |
| --- | --- |
| Owner | Delete the application, manage access, and issue or revoke application API keys. |
| Manager | Upload, download, validate, promote, and publish packs. Manage application variables and runtime settings, restart the application, and inspect live events and retained traces. |
| Caller | Read the application's invocation documentation through the API, invoke published skills, and receive the result and any execution details enabled for that call. A user whose only Cloud role is Caller has no access to management REST endpoints, Cloud UI screens, or Console for the application. |

Manager and Caller are the two assignable Cloud roles. An application administrator is a user with Manager access. Framework enforces the user's separate skill roles when executing skills, including for owners and Managers.

Each application owns its skill-role definitions and direct-user grants. Once the application exists, its owner can define a skill role for use by any skill in its namespace. A role with the same name in another application has independent grants and an independent lifecycle, even when both applications have the same owner.

A direct user must already be a member of the application before its owner can grant that user a skill role. The owner can add a user by email and then grant skill roles before the user's first sign-in.

Through REST or MCP, a signed-in user can list every application where the user currently has at least Caller access. Each entry includes the application ID, namespace, the user's Cloud role, and the [invocation documentation](#invocation-documentation) URL.

When the owner removes a direct user from an application, Go clears that user's skill-role assignments for that application. If the owner later adds the user back, earlier assignments do not return. Changing an existing member's Cloud role, including from Manager to Caller, preserves their skill-role assignments.

When the owner deletes a skill role, Go deletes its definition and all of its direct-user assignments for that application in one Postgres transaction. If the owner later creates a role with the same name, it starts with no direct-user assignments.

Each operation declares its required application permission. The shared Cloud API checks the caller's current access when each ordinary request arrives. Once Go authorizes the request, it proceeds to completion under the operation's normal rules, even if the caller's access or roles change while it is in progress. Each subsequent request, including a status poll, lookup, or retry, receives a fresh access check.

The shared Cloud API applies the per-application Delete admission rule above to management writes from every interface.

After a successful access check, Go gives a signed-in user the same `404 Not Found` response when the application does not exist or the user has no access to it. If the user has access to the application but their Cloud role does not permit the requested action, Go returns `403 Forbidden`.

After authorizing Stage, Publish, or Invoke, Go checks the application row’s lifecycle state. If it is `Deleting`, REST returns `409 Conflict` with reason `Deleting`. Stage stops before writing pack files, changing staging, or validating. Publish stops before selection warnings, the already-published check, or acceptance, so it allocates no request ID or version. Invoke stops before dispatch even if the engine still hosts the namespace. Previously admitted invocations may drain.

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

In V1 all saved application variable values are write-only through the Cloud API. Users with Manager access can list variable names, see which have a nonblank value configured, and set, replace, or remove a value. Console, REST, and MCP expose only names and configuration status when reading variable settings. Go reads the required values internally for publication, restart, and recovery.

If authorized administrators edit the same variable, the last saved edit takes effect. The Cloud API accepts the edit even if another administrator has changed the variable since the caller listed it.

### Encryption at rest

Go encrypts saved application variable values before writing them to Postgres. Each Cloud environment supplies its own variable encryption key to Go through a Kubernetes Secret. At installation, the operator saves a recovery copy in an operator-only directory on the shared volume. Restrict the directory's filesystem permissions and mount only the required data directories into Cloud workloads so the recovery directory stays outside their mounts. Include the directory in the protected off-cluster volume backup.

On a replacement cluster, the operator first [restores access to the shared volume](roadmap.md#durable-execution-traces), retrieves the saved variable encryption key, and recreates Go's Kubernetes Secret with the same key before starting Go against the recovered Postgres data.

Go writes original pack ZIP bytes to the [shared volume](roadmap.md#durable-execution-traces), computes SHA-256 over those bytes, and returns the same bytes on authorized download. The storage provider handles file encryption transparently.

## Upload and staged validation

Upload stages the pack and returns advisory validation feedback. Publish is a separate action that automatically runs fresh validation on the selected staged revision before installing it. The caller can select Publish directly, including when validation was unavailable at upload time.

Validate and Install each receive the [30-second outbound deadline](execution-engine-api.md#validate-and-publish-responses). If Validate times out during staging, Go keeps the revision and reports validation unavailable. If fresh validation times out during an accepted Publish, Go records a terminal validation-unavailable failure, returns the request ID and skips Install.

### ZIP checks before staging

An MCP companion uploads a local pack in one authenticated `upload_pack(applicationId, zipBase64)` call. The companion client reads the ZIP and base64-encodes its bytes. Go checks the signed-in user's upload permission for the application, decodes the ZIP, and passes the original bytes to the shared upload and staging operation. The tool returns that operation's outcome, including the revision ID and validation feedback for a newly staged pack.

Go authorizes the upload and checks the entire archive against the [package limits](publication-package-format.md#archive-rules). It caps the incoming ZIP bytes, counts every entry including explicit directories, and counts actual expanded bytes while reading every file, including companion assets. It rejects nonregular files, entry types other than files and directories, ambiguous paths, and paths that collide without regard to case. Report the offending path, or both paths for a collision.

Go requires one top-level directory matching the destination namespace, the [companion entry point](publication-package-format.md#required-companion-entry-point), and the [execution subtree layout](publication-package-format.md#execution-inputs), including lowercase skill paths. In `loomspan/execution.yaml`, reject reserved administrative sections and any executor configuration owner key that differs from the destination namespace. Report the offending field and its pack-relative source. A pack that passes these checks can be staged while the engine is offline.

Use one Go pack inspection routine to check archive and layout and Cloud naming and configuration rules, and to discover variable references with their authored source paths. Upload and promotion apply the pre-staging checks. Validation and installation reuse the same discovery logic for the selected revision. Framework checks skill references and the composed execution configuration, and Java enforces the provider policy during validation and preparation. The [shared variable fixtures](application-variable-conformance.md) cover the discovery rules used by both languages.

Variable discovery runs on the authored ZIP before staging and requires no application values. On a discovery failure such as `blank-name`, Go rejects the candidate locally with the corpus reason and each pack-relative source and authored YAML path in diagnostic order. The previous staged revision and its validation feedback stay intact. A valid marker with a missing or blank application value passes discovery and proceeds to ordinary staged validation. An effective mapping-key collision that depends on a supplied value also passes discovery. Java diagnoses that collision if substitution during installation produces it.

Before staging an upload or promotion, Go checks each skill’s local name against the [qualified name length limit](publication-package-format.md#namespace-and-skill-names) using the destination namespace. A total of 64 ASCII characters, including the two separator characters, passes this check. If the total exceeds 64, Go rejects the pack and identifies the offending skill and its pack-relative source file. The destination keeps its current staged revision and validation results. Go performs this check locally even when the engine is offline.

### Staging revisions and validation

Through MCP, a user with Manager access can call `validate_pack(applicationId, stagedRevisionId)` to rerun the shared validation operation and get its feedback for that revision.

Console uploads and companion uploads through MCP use the same staging operation. After upload checks pass, the Cloud API computes SHA-256 over the exact ZIP bytes. If the digest matches the target application's current staged revision, Go returns that revision and its existing validation results. Otherwise Go immediately stages a new revision without asking for confirmation. This includes the first upload and a ZIP identical to an older revision.

For a new revision, Go reuses the stored ZIP if the same bytes are still present or saves the submitted bytes. Validation results stay with their original revision. The Cloud API makes the digest comparison and staging update atomically so concurrent identical uploads resolve to one revision. A rejected upload creates no revision and preserves the current revision and its validation results. A pack staged by promotion follows the same rules.

Each new staging revision records and shows who staged it and when, including revisions created by promotion. An identical upload that returns the current revision preserves its original actor and time.

Whenever staging creates a revision, Go automatically validates it and returns validation feedback to the caller as part of staging. An authorized caller can also rerun validation for a specific staged revision through Console or MCP. Go runs its Cloud checks and requests engine validation of the authored pack even when application variables are missing or blank. Once validation completes, Go returns the engine diagnostics, all missing or blank variable references (with pack-relative source and authored YAML path), and any other pack issues found by Cloud in one result. A completed result can report an invalid pack. The result belongs to the requested revision even if another upload arrives. The pack stays staged and the current publication keeps serving.

Go applies the [engine shutdown rule](#engine-shutdown-during-publish-or-validate) to automatic and user-requested validation. If validation cannot complete, Go reports that the result is unavailable and tells the caller to retry validation. Go withholds partial findings from the caller.

If the pack is otherwise valid, report that it passed validation and identify the variables blocking publication. A user with Manager access can supply the values and publish the same revision directly. Publication checks the current values.

For each staged revision, the Cloud API keeps the last completed validation result and its timestamp visible, including feedback from automatic validation on upload. The result describes the revision and settings used at that time. Later edits leave the result and timestamp intact. V1 has no stale validation state. Publish runs fresh validation whenever it will attempt a new installation.

For a Publish request, Go fixes the selected pack revision, reads the required variable values and effective runtime settings, and holds them in memory. Go uses those values for its checks and sends the same values and settings with that revision for installation. The engine checks the combination it actually installs. V1 leaves the timing of edits that overlap those reads unspecified and requires no configuration version history or edit lock. Users save settings before selecting Publish. If they edit settings during Publish, they can select Restart afterward to apply the latest saved values. Console shows the confirmed active settings separately from subsequent edits. [Administrative runtime settings](java-execution-engine-roadmap.md#administrative-runtime-settings)

Go reads the application's active trace persistence policy from `tracePersistencePolicy` on the matching namespace entry in a fresh authenticated instance snapshot. Console compares that observed policy with the application's saved selection. A fresh snapshot supplies the active policy after a Go restart or a lost publish response. To confirm that a particular restart completed, Go still applies the [JVM and generation checks](#restarting-an-application).

### Publishing a reviewed revision

The MCP `publish_pack` tool takes the application ID and staged revision ID. It follows the same reviewed-revision confirmation and fresh-validation rules as Console. Once Go accepts the publication, the call waits for confirmation and includes the stable request ID in its response. On success it also returns the confirmed publication version. On a known failure it also returns the diagnostic. If Go loses the engine reply after dispatch and the outcome is still unconfirmed at the end of the [30-second confirmation window](#unconfirmed-publications), the response reports that the publication may still complete.

When Publish proceeds with a new installation, Go validates the selected revision against the application's current configuration. If validation passes and the required variables are configured, Go sends that same revision to the engine's publish endpoint. If validation fails or cannot complete, Go reports the outcome, keeps the revision staged, and leaves the current publication serving.

Before each install, Go checks the selected ZIP’s [variable references](publication-package-format.md#application-variables) and reports any missing or blank values with their pack-relative source and authored YAML path.

When starting a publication, restart, or recovery, Go reads the destination application’s current values for those names, computes its effective runtime settings from saved selections and Cloud defaults, and uses the [publish endpoint](execution-engine-api.md#internal-api-requests). The ZIP is the durable record of its requirements.

The caller names the staged revision to publish. If it differs from the application's current staged revision, the Cloud API returns a warning identifying both revisions. After the user confirms, Console and MCP repeat the same publish operation with the reviewed revision ID and the current revision ID shown in the warning. Go accepts that confirmation only while the warned-about revision is still current. If staging has changed again, Go returns an updated warning and waits for fresh confirmation. An accepted confirmation authorizes publication of the reviewed revision while its ZIP is retained. If cleanup has removed the ZIP, Go tells the caller to upload it again. The newer revision stays current in staging for continued work.

After authorizing **Publish** and resolving any required reviewed-revision confirmation, Go checks whether the selected revision is already the application's current publication. If so, it immediately returns “This revision is already published.” It skips validation, creates no publication version, and makes no call to Java. This also applies if a required variable has been removed since publication. To apply saved variables or runtime settings to that revision, the user selects [Restart application](#restarting-an-application).

For a new installation, Go first commits the [Publish acceptance transaction](#pack-history) with the request ID and target version, even when the engine is already known to be unavailable. Go then checks the inputs and engine eligibility and runs fresh validation. Dispatch requires an authenticated snapshot from the intended JVM on the current connection and follows the [engine shutdown rule](#engine-shutdown-during-publish-or-validate).

If the pack fails validation, a required variable is missing or blank, validation cannot complete, the current snapshot is unavailable, or the pod is terminating before Go sends the install, Go records a terminal failed Publish with the specific reason on the accepted request. The caller receives its request ID and diagnostic. The failure is visible in request history and as the latest status while this is the most recently accepted request. The revision stays staged, the current publication pointer stays in place, and the failed request releases its ZIP protection. Its target version does not appear as a confirmed publication in pack history. A lost reply after install dispatch follows the [unconfirmed publication rules](#unconfirmed-publications).

For a new publication, Java's `204 No Content` confirms installation. Before reporting success, Go records the request outcome and applies the [current-publication transaction rule](#unconfirmed-publications). If the pod dies before Console observes the acknowledged version, recovery restores the application's durably current publication.

### Dispatch eligibility for Validate and Publish

For each new Validate or Install call, the engine selector must match exactly one watched pod. That pod must have no `metadata.deletionTimestamp` and Go must have an authenticated snapshot from its JVM on the current Console connection. A sole pod with `Ready=False` and zero installed namespaces is eligible for validation and first publication once Go has that snapshot. Invoke retains its separate [routing requirements](roadmap.md#starting-point-for-placement-and-routing).

If a terminating pod and its replacement are both visible, neither receives a new Validate or Install call. During the overlap, Stage keeps the revision and reports validation unavailable. An accepted Publish records a placement failure and returns its request ID. Once the replacement is the sole match, its own current authenticated observation can make it eligible. For an Install already dispatched, the original pod UID and JVM `instanceId` identify the confirmation target. Go never resends that Install to the replacement.

### Engine shutdown during Publish or Validate

Once Go observes that Kubernetes has marked the engine pod as terminating, it stops sending publish and validate requests to that pod, even while Console remains connected. For a request awaiting dispatch, Go tells the caller that the engine is shutting down, the request was not sent, and the caller can submit a new request when the engine is available.

For a request already dispatched, Go handles any engine reply normally. If shutdown interrupts the reply to validation, Go reports that validation is unavailable and can be run again. If shutdown interrupts a publish reply, Go follows the [unconfirmed publication rules](#unconfirmed-publications). [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

### Pack history

A user with Manager access can call the read-only MCP tool `list_packs(applicationId, includeValidation=false)` to find the application's retained, downloadable packs. Each staged entry gives the staged revision ID, who staged it, and when. Pack history lists every publication whose installation Go has confirmed and whose ZIP is retained. Each published entry gives the publication version, the user who requested it, and the time Go first confirmed its installation. A restart preserves the original publisher and first confirmation time. The list marks the current staged revision and the durably current publication independently and returns the selector each entry needs for `download_pack`.

By default, staged entries omit the `validation` field. With `includeValidation=true`, each staged entry includes its last completed validation result, diagnostics, and timestamp, or `validation: null` if no check has completed. The listing reads saved data and works while the engine is offline. To run a fresh check, call `validate_pack`.

An MCP companion calls `download_pack` with the application ID and exactly one selector: a staged revision ID or a publication version. Go checks that the signed-in user currently has Manager access to the application and reads the retained ZIP for that selection through the shared download operation. The tool returns the original ZIP bytes encoded as base64 and the SHA-256 digest of the original bytes. The companion client decodes the ZIP to a local file.

The backend pack-transfer ticket requires an automated integration test in CI. Using one authenticated test-user session, an MCP client reads a valid 1,000,000-byte ZIP from disk, calls `upload_pack`, downloads the returned staged revision through `download_pack`, writes the decoded bytes to a second file, and compares the two files byte for byte. When first integrating the intended companion, verify the same local-file workflow after one sign-in without copying a token or pasting the archive into chat. Automate that compatibility check if the companion supports it; otherwise perform it once manually.

Users with Manager access can select a retained publication by version and download its original ZIP. To review a change, they can download two retained versions and compare them locally. The [pack-history quota](#go-backend-configuration) sets a byte allowance for retaining pack history.

V1 handles rollback through the ordinary publishing workflow. A user with Manager access can download an older version’s retained ZIP, stage it, and publish it again.

Go timestamps every accepted pack upload. Each stored ZIP has an upload timestamp on the filesystem. For each application, count the bytes of every eligible ZIP on disk, including superseded staged packs that were never published. Multiple revisions referencing the same file count once. A ZIP needed for the current publication, the current staged revision, or an outstanding publish or restart stays outside the quota and is protected from trimming. Go applies these protections at deletion time. Even with a zero-byte quota, cleanup must preserve the currently published ZIP for download and recovery.

When accepting a Publish, Go locks the application's Postgres row with `SELECT ... FOR UPDATE`, checks that the selected ZIP still exists, and records the accepted request and its ZIP reference in that transaction. The commit releases the lock before engine validation or dispatch. Staging and Restart use the same row lock when establishing a protected ZIP reference.

The [cleanup job](#trace-cleanup-job) calls Go once for each application processed for pack retention, supplying the application ID and its effective pack quota from the pass snapshot. Go scans that application’s pack files. When eligible bytes exceed the quota, Go selects candidates in upload timestamp order, oldest first. For each removal, it locks the original application row in a short transaction, rechecks ZIP protection and application state, and removes the file while holding the lock only if it is still eligible. If the row is gone or the application is deleting, trimming stops. Trimming ends when eligible bytes are within quota or eligible candidates are exhausted. Go returns the remaining eligible ZIP bytes and their measurement time on success. The job includes this result in its post-cleanup report.

Verify both orderings with a superseded staged revision. If Publish accepts the revision first, cleanup skips its ZIP even if the pass selected it earlier. If cleanup removes the ZIP first, Publish reports that the caller must upload it again and leaves the request unaccepted.

Pack cleanup leaves the [application operations log](#application-operations-log) intact. Users with Manager access can still read every logged entry and download any retained ZIP by publication version.

### Application status

The Cloud API exposes two separate application status fields to users with Manager access, including through their companions. Console and MCP use the same fields.

- **Serving status** reports whether the router can forward a new invocation for this application now. With requests enabled, an application ID with no recorded current publication shows **Not yet published**. Once the application has a current publication, an engine disconnect makes it “Temporarily unavailable.”
- **Latest publish or restart status** identifies the most recently accepted request, its operation (publish or restart), its target version, and its current progress or outcome under the publication and restart confirmation rules below.

A user with Manager access can select **Disable requests** or **Enable requests** for an application. While disabled, serving status shows **Requests disabled** and the Go router rejects new requests to run skills before dispatch. Calls already running finish normally. Administrators can still inspect the application and publish a fix. The switch leaves the Java engine and its installed publication unchanged. Enabling requests restores normal routing eligibility checks.

Requests stay disabled across publishes, application restarts, and Go backend restarts until a user with Manager access explicitly selects **Enable requests**. An authorized HTTP caller whose request is rejected because requests are disabled receives `503 Service Unavailable` before execution. Go omits `Retry-After` because there is no scheduled time for requests to resume.

For example, Console can show “Serving” while a publication awaits confirmation. A later engine disconnect changes serving status independently of the recorded publication outcome.

Verify slug reuse on the same running JVM. Delete an application and recreate it under the same owner and slug while the deletion's empty generation is still installed. The fresh application shows **Not yet published**, rejects invocations before dispatch, and serves the [pre-publication documentation](#invocation-documentation) with empty `paths` and no publication version. After its first confirmed publication, verify the [ordinary routing rules](roadmap.md#starting-point-for-placement-and-routing) apply.

Alongside the latest request status, Console shows the time and type of the application's last confirmed publish or restart. A confirmed restart advances this time even when the publication version stays the same.

When Go accepts a publish or restart, it assigns a stable request ID and persists it with the accepted operation before dispatch. Go records progress, outcome, and confirmation time on this Postgres request row. Recovery and status reads use the same records, including lookup by request ID, the most recently accepted operation, and the most recent confirmation. The application’s current-publication pointer follows the [transaction rule](#unconfirmed-publications). The [operations log](#application-operations-log) is a best-effort history for administrators. The caller receives that ID in the operation’s response. A reviewed-revision warning or an “already published” response creates no request ID.

If the caller loses the response before receiving the ID, the [ordinary Console reload](roadmap.md#console-navigation-and-modules) shows the latest operation and last confirmation time. If concurrent restarts leave the caller unsure which request was theirs, the user decides whether to submit a new Restart request. Users with Manager access, including through their companions, can look up that request's progress or outcome by application and request ID, including after a newer request becomes the latest. The request identifies its target publication version. Two restarts of the same publication have distinct request IDs and refer to the same published pack.

### Unconfirmed publications

If Go knows the Install request was never sent, it records a terminal failure. If the request may have reached Java and Go lacks a definitive reply, the confirmation window starts as soon as Go detects the lost reply or the outbound deadline expires, whichever comes first. Go immediately requests a snapshot refresh from the Console proxy. The refresh uses time within the confirmation window (30 seconds in V1).

For a new publication, confirmation means observing the requested namespace and version on the intended engine. For a restart, use the [restart confirmation rule](#restarting-an-application). If the window expires, Go returns the request ID and tells the caller the installation is unconfirmed and may still complete. The timeout ends the caller’s wait. Go continues checking for confirmation and never resends the original installation request, including after a Go restart.

On backend restart, Go records **Interrupted before dispatch** against the original request ID if it can establish that an accepted installation was never sent. This ends the request, including any hold it placed on Delete or ZIP trimming. The caller can submit a new Publish or Restart. Verify this by restarting Go while an accepted request is still held behind an earlier installation.

For an installation that may have been dispatched, Go reconciles the recorded request after a backend restart and after the caller's confirmation window expires. Before restoring the previous current publication, Go checks a fresh authenticated snapshot from the intended engine. Observing the requested new publication version in the requested namespace confirms that installation. For a same-version restart, apply the generation and JVM checks below.

When an installation is confirmed under the rules for its operation, Go records success against that request ID. In the same Postgres transaction, Go records its publication as current only if it has accepted no newer publish or restart for the application. This check applies even while the newer request awaits dispatch. Recovery uses the durably current publication. Each invocation follows the normal routing and Framework cutover rules.

For example, publish A loses its reply, B succeeds, and C succeeds and becomes current. A stays unconfirmed unless Go receives confirmation for A. A late confirmation records success for A and leaves C current for recovery. While A’s ZIP is retained, [pack history](#pack-history) lists A with its original publisher and the time Go received that confirmation. A is available for download or promotion by version. Only C carries the current-publication marker. B's request still shows success when a subsequent invocation runs C.

If Go accepts a newer publish or restart for the same application while the earlier installation is unconfirmed, it holds the newer request until the snapshot confirms the earlier installation or 30 seconds past the end of its configured confirmation window, whichever comes first. Either event releases the newer request for dispatch. If the newer request then fails, recovery uses the publication that was durably current when Go accepted that request. Go measures the deadline from the start of the earlier request's confirmation wait (60 seconds total with the V1 setting).

For a REST Publish held behind earlier work on the same application, the caller waits up to 90 seconds from when its accepted request first enters the queue. Moving forward in the queue keeps the original deadline. If the request becomes eligible for dispatch before or exactly at that deadline, Go proceeds and returns the usual Publish result. If it is still queued, Go returns HTTP 202 with status `pending` and the durable request ID. The request keeps its place and proceeds when eligible. The caller can follow it through the [request lookup](phase-1-rest-api.md#publish-and-request-lookup).

Framework rejects an earlier candidate as stale if the newer generation becomes active after the earlier candidate captured its base generation. V1 accepts the remaining risk that a delayed earlier request can first enter preparation after the newer generation is active and replace it on the engine. Go still applies the current-publication transaction check above.

To try again, the caller submits a new Publish or Restart. Go checks current permissions and applies the ordinary rules for the selected operation, including the already-published check for Publish. Each newly accepted operation receives its own request ID and uses the application’s current saved variables and effective runtime settings. The original request remains available for status lookup under its own ID.

### Restarting an application

Saving variables or administrative settings leaves the running generation unchanged. Restarting the application requires Manager access. Choose **Restart application** to republish the current ZIP and version with the saved edits. Other applications keep serving.

The placement controller orders Publish and Restart dispatches per application. If an earlier installation is unconfirmed, Go holds the newer request until it confirms the earlier installation or reaches the [dispatch deadline](#unconfirmed-publications), whichever comes first.

Before dispatching a restart, Go reads a fresh Console snapshot and records the JVM’s `instanceId` and the namespace’s `generationId` if present. After Java reports success, Go requests another snapshot from the same JVM to confirm the current published version:

- If the namespace was present, confirm that it serves the requested version with a different `generationId`.
- If the namespace was absent, confirm that it now appears with the requested version.

These snapshot checks are sufficient in V1 even when two restarts of the same version overlap after the dispatch deadline. If both replies are lost, either restart may have produced the observed generation. Go treats a restart whose snapshot check passes as confirmed, even though the observation cannot establish which request's saved settings took effect. V1 requires neither additional attribution tracking in Go nor a restart request ID in the engine protocol.

If preparation fails, the Cloud API reports the diagnostic and any previously active generation keeps serving. If the response is lost, the placement controller follows the [publish confirmation rule](#unconfirmed-publications). Generation IDs are process-local. A changed instance ID triggers engine recovery and fresh placement confirmation.

## Publication promotion

Promotion stages a selected publication from a source application as a revision in a destination application. The caller chooses the source and destination for each promotion through Console or MCP. The applications can serve any purpose. Cloud gives their names and the direction of promotion no special meaning.

Go owns promotion. It reuses the internal operations that serve publication downloads and ordinary uploads, with a namespace transformation between them.

1. **Export.** Check permission to read the selected source publication and stage at the destination. Read the immutable portable ZIP through the same operation used to download that publication.
2. **Transform.** When the namespaces differ, open the ZIP and write a new archive with the destination namespace as its top-level directory. Update the companion’s `SKILL.md` frontmatter `name` and the `loomspan.executor-config` owner key if present. Preserve companion instructions, skill definitions, nested settings, and explicit external references. Reuse the original ZIP bytes when no edit is needed.
3. **Import.** Pass the resulting ZIP to the ordinary destination upload and staging operation. It applies the [pre-staging checks using the destination namespace](#zip-checks-before-staging), then the usual [digest comparison and restaging rules](#staging-revisions-and-validation). The caller can review the transformed pack in the destination.

The destination uses its own variables and runtime settings. Its current publication keeps serving until an authorized user publishes the staged revision. Framework sees only ordinary validate and publish requests for the destination namespace.

## Future feature: client skill packs for invited users

When an owner grants a user access to a published application, the Cloud API could generate a client-only skill pack for that user's companion. It would derive the download from the published ZIP using the package format's export rules. The companion would use `SKILL.md` as its entry point and invoke the application through the user's authorized Cloud connection. The Cloud API would check the user's current permissions on each call. [Client-only export contract](publication-package-format.md#client-only-export)

## Invocation response visibility

- [x] Verified the Framework invocation session ID enhancement. Both `SkillTemplate.invoke(...)` and `AdmittedSkillInvocation.invoke()` return a `SkillInvocationResult` on success. For example, after `var result = skillTemplate.invoke("support", "summarize", input);`, the Java wrapper can read the original result text from `result.text()` and the canonical root session ID from `result.sessionId()`.

On failure, `SkillException.getSessionId()` returns the root ID if a session was created and `null` for a pre-session failure. A post-session `SkillAccessDeniedException` also exposes the ID. The ID is available with execution views off, when trace retention discards the trace, and when constructing a requested view fails. It matches the ID in any delivered view or retained trace.

When the engine supplies a session ID, Cloud returns it to the caller. A caller can give that ID to an application administrator, who can use it to [look up the corresponding retained trace](#look-up-retained-traces) within that application. This is the agreed V1 workflow. The Cloud response and lookup still need implementation. In the Framework repository, see `ai/thoughts/tickets/invocation-session-id.md` for direct and admitted caller examples and `ai/thoughts/verification/2026-11-25-invocation-session-id.md` for test evidence.

Go enforces both V1 invocation limits once, after the REST and MCP endpoints converge on the shared invocation path and before dispatch to Java. UI invocations use the same path. Measure the caller-facing common JSON or multipart invocation body, up to 20 MiB (20,971,520 bytes) in total and up to 25 uploaded files, both inclusive. For REST JSON, count the received body bytes including syntax and whitespace, before Go adds trusted namespace, caller or execution-view metadata. Go-added fields consume none of the caller’s allowance. For MCP file calls, measurement happens after Go decodes the base64 file data and constructs the multipart body. Count the entire common body, including metadata, decoded file bytes, part headers, and boundaries. Each uploaded file part counts once, including an empty file. The JSON metadata part counts only toward the byte limit. If either limit is exceeded, Go rejects the request before dispatch.

For a rejection by the shared invocation limit check, REST returns `413 Content Too Large` and MCP returns a tool error with the same message. The message identifies the exceeded limit (20 MiB total or 25 files) and tells the caller that the skill did not start.

The Cloud API applies the same V1 rule to REST and MCP. With permission to invoke the application, callers receive the exact `result` on success or the engine’s top-level `message` verbatim and any top-level input-validation `issues` on failure. Caller-friendly error categories and rewritten messages are deferred to V2. [Engine response fields](execution-engine-api.md#invoke-responses)

One application administrator setting controls `executionView`, `debug`, and `diagnosis` in REST and MCP invoke responses. It defaults to off for new applications. When enabled, Go automatically returns the execution view for the invoker's own call on success or failure and includes the available `debug` and `diagnosis` fields on failure. When disabled, Go omits all three fields even if the invoker asks for them. The switch applies independently of trace persistence.

## Invocation response loss

If the caller disconnects after execution starts, the engine continues the invocation under its normal runtime limits. V1 has no mechanism to cancel a running invocation.

The router removes a terminating engine from routing as soon as it observes shutdown. In single-engine V1, new requests receive a temporarily unavailable response while there is no eligible engine. If forced pod termination cuts off the result of a dispatched invocation, the same unknown-outcome rule below applies. [Engine shutdown contract](java-execution-engine-roadmap.md#planned-engine-shutdown)

An unconfirmed result after dispatch is an unknown outcome. The router does not automatically retry. [Synchronous invocation outcomes](roadmap.md#synchronous-invocation)

### Application invocation log

The invocation service owns the application's log of incoming API invocations. Each entry records who made the request, when it arrived, the requested skill entry point, and the incoming parameters. The application's administration section makes this log available to users with Manager access. These fields support later manual comparison with traces, including when a caller loses a response. V1 records and displays the invocation log independently of Framework trace capture and lookup.

If Go rejects a request because the application has requests disabled, it returns before logging and writes no per-request entry to any Go backend log.

Log each authorized invocation request that passes Cloud's request checks, including when no eligible engine is available. Record the routing decision once. When Go chooses an eligible engine, append **Engine selected** before sending the request. This entry establishes only that Go selected an engine. When Go rejects a logged request before sending it, record **Rejected before dispatch**. Requests that fail authentication or application authorization are excluded from the application-visible log.

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

When a user with Manager access sets, replaces, or removes a saved application variable, append one entry with the actor, time, variable name, and action. Omit both the old and new values.

Operations-log writes are best effort. If an append fails, Go continues the otherwise authorized operation and attempts to report the affected namespace, operation, and append error in the operator logs. The caller receives the operation's actual outcome.

## Console proxy

### Observe live engines

The Console proxy maintains its in-memory observations using the [pod and JVM identity contract](execution-engine-api.md#pod-and-jvm-identity). After an install succeeds or its response is lost, the placement controller asks the Console proxy to refresh that engine's snapshot. On disconnect, the Console proxy keeps the last observation available for inspection and marks it disconnected. The placement controller uses fresh observations from connected instances to confirm placements.

On Go backend startup or Console reconnection, the placement controller checks each application in this order:

1. If the application is `Deleting`, skip ordinary recovery. During an active deletion, follow the [drain or engine termination rules](#application-permissions). If a Go backend restart interrupted deletion, the application stays `Deleting` with its slug reserved.
2. If a publish or restart is outstanding, apply the [interruption and confirmation rules](#unconfirmed-publications). On backend restart, close a request known to be unsent. Reconcile a possibly dispatched request against a fresh authenticated Console snapshot from the intended engine before restoring a publication, including the check for a newer accepted request.
3. Compare the resulting intended publication with the fresh snapshot for its assigned engine. If the namespace and publication version match, the existing generation keeps its loaded variables and settings. Otherwise, restore the intended publication and confirm the installation through the Console proxy.

Recovery proceeds per namespace. If one restore fails, the Cloud API reports that application's failure while the placement controller continues restoring the remaining namespaces.

After a failed restore, a user with Manager access can select [Restart application](#restarting-an-application) to install the current publication with the latest saved variables and settings.

In V1 the router sends work only to intended placements. If the Console proxy observes an additional namespace, the placement controller leaves it loaded and the router excludes it from routing. Explicitly unassigning an application from an engine and unloading its namespace are deferred until Cloud supports multiple engine runtimes.

The Console proxy consumes streams from engines that can host several applications. Only application administrators and companions acting with their permissions can inspect live system events. Each viewer's subscription follows the [shared application permission rules](#application-permissions), and the proxy scopes events to the authorized namespace. Closing one viewer's subscription leaves the shared engine connection available to other viewers.

When the proxy loses the live event stream, Cloud tells each application administrator viewing the affected application “Live events temporarily unavailable.” When the proxy can deliver live events again, Cloud reports “Live events resumed.” Both notices describe event visibility for that application. Engine-wide diagnostic details stay in the platform logs.

On reconnect, the proxy resumes from the current point. The live timeline keeps the gap between the two notices visible and skips replay of events from that interval. If an affected run has a retained trace, an application administrator can inspect it in the application's run history.

### Namespace skill catalog

The Full Cloud Console proxy owns catalog retrieval, freshness, and storage. It uses the authenticated engine snapshot to identify the application's serving namespace and publication. Staging or validating a revision does not change the catalog. Through the Console connection, one namespace-scoped read returns the complete catalog from one active generation: skill names, descriptions, input schemas, output schemas where available, and effective entry-point access policies. The response identifies the engine instance, generation, and publication it actually describes.

Go keeps the lightweight engine observation in memory and reads the saved catalog from disk as needed. In the dedicated Go-owned `skill-catalogs/` directory on the [shared volume](roadmap.md#durable-execution-traces), the proxy keeps one replaceable file per application containing the complete catalog and its identity. This file is shared across documentation requests regardless of the caller's roles.

When the saved `instanceId` and `generationId` match the proxy's current observation for the serving namespace, the proxy reuses the file without querying Java. A missing file or a change to either ID prompts a full namespace read. Before replacing the file, the proxy checks the response's own identity and publication against the serving observation so a concurrent publish cannot label one generation's skills as another's. It writes the complete replacement to the same directory and atomically renames it into place. An interrupted refresh preserves the previous complete catalog. While a refresh is pending or the engine is offline, the last complete file remains available with its recorded publication version, including after Go restarts.

### Uploaded trace permissions

Full Cloud uses the shared volume's uploads directory, with a subdirectory per destination namespace. Console-only uses a persistent local uploads directory grouped by connected target and then by destination namespace, so two targets can each have a namespace named `default`.

Console lists and opens uploads through scoped API operations. The backend resolves each file from the authorized destination and an upload identifier, and uses the same scope for overwrite and deletion. Clients supply identifiers rather than filesystem paths. The directory comes from the confirmed destination, independently of the namespace recorded inside the file.

Go checks the destination application's diagnostic permissions on import, discovery, and inspection. Store the original bytes with separate destination, upload time, and optional expiry metadata. Keep uploads out of the engine's retained-trace index. Both Console profiles follow the same [import and retention workflow](roadmap.md#opening-a-trace-file).

A user with Manager access to the destination application can change an upload's expiry or delete it, regardless of who uploaded it. Go checks the user's current role in the destination application on each request, including requests made by their companion.

### Manual trace upload quota

Full Cloud enforces the [configured manual-upload quota](#go-backend-configuration) in public and internal installations. Console-only leaves local disk usage under the user's control and applies no manual-upload quota.

Whenever Go checks manual-upload usage, including at save time and when the quota screen is read, it sums the sizes of the saved manual-upload files present for that application. An expired file counts until deletion. The next check reflects the freed space directly from the filesystem.

At save time, Go checks the upload or overwrite against the active quota. When the quota check passes, the upload proceeds through the normal import workflow. Console omits quota previews and quota-specific success messages. Users can review their usage on the [quota screen](#quota-visibility).

The caller chooses how long to keep a manual upload, so lowering the quota preserves existing uploads and their expiry dates. Users with Manager access can still inspect, download, or delete them. For an overwrite, count the incoming file size minus the size of the upload it replaces. If the application is already over quota, allow changes that leave usage unchanged or reduce it. Reject increases that would put or keep the application over quota, and preserve the existing upload on rejection.

When Go rejects a manual upload or overwrite for exceeding quota, report the destination application's current manual-upload usage, its effective limit, and how many bytes the proposed usage would exceed that limit by. Use the values from the final quota check.

### Trace cleanup job

One Full Cloud cleanup job runs every five minutes with the manual-upload and engine-trace directories mounted for writing. For each application, the pass expires manual uploads, trims retained engine traces, and calls the [Go pack-trimming operation](#pack-history). Go owns all pack filesystem access for the pass. Only one pass can run at a time. If a pass is still running at a scheduled start, the job skips that start, lets the current pass finish, and resumes at the next scheduled start.

At the start of each pass, the job fetches a snapshot from Go with the active retained-trace and pack-history defaults, accepted overrides, and application IDs skipped during configuration loading. It uses that snapshot for the entire pass. For each skipped application, the job skips manual-upload expiry, retained-trace trimming, and pack-history trimming. The job skips applications in `Deleting`. If deletion overlaps a pass already working on that application, the pass tolerates files removed by Go. Retention work and usage reports belong to the original application ID, including when a later application reuses its namespace. If Go is unready or the job cannot fetch the complete snapshot, the job skips the pass and tries again at the next scheduled pass. A quota edit first applies on the next pass after it becomes active.

A zero-byte quota removes all eligible retained traces on the next pass. For each application over its retained-engine-trace quota, the job deletes eligible retained traces in order of completion, oldest first, until usage is at or below the quota or no eligible traces remain. The job skips active writes and files still in Console’s completion grace period. The quota is soft. Usage can exceed it between passes. Monitor free space on the shared volume. Time-based expiry for engine traces and cleanup of abandoned writes remain outside V1. Measure trace count, bytes per application, and growth over time to inform future quota values and whether allowances should vary by pricing tier.

Each cleanup pass is stateless. After an interruption, the next pass sees the files that remain on disk and can resume cleanup without a checkpoint. On completion it sends Go separate post-cleanup byte counts for retained engine traces and [eligible retained pack ZIPs](#pack-history), with the measurement time for each count. For each application where the job trims retained engine traces, it writes a structured audit log with the application ID, quota applied, number of traces removed, and bytes freed. These logs serve only as an audit record.

If any cleanup step fails for an application, including pack trimming, the job logs the application ID and error in the operator logs, skips all remaining work for that namespace in the pass, and continues with the next application. It reports usage only for applications whose cleanup completed successfully. If the failure occurred partway through retention trimming, the audit log records the deletions that succeeded. The next pass scans the directory afresh.

Verify slug reuse during retention. Pause a pass after it selects files for one application but before it removes them. Delete the application, recreate it with the same owner and slug, and create eligible files at the same relative paths. Resume the old pass and deliver its usage report. Verify that the new application keeps its files and that its quota screen excludes measurements for the old application ID.

### Quota visibility

In Full Cloud, users with Manager access see three rows on the same quota screen: manual trace uploads, retained engine traces, and pack history. Each row shows bytes used and the current effective limit for that application. The deployment operator manages all three limits.

Go measures manual uploads when the screen is read. For retained engine traces and pack history, Go keeps the latest successful byte count and its original measurement time per application in memory. Console shows that timestamp alongside usage. After startup, Console shows the effective limits as soon as Go loads the configuration and displays “Awaiting first measurement” in each cleanup-measured row until its first report arrives.

### Look up retained traces

The Console proxy reads the [shared trace filesystem](roadmap.md#durable-execution-traces) for historical lookups, including after the producing engine has been replaced. Only application administrators and companions acting with their permissions can browse retained runs or look up, review, and download traces for that application.

Finalized engine trace files on the shared volume are the source of truth for run history. On Go startup, the Console proxy scans them in the background to rebuild an in-memory index. Console shows that history is still loading while the initial scan is incomplete. Periodic scans discover newly retained files and remove entries for files that have disappeared.

When the Console proxy receives a run-completion event, it checks that run’s finalized file and adds the run to history as soon as the file passes the retention checks below. This check reads only the affected run. If the event arrives before finalization or the proxy misses it during a disconnect, a subsequent periodic scan discovers the eligible file.

Each entry records the entry namespace, root session ID, trace ID, root skill, start time, elapsed time, outcome, and file location. An application administrator can use the top-level `sessionId` returned to the invoker to find the retained trace within that application's history. A run appears under the initiating application even when it contains child calls. The history list uses these summaries. When an authorized caller opens a run, Console loads the selected trace for detailed analysis. Manually uploaded traces remain in the application's separate uploads view.

For each completed file in an entry-namespace directory, read `persistencePolicy` and `errored` from its terminal `TRACE_COMPLETED` record. Index `ALWAYS` traces and `ONERROR` traces with `errored: true`. Skip `NEVER` traces and `ONERROR` traces with `errored: false`, including files still present during Console’s completion grace period. The recorded policy belongs to that run and remains authoritative across application settings changes. Verify administrator permission for the application before serving an indexed trace. [Writer completion signal](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces)

With execution views off and trace retention enabled for the test runs, invoke the same skill concurrently. Verify that an administrator can use each caller's returned `sessionId` to find the corresponding retained trace in the application.
