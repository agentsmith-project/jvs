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
| `project` / `repo` | The JVS-managed folder, its stable repo identity, workspace list, and save point history. |
| `control data` | JVS-owned metadata, save point storage, and runtime state. In the ordinary shape it lives in `.jvs/`; in the external control root shape it lives outside the workspace folder. |
| `external control root` | Explicit advanced/operator control data location outside the workspace folder, selected with `--control-root <path> --workspace main`. |
| `save point` | An immutable project history node created from a workspace's managed files. |
| `save` | Create a new save point from the active workspace. |
| `history` | List project save points through the active workspace's pointer and provenance. |
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
repo clone
repo detach
repo move
repo rename
restore
save
status
view
workspace delete
workspace list
workspace move
workspace new
workspace path
workspace rename
```

Commands outside this visible surface are not part of this public contract.
They must not appear in public help, examples, or release-facing workflows.

## Conventions

- Global flags: `--repo <path>`, `--workspace <name>`,
  `--control-root <path>`, `--json`, `--debug`, `--no-progress`, and
  `--no-color`.
- Non-zero exit means failure.
- `--json` emits exactly one JSON object to stdout.
- JVS does not mutate the caller's shell CWD.
- Commands that read or materialize a save point must resolve the source to one
  concrete save point ID before acting.
- Commands that overwrite managed files must refuse unsaved changes by default
  unless the user chooses an explicit safety option.

## External Control Root

Most users use `jvs init [folder]`, and JVS stores control data in that
workspace folder's `.jvs/`. Advanced operators and platform integrations can
place the same workspace's control data in an explicit external control root.
This is an operator/platform profile and a control data location choice, not a
second product model.

- Create an external-control-root workspace with
  `jvs init [folder] --control-root C --workspace main`.
- After init, target it with
  `jvs --control-root C --workspace main <command>`.
- The current external control root contract is main-only; use
  `--workspace main` for this profile.
- A bare workspace folder cannot safely auto-discover an external control root;
  operator scripts must pass `--control-root C --workspace main` for status,
  save, history, view, restore, recovery, cleanup, doctor, and clone.
- For external control roots, human `status` labels the external control root as
  `Control data: C`, while `Folder` remains the workspace folder.
- JSON `status` uses `data.control_root` and omits `data.repo` for external
  control roots. Ordinary `.jvs/` status keeps `data.repo`.
- The folder argument is the workspace folder; `--control-root C --workspace
  main` is the explicit selector for this workflow.
- `--repo` is not an external control root selector; it remains an advanced
  target assertion for ordinary project paths.
- External control root doctor is strict inspection only: use
  `jvs --json --control-root C --workspace main doctor --strict`.
  `--repair-runtime` is rejected for this selector and is not an external
  control root repair path.
- For external control roots, successful restore run leaves no active recovery.
  Completed restore plan residue is non-blocking for `jvs recovery status`,
  `jvs doctor --strict`, and `jvs repo clone`. Pending, active, or malformed
  restore/recovery state must be reported through `jvs recovery status`,
  `jvs doctor --strict`, or the blocked command's public diagnostics.
- Any public `run_command`, `next_commands`, or `recommended_next_command`
  emitted while targeting an external control root must be copyable from a
  clean current directory and include `--control-root C --workspace main`.
- External control root repo clone uses a main-only target folder plus target
  control root:
  `jvs --control-root C --workspace main repo clone <target-folder> --target-control-root TC --save-points main`.
- External clone target workspace folder and target control root may be missing
  or empty directories. Non-empty/adopt/merge/overwrite target roots fail closed.
- For ordinary clone omitted `--save-points` means `all`; for external control
  root omitted `--save-points` means `main`. Operators should pass
  `--save-points main` explicitly for external clone scripts.
  `--save-points all` fails closed until imported-history protection is
  available for this control data location.
- Repo and workspace lifecycle commands are currently unsupported for external
  control roots: repo move, repo rename, repo detach, workspace move, workspace
  rename, workspace delete, and workspace new fail closed with no file changes.

## JSON Envelope

JSON output uses this envelope:

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": true,
  "repo_root": "/abs/project-or-control-root",
  "workspace": "main",
  "data": {},
  "error": null
}
```

