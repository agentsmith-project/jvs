# JVS Save Point Implementation Design

**Status:** Archived path, non-release-facing, and not part of the v0 public contract.

This dated path is kept only so repository links and docs classification gates
continue to resolve. The long-form draft that previously lived here has been
removed from active reader content because the product has not reached GA and
the active contract is now the save point contract.

## Active Design Entry Points

- `docs/PRODUCT_PLAN.md` defines the product promise, vocabulary, boundaries,
  release phases, and acceptance gates.
- `docs/ARCHITECTURE.md` defines the active architecture and package
  responsibilities.
- `docs/02_CLI_SPEC.md` defines the visible command surface and JSON envelope.
- `docs/06_RESTORE_SPEC.md` defines restore preview, run, and recovery.
- `docs/13_OPERATION_RUNBOOK.md` defines operational drills.
- `docs/18_MIGRATION_AND_BACKUP.md` defines backup and migration boundaries.

## Product Model

JVS manages real folders as workspaces. Users create save points, inspect
history, open read-only views, restore reviewed plans, and resolve interrupted
restore operations through recovery plans. Cleanup is reviewed deletion of
unprotected save point storage.

Visible product commands are:

```text
jvs init
jvs save -m "message"
jvs history
jvs view <save> [path]
jvs restore <save>
jvs restore --run <plan-id>
jvs recovery status [plan]
jvs recovery resume <plan>
jvs recovery rollback <plan>
jvs workspace new <name> --from <save>
jvs status
jvs doctor --strict
```

## Design Boundaries

- Do not introduce remote push/pull.
- Do not introduce signing commands or trust policy.
- Do not introduce merge/rebase flows.
- Do not introduce public partial-save or compression contracts.
- Do not introduce complex retention policy flags.
- Do not teach storage or package names as product vocabulary.

## Implementation Shape

The implementation should preserve these design properties:

- Control data is not workspace payload.
- Save point publish is atomic from the public reader's perspective.
- Restore is preview-first and history-preserving.
- Restore run revalidates the reviewed plan before writing.
- Interrupted restore leaves a recovery plan with status, resume, and rollback.
- Workspace creation starts a new history from source content.
- Strict doctor is the public health path.
- Release evidence and changelog entries use save point vocabulary only.
