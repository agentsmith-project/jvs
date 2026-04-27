package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestorePathCandidateModeWithoutSaveIDDoesNotMutate(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "notes.md"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "notes present")
	require.NoError(t, os.Remove(filepath.Join(repoRoot, "notes.md")))
	_ = savePointIDFromCLI(t, "notes removed")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--path", "notes.md")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "No save point ID was provided.")
	assert.Contains(t, stdout, "Candidates for path: notes.md")
	assert.Contains(t, stdout, firstID)
	assert.Contains(t, stdout, "Choose a save point ID")
	assert.Contains(t, stdout, "jvs restore <save> --path notes.md")
	assert.Contains(t, stdout, "No files were changed.")
	assert.NotContains(t, stdout, "Restored path")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
	require.NoFileExists(t, filepath.Join(repoRoot, "notes.md"))
	before.assertUnchanged(t, repoRoot)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "restore", "--path", "notes.md")
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "candidates", data["mode"])
	require.Equal(t, "notes.md", data["path"])
	require.Equal(t, false, data["files_changed"])
	candidates, ok := data["candidates"].([]any)
	require.True(t, ok, "candidates should be an array: %#v", data["candidates"])
	require.Len(t, candidates, 1)
	candidate, ok := candidates[0].(map[string]any)
	require.True(t, ok, "candidate should be an object: %#v", candidates[0])
	require.Equal(t, firstID, candidate["save_point_id"])
	nextCommands, ok := data["next_commands"].([]any)
	require.True(t, ok, "next_commands should be an array: %#v", data["next_commands"])
	assert.Contains(t, nextCommands, "jvs restore <save> --path notes.md")
	assert.NotContains(t, nextCommands, "jvs restore "+firstID+" --path notes.md")
	assertRestoreJSONOmitsLegacyFields(t, data)
	assertRestoreOutputOmitsLegacyVocabulary(t, string(env.Data))
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePathCandidateModeUsesGenericExecutableCommandForLeadingDashPath(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	path := "-notes.md"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, path), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "dash path")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--path="+path)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Candidates for path: "+path)
	assert.Contains(t, stdout, firstID)
	assert.Contains(t, stdout, "jvs restore <save> --path="+path)
	assert.NotContains(t, stdout, "jvs restore "+firstID)
	assert.Contains(t, stdout, "No files were changed.")
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePathJSONRestoresFileRecordsPathSourceAndStatus(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, "app.txt", data["restored_path"])
	require.Equal(t, firstID, data["from_save_point"])
	require.Equal(t, firstID, data["source_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, secondID, data["content_source"])
	require.Equal(t, false, data["history_changed"])
	require.Equal(t, true, data["files_changed"])
	require.Equal(t, true, data["path_source_recorded"])
	assertPublicPathSources(t, data["path_sources"], "app.txt", firstID)
	assertRestoreJSONOmitsLegacyFields(t, data)
	assert.NotContains(t, strings.ToLower(string(env.Data)), "source_snapshot_id")

	app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(app))
	outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside v2", string(outside))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Len(t, cfg.PathSources, 1)

	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, "Restored paths:")
	assert.Contains(t, statusOut, "app.txt from save point "+firstID)
	assert.Contains(t, statusOut, "Unsaved changes: no")
	assertStatusHumanOmitsLegacyVocabulary(t, statusOut)

	statusJSON, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	_, statusData := decodeFacadeDataMap(t, statusJSON)
	assertPublicPathSources(t, statusData["path_sources"], "app.txt", firstID)
	require.Equal(t, false, statusData["unsaved_changes"])
	assert.NotContains(t, strings.ToLower(string(decodeContractEnvelope(t, statusJSON).Data)), "source_snapshot_id")

	saveOut, err := executeCommand(createTestRootCmd(), "save", "-m", "after path restore")
	require.NoError(t, err)
	assert.Contains(t, saveOut, "Includes restored path app.txt from save point "+firstID+".")
	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, historyData := decodeFacadeDataMap(t, historyOut)
	savePoints := historyData["save_points"].([]any)
	latest := savePoints[0].(map[string]any)
	latestID := model.SnapshotID(latest["save_point_id"].(string))
	latestDesc, err := snapshot.LoadDescriptor(repoRoot, latestID)
	require.NoError(t, err)
	require.NotNil(t, latestDesc.ParentID)
	require.Equal(t, model.SnapshotID(secondID), *latestDesc.ParentID)
	require.Nil(t, latestDesc.RestoredFrom)
	require.Len(t, latestDesc.RestoredPaths, 1)
	require.Equal(t, "app.txt", latestDesc.RestoredPaths[0].TargetPath)
	require.Equal(t, model.SnapshotID(firstID), latestDesc.RestoredPaths[0].SourceSnapshotID)
}

