# Loomspan Cloud — App Lifecycle

## Status and purpose

This is a living record of the lifecycle discussion following the [namespace design](005-skill-namespaces.md). It builds on the [micro app design intent](001-micro-app-design-intent.md) and [platform component outline](002-platform-components-and-responsibilities.md).

The document distinguishes current product directions from tentative mechanisms and open questions. It is not a finalized specification or implementation plan. Later clarifications here supersede earlier suggestions where explicitly noted, particularly Share Nothing departure behavior and sandbox refresh.

No framework or sidecar code has been inspected as part of this discussion. Existing isolation, identity propagation, execution-version handling, and timeout capabilities described by the project owner remain to be verified.

## 1. Responsibility boundary

Loomspan Cloud owns the app lifecycle: sandbox and production management, publication permissions and restrictions, user registration and departure, data retention, and usage/tier management.

Loomspan Framework remains a general execution host. Its generic namespace, configuration, execution, and security capabilities support Cloud without acquiring Cloud-specific lifecycle rules.

## 2. Sandbox, production, and promotion

### Current direction

Apps have sandbox and production environments so they can be exercised before production publication. Production publication proceeds through sandbox rather than allowing direct publication of app definitions to production.

Production starts empty: sandbox test records are not promoted into production. Later publications update the existing production app and preserve its data except where approved schema changes intentionally remove it. Intentional starter/reference data has not been specified.

The tentative implementation is to publish the app twice, for example with `_sandbox` and `_prod` identifiers, and provide a Cloud operation to promote the sandbox definition to production. The suggested name `publishSandbox` is not a finalized API.

The assistant noted that separate environment publications may refine the earlier one-namespace-per-app mapping into one namespace per app environment. That mapping, identifier scheme, and its relationship to the app's logical identity still need confirmation.

### Sandbox refresh

The project owner explicitly agreed that there should be a way to refresh sandbox with **the owner's data from production**. This supersedes the earlier suggestion that sandbox refresh would be unnecessary if all production publications originated in sandbox.

This does not establish permission to copy every participant's production data. The exact meaning of the owner's data, particularly under Share Everything or where records reference shared content, remains to be defined.

### First publication

The companion prepares the app in sandbox, where its definitions and behavior can be validated and exercised. Cloud promotes a specific tested revision, creates the production schema, and installs the corresponding skills and configuration. Production becomes available only after successful publication, with no sandbox test data copied into it.

The exact required checks, artifact packaging, and method of identifying the tested revision remain open. The direction is to promote the tested artifacts, not silently include subsequent sandbox edits.

### Supported V1 schema changes

The current scope is deliberately limited:

| Change | V1 direction |
|---|---|
| Add tables or columns | Supported. |
| Rename tables or columns | Supported as renames that preserve the associated data. |
| Remove tables or columns | Supported, with destructive effects made explicit during publication review. |
| Increase a `varchar` column's size limit | Supported. |
| Other data-type changes or shrinking a `varchar` limit | Outside the selected V1 scope. |
| Uniqueness constraints | Enforced in the database and included in DDL changes. |

The brief additive-only proposal was superseded: renames and removals remain supported. There is no promise that older companion artifacts remain compatible with a new publication.

Most application constraints, relationships, and business logic should live in configuration and the DataSkill implementation rather than database logic. Uniqueness is an explicitly identified exception because it must remain correct under concurrent writes. Exact uniqueness scope, constraint-edit operations, and treatment of existing duplicates still require design.

Publication review should identify destructive changes so the user understands and approves the data loss. The precise review/approval interaction is not yet designed. Arbitrary data transformations and automatic rollback are not committed capabilities.

### Rehearsing schema changes

Before promoting a schema-changing revision, the intended workflow is to refresh sandbox from production, apply the proposed DDL there, and allow the user and companion to verify the resulting app before publishing.

The assistant proposed preserving the candidate definitions and migration before refresh, reconstructing the production schema baseline with the owner's permitted data, then applying the candidate DDL and matching skills/configuration. This is a way to test the transition rather than merely test the final schema; its exact refresh and revision mechanics remain open.

