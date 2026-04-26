# Release Evidence Ledger

This ledger records compact release evidence. It has two evidence classes:

- GA candidate readiness: pre-tag evidence for a release candidate. It may
  record required commands, expected artifact rules, and pending final checks,
  but it is not final, not tagged, and not published.
- Final tagged release: immutable evidence for an existing tag. Only this
  class may record `Status: PASS`, a final `Tag`, a final tagged commit, and
  published artifact counts.

Raw logs and `coverage.out` are not stored here.

## v0.4.0 - 2026-04-25

### Release identity

- Evidence class: GA candidate readiness
- Candidate target tag: `v0.4.0`
- Date: `2026-04-25`
- Final release status: not final, not tagged, not published; pending final tag
  and release workflow execution.
- Final tagged commit: pending final tag.
- CI run link rule: `https://github.com/agentsmith-project/jvs/actions/runs/<run_id>`
- Expected final release URL rule:
  `https://github.com/agentsmith-project/jvs/releases/tag/v0.4.0`

### Release gate summary

Command: `make release-gate`

The table lists the required final tagged release evidence. Candidate entries
must remain pending until the checks run on the final tagged commit.

| Check | Command or target | Candidate status |
| --- | --- | --- |
| Release gate | `make release-gate` | Pending final tagged run |
| Docs contract | `make docs-contract` | Pending final tagged run |
| CI contract | `make ci-contract` | Pending final tagged run |
| Race tests | `make test-race` | Pending final tagged run |
| Coverage | `make test-cover` | Pending final tagged run |
| Lint | `make lint` | Pending final tagged run |
| Build | `make build` | Pending final tagged run |
| Conformance | `make conformance` | Pending final tagged run |
| Library facade | `make library` | Pending final tagged run |
| Regression | `make regression` | Pending final tagged run |
| Fuzz ordinary tests | `make fuzz-tests` | Pending final tagged run |
| Fuzz smoke | `make fuzz` | Pending final tagged run |

### Coverage

- Coverage total: pending final `make test-cover` run.
- Coverage threshold: `60.0%`
- Evidence command: `make test-cover`
- Evidence summary: pending final tagged run.

### Representative repo evidence

- Representative repo class: existing v0 repo with checkpoint history,
  workspace state, audit chain, and runtime repair path exercised.
- Doctor command: `jvs doctor --strict`
- Doctor result: pending final tagged run, including audit chain validation.
- Verify command: `jvs verify --all`
- Verify result: pending final tagged run, including checkpoint descriptor and
  payload integrity.
- Migration repair command for copied repos:
  `jvs doctor --strict --repair-runtime`

### GA docs evidence

- Candidate docs: `docs/99_CHANGELOG.md`, `docs/12_RELEASE_POLICY.md`,
  `docs/14_TRACEABILITY_MATRIX.md`, and this ledger describe candidate
  readiness, not final release facts.
- Migration terminology: public terms remain checkpoint and workspace;
  `.jvs/snapshots` and `.jvs/worktrees` are compatibility storage names.
- Runtime-state migration boundary: mutation lock directories, operation
  records, and active GC plans are non-portable and rebuilt at the
  destination.
- Public command docs and conformance plan use active runtime operation state
  and stable v0 command flags.

### Artifact and signing evidence

- Artifact evidence class: expected final release workflow output; no
  published artifacts are recorded by this GA candidate readiness entry.
- Artifact workflow: `.github/workflows/ci.yml` release job.
- Expected final artifacts: five platform binaries, five `.sig` sidecars, five
  `.pem` sidecars, `SHA256SUMS`, `SHA256SUMS.sig`, and `SHA256SUMS.pem`.
- Signing command family: `cosign sign-blob --yes`
- Verification evidence: final release workflow checks all artifact files are
  non-empty and runs `sha256sum --check --strict SHA256SUMS`.
- Certificate identity rule:
  `https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@<workflow-ref>`
- OIDC issuer: `https://token.actions.githubusercontent.com`

### Runbook references

- Verification and recovery: `docs/13_OPERATION_RUNBOOK.md`
- Migration and backup: `docs/18_MIGRATION_AND_BACKUP.md`
- Artifact signing and verification: `docs/SIGNING.md`
