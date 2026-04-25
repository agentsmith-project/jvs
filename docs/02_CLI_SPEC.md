# CLI Spec (v0)

This spec defines the stable public JVS command surface for the current v0
line. The public vocabulary is repo, workspace, checkpoint, `current`,
`latest`, and `dirty`.

## Conventions

- Commands resolve a repository and, when needed, a workspace from the current
  path.
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

## Setup and Repo Commands

### `jvs init <repo-path> [--json]`

Create a new repo with `.jvs/` metadata and an empty `main` workspace.

### `jvs import <existing-dir> <repo-path> [--json]`

Create a new repo from an existing directory of user files and create an
initial checkpoint tagged `import`. The source must not contain `.jvs/`
metadata and must not overlap the destination repo.

### `jvs clone <source-repo> <dest-repo> [--scope full|current] [--json]`

Copy a local JVS repo to a new destination. `--scope full` copies the repo;
`--scope current` creates a new repo from the source's current workspace.
This is a local filesystem operation, not a remote protocol.

### `jvs capability <target-path> [--write-probe] [--json]`

Probe JuiceFS clone, reflink, and copy support for a target path. Without
`--write-probe`, the reflink result may be conservative.

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

List registered workspaces.

### `jvs workspace path [<name>]`

Print the canonical path for a workspace. Without `<name>`, the current
workspace is used.

### `jvs workspace rename <old> <new> [--json]`

Rename a workspace.

### `jvs workspace remove <name> [--force] [--json]`

Remove a workspace payload and metadata. Checkpoints remain available until
retention cleanup removes unprotected data. `--force` is required when the
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

Compute retention cleanup candidates without deleting them.

Required `data` fields:

- `plan_id`
- `protected_by_pin`
- `protected_by_lineage`
- `to_delete`: checkpoint IDs that would be deleted by `gc run`
- `deletable_bytes_estimate`

### `jvs gc run --plan-id <id>`

Run an accepted cleanup plan.

## v0 Boundaries

The stable v0 CLI does not include remote push/pull, signing or trust commands,
partial checkpoint contracts, compression contracts, merge/rebase, or complex
retention policy flags.
