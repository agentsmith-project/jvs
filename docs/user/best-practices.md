# Best Practices

JVS works best as a simple daily habit:

```text
save important moments -> inspect old files -> preview risky changes -> run only the matching plan
```

This page assumes you already know the basics from the
[Quickstart](quickstart.md). Use it when you want a calm routine for ordinary
work: writing, data cleanup, design files, generated outputs, agent runs, or
any folder where several files change together.

Placeholders in the examples mean:

| Placeholder | Meaning |
| --- | --- |
| `<save>` | A save point ID from `jvs save` or `jvs history` |
| `<target-folder>` | The new folder path you want JVS to create |
| `<restore-plan-id>` | The plan ID from the restore preview you just reviewed |
| `<workspace-move-plan-id>` | The plan ID from the workspace move preview you just reviewed |
| `<workspace-delete-plan-id>` | The plan ID from the workspace delete preview you just reviewed |
| `<repo-move-plan-id>` | The plan ID from the repo move preview you just reviewed |
| `<repo-rename-plan-id>` | The plan ID from the repo rename preview you just reviewed |
| `<repo-detach-plan-id>` | The plan ID from the repo detach preview you just reviewed |
| `<recovery-plan>` | The recovery plan name printed by `jvs recovery status` |
| `<cleanup-plan-id>` | The plan ID from the cleanup preview you just reviewed |
| `<view-path>` | The read-only path printed by `jvs view` |
| `<view-id>` | The view ID printed by `jvs view` |

## Adopt An Existing Folder Safely

Most people start with a folder that already matters: a project, client
deliverable, research folder, design package, or exported data folder. JVS can
start there; the folder does not need to be empty.

First, go to the exact folder you want JVS to look after:

```bash
cd /path/to/your-folder
pwd
```

Check that the printed path is the folder you meant. If you are nervous
because the folder is important, make a normal outside backup first with the
backup tool you already trust. That backup is separate from JVS.

Then start JVS:

```bash
jvs init
```

Look for `.jvs/` in the folder. That is JVS control data. Your own files stay
where they were.

Before the first save, check status:

```bash
jvs status
```

Look for the folder path and workspace name:

```text
Folder: /path/to/your-folder
Workspace: main
```

The first save records the current folder state. If that is what you want,
save it with a clear message:

```bash
jvs save -m "first save before changes"
```

## Before First Save, Know What Is In The Folder

Before your first save in an existing folder, take a minute to look at what is
there. JVS is for the folder state you choose to protect, so you should know
what that folder currently contains.

Common things to notice:

- very large outputs, exports, videos, model files, or archives
- private files, keys, credentials, personal notes, or downloaded material
- temporary files from tools that you do not want to keep as part of this
  folder's saved state
- generated output folders that are useful for recovery, or that are easy to
  recreate

If something should not be part of this folder's saved state, move it somewhere
else before the first save. Do not rely on cleanup as an undo button for a
first save you did not mean to create. Cleanup is for reviewed save point
storage later, not for deciding what belongs in the folder today.

## Save At Useful Moments

Save when the folder is in a state you might want to return to or compare
against later. Good times to save:

- before running a script, agent, export, batch edit, or outside tool that may
  change many files
- after a result is worth keeping, such as a report draft, trained output,
  cleaned data file, design pass, or reviewed package
- before accepting work from someone else into the folder
- after a restore, if the restored folder is now the state you want to keep

Before saving, check where you are:

```bash
jvs status
```

Look for:

```text
Workspace: main
Unsaved changes: yes
```

Then save with a message:

```bash
jvs save -m "baseline before supplier edits"
```

After saving, JVS prints the full save point ID. You do not need to memorize
it, but it is useful to copy when you are about to restore, view, or create a
workspace from that exact state.

## Write Messages For Future You

A good message answers: what changed, and why would I look for this later?

Helpful messages:

```bash
jvs save -m "baseline before cleanup script"
jvs save -m "draft sent to finance review"
jvs save -m "agent run 12 result before manual edits"
jvs save -m "restored customer table and reran summary"
```

Less helpful messages:

```bash
jvs save -m "stuff"
jvs save -m "final"
jvs save -m "changes"
```

When you later run:

```bash
jvs history --grep "finance"
```

the message is what helps you find the right save point without opening every
old version.

## Inspect Before Restoring

When you are unsure, look first. Viewing does not change your files.

```bash
jvs history --grep "baseline"
jvs view <save> notes.md
```

JVS prints a read-only path. Open it in your editor, or compare it with your
current file:

```bash
diff -u <view-path> notes.md
```

Close the view when you are done:

```bash
jvs view close <view-id>
```

Restore only after the old content is the one you meant to bring back.

## Restore One Path When One Thing Is Wrong

If one file or one folder is wrong, prefer path restore. It has a smaller
impact than restoring the whole folder.

Find save points that contain the path:

```bash
jvs restore --path notes.md
```

Preview the one-path restore:

```bash
jvs restore <save> --path notes.md
```

Look for:

```text
Preview only. No files were changed.
Run: `jvs restore --run <restore-plan-id>`
```

Read the path in the preview. If it is the path you meant, run the matching
restore command:

```bash
jvs restore --run <restore-plan-id>
```

Use whole-folder restore only when the whole folder should return to the save
point:

