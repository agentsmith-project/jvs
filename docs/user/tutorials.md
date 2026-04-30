# Tutorials

These tutorials are written as real work stories. The first tutorial is a
practice flow you can run with only shell file commands and JVS. The later
stories are adaptable templates: keep the JVS steps, but replace the project
commands with the tools you actually use.

Use `<save>` for the full save point ID printed by `jvs save`, or for an ID
copied from `jvs history` when JVS accepts it.
Use `<restore-plan-id>` for the plan ID printed by a restore preview.
Use `<remove-plan-id>` for the plan ID printed by a workspace remove preview.
Use `<cleanup-plan-id>` for the plan ID printed by a cleanup preview.
Anything shown in angle brackets is a placeholder, not text to type exactly.
Replace it with the value JVS printed or with the folder path on your machine:

| Placeholder | Replace it with |
| --- | --- |
| `<save>` | A save point ID from `jvs save`, or an ID from `jvs history` that JVS accepts |
| `<baseline-save>` | The save point ID for the baseline you saved earlier |
| `<restore-plan-id>` | The plan ID printed by the restore preview you just reviewed |
| `<remove-plan-id>` | The plan ID printed by the workspace remove preview you just reviewed |
| `<cleanup-plan-id>` | The plan ID printed by the cleanup preview you just reviewed |
| `<view-path>` | The read-only file or folder path printed by `jvs view` |
| `<view-id>` | The view ID printed by `jvs view` |
| `<printed-folder>` | The folder path printed by `jvs workspace new` |
| `<main-folder>` | The original folder you were working in before opening another workspace |

Commands such as `python`, `cp`, `diff`, `head`, and opening an image viewer
are examples to make the stories concrete. Do not type those lines exactly
unless you have the same files and tools. Replace them with your own scripts,
spreadsheet exports, editors, or design applications. The reusable parts are
the JVS commands and the preview-before-run habit.

A plan ID belongs to the preview that printed it. When a preview shows a
`Run:` line, use that matching run command instead of reusing an ID from a
different preview.

If JVS says a short save point ID is ambiguous or non-unique, use more of the
same ID. If the history output does not show enough characters, run
`jvs history --json` and copy the `save_point_id` field for the save point you
want.

## Choose A Tutorial

