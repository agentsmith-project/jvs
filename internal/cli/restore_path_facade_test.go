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
	plansBefore := restorePlanFileCount(t, repoRoot)

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
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
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
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
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
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)

	env, previewData := decodeFacadeDataMap(t, previewOut)
	require.True(t, env.OK, previewOut)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "preview", previewData["mode"])
	require.Equal(t, "path", previewData["scope"])
	require.Equal(t, repoRoot, previewData["folder"])
	require.Equal(t, "main", previewData["workspace"])
	require.Equal(t, "app.txt", previewData["path"])
	require.Equal(t, firstID, previewData["source_save_point"])
	require.Equal(t, secondID, previewData["newest_save_point"])
	require.Equal(t, secondID, previewData["history_head"])
	require.Equal(t, secondID, previewData["expected_newest_save_point"])
	require.NotEmpty(t, previewData["expected_path_evidence"])
	require.Equal(t, false, previewData["history_changed"])
	require.Equal(t, false, previewData["files_changed"])
	planID := previewData["plan_id"].(string)
	require.Equal(t, "jvs restore --run "+planID, previewData["run_command"])
	assertRestorePreviewImpact(t, previewData, "overwrite", 1, "app.txt")
	assertRestoreJSONOmitsLegacyFields(t, previewData)
	assert.NotContains(t, previewData, "restored_path")
	assert.NotContains(t, previewData, "content_source")
	assert.NotContains(t, strings.ToLower(string(env.Data)), "source_snapshot_id")
	assert.Equal(t, plansBefore+1, restorePlanFileCount(t, repoRoot))

	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside v2")
	before.assertUnchanged(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, planID, data["plan_id"])
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

func TestRestorePathPreviewJSONIncludesPathTransferDestination(t *testing.T) {
	repoRoot, firstID := setupRestorePathTransferFacadeRepo(t)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "src/app.txt")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "preview", data["mode"])
	require.Equal(t, "path", data["scope"])
	require.Equal(t, "src/app.txt", data["path"])
	assertRestoreExpectedPreviewTransferDestination(t, data, repoRoot, firstID, filepath.Join(repoRoot, "src", "app.txt"))
}

func TestRestorePathRunJSONIncludesPathTransferDestination(t *testing.T) {
	repoRoot, firstID := setupRestorePathTransferFacadeRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "src/app.txt")
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, planID, data["plan_id"])
	require.Equal(t, "src/app.txt", data["restored_path"])
	assertRestorePathFinalRunTransfer(t, data, repoRoot, firstID, "src/app.txt")
	assertFileContent(t, filepath.Join(repoRoot, "src", "app.txt"), "v1")
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

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "src")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "src")
	assertFileContent(t, filepath.Join(repoRoot, "src", "app.txt"), "src v2")
	assertFileContent(t, filepath.Join(repoRoot, "src", "scratch.tmp"), "scratch")

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Restored path: src")
	assert.Contains(t, stdout, "Plan: "+planID)
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

