# Overview

**Document set:** JVS v0 (JuiceFS-first, checkpoint-first)
**Date:** 2026-02-22

## Core idea
JVS versions workspaces by full checkpoints of a single workspace payload root.

## Frozen design decisions
1. No remote replication features in JVS; JuiceFS handles transport.
2. Main payload root is `repo/main/`.
3. Checkpoint publish is READY-based and auditable.
4. Restore is always inplace; use `jvs fork` to create branches.
5. Verification default is strong: checksum + payload hash. Signature/trust chain deferred to v1.x.
6. Runtime state (active `.jvs/locks/`, `.jvs/intents/`, and active
   `.jvs/gc/*.json`) is non-portable and rebuilt after migration.

## Product promise
- Current differs from latest model for safe history navigation
- Verifiable and tamper-evident history
- Filesystem-native scale on JuiceFS

## v0.x scope limitations
The following Constitution features are architecturally planned but deferred from v0.x implementation:
- **Descriptor signing and trust policy** (Constitution §7.4 justification): v0.x integrity relies on descriptor checksum + payload root hash. Signing adds protection against coordinated checksum+descriptor rewrite by an attacker with filesystem write access; this threat is accepted as residual risk in v0.x.

Descriptor schema reserves optional fields for future signature support to ensure forward compatibility.

## Non-goals
- Git parity and text merge semantics
- in-JVS authn/authz control plane
- Distributed locking or fencing mechanisms (JVS is local-first)
