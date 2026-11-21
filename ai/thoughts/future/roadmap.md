# Loomspan Cloud Roadmap

## Ground rules

Only add items we have explicitly discussed and agreed to include. Use the wording we agree on for each feature. Examples and supporting work belong in the roadmap only when we explicitly agree to include them.

## Feature structure

Give each feature its own section describing what it does. Use its “Relationship to other components” subsection to identify which module owns the functionality, which internal APIs other modules use to reach it, and where users or their companions can access it. Record these boundaries as we agree on them.

This structure supports our goal of building Loomspan Cloud as a modular system. We want to be able to upgrade and maintain each component individually, with a clear definition of how the components work together.

## Cloud overview

Users create applications, submit work to them, and see how they are running. Loomspan Cloud manages where each application runs. Users and their AI companions can work entirely in terms of applications. It will use Loomspan Framework and Loomspan Extensions. We expect to organize the project into sub-projects, each responsible for one part of the cloud. The first three we envision are the execution engine, the administration UI, and the API.

The [proposed architecture diagram](loomspan-cloud-architecture.png) shows how the cloud publishes applications, routes work, and returns diagnostics. The Supabase component shown as a candidate is selected for V1. Compute hosting and trace storage remain open.

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

If needed, we can revisit self-hosting and fork the components that require changes.

One compute hosting option under consideration is to run the REST, MCP, and administration backend and the Java execution engines in a K3s cluster we operate on Oracle Cloud Always Free compute.

### Preferred languages and frameworks

The execution engine uses Java to embed Loomspan Framework. For the cloud interfaces and diagnostics, we favor the stack already used by Loomspan Console.

| Component | Preferred stack | Rationale |
| --- | --- | --- |
| Administrative UI | React, TypeScript, and Vite | Matches the Console web stack and lets us serve the UI as static assets backed by the cloud API. |
| MCP server | Go with the official MCP Go SDK | Console already uses this SDK. Cloud MCP tools can call the same authorized operations as REST and the web UI. |
| Console proxy and aggregator | Go | Console already interprets traces in Go. Cloud can use the same approach to find and explain an application's runs across engines, subject to the user's permissions. |

Our starting preference is one Go backend deployment with separate packages for REST, MCP, and diagnostics. The Java execution engine can be deployed independently. Console's existing Go packages are internal. Following one operation, such as retrieving a trace for an authorized application, should help us decide which code is worth extracting into a shared package.

We prefer to keep the number of languages small. This direction uses three overall because Go gives us a path to sharing Console behavior. Since we expect AI to do most of the coding, the team’s current language experience carries less weight than how well a technology fits a module and how much existing software we can reuse.

Security is a primary selection criterion. We want to choose technologies we can secure and design the modules so a security audit can give us reasonable confidence in the security of the cloud as a whole.

## Authentication

In V1, users sign in by email magic link and can authorize an AI companion to act on their behalf. The companion can do everything the user can do. The shared Cloud API enforces the user’s current permissions for each operation.

We prefer one authentication strategy across the web UI, REST, and MCP wherever practical. If an interface needs a different method to be usable, we want to support as few methods as possible and keep each one simple. The goal is an authentication system we can understand and secure.

### Application owners, users, and roles

Each cloud application has an owner who manages access to it. Cloud will support two ways to invoke the application on behalf of an end user.

- **Direct authentication.** The owner defines application roles and can assign them to an email address before the user first signs in. The user gains those roles when they verify that address through the V1 magic link. Verified email identifies the user for membership purposes across supported sign-in methods. Cloud stores memberships and roles per application in Supabase and checks them when the user invokes the application.
- **Authentication by a calling application.** An application that uses Loomspan Cloud as its AI execution engine can authenticate its own end users and determine their roles. Its backend authenticates to Cloud and supplies the end user's identity and roles with each invocation. Cloud accepts those roles only from a backend the owner has authorized to assert them for the target application.

### Relationship to other components

MCP companions obtain their own tokens after the user approves the connection in a browser. Whether REST clients also need API keys remains open. Regardless of how a caller authenticates, the shared internal API should apply the same permission rules.

