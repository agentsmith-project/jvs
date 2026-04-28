# GA User Story Matrix

**Status:** active GA user story matrix

This matrix starts from user intent, not implementation vocabulary. Users think
in folders, workspace names, save points, history, read-only views, restore,
recovery, and cleanup. They do not need to learn storage mechanics from other
tools, and older implementation nouns are not user concepts.

If JVS cannot naturally satisfy a story through the public save point model,
record that gap in Product Design Improvement Candidates instead of changing
the story language to match an implementation detail.

## GA-US-01: ML Experiment Baseline And Result

Persona: ML engineer tuning a model in a local experiment folder.

Goal: keep a completed baseline result, save the risky result that produced a
new metric, and reset the folder before the next run.

Workflow:

```bash
jvs init .
python train.py --config configs/baseline.yaml --output outputs/run-42
jvs save -m "baseline result run-42"
python train.py --config configs/risky.yaml --output outputs/run-42
jvs save -m "risky result run-42"
jvs history --grep "run-42"
jvs view <baseline-save> outputs/run-42/metrics.json
jvs restore <baseline-save>
jvs restore --run <plan-id>
```

Expected behavior: the engineer can identify the completed baseline result and
risky result save points, inspect baseline metrics without changing the folder,
and restore the baseline through a preview/run pair before another run.

Acceptance criteria:

- `jvs save` records completed baseline and risky result folder states with
  distinct messages.
- `jvs history --grep` returns the experiment save points without changing
  files.
- `jvs view` opens the selected metrics artifact read-only and leaves workspace
  history unchanged.
- `jvs restore <save>` previews the reset and changes no files.
- `jvs restore --run <plan-id>` restores only after the folder still matches
  the previewed state and keeps history unchanged.

Proposed test coverage:

- Current: `TestStoryJSON_MLExperimentBaselineRunRestorePreviewFirst` covers
  completed baseline and risky `run-42` save messages,
  `history --grep "run-42"`, read-only baseline metrics view, preview, restore
  run, status, and unchanged history.
- Current: `TestStoryLocal_RestorePreviewFirstKeepsFilesAndHistoryUntilRun`

## GA-US-02: Data And ETL Stage Retry

Persona: data engineer responsible for a local or mounted ETL workspace.

Goal: save clear stage boundaries and retry a failed stage from the last known
good folder state.

Workflow:

```bash
jvs init .
python ingest.py --date "$RUN_DATE" --output raw/
jvs save -m "raw ingestion $RUN_ID"
python transform.py --input raw/ --output processed/
jvs save -m "processed data $RUN_ID"
python build_features.py --input processed/ --output features/
jvs restore <processed-save> --discard-unsaved
jvs restore --run <plan-id>
python build_features.py --input processed/ --output features/
jvs save -m "features retry $RUN_ID"
```

Expected behavior: the engineer can retry from the stage save point they trust
without publishing a partial failed stage or rewriting previous history.

Acceptance criteria:

- Stage saves are discoverable by message and save point ID.
- A failed stage that has not been saved does not appear in history.
- If a save attempt cannot finish, no partial save point should appear in
  history.
- Retry restore previews the chosen stage and uses `--discard-unsaved`, or an
  equivalent explicit safety choice, before replacing partial failed-stage
  files.
- Restore preview reports the files that would change and the run command.
- Restore run returns the folder to the chosen stage and leaves history
  unchanged.
- After restore, the retry reruns `build_features.py` and saves the successful
  retry output.
- Status JSON explains the current folder source after a restore.

Proposed test coverage:

- Current: `TestStoryJSON_DataETLStageRetryRestoresWholeFolderWithSafetyChoice`
  covers whole-folder stage restore, `--discard-unsaved`, restored stage files,
  status `content_source`, clean status, unchanged history before retry, and
  saved retry output.
- Current: `TestStoryJSON_DataETLPathRecoveryRestoresOnlyTargetPath`
- Planned/internal risk: black-box coverage for a failed save attempt not
  publishing a partial save point. The current story test covers an unsaved
  failed stage, not an injected mid-save failure.

## GA-US-03: Agent Sandbox Clean Runs

