# Troubleshooting

Start with:

```bash
jvs status
jvs doctor
```

Use `jvs doctor --strict` before a large restore or when you suspect
corruption.

## "not a JVS repository"

You are outside a folder adopted by JVS, or JVS cannot find `.jvs/`.

Fix:

```bash
cd /path/to/your/folder
jvs status
```

If the folder has not been adopted yet:

```bash
jvs init
```

## "save point ID is required"

The command needs a save point ID, or a short beginning of one that JVS can
recognize.

The easiest full ID is the one printed by `jvs save`. To choose from older save
points, start with:

```bash
jvs history
```

For a file or directory:

```bash
jvs history --path src/config.yaml
```

## "is not a save point ID"

The value is not a known save point ID, or the short ID is not enough for JVS
to recognize it. Copy the ID from `jvs save` output, or copy a longer ID from
`jvs history`, and run the command again.

## "ambiguous" Or "non-unique"

The short ID matches more than one save point. Use more characters from the
same ID and try again.

If the human history output does not show enough of the ID, run:

```bash
jvs history --json
```

Copy the `save_point_id` field for the save point you want. You only need this
when the shorter ID is not enough.

## Restore Says No Files Were Changed

This is expected for preview mode. Restore has two steps:

```bash
jvs restore <save>
jvs restore --run <restore-plan-id>
```

Run the exact command printed after `Run:`.

What is okay:

- The preview shows the folder you meant to restore.
- The preview lists files that would be overwritten, created, or deleted.
- The preview says history will not change.
- Nothing in your folder changes until `jvs restore --run <restore-plan-id>`.

## Folder Has Unsaved Changes

Restore refuses to overwrite unsaved work by default. This is a decision
preview. JVS is asking how you want to protect or discard the current local
changes.

Choose one:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` when the existing folder state matters. Use
`--discard-unsaved` only when you intend to throw away those edits.

What is okay:

- You see choices instead of a restore run.
- No files change before you choose one of the commands.
- `--save-first` creates a save point for the current folder before restore.

Be careful: `--discard-unsaved` means the current local changes are disposable.

## Restore Plan No Longer Runs

The folder changed after preview, or the plan is stale. Create a new preview:

```bash
jvs restore <save>
```

Then run the new restore plan ID.

This is a safety stop. JVS binds run commands to the previewed folder state so
an old plan cannot silently apply to a different folder state.

## Path Restore Rejects A Path

Path restore accepts workspace-relative paths only. Do not use absolute paths,
`..`, or JVS control paths.

Good:

```bash
jvs restore <save> --path src/config.yaml
```

Not accepted:

```bash
jvs restore <save> --path /tmp/config.yaml
jvs restore <save> --path ../config.yaml
jvs restore <save> --path .jvs
```

## Path Restore Did Not Restore Another File

That is expected. Path restore changes only the target path named in the plan.

Example:

```bash
jvs restore <save> --path src/config.yaml
jvs restore --run <restore-plan-id>
```

If the preview says `src/config.yaml`, files such as `src/notes.md` are not
restored or deleted by that run. Make a new preview for a different path.

## A Read-Only View Cannot Be Edited

That is intentional. Views are for inspection. Copy out anything you need, then
close the view:

```bash
jvs view close <view-id>
```

History and view commands are read-only. If `jvs history` or `jvs view` appears
to change your files, stop and check whether another command or editor changed
the folder.

## New Workspace Folder Already Exists

`workspace new` refuses to overwrite an existing folder.

Fix:

```bash
jvs workspace new ../experiment-2 --from <save>
```

or move/remove the existing folder yourself and retry.

## Workspace Delete Only Printed A Plan

That is expected. Workspace delete is two-step:

```bash
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

What is okay:

- The preview says no workspace folder was deleted.
- It shows the `Folder` and `Workspace`.
- It prints a `Run:` command.

Review the folder path before running the plan.

## Workspace Delete Refuses Because Of Local Changes

JVS is protecting local changes in that workspace. If those changes matter,
go to that folder and save them first:

```bash
cd <workspace-folder>
jvs save -m "before deleting workspace"
```

If the local changes are intentionally disposable, restore the workspace to the
save point you want before deleting it.

## Cannot Delete `main`

The `main` workspace cannot be deleted:

```bash
jvs workspace delete main
```

This protects the originally adopted folder. If you want to delete a separate
workspace, first list workspaces and choose the one you created:

```bash
jvs workspace list
jvs workspace delete experiment
```

## Cleanup Did Not Delete A Workspace Folder

That is expected. Cleanup is not workspace deletion.

Cleanup only reviews save point storage:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

To delete a workspace folder, use:

```bash
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

## Workspace Delete Did Not Free Save Point Storage

That is expected. Workspace delete deletes the selected workspace folder and
workspace entry. It does not delete save point storage.

After deleting a workspace, run cleanup separately if you want to review
storage that may no longer be protected:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

## Another Recovery Plan Blocks Restore

Finish the active recovery first:

```bash
jvs recovery status
jvs recovery resume <recovery-plan>
```

or:

```bash
jvs recovery rollback <recovery-plan>
```

## Permission Denied

Check ownership and write permissions on the folder and `.jvs/`:

```bash
ls -la .
ls -la .jvs
```

Fix permissions with your normal operating-system tools, then run:

```bash
jvs doctor
```

## Out Of Space

Free space on the filesystem that contains the folder and `.jvs/`, then rerun
the preview or command.

For large restores, use path restore when possible:

```bash
jvs restore <save> --path path/you/need
```

If you are trying to free JVS save point storage, start with:

```bash
jvs cleanup preview
```

Review the protected and reclaimable save points before running cleanup.

## Doctor Reports Findings

For leftover operation state:

```bash
jvs doctor --repair-list
jvs doctor --repair-runtime
```

If `doctor --strict` reports integrity problems, preserve the folder and
diagnostic output before making manual changes.
