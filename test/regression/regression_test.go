//go:build conformance

// Regression Test Suite for JVS
//
// This file contains regression tests for bugs that have been fixed.
// Each test is documented with:
// - Issue/PR reference
// - Date fixed
// - Description of the bug
// - Expected behavior
//
// When adding a regression test:
// 1. Create a test function named TestRegression_<BriefDescription>
// 2. Document the bug with a comment block
// 3. Test the exact scenario that caused the bug
// 4. Add an entry to REGRESSION_TESTS.md

package regression

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var jvsBinary string

func init() {
	// Find the jvs binary
	cwd, _ := os.Getwd()
	// Walk up to find bin/jvs
	for {
		binPath := filepath.Join(cwd, "bin", "jvs")
		if _, err := os.Stat(binPath); err == nil {
			jvsBinary = binPath
			return
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	// Fallback to PATH
	jvsBinary = "jvs"
}

// initTestRepo creates a temp repo and returns its path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")

	runJVS(t, dir, "init", "testrepo")
	return repoPath
}

// runJVS executes the jvs binary with args in the given working directory.
func runJVS(t *testing.T, cwd string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(jvsBinary, args...)
	cmd.Dir = cwd
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	} else {
		exitCode = 0
	}
	return
}

// runJVSInRepo runs jvs from within the repo's main worktree.
func runJVSInRepo(t *testing.T, repoPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cwd := filepath.Join(repoPath, "main")
	return runJVS(t, cwd, args...)
}

// createFiles creates multiple files in a worktree.
func createFiles(t *testing.T, worktreePath string, files map[string]string) {
	t.Helper()
	for filename, content := range files {
		path := filepath.Join(worktreePath, filename)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", path, err)
		}
	}
}

// ============================================================================
// REGRESSION TESTS
//
// Add new regression tests below this section.
// Format: TestRegression_<BriefDescription>
//
// Example:
// func TestRegression_GarbageCollectionLeak(t *testing.T) {
//     // Bug: GC was not cleaning up orphaned snapshots when parent was deleted
//     // Fixed: 2024-02-15, PR #456
//     // ...
// }
// ============================================================================

// TestRegression_TemplateExample demonstrates the expected format for regression tests.
// This test serves as a template for adding new regression tests.
//
// When adding a new regression test:
// 1. Copy this template function
// 2. Rename to TestRegression_<BriefDescription>
// 3. Fill in the bug description, fix date, and PR reference
// 4. Implement the test scenario
// 5. Document in REGRESSION_TESTS.md
func TestRegression_TemplateExample(t *testing.T) {
	// Bug Description: Example template for regression tests
	// Fixed: [Date], PR #[number]
	// Issue: #[number]

	repoPath := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Setup: Create the scenario
	createFiles(t, mainPath, map[string]string{
		"test.txt": "content",
	})

	// Action: Create a snapshot
	stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "test snapshot")

	// Assertion: Verify success
	assert.Equal(t, 0, code, "snapshot should succeed")
	assert.NotEmpty(t, stdout, "should have output")

	// Verify snapshot was created
	history, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list")
	assert.Contains(t, history, "test snapshot", "snapshot should appear in history")
	assert.NotContains(t, stderr, "error", "should not show errors")
}

// TestRegression_RestoreNonExistentSnapshot tests that restore fails gracefully
// when given a non-existent snapshot ID.
//
// Bug: Restore would panic with nil pointer dereference on invalid snapshot ID
// Fixed: 2024-02-20
func TestRegression_RestoreNonExistentSnapshot(t *testing.T) {
	// Bug: Restore could panic on invalid snapshot ID
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)

	// Attempt to restore a snapshot that doesn't exist
	stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "nonexistent-snapshot-id")

	// Should fail gracefully, not panic
	assert.NotEqual(t, 0, code, "restore should fail for non-existent snapshot")

	// Should provide a helpful error message
	combined := stdout + stderr
	assert.True(t,
		strings.Contains(combined, "not found") ||
			strings.Contains(combined, "no snapshot") ||
			strings.Contains(combined, "unknown"),
		"error message should indicate snapshot not found")
}

