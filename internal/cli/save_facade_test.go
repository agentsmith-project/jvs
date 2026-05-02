package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitSaveHistoryGoldenFacade(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("hello"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(), "save", "-m", "baseline")
	require.NoError(t, err)
	assert.Contains(t, saveOut, "Saved save point")
	assert.Contains(t, saveOut, "Workspace: main")
	assertNoOldSavePointVocabulary(t, saveOut)

	historyOut, err := executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, historyOut, "Save points")
	assert.Contains(t, historyOut, "baseline")
	assertNoOldSavePointVocabulary(t, historyOut)
}

func TestSaveCommandHumanOutputUsesSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "save", "--message", "baseline")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Saved save point")
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Copy method: ")
	assert.Contains(t, stdout, "Checked for this operation")
	assert.Contains(t, stdout, "Newest save point:")
	assert.Contains(t, stdout, "Unsaved changes: no")
	assertNoOldSavePointVocabulary(t, stdout)
}

func TestSaveCommandJSONUsesSavePointSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "baseline")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "save", env.Command)
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, "baseline", data["message"])
	require.NotEmpty(t, data["save_point_id"])
	require.Equal(t, data["save_point_id"], data["newest_save_point"])
	require.Equal(t, false, data["unsaved_changes"])
	require.NotEmpty(t, data["created_at"])
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, transfers, 1)
	primary, ok := transfers[0].(map[string]any)
	require.True(t, ok, "primary transfer should be an object: %#v", transfers[0])
	require.Equal(t, "save-primary", primary["transfer_id"])
	require.Equal(t, "save", primary["operation"])
	require.Equal(t, "materialization", primary["phase"])
	require.Equal(t, true, primary["primary"])
	require.Equal(t, "final", primary["result_kind"])
	require.Equal(t, "execution", primary["permission_scope"])
	require.Equal(t, "workspace_content", primary["source_role"])
	require.Equal(t, "save_point_staging", primary["destination_role"])
	require.NotEmpty(t, primary["source_path"])
	require.NotEmpty(t, primary["materialization_destination"])
	require.NotEmpty(t, primary["capability_probe_path"])
	require.NotEmpty(t, primary["published_destination"])
	require.NotEqual(t, primary["materialization_destination"], primary["published_destination"])
	require.Equal(t, true, primary["checked_for_this_operation"])
	require.Equal(t, "auto", primary["requested_engine"])
	require.NotEmpty(t, primary["effective_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	assert.NotContains(t, data, "restored_from")
	assertNoLegacyJSONFields(t, data)
	assertNoOldSavePointVocabulary(t, publicDataWithoutTransfers(t, data))
}

func TestHistoryCommandHumanOutputUsesSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)

	stdout, err := executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No save points yet.")
	assertNoOldSavePointVocabulary(t, stdout)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	_, err = executeCommand(createTestRootCmd(), "save", "-m", "baseline")
	require.NoError(t, err)

	stdout, err = executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Save points")
	assert.Contains(t, stdout, "baseline")
	assertNoOldSavePointVocabulary(t, stdout)
}

func TestHistoryCommandJSONUsesSavePointSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	_, err := executeCommand(createTestRootCmd(), "save", "-m", "baseline")
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "history", env.Command)
	require.Equal(t, "main", data["workspace"])
	require.NotEmpty(t, data["newest_save_point"])
	savePoints, ok := data["save_points"].([]any)
	require.True(t, ok, "save_points should be an array: %#v", data["save_points"])
	require.Len(t, savePoints, 1)
	first, ok := savePoints[0].(map[string]any)
	require.True(t, ok, "save point should be an object: %#v", savePoints[0])
	require.NotEmpty(t, first["save_point_id"])
	require.Equal(t, "baseline", first["message"])
	assertNoLegacyJSONFields(t, first)
	assertNoCheckpointSnapshotWorktreeVocabulary(t, string(env.Data))
}

func TestHistoryCommandUsesCurrentPointerAfterRestoreState(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "first")
	require.NoError(t, err)
	_, firstData := decodeFacadeDataMap(t, firstOut)
	firstID := model.SnapshotID(firstData["save_point_id"].(string))

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "second")
	require.NoError(t, err)
	_, secondData := decodeFacadeDataMap(t, secondOut)
	secondID := model.SnapshotID(secondData["save_point_id"].(string))
	require.NoError(t, worktree.NewManager(repoRoot).UpdateHead("main", firstID))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, data := decodeFacadeDataMap(t, stdout)
	require.Equal(t, string(secondID), data["newest_save_point"])
	require.Equal(t, string(firstID), data["current_pointer"])
	savePoints, ok := data["save_points"].([]any)
	require.True(t, ok, "save_points should be an array: %#v", data["save_points"])
	require.Len(t, savePoints, 1)
	firstListed := savePoints[0].(map[string]any)
	require.Equal(t, string(firstID), firstListed["save_point_id"])
	require.Equal(t, "first", firstListed["message"])
	assertNoCheckpointSnapshotWorktreeVocabulary(t, string(decodeContractEnvelope(t, stdout).Data))

	human, err := executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, human, "first")
	assert.NotContains(t, human, "second")
	assert.Contains(t, human, "Current pointer: "+string(firstID))
	assertNoCheckpointSnapshotWorktreeVocabulary(t, human)
}

