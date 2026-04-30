package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryToShowsLineageToTargetSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID := saveHistoryFixturePoint(t, repoRoot, "v1", "first")
	secondID := saveHistoryFixturePoint(t, repoRoot, "v2", "second")
	thirdID := saveHistoryFixturePoint(t, repoRoot, "v3", "third")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history", "to", secondID)
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)

	assert.Equal(t, "to", data["direction"])
	assert.Equal(t, secondID, data["target_save_point"])
	assert.Equal(t, []string{secondID, firstID}, historySavePointIDs(t, data, "save_points"))
	assert.NotContains(t, historySavePointIDs(t, data, "save_points"), thirdID)
}

func TestHistoryFromShowsDescendantTreeAndWorkspacePointers(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	baselineID := saveHistoryFixturePoint(t, repoRoot, "main v1", "baseline")

	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", baselineID)
	require.NoError(t, err)
	expFolder := filepath.Join(filepath.Dir(repoRoot), "exp")
	require.NoError(t, os.WriteFile(filepath.Join(expFolder, "app.txt"), []byte("exp v2"), 0644))
	expID := saveWorkspaceHistoryFixturePoint(t, "exp", "exp first")

	mainNextID := saveHistoryFixturePoint(t, repoRoot, "main v2", "main next")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history", "from", baselineID)
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)

	assert.Equal(t, "from", data["direction"])
	assert.Equal(t, baselineID, data["start_save_point"])
	nodeIDs := historySavePointIDs(t, data, "nodes")
	assert.ElementsMatch(t, []string{baselineID, expID, mainNextID}, nodeIDs)
	historyAssertEdge(t, data, baselineID, expID, "started_from")
	historyAssertEdge(t, data, baselineID, mainNextID, "parent")
	historyAssertWorkspacePointer(t, data, "exp", expID)
	historyAssertWorkspacePointer(t, data, "main", mainNextID)
}

func TestHistoryFromWithoutSavePointStartsAtWorkspaceSource(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	sourceID := saveHistoryFixturePoint(t, repoRoot, "source", "source")
	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.NoError(t, err)
	expFolder := filepath.Join(filepath.Dir(repoRoot), "exp")
	require.NoError(t, os.WriteFile(filepath.Join(expFolder, "app.txt"), []byte("exp v2"), 0644))
	expID := saveWorkspaceHistoryFixturePoint(t, "exp", "exp first")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "--workspace", "exp", "history", "from")
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)

	assert.Equal(t, "from", data["direction"])
	assert.Equal(t, sourceID, data["start_save_point"])
	assert.Contains(t, historySavePointIDs(t, data, "nodes"), expID)
}

func TestHistoryFromWithoutSavePointInMainStartsAtEarliestAncestor(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID := saveHistoryFixturePoint(t, repoRoot, "v1", "first")
	secondID := saveHistoryFixturePoint(t, repoRoot, "v2", "second")
	thirdID := saveHistoryFixturePoint(t, repoRoot, "v3", "third")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history", "from")
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)

	assert.Equal(t, "from", data["direction"])
	assert.Equal(t, firstID, data["start_save_point"])
	assert.ElementsMatch(t, []string{firstID, secondID, thirdID}, historySavePointIDs(t, data, "nodes"))
	historyAssertEdge(t, data, firstID, secondID, "parent")
	historyAssertEdge(t, data, secondID, thirdID, "parent")
}

func TestHistoryShowsSourceForWorkspaceWithNoOwnSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	sourceID := saveHistoryFixturePoint(t, repoRoot, "source", "source")
	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "--workspace", "exp", "history")
	require.NoError(t, err, stdout)
	assert.NotContains(t, stdout, "No save points yet.")
	assert.Contains(t, stdout, "Current pointer: "+sourceID)
	assert.Contains(t, stdout, "Workspace has not created its own save point yet.")
	assert.Contains(t, stdout, sourceID[:8])

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "--workspace", "exp", "history")
	require.NoError(t, err, jsonOut)
	_, data := decodeFacadeDataMap(t, jsonOut)
	assert.Equal(t, "current", data["direction"])
	assert.Equal(t, sourceID, data["current_pointer"])
	assert.Equal(t, sourceID, data["started_from_save_point"])
	assert.Nil(t, data["newest_save_point"])
	assert.Equal(t, []string{sourceID}, historySavePointIDs(t, data, "save_points"))
}