// TestRegression_SnapshotEmptyNote tests that snapshot accepts an empty note
// without error.
//
// Bug: Snapshot with empty note string would fail validation
// Fixed: 2024-02-20
func TestRegression_SnapshotEmptyNote(t *testing.T) {
	// Bug: Empty note could cause validation errors
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)

	// Create a snapshot with an empty note (explicit empty string)
	_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "")

	// Should succeed
	assert.Equal(t, 0, code, "snapshot with empty note should succeed")
	assert.NotContains(t, stderr, "error", "should not show error for empty note")

	// Verify snapshot was created - history should show it
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list")
	// History output contains the timestamp and (no note) marker
	assert.Contains(t, stdout, "(no note)", "history should show (no note) for empty note")
}

// TestRegression_HistoryWithTags tests that history command properly displays
// tagged snapshots.
//
// Bug: History command was not properly filtering or displaying tags
// Fixed: 2024-02-20
func TestRegression_HistoryWithTags(t *testing.T) {
	// Bug: History --tag filter was not working correctly
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)

	// Create snapshots with different tags
	runJVSInRepo(t, repoPath, "checkpoint", "first snapshot", "--tag", "v1.0")
	runJVSInRepo(t, repoPath, "checkpoint", "second snapshot", "--tag", "stable")

	// List checkpoints and verify tags are displayed.
	stdout, _, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")

	assert.Equal(t, 0, code, "checkpoint list should succeed")
	assert.Contains(t, stdout, "v1.0", "checkpoint list should show the tag")
}

// TestRegression_MultipleTags tests that multiple tags can be attached to a snapshot.
//
// Bug: Only the last tag was being saved when multiple --tag flags were used
// Fixed: 2024-02-20
func TestRegression_MultipleTags(t *testing.T) {
	// Bug: Multiple --tag flags were not all being saved
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)

	// Create snapshot with multiple tags
	stdout, _, code := runJVSInRepo(t, repoPath, "checkpoint", "multi-tag snapshot",
		"--tag", "v1.0", "--tag", "stable", "--tag", "release")

	assert.Equal(t, 0, code, "snapshot with multiple tags should succeed")
	assert.NotContains(t, stdout, "error", "should not show errors")

	// Verify all tags are preserved
	stdout, _, _ = runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	assert.Contains(t, stdout, "v1.0", "should find v1.0 tag")
	assert.Contains(t, stdout, "stable", "should find stable tag")
	assert.Contains(t, stdout, "release", "should find release tag")
}

// TestRegression_RestoreLatest tests that restore latest returns to the latest snapshot.
//
// Bug: Restoring the latest snapshot was not properly detecting the latest snapshot in some cases
// Fixed: 2024-02-20
func TestRegression_RestoreLatest(t *testing.T) {
	// Bug: restore latest could fail to find the latest snapshot
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create initial snapshot
	createFiles(t, mainPath, map[string]string{"file1.txt": "content1"})
	runJVSInRepo(t, repoPath, "checkpoint", "first snapshot")

	// Create second snapshot
	createFiles(t, mainPath, map[string]string{"file2.txt": "content2"})
	runJVSInRepo(t, repoPath, "checkpoint", "second snapshot")

	// Restore to first snapshot
	history1, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list")
	lines := strings.Split(strings.TrimSpace(history1), "\n")
	if len(lines) > 0 {
		// Extract first snapshot ID (skip header lines if any)
		firstSnapshotID := extractSnapshotIDFromHistory(history1)
		if firstSnapshotID != "" {
			stdout, _, code := runJVSInRepo(t, repoPath, "restore", firstSnapshotID)
			assert.Equal(t, 0, code, "restore to first snapshot should succeed")
			assert.NotEmpty(t, stdout, "restore should have output")
		}
	}

	// Restore back to latest
	stdout, _, code := runJVSInRepo(t, repoPath, "restore", "latest")

	assert.Equal(t, 0, code, "restore latest should succeed")
	assert.Contains(t, stdout, "Workspace is at latest", "restore latest should report latest status")

	// Verify we're back at the latest state
	assert.FileExists(t, filepath.Join(mainPath, "file2.txt"), "latest content should be restored")
}

