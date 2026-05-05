# Release Evidence Ledger

This ledger records compact release evidence for the active save point product
contract. Earlier draft material is not carried forward as active release
evidence for the published GA line.

Evidence model:

- Tag/source archive readiness: a source archive is the immutable source archive
  captured when the tag is created. It may contain readiness or candidate
  evidence from that moment, and it must not be rewritten to add facts created
  by release publication.
- publication final evidence: the GitHub Release page plus the post-release
  main ledger on `main`. This layer records publication facts such as workflow
  run, release state, published assets, checksums, signing identity, smoke
  results, and coverage after the release exists.

Evidence classes:

- GA candidate readiness: pre-tag evidence for a release candidate. It may
  record required commands, expected artifact rules, and pending final checks,
  but it is not final, not tagged, and not published. Candidate entries must
  not record final `Status: PASS`, final tag lines, final tagged commits, or
  published artifact counts.
- Final release evidence: immutable evidence for an existing tag. Only this
  class may record final pass status, a final tag, a final tagged commit, and
  published artifact counts.

Raw logs and `coverage.out` are not stored here.

## v0.4.7 - 2026-05-05

### Release identity

- Evidence class: Final release evidence
- Status: PASS
- Tag: `v0.4.7`
- Final tagged commit: `e098b6a1bb10afb815258caa850e1ff187c5cacc`
- Commit message: `fix: satisfy release lint gate`
- Tag object: `58f1a7d2881c48c42c3b0ea34dbfef059486ada8`
- Annotated tag subject: `Release v0.4.7`
- Tagger date: `2026-05-04 19:46:34 -0700`
- Changelog heading date: `2026-05-05`
- Baseline: story-e2e gate coverage for every regular `TestStory` user story,
  ordinary embedded repo clone user story coverage, external control root
  workspace-cwd explicit selector flow coverage, and public transfer
  fallback/degraded JSON cleanliness for optimized-engine fallback reporting.
- Source archive boundary: the `v0.4.7` source archive is the immutable source archive
  for the release and records readiness from tag time.
- Tag source archive evidence class: `GA candidate readiness`
- publication final evidence: GitHub Release page and post-release main ledger
  record the workflow run, release state, assets, checksum validation, signing
  identity, smoke, and coverage facts created after publication.
- Final evidence location: GitHub Release page and post-release main ledger.
- Tag movement: `v0.4.7` was not moved; the tag was not moved to add
  post-publication facts.
- CI run link rule:
  `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Canonical release URL rule:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.7`
- Canonical release URL example:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`
- Tag workflow run: `25355112705`
- Tag workflow URL:
  `https://github.com/agentsmith-project/jvs/actions/runs/25355112705`
- Tag workflow result: success.
- Tag workflow jobs passed: Build and Test, Lint, Security Scan, Release
  Toolchain Smoke, Release Gate, and Release. DCO skipped.
