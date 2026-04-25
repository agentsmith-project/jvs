# Release Policy (pre-release / v0)

## Scope

This policy governs pre-release and v0 tags. A release is valid only when the
public CLI contract, documentation, tests, and shipped artifacts describe the
same behavior.

## Versioning

- Pre-release tags may change behavior while the v0 contract is still being
  hardened, but every change must be reflected in the CLI spec and tests before
  tagging.
- v0 patch tags are for compatible fixes, documentation corrections, and
  additive clarifications that do not break documented commands or JSON fields.
- Any incompatible public command, flag, ref, state, or JSON change requires an
  explicit release note and updated conformance coverage.

## Release Gates

Before any pre-release or v0 tag:

1. `make release-gate` passes.
2. Conformance tests pass for the v0 public CLI contract.
3. Regression tests pass for previously fixed bugs and destructive-operation
   safety.
4. Fuzz targets pass for public parsers, names, tags, refs, and structured
   metadata.
5. Lint and build pass without ignored failures.
6. Coverage meets the repository threshold enforced by `make test-cover`.
7. `jvs doctor --strict` passes on a representative repo.
8. `jvs verify --all` passes on the same representative repo.

## Documentation Gates

- `README.md`, `docs/QUICKSTART.md`, `docs/02_CLI_SPEC.md`, and the
  conformance plan agree on public command names, flags, refs, JSON fields,
  and state vocabulary.
- Public docs use the v0 vocabulary: repo, workspace, checkpoint, `current`,
  `latest`, and `dirty`.
- Old or private public-facing terms must not leak into user docs, help text,
  examples, release notes, or conformance assertions.
- Unsupported future capabilities such as remote push or pull, signing
  commands, partial checkpoint contracts, compression contracts, merge/rebase,
  and complex retention policy flags are called out only as v0 boundaries.

## Contract Gates

- JSON commands emit exactly one object with the documented envelope.
- Error responses include stable machine-readable codes and useful messages.
- Dirty guards are verified for operations that overwrite or remove workspace
  contents.
- `current`, `latest`, and `dirty` semantics are tested across restore, fork,
  checkpoint, diff, and status flows.
- Repo and workspace resolution is tested from repo roots, workspace roots,
  nested workspace paths, and explicit `--repo` / `--workspace` selections.
- `init`, `import`, `clone`, and `capability` are covered because they define
  how users enter the system.

## Breaking Change Process

- Document the rationale and affected user-visible behavior.
- Update `docs/02_CLI_SPEC.md` and any affected tutorials before tagging.
- Add or adjust conformance tests before implementation is considered done.
- Describe migration impact, recovery expectations, and known limitations in
  the release notes.

## Required Release Artifacts

- Updated spec set and tutorials.
- Conformance and regression summaries.
- Coverage result from the release gate.
- Changelog entry with date, tag, highlights, breaking changes, and known
  limitations.
- Runbook references for verification, diagnosis, and recovery.
