package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/integrity"
	jvsrepo "github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type savePointCommandOutput struct {
	SavePointID string `json:"save_point_id"`
}

type statusCommandOutput struct {
	Folder          string  `json:"folder"`
	Workspace       string  `json:"workspace"`
	NewestSavePoint *string `json:"newest_save_point"`
	HistoryHead     *string `json:"history_head"`
	ContentSource   *string `json:"content_source"`
	UnsavedChanges  bool    `json:"unsaved_changes"`
	FilesState      string  `json:"files_state"`
}

func setupPublicCLIRepo(t *testing.T, name string) (repoPath string, mainPath string) {
	t.Helper()

	dir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})

	require.NoError(t, os.Chdir(dir))
	repoPath = initLegacyRepoForCLITest(t, name)
	mainPath = filepath.Join(repoPath, "main")
	require.NoError(t, os.Chdir(mainPath))
	return repoPath, mainPath
}

func runPublicCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := createTestRootCmd()
	return executeCommand(cmd, args...)
}

func createCheckpointForPublicCLI(t *testing.T, note string, _ ...string) string {
	t.Helper()

	stdout, err := runPublicCLI(t, "--json", "save", "-m", note)
	require.NoError(t, err, stdout)

	var out savePointCommandOutput
	decodePublicData(t, stdout, &out)
	require.NotEmpty(t, out.SavePointID)
	return out.SavePointID
}

func readStatusForPublicCLI(t *testing.T) statusCommandOutput {
	t.Helper()

	stdout, err := runPublicCLI(t, "--json", "status")
	require.NoError(t, err, stdout)

	var out statusCommandOutput
	decodePublicData(t, stdout, &out)
	return out
}

func runPublicRestoreJSON(t *testing.T, savePoint string, flags ...string) map[string]any {
	t.Helper()

	previewArgs := append([]string{"--json", "restore", savePoint}, flags...)
	stdout, err := runPublicCLI(t, previewArgs...)
	require.NoError(t, err, stdout)
	var preview map[string]any
	decodePublicData(t, stdout, &preview)
	planID, ok := preview["plan_id"].(string)
	require.True(t, ok, "restore preview should return plan_id: %#v", preview)
	require.NotEmpty(t, planID)

	stdout, err = runPublicCLI(t, "--json", "restore", "--run", planID)
	require.NoError(t, err, stdout)
	var restored map[string]any
	decodePublicData(t, stdout, &restored)
	return restored
}

func decodePublicData(t *testing.T, stdout string, target any) contractEnvelope {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NoError(t, json.Unmarshal(env.Data, target), stdout)
	return env
}

func TestPublicCLIStatusAndSavePointCleanliness(t *testing.T) {
	_, mainPath := setupPublicCLIRepo(t, "statusrepo")

	require.NoError(t, os.WriteFile("file.txt", []byte("before save"), 0644))
	dirty := readStatusForPublicCLI(t)
	assert.True(t, dirty.UnsavedChanges)
	assert.Equal(t, "main", dirty.Workspace)
	assert.Equal(t, mainPath, dirty.Folder)
	assert.Nil(t, dirty.ContentSource)
	assert.Nil(t, dirty.NewestSavePoint)
	assert.Equal(t, "not_saved", dirty.FilesState)

	id := createCheckpointForPublicCLI(t, "first save point")
	clean := readStatusForPublicCLI(t)
	assert.False(t, clean.UnsavedChanges)
	require.NotNil(t, clean.ContentSource)
	require.NotNil(t, clean.NewestSavePoint)
	require.NotNil(t, clean.HistoryHead)
	assert.Equal(t, id, *clean.ContentSource)
	assert.Equal(t, id, *clean.NewestSavePoint)
	assert.Equal(t, id, *clean.HistoryHead)
	assert.Equal(t, "matches_save_point", clean.FilesState)
}