On error, `ok` is false, `data` is null, and `error` contains a stable code,
message, and optional hint.

Top-level `repo_root` names the resolved control data root for the command. For
ordinary `.jvs/` projects that is the project folder. For external control root
commands it is the explicit control root, not the workspace folder; command data
uses `data.folder` for the workspace folder and `data.control_root` for the
control root when those fields apply.

## Transfer Reporting JSON

The implemented public CLI contract exposes `data.transfers[]` for commands
that currently materialize or copy files: save, restore preview/run, workspace
new, view, and repo clone. Each transfer record describes the source role,
destination role, materialization destination, capability probe path, published
destination, effective engine, optimized flag, performance class, degraded
reasons, warnings, and whether the copy method was checked for this operation.

Transfer roles and locations are public reporting vocabulary, not internal
storage evidence. Roles use user-facing concepts such as `workspace_content`,
`save_point_content`, `content_view`, `temporary_folder`, `save_point_storage`,
and `target_main_workspace`. Location fields may be absolute user folder paths
when the location is user-visible, or stable references such as
`save_point:<id>`, `content_view:<view-id>[/path]`, `control_data`, and
`temporary_folder` when the operation touches JVS-owned control data or runtime
state. Public JSON must not require clients to know, parse, or depend on the
internal `.jvs` layout.

The `warnings` and `degraded_reasons` arrays are also public summaries. They
must preserve useful human context without exposing raw engine stdout/stderr
diagnostics, temporary staging paths, or internal storage paths.

This contract does not promise `data.transfers[]` for commands that do not
materialize or copy files. Do not infer transfers for status, history, cleanup,
doctor, view close, workspace rename, workspace move/delete, repo move/rename,
or repo detach unless a future release explicitly promotes that field.

## Save Point IDs

Public commands that accept `<save>` require one concrete save point:

- a full save point ID
- a unique save point ID prefix

Labels, messages, and tags are not restore or view targets in the save point
contract. Search commands may return candidates, but the user or automation
must choose an explicit save point ID before a mutating operation.

## Setup

### `jvs init [folder] [--control-root C --workspace main] [--json]`

Adopt an existing folder. When no folder is provided, the shell working folder
is used. Existing files stay in place.

There are two supported control data shapes:

- Ordinary `.jvs/` project: `jvs init [folder]` stores control data in the
  workspace folder's `.jvs/` and registers the folder as workspace `main`.
- External control root: `jvs init [folder] --control-root C --workspace main`
  stores control data in `C` while the folder remains the workspace folder.

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

## Repo Clone

### `jvs repo clone <target-folder> [--target-control-root TC] [--save-points all|main] [--dry-run] [--json]`

Clone a local or mounted JVS project into a new folder. The source is the
current discovered project, or the project named by global `--repo <path>`.
`repo clone` is not Git clone and must reject URL, ssh, and scp-like remote
inputs.

Rules:

- Ordinary `.jvs/` clone requires `<target-folder>` to be missing; it must not
  already exist.
- External control root clone uses the source selector
  `--control-root C --workspace main` plus `--target-control-root TC`. Its
  target workspace folder and target control root may be missing or empty
  directories; non-empty target roots fail closed.
- `<target-folder>` must be outside every source workspace.
- The target gets a new repository identity.
- The target creates only workspace `main`.
- Source workspaces other than `main`, registered workspace path records for
  those source workspaces, runtime state, and cleanup plans are not copied.
- If any source workspace has unsaved changes, including source workspaces
  other than `main`, clone fails closed. Save those changes as a save point
  first if they should be included; source workspaces other than `main` are not
  created in the target.
- `--save-points all` copies all ready save points and writes durable imported
  clone history metadata for cleanup protection.
