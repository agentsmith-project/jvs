# Restore Spec (v0)

## Overview

`jvs restore` materializes a checkpoint into the targeted workspace. Restore is
in place: it replaces the live workspace contents and moves `current` to the
requested checkpoint.

If `current` differs from `latest` after a historical restore, checkpoint
creation is blocked to preserve linear workspace lineage. Use `jvs fork` to
continue from that historical checkpoint in another workspace.

## Command

```bash
jvs restore <ref|current|latest> [--discard-dirty|--include-working] [--json]
```

## Refs

`<ref>` may be:
- `current`
- `latest`
- a full checkpoint ID
- a unique checkpoint ID prefix
- an exact tag that resolves to one checkpoint

`dirty` is not a checkpoint ref. Notes/messages are not refs.

## Dirty Safety

Restore refuses to overwrite dirty workspace contents by default.

- `--include-working` creates a checkpoint for dirty work before restoring.
- `--discard-dirty` discards dirty work and restores the target checkpoint.
- `--include-working` and `--discard-dirty` are mutually exclusive.

## Behavior

1. Resolve the requested ref to one checkpoint.
2. Verify that the checkpoint descriptor exists and passes integrity checks.
3. Apply dirty-safety handling.
4. Atomically replace workspace contents with checkpoint contents.
5. Update the workspace `current` checkpoint to the restored checkpoint.
6. Leave `latest` as the newest checkpoint on the workspace line.

Restoring `latest` returns the workspace to its newest checkpoint.

## Safety

Restore is safe by default:
- existing checkpoints are preserved
- the lineage chain remains intact
- v0 GC continues to protect checkpoints reachable from live workspace roots

## Examples

```bash
# Restore to a specific checkpoint
jvs restore 1771589366482-abc12345

# Restore by exact tag
jvs restore v1.0

# Return to the latest checkpoint
jvs restore latest

# Continue from a historical checkpoint in another workspace
jvs fork v1.0 hotfix-123
```

## Error Handling

| Error | Cause | Resolution |
|-------|-------|------------|
| Checkpoint not found | Invalid ID, ambiguous prefix, or unresolved tag | Use `jvs checkpoint list` to find valid IDs |
| Dirty workspace | Restore would overwrite uncheckpointed changes | Use `--include-working` or `--discard-dirty` intentionally |
| Current differs from latest | Attempted `jvs checkpoint` after restoring an older checkpoint | Use `jvs restore latest` or create another workspace with `jvs fork` |

## v0 Boundaries

The stable v0 restore contract does not include note-prefix refs, interactive
restore selection, or legacy restore flags such as `--inplace`, `--force`, and
`--reason`.
