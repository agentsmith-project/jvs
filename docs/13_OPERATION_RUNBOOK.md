# Operation Runbook

**Status:** active save point operations runbook

## External Control Root Operator Entry

Use this section when an operator or platform integration gives you a trusted
external control root `C` and a workspace folder `W`. These operations are
explicit-target operations:

```bash
jvs init W --control-root C --workspace main --json
jvs --control-root C --workspace main status --json
jvs --control-root C --workspace main save -m "baseline" --json
jvs --control-root C --workspace main doctor --strict --json
```

Every external control root command must include
`--control-root C --workspace main`. Do not rely on CWD, `--repo`, or workspace
folder discovery for this advanced workflow. A bare workspace folder cannot
safely auto-discover an external control root, so a platform runner or wrapper
must pass the selector.

Do not run a plain `jvs init` from the workspace folder when the operator
intends to use external control data. A naked workspace-folder command creates
a different default `.jvs/` project instead of selecting the externally
controlled workspace.

The external control root doctor entry is `doctor --strict --json` only.
`--repair-runtime`, `--repair-list`, and other repair variants fail closed for
external control roots. Use strict JSON diagnostics to decide whether to
resume recovery, roll back recovery, restore missing roots, or escalate.

Pending restore previews and active or malformed recovery state block mutation
before save, cleanup run, clone publish, and lifecycle commands. Pending
restore previews close through `restore --run <plan-id>` after operator review:

```bash
jvs --control-root C --workspace main restore --run <plan-id>
```

Stale restore previews close through `restore discard <restore-plan-id>`:

```bash
jvs --control-root C --workspace main restore discard <restore-plan-id>
```

Active recovery plans close through `recovery status`, `recovery resume`, or
`recovery rollback`:

```bash
jvs --control-root C --workspace main recovery status
jvs --control-root C --workspace main recovery resume <recovery-plan>
jvs --control-root C --workspace main recovery rollback <recovery-plan>
```

Do not treat a pending or stale restore preview as an active recovery plan.
Successful `restore --run` leaves no active recovery, and completed plan
residue is non-blocking for `recovery status`, `doctor --strict --json`, and
`repo clone`. Do not read or delete private control data files to decide
whether a workspace is usable; use the public commands above and the strict
doctor report.

Malformed restore state is diagnosed through public `recovery status` and
`doctor --strict --json` output. Do not ask callers to remove private control
files; preserve the diagnostics and escalate if strict doctor cannot classify
the state.

An external workspace locator is for human discovery only. It can help a person
find the intended control root and workspace name, but it is not runtime
authority. Runtime authority comes from the explicit control root and the
control registry. If the workspace folder contains a root-level `.jvs` path in
an external-control-root workflow, the strict profile fails closed instead of
reading it as authority.

## Daily Checks

1. For the default control data location, run `jvs doctor --strict`. For an
   external control root, run
   `jvs --control-root C --workspace main doctor --strict --json`.
2. Review active recovery plans:
   ```bash
   jvs recovery status
   ```
   For an external control root, add the explicit selector:
   ```bash
   jvs --control-root C --workspace main recovery status
   ```

## Incident: Doctor Failure

1. Freeze writers for the affected folder/repo.
2. Preserve command output with `--json` where available.
3. Classify the failure: layout, publish state, audit chain, descriptor
   checksum, workspace file hash, runtime state, or recovery plan.
4. Do not run destructive cleanup until the failure class is known.
5. Escalate tamper or audit-chain failures and preserve `.jvs/audit/` and the
   affected save point descriptors as evidence.

## Incident: Runtime Artifacts

Use this only when control data is in the workspace folder's `.jvs/`. It does
not rewrite durable save point history. This is not an external control root
repair path; external control roots use `doctor --strict --json`, and repair
variants fail closed.

1. Run `jvs doctor --strict --json`.
2. Run `jvs doctor --repair-list` and confirm only public runtime repairs are
   available:
   - `clean_locks`: removes stale write-coordination runtime state
   - `rebind_workspace_paths`: rebinds safe workspace folder paths after a
     filesystem migration
   - `clean_runtime_tmp`: removes stale JVS runtime temporary files
   - `clean_runtime_operations`: removes abandoned operation records
   - `clean_runtime_cleanup_plans`: removes stale cleanup preview/run plan
     state
