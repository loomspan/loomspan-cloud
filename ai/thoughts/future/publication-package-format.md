# Loomspan Cloud publication ZIP

A V1 publication pack contains the companion instructions, complete Framework skill set, and execution configuration for one application. V1 requires neither a manifest nor a version file inside the archive. Cloud records `formatVersion: 1` outside the ZIP.

[Go upload checks](go-backend-roadmap.md#zip-checks-before-staging) · [Java ZIP adapter checks](java-execution-engine-roadmap.md#cloud-zip-adapter)

## Directory layout

The ZIP’s top-level directory and the companion’s `SKILL.md` frontmatter `name` both equal the destination application’s namespace. Paths below are relative to that directory. [Agent Skills specification](https://agentskills.io/specification)

```text
u42-order-support/
  SKILL.md
  references/
    order-statuses.md
  loomspan/
    execution.yaml
    skills/
      orders/
        find-order.yaml
      explain-status.yaml
```

## Required companion entry point

`SKILL.md` must have valid Agent Skills YAML frontmatter with a directory-matching `name` and a `description` explaining what the application does and when to use it. Its Markdown body must contain instructions for callers. For example:

```markdown
---
name: u42-order-support
description: Look up an order and explain its status to a support representative.
---

Ask for the order number. Use the published order lookup skill in the
authorized Loomspan application. Consult references/order-statuses.md
when explaining the result.
```

## Execution inputs

Only these files are allowed under `loomspan/`. Each must be a valid document of its indicated type.

| Path | Count | Contents |
| --- | --- | --- |
| `loomspan/execution.yaml` | Exactly one | The portable configuration for connections, models, and executor settings. Use `loomspan: {}` for an empty configuration, including in a pack with zero skills. Any namespace key under `loomspan.executor-config` must match the target application. |
| `loomspan/skills/**/*.yaml` | Zero or more | One Framework skill per file. Every directory and filename in a skill definition’s pack-relative path must be lowercase. Report the full path and the offending component on a spelling error. |

`loomspan.session` and `loomspan.execution-trace` are reserved for administrative settings. Either key, even with an empty section, makes `loomspan/execution.yaml` invalid. Downloads contain the portable pack. Promotion uses the destination application's administrative settings.

Literal values are allowed wherever the underlying Framework field accepts them.

### Namespace and skill names

A namespace and a local skill name each match `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`, contain 1–64 ASCII characters, and exclude the substrings `anthropic` and `claude`. Validate names as written. Each skill must satisfy `length(namespace) + 2 + length(localName) <= 64` and have a unique local name within its namespace.

In authored YAML, `find-order` refers to the skill in the owning namespace. A qualified reference names both components explicitly, as in `u42-order-support:find-order`.

### Application variables

A mapping with exactly one entry such as `{ $var: app.region }` refers to a case-sensitive variable name. The name must be a nonblank YAML string with no surrounding whitespace. A mapping with additional keys is ordinary data and can contain nested markers. Each resolved value is one literal string.

In `loomspan/execution.yaml`, scan each immediate value under `loomspan.connections` and `loomspan.executor-config`. In each skill document whose root declares `executor`, scan its top-level `config` when it is a mapping. Within these sections, markers can occur in mapping keys, values, and sequence elements. The connection names and namespace selector keys sit outside the scan. A name used only by a skill still belongs in the install map. [Discovery examples and conformance checks](application-variable-conformance.md)

## Client-only export

The client-only form contains the named directory and every entry at its original relative path except the top-level `loomspan/` subtree. Treat all other material, including `README.md`, scripts, references, assets, and custom folders, as distributable to invited users. Keep private maintenance notes outside the publication ZIP.

## Archive rules

Only regular files and directories are allowed. Symbolic links, other entry types, and ambiguous paths such as `references/../loomspan/execution.yaml` are invalid. Paths retain their authored spelling and must be unique when compared without regard to case across the entire pack. For example, `references/Orders.md` and `references/orders.md` cannot appear together.

These limits are inclusive and cover the entire archive, including companion materials.

| Measure | V1 maximum |
| --- | --- |
| Whole ZIP, including ZIP metadata | 1 MB (1,000,000 bytes) |
| Total expanded contents of all files | 10 MB (10,000,000 bytes) |
| Archive entries, including explicit directory entries | 1,000 |

V1 imposes no separate per-file package size limit.
