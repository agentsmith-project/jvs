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

// initTestRepo creates a temp repo and returns its folder path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")

	stdout, stderr, code := runJVS(t, dir, "init", "testrepo")
	require.Equal(t, 0, code, "jvs init failed\nstdout=%s\nstderr=%s", stdout, stderr)
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

// runJVSInRepo runs jvs from within the repo's main workspace folder.
func runJVSInRepo(t *testing.T, repoPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runJVS(t, repoPath, args...)
}

type regressionJSONEnvelope struct {
	Command string          `json:"command"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

type regressionSavePointData struct {
	SavePointID          string `json:"save_point_id"`
	Workspace            string `json:"workspace"`
	Message              string `json:"message"`
	NewestSavePoint      string `json:"newest_save_point"`
	StartedFromSavePoint string `json:"started_from_save_point,omitempty"`
	UnsavedChanges       bool   `json:"unsaved_changes"`
}

type regressionRestoreData struct {
	Mode                    string `json:"mode"`
	PlanID                  string `json:"plan_id,omitempty"`
	Folder                  string `json:"folder"`
	Workspace               string `json:"workspace"`
	SourceSavePoint         string `json:"source_save_point"`
	RestoredSavePoint       string `json:"restored_save_point,omitempty"`
	NewestSavePoint         string `json:"newest_save_point"`
	HistoryHead             string `json:"history_head"`
	ExpectedNewestSavePoint string `json:"expected_newest_save_point,omitempty"`
	ContentSource           string `json:"content_source,omitempty"`
	UnsavedChanges          bool   `json:"unsaved_changes"`
	FilesState              string `json:"files_state,omitempty"`
	HistoryChanged          bool   `json:"history_changed"`
	FilesChanged            bool   `json:"files_changed"`
}

type regressionStatusData struct {
	Folder               string  `json:"folder"`
	Workspace            string  `json:"workspace"`
	NewestSavePoint      *string `json:"newest_save_point"`
	HistoryHead          *string `json:"history_head"`
	ContentSource        *string `json:"content_source"`
	StartedFromSavePoint *string `json:"started_from_save_point,omitempty"`
	UnsavedChanges       bool    `json:"unsaved_changes"`
	FilesState           string  `json:"files_state"`
}

type regressionHistoryData struct {
	Workspace            string                       `json:"workspace"`
	SavePoints           []regressionHistorySavePoint `json:"save_points"`
	NewestSavePoint      string                       `json:"newest_save_point,omitempty"`
	StartedFromSavePoint string                       `json:"started_from_save_point,omitempty"`
}

type regressionHistorySavePoint struct {
	SavePointID string `json:"save_point_id"`
	Workspace   string `json:"workspace"`
	Message     string `json:"message,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type regressionWorkspaceNewData struct {
	Mode                 string  `json:"mode"`
	Status               string  `json:"status"`
	Workspace            string  `json:"workspace"`
	Folder               string  `json:"folder"`
	StartedFromSavePoint string  `json:"started_from_save_point"`
	ContentSource        string  `json:"content_source"`
	NewestSavePoint      *string `json:"newest_save_point"`
	HistoryHead          *string `json:"history_head"`
	OriginalUnchanged    bool    `json:"original_workspace_unchanged"`
	UnsavedChanges       bool    `json:"unsaved_changes"`
}

type regressionCleanupData struct {
	PlanID                   string   `json:"plan_id"`
	ProtectedSavePoints      []string `json:"protected_save_points"`
	ProtectedByHistory       int      `json:"protected_by_history"`
	CandidateCount           int      `json:"candidate_count"`
	ReclaimableSavePoints    []string `json:"reclaimable_save_points"`
	ReclaimableBytesEstimate int64    `json:"reclaimable_bytes_estimate"`
}

type regressionCleanupRunData struct {
	PlanID string `json:"plan_id"`
	Status string `json:"status"`
}

type regressionDoctorData struct {
	Healthy bool `json:"healthy"`
	Repairs []struct {
		Action  string `json:"action"`
		Success bool   `json:"success"`
		Message string `json:"message"`
		Cleaned int    `json:"cleaned,omitempty"`
	} `json:"repairs,omitempty"`
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

func requireRegressionJSONError(t *testing.T, stdout string) regressionJSONEnvelope {
	t.Helper()

	var envelope regressionJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout should be a JSON envelope:\n%s", stdout)
	require.False(t, envelope.OK, "JSON envelope should report failure:\n%s", stdout)
	require.NotNil(t, envelope.Error, "JSON error envelope should include error:\n%s", stdout)
	return envelope
}

func createRegressionSavePoint(t *testing.T, repoPath, message string) string {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "save", "-m", message)
	var data regressionSavePointData
	requireRegressionJSONData(t, stdout, &data)
	require.NotEmpty(t, data.SavePointID, "save JSON should include full save_point_id:\n%s", stdout)
	require.Equal(t, message, data.Message, "save JSON should preserve message:\n%s", stdout)
	require.Equal(t, data.SavePointID, data.NewestSavePoint, "save should make the new save point newest:\n%s", stdout)
	return data.SavePointID
}

