# JVS Product Plan

**Status:** active save point product direction

JVS is a local-first, filesystem-native CLI for saving real folders as save
points. This plan coordinates product, CLI, docs, implementation, QA, and
release work around that single public contract.

## Product Promise

JVS lets a human or agent save, inspect, view, restore, and recover work in a
real filesystem folder without a server, staging area, diff database, or
remote protocol.

Core promises:

- Local-first: operations work against local or mounted filesystem paths.
- Filesystem-native: a workspace is a real directory.
- Save point-first: the public saved unit is an immutable saved copy of
  managed files in one folder.
- Verifiable: published save points can be checked through descriptors,
  payload hashes, doctor checks, and audit records.
- Automation-safe: public state has deterministic CLI and JSON output.
- Recoverable: destructive restore runs are preview-first and have recovery
  plans.

## Public Terminology

Use these terms in public docs, help text, release notes, JSON fields where
feasible, and error messages.

| Term | Meaning |
| --- | --- |
| `folder` | The real filesystem directory where the user works. |
| `workspace` | The JVS name for a managed real folder. `main` is the default. |
| `JVS project` | The control data and workspace registry for save points. |
| `managed file` | A workspace file captured by save and managed by restore. |
| `JVS control/runtime state` | Non-payload data used by JVS for control and active operations; not saved, viewed, restored, or deleted as user payload. |
| `save point` | An immutable saved copy of managed files plus creation facts. |
| `save` | Create a save point from the active workspace. |
| `history` | List save points or find candidates. |
| `view` | Read-only view of a save point or path inside it. |
| `restore` | Copy a save point, or a path in it, into a real workspace. |
| `unsaved changes` | Managed files differ from known source state, or safety cannot be proven. |
| `cleanup` | Delete unprotected save point storage after preview and review. |
| `recovery plan` | Durable status/resume/rollback plan for interrupted restore. |

Internal package names, storage paths, and metadata fields do not define
product vocabulary, commands, selectors, examples, or fallback paths.

## Primary User Journey

```bash
jvs init
jvs save -m "baseline"
jvs history
jvs view <save> [path]
jvs restore <save>
```

Users should see the real folder first:

```text
Folder: /real/path
Workspace: main
Newest save point: none
Unsaved changes: yes
```

## Repository And Workspace Model

The adopted folder remains the user's working folder. JVS control data lives in
`.jvs/` or equivalent project storage and is never part of a save point.

Invariants:

- Workspace folders are real directories.
- JVS control data and runtime state are not user payload.
- A save point captures exactly one workspace's managed files.
- Published save point content and creation facts are immutable.
- Restore copies content into a workspace; it does not rewrite history.
- `workspace new <folder> --from <save>` starts a new workspace from source
  content at an explicit target folder but does not inherit the source history.
- JVS does not implement push, pull, merge, rebase, or server-side auth.

## Visible Public CLI

The visible root help surface is organized around the save point user journey:

```bash
jvs init [folder]
jvs save -m "message"
jvs history [--path <path>] [--limit <n>|-n <n>]
jvs history to <save>
jvs history from [<save>]
jvs view <save> [path]
jvs restore <save>
jvs restore <save> --path <path>
jvs restore --run <plan-id>
jvs recovery status [plan]
jvs recovery resume <plan>
jvs recovery rollback <plan>
jvs workspace new <folder> --from <save> [--name <name>]
jvs status
jvs doctor [--strict] [--repair-runtime] [--repair-list]
```

`completion` remains a utility command.

## Setup

`jvs init [folder]` adopts the shell working folder or an explicit folder. It
does not move or copy user files. It registers workspace `main`, reports engine
probe information, and starts with no save point.

Setup JSON reports stable engine and filesystem messaging such as
`effective_engine` and `warnings`.

## Save, History, And View

`jvs save -m "message"` creates a save point from managed files.

Rules:

- Save uses staging-before-publish and capacity gates.
- Failed saves do not publish partial save points.
- Save records provenance from workspace creation and restore when applicable.
- Save point IDs in machine output are full IDs.

`jvs history` lists save points for the workspace. `history to <save>` shows
history ending at a save point. `history from [<save>]` shows history starting
from a save point, or from the active workspace's current pointer when omitted.
`--limit`/`-n` control display length, and `--limit 0` means no limit.
`history --path <path>` returns candidates and next commands; it does not mutate
files.

`jvs view <save> [path]` opens a read-only view. Active views protect their
source save point from cleanup while active.

## Restore

Restore is preview-first:

```bash
jvs restore <save>
jvs restore <save> --path <path>
jvs restore --run <plan-id>
```

Rules:

- Preview computes file impact and writes a plan. It changes no files.
- Run binds to a plan and revalidates expected target state before writing.
- Whole-workspace restore replaces managed files from the save point.
- Path restore replaces only the selected workspace-relative path.
- History is not changed by restore.
- Unsaved changes are refused by default.
- `--save-first` preserves unsaved changes as a save point before run.
- `--discard-unsaved` discards unsaved changes for the operation scope.
- Interrupted restore produces a recovery plan with status/resume/rollback.

## Workspaces

```bash
jvs workspace new <folder> --from <save> [--name <name>]
```

Rules:

- `<folder>` is the explicit target path and must not already exist.
- The workspace name defaults to the target folder basename; `--name <name>`
  overrides it.
- The new workspace is a real folder.
- Source content is copied from the save point.
- The original workspace is unchanged.
- The new workspace has `Newest save point: none`.
- The first save in the new workspace records `started_from_save_point`.

Workspace selection is done by changing directories or using `--workspace`.
JVS does not virtualize the shell working directory.