func TestSaveCommandAfterRestoreCreatesNewSavePointFromNewestParent(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "first")
	require.NoError(t, err)
	_, firstData := decodeFacadeDataMap(t, firstOut)
	firstID := model.SnapshotID(firstData["save_point_id"].(string))

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "second")
	require.NoError(t, err)
	_, secondData := decodeFacadeDataMap(t, secondOut)
	secondID := model.SnapshotID(secondData["save_point_id"].(string))
	restorePreviewOut, err := executeCommand(createTestRootCmd(), "restore", string(firstID))
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, restorePreviewOut)
	restoreOut, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.NoError(t, err)
	assert.Contains(t, restoreOut, "Restored save point: "+string(firstID))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v3"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "third")
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	assertNoOldSavePointVocabulary(t, publicDataWithoutTransfers(t, data))

	thirdID := model.SnapshotID(data["save_point_id"].(string))
	thirdDesc, err := snapshot.LoadDescriptor(repoRoot, thirdID)
	require.NoError(t, err)
	require.NotNil(t, thirdDesc.ParentID)
	require.Equal(t, secondID, *thirdDesc.ParentID)
	require.NotNil(t, thirdDesc.RestoredFrom)
	require.Equal(t, firstID, *thirdDesc.RestoredFrom)
	require.Equal(t, string(firstID), data["restored_from"])
	require.NoError(t, snapshot.VerifySnapshot(repoRoot, thirdID, true))

	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	require.Equal(t, thirdID, cfg.HeadSnapshotID)
	require.Equal(t, thirdID, cfg.LatestSnapshotID)

	humanRepoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(humanRepoRoot, "app.txt"), []byte("v1"), 0644))
	humanFirstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "first")
	require.NoError(t, err)
	_, humanFirstData := decodeFacadeDataMap(t, humanFirstOut)
	humanFirstID := model.SnapshotID(humanFirstData["save_point_id"].(string))
	require.NoError(t, os.WriteFile(filepath.Join(humanRepoRoot, "app.txt"), []byte("v2"), 0644))
	_, err = executeCommand(createTestRootCmd(), "--json", "save", "-m", "second")
	require.NoError(t, err)
	restoreHumanPreviewOut, err := executeCommand(createTestRootCmd(), "restore", string(humanFirstID))
	require.NoError(t, err)
	restoreHumanPlanID := restorePlanIDFromHumanOutput(t, restoreHumanPreviewOut)
	restoreHumanOut, err := executeCommand(createTestRootCmd(), "restore", "--run", restoreHumanPlanID)
	require.NoError(t, err)
	assert.Contains(t, restoreHumanOut, "Restored save point: "+string(humanFirstID))
	require.NoError(t, os.WriteFile(filepath.Join(humanRepoRoot, "app.txt"), []byte("v3"), 0644))

	human, err := executeCommand(createTestRootCmd(), "save", "-m", "third")
	require.NoError(t, err)
	assert.Contains(t, human, "Saved save point")
	assert.Contains(t, human, "Created from restored save point "+string(humanFirstID))
	assert.NotContains(t, human, "restored_from")
	assertNoOldSavePointVocabulary(t, human)
}

func TestRootHelpShowsSaveAndHistoryNotCheckpoint(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "save")
	assert.Contains(t, stdout, "history")
	assert.Contains(t, stdout, "save point")
	assertNoCheckpointSnapshotWorktreeVocabulary(t, stdout)
}

func TestSaveAndHistoryHelpUseSavePointVocabulary(t *testing.T) {
	for _, args := range [][]string{{"save", "--help"}, {"history", "--help"}} {
		stdout, err := executeCommand(createTestRootCmd(), args...)
		require.NoError(t, err)
		assert.Contains(t, stdout, "save point")
		assertNoOldSavePointVocabulary(t, stdout)
	}
}

func TestSaveCommandDoesNotCaptureControlData(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.FileExists(t, filepath.Join(repoRoot, ".jvs", "format_version"))

	_, err := executeCommand(createTestRootCmd(), "save", "-m", "baseline")
	require.NoError(t, err)

	savePoints, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	require.Len(t, savePoints, 1)
	assert.NoDirExists(t, filepath.Join(repoRoot, ".jvs", "snapshots", string(savePoints[0].SnapshotID), ".jvs"))
}

func setupAdoptedSaveFacadeRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	require.NoError(t, os.Chdir(repoRoot))
	_, err = executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err)
	return repoRoot
}

func decodeFacadeDataMap(t *testing.T, stdout string) (contractEnvelope, map[string]any) {
	t.Helper()
	env := decodeContractEnvelope(t, stdout)
	require.NotNil(t, env.Data, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return env, data
}

func publicDataWithoutTransfers(t *testing.T, data map[string]any) string {
	t.Helper()
	clone := make(map[string]any, len(data))
	for key, value := range data {
		if key == "transfers" {
			continue
		}
		clone[key] = value
	}
	payload, err := json.Marshal(clone)
	require.NoError(t, err)
	return string(payload)
}

func assertNoLegacyJSONFields(t *testing.T, data map[string]any) {
	t.Helper()
	for _, key := range []string{
		"checkpoint_id",
		"snapshot_id",
		"parent_checkpoint_id",
		"head_snapshot",
		"latest_snapshot",
		"current",
		"latest",
	} {
		assert.NotContains(t, data, key)
	}
}

func assertNoOldSavePointVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree", "head", "current", "latest", "detached", "fork", "commit"} {
		assert.NotContains(t, lower, word)
	}
}

func assertNoCheckpointSnapshotWorktreeVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree"} {
		assert.NotContains(t, lower, word)
	}
}
