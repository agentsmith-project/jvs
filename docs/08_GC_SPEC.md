# Cleanup Spec

**Status:** active save point cleanup semantics

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `cleanup`.

Public product language is cleanup. Cleanup is review-first deletion of
unprotected save point storage. It is not a normal user path for creating,
viewing, or restoring work.

## Public Cleanup Contract

Cleanup is two-stage:

```text
cleanup preview -> cleanup run
```

Rules:

- Preview never deletes.
- Run binds to a reviewed plan.
- Run revalidates repository identity, plan identity, source state, and
  protection rules before deleting.
- Run fails and requires a fresh cleanup preview when either the protected save
  point set or reclaimable candidate set differs from the reviewed plan.
- Cleanup protects live workspace needs, active views, active source reads,
  active operations, and active recovery plans.
- Labels do not protect save points.
- Kept save points and direct explanatory sources are protected when the public
  keep contract is promoted.
- Deleted save points require tombstone/audit information so later view or
  restore attempts can produce a `deleted-save` style error.

## Protected Save Points

At minimum cleanup protects save points needed by:

- live workspace history/content state
- `started_from_save_point`
- whole-workspace restore provenance
- restored path provenance
- active read/materialization operations
- active read-only views
- active restore recovery plans
- kept save points when keep is available

Protection explanations are grouped by stable generic reason:

- `history`
- `open_view`
- `active_recovery`
- `active_operation`

Each group reports its `reason`, `count`, and protected `save_points`. Reasons
come from cleanup's structural sources: workspace history/provenance,
documented active source pins, active read-only view pins, active operation
intents, and active recovery plan state.

JSON keeps the stable reason tokens. Human cleanup output renders them as
workspace history, open views, active recovery plans, and active operations.

## Plan Evidence

Cleanup plan evidence must use save point terminology:

- `plan_id`
- `created_at`
- `protected_save_points`
- `protection_groups`
- `candidate_count`
- `reclaimable_save_points`
- `reclaimable_bytes_estimate`

Any implementation storage fields with different names are internal storage
details and must not be used as product vocabulary.

## Runtime And Migration Boundary

Cleanup runtime plan files are not portable backup or migration authority.
Physical sync procedures must exclude runtime cleanup state and create a fresh
cleanup preview after migration.
