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
- A tag source archive is the immutable tag snapshot. It can only contain facts
  available when the tag was created, including source readiness or candidate
  evidence, and must not be rewritten to add post-publication facts.
- publication final evidence lives on the GitHub Release page and the
  post-release main ledger on `main`. It records exact tag, commit, workflow
  run, release state, gate, coverage, representative repo, artifact, checksum,
  signing, smoke, and release URL facts after publication.

## Release Gates

Before any pre-release or v0 tag:

1. `make release-gate` passes.
2. Public CLI help smoke tests pass for the visible save point surface:
   `init`, `save`, `history`, `view`, `restore`, `workspace new`, `status`,
   `recovery`, and `doctor`.
3. Restore preview/run/recovery tests pass for whole-workspace and path
   restore.
4. Workspace creation tests prove `workspace new <folder> --from <save>` uses
   an explicit target folder, starts a new history with
   `newest_save_point: null`, and records `started_from_save_point`.
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
11. Release-chain external tools used by CI, security scanning, and artifact
    signing are installed from reviewed pinned versions; `@latest` is not a
    valid release workflow dependency.

CI enforces the same release gate for publishing paths. A `v*` tag push or
manual `workflow_dispatch` release for an existing `v*` tag must pass the
`release-gate` job before artifacts are published. Manual releases validate
and check out `refs/tags/<tag>`; a same-named branch or ambiguous ref is not a
release input.

## Documentation Gates

- `docs/02_CLI_SPEC.md`, `docs/06_RESTORE_SPEC.md`, command help, and
  conformance assertions agree on public command names, flags, IDs, JSON
  fields, and state vocabulary.
- `docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md` is a supporting non-release-facing reference
  for clean redesign context, not a release-facing contract document.
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
- `workspace new <folder> --from <save>` does not inherit source history and
  does not infer the target path from a workspace name.
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
  evidence class for each entry and distinguishes source archive readiness
  from publication final evidence.

Candidate entries may record target release identity, required
`make release-gate` status checks, docs-contract, ci-contract, test-race,
test-cover, lint, build, conformance, library, regression, fuzz-tests, fuzz,
coverage threshold, representative repo requirements, runbook references,
artifact rules, and signing workflow rules while marking final-only facts
pending.

Final tagged release entries must not leave unresolved candidate or pending
publication facts. If the tag source archive still records readiness or
candidate evidence, the final entry must explicitly label that tag snapshot,
state that the tag was not moved, and place publication final evidence in the
GitHub Release page plus the post-release main ledger. Final entries must
record exact coverage result and threshold, final tag and tagged commit,
release-gate result, representative repo results, published artifact,
checksum, smoke, and signing evidence, and final release URL.