func TestRestoreDirtyPathDecisionIsScopedToTargetPath(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside local edit"), 0644))

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside local edit")

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
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
	plansBefore := restorePlanFileCount(t, dirtyRepo)

	stdout, err = executeCommand(createTestRootCmd(), "restore", dirtyFirstID, "--path", "app.txt")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, "Preview only. No files were changed.", lines[0])
	assert.Contains(t, stdout, "Decision needed: path has unsaved changes in app.txt.")
	assert.Contains(t, stdout, "Folder: "+dirtyRepo)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Scope: path")
	assert.Contains(t, stdout, "Path: app.txt")
	assert.NotContains(t, stdout, "Plan: ")
	assert.Contains(t, stdout, "Source save point: "+dirtyFirstID)
	assert.Contains(t, stdout, "Managed files to overwrite: 1")
	assert.Contains(t, stdout, "app.txt")
	assert.Contains(t, stdout, "History will not change.")
	assert.Contains(t, stdout, "Expected path evidence: ")
	assert.Contains(t, stdout, "Rerun preview with one safety option:")
	assert.Contains(t, stdout, "jvs restore "+dirtyFirstID+" --path app.txt --save-first")
	assert.Contains(t, stdout, "jvs restore "+dirtyFirstID+" --path app.txt --discard-unsaved")
	assert.NotContains(t, stdout, "Run: `jvs restore --run")
	assertRestoreOutputOmitsLegacyVocabulary(t, strings.ReplaceAll(stdout, dirtyRepo, ""))
	app, readErr := os.ReadFile(filepath.Join(dirtyRepo, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "target local edit", string(app))
	assert.Equal(t, plansBefore, restorePlanFileCount(t, dirtyRepo))
	before.assertUnchanged(t, dirtyRepo)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "restore", dirtyFirstID, "--path", "app.txt")
	require.NoError(t, err)
	env, decisionData := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	require.Equal(t, "decision_preview", decisionData["mode"])
	require.Equal(t, "path", decisionData["scope"])
	require.Equal(t, "app.txt", decisionData["path"])
	require.Equal(t, dirtyFirstID, decisionData["source_save_point"])
	require.Equal(t, false, decisionData["history_changed"])
	require.Equal(t, false, decisionData["files_changed"])
	require.NotContains(t, decisionData, "plan_id")
	require.NotContains(t, decisionData, "run_command")
	nextCommands, ok := decisionData["next_commands"].([]any)
	require.True(t, ok, "next_commands should be an array: %#v", decisionData["next_commands"])
	assert.Contains(t, nextCommands, "jvs restore "+dirtyFirstID+" --path app.txt --save-first")
	assert.Contains(t, nextCommands, "jvs restore "+dirtyFirstID+" --path app.txt --discard-unsaved")
	assertRestorePreviewImpact(t, decisionData, "overwrite", 1, "app.txt")
	assertRestoreJSONOmitsLegacyFields(t, decisionData)
	assert.Equal(t, plansBefore, restorePlanFileCount(t, dirtyRepo))
	before.assertUnchanged(t, dirtyRepo)

	previewOut, err = executeCommand(createTestRootCmd(), "restore", dirtyFirstID, "--path", "app.txt", "--discard-unsaved")
	require.NoError(t, err)
	planID = assertPathRestorePreviewHuman(t, previewOut, dirtyRepo, dirtyFirstID, "", "app.txt")
	assert.Contains(t, previewOut, "Run options: discard unsaved changes")
	assertFileContent(t, filepath.Join(dirtyRepo, "app.txt"), "target local edit")
	assertFileContent(t, filepath.Join(dirtyRepo, "outside.txt"), "outside local edit")

	stdout, err = executeCommand(createTestRootCmd(), "restore", "--run", planID)
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

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "app.txt", "--save-first")
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	require.Equal(t, "preview", previewData["mode"])
	require.Equal(t, "path", previewData["scope"])
	require.Equal(t, "app.txt", previewData["path"])
	require.Equal(t, secondID, previewData["expected_newest_save_point"])
	previewOptions, ok := previewData["options"].(map[string]any)
	require.True(t, ok, "preview options should be exposed: %#v", previewData["options"])
	require.Equal(t, true, previewOptions["save_first"])
	planID := previewData["plan_id"].(string)
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "target local edit")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside local edit")

	restoreOut, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)
	_, restoreData := decodeFacadeDataMap(t, restoreOut)
	require.Equal(t, "run", restoreData["mode"])
	require.Equal(t, planID, restoreData["plan_id"])
	saveFirstID, ok := restoreData["newest_save_point"].(string)
	require.True(t, ok)
	require.NotEmpty(t, saveFirstID)
	require.NotEqual(t, secondID, saveFirstID)
	require.Equal(t, saveFirstID, restoreData["history_head"])
	require.Equal(t, saveFirstID, restoreData["content_source"])
	require.Equal(t, firstID, restoreData["from_save_point"])
	assertSaveFirstRestoreRunTransfers(t, restoreData, repoRoot, firstID, filepath.Join(repoRoot, "app.txt"), "restore-path-run-primary", ".restore-path-tmp-")
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

func TestRestorePathRunRejectsChangedTargetEvidenceWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target changed after preview"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "folder changed since preview")
	assert.Contains(t, err.Error(), "run preview again")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "target changed after preview")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside v2")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePathRunRejectsChangedNewestWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v3"), 0644))
	thirdID := savePointIDFromCLI(t, "third")

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "folder changed since preview")
	assert.Contains(t, err.Error(), "run preview again")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside v3")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(thirdID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(thirdID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	require.NotEqual(t, secondID, thirdID)
}

