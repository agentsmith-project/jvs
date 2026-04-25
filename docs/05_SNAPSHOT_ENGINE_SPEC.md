# Checkpoint Engine Spec (v0)

JVS provides one checkpoint command with pluggable materialization engines.

## Engines

### `juicefs-clone` (preferred)

```bash
juicefs clone <SRC_WORKSPACE> <DST_CHECKPOINT> [-p]
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

Checkpoint descriptors MUST record materialization metadata from the engine clone result:

- `engine`: requested materialization engine.
- `actual_engine`: engine that wrote the payload.
- `effective_engine`: actual public materialization class after fallback.
- `degraded_reasons`: degradation reasons for that materialization.
- `metadata_preservation` and `performance_class`.

## Atomic publish and durability protocol (MUST)

1. Verify preconditions (source exists, consistency policy).
2. Create runtime operation record `.jvs/intents/<id>.json`; fsync the record
   file and parent dir.
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
11. Update internal `current`/`latest` checkpoint metadata in
    `.jvs/worktrees/<name>/config.json` last; fsync parent dir.
12. Remove or complete the runtime operation record; append audit event.

Success return is allowed only after steps 1-12 complete.

## Integrity and verification model (MUST)

Descriptor MUST include:

- `descriptor_checksum`
- `payload_root_hash`

`jvs verify` defaults to checksum + payload hash validation.

## Internal READY marker

Path: `.jvs/snapshots/<id>/.READY`

Required contents:

- checkpoint id (stored internally as `snapshot_id`)
- created_at
- engine
- descriptor checksum
- payload root hash

## Payload root hash computation (MUST)

The `payload_root_hash` is a deterministic hash over the checkpoint payload tree.

### Algorithm

1. Walk the materialized checkpoint storage directory recursively in **byte-order sorted** path order.
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

- Orphan `*.tmp` payload directories and incomplete runtime operation records
  are non-visible.
- `jvs doctor --strict` MUST report stale runtime artifacts, missing READY
  markers, descriptor/payload mismatches, and unsafe workspace current/latest
  metadata inconsistencies.
- The stable public automatic repair surface is limited to runtime-safe
  repairs listed by `jvs doctor --repair-list`: `clean_locks`,
  `clean_runtime_tmp`, and `clean_runtime_operations`.
- Repairs that would rewrite checkpoint lineage, regenerate durable indexes, or
  rewrite audit history are outside the stable v0 public repair surface.