func TestRestorePathDirectoryRestoresScopeOnly(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("src v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("src v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "scratch.tmp"), []byte("scratch"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "src")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Restored path: src")
	assert.Contains(t, stdout, "From save point: "+firstID)
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assert.Contains(t, stdout, "records this restored path")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)

	src, err := os.ReadFile(filepath.Join(repoRoot, "src", "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "src v1", string(src))
	assert.NoFileExists(t, filepath.Join(repoRoot, "src", "scratch.tmp"))
	outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside v2", string(outside))
}

func TestRestorePathDirtyScopeDefaultsRejectsAndDiscardUnsavedIsScoped(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside local edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored path: app.txt")
	outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside local edit", string(outside))

	dirtyRepo := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "outside.txt"), []byte("outside v1"), 0644))
	dirtyFirstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "outside.txt"), []byte("outside v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "app.txt"), []byte("target local edit"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirtyRepo, "outside.txt"), []byte("outside local edit"), 0644))
	before := captureViewMutationSnapshot(t, dirtyRepo)

	stdout, err = executeCommand(createTestRootCmd(), "restore", dirtyFirstID, "--path", "app.txt")
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unsaved changes in app.txt")
	assert.Contains(t, err.Error(), "--discard-unsaved")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	app, readErr := os.ReadFile(filepath.Join(dirtyRepo, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "target local edit", string(app))
	before.assertUnchanged(t, dirtyRepo)

	stdout, err = executeCommand(createTestRootCmd(), "restore", dirtyFirstID, "--path", "app.txt", "--discard-unsaved")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored path: app.txt")
	app, err = os.ReadFile(filepath.Join(dirtyRepo, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(app))
	outside, err = os.ReadFile(filepath.Join(dirtyRepo, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside local edit", string(outside))
}

func TestRestorePathSaveFirstAndLaterSaveRecordsRestoredPaths(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target local edit"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside local edit"), 0644))

	restoreOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "app.txt", "--save-first")
	require.NoError(t, err)
	_, restoreData := decodeFacadeDataMap(t, restoreOut)
	saveFirstID, ok := restoreData["newest_save_point"].(string)
	require.True(t, ok)
	require.NotEmpty(t, saveFirstID)
	require.NotEqual(t, secondID, saveFirstID)
	require.Equal(t, saveFirstID, restoreData["history_head"])
	require.Equal(t, saveFirstID, restoreData["content_source"])
	require.Equal(t, firstID, restoreData["from_save_point"])
	assertPublicPathSources(t, restoreData["path_sources"], "app.txt", firstID)

	saveFirstDesc, err := snapshot.LoadDescriptor(repoRoot, model.SnapshotID(saveFirstID))
	require.NoError(t, err)
	require.NotNil(t, saveFirstDesc.ParentID)
	require.Equal(t, model.SnapshotID(secondID), *saveFirstDesc.ParentID)

	outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside local edit", string(outside))

	saveOut, err := executeCommand(createTestRootCmd(), "save", "-m", "after path restore")
	require.NoError(t, err)
	assert.Contains(t, saveOut, "Includes restored path app.txt from save point "+firstID+".")
	assertNoOldSavePointVocabulary(t, saveOut)

	saveJSON, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, historyData := decodeFacadeDataMap(t, saveJSON)
	savePoints := historyData["save_points"].([]any)
	latest := savePoints[0].(map[string]any)
	latestID := model.SnapshotID(latest["save_point_id"].(string))
	latestDesc, err := snapshot.LoadDescriptor(repoRoot, latestID)
	require.NoError(t, err)
	require.NotNil(t, latestDesc.ParentID)
	require.Equal(t, model.SnapshotID(saveFirstID), *latestDesc.ParentID)
	require.Nil(t, latestDesc.RestoredFrom)
	require.Len(t, latestDesc.RestoredPaths, 1)
	require.Equal(t, "app.txt", latestDesc.RestoredPaths[0].TargetPath)
	require.Equal(t, model.SnapshotID(firstID), latestDesc.RestoredPaths[0].SourceSnapshotID)

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.PathSources)
}