func TestHistoryRejectsAllFlag(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	_ = saveHistoryFixturePoint(t, repoRoot, "baseline", "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "history", "--all")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unknown flag: --all")
}

func TestHistoryDefaultLimitAndLimitZero(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	var lastID string
	for i := 1; i <= 35; i++ {
		lastID = saveHistoryFixturePoint(t, repoRoot, fmt.Sprintf("v%02d", i), fmt.Sprintf("save %02d", i))
	}

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)
	assert.Equal(t, float64(30), data["limit"])
	assert.Equal(t, true, data["truncated"])
	defaultIDs := historySavePointIDs(t, data, "save_points")
	require.Len(t, defaultIDs, 30)
	assert.Equal(t, lastID, defaultIDs[0])

	stdout, err = executeCommand(createTestRootCmd(), "--json", "history", "--limit", "0")
	require.NoError(t, err, stdout)
	_, data = decodeFacadeDataMap(t, stdout)
	assert.Equal(t, float64(0), data["limit"])
	assert.Equal(t, false, data["truncated"])
	require.Len(t, historySavePointIDs(t, data, "save_points"), 35)
}

func saveHistoryFixturePoint(t *testing.T, repoRoot, content, message string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte(content), 0644))
	return savePointIDFromCLI(t, message)
}

func saveWorkspaceHistoryFixturePoint(t *testing.T, workspace, message string) string {
	t.Helper()
	stdout, err := executeCommand(createTestRootCmd(), "--json", "--workspace", workspace, "save", "-m", message)
	require.NoError(t, err, stdout)
	_, data := decodeFacadeDataMap(t, stdout)
	id, ok := data["save_point_id"].(string)
	require.True(t, ok, "save_point_id should be a string: %#v", data["save_point_id"])
	require.NotEmpty(t, id)
	return id
}

func historySavePointIDs(t *testing.T, data map[string]any, field string) []string {
	t.Helper()
	raw, ok := data[field].([]any)
	require.True(t, ok, "%s should be an array: %#v", field, data[field])
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		require.True(t, ok, "%s entry should be an object: %#v", field, item)
		id, ok := record["save_point_id"].(string)
		require.True(t, ok, "save_point_id should be a string: %#v", record)
		ids = append(ids, id)
	}
	return ids
}

func historyAssertEdge(t *testing.T, data map[string]any, from, to, edgeType string) {
	t.Helper()
	raw, ok := data["edges"].([]any)
	require.True(t, ok, "edges should be an array: %#v", data["edges"])
	for _, item := range raw {
		edge, ok := item.(map[string]any)
		require.True(t, ok, "edge should be an object: %#v", item)
		if edge["from"] == from && edge["to"] == to && edge["type"] == edgeType {
			return
		}
	}
	t.Fatalf("missing history edge %s -> %s (%s): %#v", from, to, edgeType, raw)
}

func historyAssertWorkspacePointer(t *testing.T, data map[string]any, workspace, savePointID string) {
	t.Helper()
	raw, ok := data["workspace_pointers"].([]any)
	require.True(t, ok, "workspace_pointers should be an array: %#v", data["workspace_pointers"])
	for _, item := range raw {
		pointer, ok := item.(map[string]any)
		require.True(t, ok, "workspace pointer should be an object: %#v", item)
		if pointer["workspace"] == workspace && pointer["save_point_id"] == savePointID {
			return
		}
	}
	t.Fatalf("missing workspace pointer %s -> %s: %#v", workspace, savePointID, raw)
}
