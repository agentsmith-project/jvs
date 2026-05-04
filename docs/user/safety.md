# Safety

JVS is safest when you treat it as a review-first tool:

```text
save -> inspect -> preview -> run
```

The important habit is simple: read the preview, check the folder and path, and
run only the command JVS prints after `Run:`.

## Quick Safety Checklist

Before a command that can change files, folders, or JVS control data:

- Run `jvs status` and check that the `Folder` is the folder you meant to use.
- For restore or workspace creation, run `jvs history` or
  `jvs history --path <path>` to find the save point you want.
- Use `jvs view <save> [path]` to inspect saved content before restoring files.
- For folder operations, read the source folder, target folder, workspace name,
  and project name before running anything.
- For preview/run commands, run only the command JVS prints after `Run:`.
- For `workspace new`, `workspace rename`, and `repo clone` without `--dry-run`,
  check the target folder or name before pressing Enter.
- Use `--save-first` when current local changes matter.
- Use `--discard-unsaved` only when current local changes are intentionally
  disposable.

Seeing `Preview`, `Plan`, `No files were changed`, or a printed `Run:` command
means JVS has not made the destructive change yet.

## Commands That Inspect

These commands do not change workspace files or move folders. `jvs view` opens a
read-only view path for inspection.

```bash
jvs status
jvs history
jvs history --path src/config.yaml
jvs view <save> src/config.yaml
jvs workspace list
jvs workspace path [name]
jvs doctor
jvs doctor --strict
```

## Commands That Preview Or Check Only

These commands let you review the impact before the high-impact action happens:

```bash
jvs restore <save>
jvs restore <save> --path src/config.yaml
jvs workspace move experiment ../experiment-archive
jvs workspace delete experiment
jvs repo clone ../project-copy --dry-run
jvs repo move ../project-on-ssd
jvs repo rename project-review
jvs repo detach
jvs cleanup preview
```

What you should see:

- `jvs history` prints save point messages with copyable IDs or short IDs.
- `jvs view` prints a read-only view path.
- `jvs restore <save>` prints a plan and a `Run:` command.
- `jvs workspace move <name> <new-folder>` says preview only and prints a move
  plan.
- `jvs workspace delete <name>` says preview only and prints a delete plan.
- `jvs repo move`, `jvs repo rename`, and `jvs repo detach` say preview only and
  print a matching `Run:` command.
- `jvs repo clone <target-folder> --dry-run` checks the clone and does not create
  the target project folder.
- `jvs cleanup preview` prints what save point storage can be cleaned.

If the output says `No files were changed`, that is good. It means you are
still reviewing.

## Commands That Change Files, Folders, Or Control Data

These commands are the important moments. Some run a reviewed plan; others create
or rename something immediately after checking the arguments.

```bash
jvs init [folder]
jvs save -m "message"
jvs workspace new <folder> --from <save>
jvs workspace rename <old> <new>
jvs repo clone <target-folder>
jvs restore --run <restore-plan-id>
jvs workspace move --run <workspace-move-plan-id>
jvs workspace delete --run <workspace-delete-plan-id>
jvs repo move --run <repo-move-plan-id>
jvs repo rename --run <repo-rename-plan-id>
jvs repo detach --run <repo-detach-plan-id>
jvs recovery resume <recovery-plan>
jvs recovery rollback <recovery-plan>
jvs cleanup run --plan-id <cleanup-plan-id>
```

What they change:

| Command | What it can change | What it does not change |
| --- | --- | --- |
| `jvs init [folder]` | Adds JVS control data to the folder | Existing user files |
| `jvs save -m "message"` | Creates a save point from the current workspace | Workspace files |
| `jvs workspace new <folder> --from <save>` | Creates another real workspace folder | The original workspace folder |
| `jvs workspace rename <old> <new>` | Changes the JVS workspace name | The real folder path |
| `jvs repo clone <target-folder>` | Creates a new local JVS project folder | The source project |
| `jvs restore --run <restore-plan-id>` | Workspace files in the restore plan | Save point history |
| `jvs workspace move --run <workspace-move-plan-id>` | The selected workspace folder path and workspace entry | The workspace name |
| `jvs workspace delete --run <workspace-delete-plan-id>` | The selected workspace folder and workspace entry | Save point storage |
| `jvs repo move --run <repo-move-plan-id>` | The project folder path | Repo identity and save point history |
| `jvs repo rename --run <repo-rename-plan-id>` | The project folder name | Repo identity and save point history |
| `jvs repo detach --run <repo-detach-plan-id>` | Archives JVS control data and stops active JVS management | Project working files |
| `jvs recovery resume <recovery-plan>` | Continues an interrupted restore | Save point history |
| `jvs recovery rollback <recovery-plan>` | Restores the protected pre-restore folder state when possible | Save point history |
| `jvs cleanup run --plan-id <cleanup-plan-id>` | Save point storage listed by the cleanup plan | Workspace folders |

