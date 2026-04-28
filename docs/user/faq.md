# FAQ

## What Is JVS?

JVS is a local tool for saving real folders as save points. It is useful when a
working directory contains generated files, data, models, build outputs, or
other state that is awkward to recreate by hand.

## Does `jvs init` Move My Files?

No. `jvs init [folder]` adopts the folder in place and creates `.jvs/` control
data. Your files remain where they are.

## What Is A Workspace?

A workspace is a JVS-managed real folder. `main` is the default workspace name.
The folder path is what your editor, scripts, and shell commands use.

## What Is A Save Point?

A save point is a saved copy of the managed files in a workspace, with a
message and creation facts. Create one with:

```bash
jvs save -m "baseline"
```

## Can I Save Only One File?

`jvs save` saves the managed files in the workspace. To recover only one file
or directory later, use path restore:

```bash
jvs restore --path src/config.yaml
jvs restore <save> --path src/config.yaml
```

## Does Restore Change History?

No. Restore copies managed files from a save point into the workspace. History
is kept. A later save creates a new save point in that workspace.

## Why Does Restore Stop When I Have Unsaved Changes?

JVS refuses to overwrite unsaved work unless you choose what should happen:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

## How Do I Find The Right Save Point?

Start with history:

```bash
jvs history
jvs history --grep "baseline"
jvs history --path src/config.yaml
```

Then inspect before restoring:

```bash
jvs view <save> src/config.yaml
```

## How Do I Continue From An Older Save Point In Another Folder?

Use `workspace new`:

```bash
jvs workspace new experiment --from <save>
```

The command prints the new folder path. The original workspace is unchanged.

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