- Release URL:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.7`
- Release state: `draft=false`, `prerelease=false`
- Published at: `2026-05-05T02:51:57Z`
- Release listing: GitHub release list shows `v0.4.7` as Latest.
- Scope: final release evidence for the `make story-e2e` gate selecting all
  regular `TestStory` user stories, the ordinary embedded repo clone user
  story, external control root workspace-cwd explicit selector behavior, and
  public transfer fallback/degraded JSON cleanliness. The covered JSON boundary
  keeps pure JSON output, public transfer roles, materialization destinations,
  published destinations, `degraded_reasons`, and `warnings` clean when
  `juicefs-clone` falls back to `copy`.

### Release gate summary

Local final release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

Local final release gate result: `RELEASE GATE PASSED`.

The table records the final release-gate evidence source. Rows marked "PASS via
release-gate suite" were qualified by the local final release gate and, for
tag-gated publication, by workflow run `25355112705`.

| Check | Command or target | Final evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | PASS; local result `RELEASE GATE PASSED`; tag workflow Release Gate job passed |
| Story e2e gate | `make story-e2e` plus `TestStoryE2EGate_CoversRegularUserStories` | PASS via release-gate conformance suite |
| Docs contract | `make docs-contract` | PASS via release-gate suite |
| CI contract | `make ci-contract` | PASS via release-gate suite |
| Race tests | `make test-race` | PASS via release-gate suite |
| Coverage | `make test-cover` | PASS; `69.4% >= 60%` |
| Lint | `make lint` | PASS via release-gate suite; tag workflow Lint job passed |
| Build | `make build` | PASS via release-gate suite; tag workflow Build and Test job passed; local `make build` passed |
| Release cross-build | `make release-build` | PASS via release-gate suite; local `make release-build` passed; release job published five platform binaries |
| Conformance | `make conformance` | PASS via release-gate suite |
| Library facade | `make library` | PASS via release-gate suite |
| Regression | `make regression` | PASS via release-gate suite |
| Fuzz ordinary tests | `make fuzz-tests` | PASS via release-gate suite |
| Fuzz smoke | `make fuzz` | PASS via release-gate suite |
| Security scan | GitHub Actions Security Scan job | PASS in tag workflow run `25355112705` |
| Release toolchain smoke | GitHub Actions Release Toolchain Smoke job | PASS in tag workflow run `25355112705` |
| Release publication | GitHub Actions Release job | PASS in tag workflow run `25355112705` |
| DCO | GitHub Actions DCO job | SKIPPED in tag workflow run `25355112705` |

### Coverage

- Coverage total: `69.4%`
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: local final `make release-gate` output.
- Evidence summary: `69.4% >= 60%`.

### Representative repo evidence

- Representative repo evidence source: final local `make release-gate`,
  including conformance and regression targets. This entry does not claim a
  separate ad hoc external repository beyond the release-gate suite and the
  recorded downloaded release-asset smoke.
- Representative repo coverage: release-gate suite coverage for save point
  history, strict doctor, integrity checks, restore preview/run and recovery
  behavior, runtime repair path, story-e2e gate coverage, embedded repo clone
  behavior, external control root workspace-cwd selectors, and transfer
  fallback/degraded JSON cleanliness.
- Story e2e evidence: conformance ran
  `TestStoryE2EGate_CoversRegularUserStories`, which dry-runs
  `make story-e2e`, lists regular `TestStory` user stories, and fails if any
  regular user story is missing from the gate pattern.
- Embedded repo clone evidence:
  `TestStoryRepoCloneEmbeddedProjectKeepsIdentityHistoryAndMainWorkspaceUsable`
  covers ordinary embedded repo clone behavior, source repo identity
  preservation, fresh target repo identity, copied save point choices, target
  `main` usability, follow-on target saves, source repo isolation, and public
  clone transfer cleanliness.
- External control root evidence:
  `TestStorySeparatedOpsWorkspaceCWDWithExplicitControlRootCoreFlow` covers
  external control root workspace-cwd operation with explicit `--control-root`
  and `--workspace main` selectors across status, save, history, view, view
  close, restore preview, and restore run while the workspace folder stays free
  of control data.
- Public transfer fallback/degraded evidence:
  `TestStory_PublicTransferJSONDegradedSaveKeepsPublicWarningsClean` covers
  `juicefs-clone` requested-engine fallback to `copy`, pure JSON output,
  public `degraded_reasons` and `warnings`, and absence of
  internal `.jvs`, content storage, and stdout/stderr detail leakage.
- Doctor command: `jvs doctor --strict`
- Doctor result: PASS via release-gate suite.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill, lifecycle recovery drill, runtime repair evidence, repo clone
  coverage, story-e2e gate coverage, and transfer fallback/degraded JSON
  coverage: PASS via release-gate conformance and regression coverage.

### GA docs evidence

- GA docs readiness scope: `docs/99_CHANGELOG.md`, this ledger, and
  release-facing story, clone, external control root, and transfer evidence
  define the release evidence contract for `v0.4.7`.
- Changelog scope: story-e2e gate coverage for all regular `TestStory` user
  stories, ordinary embedded repo clone user story coverage, external control
  root workspace-cwd explicit selector flow coverage, and public transfer
  fallback/degraded JSON cleanliness.
- Public JSON cleanliness evidence: fallback/degraded transfer reporting keeps
  public transfer roles, materialization destinations, published destinations,
  `degraded_reasons`, and `warnings` in the JSON contract without exposing
  internal `.jvs`, content storage, or stdout/stderr details.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: `.github/workflows/ci.yml` release job in tag workflow run
  `25355112705`.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Published artifact count: `12`
- Published artifacts:
  `jvs-darwin-amd64`,
  `jvs-darwin-amd64.bundle`,
  `jvs-darwin-arm64`,
  `jvs-darwin-arm64.bundle`,
  `jvs-linux-amd64`,
  `jvs-linux-amd64.bundle`,
  `jvs-linux-arm64`,
  `jvs-linux-arm64.bundle`,
  `jvs-windows-amd64.exe`,
  `jvs-windows-amd64.exe.bundle`,
  `SHA256SUMS`,
  and `SHA256SUMS.bundle`.
- Published asset validation: after release download to
  `/tmp/jvs-release-v0.4.7`, `sha256sum --check --strict SHA256SUMS` returned
  OK for all five binaries.
- Linux binary smoke: after `chmod +x jvs-linux-amd64`,
  `./jvs-linux-amd64 --help` printed current JVS save point help successfully.
- Signing workflow: final artifacts include five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`
  produced by the tag-gated release workflow.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification: the release job verified artifacts and signatures
  before upload with the release workflow certificate identity and OIDC issuer.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- Certificate identity used:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.7`
- OIDC issuer: `https://token.actions.githubusercontent.com`
- Local signature verification: no local cosign verification is claimed in
  this ledger because local cosign was not installed.

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`

## v0.4.6 - 2026-05-03

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.6`
- Candidate state: not final, not tagged, and not published; this entry is
  pending final tag creation and publication through the normal CI release
  flow.