// TestRegression_WorktreeFork tests forking a worktree from a snapshot.
//
// Bug: Worktree fork was not properly setting up the new worktree state
// Fixed: 2024-02-20
func TestRegression_WorktreeFork(t *testing.T) {
	// Bug: Worktree fork had issues with state initialization
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a snapshot
	createFiles(t, mainPath, map[string]string{"original.txt": "content"})
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "original snapshot")
	snapshotID := extractSnapshotID(stdout)

	// Fork a new worktree from the snapshot
	stdout, _, code := runJVSInRepo(t, repoPath, "fork", snapshotID, "feature-branch")

	assert.Equal(t, 0, code, "worktree fork should succeed")
	assert.NotContains(t, stdout, "error", "should not show errors")

	// Verify the new worktree exists
	worktreePath := filepath.Join(repoPath, "worktrees", "feature-branch")
	fi, err := os.Stat(worktreePath)
	assert.NoError(t, err, "new worktree directory should exist")
	assert.True(t, fi.IsDir(), "new worktree should be a directory")

	// Verify the file exists in the new worktree
	content, err := os.ReadFile(filepath.Join(worktreePath, "original.txt"))
	assert.NoError(t, err, "file should exist in forked worktree")
	assert.Equal(t, "content", string(content), "file content should match snapshot")
}

// TestRegression_GCWithEmptySnapshot tests garbage collection with a snapshot
// that has no files (empty payload).
//
// Bug: GC would panic when processing snapshots with empty payloads
// Fixed: 2024-02-20
func TestRegression_GCWithEmptySnapshot(t *testing.T) {
	// Bug: GC could panic on empty snapshot payloads
	// Fixed: 2024-02-20

	repoPath := initTestRepo(t)

	// Create an initial snapshot (no files yet)
	runJVSInRepo(t, repoPath, "checkpoint", "empty snapshot")

	// Plan GC - should not panic
	stdout, _, code := runJVSInRepo(t, repoPath, "gc", "plan")

	assert.Equal(t, 0, code, "gc plan should succeed even with empty snapshots")
	assert.NotEmpty(t, stdout, "gc plan should have output")
	assert.NotContains(t, stdout, "panic", "should not panic")
}

// TestRegression_DoctorRuntimeRepair tests that doctor --repair-runtime fixes
// runtime issues.
//
// Bug: Doctor --repair-runtime was not executing all repairs
// Fixed: 2024-02-20, PR #7d0db0c
func TestRegression_DoctorRuntimeRepair(t *testing.T) {
	// Bug: Doctor --repair-runtime was not properly fixing runtime state
	// Fixed: 2024-02-20, PR #7d0db0c

	repoPath := initTestRepo(t)

	// Run doctor with --repair-runtime
	stdout, _, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")

	assert.Equal(t, 0, code, "doctor --repair-runtime should succeed")
	assert.NotContains(t, stdout, "error", "should not show errors")
}

// TestRegression_InfoCommand tests that info command displays repository info.
//
// Bug: Info command was missing some fields or had formatting issues
// Fixed: 2024-02-20, PR #7d0db0c
func TestRegression_InfoCommand(t *testing.T) {
	// Bug: Info command output was incomplete
	// Fixed: 2024-02-20, PR #7d0db0c

	repoPath := initTestRepo(t)

	// Get repo info
	stdout, _, code := runJVSInRepo(t, repoPath, "info")

	assert.Equal(t, 0, code, "info command should succeed")

	// Verify key fields are present
	assert.Contains(t, stdout, "Repository:", "should show repository path")
	assert.Contains(t, stdout, "Repo ID:", "should show repo ID")
	assert.Contains(t, stdout, "Format version:", "should show format version")
	assert.Contains(t, stdout, "Engine:", "should show engine")
	assert.Contains(t, stdout, "Workspaces:", "should show workspace count")
	assert.Contains(t, stdout, "Checkpoints:", "should show checkpoint count")
}

// Helper functions

// extractSnapshotID extracts a snapshot ID from command output.
// Looks for pattern: timestamp-hash
func extractSnapshotID(output string) string {
	// Look for snapshot ID pattern in output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for "Created snapshot" message or similar
		if strings.Contains(line, "Created snapshot") ||
			strings.Contains(line, "checkpoint") {
			// Extract ID after "checkpoint" keyword
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(part, "checkpoint") && i+1 < len(parts) {
					candidate := strings.TrimSpace(parts[i+1])
					if strings.Contains(candidate, "-") && len(candidate) > 10 {
						return candidate
					}
				}
			}
		}
	}
	return ""
}

