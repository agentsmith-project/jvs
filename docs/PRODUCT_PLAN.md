# JVS Product Plan

Status: stable v0 product direction

This document records the accepted product design for JVS as a local-first,
filesystem-native, verifiable CLI tool for workspace checkpoints. It is the
coordination document for product, CLI, docs, implementation, QA, and release
work.

## Product Promise

JVS lets a human or agent create, inspect, verify, and restore checkpoints of
real filesystem workspaces without a server, staging area, diff database, or
remote protocol.

Core promises:

- Local-first: all operations work against local or mounted filesystem paths.
  JuiceFS may provide storage, sharing, and transport, but JVS does not run or
  require a service.
- Filesystem-native: a workspace is a real directory. Tools see normal files.
  JVS does not virtualize paths or hide alternate working states.
- Checkpoint-first: the versioned unit is a complete workspace tree, not a text
  diff or Git-style index.
- Verifiable: checkpoints are published atomically and can be checked with
  descriptor checksums, payload hashes, doctor checks, and audit records.
- Automation-safe: every user-visible state has a deterministic CLI and JSON
  representation.

## Public Terminology

Use these terms in user-facing docs, command help, JSON fields where feasible,
errors, and release notes.

| Term | Meaning |
| --- | --- |
| `repo` | A JVS repository root containing `.jvs/` control-plane metadata and one or more registered workspaces. |
| `workspace` | A real payload directory registered in a repo. The default workspace is `main`. Payload directories contain user files only, never `.jvs/`. |
| `checkpoint` | An immutable, full-tree record of one workspace at one point in time. |
| `current` | The checkpoint currently materialized in a workspace according to JVS metadata. After restore, `current` may be older than `latest`. |
| `latest` | The newest checkpoint on the workspace's active line. Restoring `latest` returns the workspace to the normal advance point. |
| `dirty` | The live workspace files differ from `current`, or JVS cannot prove they match `current`. Dirty work is uncheckpointed user state. |

Implementation compatibility note: older package, storage, and JSON internals
may still contain historical names while the public CLI converges. New stable
docs and examples must use the public terms above.

## Repository and Workspace Model

The repo root is not the workspace payload. The default workspace is `main`.
Additional workspaces are real directories resolved through JVS metadata and
public `workspace` commands.

Invariants:

- `.jvs/` is never inside a workspace payload and is never checkpointed.
- Workspace payload roots contain zero control-plane artifacts.
- A checkpoint captures exactly one workspace payload root.
- Published checkpoints are immutable. Mutation after publication is
  corruption.
- JVS does not implement push, pull, merge, rebase, or server-side auth.

## Internal On-Disk Compatibility Layout

This section describes physical storage names retained for compatibility. They
are not public CLI vocabulary. The current layout is:

```text
repo/
├── .jvs/              # control plane: metadata, descriptors, audit, gc
├── main/              # default workspace payload
└── worktrees/         # current on-disk location for additional workspace payloads
    └── <name>/
```

The physical `worktrees/` directory name is an on-disk compatibility detail.
Public commands and docs should say workspace.

## Targeting Rules

All commands have deterministic targeting. A command must never silently act on
the wrong repo or workspace.

Global flags:

```bash
jvs [--repo <repo-root>] [--workspace <name>] [--json] <command> ...
```

Command classes:

- path-scoped setup commands: `init`, `import`, `clone`, and `capability`.
  These commands are repo-free: they operate on explicit path arguments, do
  not require a current repo or workspace, and must not use `--repo` or
  `--workspace` as substitutes for required path arguments.
- Repo-scoped commands: `info`, `workspace list`, `workspace rename`,
  `workspace remove`, `verify --all`, `doctor`, and `gc`. These commands
  operate on the current repo.
- Workspace-scoped commands: `status`, `checkpoint`, `checkpoint list`,
  `diff`, `restore`, `fork`, and `workspace path` without `<name>`.
- `workspace path <name>` is repo-scoped because the named workspace selects
  the payload path inside the resolved repo.

