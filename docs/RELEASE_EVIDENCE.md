# Release Evidence Ledger

This ledger records compact release evidence. It has two evidence classes:

- GA candidate readiness: pre-tag evidence for a release candidate. It may
  record required commands, expected artifact rules, and pending final checks,
  but it is not final, not tagged, and not published.
- Final tagged release: immutable evidence for an existing tag. Only this
  class may record `Status: PASS`, a final `Tag`, a final tagged commit, and
  published artifact counts.

Raw logs and `coverage.out` are not stored here.

## v0.4.2 - 2026-04-27

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.2`
- Candidate status: not final, not tagged, and not published; pending final tag
  creation and release-gate qualification.
- Changelog heading date: `2026-04-27`
- Baseline: compatible fixes, release evidence updates, and story coverage
  after the published `v0.4.1` release.
- CI run link rule:
  `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Release URL rule after publication:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2`
- Scope: partial checkpoint overlapping path canonicalization, EMPTY workspace
  first-checkpoint docs and acceptance coverage, local user-story E2E coverage,
  JuiceFS story qualification requirements, destination-owned transfer staging
  checks, and post-`v0.4.1` release evidence finalization on main.

### Release gate summary

Command required before final tag: `make release-gate`

The table lists required checks for the candidate. Final-only results remain
pending final tag qualification and the tag-gated release workflow.

| Check | Command or target | Candidate evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | required before pending final tag |
| Docs contract | `make docs-contract` | local pre-update verification completed successfully; required again in final release gate |
| CI contract | `make ci-contract` | required before pending final tag |
| Race tests | `make test-race` | required before pending final tag |
| Coverage | `make test-cover` | required before pending final tag |
| Lint | `make lint` | required before pending final tag |
| Build | `make build` | required before pending final tag |
| Conformance | `make conformance` | required before pending final tag |
| Library facade | `make library` | required before pending final tag |
| Regression | `make regression` | required before pending final tag |
| Fuzz ordinary tests | `make fuzz-tests` | required before pending final tag |
| Fuzz smoke | `make fuzz` | required before pending final tag |

### Coverage

- Coverage total: pending final release-gate `make test-cover` result.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence summary: final coverage evidence must replace this candidate note
  after the tag-gated release gate completes.

### Representative repo evidence

- Representative repo class: required existing v0 repo with checkpoint history,
  workspace state, audit chain validation, and runtime repair path.
- Doctor command: `jvs doctor --strict`
- Doctor result: pending final release qualification.
- Verify command: `jvs verify --all`
- Verify result: pending final release qualification.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Required evidence: final release qualification must record strict doctor,
  full verification, and runtime repair results before this candidate can be
  converted into final tagged release evidence.

### GA docs evidence

- GA docs: `docs/99_CHANGELOG.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/14_TRACEABILITY_MATRIX.md`, and this ledger define the release
  readiness and evidence contract for `v0.4.2`.
- Changelog scope: partial checkpoint overlapping path folding, EMPTY workspace
  first-checkpoint docs/acceptance alignment, user-story E2E and JuiceFS story
  coverage, destination-owned transfer staging checks, and post-`v0.4.1`
  release evidence finalization on main.
- Runtime-state migration boundary: active `.jvs/locks/`, `.jvs/intents/`,
  and `.jvs/gc/*.json` runtime state remains non-portable and must be rebuilt
  at the destination.

### Artifact and signing evidence

- Artifact workflow: `.github/workflows/ci.yml` release job.
- Artifact publication: not published for this candidate; artifact counts,
  checksums, and download verification are pending final tag publication.
- Signing workflow: final artifacts must include `.sig` and `.pem` sidecars
  produced by the tag-gated release workflow.
- Signing command family: `cosign sign-blob --yes`
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Verification and recovery: `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and verification: `docs/SIGNING.md`

## v0.4.1 - 2026-04-26

### Release identity

- Evidence class: Final tagged release
- Tag: `v0.4.1`
- Tag type: annotated tag
- Status: PASS
- Workflow conclusion: success
- Changelog heading date: `2026-04-26`
- Published at: `2026-04-27T05:12:36Z`
- CI run:
  `https://github.com/agentsmith-project/jvs/actions/runs/24977618650`
- Baseline: compatible fixes after the published `v0.4.0` release.
- CI run link rule: `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Release URL:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.1`
- Tag object: `2497e7d859291cc46408b5e77c6273ff8472a867`
- Final tagged commit: `01295c1ae26ff1554c892f7f74ee7c0e5e57e6d9`
- Tagger date: `2026-04-26 22:08:46 -0700`
- GitHub release state: non-draft, non-prerelease.
- Release workflow jobs: build and test job `73132546034` success; security
  scan job `73132546041` success; lint job `73132546053` success;
  release-gate job `73132546057` success; release job `73132775828` success.
- Scope: final release evidence documentation, CI/release validation metadata,
  and deterministic fuzz smoke hardening.

### Release gate summary

Command: `make release-gate`

Final line: `RELEASE GATE PASSED`

The table lists the final tagged release evidence from the published workflow
run.

| Check | Command or target | Status |
| --- | --- | --- |
| Release gate | `make release-gate` in job `73132546057` | PASS |
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
  workspace state, audit chain validation, and runtime repair path exercised
  on an ordinary `/tmp` copy engine.
- Doctor command: `jvs doctor --strict`
- Doctor result: healthy after runtime repair validation.
- Verify command: `jvs verify --all`
- Verify result: all checkpoint checksums and payload hashes valid.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Independent verification: a temporary repo with 2 workspaces (`main` and
  `active-review`) and 4 checkpoint descriptors was created. Initial
  `jvs doctor --strict` returned exit 0, healthy true, and 0 findings; initial
  `jvs verify --all` returned exit 0 with checkpoint count 4 and 0 bad or
  tampered checkpoints. After injecting stale `.jvs/locks/repo.lock`,
  `.jvs-tmp-release-evidence`, `.jvs/snapshots/runtime-release-evidence.tmp`,
  and `.jvs/intents/stale-release-evidence.json`, strict doctor found
  `E_REPO_LOCK_STALE`, `E_INTENT_ORPHAN`, and `E_TMP_ORPHAN`.
  `jvs doctor --strict --repair-runtime` returned exit 0, healthy true, and 0
  findings after cleaning locks=1, runtime_tmp=2, and runtime_operations=1.
  Final `jvs doctor --strict` returned exit 0, and final `jvs verify --all`
  returned exit 0 with checkpoint count 4 and 0 bad or tampered checkpoints.

### GA docs evidence

- Release docs: `docs/99_CHANGELOG.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/14_TRACEABILITY_MATRIX.md`, and this ledger describe final release
  facts for `v0.4.1`.
- Changelog scope: final release evidence documentation, CI/release validation
  metadata, and deterministic fuzz smoke hardening after `v0.4.0`.
- Runtime-state migration boundary: active `.jvs/locks/`, `.jvs/intents/`,
  and `.jvs/gc/*.json` runtime state remains non-portable and must be rebuilt
  at the destination.

### Artifact and signing evidence

- Artifact workflow: `.github/workflows/ci.yml` release job.
- Published artifact count: `18`
- Published artifacts: five platform binaries, five `.sig` sidecars, five
  `.pem` sidecars, `SHA256SUMS`, `SHA256SUMS.sig`, and `SHA256SUMS.pem`.
- Signing command family: `cosign sign-blob --yes`
- Verification evidence: final release workflow checked all artifact files are
  non-empty and ran `sha256sum --check --strict SHA256SUMS`.
- Certificate identity for all six `.pem` files:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.1`
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`
- Independent post-publish verification: `sha256sum -c --strict SHA256SUMS`
  passed for all five binaries.

| Asset | SHA-256 |
| --- | --- |
| `jvs-linux-amd64` | `f442290b57e8595f21791249d672022a1957d5ce7b62b063dec5051a77c2011e` |
| `jvs-linux-arm64` | `d12c7eeed19678bd562471f2d07a5dfc98fd91354dddcb8db0c26b46339b6346` |
| `jvs-darwin-amd64` | `a3ec237f1028ba886918b553c56dc23c59bac7b5e3f36699b69ca42f8562be50` |
| `jvs-darwin-arm64` | `d1aa70ac0abbfc80c5045c8565dc3332e391fa09aa601eb0f9bdf067c4d685be` |
| `jvs-windows-amd64.exe` | `7b97d8ab96e8a184b92b4d139ac0d6ce2e26beaf6115001cb7a534cfd7ab22e2` |

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
