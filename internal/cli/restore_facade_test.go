package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreHelpUsesSavePointVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "restore", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "save point")
	assert.Contains(t, stdout, "folder")
	assert.Contains(t, stdout, "workspace")
	assert.Contains(t, stdout, "--discard-unsaved")
	assert.Contains(t, stdout, "--save-first")
	assert.NotContains(t, stdout, "--discard-dirty")
	assert.NotContains(t, stdout, "--include-working")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
}

func TestRestoreHumanOutputUsesSavePointStatusFacts(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Managed files now match save point "+firstID+".")
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assert.Contains(t, stdout, "Next save creates a new save point after "+secondID+".")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
}

func TestRestoreJSONUsesSavePointSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, "matches_save_point", data["files_state"])
	require.Equal(t, false, data["history_changed"])
	assertRestoreJSONOmitsLegacyFields(t, data)
}

func TestRestoreDirtyRefusalUsesUnsavedChangesVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unsaved changes")
	assert.Contains(t, err.Error(), "--save-first")
	assert.Contains(t, err.Error(), "--discard-unsaved")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())

	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "local edit", string(content))

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", firstID)
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "unsaved changes")
	assert.Contains(t, env.Error.Message, "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, env.Error.Message)
}

func TestRestoreCurrentSourceDescriptorErrorUsesSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)
	require.NoError(t, os.Remove(filepath.Join(repoRoot, ".jvs", "descriptors", secondID+".json")))

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "source save point")
	assert.NotContains(t, err.Error(), "load current checkpoint")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())

	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "v2", string(content))

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", firstID)
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "source save point")
	assert.NotContains(t, env.Error.Message, "load current checkpoint")
	assertRestoreOutputOmitsLegacyVocabulary(t, env.Error.Message)

	content, readErr = os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "v2", string(content))
}

func TestRestoreDiscardUnsavedRestoresWithoutLegacyFields(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--discard-unsaved")
	require.NoError(t, err)

	_, data := decodeFacadeDataMap(t, stdout)
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, "matches_save_point", data["files_state"])
	require.Equal(t, false, data["history_changed"])
	assertRestoreJSONOmitsLegacyFields(t, data)

	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "v1", string(content))
}

func TestRestoreSaveFirstUsesSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--save-first")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	newest, ok := data["newest_save_point"].(string)
	require.True(t, ok)
	require.NotEmpty(t, newest)
	require.NotEqual(t, firstID, newest)
	require.Equal(t, newest, data["history_head"])
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["history_changed"])
	assertRestoreJSONOmitsLegacyFields(t, data)

	historyOut, err := executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, historyOut, "save before restore")
	assertRestoreOutputOmitsLegacyVocabulary(t, historyOut)
}

func TestRestoreOutputShowsHistoryNotChangedAfterRestoringOlderSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
}

func assertRestoreOutputOmitsLegacyVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{
		"checkpoint",
		"snapshot",
		"worktree",
		"current",
		"latest",
		"dirty",
		"at_latest",
		"head",
		"detached",
		"fork",
		"commit",
	} {
		assert.NotContains(t, lower, word)
	}
}

func assertRestoreJSONOmitsLegacyFields(t *testing.T, data map[string]any) {
	t.Helper()
	for _, key := range []string{
		"checkpoint_id",
		"current",
		"latest",
		"dirty",
		"at_latest",
		"snapshot_id",
		"head_snapshot",
		"latest_snapshot",
		"worktree",
	} {
		assert.NotContains(t, data, key)
	}
}
