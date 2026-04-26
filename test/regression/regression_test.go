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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type regressionJSONEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

type regressionCheckpointData struct {
	CheckpointID string `json:"checkpoint_id"`
}

type regressionRestoreData struct {
	CheckpointID string `json:"checkpoint_id"`
	Current      string `json:"current"`
	Latest       string `json:"latest"`
	Dirty        bool   `json:"dirty"`
	AtLatest     bool   `json:"at_latest"`
	Status       string `json:"status"`
}

type regressionStatusData struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	Dirty    bool   `json:"dirty"`
	AtLatest bool   `json:"at_latest"`
}

type regressionForkData struct {
	Workspace      string `json:"workspace"`
	BaseCheckpoint string `json:"base_checkpoint"`
	Current        string `json:"current"`
	Latest         string `json:"latest"`
}

func runJVSJSONInRepo(t *testing.T, repoPath string, args ...string) (stdout, stderr string) {
	t.Helper()
	allArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVSInRepo(t, repoPath, allArgs...)
	require.Equal(t, 0, code, "jvs --json %s failed\nstdout=%s\nstderr=%s", strings.Join(args, " "), stdout, stderr)
	return stdout, stderr
}

func requireRegressionJSONData(t *testing.T, stdout string, target any) {
	t.Helper()

	var envelope regressionJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout should be a JSON envelope:\n%s", stdout)
	require.True(t, envelope.OK, "JSON envelope should be ok:\n%s", stdout)
	require.NoError(t, json.Unmarshal(envelope.Data, target), "JSON envelope data should match target:\n%s", stdout)
}

func createRegressionCheckpoint(t *testing.T, repoPath, note string) string {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "checkpoint", note)
	var data regressionCheckpointData
	requireRegressionJSONData(t, stdout, &data)
	require.NotEmpty(t, data.CheckpointID, "checkpoint JSON should include full checkpoint_id:\n%s", stdout)
	return data.CheckpointID
}

func restoreRegressionCheckpoint(t *testing.T, repoPath, ref string) regressionRestoreData {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "restore", ref)
	var data regressionRestoreData
	requireRegressionJSONData(t, stdout, &data)
	require.NotEmpty(t, data.CheckpointID, "restore JSON should include checkpoint_id:\n%s", stdout)
	return data
}

func readRegressionStatus(t *testing.T, repoPath string) regressionStatusData {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "status")
	var data regressionStatusData
	requireRegressionJSONData(t, stdout, &data)
	return data
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
	firstCheckpointID := createRegressionCheckpoint(t, repoPath, "first snapshot")

	// Create second snapshot
	createFiles(t, mainPath, map[string]string{"file2.txt": "content2"})
	secondCheckpointID := createRegressionCheckpoint(t, repoPath, "second snapshot")

	// Restore to the first checkpoint by full JSON checkpoint ID.
	restoredFirst := restoreRegressionCheckpoint(t, repoPath, firstCheckpointID)
	assert.Equal(t, firstCheckpointID, restoredFirst.CheckpointID, "restore should target first checkpoint")
	assert.Equal(t, firstCheckpointID, restoredFirst.Current, "current should move to first checkpoint")
	assert.Equal(t, secondCheckpointID, restoredFirst.Latest, "latest should remain second checkpoint")
	assert.False(t, restoredFirst.AtLatest, "restoring first should detach from latest")
	assert.False(t, restoredFirst.Dirty, "restore should leave workspace clean")
	assert.FileExists(t, filepath.Join(mainPath, "file1.txt"), "first content should be restored")
	assert.NoFileExists(t, filepath.Join(mainPath, "file2.txt"), "latest-only content should be absent after restoring first")

	statusAfterFirst := readRegressionStatus(t, repoPath)
	assert.Equal(t, firstCheckpointID, statusAfterFirst.Current, "status current should be first checkpoint")
	assert.Equal(t, secondCheckpointID, statusAfterFirst.Latest, "status latest should remain second checkpoint")
	assert.False(t, statusAfterFirst.AtLatest, "status should report detached after first restore")
	assert.False(t, statusAfterFirst.Dirty, "status should report clean after first restore")

	// Restore back to latest and verify the full latest checkpoint is active.
	restoredLatest := restoreRegressionCheckpoint(t, repoPath, "latest")
	assert.Equal(t, secondCheckpointID, restoredLatest.CheckpointID, "restore latest should target second checkpoint")
	assert.Equal(t, secondCheckpointID, restoredLatest.Current, "current should move to latest checkpoint")
	assert.Equal(t, secondCheckpointID, restoredLatest.Latest, "latest should be second checkpoint")
	assert.True(t, restoredLatest.AtLatest, "restore latest should report at_latest")
	assert.False(t, restoredLatest.Dirty, "restore latest should leave workspace clean")

	// Verify we're back at the latest state
	assert.FileExists(t, filepath.Join(mainPath, "file2.txt"), "latest content should be restored")

	statusAfterLatest := readRegressionStatus(t, repoPath)
	assert.Equal(t, secondCheckpointID, statusAfterLatest.Current, "status current should be latest checkpoint")
	assert.Equal(t, secondCheckpointID, statusAfterLatest.Latest, "status latest should be second checkpoint")
	assert.True(t, statusAfterLatest.AtLatest, "status should report at_latest after latest restore")
	assert.False(t, statusAfterLatest.Dirty, "status should report clean after latest restore")
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
	checkpointID := createRegressionCheckpoint(t, repoPath, "original snapshot")

	// Fork a new worktree from the snapshot
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", checkpointID, "feature-branch")

	require.Equal(t, 0, code, "worktree fork should succeed\nstdout=%s\nstderr=%s", stdout, stderr)
	var forked regressionForkData
	requireRegressionJSONData(t, stdout, &forked)
	assert.Equal(t, "feature-branch", forked.Workspace, "fork JSON should name the new workspace")
	assert.Equal(t, checkpointID, forked.BaseCheckpoint, "fork should use the full checkpoint ID as base")
	assert.Equal(t, checkpointID, forked.Current, "forked workspace should start at the checkpoint")
	assert.Equal(t, checkpointID, forked.Latest, "forked workspace should be at latest")

	// Verify the new worktree exists
	worktreePath := filepath.Join(repoPath, "worktrees", "feature-branch")
	fi, err := os.Stat(worktreePath)
	require.NoError(t, err, "new worktree directory should exist")
	require.True(t, fi.IsDir(), "new worktree should be a directory")

	// Verify the file exists in the new worktree
	content, err := os.ReadFile(filepath.Join(worktreePath, "original.txt"))
	require.NoError(t, err, "file should exist in forked worktree")
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
