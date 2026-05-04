# GA User Story Matrix

**Status:** active GA user story matrix

This matrix starts from user intent, not implementation vocabulary. Users think
in folders, workspace names, save points, history, read-only views, restore,
recovery, and cleanup. They do not need to learn storage mechanics from other
tools, and older implementation nouns are not user concepts.

GA commitments are generic filesystem capabilities. Domain-specific examples
can illustrate the same commands, but they do not add product commitments,
special save semantics, or GA presets. If a future workflow needs behavior that
does not naturally fit the public save point model, record it in Product Design
Improvement Candidates instead of changing story language to match an
implementation detail.

## GA-US-01: Managed Folder Save And Restore

Persona: user working in a local project folder with files that matter.

Goal: save a known-good folder state, try a risky edit, inspect history, and
restore the known-good state only after reviewing a plan.

Workflow:

```bash
jvs init .
jvs save -m "managed report baseline"
jvs save -m "managed report update"
jvs history --grep "managed report"
jvs view <baseline-save> managed/report.txt
jvs restore <baseline-save>
jvs restore --run <plan-id>
```

Expected behavior: the user can identify the relevant save point, inspect a
saved file read-only, preview the folder restore, and run the reviewed plan
without rewriting history.

Acceptance criteria:

- `jvs save` creates distinct save points for managed folder states.
- `jvs history --grep` returns matching save points without changing files.
- `jvs view` opens saved content read-only and leaves workspace history
  unchanged.
- `jvs restore <save>` creates a preview plan and changes no files.
- `jvs restore --run <plan-id>` revalidates the previewed state before writing.
- Whole-workspace restore leaves save point history unchanged.

Current coverage:

- Story-local and story-json restore flows cover preview-first restore,
  read-only view, status after restore, and unchanged history.
- Restore preview/run conformance covers the public plan contract.

## GA-US-02: Managed Path Discovery And Repair

Persona: user who remembers one file or directory path and wants to recover
only that path.

Goal: find save points that contain the path, inspect the saved path, and
restore that path without clobbering unrelated current work.

Workflow:

```bash
jvs history --path managed/report.txt
jvs view <save> managed/report.txt
jvs restore <save> --path managed/report.txt
jvs restore --run <plan-id>
jvs save -m "recover managed report"
```

Expected behavior: discovery starts from the workspace-relative path. Restore
changes only the selected managed path, while unrelated files such as
`managed/notes.txt` or `cache/tmp.bin` keep their current contents.

Acceptance criteria:

- `jvs history --path <path>` returns candidates and next commands without
  mutating the folder.
- `jvs view <save> <path>` lets the user inspect the saved path read-only.
- Path restore preview changes no files and reports the exact path.
- Path restore run changes only the requested managed path.
- Unrelated paths outside the requested path, such as `managed/notes.txt` or
  `cache/tmp.bin`, are not restored or deleted by a path restore.
- A later save records the recovered folder state as a normal save point.

Current coverage:

- Path history, view, preview, run, and restored path source conformance.
- `TestBoundaryJSON_PathRestoreKeepsCacheLikeUnrelatedPath` covers a generic
  path restore with `managed/report.txt` and unrelated `cache/tmp.bin`.

## GA-US-03: Workspace From Save Point

Persona: user who wants an isolated real folder that starts from a saved state.

Goal: create a separate workspace folder from a save point and work inside that
folder directly.

Workflow:

```bash
jvs workspace new ../generic-copy --from <save>
cd "$(jvs workspace path generic-copy)"
jvs status
jvs save -m "generic copy first save"
```

Expected behavior: the new workspace is a real folder. Commands run inside the
printed folder target that workspace, while the source workspace and source
history stay unchanged.

Acceptance criteria:

- `jvs workspace new <folder> --from <save>` creates a separate real folder at
  the explicit target path.
- The source workspace is unchanged.
- The printed folder carries only the control information needed for command
  targeting; that information is not workspace content.
- The new workspace starts with no newest save point until first save.
- Status and JSON record `started_from_save_point`.
- First save in the new workspace starts its own history and records
  provenance.

Current coverage:

- Workspace creation conformance covers distinct folders, source preservation,
  entered-folder targeting, first-save history, and `started_from_save_point`.
- Boundary conformance covers workspace targeting state as JVS control data,
  not workspace content.

## GA-US-04: Managed Content Boundary

Persona: user who expects JVS to manage only their files, not JVS control data.

Goal: save, view, and restore managed content while JVS control data and
runtime state remain outside managed workspace content.

Workflow:

```bash
jvs save -m "managed report"
jvs view <save>
jvs restore <save> --path managed/report.txt
jvs restore --run <plan-id>
jvs doctor --strict
```

Expected behavior: managed paths such as `managed/report.txt` can be saved,
viewed, and restored. JVS control data and runtime state for workspace
targeting, active operations, restore plans, recovery plans, and cleanup plans
are never saved, viewed, restored, deleted, or recreated as user files.

Acceptance criteria:

- Save captures managed files and excludes JVS control data.
- View materializes only saved content and rejects attempts to view JVS control
  data as a path inside a save point.
- Restore rejects JVS control paths as restore targets.
- Whole-workspace restore does not delete or recreate JVS control data or
  runtime state as user files.
- Public JSON and human output keep the boundary understandable without
  exposing storage mechanics as user workflow concepts.
- `jvs doctor --strict` validates repository health through the public health
  path.

Current coverage:

- Managed-file boundary, migration/runtime-state boundary, and doctor layout checks.
- `TestBoundaryJSON_UserPayloadExcludesJVSControlData` covers managed-file
  purity for JVS control data, restore plans, recovery plans, cleanup plans,
  and active operation state.

