# Changelog

This changelog is release-facing. Because JVS has not reached GA, earlier
draft material is not carried forward as active reader content. The active
product vocabulary is folder, workspace, save point, save, history, view,
restore, recovery plan, doctor, and cleanup.

## v0.4.2 - 2026-04-27

### Highlights

- GA candidate for the save point public contract. Visible help and active
  specs lead with `init`, `save`, `history`, `view`, `restore`,
  `workspace new`, `recovery`, `status`, and `doctor`.
- Restore is preview-first: `jvs restore <save>` creates a plan and changes no
  files; `jvs restore --run <plan-id>` revalidates and applies the reviewed
  plan; `jvs recovery status|resume|rollback` closes interrupted restore
  workflows.
- `jvs workspace new <name> --from <save>` creates another real workspace from
  a save point, leaves the source workspace unchanged, starts with
  `Newest save point: none`, and records `started_from_save_point` on first
  save.
- Cleanup is documented as reviewed deletion of unprotected save point storage:
  preview first, then run the reviewed plan.
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
- v0 does not include signing commands.
- v0 does not include public partial-save contracts.
- v0 does not include compression contracts.
- v0 does not include merge/rebase.
- v0 does not include complex retention policy flags.
- Cleanup command promotion is still pending; release-facing docs describe the
  cleanup preview/run contract.
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

- Existing repositories do not need an on-disk migration for this candidate;
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

- See the [release evidence ledger](RELEASE_EVIDENCE.md#v042---2026-04-27)
  for the `v0.4.2` GA candidate readiness record. It is not final, not tagged,
  and not published; final tag, release-gate, coverage, representative repo,
  restore drill, artifact, and signing evidence remain pending final release
  qualification.

### Release artifacts

- No release artifacts have been published for this candidate entry. Final
  binaries, `SHA256SUMS`, `.sig`, and `.pem` artifacts must be produced by the
  tag-gated release workflow after `make release-gate` succeeds.
