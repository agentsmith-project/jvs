# Changelog

This changelog is release-facing. Earlier draft material is not carried forward
as active reader content for the published GA line. The active product
vocabulary is folder, workspace, save point, save, history, view, restore,
recovery plan, doctor, and cleanup.

## v0.4.7 - 2026-05-05

### Highlights

- GA candidate readiness now covers the story-e2e gate as a release-facing
  aggregation point: `make story-e2e` is checked so every regular `TestStory`
  user story is selected by the gate before publication.
- The ordinary embedded repo clone user story is covered end to end, including
  source repo identity preservation, fresh target repo identity, `main`
  workspace usability after clone, `--save-points all` / `--save-points main`,
  follow-on target saves, and source repo isolation.
- External control root coverage now includes the workspace-cwd explicit
  selector flow: from the workspace folder, `--control-root` plus
  `--workspace main` drives status, save, history, view, view close, restore
  preview, and restore run while control data stays in the control root.
- Public transfer fallback/degraded JSON cleanliness is covered for the
  `juicefs-clone` requested-engine path when the optimized engine is
  unavailable and the command falls back to normal `copy` behavior.
- Public transfer JSON remains clean in fallback/degraded cases: pure JSON
  output keeps user-facing `degraded_reasons`, `warnings`, transfer roles,
  materialization destinations, and published destinations without leaking
  internal `.jvs`, content storage, or stdout/stderr details.
- Release-facing identity remains `github.com/agentsmith-project/jvs`; release
  URLs use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- None for the stable v0 public CLI contract.
- This main-branch version bump is a GA candidate/readiness record only; it
  does not create a final tag, publish release artifacts, or change the on-disk
  repository layout version.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- v0 does not yet include a first-class user-facing portability or backup
  workflow.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the fresh destination with
  `jvs doctor --strict --repair-runtime`.
- `recovery`: interrupted lifecycle operations and restore runs are expected to
  fail closed and report recovery evidence before another mutating operation
  continues in the same repo or workspace.
- `usability`: release readiness now depends on the story-e2e gate selecting
  every regular `TestStory` user story, plus explicit-selector coverage for
  external control root workflows and clean fallback/degraded transfer JSON.

### Migration notes

- Existing repositories do not need an on-disk migration for the `v0.4.7`
  candidate target; `.jvs/format_version` remains a repository layout version,
  not the application release version.
- This candidate records story-e2e gate, embedded repo clone, external control
  root workspace-cwd selector, and public transfer fallback/degraded JSON
  readiness for existing v0 repository storage; it does not add a compatibility
  mode or change repository storage.
- After upgrading to this candidate, run `jvs doctor --strict` on a
  representative repo before relying on it for release workflows.
- For physical backup or storage migration, start with a fresh destination, run
  an offline whole-folder copy, run
  `jvs doctor --strict --repair-runtime` at the destination, then run a fresh
  cleanup preview before any cleanup run.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- User-facing portability and backup workflow remains a documented product gap,
  not a new v0 CLI promise.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v047---2026-05-05)
  for the `v0.4.7` GA candidate readiness record.
- Evidence class: GA candidate readiness.
- Candidate target tag: `v0.4.7`
- Candidate state: not final, not tagged, and not published; the release is
  pending final tag creation and publication through the normal CI release
  flow.
- Source archive boundary: no immutable `v0.4.7` tag source archive exists yet; when
  the pending final tag is created, its source archive will record readiness
  from tag time.
- publication final evidence remains pending; the future GitHub Release page
  and post-release main ledger will record workflow run, release state,
  artifacts, checksums, signing identity, smoke, and coverage facts after the
  release exists.
- Readiness scope since `v0.4.6`: story-e2e gate coverage for every regular
  `TestStory` user story, ordinary embedded repo clone user story coverage,
  external control root workspace-cwd explicit selector flow coverage, and
  public transfer fallback/degraded JSON cleanliness for optimized-engine
  fallback reporting.

### Release artifacts

- No final `v0.4.7` release artifacts are published from this main-branch
  candidate entry.