Persona: agent infrastructure engineer running repeated local agent tasks.

Goal: reset one folder before a run or create separate real folders for
parallel agent runs.

Workflow:

```bash
jvs init .
cp -r /baseline/agent-env/* .
BASELINE=$(jvs save -m "agent baseline" --json | jq -r '.data.save_point_id')
jvs workspace new run-a --from "$BASELINE"
jvs workspace new run-b --from "$BASELINE"
cd <printed-folder>
jvs status
python agent.py --seed 1 --output results.json
jvs save -m "agent run-a result"
```

Expected behavior: each agent run starts from an explicit save point in a real
folder, the printed folder can be entered and used directly, and result capture
is independent per workspace. The printed folder carries enough JVS project
information for commands run inside it to find the owning project and
workspace; users do not manage that information as payload.

Acceptance criteria:

- `jvs workspace new <name> --from <save>` creates a separate real folder and
  leaves the source workspace unchanged.
- After entering the printed folder, `jvs status` and `jvs save` target that
  workspace directly without requiring the user to return to the source folder
  or pass `--workspace <name>`.
- The printed folder includes JVS project information for command targeting,
  while managed user files remain the saved payload.
- The new workspace starts with no newest save point until the first save.
- Status and JSON record `started_from_save_point`.
- First save in the new workspace starts its own history and records
  provenance.
- Automation can read save point IDs and status through the public JSON
  envelope.

Proposed test coverage:

- Current: `TestStoryJSON_AgentSandboxWorkspaceIsolation`
- Current: `TestStoryJSON_TargetingMainFromNonTargetCWD`
- Planned: local parallel agent run coverage.

## GA-US-04: Game Asset Variant And Recovery

Persona: technical artist or game developer editing large binary assets.

Goal: save a stable asset state, try risky edits, inspect an earlier asset, and
recover one asset path or the whole folder after a bad edit.

Workflow:

```bash
jvs init .
jvs save -m "before character model work"
# Work in Unity or Unreal.
jvs save -m "hero armor pass"
jvs history --path Assets/Characters/Hero
jvs view <save> Assets/Characters/Hero
jvs view close <view-id>
jvs restore <save> --path Assets/Characters/Hero
jvs restore --run <plan-id>
```

Expected behavior: the artist works with save points and paths, not manual
file-by-file repair. Inspecting an earlier asset is read-only, and path restore
changes only the selected asset path after preview.

Acceptance criteria:

- `jvs history --path` returns candidate save points for an asset path without
  changing files.
- `jvs view <save> <path>` opens an earlier asset read-only.
- Path restore preview reports the selected path and impact.
- Path restore run replaces only that managed path and keeps history
  unchanged.
- Active views protect their source save point from cleanup while open.

Proposed test coverage:

- Current shared path-restore coverage:
  `TestStoryJSON_DataETLPathRecoveryRestoresOnlyTargetPath`
- Current shared read-only view coverage:
  `TestStoryJSON_MistakenDeletionRecoveryRestoresOnlyDeletedPath`
- Planned: game asset read-only view and active-view cleanup protection
  coverage.

## GA-US-05: Developer Generated State Boundary

Persona: application developer whose toolchain creates generated files,
reports, and local caches.

Goal: save generated state that matters to the project while keeping local
control data and excluded cache folders outside the user payload.

Workflow:

```bash
jvs init .
npm run generate
jvs save -m "generated api client"
jvs history --path generated/client.ts
jvs restore <save> --path generated/client.ts
jvs restore --run <plan-id>
jvs doctor --strict
```

Expected behavior: the developer can save and recover generated artifacts that
belong in the managed folder, while JVS control data and ignored or unmanaged
files are not restored or deleted as user payload.

Acceptance criteria:

- Save captures managed generated files and excludes JVS control data.
- History path discovery can find a generated artifact by workspace-relative
  path.
- Path restore replaces only the selected generated artifact.
- Ignored or unmanaged cache files are not restored or deleted by restore.
- `jvs doctor --strict` validates repository health through the public health
  path.

Proposed test coverage:

- Planned: developer generated artifact recovery coverage.
- Planned: generated state and JVS project information boundary coverage.

