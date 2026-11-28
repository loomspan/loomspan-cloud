# Phase 1 Cloud REST API

The public base path is `/api/cloud/v1/applications`. Application routes below are relative to that path. Send a user bearer token on every request. Responses use JSON. Create derives the owner from the authenticated identity. For application routes, Go checks current permission for the namespace name in the URL. Manager includes Caller permissions. [Authentication and permissions](go-backend-roadmap.md#application-permissions)

| Method and route | Permission | Request body |
| --- | --- | --- |
| `POST` base path | Signed-in user | JSON `{"slug":"sales"}` |
| `POST /{applicationId}/staging` | Manager | Raw original ZIP bytes, `Content-Type: application/zip` |
| `POST /{applicationId}/publish` | Manager | JSON `{"selectedRevisionId":"…"}` |
| `GET /{applicationId}/requests/{requestId}` | Manager | None |
| `POST /{applicationId}/invoke` | Caller | JSON `{"skill":"echo","input":{"text":"hello"}}` |

Use `application/json` for JSON requests. Missing or invalid bearer credentials return 401. Go caches the configured HTTPS issuer’s signing-key set in memory for a fixed 10 minutes from the last successful fetch. While the set is younger than 10 minutes, a matching key permits local token verification. Only a successful fetch starts a new window.

If the cache is empty, its age reaches 10 minutes, or the token’s key ID is absent, Go fetches the set once for that verification. A failed fetch returns 503 even if the expired set contains the key. A successful fetch replaces the set. If the key is absent from the new set, Go returns 401. Both authentication responses precede application lookup, writes and engine dispatch. Issuer reachability is independent of backend readiness. Callers can retry verification after the issuer recovers.

A missing or inaccessible application returns the same 404. Insufficient permission returns 403. An unavailable access lookup returns 503. After authorization, Stage, Publish and Invoke return 409 with `reason: "Deleting"` for a deleting application. Errors carry a human-readable `message`. [Application lifecycle](go-backend-roadmap.md#application-permissions)

## Create and stage

By design, the V1 field `applicationId` carries the assigned namespace (for example, `u42-sales`) wherever it appears in a route or response. Create returns `{applicationId, namespace, existing}` with 201 for a new application or 200 for an existing owner/slug pair. An invalid slug returns 400. A reserved slug returns 409 with the existing `applicationId` and `state: "Deleting"`.

Stage accepts up to 1,000,000 ZIP bytes inclusive. Oversize ZIP bodies return 413. Malformed packs and other pre-staging rule violations return 400 with the diagnostic and preserve the prior revision. Once staged, return 200 with `{stagedRevisionId, validation}`. A completed validation object carries `status: "completed"`, the engine report (`valid`, `issues`, `skills`), Cloud findings, and each missing variable occurrence with its name, pack-relative source and authored path. Use `engineReport`, `cloudIssues` and `missingVariables` for those three fields. An incomplete validation object carries `status: "unavailable"` and a `message` explaining that the ZIP remains staged, validation was unavailable, and an explicit Publish runs fresh validation before installation. Partial findings are withheld. An identical current upload keeps the same revision ID. Return its saved completed report, including an invalid report. If no completed report was saved and no attempt is running, retry validation and return the new feedback. Go constructs the internal multipart request to Java. [Staging rules](go-backend-roadmap.md#staging-revisions-and-validation)

If variable discovery fails (for example, a marker has a blank name), Stage returns 400 with the discovery reason and ordered pack-relative source and authored YAML path for each location. It creates no revision, preserves prior validation feedback and makes zero Java calls. An otherwise acceptable ZIP with a valid marker whose value is missing or blank follows the ordinary 200 staging and validation rules above. A mapping-key collision that arises only after substitution also passes discovery.

If an identical upload arrives for the current revision while its validation is in progress, both Stage requests return 200 with the same `stagedRevisionId` and `validation` from that single attempt.

## Publish and request lookup

The Publish POST has a 210-second response budget at the Go HTTP server and ingress. This allows up to 90 seconds in the queue, 30 seconds each for Validate, Install and post-dispatch confirmation, and 30 seconds of margin. Go responds as soon as it has the applicable outcome.

After authorization and the `Deleting` check, if the application has no staged revision, return 409 with `code: "no_staged_revision"` and a `message` directing the caller to stage a pack first. This allocates no request ID or target version and makes zero Java calls.

Check the selected revision against staging before checking the current publication. A stale selection returns 409 with `{"code":"stale_selection","selectedRevisionId":"A","currentStagedRevisionId":"B","message":"…"}`. The warning ends the Phase 1 request. An already-published selection returns 200 with `{"code":"already_published","message":"This revision is already published."}`. Both responses omit the request ID and target version. If the selected ZIP was removed before acceptance, return 409 with a message asking for re-upload.

Once accepted, include `requestId`, `operation: "publish"`, `targetVersion` and `status` in the Publish response. A confirmed installation returns 200 with `status: "succeeded"` and `confirmedAt`. When the 30-second confirmation wait expires, return 202 with `status: "unconfirmed"` and a message explaining the install may still complete. A terminal failure includes `status: "failed"` and the diagnostic in `message`. Use 422 for an invalid candidate or missing required value, 503 when validation or placement is unavailable, and preserve a known engine rejection status and diagnostic for a rejected install. [Publish ordering and confirmation](go-backend-roadmap.md#unconfirmed-publications)

If an accepted Publish is still queued for its first install dispatch 90 seconds after it entered the queue, return 202 with `status: "pending"`, `requestId`, `operation: "publish"` and `targetVersion`. It keeps its queue position and continues after the response. Eligibility at the exact deadline takes precedence and the POST waits for the ordinary Publish outcome.

GET returns 200 with the same request fields and their durable values, including current progress while pending, `message` for a failure and `confirmedAt` once confirmed. A timed-out caller can read the original request after a newer Publish is accepted. Scope the lookup by both IDs. An authorized Manager gets the same 404 for an unknown request and one belonging to another application. Phase 2 owns the latest-operation lookup.

## Invoke

Go derives the namespace and caller identity. The inclusive 20 MiB allowance measures the received caller JSON body in bytes, including JSON syntax and whitespace, before decoding or adding namespace, caller or execution-view metadata. HTTP headers are outside the body. Go enforces the limit before dispatch even when the client omits Content-Length. [Shared limits and Phase 3 multipart accounting](go-backend-roadmap.md#invocation-response-visibility)

| Result | HTTP | JSON response |
| --- | --- | --- |
| Skill returns | 200 | `result` contains the exact result string and `sessionId` carries the Framework session ID. |
| No current publication | 409 | `outcome: "rejected_before_execution"`, `reason: "not_yet_published"`, `message`. |
| Application deleting | 409 | Same outcome, `reason: "Deleting"`, `message`. |
| Requests disabled or no eligible engine | 503 | Same outcome, `reason: "requests_disabled"` or `"no_eligible_engine"`, `message`. Omit Retry-After for disabled requests. |
| Body exceeds 20,971,520 bytes | 413 | Same outcome, `reason: "body_too_large"`, `message` identifying the limit and stating the skill did not start. |
| Authenticated engine returns 503 | 503 | Same outcome, `reason: "engine_rejected"`, verbatim engine `message`. |
| Result lost after dispatch, including an intermediary error without a confirmed engine response | 502 | `outcome: "unknown"`, `message` explaining that the skill may have run and retrying could repeat side effects. Go never automatically resends the invocation. |
| Other known engine failure | Engine status | Verbatim engine `message`, any top-level validation `issues`, and `sessionId` when supplied. |

Phase 1 keeps execution views off and omits `executionView`, `debug` and `diagnosis`. A pre-session failure omits `sessionId`. [Engine response fields](execution-engine-api.md#invoke-responses)