- Changelog heading date: `2026-05-03`
- Release identity: main-branch GA candidate readiness for the next published
  save point public-contract release after `v0.4.5`.
- Baseline: repo/workspace lifecycle management for repo move/rename/detach,
  workspace move/rename/delete preview/run and recovery posture, external
  workspace pending lifecycle evidence, machine-readable
  `recommended_next_command`, repo clone workflow, and filesystem-aware
  transfer planning/implementation, plus pre-GA public vocabulary cleanup.
- Source archive boundary: no immutable `v0.4.6` tag source archive exists yet.
  When the pending final tag is created, the source archive will be the
  immutable source archive and will record readiness from tag time.
- publication final evidence: pending. The future GitHub Release page and
  post-release main ledger will record workflow run, release state, artifacts,
  checksum validation, signing identity, smoke, and coverage facts after the
  release exists.
- Candidate release URL target:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.6`
- Scope: repo and workspace lifecycle readiness for safe folder moves, name
  changes, repo detach metadata, registered workspace updates, external
  workspace pending lifecycle discovery, `recommended_next_command` recovery
  guidance, repo clone workflow coverage, imported clone history protection,
  cleanup boundaries, filesystem-aware transfer behavior across save, view,
  restore, workspace creation, and clone paths, and public vocabulary cleanup
  for Go facade fields, error codes, and transfer JSON reporting.

### Release gate summary

Candidate release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

The table records required checks for the pending final tag. This candidate
entry records readiness expectations only; final results are intentionally left
to CI and the post-release evidence ledger.

| Check | Command or target | Candidate readiness evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | Required before final publication; pending final run |
| Docs contract | `make docs-contract` | Required by release-gate suite; pending final run |
| CI contract | `make ci-contract` | Required by release-gate suite; pending final run |
| Race tests | `make test-race` | Required by release-gate suite; pending final run |
| Coverage | `make test-cover` | Required with threshold enforcement; pending final run |
| Lint | `make lint` | Required by release-gate suite; pending final run |
| Build | `make build` | Required by release-gate suite; pending final run |
| Release cross-build | `make release-build` | Required before artifact publication; pending final run |
| Conformance | `make conformance` | Required by release-gate suite; pending final run |
| Library facade | `make library` | Required by release-gate suite; pending final run |
| Regression | `make regression` | Required by release-gate suite; pending final run |
| Fuzz ordinary tests | `make fuzz-tests` | Required by release-gate suite; pending final run |
| Fuzz smoke | `make fuzz` | Required by release-gate suite; pending final run |

### Coverage

- Coverage total: pending final `make test-cover` evidence.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: pending final `make release-gate` output.

### Representative repo evidence

- Representative repo evidence source: pending final `make release-gate`,
  including conformance and regression targets.
- Representative repo readiness: release-gate coverage is expected to cover
  save point history, repo clone workflow, repo move/rename/detach,
  workspace move/rename/delete preview/run paths, external workspace pending
  lifecycle evidence, strict doctor, integrity checks, restore preview/run and
  recovery behavior, runtime repair path, cleanup boundaries, and
  filesystem-aware transfer planning before publication.
- Public vocabulary readiness: release-gate coverage is expected to keep
  `pkg/jvs.SavePoint` on `ContentRootHash` / `content_root_hash`, public
  errors on `E_SAVE_POINT_*` and `E_CLEANUP_*`, and transfer JSON public
  references plus free-text sanitizer output on content/save point vocabulary.
- Pending lifecycle readiness: interrupted repo or workspace lifecycle work is
  expected to report the pending operation and machine-readable
  `recommended_next_command` instead of treating the half-completed state as
  healthy.
- Doctor command: `jvs doctor --strict`
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill, lifecycle recovery drill, runtime repair evidence, repo clone
  coverage, and transfer coverage: pending final release-gate conformance and
  regression coverage.

### GA docs evidence

- GA docs readiness scope: `docs/99_CHANGELOG.md`, this ledger, and
  release-facing repo, workspace, clone, and transfer docs define the candidate
  readiness contract for `v0.4.6`.
- Changelog scope: repo/workspace lifecycle management,
  repo move/rename/detach, workspace move/rename/delete preview/run and
  recovery posture, external workspace pending lifecycle evidence,
  machine-readable `recommended_next_command`, repo clone workflow, and
  filesystem-aware transfer planning/implementation.
- Public vocabulary cleanup evidence: this pre-GA public vocabulary cleanup is
  a clean break with no backward-compatible aliases. The user-visible surface
  is `ContentRootHash` / `content_root_hash`, `E_SAVE_POINT_*`,
  `E_CLEANUP_*`, transfer JSON public references, and transfer free-text
  sanitizer output.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: pending tag-gated `.github/workflows/ci.yml` release job.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Expected artifact set after final publication: five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`; this
  candidate entry is not published and records no final asset result.
