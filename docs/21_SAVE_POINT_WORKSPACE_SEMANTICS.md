# Save Point And Workspace Semantics

**Status:** Implemented design record for the current save point/workspace model; active clean redesign, non-release-facing, and not part of the v0 public contract. The release-facing source of truth remains the user docs and CLI spec.

This note tracks the clean redesign direction for contributors. The release
path is defined by the active user and contract docs:

- [User docs](user/README.md)
- [CLI spec](02_CLI_SPEC.md)
- [Product plan](PRODUCT_PLAN.md)
- [Restore spec](06_RESTORE_SPEC.md)
- [Cleanup spec](08_GC_SPEC.md)
- [Operation runbook](13_OPERATION_RUNBOOK.md)

## Public Model

JVS presents a small product model:

- A folder is the real directory where work happens.
- A workspace is a JVS-managed real folder.
- A save point is created from a workspace and then belongs to the project
  history graph.
- A workspace points at save points in that project history graph.
- History lists project save points through the active workspace's view of that
  graph.
- View opens a read-only view of a save point.
- Restore is preview-first and plan-bound.
- Recovery inspects, resumes, or rolls back interrupted restore work.
- Doctor checks health and performs narrow runtime repair.
- Cleanup deletes unprotected save point storage only after preview and review.

## Design Direction

- Public docs and help use save point vocabulary only.
- Do not reintroduce branch vocabulary: save points are project history nodes,
  not branches, and workspaces are named real folders that point at those nodes.
- Restore run must revalidate the preview plan before writing files.
- Workspace creation from a save point starts a new history and records source
  provenance.
- Unsaved changes are refused before destructive operations unless the caller
  explicitly chooses a safety option.
- Cleanup protects live workspace needs, active views, active source reads,
  active operations, and recovery plans.
- Internal storage names and package names are code facts, not user workflow
  concepts.

## Release Boundary

This file is a design coordination note. It must not be used as the public user
entry point, command reference, or release evidence ledger.