func previewRegressionRestore(t *testing.T, repoPath, ref string) regressionRestoreData {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "restore", ref)
	var data regressionRestoreData
	requireRegressionJSONData(t, stdout, &data)
	require.Equal(t, "preview", data.Mode, "restore should preview before changing files:\n%s", stdout)
	require.NotEmpty(t, data.PlanID, "restore preview should include plan_id:\n%s", stdout)
	require.Equal(t, ref, data.SourceSavePoint, "restore preview should target the requested save point:\n%s", stdout)
	return data
}

func runRegressionRestorePlan(t *testing.T, repoPath, planID string) regressionRestoreData {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "restore", "--run", planID)
	var data regressionRestoreData
	requireRegressionJSONData(t, stdout, &data)
	require.Equal(t, "run", data.Mode, "restore run should execute a preview plan:\n%s", stdout)
	require.Equal(t, planID, data.PlanID, "restore run should report the executed plan:\n%s", stdout)
	require.NotEmpty(t, data.RestoredSavePoint, "restore run should include restored_save_point:\n%s", stdout)
	return data
}

func restoreRegressionSavePoint(t *testing.T, repoPath, ref string) regressionRestoreData {
	t.Helper()

	preview := previewRegressionRestore(t, repoPath, ref)
	return runRegressionRestorePlan(t, repoPath, preview.PlanID)
}

func readRegressionStatus(t *testing.T, repoPath string) regressionStatusData {
	t.Helper()

	stdout, _ := runJVSJSONInRepo(t, repoPath, "status")
	var data regressionStatusData
	requireRegressionJSONData(t, stdout, &data)
	return data
}

func runJVSJSONForWorkspace(t *testing.T, repoPath, workspace string, args ...string) (stdout, stderr string) {
	t.Helper()
	allArgs := append([]string{"--json", "--workspace", workspace}, args...)
	stdout, stderr, code := runJVSInRepo(t, repoPath, allArgs...)
	require.Equal(t, 0, code, "jvs --json --workspace %s %s failed\nstdout=%s\nstderr=%s", workspace, strings.Join(args, " "), stdout, stderr)
	return stdout, stderr
}

func stringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// createFiles creates multiple files in a workspace folder.
func createFiles(t *testing.T, workspacePath string, files map[string]string) {
	t.Helper()
	for filename, content := range files {
		path := filepath.Join(workspacePath, filename)
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
// func TestRegression_CleanupLeak(t *testing.T) {
//     // Bug: cleanup was not cleaning up orphaned save point storage
//     // Fixed: 2024-02-15, PR #456
//     // ...
// }
// ============================================================================

// TestRegression_RestoreNonExistentSavePoint tests that restore fails
// gracefully when given a non-existent save point ID.
//
// Bug: Restore would panic with nil pointer dereference on invalid save point ID
// Fixed: 2024-02-20
func TestRegression_RestoreNonExistentSavePoint(t *testing.T) {
	repoPath := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", "1777360000000-deadbeef")

	assert.NotEqual(t, 0, code, "restore should fail for a non-existent save point")
	assert.Empty(t, strings.TrimSpace(stderr), "JSON errors should be emitted on stdout")
	env := requireRegressionJSONError(t, stdout)
	assert.Equal(t, "restore", env.Command)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "save point")
	assert.Contains(t, env.Error.Message, "not available")
	assert.NotContains(t, stdout+stderr, "panic", "restore should not panic")
}

// TestRegression_SaveRequiresMessage verifies the current public save command
// rejects an empty message cleanly.
//
// Bug: Empty save text could produce unclear validation behavior
// Fixed: 2024-02-20
func TestRegression_SaveRequiresMessage(t *testing.T) {
	repoPath := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "save", "-m", "")

	assert.NotEqual(t, 0, code, "save with an empty message should fail")
	assert.Empty(t, strings.TrimSpace(stderr), "JSON errors should be emitted on stdout")
	env := requireRegressionJSONError(t, stdout)
	assert.Equal(t, "save", env.Command)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "save point message is required")
	assert.NotContains(t, stdout+stderr, "panic", "save should not panic")
}

