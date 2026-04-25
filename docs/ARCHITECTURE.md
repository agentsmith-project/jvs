# JVS Architecture

**Version:** v0 public contract
**Last Updated:** 2026-04-25
**Status:** Active

---

## Overview

JVS (Juicy Versioned Workspaces) is a filesystem-native checkpoint layer for
real workspace directories. It keeps control-plane metadata under the repo root
and leaves workspace payload directories as ordinary files that existing tools
can read and write directly.

JVS chooses the best available materialization engine for the target
filesystem: JuiceFS clone when supported, reflink copy when available, and
portable recursive copy everywhere else. Public performance claims are scoped
to the selected engine and filesystem capability.

### Design Principles

1. **Control/data separation** - Metadata lives under `.jvs/`; workspace
   payload roots contain user data only.
2. **Filesystem-native operation** - Workspaces are normal directories, not a
   virtual filesystem or hidden index.
3. **Checkpoint-first history** - The stable versioned unit is a complete
   workspace tree at one point in time.
4. **Local-first behavior** - JVS does not require a server or remote protocol.
5. **Verifiable state** - Descriptor checksums, payload hashes, doctor checks,
   and audit records make repository state inspectable.

---

## Public Surface

The v0 public contract uses these terms consistently:

| Term | Meaning |
| --- | --- |
| `repo` | A repository root containing `.jvs/` metadata plus one or more workspaces. |
| `workspace` | A real payload directory registered in the repo. |
| `checkpoint` | An immutable full-tree record of a workspace. |
| `current` | The checkpoint currently materialized in a workspace according to JVS metadata. |
| `latest` | The newest checkpoint on the workspace's active line. |
| `dirty` | Live workspace files differ from `current`, or JVS cannot prove they match. |

Stable user-facing commands are organized around setup, checkpointing,
workspace targeting, verification, doctor checks, and two-phase cleanup:

```text
init, import, clone, capability
info, status
checkpoint, checkpoint list, diff, restore
fork, workspace list, workspace path, workspace rename, workspace remove
verify, doctor, gc plan, gc run
```

`restore current` and `restore latest` are the public refs for restoring the
materialized checkpoint or returning to the latest checkpoint. No other
restoration mode is part of the v0 CLI contract.

---

## System Components

```text
+------------------------------------------------------------------+
|                            JVS CLI                               |
| init, checkpoint, restore, fork, workspace, verify, doctor, gc    |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                       Public output facade                       |
| text output, JSON envelopes, stable error codes, progress policy |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                         Core services                            |
| checkpoint creation | restore | workspace lifecycle | repository |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                    Engines and verification                      |
| juicefs-clone | reflink-copy | copy | checksums | payload hashes |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                        Storage boundary                          |
| .jvs control plane + workspace payload directories               |
+------------------------------------------------------------------+
```

---

## CLI Layer

**Package:** `internal/cli/`

The CLI layer owns user-facing behavior:

- Parse commands, global targeting flags, and command-specific flags.
- Resolve the repo and workspace deterministically.
- Enforce dirty-state safety before destructive operations.
- Render human output and JSON envelopes.
- Map failures to stable error classes and hints.

The CLI should speak public v0 vocabulary even when it calls packages whose
names retain older implementation terminology.

---

## Checkpoint Lifecycle

Checkpoint creation captures the targeted workspace payload and publishes it
atomically. A successful checkpoint must not become visible until both payload
and descriptor durability requirements are complete.

High-level flow:

```text
User: jvs checkpoint "fixed bug"
        |
        v
CLI validates targeting and dirty state
        |
        v
Creator materializes payload through selected engine
        |
        v
Payload hash and descriptor checksum are computed
        |
        v
Checkpoint is published atomically
        |
        v
Workspace current/latest metadata and audit log are updated
```

User-visible rules:

- A checkpoint captures exactly one workspace payload root.
- Published checkpoints are immutable.
- A new checkpoint advances only from the workspace's latest checkpoint.
- Checkpoint refs are `current`, `latest`, full IDs, unique prefixes, and
  unambiguous tags.

---

## Restore Lifecycle

Restore replaces the live workspace contents with a referenced checkpoint.

High-level flow:

```text
User: jvs restore latest
        |
        v
CLI resolves the checkpoint ref
        |
        v
Dirty-state policy is enforced
        |
        v
Descriptor and payload integrity are checked
        |
        v
Workspace payload is replaced from checkpoint storage
        |
        v
Workspace current metadata is updated
```