## GA-US-06: Mistaken Content Deletion Recovery

Persona: content author, designer, or developer who deleted a file or folder
while continuing other work.

Goal: find the earlier save point containing the missing content and recover
only that path without clobbering unrelated current work.

Workflow:

```bash
jvs history --path content/chapter-03.md
jvs view <save> content/chapter-03.md
jvs restore <save> --path content/chapter-03.md
jvs restore --run <plan-id>
jvs save -m "recover chapter 03"
```

Expected behavior: discovery starts from the path the user remembers. The user
can inspect the old content and restore just that path through a reviewed plan.

Acceptance criteria:

- `jvs history --path <path>` returns candidates and next commands without
  mutating the folder.
- `jvs view <save> <path>` lets the user inspect the missing content read-only.
- Path restore preview changes no files and reports the exact path.
- Restore run changes only the requested path.
- A later save records the recovered folder state as a normal save point.

Proposed test coverage:

- Current: `TestStoryJSON_MistakenDeletionRecoveryRestoresOnlyDeletedPath`
  covers a real deleted content file, `history --path`, read-only view, path
  restore preview/run, unchanged unrelated work, path source status, and a
  follow-up save point.

## GA-US-07: Interrupted Restore Recovery

Persona: developer or operator whose restore was interrupted by a process
failure, machine restart, or storage error.

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
available action, blocks conflicting restore runs while recovery is active, and
closes the recovery plan after a safe resume or rollback.

Acceptance criteria:

- Restore run creates a recovery plan before mutating files.
- `jvs recovery status` lists active recovery plans.
- `jvs recovery resume <plan>` can complete or confirm the restore.
- `jvs recovery rollback <plan>` returns to the saved pre-restore state when
  evidence proves it is safe.
- Active recovery plans protect referenced save points from cleanup.

Proposed test coverage:

- Planned: interrupted restore status, resume, rollback, and cleanup
  protection coverage.

## GA-US-08: Cleanup Review Before Deleting Old Data

Persona: developer or team lead reclaiming disk space after experiments,
asset work, or ETL retries.

Goal: see what cleanup would remove, understand what is protected, and delete
only after reviewing a bound cleanup plan.

Workflow:

```bash
jvs cleanup preview
jvs cleanup preview --json
jvs cleanup run --plan-id <plan-id>
jvs doctor --strict
```

Expected behavior: cleanup is preview-first. It protects live workspace needs,
active views, active source operations, active recovery plans, and save points
that must remain available for current workflows.

Acceptance criteria:

- Cleanup preview does not delete anything.
- Preview explains protected and reclaimable save point storage in public
  terms.
- Cleanup run binds to a reviewed plan and revalidates before deletion.
- Active views and recovery plans protect their referenced save points.
- Cleanup does not rewrite durable history, workspace provenance, or audit
  history.

Proposed test coverage:

- Planned: cleanup preview/run protection coverage.
- Planned: cleanup JSON plan evidence coverage.

## Product Design Improvement Candidates

These candidates are not GA acceptance criteria unless promoted into the
public contract.

- Guided story flows could help users pick a baseline, result, or recovery
  candidate without turning messages, labels, or tags into restore targets.
- Domain presets could make it easier to separate generated artifacts that
  should be saved from cache folders that should stay unmanaged.
- Cleanup preview could group protection reasons by user workflow, such as
  open read-only views, active recovery plans, and current workspace needs.
- Story-level metadata for external run IDs, dataset dates, or build IDs could
  improve discovery while remaining metadata only.
- Recovery status could provide clearer next-command recommendations for
  interrupted automation logs.

## Safety Principles

- Commands that replace or delete files surface a preview plan or explicit
  safety choice.
- `jvs view` is read-only.
- `jvs history --path` is the discovery path for one file or directory.
- `jvs workspace new <name> --from <save>` creates a separate real folder.
- `jvs recovery status`, `jvs recovery resume`, and `jvs recovery rollback`
  are the public path for interrupted restore.
- `jvs cleanup preview` and `jvs cleanup run --plan-id <plan-id>` keep cleanup
  review-first.
