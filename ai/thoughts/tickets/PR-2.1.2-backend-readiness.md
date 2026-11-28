# PR 2.1.2 — Start the Full Cloud backend on K3s with durable storage and Postgres

## Outcome

A deployed Go process becomes ready on valid configuration and a migrated database. An operator can bring Postgres online after Go starts and see that same process recover.

## Context

GitHub PR 2, Go backend Phase 1. Build on PR 2.1.1’s Full Cloud entry point. This ticket supplies the runtime used by all remaining Phase 1 work. Schema migrations for specific business features belong to the tickets that introduce them.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Configuration and migrations](../future/go-backend-roadmap.md#go-backend-configuration) · [K3s ingress](../future/roadmap.md#starting-point-for-placement-and-routing) · [Shared storage and installation](../future/roadmap.md#durable-execution-traces)

## Requirements

Deploy one Go replica with the `Recreate` strategy. A planned upgrade can briefly interrupt the Cloud API. Keeping the old and new Go processes from serving concurrently preserves Phase 1’s in-process Publish ordering.

Install a Role in the engine namespace granting `list` and `watch` on core `pods`. Bind it there to the Go backend service account in the management namespace. Hand that identity and the engine label selector to PR 2.1.3.

Set the Phase 1 synchronous Invoke response budget to 90 seconds at both the ingress and the Go HTTP server. Scope the timeout so long-lived Console streams retain their streaming behavior. Configure ingress to pass the inclusive 20 MiB invocation boundary and the one-byte-over case through to Go for its documented response. The 1,000,000-byte staging limit applies to the ZIP itself.

Build a repeatable Full Cloud installer in this repository using a script and deployment manifests. Take the designated K3s context as an explicit input. The operator supplies the test cluster and authorized database access. Use the installation’s Supabase issuer in production. The Phase 1 test deployment takes the local test issuer from PR 2.1.4 through the same startup setting. Record nonsecret environment identifiers and credential references in the handoff. The archived PR 1 cluster name is historical.

For the default encrypted Longhorn installation, prompt without echo for the operator’s saved volume passphrase and create the StorageClass-referenced Secret before provisioning. Also accept a compatible operator-supplied volume. Keep passphrases and database credentials out of logs and evidence.

- Treat the sibling `loomspan-cloud-test-suite/deploy/k3s` manifests and this repository’s `management-backend/` at commit `75358f8` as historical PR 1 examples. Reuse individual deployment patterns only where they satisfy the current contracts. The milestone supplied a full `database-url` Secret, supported transaction pooling and applied migrations separately. This deployment reads connection settings from YAML and the password from its Secret-mounted file, supports the direct or session-pooler endpoint, and runs embedded Goose migrations.
- Mount the pack directory writable by Go. The milestone’s read-only pack PVC and pre-seeded global `<digest>.zip` lookup served a fixture workflow. PR 2.1.5 owns staging original bytes and application-scoped ZIP references; preserve its ability to account for, protect and remove packs per application. The old global path is historical, while SHA-256 over exact ZIP bytes remains required.
- Install the Go deployment and Service behind the installation’s single HTTPS ingress. Configure ingress timeouts for synchronous calls. Establish the shared ReadWriteMany volume and the durable pack/config paths. The default is encrypted Longhorn; allow a compatible operator-supplied volume. Follow the volume contract for passphrase provisioning, Secret references, atomic rename and cross-node locks. Keep workload mounts scoped, including the read-only Go config directory and exclusion of operator-only recovery material.
- The installer supplies the engine discovery label and matching pod selector, fixed read-only mount paths for the management client certificate, private key and trusted CA, and a fixed writable pack mount path. Keep these conventions in the deployment and its Go wiring. Record the exact selector and TLS paths for PR 2.1.3 and the pack root for PR 2.1.5. The mounted YAML remains the sole operator-edited source of Go settings.
- At startup, validate both YAML sections before readiness. In `startup`, read the trusted Supabase issuer and Postgres host, port, database, user, TLS settings and password-file path. Mount the password from a separate Secret at that path. If the file is unreadable or unparseable, or an installation-wide setting is invalid, stay unready, log the identifiable setting and periodically reread the file.
- Implement the full `reloadable` schema. The subscription permission-check interval defaults to 60 seconds and must be positive. Manual-upload, engine-trace and pack-history quotas each default to 100,000,000 bytes. Each application ID can override any subset. An explicit zero sets a zero-byte quota and removing an override restores the current default.
- After readiness, periodically reread the same file and validate `reloadable` independently. Apply valid reloadable settings together. Startup edits take effect on restart. A read, parse or installation-wide reload validation failure logs the error and preserves the last valid settings. At startup and on reload, an invalid application entry logs its ID and error and marks that whole application skipped for quota processing until corrected. Other applications continue with their effective settings.
- Use native PostgreSQL for runtime and migrations, including hosted Supabase direct or session-pooler endpoints. Embed versioned SQL with Goose and enable its Postgres migration lock. While Postgres is offline stay unready and retry with capped backoff; migration failure is logged and keeps Go unready. Separate backend readiness from whether an application is published or an engine is available.

## Acceptance criteria

- [ ] On the designated shared volume, place two pods on different schedulable nodes and record their actual node names. While the first holds an exclusive lock, the second must fail to acquire it on the same file and must succeed after release. If only one node is available, continue independent checks and carry this check as pending until verified across nodes.

- [ ] Deploy into K3s, mount configuration and persistent pack storage, and reach the Full Cloud service through ingress. A replacement Go pod sees the same mounted data.
- [ ] Start with invalid configuration, correct it, and show the running process proceeds. Start with Postgres offline, restore connectivity and verify the original process migrates and becomes ready.
- [ ] Exercise the running reload loop with a file edit that changes several reloadable settings and a startup value. Verify the reloadable changes take effect together and the startup value takes effect only after restart. Check last-valid retention after a malformed file or invalid global setting. With two application IDs, check an explicit zero, override removal, isolation of a bad entry at startup and on reload, and recovery after correction.
- [ ] Pending migrations run once under the Goose lock; subsequent startup sees the current schema, and a failing migration leaves readiness false with an actionable log. Verify the shared-volume capabilities and Secret/mount boundaries used by the deployment.

## Handoff and deferred work

Use the module and build commands from [PR 2.1.1](PR-2.1.1-console-home.md). Hand the runnable deployment, config/Secret names, migration entry point and readiness check to [PR 2.1.3](PR-2.1.3-engine-observation.md) and [PR 2.1.4](PR-2.1.4-owner-applications.md). [PR 2.1.4](PR-2.1.4-owner-applications.md) owns the owner/application schema, [PR 2.1.5](PR-2.1.5-stage-and-validate.md) staging records and ZIP storage, and [PR 2.1.6](PR-2.1.6-first-publication.md) accepted requests and the current-publication pointer. [Phase 4](../phases/go-backend-phase-4-diagnostics-and-deletion.md) owns live quota processing, quota reads and subscription interval consumers. Keep credentials out of handoff evidence.

## Pipeline notes

At pickup, read the prerequisite tickets’ implementation handoffs and check this ticket for incoming notes. If a check carries over, record its command, observed failure, reason and receiving ticket in the existing implementation artifact. Link that artifact under the receiving ticket’s Pipeline notes so a fresh context can find it. The recipient reruns the check and records the result.

The developer commits between tickets locally. Use the current checkout and its local history as the integration baseline; no push, remote PR, or merge is required between tickets. An intermediate build may be broken where integration belongs to a later Phase 1 ticket. Exercise the checks that can run now. In the handoff, record the affected build or check, the cause, and which ticket will complete and verify it. Skip temporary compatibility code whose only purpose would be to make this checkpoint build.

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** Deployment, storage, configuration, credentials and migration readiness span multiple failure boundaries.
- **Reassessment triggers:** Reassess deployment details if existing installer assets already supply part of this contract.
