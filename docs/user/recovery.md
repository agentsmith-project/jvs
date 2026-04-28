# Recovery

Restore writes recovery evidence before it changes files. If a restore is
interrupted by a crash, power loss, process kill, or filesystem error, JVS can
show the active recovery plan and help you finish safely.

## Check Recovery Status

```bash
jvs recovery status
```

For one plan:

```bash
jvs recovery status <recovery-plan>
```

The output includes the folder, workspace, source save point, path when the
restore was path-scoped, and a recommended next command.

## Resume

Use resume when you still want the restore outcome:

```bash
jvs recovery resume <recovery-plan>
```

JVS rechecks the recovery evidence and continues the interrupted restore.

## Rollback

Use rollback when you want the protected pre-restore folder state:

```bash
jvs recovery rollback <recovery-plan>
```

JVS restores the backup it created for the operation when that backup is still
needed and available.

## After Recovery

Run:

```bash
jvs status
jvs doctor
```

If the folder now has the files you want, create a save point:

```bash
jvs save -m "after recovery"
```

## What Not To Do

Do not manually delete `.jvs/recovery-plans/`, restore backup folders, or source
protection records while a recovery plan is active. Use `jvs recovery status`,
`jvs recovery resume`, or `jvs recovery rollback` so JVS can close the plan
cleanly.

## Runtime Repairs

If `doctor` reports safe runtime leftovers after a crash:

```bash
jvs doctor --repair-list
jvs doctor --repair-runtime
```

Runtime repair is separate from restore recovery. Finish active recovery plans
before starting another restore.
