# CLI Spec

**Status:** active save point public contract

This spec defines the user-facing JVS command surface. The public model is a
real folder with save points.

## Public Vocabulary

Use these terms in public help, examples, release notes, and operator-facing
procedures.

| Term | Meaning |
| --- | --- |
| `folder` | The real filesystem directory where the user works. |
| `workspace` | The JVS name for a managed real folder. `main` is the default. |
| `save point` | An immutable saved copy of the managed files in one workspace. |
| `save` | Create a new save point from the active workspace. |
| `history` | List save points and find candidates by path or message. |
| `view` | Open a read-only view of a save point or a path inside it. |
| `restore` | Copy managed files from a save point into a workspace. |
| `unsaved changes` | Managed files differ from the known save point/source state, or JVS cannot prove they match. |
| `cleanup` | Product term for deleting unprotected save point storage after preview and review. |
| `recovery plan` | Durable plan that lets an interrupted restore be inspected, resumed, or rolled back. |

## Root Help Surface

The visible root help starts with the low-mental-overhead path:

```text
jvs init
jvs save -m "baseline"
jvs history
jvs view <save> [path]
jvs restore <save>
```

Visible public commands:

```text
cleanup preview
cleanup run
completion
doctor
history
init
recovery
restore
save
status
view
workspace new
```

Commands outside this visible surface are not part of this public contract.
They must not appear in public help, examples, or release-facing workflows.

## Conventions

- Global flags: `--repo <path>`, `--workspace <name>`, `--json`, `--debug`,
  `--no-progress`, and `--no-color`.
- Non-zero exit means failure.
- `--json` emits exactly one JSON object to stdout.
- JVS does not mutate the caller's shell CWD.
- Commands that read or materialize a save point must resolve the source to one
  concrete save point ID before acting.
- Commands that overwrite managed files must refuse unsaved changes by default
  unless the user chooses an explicit safety option.

## JSON Envelope

