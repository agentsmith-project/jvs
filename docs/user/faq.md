# FAQ

## What Is JVS?

JVS is a local tool for saving real folders as save points. It is useful when a
working directory contains generated files, data, models, build outputs, or
other state that is awkward to recreate by hand.

## Does `jvs init` Move My Files?

No. `jvs init [folder]` adopts the folder in place and creates `.jvs/` control
data. Your files remain where they are.

## What Is A Workspace?

A workspace is a real folder JVS knows by name. `main` is the default workspace
name. The folder path is what your editor, scripts, and shell commands use.

## What Is A Save Point?

A save point is a saved copy of the files in a workspace, with a message and
creation facts. Create one with:

```bash
jvs save -m "baseline"
```

## Can I Save Only One File?

`jvs save` saves the files in the workspace. To recover only one file
or directory later, use path restore:

```bash
jvs restore --path src/config.yaml
jvs restore <save> --path src/config.yaml
```

## Does Restore Change History?

No. Restore copies files from a save point into the workspace. History
is kept. A later save creates a new save point in that workspace.

## Which Commands Only Preview?

These commands are safe review steps:

```bash
jvs restore <save>
jvs restore <save> --path src/config.yaml
jvs workspace remove experiment
jvs cleanup preview
```

Seeing a plan, `No files were changed`, or a printed `Run:` command means JVS
has not made the destructive change yet.

## Which Commands Actually Change Files Or Remove Folders?

These commands run a reviewed plan:

```bash
jvs restore --run <plan-id>
jvs workspace remove --run <plan-id>
jvs cleanup run --plan-id <plan-id>
```

`restore --run` changes workspace files named by the plan. `workspace remove
--run` removes the selected workspace folder. `cleanup run` deletes only save
point storage listed by a cleanup plan.

## Why Does Restore Stop When I Have Unsaved Changes?

JVS refuses to overwrite unsaved work unless you choose what should happen.
The default is a decision preview, not a file change.

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` when the current folder state matters. Use
`--discard-unsaved` only when you are intentionally throwing away current local
changes for this restore.

## Why Did Path Restore Leave Other Files Alone?

That is the safety promise. Path restore changes only the path you named:

```bash
jvs restore <save> --path src/config.yaml
jvs restore --run <plan-id>
```

If the plan says `src/config.yaml`, unrelated files are not restored or
deleted by that run.

## How Do I Find The Right Save Point?

Start with history:

```bash
jvs history
jvs history --grep "baseline"
jvs history --path src/config.yaml
```

`jvs save` prints the full save point ID when you create one. `jvs history`
shows a copyable ID or short ID, which is usually enough for `<save>`.

Then inspect before restoring:

```bash
jvs view <save> src/config.yaml
```

If JVS says the ID is ambiguous or non-unique, use more characters from the
same ID. If you need the full value, run `jvs history --json` and copy the
`save_point_id` field for the save point you chose.

## JVS Says My Save Point ID Is Ambiguous. What Now?

Use a longer version of the same ID. The full ID is printed by `jvs save` when
the save point is created. If you are choosing from history and the short ID is
not enough, run:

```bash
jvs history --json
```

Copy the `save_point_id` field for the save point you want, then run the
command again.

## How Do I Continue From An Older Save Point In Another Folder?

Use `workspace new`:

```bash
jvs workspace new experiment --from <save>
```

The command prints the new folder path. The original workspace is unchanged.

## How Do I Remove A Workspace Folder?

Use the two-step remove flow:

```bash
jvs workspace remove experiment
jvs workspace remove --run <plan-id>
```

The first command is a preview. Check the folder path, workspace name, and
local-change status. The run removes that workspace folder and workspace entry.
It does not remove save point storage.

The `main` workspace cannot be removed.

## Should I Use Cleanup To Delete A Workspace?

No. Cleanup does not delete workspace folders.

Use cleanup only after reviewing save point storage:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <plan-id>
```

If you want to remove a workspace folder, use `jvs workspace remove <name>`.

## What Does `--force` Mean For Workspace Remove?

`--force` means the local changes in that workspace can be discarded by the
remove run. Use it only after checking the folder path:

```bash
jvs workspace remove experiment --force
```

It still creates a preview first. Run only the printed `Run:` command after
review.

## Are History And View Read-Only?

Yes. `jvs history`, `jvs history --path <path>`, and `jvs view <save> [path]`
do not change workspace files or history. A view is for inspection; close it
when finished:

```bash
jvs view close <view-id>
```

## Does JVS Replace Git?

No. Git is still a strong fit for source code review, branches, merges, and
remote collaboration. JVS focuses on whole-folder save and restore for local
workspace state.

## Does JVS Require A Special Filesystem?

No. JVS works on ordinary filesystems. When the storage layer supports faster
copy behavior, JVS can use it; otherwise it falls back to portable file copies.

## Is JVS A Backup System?

No. Save points help recover local folder state, but you still need normal
backups for disk loss, account loss, or machine loss. Back up both your folder
contents and the `.jvs/` control data using your storage tools.

## What Should I Do After A Crash During Restore?

Run:

```bash
jvs recovery status
```

Then choose:

```bash
jvs recovery resume <recovery-plan>
jvs recovery rollback <recovery-plan>
```

See [Recovery](recovery.md).
