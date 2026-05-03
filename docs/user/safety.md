# Safety

JVS is safest when you treat it as a review-first tool:

```text
save -> inspect -> preview -> run
```

The important habit is simple: read the preview, check the folder and path, and
run only the command JVS prints after `Run:`.

## Quick Safety Checklist

Before a restore, workspace deletion, or cleanup:

- Run `jvs status` and check that the `Folder` is the folder you meant to use.
- Run `jvs history` or `jvs history --path <path>` to find the save point you
  want.
- Use `jvs view <save> [path]` to inspect saved content before changing files.
- Prefer path restore when you only need one file or directory.
- Use `--save-first` when current local changes matter.
- Use `--discard-unsaved` only when current local changes are intentionally
  disposable.

Seeing `Preview`, `Plan`, `No files were changed`, or a printed `Run:` command
means JVS has not made the destructive change yet.

## Commands That Only Read Or Preview

These commands do not change your workspace files:

```bash
jvs status
jvs history
jvs history --path src/config.yaml
jvs view <save> src/config.yaml
jvs restore <save>
jvs restore <save> --path src/config.yaml
jvs workspace delete experiment
jvs cleanup preview
```

What you should see:

- `jvs history` prints save point messages with copyable IDs or short IDs.
- `jvs view` prints a read-only view path.
- `jvs restore <save>` prints a plan and a `Run:` command.
- `jvs workspace delete <name>` says preview only and prints a delete plan.
- `jvs cleanup preview` prints what save point storage can be cleaned.

If the output says `No files were changed`, that is good. It means you are
still reviewing.

## Commands That Change Files Or Remove Folders

These commands perform the reviewed action:

```bash
jvs restore --run <restore-plan-id>
jvs workspace delete --run <workspace-delete-plan-id>
jvs cleanup run --plan-id <cleanup-plan-id>
```

What they change:

| Command | What it can change | What it does not change |
| --- | --- | --- |
| `jvs restore --run <restore-plan-id>` | Workspace files in the restore plan | Save point history |
| `jvs workspace delete --run <workspace-delete-plan-id>` | The selected workspace folder and workspace entry | Save point storage |
| `jvs cleanup run --plan-id <cleanup-plan-id>` | Save point storage listed by the cleanup plan | Workspace folders |

Run commands are tied to the preview plan. If the folder changed after preview,
JVS should stop and ask you to make a fresh preview.

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

The `main` workspace cannot be deleted. If you want to stop using JVS for a
folder, keep a normal backup first and ask for project-specific guidance.

If the workspace has local changes, save or restore them before deleting the
workspace.

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
