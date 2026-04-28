# Conformance Test Plan

**Status:** active save point contract coverage plan

This plan defines release-blocking public contract coverage for the save point
UX.

## Mandatory Contract Areas

### CLI Help And Vocabulary

- Root help shows the save point path: `init`, `save`, `history`, `view`,
  `restore`, `workspace new`, `status`, `recovery`, and `doctor`.
- Public help and examples use folder, workspace, save point, save, history,
  view, restore, unsaved changes, recovery plan, and cleanup.
- Internal storage names do not appear as public commands, selectors, workflow
  concepts, or examples.

### JSON Envelope

- Every `--json` command emits one envelope with `schema_version`, `command`,
  `ok`, `repo_root`, `workspace`, `data`, and `error`.
- Error envelopes set `ok: false`, `data: null`, and include stable error
  fields.

### Setup And Status

- `jvs init [folder]` adopts the real folder without moving user files.
- Initial status shows `Newest save point: none` and unsaved changes.
- `jvs status --json` exposes `folder`, `workspace`, `newest_save_point`,
  `history_head`, `content_source`, `started_from_save_point` when applicable,
  `unsaved_changes`, `files_state`, and restored path sources when present.

### Save And History

- `jvs save -m <message>` creates a save point from managed files.
- Failed capacity/staging checks do not publish partial save points.
- `jvs history` lists workspace save points.
- `jvs history --path <path>` returns candidates and next commands without
  changing files.
- Messages, labels, and tags are discovery metadata, not restore/view targets.

### View

- `jvs view <save> [path]` opens a read-only view.
- View does not change the real folder, workspace metadata, or history.
- Active views protect their source save point from cleanup.

### Restore Preview And Run

- `jvs restore <save>` creates a preview plan and changes no files.
- `jvs restore <save> --path <path>` previews path restore and changes no
  files.
- `jvs restore --path <path>` lists candidates only.
- Preview output includes plan ID, impact counts, expected target evidence,
  and run command.
- `jvs restore --run <plan-id>` reloads the plan, revalidates target evidence,
  and writes files only after validation.
- Whole-workspace and path restore leave history unchanged.
- `--save-first` and `--discard-unsaved` are mutually exclusive.

### Restore Recovery

- Restore run creates a recovery plan before mutating files.
- Interrupted restore exposes `jvs recovery status <plan>`.
- `jvs recovery resume <plan>` can complete or confirm restore.
- `jvs recovery rollback <plan>` can return to saved pre-restore state when
  evidence proves it is safe.
- Active recovery plans block another restore in the same workspace and protect
  referenced save points from cleanup.

### Workspace Creation

- `jvs workspace new <name> --from <save>` creates a real workspace folder.
- The source workspace is unchanged.
- The new workspace has no newest save point until first save.
- Status and JSON record `started_from_save_point`.
- First save in the new workspace starts a new history and records provenance.

### Doctor And Runtime Repair

- `jvs doctor --strict` validates repository health.
- `jvs doctor --repair-list` lists only public runtime repair IDs:
  `clean_locks`, `clean_runtime_tmp`, and `clean_runtime_operations`.
- `jvs doctor --strict --repair-runtime` does not rewrite durable save point
  history, workspace provenance, or audit history.

### Cleanup Layering

- Public docs describe cleanup preview/run semantics.
- Cleanup must protect live workspace needs, active views, active source
  operations, and active recovery plans.

## Release Gate Expectations

- Public command smoke tests match visible help.
- Restore preview/run/recovery tests cover whole-workspace and path flows.
- Workspace new tests cover `started_from_save_point`.
- Migration tests exclude runtime state and run the restore drill.
- Performance and benchmark docs scope engine claims and label internal
  package names as implementation facts only.
