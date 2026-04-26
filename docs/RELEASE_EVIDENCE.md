# Release Evidence Ledger

This ledger records compact release evidence. It has two evidence classes:

- GA candidate readiness: pre-tag evidence for a release candidate. It may
  record required commands, expected artifact rules, and pending final checks,
  but it is not final, not tagged, and not published.
- Final tagged release: immutable evidence for an existing tag. Only this
  class may record `Status: PASS`, a final `Tag`, a final tagged commit, and
  published artifact counts.

Raw logs and `coverage.out` are not stored here.

## v0.4.1 - 2026-04-26

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.1`
- Candidate status: not final, not tagged, not published, and pending final tag.
- Pending final facts: exact release-gate result, published artifacts, and
  signing evidence.
- Changelog heading date: `2026-04-26`
- Baseline: compatible fixes after the published `v0.4.0` release.
- CI run link rule: `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Release URL rule after tagging:
  `https://github.com/agentsmith-project/jvs/releases/tag/<tag>`
- Scope: final release evidence documentation, CI/release validation metadata,
  and deterministic fuzz smoke hardening.

### Release gate readiness

Command: `make release-gate`

The table lists required candidate checks. This entry records readiness
requirements only and does not claim final release results.

| Check | Command or target | Candidate readiness |
| --- | --- | --- |
| Release gate | `make release-gate` | Required before tag |
| Docs contract | `make docs-contract` | Required before tag |
| CI contract | `make ci-contract` | Required before tag |
| Race tests | `make test-race` | Required before tag |
| Coverage | `make test-cover` | Required before tag |
| Lint | `make lint` | Required before tag |
| Build | `make build` | Required before tag |
| Conformance | `make conformance` | Required before tag |
| Library facade | `make library` | Required before tag |
| Regression | `make regression` | Required before tag |
| Fuzz ordinary tests | `make fuzz-tests` | Required before tag |
| Fuzz smoke | `make fuzz` | Required before tag |

### Coverage

- Coverage total: pending final release-gate result.
- Coverage threshold: `60.0%`, enforced by `make test-cover`.
- Evidence command: `make test-cover`

### Representative repo evidence

- Representative repo class: existing v0 repo with checkpoint history,
  workspace state, audit chain validation, and runtime repair path exercised.
