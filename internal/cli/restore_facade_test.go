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

func TestRestoreRejectsRemovedLegacySafetyFlagsWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)

	for _, flag := range []string{"--include-working", "--discard-dirty"} {
		t.Run(flag, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, flag)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "unknown flag: "+flag)
			assert.NotContains(t, err.Error(), "options are fixed by the preview plan")
			assert.NotContains(t, err.Error(), "--save-first")
			assert.NotContains(t, err.Error(), "--discard-unsaved")
			assert.NotContains(t, err.Error(), "deprecated")
			assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestRestoreHumanOutputUsesSavePointStatusFacts(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Plan: "+planID)
	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Managed files now match save point "+firstID+".")
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assert.Contains(t, stdout, "Next save creates a new save point after "+secondID+".")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
}

func TestRestoreAcceptsUniqueSavePointPrefix(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)
	firstPrefix := uniqueSavePointPrefix(firstID, secondID)

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstPrefix)
	require.NoError(t, err)
	assert.Contains(t, previewOut, "Source save point: "+firstID)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "v1", string(content))
}

func TestRestoreJSONUsesSavePointSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, planID, data["plan_id"])
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, firstID, data["source_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, "matches_save_point", data["files_state"])
	require.Equal(t, false, data["history_changed"])
	require.Equal(t, true, data["files_changed"])
	assertRestoreJSONOmitsLegacyFields(t, data)
}

func TestRestoreDirtyDecisionPreviewUsesUnsavedChangesVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	plansBefore := restorePlanFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Decision needed: folder has unsaved changes.")
	assert.Contains(t, stdout, "--save-first")
	assert.Contains(t, stdout, "--discard-unsaved")
	assert.Contains(t, stdout, "No files were changed.")
	assert.NotContains(t, stdout, "Plan: ")
	assert.NotContains(t, stdout, "Run: `jvs restore --run")
	assertRestoreOutputOmitsLegacyVocabulary(t, strings.ReplaceAll(stdout, repoRoot, ""))

	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "local edit", string(content))
	require.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", firstID)
	require.Zero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	require.Nil(t, env.Error)
	_, data := decodeFacadeDataMap(t, jsonOut)
	require.Equal(t, "decision_preview", data["mode"])
	require.NotContains(t, data, "plan_id")
	require.NotContains(t, data, "run_command")
	nextCommands, ok := data["next_commands"].([]any)
	require.True(t, ok, "next_commands should be an array: %#v", data["next_commands"])
	assert.Contains(t, nextCommands, "jvs restore "+firstID+" --save-first")
	assert.Contains(t, nextCommands, "jvs restore "+firstID+" --discard-unsaved")
	require.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
}

func TestRestoreRejectsOldRefsAndFuzzyInputsWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("tagged"), 0644))
	const tag = "restore-release"
	_ = savePointIDFromCLI(t, "tagged legacy ref")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("newest"), 0644))
	_ = savePointIDFromCLI(t, "newest")
	before := captureViewMutationSnapshot(t, repoRoot)

	for _, ref := range []string{"current", "latest", "dirty", tag, "tagged legacy ref"} {
		t.Run(ref, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "restore", ref)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "save point ID")
			assert.Contains(t, err.Error(), "Choose a save point ID")
			assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
			content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
			require.NoError(t, readErr)
			require.Equal(t, "newest", string(content))
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestRestoreRejectsOldRefsWithCleanJSONErrorWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	_, _ = createTwoSavePoints(t, repoRoot)
	before := captureViewMutationSnapshot(t, repoRoot)

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", "latest")
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "save point ID")
	assert.Contains(t, env.Error.Message, "Choose a save point ID")
	assertRestoreOutputOmitsLegacyVocabulary(t, env.Error.Message)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreRejectsAmbiguousSavePointPrefixWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID, secondID := createTwoSavePoints(t, repoRoot)
	prefix := commonSavePointPrefix(firstID, secondID)
	require.NotEmpty(t, prefix)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", prefix)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "matches multiple save points")
	assert.Contains(t, err.Error(), "full save point ID")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "v2", string(content))
	before.assertUnchanged(t, repoRoot)
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

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--discard-unsaved")
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	_, data := decodeFacadeDataMap(t, stdout)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, planID, data["plan_id"])
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

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--save-first")
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
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

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
}

func commonSavePointPrefix(first, second string) string {
	limit := len(first)
	if len(second) < limit {
		limit = len(second)
	}
	i := 0
	for i < limit && first[i] == second[i] {
		i++
	}
	return first[:i]
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