- Artifact plan for the pending final tag remains the standard five platform
  binaries, matching `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`.
- Signing remains outside in-JVS commands and is expected to use the release
  workflow's Sigstore/cosign v3 bundle flow.
- Final artifact, checksum, signing, and smoke evidence will be recorded only
  after CI creates the release from the final tag.

## v0.4.6 - 2026-05-03

### Highlights

- GA candidate readiness now covers repo/workspace lifecycle management:
  repo move/rename/detach and workspace move/rename/delete all have
  preview/run and recovery posture recorded for the public contract.
- Repo and workspace lifecycle operations now preserve safer handoffs across
  folder moves and name changes, including destination checks, registered
  workspace updates, detach metadata, and fail-closed recovery evidence.
- External workspace pending lifecycle evidence is surfaced through status and
  doctor with a machine-readable `recommended_next_command`, so interrupted
  repo or workspace lifecycle work points to the same command the user should
  rerun.
- The repo clone workflow is covered as a user-facing flow, including imported
  clone history protection and follow-on cleanup boundaries.
- Filesystem-aware transfer planning/implementation is covered across save,
  view, restore, workspace creation, and clone paths so copy behavior matches
  the destination while preserving preview/run review points.
- Pre-GA public vocabulary cleanup is now release-evidenced across the Go
  facade, machine-readable error codes, transfer JSON public references, and
  transfer free-text sanitizer output.
- Release-facing identity remains `github.com/agentsmith-project/jvs`; release
  URLs use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- This is a pre-GA public vocabulary cleanup and clean break for automation
  that already integrated with candidate builds; no backward-compatible aliases
  are provided.
- Public Go facade consumers must read `ContentRootHash` /
  `content_root_hash` from `pkg/jvs.SavePoint` metadata; the earlier candidate
  hash field is not kept as an alias.
- Error-code matchers must use the public `E_SAVE_POINT_*` and `E_CLEANUP_*`
  families.
- CLI JSON consumers must treat transfer JSON public references and free-text
  sanitizer output as content/save point vocabulary, including stable
  references such as `save_point:<id>`, `content_view:<view-id>[/path]`,
  `control_data`, and `temporary_folder`.
- This main-branch version bump is a GA candidate/readiness record only; it
  does not create a final tag, publish release artifacts, or change the on-disk
  repository layout version.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- v0 does not yet include a first-class user-facing portability or backup
  workflow.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the fresh destination with
  `jvs doctor --strict --repair-runtime`.
- `recovery`: interrupted lifecycle operations and restore runs are expected to
  fail closed and report recovery evidence before another mutating operation
  continues in the same repo or workspace.
- `usability`: lifecycle commands rely on preview/run review, explicit target
  folders or names, and `recommended_next_command` guidance when recovery is
  needed.

### Migration notes

- Existing repositories do not need an on-disk migration for the `v0.4.6`
  candidate target; `.jvs/format_version` remains a repository layout version,
  not the application release version.
- This candidate records repo/workspace lifecycle, repo clone, and transfer
  planning readiness for existing v0 repository storage; it does not add a
  compatibility mode or change repository storage.
- After upgrading to this candidate, run `jvs doctor --strict` on a
  representative repo before relying on it for release workflows.
- For physical backup or storage migration, start with a fresh destination, run
  an offline whole-folder copy, run
  `jvs doctor --strict --repair-runtime` at the destination, then run a fresh
  cleanup preview before any cleanup run.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- User-facing portability and backup workflow remains a documented product gap,
  not a new v0 CLI promise.
- Automation built against earlier candidate vocabulary must update public field
  and code matches to `ContentRootHash` / `content_root_hash`,
  `E_SAVE_POINT_*`, `E_CLEANUP_*`, and the content-based transfer JSON
  summaries; this clean break does not include a compatibility alias layer.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v046---2026-05-03)
  for the `v0.4.6` GA candidate readiness record.
- Evidence class: GA candidate readiness.
- Candidate target tag: `v0.4.6`
- Candidate state: not final, not tagged, and not published; the release is
  pending final tag creation and publication through the normal CI release
  flow.
