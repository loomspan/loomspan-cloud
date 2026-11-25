---
date: 2026-11-24
repository: loomspan-cloud
branch: main
commit: bc217016f62528855c70cbf505dd092657cabe18
ticket: ai/thoughts/tickets/pr-1.3-authenticate-and-observe-engine-pods.md
tags: [execution-engine, console, kubernetes, mtls]
---

# K3s engine authentication and Console observation research

## Research Question

Which parts of service authentication, pod discovery, and Console observation exist in Cloud or its embedded Framework today, and what evidence is available for the single-engine integration?

## Summary

The checked-in Cloud engine is a Spring Boot bootstrap with no Go module or deployment manifests. Framework provides the instance snapshot, API-key authorization, and activity stream. The adjacent Framework checkout exposes trace persistence on each namespace row, and the previously installed starter snapshot predates that change.

## Repository State

Cloud `main` matched `origin/main` at `bc217016f62528855c70cbf505dd092657cabe18` during research on November 24 at 15:13 Pacific. The simultaneous PR 1.1 research, planning, and implementation changes belong to that preceding ticket. The following observations about the Cloud baseline use the committed tree. The adjacent Framework source was inspected read-only at `2ca9caf4b3d0e84540fbc975f0c7509face16ed5`. It had unrelated deletions of prior process documents. The cached JAR was inspected separately.

## Current Behavior and Data Flow

- The Cloud engine starts through `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6`. The committed `execution-engine/pom.xml:7` selects Spring Boot 4.2.0-RC2, Java 21, Web MVC, and Framework `1.0.0-beta.10-SNAPSHOT`. The committed tree has no server identity configuration, servlet certificate check, K3s objects, Go backend, pod watcher, or Go Console projection. PR 1.2 owns the planned Java validation and publish routes. `ai/thoughts/future/execution-engine-api.md:22` specifies their HTTPS contract.
- Framework observability is opt-in with `loomspan.observability.enabled` and `loomspan.observability.auth.api-key`. In a servlet application, the route registrar checks configuration and reserved-path collisions, creates a runtime with a UUID and registers GET routes independently of Cloud publications. Framework rejects unsuitable activation configuration. The API-key filter on `/_loomspan/observability/v1` and its descendants runs for all servlet dispatcher types and supports async. It checks a single `X-loomspan-Api-Key` header, requiring 32 to 512 printable, non-space ASCII characters, and compares it to the configured key. A rejected key gets JSON HTTP 401 without the instance identity. An accepted key creates Framework's observability operator authentication for the request, attaches `X-loomspan-Instance-Id`, and restores the preceding security context on exit.
- `GET /_loomspan/observability/v1/instance` requires the Framework operator, disallows query parameters, and reports the runtime UUID in `instanceId` and a `namespaces` array. Each source-checkout row holds `namespace`, `generationId`, nullable `publicationVersion`, and `tracePersistencePolicy` from one generation under the generation monitor. Framework can return a namespace with zero skills or null publication version. The top-level trace policy describes the active `default` generation, with an `ONERROR` fallback if that generation is absent or has no runtime. The snapshot operation does not require live monitoring availability. Observability creates the UUID during runtime activation, independent of namespace installation and pod identity.
- `GET /_loomspan/observability/v1/activity` requires live monitoring, exactly one `instanceId` and `afterCursor` query parameter, and an Accept value compatible with `text/event-stream`. It rejects `Last-Event-ID`. Framework rejects a stale UUID or evicted replay position with HTTP 410 and a future cursor as invalid. An initial authorized `GET /_loomspan/observability/v1/active-executions` exposes `resumeCursor` as a decimal string in the response, even when its item list is empty. There is no cursor in the instance snapshot. The SSE stream sends a handshake first with the instance ID and requested cursor, then activity frames whose ID is the delivery cursor. Stream admission and disconnect are separate from the instance snapshot.
- The Docker, TLS, and K3s integration remains design in this checkout. The roadmap specifies a cert-manager-owned shared Java server credential and one Go client credential, an engine ingress policy for labeled Go pods in the management namespace, and a separate common Console key per environment. The pod watch identifies the Kubernetes object and IP, and a direct authenticated request identifies the actual runtime, because the server credential is shared. The Go roadmap calls for inspection of the last disconnected snapshot, re-observation on connection/reconnection or refresh, and routing eligibility from a current snapshot on a ready, non-terminating pod. The Java roadmap keeps first publication independent of readiness and the presence of Cloud-installed namespaces.

## Key Components

