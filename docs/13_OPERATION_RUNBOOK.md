# Operation Runbook

**Status:** active save point operations runbook

## Daily Checks

1. Run `jvs doctor --strict`.
2. Review active recovery plans:
   ```bash
   jvs recovery status
   ```

## Incident: Doctor Failure

1. Freeze writers for the affected folder/repo.
2. Preserve command output with `--json` where available.
3. Classify the failure: layout, publish state, audit chain, descriptor
   checksum, payload hash, runtime state, or recovery plan.
4. Do not run destructive cleanup until the failure class is known.
5. Escalate tamper or audit-chain failures and preserve `.jvs/audit/` and the
   affected save point descriptors as evidence.

## Incident: Runtime Artifacts

Use this only for runtime state. It does not rewrite durable save point
history.

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
exists. Active recovery plans protect referenced source save points from
cleanup until resolved.

## Restore Drill

Run this drill for release qualification and after backup/migration changes.

1. Create or restore a test folder with at least two save points.
2. Preview a whole-workspace restore:
   ```bash
   jvs restore <older-save>
   ```
3. Confirm the output says preview only, no files changed, history will not
   change, and includes `Run: jvs restore --run <plan-id>`.
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
   `rollback` close the loop.

## Migration Runbook

Use the offline whole-folder copy procedure in
`docs/18_MIGRATION_AND_BACKUP.md`.

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