- Source archive boundary: no immutable `v0.4.6` tag source archive exists yet; when
  the pending final tag is created, its source archive will record readiness
  from tag time.
- publication final evidence remains pending; the future GitHub Release page
  and post-release main ledger will record workflow run, release state,
  artifacts, checksums, signing identity, smoke, and coverage facts after the
  release exists.
- Readiness scope since `v0.4.5`: repo/workspace lifecycle management,
  repo move/rename/detach, workspace move/rename/delete preview/run and
  recovery posture, external workspace pending lifecycle evidence,
  machine-readable `recommended_next_command`, repo clone workflow, and
  filesystem-aware transfer planning/implementation, plus pre-GA public
  vocabulary cleanup for `ContentRootHash` / `content_root_hash`,
  `E_SAVE_POINT_*`, `E_CLEANUP_*`, transfer JSON public references, and
  free-text sanitizer output.

### Release artifacts

- No final `v0.4.6` release artifacts are published from this main-branch
  candidate entry.
- Artifact plan for the pending final tag remains the standard five platform
  binaries, matching `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`.
- Signing remains outside in-JVS commands and is expected to use the release
  workflow's Sigstore/cosign v3 bundle flow.
- Final artifact, checksum, signing, and smoke evidence will be recorded only
  after CI creates the release from the final tag.

## v0.4.5 - 2026-05-01

### Highlights

- GA candidate readiness for workspace user stories: conformance now follows
  the ordinary path of creating an explicit sibling workspace folder from a
  save point, entering that folder, and using status, save, history, and
  restore from there.
- Workspace visibility is covered end to end across more than one workspace:
  list, path, and status show real folders, current pointers, newest save
  points, started-from sources, and unsaved changes without extra vocabulary.
- History coverage matches the public model: `jvs history to <save>` looks
  backward, `jvs history from [<save>]` looks forward, and
  `jvs history from` inside a workspace starts from that workspace's source
  while the workspace label moves to the new save point after saving.
- Explicit folder safety is covered: bare name-shaped workspace creation and
  workspace folders created inside an existing workspace are rejected,
  `../analysis-run` succeeds, and `--name` can name the workspace independently
  of the folder basename.
- Release-facing identity remains `github.com/agentsmith-project/jvs`; release
  URLs use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- None for the stable v0 public CLI contract.
- This main-branch version bump is a GA candidate/readiness record only; it
  does not create a final tag, publish release artifacts, or change the on-disk
  repository layout version.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- v0 does not yet include a first-class user-facing portability or backup
  workflow.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the fresh destination with
  `jvs doctor --strict --repair-runtime`.
- `recovery`: restore run remains recoverable through recovery plans, but
  operators must resolve active plans before starting another restore in the
  same workspace.
- `usability`: workspace workflows rely on explicit folder choices, folder-local
  commands, and clear history direction, so user-story coverage now exercises
  those paths directly.

### Migration notes

- Existing repositories do not need an on-disk migration for the `v0.4.5`
  candidate target; `.jvs/format_version` remains a repository layout version,
  not the application release version.
- This candidate records workspace user-story coverage/readiness for the
  existing explicit-folder workspace behavior; it does not add a compatibility
  mode or change repository storage.
- After upgrading to this candidate, run `jvs doctor --strict` on a
  representative repo before relying on it for release workflows.
- For physical backup or storage migration, start with a fresh destination, run
  an offline whole-folder copy, run
  `jvs doctor --strict --repair-runtime` at the destination, then run a fresh
  cleanup preview before any cleanup run.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- User-facing portability and backup workflow remains a documented product gap,
  not a new v0 CLI promise.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v045---2026-05-01)
  for the `v0.4.5` GA candidate readiness record.
- Evidence class: GA candidate readiness.
- Candidate target tag: `v0.4.5`
- Candidate state: not final, not tagged, and not published; the release is
  pending final tag creation and publication through the normal CI release
  flow.
- Source archive boundary: no immutable `v0.4.5` tag source archive exists yet; when
  the pending final tag is created, its source archive will record readiness
  from tag time.
