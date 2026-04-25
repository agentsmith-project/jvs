# Snapshot Engine Spec (v7.0)

JVS provides one snapshot command with pluggable engines.

## Engines
### `juicefs-clone` (preferred)
```bash
juicefs clone <SRC_WORKTREE> <DST_SNAPSHOT> [-p]
```

### `reflink-copy` (fallback)
Recursive file walk with reflink where supported.

### `copy` (fallback everywhere)
Recursive deep copy.

## Engine selection (MUST)
1. JuiceFS mount + `juicefs` CLI -> `juicefs-clone`
2. reflink probe success -> `reflink-copy`
3. fallback -> `copy`

Override: `JVS_SNAPSHOT_ENGINE=juicefs-clone|reflink-copy|copy`

Engine performance classes (Constitution §1):
- `juicefs-clone`: constant-time metadata clone when source and destination are on a supported JuiceFS mount and the `juicefs` CLI succeeds.
- `reflink-copy`: linear tree walk with copy-on-write data sharing for files where reflink succeeds.
- `copy`: linear data copy fallback.

## Metadata behavior declaration (MUST)
Implementation MUST define behavior for:
- symlinks
- hardlinks
- mode/owner/timestamps
- xattrs
- ACLs

If preservation is degraded, command MUST fail or write explicit degraded fields. Silent downgrade is forbidden.

For v0, hardlink identity is not guaranteed by the public engine metadata contract. `metadata_preservation.hardlinks` MUST describe hardlink identity as not guaranteed rather than promising per-occurrence hardlink accounting.

Public setup JSON MUST expose:
- `effective_engine`: the engine expected for subsequent materialization in the target repository.
- `transfer_engine`: for import/clone setup commands, the requested source-to-destination transfer strategy selected for this command.
- `transfer_mode`: the actual transfer class after fallback for this command.
- `degraded_reasons`: machine-readable reasons for this command's transfer degradation only.
- `metadata_preservation`: the preservation contract for `effective_engine`.
- `performance_class`: the performance class for `effective_engine`.

Snapshot descriptors MUST record materialization metadata from the engine clone result:
- `engine`: requested materialization engine.
- `actual_engine`: engine that wrote the payload.
- `effective_engine`: actual public materialization class after fallback.
- `degraded_reasons`: degradation reasons for that materialization.
- `metadata_preservation` and `performance_class`.

## Atomic publish and durability protocol (MUST)
1. Verify preconditions (source exists, consistency policy).
2. Create intent `.jvs/intents/snapshot-<id>.json`; fsync intent file and parent dir.
3. Materialize payload into `.jvs/snapshots/<id>.tmp/`.
4. Compute `payload_root_hash` over the materialized tmp payload.
5. Fsync all new files and directories in snapshot tmp tree.
6. Build descriptor tmp `.jvs/descriptors/<id>.json.tmp` with:
   - `descriptor_checksum`
   - `payload_root_hash`
7. Fsync descriptor tmp file.
8. Write `.READY` in snapshot tmp with descriptor checksum; fsync.
9. Rename snapshot tmp -> `.jvs/snapshots/<id>/`; fsync snapshots parent dir.
10. Rename descriptor tmp -> `.jvs/descriptors/<id>.json`; fsync descriptors parent dir.
11. Update `head_snapshot_id` in `.jvs/worktrees/<name>/config.json` last; fsync parent dir.
12. Mark intent completed; append audit event.

Success return is allowed only after steps 1-12 complete.

## Integrity and verification model (MUST)
Descriptor MUST include:
- `descriptor_checksum`
- `payload_root_hash`

`jvs verify` defaults to checksum + payload hash validation.

## READY marker
Path: `.jvs/snapshots/<id>/.READY`
Required contents:
- snapshot id
- created_at
- engine
- descriptor checksum
- payload root hash

## Payload root hash computation (MUST)
The `payload_root_hash` is a deterministic hash over the snapshot payload tree.

### Algorithm
1. Walk the materialized snapshot directory recursively in **byte-order sorted** path order.
2. For each entry, compute a record: `<type>:<relative_path>:<metadata>:<content_hash>`.
   - `type`: `file`, `symlink`, or `dir`.
   - `relative_path`: path relative to snapshot root, using `/` separator, NFC normalized.
   - For `file`: `content_hash` = SHA-256 of file content; `metadata` = `mode:size`.
   - For `symlink`: `content_hash` = SHA-256 of link target string; `metadata` = empty.
   - For `dir`: `content_hash` = empty; `metadata` = empty. Dirs are included for structure completeness.
3. Concatenate all records with newline separator.
4. Compute SHA-256 of the concatenated result.

### Properties
- Deterministic: same payload always produces same hash.
- Detects file content changes, permission changes, added/removed files, and symlink target changes.
- Empty directories are included in the hash.

## Crash recovery
- Orphan `*.tmp` and incomplete intents are non-visible.
- **Head pointer orphan**: if a READY snapshot exists with a descriptor but `head_snapshot_id` in `.jvs/worktrees/<name>/config.json` does not reference it, `jvs doctor --strict` MUST detect this as `head_orphan` and offer `advance_head` repair to point head to the latest READY snapshot in the lineage chain.
- `jvs doctor --strict` MUST classify repair actions:
  - `clean_tmp` — remove orphan `.tmp` snapshot and descriptor files
  - `rebuild_index` — regenerate `index.sqlite` from snapshot/descriptor state
  - `audit_repair` — recompute audit hash chain over present records (does not recover missing records; missing records indicate tampering and require escalation)
  - `advance_head` — advance head to latest READY snapshot when head is stale
  - `clean_intents` — remove completed or abandoned intent files (runtime state rebuild)