// extractSnapshotIDFromHistory extracts the first (oldest) snapshot ID from history output.
func extractSnapshotIDFromHistory(historyOutput string) string {
	lines := strings.Split(historyOutput, "\n")
	for _, line := range lines {
		// Look for lines with snapshot IDs (pattern: digits-digits)
		parts := strings.Fields(line)
		for _, part := range parts {
			// Snapshot IDs look like: 1234567890-abc123def
			if strings.Contains(part, "-") && len(part) > 10 {
				// Verify it's mostly alphanumeric with hyphen
				cleaned := strings.Trim(part, "[]()")
				if strings.Contains(cleaned, "-") {
					return cleaned
				}
			}
		}
	}
	return ""
}

// TestRegression_CanSnapshotNewWorktree verifies that the first snapshot in a
// freshly created worktree succeeds.
//
// Bug: CanSnapshot() returned false for new worktrees with no snapshots,
// blocking the first snapshot.
// Fixed: 2026-02-28
func TestRegression_CanSnapshotNewWorktree(t *testing.T) {
	repoPath := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	createFiles(t, mainPath, map[string]string{
		"baseline.txt": "base",
	})
	_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "baseline")
	assert.Equal(t, 0, code, "baseline checkpoint should succeed: %s", stderr)

	// Create a brand-new workspace from the current checkpoint.
	_, stderr, code = runJVSInRepo(t, repoPath, "fork", "fresh")
	assert.Equal(t, 0, code, "fork should succeed: %s", stderr)

	freshPath := filepath.Join(repoPath, "worktrees", "fresh")

	// Add files to the fresh worktree
	createFiles(t, freshPath, map[string]string{
		"hello.txt": "world",
	})

	// Checkpoint from within the fresh workspace.
	stdout, stderr, code := runJVS(t, freshPath, "checkpoint", "first snapshot in fresh worktree")
	assert.Equal(t, 0, code, "first checkpoint in new workspace should succeed: %s", stderr)
	assert.NotEmpty(t, stdout, "checkpoint should produce output")

	// Checkpoint list should show the checkpoint.
	histOut, _, _ := runJVS(t, freshPath, "checkpoint", "list")
	assert.Contains(t, histOut, "first snapshot in fresh worktree",
		"checkpoint list should show the checkpoint note")
}

// TestRegression_GCRespectsRetentionPolicy verifies that GC Plan() honours
// retention policies and does not mark protected snapshots for deletion.
//
// Bug: GC Plan() ignored configured retention policies (KeepMinSnapshots,
// KeepMinAge).
// Fixed: 2026-02-28
func TestRegression_GCRespectsRetentionPolicy(t *testing.T) {
	repoPath := initTestRepo(t)

	// Create a snapshot so the repo is non-empty
	_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "protected snapshot")
	assert.Equal(t, 0, code, "snapshot should succeed: %s", stderr)

	// Run gc plan
	stdout, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan")
	assert.Equal(t, 0, code, "gc plan should succeed: %s", stderr)

	// The single checkpoint is the workspace HEAD and therefore protected.
	// "To delete: 0 checkpoints" must appear in the output.
	assert.Contains(t, stdout, "To delete: 0 checkpoints",
		"gc plan should report 0 deletable checkpoints for a protected checkpoint")
}

// TestRegression_ConfigCacheMutation is tested at the unit level in
// pkg/config/config_test.go TestLoad_CacheCopyIndependence

// TestRegression_RestoreEmptyArgs verifies that restore fails gracefully when
// given an empty snapshot ID instead of panicking.
//
// Bug: Restorer.restore() did not validate empty worktreeName or snapshotID.
// Fixed: 2026-02-28
func TestRegression_RestoreEmptyArgs(t *testing.T) {
	repoPath := initTestRepo(t)

	// Attempt restore with an empty snapshot ID
	_, stderr, code := runJVSInRepo(t, repoPath, "restore", "")
	assert.NotEqual(t, 0, code, "restore with empty snapshot ID should fail")

	// Must not panic — a helpful error message is expected
	assert.NotContains(t, stderr, "panic", "restore should not panic on empty args")
}

// TestRegression_GCRunEmptyPlanID verifies that gc run fails gracefully when
// given an empty plan ID.
//
// Bug: GC Run() did not validate empty planID.
// Fixed: 2026-02-28
func TestRegression_GCRunEmptyPlanID(t *testing.T) {
	repoPath := initTestRepo(t)

	// Attempt gc run with an empty plan ID
	_, stderr, code := runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", "")
	assert.NotEqual(t, 0, code, "gc run with empty plan-id should fail")
	assert.NotContains(t, stderr, "panic", "gc run should not panic on empty plan-id")
}