| Component | Source evidence |
| --- | --- |
| Startup and dependency contract | `execution-engine/src/main/java/ai/loomspan/cloud/engine/ExecutionEngineApplication.java:6`, `execution-engine/pom.xml:7`, `execution-engine/pom.xml:16` |
| Framework Console opt-in and API-key registration | `../loomspan-framework/src/main/java/ai/loomspan/autoconfigure/LoomspanProperties.java:425`, `../loomspan-framework/src/main/java/ai/loomspan/autoconfigure/LoomspanObservabilityWebAutoConfiguration.java:115`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityApiKeyFilter.java:24` |
| Console routes and UUID lifetime | `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRouteRegistrar.java:70`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRouteRegistrar.java:131` |
| Snapshot and generation policy | `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRestController.java:54`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/dto/ObservabilityDtos.java:22`, `../loomspan-framework/src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:141`, `../loomspan-framework/src/main/java/ai/loomspan/internal/skill/SkillGenerationManager.java:228` |
| SSE baseline and admission | `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRestController.java:110`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRestController.java:155`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityRestController.java:355`, `../loomspan-framework/src/main/java/ai/loomspan/internal/observability/web/ObservabilityActivityStream.java:100` |
| Cloud trust, discovery and lifecycle contract | `ai/thoughts/future/roadmap.md:96`, `ai/thoughts/future/execution-engine-api.md:14`, `ai/thoughts/future/java-execution-engine-roadmap.md:25`, `ai/thoughts/future/go-backend-roadmap.md:108` |

## Existing Tests and Fixtures

- Cloud has no baseline test for mTLS, Console, or K3s. The earlier offline Maven run on the bootstrap assembled a Boot JAR. That establishes dependency resolution and packaging for the earlier checkout.
- Framework's `../loomspan-framework/src/test/java/ai/loomspan/internal/observability/web/ObservabilityApiKeyFilterTest.java:46` exercises independent Console operator identity and security-context restoration. At line 88 it tests malformed key headers. `ObservabilityRestIntegrationTest.java:60` exercises early discovery of an empty namespace, and lines 80, 173, and 185 cover key rejection, stream baseline, and activity request failures.
- Framework's `../loomspan-framework/src/test/java/ai/loomspan/internal/observability/web/NamespacePublicationObservationIntegrationTest.java:29` exercises distinct namespace policies, including the independent default policy. At line 86 it exercises version visibility and independent engine identities around restore. `ObservabilitySseIntegrationTest.java:64` exercises authenticated SSE handshake, replay, and resumption. These adjacent tests describe existing source coverage. This research did not execute them or contact a cluster.

## Dependencies and Operational Constraints

- The preexisting local Maven artifact `ai.loomspan:loomspan-spring-boot-starter:1.0.0-beta.10-SNAPSHOT` contains the route registrar, controller, API-key filter, and JSON DTOs. Inspecting the class contents confirms `generationId` and `publicationVersion` on `ObservabilityDtos$NamespaceStatus.class`, but the class has no `tracePersistencePolicy`. The adjacent Framework starter's `pom.xml:8` uses the same coordinates, and its HEAD `2ca9caf4` adds the namespace field. A baseline Cloud compile against the cached artifact does not demonstrate this wire contract. The namespace acceptance check requires an artifact containing that Framework change.
- Local Boot `spring-boot-web-server` 4.2.0-RC2 metadata exposes `server.ssl.client-auth`, `server.ssl.bundle`, and PEM certificate, private-key, and trust certificate configuration. The Cloud project does not configure these fields today. Issuer trust and acceptance of a specific Go leaf identity are separate checks in the ticket, and Kubernetes pod identity remains a separate observation.
- Go 1.26.1, Maven 3.9.11, and JDK 21 are available. `kubectl config get-contexts` had no contexts, and the Docker CLI could not reach the local Docker Desktop Linux daemon. The supplied WSL Ubuntu check did not find K3s. The Go module cache has no `k8s.io/client-go` download directory. No authenticated cluster, cert-manager deployment, or live Kubernetes watch was verified.

## Historical Context

Cloud commit `bc21701` added the five implementation tickets over the Java bootstrap. The PR 1 sequence allows intermediate commits and places cumulative end-to-end K3s recovery verification in PR 1.5. Framework commit `2ca9caf4` adds the namespace trace persistence field on top of its independent Console implementation.

## Open Questions

- Which K3s test environment and certificate issuer configuration will be available for deployed verification? The current checkout does not contain a cluster or issuer baseline.
- Which certificate identity labels and renewal delivery behavior will the deployment use? The contracts specify credential ownership and peer identity while leaving concrete DNS/SAN names and workload configuration to deployment.
- How will the implementation reject observations from superseded pod or connection work and coordinate instance snapshots, stream baselines, and refreshes? The Go projection has no existing implementation.

## Step Report: 1_research_codebase

STATUS: complete
ARTIFACTS:
  - ai/thoughts/research/pr-1.3-authenticate-and-observe-engine-pods.md
SUMMARY: Traced the source and cached Framework Console snapshot, key check, and SSE stream. Found the cached starter lacks per-namespace trace policy and the checkout has no Go or K3s implementation baseline.
DECISIONS:
  - none
DEVELOPER QUESTION: none
EVIDENCE: none
RECOMMENDATION: none
NEXT: Begin implementation and testing planning when the preceding Java API contract is available.
