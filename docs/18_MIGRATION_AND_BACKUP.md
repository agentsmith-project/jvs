# Migration & Backup

JVS does not provide remote replication. Use filesystem or JuiceFS replication
tools and preserve the save point control plane carefully.

## Recommended Method

Use `juicefs sync` or an equivalent storage-level sync for repository
migration. Treat active runtime state as non-portable.

## Pre-Migration Gates

1. Freeze writers and stop agent jobs.
2. Ensure no active restore recovery plans:
   ```bash
   jvs recovery status
   ```
3. Run:
   ```bash
   jvs doctor --strict
   ```
4. Create final save points for critical workspaces:
   ```bash
   jvs save -m "pre-migration"
   ```
5. Record the newest save point IDs with `jvs history --json`.

## Runtime-State Policy

Runtime state is non-portable and must not be migrated as authoritative state:

- active `.jvs/locks/` repository mutation locks
- active `.jvs/intents/` operation records
- active `.jvs/gc/*.json` internal cleanup runtime plans

Destination MUST rebuild runtime state:

```bash
jvs doctor --strict --repair-runtime
```

Mutation locks are host/process-specific; a lock copied from another host is
treated as held and can block destination writes until repaired. Active cleanup
runtime plans are repository-bound runtime state; create a new cleanup preview
after migration instead of reusing copied plans.

## Migration Flow

1. Mount source and destination volumes.
2. Sync repository data excluding runtime state:
   ```bash
   juicefs sync /mnt/src/myrepo/ /mnt/dst/myrepo/ \
     --exclude '.jvs/locks/**' \
     --exclude '.jvs/intents/**' \
     --exclude '.jvs/gc/*.json' \
     --update --threads 16
   ```
3. Validate destination:
   ```bash
   cd /mnt/dst/myrepo
   jvs doctor --strict --repair-runtime
   jvs doctor --strict
   jvs history --json | jq '.data.save_points[:10]'
   ```
4. Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`.

## What To Sync

Portable save point state:

- `.jvs/format_version`
- `.jvs/descriptors/`
- `.jvs/audit/`
- `.jvs/snapshots/` as the internal storage directory for save point payloads
- `.jvs/worktrees/` as the internal storage directory for workspace metadata
- `.jvs/gc/tombstones/` if present

Payload state:

- the adopted main folder contents
- selected additional workspace folders

Do not copy `.jvs/locks/`, `.jvs/intents/`, or `.jvs/gc/*.json` as
authoritative state.

## Backup Restore Drill

1. Restore backup to a fresh volume.
2. Run `jvs doctor --strict --repair-runtime`.
3. Run `jvs doctor --strict`.
4. Confirm history is readable:
   ```bash
   jvs history
   ```
5. Create a new workspace from an older save point:
   ```bash
   jvs workspace new restore-drill --from <older-save>
   ```
6. In the new workspace, run `jvs status` and confirm:
   - `Started from save point` is the source save point
   - `Newest save point: none`
   - `Unsaved changes: no`
7. Preview and run a path restore in the drill workspace.
8. Record the source save point, new workspace name, restore plan ID, and final
   status in the operations log.

## Historical/Internal Terminology

Public migration language is save point and workspace. `.jvs/snapshots` and
`.jvs/worktrees` are internal storage names; they are not a rollback to older
public terminology and provide no user-facing behavior.
