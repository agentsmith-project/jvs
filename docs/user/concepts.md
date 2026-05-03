# Concepts

JVS is built around a small set of everyday ideas:

```text
folder -> workspace -> save point -> history -> view -> restore -> cleanup
```

## Folder

A folder is the real directory where your files live. It might be a project, a
writing folder, a design folder, or any other directory you already use.

When you run:

```bash
jvs init
```

JVS adopts the folder you are in. With `jvs init /path/to/folder`, it adopts the
folder you name. In both cases, your existing files stay in place.

JVS adds `.jvs/` control data inside the folder. Treat that as JVS's own notes.
You normally do not open it or edit it yourself.

## Workspace

A workspace is a folder JVS knows by name. The first workspace is named `main`.
Most command output shows both the real folder path and the workspace name:

```text
Folder: /path/to/myproject
Workspace: main
```

Use the folder path when you open files in an editor. Use the workspace name
when a JVS command asks which workspace you mean.

You can create another workspace from a save point:

```bash
jvs workspace new ../experiment --from <save>
```

That creates another real folder at `../experiment`. The workspace name
defaults to the folder name, so this one is named `experiment`. The original
workspace is unchanged.

## Save Point

A save point is a saved state of a workspace. It includes the files JVS saves,
your message, and the time it was created.

Create one with:

```bash
jvs save -m "baseline before changes"
```

JVS prints the full save point ID. You can use that ID, or a shorter beginning
of it that JVS accepts, in commands such as:

```bash
jvs view <save>
jvs restore <save>
jvs workspace new ../experiment --from <save>
```

Save points are not edited in place. If you restore old files and then save
again, JVS creates a new save point.

`jvs history` also shows a copyable ID or short ID for each save point. That is
usually enough. If JVS says the ID is ambiguous or non-unique, use a longer
piece of the same ID. If you need the full value, run `jvs history --json` and
copy the `save_point_id` field for the save point you chose.

## History

History is the list of save points for a workspace:

```bash
jvs history
```

Use history when you need to choose a save point by message or time. You can
usually copy the ID shown there into commands that ask for `<save>`. If you
only remember a file or folder you want back, ask for candidates:

```bash
jvs history --path notes.md
jvs restore --path notes.md
```

When you want a narrower view of history, use:

```bash
jvs history to <save>
jvs history from [<save>]
jvs history --limit 20
jvs history --limit 0
```

`--limit 0` means no limit. `jvs history from` without a save point starts from
the active workspace's current position.

## View

View is for looking without changing anything:

```bash
jvs view <save>
jvs view <save> notes.md
```

JVS prints a read-only view path. Open that path to compare old content with
your current workspace. Close the view when you are done:

```bash
jvs view close <view-id>
```

View does not change workspace files and does not change history.

## Restore

Restore brings files from a save point back into a workspace. Restore is
preview-first:

```bash
jvs restore <save>
jvs restore --run <restore-plan-id>
```

The first command creates a plan and changes nothing. The plan tells you the
folder, workspace, save point, and file impact. The second command runs that
reviewed plan after JVS rechecks that the folder still matches the preview.

If the folder has unsaved changes, choose one of these during preview:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` when the current folder state matters. Use
`--discard-unsaved` only when those local changes are intentionally disposable.

For a smaller restore, name one path:

```bash
jvs restore <save> --path notes.md
```

## Workspace Deletion

Deleting a workspace is also preview-first:

```bash
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

The preview does not delete the folder. Read the folder path and workspace name
before running the plan. Running the plan deletes that workspace folder and its
workspace entry. It does not remove save point storage.

If the workspace has unsaved changes, JVS fails closed. Save or restore those
changes before deleting the workspace.

## Cleanup

Cleanup frees save point storage JVS no longer needs. Cleanup is preview-first:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

Preview shows what can be cleaned and prints a plan ID. Run deletes only the
save point storage selected by that reviewed plan. Cleanup does not remove
workspace folders.

## Recovery

Before restore changes files, JVS prepares recovery information. If the restore
is interrupted, JVS can show what is waiting:

```bash
jvs recovery status
```

Then either continue the restore:

```bash
jvs recovery resume <recovery-plan>
```

or return the folder to the protected pre-restore state:

```bash
jvs recovery rollback <recovery-plan>
```
