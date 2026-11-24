# Private Framework Generation Testing Plan

## Change Summary

The Java wrapper accepts authored ZIPs and Cloud runtime metadata, validates a private candidate without side effects, and acknowledges an installation only after Framework activates a detached, policy-checked generation.

## Impacted Areas and Risks

| Category | Risk | Planned evidence |
| --- | --- | --- |
| HTTP contract | Multipart failures are rendered as a generic JSON error, setting types coerce, or `200` represents an incomplete check | Embedded HTTP requests with original ZIP bytes and per-field type, presence, and domain mutations |
| Candidate policy | A variable changes the effective driver, enables Vertex mode, or creates a credential field that bypasses an authored check | Exact-map typed preflight, real Framework binding, synthetic service-account fixture, and a reloader spy recording prepare calls |
| Namespace state | Validation, bad values, same-version restart, or a commit exception changes the wrong generation | Real public reloader snapshot checks and a controlled stale-base or rejected publication |
| Authored YAML | Composition changes aliases, keys, or scalar identity | Worked/collision cases plus alias and literal placeholder assertions on the installed runtime and executor |

## Existing Coverage and Environment Constraints

Cloud introduces JUnit Jupiter and generated ZIP/multipart fixtures in PR 1.1. Its Boot build is available offline using Maven 3.9.11 and JDK 21. The checked Framework snapshot contains the required parser and reloader APIs. `loomspan-variable-fixtures/worked` and `collision` from Framework supply local input and expectations. Put the minimal needed extracts into Cloud test fixtures or inline equivalent YAML anchored to those corpus files so tests run without a sibling checkout. Test host executors and provider configuration are process-local. No test requires a configured provider, ADC, service credentials, K3s, or a remote service.

## Failing Test First

- Name — `validatesPortablePublicationWithCompleteRuntimeSettings`.
- Type — HTTP integration, initially using a reference-free pack.
- Location — `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationControllerTest.java`.
- Arrange, act, assert — POST a ZIP whose execution YAML is `loomspan: {}` and exact `namespace` and `runtimeSettings` metadata to `/internal/v1/validate`. Check a `200` JSON response carrying `valid: true`, `issues: []`, `skills: []`. No variable map is present.
- Expected pre-implementation failure — endpoint returns `404`.

## Tests to Add or Update

### 1. Endpoint wire and parser checks

- Type and location — Embedded Boot servlet on a local random port in `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationControllerTest.java` using JDK HTTP requests. Exercise the real `PublicationZipAdapter` with the original bytes. Use a recording reloader or the real starter as needed.
- Assert both endpoints require multipart pack and JSON metadata with exact media types. Test missing, duplicate, and wrong-kind parts, invalid JSON, duplicate JSON keys, and a parser-rejected oversized pack. Assert non-success `text/plain` and the relevant message, including cases rejected before controller dispatch.
- Iterate over every required runtime setting leaf on both endpoints. For each leaf test omission and a wrong wire type. Include explicit null, fractional numbers, integral overflow, quoted integer values, zero depth, a nonpositive or malformed duration, an attachment size without a unit, negative quotas, and an unknown trace policy. Assert the exact offending path and no validation or preparation call. Test the documented V1 values, all three exact trace strings, and one different valid administrator-supplied limit to prove the Java wrapper consumes the value instead of silently using defaults.
- Publish without `publicationVersion`, a blank version, null or missing `variableValues`, an array, blank/null/non-string entries, and a reference-free `{}` case. A valid opaque version with punctuation or internal spaces is passed unchanged. Check no silent host-value fallback. Verify successful HTTP 204 has an empty body.

### 2. Completed, independent, and interrupted validation

- Type and location — `PublicationControllerTest.java` for response shape and injected operational failures, and `execution-engine/src/test/java/ai/loomspan/cloud/engine/PublicationGenerationTest.java` for integration with the real Framework.
- Compose a valid Gemini Vertex connection with project/location and omitted credentials. Include a skill with an unknown callable and a nested exact pack-relative source. Assert `200`, `valid: false`, `skills: []`, `ERROR` with `sourceName: loomspan/execution.yaml` and the credentials path, and a second issue for the unknown callable with its original skill source. Assert optional fields appear only when available. Validation neither invokes prepare nor changes the active generation ID or version. A fixture with staged unresolved variables succeeds without an explicit value map where all static checks pass.
- Exercise a warning-only `SkillValidationResult` with `ValidatedSkill`, allowing a deterministic stub if producing a stable Framework warning requires unrelated setup. Check `valid: true`, warning severity, and exact `namespace`, `localName`, and `kind`. When validation throws a `SkillReloadException` with a chosen message, the HTTP response is non-success `text/plain` with that message verbatim and has no report object.

### 3. Framework variable and generation conformance