func TestPublicCLISaveCapturesManagedFilesWithReflinkEngine(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "savereflink")
	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineReflinkCopy))

	require.NoError(t, os.WriteFile("config.yaml", []byte("config"), 0644))
	require.NoError(t, os.WriteFile("README.md", []byte("readme"), 0644))

	id := createCheckpointForPublicCLI(t, "all managed files")

	savePointDir := filepath.Join(repoPath, ".jvs", "snapshots", id)
	content, err := os.ReadFile(filepath.Join(savePointDir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "config", string(content))
	readme, err := os.ReadFile(filepath.Join(savePointDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "readme", string(readme))
}

func TestPublicCLIStatusTreatsRootReadyAsDirtyOrReserved(t *testing.T) {
	setupPublicCLIRepo(t, "reservedstatus")

	require.NoError(t, os.WriteFile("file.txt", []byte("before save"), 0644))
	createCheckpointForPublicCLI(t, "baseline")
	require.NoError(t, os.WriteFile(".READY", []byte("user data"), 0644))

	stdout, err := runPublicCLI(t, "--json", "status")
	if err != nil {
		assert.Contains(t, err.Error(), "reserved")
		return
	}

	var status statusCommandOutput
	decodePublicData(t, stdout, &status)
	assert.True(t, status.UnsavedChanges, "root .READY must not be reported clean")
}

func TestPublicCLIDirtyRestoreRequiresExplicitChoice(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "dirtyrestore")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")

	_ = runPublicRestoreJSON(t, first)
	require.NoError(t, os.WriteFile("file.txt", []byte("local edit"), 0644))
	statusBefore := readStatusForPublicCLI(t)
	historyBefore, err := runPublicCLI(t, "--json", "history")
	require.NoError(t, err, historyBefore)
	plansBefore := restorePlanFileCount(t, repoPath)

	stdout, err := runPublicCLI(t, "--json", "restore", second)
	require.NoError(t, err, stdout)

	var preview map[string]any
	decodePublicData(t, stdout, &preview)
	assert.Equal(t, "decision_preview", preview["mode"])
	assert.Equal(t, second, preview["source_save_point"])
	assert.Equal(t, second, preview["newest_save_point"])
	assert.Equal(t, second, preview["history_head"])
	assert.Equal(t, false, preview["history_changed"])
	assert.Equal(t, false, preview["files_changed"])
	assert.NotContains(t, preview, "plan_id")
	assert.NotContains(t, preview, "run_command")
	nextCommands, ok := preview["next_commands"].([]any)
	require.True(t, ok, "decision preview should expose next_commands: %#v", preview)
	assert.Contains(t, nextCommands, "jvs restore "+second+" --save-first")
	assert.Contains(t, nextCommands, "jvs restore "+second+" --discard-unsaved")

	content, err := os.ReadFile("file.txt")
	require.NoError(t, err)
	assert.Equal(t, "local edit", string(content))
	assert.Equal(t, statusBefore, readStatusForPublicCLI(t))
	historyAfter, err := runPublicCLI(t, "--json", "history")
	require.NoError(t, err, historyAfter)
	assert.JSONEq(t, string(decodeContractEnvelope(t, historyBefore).Data), string(decodeContractEnvelope(t, historyAfter).Data))
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoPath))

	restored := runPublicRestoreJSON(t, second, "--discard-unsaved")
	assert.Equal(t, second, restored["restored_save_point"])
	assert.Equal(t, second, restored["newest_save_point"])
	assert.Equal(t, second, restored["history_head"])
	assert.Equal(t, second, restored["content_source"])
	assert.Equal(t, false, restored["unsaved_changes"])
	assert.Equal(t, "matches_save_point", restored["files_state"])
	assert.Equal(t, false, restored["history_changed"])
}

func TestPublicCLIDirtyRestoreSaveFirstCreatesSavePoint(t *testing.T) {
	setupPublicCLIRepo(t, "savefirstrestore")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")
	require.NoError(t, os.WriteFile("file.txt", []byte("local edit"), 0644))

	restored := runPublicRestoreJSON(t, first, "--save-first")
	assert.Equal(t, first, restored["restored_save_point"])

	status := readStatusForPublicCLI(t)
	assert.False(t, status.UnsavedChanges)
	require.NotNil(t, status.ContentSource)
	require.NotNil(t, status.NewestSavePoint)
	assert.Equal(t, first, *status.ContentSource)
	assert.NotEqual(t, second, *status.NewestSavePoint)

	stdout, err := runPublicCLI(t, "--json", "history")
	require.NoError(t, err, stdout)
	var history map[string]any
	decodePublicData(t, stdout, &history)
	assert.Equal(t, first, history["current_pointer"])
	savePoints, ok := history["save_points"].([]any)
	require.True(t, ok, "history should expose save_points: %#v", history)
	require.Len(t, savePoints, 1)
	sourcePoint, ok := savePoints[0].(map[string]any)
	require.True(t, ok, "save point should be an object: %#v", savePoints[0])
	assert.Equal(t, first, sourcePoint["save_point_id"])
}

