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

The command needs a save point ID or a unique ID prefix.

Find one:

```bash
jvs history
```

For a file or directory:

```bash
jvs history --path src/config.yaml
```

## "is not a save point ID"

The value is not a known save point ID or unique prefix. Copy an ID from
`jvs history` and run the command again.

## Restore Says No Files Were Changed

This is expected for preview mode. Restore has two steps:

```bash
jvs restore <save>
jvs restore --run <plan-id>
```

Run the exact command printed after `Run:`.

## Folder Has Unsaved Changes

Restore refuses to overwrite unsaved work by default.

Choose one:

```bash
jvs restore <save> --save-first
jvs restore <save> --discard-unsaved
```

Use `--save-first` when the existing folder state matters. Use
`--discard-unsaved` only when you intend to throw away those edits.

## Restore Plan No Longer Runs

The folder changed after preview, or the plan is stale. Create a new preview:

```bash
jvs restore <save>
```

Then run the new plan ID.

## Path Restore Rejects A Path

Path restore accepts workspace-relative paths only. Do not use absolute paths,
`..`, or paths inside `.jvs/`.

Good:

```bash
jvs restore <save> --path src/config.yaml
```

Not accepted:

```bash
jvs restore <save> --path /tmp/config.yaml
jvs restore <save> --path ../config.yaml
jvs restore <save> --path .jvs/descriptors
```

## A Read-Only View Cannot Be Edited

That is intentional. Views are for inspection. Copy out anything you need, then
close the view:

```bash
jvs view close <view-id>
```

## New Workspace Folder Already Exists

`workspace new` refuses to overwrite an existing folder.

Fix:

```bash
jvs workspace new experiment-2 --from <save>
```

or move/remove the existing folder yourself and retry.

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

## Doctor Reports Findings

For runtime leftovers:

```bash
jvs doctor --repair-list
jvs doctor --repair-runtime
```

If `doctor --strict` reports integrity problems, preserve the folder and
diagnostic output before making manual changes.
