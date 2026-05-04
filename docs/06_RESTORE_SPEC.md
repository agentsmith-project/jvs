# Restore Spec

**Status:** active save point public contract

`jvs restore` copies managed files from a save point into the active workspace.
Restore does not rewrite save point history. The default operation is a preview
plan; files change only after `jvs restore --run <plan-id>`.

## Command Forms

```bash
jvs restore <save>
jvs restore <save> --path <workspace-relative-path>
jvs restore --path <workspace-relative-path>
jvs restore --run <plan-id>
```

Optional safety flags for preview:

```bash
--save-first
--discard-unsaved
--json
```

`--save-first` and `--discard-unsaved` are mutually exclusive.

## Save Point Resolution

`<save>` must resolve to one concrete save point by full ID or unique ID
prefix. Labels, messages, and tags are not restore targets in the save point
contract.

Use discovery commands first when the ID is not known:

```bash
jvs history
jvs history --path src/config.json
jvs restore --path src/config.json
```

Discovery output lists candidates and next commands. It never changes files.

## Preview

A restore preview:

1. resolves the source save point
2. registers the restore preview as an active operation while reading the save
   point
3. refuses relevant unsaved changes unless `--save-first` or
   `--discard-unsaved` was selected
4. checks source integrity and write capacity
5. computes managed files to overwrite, delete, and create
6. records expected target state evidence
7. writes a plan and prints `jvs restore --run <plan-id>`

Preview output must say that no files were changed and history will not change.
For whole-workspace restore it records expected folder evidence. For path
restore it records expected path evidence.

## Run

`jvs restore --run <plan-id>` executes the stored plan. Runtime options are
fixed by the plan; changing source, path, `--save-first`, or
`--discard-unsaved` requires a new preview.

Run behavior:

1. load the restore plan
2. refuse if the workspace has an active recovery plan
3. register the restore run as an active operation while materializing the save
   point
4. revalidate the expected workspace/path evidence and newest save point
5. check source integrity and write capacity again
6. create a recovery plan and backup before replacing files
7. replace the whole workspace or selected path
8. update workspace file-source metadata
9. leave save point history unchanged
10. resolve the recovery plan after the final state is confirmed

After whole-workspace restore, managed files match the source save point and
the newest save point remains the same. The next `jvs save` creates a new save
point after the workspace's newest save point and records restored provenance.

After path restore, only the selected path is replaced. The next `jvs save`
records restored path provenance.

## Unsaved Changes

Restore refuses to overwrite unsaved managed files by default.

- `--save-first` creates a save point for unsaved changes before the run.
- `--discard-unsaved` discards unsaved changes for the restore scope.
- For path restore, the unsaved-change guard applies to the selected path.
- If JVS cannot prove the target state is safe, it treats the scope as having
  unsaved changes.

## Recovery

Restore creates a durable recovery plan before mutating files. If restore does
not finish safely, the error names the recovery plan and points to:

```bash
jvs recovery status <recovery-plan>
jvs recovery resume <recovery-plan>
jvs recovery rollback <recovery-plan>
```

Recovery requirements:

- An active recovery plan blocks new restore runs in the same workspace.
- `recovery status` shows source save point, folder, workspace, optional path,
  backup availability, last error, and recommended next command.
- `recovery resume` retries or confirms the restore and leaves history
  unchanged.
- `recovery rollback` restores the saved pre-restore state when evidence
  proves it is safe. When a retained recovery backup is required, its content
  evidence must still match the saved pre-restore evidence before any rollback
  copy is attempted.
- Source save points referenced by active recovery plans remain protected from
  cleanup until the plan is resolved.

## Safety Guarantees

- Preview does not change files.
- Run revalidates the target state before writing.
- Managed files are replaced only inside the workspace boundary.
- JVS control data and runtime state are not user files; restore leaves them
  alone.
- Save point history is not moved by restore.
- A failed or interrupted restore must either leave files unchanged or produce
  a recovery plan that closes the loop with resume or rollback.

## Examples

```bash
# Preview a whole-workspace restore.
jvs restore 1771589366482-abc12345

# Execute the reviewed plan.
jvs restore --run 6f2c1e3a-0c6d-41db-a111-3ac881a7a901

# Find candidates for a missing path.
jvs restore --path src/config.json

# Preview a single-path restore.
jvs restore 1771589366482-abc12345 --path src/config.json

# Preserve unsaved work before the run.
jvs restore 1771589366482-abc12345 --save-first
```

## Error Handling

| Error | Cause | Resolution |
| --- | --- | --- |
| Save point not found | Invalid ID or non-unique prefix | Use `jvs history` or `jvs history --path <path>` and choose one ID |
| Unsaved changes | Restore would overwrite unsaved managed files | Save first, or preview again with `--save-first` or `--discard-unsaved` |
| Target changed since preview | New save point or file evidence changed after preview | Run a new preview |
| Active recovery plan | A previous restore is unresolved | Use `jvs recovery status`, then resume or rollback |
| Recovery backup missing/unsafe or content evidence mismatch | Rollback cannot prove the saved state | Leave the recovery plan active; escalate with the recovery plan and preserved evidence |
