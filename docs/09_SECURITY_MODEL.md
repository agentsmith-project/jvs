# Security Model (v0)

## Scope
Defines trust, integrity, and operational security requirements for JVS repositories.

## Security objectives
- detect descriptor and payload corruption or tampering via checksums and hashes
- preserve auditable operation history via tamper-evident audit trail

## Supported algorithms (MUST)

### Hash algorithms
- `sha256` (default): SHA-256 for `descriptor_checksum` and `payload_root_hash`.
- Future additions MUST be registered in this spec before use.

Algorithm identifiers in descriptors MUST match values defined here exactly.

## Integrity model (MUST)
1. descriptor checksum layer
2. payload root hash layer

Checkpoint integrity requires both layers to pass.

## Verification policy
- `jvs verify` defaults to strong verification (checksum + payload hash).
- `jvs verify --all` applies checkpoint-scoped checks to every checkpoint and returns per-checkpoint results.
- Repository-health checks, including audit chain validation, are performed by `jvs doctor --strict`.

## Audit requirements
Every mutating operation MUST append audit record with:
- actor identity
- operation type
- target checkpoint/workspace
- reason for dangerous operations

## Audit log format (MUST)

### Storage
Path: `.jvs/audit/audit.jsonl`

Format: JSON Lines (one JSON object per line, append-only).

### Internal Record Schema (MUST)
Each audit record MUST contain:
- `event_id`: unique event identifier (UUID v4)
- `timestamp`: ISO 8601 with timezone
- `operation`: operation type (`snapshot`, `restore`, `gc_run`, `ref_create`, `ref_delete`, `worktree_create`, `worktree_remove`, `worktree_rename`, `doctor_repair`)
- `actor`: actor identity string
- `target`: affected checkpoint/workspace ID
- `reason`: mandatory for dangerous operations, nullable otherwise
- `prev_hash`: SHA-256 hash of the previous audit record (empty string for first record)
- `record_hash`: SHA-256 hash of this record (all fields except `record_hash` itself, serialized as canonical JSON)

Canonical JSON rules for `record_hash` computation:
- keys sorted lexicographically by Unicode code point
- no whitespace between tokens
- UTF-8 encoding
- strings escaped per RFC 8259
- numbers: no leading zeros, no trailing zeros in fractions, no positive sign
- null values serialized as `null`

### Integrity chain (MUST)
- Each record includes `prev_hash` linking to the prior record, forming a hash chain.
- `jvs doctor --strict` MUST validate the audit hash chain and report `E_AUDIT_CHAIN_BROKEN` on mismatch.
- `jvs verify --all` MUST remain checkpoint-scoped and preserve its per-checkpoint result contract.

### Rotation (SHOULD)
- When `audit.jsonl` exceeds 100 MB, rotate to `audit-<timestamp>.jsonl`.
- Rotated files are portable audit state and included in migration.
- Rotation appends a final chain-closing record to the old file and a chain-opening record to the new file with `prev_hash` referencing the old file's last `record_hash`.

## v0.x accepted risks
- An attacker with filesystem write access can rewrite a descriptor and its checksum consistently without detection. Descriptor signing (planned for v1.x) will close this gap.
- This risk is acceptable for v0.x local single-user and agent workflows.

## Non-goals
- encryption-at-rest policy management
- in-JVS authn/authz framework
- Descriptor signing and trust policy (deferred to v1.x)