```bash
jvs restore <save>
jvs restore --run <restore-plan-id>
```

If the folder has unsaved changes, decide whether to save them first or discard
them for this restore:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Choose `--save-first` when you might want today's work later. Choose
`--discard-unsaved` only when those local changes are intentionally disposable.

## Keep Preview And Run Together

Restore, workspace move, workspace deletion, repo move, repo rename, repo
detach, and cleanup are preview-first. The first command shows a plan. The
second command runs that same plan.

Do not mix plan IDs between operations:

| Preview | Matching run |
| --- | --- |
| `jvs restore <save>` | `jvs restore --run <restore-plan-id>` |
| `jvs workspace move experiment ../experiment-archive` | `jvs workspace move --run <workspace-move-plan-id>` |
| `jvs workspace delete experiment` | `jvs workspace delete --run <workspace-delete-plan-id>` |
| `jvs repo move ../project-on-ssd` | `jvs repo move --run <repo-move-plan-id>` |
| `jvs repo rename project-review` | `jvs repo rename --run <repo-rename-plan-id>` |
| `jvs repo detach` | `jvs repo detach --run <repo-detach-plan-id>` |
| `jvs cleanup preview` | `jvs cleanup run --plan-id <cleanup-plan-id>` |

Use the `Run:` line printed by the preview you just reviewed. If you preview
again, use the newer plan ID.

`jvs repo clone <target-folder> --dry-run` is also a review step, but it is not a
plan you run by ID. It checks the clone without creating the target folder. Run
`jvs repo clone <target-folder>` only when you are ready for JVS to create a new
local project folder.

`jvs workspace new <folder> --from <save>` creates a new real folder, and
`jvs workspace rename <old> <new>` changes the JVS workspace name immediately.
Check the target folder or name before pressing Enter.

When a restore was interrupted, start with `jvs recovery status`.
`jvs recovery resume <recovery-plan>` and
`jvs recovery rollback <recovery-plan>` may change files while finishing or
rolling back that interrupted restore.

What to check before running:

- the folder path is the folder you intended
- the workspace name is the workspace you intended
- the project folder is the project you intended
- the save point or cleanup storage matches your goal
- the listed file impact is acceptable

## Use Workspaces For Risky Or Temporary Work

A workspace is another real folder JVS knows by name. Use one when you want a
separate place for an experiment, an outside helper, an agent run, or a
temporary direction you may throw away.

Create it from a save point:

```bash
jvs workspace new ../experiment --from <save>
```

JVS prints the new folder path:

```text
Folder: /path/to/experiment
Workspace: experiment
```

Move into that folder for the risky work:

```bash
cd "$(jvs workspace path experiment)"
jvs status
```

Save useful progress inside the experiment workspace:

```bash
jvs save -m "experiment result before review"
```

When you are done, go back to your original folder before deleting the
experiment workspace:

```bash
cd /path/to/main-folder
jvs workspace delete experiment
jvs workspace delete --run <workspace-delete-plan-id>
```

Do not run deletion from inside the folder you are about to delete. After the
run, check from the original folder:

```bash
jvs workspace list
```

The deleted workspace should no longer appear.

## Clean Up Only After Reviewing

Cleanup is for save point storage JVS no longer needs. It is not an undo
command, and it does not delete workspace folders.

Start with a preview:

```bash
jvs cleanup preview
```

Look for:

```text
Plan ID: <cleanup-plan-id>
Reclaimable: ...
Run: jvs cleanup run --plan-id <cleanup-plan-id>
```

Run cleanup only when the preview matches your intent:

```bash
jvs cleanup run --plan-id <cleanup-plan-id>
```

Good times to consider cleanup:

- after deleting an experiment workspace you no longer need
- after confirming the remaining workspaces still have the save points you
  care about
- when storage use matters and the preview clearly lists what JVS can remove

Common cleanup mistakes:

- using cleanup when you meant restore
- using cleanup when you meant `jvs workspace delete <name>`
- running an old cleanup plan after making more changes; preview again instead

## Back Up And Move Folders Carefully

JVS helps with local folder history, but it is not a replacement for normal
backups. Keep using your usual backup system for disk loss, laptop loss, cloud
account problems, or accidental deletion outside JVS.

When backing up a JVS folder, back up the whole folder, including `.jvs/`, if
you want the save point history to come with it. If you copy only the visible
project files and leave `.jvs/` behind, you may still have the files, but not
the JVS save points.

When moving or copying a JVS folder:

- copy the whole folder as one unit
- open the copied folder and run `jvs status`
- if JVS reports a problem, run `jvs doctor` before doing restore or cleanup
- avoid moving pieces of `.jvs/` by hand

If you only need a normal backup of today's files, your regular backup tool is
enough. If you expect to use JVS history after the move, keep the folder and
JVS control data together.

## A Simple Daily Routine

For normal work, this rhythm is enough:

```bash
jvs status
jvs save -m "short useful message"
jvs history --limit 10
```

Before risky work:

```bash
jvs save -m "before risky change"
```

Before bringing old content back:

```bash
jvs view <save> path/you/care/about
jvs restore <save> --path path/you/care/about
jvs restore --run <restore-plan-id>
```

Before deleting a workspace folder or cleaning storage, slow down and read the
preview. The few seconds you spend checking the `Run:` line are the main safety
habit.
