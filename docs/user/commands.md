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
jvs workspace new ../experiment --from <save>
jvs workspace new ../experiment --from <save> --name test-copy
jvs workspace remove experiment
jvs workspace remove --run <remove-plan-id>
```

Common commands:

| Command | Use |
| --- | --- |
| `jvs workspace list` | Show known workspaces, their folders, current pointer, newest save point, and source save point when known |
| `jvs workspace list --status` | Also check whether listed workspaces have unsaved changes |
| `jvs workspace path [name]` | Print the folder path for a workspace |
| `jvs workspace rename <old> <new>` | Rename a workspace |
| `jvs workspace new <folder> --from <save>` | Create another workspace folder at a path you choose |
| `jvs workspace remove <name>` | Preview removal of a workspace folder |
| `jvs workspace remove --run <remove-plan-id>` | Run a reviewed remove plan |

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

`workspace remove` is preview-first. The preview does not delete the folder.
Review the folder path, workspace name, unsaved-change status, and printed
`Run:` command before running the plan.

Use the remove plan ID from the workspace remove preview you just reviewed. Do
not reuse a restore or cleanup plan ID for workspace removal.

`workspace remove --run <remove-plan-id>` removes the selected workspace folder
and workspace entry. It does not remove save point storage. Use
`jvs cleanup preview`, then `jvs cleanup run --plan-id <cleanup-plan-id>` for
reviewed cleanup of save point storage.

If the workspace has unsaved changes, add `--force` to the preview command
only when those local changes are intentionally disposable:

```bash
jvs workspace remove experiment --force
```

## `jvs repo clone <target-folder> [--save-points all|main] [--dry-run]`

Copy the current local JVS project into a new folder. The source is the project
you are in, or the project named by global `--repo <path>`.

```bash
jvs repo clone ../project-copy
jvs repo clone ../project-copy-preview --save-points main --dry-run
```

Use the first form when you want a full local copy with saved history. Use the
second form to preview a smaller copy before JVS creates any files.

Behavior:

- `<target-folder>` must be a new folder path that does not already exist.
- `<target-folder>` must be outside every source workspace. Do not choose a
  folder inside any workspace of the project you are copying.
- By default, JVS copies all save points, the same as `--save-points all`.
- Even in the default mode, the target creates only one workspace, named
  `main`, at `<target-folder>`.
- Other workspaces from the source project are not created in the target.
- `--save-points main` copies only the saved history needed by source `main`,
  including earlier save points that history depends on.
- `--dry-run` checks what would happen and prints the plan, but does not create
  the target folder or any files.

The source project is unchanged.

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