func TestRestorePathHeldMutationLockPreventsOutsideScopedChecks(t *testing.T) {
	t.Run("dirty check", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
		_ = savePointIDFromCLI(t, "second")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target local edit"), 0644))

		lock, err := repo.AcquireMutationLock(repoRoot, "test holder")
		require.NoError(t, err)
		defer lock.Release()

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "mutation lock")
		assert.NotContains(t, err.Error(), "unsaved changes in app.txt")
	})

	t.Run("source evidence check", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "present.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")

		lock, err := repo.AcquireMutationLock(repoRoot, "test holder")
		require.NoError(t, err)
		defer lock.Release()

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "missing.txt")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "mutation lock")
		assert.NotContains(t, err.Error(), "path does not exist")
	})
}

func TestRestorePathEditedSourceReconcilesStatusAndSave(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	_, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("edited after restore"), 0644))

	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, "Restored paths:")
	assert.Contains(t, statusOut, "app.txt from save point "+firstID)
	assert.Contains(t, statusOut, "modified after restore")

	statusJSON, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	_, statusData := decodeFacadeDataMap(t, statusJSON)
	assertPublicPathSourceStatus(t, statusData["path_sources"], "app.txt", firstID, "modified_after_restore")
	require.Equal(t, true, statusData["unsaved_changes"])

	saveOut, err := executeCommand(createTestRootCmd(), "save", "-m", "after edit")
	require.NoError(t, err)
	assert.Contains(t, saveOut, "Includes restored path app.txt from save point "+firstID+".")
	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, historyData := decodeFacadeDataMap(t, historyOut)
	savePoints := historyData["save_points"].([]any)
	latest := savePoints[0].(map[string]any)
	latestID := model.SnapshotID(latest["save_point_id"].(string))
	latestDesc, err := snapshot.LoadDescriptor(repoRoot, latestID)
	require.NoError(t, err)
	require.NotNil(t, latestDesc.ParentID)
	require.Equal(t, model.SnapshotID(secondID), *latestDesc.ParentID)
	require.Len(t, latestDesc.RestoredPaths, 1)
	require.Equal(t, "app.txt", latestDesc.RestoredPaths[0].TargetPath)
	require.Equal(t, model.PathSourceModifiedAfterRestore, latestDesc.RestoredPaths[0].Status)
}

func TestRestorePathDeletedSourceClearsStatusAndSaveRestoredPath(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	_, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(repoRoot, "app.txt")))

	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)
	assert.NotContains(t, statusOut, "Restored paths:")
	statusJSON, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	_, statusData := decodeFacadeDataMap(t, statusJSON)
	assertNoPublicPathSources(t, statusData["path_sources"])
	require.Equal(t, true, statusData["unsaved_changes"])

	_, err = executeCommand(createTestRootCmd(), "save", "-m", "after delete")
	require.NoError(t, err)
	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, historyData := decodeFacadeDataMap(t, historyOut)
	savePoints := historyData["save_points"].([]any)
	latest := savePoints[0].(map[string]any)
	latestID := model.SnapshotID(latest["save_point_id"].(string))
	latestDesc, err := snapshot.LoadDescriptor(repoRoot, latestID)
	require.NoError(t, err)
	require.NotNil(t, latestDesc.ParentID)
	require.Equal(t, model.SnapshotID(secondID), *latestDesc.ParentID)
	require.Empty(t, latestDesc.RestoredPaths)
}

func TestRestorePathErrorsPreserveUserPathNamesWithLegacyWords(t *testing.T) {
	for _, path := range []string{"current.txt", "latest.txt", "dirty.txt", "head.txt"} {
		t.Run("dirty_"+path, func(t *testing.T) {
			repoRoot := setupAdoptedSaveFacadeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, path), []byte("v1"), 0644))
			firstID := savePointIDFromCLI(t, "first")
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, path), []byte("v2"), 0644))
			_ = savePointIDFromCLI(t, "second")
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, path), []byte("local edit"), 0644))

			stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), path)
			assert.NotContains(t, err.Error(), strings.ReplaceAll(path, strings.Split(path, ".")[0], "source"))
			assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, err.Error(), path)
		})

		t.Run("missing_"+path, func(t *testing.T) {
			repoRoot := setupAdoptedSaveFacadeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "present.txt"), []byte("v1"), 0644))
			firstID := savePointIDFromCLI(t, "first")

			stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), path)
			assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, err.Error(), path)
		})
	}
}

