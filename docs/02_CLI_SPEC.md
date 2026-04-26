# CLI Spec (v0)

This spec defines the stable public JVS command surface for the current v0
line. The public vocabulary is repo, workspace, checkpoint, `current`,
`latest`, and `dirty`.

## Conventions

- path-scoped setup commands (`init`, `import`, `clone`, and `capability`) are
  repo-free. They operate on explicit path arguments and do not require CWD to
  be inside a JVS repo.
- Repo-scoped and workspace-scoped commands require CWD to be inside a JVS
  repo and resolve that repo from the current path.
- For repo-scoped and workspace-scoped commands, `--repo` is an assertion, not
  an alternate discovery root. It must resolve to the current repo or to a path
  inside the current repo.
- Global flags: `--repo <repo-root>`, `--workspace <name>`, `--json`,
  `--debug`, `--no-progress`, and `--no-color`.
- Non-zero exit means failure.
- `--json` emits exactly one JSON object to stdout.
- JVS does not mutate the caller's shell CWD.

## JSON Envelope

JSON output uses this envelope:

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": true,
  "repo_root": "/abs/repo",
  "workspace": "main",
  "data": {},
  "error": null
}
```

On error, `ok` is false, `data` is null, and `error` contains:

```json
{
  "code": "E_NOT_REPO",
  "message": "not a JVS repository",
  "hint": "run `jvs init <repo-path>`"
}
```

## Refs and Workspace State

Checkpoint refs accepted by workspace commands:

- `current`: the checkpoint currently materialized in the targeted workspace.
- `latest`: the newest checkpoint on the workspace's active line.
- A full checkpoint ID.
- A unique checkpoint ID prefix.
- An exact tag that resolves to one checkpoint.

`dirty` is reserved for uncheckpointed workspace contents and is not a
checkpoint ref. Notes/messages are not refs.

## Setup Commands

### `jvs init <repo-path> [--json]`

Create a new repo with `.jvs/` metadata and an empty `main` workspace at the
explicit path. This is a path-scoped setup command and is repo-free.

### `jvs import <existing-dir> <repo-path> [--json]`

Create a new repo from an existing directory of user files and create an
initial checkpoint tagged `import`. The source must not contain `.jvs/`
metadata and must not overlap the destination repo. This is a path-scoped
setup command and is repo-free.

### `jvs clone <source-repo> <dest-repo> [--scope full|current] [--json]`

Copy a local JVS repo to a new destination. This is a path-scoped setup
command and is repo-free. It is a local filesystem operation, not a remote
protocol.

`--scope current` copies the live source workspace contents into the
destination repo's `main` workspace, creates exactly one initial checkpoint
tagged `clone`, and makes that checkpoint both `current` and `latest`. It does
not preserve source history or other workspaces.

`--scope full` creates a new repo identity while preserving user-visible
checkpoints, workspaces, and refs from the source repo. It must exclude active
runtime operation state, including mutation lock directories, operation
records, and active GC plans, and rebuild clean runtime directories in the
destination.

### `jvs capability <target-path> [--write-probe] [--json]`

Probe JuiceFS clone, reflink, and copy support for a target path. This is a
path-scoped setup command and is repo-free. Without `--write-probe`, the
reflink result may be conservative.

### Setup JSON

Setup commands that support `--json` must expose stable engine and filesystem
messaging:

- `capabilities`: the target or destination capability report.
- `effective_engine`: the engine expected for subsequent materialization in a
  created repo, or the recommended engine for `capability`.
- `warnings`: user-visible filesystem or engine caveats; empty when there are
  none.
- `transfer_engine`: for `import` and `clone`, the engine or transfer strategy
  used to copy source data.
- `degraded_reasons`: for `import` and `clone`, machine-readable reasons a
  requested optimized transfer fell back or preserved less metadata.
- `optimized_transfer`: for `clone --scope full`, true only when the full clone
  used an optimized filesystem transfer rather than portable recursive copy.

## Repo Commands

### `jvs info [--json]`

Show repo metadata and engine summary.

Required `data` semantics:

- repo root
- repo ID
- format version
- selected engine summary
- workspace count
- checkpoint count

## Utility Commands

### `jvs completion <bash|zsh|fish|powershell>`

Generate a shell completion script for the requested shell. This command does
not require a repo or workspace and does not define JSON output.

## Workspace Commands

`workspace list`, `workspace rename`, and `workspace remove` are repo-scoped.
`workspace path <name>` is repo-scoped. `workspace path` without `<name>` is
workspace-scoped and resolves the current workspace from CWD or
`--workspace`.

### `jvs status [--json]`

Show the targeted workspace state.

Required `data` fields:

- `current`
- `latest`
- `dirty`
- `at_latest`
- `workspace`
- `repo`
- `engine`
- `recovery_hints`

### `jvs workspace list [--json]`

List registered workspaces in the resolved repo.

### `jvs workspace path [<name>]`

Print the canonical path for a workspace. Without `<name>`, the current
workspace is used.

### `jvs workspace rename <old> <new> [--json]`

Rename a workspace in the resolved repo.

### `jvs workspace remove <name> [--force] [--json]`

Remove a workspace payload and metadata. Checkpoints remain available until
GC removes unprotected data. `--force` is required when the
workspace current checkpoint differs from latest.

## Checkpoint Commands

### `jvs checkpoint [note] [--tag <tag>]... [--file <path>] [--json]`

Create a checkpoint from the current workspace. `--tag` may be repeated. Use
`--file` or `-F` to read the note from a file.

Rules:

- A checkpoint may advance only when the workspace current checkpoint is also
  latest.
- Tags must not be `current`, `latest`, or `dirty`.
- Public v0 docs do not define partial checkpoint or compression contracts.

### `jvs checkpoint list [--json]`

List checkpoints. When run inside a workspace, human output marks entries that
are `current` or `latest`.

### `jvs diff <from> <to> [--stat] [--json]`

Compare two checkpoints. Refs can be `current`, `latest`, full checkpoint IDs,
unique ID prefixes, or exact tags.

Required `data` semantics:

- source checkpoint ID
- target checkpoint ID
- source and target checkpoint times
- added paths
- removed paths
- modified paths
- added, removed, and modified totals

## Restore and Fork Commands

### `jvs restore <ref|current|latest> [--discard-dirty|--include-working] [--json]`

Replace the live workspace contents with the requested checkpoint.

Dirty safety:

- Restore refuses to overwrite dirty work by default.
- `--include-working` creates a checkpoint of dirty work before restoring.
- `--discard-dirty` discards dirty work and restores the target checkpoint.
- The two dirty flags are mutually exclusive.

### `jvs fork [<ref> <name>|<name>] [--from <ref>] [--discard-dirty|--include-working] [--json]`

Create another workspace from a checkpoint.

Forms:

- `jvs fork <name>` creates `<name>` from `current`.
- `jvs fork <ref> <name>` creates `<name>` from `<ref>`.
- `jvs fork --from <ref> <name>` is the explicit form.

Fork uses the same dirty-safety flags as restore.

## Verification, Doctor, and Retention

### `jvs verify [<checkpoint-id>] [--all] [--json]`

Verify descriptor checksum and payload root hash. With no argument or with
`--all`, verify all checkpoints.

Required `data` fields for each result:

- `checksum_valid`
- `payload_hash_valid`
- `tamper_detected`

### `jvs doctor [--strict] [--repair-runtime] [--repair-list] [--json]`

Check repository health. `--strict` includes full integrity verification.
`--repair-runtime` runs safe automatic runtime cleanup. `--repair-list` prints
available repair actions.

### `jvs gc plan [--json]`

Compute cleanup candidates without deleting them. v0 GC protects live workspace
lineage and active operation records; it does not expose public pin or retention
policy fields.

Required `data` fields:

- `plan_id`
- `created_at`: plan creation timestamp
- `protected_checkpoints`: checkpoint IDs protected by v0 GC safety rules
- `candidate_count`
- `protected_by_lineage`
- `to_delete`: checkpoint IDs that would be deleted by `gc run`
- `deletable_bytes_estimate`

`protected_checkpoints` is informational and contains the public checkpoint IDs
that v0 GC kept because they are reachable from live workspace roots or active
operation records. It is not a pin, tag-retention, or retention-policy surface.

### `jvs gc run --plan-id <id>`

Run an accepted cleanup plan.

## v0 Boundaries

The stable v0 CLI does not include remote push/pull, signing or trust commands,
partial checkpoint contracts, compression contracts, merge/rebase, or complex
retention policy flags.