Promotion applies the tested changes to production, rather than copying the modified sandbox database over production. Owner-only sandbox data may not expose every case present in other participants' records, so rehearsal is not a guarantee that all production checks will pass.

The assistant recommended that Cloud generate DDL from supported change operations, rather than accept arbitrary companion-authored DDL. That implementation approach has not been separately finalized.

### Later publication: stop admission, drain, publish, reopen

The agreed publication model accepts temporary unavailability for the affected app:

1. Stop accepting new application operations for that app.
2. Let already accepted queued and running work drain before changing the production schema or definitions.
3. Apply the schema changes, if any, and publish the matching skills and configuration.
4. Activate the new app version and reopen access only after publication succeeds.

Other apps are not intended to be closed by this app's publication. The gate mechanism and exact ordering of version activation, verification, and reopening remain implementation details.

The project owner believes the framework already enforces timeouts, so accepted work should finish or time out. The implementation should be inspected and reused rather than assuming a new timeout mechanism is needed. In particular, confirming that timed-out work no longer mutates data is part of verifying safe drainage, not a completed finding.

The assistant recommended returning a temporary publishing response for new operations instead of holding them for execution after the schema changes. Exact response codes, payloads, and retry guidance have not been selected.

### App versions and companion refresh

The app must expose a version that the companion can compare with its installed local artifacts. After a new publication, an outdated companion receives a response directing it to refresh those artifacts.

The accepted version-check direction is to check the companion's app version before executing an operation. An incompatible/outdated request is rejected without performing the requested application operation; the companion refreshes and submits a request appropriate to the new definition. This avoids applying an old request and only then reporting that its instructions were stale.

The proposed simple V1 policy requires the current app version rather than maintaining older operation contracts. Version identifiers, request/response fields, download mechanics, and handling of absent version information remain open. Retention of configuration for already running work does not establish support for new calls against old app definitions.

### Asynchronous publication status and failure

Publication is an asynchronous Cloud event. The owner can track its status in the Cloud dashboard, and the companion can check the status.

The agreed status model is:

```text
Publishing -> Published
Publishing -> Error
```

If publication partially fails, the app remains unavailable until the owner addresses the issues through their companion. Cloud must not reopen an app with mismatched schema and definitions merely because some publication steps succeeded. Automatic rollback is not promised.

The event identifier, status API, diagnostic payloads, persistence, repair/retry operations, and behavior after a Cloud process restart remain open. Administrative status and repair access must be distinguished from the unavailable app-operation path; the exact interfaces are not defined.

### Publication and refresh details still open

- What automated checks and user verification are required before promotion?
- How is the tested revision fixed, and how is a changed production baseline detected?
- Which artifacts form a publication, and how are they coordinated with database DDL?
- How are duplicate or concurrent publication submissions handled in the single-instance V1 deployment?
- How are publication progress and partially applied changes recorded for owner-assisted repair?
- Does sandbox refresh replace or merge data, and how are existing experiments preserved?
- How is refresh coordinated with active sandbox work and production data changes?
- Who can access sandbox, and how is its data kept separate from production?
- How are configuration-only changes that invalidate existing records checked?

## 3. Allowed published capabilities

Cloud will restrict the skill types users may publish. The project owner identified Data Skills and TypeSafe skills as likely initial supported types, with additional types possible later. The exact initial list remains to be finalized.

These are Cloud restrictions, not restrictions on the general capabilities of Loomspan Framework. The assistant recommended checking allowed types when accepting app definitions; enforcement locations and checks have not been designed.

The project owner expects existing framework boundaries to help keep skill executions separate and believes the framework tracks calling-user information. The sidecar's JWT propagation was identified as something to inspect and learn from.

Cloud will not use loomspan-sidecar for this project. Its implementation can be a reference for supplying trusted identity to JavaSkills, including the DataSkill implementation, without adopting the sidecar as the Cloud runtime.

## 4. V1 interaction boundary

For V1, micro apps operate only in response to AI companion requests.