Resolution rules:

1. path-scoped setup commands resolve only the explicit paths in their command
   line and do not discover a current repo or workspace.
2. Repo-scoped and workspace-scoped commands require CWD to be inside a JVS
   repo. Without `--repo`, JVS walks upward from CWD until it finds `.jvs/`.
   With `--repo`, the flag is an assertion, not an alternate discovery root:
   it must resolve to the current repo or to a path inside the current repo,
   otherwise the command fails with a targeting mismatch.
3. `--workspace` selects a registered workspace by name inside the resolved
   repo.
4. Without `--workspace`, workspace-scoped commands infer the workspace from
   CWD if CWD is inside a registered workspace payload.
5. If CWD is the repo root or inside `.jvs/`, workspace-scoped commands
   require `--workspace`.
6. If `--repo` and the inferred CWD workspace disagree, the command fails
   instead of guessing.

## v0 Stable CLI Contract

The stable public CLI is organized around repos, workspaces, checkpoints,
verification, doctor checks, and two-phase storage cleanup.

### Setup

```bash
jvs init <repo-path>
jvs import <source-dir> <repo-path>
jvs clone <source-repo> <dest-repo> [--scope full|current]
jvs capability <target-path> [--write-probe]
```

Rules:

- These are path-scoped setup commands and are repo-free. They do not depend
  on CWD being inside a JVS repo.
- `init` creates a new repo and an empty default `main` workspace at the
  explicit `<repo-path>`.
- `import` treats `<source-dir>` as payload, creates a new repo at
  `<repo-path>`, and creates an initial checkpoint tagged `import`.
- `clone --scope current` copies the live source workspace contents into the
  destination repo's `main` workspace and publishes exactly one initial
  checkpoint tagged `clone`. It does not copy source history or other
  workspaces.
- `clone --scope full` creates a new repo identity while preserving
  user-visible checkpoints, workspaces, and refs. It excludes active
  `.jvs/locks/`, `.jvs/intents/`, and `.jvs/gc/*.json` runtime state, and
  rebuilds clean runtime state at the destination.
- `clone` is a local filesystem operation, not a remote protocol.
- `capability` reports available materialization support for JuiceFS clone,
  reflink, and copy for the explicit target path.
- Setup JSON must give automation stable engine and filesystem messaging:
  `capabilities`, `effective_engine`, `warnings`, and, for transfer setup
  commands, `transfer_engine` plus `degraded_reasons`. Full clone JSON also
  reports `optimized_transfer`.

### Introspection

```bash
jvs info
jvs status
```

`info` reports repo format, repo ID, workspace count, checkpoint count, and
engine summary. `status` reports the targeted workspace, `current`, `latest`,
whether the workspace is at latest, and `dirty`.

### Checkpoints

```bash
jvs checkpoint [<message>] [--tag <tag>]...
jvs checkpoint list
jvs diff <from-ref> <to-ref> [--stat]
```

Rules:

- `checkpoint` captures the live targeted workspace.
- A new checkpoint may advance only when `current == latest`.
- If `current != latest`, checkpoint creation fails and suggests `restore
  latest` or creating another workspace with `fork`.
- Tags are immutable checkpoint metadata and must not use reserved words:
  `current`, `latest`, or `dirty`.
- v0 does not publish partial checkpoint or compression as stable contracts.

### Restore

```bash
jvs restore <ref|current|latest> [--discard-dirty|--include-working]
```

Restore replaces the live workspace contents with the referenced checkpoint.
If the workspace is dirty, restore must fail unless the user explicitly chooses
one of the safety options:

- `--discard-dirty`: discard dirty live changes and restore the target
  checkpoint.
- `--include-working`: create a checkpoint of dirty live state first, then
  restore the requested ref.

`restore latest` is the public spelling for returning to the latest
checkpoint. `restore current` is idempotent except for explicitly handling
dirty state.

### Workspaces