- Doctor command: `jvs doctor --strict` required before tagging.
- Verify command: `jvs verify --all` required before tagging.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime` required before tagging.

### GA docs evidence

- GA docs readiness: `docs/99_CHANGELOG.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/14_TRACEABILITY_MATRIX.md`, and this ledger must describe candidate
  readiness without claiming final release facts for `v0.4.1`.
- Changelog scope: final release evidence documentation, CI/release validation
  metadata, and deterministic fuzz smoke hardening after `v0.4.0`.
- Runtime-state migration boundary: active `.jvs/locks/`, `.jvs/intents/`,
  and `.jvs/gc/*.json` runtime state remains non-portable and must be rebuilt
  at the destination.

### Artifact and signing readiness

- Artifact workflow: `.github/workflows/ci.yml` release job.
- Published artifacts: not published; pending final release workflow.
- Expected artifact set after tagging: platform binaries, `SHA256SUMS`, `.sig`
  sidecars, and `.pem` sidecars.
- Signing command family: `cosign sign-blob --yes`
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Verification and recovery: `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and verification: `docs/SIGNING.md`

## v0.4.0 - 2026-04-25

### Release identity

- Evidence class: Final tagged release
- Tag: `v0.4.0`
- Status: PASS
- Changelog heading date: `2026-04-25`
- Published at: `2026-04-26T14:24:35Z`
- CI run:
  `https://github.com/agentsmith-project/jvs/actions/runs/24958799554`
- CI run link rule: `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Release URL:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`
- Tag object: `c377696cb80d9215444c2daf4e0fb02ebdf48550`
- Final tagged commit: `7103a80cd3aebf5d2bf20ddb4c4167c96310ce60`
- GitHub release state: non-draft, non-prerelease.
- Release workflow jobs: release job `73081986600` success; release-gate
  job `73081695699` success.
- This release evidence ledger entry records the final tag, release gate,
  coverage, representative repo, GA docs, artifact, signing, and runbook
  evidence for the published GitHub release.

### Release gate summary

Command: `make release-gate`

The table lists the final tagged release evidence from the published workflow
run.

| Check | Command or target | Status |
| --- | --- | --- |
| Release gate | `make release-gate` in job `73081695699` | PASS |
| Docs contract | `make docs-contract` | PASS |
| CI contract | `make ci-contract` | PASS |
| Race tests | `make test-race` | PASS |
| Coverage | `make test-cover` | PASS |
| Lint | `make lint` | PASS |
| Build | `make build` | PASS |
| Conformance | `make conformance` | PASS |
| Library facade | `make library` | PASS |
| Regression | `make regression` | PASS |
| Fuzz ordinary tests | `make fuzz-tests` | PASS |
| Fuzz smoke | `make fuzz` | PASS |

### Coverage

- Coverage total: `73.3%`
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence summary: CI release-gate log reported
  `OK: coverage 73.3% >= 60% threshold`.

### Representative repo evidence

- Representative repo class: existing v0 repo with checkpoint history,
  workspace state, audit chain, and runtime repair path exercised.
- Doctor command: `jvs doctor --strict`
- Doctor result: healthy after runtime repair validation.
- Verify command: `jvs verify --all`
- Verify result: all checkpoint checksums and payload hashes valid.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Independent verification: a repo with 3 checkpoints and an extra workspace
  was created, stale `.jvs/intents/orphan.json` was injected,
  `jvs doctor --strict --repair-runtime` cleaned 1 stale operation record,
  `jvs doctor --strict` returned healthy, and `jvs verify --all` returned all
  checkpoint checksums and payload hashes valid.

### GA docs evidence

- Release docs: `docs/99_CHANGELOG.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/14_TRACEABILITY_MATRIX.md`, and this ledger describe final release
  facts for `v0.4.0`.
- Migration terminology: public terms remain checkpoint and workspace;
  `.jvs/snapshots` and `.jvs/worktrees` are compatibility storage names.
- Runtime-state migration boundary: mutation lock directories, operation
  records, and active GC plans are non-portable and rebuilt at the
  destination.
- Public command docs and conformance plan use active runtime operation state
  and stable v0 command flags.

### Artifact and signing evidence

- Published artifact count: `18`
- Published artifacts: five platform binaries, five `.sig` sidecars, five
  `.pem` sidecars, `SHA256SUMS`, `SHA256SUMS.sig`, and `SHA256SUMS.pem`.
- Artifact workflow: `.github/workflows/ci.yml` release job.
- Signing command family: `cosign sign-blob --yes`
- Verification evidence: final release workflow checked all artifact files are
  non-empty and ran `sha256sum --check --strict SHA256SUMS`.
- Certificate identity:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.0`
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`
- Tlog indexes: `1390604743`, `1390604759`, `1390604774`, `1390604787`,
  `1390604811`, `1390604832`.
- Independent post-publish verification: all 18 release assets were
  downloaded to a temporary directory; `sha256sum --check --strict SHA256SUMS`
  passed for all five binaries; `cosign verify-blob` passed for `SHA256SUMS`
  and all five binaries with the certificate identity and OIDC issuer above.

| Asset | SHA-256 |
| --- | --- |
| `jvs-linux-amd64` | `1c91ab6c7eb395b057393f481c963bace4689e6af82e14c867e6d99431934d24` |
| `jvs-linux-arm64` | `82315e507354b32626b02caaf9c8415b74ebe008c2a0b53d8da688716482a66c` |
| `jvs-darwin-amd64` | `4d50d941d60598cff995e0398c395d91b9460dd831145753929da4f4c93bdb10` |
| `jvs-darwin-arm64` | `67e1e432f112e2e76f149738dc388f2964b6782a609309103f630f8bf7f55828` |
| `jvs-windows-amd64.exe` | `a833ed9bb525b5b64a5f64dcbe19f3beb1ebb2fea219638cf9ba5a2426df6fa5` |
| `SHA256SUMS` | `3c57af6eda35d3afa89ad30407c36dd1d2a88073d2038290800c29c5b711901a` |

### Runbook references

- Verification and recovery: `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and verification: `docs/SIGNING.md`