Restoring an older checkpoint makes `current` older than `latest`. Users return
to the normal advance point with `jvs restore latest` or create a separate
workspace with `jvs fork`.

---

## Workspace Lifecycle

The default workspace is `main`. Additional workspaces are created with
`jvs fork` and managed with `jvs workspace ...` commands.

Responsibilities:

- Create a workspace from `current`, `latest`, or an explicit checkpoint ref.
- List, rename, remove, and resolve workspace payload paths.
- Preserve payload/control-plane separation.
- Refuse unsafe removal unless the user explicitly forces it.

Switching workspaces is done by changing directories or by targeting with
`--workspace`. JVS does not virtualize the current directory.

---

## Repository Model

The repository coordinates checkpoint descriptors, workspace metadata, runtime
operation records, audit records, and cleanup plans.

Responsibilities:

- Maintain repo identity and format version.
- Load and validate checkpoint descriptors.
- Maintain per-workspace `current` and `latest` metadata.
- Resolve checkpoint refs.
- Protect active lineage during storage cleanup.
- Keep runtime state separate from durable published state.

---

## Engine Model

JVS materializes checkpoint payloads through an engine abstraction. Engine
selection is automatic and visible to users through setup output, `jvs info`,
`jvs capability`, and checkpoint metadata.

| Engine | Public class | Requirement |
| --- | --- | --- |
| `juicefs-clone` | Constant-time metadata clone when supported | JuiceFS mount plus successful clone support |
| `reflink-copy` | Tree walk with copy-on-write data sharing where supported | Filesystem reflink support |
| `copy` | Portable recursive copy | Any supported filesystem |

Engine metadata must report the effective engine, performance class,
metadata-preservation behavior, and degradation reasons when fallback occurs.

---

## Integrity Model

JVS uses two checkpoint integrity layers:

- **Descriptor checksum:** SHA-256 over canonical descriptor fields, excluding
  mutable verification state.
- **Payload root hash:** Deterministic SHA-256 over the materialized checkpoint
  payload tree.

`jvs verify` checks descriptor and payload integrity for one checkpoint or all
checkpoints. `jvs doctor --strict` additionally validates repository layout,
publish state, workspace lineage, runtime hygiene, and audit-chain health.

---

## Doctor And Repair

Doctor checks repository health and reports findings with stable severity and
machine-readable IDs.

Checks include:

- Control-plane layout and format version.
- Checkpoint descriptor and payload consistency.
- Atomic publish state.
- Workspace `current`/`latest` metadata consistency.
- Runtime operation leftovers.
- Audit-chain integrity in strict mode.

The stable public automatic repair surface is intentionally narrow:

- `clean_locks`
- `clean_runtime_tmp`
- `clean_runtime_operations`

Repairs that rewrite lineage, rebuild durable indexes, or rewrite audit history
are outside the v0 stable public repair surface.

---

## Storage Cleanup

Storage cleanup is two phase:

```text
jvs gc plan
jvs gc run --plan-id <id>
```

Protection rules:

- Checkpoints reachable from active workspace lineage are protected.
- Checkpoints referenced by active runtime operation records are protected.
- Tags are metadata only for v0 cleanup and do not create protection.
- v0 publishes no pin command, tag-retention rule, or minimum-age policy.

The plan output is the public contract for automation. The run step must use a
previous plan ID so deletion decisions are reviewable before execution.

---

## Audit Boundary

Audit records make operation history tamper-evident by linking each record to
the previous record hash. The audit chain is checked by `jvs doctor --strict`.

Representative event classes include checkpoint creation, restore, workspace
management, verification, doctor checks, and cleanup runs. Signing, external
trust policy, and key management are future directions, not v0 stable commands.

---

## On-Disk Compatibility Layout

This section describes internal storage names that remain for compatibility.
They are not public CLI vocabulary.

```text
repo/
|-- .jvs/
|   |-- descriptors/
|   |-- snapshots/
|   |-- worktrees/
|   |-- audit/
|   |-- gc/
|   `-- intents/
|-- main/
`-- worktrees/
    `-- <name>/