| Start with | Good fit | Main JVS practice |
| --- | --- | --- |
| [Practice: Copyable Document Folder](#practice-copyable-document-folder) | You want a safe copyable first run | Save, decision preview, path restore with `--save-first` |
| [Client Delivery Package](#client-delivery-package) | Reports, contracts, exports, or delivery folders | View a saved file, restore one file or one delivery folder |
| [Media Sorting Session](#media-sorting-session) | Photos, video, audio, captions, or selected media folders | Save import/select milestones, restore one subfolder |
| [Course Or Research Materials Pack](#course-or-research-materials-pack) | Lessons, readings, slides, handouts, or research packets | Find a file with `history --path`, restore a file or folder |
| [Data Experiment](#data-experiment) | Model runs, parameters, input data, or generated results | Save baselines, inspect old config, restore with `--save-first` |
| [Data Cleaning And Analysis Recovery](#data-cleaning-and-analysis-recovery) | Cleaned tables, analysis files, or accidental deletes | Find a missing file, view it, restore one path |
| [Agent Sandbox](#agent-sandbox) | Risky agent, script, or teammate experiments | Create another workspace, remove it, keep cleanup separate |
| [Game Or Design Asset Pack](#game-or-design-asset-pack) | Binary assets, art exports, audio, config, and notes | Save asset milestones, view old assets, restore one asset folder |
| [Everyday Document Project](#everyday-document-project) | Reports, proposals, campaign copy, or ordinary writing | Save writing milestones, view old drafts, restore a file or folder |

## Practice: Copyable Document Folder

### When To Use

Use this first if you want a safe practice run. It creates a tiny document
folder from scratch, saves two moments, previews a restore with unsaved
changes, then restores one file.

### Prepare

Create a new practice folder:

```bash
mkdir jvs-practice-report
cd jvs-practice-report
jvs init

mkdir -p drafts data exports
printf "# Practice Report\n\nOpening notes.\n" > drafts/report.md
printf "source,total\nonline,42\n" > data/survey.csv
jvs save -m "practice baseline"
```

Add a second saved moment:

```bash
printf "\n## Findings\nCustomers asked for clearer onboarding.\n" >> drafts/report.md
printf "Practice export\n" > exports/report.txt
jvs save -m "practice findings and export"
```

### Commands

Make a bad edit:

```bash
printf "# Practice Report\n\nWrong replacement text.\n" > drafts/report.md
jvs status
```

Ask JVS what would happen if you restored the report without choosing a safety
option:

```bash
jvs history --path drafts/report.md
jvs restore <baseline-save> --path drafts/report.md
```

Because the report has unsaved changes, JVS should show a decision preview.
That preview changes no files and does not print a runnable plan. Now rerun
the preview with the safety choice that keeps your bad edit as a save point
before restoring:

```bash
jvs restore <baseline-save> --path drafts/report.md --save-first
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `drafts/report.md` contains the baseline report text again.
- `exports/report.txt` still exists, because the restore targeted only one
  file.
- `jvs history` includes a save point for the state JVS protected before the
  restore.
- `jvs status` reports no unsaved changes after the restore finishes.

### Common Pitfalls

- Do not run `jvs restore --run <restore-plan-id>` until you have reviewed the
  restore preview that printed that exact restore plan ID.
- A decision preview is not runnable. Add `--save-first` or
  `--discard-unsaved` and preview again.
- `--path drafts/report.md` means only that file is restored. Use a
  whole-folder restore only when you have read the broader impact summary.

### Next Step

Try the same pattern in one of your own folders: save, make a harmless edit,
preview a path restore, then decide whether to use `--save-first` or
`--discard-unsaved`.

## Client Delivery Package

### When To Use

Use this when one folder contains drafts, source notes, exported files, and the
package you plan to send to a client, reviewer, or partner. The goal is to
recover from a mistaken overwrite without rolling back every draft.

### Prepare

This story uses small text files to stand in for PDFs, spreadsheets, and signed
documents. Replace the `printf` lines with exports from your word processor,
spreadsheet, contract tool, or design application.

```bash
mkdir client-delivery-pack
cd client-delivery-pack
jvs init

mkdir -p drafts source exports delivery
printf "Report draft v1\n" > drafts/report.md
printf "Contract terms v1\n" > drafts/contract.md
printf "Survey rows v1\n" > source/survey.csv
printf "Client report PDF v1\n" > delivery/report.pdf
printf "Contract PDF v1\n" > delivery/contract.pdf
jvs save -m "client packet baseline"
```

### Commands

Make a second package and save it:

```bash
printf "Report draft v2 with chart notes\n" > drafts/report.md
printf "Client report PDF v2\n" > delivery/report.pdf
printf "Contract PDF v2\n" > delivery/contract.pdf
jvs save -m "client review packet v2"
```

Simulate an accidental overwrite right before delivery:

```bash
printf "Wrong report from another client\n" > delivery/report.pdf
jvs status
```

Find a save point that has the delivery file:

```bash
jvs history --path delivery/report.pdf
jvs view <save> delivery/report.pdf
```

Open `<view-path>` with your normal PDF viewer or editor. When you have checked
that it is the right report, close the view:

```bash
jvs view close <view-id>
```

Restore only the delivery report while protecting the accidental overwrite in
case you need to inspect it later:

```bash
jvs restore <save> --path delivery/report.pdf --save-first
jvs restore --run <restore-plan-id>
```

If a whole delivery folder was exported incorrectly, preview that folder
instead:

```bash
jvs restore <save> --path delivery --save-first
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `delivery/report.pdf` is back to the selected saved version.
- `drafts/report.md` and `source/survey.csv` were not rolled back by the
  one-file restore.
- `jvs history` includes the save point created by `--save-first`.
- `jvs status` reports no unsaved changes after the restore finishes.

### Common Pitfalls

- Do not restore the whole folder when only the delivery export is wrong.
  Start with `--path delivery/report.pdf` or `--path delivery`.
- A file extension such as `.pdf` does not change the JVS workflow. JVS restores
  files by path.
- Keep save messages specific, such as `client review packet v2`, so
  `jvs history --grep "client"` is useful later.
- Do not reuse a restore plan ID after previewing a different path.

### Next Step

Before sending the packet, save the exact delivered state:

```bash
jvs save -m "sent client delivery package"
```

## Media Sorting Session

### When To Use

Use this when you are organizing photos, video clips, audio takes, captions, or
exported edits. The goal is to save import and selection milestones, then
recover one subfolder after a mistaken delete or batch export.

### Prepare

This story uses placeholder text files with media-like names so the commands
can run anywhere. Replace them with files imported from your camera, recorder,
phone, or editing software.

```bash
mkdir media-sorting-session
cd media-sorting-session
jvs init

mkdir -p import selects edits audio delivery
printf "raw photo 001\n" > import/photo-001.jpg
printf "raw photo 002\n" > import/photo-002.jpg
printf "video clip 001\n" > import/clip-001.mov
printf "room audio 001\n" > audio/room-001.wav
jvs save -m "raw media import"
```

### Commands

Create a first selection and save it:

```bash
cp import/photo-001.jpg selects/hero-photo.jpg
cp import/clip-001.mov selects/opening-clip.mov
printf "Crop hero-photo tighter\n" > edits/edit-notes.md
jvs save -m "first media selects"
```

Simulate a mistake where the selected media folder is removed:

```bash
rm -f selects/hero-photo.jpg selects/opening-clip.mov
jvs status
```

Find and inspect the saved selection folder:

```bash
jvs history --path selects
jvs view <save> selects
```

Open `<view-path>` in your file browser or media tool. Close the view when you
are done checking:

```bash
jvs view close <view-id>
```

Restore only the selection folder:

```bash
jvs restore <save> --path selects --discard-unsaved
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- The `selects` folder contains the selected files again.
- Files under `import`, `audio`, and `edits` were not replaced by the path
  restore.
- `jvs history` still shows the raw import and first selection save points.
- `jvs status` reports no unsaved changes if the restore exactly returned the
  selected folder to the saved state.

### Common Pitfalls

- Do not use cleanup to recover media. Use `history`, `view`, and `restore`.
- Restore a folder such as `selects` when the mistake is limited to that
  folder.
- Use `--save-first` instead of `--discard-unsaved` if the removed or replaced
  files might still be useful.
- Large media folders can make preview impact summaries feel important; read
  them before running the restore plan.

### Next Step

After the selection is correct again, save the reviewed state:

```bash
jvs save -m "media selects restored and reviewed"
```

## Course Or Research Materials Pack

### When To Use

Use this when a folder contains lesson drafts, readings, slides, handouts,
research notes, and a final package. The goal is to reorganize freely while
still being able to find an older lesson, reading list, or handout folder.

### Prepare

This story uses simple text files as stand-ins for slides, PDFs, and handouts.
Replace them with exports from your teaching, research, or document tools.

```bash
mkdir course-materials-pack
cd course-materials-pack
jvs init

mkdir -p drafts readings slides handouts final
printf "Workshop outline v1\n" > drafts/outline.md
printf "Reading list v1\n" > readings/list.md
printf "Slide deck v1\n" > slides/session-1.txt
printf "Checklist v1\n" > handouts/checklist.md
jvs save -m "course outline and starter materials"
```

### Commands

Reorganize the materials and save the first complete package:

```bash
printf "Workshop outline v2 with exercises\n" > drafts/outline.md
printf "Slide deck v2 with activity\n" > slides/session-1.txt
printf "Participant checklist v2\n" > handouts/checklist.md
printf "Final packet v1\n" > final/packet.txt
jvs save -m "first complete course packet"
```

Make another reorganization that accidentally removes an older reading list:

```bash
rm readings/list.md
printf "Final packet v2 without reading list\n" > final/packet.txt
jvs status
```

Find the save point that still had the reading list:

```bash
jvs history --path readings/list.md
jvs view <save> readings/list.md
```

Open `<view-path>` in your editor. If it is the reading list you need, close
the view and restore just that file:

```bash
jvs view close <view-id>
jvs restore <save> --path readings/list.md --save-first
jvs restore --run <restore-plan-id>
```

Restore a whole handouts folder if a batch export replaced it:

```bash
jvs restore <save> --path handouts --save-first
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `readings/list.md` exists again after the one-file restore.
- A handouts restore changes only the `handouts` folder.
- `final/packet.txt` is not replaced unless you restore the whole folder or
  choose `--path final`.
- `jvs history` includes the save point created by `--save-first`.

### Common Pitfalls

- Do not assume the newest final packet is the source for every older handout.
  Use `history --path` for the file or folder you care about.
- If you are unsure which save point is right, use `jvs view` before restoring.
- A restored reading or handout is not a new course milestone until you run
  `jvs save`.
- Use one path restore per mistake when several folders were reorganized in
  different ways.

### Next Step

After the course materials are correct, save the version you will distribute:

```bash
jvs save -m "course packet ready to distribute"
```

## Data Experiment

### When To Use

Use this when you are trying model inputs, parameters, and result files in the
same project folder. The goal is to keep a baseline, compare experiment
states, restore one file or a whole folder, and protect today's work before a
larger restore.

### Prepare

Start in a folder that has your scripts, input data, configuration files, and
output folder.

The non-JVS commands below stand in for your own data copy and training tools.
If you do not have these files or scripts, use the practice tutorial above or
replace the commands with your own workflow.

```bash
mkdir customer-risk-experiment
cd customer-risk-experiment
jvs init

mkdir -p data configs outputs
cp ~/Downloads/customer_sample.csv data/train.csv
printf "learning_rate: 0.01\nmax_depth: 4\n" > configs/train.yaml
python prepare.py --input data/train.csv --output data/prepared.csv
```

Save the baseline before the first training run:

```bash
jvs status
jvs save -m "baseline prepared data and default parameters"
```

### Commands

Run a first experiment and save the result:

Replace `python train.py ...` with the command that creates your own result
files.

```bash
python train.py --config configs/train.yaml --output outputs/run-001
jvs status
jvs save -m "run 001 default parameters"
```

Change a parameter, run again, and save again:

```bash
printf "learning_rate: 0.03\nmax_depth: 6\n" > configs/train.yaml
python train.py --config configs/train.yaml --output outputs/run-002
jvs save -m "run 002 higher learning rate and depth"
```

Find the baseline or a specific run:

```bash
jvs history --grep "baseline"
jvs history --grep "run 002"
```

Look at an old configuration before changing anything:

```bash
jvs view <save> configs/train.yaml
```

The command prints a read-only path. Open that path in your editor or compare
it with your working file. If you do not use `diff`, open both files in the
tool you normally use:

```bash
diff -u <view-path> configs/train.yaml
jvs view close <view-id>
```

Restore only the parameter file from the baseline, while saving today's work
first:

```bash
jvs restore <baseline-save> --path configs/train.yaml --save-first
jvs restore --run <restore-plan-id>
```

Restore the whole folder to the baseline, again saving today's work first:

```bash
jvs restore <baseline-save> --save-first
jvs restore --run <restore-plan-id>
```

If a restore preview finds unsaved changes and you did not choose
`--save-first` or `--discard-unsaved`, JVS shows a decision preview. It changes
no files and does not print a runnable plan. Run the preview again with one of
the two safety options when you are ready.

### How To Know It Worked

- `jvs history --grep "baseline"` shows the baseline save point.
- `jvs history --grep "run"` shows each saved experiment run.
- After a one-file restore, `configs/train.yaml` matches the selected save
  point, and the rest of the folder is left alone.
- After a whole-folder restore, `jvs status` reports no unsaved changes.
- If you used `--save-first`, `jvs history` includes a new save point for the
  work JVS protected before restore.

### Common Pitfalls

- Do not run a whole-folder restore when you only need one file. Use
  `--path configs/train.yaml` or `--path outputs/run-002` for a smaller
  change.
- Do not use `--discard-unsaved` for experiment work you still need. Use
  `--save-first` when in doubt.
- A preview is not the restore. Files change only after `jvs restore --run
  <restore-plan-id>`.
- A view is read-only. Copy from it or restore from it; do not treat it as the
  active project folder.

### Next Step

Write save messages that include the question you were testing, such as
`run 003 smaller sample and no outlier removal`. That makes
`jvs history --grep` useful later.

## Data Cleaning And Analysis Recovery

### When To Use

Use this when a cleaning script, spreadsheet export, or manual edit removes a
file you still need. You remember the file path but not the exact save point.

### Prepare

Set up a small analysis folder and save clean stages as you work:

The `cp` and `python` lines are examples for an analysis project. Replace them
with your spreadsheet export, notebook run, or cleaning tool.

```bash
mkdir churn-analysis
cd churn-analysis
jvs init

mkdir -p raw cleaned notebooks
cp ~/Downloads/customers.csv raw/customers.csv
python clean_customers.py raw/customers.csv cleaned/customers.csv
jvs save -m "cleaned customer table"

python summarize.py cleaned/customers.csv notebooks/summary.md
jvs save -m "summary after customer cleanup"
```

Now simulate the mistake:

```bash
rm cleaned/customers.csv
jvs status
```

### Commands

Find save points that had the missing file:

```bash
jvs history --path cleaned/customers.csv
```

Open the file from a likely save point before restoring it:

Use `head` only if it is a familiar way for you to preview a text file. Opening
`<view-path>` in an editor is just as good.

```bash
jvs view <save> cleaned/customers.csv
head <view-path>
jvs view close <view-id>
```

Preview the path restore. Because the file was deleted locally, choose the
safety option that matches your intent. If the deletion was accidental and you
do not need to keep it, use:

```bash
jvs restore <save> --path cleaned/customers.csv --discard-unsaved
```

Review the preview, then run the printed command:

```bash
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `cleaned/customers.csv` exists again.
- `jvs status` reports no unsaved changes if the path restore exactly returned
  the folder to a saved state.
- `jvs history` is unchanged by restore; it still shows your earlier save
  points.
- The preview lists only the selected path, not the whole analysis folder.

### Common Pitfalls

- `jvs history --path cleaned/customers.csv` searches by path. It does not
  restore anything.
- `jvs view` is for checking first. It does not put the file back into your
  folder.
- If you choose `--discard-unsaved`, local changes at the selected path are
  disposable for that operation.
- If another file changed by mistake too, restore that file separately or
  preview a whole-folder restore and read the impact carefully.

### Next Step

After the file is back, rerun the analysis that depended on it and save the
repaired state:

```bash
python summarize.py cleaned/customers.csv notebooks/summary.md
jvs save -m "repaired customer table and summary"
```

## Agent Sandbox

### When To Use

Use this when you want an agent, script, or teammate to explore a risky change
in another real folder while `main` stays available.

### Prepare

Save the state you want the experiment to start from:

```bash
cd product-notes
jvs status
jvs save -m "before agent experiment"
jvs history --limit 3
```

Choose the save point ID or short ID from the history output. If JVS says it is
ambiguous or non-unique, use a longer or full ID.

### Commands

Create a new workspace from that save point:

```bash
jvs workspace new agent-run-42 --from <save>
```

JVS prints a folder path. Move into that folder and let the agent or script
work there:

Replace `./run-agent-task.sh` with the agent, script, or manual edit you want
to isolate. The important part is that it runs in `<printed-folder>`, not in
`<main-folder>`.

```bash
cd <printed-folder>
./run-agent-task.sh
jvs status
jvs save -m "agent run 42 result"
```

Return to the original folder and confirm it was not changed:

```bash
cd <main-folder>
jvs status
jvs workspace list
```

When you are done with the experiment folder, preview its removal:

```bash
jvs workspace remove agent-run-42
```

Review the folder path and printed `Run:` command, then run it:

```bash
jvs workspace remove --run <remove-plan-id>
```

Check that it is gone:

```bash
jvs workspace list
```

Storage cleanup is a separate reviewed step. Only do it when you specifically
want to free space and have read the preview:

```bash
jvs cleanup preview
jvs cleanup run --plan-id <cleanup-plan-id>
```

### How To Know It Worked

- `jvs workspace list` shows both `main` and `agent-run-42` after creation.
- The agent writes files under the printed experiment folder, not under
  `main`.
- After the remove plan runs, `jvs workspace list` no longer shows
  `agent-run-42`, and the printed experiment folder path is gone.
- Cleanup, if you choose to run it, reports completion after its own preview
  and its own plan ID. A fresh cleanup preview should show less storage to
  clean, or nothing to clean for that plan.

### Common Pitfalls

- Do not paste the agent's commands into `main` if the goal is isolation.
- `workspace remove` is preview-first; the preview alone does not delete the
  folder.
- If the experiment workspace has unsaved changes, decide whether those files
  are still needed before using `--force` on the remove preview.
- Do not treat cleanup as part of workspace removal. It is a separate storage
  task with a separate review step.

### Next Step

If the experiment produced useful work, copy the files you want back into
`main` deliberately, or keep the experiment workspace and save more progress
there.

## Game Or Design Asset Pack

### When To Use

Use this when a folder contains artwork, exported assets, configuration files,
and notes that should be saved together. This works well for game prototypes,
UI kits, level packs, sound sets, or brand design folders.

### Prepare

Create the project folder and save the first playable or reviewable state:

The `cp ~/Downloads/...` commands represent files exported from your art,
audio, or design tools. Replace them with your real asset paths. If you are
only practicing, create small placeholder files instead.

```bash
mkdir city-level-pack
cd city-level-pack
jvs init

mkdir -p assets/ui assets/audio configs notes
cp ~/Downloads/title.png assets/ui/title.png
cp ~/Downloads/theme.wav assets/audio/theme.wav
printf "enemy_speed: 1.0\nspawn_rate: 4\n" > configs/balance.yaml
printf "First pass art direction\n" > notes/art-direction.md
jvs save -m "first playable city level pack"
```

### Commands

Save a later art and tuning pass:

```bash
cp ~/Downloads/title-v2.png assets/ui/title.png
printf "enemy_speed: 1.2\nspawn_rate: 5\n" > configs/balance.yaml
jvs save -m "updated title art and balance tuning"
```

Look at an older asset before deciding what to restore:

```bash
jvs history --grep "first playable"
jvs view <save> assets/ui/title.png
```

Open `<view-path>` in your image viewer. If the file is not an image, open it
with the viewer or editor that matches that asset type. Close the view after
checking it:

```bash
jvs view close <view-id>
```

Restore only the balance file:

```bash
jvs restore <save> --path configs/balance.yaml --discard-unsaved
jvs restore --run <restore-plan-id>
```

Restore a folder of assets if a batch export went wrong:

```bash
jvs restore <save> --path assets/ui --discard-unsaved
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `jvs history` shows named save points for playable or reviewable moments.
- `jvs view <save> assets/ui/title.png` gives you a read-only file to inspect.
- A path restore for `configs/balance.yaml` changes that file without replacing
  the entire project folder.
- A path restore for `assets/ui` changes only that folder.

### Common Pitfalls

- Do not use cleanup as an undo command. Use `view` and `restore` for old
  versions.
- Do not run cleanup just because assets are large. First run `jvs cleanup
  preview`, read what it says, and continue only if freeing that storage is
  truly your goal.
- Asset tools often rewrite several files at once. Run `jvs status` before
  restore so you know whether there are unsaved changes.
- If you want to keep a new art pass before returning to an older one, use
  `--save-first`.

### Next Step

Save review milestones with plain messages, for example
`review build with final title art`. Designers and producers can then find the
right save point with `jvs history --grep "review"`.

## Everyday Document Project

### When To Use

Use this for reports, proposals, grant drafts, policy documents, campaign
copy, meeting packets, or any folder where several ordinary files change
together over time.

### Prepare

Create or adopt the folder:

```bash
mkdir spring-report
cd spring-report
jvs init

mkdir -p drafts data exports
printf "# Spring Report\n\nOpening notes.\n" > drafts/report.md
printf "source,total\nonline,42\n" > data/survey.csv
jvs save -m "first report outline and survey data"
```

### Commands

Save useful writing moments:

```bash
printf "\n## Findings\nCustomers asked for clearer onboarding.\n" >> drafts/report.md
jvs save -m "added findings section"

printf "\n## Recommendations\nRun a two-week onboarding review.\n" >> drafts/report.md
jvs save -m "added recommendations"
```

List the writing history:

```bash
jvs history
jvs history --grep "recommendations"
```

Read an older draft without changing your folder:

```bash
jvs view <save> drafts/report.md
```

Open `<view-path>` in your editor or copy a paragraph from it, then close the
view:

```bash
jvs view close <view-id>
```

Restore one document after a bad edit:

```bash
jvs restore <save> --path drafts/report.md --save-first
jvs restore --run <restore-plan-id>
```

Restore the whole folder before sending a package:

```bash
jvs restore <save> --save-first
jvs restore --run <restore-plan-id>
```

### How To Know It Worked

- `jvs history` shows your named writing milestones.
- `jvs view` gives you a read-only copy of an older draft.
- A one-document restore changes only that document.
- A whole-folder restore returns drafts, data, and exports to the selected save
  point.
- `--save-first` leaves a save point for the version you had right before the
  restore.

### Common Pitfalls

- Save before sending a document to someone else. A message like
  `sent to finance review` is easier to find later than `final`.
- Do not restore the whole folder when only one draft needs repair.
- If JVS shows a decision preview because there are unsaved changes, read the
  next commands and rerun with `--save-first` unless you are sure the changes
  can be discarded.
- A restored document is not a new writing milestone until you run `jvs save`.

### Next Step

After the folder is correct, save the shared version:

```bash
jvs save -m "sent spring report for review"
```
