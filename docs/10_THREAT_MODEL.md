# Threat Model

**Status:** active save point threat model

## Assets

- save point content
- save point descriptors
- workspace metadata and provenance
- restore preview plans
- recovery plans and restore backups
- audit records
- runtime operation state

## Threats

1. Concurrent writes during save or restore.
2. Crash during save publish or restore run.
3. Manual edits to descriptors, saved content, or audit records.
4. Runtime state copied across hosts during backup/migration.
5. Cleanup deleting a save point still needed by workspace history, open views,
   active recovery plans, or active operations.
6. A user treating labels/messages/tags as restore targets or retention
   protection.

## Controls

- Workspace/repository mutation guards around mutating operations.
- Save staging-before-publish.
- Restore preview/run plan binding and expected target evidence.
- Recovery status/resume/rollback for interrupted restore.
- Descriptor checksum, content hash, and audit chain validation.
- Runtime repair limited to safe runtime cleanup actions.
- Migration guidance treats non-portable JVS runtime state as
  destination-local and rebuilds it with `jvs doctor --strict --repair-runtime`.
- Cleanup protects workspace history, open views, active recovery plans, active operations,
  and imported clone history.

## Residual Risks

- A local attacker with full write access can rewrite coordinated descriptor,
  saved content, and checksum state.
- JVS does not provide signer trust, remote attestation, or server-side access
  control in the public contract.
- Operators must coordinate external writers and resolve active recovery plans
  before continuing destructive workflows.