func TestRestorePathOldRefErrorsDoNotLeakLegacyWordsWhenPathIsSubstring(t *testing.T) {
	cases := []struct {
		ref  string
		path string
	}{
		{ref: "latest", path: "a"},
		{ref: "current", path: "e"},
	}

	for _, tc := range cases {
		t.Run(tc.ref+"_human", func(t *testing.T) {
			repoRoot := setupAdoptedSaveFacadeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, tc.path), []byte("v1"), 0644))
			_ = savePointIDFromCLI(t, "baseline")

			stdout, err := executeCommand(createTestRootCmd(), "restore", tc.ref, "--path", tc.path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "save point ID")
			assert.Contains(t, err.Error(), "Choose a save point ID")
			assert.NotContains(t, strings.ToLower(err.Error()), tc.ref)
			assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, err.Error(), tc.path)
		})

		t.Run(tc.ref+"_json", func(t *testing.T) {
			repoRoot := setupAdoptedSaveFacadeRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(repoRoot, tc.path), []byte("v1"), 0644))
			_ = savePointIDFromCLI(t, "baseline")

			jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", tc.ref, "--path", tc.path)
			require.NotZero(t, exitCode)
			require.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, jsonOut)
			require.False(t, env.OK, jsonOut)
			require.NotNil(t, env.Error)
			assert.Contains(t, env.Error.Message, "save point ID")
			assert.Contains(t, env.Error.Message, "Choose a save point ID")
			assert.NotContains(t, strings.ToLower(env.Error.Message), tc.ref)
			assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, env.Error.Message, tc.path)
		})
	}
}

func TestRestorePathRejectsUnsafePathsAndOldRefsWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	before := captureViewMutationSnapshot(t, repoRoot)

	for _, path := range []string{
		"",
		filepath.Join(repoRoot, "file.txt"),
		"../file.txt",
		".",
		"bad\x00path",
		`C:\Users\alice\file.txt`,
		`src\app.txt`,
		".jvs/format_version",
	} {
		t.Run(safeHistoryPathTestName(path), func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "workspace-relative path")
			assert.Contains(t, err.Error(), "No files were changed.")
			assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
			before.assertUnchanged(t, repoRoot)
		})
	}

	for _, ref := range []string{"latest", "current", "dirty", "first"} {
		t.Run("ref_"+ref, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "restore", ref, "--path", "file.txt")
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "save point ID")
			assert.Contains(t, err.Error(), "Choose a save point ID")
			assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func assertPublicPathSources(t *testing.T, raw any, targetPath, sourceSavePoint string) {
	t.Helper()
	assertPublicPathSourceStatus(t, raw, targetPath, sourceSavePoint, "exact")
}

func assertPublicPathSourceStatus(t *testing.T, raw any, targetPath, sourceSavePoint, status string) {
	t.Helper()
	sources, ok := raw.([]any)
	require.True(t, ok, "path_sources should be an array: %#v", raw)
	require.Len(t, sources, 1)
	source, ok := sources[0].(map[string]any)
	require.True(t, ok, "path source should be an object: %#v", sources[0])
	require.Equal(t, targetPath, source["target_path"])
	require.Equal(t, sourceSavePoint, source["source_save_point"])
	require.Equal(t, targetPath, source["source_path"])
	require.Equal(t, status, source["status"])
}

func assertNoPublicPathSources(t *testing.T, raw any) {
	t.Helper()
	if raw == nil {
		return
	}
	sources, ok := raw.([]any)
	require.True(t, ok, "path_sources should be an array: %#v", raw)
	require.Empty(t, sources)
}

func assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t *testing.T, value, path string) {
	t.Helper()
	withoutPath := strings.ReplaceAll(strings.ToLower(value), strings.ToLower(path), "")
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
		assert.NotContains(t, withoutPath, word)
	}
}