// TestRegression_HistoryGrepFiltersMessages tests that history filters save
// points by public message text.
//
// Bug: History filtering could omit or include the wrong saved entries
// Fixed: 2024-02-20
func TestRegression_HistoryGrepFiltersMessages(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"app.txt": "alpha"})
	alphaID := createRegressionSavePoint(t, repoPath, "alpha milestone")
	createFiles(t, repoPath, map[string]string{"app.txt": "release"})
	releaseID := createRegressionSavePoint(t, repoPath, "release candidate")

	stdout, _ := runJVSJSONInRepo(t, repoPath, "history", "--grep", "release")
	var history regressionHistoryData
	requireRegressionJSONData(t, stdout, &history)

	require.Len(t, history.SavePoints, 1, "history --grep should return only matching save points:\n%s", stdout)
	assert.Equal(t, releaseID, history.SavePoints[0].SavePointID)
	assert.Equal(t, "release candidate", history.SavePoints[0].Message)
	assert.Equal(t, releaseID, history.NewestSavePoint, "newest save point should remain the workspace head")
	assert.NotContains(t, stdout, alphaID, "non-matching save point should be filtered out")
}

// TestRegression_RestorePreviewRun tests the preview-first restore flow and
// verifies that restore changes files without rewriting workspace history.
//
// Bug: Restore could move to the wrong saved content or report stale status
// Fixed: 2024-02-20
func TestRegression_RestorePreviewRun(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"file1.txt": "content1"})
	firstSavePointID := createRegressionSavePoint(t, repoPath, "first save point")

	createFiles(t, repoPath, map[string]string{"file2.txt": "content2"})
	secondSavePointID := createRegressionSavePoint(t, repoPath, "second save point")

	previewFirst := previewRegressionRestore(t, repoPath, firstSavePointID)
	assert.Equal(t, firstSavePointID, previewFirst.SourceSavePoint)
	assert.Equal(t, secondSavePointID, previewFirst.NewestSavePoint)
	assert.Equal(t, secondSavePointID, previewFirst.HistoryHead)
	assert.Equal(t, secondSavePointID, previewFirst.ExpectedNewestSavePoint)
	assert.False(t, previewFirst.HistoryChanged, "preview should not change history")
	assert.False(t, previewFirst.FilesChanged, "preview should not change files")

	restoredFirst := runRegressionRestorePlan(t, repoPath, previewFirst.PlanID)
	assert.Equal(t, firstSavePointID, restoredFirst.RestoredSavePoint)
	assert.Equal(t, firstSavePointID, restoredFirst.ContentSource)
	assert.Equal(t, secondSavePointID, restoredFirst.NewestSavePoint)
	assert.Equal(t, secondSavePointID, restoredFirst.HistoryHead)
	assert.False(t, restoredFirst.HistoryChanged, "restore should leave history unchanged")
	assert.True(t, restoredFirst.FilesChanged, "restore should change files")
	assert.False(t, restoredFirst.UnsavedChanges, "restore should leave workspace clean")
	assert.Equal(t, "matches_save_point", restoredFirst.FilesState)
	assert.FileExists(t, filepath.Join(repoPath, "file1.txt"), "first content should be restored")
	assert.NoFileExists(t, filepath.Join(repoPath, "file2.txt"), "newer-only content should be absent after restoring first")

	statusAfterFirst := readRegressionStatus(t, repoPath)
	assert.Equal(t, firstSavePointID, stringValue(statusAfterFirst.ContentSource), "status content source should be first save point")
	assert.Equal(t, secondSavePointID, stringValue(statusAfterFirst.NewestSavePoint), "history head should remain second save point")
	assert.False(t, statusAfterFirst.UnsavedChanges, "status should report clean after restore")
	assert.Equal(t, "matches_save_point", statusAfterFirst.FilesState)

	restoredSecond := restoreRegressionSavePoint(t, repoPath, secondSavePointID)
	assert.Equal(t, secondSavePointID, restoredSecond.RestoredSavePoint)
	assert.Equal(t, secondSavePointID, restoredSecond.ContentSource)
	assert.Equal(t, secondSavePointID, restoredSecond.NewestSavePoint)
	assert.Equal(t, secondSavePointID, restoredSecond.HistoryHead)
	assert.False(t, restoredSecond.UnsavedChanges, "restoring newest save point should leave workspace clean")
	assert.FileExists(t, filepath.Join(repoPath, "file2.txt"), "newest content should be restored")
}