- publication final evidence remains pending; the future GitHub Release page
  and post-release main ledger will record workflow run, release state,
  artifacts, checksums, signing identity, smoke, and coverage facts after the
  release exists.
- Readiness scope since `v0.4.4`: workspace user-story coverage/readiness for
  explicit workspace folder creation, folder-local status/save/history/restore,
  multi-workspace list/status/path, `jvs history from` default source and
  workspace pointer movement, rejection of implicit workspace creation, and
  `--name`/folder basename decoupling.

### Release artifacts

- No final `v0.4.5` release artifacts are published from this main-branch
  candidate entry.
- Artifact plan for the pending final tag remains the standard five platform
  binaries, matching `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`.
- Signing remains outside in-JVS commands and is expected to use the release
  workflow's Sigstore/cosign v3 bundle flow.
- Final artifact, checksum, signing, and smoke evidence will be recorded only
  after CI creates the release from the final tag.

## v0.4.4 - 2026-04-30

### Highlights

- GA candidate readiness for user documentation discoverability: the user docs
  index and release-facing docs index now point new users to Best Practices as
  the everyday routine after Quickstart.
- More non-technical tutorials cover client delivery packages, media sorting
  sessions, and course or research materials, so users can practice save,
  view, path restore, and preview/run workflows without adopting a developer
  or data-science scenario first.
- Workflow placeholder clarity is now guarded by conformance: workflow pages
  must explain typed placeholders or link to a user-doc explanation, and
  `<view-path>` remains the canonical placeholder for the path printed by
  `jvs view`.
- Product gap tracking now records a future user-facing portability and backup
  workflow so ordinary folder move/copy expectations stay visible without
  changing the v0 GA surface.
- Release-facing identity remains `github.com/agentsmith-project/jvs`; release
  URLs use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- None for the stable v0 public CLI contract.
- This main-branch version bump is a GA candidate/readiness record only; it
  does not create a final tag, publish release artifacts, or change the on-disk
  repository layout version.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- v0 does not yet include a first-class user-facing portability or backup
  workflow.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the fresh destination with
  `jvs doctor --strict --repair-runtime`.
- `recovery`: restore run remains recoverable through recovery plans, but
  operators must resolve active plans before starting another restore in the
  same workspace.
- `usability`: user workflows rely on clear placeholder names and
  preview-before-run habits, so user-doc workflow pages now carry explicit
  placeholder explanations and conformance coverage.

### Migration notes

- Existing repositories do not need an on-disk migration for the `v0.4.4`
  candidate target; `.jvs/format_version` remains a repository layout version,
  not the application release version.
- After upgrading to this candidate, run `jvs doctor --strict` on a
  representative repo before relying on it for release workflows.
- For physical backup or storage migration, start with a fresh destination, run
  an offline whole-folder copy, run
  `jvs doctor --strict --repair-runtime` at the destination, then run a fresh
  cleanup preview before any cleanup run.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- User-facing portability and backup workflow remains a documented product gap,
  not a new v0 CLI promise.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v044---2026-04-30)
  for the `v0.4.4` GA candidate readiness record.
- Evidence class: GA candidate readiness.
- Candidate target tag: `v0.4.4`
- Candidate state: not final, not tagged, and not published; the release is
  pending final tag creation and publication through the normal CI release
  flow.
- Source archive boundary: no immutable `v0.4.4` tag source archive exists yet; when
  the pending final tag is created, its source archive will record readiness
  from tag time.
- publication final evidence remains pending; the future GitHub Release page
  and post-release main ledger will record workflow run, release state,
  artifacts, checksums, signing identity, smoke, and coverage facts after the
  release exists.
- Readiness scope since `v0.4.3`: Best Practices user entry, non-technical
  tutorials, workflow placeholder conformance, user-doc index updates, and the
  portability and backup workflow product gap record.

### Release artifacts

- No final `v0.4.4` release artifacts are published from this main-branch
  candidate entry.
- Artifact plan for the pending final tag remains the standard five platform
  binaries, matching `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`.