- Signing workflow expectation: final artifacts are signed by the tag-gated
  release workflow using Sigstore/cosign v3 bundle files.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification expectation: the release job must run `test -s` for
  every artifact, `sha256sum --check --strict SHA256SUMS`, and
  `cosign verify-blob <artifact> --bundle <artifact>.bundle` with the release
  workflow certificate identity and OIDC issuer.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`

## v0.4.5 - 2026-05-01

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.5`
- Candidate state: not final, not tagged, and not published; this entry is
  pending final tag creation and publication through the normal CI release
  flow.
- Changelog heading date: `2026-05-01`
- Release identity: main-branch GA candidate readiness for the next published
  save point public-contract release after `v0.4.4`.
- Baseline: workspace user-story coverage for explicit workspace folders,
  folder-local status, save, history, and restore, multi-workspace list/status
  visibility, `jvs history from` default source behavior, workspace pointer
  movement after saving, implicit workspace creation rejection, and `--name`
  decoupling from folder basename.
- Source archive boundary: no immutable `v0.4.5` tag source archive exists yet.
  When the pending final tag is created, the source archive will be the
  immutable source archive and will record readiness from tag time.
- publication final evidence: pending. The future GitHub Release page and
  post-release main ledger will record workflow run, release state, artifacts,
  checksum validation, signing identity, smoke, and coverage facts after the
  release exists.
