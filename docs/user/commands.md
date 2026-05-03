# Command Reference

This page follows the public help surface shown by `jvs --help`. If this is
your first time using JVS, start with the [Quickstart](quickstart.md), then come
back here for exact command forms.

For a plain-language map of which commands change files and which commands only
preview or inspect, see the [User Guide](README.md#what-changes-files).

## Global Flags

| Flag | Use |
| --- | --- |
| `--json` | Emit one JSON object for scripts |
| `--workspace <name>` | Target a named workspace |
| `--repo <path>` | Advanced target assertion for the JVS project path |
| `--no-progress` | Hide progress bars |
| `--no-color` | Disable colored output |
| `--debug` | Enable debug logging |

Most users can stay inside the folder they want to operate on and omit
`--repo` and `--workspace`.

## `jvs init [folder]`

Adopt a folder for JVS. With no argument, adopts the directory you are in.

```bash
jvs init
jvs init /path/to/folder
```

Behavior:

- Existing files stay in place.
- `.jvs/` control data is created in the folder.
- The folder is registered as workspace `main`.
- The first save point is created later with `jvs save`.

## `jvs status`

Show the active folder, workspace, current pointer, newest save point, file
source, started-from save point when known, and unsaved changes.

```bash
jvs status
jvs status --json
```

Human output uses phrases such as `Files match save point`, `Files changed
since save point`, and `Unsaved changes`.

## `jvs save -m "message"`

Create a save point from the active workspace.

```bash
jvs save -m "baseline"
jvs save --message "before migration"
```

A message is required. The command prints the new full save point ID.

## `jvs history`

List save points for the active workspace.

```bash
jvs history
jvs history to <save>
jvs history from [<save>]
jvs history --limit 10
jvs history -n 10
jvs history --limit 0
jvs history --grep "baseline"
jvs history --path src/config.yaml
```

Flags:

| Flag | Use |
| --- | --- |
| `--limit`, `-n` | Limit displayed save points; `0` means no limit |
| `--grep`, `-g` | Search by message substring |
| `--path` | Find save points containing a workspace-relative path |

Direction commands:

| Command | Use |
| --- | --- |
| `jvs history to <save>` | Show the path of history ending at a save point |
| `jvs history from [<save>]` | Show history starting from a save point; omit `<save>` to start from the active workspace's current position |

Human history output shows a copyable ID or short ID for each save point. That
short form is usually enough in commands that ask for `<save>`. If JVS says it
is ambiguous or non-unique, use more characters from the same ID. If you need
the full value, run `jvs history --json` and copy the `save_point_id` field.

## `jvs view <save-point> [path]`

Open a read-only view of a save point, or of one path inside it.

```bash
jvs view <save>
jvs view <save> src/config.yaml
jvs view close <view-id>
```

The save point must be a full ID or an unambiguous ID prefix. View does not
change workspace files or history.

## `jvs restore [save-point] [--path path]`

Create or run a restore plan. `jvs restore <save>` is preview-only; files change
only when you run `jvs restore --run <restore-plan-id>`.

```bash
jvs restore <save>
jvs restore <save> --path src/config.yaml
jvs restore --path src/config.yaml
jvs restore --run <restore-plan-id>
```

Modes:

| Command | Use |
| --- | --- |
| `jvs restore <save>` | Preview whole-folder restore |
| `jvs restore <save> --path <path>` | Preview one-path restore |
| `jvs restore --path <path>` | List candidate save points for a path |
| `jvs restore --run <restore-plan-id>` | Execute a reviewed restore plan |

Safety flags:

| Flag | Use |
| --- | --- |
| `--save-first` | Save unsaved changes before restore |
| `--discard-unsaved` | Discard unsaved changes for this operation |

`--save-first` and `--discard-unsaved` cannot be used together.

## `jvs workspace`

Manage workspace folders. For the natural meaning of workspace and folder, see
[Concepts](concepts.md#workspace).

```bash
jvs workspace list
jvs workspace list --status
jvs workspace path [name]
jvs workspace rename <old> <new>
jvs workspace move <name> <new-folder>
jvs workspace move --run <workspace-move-plan-id>
jvs workspace new ../experiment --from <save>
jvs workspace new ../experiment --from <save> --name test-copy
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

Common commands:

| Command | Use |
| --- | --- |
| `jvs workspace list` | Show known workspaces, their folders, current pointer, newest save point, and source save point when known |
| `jvs workspace list --status` | Also check whether listed workspaces have unsaved changes |
| `jvs workspace path [name]` | Print the folder path for a workspace |
| `jvs workspace rename <old> <new>` | Rename a workspace without moving its folder |
| `jvs workspace move <name> <new-folder>` | Preview moving a workspace folder without changing its workspace name |
| `jvs workspace move --run <workspace-move-plan-id>` | Run a reviewed workspace move plan |
| `jvs workspace new <folder> --from <save>` | Create another workspace folder at a path you choose |
| `jvs workspace delete <name>` | Preview deletion of a workspace folder |
| `jvs workspace delete --run <workspace-delete-plan-id>` | Run a reviewed delete plan |

For `workspace new`, `<folder>` is the target folder path. The folder must not
already exist. The workspace name defaults to the folder name; use
`--name <name>` only when you want a different workspace name.

`workspace new` prints:

- `Folder`: the absolute path to the new real folder.
- `Workspace`: the new workspace name.
- `Started from save point`: the source save point.

The original workspace is unchanged.

To move into a workspace folder, ask JVS for the path and let your shell change
directories:

```bash
cd "$(jvs workspace path experiment)"
```

`workspace delete` is preview-first. The preview does not delete the folder.
Review the folder path, workspace name, unsaved-change status, and printed
`Run:` command before running the plan.

Use the delete plan ID from the workspace delete preview you just reviewed. Do
not reuse a restore or cleanup plan ID for workspace deletion.

`workspace move` is preview-first. The preview does not move files. Run the
printed `jvs workspace move --run <workspace-move-plan-id>` command from
outside the source workspace folder.

`workspace delete --run <workspace-delete-plan-id>` deletes the selected workspace folder
and workspace entry. It does not remove save point storage. Use
`jvs cleanup preview`, then `jvs cleanup run --plan-id <cleanup-plan-id>` for
reviewed cleanup of save point storage.

If the workspace has unsaved changes, `workspace delete` fails closed. Save or
restore those changes before deleting the workspace.

## `jvs repo`

Manage JVS project folders.

```bash
jvs repo clone <target-folder>
jvs repo move <new-folder>
jvs repo move --run <repo-move-plan-id>
jvs repo rename <new-folder-name>
jvs repo rename --run <repo-rename-plan-id>
jvs repo detach
jvs repo detach --run <repo-detach-plan-id>
```

Common commands:

| Command | Use |
| --- | --- |
| `jvs repo clone <target-folder>` | Copy the current local JVS project into a new folder with a new repo identity |
| `jvs repo move <new-folder>` | Preview moving the current project folder while keeping `repo_id` and save point history |
| `jvs repo move --run <repo-move-plan-id>` | Run a reviewed repo move plan |
| `jvs repo rename <new-folder-name>` | Preview renaming the project folder within the same parent directory |
| `jvs repo rename --run <repo-rename-plan-id>` | Run a reviewed repo rename plan |
| `jvs repo detach` | Preview stopping JVS management of the current project folder while keeping working files |
| `jvs repo detach --run <repo-detach-plan-id>` | Run a reviewed detach plan |

### `jvs repo clone <target-folder> [--save-points all|main] [--dry-run]`

Copy the current local JVS project into a new folder. The source is the project
you are in, or the project named by global `--repo <path>`.

These examples assume you are running the command from the project folder. If
you are inside a `main` subfolder, choose a target outside the project folder,
such as `../../project-copy`.

```bash
jvs repo clone ../project-copy
jvs repo clone ../project-copy-preview --save-points main --dry-run
```

Use the first form when you want a full local copy with saved history. Use the
second form to preview a smaller copy before JVS creates any files.

Behavior:

- `<target-folder>` must be a new folder path that does not already exist.
- `<target-folder>` must be outside the source project and every source
  workspace. Do not choose a folder inside the project you are copying.
- By default, JVS copies all save points, the same as `--save-points all`.
- Even in the default mode, the target creates only one workspace, named
  `main`, at `<target-folder>`.
- Other workspaces from the source project are not created in the target.
- `--save-points main` copies only the saved history needed by source `main`,
  including earlier save points that history depends on.
- `--dry-run` checks what would happen and prints the plan, but does not create
  the target folder or any files.

The source project is unchanged.

### `jvs repo move`

Move the current JVS project folder. This is preview-first: the preview writes
a plan and does not move files. Run the printed command after review.

```bash
jvs repo move ../project-on-ssd
jvs repo move --run <repo-move-plan-id>
jvs --repo <old-repo-root> repo move --run <repo-move-plan-id>
```

`repo move` keeps the same `repo_id`, save point history, workspace names, and
external workspace folders. It updates the main workspace path and registered
external workspace connections to the new project folder. If your shell is
inside the old project folder, use the printed safe run command from a parent
folder.

### `jvs repo rename`

Rename the current JVS project folder inside its current parent directory. This
is basename-only sugar over `repo move`: pass a folder basename such as
`project-review`, not a path.

```bash
jvs repo rename project-review
jvs repo rename --run <repo-rename-plan-id>
jvs --repo <old-repo-root> repo rename --run <repo-rename-plan-id>
```

`repo rename` is preview-first and keeps the same `repo_id`, save point
history, workspace names, and external workspace folders.

### `jvs repo detach`

Stop JVS managing the current project folder while keeping the project working
files in place. This is preview-first.

`jvs repo detach` checks the active project identity, verifies that `main` is
the project folder, and verifies registered external workspace connections
before it prints a plan. The preview does not archive metadata and does not
move or delete files.

Run the printed command after review:

```bash
jvs repo detach
jvs repo detach --run <repo-detach-plan-id>
```

The run archives JVS metadata under `.jvs-detached`, marks registered external
workspace connections as detached/orphaned, and leaves save point storage in
the archive. After success, ordinary `jvs status` from the project folder no
longer treats it as an active JVS repo.

## `jvs recovery`

Recover an interrupted restore.

```bash
jvs recovery status
jvs recovery status <recovery-plan>
jvs recovery resume <recovery-plan>
jvs recovery rollback <recovery-plan>
```

Use `resume` to continue the restore, or `rollback` to restore the protected
pre-restore folder state.

## `jvs cleanup`

Free save point storage that JVS no longer needs.

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

Cleanup is two-step. `preview` shows the plan and does not delete anything.
After you review the plan, `run` rechecks that exact plan before deleting
unneeded save point storage. Cleanup does not delete workspace folders, your
files, JVS control data, active recovery information, or history.

| Command | Use |
| --- | --- |
| `jvs cleanup preview` | Show what can be cleaned and print a plan ID |
| `jvs cleanup run --plan-id <cleanup-plan-id>` | Run a reviewed cleanup plan |

## `jvs doctor`

Check repository health.

```bash
jvs doctor
jvs doctor --strict
jvs doctor --repair-list
jvs doctor --repair-runtime
```

`--strict` performs deeper integrity checks. `--repair-runtime` runs safe
automatic repairs for leftover state from interrupted JVS operations.

## Shell Completion

```bash
jvs completion bash
jvs completion zsh
jvs completion fish
jvs completion powershell
```