In both models, the shared Cloud API checks permission to invoke the target application before routing to its namespace. The Java engine wrapper establishes the end user's identity and application roles in the Framework security context. Framework then enforces each skill's role requirements at entry and when calling child skills. Cloud separately enforces management permissions such as publishing the application or managing its users.

The Go management backend and Java engine authenticate each other with mutual TLS (mTLS). Go verifies the engine's service identity, and the engine requires Go's client identity on its internal API, including for connections from within the engine pod. With each invocation request, Go sends the authorized application namespace, caller identity, and roles for that application separately from the skill input. The engine uses this context for that invocation.

## Execution engine

An execution engine is an internal Loomspan Cloud component that can run many tenants’ namespaces together. Each cloud application maps to a Framework namespace. We will build it in Java so it can use Loomspan Framework directly. We expect to embed the framework in an engine tailored to running inside Loomspan Cloud.

The Java execution engine will be disposable. If we use Kubernetes, we prefer to start with K3s to keep the initial cluster simple to operate. The hosting choice and all integration points between Loomspan Cloud and Kubernetes remain open.

### Cloud-managed engine lifecycle

When an owner publishes an update, the current version keeps serving until Cloud confirms the new version is ready. New invocations then use the new version. Invocations already running finish on the version they started with. If the update fails, the current version stays live and the owner can see that the publication failed.

Loomspan Cloud holds each application's current published skills and runtime configuration independently of the engines and their cluster. The cloud owns the policy for how namespaces run, decides which engines should host each namespace, and tracks which namespaces are actually installed on each engine.

The cloud requests engine capacity and watches for availability. A new engine starts ready to receive configuration. The cloud pushes the complete publication for each assigned namespace through the engine's internal API and confirms it is available there before directing work to it. If an engine or cluster is lost, the cloud can repeat that process on a replacement using the saved publications.

Framework's `SkillReloader.prepare` accepts an optional, opaque `publicationVersion` for a complete namespace update. Cloud will assign one durable value to each complete publication of skills and runtime configuration and reuse it when installing that publication on another engine. After a successful publish, the engine's instance observation endpoint reports the host-supplied version alongside the active Framework `generationId`, which is local to the engine process.

For invocation, the Java wrapper can use `SkillTemplate` with a namespace-qualified skill name. An authorized host can invoke a private namespace directly, so the cloud must check that the caller may invoke the application before routing the request.

### Starting point for placement and routing

Start with one execution engine and two responsibilities in the Go backend. A controller compares the cloud's intended namespace placements with the publications each engine instance has confirmed. It pushes missing publications and treats a replacement instance as empty, including when the Java process restarts within the same pod. A routing function sends an authorized invocation only to an available instance that has confirmed the requested application version.

If we use K3s, one candidate is a Kubernetes Deployment to maintain the engine pool, with the controller watching pods for availability. K3s ingress could direct public requests to the Go backend through a Kubernetes Service. The backend would then select the specific engine for the application. The integration details remain open.

If we use K3s, an ingress NetworkPolicy on the engine pods will restrict pod-to-pod access to the engine API port to Go management backend pods in the management namespace.

On K3s, cert-manager will provision and rotate the mTLS certificates for communication between the Go management backend and the Java engines. The Go backend will use one client credential. All Java engine pods, including new and replacement pods, will use one shared server credential.

### Open questions for a Kubernetes deployment

- **Capacity and replacement.** Would Loomspan request a replica count through a Kubernetes Deployment and let Kubernetes schedule and replace the pods? How would Loomspan discover and address a specific replacement engine?
- **Readiness.** How would Loomspan know a new engine can receive configuration?
- **Cluster recovery.** After cluster replacement, how would the cloud establish contact with new engines?
- **Application credentials.** What role, if any, would Kubernetes Secrets have in managing application credentials?

### Durable execution traces

When tracing is enabled, Cloud retains execution traces on a durable volume. Users and their companions can inspect traces for their authorized applications until the traces expire, even if the engine is lost or the cluster is replaced. The retention period is still to be determined.

A shared filesystem is one candidate for the volume. The storage backend, how multiple engines write to it concurrently, and how a replacement cluster regains access remain open.

### Tenant isolation and shared capacity