```

Compatibility notes:

- Checkpoint payloads are stored under `.jvs/snapshots/<id>/`.
- Workspace metadata is stored under `.jvs/worktrees/<name>/config.json`.
- Additional workspace payloads currently live under `repo/worktrees/<name>/`.
- Some internal JSON fields retain historical names such as `snapshot_id`,
  `worktree_name`, and `head_snapshot_id`; public JSON facades should expose
  checkpoint/workspace/current/latest terms where feasible.

---

## Internal Package Map

This section is implementation-facing and may use package names that preserve
historical terminology.

| Package | Responsibility |
| --- | --- |
| `internal/cli` | CLI commands, targeting, JSON facade, error rendering. |
| `internal/snapshot` | Checkpoint creation and atomic publish implementation. |
| `internal/restore` | Workspace payload replacement from checkpoint storage. |
| `internal/worktree` | Workspace metadata and on-disk compatibility paths. |
| `internal/repo` | Repository discovery, identity, format, and path helpers. |
| `internal/engine` | JuiceFS clone, reflink, and copy materialization engines. |
| `internal/integrity` | Descriptor checksums and payload hashing. |
| `internal/verify` | Checkpoint verification and lineage helpers. |
| `internal/doctor` | Repository health checks and safe runtime repair. |
| `internal/gc` | Plan/run storage cleanup implementation. |
| `internal/audit` | Tamper-evident audit append and verification helpers. |

---

## Internal Publish Protocol

The checkpoint creator follows a durable publish protocol:

1. Validate source workspace and targeting preconditions.
2. Create a runtime operation record and fsync it.
3. Materialize payload into a temporary checkpoint directory.
4. Compute the payload root hash.
5. Fsync materialized files and directories.
6. Build and fsync the descriptor temporary file.
7. Write and fsync the READY marker.
8. Atomically rename payload and descriptor into published locations.
9. Update workspace current/latest metadata last.
10. Complete runtime cleanup and append the audit event.

Crash recovery treats incomplete runtime records and temporary payloads as
non-visible until doctor reports or safely cleans them.

---

## Extension Points

### Adding A Materialization Engine

1. Implement the engine interface in `internal/engine/`.
2. Register detection and fallback behavior in the engine factory.
3. Declare metadata-preservation and performance-class behavior.
4. Update `05_SNAPSHOT_ENGINE_SPEC.md` and conformance coverage.

### Adding A Public Command

1. Define the user-facing contract in `02_CLI_SPEC.md`.
2. Implement command handling in `internal/cli/`.
3. Add error classes and JSON envelope coverage.
4. Add conformance tests and docs-contract examples.

### Adding Audit Events

1. Define the event type in `pkg/model/audit.go`.
2. Append records from the operation boundary.
3. Update `09_SECURITY_MODEL.md`.
4. Add strict doctor or audit-chain conformance coverage.

---

## Performance Characteristics

| Operation | `juicefs-clone` on supported JuiceFS | `reflink-copy` when supported | `copy` fallback |
| --- | --- | --- | --- |
| Checkpoint materialization | Constant-time metadata clone | Linear tree walk with shared data blocks | Linear data copy |
| Restore materialization | Constant-time metadata clone where available | Linear tree walk with shared data blocks | Linear data copy |
| Verify | Linear in payload size | Linear in payload size | Linear in payload size |
| Cleanup planning | Linear in checkpoint graph size | Linear in checkpoint graph size | Linear in checkpoint graph size |

JVS must report engine fallback and degradation reasons so automation can avoid
assuming constant-time behavior when the filesystem cannot provide it.

---

## Related Documents

- [CONSTITUTION.md](CONSTITUTION.md) - Core principles and non-goals
- [00_OVERVIEW.md](00_OVERVIEW.md) - Product overview
- [01_REPO_LAYOUT_SPEC.md](01_REPO_LAYOUT_SPEC.md) - Repository layout
- [02_CLI_SPEC.md](02_CLI_SPEC.md) - Stable command contract
- [03_WORKTREE_SPEC.md](03_WORKTREE_SPEC.md) - On-disk workspace compatibility
- [04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md](04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md) - Checkpoint scope and lineage
- [05_SNAPSHOT_ENGINE_SPEC.md](05_SNAPSHOT_ENGINE_SPEC.md) - Engine details
- [08_GC_SPEC.md](08_GC_SPEC.md) - Storage cleanup
- [09_SECURITY_MODEL.md](09_SECURITY_MODEL.md) - Integrity and audit
- [10_THREAT_MODEL.md](10_THREAT_MODEL.md) - Threat analysis
