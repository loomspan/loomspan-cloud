# PR 1.1 — Adapt Cloud publication ZIPs for Framework

## Outcome

Java can read the execution inputs from a Cloud publication ZIP within V1 resource limits and preserve the source paths Framework needs for diagnostics. Validate and publish can use the same adapter for the original ZIP bytes.

## Requirements

- Accept the ZIP from the multipart `pack` part and enforce the inclusive 1,000,000-byte limit on that part before it is fully buffered in memory. Count the ZIP metadata in that limit. Enforce the inclusive 1,000-entry limit, including explicit directories, before allocating a full archive index. Read in bounded chunks and enforce the inclusive 10,000,000-byte expanded limit before retaining each chunk. Count expansion performed to skip companion files. Skip their contents when the reader supports doing so.
- Check every entry path for ambiguity and require exactly one top-level directory named for the requested namespace. Under `loomspan/`, enforce the execution layout, lowercase skill paths, regular execution files, and unique paths ignoring case. Require exactly one `loomspan/execution.yaml`. Its minimal portable content is `loomspan: {}`. The only other permitted files there are one-skill documents at `loomspan/skills/**/*.yaml`, including files directly under `skills/`. Preserve authored spelling. For example, reject `references/../loomspan/execution.yaml` even when the reader skips companion content. Apply the archive limits without adding a per-file cap. Report the offending path, including the offending component for a lowercase violation.
- Reject authored `loomspan.session` and `loomspan.execution-trace` sections even when empty. Report the offending field paths. Preserve each skill’s exact pack-relative path as its Framework `SkillDocument.sourceName`. Leave validation of Framework definitions, references, and the executor configuration owner key to Framework.

## Acceptance criteria

- [ ] A valid archive with companion files, a nested skill, and portable execution configuration yields the expected execution documents and exact source names such as `loomspan/skills/orders/find-order.yaml` (relative to the namespace directory). A valid zero-skill archive also adapts successfully.
- [ ] ZIP bytes, expanded bytes, and entry count are accepted at their exact limits and rejected above them. Verification demonstrates the limits apply during reading and that directory entries and any expansion performed while skipping count as specified.
- [ ] An ambiguous path, incorrect top-level directory, invalid execution layout, uppercase skill path, nonregular execution file, or case-insensitive execution path collision is rejected. Reserved configuration sections fail with their field paths.

## Context

GitHub PR 1, ticket 1.1 of 5. Start here. [PR 1.2](pr-1.2-validate-and-publish-engine-generations.md) wires the adapter into both endpoints and handles Framework candidate validation and installation. Go’s whole-archive staging checks and the companion entry point belong to the upload workflow.

[Java ZIP adapter](../future/java-execution-engine-roadmap.md#cloud-zip-adapter) · [Execution layout and archive rules](../future/publication-package-format.md#execution-inputs)

## Execution profile

- **Recommended:** Full 5-Step Pipeline
- **Confidence:** high
- **Rationale:** The adapter enforces a trust and resource boundary for tenant-supplied archives.
- **Reassessment triggers:** Verify where multipart bytes are first buffered and whether the ZIP reader exposes the entry types and actual expansion needed by the contract.

## Pipeline notes

PR 1 tickets are committed locally in order without syncing between them. Intermediate commits may have incomplete wiring or build failures at the ticket boundary. Verify this ticket’s behavior where possible and record any verification that depends on later tickets. The cumulative milestone must build and pass its end-to-end checks in PR 1.5.