// TestRegression_WorkspaceNewFromSavePoint tests creating a workspace from a
// save point.
//
// Bug: Workspace creation from saved content did not initialize state correctly
// Fixed: 2024-02-20
func TestRegression_WorkspaceNewFromSavePoint(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"original.txt": "content"})
	savePointID := createRegressionSavePoint(t, repoPath, "original save point")

	featureFolder := filepath.Join(filepath.Dir(repoPath), "feature-branch")
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "new", featureFolder, "--from", savePointID)

	require.Equal(t, 0, code, "workspace new should succeed\nstdout=%s\nstderr=%s", stdout, stderr)
	var created regressionWorkspaceNewData
	requireRegressionJSONData(t, stdout, &created)
	assert.Equal(t, "new", created.Mode)
	assert.Equal(t, "created", created.Status)
	assert.Equal(t, "feature-branch", created.Workspace)
	assert.Equal(t, featureFolder, created.Folder)
	assert.Equal(t, savePointID, created.StartedFromSavePoint)
	assert.Equal(t, savePointID, created.ContentSource)
	assert.Nil(t, created.NewestSavePoint, "new workspace should not have its own newest save point yet")
	assert.Nil(t, created.HistoryHead, "new workspace history should start empty")
	assert.True(t, created.OriginalUnchanged, "source workspace should remain unchanged")
	assert.False(t, created.UnsavedChanges, "new workspace should start clean")

	fi, err := os.Stat(created.Folder)
	require.NoError(t, err, "new workspace folder should exist")
	require.True(t, fi.IsDir(), "new workspace should be a directory")

	content, err := os.ReadFile(filepath.Join(created.Folder, "original.txt"))
	require.NoError(t, err, "file should exist in created workspace")
	assert.Equal(t, "content", string(content), "file content should match save point")
}

// TestRegression_CleanupPreviewWithEmptySavePoint tests cleanup with a save
// point that has no managed files.
//
// Bug: Cleanup could panic when processing empty saved content
// Fixed: 2024-02-20
func TestRegression_CleanupPreviewWithEmptySavePoint(t *testing.T) {
	repoPath := initTestRepo(t)

	savePointID := createRegressionSavePoint(t, repoPath, "empty save point")

	stdout, _ := runJVSJSONInRepo(t, repoPath, "cleanup", "preview")
	var preview regressionCleanupData
	requireRegressionJSONData(t, stdout, &preview)

	assert.NotEmpty(t, preview.PlanID, "cleanup preview should create a runnable plan")
	assert.Contains(t, preview.ProtectedSavePoints, savePointID, "active save point should be protected")
	assert.Empty(t, preview.ReclaimableSavePoints, "active save point should not be reclaimable")
	assert.NotContains(t, stdout, "panic", "cleanup preview should not panic")

	runOut, _ := runJVSJSONInRepo(t, repoPath, "cleanup", "run", "--plan-id", preview.PlanID)
	var run regressionCleanupRunData
	requireRegressionJSONData(t, runOut, &run)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, "completed", run.Status)
}

// TestRegression_DoctorRuntimeRepair tests that doctor --repair-runtime fixes
// runtime issues.
//
// Bug: Doctor --repair-runtime was not executing all repairs
// Fixed: 2024-02-20, PR #7d0db0c
func TestRegression_DoctorRuntimeRepair(t *testing.T) {
	repoPath := initTestRepo(t)

	stdout, _ := runJVSJSONInRepo(t, repoPath, "doctor", "--repair-runtime")
	var doctor regressionDoctorData
	requireRegressionJSONData(t, stdout, &doctor)

	assert.True(t, doctor.Healthy, "doctor should report a healthy repository")
	require.NotEmpty(t, doctor.Repairs, "doctor --repair-runtime should report repair actions")
	for _, repair := range doctor.Repairs {
		assert.True(t, repair.Success, "repair action %s should succeed", repair.Action)
	}
}

// TestRegression_StatusCommand tests that status displays current folder and
// save point state.
//
// Bug: Repository status output was missing fields or had formatting issues
// Fixed: 2024-02-20, PR #7d0db0c
func TestRegression_StatusCommand(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"status.txt": "content"})
	savePointID := createRegressionSavePoint(t, repoPath, "status baseline")

	status := readRegressionStatus(t, repoPath)

	assert.Equal(t, repoPath, status.Folder)
	assert.Equal(t, "main", status.Workspace)
	assert.Equal(t, savePointID, stringValue(status.NewestSavePoint))
	assert.Equal(t, savePointID, stringValue(status.HistoryHead))
	assert.Equal(t, savePointID, stringValue(status.ContentSource))
	assert.False(t, status.UnsavedChanges)
	assert.Equal(t, "matches_save_point", status.FilesState)
}

