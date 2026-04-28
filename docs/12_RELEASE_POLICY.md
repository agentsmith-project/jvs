# Release Policy

## Scope

This policy governs pre-release and v0 tags while the public UX converges on
the save point contract. A release is valid only when CLI help, public docs,
tests, and shipped artifacts describe the same user-visible behavior.

## Versioning

- Pre-release tags may make breaking public UX changes while the contract is
  still being hardened.
- Any public command, flag, state vocabulary, ref semantics, or JSON field
  change requires an explicit changelog entry, release evidence, and
  conformance coverage.
- Final tagged release evidence must replace candidate placeholders with exact
  tag, commit, gate, coverage, representative repo, artifact, and signing
  facts.

## Release Gates

Before any pre-release or v0 tag:

1. `make release-gate` passes.
2. Public CLI help smoke tests pass for the visible save point surface:
   `init`, `save`, `history`, `view`, `restore`, `workspace new`, `status`,
   `recovery`, and `doctor`.
3. Restore preview/run/recovery tests pass for whole-workspace and path
   restore.
4. Workspace creation tests prove `workspace new --from <save>` starts a new
   history with `newest_save_point: null` and records
   `started_from_save_point`.
5. Regression tests pass for previously fixed destructive-operation safety
   bugs.
6. Fuzz targets pass for public parsers, names, save point IDs, and structured
   metadata.
7. Lint and build pass without ignored failures.
8. Coverage meets the repository threshold enforced by `make test-cover`.
9. `jvs doctor --strict` passes on a representative repo, including audit
   chain validation.
10. `jvs doctor --strict` integrity evidence is recorded for the same
    representative repo.

CI enforces the same release gate for publishing paths. A `v*` tag push or
manual `workflow_dispatch` release for an existing `v*` tag must pass the
`release-gate` job before artifacts are published. Manual releases validate
and check out `refs/tags/<tag>`; a same-named branch or ambiguous ref is not a
release input.

## Documentation Gates

- `docs/02_CLI_SPEC.md`, `docs/06_RESTORE_SPEC.md`,
  `docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`, command help, and
  conformance assertions agree on public command names, flags, IDs, JSON
  fields, and state vocabulary.
- Public docs use save point vocabulary: folder, workspace, save point, save,
  history, view, restore, unsaved changes, cleanup, and recovery plan.
- Implementation storage names may appear only as storage or code facts. They
  must not define public commands, selectors, examples, or workflow concepts.
- Unsupported future capabilities such as remote push or pull, signing
  commands, merge/rebase, label-as-ref restore, public partial-save contracts,
  public compression contracts, and complex retention policy flags are called
  out only as boundaries.

## Contract Gates

- JSON commands emit exactly one object with the documented envelope.
- Error responses include stable machine-readable codes and useful messages.
- Unsaved-change guards are verified for operations that overwrite or remove
  managed files.
- Restore is preview-first. Run binds to a plan ID and revalidates expected
  target state.
- Restore failures produce either no file changes or an active recovery plan
  with status/resume/rollback.
- `workspace new --from <save>` does not inherit source history.
- Cleanup documentation describes preview-first, reviewed deletion of
  unprotected save point storage.
- Repo and workspace targeting is tested from folders, nested paths, explicit
  `--repo` assertions, and `--workspace` selections.

## Breaking Change Process

- Document the rationale and affected user-visible behavior.
- Update `docs/02_CLI_SPEC.md`, `docs/06_RESTORE_SPEC.md`, and affected
  release/ops docs before tagging.
- Add or adjust conformance tests before implementation is considered done.
- Describe migration impact, recovery expectations, known limitations, and
  risk labels in the release notes.
- Avoid fallback wording that makes non-public implementation details sound
  like the public contract.

## Required Release Artifacts

- Updated spec set and tutorials that match visible help.
- Conformance and regression summaries.
- Coverage result from the release gate.
- Changelog entry with date, tag or candidate target, highlights, breaking
  changes, known limitations, risk labels, migration notes, and release
  artifacts.
- Runbook references for verification, diagnosis, restore recovery, migration,
  and cleanup layering.
- Release evidence ledger (`docs/RELEASE_EVIDENCE.md`) that records the
  evidence class for each entry.

Candidate entries may record target release identity, required
`make release-gate` status checks, docs-contract, ci-contract, test-race,
test-cover, lint, build, conformance, library, regression, fuzz-tests, fuzz,
coverage threshold, representative repo requirements, runbook references,
artifact rules, and signing workflow rules while marking final-only facts
pending.

Final tagged release entries must not contain candidate or pending language.
They must record exact coverage result and threshold, final tag and tagged
commit, release-gate result, representative repo results, published artifact
and signing evidence, and final release URL.
