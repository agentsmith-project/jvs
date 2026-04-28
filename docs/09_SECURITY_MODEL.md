# Security Model

**Status:** active save point security model

JVS provides local integrity and tamper-evidence for save point storage. It
does not provide remote trust, signer identity, key management, or server-side
authorization in the public contract.

## Protected Assets

- save point descriptors
- save point payload storage
- workspace metadata and provenance
- recovery plans and restore backups
- audit records
- runtime locks and operation records

## Trust Boundary

JVS assumes the local filesystem and operator account are the trust boundary.
An attacker with arbitrary write access to the repository can corrupt or
rewrite state. JVS detects many classes of corruption, but it is not a
cryptographic transparency log or remote attestation system.

## Integrity Checks

Save point integrity uses:

- descriptor checksum
- payload root hash
- publish-ready markers
- audit chain checks

`jvs doctor --strict` is the public health command.

## Audit Chain

Audit records link each record to the previous record hash. Strict doctor uses
the audit chain to detect truncation or manual edits.

Internal audit records may contain storage-oriented operation names. Public
output must use save point, workspace, restore, recovery, doctor, and cleanup
terms.

## Runtime Safety

- Mutating operations hold repository/workspace locks as appropriate.
- Restore run binds to a preview plan and revalidates expected target state.
- Interrupted restore creates or preserves a recovery plan.
- Active recovery plans block new restore runs in the same workspace.
- Runtime repair is limited to `clean_locks`, `clean_runtime_tmp`, and
  `clean_runtime_operations`.

## Non-Goals

- signing commands
- signer trust policy
- remote push/pull authentication
- server-side authorization
- automatic durable history rewrite or audit repair
