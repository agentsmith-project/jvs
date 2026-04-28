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

## Plan Evidence

Cleanup plan evidence must use save point terminology:

- `plan_id`
- `created_at`
- `protected_save_points`
- `candidate_count`
- `reclaimable_save_points`
- `reclaimable_bytes_estimate`
- `protections`

Any implementation storage fields with different names are internal storage
details and must not be used as product vocabulary.

## Runtime And Migration Boundary

Cleanup runtime plan files are not portable backup or migration authority.
Physical sync procedures must exclude runtime cleanup state and create a fresh
cleanup preview after migration.
