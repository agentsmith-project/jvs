# Threat Model

**Status:** active save point threat model

## Assets

- save point payloads
- save point descriptors
- workspace metadata and provenance
- restore preview plans
- recovery plans and restore backups
- audit records
- runtime operation state

## Threats

1. Concurrent writes during save or restore.
2. Crash during save publish or restore run.
3. Manual edits to descriptors, payloads, or audit records.
4. Runtime state copied across hosts during backup/migration.
5. Cleanup deleting a source save point still needed by live workspaces, active
   views, active operations, or recovery plans.
6. A user treating labels/messages/tags as restore targets or retention
   protection.

## Controls

- Workspace/repository mutation guards around mutating operations.
- Save staging-before-publish.
- Restore preview/run plan binding and expected target evidence.
- Recovery status/resume/rollback for interrupted restore.
- Descriptor checksum, payload hash, and audit chain validation.
- Runtime repair limited to safe runtime cleanup actions.
- Migration docs exclude `.jvs/locks/`, `.jvs/intents/`, and active
  `.jvs/gc/*.json` plans.
- Cleanup protects active sources and recovery plans.

## Residual Risks

- A local attacker with full write access can rewrite coordinated descriptor,
  payload, and checksum state.
- JVS does not provide signer trust, remote attestation, or server-side access
  control in the public contract.
- Operators must coordinate external writers and resolve active recovery plans
  before continuing destructive workflows.