func TestPublicCLIRestoreNewestAndJSONBoolConsistency(t *testing.T) {
	setupPublicCLIRepo(t, "restorelatest")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")

	restored := runPublicRestoreJSON(t, first)
	assert.Equal(t, first, restored["restored_save_point"])
	assert.Equal(t, second, restored["newest_save_point"])
	assert.Equal(t, first, restored["content_source"])
	assert.Equal(t, false, restored["unsaved_changes"])

	restored = runPublicRestoreJSON(t, second)
	assert.Equal(t, second, restored["restored_save_point"])
	assert.Equal(t, second, restored["newest_save_point"])
	assert.Equal(t, second, restored["content_source"])
	assert.Equal(t, false, restored["unsaved_changes"])

	status := readStatusForPublicCLI(t)
	assert.IsType(t, true, status.UnsavedChanges)
}

func TestPublicCLIWorkspacePathJSONUsesEnvelope(t *testing.T) {
	_, mainPath := setupPublicCLIRepo(t, "pathjson")

	stdout, err := runPublicCLI(t, "--json", "workspace", "path")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)

	var out map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &out), stdout)
	assert.Equal(t, "main", out["workspace"])
	assert.Equal(t, mainPath, out["path"])
	assert.NotContains(t, strings.TrimSpace(stdout), "\n"+mainPath)
}

func TestPublicCLICleanupRunJSONUsesEnvelope(t *testing.T) {
	setupPublicCLIRepo(t, "cleanuprunjson")

	planOut, err := runPublicCLI(t, "--json", "cleanup", "preview")
	require.NoError(t, err, planOut)
	var plan map[string]any
	decodePublicData(t, planOut, &plan)
	planID, _ := plan["plan_id"].(string)
	require.NotEmpty(t, planID)

	runOut, err := runPublicCLI(t, "--json", "cleanup", "run", "--plan-id", planID)
	require.NoError(t, err, runOut)
	env := decodeContractEnvelope(t, runOut)
	require.True(t, env.OK, runOut)
	assert.Equal(t, "cleanup run", env.Command)
	assert.JSONEq(t, `{"plan_id":"`+planID+`","status":"completed"}`, string(env.Data))
}

func TestPublicCLICleanupPreviewJSONUsesSavePointFields(t *testing.T) {
	setupPublicCLIRepo(t, "cleanuppreviewjson")

	stdout, err := runPublicCLI(t, "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)

	var plan map[string]any
	decodePublicData(t, stdout, &plan)
	assert.Contains(t, plan, "created_at")
	assert.Contains(t, plan, "protected_save_points")
	assert.Contains(t, plan, "reclaimable_save_points")
	assert.IsType(t, []any{}, plan["protected_save_points"])
	assert.IsType(t, []any{}, plan["reclaimable_save_points"])
	assert.NotContains(t, plan, "protected_checkpoints")
	assert.NotContains(t, plan, "to_delete")
	assert.NotContains(t, plan, "delete_checkpoints")
	assert.NotContains(t, plan, "retention")
	assert.NotContains(t, plan, "retention_policy")
}

func TestPublicCLICleanupRunJSONMissingPlanUsesStableError(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "cleanupmissingplan")

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "cleanup", "run", "--plan-id", "missing")
	require.Equal(t, 1, exitCode, "cleanup run unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "cleanup run", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_CLEANUP_PLAN_MISMATCH", env.Error.Code)
	assert.NotContains(t, strings.ToLower(env.Error.Message), "gc")
	assert.NotContains(t, env.Error.Message, "control leaf")
	assert.NotContains(t, env.Error.Message, "stat ")
	assert.NotContains(t, env.Error.Message, repoPath)
	assert.NotContains(t, env.Error.Message, ".jvs")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIWorkspaceRemoveRejectsUnsavedChangesByDefault(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "dirtyremove")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	base := createCheckpointForPublicCLI(t, "base")

	stdout, err := runPublicCLI(t, "--json", "workspace", "new", "../feature", "--from", base)
	require.NoError(t, err, stdout)

	featureFile := filepath.Join(repoPath, "feature", "file.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("unsaved"), 0644))

	stdout, err = runPublicCLI(t, "workspace", "remove", "feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unsaved changes")
	assert.NotContains(t, err.Error(), "checkpoint")
	assert.FileExists(t, featureFile)

	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "feature", "--force")
	require.NoError(t, err, stdout)
	var preview map[string]any
	decodePublicData(t, stdout, &preview)
	planID, ok := preview["plan_id"].(string)
	require.True(t, ok, "workspace remove preview should expose plan_id: %#v", preview)
	assert.DirExists(t, filepath.Join(repoPath, "feature"))

	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "--run", planID)
	require.NoError(t, err, stdout)
	assert.NoDirExists(t, filepath.Join(repoPath, "feature"))
}

