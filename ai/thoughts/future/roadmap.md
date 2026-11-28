# Loomspan Cloud Roadmap

## Ground rules

Only add items we have explicitly discussed and agreed to include. Use the wording we agree on for each feature. Examples and supporting work belong in the roadmap only when we explicitly agree to include them.

## Feature structure

Give each feature its own section describing what it does. Use its “Relationship to other components” subsection to identify which module owns the functionality, which internal APIs other modules use to reach it, and where users or their companions can access it. Record these boundaries as we agree on them.

This structure supports our goal of building Loomspan Cloud as a modular system. We want to be able to upgrade and maintain each component individually, with a clear definition of how the components work together.

## Cloud overview

Users and their AI companions create applications, submit work, and inspect results. Cloud manages where each application runs. Its main components are the Java execution engine, Go backend, and administration UI.

The [architecture diagram](loomspan-cloud-architecture.png) shows the V1 component boundaries. The [Go roadmap](go-backend-roadmap.md) owns backend workflows, the [package specification](publication-package-format.md) defines valid archives, and the [V1 brief](v1-prototype-brief.md) describes the end-user demonstration.

### Guiding principle: an AI-first cloud

Design Loomspan Cloud first for people working through their own AI companions. Users should be able to describe what they want to accomplish and rely on their agent to act on their behalf. They should also be able to work directly in the web UI. This priority applies whether an agent uses REST or MCP.

**Discovery and setup.** A user should be able to ask their companion “What is Loomspan, and can it help with my problem?” After the user describes the problem, the agent should be able to help them get set up, create their application and its execution logic, configure its skills and runtime, and take it live.

**Runtime and monitoring.** A user should be able to ask “How is my system running?”, “What is it doing right now?”, “How much work is running?”, or “Are there any errors?” The agent should have access to the operational information it needs to investigate the user’s questions and answer them directly. Getting an answer through the companion should be easier for the user than signing in to the administration UI and finding it themselves.

### Start with the simplest version of each component

Choose technologies and an architecture we expect to keep as Loomspan Cloud grows. On that foundation, build the smallest, simplest version of each component that meets its current requirements. Add complexity in later iterations when a concrete need calls for it.

### Keep the framework independent of its host

Loomspan Framework is the common core. An application can embed it directly, Loomspan Sidecar can run it alongside another application, and Loomspan Cloud can embed it in a shared execution engine. Changes made for the cloud must preserve all three uses.

### Internal APIs and external interfaces

Full Cloud exposes the web UI, REST for programmatic access, and MCP for AI companions. Across all three, the caller's credentials and application permissions determine which operations they can perform. For example, an owner can delete an application through REST, the UI, or MCP with their companion.

All externally accessible operations should pass through a shared internal API before reaching the underlying platform. This is where Loomspan Cloud should enforce permissions and the rules for administration and debugging. For example, a request to read an application’s traces should go through the same access check whether it comes from the web UI, REST, or MCP.

Within the platform, modules also communicate through internal APIs. Each module owns its functionality and defines how other modules can call it. The four components inside the Go backend use in-process calls. The Java engine and the scheduled cleanup job communicate with Go across process boundaries.

V1 Cloud performs no content sanitization or redaction, including in logs, diagnostics, and API responses.

### Platform authentication and database

V1 uses Supabase Auth for sign-in and Postgres for Loomspan Cloud’s internal records, including application memberships and role assignments. The operator supplies the Auth and database endpoints at installation. Operators can use hosted Supabase or run Supabase Auth and Postgres inside their own network. For an internal installation, the operator provisions and maintains both services and configures delivery of one-time email sign-in codes.

