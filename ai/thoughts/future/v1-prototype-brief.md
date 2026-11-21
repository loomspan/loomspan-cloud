# Loomspan Cloud V1 prototype brief

A user can take an application from setup through its first run and explain the result, either directly or with their AI companion. The companion acts with the user's permissions throughout.

## Manage an application and its users

A user signs in by email magic link and can create an application. The application maps to a Framework namespace. Its owner defines application roles and can assign them to an email address before the user first signs in. Once the user verifies that address through the magic link, they gain the assigned access to that application.

Users can authorize an MCP companion in their browser. The companion receives its own tokens and can reconnect across sessions with the user’s current permissions. Token lifetimes remain open.

V1 uses hosted Supabase Auth for sign-in and hosted Supabase Postgres solely for Loomspan Cloud’s own platform data, including application memberships and role assignments.

## Prepare and publish skills

The owner or companion can configure a shared executor once for the application. Every skill in the application that uses that executor can reuse those settings.

Cloud manages application-scoped variable values. Loomspan Framework owns variable-reference syntax and resolution.

The owner and companion can prepare skills and runtime configuration, see how the skills fit together, distinguish staged work from the published version, and publish an update. The current version keeps serving until the update is ready. If publication fails, the owner can see the failure.

The first V1 authoring milestone is upload and download of the complete skill pack and runtime configuration as one documented ZIP. The owner or companion can download the staged or published pack, upload it to another application's staging area, and publish it as a unit.

For example, an owner can publish and test changes in `orders_qa`, download the tested publication, and upload that same ZIP to `orders` for publication. The skills and configuration resolve their local references in the destination application, so moving the pack requires no namespace edits.

The companion should have access to Framework's skill definitions and authoring guidance. A visual skill editor is a possible later addition.

## Try a published skill

The user can select a published skill in a playground, supply input, invoke it, and see the result or error. The companion can invoke the skill through MCP with the same permissions. This lets the user exercise an application before connecting an external client.

## Inspect the application and its runs

Through the cloud Console UI or through MCP, the user or companion can inspect published skills and their version, see active sessions, find previous runs, and examine a run's trace when tracing is enabled. Retained traces remain accessible to authorized users after engine replacement. The retention period is open. Longer-term error summaries, timing statistics, and dashboards are possible additions.

## Proposed V1 demonstration

An owner signs in, connects a companion, creates an application, and grants a second user a role that permits skill invocation. The owner and companion stage a skill pack and publish it.

- **Application permissions.** The second user can invoke the published skill in the playground, and their companion can invoke it through MCP. Cloud denies both callers permission to publish an update or read traces for an application they have no access to.
- **From result to trace.** With tracing enabled, several users invoke the same skill. From a particular playground result, an authorized user can open the trace for that exact invocation or ask their companion to find it and explain the result.