// TestRegression_CanSaveNewWorkspace verifies that the first save in a freshly
// created workspace succeeds.
//
// Bug: first-save validation returned false for new workspaces with no save
// points, blocking the first save.
// Fixed: 2026-02-28
func TestRegression_CanSaveNewWorkspace(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"baseline.txt": "base"})
	baseID := createRegressionSavePoint(t, repoPath, "baseline")

	freshFolder := filepath.Join(filepath.Dir(repoPath), "fresh")
	stdout, _ := runJVSJSONInRepo(t, repoPath, "workspace", "new", freshFolder, "--from", baseID)
	var fresh regressionWorkspaceNewData
	requireRegressionJSONData(t, stdout, &fresh)
	assert.Equal(t, "fresh", fresh.Workspace)
	assert.Equal(t, freshFolder, fresh.Folder)

	createFiles(t, fresh.Folder, map[string]string{"hello.txt": "world"})

	saveOut, _ := runJVSJSONForWorkspace(t, repoPath, "fresh", "save", "-m", "first save in fresh workspace")
	var saved regressionSavePointData
	requireRegressionJSONData(t, saveOut, &saved)
	assert.Equal(t, "fresh", saved.Workspace)
	assert.NotEmpty(t, saved.SavePointID, "save should produce a save point ID")
	assert.Equal(t, saved.SavePointID, saved.NewestSavePoint)
	assert.Equal(t, baseID, saved.StartedFromSavePoint)

	historyOut, _ := runJVSJSONForWorkspace(t, repoPath, "fresh", "history")
	var history regressionHistoryData
	requireRegressionJSONData(t, historyOut, &history)
	require.Len(t, history.SavePoints, 1, "fresh workspace history should include its first save:\n%s", historyOut)
	assert.Equal(t, saved.SavePointID, history.SavePoints[0].SavePointID)
	assert.Equal(t, "first save in fresh workspace", history.SavePoints[0].Message)
	assert.Equal(t, baseID, history.StartedFromSavePoint)
}

// TestRegression_CleanupRespectsProtectedSavePoint verifies that cleanup
// preview does not mark the active save point as reclaimable.
//
// Bug: Cleanup preview ignored configured retention/protection rules.
// Fixed: 2026-02-28
func TestRegression_CleanupRespectsProtectedSavePoint(t *testing.T) {
	repoPath := initTestRepo(t)

	createFiles(t, repoPath, map[string]string{"protected.txt": "content"})
	savePointID := createRegressionSavePoint(t, repoPath, "protected save point")

	stdout, _ := runJVSJSONInRepo(t, repoPath, "cleanup", "preview")
	var preview regressionCleanupData
	requireRegressionJSONData(t, stdout, &preview)

	assert.Contains(t, preview.ProtectedSavePoints, savePointID)
	assert.Equal(t, 0, preview.CandidateCount)
	assert.Empty(t, preview.ReclaimableSavePoints, "protected save point should not be reclaimable")
	assert.Equal(t, int64(0), preview.ReclaimableBytesEstimate)
}

// TestRegression_ConfigCacheMutation is tested at the unit level in
// pkg/config/config_test.go TestLoad_CacheCopyIndependence

// TestRegression_RestoreEmptyArgs verifies that restore fails gracefully when
// given an empty save point ID instead of panicking.
//
// Bug: restore did not validate empty workspace name or save point ID.
// Fixed: 2026-02-28
func TestRegression_RestoreEmptyArgs(t *testing.T) {
	repoPath := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", "")
	assert.NotEqual(t, 0, code, "restore with empty save point ID should fail")
	assert.Empty(t, strings.TrimSpace(stderr), "JSON errors should be emitted on stdout")
	env := requireRegressionJSONError(t, stdout)

	assert.Equal(t, "restore", env.Command)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "save point ID is required")
	assert.NotContains(t, stdout+stderr, "panic", "restore should not panic on empty args")
}

// TestRegression_CleanupRunEmptyPlanID verifies that cleanup run fails
// gracefully when given an empty plan ID.
//
// Bug: cleanup run did not validate empty plan ID.
// Fixed: 2026-02-28
func TestRegression_CleanupRunEmptyPlanID(t *testing.T) {
	repoPath := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "cleanup", "run", "--plan-id", "")
	assert.NotEqual(t, 0, code, "cleanup run with empty plan-id should fail")
	assert.Empty(t, strings.TrimSpace(stderr), "JSON errors should be emitted on stdout")
	env := requireRegressionJSONError(t, stdout)

	assert.Equal(t, "cleanup run", env.Command)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "--plan-id is required")
	assert.NotContains(t, stdout+stderr, "panic", "cleanup run should not panic on empty plan-id")
}