func TestPublicCLIStableJSONErrorsUsePublicVocabulary(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "publicerrors")
	require.NoError(t, os.Chdir(repoPath))

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "status")
	require.Equal(t, 1, exitCode, "status unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
	assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIErrorsPreserveUserRepoPathsWithSpacesAndLegacyWords(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	targetRepo := initLegacyRepoForCLITest(t, "target worktree snapshot history")
	currentRepo := initLegacyRepoForCLITest(t, "current worktree snapshot history")
	stdout, stderr, exitCode := runContractSubprocess(
		t,
		filepath.Join(currentRepo, "main"),
		"--json", "--repo", targetRepo, "--workspace", "main", "status",
	)
	require.Equal(t, 1, exitCode, "mismatched repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_TARGET_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "targeting mismatch")
	assert.Contains(t, env.Error.Message, targetRepo)
	assert.Contains(t, env.Error.Message, currentRepo)
	assertPublicErrorOmitsLegacyVocabularyExcept(t, env.Error, targetRepo, currentRepo)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIRepoFlagMismatchRejectsChildLocatorForgery(t *testing.T) {
	cases := []struct {
		name          string
		writeLocator  func(t *testing.T, dir, targetRepo string)
		wantErrorCode string
	}{
		{
			name: "forged target locator",
			writeLocator: func(t *testing.T, dir, targetRepo string) {
				t.Helper()
				writePublicCLITestWorkspaceLocator(t, dir, targetRepo)
			},
			wantErrorCode: "E_TARGET_MISMATCH",
		},
		{
			name: "invalid locator",
			writeLocator: func(t *testing.T, dir, _ string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, jvsrepo.JVSDirName), []byte("{not-json"), 0644))
			},
			wantErrorCode: "E_TARGET_MISMATCH",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			originalWd, _ := os.Getwd()
			defer os.Chdir(originalWd)

			require.NoError(t, os.Chdir(dir))
			targetRepo := initLegacyRepoForCLITest(t, "target")
			currentRepo := initLegacyRepoForCLITest(t, "current")
			child := filepath.Join(currentRepo, "main", "nested")
			require.NoError(t, os.MkdirAll(child, 0755))
			tc.writeLocator(t, child, targetRepo)

			stdout, stderr, exitCode := runContractSubprocess(t, child, "--json", "--repo", targetRepo, "history")
			require.Equal(t, 1, exitCode, "mismatched repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			require.NotNil(t, env.Error)
			assert.Equal(t, tc.wantErrorCode, env.Error.Code)
			assert.Contains(t, env.Error.Message, targetRepo)
			assert.Contains(t, env.Error.Message, currentRepo)
			assert.JSONEq(t, `null`, string(env.Data))
		})
	}
}