- `--save-points main` copies the source `main` history/provenance closure.
- `--dry-run` plans only; it must not create the target or write probe files.
- On failure, the source project is unchanged and the target must not be an
  active JVS repo. If rollback cannot safely remove a published target folder or
  target control data root, the target folder or target control data root may
  remain at the target path or be moved to a hidden quarantine; in either case,
  inspect/remove manually. If a path is moved to quarantine, JVS tells the user
  `target folder was quarantined at ...; inspect and remove it manually` or
  `target control root was quarantined at ...; inspect and remove it manually`.
  Preexisting empty target directories are restored to empty when possible.

Human output must show:

- `Source`
- `Target`
- save points copied
- `Workspaces created: main only`
- source workspaces not created
- copy method for save point storage and main workspace
- strict doctor result for a completed clone

Shared JSON `data` fields include:

- `operation`
- `source_repo_root`
- `source_repo_id`
- `save_points_mode`
- `save_points_copied_count`
- `save_points_copied`
- `workspaces_created`
- `source_workspaces_not_created`
- `runtime_state_copied`
- `transfers`
- `clone_manifest` when `--save-points all` completes
- `dry_run` for dry runs

Ordinary `.jvs/` completed clone JSON includes:

- `target_repo_root`
- `target_repo_id`

External control root completed clone JSON includes:

- `target_folder`
- `target_control_root`
- `target_repo_id`

External control root completed clone JSON does not require `target_repo_root`.

Dry-run JSON is a planning result. Because dry-run does not create the target
repo, it must not require an actual `target_repo_id`; completed clone JSON must
include `target_repo_id`.

## Repo Move

### `jvs repo move <new-folder> [--json]`

Preview moving the whole JVS project folder to `<new-folder>`. This is not
`repo clone`: the command keeps the same `repo_id`, save point history,
workspace names, and registered workspace paths. The preview writes only a
repo move plan and must not move files.

Rules:

- `<new-folder>` must not already exist.
- The move uses same-filesystem no-overwrite atomic rename in the public v0
  contract.
- The preview and run must verify every registered workspace path record is
  reachable, writable, well-formed, and fresh for the current
  repo identity.
- The preview prints a reviewed run command. When the current directory is
  inside the source repo folder, the safe run command must use
  `jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>` from a safe
  parent folder.

Required preview JSON `data` fields include:

- `mode: "preview"`
- `operation: "repo_move"`
- `plan_id`
- `source_repo_root`
- `target_repo_root`
- `repo_id`
- `move_method`
- `folder_moved: false`
- `repo_id_changed: false`
- `save_point_history_changed: false`
- `main_workspace_updated: false`
- `external_workspaces` (registered workspace path records)
- `run_command`
- `safe_run_command`

### `jvs repo move --run <repo-move-plan-id> [--json]`

Run a reviewed repo move plan. Run must revalidate the source and destination
identities, registered workspace path records, locks, and current directory
safety before writing a durable lifecycle operation journal or moving the repo
folder.

Required run JSON `data` fields include:

- `mode: "run"`
- `operation: "repo_move"`
- `plan_id`
- `status: "moved"`
- `source_repo_root`
- `target_repo_root`
- `repo_root`
- `repo_id`
- `folder_moved: true`
- `repo_id_changed: false`
- `save_point_history_changed: false`
- `main_workspace_updated: true`
- `external_workspaces_updated` (registered workspace path records updated)

## Repo Rename

### `jvs repo rename <new-folder-name> [--json]`

Preview renaming the project folder within the same parent directory. Repo
rename is basename-only sugar over repo move: `<new-folder-name>` must be a
folder basename, not an absolute path, relative path, `.`, `..`, or a string
with a path separator. It preserves `repo_id`, save point history, workspace
names, and registered workspace paths.

Required preview JSON `data` fields include:

- `mode: "preview"`
- `operation: "repo_rename"`
- `plan_id`
- `source_repo_root`
- `target_repo_root`
- `repo_id`
- `move_method`
- `folder_moved: false`
- `repo_id_changed: false`
- `save_point_history_changed: false`
- `main_workspace_updated: false`
- `external_workspaces` (registered workspace path records)
- `run_command`
- `safe_run_command`

### `jvs repo rename --run <repo-rename-plan-id> [--json]`

Run a reviewed repo rename plan. Run follows the same safety and lifecycle
journal rules as `repo move`; safe retry from outside the old repo folder uses
`jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>`.

Required run JSON `data` fields include:

- `mode: "run"`
- `operation: "repo_rename"`
- `plan_id`
- `status: "moved"`
- `source_repo_root`
- `target_repo_root`
- `repo_root`
- `repo_id`
- `folder_moved: true`
- `repo_id_changed: false`
- `save_point_history_changed: false`
- `main_workspace_updated: true`
- `external_workspaces_updated` (registered workspace path records updated)

## Repo Detach

### `jvs repo detach [--json]`

Preview stopping JVS management of the current project folder while preserving
working files. This command is not destructive project deletion.

Preview rules:

- The current project must be an active JVS repo.
- Workspace `main` must be the project folder.
- All registered workspace path records must be reachable, writable,
  well-formed, and fresh for the current repo identity.
- The preview writes only the detach plan. It must not write a lifecycle
  journal, archive metadata, move files, or delete files.

Required preview JSON `data` fields include:

- `mode: "preview"`
- `plan_id`
- `operation_id`
- `repo_root`
- `repo_id`
- `archive_path`
- `external_workspaces` (registered workspace path records)
- `run_command`

### `jvs repo detach --run <repo-detach-plan-id> [--json]`

Run a reviewed detach plan. Before archiving active metadata, the command must
write a durable lifecycle operation journal whose `operation_id` is distinct
from and not derived from the plan ID.

Run rules:

- Archive active `.jvs` metadata under `.jvs-detached/<repo-id>-<operation-id>-<utc-timestamp>/`.
- Write a durable `DETACHING` marker in the archive directory before moving
  active metadata.
- Publish `DETACHED` metadata after registered workspace path records are
  marked detached/orphaned.
- Preserve project working files and save point storage.
- After success, ordinary discovery from the project folder must not report an
  active JVS repo.
- If active metadata was archived but the lifecycle did not finish, rerunning
  this command from the project folder must resume by scanning only the local
  `.jvs-detached` archive markers.

Required run JSON `data` fields include:

- `mode: "run"`
- `status: "detached"`
- `plan_id`
- `operation_id`
- `repo_root`
- `repo_id`
- `archive_path`
- `working_files_preserved`
- `active_repo_detached`
- `save_point_storage_removed`
- `external_workspaces_updated` (registered workspace path records updated)
- `recommended_next_command`

## Status

### `jvs status [--json]`

Show the active folder, workspace, current pointer, newest save point, file
source, restored paths, source save point when applicable, and whether the
folder has unsaved changes.

Required JSON `data` fields:

- `repo` for ordinary `.jvs/` workspaces only
- `control_root` for external control root workspaces only
- `folder`
- `workspace`
- `newest_save_point`
- `history_head`
- `content_source`
- `started_from_save_point` when applicable
- `unsaved_changes`
- `files_state`
- `path_sources` when applicable

Human output must prefer public status words such as `Folder`, `Workspace`,
`Current save point`, `Newest save point`, `Files match save point`, `Files
changed since save point`, `Files were last restored from`, `Started from save
point`, and `Unsaved changes`. Ordinary `.jvs/` status prints `Repo`; external
control root status prints `Control data` instead.

## Save And History

### `jvs save [-m message] [--json]`

Create a save point from the active workspace and add it to the project history
graph. A message is required, either as `-m/--message` or as the positional
message accepted by the implementation.

Rules:

- The save captures the workspace managed files, excluding JVS control data and
  runtime state. GA has no configurable file filtering.