## GA-US-05: Read-Only Views For Files And Directories

Persona: user who needs to inspect saved content without changing the real
folder.

Goal: open a read-only view of a save point, a file, or a directory, including
large managed content.

Workflow:

```bash
jvs view <save> large/blob.bin
jvs view <save> large
jvs view close <view-id>
```

Expected behavior: view paths are read-only. Opening or closing a view does not
change the real folder, workspace metadata, or history.

Acceptance criteria:

- `jvs view <save>` opens a read-only view of the saved content.
- `jvs view <save> <path>` opens a read-only view of a file or directory path.
- Large files and directories use the same read-only contract as small files.
- Active views protect their source save point from cleanup while open.
- `jvs view close <view-id>` removes the view and leaves workspace state
  unchanged.

Current coverage:

- View conformance covers file and path views, close behavior, and cleanup
  protection.
- `TestViewJSON_LargeFileAndDirectoryViewsAreReadOnly` covers generic
  `large/blob.bin` and `large` directory views.

## GA-US-06: Mistaken Deletion Recovery

Persona: user who deleted a managed file or directory while continuing other
work.

Goal: recover only the missing path from an earlier save point without losing
unrelated current changes.

Workflow:

```bash
jvs history --path managed/report.txt
jvs view <save> managed/report.txt
jvs restore <save> --path managed/report.txt
jvs restore --run <plan-id>
```

Expected behavior: the user can find the save point containing the missing
path, inspect it, and restore just that path through a reviewed plan.

Acceptance criteria:

- Missing-path discovery does not mutate the folder.
- Read-only view shows the saved content.
- Path restore preview reports the selected path and impact.
- Path restore run recreates only the selected path.
- Unrelated current work remains unchanged.

Current coverage:

- Story-json deletion recovery covers deleted-path discovery, read-only view,
  path restore preview/run, unrelated work preservation, path source status,
  and a follow-up save point.

## GA-US-07: Interrupted Restore Recovery

Persona: user whose restore was interrupted by a process failure, machine
restart, or storage error.

Goal: understand the restore state and either complete it or roll it back
without guessing which files were changed.

Workflow:

```bash
jvs recovery status
jvs recovery status <recovery-plan>
jvs recovery resume <recovery-plan>
jvs recovery rollback <recovery-plan>
```

Expected behavior: recovery is an explicit public workflow. JVS shows the
available action from the current recovery state, blocks conflicting restore
runs while recovery is active, and closes the recovery plan after a safe resume
or rollback.

Acceptance criteria:

- Restore run creates a recovery plan before mutating files.
- `jvs recovery status` lists active recovery plans.
- `jvs recovery status <plan>` shows backup availability and derives the
  recommended next command from the live plan state instead of replaying a
  stored command.
- If backup or evidence is insufficient for a safe automated next step,
  recovery status omits the recommendation instead of suggesting a command
  that would fail.
- `jvs recovery resume <plan>` can complete or confirm the restore.
- `jvs recovery rollback <plan>` returns to the saved pre-restore state when
  evidence proves it is safe.
- Active recovery plans protect referenced save points from cleanup.

Current coverage:

- Recovery conformance covers interrupted restore status, live-derived
  recommended next commands, resume, rollback, active-plan blocking, and
  cleanup protection.

## GA-US-08: Cleanup Review Before Deleting Old Data

Persona: user reclaiming disk space after save points accumulate.

Goal: see what cleanup would remove, understand what is protected, and delete
only after reviewing a bound cleanup plan.

Workflow:

```bash
jvs cleanup preview
jvs cleanup preview --json
jvs cleanup run --plan-id <plan-id>
jvs doctor --strict
```

Expected behavior: cleanup is preview-first. It only plans deletion of
unprotected save point storage, and it protects workspace history, open views,
active recovery plans, and active operations.

Acceptance criteria:

- Cleanup preview does not delete anything.
- Preview explains protected and reclaimable save point storage in public
  terms.
- Preview groups protected save points by reason, including history, open
  read-only views, active recovery plans, and active operations.
- Cleanup run binds to a reviewed plan and revalidates before deletion.
- Active views and recovery plans protect their referenced save points.
- Cleanup does not rewrite durable history, workspace provenance, or audit
  history.
- Cleanup does not delete workspace folders, user cache directories, JVS
  control data, or runtime state; it does not prune history or apply a
  retention policy.

Current coverage:

- Cleanup preview/run conformance covers plan binding, protection groups,
  active recovery protection, and JSON evidence.

## Product Design Improvement Candidates

These candidates are not GA acceptance criteria unless promoted into the
public contract.

- Guided selection could help users pick a save point or path candidate
  without turning messages, labels, or tags into restore targets.
- Future managed-file boundary features are outside GA; GA does not provide
  configurable file selection.
- Domain-specific presets, templates, or workflow bundles are explicitly out
  of the GA plan. They can be reconsidered only if they compile to the generic
  capabilities above and do not change save, view, restore, recovery, or
  cleanup semantics.

## Safety Principles

- Commands that replace or delete files surface a preview plan or explicit
  safety choice.
- `jvs view` is read-only.
- `jvs history --path` is the discovery path for one file or directory.
- `jvs workspace new <folder> --from <save>` creates a separate real folder.
- Workspace removal must preview first, run only a reviewed plan, and leave
  save point storage deletion to cleanup.
- JVS control data and runtime state are never workspace content.
- `jvs recovery status`, `jvs recovery resume`, and `jvs recovery rollback`
  are the public path for interrupted restore.
- `jvs cleanup preview` and `jvs cleanup run --plan-id <plan-id>` keep cleanup
  review-first.