Run commands are tied to the preview plan for the same operation. If the folder
changed after preview, JVS should stop and ask you to make a fresh preview.

## Save Points And History

`jvs save -m "message"` creates a new save point from the current workspace.
It does not rewrite older save points.

`jvs save` prints the full save point ID. `jvs history` usually shows enough
of the ID to copy into commands. If JVS says the ID is ambiguous or
non-unique, use a longer or full ID; `jvs history --json` includes the full
`save_point_id` value.

`jvs restore --run <restore-plan-id>` copies files from a save point into your
workspace. Restore does not change history. After restoring, make a new save if
you want the recovered state to become the newest save point:

```bash
jvs save -m "recovered config"
```

## History And View Are Read-Only

`jvs history` and `jvs history --path <path>` only search.

`jvs view <save> [path]` opens saved content read-only. It does not change:

- workspace files
- history
- the newest save point

Close views when finished:

```bash
jvs view close <view-id>
```

## Restore With Local Changes

When the target folder has local changes, restore does not overwrite them by
default. Instead, JVS gives a decision preview: it tells you that local changes
exist and asks you to choose how to proceed.

Choose one:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` when the current local changes matter. JVS saves them first,
then prepares the restore.

Use `--discard-unsaved` only when you are sure the current local changes can be
thrown away for this restore.

## Path Restore For Small Repairs

For one file or directory, start with path discovery:

```bash
jvs history --path src/config.yaml
jvs view <save> src/config.yaml
jvs restore <save> --path src/config.yaml
jvs restore --run <restore-plan-id>
```

Path restore changes only the selected path. If you restore
`src/config.yaml`, unrelated files such as `src/notes.md` are not restored or
deleted by that run.

Seeing the requested path in the preview is the key safety check.

## Workspace Delete Is Not Cleanup

Use workspace deletion when you want to delete a separate workspace folder:

```bash
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

The first command is only a preview. It should show the folder, workspace name,
local-change status, and `Run:` command.

The run deletes the selected workspace folder and workspace entry. It does not
remove save point storage. To review save point storage cleanup later, use:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

The `main` workspace cannot be deleted. If you want to stop JVS managing the
whole project while keeping project files in place, use repo detach:

```bash
jvs repo detach
jvs repo detach --run <repo-detach-plan-id>
```

Read the detach preview carefully. The run archives JVS control data and leaves
working files in place.

If the workspace has local changes, save or restore them before deleting the
workspace.

## Repo Clone, Move, Rename, And Detach

`jvs repo clone <target-folder> --dry-run` is a check. It does not create the
target folder or a new repo identity.

`jvs repo clone <target-folder>` creates a new local JVS project folder with a
new repo identity. The source project is unchanged.

If clone fails, the target is not an active JVS repo. When JVS cannot safely
remove a target folder or target control data root after rollback, the target
folder or target control data root may remain at the target path or be moved to
a hidden quarantine; in either case, inspect/remove manually. If JVS moves a
path to quarantine, it prints
`target folder was quarantined at ...; inspect and remove it manually` or
`target control root was quarantined at ...; inspect and remove it manually`.

Repo move, rename, and detach are preview-first:

```bash
jvs repo move <new-folder>
jvs repo move --run <repo-move-plan-id>
jvs repo rename <new-folder-name>
jvs repo rename --run <repo-rename-plan-id>
jvs repo detach
jvs repo detach --run <repo-detach-plan-id>
```

Move and rename keep save point history. Detach keeps project working files in
place and archives JVS control data.

## Cleanup Is Not Workspace Deletion

Cleanup is for save point storage that JVS no longer needs:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

Cleanup does not delete workspace folders. If `jvs workspace list` shows a
workspace you no longer want, use `jvs workspace delete <name>` and review its
plan.

## Health Checks And Backups

Before high-impact work, run:

```bash
jvs doctor
jvs doctor --strict
```

Save points help with local folder recovery, but they are not a replacement for
backups. Use your normal storage backup process for machine loss, disk loss, or
account loss.
