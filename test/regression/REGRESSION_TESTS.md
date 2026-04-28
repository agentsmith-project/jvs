# Regression Tests

This directory contains regression tests for fixed JVS bugs. The suite follows
the current public product vocabulary: folders, workspaces, save points,
history, view, restore, recovery, doctor, and cleanup.

## Purpose

Regression tests serve as a permanent record of bugs that were found and fixed.
They:

1. Prevent recurrence by keeping fixed behavior covered.
2. Document the user-visible scenario that broke.
3. Improve confidence when public flows are refactored.
4. Support release and quality evidence.

## Adding a Regression Test

When you fix a bug, add a focused test that exercises the public behavior that
used to fail.

### 1. Create the Test Function

```go
// TestRegression_CleanupLeak tests that cleanup ignores protected save points.
//
// Bug: cleanup marked protected save points as reclaimable
// Fixed: 2026-04-28, PR #456
// Issue: #123
func TestRegression_CleanupLeak(t *testing.T) {
    // Test code here
}
```

### 2. Follow the Naming Convention

Use the format: `TestRegression_<BriefDescription>`

- Use a descriptive name that explains what was broken.
- Use CamelCase with underscores.
- Include issue or PR references in the comment block when available.

### 3. Document the Bug

Each test should include a comment block with:

```go
// TestRegression_CleanupLeak tests that [what it tests].
//
// Bug: [brief description of what was broken]
// Fixed: [YYYY-MM-DD], PR #[number]
// Issue: #[number]
```

### 4. Update This Document

Add an entry to the Regression Test Catalog:

```markdown
| `TestRegression_CleanupLeak` | 2026-04-28, PR #456 | Cleanup ignores protected save points |
```

### 5. Keep the Public Surface Current

Regression tests must use current public commands and JSON fields:

- Create save points with `jvs save -m "message"`.
- Read saved entries with `jvs history`.
- Create workspaces with `jvs workspace new <name> --from <save>`.
- Restore with preview first: `jvs restore <save>`, then `jvs restore --run <plan-id>`.
- Clean up with `jvs cleanup preview`, then `jvs cleanup run --plan-id <plan-id>`.
- Use JSON envelope fields such as `save_point_id`, `newest_save_point`,
  `history_head`, `content_source`, `plan_id`, `protected_save_points`, and
  `reclaimable_save_points`.

Do not add compatibility coverage for removed public commands or removed JSON
field names.

## Running Regression Tests

Regression tests use the `conformance` build tag and are wrapped by the
Makefile target:

```bash
# Run all regression tests
make regression

# Run the package directly
PATH="$(pwd)/bin:$PATH" go test -tags conformance -count=1 -v ./test/regression/...

# Run a specific regression test
PATH="$(pwd)/bin:$PATH" go test -tags conformance -count=1 -v ./test/regression/... -run TestRegression_RestorePreviewRun
```

## Regression Test Catalog

| Test Name | Fixed | Description |
|-----------|-------|-------------|
| `TestRegression_RestoreNonExistentSavePoint` | 2024-02-20 | Restore fails gracefully for a missing save point |
| `TestRegression_SaveRequiresMessage` | 2024-02-20 | Save rejects an empty message with a stable JSON error |
| `TestRegression_HistoryGrepFiltersMessages` | 2024-02-20 | History message filtering returns only matching save points |
| `TestRegression_RestorePreviewRun` | 2024-02-20 | Restore uses preview/run and keeps history unchanged |
| `TestRegression_WorkspaceNewFromSavePoint` | 2024-02-20 | Workspace creation from a save point initializes content and state |
| `TestRegression_CleanupPreviewWithEmptySavePoint` | 2024-02-20 | Cleanup previews and runs successfully with an empty save point |
| `TestRegression_DoctorRuntimeRepair` | 2024-02-20, PR #7d0db0c | Doctor runtime repair reports successful repair actions |
| `TestRegression_StatusCommand` | 2024-02-20, PR #7d0db0c | Status reports folder, workspace, save point, and file state |
| `TestRegression_CanSaveNewWorkspace` | 2026-02-28 | First save succeeds in a newly created workspace |
| `TestRegression_CleanupRespectsProtectedSavePoint` | 2026-02-28 | Cleanup does not reclaim the active save point |
| `TestRegression_RestoreEmptyArgs` | 2026-02-28 | Restore fails gracefully for an empty save point ID |
| `TestRegression_CleanupRunEmptyPlanID` | 2026-02-28 | Cleanup run fails gracefully for an empty plan ID |

## Test Categories

### Save Point Operations

- `TestRegression_SaveRequiresMessage` - Empty message handling
- `TestRegression_CleanupPreviewWithEmptySavePoint` - Empty saved content handling
- `TestRegression_CanSaveNewWorkspace` - First save in a new workspace

### Restore Operations

- `TestRegression_RestoreNonExistentSavePoint` - Missing save point handling
- `TestRegression_RestorePreviewRun` - Preview-first restore flow
- `TestRegression_RestoreEmptyArgs` - Empty restore target handling

### History And Status

- `TestRegression_HistoryGrepFiltersMessages` - Message filtering
- `TestRegression_StatusCommand` - Folder and save point status

### Workspace Management

- `TestRegression_WorkspaceNewFromSavePoint` - Workspace creation from a save point

### Cleanup

- `TestRegression_CleanupPreviewWithEmptySavePoint` - Preview and run with empty saved content
- `TestRegression_CleanupRespectsProtectedSavePoint` - Protected save point handling
- `TestRegression_CleanupRunEmptyPlanID` - Plan ID validation

### Doctor

- `TestRegression_DoctorRuntimeRepair` - Runtime repair execution

## When Not To Add A Regression Test

Regression tests are not for:

1. New features. Use the standard test suite.
2. Refactoring. Improve existing tests unless a historical bug is involved.
3. Performance issues. Use benchmarks.
4. Documentation bugs. Fix the docs directly.
5. Trivial fixes where a test would not prevent recurrence.

## Best Practices

### Do

- Make tests independent and isolated.
- Use clear, descriptive test names.
- Exercise the exact public scenario that broke.
- Keep tests fast and focused.
- Use existing helpers such as `runJVS`, `runJVSInRepo`, and `createFiles`.
- Prefer JSON assertions for machine-readable public contracts.

### Do Not

- Do not mock at a level that hides the bug.
- Do not test implementation details when public behavior is enough.
- Do not make tests depend on each other.
- Do not add tests for unfixed bugs.
- Do not keep compatibility tests for removed public behavior.

## Review Process

Regression tests require:

1. Issue or PR reference when available.
2. Clear documentation of the bug.
3. Verification that the test fails against the broken behavior and passes on
   the fix.
4. Alignment with the current public command and JSON contract.
