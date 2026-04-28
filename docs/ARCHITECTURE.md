# JVS Architecture

**Status:** active save point architecture
**Last Updated:** 2026-04-27

## Overview

JVS is a filesystem-native save point layer for real workspace folders. It
keeps control data under `.jvs/` or equivalent project storage and leaves
workspace folders as ordinary directories that existing tools can read and
write directly.

JVS chooses the best available materialization engine for the target
filesystem: JuiceFS clone when supported, reflink copy when available, and
portable recursive copy everywhere else. Public performance claims are scoped
to the selected engine and filesystem support.

## Design Principles

1. Control/data separation: JVS control data is never managed workspace
   payload.
2. Filesystem-native operation: a workspace is a normal real folder.
3. Save point-first history: the public saved unit is a save point, not a Git
   commit or branch.
4. Preview before destructive restore: file replacement is plan-bound and
   revalidated.
5. Recoverable mutation: interrupted restore has status/resume/rollback.
6. Verifiable state: checksums, payload hashes, doctor checks, and audit
   records make repository state inspectable.

## Public Surface

The public contract uses these terms:

| Term | Meaning |
| --- | --- |
| `folder` | The real filesystem directory where work happens. |
| `workspace` | The JVS name for a managed real folder. |
| `save point` | Immutable saved managed-file content plus creation facts. |
| `history` | Save point listing and discovery. |
| `view` | Read-only materialization of a save point or path. |
| `restore` | Copy save point content into a workspace without rewriting history. |
| `cleanup` | Product term for plan-bound deletion of unprotected save point storage. |
| `recovery plan` | Durable interrupted-restore recovery record. |

Visible user-facing commands are organized around:

```text
init
save, history, view, restore
workspace new
status, doctor, recovery
completion
```

Commands outside this visible surface are not public product vocabulary.

## System Components

```text
+------------------------------------------------------------------+
|                            JVS CLI                               |
| init | save | history | view | restore | workspace new | recovery |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                       Public output facade                       |
| save point terms | JSON envelopes | stable errors | progress     |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                         Core services                            |
| save creation | restore plans | workspace lifecycle | repository  |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                    Engines and verification                      |
| juicefs-clone | reflink-copy | copy | checksums | payload hashes |
+-------------------------------+----------------------------------+
                                |
+-------------------------------v----------------------------------+
|                        Storage boundary                          |
| .jvs control plane + real workspace folders                      |
+------------------------------------------------------------------+
```

## CLI Layer

**Package:** `internal/cli/`

The CLI layer owns user-facing behavior:

- Parse commands, global targeting flags, and command-specific flags.
- Resolve the project and workspace deterministically.
- Enforce unsaved-change safety before destructive operations.
- Render human output and JSON envelopes.
- Map errors from internal terms to save point vocabulary.
- Keep public help aligned with the save point command surface.

## Save Point Lifecycle

Save captures managed files and publishes a new save point atomically.

```text
User: jvs save -m "fixed bug"
        |
        v
CLI resolves workspace and message
        |
        v
Capacity and mutation preconditions are checked
        |
        v
Creator materializes managed files into unpublished staging
        |
        v
Payload hash and descriptor checksum are computed
        |
        v
Save point is published atomically
        |
        v
Workspace newest save point, provenance, and audit log are updated
```

Rules:

- Control data and ignored/unmanaged files are not captured.
- A save point becomes visible only after payload and descriptor durability
  requirements are complete.
- Failed or interrupted saves must not expose partial save points.
- Save records provenance from workspace creation and restore when applicable.

## Restore Lifecycle

Restore is split into preview and run.

```text
User: jvs restore <save>
        |
        v
CLI resolves source save point
        |
        v
Preview computes impact and expected target evidence
        |
        v
Plan is written; no files are changed
        |
        v
User: jvs restore --run <plan-id>
        |
        v
Run reloads plan and revalidates target evidence
        |
        v
Recovery plan and backup are created
        |
        v
Managed files are replaced
        |
        v
Workspace file-source metadata is updated; history is unchanged
```

Whole-workspace restore replaces managed files so they match the source save
point. Path restore replaces one workspace-relative path. Both modes leave save
point history unchanged and record provenance for the next save.

