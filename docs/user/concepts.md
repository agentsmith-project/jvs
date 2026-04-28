# Concepts

JVS saves real folders as save points. The mental model is intentionally small:

```text
folder -> workspace -> save point -> history -> view -> restore
```

## Folder

A folder is the real directory where your files live. `jvs init [folder]`
adopts that folder in place. When no folder is provided, JVS adopts the
directory you are in.

JVS stores control data in `.jvs/`. That control data is not part of save
points and is not managed by restore.

## Workspace

A workspace is a JVS-managed real folder. `main` is the default workspace name.
Most human output shows both:

```text
Folder: /path/to/myproject
Workspace: main
```

Use the folder path for your tools. Use the workspace name when selecting a
workspace with a command flag.

## Save Point

A save point is a saved copy of the managed files in one workspace, plus the
message and creation facts for that save. Save point contents are not edited in
place.

Create one with:

```bash
jvs save -m "baseline"
```

A save point is referenced by its save point ID. A unique prefix is accepted by
`view`, `restore`, and `workspace new --from`.

## History

History is the list of save points for a workspace:

```bash
jvs history
```

Helpful filters:

```bash
jvs history --path src/config.yaml
jvs history --grep "baseline"
jvs history --limit 10
```

`history --path` is useful when you only remember the file you want to recover.

## View

`view` opens a read-only copy of a save point, or a path inside a save point:

```bash
jvs view <save>
jvs view <save> src/config.yaml
```

View does not change your workspace and does not change history. Close a view
when you are done:

```bash
jvs view close <view-id>
```

## Restore

`restore` copies managed files from a save point into a workspace. It does not
delete or rewrite history.

Restore is preview-first:

```bash
jvs restore <save>
jvs restore --run <plan-id>
```

Whole-folder restore can overwrite or delete managed files, so the preview
shows the impact. Path restore is narrower:

```bash
jvs restore <save> --path src/config.yaml
```

If the folder has unsaved changes, choose one explicit option:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

## Workspace New

`workspace new` creates another real folder from a save point:

```bash
jvs workspace new experiment --from <save>
```

The source workspace is unchanged. The new workspace starts from copied files;
its own history begins when you save inside that new folder.

When `main` was adopted in place with `jvs init`, a new workspace is created as
a sibling folder next to the original folder. The command prints the exact
folder path.

## Recovery

Restore creates recovery evidence before changing files. If a restore is
interrupted, JVS can show the active plan:

```bash
jvs recovery status
```

Then either continue the restore:

```bash
jvs recovery resume <recovery-plan>
```

or roll the folder back to the protected pre-restore state:

```bash
jvs recovery rollback <recovery-plan>
```
