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

func TestHistoryPathFindsFileCandidateWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "notes.md"), []byte("v1"), 0644))
	firstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "notes present")
	require.NoError(t, err)
	_, firstData := decodeFacadeDataMap(t, firstOut)
	firstID := firstData["save_point_id"].(string)

	require.NoError(t, os.Remove(filepath.Join(repoRoot, "notes.md")))
	_, err = executeCommand(createTestRootCmd(), "save", "-m", "notes removed")
	require.NoError(t, err)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path", "notes.md")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Candidates for path: notes.md")
	assert.Contains(t, stdout, firstID)
	assert.Contains(t, stdout, "notes present")
	assert.NotContains(t, stdout, "notes removed")
	assert.Contains(t, stdout, "Next:")
	assert.Contains(t, stdout, "jvs view "+firstID+" -- notes.md")
	assert.NotContains(t, stdout, "jvs restore")
	assert.Contains(t, stdout, "No files were changed.")
	assertNoOldSavePointVocabulary(t, stdout)
	require.NoFileExists(t, filepath.Join(repoRoot, "notes.md"))
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathFindsDirectoryCandidateWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("v1"), 0644))
	firstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "src present")
	require.NoError(t, err)
	_, firstData := decodeFacadeDataMap(t, firstOut)
	firstID := firstData["save_point_id"].(string)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path", "src/")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Candidates for path: src")
	assert.Contains(t, stdout, firstID)
	assert.Contains(t, stdout, "src present")
	assert.Contains(t, stdout, "jvs view "+firstID+" -- src")
	assert.Contains(t, stdout, "No files were changed.")
	assertNoOldSavePointVocabulary(t, stdout)
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathNoCandidatesSucceedsWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_, err := executeCommand(createTestRootCmd(), "save", "-m", "baseline")
	require.NoError(t, err)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path", "missing.txt")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Candidates for path: missing.txt")
	assert.Contains(t, stdout, "No candidates found.")
	assert.Contains(t, stdout, "No files were changed.")
	assertNoOldSavePointVocabulary(t, stdout)
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathJSONUsesPublicSchemaWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "notes.md"), []byte("v1"), 0644))
	firstOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "notes present")
	require.NoError(t, err)
	_, firstData := decodeFacadeDataMap(t, firstOut)
	firstID := firstData["save_point_id"].(string)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history", "--path", "notes.md")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "history", env.Command)
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, "notes.md", data["path"])
	candidates, ok := data["candidates"].([]any)
	require.True(t, ok, "candidates should be an array: %#v", data["candidates"])
	require.Len(t, candidates, 1)
	firstCandidate, ok := candidates[0].(map[string]any)
	require.True(t, ok, "candidate should be an object: %#v", candidates[0])
	require.Equal(t, firstID, firstCandidate["save_point_id"])
	require.Equal(t, "notes present", firstCandidate["message"])
	require.NotEmpty(t, firstCandidate["created_at"])
	nextCommands, ok := data["next_commands"].([]any)
	require.True(t, ok, "next_commands should be an array: %#v", data["next_commands"])
	require.Len(t, nextCommands, 1)
	assert.Contains(t, nextCommands, "jvs view "+firstID+" -- notes.md")
	assertNoLegacyJSONFields(t, data)
	assertNoOldSavePointVocabulary(t, string(env.Data))
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathRejectsUnsupportedFiltersWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "baseline")
	before := captureViewMutationSnapshot(t, repoRoot)

	cases := []struct {
		name string
		args []string
	}{
		{name: "grep", args: []string{"history", "--path", "file.txt", "--grep", "base"}},
		{name: "limit", args: []string{"history", "--path", "file.txt", "--limit", "1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), tc.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "history --path only searches path candidates in this workspace")
			assert.Contains(t, err.Error(), "No files were changed.")
			assertNoOldSavePointVocabulary(t, err.Error())
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestHistoryRejectsTagFlagAsUnknownPublicSurface(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "baseline")
	before := captureViewMutationSnapshot(t, repoRoot)

	for _, args := range [][]string{
		{"history", "--tag", "v1"},
		{"history", "--path", "file.txt", "--tag", "v1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), args...)

			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "unknown flag: --tag")
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestHistoryPathRejectsTaintedPayloadWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	savePointID := savePointIDFromCLI(t, "tainted")
	injectControlDataIntoSavePoint(t, repoRoot, model.SnapshotID(savePointID))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path", "file.txt")
	require.Error(t, err)

	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "control data")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertNoOldSavePointVocabulary(t, err.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathNextCommandShellQuotesPath(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs"), 0755))
	path := "docs/O'Brien notes.md"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(path)), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "quoted path")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path", path)
	require.NoError(t, err)

	expected := "jvs view " + firstID + " -- 'docs/O'\\''Brien notes.md'"
	assert.Contains(t, stdout, expected)
	assert.NotContains(t, stdout, "jvs view "+firstID+" "+path)
	assert.NotContains(t, stdout, "jvs restore")
	assert.Contains(t, stdout, "No files were changed.")
	assertNoOldSavePointVocabulary(t, stdout)
	before.assertUnchanged(t, repoRoot)
}

func TestHistoryPathNextCommandUsesDashDashForLeadingDashPath(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	path := "-notes.md"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, path), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "dash path")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "history", "--path="+path)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Candidates for path: "+path)
	assert.Contains(t, stdout, "jvs view "+firstID+" -- "+path)
	assert.NotContains(t, stdout, "jvs view "+firstID+" "+path)
	assert.NotContains(t, stdout, "jvs restore")
	assert.Contains(t, stdout, "No files were changed.")
	assertNoOldSavePointVocabulary(t, stdout)
	before.assertUnchanged(t, repoRoot)

	viewOut, err := executeCommand(createTestRootCmd(), "view", firstID, "--", path)
	require.NoError(t, err)
	assert.Contains(t, viewOut, "Path inside save point: "+path)
	viewPath := viewPathFromHumanOutput(t, viewOut)
	restoreViewWriteBitsForCleanup(t, viewPath)
}

func TestHistoryPathRejectsUnsafePathsWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "baseline")
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
			stdout, err := executeCommand(createTestRootCmd(), "history", "--path", path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "workspace-relative path")
			assert.Contains(t, err.Error(), "No files were changed.")
			assertNoOldSavePointVocabulary(t, err.Error())
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestHistoryPathHelpUsesSavePointVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "history", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "--path")
	assert.Contains(t, stdout, "jvs history --path notes.md")
	assert.Contains(t, stdout, "jvs view <save> <path>")
	assert.NotContains(t, stdout, "restore <save> --path")
	assert.Contains(t, stdout, "save point")
	assertNoOldSavePointVocabulary(t, stdout)
}

func safeHistoryPathTestName(path string) string {
	if path == "" {
		return "empty"
	}
	name := strings.ReplaceAll(path, "\x00", "_nul_")
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	name = strings.ReplaceAll(name, `\`, "_")
	return name
}
