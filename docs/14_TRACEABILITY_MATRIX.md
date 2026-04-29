# Traceability Matrix

**Status:** active save point traceability matrix

## Promise 1: Real Folder Save Points

Normative docs:

- `docs/02_CLI_SPEC.md`
- `docs/PRODUCT_PLAN.md`
- `docs/ARCHITECTURE.md`

Evidence:

- init/status/save/history conformance
- docs-contract help-surface checks

Supporting non-release-facing reference:

- `docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`

## Promise 2: Restore Is Preview-First And Recoverable

Normative docs:

- `docs/06_RESTORE_SPEC.md`
- `docs/13_OPERATION_RUNBOOK.md`
- `docs/02_CLI_SPEC.md`

Evidence:

- restore preview/run conformance
- path restore conformance
- recovery status/resume/rollback tests, including live-derived recommended
  next commands and backup-unavailable status behavior
- restore drill recorded in release evidence

## Promise 3: Workspace Creation Starts New History

Normative docs:

- `docs/02_CLI_SPEC.md`
- `docs/03_WORKTREE_SPEC.md`
- `docs/PRODUCT_PLAN.md`

Evidence:

- `workspace new --from <save>` conformance
- entered workspace folder `status` and first-save story coverage
- status JSON `started_from_save_point`
- first-save provenance tests

## Promise 4: Control Data Is Not Payload

Normative docs:

- `docs/01_REPO_LAYOUT_SPEC.md`
- `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md`
- `docs/09_SECURITY_MODEL.md`

Evidence:

- payload purity and generic boundary tests
- migration/runtime-state boundary tests
- doctor layout checks

## Promise 5: Integrity And Audit Are Verifiable

Normative docs:

- `docs/09_SECURITY_MODEL.md`
- `docs/05_SNAPSHOT_ENGINE_SPEC.md`
- `docs/12_RELEASE_POLICY.md`

Evidence:

- strict doctor tests
- audit-chain tests
- integrity evidence recorded through the public health path

## Promise 6: Cleanup Is Review-First

Normative docs:

- `docs/08_GC_SPEC.md`
- `docs/13_OPERATION_RUNBOOK.md`
- `docs/18_MIGRATION_AND_BACKUP.md`

Evidence:

- cleanup protection and protection-group tests
- migration tests excluding runtime cleanup plan files

## Promise 7: Engine Claims Are Scoped

Normative docs:

- `docs/05_SNAPSHOT_ENGINE_SPEC.md`
- `docs/PERFORMANCE.md`
- `docs/PERFORMANCE_RESULTS.md`
- `docs/BENCHMARKS.md`

Evidence:

- benchmark package evidence
- engine fallback/degradation checks
- docs checks forbidding unconditional O(1) claims

## Promise 8: Candidate And Final Evidence Are Separate

Normative docs:

- `docs/12_RELEASE_POLICY.md`
- `docs/99_CHANGELOG.md`
- `docs/RELEASE_EVIDENCE.md`

Evidence:

- release evidence ledger checks
- changelog readiness sections
- final tag validation checks

## Promise 9: GA Stories Preserve User Mental Models

Normative docs:

- `docs/20_USER_SCENARIOS.md`
- `docs/PRODUCT_PLAN.md`

Supporting research:

- `docs/TARGET_USERS.md`

Evidence:

- `docs/11_CONFORMANCE_TEST_PLAN.md` User Story Matrix Coverage
- `make story-e2e` for current generic story coverage: managed folder save and
  restore, managed path discovery/view/restore, deletion recovery, restore
  preview/run, and workspace creation
- boundary conformance for JVS control data, workspace locators, restore plans,
  recovery plans, cleanup plans, cache-like unrelated paths, and read-only
  large file or directory views
- `make story-juicefs-local` when the real local JuiceFS profile is required
- Product Design Improvement Candidates in `docs/20_USER_SCENARIOS.md` for
  future ideas that are not GA commitments
- domain-specific presets remain outside the GA plan unless promoted into the
  public contract
