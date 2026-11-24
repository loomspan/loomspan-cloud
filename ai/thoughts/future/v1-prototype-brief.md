# Loomspan Cloud V1 prototype brief

A user can take an application from setup through its first run and explain the result, either directly or with their AI companion. The companion acts with the user's permissions throughout.

## Manage an application and its users

A user signs in by email magic link and can create an application. The application maps to a Framework namespace. Its owner defines application roles and can assign them to an email address before the user first signs in. Once the user verifies that address through the magic link, they gain the assigned access to that application.

Users can authorize an MCP companion in their browser. The companion receives its own tokens and can reconnect across sessions with the user’s current permissions. Token lifetimes remain open.

## Prepare and publish skills

The owner or companion can configure a shared executor once for the application. Every skill in the application that uses that executor can reuse those settings.

In Console administration, owners can change trace persistence, view the Cloud-managed session limits, and see which settings are active or pending. Saved changes to variables and settings take effect together on the next successful publication. [Runtime settings and defaults](java-execution-engine-roadmap.md#administrative-runtime-settings)

Owners can choose **Restart application** to apply saved edits using the current published pack.

The owner and companion can prepare skills and runtime configuration, see how the skills fit together, distinguish staged work from the published version, and publish an update. Before publishing, they can ask Framework to validate the staged pack and review any issues while the current version keeps serving. If publication fails, the owner can see the failure.

The first V1 authoring milestone is upload and download of the portable skill pack as one [publication ZIP](publication-package-format.md). The owner or companion can download a staged or published pack. [Go upload checks](go-backend-roadmap.md#zip-checks-before-staging)

The owner can review one staging revision while a developer uploads another. Publishing the reviewed revision requires confirmation when a newer revision is staged. The newer work stays available. [Staging and publication workflow](go-backend-roadmap.md#upload-and-staged-validation)

An owner can promote a tested QA publication to production through Console or MCP, review the staged pack, and download the original QA publication. [Promotion workflow](go-backend-roadmap.md#publication-promotion)

The companion should have access to Framework's skill definitions and authoring guidance. A visual skill editor is a possible later addition.

## Invoke a published skill

Users can invoke a published skill through REST or through their companion over MCP. An application administrator controls whether each invoker automatically receives the available execution view on success or failure. The same switch controls whether the invoker receives debug output and structured diagnosis on failure. It defaults to off for new applications. [Invocation response visibility](go-backend-roadmap.md#invocation-response-visibility)

## Inspect the application and its runs

Through the cloud Console UI or through MCP, the user or companion can inspect published skills and their version. Cloud run history covers invocations with a retained trace. Only application administrators and companions acting with their permissions can inspect live sessions and events, browse retained runs, or review and download their traces, including after engine replacement. Cloud will revisit [retention and cleanup](roadmap.md#durable-execution-traces) after observing V1 usage. Longer-term error summaries, timing statistics, and dashboards are possible additions.

## Proposed V1 demonstration

An owner signs in, connects a companion, creates an application, and grants a second user a role that permits skill invocation. The owner and companion stage a skill pack and publish it.

- **Application permissions.** The second user can invoke the published skill through REST, and their companion can invoke it through MCP. Invocation permission alone grants neither caller access to live events or retained traces, even when the administrator enables execution views.
- **Inspect a run.** With tracing enabled, several users invoke the same skill. An application administrator asks their companion to find the retained trace for a particular invocation and explain the result.
