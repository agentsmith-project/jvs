# Migration & Backup (v0)

JVS does not provide remote replication. Use JuiceFS replication tools.

## Recommended method
Use `juicefs sync` for repository migration.

## Pre-migration gates (MUST)
1. freeze writers and stop agent jobs
2. ensure no active operations
3. run:
```bash
jvs doctor --strict
jvs verify --all
```
4. create final checkpoints for critical workspaces

## Runtime-state policy (MUST)
Runtime state is non-portable and must not be migrated as authoritative state:
- active `.jvs/intents/`
- active `.jvs/gc/<plan_id>.json` plans

Destination MUST rebuild runtime state:
```bash
jvs doctor --strict --repair-runtime
```

## Release-facing migration notes
Existing v0 repositories do not need an on-disk migration for the Phase 4 GA
readiness update. After upgrading, run `jvs doctor --strict` and
`jvs verify --all` on a representative repo. After a physical backup or storage
migration, also run `jvs doctor --strict --repair-runtime` at the destination
before verification.

## Migration flow
1. mount source and destination volumes
2. sync repository excluding runtime state
```bash
juicefs sync /mnt/src/myrepo/ /mnt/dst/myrepo/ \
  --exclude '.jvs/intents/**' \
  --exclude '.jvs/gc/*.json' \
  --update --threads 16
```
3. validate destination
```bash
cd /mnt/dst/myrepo/main
jvs doctor --strict --repair-runtime
jvs verify --all
jvs checkpoint list --json | jq '.data[:10]'
```

## What to sync
Portable checkpoint state:
- `.jvs/format_version`
- `.jvs/worktrees/`
- `.jvs/snapshots/`
- `.jvs/descriptors/`
- `.jvs/audit/`
- `.jvs/gc/tombstones/`

Active GC plans are repository-bound runtime state. Full repository clone and
the recommended sync exclusions leave them behind. If a plan is copied
out-of-band, it will fail safe with `E_GC_PLAN_MISMATCH`; create a new plan
after migration.

Optional payload state:
- `main/`
- selected workspace payload directories

## Restore drill (SHOULD)
1. restore backup to fresh volume
2. run strict doctor + verify
3. restore at least one older checkpoint into a new workspace
4. record drill result in operations log