Other supported interaction types may be added to Cloud later. Cloud-specific interaction features should remain outside the generic framework. No initial scheduling, event-triggered app execution, or background app-action capability has been selected.

This boundary describes micro app interactions; it does not prohibit Cloud's own lifecycle administration, such as asynchronous publication or archiving data.

Multiple Cloud instances are explicitly outside V1. Publication admission and drainage do not need a distributed multi-instance design for the initial release. Single-instance restart and recovery behavior still needs definition.

## 5. App registration and participation

The project owner wants app interaction tied to an app registration on the user's profile, making departure an explicit event Cloud can track.

The exact registration trigger is open. In particular, the assistant suggested distinguishing discovery from participation; that distinction has not been finalized. How registration relates to invitation acceptance, access checks, and the first authenticated operation still needs design.

App ownership and participation are related but distinct: a participant may use the app under its data strategy, while ownership controls changes to the shared app definition.

## 6. Leaving an app and data retention

These behaviors belong to Cloud and depend on the selected data strategy.

| Strategy | Current departure direction |
|---|---|
| Share Nothing | Archive the departing user's data when they leave, since other users could not see it anyway. |
| Owner Edit / Everyone Read | A departing user can choose whether their contributed data is removed or retained for others. |
| Share Everything | Shared data remains when a participant leaves. |

The Share Nothing rule is an explicit correction from the project owner. It replaces the earlier idea of asking whether that private data should remain for other participants.

“Archive,” “remove,” and “clean up” are lifecycle intentions here, not defined storage operations or guarantees about recovery. Archive location, retention duration, access, eventual deletion, and behavior upon rejoining remain open.

### When the app owner leaves

For Owner Edit / Everyone Read, the project owner clarified that the primary user/app owner can still have their own data cleaned up when leaving. Remaining participants can continue using the app as it stands, but no future changes to the shared app definition would be possible without an owner.

This means continued use under the existing data strategy; it does not freeze all participant data or grant participants authority to redesign the app.

The project owner proposed a **possible ownership-transfer option**: the departing owner could promote another remaining participant to become the app owner. This is a candidate feature, not yet a finalized requirement or transfer workflow. It would be an explicit evolution of the earlier creator-only editing rule.

How owner departure behaves under other strategies, how retained records owned by a departed participant can be maintained, and what happens if no successor is selected remain to be worked through. The stated Owner Edit behavior should not silently be generalized to all cases.

### When no registered participants remain

The project owner's direction is that all app data can be archived and removed once nobody remains registered to the app.

What removal means operationally, when it occurs, and whether the app definition itself is retained have not been decided. Restoration or later reactivation has not been promised.

## 7. Usage, tiers, and dashboard

Cloud will track usage from the outset and have free and paid tiers. Tier limits are not yet defined. The project owner expects usage will likely be displayed on the user's Cloud dashboard; the dashboard presentation remains tentative.

The assistant recommended separating usage measurement from tier policies so allowances can change independently. That is a proposed implementation principle, not an agreed metering design.

Open questions include which resources are measured, who bears usage for shared-app operations, how sandbox usage is accounted for, and how limits are enforced. No pricing, quotas, billing mechanism, or attribution model has been selected.

## 8. Connections to earlier lifecycle ideas

Earlier discussions identified change history, companion-assisted backups, and forking as intended directions. They remain relevant but are not resolved by sandbox/production support.

- The Git-like workflow is meant to help companions understand and revise changes; no automatic rollback capability has been committed to.
- Readable backups are intended to assist with data repair; backup contents and recovery behavior remain open, and dedicated restore was not expected initially.
- Forking is intended to let users evolve or freeze an app independently; copied artifacts, data, membership, and version semantics remain open.
- Sandbox refresh is a specific requested capability and should not be treated as a general backup restore or fork operation.

## 9. Suggested next design work

The assistant suggested walking through one app's lifecycle: sandbox creation, testing, promotion, invitation and registration, ordinary use, a schema change, sandbox refresh, participant departure, and owner departure.

That exercise can clarify the publication and data boundaries before selecting APIs or storage mechanisms. It is a proposed next discussion, not a completed design or additional feature commitment.