- Type and location — Real starter and test host executor in `PublicationGenerationTest.java`. Use generated ZIP parts passed through the handler/service. Reuse `PublicationZipAdapterTest.part` if its shape survives PR 1.1, or use the same JUnit/Spring mock-part pattern for direct service tests. Use the public `SkillReloader.snapshot` and direct `SkillTemplate.invoke` to inspect and run results. Test internal access to the active runtime only for provider-bound settings or private visibility evidence unavailable in `SkillCatalog`.
- Drive the worked fixture with its six occurrences and four values `provider.key`, `app.region`, `records.url`, and `records.lookup-path`. Assert the OpenAI connection reflects the expected key and header. The fixed `fixture-host:http` executor is public to both target namespaces and sees each tenant's `records.url`, two executor occurrences of the region, and the skill-only lookup path through the captured consumer configuration. Change the fixture's executor-configuration owner to match each test target. Verify the skill-only name is accepted by full prepare even though preflight sees only the execution names. Verify a literal containing placeholder syntax remains literal. An alias-bearing configuration branch exercises mapping-valued variable keys through the adapter and composer, and the executor verifies both aliased values.
- Install the same names in two private namespaces with distinct values; set a third value in a host property source. Assert both installed outputs remain distinct and a missing, blank, or extra variable map entry fails even if the host would fill it. Use the corpus collision skill where a variable key resolves to an existing key. Verify Framework reports the authored collision and current generation ID, version, and callable results remain unchanged.
- Capture an invocation in the test executor with deterministic latches, then publish the same pack and opaque version with changed values. Assert the new generation ID differs, a new invocation sees the edited value, and the earlier accepted invocation returns its original value after release. Use bounded waits and always release the latch on failure. Then install `loomspan: {}` and zero skills. Assert its namespace and new version still exist, and the prior YAML skill is absent.
- Confirm the successful installed generation is private using actual generation metadata or an expected Framework visibility-denied cross-namespace reference. Supply runtime metadata differing from startup defaults and inspect the captured active session/trace settings. Keep the host trace directory independent.

### 4. Authored and effective Tenant Vertex checks

- Type and location — Deterministic credential and parser cases in `execution-engine/src/test/java/ai/loomspan/cloud/engine/TenantVertexPolicyTest.java` plus endpoint and serving-state assertions in `PublicationGenerationTest.java` or `PublicationControllerTest.java`.
- Generate a temporary RSA key in the fixture or use a checked-in clearly synthetic Framework test key and encode service-account JSON with synthetic identifiers. Credential parsing must be CPU-local and must never read system ADC or obtain a provider token.
- Assert a literal valid service-account string and a correct variable value pass, while omitted declaration, blank or missing values, JSON of the wrong credential type, malformed JSON, and known authored invalid credential text fail. For the omitted declaration case make a synthetic host ADC credential available to a test JVM, for example by pointing `GOOGLE_APPLICATION_CREDENTIALS` at a synthetic temporary JSON file in a dedicated child process. Establish the host credential is locally discoverable without requesting a token, and show preflight still blocks before `SkillReloader.prepare`. Assert the diagnostic contains the target connection/field path and does not contain the JSON. Validate authored `{ $var: ... }` and unresolved keys without consulting values.
- Exercise markers selecting the driver, `gemini` or `vertex-ai` key where supported, Vertex mode value, and credentials. Resolve one request to a connection that needs credentials and lacks them and another to a compliant connection. Include an effective `credentials-file` or `credentials-uri` from a marker key and explicit authored file/URI cases. Assert ordinary Gemini API-key mode passes the Cloud credential check. Test repeat/new version and a fresh test-engine context going through exactly the same endpoint so restart and restore cannot bypass preflight.
- For representative policy failures prove a mock/spy recorded no call to prepare, meaning the runtime factory could not build a client. As a separate integration test, install a serving generation via the real starter, attempt the failed replacement, and assert the same ID/version and output remain active. Permitted mode tests never call a live model.

### 5. Publication exception and prior generation

- Type and location — Controller and real generation tests listed above.
- Test a rejected configuration before prepare and an actual effective Framework prepare collision after preflight succeeds. Assert preserved failure text and active identity. For a deterministic publish-only failure use a delegated recording reloader around the real starter and reject inside `publish` after producing a real detached candidate, or deliberately publish a competing candidate before forwarding and assert the winning previously active generation remains. Check HTTP non-success text, the exact exception message, and disposal of the detached candidate. This verifies the controller's response boundary independently of provider preparation errors.

## Safe Verification Commands

Run from `execution-engine/` with JDK 21.

- Focused — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest' test`.
- Related suite — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode '-Dtest=PublicationControllerTest,TenantVertexPolicyTest,PublicationGenerationTest' test`.
- Full safe suite — `& C:/hamdev/maven39/bin/mvn.cmd --offline --batch-mode verify`.
- Optional alternate local snapshot location — for the orchestrator's refreshed isolated Framework build, append `"-Dmaven.repo.local=$((Get-Location).Path)/target/pr1-framework/repository"` and `"-Dmaven.repo.local.tail=$env:USERPROFILE/.m2/repository"` to the command. No change to the build's declared dependency is required.

## Optional Developer Checks

None. All Java requirements have deterministic local coverage. Actual multi-process recovery belongs to the later PR 1 milestone.

## Exit Criteria

- [x] The first HTTP acceptance test fails with the route absent and passes after implementation.
- [x] Both endpoints exercise the real multipart and ZIP parser and verify every metadata field and wire type.
- [x] Validation retains independent findings and serving generation, and operational failure has the required text response.
- [x] Effective credentials are checked before preparation on every publication route and evaluated across literal and variable modes.
- [x] Real Framework checks show the worked-corpus values, literal and key semantics, namespace privacy, isolation, same-version capture, zero-skill install, and prior-generation retention on failure.
- [x] Focused and full offline Maven suites pass without external calls or destructive operations.

## Implementation Verification

- The first loopback POST to `/internal/v1/validate` returned `404` before implementation. The same request returned a completed `200` report once the controller was installed.
- `PublicationControllerTest` passed all eight loopback tests. `PublicationGenerationTest` passed all four real-Framework tests. `TenantVertexPolicyTest` passed all five credential-policy tests, including the dedicated JVM with a synthetic Application Default Credential.
- Offline Maven verification reported 49 tests with zero failures, errors, or skips and packaged the executable Spring Boot JAR. The same full verification passed against the isolated refreshed starter from Framework `2ca9caf`. The tests used loopback HTTP and synthetic credentials.
