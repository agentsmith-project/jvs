# JVS Save Point Implementation Plan

**Status:** Archived path, non-release-facing, and not part of the v0 public contract.

This dated path is kept only so repository links and docs classification gates
continue to resolve. The long task list that previously lived here has been
removed from active reader content because the product has not reached GA and
the active plan is the save point/workspace/recovery plan below.

## Goal

Ship the v0 save point contract with a small, coherent public surface:

- real folder adoption
- save point creation
- history and path discovery
- read-only view
- restore preview/run/recovery
- workspace creation from a save point
- strict doctor health checks
- reviewed cleanup semantics
- release evidence that matches the visible product contract

## Workstream 1: Public Command Contract

Source docs:

- `docs/02_CLI_SPEC.md`
- `docs/PRODUCT_PLAN.md`

Tasks:

- Keep root help aligned to the visible save point command set.
- Keep public examples on `init`, `save`, `history`, `view`, `restore`,
  `workspace new`, `recovery`, `status`, and `doctor`.
- Keep JSON output inside the documented envelope.
- Keep unsupported future surfaces out of examples and release notes.

## Workstream 2: Save Point Core

Source docs:

- `docs/PRODUCT_PLAN.md`
- `docs/ARCHITECTURE.md`

Tasks:

- Ensure save captures managed files only.
- Ensure control data is never payload.
- Publish save points atomically.
- Keep engine performance claims scoped to engine and filesystem support.
- Preserve provenance from workspace creation and restore.

## Workstream 3: Restore And Recovery

Source docs:

- `docs/06_RESTORE_SPEC.md`
- `docs/13_OPERATION_RUNBOOK.md`

Tasks:

- Make restore preview the default operation.
- Bind restore run to a reviewed plan.
- Revalidate target evidence before writes.
- Leave save point history unchanged by restore.
- Create recovery plans before destructive writes.
- Cover status, resume, and rollback drills.

## Workstream 4: Workspace Creation

Source docs:

- `docs/02_CLI_SPEC.md`
- `docs/PRODUCT_PLAN.md`

Tasks:

- Support `jvs workspace new <name> --from <save>`.
- Leave the source workspace unchanged.
- Start the new workspace with no newest save point.
- Record `started_from_save_point` for the first save.

## Workstream 5: Operations, Migration, And Release

Source docs:

- `docs/12_RELEASE_POLICY.md`
- `docs/13_OPERATION_RUNBOOK.md`
- `docs/18_MIGRATION_AND_BACKUP.md`
- `docs/99_CHANGELOG.md`
- `docs/RELEASE_EVIDENCE.md`

Tasks:

- Require `make docs-contract` and `make release-gate` before final release.
- Record strict doctor evidence on representative repositories.
- Run restore and recovery drills after migration.
- Exclude non-portable runtime state from physical syncs.
- Keep changelog and release evidence aligned to the active save point
  contract.

## Done Criteria

- Active docs use the save point vocabulary.
- Release-facing examples use only visible commands.
- Restore preview/run/recovery behavior is documented and tested.
- Workspace creation semantics are documented and tested.
- Cleanup is documented as preview-first reviewed deletion.
- `make docs-contract` passes.