- Signing remains outside in-JVS commands and is expected to use the release
  workflow's Sigstore/cosign v3 bundle flow.
- Final artifact, checksum, signing, and smoke evidence will be recorded only
  after CI creates the release from the final tag.

## v0.4.3 - 2026-04-29

### Highlights

- GA candidate readiness for cleanup public boundary hardening: active docs and
  conformance checks keep cleanup protection reasons on the stable public
  surface and avoid leaking internal runtime mechanisms.
- GA safety and clarity hardening: release-facing docs keep strict doctor,
  integrity, recovery, and runtime repair guidance explicit before release
  operations.
- Migration whole-folder copy hardening: backup and migration examples now
  fail closed around a fresh destination, use an offline whole-folder copy, and
  rebuild destination-local runtime state with
  `jvs doctor --strict --repair-runtime`.
- Compact shell function guard scope coverage keeps migration guard examples
  narrow while preserving the v0 boundary against remote transfer behavior.
- Release-facing identity remains `github.com/agentsmith-project/jvs`; release
  URLs use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- None for the stable v0 public CLI contract.
- This main-branch version bump is a GA candidate/readiness record only; it
  does not create a final tag, publish release artifacts, or change the on-disk
  repository layout version.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the fresh destination with
  `jvs doctor --strict --repair-runtime`.
- `recovery`: restore run remains recoverable through recovery plans, but
  operators must resolve active plans before starting another restore in the
  same workspace.

### Migration notes

- Existing repositories do not need an on-disk migration for the `v0.4.3`
  candidate target; `.jvs/format_version` remains a repository layout version,
  not the application release version.
- After upgrading to this candidate, run `jvs doctor --strict` on a
  representative repo before relying on it for release workflows.
- For physical backup or storage migration, start with a fresh destination, run
  an offline whole-folder copy, run
  `jvs doctor --strict --repair-runtime` at the destination, then run a fresh
  cleanup preview before any cleanup run.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v043---2026-04-29)
  for the `v0.4.3` GA candidate readiness record.
- Evidence class: GA candidate readiness.
- Candidate target tag: `v0.4.3`
- Candidate state: not final, not tagged, and not published; the release is
  pending final tag creation and publication through the normal CI release
  flow.
- Source archive boundary: no immutable `v0.4.3` tag source archive exists yet; when
  the pending final tag is created, its source archive will record readiness
  from tag time.
- publication final evidence remains pending; the future GitHub Release page
  and post-release main ledger will record workflow run, release state,
  artifacts, checksums, signing identity, smoke, and coverage facts after the
  release exists.
- Readiness scope since `v0.4.2`: cleanup public boundary hardening, GA
  safety/clarity, migration whole-folder copy fail-closed docs/conformance
  hardening, and compact shell function guard scope coverage.

### Release artifacts

- No final `v0.4.3` release artifacts are published from this main-branch
  candidate entry.
- Artifact plan for the pending final tag remains the standard five platform
  binaries, matching `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`.
- Signing remains outside in-JVS commands and is expected to use the release
  workflow's Sigstore/cosign v3 bundle flow.
- Final artifact, checksum, signing, and smoke evidence will be recorded only
  after CI creates the release from the final tag.

## v0.4.2 - 2026-04-28

### Highlights

- GA release for the save point public contract. Visible help and active specs
  lead with `init`, `save`, `history`, `view`, `restore`, `workspace new`,
  `cleanup`, `recovery`, `status`, and `doctor`.
- Restore is preview-first: `jvs restore <save>` creates a plan and changes no
  files; `jvs restore --run <restore-plan-id>` revalidates and applies the
  reviewed restore plan; `jvs recovery status|resume|rollback` closes
  interrupted restore workflows.
- `jvs workspace new <folder> --from <save>` creates another real workspace
  folder at an explicit path, leaves the source workspace unchanged, starts
  with `Newest save point: none`, and records `started_from_save_point` on
  first save. `--name <name>` only overrides the default workspace name.
- `jvs cleanup preview` creates a reviewed deletion plan for unprotected save
  point storage; `jvs cleanup run --plan-id <plan>` revalidates and runs the
  reviewed plan.
