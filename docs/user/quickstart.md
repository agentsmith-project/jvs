# Quickstart

This guide starts from an ordinary folder. Your files stay in that folder.

## 1. Prepare A Folder

```bash
mkdir myproject
cd myproject
jvs init
```

`jvs init` creates JVS control data in `.jvs/` and registers the folder as the
`main` workspace. It does not move or copy your files.

Expected shape:

```text
myproject/
├── .jvs/          # JVS control data
└── ...            # your files
```

## 2. Save A Baseline

```bash
echo "Hello JVS" > notes.md
jvs status
jvs save -m "baseline"
```

`jvs status` shows the folder path, workspace name, newest save point, file
state, and whether there are unsaved changes.

## 3. Edit And Save Again

```bash
echo "More work" >> notes.md
jvs status
jvs save -m "added notes"
jvs history
```

`jvs history` prints save point IDs and messages. Use a full ID or a unique ID
prefix anywhere the docs write `<save>`.

## 4. Inspect A Save Point

```bash
jvs view <save> notes.md
```

The command prints a read-only view path. Your workspace and history are not
changed. When you are done:

```bash
jvs view close <view-id>
```

## 5. Restore Safely

Make a bad edit:

```bash
echo "bad edit" > notes.md
```

Preview a restore:

```bash
jvs restore <save> --discard-unsaved
```

JVS prints a plan, the managed files that would be overwritten, created, or
deleted, and a `Run:` command. No files change during preview.

Run the printed command:

```bash
jvs restore --run <plan-id>
```

History is not changed by restore. A later `jvs save -m "recovered"` creates a
new save point after the newest save point in that workspace.

## 6. Restore One Path

When one file or directory is wrong, search for candidates first:

```bash
jvs restore --path notes.md
```

Then preview and run a path restore:

```bash
jvs restore <save> --path notes.md
jvs restore --run <plan-id>
```

Unrelated paths outside the requested path are kept.

## 7. Create Another Workspace

To continue from a save point in another real folder:

```bash
jvs workspace new experiment --from <save>
```

The command prints the new folder path. The original workspace is unchanged.
The new workspace starts with files copied from the save point, and its own
history begins when you run `jvs save -m "message"` there.

## Next

- Read [Concepts](concepts.md) for the mental model.
- Use [Command Reference](commands.md) for flags.
- Keep [Safety](safety.md) and [Recovery](recovery.md) nearby before large
  restores.