```bash
jvs fork [<ref> <name>|<name>] [--from <ref>]
jvs workspace list
jvs workspace path [<name>]
jvs workspace remove <name> [--force]
jvs workspace rename <old> <new>
```

Rules:

- A workspace is a real directory; switching workspaces is done by `cd` or by
  targeting with `--workspace`.
- `fork <name>` materializes a new workspace from `current`.
- `fork <ref> <name>` or `fork --from <ref> <name>` materializes from a
  specific checkpoint ref.
- `fork` uses the same dirty-safety flags as restore:
  `--discard-dirty` and `--include-working`.
- Removing a workspace never removes its checkpoints; retention is controlled
  by `gc`.

### Verification, Doctor, and Storage Cleanup

```bash
jvs verify [<checkpoint-id>] [--all]
jvs doctor [--strict] [--repair-runtime] [--repair-list]
jvs gc plan
jvs gc run --plan-id <id>
```

Rules:

- `verify` checks descriptor checksum and payload root hash.
- `doctor --strict` validates layout, publish state, lineage, runtime hygiene,
  and integrity.
- `doctor --repair-runtime` is limited to safe runtime cleanup.
- Storage cleanup is two phase in v0: `gc plan` first, then
  `gc run --plan-id <id>`.
- v0 does not include complex retention policy flags.

## Dirty Safety

Dirty safety protects live user state from accidental overwrite.

Required behavior:

- `status` must always expose `dirty`.
- Commands that overwrite or delete workspace files must refuse dirty state by
  default.
- The accepted bypasses are explicit flags whose names state the outcome:
  `--discard-dirty` and `--include-working`.
- Read-only commands must work in dirty workspaces.
- `checkpoint` is the normal way to turn dirty live state into a clean
  checkpoint.
- If JVS cannot reliably determine cleanliness, it must report `dirty: true`.

Dirty detection can use cached indexes for speed, but correctness-sensitive
operations must either validate the cache or conservatively report dirty.

## Engine Transparency

JVS may choose different materialization engines per filesystem, but users and
automation must be able to see what happened.

Supported engines:

- `juicefs-clone`: preferred when the target is on JuiceFS and clone support is
  available.
- `reflink-copy`: filesystem copy-on-write where supported.
- `copy`: portable recursive copy fallback.

Requirements:

- Auto selection must record the selected engine and fallback reason in command
  output or checkpoint metadata.
- If a future explicit engine selection is added, unavailability must be an
  error. Silent fallback is forbidden for explicit choices.
- Metadata preservation behavior must be declared as it matures: symlinks,
  hardlinks, modes, ownership, timestamps, xattrs, and ACLs.
- `info`, `capability`, and `doctor` must expose engine probes or engine
  summary.
- Public docs must not make unconditional performance promises independent of
  engine and filesystem support.

## JSON Envelope

With `--json`, every command emits exactly one JSON object to stdout. Progress
and human text must not be mixed into stdout.

