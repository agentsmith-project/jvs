# Threat Model (v0)

## Assets
- checkpoint payloads
- descriptors and lineage metadata
- audit trail
- descriptor checksums and payload hashes

## Adversary assumptions
- can read and write files in repository path with compromised local account
- cannot break strong cryptography
- can attempt concurrent write operations

## Key threats and controls
1. Concurrent writes causing data races
   Control: JVS v0 relies on filesystem-level mutual exclusion; users are responsible for coordinating concurrent access.
2. Descriptor and checksum both rewritten
   Control: descriptor checksum + payload root hash detect independent tampering. Coordinated rewrite is a v0.x accepted risk (see 09_SECURITY_MODEL.md).
3. Path traversal on workspace operations
   Control: strict name validation + canonical path boundary checks.
4. Crash during checkpoint publish
   Control: tmp+READY protocol + fsync durability sequence.
5. Runtime-state poisoning after migration
   Control: runtime-state exclusion and rebuild at destination.

## Residual risks
- filesystem or kernel bugs bypassing expected durability semantics
- coordinated descriptor + checksum rewrite by attacker with filesystem write access (mitigated by signing in v1.x)

## Risk labeling
Commands and JSON output MUST label high-risk states:
- `best_effort` checkpoints