- Candidate release URL target:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.5`
- Scope: workspace user-story coverage/readiness for explicit sibling folder
  creation from a save point, natural commands inside the workspace folder,
  repo/workspace isolation, multi-workspace tracking, history direction and
  default source behavior, current workspace pointer movement, rejection of
  unsafe implicit folder creation, and workspace name/folder basename
  decoupling.

### Release gate summary

Candidate release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

The table records required checks for the pending final tag. This candidate
entry records readiness expectations only; final results are intentionally left
to CI and the post-release evidence ledger.

| Check | Command or target | Candidate readiness evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | Required before final publication; pending final run |
| Docs contract | `make docs-contract` | Required by release-gate suite; pending final run |
| CI contract | `make ci-contract` | Required by release-gate suite; pending final run |
| Race tests | `make test-race` | Required by release-gate suite; pending final run |
| Coverage | `make test-cover` | Required with threshold enforcement; pending final run |
| Lint | `make lint` | Required by release-gate suite; pending final run |
| Build | `make build` | Required by release-gate suite; pending final run |
| Release cross-build | `make release-build` | Required before artifact publication; pending final run |
| Conformance | `make conformance` | Required by release-gate suite; pending final run |
| Library facade | `make library` | Required by release-gate suite; pending final run |
| Regression | `make regression` | Required by release-gate suite; pending final run |
| Fuzz ordinary tests | `make fuzz-tests` | Required by release-gate suite; pending final run |
| Fuzz smoke | `make fuzz` | Required by release-gate suite; pending final run |

### Coverage

- Coverage total: pending final `make test-cover` evidence.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: pending final `make release-gate` output.

### Representative repo evidence

- Representative repo evidence source: pending final `make release-gate`,
  including conformance and regression targets.
- Representative repo readiness: release-gate coverage is expected to cover
  save point history, explicit workspace folder creation, workspace state,
  folder-local workspace commands, strict doctor, integrity checks, restore
  preview/run/recovery behavior, and runtime repair path before publication.
- Workspace story readiness: story e2e coverage is expected to exercise the
  ordinary user path from a repo save point into a separate workspace folder,
  then through workspace-local status, save, history, and restore.
- Doctor command: `jvs doctor --strict`
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill, recovery drill, and runtime repair evidence: pending final
  release-gate conformance and regression coverage.

### GA docs evidence

- GA docs readiness scope: `docs/99_CHANGELOG.md`, this ledger, and
  release-facing workspace command docs define the candidate readiness contract
  for `v0.4.5`.
- Changelog scope: workspace user-story coverage/readiness for explicit
  workspace folder creation, folder-local status/save/history/restore,
  multi-workspace list/status/path, `jvs history from` default source and
  workspace pointer movement, rejection of implicit workspace creation, and
  `--name`/folder basename decoupling.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: pending tag-gated `.github/workflows/ci.yml` release job.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Expected artifact set after final publication: five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`; this
  candidate entry is not published and records no final asset result.
- Signing workflow expectation: final artifacts are signed by the tag-gated
  release workflow using Sigstore/cosign v3 bundle files.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification expectation: the release job must run `test -s` for
  every artifact, `sha256sum --check --strict SHA256SUMS`, and
  `cosign verify-blob <artifact> --bundle <artifact>.bundle` with the release
  workflow certificate identity and OIDC issuer.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`

## v0.4.4 - 2026-04-30

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.4`
- Candidate state: not final, not tagged, and not published; this entry is
  pending final tag creation and publication through the normal CI release
  flow.
- Changelog heading date: `2026-04-30`
- Release identity: main-branch GA candidate readiness for the next published
  save point public-contract release after `v0.4.3`.
- Baseline: user documentation GA readiness, Best Practices discoverability,
  non-technical workflow tutorials, workflow placeholder conformance, and
  product gap visibility for portability and backup workflow.
- Source archive boundary: no immutable `v0.4.4` tag source archive exists yet.
  When the pending final tag is created, the source archive will be the
  immutable source archive and will record readiness from tag time.
- publication final evidence: pending. The future GitHub Release page and
  post-release main ledger will record workflow run, release state, artifacts,
  checksum validation, signing identity, smoke, and coverage facts after the
  release exists.
- Candidate release URL target:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.4`
- Scope: Best Practices user entry, user docs index discoverability,
  non-technical tutorials for client delivery, media sorting, and course or
  research materials, workflow placeholder explanation/conformance, and
  user-facing portability and backup workflow gap recording.

### Release gate summary

Candidate release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

The table records required checks for the pending final tag. This candidate
entry records readiness expectations only; final results are intentionally left
to CI and the post-release evidence ledger.

| Check | Command or target | Candidate readiness evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | Required before final publication; pending final run |
| Docs contract | `make docs-contract` | Required by release-gate suite; pending final run |
| CI contract | `make ci-contract` | Required by release-gate suite; pending final run |
| Race tests | `make test-race` | Required by release-gate suite; pending final run |
| Coverage | `make test-cover` | Required with threshold enforcement; pending final run |
| Lint | `make lint` | Required by release-gate suite; pending final run |
| Build | `make build` | Required by release-gate suite; pending final run |
| Release cross-build | `make release-build` | Required before artifact publication; pending final run |
| Conformance | `make conformance` | Required by release-gate suite; pending final run |
| Library facade | `make library` | Required by release-gate suite; pending final run |
| Regression | `make regression` | Required by release-gate suite; pending final run |
| Fuzz ordinary tests | `make fuzz-tests` | Required by release-gate suite; pending final run |
| Fuzz smoke | `make fuzz` | Required by release-gate suite; pending final run |

### Coverage

- Coverage total: pending final `make test-cover` evidence.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: pending final `make release-gate` output.

### Representative repo evidence

- Representative repo evidence source: pending final `make release-gate`,
  including conformance and regression targets.
- Representative repo readiness: release-gate coverage is expected to cover
  save point history, workspace state, strict doctor, integrity checks, restore
  preview/run/recovery behavior, and runtime repair path before publication.
- Doctor command: `jvs doctor --strict`
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill, recovery drill, and runtime repair evidence: pending final
  release-gate conformance and regression coverage.

### GA docs evidence

- GA docs readiness scope: `docs/README.md`, `docs/user/README.md`,
  `docs/user/best-practices.md`, `docs/user/tutorials.md`,
  `docs/99_CHANGELOG.md`, and this ledger define the user documentation
  candidate readiness contract for `v0.4.4`.
- Supporting product-gap record: `docs/PRODUCT_GAPS_FOR_NEXT_PLAN.md` records
  the user-facing portability and backup workflow gap as future product work,
  not as a new v0 CLI promise.
- Changelog scope: Best Practices user entry, non-technical tutorials,
  workflow placeholder conformance, user-doc index updates, and the
  portability and backup workflow product gap record.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: pending tag-gated `.github/workflows/ci.yml` release job.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Expected artifact set after final publication: five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`; this
  candidate entry is not published and records no final asset result.
