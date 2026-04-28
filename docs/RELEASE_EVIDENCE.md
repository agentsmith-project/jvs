# Release Evidence Ledger

This ledger records compact release evidence for the active save point product
contract. Because JVS has not reached GA, earlier draft material is not carried
forward as active release evidence.

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

## v0.4.2 - 2026-04-27

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.2`
- Candidate status: not final, not tagged, and not published; pending final tag
  creation and release-gate qualification.
- Changelog heading date: `2026-04-27`
- Baseline: save point public-contract convergence and GA docs alignment.
- CI run link rule:
  `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Release URL rule after publication:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2`
- Canonical release URL example:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`
- Scope: public save point CLI/docs convergence, restore preview/run/recovery,
  `workspace new --from <save>`, cleanup preview/run semantics, EMPTY folder
  first-save docs and acceptance coverage, user-story E2E coverage, JuiceFS
  story qualification requirements, destination-owned transfer staging checks,
  and GA release evidence alignment.

### Release gate summary

Command required before final tag: `make release-gate`

The table lists required checks for the candidate. Final-only results remain
pending final tag qualification and the tag-gated release workflow.

| Check | Command or target | Candidate evidence |
| --- | --- | --- |
| Release gate | `make release-gate` | required before pending final tag |
| Docs contract | `make docs-contract` | required after save point docs convergence and again in final release gate |
| CI contract | `make ci-contract` | required before pending final tag |
| Race tests | `make test-race` | required before pending final tag |
| Coverage | `make test-cover` | required before pending final tag |
| Lint | `make lint` | required before pending final tag |
| Build | `make build` | required before pending final tag |
| Release cross-build | `make release-build` | required before pending final tag; builds the five release artifacts |
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

- Representative repo class: required existing repo with save point history,
  workspace state, audit chain validation, restore preview/run/recovery drill,
  and runtime repair path.
- Doctor command: `jvs doctor --strict`
- Doctor result: pending final release qualification.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`
- Required evidence: final release qualification must record strict doctor,
  integrity, restore drill, recovery drill, and runtime repair results before
  this candidate can be converted into final release evidence.

### GA docs evidence

- GA docs: `docs/02_CLI_SPEC.md`, `docs/06_RESTORE_SPEC.md`,
  `docs/12_RELEASE_POLICY.md`, `docs/13_OPERATION_RUNBOOK.md`,
  `docs/18_MIGRATION_AND_BACKUP.md`, `docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`,
  `docs/99_CHANGELOG.md`, `docs/PRODUCT_PLAN.md`, `docs/ARCHITECTURE.md`, and
  this ledger define the release readiness and evidence contract for `v0.4.2`.
- Changelog scope: save point public CLI/docs convergence, restore
  preview/run/recovery, workspace new semantics, cleanup preview/run
  semantics, EMPTY folder first-save docs/acceptance alignment, user-story E2E
  and JuiceFS story coverage, destination-owned transfer staging checks, and
  GA release evidence alignment.
- Runtime-state migration boundary: active `.jvs/locks/`, `.jvs/intents/`,
  and `.jvs/gc/*.json` runtime state remains non-portable and must be rebuilt
  at the destination.

### Artifact and signing evidence

- Artifact workflow: `.github/workflows/ci.yml` release job.
- Release gate includes `make release-build`, matching the release job's five
  platform binaries: Linux x86_64, Linux ARM64, macOS x86_64, macOS ARM64, and
  Windows x86_64.
- Release toolchain smoke: non-publishing `release-toolchain-smoke` job installs
  `sigstore/cosign-installer@v4.1.1` with `cosign-release: v3.0.5` and
  verifies `cosign version` on pull request, main push, tag push, and
  `workflow_dispatch` paths before release publication.
- Artifact publication: not published for this candidate; artifact counts,
  checksums, and download validation are pending final tag publication.
- Signing workflow: final artifacts must include five platform binaries, five
  matching binary `.bundle` files, `SHA256SUMS`, and `SHA256SUMS.bundle`
  produced by the tag-gated release workflow.
- Signing command family:
  `cosign sign-blob --yes --bundle=<artifact>.bundle <artifact>`
- Pre-upload verification: the release job must run `test -s` for every
  published artifact, `sha256sum --check --strict SHA256SUMS`, and
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