func TestPublicCLIRepoFlagPathPrefersPhysicalAncestorOverForgedLocator(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	targetRepo := initLegacyRepoForCLITest(t, "target")
	attackerRepo := initLegacyRepoForCLITest(t, "attacker")
	child := filepath.Join(targetRepo, "main", "nested")
	require.NoError(t, os.MkdirAll(child, 0755))
	writePublicCLITestWorkspaceLocator(t, child, attackerRepo)

	outside := filepath.Join(dir, "outside")
	require.NoError(t, os.MkdirAll(outside, 0755))
	stdout, stderr, exitCode := runContractSubprocess(t, outside, "--json", "--repo", child, "status")
	require.Equal(t, 0, exitCode, "status unexpectedly failed: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, targetRepo, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)

	var status statusCommandOutput
	require.NoError(t, json.Unmarshal(env.Data, &status), stdout)
	assert.Equal(t, filepath.Join(targetRepo, "main"), status.Folder)
	assert.Equal(t, "main", status.Workspace)
}

func TestPublicCLIRepoFlagRejectsExternalWorkspaceLocatorMismatchWithExplicitWorkspace(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoA := initLegacyRepoForCLITest(t, "repo-a")
	repoB := initLegacyRepoForCLITest(t, "repo-b")
	repoAMain := filepath.Join(repoA, "main")
	require.NoError(t, os.Chdir(repoAMain))
	require.NoError(t, os.WriteFile("file.txt", []byte("repo a base"), 0644))
	base := createCheckpointForPublicCLI(t, "repo a base")
	featurePath := filepath.Join(dir, "repo-a-feature")
	stdout, err := runPublicCLI(t, "--json", "workspace", "new", featurePath, "--from", base, "--name", "feature")
	require.NoError(t, err, stdout)

	stdout, stderr, exitCode := runContractSubprocess(t, featurePath, "--json", "--repo", repoB, "--workspace", "main", "status")
	require.Equal(t, 1, exitCode, "mismatched repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_TARGET_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, repoA)
	assert.Contains(t, env.Error.Message, repoB)

	stdout, stderr, exitCode = runContractSubprocess(t, featurePath, "--json", "--repo", repoA, "--workspace", "main", "status")
	require.Equal(t, 0, exitCode, "same-repo explicit workspace target failed: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env = decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK, stdout)
	var status statusCommandOutput
	require.NoError(t, json.Unmarshal(env.Data, &status), stdout)
	assert.Equal(t, repoAMain, status.Folder)
	assert.Equal(t, "main", status.Workspace)
}

func TestPublicCLIRepoFlagPropagatesMalformedLocatorFromCurrentWorkspace(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "target")
	child := filepath.Join(repoPath, "main", "nested")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(child, jvsrepo.JVSDirName), []byte("{not-json"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, child, "--json", "--repo", repoPath, "history")
	require.Equal(t, 1, exitCode, "malformed locator unexpectedly defaulted to main: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "parse JVS workspace locator")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIRepoFlagPropagatesControlDiscoveryError(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	targetRepo := initLegacyRepoForCLITest(t, "target")
	currentRepo := initLegacyRepoForCLITest(t, "current")
	child := filepath.Join(currentRepo, "main", "nested")
	require.NoError(t, os.MkdirAll(child, 0755))
	if err := os.Symlink(jvsrepo.JVSDirName, filepath.Join(child, jvsrepo.JVSDirName)); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, exitCode := runContractSubprocess(t, child, "--json", "--repo", targetRepo, "history")
	require.Equal(t, 1, exitCode, "control discovery error unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "stat JVS control directory")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIJSONErrorsPreserveUserRepoPathWhenTargetIsMissing(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "missingtarget")

	missingTarget := "missing worktree snapshot history"
	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoPath, "main"), "--json", "--repo", missingTarget, "status")
	require.Equal(t, 1, exitCode, "missing --repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_REPO", env.Error.Code)
	assert.Contains(t, env.Error.Message, missingTarget)
	assertPublicErrorOmitsLegacyVocabularyExcept(t, env.Error, missingTarget)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIErrorVocabularyPreservesQuotedUserText(t *testing.T) {
	got := publicCLIErrorVocabulary(`snapshot "worktree-snapshot-history" history`)
	assert.Equal(t, `save point "worktree-snapshot-history" history`, got)
	assert.Equal(t, "source_workspace total_save_points save_point_id", publicCLIErrorVocabulary("source_worktree total_snapshots snapshot_id"))
	got = publicCLIErrorVocabulary("snapshot /tmp/missing worktree snapshot history, history")
	assert.Equal(t, "save point /tmp/missing worktree snapshot history, history", got)
	got = publicCLIErrorVocabulary("--repo is not inside a JVS repository: missing worktree snapshot history")
	assert.Equal(t, "--repo is not inside a JVS repository: missing worktree snapshot history", got)
	got = publicCLIErrorVocabulary("snapshot (/tmp/missing worktree snapshot history) history")
	assert.Equal(t, "save point (/tmp/missing worktree snapshot history) history", got)
	got = publicCLIErrorVocabulary("snapshot (worktree snapshot history) history")
	assert.Equal(t, "save point (workspace save point history) history", got)
}

func writePublicCLITestWorkspaceLocator(t *testing.T, dir, repoRoot string) {
	t.Helper()

	r, err := jvsrepo.DiscoverControlRepo(repoRoot)
	require.NoError(t, err)
	data, err := json.Marshal(map[string]any{
		"type":           "jvs-workspace",
		"format_version": jvsrepo.FormatVersion,
		"repo_root":      repoRoot,
		"repo_id":        r.RepoID,
		"workspace_name": "main",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, jvsrepo.JVSDirName), data, 0644))
}

func TestPublicCLIJSONUsesSavePointWorkspaceTerms(t *testing.T) {
	setupPublicCLIRepo(t, "terms")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	createCheckpointForPublicCLI(t, "first")

	cases := [][]string{
		{"--json", "history"},
		{"--json", "workspace", "list"},
		{"--json", "cleanup", "preview"},
		{"--json", "status"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, err := runPublicCLI(t, args...)
			require.NoError(t, err, stdout)
			env := decodeContractEnvelope(t, stdout)
			require.True(t, env.OK, stdout)
			assert.NotContains(t, string(env.Data), "snapshot_id")
			assert.NotContains(t, string(env.Data), "checkpoint")
			assert.NotContains(t, string(env.Data), "worktree")
			assert.NotContains(t, string(env.Data), "head_snapshot")
			assert.NotContains(t, string(env.Data), "latest_snapshot")
			assert.NotContains(t, string(env.Data), "from_snapshot")
			assert.NotContains(t, string(env.Data), "to_snapshot")
			assert.NotContains(t, strings.ToLower(string(env.Data)), "gc")
		})
	}
}

func TestPublicCLIDoctorAndCleanupJSONHideInternalContractFields(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "publicjsoncontract")

	emptyPlanOut, err := runPublicCLI(t, "--json", "cleanup", "preview")
	require.NoError(t, err, emptyPlanOut)
	assertPublicJSONDataOmitsInternalContractFields(t, emptyPlanOut)

	require.NoError(t, os.WriteFile("file.txt", []byte("main"), 0644))
	mainID := createCheckpointForPublicCLI(t, "main")
	stdout, err := runPublicCLI(t, "--json", "workspace", "new", "../old-feature", "--from", mainID)
	require.NoError(t, err, stdout)
	featurePath := filepath.Join(repoPath, "old-feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	createCheckpointForPublicCLI(t, "feature")
	require.NoError(t, os.Chdir(mainPath))
	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "old-feature", "--force")
	require.NoError(t, err, stdout)
	var removePreview map[string]any
	decodePublicData(t, stdout, &removePreview)
	removePlanID, ok := removePreview["plan_id"].(string)
	require.True(t, ok, "workspace remove preview should expose plan_id: %#v", removePreview)
	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "--run", removePlanID)
	require.NoError(t, err, stdout)
	makeAllDescriptorsOldForPublicCLI(t, repoPath)

	nonEmptyPlanOut, err := runPublicCLI(t, "--json", "cleanup", "preview")
	require.NoError(t, err, nonEmptyPlanOut)
	assertPublicJSONDataOmitsInternalContractFields(t, nonEmptyPlanOut)
	var plan map[string]any
	decodePublicData(t, nonEmptyPlanOut, &plan)
	assert.NotZero(t, plan["candidate_count"])

	require.NoError(t, os.RemoveAll(mainPath))
	doctorOut, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "doctor")
	require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", doctorOut, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	assertPublicJSONDataOmitsInternalContractFields(t, doctorOut)
	env := decodeContractEnvelope(t, doctorOut)
	assert.True(t, env.OK)
	var doctorData map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &doctorData), doctorOut)
	assert.Equal(t, false, doctorData["healthy"])
}

