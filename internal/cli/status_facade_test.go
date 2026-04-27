package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusFacadeAfterInitUsesSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("draft"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Newest save point: none")
	assert.Contains(t, stdout, "Not saved yet.")
	assert.Contains(t, stdout, "Unsaved changes: yes")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)
}

func TestStatusFacadeAfterSaveShowsFilesMatchNewest(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	saveID := savePointIDFromCLI(t, "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Newest save point: "+saveID)
	assert.Contains(t, stdout, "Files match save point: "+saveID)
	assert.Contains(t, stdout, "Unsaved changes: no")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)
}

func TestStatusFacadeAfterEditShowsChangedSinceSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	saveID := savePointIDFromCLI(t, "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Newest save point: "+saveID)
	assert.Contains(t, stdout, "Files changed since save point: "+saveID)
	assert.Contains(t, stdout, "Unsaved changes: yes")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)
}

func TestStatusFacadeAfterWholeRestoreKeepsNewestAndShowsSource(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	_, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-dirty")
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Newest save point: "+secondID)
	assert.Contains(t, stdout, "Files match save point: "+firstID)
	assert.Contains(t, stdout, "Unsaved changes: no")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)
}

func TestStatusFacadeAfterRestoreThenEditKeepsRestoredSourceAndDirty(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	_, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-dirty")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("edited from restored"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Newest save point: "+secondID)
	assert.Contains(t, stdout, "Files were last restored from: "+firstID)
	assert.NotContains(t, stdout, "Files changed since save point: "+firstID)
	assert.Contains(t, stdout, "Unsaved changes: yes")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	_, data := decodeFacadeDataMap(t, jsonOut)
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, true, data["unsaved_changes"])
	require.Equal(t, "restored_then_changed", data["files_state"])
	assertStatusJSONOmitsLegacyFields(t, data)
}

func TestStatusFacadeJSONDoesNotExposeLegacyFields(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)
	_, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-dirty")
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "status", env.Command)
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, "matches_save_point", data["files_state"])
	assertStatusJSONOmitsLegacyFields(t, data)
}

func TestStatusHelpUsesSavePointVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "status", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "folder")
	assert.Contains(t, stdout, "workspace")
	assert.Contains(t, stdout, "save point")
	assertStatusHumanOmitsLegacyVocabulary(t, stdout)
}

func savePointIDFromCLI(t *testing.T, message string) string {
	t.Helper()
	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", message)
	require.NoError(t, err)
	_, data := decodeFacadeDataMap(t, stdout)
	id, ok := data["save_point_id"].(string)
	require.True(t, ok, "save_point_id should be a string: %#v", data["save_point_id"])
	require.NotEmpty(t, id)
	return id
}

func createTwoSavePoints(t *testing.T, repoRoot string) (string, string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	require.NotEqual(t, model.SnapshotID(firstID), model.SnapshotID(secondID))
	return firstID, secondID
}

func assertStatusHumanOmitsLegacyVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree", "current", "latest", "detached", "fork", "commit"} {
		assert.NotContains(t, lower, word)
	}
	assert.NotContains(t, value, "Dirty:")
	assert.NotContains(t, value, "At latest:")
}

func assertStatusJSONOmitsLegacyFields(t *testing.T, data map[string]any) {
	t.Helper()
	for _, key := range []string{
		"current",
		"latest",
		"dirty",
		"at_latest",
		"checkpoint_id",
		"snapshot_id",
		"head_snapshot",
		"latest_snapshot",
		"worktree",
	} {
		assert.NotContains(t, data, key)
	}
}