- Release-facing identity is `github.com/agentsmith-project/jvs`; release URLs
  use the canonical GitHub project, for example
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`.

### Breaking changes

- Active docs and visible help now present the save point contract as the only
  public user surface.
- Restore examples must use preview/run and the public safety flags
  `--save-first` and `--discard-unsaved`.
- Workspace creation examples must use `jvs workspace new <folder> --from
  <save>` with an explicit folder path.

### Known limitations

- v0 does not include remote push/pull.
- v0 does not include in-JVS signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- Strict integrity checks can be I/O intensive on large workspaces.
- Descriptor signing and in-JVS trust policy remain outside the stable v0
  repository format.

### Risk labels

- `integrity`: descriptor checksum and content hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: non-portable JVS runtime state is destination-local and must be
  rebuilt at the destination with `jvs doctor --strict --repair-runtime`.
- `recovery`: restore run is recoverable through recovery plans, but operators
  must resolve active plans before starting another restore in the same
  workspace.

### Migration notes

- Existing repositories do not need an on-disk migration for `v0.4.2`;
  `.jvs/format_version` remains a repository layout version, not the
  application release version.
- After upgrading, run `jvs doctor --strict` on a representative repo before
  relying on it for release workflows.
- After a physical backup or storage migration, run
  `jvs doctor --strict --repair-runtime` at the destination before the final
  strict doctor check.
- Do not treat non-portable JVS runtime state as authoritative during physical
  backup or storage migration; rebuild it at the destination before resuming
  JVS writes.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v042---2026-04-28)
  for the `v0.4.2` final GA release evidence record.
- Source archive boundary: the `v0.4.2` source archive is the immutable source archive
  for the release and records readiness from tag time.
- Tag source archive evidence class: `GA candidate readiness`
- publication final evidence is recorded on the GitHub Release page and in the
  post-release main ledger after the release exists.
- Final evidence location: GitHub Release page and post-release main ledger.
- Tag movement: `v0.4.2` was not moved; the tag was not moved to add
  post-publication facts.
- Final tag `v0.4.2` points at commit
  `c21b676dfb04d32f8cf3b9fa301e465f6886ca94`
  (`ci: publish release signatures as bundles`).
- Tag workflow run `25056873829` succeeded:
  `https://github.com/agentsmith-project/jvs/actions/runs/25056873829`.
  Passed jobs were Build and Test, Lint, Security Scan, Release Toolchain
  Smoke, Release Gate, and Release.
- Local final release gate passed with
  `env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`;
  coverage was `68.7% >= 60%`.
- Representative repo, strict doctor, integrity, restore drill, recovery drill,
  and runtime repair evidence are recorded as release-gate suite coverage, not
  as a separate external repository claim.
- Release state: `draft=false`, `prerelease=false`.

### Release artifacts

- Release URL:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2`
- Asset count: `12`
- Published assets: `jvs-darwin-amd64`, `jvs-darwin-amd64.bundle`,
  `jvs-darwin-arm64`, `jvs-darwin-arm64.bundle`, `jvs-linux-amd64`,
  `jvs-linux-amd64.bundle`, `jvs-linux-arm64`, `jvs-linux-arm64.bundle`,
  `jvs-windows-amd64.exe`, `jvs-windows-amd64.exe.bundle`, `SHA256SUMS`, and
  `SHA256SUMS.bundle`.
- Published asset validation after release download to
  `/tmp/jvs-release-v0.4.2`: `sha256sum --check --strict SHA256SUMS` returned
  OK for all five binaries.
- Linux binary smoke: `./jvs-linux-amd64 --help` printed current save point help
  and exited successfully.
- Local signature verification used cosign `v3.0.5`, certificate identity
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.2`,
  and issuer `https://token.actions.githubusercontent.com`; verification was
  OK for `jvs-linux-amd64`, `jvs-linux-arm64`, `jvs-darwin-amd64`,
  `jvs-darwin-arm64`, `jvs-windows-amd64.exe`, and `SHA256SUMS` with matching
  `.bundle` files.