- Signing workflow expectation: final artifacts are signed by the tag-gated
  release workflow using Sigstore/cosign v3 bundle files.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification expectation: the release job must run `test -s` for
  every artifact, `sha256sum --check --strict SHA256SUMS`, and
  `cosign verify-blob <artifact> --bundle <artifact>.bundle` with the release
  workflow certificate identity and OIDC issuer.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`

## v0.4.3 - 2026-04-29

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.3`
- Candidate state: not final, not tagged, and not published; this entry is
  pending final tag creation and publication through the normal CI release
  flow.
- Changelog heading date: `2026-04-29`
- Release identity: main-branch GA candidate readiness for the next published
  save point public-contract release after `v0.4.2`.
- Baseline: cleanup public boundary hardening, GA safety/clarity, migration
  whole-folder copy fail-closed docs/conformance hardening, and compact shell
  function guard scope coverage.
- Source archive boundary: no immutable `v0.4.3` tag source archive exists yet.
  When the pending final tag is created, the source archive will be the
  immutable source archive and will record readiness from tag time.
- publication final evidence: pending. The future GitHub Release page and
  post-release main ledger will record workflow run, release state, artifacts,
  checksum validation, signing identity, smoke, and coverage facts after the
  release exists.
- Candidate release URL target:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.3`
- Scope: cleanup public boundary hardening, public GA safety and clarity,
  migration fresh-destination whole-folder copy guidance, strict doctor runtime
  repair guidance, compact shell function guard scope coverage, and candidate
  release evidence alignment.

### Release gate summary

Candidate release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

The table records required checks for the pending final tag. This candidate
entry records readiness expectations only; final results are intentionally left
to CI and the post-release evidence ledger.

| Check | Command or target | Candidate readiness evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | Required before final publication; pending final run |
| Docs contract | `make docs-contract` | Required by release-gate suite; pending final run |
| CI contract | `make ci-contract` | Required by release-gate suite; pending final run |
| Race tests | `make test-race` | Required by release-gate suite; pending final run |
| Coverage | `make test-cover` | Required with threshold enforcement; pending final run |
| Lint | `make lint` | Required by release-gate suite; pending final run |
| Build | `make build` | Required by release-gate suite; pending final run |
| Release cross-build | `make release-build` | Required before artifact publication; pending final run |
| Conformance | `make conformance` | Required by release-gate suite; pending final run |
| Library facade | `make library` | Required by release-gate suite; pending final run |
| Regression | `make regression` | Required by release-gate suite; pending final run |
| Fuzz ordinary tests | `make fuzz-tests` | Required by release-gate suite; pending final run |
| Fuzz smoke | `make fuzz` | Required by release-gate suite; pending final run |

### Coverage

- Coverage total: pending final `make test-cover` evidence.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: pending final `make release-gate` output.

### Representative repo evidence

- Representative repo evidence source: pending final `make release-gate`,
  including conformance and regression targets.
- Representative repo readiness: release-gate coverage is expected to cover
  save point history, workspace state, strict doctor, integrity checks, restore
  preview/run/recovery behavior, and runtime repair path before publication.
- Doctor command: `jvs doctor --strict`
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill, recovery drill, and runtime repair evidence: pending final
  release-gate conformance and regression coverage.

### GA docs evidence

- GA docs readiness scope: `docs/02_CLI_SPEC.md`,
  `docs/06_RESTORE_SPEC.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/13_OPERATION_RUNBOOK.md`, `docs/18_MIGRATION_AND_BACKUP.md`,
  `docs/99_CHANGELOG.md`, `docs/PRODUCT_PLAN.md`, `docs/ARCHITECTURE.md`, and
  this ledger define the candidate readiness contract for `v0.4.3`.
- Changelog scope: cleanup public boundary hardening, GA safety/clarity,
  migration whole-folder copy fail-closed docs/conformance hardening, compact
  shell function guard scope coverage, and candidate release evidence
  alignment.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: pending tag-gated `.github/workflows/ci.yml` release job.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Expected artifact set after final publication: five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`; this
  candidate entry is not published and records no final asset result.