Application owners can publish skills and runtime configuration. Loomspan Cloud controls which executable extensions are installed in the shared engine.

An application’s complete skill pack and runtime configuration travel together and are published as one version. References within the pack to the application itself are relative to the destination application, including in its configuration. Explicit references to shared executors and other external dependencies keep their targets.

Each application can configure a shared executor once for all its skills that use it.

Cloud publishes each customer application into its own private Framework namespace. Even when tenants share an execution engine, the engine must prevent one tenant’s application from calling or interfering with another tenant’s application.

We expect typical workloads to use little CPU or memory. Our goal is to run many tenants’ namespaces in one execution engine and add engines when performance or redundancy requires them. Customer count alone should not drive provisioning. Sharing capacity should reduce the need to pause, time out, or remove idle environments to reclaim the resources a dedicated executor would consume.

### Relationship to other components

Loomspan Sidecar serves as a reference for how to embed the framework in another project. We expect the cloud engine to have its own implementation. Users will submit work to their applications through the cloud API and see or manage those applications through the administration UI.

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

Each execution engine currently emits a single stream containing trace records from all its namespaces. Someone who embeds the framework in their own application and controls the server should continue to be able to use the existing console to inspect all of those namespaces.

We expect cloud features to require enhancements to the way Loomspan Console communicates with an execution engine. We will identify those changes as cloud requirements become clearer and preserve the existing Console's ability to connect directly to an engine.

The cloud Console proxy will consume the framework stream and enforce namespace access for each signed-in user. The framework should be able to emit the same data whether the console connects directly or through Loomspan Cloud. We still need to determine what framework changes, if any, are needed to isolate that data by namespace. Providing the console experience in the cloud will likely require a new front end and a cloud-specific MCP server, both subject to the same namespace permissions.

### In-memory projection of observed engine state

Within the Go management backend, only the Console proxy will open and read raw engine streams. It will use the backend's existing mTLS client credential to authenticate to each engine's Console endpoint. The proxy will maintain an in-memory projection of observed engine state. The rest of the management backend, including the placement controller, invocation router, and diagnostics services, will read that state through the projection.

For each engine instance, the projection records the engine's reported state and installed Loomspan namespaces. For every namespace, including one that has never served an invocation, it records the active `generationId` and the optional `publicationVersion` reported by Console. The router selects an available engine whose confirmed `publicationVersion` for the target namespace matches the requested cloud publication.

When running on Kubernetes, the cloud engine wrapper will read the pod name, Kubernetes namespace, and pod UID from environment variables populated by the Kubernetes Downward API. It will report these fields over the Console connection alongside an engine instance ID generated once at JVM startup. The pod UID identifies the pod across container restarts. A new engine instance ID tells the proxy that the JVM restarted, even in the same pod, so the replacement must confirm its own installed generations.

The projection remains readable when a connection drops and marks that engine's last observation as disconnected. On connection or reconnection, the proxy reads `GET /_loomspan/observability/v1/instance` before treating placements as confirmed for routing. The response includes the process `instanceId` and a row for each installed namespace with its active `generationId` and `publicationVersion` (null if omitted). After a lost publication acknowledgment, a fresh snapshot lets the controller check whether the requested publication is active.

## API

The API will let users submit work and will likely also provide status and metrics for the work they have running. We expect to expose a REST endpoint.

### Synchronous invocation

In the first release, clients will invoke an application synchronously and receive its result or reported error on the same request. If the API cannot return an execution result, it will distinguish two outcomes.

- **Rejected before execution** means the cloud can confirm that the skill never started. If no engine is ready to serve the application's requested version, Cloud promptly reports that the application is temporarily unavailable. The caller can safely retry that invocation.
- **Outcome unknown** means the cloud dispatched the invocation and lost contact with the engine before it could confirm the result. The skill may have completed or produced side effects. Retrying could repeat them.

Framework admission is single-use and local to the engine process. We will defer a durable job queue until there is a requirement for accepted work to survive engine loss.

### Relationship to other components

Users will submit work to an application through the API. Internally, an execution engine will run that work. We also expect to support authenticated MCP access so users can work with their Loomspan Cloud configuration through a coding agent or assistant.