- Save must hold the workspace mutation lock.
- Capacity and staging checks must fail before publishing a partial save point.
- If the workspace was created with `workspace new <folder> --from <save>`,
  the first save has no inherited history parent and records
  `started_from_save_point`.
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
- `transfers`

### `jvs history [--path <path>] [--limit <n>|-n <n>] [--grep <text>] [--json]`

Show project save points through the active workspace's pointer and provenance.
`--path` searches for save points that contain a workspace-relative path and
returns candidates without changing files. `--grep` filters by message
substring. `--limit` and `-n` limit displayed save points; `--limit 0` means no
limit. Messages and tags are not restore/view targets.

### `jvs history to <save> [--limit <n>|-n <n>] [--json]`

Show the history path ending at one concrete save point.

### `jvs history from [<save>] [--limit <n>|-n <n>] [--json]`

Show history starting from a save point. When `<save>` is omitted, start from
the active workspace's source/started-from save point; if there is no explicit
source, start from the earliest ancestor of the current workspace pointer.

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

Required JSON `data` fields include:

- `folder`
- `workspace`
- `save_point`
- `view_id`
- `view_path` (the read-only content path)
- `read_only`
- `no_workspace_or_history_changed`
- `transfers`

### `jvs view close <view-id|path>`

Close a read-only view, clear JVS-owned view state, and release the associated
active view cleanup protection.

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
- `transfers`
- `history_changed: false`
- `files_changed: false`
- `run_command`

For external control roots, `run_command` and any `next_commands` are full
commands that include `--control-root C --workspace main`.

### `jvs restore --run <restore-plan-id> [--json]`

Execute a previously created restore preview plan. Run must reload the plan and
revalidate the expected target state before writing files. Runtime options are
fixed by the preview plan; changing `--save-first`, `--discard-unsaved`, or
`--path` requires a new preview.

On success, a successful restore run leaves no active recovery and completed
restore plan residue is non-blocking. Only pending, stale, active, or malformed
restore/recovery state blocks mutation or clone publish; `jvs recovery status`,
`jvs doctor --strict`, and a blocked `jvs repo clone` must report a public
diagnostic entry instead of requiring callers to inspect control data files.
External `jvs recovery status --json` uses `restore_state` for pending or
stale restore preview diagnostics, including `pending_restore_preview`,
`stale_restore_preview`, and `blocking` semantics.

### `jvs restore discard <restore-plan-id>` [--json]

Discard a restore preview plan without changing managed files or workspace
history. This is the public cleanup entry for a stale restore preview, including
external control root previews whose workspace folder changed after preview.
Callers must not remove private control data files directly.

Required JSON `data` fields:

- `mode: "discard"`
- `plan_id`
- `folder`
- `workspace`
- `source_save_point`
- `path` for path restore plans
- `plan_discarded: true`
- `files_changed: false`
- `history_changed: false`

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
- `transfers`

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
- `transfers`

## Restore Recovery

### `jvs recovery status [recovery-plan] [--json]`

List active recovery plans or show one plan. A recovery plan records the
restore plan, workspace, folder, source save point, optional path, last error,
backup availability, and recommended next command.

Successful restore run leaves no active recovery. Completed restore plan
residue is non-blocking and is not an active recovery plan.

For external control roots, status JSON may include `data.restore_state`
(`restore_state`) when a
restore preview state is relevant and no active recovery detail is requested.
The object fields are:

- `state`
- `blocking`
- `plan_id`
- `recovery_plan_id`
- `message`
- `recommended_next_command`

Stable `restore_state.state` enum values and blocking semantics:

- `stable`: non-blocking; normally omitted.
- `pending_restore_preview`: blocking; the preview is still runnable and the
  recommended next command is `restore --run <restore-plan-id>`.
- `stale_restore_preview`: blocking; the workspace folder changed after
  preview and the recommended next command is
  `restore discard <restore-plan-id>`.
- `active_recovery`: blocking; surfaced through `plans[]` or recovery detail,
  with `recovery status`, `recovery resume`, or `recovery rollback` as the next
  public commands.