- Signing workflow expectation: final artifacts are signed by the tag-gated
  release workflow using Sigstore/cosign v3 bundle files.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification expectation: the release job must run `test -s` for
  every artifact, `sha256sum --check --strict SHA256SUMS`, and
  `cosign verify-blob <artifact> --bundle <artifact>.bundle` with the release
  workflow certificate identity and OIDC issuer.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`

## v0.4.2 - 2026-04-28

### Release identity

- Evidence class: Final release evidence
- Status: PASS
- Tag: `v0.4.2`
- Final tagged commit: `c21b676dfb04d32f8cf3b9fa301e465f6886ca94`
- Commit message: `ci: publish release signatures as bundles`
- Changelog heading date: `2026-04-28`
- Baseline: save point public-contract convergence and GA docs alignment.
- Source archive boundary: the `v0.4.2` source archive is the immutable source archive
  for the release and records readiness from tag time.
- Tag source archive evidence class: `GA candidate readiness`
- publication final evidence: GitHub Release page and post-release main ledger
  record the workflow run, release state, assets, checksum validation, signing
  identity, smoke, and coverage facts created after publication.
- Final evidence location: GitHub Release page and post-release main ledger.
- Tag movement: `v0.4.2` was not moved; the tag was not moved to add
  post-publication facts.
- CI run link rule:
  `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Canonical release URL rule:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2`
- Canonical release URL example:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`
- Tag workflow run: `25056873829`
- Tag workflow URL:
  `https://github.com/agentsmith-project/jvs/actions/runs/25056873829`
- Tag workflow result: success.
- Tag workflow jobs passed: Build and Test, Lint, Security Scan, Release
  Toolchain Smoke, Release Gate, and Release.