func TestRestorePathSaveFirstRunValidatesSourceBeforeSafetySave(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target local edit"), 0644))
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt", "--save-first")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".jvs", "snapshots", firstID, "app.txt"), []byte("tainted source"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "source save point is not restorable")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "target local edit")
	require.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePathRunRejectsRuntimeBehaviorFlagsWithoutMutation(t *testing.T) {
	t.Run("discard preview then save-first run", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt", "--discard-unsaved")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := captureViewMutationSnapshot(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID, "--save-first")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "options are fixed by the preview plan")
		assert.Contains(t, err.Error(), "--save-first")
		assert.Contains(t, err.Error(), "No files were changed.")
		assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "target local edit")
		before.assertUnchanged(t, repoRoot)
	})

	t.Run("save-first preview then removed legacy discard flag json", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("target local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--path", "app.txt", "--save-first")
		require.NoError(t, err)
		_, previewData := decodeFacadeDataMap(t, previewOut)
		planID := previewData["plan_id"].(string)
		before := captureViewMutationSnapshot(t, repoRoot)

		jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", "--run", planID, "--discard-dirty")
		require.NotZero(t, exitCode)
		require.Empty(t, strings.TrimSpace(stderr))
		env := decodeContractEnvelope(t, jsonOut)
		require.False(t, env.OK, jsonOut)
		require.NotNil(t, env.Error)
		assert.Contains(t, env.Error.Message, "unknown flag: --discard-dirty")
		assert.NotContains(t, env.Error.Message, "options are fixed by the preview plan")
		assert.NotContains(t, env.Error.Message, "--discard-unsaved")
		assert.NotContains(t, env.Error.Message, "deprecated")
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "target local edit")
		before.assertUnchanged(t, repoRoot)
	})
}

func TestRestorePathEditedSourceReconcilesStatusAndSave(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", planID)
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
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := assertPathRestorePreviewHuman(t, previewOut, repoRoot, firstID, secondID, "app.txt")
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", planID)
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
			require.NoError(t, err)
			assert.Contains(t, stdout, "Decision needed: path has unsaved changes in "+path+".")
			assert.Contains(t, stdout, path)
			assert.NotContains(t, stdout, strings.ReplaceAll(path, strings.Split(path, ".")[0], "source"))
			assert.NotContains(t, stdout, "Plan: ")
			assert.NotContains(t, stdout, "Run: `jvs restore --run")
			assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, strings.ReplaceAll(stdout, repoRoot, ""), path)
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

func setupRestorePathTransferFacadeRepo(t *testing.T) (repoRoot, firstID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("v1"), 0644))
	firstID = savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	return repoRoot, firstID
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

func assertPathRestorePreviewHuman(t *testing.T, stdout, repoRoot, sourceID, newestID, path string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.NotEmpty(t, lines)
	require.Equal(t, "Preview only. No files were changed.", lines[0])
	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Plan: ")
	assert.Contains(t, stdout, "Scope: path")
	assert.Contains(t, stdout, "Path: "+path)
	assert.Contains(t, stdout, "Source save point: "+sourceID)
	assert.Contains(t, stdout, "Managed files to overwrite:")
	assert.Contains(t, stdout, "History will not change.")
	if newestID != "" {
		assert.Contains(t, stdout, "Newest save point is still "+newestID+".")
		assert.Contains(t, stdout, "You can return to save point "+newestID+".")
		assert.Contains(t, stdout, "Expected newest save point: "+newestID)
	}
	assert.Contains(t, stdout, "Expected path evidence: ")
	assert.NotContains(t, stdout, "Restored path:")
	planID := restorePlanIDFromHumanOutput(t, stdout)
	assert.Contains(t, stdout, "Run: `jvs restore --run "+planID+"`")
	assertRestorePathErrorOmitsLegacyVocabularyOutsidePath(t, strings.ReplaceAll(stdout, repoRoot, ""), path)
	assertRestorePlanFileExists(t, repoRoot, planID)
	return planID
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
