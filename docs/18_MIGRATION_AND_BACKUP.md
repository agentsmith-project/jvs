# Migration & Backup

JVS does not provide remote replication or hot migration. Use an offline
whole-folder copy of the managed folder/repository, then let the destination
rebuild runtime state before any writes resume.

## Recommended Method

Use a cold maintenance window. Stop all JVS writers, stop agent jobs, verify
there are no active operations that need cleanup protection, then use ordinary
filesystem copy into a fresh destination path. Copy the managed
folder/repository as a whole. The destination path must not exist before
copying; do not overlay an existing repository or folder. Treat copied
non-portable JVS runtime state as raw bytes
only: the destination must rebuild runtime state before use.

## Pre-Migration Gates

1. Announce a maintenance window, stop all JVS writers, and stop agent jobs.
2. Confirm the source folder status is readable:
   ```bash
   jvs status
   ```
3. Ensure there are no active restore recovery plans. If any plan is listed,
   resume or roll it back before copying:
   ```bash
   jvs recovery status
   ```
4. Check cleanup protection state and wait until there are no open views,
   active recovery plans, or active operations:
   ```bash
   jvs cleanup preview --json
   ```
   Review `protection_groups`. Any `open_view`, `active_recovery`, or
   `active_operation` count means the migration window is not quiet yet. Do not
   run or carry this preview forward; create a fresh cleanup preview on the
   destination after validation.
5. Run:
   ```bash
   jvs doctor --strict
   ```
6. Create final save points for critical workspaces:
   ```bash
   jvs save -m "pre-migration"
   ```
7. Record the newest save point IDs with `jvs history --json`.

## Runtime-State Policy

Runtime state is non-portable and must not be migrated as authoritative state:

- in-flight write coordination
- abandoned operation bookkeeping
- active cleanup preview/run plans
- destination-local workspace folder path bindings

Destination MUST rebuild runtime state:

```bash
jvs doctor --strict --repair-runtime
```

In-flight runtime state is host/process-specific and can block destination
writes until repaired. Active cleanup preview/run plans are repository-bound
runtime state; the repair command invalidates copied cleanup plans so the
destination must create a new cleanup preview instead of reusing copied plan
IDs. Workspace folder path bindings that still point at the source volume are
destination-local: adopted `main` binds to the current folder, and external
workspace folders rebind only when the destination sibling can be proven safe.
If an external workspace sibling is missing, a symlink, or has content that
does not match the recorded content source, `doctor --strict --repair-runtime`
remains unhealthy and reports the workspace path binding until the destination
sibling is present with matching content.

## Migration Flow

1. Keep the source in the maintenance window: stop all JVS writers, keep agent
   jobs stopped, and proceed only after the pre-migration gates show no active
   operations.
2. Mount source and destination volumes, then copy the managed
   folder/repository as a whole with an ordinary filesystem copy. Use a fresh
   destination path; this example fails before copying if the destination path
   already exists:
   ```bash
   test ! -e /mnt/dst/myrepo &&
   mkdir -p /mnt/dst &&
   cp -a /mnt/src/myrepo /mnt/dst/myrepo
   ```
   The copy is an offline whole-folder copy. Do not hand-select JVS control
   paths, do not overlay a non-empty destination, and do not treat copied
   non-portable JVS runtime state as authoritative.
3. Validate destination and rebuild runtime state before any destination write:
   ```bash
   cd /mnt/dst/myrepo
   jvs doctor --strict --repair-runtime
   jvs doctor --strict
   jvs status
   jvs history --json
   jvs cleanup preview
   ```
   A non-zero doctor result means some runtime state could not be rebuilt
   safely; fix the reported binding or runtime issue before using the copied
   repository. The cleanup preview here is the fresh cleanup preview for the
   destination; do not reuse a cleanup preview created before migration.
4. Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`.
5. Resume writers only after doctor, status, history, the fresh cleanup
   preview, and the restore drill pass on the destination.

## Copy Boundary

Copy the managed folder/repository as a whole. This includes user payload and
JVS durable control data:

- repository identity and format records
- save point descriptors and payload storage
- workspace metadata
- audit records
- durable cleanup evidence when present

Payload state:

- the adopted main folder contents
- selected additional workspace folders

Non-portable JVS runtime state may exist in the raw copied bytes, but it is not
authoritative product state. Rebuild it on the destination with
`jvs doctor --strict --repair-runtime` and create a fresh cleanup preview there.

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
