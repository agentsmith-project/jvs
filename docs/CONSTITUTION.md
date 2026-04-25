# JVS Constitution

Version: v0 public contract
Status: Active release-facing governance
Scope: Architecture, product philosophy, and design governance

---

## 1. Core Mission

JVS is a filesystem-native checkpoint system for real workspace directories.
It gives humans and automation a local-first way to create, inspect, verify,
restore, and clean up checkpointed workspace state without a server, staging
area, merge engine, or remote protocol.

JVS is not a Git replacement. It is a workspace checkpoint layer.

---

## 2. Foundational Principles

### 2.1 Checkpoint First, Not Diff First

The primary versioned unit is a complete workspace tree.

Implications:

- No staging area.
- No merge or rebase model.
- No text-patch object store.
- No mandatory content-addressed object graph for v0.

### 2.2 Filesystem As Source Of Truth

A workspace is a real directory. Tools read and write ordinary files.

JVS must not:

- Virtualize the workspace.
- Hide alternate working states behind path remapping.
- Require users to switch state through an in-process shell.

Users and agents select workspaces by changing directories or by using
explicit targeting flags.

### 2.3 Control Plane And Payload Separation

JVS separates metadata from user payload:

| Layer | Responsibility |
| --- | --- |
| Control plane | `.jvs/` metadata, descriptors, audit records, cleanup plans, and runtime records |
| Payload | Workspace files that users and tools modify |

Critical rules:

- `.jvs/` must never be checkpointed as payload.
- Workspace payload roots must contain no control-plane artifacts.
- A checkpoint captures exactly one workspace payload root.

### 2.4 Local First

JVS assumes the filesystem already exists and is mounted. It may benefit from
JuiceFS, reflink-capable filesystems, or plain local disks, but it does not
manage object storage, credentials, or replication.

---

## 3. Public Vocabulary

Release-facing docs, CLI help, examples, and stable JSON should use:

| Term | Meaning |
| --- | --- |
| `repo` | Repository root containing `.jvs/` plus registered workspaces. |
| `workspace` | Real payload directory managed by JVS. |
| `checkpoint` | Immutable full-tree record of one workspace. |
| `current` | Checkpoint currently materialized in the targeted workspace. |
| `latest` | Newest checkpoint on that workspace's active line. |
| `dirty` | Live files differ from `current`, or JVS cannot prove they match. |

Historical implementation terms may remain in package names, storage paths, or
compatibility JSON only when explicitly scoped as internal/on-disk details.

---

## 4. Public CLI Shape

The stable command surface is:

```text
jvs init <repo-path>
jvs import <source-dir> <repo-path>
jvs clone <source-repo> <dest-repo> [--scope full|current]
jvs capability <target-path> [--write-probe]

jvs info
jvs status
jvs checkpoint [message] [--tag tag]...
jvs checkpoint list
jvs diff <from-ref> <to-ref> [--stat]
jvs restore <ref|current|latest> [--discard-dirty|--include-working]
jvs fork [<ref> <name>|<name>] [--from <ref>]
jvs workspace list
jvs workspace path [<name>]
jvs workspace rename <old> <new>
jvs workspace remove <name> [--force]
jvs verify [<checkpoint-id>|--all]
jvs doctor [--strict] [--repair-runtime] [--repair-list]
jvs gc plan
jvs gc run --plan-id <id>
```

Commands that overwrite or remove workspace files must refuse dirty state by
default. The accepted bypasses are explicit: `--discard-dirty` and
`--include-working`.

---

## 5. Repository And Workspace Model

The repo root is the control plane plus workspace container. It is not itself
the default payload root.

```text
repo/
|-- .jvs/
|-- main/
`-- workspaces managed by JVS
```

The default workspace is `main`. Additional workspaces are real directories
resolved by JVS metadata and exposed through `jvs workspace path`.

---

## 6. Checkpoint Model

A checkpoint must capture only the targeted workspace payload. It must not
include:

- The `.jvs/` control plane.
- Other workspace payload roots.
- Runtime locks, temporary operation records, or cleanup plans.

After publication:

- The checkpoint payload is immutable.
- The descriptor is immutable.
- Detected mutation is repository corruption.

---

## 7. Engine Principle

JVS adapts to filesystem capability:

| Engine | Behavior |
| --- | --- |
| `juicefs-clone` | Metadata clone when supported by a JuiceFS mount. |
| `reflink-copy` | Tree walk with copy-on-write data sharing where supported. |
| `copy` | Portable recursive copy fallback. |

Same public workflow, different materialization engine. Engine choice,
fallback, performance class, and metadata-preservation behavior must be visible
to users and automation.

---

## 8. Safety And Determinism

Required behavior:

- Every command targets one resolved repo and, where relevant, one workspace.
- `status` exposes `current`, `latest`, and `dirty`.
- Restore uses explicit refs such as checkpoint IDs, tags, `current`, and
  `latest`.
- Creating a new checkpoint is allowed only from the workspace's latest point.
- Users create another workspace with `jvs fork` when they need a separate
  line of work.

JVS should prefer refusal with a clear error over guessing the user's intent.

---

## 9. Integrity And Audit

Checkpoint history must be verifiable and tamper-evident.

Required properties:

- Descriptors carry checksums.
- Payloads carry deterministic root hashes.
- `jvs verify` validates checkpoint integrity.
- `jvs doctor --strict` validates repository layout, publish state, lineage,
  runtime hygiene, and audit-chain health.
- Audit records form a hash chain.

Signing, trust policy, and key management are future directions and are not v0
stable commands.

---

## 10. Storage Cleanup

Cleanup is two phase:

```text
jvs gc plan
jvs gc run --plan-id <id>
```

The v0 public contract protects active workspace lineage and active runtime
operation records. It does not publish pin commands, tag-retention rules, or
minimum-age retention flags.

---

## 11. Internal And On-Disk Compatibility

This section is implementation-facing and may use historical terms.

Compatibility paths and fields include:

- `.jvs/snapshots/<checkpoint-id>/` for checkpoint payload storage.
- `.jvs/worktrees/<name>/config.json` for workspace metadata.
- Internal field names such as `snapshot_id`, `worktree_name`, and
  `head_snapshot_id`.
- Package names such as `internal/snapshot` and `internal/worktree`.

These names are compatibility details, not public CLI vocabulary.

---

## 12. Non-Goals

JVS must not add these to the v0 stable contract:

- Git-compatible commit graph, staging, merge, or rebase.
- Built-in remote hosting, push, pull, or credential management.
- File locking or distributed collaboration coordination.
- Partial checkpoint contracts.
- Ignore-file contracts.
- Compression contracts.
- Complex retention policies beyond two-phase cleanup.
- Hidden virtual workspaces or path remapping.
- Built-in container, scheduler, editor, or database integrations.

Any future feature that violates these boundaries requires an explicit product
contract change before implementation.

---

## 13. Design Style

JVS favors:

- Determinism over guesswork.
- Filesystem realism over virtual abstraction.
- Visible engine behavior over hidden fallback.
- Small stable contracts over broad feature promises.
- External integration over owning unrelated infrastructure.

---

## 14. Motto

Real directories. Real checkpoints. Explicit state.