If a restore cannot finish safely, the recovery plan remains active and blocks
new restore runs in that workspace until `jvs recovery resume` or
`jvs recovery rollback` resolves it.

## Workspace Lifecycle

`jvs workspace new <name> --from <save>` creates a real workspace folder from
source save point content.

Rules:

- The source workspace and save point are unchanged.
- The new workspace's newest save point is `none`.
- The new workspace records `started_from_save_point`.
- The first save in the new workspace starts a new history rather than
  inheriting the source history.

Workspace selection is done by changing directories or using `--workspace`.
JVS does not virtualize the shell working directory.

## Repository Model

The repository/project coordinates:

- repo identity and format version
- save point descriptors and payload storage
- workspace metadata and real paths
- runtime operation records
- recovery plans
- audit records
- cleanup plans/tombstones

The repository must keep runtime state separate from durable published state.
Runtime lock files, operation records, and cleanup runtime plan files are
non-portable across backup/migration.

## Engine Model

JVS materializes save point payloads through an engine abstraction.

| Engine | Public class | Requirement |
| --- | --- | --- |
| `juicefs-clone` | Constant-time metadata clone when supported | JuiceFS mount plus successful clone support |
| `reflink-copy` | Tree walk with copy-on-write data sharing where supported | Filesystem reflink support |
| `copy` | Portable recursive copy | Any supported filesystem |

Engine metadata must report effective engine, performance class,
metadata-preservation behavior, and fallback/degradation reasons when they
matter.

## Integrity Model

JVS uses two save point integrity layers:

- Descriptor checksum: SHA-256 over canonical descriptor fields, excluding
  mutable verification state.
- Payload root hash: deterministic SHA-256 over the materialized managed-file
  tree.

`jvs doctor --strict` validates repository layout, publish state, workspace
provenance/lineage, runtime hygiene, integrity, and audit-chain health.

## Doctor And Repair

Doctor checks repository health and reports stable findings.

Automatic repair is limited to runtime state:

- `clean_locks`
- `clean_runtime_tmp`
- `clean_runtime_operations`

Repairs that rewrite durable save point history, workspace provenance, or
audit history are outside the public automatic repair surface.

## Cleanup Architecture

Cleanup is a product concept for reviewed deletion of unprotected save point
storage.

Required product layering:

- Public docs say cleanup preview/run.
- Cleanup preview must not delete.
- Cleanup run must bind to a reviewed plan and revalidate before deletion.
- Cleanup protects live workspace needs, active views, active source reads,
  active operations, active recovery plans, and kept save points when keep is
  promoted.
- Deleted save points require tombstone/audit information for later errors.

## Audit Boundary

Audit records make operation history tamper-evident by linking each record to
the previous record hash. The audit chain is checked by `jvs doctor --strict`.

Representative event classes include save creation, restore preview/run,
workspace creation, recovery, doctor checks, and cleanup runs.
Signing, external trust policy, and key management are future directions.

## Implementation Storage Boundary

JVS owns its control data and implementation storage. Architecture docs may
refer to implementation packages when needed, but public docs, CLI help, JSON
facades, and release notes use folder, workspace, save point, doctor,
recovery, and cleanup terminology.

## Internal Component Map

The implementation is split into CLI targeting/rendering, repository identity,
save point publish, restore planning, recovery, workspace metadata, engine
materialization, integrity, doctor, cleanup, audit, and source-protection
components. Package paths are code facts and must not appear as product
vocabulary in user-facing guidance.

## Related Documents

- [21_SAVE_POINT_WORKSPACE_SEMANTICS.md](21_SAVE_POINT_WORKSPACE_SEMANTICS.md)
  - detailed save point semantics
- [02_CLI_SPEC.md](02_CLI_SPEC.md) - CLI contract
- [06_RESTORE_SPEC.md](06_RESTORE_SPEC.md) - restore preview/run/recovery
- [13_OPERATION_RUNBOOK.md](13_OPERATION_RUNBOOK.md) - operations and drills
- [18_MIGRATION_AND_BACKUP.md](18_MIGRATION_AND_BACKUP.md) - backup/migration
- [12_RELEASE_POLICY.md](12_RELEASE_POLICY.md) - release gates
- [RELEASE_EVIDENCE.md](RELEASE_EVIDENCE.md) - release evidence ledger
