# Checkpoint Scope & Lineage Spec (v0)

This spec defines checkpoint scope, internal descriptor lineage, and the
integrity responsibilities shared by `jvs verify` and `jvs doctor --strict`.
Public vocabulary is checkpoint/workspace/current/latest. On-disk descriptor
compatibility details are documented in the internal schema section below.

## Checkpoint ID generation (MUST)

Format: `<timestamp_ms>-<random_hex8>`

- `<timestamp_ms>`: Unix epoch milliseconds at checkpoint creation, zero-padded to 13 digits.
- `<random_hex8>`: 8 lowercase hex characters from cryptographic random source.
- Example: `1708300800000-a3f7c1b2`

Properties:

- Lexicographic sort approximates creation order.
- Collision probability is negligible (32-bit random within same millisecond).
- Short display IDs are the first 8 characters of the full ID.
- Checkpoint IDs MUST be treated as opaque strings by consumers; ordering is
  advisory only.

## Scope (MUST)

A checkpoint captures only the targeted workspace payload root:

- inside `repo/main/` -> source `repo/main/`
- inside `repo/worktrees/<name>/` -> source that workspace root

Payload roots contain pure user data (no control-plane artifacts), so no exclusion logic is required.

Checkpoint payloads MUST NOT include:

- `.jvs/` directory
- other workspace payload roots

## Storage and immutability (MUST)

Published checkpoint payloads live at:
`repo/.jvs/snapshots/<checkpoint-id>/`

After READY publication:

- payload is immutable
- descriptor is immutable
- detected mutation marks the checkpoint `corrupt`

## Internal descriptor schema (MUST)

Path:
`repo/.jvs/descriptors/<checkpoint-id>.json`

Required fields:

- `snapshot_id` (public `checkpoint_id`)
- `worktree_name` (public `workspace`)
- `parent_id` (or null)
- `created_at`
- `note` (optional)
- `tags` (optional array)
- `engine`
- `descriptor_checksum`
- `payload_root_hash`
- `integrity_state` (`verified|unverified|corrupt`)

## Descriptor checksum coverage (MUST)

`descriptor_checksum` is computed over all descriptor fields **except**:

- `descriptor_checksum` itself
- `integrity_state`

Computation:

1. Serialize covered fields as canonical JSON (sorted keys, no whitespace, UTF-8, no trailing zeros in numbers).
2. Compute SHA-256 of the serialized bytes.

## Lineage rules

- Lineage is per workspace via `parent_id` chain.
- `jvs fork` from an older checkpoint starts a new workspace lineage branch.
- merge/rebase remains out of scope for v0.x.

## Lineage integrity checks (MUST)

`jvs doctor --strict` MUST detect:

- missing parent descriptor
- parent cycles
- workspace `current`/`latest` pointer mismatch
- descriptor checksum mismatch
- payload hash mismatch

`jvs verify [checkpoint-id]` and `jvs verify --all` remain checkpoint-scoped:
they MUST validate descriptor checksum and payload root hash, but audit-chain
and workspace-lineage health belong to `jvs doctor --strict`.

All doctor findings MUST include machine-readable severity.