- `completed_restore_residue`: non-blocking; normally omitted.
- `malformed_blocking`: blocking; `recovery status` fails closed with public
  diagnostics and points to `doctor --strict --json`.

For external control roots, `recommended_next_command` must include
`--control-root C --workspace main`.

### `jvs recovery resume <recovery-plan> [--json]`

Resume an interrupted restore. On success, history remains unchanged, the
recovery backup is removed, and cleanup protection held by the recovery plan is
released.

### `jvs recovery rollback <recovery-plan> [--json]`

Return the workspace to the saved pre-restore state when that can be verified.
On success, history is restored to the pre-restore metadata state, recovery
backup state is resolved, and recovery cleanup protection is released.

## Workspace Creation

### `jvs workspace new <folder> --from <save> [--name <name>] [--json]`

Create another real workspace folder from a save point.

Rules:

- `<folder>` is the explicit target path for the new real folder.
- Relative folders are resolved from the command's current directory.
- The target folder must not already exist.
- The workspace name defaults to the target folder basename.
- `--name <name>` overrides the default workspace name.
- `--from` is required and must resolve to one save point ID.
- The new workspace starts with managed files copied from the source save
  point.
- The source workspace is not changed.
- The new workspace does not inherit the source history.
- `Newest save point` for the new workspace is `none` until its first save.
- The first save in the new workspace records `started_from_save_point`.
- Human output must print an absolute `Folder`.

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
- `transfers`

### `jvs workspace list [--status] [--json]`

Show known workspaces. Human output must show each workspace name, absolute
folder, current pointer, newest save point, and `Started from save point` when
known. With `--status`, it also checks whether each workspace has unsaved
changes.

### `jvs workspace path [name]`

Print the folder path for a workspace so users can jump with shell commands
such as `cd "$(jvs workspace path <name>)"`. JVS cannot change the caller's
shell directory after a command finishes.

### `jvs workspace rename <old> <new> [--dry-run] [--json]`

Rename a workspace without moving its folder. Workspace rename is a name-only
metadata operation: the workspace folder path, save point history, and managed
files stay unchanged. `main` is immutable; renaming the project folder uses
`jvs repo rename <new-folder-name>`.

Rules:

- `<old>` must be an existing non-main workspace name.
- `<new>` must be a valid unused workspace name.
- `--dry-run` checks and reports the planned metadata change without writing a
  lifecycle operation journal or changing workspace metadata.
- A normal rename writes a durable lifecycle operation journal before changing
  registry or registered workspace path metadata.
- If a registered workspace path record is present, it must be reachable,
  writable, well-formed, and fresh for the old workspace name before the
  command updates it.

Required JSON `data` fields include:

- `mode`
- `status`
- `operation`
- `operation_id` when metadata was changed
- `old_workspace`
- `workspace`
- `folder`
- `folder_moved: false`
- `workspace_connection_updated` (registered workspace path record updated)
- `save_point_history_changed: false`

### `jvs workspace move <name> <new-folder>`

Preview moving a workspace folder without changing its workspace name. This is
a preview-first reviewed-plan flow. The preview must change no files and must
show the old folder, new folder, selected workspace, and the reviewed run
command.

Rules:

- `<name>` must be an existing non-main workspace.
- `<new-folder>` must not already exist.
- The move uses same-filesystem no-overwrite atomic rename in the public v0
  contract.
- The workspace name and save point history are unchanged.
- Run must fail closed before mutation if the current directory is inside the
  source workspace folder, and must print a safe command to rerun the same plan
  from outside the affected folder.

Required preview JSON `data` fields include:

- `mode: "preview"`
- `plan_id`
- `workspace`
- `source_folder`
- `target_folder`
- `newest_save_point`
- `content_source`
- `expected_newest_save_point`
- `expected_content_source`
- `expected_folder_evidence`
- `unsaved_changes`
- `files_state`
- `folder_moved: false`
- `files_changed: false`
- `workspace_name_changed: false`
- `save_point_history_changed: false`
- `move_method`
- `run_command`