Workspace removal is a preview-first reviewed-plan flow in the public
contract. Running a reviewed removal plan removes only the selected workspace
folder and registry entry, protects unsaved work, leaves save point storage
unchanged, and leaves deletion of unprotected save point storage to cleanup.

## Recovery

Restore recovery is a public workflow.

- `jvs recovery status` lists active recovery plans.
- `jvs recovery status <plan>` shows plan details and recommended next command.
- `jvs recovery resume <plan>` completes or retries the restore.
- `jvs recovery rollback <plan>` returns to the saved pre-restore state when
  evidence proves it is safe.

Active recovery plans block additional restore runs in the same workspace and
protect referenced source save points from cleanup.

## Doctor And Integrity

`jvs doctor --strict` validates repository health, runtime hygiene, publish
state, lineage/provenance consistency, integrity, and audit-chain health.

Public automatic repair is intentionally narrow:

- `clean_locks`
- `rebind_workspace_paths`
- `clean_runtime_tmp`
- `clean_runtime_operations`
- `clean_runtime_cleanup_plans`

Doctor must not rewrite durable save point history, workspace provenance, or
audit history as an automatic repair.
After a physical copy or storage migration, adopted `main` is rebound to the
current folder. External workspace folders rebind only with destination-local
content evidence.
Migration guidance uses an offline whole-folder copy of the managed
folder/repository as a whole to a fresh destination, followed on the destination
by `jvs doctor --strict --repair-runtime` and a fresh cleanup preview before
writes resume.

## Cleanup

Public product language is cleanup. Cleanup is two-stage reviewed deletion of
unprotected save point storage:

```text
cleanup preview -> cleanup run
```

Required semantics:

- Preview does not delete.
- Run binds to a reviewed plan and revalidates before deletion.
- Cleanup protects workspace history, open views, active recovery plans, and
  active operations.
- Cleanup does not delete workspace folders, user cache directories, JVS
  control data, or runtime state; it does not prune history or apply a
  retention policy.
- Labels do not protect save points.
- Deleting save point storage must preserve enough tombstone/audit information
  for later view/restore errors.

## Engine Transparency

JVS may choose different materialization engines per filesystem:

- `juicefs-clone` when JuiceFS clone support is available
- `reflink-copy` when filesystem copy-on-write is available
- `copy` as portable fallback

Users and automation must be able to see the effective engine and fallback
reason in setup output, status output, command JSON, or save point metadata.
Public docs must not make unconditional performance promises independent of
engine and filesystem support.

## JSON Envelope

With `--json`, every command emits exactly one JSON object to stdout:

```json
{
  "schema_version": 1,
  "command": "status",
  "ok": true,
  "repo_root": "/abs/folder",
  "workspace": "main",
  "data": {},
  "error": null
}
```

Rules:

- Exit code is zero only when `ok` is true.
- Paths are absolute canonical paths where a path is returned.
- Public save point fields use save point names where feasible, such as
  `save_point_id`, `newest_save_point`, `source_save_point`,
  `started_from_save_point`, and `history_head`.
- Internal storage fields may appear only as implementation details.

## Implementation Boundary

Implementation packages, storage paths, and metadata field names are not public
product vocabulary. Public docs, help text, release notes, JSON facades, and
error messages must use folder, workspace, save point, history, view, restore,
doctor, recovery, and cleanup terminology.

## Release Phases And Acceptance Gates

### Phase 0: Save Point Alignment

Deliverables:

- Public root help and active docs use save point vocabulary.
- Restore preview/run/recovery semantics are documented.

Gate:

- Product, CLI, docs, QA, and release agree that save point is the public
  contract.

### Phase 1: Core Save Point Tool

Deliverables:

- `init`, `save`, `history`, `view`, `restore`, `workspace new`, `status`,
  `recovery`, and `doctor`.
- Unsaved-change safety.
- Restore preview/run plan binding.
- Workspace new `started_from_save_point` behavior.
- Engine transparency for materialization.

Gate:

- Golden CLI tests cover public help, JSON envelopes, save point IDs,
  unsaved-change safety, and docs command smoke.

### Phase 2: Recovery And Integrity

Deliverables:

- Strong doctor/integrity checks.
- Restore recovery status/resume/rollback.
- Runtime repair limited to public safe actions.
- Migration and backup drills.

Gate:

- Crash simulation proves incomplete save/restore operations are invisible,
  repairable, or recoverable.

### Phase 3: Cleanup Controls

Deliverables:

- Cleanup preview semantics.
- Cleanup run semantics.
- Size impact and protection reasons.
- Tombstone/deleted-save behavior.
- Active view/recovery/operation protection.

Gate:

- Cleanup docs and evidence use save point terminology only.

### Phase 4: GA Release

Deliverables:

- Public docs and examples use final save point terminology.
- Changelog and release evidence identify public CLI changes honestly.
- Candidate evidence is clearly separate from final tagged release evidence.
- Performance claims remain engine-scoped.
- Migration notes explain internal storage names separately from public terms.

Gate:

- `make release-gate` passes.
- Doctor evidence passes on representative repos.
- Restore/recovery drill is recorded.
- No failed mandatory conformance test ships.

## Non-Goals

JVS will not add these to the public contract:

- Git-style commit graph, staging area, merge, rebase, or conflict
  resolution.
- Built-in remote hosting, push/pull protocol, object-store lifecycle
  management, or credential manager.
- Signing commands, signer trust policy, or key management as stable CLI.
- Label, message, or tag as direct restore/view targets.
- Public partial-save contracts.
- Public compression contracts.
- Complex retention policy flags.
- Hidden virtual workspaces or path remapping.
- Server-side authorization or multi-user locking as a substitute for external
  operational coordination.
