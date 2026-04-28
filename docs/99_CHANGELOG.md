# Changelog

This changelog is release-facing. Earlier draft material is not carried forward
as active reader content for the published GA line. The active product
vocabulary is folder, workspace, save point, save, history, view, restore,
recovery plan, doctor, and cleanup.

## v0.4.2 - 2026-04-28

### Highlights

- GA release for the save point public contract. Visible help and active specs
  lead with `init`, `save`, `history`, `view`, `restore`, `workspace new`,
  `cleanup`, `recovery`, `status`, and `doctor`.
- Restore is preview-first: `jvs restore <save>` creates a plan and changes no
  files; `jvs restore --run <plan-id>` revalidates and applies the reviewed
  plan; `jvs recovery status|resume|rollback` closes interrupted restore
  workflows.
- `jvs workspace new <name> --from <save>` creates another real workspace from
  a save point, leaves the source workspace unchanged, starts with
  `Newest save point: none`, and records `started_from_save_point` on first
  save.
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
- Workspace creation examples must use `jvs workspace new <name> --from
  <save>`.

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

- `integrity`: descriptor checksum and payload hash detect independent
  corruption; coordinated descriptor-plus-checksum rewrite remains a v0
  residual risk.
- `migration`: active `.jvs/locks/`, `.jvs/intents/`, and `.jvs/gc/*.json`
  runtime state is non-portable and must be rebuilt at the destination with
  `jvs doctor --strict --repair-runtime`.
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
- Exclude active `.jvs/locks/`, `.jvs/intents/`, and `.jvs/gc/*.json` runtime
  state during physical sync; copied mutation locks may block destination
  writes until repaired.
- Run the restore drill from `docs/13_OPERATION_RUNBOOK.md`, including
  preview/run and recovery status/resume/rollback coverage.

### Release evidence

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v042---2026-04-28)
  for the `v0.4.2` final GA release evidence record.
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
