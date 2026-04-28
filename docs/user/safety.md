# Safety

JVS is built around explicit file changes. The safest path is:

```text
save -> inspect -> preview restore -> run restore
```

## What Save Changes

`jvs save -m "message"` creates a new save point from managed files in the
workspace. It does not rewrite earlier save points.

JVS control data under `.jvs/` is not saved as workspace content.

## What View Changes

`jvs view <save>` opens a read-only copy for inspection.

It does not change:

- workspace files
- history
- the newest save point

Close views when finished:

```bash
jvs view close <view-id>
```

## What Restore Changes

`jvs restore <save>` previews a restore plan. Preview changes nothing.

`jvs restore --run <plan-id>` changes managed files after JVS rechecks the
folder state. The plan shows how many managed files would be overwritten,
deleted, or created.

Restore does not change history.

## Unsaved Changes

If managed files have unsaved changes, restore refuses to proceed until you
choose one option:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` to protect the folder state before restoring. Use
`--discard-unsaved` only when the unsaved changes are intentionally disposable.

## Prefer Path Restore For Small Recovery

For one file or directory, use:

```bash
jvs restore --path src/config.yaml
jvs restore <save> --path src/config.yaml
```

Path restore has a smaller impact than whole-folder restore and still uses a
preview plan.

## Workspace Boundaries

JVS manages files inside the registered workspace folder. It refuses unsafe
workspace overlap and rejects paths that escape the workspace.

Do not manually copy another `.jvs/` control directory into the folder, and do
not nest another workspace inside the folder you want to save.

## Health Checks

Use:

```bash
jvs doctor
jvs doctor --strict
```

`doctor` checks repository health. `--strict` performs deeper integrity checks
and is useful before high-impact restore work.

## Backups

Save points help with folder recovery, but they are not a replacement for
backups. Use your normal storage backup process for machine loss, disk loss,
or account loss.
