# PR 2.1.2 — Start the Full Cloud backend on K3s with durable storage and Postgres

## Outcome

A deployed Go process becomes ready on valid configuration and a migrated database. An operator can bring Postgres online after Go starts and see that same process recover.

## Context

GitHub PR 2, Go backend Phase 1. Build on PR 2.1.1’s Full Cloud entry point. This ticket supplies the runtime used by all remaining Phase 1 work. Schema migrations for specific business features belong to the tickets that introduce them.

[Phase 1 boundary and completion evidence](../phases/go-backend-phase-1-first-application.md) · [Configuration and migrations](../future/go-backend-roadmap.md#go-backend-configuration) · [K3s ingress](../future/roadmap.md#starting-point-for-placement-and-routing) · [Shared storage and installation](../future/roadmap.md#durable-execution-traces)

## Requirements

- Install the Go deployment and Service behind the installation’s single HTTPS ingress. Configure ingress timeouts for synchronous calls. Establish the shared ReadWriteMany volume and the durable pack/config paths. The default is encrypted Longhorn; allow a compatible operator-supplied volume. Follow the volume contract for passphrase provisioning, Secret references, atomic rename and cross-node locks. Keep workload mounts scoped, including the read-only Go config directory and exclusion of operator-only recovery material.
- The installer creates one YAML config with startup and reloadable sections. Validate installation-wide settings before readiness. On invalid or unreadable config, log the identifiable setting and reread periodically. Supply the trusted issuer, Postgres host/port/database/user/TLS and password-file path; mount the password from a separate Kubernetes Secret. Quota processing and subscription consumers arrive later.
- Use native PostgreSQL for runtime and migrations, including hosted Supabase direct or session-pooler endpoints. Embed versioned SQL with Goose and enable its Postgres migration lock. While Postgres is offline stay unready and retry with capped backoff; migration failure is logged and keeps Go unready. Separate backend readiness from whether an application is published or an engine is available.

## Acceptance criteria

- [ ] Deploy into K3s, mount configuration and persistent pack storage, and reach the Full Cloud service through ingress. A replacement Go pod sees the same mounted data.
- [ ] Start with invalid configuration, correct it, and show the running process proceeds. Start with Postgres offline, restore connectivity and verify the original process migrates and becomes ready.
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
