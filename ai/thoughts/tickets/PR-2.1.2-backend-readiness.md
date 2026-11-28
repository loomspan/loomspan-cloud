# PR 2.1.2 — Start the Full Cloud backend on K3s with durable storage and Postgres

## Outcome

A deployed Go process becomes ready on valid configuration and a migrated database. An operator can bring Postgres online after Go starts and see that same process recover.

## Context

GitHub PR 2, Go backend Phase 1. Build on PR 2.1.1’s Full Cloud entry point. This ticket supplies the runtime used by all remaining Phase 1 work. Schema migrations for specific business features belong to the tickets that introduce them.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Configuration and migrations](../future/go-backend-roadmap.md#go-backend-configuration) · [K3s ingress](../future/roadmap.md#starting-point-for-placement-and-routing) · [Shared storage and installation](../future/roadmap.md#durable-execution-traces)

## Requirements

Deploy one Go replica with the `Recreate` strategy. A planned upgrade can briefly interrupt the Cloud API. Keeping the old and new Go processes from serving concurrently preserves Phase 1’s in-process Publish ordering.

The installer creates the management and engine namespaces before applying namespaced resources. Install a Role in the engine namespace granting `list` and `watch` on core `pods`. Bind it there to the Go backend service account in the management namespace. Hand that identity and the engine label selector to PR 2.1.3.

Set route-specific response budgets at both ingress and the Go HTTP server. Allow 210 seconds for the Publish POST and 90 seconds for synchronous Invoke. Preserve long-lived Console streams. Configure ingress to pass the inclusive 20 MiB invocation boundary and the one-byte-over case through to Go for its documented response. The 1,000,000-byte staging limit applies to the ZIP itself.

Build a repeatable Full Cloud installer in this repository using a script and deployment manifests. Take the designated K3s context as an explicit input. The operator supplies the test cluster and authorized database access. Document the production prerequisite for the dedicated Supabase issuer: the operator enables Confirm Email and ensures every sign-in method enabled for Cloud verifies control of the supplied email address. The Phase 1 test deployment takes the local test issuer from PR 2.1.4 through the same startup setting. Record nonsecret environment identifiers and credential references in the handoff. The archived PR 1 cluster name is historical.

On first installation, require a Longhorn-capable K3s cluster. Prompt without echo for the operator’s saved volume passphrase and create the StorageClass-referenced Secret before provisioning the encrypted Longhorn `ReadWriteMany` volume. Keep passphrases and database credentials out of logs and evidence.

Document an unattended rerun against the designated K3s context after first installation. Reuse the installed volume and Secret. If the existing installation’s storage or Secret is missing or incompatible, stop with a diagnostic identifying the affected resource and preserve the existing data for recovery. Document how to use the saved passphrase on a replacement cluster to recover the surviving volume or its encrypted backup.

- Treat the sibling `loomspan-cloud-test-suite/deploy/k3s` manifests and this repository’s `management-backend/` at commit `75358f8` as historical PR 1 examples. Reuse individual deployment patterns only where they satisfy the current contracts. The milestone supplied a full `database-url` Secret, supported transaction pooling and applied migrations separately. This deployment reads connection settings from YAML and the password from its Secret-mounted file, supports the direct or session-pooler endpoint, and runs embedded Goose migrations.
- Mount the pack directory writable by Go. The milestone’s read-only pack PVC and pre-seeded global `<digest>.zip` lookup served a fixture workflow. PR 2.1.5 owns staging original bytes and application-scoped ZIP references; preserve its ability to account for, protect and remove packs per application. The old global path is historical, while SHA-256 over exact ZIP bytes remains required.
- Install the Go deployment and Service behind the installation’s single HTTPS ingress. Configure ingress timeouts for synchronous calls. Establish the shared ReadWriteMany volume and the durable pack/config paths. Configure access in both namespaces so the engine can mount only the trace directory and Go can access the same files. Follow the volume contract for passphrase provisioning, Secret references, atomic rename and cross-node locks. Keep workload mounts scoped, including the read-only Go config directory and exclusion of operator-only recovery material.
- The installer supplies the engine discovery label and matching pod selector, fixed read-only mount paths for the management client certificate, private key and trusted CA, and a fixed writable pack mount path. Keep these conventions in the deployment and its Go wiring. Record the exact selector and TLS paths for PR 2.1.3 and the pack root for PR 2.1.5. The mounted YAML remains the sole operator-edited source of Go settings.
- At startup, validate both YAML sections before readiness. In `startup`, read the trusted Supabase issuer and Postgres host, port, database, user, TLS settings and password-file path. Mount the password from a separate Secret at that path. If the file is unreadable or unparseable, or an installation-wide setting is invalid, stay unready, log the identifiable setting and periodically reread the file.
- Validate the issuer URL locally for both production and the test fixture. Require an absolute HTTPS URL with a host. The readiness check performs no issuer DNS lookup, connection or signing-key fetch.
- Implement the full `reloadable` schema. The subscription permission-check interval defaults to 60 seconds and must be positive. Manual-upload, engine-trace and pack-history quotas each default to 100,000,000 bytes. Each namespace can override any subset. An explicit zero sets a zero-byte quota and removing an override restores the current default.
- After readiness, periodically reread the same file and validate `reloadable` independently. Apply valid reloadable settings together. Startup edits take effect on restart. A read, parse or installation-wide reload validation failure logs the error and preserves the last valid settings. At startup and on reload, an invalid application entry logs its ID and error and marks that whole application skipped for quota processing until corrected. Other applications continue with their effective settings.
- Use native PostgreSQL for runtime and migrations, including hosted Supabase direct or session-pooler endpoints. Embed versioned SQL with Goose and enable its Postgres migration lock. While Postgres is offline stay unready and retry with capped backoff; migration failure is logged and keeps Go unready. Separate backend readiness from whether an application is published or an engine is available.

## Acceptance criteria

- [ ] Run a fresh installation with the hidden passphrase prompt. Verify the bound `ReadWriteMany` volume is encrypted in Longhorn. Write a marker, rerun the installer with stdin closed, and verify the same volume identity, Secret value and marker bytes. Record only the Secret comparison result. Use this volume for the persistence and cross-node locking checks below. Exercise a rerun with an unavailable storage prerequisite and verify an actionable error with no replacement volume provisioned.
- [ ] Commit a credential-free example YAML file alongside the installer and load it in a test using Go’s production configuration parser. Show the exact keys, nesting and value types for both `startup` and `reloadable`, including the Postgres password-file path, subscription interval, all three quota defaults, and a partial application override with an explicit zero. Verify the zero and inherited defaults in the parsed configuration.
- [ ] At startup and on reload, validate override keys using the application namespace syntax and 61-character limit and quota values locally without querying Postgres. Accept a well-formed namespace name with no current application, including while Postgres is offline at startup. Its override applies when a matching namespace exists, including after recreation. Exercise a malformed entry alongside the valid override and verify the existing per-entry error handling isolates it.
- [ ] With the other startup prerequisites satisfied, verify that a malformed issuer URL and an HTTP issuer each keep Go unready. Correct the setting to a valid HTTPS issuer URL while the issuer is offline and verify the same process becomes ready.
- [ ] Verify the effective Publish and Invoke budgets at both ingress and Go. PR 2.1.6 exercises a Publish POST through this deployment that completes after 90 seconds.
- [ ] On the designated shared volume, place two pods on different schedulable nodes and record their actual node names. While the first holds an exclusive lock, the second must fail to acquire it on the same file and must succeed after release. If only one node is available, continue independent checks and carry this check as pending until verified across nodes.

- [ ] Deploy into K3s, mount configuration and persistent pack storage, and reach the Full Cloud service through ingress. A replacement Go pod sees the same mounted data.
- [ ] Start with invalid configuration, correct it, and show the running process proceeds. Start with Postgres offline, restore connectivity and verify the original process migrates and becomes ready.
- [ ] Exercise the running reload loop with a file edit that changes several reloadable settings and a startup value. Verify the reloadable changes take effect together and the startup value takes effect only after restart. Check last-valid retention after a malformed file or invalid global setting. With two namespace names, check an explicit zero, override removal, isolation of a bad entry at startup and on reload, and recovery after correction.
- [ ] Pending migrations run once under the Goose lock; subsequent startup sees the current schema, and a failing migration leaves readiness false with an actionable log. Verify the shared-volume capabilities and Secret/mount boundaries used by the deployment.

## Handoff and deferred work

Establish the engine-trace directory on the shared volume and hand PR 2.1.3 the installer-configured mount for the same backing directory in the engine namespace. Keep the management client TLS Secret mount required. On a clean cluster, the Go pod can wait for PR 2.1.3 to issue that Secret. Run independent checks now and use the Pipeline notes below to hand off each blocked deployment or readiness check to PR 2.1.3.

Use the module and build commands from [PR 2.1.1](PR-2.1.1-console-home.md). Hand the runnable deployment, config/Secret names, migration entry point and readiness check to [PR 2.1.3](PR-2.1.3-engine-observation.md) and [PR 2.1.4](PR-2.1.4-owner-applications.md). [PR 2.1.4](PR-2.1.4-owner-applications.md) owns the owner/application schema, [PR 2.1.5](PR-2.1.5-stage-and-validate.md) staging records and ZIP storage, and [PR 2.1.6](PR-2.1.6-first-publication.md) accepted requests and the current-publication pointer. [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns live quota processing, quota reads and subscription interval consumers. Keep credentials out of handoff evidence.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Deployment, storage, configuration, credentials and migration readiness span multiple failure boundaries.
- **Reassessment triggers:** Reassess deployment details if existing installer assets already supply part of this contract.
