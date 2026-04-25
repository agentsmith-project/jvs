# GC Spec (v0)

## Goal
Control checkpoint storage growth without breaking recoverability.

v0 GC is deliberately small: it exposes only a two-stage workflow:

```bash
jvs gc plan
jvs gc run --plan-id <id>
```

There are no public pin commands, retention flags, tag-retention rules, partial
cleanup modes, or compression-specific cleanup modes in v0.

## Objects
- checkpoint payload: `.jvs/snapshots/<id>/`
- descriptor: `.jvs/descriptors/<id>.json`
- plan: deletion proposal written under `.jvs/gc/`
- tombstone: pending or completed delete marker under `.jvs/gc/tombstones/`

Active GC plans are runtime state. A plan is bound to the repository identity
that created it and must not be treated as portable migration state. Full
repository clone excludes active plan files under `.jvs/gc/*.json`.

## Protection Rules (MUST)
Non-deletable checkpoints:
- current, latest, and base checkpoints of all live workspaces
- ancestors reachable from those live workspace roots
- checkpoints referenced by active operation records in `.jvs/intents/`

Removed workspaces no longer protect their former checkpoint lineage. Such
orphaned checkpoints can appear in `to_delete` immediately after planning.

## `jvs gc plan` (MUST)
- accepts no positional arguments
- computes candidates without deleting data
- writes a plan with:
  - schema version
  - `plan_id`
  - creating `repo_id`
  - candidate checkpoint IDs
  - protected lineage count
  - estimated reclaimable bytes from payload plus descriptor files
- fails without writing a plan when the audit log cannot be safely extended
- appends a `gc_plan` audit event after writing a successful plan
- human output starts with `Plan ID: <id>`
- JSON `data` includes:
  - `plan_id`
  - `created_at`
  - `protected_checkpoints`
  - `candidate_count`
  - `protected_by_lineage`
  - `to_delete`
  - `deletable_bytes_estimate`

`protected_checkpoints` contains the public checkpoint IDs kept by v0 GC safety
rules: live workspace roots, their ancestors, and checkpoints referenced by
active operation records. It is not a pin list and does not imply any public
retention policy.

Public v0 JSON must not expose pin, retention, or future policy fields.

## `jvs gc run --plan-id <id>` Two-Phase Protocol (MUST)
`gc run` accepts no positional arguments.

### Phase A: mark
1. load the requested plan
2. verify plan schema, `plan_id`, and repository identity
3. revalidate candidate set equality; else fail `E_GC_PLAN_MISMATCH`
4. write tombstones with `gc_state=marked`

### Phase B: commit
5. delete checkpoint payload and descriptor files for each pending tombstone
6. write commit record with `gc_state=committed`
7. append batch audit event

## Failure Handling
- missing, stale, repository-mismatched, or self-mismatched plans fail with
  `E_GC_PLAN_MISMATCH`
- if commit fails mid-batch, stop immediately
- set failed tombstones `gc_state=failed` with reason
- rerun continues from failed markers safely
- already deleted items are idempotent, not corruption
- failed runs MUST return an error and MUST NOT delete the saved plan
