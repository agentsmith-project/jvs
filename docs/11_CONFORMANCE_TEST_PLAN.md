# Conformance Test Plan (v0)

## Purpose

Define the release-blocking public CLI contract tests for the v0 line. This
plan is intentionally scoped to observable behavior documented in
`docs/02_CLI_SPEC.md`; implementation layout and private storage choices are
out of scope.

## Profiles

- `contract`: fast checks for the public command surface and JSON shape.
- `release`: full contract, regression, integrity, and destructive-operation
  safety checks.

The `release` profile is mandatory before any pre-release or v0 tag.

## Mandatory Contract Areas

### Public Command Surface

- The documented commands exist and reject unsupported forms with stable error
  classes: `init`, `import`, `clone`, `capability`, `info`, `status`,
  `workspace list`, `workspace path`, `workspace rename`, `workspace remove`,
  `checkpoint`, `checkpoint list`, `diff`, `restore`, `fork`, `verify`,
  `doctor`, `gc plan`, `gc run`, and `completion`.
- Commands that document `--json` accept it; commands that do not document a
  flag reject it consistently.
- Global flags `--repo`, `--workspace`, `--json`, `--debug`, `--no-progress`,
  and `--no-color` are accepted according to the CLI spec.
- Public help and examples use the v0 vocabulary: repo, workspace,
  checkpoint, `current`, `latest`, and `dirty`.

### JSON Envelope

- Successful JSON output emits exactly one object to stdout with
  `schema_version`, `command`, `ok`, `repo_root`, `workspace`, `data`, and
  `error`.
- Failed JSON output sets `ok` to false, sets `data` to null, and returns an
  `error` object containing `code`, `message`, and, when useful, `hint`.
- Non-JSON diagnostics do not corrupt JSON stdout.
- Failure exits are non-zero and use stable machine-readable error classes.

### Repo and Workspace Resolution

- path-scoped setup commands (`init`, `import`, `clone`, and `capability`) are
  repo-free and resolve only their explicit path arguments.
- Repo-scoped and workspace-scoped commands require CWD to be inside a JVS
  repo and resolve the repository from the current path.
- For repo-scoped and workspace-scoped commands, `--repo` is an assertion, not
  an alternate discovery root; it must resolve to the current repo or a path
  inside the current repo.
- Workspace-scoped commands resolve the targeted workspace from the current
  path or `--workspace`.
- Running from the repo root and from nested workspace paths produces the same
  targeted state when the same workspace is selected.
- `workspace list`, `workspace rename`, and `workspace remove` are
  repo-scoped.
- `workspace path <name>` is repo-scoped; `workspace path` without a name is
  workspace-scoped.
- `jvs workspace path [name]` returns a canonical path.
- No command mutates the caller's shell CWD.

### Setup, Clone, and Capability

- `jvs init <repo-path>` creates a new repo with a `main` workspace and is
  rejected for invalid, overlapping, or unsafe paths.
- `jvs import <existing-dir> <repo-path>` copies user files into a new repo,
  creates an initial checkpoint tagged `import`, rejects sources containing
  `.jvs/`, and rejects overlapping source and destination paths.
- `jvs clone <source-repo> <dest-repo>` with `--scope current` copies live
  source workspace contents into the destination `main` workspace and creates
  one initial checkpoint tagged `clone`.
- `jvs clone <source-repo> <dest-repo>` with `--scope full` creates a new repo
  identity, preserves user-visible checkpoints and workspaces, and excludes
  active runtime operation state, including mutation lock directories,
  operation records, and active GC plans.
- `jvs clone` is validated as a local filesystem operation and does not expose
  remote push or pull semantics.
- `jvs capability <target-path>` reports engine support in both probe modes,
  including conservative results when `--write-probe` is omitted.
- Setup JSON reports stable filesystem and engine messaging: `capabilities`,
  `effective_engine`, `warnings`, and, for transfer setup commands,
  `transfer_engine` and `degraded_reasons`.
- Full clone JSON additionally reports `optimized_transfer`.

### Status, Refs, and State

- `jvs status --json` reports `current`, `latest`, `dirty`, `at_latest`,
  `workspace`, `repo`, `engine`, and `recovery_hints`.
- `current` means the checkpoint materialized in the targeted workspace.
- `latest` means the newest checkpoint on the targeted workspace's active
  line.
- `dirty` means uncheckpointed workspace contents and is never accepted as a
  checkpoint ref.
- Workspace commands accept `current`, `latest`, full checkpoint IDs, unique
  ID prefixes, and exact tags where refs are documented.
- Tags named `current`, `latest`, or `dirty` are rejected.

### Checkpoint, Diff, Restore, and Fork

- `jvs checkpoint` records the targeted workspace contents, note, repeated
  tags, and file-provided notes; it advances only when `current` is also
  `latest`.
- `jvs checkpoint list` marks `current` and `latest` in workspace context.
- `jvs diff <from> <to>` reports source and target IDs, times, changed paths,
  and changed-path totals.
- `jvs restore <ref>` replaces the targeted workspace with the selected
  checkpoint and updates `current` without changing unrelated workspaces.
- `jvs fork` supports `jvs fork <name>`, `jvs fork <ref> <name>`, and
  `jvs fork --from <ref> <name>` with equivalent ref resolution.

### Dirty Guards

- Destructive workspace operations refuse to overwrite uncheckpointed changes
  by default.
- `--include-working` creates a checkpoint for dirty contents before
  `restore` or `fork`.
- `--discard-dirty` discards dirty contents before `restore` or `fork`.
- `--include-working` and `--discard-dirty` are mutually exclusive.
- `workspace remove --force` is required when the workspace's `current`
  checkpoint differs from `latest`.

### Integrity, Doctor, and Retention

- `jvs verify [checkpoint-id]` and `jvs verify --all` detect descriptor and
  payload integrity failures and return structured result fields.
- `jvs doctor --strict` includes full checkpoint integrity and audit chain
  verification, and reports actionable repair information without inventing
  public state terms.
- `jvs gc plan` returns a stable plan ID, protection reasons, deletion
  candidates, and an estimated byte count.
- `jvs gc run --plan-id <id>` rejects mismatched or stale plan IDs.

## Acceptance

- The `release` profile requires 100% pass.
- Any failed mandatory contract area blocks release.
- Tests that touch destructive behavior must assert preserved user data on the
  default path and explicit behavior only when safety flags are provided.
- Contract tests and public docs must agree on command names, flags, JSON
  fields, refs, and public terminology.
