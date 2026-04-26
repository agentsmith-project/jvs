# Traceability Matrix (v0)

This matrix maps product promises to normative specs and conformance contract
areas.

## Promise 1: Current/latest workspace state model
- Product statement:
  - `README.md` (Core guarantees)
  - `docs/00_OVERVIEW.md` (Product promise)
- Normative specs:
  - `docs/06_RESTORE_SPEC.md` (in-place restore, current/latest state, fork command)
  - `docs/02_CLI_SPEC.md` (`restore` and `fork` contract)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Status, Refs, and State
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Checkpoint, Diff, Restore, and Fork
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Dirty Guards

## Promise 2: Verifiable tamper-evident checkpoint lineage
- Product statement:
  - `README.md` (strong default verification)
  - `docs/00_OVERVIEW.md` (verification model)
- Normative specs:
  - `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` (descriptor schema incl. payload hash)
  - `docs/05_SNAPSHOT_ENGINE_SPEC.md` (payload hash generation + READY/durability)
  - `docs/09_SECURITY_MODEL.md` (integrity model and audit)
  - `docs/02_CLI_SPEC.md` (`verify` default strong mode)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Integrity, Doctor, and Retention

## Promise 3: Safe migration semantics
- Product statement:
  - `README.md` (exclude runtime state)
  - `docs/00_OVERVIEW.md` (runtime-state non-portable)
  - Runtime-state boundary: active `.jvs/locks/`, `.jvs/intents/`, and
    `.jvs/gc/*.json` are non-portable.
- Normative specs:
  - `docs/18_MIGRATION_AND_BACKUP.md` (exclude mutation locks, operation records, and active GC plans; rebuild runtime)
  - `docs/01_REPO_LAYOUT_SPEC.md` (portability classes)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Setup, Clone, and Capability
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Integrity, Doctor, and Retention

## Promise 4: Safe cleanup and deletion
- Product statement:
  - `docs/00_OVERVIEW.md` (verifiable checkpoint lineage, operational safety)
- Normative specs:
  - `docs/08_GC_SPEC.md` (plan/mark/commit protocol)
  - `docs/02_CLI_SPEC.md` (`gc plan`, `gc run --plan-id`)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Integrity, Doctor, and Retention

## Promise 5: Auditable operation trail with tamper evidence
- Product statement:
  - `docs/00_OVERVIEW.md` (verifiable and tamper-evident operation trail)
- Normative specs:
  - `docs/09_SECURITY_MODEL.md` (audit log format, hash chain, record schema)
  - `docs/02_CLI_SPEC.md` (`doctor` audit chain validation)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Integrity, Doctor, and Retention

## Promise 6: Deterministic checkpoint identity and integrity
- Product statement:
  - `docs/00_OVERVIEW.md` (verifiable checkpoint lineage)
- Normative specs:
  - `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` (checkpoint ID generation)
  - `docs/05_SNAPSHOT_ENGINE_SPEC.md` (payload root hash computation)
  - `docs/09_SECURITY_MODEL.md` (integrity hash algorithms)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Checkpoint, Diff, Restore, and Fork
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Integrity, Doctor, and Retention

## Promise 7: Pure payload roots with centralized control plane
- Product statement:
  - `docs/CONSTITUTION.md` §2.3 (control-plane/data-plane separation)
  - `docs/CONSTITUTION.md` §4.2 (JuiceFS clone lacks exclude filters)
- Normative specs:
  - `docs/01_REPO_LAYOUT_SPEC.md` (layout invariants, workspace discovery)
  - `docs/03_WORKTREE_SPEC.md` (centralized metadata under `.jvs/worktrees/`)
  - `docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md` (no exclusion logic required)
- Conformance contract areas:
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Repo and Workspace Resolution
  - `docs/11_CONFORMANCE_TEST_PLAN.md` §Setup, Clone, and Capability

## Release gating trace
- Normative release policy:
  - `docs/12_RELEASE_POLICY.md`
- Required operational checks:
  - `docs/13_OPERATION_RUNBOOK.md`
- Conformance execution:
  - `docs/11_CONFORMANCE_TEST_PLAN.md`
- Phase 4 GA artifacts:
  - `docs/99_CHANGELOG.md` (changelog and generated release-note source)
  - `docs/RELEASE_EVIDENCE.md` (persistent release evidence ledger)
  - `.github/workflows/ci.yml` (CI release workflow and release notes gate)
  - `docs/PERFORMANCE_RESULTS.md` (GA performance result boundaries)
  - `docs/18_MIGRATION_AND_BACKUP.md` (migration notes)
  - `SECURITY.md` and `docs/SIGNING.md` (artifact verification)
