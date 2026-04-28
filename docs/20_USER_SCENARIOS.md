# User Scenarios

## Save Work

Goal: record useful moments while working in a normal folder.

```bash
jvs init .
jvs save -m "empty baseline"
echo "hello" > notes.md
jvs save -m "initial notes"
jvs history
```

Expected result: history lists the save points created by this workspace.

## Inspect Before Restoring

Goal: look at an older file without changing the real folder.

```bash
jvs history --path notes.md
jvs view <save> notes.md
```

Expected result: the view is read-only and the workspace history is unchanged.

## Restore A Path

Goal: recover one file from a chosen save point.

```bash
jvs restore <save> --path notes.md
jvs restore --run <plan-id>
```

Expected result: only the requested path is replaced after the preview plan is
run.

## Restore The Folder

Goal: replace managed files with a chosen save point.

```bash
jvs restore <save>
jvs restore --run <plan-id>
```

Expected result: JVS shows a preview plan before changing files.

## Start Another Workspace

Goal: try work in a separate real folder.

```bash
jvs workspace new experiment-a --from <save>
jvs --workspace experiment-a save -m "experiment result"
```

Expected result: the new workspace starts from the selected save point and then
records its own history.

## Recover An Interrupted Restore

Goal: make restore failures explicit and recoverable.

```bash
jvs recovery status
jvs recovery resume <plan>
jvs recovery rollback <plan>
```

Expected result: recovery reports available actions and closes the operation.

## Safety Principles

- Commands that replace files surface a preview plan or explicit safety choice.
- `jvs view` is read-only.
- `jvs history --path` is the discovery path for one file or directory.
- `jvs workspace new <name> --from <save>` creates a separate real folder.
