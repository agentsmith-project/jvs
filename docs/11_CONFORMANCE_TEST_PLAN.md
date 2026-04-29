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
- A save that cannot finish must not appear as a new save point in history.
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
- Preview output includes plan ID, the files that would change, the current
  folder or path state that will be checked again, and the run command.
- `jvs restore --run <plan-id>` reloads the plan, checks that the folder or
  path still matches the previewed state, and writes files only after that
  check passes.
- Whole-workspace and path restore leave history unchanged.
- `--save-first` and `--discard-unsaved` are mutually exclusive.

### Restore Recovery

- Restore run creates a recovery plan before mutating files.
- Interrupted restore exposes `jvs recovery status <plan>`.
- Recovery status derives the public recommended next command from live
  plan/evidence/backup state and omits it when the available evidence would
  make resume or rollback unsafe.
- `jvs recovery resume <plan>` can complete or confirm restore.
- `jvs recovery rollback <plan>` can return to saved pre-restore state when
  evidence proves it is safe.
- Active recovery plans block another restore in the same workspace and protect
  referenced save points from cleanup.

### Workspace Creation

- `jvs workspace new <name> --from <save>` creates a real workspace folder.
- The source workspace is unchanged.
- After entering the printed workspace folder, `jvs status` and `jvs save`
  target that workspace directly without requiring `--workspace <name>`.
- The new workspace has no newest save point until first save.
- Status and JSON record `started_from_save_point`.
- First save in the new workspace starts a new history and records provenance.

### Doctor And Runtime Repair

- `jvs doctor --strict` validates repository health.
- `jvs doctor --repair-list` lists only public runtime repair IDs:
  `clean_locks`, `rebind_workspace_paths`, `clean_runtime_tmp`,
  `clean_runtime_operations`, and `clean_runtime_cleanup_plans`.
- `jvs doctor --strict --repair-runtime` does not rewrite durable save point
  history, workspace provenance, or audit history.
- Physical-copy migration tests cover destination-local workspace path rebinding
  while source paths are both offline and still mounted, including external
  workspace siblings that are safely rebound, missing, or content-mismatched.

### Cleanup Layering

- Public docs describe cleanup preview/run semantics.
- Cleanup must protect workspace history, open views, active recovery plans,
  and active operations.
- Cleanup preview JSON exposes `plan_id`, `created_at`,
  `protected_save_points`, `protection_groups`, `protected_by_history`,
  `candidate_count`, `reclaimable_save_points`, and
  `reclaimable_bytes_estimate`.
- Cleanup preview exposes protection groups by stable public reason and keeps
  protection group save points in the same public save point ID type as other
  cleanup fields.

### User Story Matrix Coverage

- `docs/20_USER_SCENARIOS.md` is the GA user story matrix and must stay
  user-mental-model first.
- Story coverage proves persona goals and workflows, not implementation
  storage details, domain presets, or another tool's mental model.
- `make story-local` covers the current human CLI story slices.
- `make story-json` covers the current JSON story slices and public fields
  that automation depends on.
- `make story-e2e` combines those current story gates. It is a first-batch
  story gate for generic save point workflows, not a claim that every example
  in the matrix is automated yet.
- `make story-juicefs-local` qualifies the covered story slices on a real
  local JuiceFS profile when that release profile is required.
- If a story is not naturally supported by the public save point model, the
  candidate belongs in Product Design Improvement Candidates in
  `docs/20_USER_SCENARIOS.md`.
- Domain-specific presets, templates, and workflow bundles are outside the GA
  plan unless they are explicitly promoted into the public contract.

## Release Gate Expectations

- Public command smoke tests match visible help.
- Restore preview/run/recovery tests cover whole-workspace and path flows.
- Recovery status tests cover live-derived next-command recommendations,
  including backup-unavailable cases that must not recommend stale resume
  commands.
- Workspace new tests cover `started_from_save_point`.
- Boundary tests cover managed payload purity: JVS control data and runtime
  state for workspace targeting, active operations, restore plans, recovery
  plans, and cleanup plans are not user payload.
- Path restore tests cover unrelated cache-like user files staying untouched
  when another managed path is restored.
- View tests cover read-only behavior for files and directories, including
  large managed paths.
- Cleanup tests cover protection groups for history, open views, active
  recovery, and active operations.
- Migration tests cover offline whole-folder copy to a fresh destination,
  destination `jvs doctor --strict --repair-runtime`, a fresh cleanup preview,
  and the restore drill.
- Performance and benchmark docs scope engine claims and label internal
  package names as implementation facts only.
- User story tests must match the current coverage stated in
  `docs/20_USER_SCENARIOS.md`. Future ideas remain Product Design Improvement
  Candidates until they are promoted into the public contract, with
  `make story-juicefs-local` added when the real local JuiceFS profile is
  release-blocking.
