# Execution engine API

This document defines the V1 interface between the Go backend and the Java execution engine.

## Integration contracts

| Integration | Caller and input | Receiver and observable outcome |
| --- | --- | --- |
| Validate a staged pack | Go sends the staged ZIP, target namespace, and destination application’s effective runtime settings at the owner or companion's request. | The wrapper calls `SkillReloader.validate` with the skill documents, execution configuration, and private visibility. Framework checks the complete candidate, including skill references, without creating provider clients or activating a generation. Java returns Framework diagnostics, preserving source paths when available. |
| Publish a pack or restore a publication | The Go placement controller sends the original ZIP, the target namespace, the Cloud-assigned opaque `publicationVersion`, the required `variableValues` map, and the effective runtime settings to the engine's internal API. | The Java wrapper publishes the supplied skill definitions and composed configuration into the target namespace. Go can confirm the version through Console before routing work. Restoration reuses the saved ZIP and version with the target application’s current variables and effective runtime settings at restore time. |
| Observe an engine | The Go Console proxy connects to the engine and reads `GET /_loomspan/observability/v1/instance` at connection and reconnection, then consumes the engine stream. | Each active namespace entry reports `generationId`, `publicationVersion`, and `tracePersistencePolicy` from the same generation, even before first invocation. Go reads the application's active policy from its namespace entry. The top-level policy describes the default namespace. The Console proxy maintains the observations used by the placement controller and router, including confirmation of a publish whose acknowledgment was lost. |
| Invoke a skill | The Cloud API authorizes the caller for the application. The router sends the authorized namespace, local skill name, and input to a confirmed engine. | Java sets the Framework security context from the separate `caller` object and returns the [invoke response](#invoke-responses). |

Go and Java authenticate each other using mTLS on the internal API and Console connection. For Console requests, every engine also enforces Framework's existing API-key check. A Kubernetes Secret supplies the same Console API key to the Go proxy and every engine in that environment. Development, QA, and production each use their own key.

### Pod and JVM identity

The Console proxy learns each engine pod's Kubernetes namespace, name, UID, and address from the K3s pod watch. It connects directly to that pod's Framework Console endpoint and pairs the pod UID with the `instanceId` returned by the authenticated instance snapshot. The namespaces, generation IDs, and publication versions in that snapshot belong to the JVM observed on that connection.

On disconnect, the Console proxy marks the observation disconnected. The router requires a fresh snapshot confirming the intended namespace and publication version on an eligible pod before it routes there again. A new `instanceId` for the same pod UID identifies a JVM restart. A new pod UID identifies a replacement pod even if the name or address is reused. The Console proxy associates each new snapshot with the discovered pod it queried.

## Internal API requests

Go sends three synchronous HTTPS requests over the authenticated mTLS connection. Each request carries the target namespace in its body.

| Endpoint | Request body |
| --- | --- |
| `POST /internal/v1/validate` | Multipart pack and JSON metadata containing `namespace` and `runtimeSettings`. Staged validation does not require variable values. |
| `POST /internal/v1/publish` | Multipart pack and JSON metadata containing `namespace`, `runtimeSettings`, `publicationVersion`, and `variableValues`. For publication, Java requires a non-null `variableValues` map with exactly one nonblank value per distinct referenced name, including an empty map for a reference-free pack. |
| `POST /internal/v1/invoke` | A JSON or multipart [invoke request](#invoke-requests) with `namespace`, `skill`, `input`, and the [caller object](#invocation-identity-and-roles). Go sends `executionView: "always"` when the administrator enables execution views and omits the field otherwise. |

Validate and publish use `multipart/form-data` with two named parts. The [Java ZIP adapter](java-execution-engine-roadmap.md#cloud-zip-adapter) receives the `pack` part, which contains the original ZIP bytes as `application/zip`. The `metadata` part contains the JSON object as `application/json`. Restore and restart use the same publish endpoint with the saved ZIP and publication version, the application's current variable values, and its current effective runtime settings.

### Runtime settings

For both validate and publish, Go sends the complete effective settings for the destination application in the JSON metadata. The following example uses the [V1 Cloud policy values](java-execution-engine-roadmap.md#administrative-runtime-settings).

```json
{
  "runtimeSettings": {
    "session": {
      "mission-timeout": "60s",
      "max-depth": 32,
      "attachments": { "max-size": "20971520B" },
      "quotas": {
        "max-skill-invocations": 64,
        "max-tool-invocations": 128,
        "max-linter-retries": 32,
        "max-model-calls": 64,
        "max-provider-attempts": 192,
        "max-usage-units": 200000
      }
    },
    "execution-trace": { "persistence": "ONERROR" }
  }
}
```

All fields shown inside `runtimeSettings` are required. Encode depth and quotas as JSON integers, timeout as a duration string, and maximum attachment size as a size string with an explicit unit (`20971520B` is 20 MiB). Go supplies the application's selected persistence policy: `NEVER`, `ONERROR`, or `ALWAYS`. The Java wrapper reads the portable configuration from the ZIP and adds `session` and `execution-trace` under `loomspan` to build one in-memory Framework configuration. The trace directory comes from engine startup configuration.

### Invoke requests

JSON and multipart invocations both follow the [V1 invocation cutover rule](java-execution-engine-roadmap.md#publication-and-cutover).

Go enforces the [caller-facing byte and file-count limits](go-backend-roadmap.md#invocation-response-visibility) before sending the internal invoke request. The Java wrapper relies on that check.

Go sends the authorized application’s `namespace` and the skill’s `localName` from validation or the catalog in the `skill` field. Java passes the two components separately to `SkillTemplate.invoke`. The required `input` object holds the skill’s named arguments. Use `{}` when there are no ordinary input fields.

For `application/json`, the request object is the entire body. For `multipart/form-data`, send that object in one required `application/json` part named `request`, with zero or more binary file parts. A multipart request containing only `request` behaves like a JSON request. When uploading files, include a `files` array in the request object. Each entry names the multipart `part` and gives its destination `path` as a JSON Pointer relative to `input`.

For example, the following `request` part accompanies two binary parts named `invoiceA` and `invoiceB`. Each file part carries its filename and content type.

```json
{
  "namespace": "u42-claims",
  "skill": "review-invoices",
  "caller": { "issuer": "partner-portal", "subject": "user-123", "roles": ["reviewer"] },
  "input": { "question": "Do these invoices match the claim?" },
  "files": [
    { "part": "invoiceA", "path": "/documents/0" },
    { "part": "invoiceB", "path": "/documents/1" }
  ]
}
```

Java uses the skill's declared input shape to construct missing object or array parents and supplies each upload as a repeatable resource at its mapped position before invoking Framework. For a file-only skill, send `"input": {}` and map the upload to the skill's file argument. Omit `files` when there are no uploads. Reject missing or ambiguous part mappings, duplicate destinations, overwrites of supplied values (including explicit nulls), and mappings that leave gaps in an array.

## Validate and publish responses

For staged validation, Java runs every independent wrapper and Framework check it can complete. It returns their findings together in the `200 OK` validation result with `valid`, `issues`, and `skills`. An authored Vertex credential-policy violation is a structured issue. Java continues checking unrelated skill references when they can be validated. Each issue carries `severity`, `sourceName`, and `message`, plus `skillName` and `fieldPath` when available. Each validated skill carries `namespace`, `localName`, and `kind`. Warnings alone leave `valid` true. Any `ERROR` yields `valid: false` and an empty `skills` array.

After Framework's `publish` method returns successfully, Java responds with `204 No Content`. [Go publication handling](go-backend-roadmap.md#publishing-a-reviewed-revision)

If a validation check fails partway through, Java preserves every finding already collected and includes the failure message in the same result. If the wrapper rejects the ZIP or request metadata before validation can begin, Java returns a non-success HTTP status with the message as `text/plain`. Go includes that message verbatim alongside its own findings in the caller's validation result.

For publish, a rejected ZIP or request metadata, or an exception during preparation or publication, yields a non-success HTTP status with the message as `text/plain`. Go passes the message to the authorized caller exactly as received. If the publish response is lost after dispatch, Go follows the [publish confirmation and retry rule](go-backend-roadmap.md#unconfirmed-publications).

## Invocation identity and roles

- **Direct user call.** The user authenticates to Cloud. Go sends the authenticated user's identity and the roles Cloud has assigned that user for the target application.
- **Calling application.** The backend authenticates to Cloud and supplies its own stable ID for the end user along with that user's roles on each invocation. Go accepts the assertion only from a backend the owner has authorized for the target application. Go identifies the end user by the authenticated backend and the supplied user ID together.

Go sends a required `caller` object alongside the skill input on every internal invocation request. For a direct call, `issuer` identifies Cloud and `subject` identifies the signed-in user. For a calling application, Go sets `issuer` from the authenticated backend and `subject` from the end-user ID that backend supplies. Java uses the pair as the Spring Security principal. If `caller` is absent or either identity field is missing or blank, Java rejects the request before invoking the skill.

The `roles` field is a required array of application role names. Go sends the names as defined for the target application. Java adds the `ROLE_` prefix once to each name when constructing the Spring Security authorities. For example, a wire value of `"reviewer"` grants `ROLE_reviewer`, which satisfies a skill requirement of `rbac_roles: [reviewer]`. An empty array (`"roles": []`) is valid for an identified user authorized to invoke the application and grants no role authorities.

## Invoke responses

Every response Java sends from `POST /internal/v1/invoke` has an `application/json` body. A normal return from `SkillTemplate.invoke` produces `200 OK` with the exact Framework string in `result`, including when the string contains JSON. An exception produces a non-success status with its original `message` and a `debug` string containing Java's full standard exception output, including the exception class, stack trace, causes, and suppressed exceptions. Include the complete structured `diagnosis` and any input-validation `issues` the exception carries. Include the full available `executionView` according to the [requested mode](#invocation-metadata).

For invoke, Java reserves `503 Service Unavailable` for a confirmed rejection before the skill started, including when a request reaches an engine that has closed admission during shutdown. The JSON `message` explains the rejection. Go uses the status alone as confirmation when it receives the authenticated engine's response. If Go loses contact after dispatch or receives only an intermediary's error, the execution outcome is unknown.

**Implementation note for both examples below.** These examples illustrate field placement. Java must send Go all available invocation result or failure details, including Framework details absent from the examples. The sample debug output is abbreviated for readability. Send it in full in the implementation. Go applies the [caller visibility rule](go-backend-roadmap.md#invocation-response-visibility).

Example success with a requested view:

```json
{"result":"Claim approved.","executionView":{"sessionId":"s-123","events":[]}}
```

Example failure with a requested view:

```json
{"message":"I need a claim number to continue.","debug":"ai.loomspan.api.PlannerInabilityException: I need a claim number to continue.\n\tat …","executionView":{"sessionId":"s-124","events":[]}}
```

## Invocation metadata

V1 does not support caller-requested cancellation. Once an invocation starts, it continues under Framework's normal execution limits if the caller disconnects or times out while waiting for the response.

Go derives the internal execution-view mode from the application's administrator-controlled switch on each invocation. The switch defaults to off for new applications. When off, Go omits the field and Java invokes without an observer and omits `executionView` from the response.

When the switch is on, Go sends `always`. Java captures Framework's `SkillExecutionView` through the observer and includes `executionView` on success and error. Go returns the view to the invoker automatically and includes the available `debug` and `diagnosis` fields on failure. When the switch is off, Go omits all three fields from the caller-facing response. REST and MCP callers cannot override the switch.

When the response calls for the field, Java sends the complete view (session ID and events) if it received one, or `"executionView": null` otherwise. This covers rejection before a session starts and failures that prevent Framework from delivering the view during finalization or mapping. An unavailable view leaves the invocation's result or error details and HTTP status unchanged. View delivery is independent of trace retention.
