# Loomspan Cloud Roadmap

## Ground rules

Only add items we have explicitly discussed and agreed to include. Use the wording we agree on for each feature. Examples and supporting work belong in the roadmap only when we explicitly agree to include them.

## Feature structure

Give each feature its own section describing what it does. Use its “Relationship to other components” subsection to identify which module owns the functionality, which internal APIs other modules use to reach it, and where users or their companions can access it. Record these boundaries as we agree on them.

This structure supports our goal of building Loomspan Cloud as a modular system. We want to be able to upgrade and maintain each component individually, with a clear definition of how the components work together.

## Cloud overview

Users and their AI companions create applications, submit work, and inspect results. Cloud manages where each application runs. Its main components are the Java execution engine, Go backend, and administration UI.

The [architecture diagram](loomspan-cloud-architecture.png) shows the V1 component boundaries. Start component scoping at the [first single-engine milestone](java-execution-engine-roadmap.md#first-single-engine-milestone). The [Go roadmap](go-backend-roadmap.md) owns backend workflows, the [package specification](publication-package-format.md) defines valid archives, and the [V1 brief](v1-prototype-brief.md) describes the end-user demonstration.

### Guiding principle: an AI-first cloud

Design Loomspan Cloud first for people working through their own AI companions. Users should be able to describe what they want to accomplish and rely on their agent to act on their behalf. They should also be able to work directly in the web UI. This priority applies whether an agent uses REST or MCP.

**Discovery and setup.** A user should be able to ask their companion “What is Loomspan, and can it help with my problem?” After the user describes the problem, the agent should be able to help them get set up, create their application and its execution logic, configure its skills and runtime, and take it live.

**Runtime and monitoring.** A user should be able to ask “How is my system running?”, “What is it doing right now?”, “How much work is running?”, or “Are there any errors?” The agent should have access to the operational information it needs to investigate the user’s questions and answer them directly. Getting an answer through the companion should be easier for the user than signing in to the administration UI and finding it themselves.

### Start with the simplest version of each component

Choose technologies and an architecture we expect to keep as Loomspan Cloud grows. On that foundation, build the smallest, simplest version of each component that meets its current requirements. Add complexity in later iterations when a concrete need calls for it.

### Keep the framework independent of its host

Loomspan Framework is the common core. An application can embed it directly, Loomspan Sidecar can run it alongside another application, and Loomspan Cloud can embed it in a shared execution engine. Changes made for the cloud must preserve all three uses.

### Internal APIs and external interfaces

We expect three external interfaces: a web UI for people working directly, REST for programmatic access, and MCP for AI companions. Each may expose a different set of features.

All externally accessible operations should pass through a shared internal API before reaching the underlying platform. This is where Loomspan Cloud should enforce permissions and the rules for administration and debugging. For example, a request to read an application’s traces should go through the same access check whether it comes from the web UI, REST, or MCP.

Within the platform, modules also communicate through internal APIs. Each module owns its functionality and defines how other modules can call it.

### Platform authentication and database

V1 uses hosted Supabase Auth for sign-in and hosted Supabase Postgres as Loomspan Cloud’s internal database. Cloud uses the database for platform records such as application memberships and role assignments. Customer applications do not receive a database service as part of this decision.

Development, QA, and production will use K3s so the Go backend and Java engines use the same deployment model in every environment. Oracle Cloud Always Free compute remains a hosting option.

### Preferred languages and frameworks

The execution engine uses Java to embed Loomspan Framework. For the cloud interfaces and diagnostics, we favor the stack already used by Loomspan Console.

| Component | Preferred stack | Rationale |
| --- | --- | --- |
| Administrative UI | React, TypeScript, and Vite | Matches the Console web stack and lets us serve the UI as static assets backed by the cloud API. |
| MCP server | Go with the official MCP Go SDK | Console already uses this SDK. Cloud MCP tools can call the same authorized operations as REST and the web UI. |
| Console proxy and aggregator | Go | Console already interprets traces in Go. Cloud can use the same approach to find and explain an application's runs across engines, subject to the user's permissions. |

Our starting preference is one Go backend deployment with separate packages for REST, MCP, and diagnostics. The Java execution engine can be deployed independently. Console's existing Go packages are internal. Following one operation, such as retrieving a trace for an authorized application, should help us decide which code is worth extracting into a shared package.

Security is a primary selection criterion. We want to choose technologies we can secure and design the modules so a security audit can give us reasonable confidence in the security of the cloud as a whole.

## Authentication

In V1, users sign in by email magic link and can authorize an AI companion to act on their behalf. The companion can do everything the user can do. The shared Cloud API enforces the user’s current permissions for each operation.

We prefer one authentication strategy across the web UI, REST, and MCP wherever practical. If an interface needs a different method to be usable, we want to support as few methods as possible and keep each one simple. The goal is an authentication system we can understand and secure.

### Application owners, users, and roles

Each cloud application has an owner who manages access to it. Cloud will support two ways to invoke the application on behalf of an end user.

- **Direct authentication.** The owner defines application roles and can assign them to an email address before the user first signs in. The user gains those roles when they verify that address through the V1 magic link. Verified email identifies the user for membership purposes across supported sign-in methods. Cloud stores memberships and roles per application in Supabase and checks them when the user invokes the application.
- **Authentication by a calling application.** An application that uses Loomspan Cloud as its AI execution engine can authenticate its own end users and determine their roles. Its backend authenticates to Cloud and supplies its own stable ID for the end user along with that user's roles on each invocation. Cloud accepts the ID and roles only from a backend the owner has authorized to assert them for the target application. The user ID is scoped to that backend.

### Relationship to other components

MCP companions obtain their own tokens after the user approves the connection in a browser. Whether REST clients also need API keys remains open. Regardless of how a caller authenticates, the shared internal API should apply the same permission rules.

In both models, the shared Cloud API checks permission to invoke the target application before routing to its namespace. The Java engine wrapper establishes the end user's identity and application roles in the Framework security context. Framework then enforces each skill's role requirements at entry and when calling child skills. Cloud separately enforces management permissions such as publishing the application or managing its users.

Internal engine requests require authenticated Go client identity, including requests originating inside the engine pod. The [engine integration contracts](execution-engine-api.md#integration-contracts) define transport authentication and invocation metadata.

## Execution engine

The [Java execution engine roadmap](java-execution-engine-roadmap.md) owns the design of the disposable Spring Boot engine and its first single-engine milestone. The Go placement controller sends publications, the Console proxy observes installed versions, and the router sends authorized invocations. See the [engine integration contracts](execution-engine-api.md#integration-contracts) for the responsibilities on each side of these exchanges.

### Cloud-managed engine lifecycle

The Go backend keeps the original uploaded ZIP on a durable mounted filesystem. Each publication refers to an immutable ZIP containing the skills and portable execution configuration. Supabase Postgres holds the application's publication history, the filesystem reference and status for each publication, and the identity of the current publication. This keeps the pack available for download and engine recovery independently of the engine cluster and keeps pack size and history from driving Supabase database usage.

Cloud owns namespace placement. Each publication has a durable opaque version that follows its immutable ZIP across engine instances.

### Starting point for placement and routing

V1 starts with one engine. Go reconciles intended placements with observed publications and routes authorized invocations to confirmed instances. [Go placement and recovery](go-backend-roadmap.md#observe-live-engines)

K3s ingress could direct public requests to the Go backend through a Kubernetes Service. The ingress design remains open.

An ingress NetworkPolicy on the engine pods will restrict pod-to-pod access to the engine API port to Go management backend pods in the management namespace.

On K3s, cert-manager will provision and rotate the mTLS certificates for communication between the Go management backend and the Java engines. The Go backend will use one client credential. All Java engine pods, including new and replacement pods, will use one shared server credential.

### Open questions for a Kubernetes deployment

- **Pod management.** A Kubernetes Deployment is a candidate for maintaining engine pods. Go can publish as soon as the Console proxy reads the first authenticated instance snapshot from the intended engine JVM. Go can route invocations once the proxy confirms the requested namespace and publication version on an eligible engine. Whether K3s needs a readiness probe for rollouts remains open. [Java startup contract](java-execution-engine-roadmap.md#startup-and-readiness)
- **Cluster recovery.** How will a replacement cluster regain its credentials and mount the durable filesystems so the controller can restore applications?
- **Application credentials.** What role, if any, would Kubernetes Secrets have in managing application credentials?

### Durable execution traces

Authorized users and companions can inspect saved traces, including after engine or cluster replacement.

Each engine pod mounts a shared persistent filesystem with the [entry-namespace trace layout](java-execution-engine-roadmap.md#application-diagnostics-and-retained-traces). Verify that completed durable writes survive pod loss and that a replacement cluster can access them. The trace volume must support simultaneous writes from engine pods on different K3s nodes and reads from the Go proxy. Storage provider selection and whether traces and publications share a service remain open.

V1 proceeds with trace expiry and filesystem cleanup deferred, including cleanup of abandoned writes. Measure trace count, bytes per application, and growth over time under real Cloud workloads. Use those observations to decide the retention window, count or storage limits, cleanup ownership, and whether allowances should vary by pricing tier.

The [Go diagnostics roadmap](go-backend-roadmap.md#look-up-retained-traces) defines run history and authorized lookup.

### Tenant isolation and shared capacity

Application owners can publish skills and runtime configuration. Cloud enables only Loomspan extensions it has vetted and configured for the shared engine. Vetting an extension includes verifying that it keeps each tenant's supplied configuration, credentials, and data isolated when multiple applications use it in the same JVM.

Each application can configure a shared executor once for all its skills that use it.

Cloud publishes each customer application into its own private Framework namespace. Even when tenants share an execution engine, the engine must prevent one tenant’s application from calling or interfering with another tenant’s application.

We expect typical workloads to use little CPU or memory. Our goal is to run many tenants’ namespaces in one execution engine and add engines when performance or redundancy requires them. Customer count alone should not drive provisioning. Sharing capacity should reduce the need to pause, time out, or remove idle environments to reclaim the resources a dedicated executor would consume.

### Relationship to other components

Cloud names each application's Framework namespace using `u{numericUserId}_{appSlug}`. The first underscore separates the numeric user ID from the application slug, which may itself contain underscores. For example, user `42` publishing `hello_world` gets namespace `u42_hello_world`. The complete namespace must fit Framework's 32-character limit. In the first release, Cloud treats an application rename as publishing a new application under the new slug.

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

The UI will give users a view of their running applications. The status information and management controls it will offer are still open.

## Loomspan Console in the cloud

Signed-in users should have the full capabilities of Loomspan Console for the applications they are authorized to access. That boundary applies both to the data they can see, including trace records, and to the applications they can debug and manage. It must hold when several tenants share an execution engine.

### Relationship to other components

An engine emits one Console stream across its namespaces. The Cloud proxy scopes access to each authorized application. Standalone hosts retain direct Console access to their engines.

### In-memory projection of observed engine state

Placement, routing, and diagnostics use the [Go proxy’s live engine view](go-backend-roadmap.md#console-proxy). The [engine integration contracts](execution-engine-api.md#integration-contracts) define its snapshot and stream.

## API

The API will let users submit work and will likely also provide status and metrics for the work they have running. We expect to expose a REST endpoint.

### Synchronous invocation

In the first release, clients invoke an application synchronously. Callers can opt in to the [Framework execution view](execution-engine-api.md#invocation-metadata) on each invocation, even when tracing is disabled.

If the API cannot return an execution result, it will distinguish two outcomes.

- **Rejected before execution** means the cloud can confirm that the skill never started. If no engine is ready to serve the application's requested version, Cloud promptly reports that the application is temporarily unavailable. The caller can safely retry that invocation.
- **Outcome unknown** means the cloud dispatched the invocation and lost contact with the engine before it could confirm the result. The skill may have completed or produced side effects. Retrying could repeat them.

Framework admission is single-use and local to the engine process. We will defer a durable job queue until there is a requirement for accepted work to survive engine loss.

### Relationship to other components

Users will submit work to an application through the API. Internally, an execution engine will run that work. We also expect to support authenticated MCP access so users can work with their Loomspan Cloud configuration through a coding agent or assistant.