- Release URL:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2`
- Release state: `draft=false`, `prerelease=false`
- Scope: public save point CLI/docs convergence, restore preview/run/recovery,
  `workspace new --from <save>`, cleanup preview/run semantics, EMPTY folder
  first-save docs and acceptance coverage, user-story E2E coverage, JuiceFS
  story qualification requirements, destination-owned transfer staging checks,
  and GA release evidence alignment.

### Release gate summary

Local final release gate command:
`env -u NO_COLOR CI=true GITHUB_ACTIONS=true TERM=xterm-256color make release-gate`

Local final release gate result: `RELEASE GATE PASSED`.

The table records the final release-gate evidence source. Rows marked "PASS via
release-gate suite" were qualified by the local final release gate and, for
tag-gated publication, by workflow run `25056873829`.

| Check | Command or target | Final evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | PASS; local result `RELEASE GATE PASSED`; tag workflow Release Gate job passed |
| Docs contract | `make docs-contract` | PASS via release-gate suite |
| CI contract | `make ci-contract` | PASS via release-gate suite |
| Race tests | `make test-race` | PASS via release-gate suite |
| Coverage | `make test-cover` | PASS; `68.7% >= 60%` |
| Lint | `make lint` | PASS via release-gate suite; tag workflow Lint job passed |
| Build | `make build` | PASS via release-gate suite; tag workflow Build and Test job passed |
| Release cross-build | `make release-build` | PASS via release-gate suite; release job published five platform binaries |
| Conformance | `make conformance` | PASS via release-gate suite |
| Library facade | `make library` | PASS via release-gate suite |
| Regression | `make regression` | PASS via release-gate suite |
| Fuzz ordinary tests | `make fuzz-tests` | PASS via release-gate suite |
| Fuzz smoke | `make fuzz` | PASS via release-gate suite |
| Security scan | GitHub Actions Security Scan job | PASS in tag workflow run `25056873829` |
| Release toolchain smoke | GitHub Actions Release Toolchain Smoke job | PASS in tag workflow run `25056873829` |

### Coverage

- Coverage total: `68.7%`
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence source: local final `make release-gate` output.
- Evidence summary: `68.7% >= 60%`.

### Representative repo evidence

- Representative repo evidence source: final local `make release-gate`,
  including conformance and regression targets. This entry does not claim a
  separate ad hoc external repository beyond the release-gate suite.
- Representative repo coverage: release-gate suite coverage for save point
  history, workspace state, strict doctor, integrity checks, restore
  preview/run/recovery behavior, and runtime repair path.
- Doctor command: `jvs doctor --strict`
- Doctor result: PASS via release-gate suite.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Restore drill and recovery drill result: PASS via release-gate conformance and
  regression coverage.
- Runtime repair result: PASS via release-gate conformance and regression
  coverage.

### GA docs evidence

- GA release-facing docs: `docs/02_CLI_SPEC.md`,
  `docs/06_RESTORE_SPEC.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/13_OPERATION_RUNBOOK.md`, `docs/18_MIGRATION_AND_BACKUP.md`,
  `docs/99_CHANGELOG.md`, `docs/PRODUCT_PLAN.md`, `docs/ARCHITECTURE.md`, and
  this ledger define the release readiness and evidence contract for `v0.4.2`.
- Supporting non-release-facing reference:
  `docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md` informs clean redesign context
  but is not part of the v0 public contract or release-facing doc set.
- Changelog scope: save point public CLI/docs convergence, restore
  preview/run/recovery, workspace new semantics, cleanup preview/run
  semantics, EMPTY folder first-save docs/acceptance alignment, user-story E2E
  and JuiceFS story coverage, destination-owned transfer staging checks, and
  GA release evidence alignment.
- Runtime-state migration boundary: non-portable JVS runtime state remains
  destination-local and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.

### Artifact and signing evidence

- Artifact workflow: `.github/workflows/ci.yml` release job in tag workflow run
  `25056873829`.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Release toolchain smoke: non-publishing `release-toolchain-smoke` job installs
  `sigstore/cosign-installer@v4.1.1` with `cosign-release: v3.0.5` and
  verifies `cosign version` on pull request, main push, tag push, and
  `workflow_dispatch` paths before release publication.
- Published artifact count: `12`
- Published artifacts:
  `jvs-darwin-amd64`,
  `jvs-darwin-amd64.bundle`,
  `jvs-darwin-arm64`,
  `jvs-darwin-arm64.bundle`,
  `jvs-linux-amd64`,
  `jvs-linux-amd64.bundle`,
  `jvs-linux-arm64`,
  `jvs-linux-arm64.bundle`,
  `jvs-windows-amd64.exe`,
  `jvs-windows-amd64.exe.bundle`,
  `SHA256SUMS`,
  and `SHA256SUMS.bundle`.
- Published asset validation: after release download to
  `/tmp/jvs-release-v0.4.2`, `sha256sum --check --strict SHA256SUMS` returned
  OK for all five binaries.
- Linux binary smoke: `./jvs-linux-amd64 --help` printed current save point help
  and exited successfully.
- Signing workflow: final artifacts include five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`
  produced by the tag-gated release workflow.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification: the release job must run `test -s` for every
  published artifact, `sha256sum --check --strict SHA256SUMS`, and
  `cosign verify-blob <artifact> --bundle <artifact>.bundle` with the release
  workflow certificate identity and OIDC issuer.
- Local cosign version: `v3.0.5`, installed with
  `go install github.com/sigstore/cosign/v3/cmd/cosign@v3.0.5`.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- Certificate identity used:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.2`
- OIDC issuer: `https://token.actions.githubusercontent.com`
- Local signature verification: `cosign verify-blob` verified OK for
  `jvs-linux-amd64`, `jvs-linux-arm64`, `jvs-darwin-amd64`,
  `jvs-darwin-arm64`, `jvs-windows-amd64.exe`, and `SHA256SUMS` with their
  matching `.bundle` files, using the certificate identity and OIDC issuer
  recorded above.

### Runbook references

- Restore recovery, strict doctor, and cleanup semantics:
  `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and checksum validation: `docs/SIGNING.md`