### `jvs workspace move --run <workspace-move-plan-id> [--json]`

Run a reviewed workspace move plan. Run must reload and revalidate the plan,
write a durable lifecycle operation journal, move the workspace folder, update
the workspace registry path, and consume the plan only after verification.

Required run JSON `data` fields include:

- `mode: "run"`
- `plan_id`
- `status: "moved"`
- `workspace`
- `source_folder`
- `target_folder`
- `folder`
- `folder_moved: true`
- `files_changed: true`
- `workspace_name_changed: false`
- `save_point_history_changed: false`

### `jvs workspace delete <name>`

Workspace deletion is a preview-first reviewed-plan flow in the public spec.
Preview must change no files, protect unsaved work, and show the selected
workspace folder and registry change. Run must bind to a reviewed plan and
delete only the selected workspace folder and workspace registry entry. Save
point storage is unchanged; deleting unprotected save point storage is a
separate reviewed cleanup flow.

Required preview JSON `data` fields:

- `mode: "preview"`
- `plan_id`
- `workspace`
- `folder`
- `newest_save_point`
- `content_source`
- `expected_newest_save_point`
- `expected_content_source`
- `expected_folder_evidence`
- `unsaved_changes`
- `files_state`
- `options`
- `folder_removed: false`
- `files_changed: false`
- `workspace_metadata_removed: false`
- `save_point_storage_removed: false`
- `run_command`
- `cleanup_preview_run`

### `jvs workspace delete --run <workspace-delete-plan-id> [--json]`

Run a reviewed workspace delete plan. Run must reload and revalidate the plan,
fail closed if the current directory is inside the target workspace folder,
write a durable lifecycle operation journal, delete only the selected workspace
folder and workspace registry entry, and leave save point storage to the
separate cleanup flow.

Required run JSON `data` fields:

- `mode: "run"`
- `plan_id`
- `status: "deleted"`
- `workspace`
- `folder`
- `newest_save_point`
- `content_source`
- `unsaved_changes`
- `files_state`
- `folder_removed: true`
- `files_changed: true`
- `workspace_metadata_removed: true`
- `save_point_storage_removed: false`
- `cleanup_command`
- `cleanup_preview_run`

## Doctor

### `jvs doctor [--strict] [--repair-runtime] [--repair-list] [--json]`

Check repository health. `--strict` includes full save point integrity
verification.

For ordinary `.jvs/` projects, `--repair-runtime` is limited to safe runtime
cleanup and destination-local registered workspace path rebinding after
filesystem migration. If a physically copied ordinary project still has a
registered workspace path that points at the source folder, strict doctor
reports an unhealthy workspace path binding until `--repair-runtime` can prove
and store the destination binding. A skipped or failed rebind therefore leaves
`doctor --strict --repair-runtime` unhealthy.

For external control roots, doctor is strict inspection only:
`jvs --json --control-root C --workspace main doctor --strict`.
`--repair-runtime` is rejected for that explicit selector and must not be used
to repair external control root workspace binding.

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
views, active recovery plans, active operations, and imported clone history.
Cleanup only deletes unprotected save point storage. It must not delete
workspace folders, user cache directories, JVS control data, or runtime state;
it must not prune workspace history or apply a retention policy.
Stable cleanup reasons: workspace history; open views; active recovery plans;
active operations; imported clone history.

Cleanup preview must explain protected save points by stable generic reasons:

- `history`
- `open_view`
- `active_recovery`
- `active_operation`
- `imported_clone_history`

JSON uses those stable reason tokens. Human output must render them as natural
labels: workspace history, open views, active recovery plans, and active
operations, and imported clone history.

### `jvs cleanup preview [--json]`

Create a cleanup plan for save point storage that is no longer needed by
workspace history, open views, active recovery plans, or active operations.
Imported clone history is also protected when durable repo clone metadata is
present.
Preview does not delete anything.

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
