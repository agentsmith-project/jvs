# Examples

These examples use the save point workflow from the public command surface.
Anything shown in angle brackets is a placeholder, not text to type exactly:

| Placeholder | Replace it with |
| --- | --- |
| `<save>` | A save point ID from `jvs save`, or an ID from `jvs history` that JVS accepts |
| `<baseline-save>` | The save point ID for the baseline state you saved earlier |
| `<restore-plan-id>` | The plan ID printed by the restore preview you just reviewed |
| `<view-path>` | The read-only file or folder path printed by `jvs view` |
| `<view-id>` | The view ID printed by `jvs view` |
| `<printed-folder>` | The folder path printed by `jvs workspace new` |

If a short save point ID is ambiguous, use a longer or full ID. If history does
not show enough characters, `jvs history --json` includes the full
`save_point_id` value.

The JVS commands are reusable as written. Project commands such as copying
files, running scripts, editing files, or opening tools are examples only;
replace them with your own files, scripts, editor, or toolchain.

## Experiment Folder

Create repeatable experiment states:

```bash
mkdir experiments
cd experiments
jvs init

cp -r /data/input ./input
python prepare.py
jvs save -m "baseline input and environment"

python train.py --run 1
jvs save -m "run 1 result"

python train.py --run 2
jvs save -m "run 2 result"
jvs history --grep "run"
```

To return the folder to the baseline state:

```bash
jvs restore <baseline-save> --discard-unsaved
jvs restore --run <restore-plan-id>
```

## Recover One File

Find a save point that contains a missing file:

```bash
jvs restore --path config/app.yaml
```

Preview and run the one-path restore:

```bash
jvs restore <save> --path config/app.yaml
jvs restore --run <restore-plan-id>
```

Only that path is restored. History is unchanged.

## Inspect Before Restoring

Open a read-only view of the path you care about:

```bash
jvs view <save> src/config.yaml
```

The command prints a view path. Compare it with your folder using normal tools:

```bash
diff -u <view-path> src/config.yaml
```

Close the view when finished:

```bash
jvs view close <view-id>
```

## Create A Second Workspace

Start another real folder from a known save point:

```bash
jvs workspace new ../investigation --from <save>
```

JVS prints the new folder path. Move into that folder and save its own progress:

```bash
cd "$(jvs workspace path investigation)"
python reproduce_issue.py
jvs save -m "reproduced issue"
```

The original workspace is unchanged.

## Agent Or CI Loop

Use explicit save and restore steps around a run:

```bash
jvs status --json
jvs save -m "before run ${RUN_ID}"

./run-task.sh

jvs save -m "after run ${RUN_ID}"
```

To reset for the next run:

```bash
jvs restore <baseline-save> --discard-unsaved
jvs restore --run <restore-plan-id>
```

Scripts should parse JSON output rather than human text when they need stable
fields:

```bash
jvs history --json
jvs restore <save> --json
jvs restore --run <restore-plan-id> --json
```

## Health Check Before A Risky Restore

Before replacing many files:

```bash
jvs doctor --strict
jvs restore <save>
```

Review the preview. If the plan matches your intent, run the command printed
by JVS.
