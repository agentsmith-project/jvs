# Cleanup Spec

**Status:** active save point cleanup semantics

The repository filename is retained for manifest stability only. Public docs,
help, examples, JSON, and release notes use `cleanup`.

Public product language is cleanup. Cleanup is reviewed deletion of
unprotected save point storage. It is not a normal user path for creating,
viewing, or restoring work. Cleanup does not delete workspace folders, user
cache directories, JVS control data, or runtime state; it does not prune
workspace history or apply a retention policy.

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
- Cleanup protects workspace history, open views, active recovery plans,
  active operations, and imported clone history.
- Labels do not protect save points.
- Kept save points and direct explanatory sources are protected when the public
  keep contract is promoted.
- Deleted save point storage requires tombstone/audit information so later view
  or restore attempts can produce a `deleted-save` style error.

## Protected Save Points

At minimum cleanup protects save points needed by:

- workspace history
- open views
- active recovery plans
- active operations
- imported clone history

Protection explanations are grouped by stable generic reason:

- `history`
- `open_view`
- `active_recovery`
- `active_operation`
- `imported_clone_history`

Each group reports its `reason`, `count`, and protected `save_points`. Reasons
come from cleanup's public protection boundary: workspace history/provenance,
open read-only views, active recovery plans, active operations, and imported
clone history recorded by durable repo clone metadata.

JSON keeps the stable reason tokens. Human cleanup output renders them as
workspace history, open views, active recovery plans, active operations, and
imported clone history.

## Plan Evidence

Cleanup plan evidence must use save point terminology:

- `plan_id`
- `created_at`
- `protected_save_points`
- `protection_groups`
- `protected_by_history`
- `candidate_count`
- `reclaimable_save_points`
- `reclaimable_bytes_estimate`

Any implementation storage fields with different names are internal storage
details and must not be used as product vocabulary.

## Runtime And Migration Boundary

Cleanup preview/run runtime state is not portable backup or migration
authority.
Migration uses an offline whole-folder copy of the managed folder/repository as
a whole to a fresh destination. On the destination, run
`jvs doctor --strict --repair-runtime` and create a fresh cleanup preview before
using cleanup.
