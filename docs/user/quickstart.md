# Quickstart

This guide starts from an ordinary folder. Your files stay in that folder, and
you can use your normal editor, terminal, and tools while JVS keeps save points
for you.

The commands use a new folder named `myproject`. If you already have a folder,
move into it and start at step 1 with `jvs init`.

## 1. Prepare A Folder

```bash
mkdir myproject
cd myproject
jvs init
```

What to look for:

- The command should finish without an error.
- The folder now contains `.jvs/`, which is JVS control data.
- Your own files, if any, stay where they were.

Expected shape:

```text
myproject/
├── .jvs/
└── your files
```

Next: create something worth saving.

## 2. Save A Baseline

```bash
echo "Hello JVS" > notes.md
jvs status
```

In the status output, look for:

```text
Workspace: main
Newest save point: none
Unsaved changes: yes
```

Now save:

```bash
jvs save -m "baseline"
```

The output includes the full save point ID. It is a long value like:

```text
Saved save point 1700000000000-abc12345
```

Copy the ID when you need to name this saved state later. In this guide,
`<baseline-save>` means that ID, or a shorter beginning of it that JVS accepts.

Next: make a second save point so history has something to show.

## 3. Edit And Save Again

```bash
echo "More work" >> notes.md
jvs status
```

Look for:

```text
Unsaved changes: yes
```

Save again:

```bash
jvs save -m "added notes"
```

You now have two save points: the baseline and the later version.

Next: list them.

## 4. Check History

```bash
jvs history
```

Look for your messages:

```text
baseline
added notes
```

History shows each save point with a copyable ID or short ID. The short form is
usually enough anywhere the docs write `<save>`.

If JVS says the ID is ambiguous or non-unique, use more characters from the
same ID. If you need the full ID again, `jvs save` printed it when you created
the save point, and `jvs history --json` includes it in the `save_point_id`
field.

Useful checks:

```bash
jvs status
jvs history --limit 10
```

Next: view an older save point without changing your folder.

## 5. View A Save Point Without Changing Files

```bash
jvs view <baseline-save> notes.md
```

JVS prints a `View:` line and a `View path:` line. Open the view path with
your editor or a normal command:

```bash
cat <printed-view-path>
```

What to know:

- A view is read-only.
- Your workspace files do not change.
- History does not change.

When you are done, close the view:

```bash
jvs view close <view-id>
```

Next: try a restore preview.

## 6. Restore Safely

Make a bad edit:

```bash
echo "bad edit" > notes.md
jvs status
```

Look for `Unsaved changes: yes`. Because the folder has unsaved changes, you
must choose what should happen to them before restore. For this quickstart, the
bad edit is disposable, so preview a restore with:

```bash
jvs restore <baseline-save> --discard-unsaved
```

This is only a preview. Look for:

```text
Preview only. No files were changed.
Plan: <plan-id>
Run: `jvs restore --run <plan-id>`
```

Also read the impact lines. They tell you how many files would be overwritten,
created, or deleted.

If the plan looks right, run the printed command:

```bash
jvs restore --run <plan-id>
```

Check the result:

```bash
cat notes.md
jvs status
```

You should see the baseline content again. Restore does not rewrite history; it
changes the files in the workspace. If you want this restored state to become
your newest saved state, run:

```bash
jvs save -m "recovered baseline"
```

Next: restore one path instead of the whole folder.

## 7. Restore One Path

When one file or folder is wrong, ask JVS which save points contain that path:

```bash
jvs restore --path notes.md
```

Pick a save point from the candidate list, then preview a one-path restore:

```bash
jvs restore <save> --path notes.md
```

Look for the same preview clues:

```text
Preview only. No files were changed.
Run: `jvs restore --run <plan-id>`
```

Run only if the path and save point are the ones you intended:

```bash
jvs restore --run <plan-id>
```

Check the path afterward:

```bash
cat notes.md
jvs status
```

The file should match the save point you chose. Other files should be left as
they were.

Next: create a second workspace for experiments.

## 8. Create Another Workspace

Use a save point as the starting place for another real folder:

```bash
jvs workspace new experiment --from <baseline-save>
```

Look for:

```text
Folder: /path/to/experiment
Workspace: experiment
Started from save point: <baseline-save>
```

Move into the printed folder:

```bash
cd /path/to/experiment
jvs status
```

This workspace has its own folder. The original `myproject` workspace is
unchanged.

Next: learn how removal works without accidentally deleting the folder first.

## 9. Preview Workspace Removal

From any folder inside the same JVS project, preview removal:

```bash
jvs workspace remove experiment
```

Look for:

```text
Preview only. No workspace folder was removed.
Folder: /path/to/experiment
Workspace: experiment
Run: `jvs workspace remove --run <plan-id>`
```

The preview does not delete the folder. If the folder has unsaved changes, JVS
will ask you to preview with `--force` only when those changes are intentionally
disposable:

```bash
jvs workspace remove experiment --force
```

Run the printed command only after checking the folder path:

```bash
jvs workspace remove --run <plan-id>
```

This removes the workspace folder and workspace entry. It does not remove save
point storage.

Check afterward:

```bash
jvs workspace list
```

The `experiment` workspace should no longer appear. The folder path shown in
the preview should be gone. Save point storage remains until you review and run
cleanup.

Next: preview cleanup.

## 10. Preview Cleanup

Cleanup is also preview-first:

```bash
jvs cleanup preview
```

Look for:

```text
Plan ID: <plan-id>
Reclaimable: ...
Run: jvs cleanup run --plan-id <plan-id>
```

If the plan lists storage you really want JVS to remove, run:

```bash
jvs cleanup run --plan-id <plan-id>
```

Cleanup never removes your workspace folders. It only removes save point
storage selected by the reviewed cleanup plan.

Look for a completed cleanup message. To double-check, run another preview:

```bash
jvs cleanup preview
```

It should show less storage to clean, or nothing to clean for the plan you just
ran.

## Next

- Read [Concepts](concepts.md) for the mental model.
- Use [Command Reference](commands.md) when you need flags or exact command
  forms.
- Keep [Safety](safety.md) and [Recovery](recovery.md) nearby before large
  restores.