func assertPublicJSONDataOmitsInternalContractFields(t *testing.T, stdout string) {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	data := string(env.Data)
	for _, forbidden := range []string{
		"snapshot_id",
		"checkpoint",
		"worktree",
		"head_snapshot",
		"latest_snapshot",
		"keep_min_snapshots",
		"protected_by_pin",
		"protected_by_retention",
		"retention",
	} {
		assert.NotContains(t, data, forbidden)
	}
}

func makeAllDescriptorsOldForPublicCLI(t *testing.T, repoPath string) {
	t.Helper()

	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	entries, err := os.ReadDir(descriptorsDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(descriptorsDir, entry.Name())
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var desc model.Descriptor
		require.NoError(t, json.Unmarshal(data, &desc))
		desc.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		checksum, err := integrity.ComputeDescriptorChecksum(&desc)
		require.NoError(t, err)
		desc.DescriptorChecksum = checksum
		data, err = json.MarshalIndent(desc, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0644))
		updateReadyDescriptorChecksumForPublicCLI(t, repoPath, desc.SnapshotID, checksum)
	}
}

func updateReadyDescriptorChecksumForPublicCLI(t *testing.T, repoPath string, snapshotID model.SnapshotID, checksum model.HashValue) {
	t.Helper()

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	for _, name := range []string{".READY", ".READY.gz"} {
		path := filepath.Join(snapshotDir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		var marker map[string]any
		require.NoError(t, json.Unmarshal(data, &marker))
		marker["descriptor_checksum"] = string(checksum)
		data, err = json.MarshalIndent(marker, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0644))
	}
}