JSON output uses this envelope:

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": true,
  "repo_root": "/abs/folder",
  "workspace": "main",
  "data": {},
  "error": null
}
```

On error, `ok` is false, `data` is null, and `error` contains a stable code,
message, and optional hint.

## Save Point IDs

Public commands that accept `<save>` require one concrete save point:

- a full save point ID
- a unique save point ID prefix

Labels, messages, and tags are not restore or view targets in the save point
contract. Search and filtering commands may return candidates, but the user or
automation must choose an explicit save point ID before a mutating operation.

## Setup

### `jvs init [folder] [--json]`

Adopt an existing folder. When no folder is provided, the shell working folder
is used. Existing files stay in place; JVS stores control data in `.jvs/` and
registers the folder as workspace `main`.

Human output must show:

- `Folder`
- `Workspace: main`
- that files were not moved or copied
- `Newest save point: none`
- `Unsaved changes: yes`
- the next suggested save command

Required JSON `data` fields include:

- `folder`
- `workspace`
- `repo_root`
- `format_version`
- `repo_id`
- `newest_save_point`
- `unsaved_changes`
- setup engine probe fields such as `effective_engine` and `warnings`

## Status

### `jvs status [--json]`

Show the active folder, workspace, newest save point, file source, restored
paths, and whether the folder has unsaved changes.

Required JSON `data` fields:

- `folder`
- `workspace`
- `newest_save_point`
- `history_head`
- `content_source`
- `started_from_save_point` when applicable
- `unsaved_changes`
- `files_state`
- `path_sources` when applicable

Human output must prefer public status words such as `Newest save point`,
`Files match save point`, `Files changed since save point`, `Files were last
restored from`, `Started from save point`, and `Unsaved changes`.

## Save And History

### `jvs save [-m message] [--json]`

Create a save point for the active workspace. A message is required, either as
`-m/--message` or as the positional message accepted by the implementation.

Rules:

- The save captures the workspace managed files, excluding JVS control data and
  ignored/unmanaged files.
- Save must hold the workspace mutation lock.
- Capacity and staging checks must fail before publishing a partial save point.
- If the workspace was created with `workspace new --from`, the first save has
  no inherited history parent and records `started_from_save_point`.
- If files were restored before saving, the new save records whole-workspace or
  path provenance so later status and cleanup protection can explain it.

Required JSON `data` fields:

- `save_point_id`
- `workspace`
- `message`
- `created_at`
- `newest_save_point`
- `started_from_save_point` when applicable
- `restored_from` when applicable
- `restored_paths` when applicable
- `unsaved_changes`

### `jvs history [--path <path>] [-n <limit>] [--grep <text>] [--tag <tag>] [--all] [--json]`

Show save points for the active workspace. `--path` searches for save points
that contain a workspace-relative path and returns candidates without changing
files. `--grep` and `--tag` are discovery filters only; neither messages nor
tags become restore/view targets.

Required JSON `data` fields:

- `workspace`
- `save_points`
- `newest_save_point`
- `started_from_save_point` when applicable

For `history --path`, required JSON `data` fields:

- `folder`
- `workspace`
- `path`
- `candidates`
- `next_commands`

`history --path` is a discovery flow. It must not restore or view anything by
itself.

## View

### `jvs view <save-point> [path]`

Open a read-only view of a save point, or a path inside it. The real folder,
workspace, and history are not changed.

Rules:

- The source must resolve to a full or unique save point ID.
- The view path is read-only.
- Open views protect their source save point from cleanup while the operation
  is active.

### `jvs view close <view-id|path>`

Close a read-only view and release the associated active view protection.

## Restore

### `jvs restore [save-point] [--path <path>] [--save-first|--discard-unsaved] [--json]`

Create a restore preview plan. Preview is the default. It does not change
files and does not change workspace history.

Forms:

- `jvs restore <save>` previews whole-workspace restore from a save point.
- `jvs restore <save> --path <path>` previews single-path restore.
- `jvs restore --path <path>` lists candidate save points for that path.

Safety options:

- `--save-first` creates a save point for unsaved changes before restore run.
- `--discard-unsaved` discards unsaved changes for the operation.
- The two options are mutually exclusive.

Required preview JSON `data` fields:

- `mode: "preview"`
- `plan_id`
- `scope`
- `folder`
- `workspace`
- `source_save_point`
- `path` for path restores
- `newest_save_point`
- `history_head`
- `expected_newest_save_point`
- `expected_folder_evidence` or `expected_path_evidence`
- `managed_files`
- `options`
- `history_changed: false`
- `files_changed: false`
- `run_command`

### `jvs restore --run <plan-id> [--json]`

Execute a previously created restore preview plan. Run must reload the plan and
revalidate the expected target state before writing files. Runtime options are
fixed by the preview plan; changing `--save-first`, `--discard-unsaved`, or
`--path` requires a new preview.

Required run JSON fields for whole-workspace restore:

- `mode: "run"`
- `plan_id`
- `folder`
- `workspace`
- `restored_save_point`
- `source_save_point`
- `newest_save_point`
- `history_head`
- `content_source`
- `unsaved_changes`
- `files_state`
- `history_changed: false`
- `files_changed: true`

Required run JSON fields for path restore:

- `mode: "run"`
- `plan_id`
- `folder`
- `workspace`
- `restored_path`
- `from_save_point`
- `source_save_point`
- `newest_save_point`
- `history_head`
- `content_source`
- `path_source_recorded`
- `path_sources`
- `unsaved_changes`
- `files_state`
- `history_changed: false`
- `files_changed: true`

## Restore Recovery

### `jvs recovery status [recovery-plan] [--json]`

List active recovery plans or show one plan. A recovery plan records the
restore plan, workspace, folder, source save point, optional path, last error,
backup availability, and recommended next command.

### `jvs recovery resume <recovery-plan> [--json]`

Resume an interrupted restore. On success, history remains unchanged, the
recovery backup is removed, and cleanup protection held by the recovery plan is
released.

### `jvs recovery rollback <recovery-plan> [--json]`

Return the workspace to the saved pre-restore state when that can be verified.
On success, history is restored to the pre-restore metadata state, recovery
backup state is resolved, and recovery cleanup protection is released.

## Workspace Creation

### `jvs workspace new <name> --from <save> [--json]`

Create another real workspace from a save point.

Rules:

- `--from` is required and must resolve to one save point ID.
- The new workspace starts with managed files copied from the source save
  point.
- The source workspace is not changed.
- The new workspace does not inherit the source history.
- `Newest save point` for the new workspace is `none` until its first save.
- The first save in the new workspace records `started_from_save_point`.

Required JSON `data` fields:

- `mode: "new"`
- `status`
- `workspace`
- `folder`
- `started_from_save_point`
- `content_source`
- `newest_save_point`
- `history_head`
- `original_workspace_unchanged`
- `unsaved_changes`

## Doctor

### `jvs doctor [--strict] [--repair-runtime] [--repair-list] [--json]`

Check repository health. `--strict` includes full save point integrity
verification. `--repair-runtime` is limited to safe runtime cleanup and
destination-local workspace folder path rebinding after filesystem migration.
If a copied external workspace still points at a source folder, strict doctor
reports an unhealthy workspace path binding until `--repair-runtime` can prove
and store the destination sibling binding. A skipped or failed rebind therefore
leaves `doctor --strict --repair-runtime` unhealthy.

Public automatic repair actions:

- `clean_locks`
- `rebind_workspace_paths`
- `clean_runtime_tmp`
- `clean_runtime_operations`
- `clean_runtime_cleanup_plans`

Doctor must not rewrite durable save point history, workspace provenance, or
audit history as an automatic repair.

## Cleanup Layering

`cleanup` is the public product term. Cleanup must remain two-stage: preview
first, then run a reviewed plan. A cleanup run must revalidate its plan before
deleting anything and must protect the stable reasons: workspace history, open
views, active recovery plans, and active operations.
Stable cleanup reasons: workspace history; open views; active recovery plans;
active operations.

Cleanup preview must explain protected save points by stable generic reasons:

- `history`
- `open_view`
- `active_recovery`
- `active_operation`

JSON uses those stable reason tokens. Human output must render them as natural
labels: workspace history, open views, active recovery plans, and active
operations.

### `jvs cleanup preview [--json]`

Create a cleanup plan for save point storage that is no longer needed by
protected history or active operations. Preview does not delete anything.

Human output must show:

- `Plan ID`
- protected save points grouped by reason
- `Reclaimable`
- `Estimated reclaim`
- the matching `jvs cleanup run --plan-id <plan-id>` command

Required JSON `data` fields:

- `plan_id`
- `created_at`
- `protected_save_points`
- `protection_groups`
- `protected_by_history`
- `candidate_count`
- `reclaimable_save_points`
- `reclaimable_bytes_estimate`

Each `protection_groups` entry contains:

- `reason`
- `count`
- `save_points`

### `jvs cleanup run --plan-id <plan-id> [--json]`

Run a reviewed cleanup plan. Run must reload and revalidate the plan before it
deletes unneeded save point storage. If the protected save point set or
reclaimable candidate set has changed since preview, run must fail and require a
fresh `jvs cleanup preview`.

Required JSON `data` fields:

- `plan_id`
- `status`

## Implementation Boundary

Implementation packages, storage paths, and metadata field names are not public
CLI vocabulary. Public help, examples, JSON facades, and release notes must use
folder, workspace, save point, history, view, restore, doctor, recovery, and
cleanup terminology.

## Boundaries

The public contract does not include remote push/pull, signing or trust
commands, merge/rebase, label-as-ref restore, tag-as-ref restore, public
partial-save contracts, public compression contracts, or complex retention
policy flags.