3. Run `jvs doctor --strict --repair-runtime`.
4. Rerun `jvs doctor --strict`.

## Incident: Restore Did Not Finish

1. Freeze writes in the affected workspace.
2. Read the recovery plan ID from the failed restore output.
3. Inspect the plan:
   ```bash
   jvs recovery status <recovery-plan>
   ```
4. If the plan says the restore target can be confirmed or retried, run:
   ```bash
   jvs recovery resume <recovery-plan>
   ```
5. If the operator decision is to return to the saved pre-restore state, run:
   ```bash
   jvs recovery rollback <recovery-plan>
   ```
6. Rerun `jvs status` and `jvs recovery status`.
7. Record the plan ID, final command, and outcome in the operations log.

Do not start another restore in that workspace while an active recovery plan
exists. Do not start any other mutation in that workspace while pending restore
preview or active recovery state exists. A pending preview closes through
`jvs restore --run <plan-id>`; active recovery uses `jvs recovery status`,
`resume`, or `rollback`. Active recovery plans protect referenced source save
points from cleanup until resolved.

After a successful `jvs restore --run`, `jvs recovery status` should have no
active recovery to resolve. Any retained completed plan record is JVS-owned
control data and should not be removed by platform scripts.

## Restore Drill

Run this drill for release qualification and after backup/migration changes.

1. Create or restore a test folder with at least two save points.
2. Preview a whole-workspace restore:
   ```bash
   jvs restore <older-save>
   ```
3. Confirm the output says preview only, no files changed, history will not
   change, and includes the restore run command for the selected workspace.
4. Run the plan:
   ```bash
   jvs restore --run <plan-id>
   ```
5. Confirm managed files match the source save point and newest save point did
   not move.
6. Preview and run a path restore:
   ```bash
   jvs restore <save> --path <path>
   jvs restore --run <plan-id>
   ```
7. Confirm `jvs status` records restored path provenance.
8. Simulate an interrupted restore in a controlled test environment or use a
   stored failure fixture; prove `jvs recovery status`, `resume`, and
   `rollback` close the active recovery loop. A normal pending preview is
   closed by `jvs restore --run <plan-id>`.

## Migration Runbook

Use the offline whole-folder copy procedure in
`docs/18_MIGRATION_AND_BACKUP.md` for the default control data location. Do not
use it to move control data into or out of a workspace.

1. Stop all JVS writers and stop agent jobs.
2. Confirm the source folder status is readable:
   ```bash
   jvs status
   ```
3. Ensure there are no active recovery plans:
   ```bash
   jvs recovery status
   ```
   Resolve any listed plan before copying.
4. Run `jvs cleanup preview --json` and wait until cleanup protection shows no
   open views, active recovery plans, or active operations. Do not reuse this
   preview after migration.
5. Run `jvs doctor --strict`.
6. Create final save points for critical workspaces.
7. Use ordinary filesystem copy of the managed folder/repository as a whole,
   while writers remain stopped. The fresh destination path must not exist; this
   example fails before copying if that path already exists. Do not overlay a
   non-empty destination:
   ```bash
   test ! -e /mnt/dst/myrepo &&
   mkdir -p /mnt/dst &&
   cp -a /mnt/src/myrepo /mnt/dst/myrepo
   ```
8. Run `jvs doctor --strict --repair-runtime` on the destination.
9. Run `jvs doctor --strict`, `jvs status`, and a fresh cleanup preview on the
   destination; record the results.
10. Run the restore drill above on the destination.

## Cleanup Runbook

Public product language is cleanup. Cleanup is a preview-first, reviewed
deletion flow for unprotected save point storage. This runbook records the
operational requirements; it does not define an alternate public command
surface.

1. Confirm doctor is healthy.
2. Confirm there are no active restore recovery plans unless the expected
   cleanup policy explicitly protects them:
   ```bash
   jvs recovery status
   ```
3. Create a cleanup preview through the approved release build surface.
4. Review protected save points, candidates, and estimated reclaim.
5. Execute only the reviewed cleanup plan.
6. Rerun doctor and record the result.
