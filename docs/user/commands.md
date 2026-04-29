# Command Reference

This page follows the public help surface shown by `jvs --help`.

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

Show the active folder, workspace, newest save point, file source, and unsaved
changes.

```bash
jvs status
jvs status --json
```

Human output uses phrases such as `Files match save point`, `Files changed
since save point`, and `Unsaved changes`.

## `jvs save -m "message"`

Create a save point from managed files in the active workspace.

```bash
jvs save -m "baseline"
jvs save --message "before migration"
```

A message is required. The command prints the new save point ID.

## `jvs history`

List save points for the active workspace.

```bash
jvs history
jvs history --limit 10
jvs history --grep "baseline"
jvs history --path src/config.yaml
jvs history --all
```

Flags:

| Flag | Use |
| --- | --- |
| `--limit`, `-n` | Limit displayed save points; `0` means all |
| `--grep`, `-g` | Filter by message substring |
| `--path` | Find save points containing a workspace-relative path |
| `--all` | Show save points across workspaces |

## `jvs view <save-point> [path]`

Open a read-only view of a save point, or of one path inside it.

```bash
jvs view <save>
jvs view <save> src/config.yaml
jvs view close <view-id>
```

The save point must be a full ID or a unique ID prefix. View does not change
workspace files or history.

## `jvs restore [save-point] [--path path]`

Create or run a restore plan.

```bash
jvs restore <save>
jvs restore <save> --path src/config.yaml
jvs restore --path src/config.yaml
jvs restore --run <plan-id>
```

Modes:

| Command | Use |
| --- | --- |
| `jvs restore <save>` | Preview whole-folder restore |
| `jvs restore <save> --path <path>` | Preview one-path restore |
| `jvs restore --path <path>` | List candidate save points for a path |
| `jvs restore --run <plan-id>` | Execute a preview plan |

Safety flags:

| Flag | Use |
| --- | --- |
| `--save-first` | Save unsaved changes before restore |
| `--discard-unsaved` | Discard unsaved changes for this operation |

`--save-first` and `--discard-unsaved` cannot be used together.

## `jvs workspace`

Manage workspace folders.

```bash
jvs workspace list
jvs workspace path [name]
jvs workspace rename <old> <new>
jvs workspace new experiment --from <save>
jvs workspace remove experiment
jvs workspace remove --run <plan-id>
```

Common commands:

| Command | Use |
| --- | --- |
| `jvs workspace list` | Show known workspaces |
| `jvs workspace path [name]` | Print the folder path for a workspace |
| `jvs workspace rename <old> <new>` | Rename a workspace |
| `jvs workspace new <name> --from <save>` | Create another workspace folder from a save point |
| `jvs workspace remove <name>` | Preview removal of a workspace folder |
| `jvs workspace remove --run <plan-id>` | Run a reviewed remove plan |

`workspace new` prints:

- `Folder`: where the new real folder is.
- `Workspace`: the new workspace name.
- `Started from save point`: the source save point.

The original workspace is unchanged.

`workspace remove` is preview-first. The preview does not delete the folder.
Review the folder path, workspace name, unsaved-change status, and printed
`Run:` command before running the plan.

`workspace remove --run <plan-id>` removes the selected workspace folder and
workspace entry. It does not remove save point storage. Use
`jvs cleanup preview`, then `jvs cleanup run --plan-id <plan-id>` for reviewed
cleanup of unprotected save point storage.

If the workspace has unsaved changes, add `--force` to the preview command
only when those local changes are intentionally disposable:

```bash
jvs workspace remove experiment --force
```

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
jvs cleanup run --plan-id <plan-id>
```

Cleanup is two-step. `preview` shows the plan and does not delete anything.
After you review the plan, `run` rechecks that exact plan before deleting
unneeded save point storage. Cleanup does not delete workspace folders, user
cache directories, JVS control data, runtime state, or history.

| Command | Use |
| --- | --- |
| `jvs cleanup preview` | Show what can be cleaned and print a plan ID |
| `jvs cleanup run --plan-id <plan-id>` | Run a reviewed cleanup plan |

## `jvs doctor`

Check repository health.

```bash
jvs doctor
jvs doctor --strict
jvs doctor --repair-list
jvs doctor --repair-runtime
```

`--strict` performs deeper integrity checks. `--repair-runtime` runs safe
automatic repairs for runtime leftovers.

## Shell Completion

```bash
jvs completion bash
jvs completion zsh
jvs completion fish
jvs completion powershell
```