Success envelope:

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": true,
  "repo_root": "/abs/repo",
  "workspace": "main",
  "data": {
    "current": "1708300800000-a3f7c1b2",
    "latest": "1708300800000-a3f7c1b2",
    "dirty": false,
    "at_latest": true
  },
  "error": null
}
```

Error envelope:

```json
{
  "schema_version": 1,
  "command": "restore",
  "ok": false,
  "repo_root": "/abs/repo",
  "workspace": "main",
  "data": null,
  "error": {
    "code": "E_USAGE",
    "message": "workspace has dirty changes",
    "hint": ""
  }
}
```

Envelope rules:

- Exit code is zero only when `ok` is true.
- Paths are absolute canonical paths where a path is returned.
- Checkpoint refs in machine output should be full checkpoint IDs.
- Error codes are stable API once assigned and must be covered by tests.

## Ref Rules

Refs identify checkpoints. They are not filesystem paths.

Accepted refs:

- `current`: the checkpoint currently materialized in the targeted workspace.
- `latest`: the latest checkpoint on the targeted workspace lineage.
- Full checkpoint ID.
- Unique checkpoint ID prefix.
- Tag name, if it resolves to exactly one checkpoint.

Rules:

- `current`, `latest`, and `dirty` are reserved words and cannot be checkpoint
  tags or workspace names.
- Notes/messages are never refs. Use list/search flows for messages.
- Ambiguous refs fail.
- Missing refs fail.
- External integrations should store full checkpoint IDs, not prefixes or tags.

## Trust and Tamper-Evidence Boundary

v0 provides integrity and tamper-evidence boundaries through descriptor
checksums, payload hashes, doctor checks, and the audit chain. It does not
provide signer identity, key management, trust policy enforcement, or a stable
`trust` CLI.

Future work may add:

- Descriptor signatures.
- Repo-portable trust material.
- `verify` or `doctor` trust evaluation.
- Restore or clone policy enforcement for signed checkpoints.

Those items are future directions. They are not v0 stable commands or release
requirements.

## Release Phases and Acceptance Gates

### Phase 0: Product alignment

Deliverables:

- This product plan exists and is referenced by implementation planning.
- Public docs use `checkpoint`, `workspace`, `current`, `latest`, and `dirty`.
- Stable entry docs avoid historical command terminology except migration or
  implementation compatibility notes.

Gate:

- Product, CLI, docs, and QA agree that this document is the target v0
  contract.

### Phase 1: Core local checkpoint tool

Deliverables:

- `init`, `import`, `clone`, `capability`, `info`, `status`, `checkpoint`,
  `checkpoint list`, `restore latest`, `fork`, `workspace list/path/remove`,
  `verify`, `doctor`, and `gc`.
- CWD and `--repo`/`--workspace` targeting.
- Dirty detection and dirty restore/fork protection.
- Engine selection with transparent output.

Gate:

- Golden CLI tests cover targeting, dirty safety, ref resolution, JSON
  envelopes, and docs command smoke.
- Existing release gate remains green.
- No workspace payload contains control-plane artifacts after setup or
  checkpoint operations.

### Phase 2: Verification and repair

Deliverables:

- Strong `verify`, strict `doctor`, atomic publish recovery, audit chain, and
  `gc plan`/`gc run`.
- Import and clone verification paths.
- Engine metadata preservation declarations.

Gate:

- Contract and conformance suites pass.
- Crash simulation proves incomplete checkpoints are invisible and repairable.
- Descriptor checksum, payload hash, lineage, and audit tamper tests fail with
  stable machine-readable error codes.

### Phase 3: Future trust layer

Potential deliverables:

- Trust keyring and policy under `.jvs/`.
- Descriptor signatures and trust status in verification output.
- Explicit trust evaluation in `verify` or `doctor`.

Gate:

- Unsigned, untrusted, revoked, and trusted checkpoint cases are covered.
- Private key material is never stored inside the repo.
- Trust behavior is documented as a new stable contract before release.

### Phase 4: GA release

Deliverables:

- Public docs and examples use final terminology.
- Performance results cover `juicefs-clone`, `reflink-copy`, and `copy`
  without unconditional claims.
- Migration notes explain historical terminology separately from stable docs.

Gate:

- `make release-gate` passes.
- `jvs doctor --strict` and `jvs verify --all` pass on representative repos.
- Release notes include known limitations and risk labels.
- No failed mandatory conformance test ships.

## Non-goals

JVS will not add these features to the v0 stable contract:

- Git-compatible commit graph, staging area, merge, rebase, or conflict
  resolution.
- Built-in remote hosting, push/pull protocol, object-store lifecycle
  management, or credential manager.
- Signing commands, signer trust policy, or key management as v0 stable CLI.
- Partial checkpoint contracts.
- Compression contracts.
- Complex retention policy flags beyond two-phase `gc plan` and
  `gc run --plan-id`.
- Hidden virtual workspaces or path remapping.
- Server-side authorization or multi-user locking as a substitute for external
  operational coordination.
