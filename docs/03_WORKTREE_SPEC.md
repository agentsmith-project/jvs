# Workspace Metadata Spec (v0)

## Purpose

This spec defines workspace identity, payload-root separation, and workspace
removal semantics for the stable v0 contract. Public CLI and JSON vocabulary is
workspace/checkpoint/current/latest. On-disk compatibility details are
documented in the internal schema sections below.

## Workspace identity

Workspace metadata is stored centrally under the control plane:

```text
repo/.jvs/worktrees/<name>/
└── config.json       # sole authoritative source for workspace metadata
```

`config.json` is the only authoritative source for workspace identity and
state. No separate legacy pointer files exist.

Workspace payload directories (`repo/main/`, `repo/worktrees/<name>/`) contain
pure user data only: no control-plane artifacts. This ensures `juicefs clone`
captures a clean payload without exclusion logic (see
`01_REPO_LAYOUT_SPEC.md` §Workspace discovery).

## Internal `config.json` schema (MUST)

Path: `repo/.jvs/worktrees/<name>/config.json`

Required fields:

- `name`: workspace name (matches directory name)
- `created_at`: ISO 8601 timestamp
- `base_snapshot_id`: internal checkpoint ID used to create this workspace;
  nullable or omitted for `main`
- `head_snapshot_id`: internal `current` checkpoint ID; nullable before the
  first checkpoint
- `latest_snapshot_id`: internal `latest` checkpoint ID; nullable before the
  first checkpoint

Optional fields:

- `label`: human-readable description

## Naming and path rules (MUST)

- Name charset: `[a-zA-Z0-9._-]+`
- Name MUST NOT contain separators, `..`, control chars, or empty segments.
- Name MUST normalize to NFC before validation.
- Canonical resolved path MUST remain under `repo/worktrees/` or be
  `repo/main/`.
- Operations MUST fail on symlink escape detection.

## Lifecycle

create -> active -> checkpoint -> restore(optional) -> remove

### Remove semantics (MUST)

`jvs workspace remove <name> [--force]` MUST:

1. Delete the payload directory (`repo/worktrees/<name>/`).
2. Delete the workspace metadata directory (`.jvs/worktrees/<name>/`).
3. Append an audit event recording the removal.

Removal MUST require `--force` when the workspace `current` checkpoint differs
from `latest`.

Removing a workspace does not remove checkpoints. Removed workspace roots no
longer participate in v0 GC protection, so unprotected orphaned checkpoints may
appear as deletion candidates in `jvs gc plan`. v0 exposes no pin commands or
retention-policy knobs.
