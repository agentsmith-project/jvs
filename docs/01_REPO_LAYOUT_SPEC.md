# Repository Layout Spec (v0)

## Definitions
- Volume: mounted filesystem (JuiceFS preferred)
- Repository: directory containing `.jvs/` and standard JVS layout
- Workspace: pure payload directory registered in `.jvs/worktrees/<name>/`

## Standard on-disk layout (MUST)
```
repo/
├── .jvs/
│   ├── format_version
│   ├── worktrees/      # workspace metadata (centralized internal path)
│   │   ├── main/
│   │   │   └── config.json
│   │   └── <name>/
│   │       └── config.json
│   ├── snapshots/
│   ├── descriptors/
│   ├── intents/        # in-flight operation records; not migrated as-is
│   ├── audit/          # append-only audit events
│   ├── gc/             # gc plans, tombstones, and runtime cleanup state
│   └── index.sqlite    # optional, rebuildable
│
├── main/               # pure payload — zero control-plane artifacts
│   └── <workspace payload...>
│
└── worktrees/
    └── <name>/         # pure payload — zero control-plane artifacts
        └── <workspace payload...>
```

## `format_version` (MUST)
Path: `.jvs/format_version`

Contents: single line with integer format version.
- `jvs init` writes `1`.
- JVS MUST read `format_version` before any operation.
- If `format_version` > supported version, fail with `E_FORMAT_UNSUPPORTED`.
- If `format_version` < current version and migration is available, `jvs doctor --strict` SHOULD report upgrade recommendation.
- Format version increments only on incompatible on-disk layout changes.

## Checkpoint tags (MUST)
Tags are embedded directly in checkpoint descriptors as a `tags` array field.

### Rules (MUST)
- Tag strings follow the same safety rules as workspace names: `[a-zA-Z0-9._-]+`
- Tags are optional and may be empty.
- Tags are part of the descriptor and thus immutable once the checkpoint is created.
- Use `jvs checkpoint list` or `jvs checkpoint list --json` to list tagged checkpoints.
- Use `jvs restore <tag>` only when the exact tag resolves to one checkpoint.

## Invariants (MUST)
- `.jvs/` MUST NOT exist under any payload root.
- Payload roots MUST contain zero control-plane artifacts (no hidden metadata directories).
- Payload roots MUST NOT contain the reserved additional-workspace container.
- Workspace roots MUST resolve to canonical paths under repo root.
- All control-plane paths MUST reject symlink traversal outside repo root.
- Every workspace payload directory MUST have a corresponding entry in `.jvs/worktrees/<name>/config.json`.

## Portability classes
- Portable history state: `format_version`, `.jvs/worktrees/`, `.jvs/snapshots/`, `.jvs/descriptors/`, `.jvs/audit/`, and committed tombstones under `.jvs/gc/tombstones/`.
- Rebuildable cache state: `index.sqlite`.
- Runtime state (non-portable): active `.jvs/intents/` operation records and active `.jvs/gc/*.json` plans.

## Why `repo/main/` exists
JuiceFS clone performs 1:1 directory clone without excludes.
Separating `main/` from `.jvs/` guarantees clean checkpoint payload scope.
Workspace metadata is stored under `.jvs/worktrees/` (not inside payload roots) for the same reason — clone cannot exclude subdirectories, so payload roots must contain zero control-plane artifacts.

## Workspace discovery
JVS locates workspace metadata by:
1. Walking up from CWD to find the repo root (directory containing `.jvs/`).
2. Computing the relative path of CWD within the repo.
3. Mapping to the workspace name: `main/...` → `main`; additional workspace payload paths map to `<name>`.
4. Loading `.jvs/worktrees/<name>/config.json`.
