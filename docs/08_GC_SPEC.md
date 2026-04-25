# GC Spec (v7.0)

## Goal
Control snapshot storage growth without breaking recoverability.

## Objects
- snapshot: `.jvs/snapshots/<id>/`
- descriptor: `.jvs/descriptors/<id>.json`
- pin: retention protection entry
- plan: deterministic deletion proposal
- tombstone: pending-delete marker

## Protection rules (MUST)
Non-deletable snapshots:
- current heads of all worktrees
- ancestors reachable from protected heads
- pinned snapshots
- snapshots referenced by active intents

## Pin model

Note: v0.x does not include a CLI command for pin management. Pins can be created by writing JSON files directly to `.jvs/gc/pins/<pin_id>.json`. Readers MAY also honor legacy `.jvs/pins/<pin_id>.json` files for backward compatibility, but new pins SHOULD use `.jvs/gc/pins`. A `jvs gc pin/unpin` CLI interface is planned for v1.x.

Pin fields:
- `pin_id`
- `snapshot_id`
- `reason`
- `created_at`
- `expires_at` (nullable)

## Retention policy
- `keep_last_n`
- `keep_days`
- `keep_tag_prefixes`
- `max_repo_bytes` (optional)

## `jvs gc plan` (MUST)
- read-only
- deterministic output for fixed inputs
- writes plan metadata with `plan_id`
- JSON includes:
  - `plan_id`
  - `candidate_count`
  - `protected_by_pin`
  - `protected_by_lineage`
  - `deletable_bytes_estimate`

## `jvs gc run --plan-id <id>` two-phase protocol (MUST)
### Phase A: mark
1. load accepted plan id
2. revalidate candidate set equality; else fail `E_GC_PLAN_MISMATCH`
3. write tombstones with `gc_state=marked`

### Phase B: commit
4. delete snapshot/descriptor pair per tombstone
5. write commit record with `gc_state=committed`
6. append batch audit event

## Failure handling
- if commit fails mid-batch, stop immediately
- set failed tombstones `gc_state=failed` with reason
- rerun continues from failed markers safely
- already deleted items are idempotent, not corruption
- failed runs MUST return an error and MUST NOT delete the saved plan