The Go backend uses a native PostgreSQL connection for migrations and runtime database operations in both hosted and internal installations. [Go backend configuration](go-backend-roadmap.md#go-backend-configuration)

Development, QA, and production will use K3s so the Go backend and Java engines use the same deployment model in every environment. Oracle Cloud Always Free compute remains a hosting option.

### Preferred languages and frameworks

The execution engine uses Java to embed Loomspan Framework. For the cloud interfaces and diagnostics, we favor the stack already used by Loomspan Console.

| Component | Preferred stack | Rationale |
| --- | --- | --- |
| Administrative UI | React, TypeScript, and Vite | Matches the Console web stack and lets us serve the UI as static assets backed by the cloud API. |
| MCP server | Go with the official MCP Go SDK | Console already uses this SDK. Cloud MCP tools can call the same authorized operations as REST and the web UI. |
| Console proxy and aggregator | Go | Console already interprets traces in Go. Cloud can use the same approach to find and explain an application's runs across engines, subject to the user's permissions. |

The Java execution engine can be deployed independently. Loomspan Cloud will own the shared Console. Its two deployment profiles use the same diagnostic behavior across REST, MCP, and the browser. [Console deployment profiles](#console-deployment-profiles)

Security is a primary selection criterion. We want to choose technologies we can secure and design the modules so a security audit can give us reasonable confidence in the security of the cloud as a whole.

## Authentication

A person working with their AI companion uses one Cloud identity through the web UI, REST, and MCP. The same application memberships and permissions apply across all three. Connecting through MCP requires no separate Loomspan account, role assignment, or manual client registration by the user or operator. The web UI and MCP use the same browser sign-in flow. A user visiting the web UI signs in with a six-digit code sent to their email address. Connecting a companion works as follows.

1. The companion starts an authorization request for the installation's MCP endpoint and opens the browser. The page names the requesting companion and explains that it will act with the user's current permissions.
2. If the user is signed out, they enter their email, receive a six-digit code, and enter it in the same tab. Selecting **Sign in and connect** verifies the email and connects the named client in one submission. If the browser already has a valid Cloud session, the page shows the signed-in email and offers **Connect [companion name]**. One click connects using that session.
3. Once connected, the browser redirects to the companion's callback with an OAuth authorization code. The companion exchanges that code with the installation's Supabase issuer for its own access token. The user enters the email code only in the browser and does not copy credentials into the companion.
4. The companion automatically calls the [application-listing operation](go-backend-roadmap.md#application-permissions). Go verifies the token and uses the verified email to look up current memberships. Each entry identifies the application, namespace, and user's Cloud role. Subsequent operations use the companion's token and check current application permissions even if membership has changed since discovery.

We prefer one authentication strategy across the web UI, REST, and MCP wherever practical. If an interface needs a different method to be usable, we want to support as few methods as possible and keep each one simple. The goal is an authentication system we can understand and secure.

### Application owners, users, and roles

Cloud applies the per-application [ownership and role rules](go-backend-roadmap.md#application-permissions) to two forms of invocation.

- **Direct authentication.** The owner can assign a Cloud role and separately grant owner-defined skill roles to an email address before the user first signs in. Invitation notifications are outside V1 scope. The user gains those permissions when they verify that address with the one-time email code. Verified email identifies the user for membership purposes across supported sign-in methods. Cloud stores memberships and roles per application in Supabase and checks them when the user invokes the application.
- **Calling application.** A third-party backend authenticates to Cloud and asserts its end user's identity and skill roles under the [application API key contract](go-backend-roadmap.md#application-api-keys).

### Relationship to other components

Each Full Cloud installation trusts one dedicated Supabase Auth issuer, which signs user access tokens with an asymmetric key. Go retrieves public verification keys from the trusted issuer configured for the installation and verifies the signature, issuer, intended audience, and expiry locally. A replacement Go backend retrieves those public keys using the restored configuration.

Full Cloud MCP companions obtain their own Supabase access tokens through [the shared sign-in flow](go-backend-roadmap.md#companion-sign-in). Browser, REST, and MCP share token verification. For MCP, the installer derives the expected audience from the installation's public URL using the canonical public MCP endpoint URL. Go requires the verified token's audience to match that URL or include it in the audience array. For each companion operation, Go checks the user's current permission for the requested action. Personal API keys and unattended scripts that manage applications on a user's behalf are outside V1.

The issuer and audience design above is settled for V1. Backend tickets can use this contract directly without a separate design decision on the literal Supabase audience for browser or REST requests.

In both models, the shared Cloud API checks permission to invoke the target application before routing to its namespace. Go uses Cloud ownership and membership to authorize Cloud operations. The Java engine wrapper receives the end user's identity and only the skill roles for the target application. Framework then enforces each skill's role requirements at entry and when calling child skills. Cloud separately enforces management permissions such as publishing the application or managing its users.

Internal engine requests require authenticated Go client identity, including requests originating inside the engine pod. The [engine integration contracts](execution-engine-api.md#integration-contracts) define transport authentication and invocation metadata.

### Authentication acceptance criteria

- Starting with a supported companion and the installation's MCP URL, exercise the browser handoff above both signed out and with a valid Cloud session. Verify the named companion connects in one action in each case and only the signed-out user needs the emailed code. Complete both flows through callback, token exchange, and automatic application discovery using the companion's own token, with no separate account, manual client registration, or companion-specific role assignment.
- Use an account that owns one application and is a Caller in another. Verify discovery lists both namespaces with the correct Cloud role and omits inaccessible applications, without asking the user to configure namespaces or roles.
- Verify the access token issued by the configured sign-in flow contains the derived MCP audience and permits an authorized MCP operation. Go rejects an otherwise valid token from the trusted issuer before dispatch when that audience is missing or wrong. Go also rejects a token from another installation's issuer.
- Verify a Manager can perform a Manager operation for the target application. On skill invocation, verify the engine receives only the user's assigned skill roles for that application, including an empty roles array when the user has none.

## Execution engine

The [Java execution engine roadmap](java-execution-engine-roadmap.md) owns the design of the disposable Spring Boot engine. The Go placement controller sends publications, the Console proxy observes installed versions, and the router sends authorized invocations. See the [engine integration contracts](execution-engine-api.md#integration-contracts) for the responsibilities on each side of these exchanges.

### Cloud-managed engine lifecycle

The Go backend keeps the original uploaded ZIP on a durable mounted filesystem. Each publication refers to an immutable ZIP containing the skills and portable execution configuration. Supabase Postgres holds the application's publication history, the filesystem reference and status for each publication, and the identity of the current publication. This keeps the pack available for download and engine recovery independently of the engine cluster and keeps pack size and history from driving Supabase database usage.

Cloud owns namespace placement. Each publication has a durable opaque version that follows its immutable ZIP across engine instances.

### Starting point for placement and routing

In V1, the installation’s single engine is the intended placement for every application. For an authorized invocation with requests enabled, Go rejects the request before dispatch if the target application ID has no recorded current publication. This applies even when the engine still hosts an empty generation from a deleted application with the same namespace. Once Go records the application's first publication, the router uses the Console proxy’s live engine view. It forwards the request when the assigned engine is ready, connected, accepting requests, and observed to host the application’s namespace. If no engine meets those conditions, Go reports that the application is temporarily unavailable.

Each Full Cloud installation exposes the browser UI, REST API, and MCP endpoint on one hostname through a single HTTPS ingress on K3s. The ingress routes all three interfaces through a Kubernetes Service to the Go backend. Go checks caller permissions on each Cloud operation. The Java engine API stays internal. Configure ingress timeouts to allow synchronous invocations to run for their intended duration.

An ingress NetworkPolicy on the engine pods will restrict pod-to-pod access to the engine API port to Go backend pods in the management namespace.

On K3s, cert-manager will provision and rotate the mTLS certificates for communication between the Go backend and the Java engines. The Go backend will use one client credential. All Java engine pods, including new and replacement pods, will use one shared server credential.

On a full cluster replacement, the installation configures Go and the Java engines to trust the replacement installation’s issuer. Cert-manager issues a fresh Go client credential and a fresh shared engine server credential. The operator can recover the installation without the previous client and server private keys.

On a full cluster replacement, the installer also generates a fresh Console API key for the environment and writes it to the Kubernetes Secret shared by the Go Console proxy and the Java engines. An ordinary pod replacement reuses the existing Secret. [Console authentication contract](execution-engine-api.md#integration-contracts)

A single-replica Kubernetes Deployment maintains the V1 engine pod. It uses the `Recreate` strategy so a planned rollout terminates the old pod before starting its replacement. Go restores the intended publications on the replacement JVM.

During rollouts, K3s counts an engine pod as ready once its readiness probe confirms that the JVM can serve its Console instance snapshot, even with zero applications installed. [Java startup contract](java-execution-engine-roadmap.md#startup-and-readiness)

### Durable execution traces

Full Cloud uses one shared `ReadWriteMany` volume with separate directories for engine-written traces, manually uploaded traces, staged or published skill packs, [application invocation and operations logs](go-backend-roadmap.md#application-operations-log), and [namespace skill catalogs](go-backend-roadmap.md#namespace-skill-catalog). The storage provider encrypts the entire volume when encryption is enabled. A fresh V1 installation provisions an encrypted Longhorn volume by default. An operator can supply a compatible shared volume with or without encryption. The engine writes to the [engine trace directory](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces). Go mounts the engine trace directory read-write so its [application deletion routine](go-backend-roadmap.md#application-permissions) can remove the deleted application's trace directory. Go also has write access to the uploads, packs, application log, and catalog directories. The provider must support concurrent access across K3s nodes, same-directory atomic rename, and exclusive file locks honored across nodes. Verify locking on the mounted volume using pods on two different nodes: while one pod holds an exclusive lock, the other must be unable to acquire it on the same file. After release, the second pod must be able to acquire it.

The volume also holds the [Go backend configuration directory](go-backend-roadmap.md#go-backend-configuration) and the separate [operator-only key recovery directory](go-backend-roadmap.md#encryption-at-rest). Include both in the protected off-cluster volume backup so each backup captures the configuration alongside the data.

For the default encrypted Longhorn volume, the operator generates a volume passphrase, saves it in a password manager accessible after cluster loss, and enters it at a hidden installer prompt. Before provisioning the volume, the installer creates the Kubernetes Secret referenced by the encrypted StorageClass. On a replacement cluster, the operator supplies the same saved passphrase to recreate the Secret before mounting the surviving volume or restoring its encrypted backup.

For example, an operator with an internal NFS server can export a directory for traces, bind a Kubernetes persistent volume for that export to a `ReadWriteMany` claim, and supply the claim at installation. The operator chooses whether the storage backing the NFS export is encrypted at rest.

To migrate the shared volume, pause writers, copy or restore all directories, and switch the workload mounts. Verify that Go starts with the restored settings. Verify a stored pack, a manual upload, historical trace lookup, and a newly completed trace before retiring the old volume. Verify that a replacement cluster can recover the files using an off-cluster backup and any required volume key.

The [Full Cloud cleanup job](go-backend-roadmap.md#trace-cleanup-job) expires manual uploads, trims retained engine traces, and requests pack-history trimming from Go by application ID. Only Go needs the pack directory mounted for retention.

The [Go diagnostics roadmap](go-backend-roadmap.md#look-up-retained-traces) defines run history and authorized lookup.

### Tenant isolation and shared capacity

Users with Manager access can publish skills and runtime configuration. Cloud enables only Loomspan extensions it has vetted and configured for the shared engine. Vetting an extension includes verifying that it keeps each tenant's supplied configuration, credentials, and data isolated when multiple applications use it in the same JVM.

Each application can configure a shared executor once for all its skills that use it.

Cloud publishes each customer application into its own private Framework namespace. Even when tenants share an execution engine, the engine must prevent one tenant’s application from calling or interfering with another tenant’s application.

We expect typical workloads to use little CPU or memory. Our goal is to run many tenants’ namespaces in one execution engine and add engines when performance or redundancy requires them. Customer count alone should not drive provisioning. Sharing capacity should reduce the need to pause, time out, or remove idle environments to reclaim the resources a dedicated executor would consume.

### Relationship to other components

Cloud assigns each application a private namespace of the form `u{numericUserId}-{appSlug}`, such as `u42-order-support`. The complete namespace follows the [publication naming rules](publication-package-format.md#namespace-and-skill-names). In V1, renaming an application means publishing a new application under the new slug.

## Landing page and progressive discovery

The Loomspan Cloud landing page should briefly explain what the service is, why someone might use it, and the kinds of problems it can solve. Pricing can follow once we have built and used the service. The page could also offer a prompt visitors can give their own AI companion, such as “Here’s the problem I’m trying to solve. Where would Loomspan Cloud help, and can you show me an example?”

The website should give the companion a short introduction and show it what further information is available and where to find it. As the visitor asks follow-up questions, the companion should be able to move from explaining a strength of Loomspan Cloud to showing how it applies to the visitor’s problem. This is the role we want the companion to play in a prospective user’s decision: it can make the case for Loomspan Cloud through examples that matter to that person. Hosted skills are one possible way to publish this material. The format is open.

### Relationship to other components

Discovery should reach from a worked example to the API calls a companion could use to carry it out. It should also let the companion explain what debugging traces and runtime statistics are available. When the user asks about their own running application, the companion should be able to follow that same path to the traces and statistics the user is authorized to see.

## First application with an AI companion

When a visitor says “Let’s try it,” their companion should be able to continue in the same conversation. Within a few minutes, it should be able to set up the user’s environment, create and publish a small first application, and show it working.

### Relationship to other components

The companion should be able to move directly from discovery to setup, call the published application through the cloud API, then read the debug logs and execution traces for that run through the cloud console using the user’s access. It should be able to explain how the application produced the result while the conversation is still about the example. The goal is for the user to see that the same companion can manage the application from creation through debugging, so trying the next idea feels as easy as asking.

## Administration UI

The web UI will give visitors a place to learn what Loomspan Cloud is and sign up. Users will be able to manage their billing there.

### Relationship to other components

The UI shows users with Manager access the [application status reported by the Cloud API](go-backend-roadmap.md#application-status). The design of the management controls remains open.

## Loomspan Console in the cloud

### Console deployment profiles

V1 has one Console with shared diagnostic operations, browser views, and MCP tools. The deployment profile determines how Console obtains authorized access to a target.

- **Full Cloud.** Public and internal deployments use the same Cloud profile. The application picker lists only applications where the user has Manager access. A user whose only role for an application is Caller cannot open any Cloud UI or Console screen for that application, including by direct link. Diagnostic access follows the [application permissions](go-backend-roadmap.md#application-permissions) for the selected namespace, even when several applications share an engine.
- **Console-only.** A developer runs the Console executable to inspect a Framework application directly. In V1 the browser UI, REST API, and MCP endpoint listen only on loopback. The browser uses local pairing and MCP uses its own local credential. Console can connect outbound to a remote Framework target using authenticated HTTPS with certificate verification. The operator can inspect every namespace that the connected target exposes to those credentials, including `default` when present. This diagnostic access is independent of Cloud application ownership.

Each browser tab keeps its own selection. Every scoped MCP or REST call explicitly names the application ID in Full Cloud or the namespace on the connected target in Console-only. In Full Cloud, Go resolves the application ID to its namespace and checks the caller's permission for the requested operation. Calls from a companion and selections in other tabs leave the current tab's selection intact.

### Opening a trace file

In both profiles, Console reads the trace's recorded entry namespace and proposes the matching namespace if it exists among the caller's permitted import destinations. Otherwise it proposes the current namespace. The caller can choose any permitted destination. Before saving, show both the recorded namespace and the destination namespace and ask the caller to confirm. Preserve the original trace and its metadata. Store the confirmed destination separately to determine where the upload is visible.

If the destination already has a manual upload with the same trace ID, ask “Overwrite the existing uploaded trace?” Choosing **Overwrite** replaces that upload with the incoming file. Choosing **Cancel** keeps the existing upload and its expiry. The choice applies only to the manual upload in that destination.

At confirmation, show **Retain for** with the choices **1 week**, **1 month**, **1 year**, and **Keep until deleted**. A new upload defaults to **1 week**. An overwrite keeps the existing expiry date or **Keep until deleted** unless the caller explicitly chooses a new retention period. Show the expiry date for a timed choice. Record the upload time at confirmation and calculate any newly selected period from that time. Users with permission to manage the upload can change its expiry after upload or delete it early.

Full Cloud binds each upload to its destination application and checks [upload permissions](go-backend-roadmap.md#uploaded-trace-permissions). Console-only binds it to the connected target and destination namespace. Switching scope shows that scope's uploads. Uploads survive Console restarts and Cloud pod replacement and appear separately from engine-recorded run history. Expiry or manual deletion removes only that saved import, even if its trace ID matches a retained run.

### Console navigation and modules

Console handles a browser disconnect the same way on every screen. It shows “Disconnected” and reloads the current view when the connection returns. The reload reads the current state. Repeating an action requires a new user request.

When Console moves from Loomspan Framework to Loomspan Cloud, design its navigation around complete user workflows. Both profiles use the same diagnostic views and navigation labels. Group Cloud management workflows, such as publications and membership, under a distinct top-level management section.

Each profile selects its modules in one place. Each module contributes its menu entries and routes together, so Console-only can omit the entire management section and its pages. Shared diagnostic screens should work in either profile without scattered profile checks. Within an enabled module, show actions appropriate to the user's permissions and enforce those permissions in the API.

Review the same trace in both profiles as a design check: the path through diagnostics should be recognizable in both, and Console-only should have no empty management menus, disabled placeholders, or links into omitted pages.

### Cloud diagnostics

The [Console proxy](go-backend-roadmap.md#console-proxy) owns application-scoped access to live events and retained runs.

### Relationship to other components

An engine emits one Console stream across its namespaces. The Cloud proxy scopes access to each authorized application. Standalone hosts retain direct Console access to their engines.

### In-memory projection of observed engine state

Placement, routing, and diagnostics use the [Go proxy’s live engine view](go-backend-roadmap.md#console-proxy). The [engine integration contracts](execution-engine-api.md#integration-contracts) define its snapshot and stream.

## API

The API will let users submit work and will likely also provide status and metrics for the work they have running. We expect to expose a REST endpoint.

### Synchronous invocation

V1 invocations are synchronous. The [Go response visibility rules](go-backend-roadmap.md#invocation-response-visibility) define which execution details the caller receives.

If the API cannot return an execution result, it will distinguish two outcomes.

- **Rejected before execution** means the cloud can confirm that the skill never started. If Go has no eligible engine for the application, Cloud promptly reports that the application is temporarily unavailable. The caller can safely retry that invocation.
- **Outcome unknown** means the cloud dispatched the invocation and lost contact with the engine before it could confirm the result. The skill may have completed or produced side effects. Retrying could repeat them.

Framework admission is single-use and local to the engine process. We will defer a durable job queue until there is a requirement for accepted work to survive engine loss.

### Relationship to other components

Users will submit work to an application through the API. Internally, an execution engine will run that work. We also expect to support authenticated MCP access so users can work with their Loomspan Cloud configuration through a coding agent or assistant.
