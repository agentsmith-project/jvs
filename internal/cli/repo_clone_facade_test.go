package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoCloneCommandClonesCurrentRepoToExplicitMissingTarget(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	target := filepath.Join(t.TempDir(), "target")

	stdout, err := executeCommand(createTestRootCmd(), "repo", "clone", target, "--save-points", "main")
	require.NoError(t, err, stdout)

	assert.Contains(t, stdout, "Cloned JVS project")
	assert.Contains(t, stdout, "Source: "+repoRoot)
	assert.Contains(t, stdout, "Target: "+target)
	assert.Contains(t, stdout, "Save points copied: main history closure (1)")
	assert.Contains(t, stdout, "Workspaces created: main only")
	assert.Contains(t, stdout, "Source workspaces not created: none")
	assert.Contains(t, stdout, "Save point storage: Copy method:")
	assert.Contains(t, stdout, "Main workspace: Copy method:")
	assert.Contains(t, stdout, "Doctor strict: passed")

	sourceRepo, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	targetRepo, err := repo.Discover(target)
	require.NoError(t, err)
	assert.NotEqual(t, sourceRepo.RepoID, targetRepo.RepoID)

	assertFileContent(t, filepath.Join(target, "app.txt"), "v1")
	assert.NoDirExists(t, filepath.Join(target, "main"))
	cfg, err := repo.LoadWorktreeConfig(target, "main")
	require.NoError(t, err)
	assert.Equal(t, target, cfg.RealPath)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.LatestSnapshotID)

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(target))
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWD)) })

	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	statusData := decodeContractDataMap(t, statusOut)
	assert.Equal(t, target, statusData["folder"])
	assert.Equal(t, false, statusData["unsaved_changes"])

	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err, historyOut)
	historyData := decodeContractDataMap(t, historyOut)
	assert.Equal(t, sourceID, historyData["newest_save_point"])

	doctorOut, err := executeCommand(createTestRootCmd(), "--json", "doctor", "--strict")
	require.NoError(t, err, doctorOut)
	doctorData := decodeContractDataMap(t, doctorOut)
	assert.Equal(t, true, doctorData["healthy"])

	require.NoError(t, os.WriteFile(filepath.Join(target, "app.txt"), []byte("v2"), 0644))
	saveOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "target work")
	require.NoError(t, err, saveOut)
	saveData := decodeContractDataMap(t, saveOut)
	assert.NotEqual(t, sourceID, saveData["save_point_id"])
}

func TestRepoCloneJSONIncludesTwoRepoCloneTransfers(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	target := filepath.Join(t.TempDir(), "target")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "clone", target)
	require.NoError(t, err, stdout)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "repo clone", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, target, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assert.Equal(t, "repo_clone", data["operation"])
	assert.Equal(t, repoRoot, data["source_repo_root"])
	assert.Equal(t, target, data["target_repo_root"])
	assert.Equal(t, "all", data["save_points_mode"])
	assert.Equal(t, float64(1), data["save_points_copied_count"])
	assert.Equal(t, []any{"main"}, data["workspaces_created"])
	assert.Equal(t, false, data["runtime_state_copied"])
	assert.Equal(t, sourceID, data["newest_save_point"])

	transfers := requireRepoCloneTransferMaps(t, data, 2)
	assertRepoCloneTransferMap(t, transfers[0], "repo-clone-save-points", "save_point_storage_copy", "save_point_storage", "target_save_point_storage", "final", "execution")
	assertRepoCloneTransferMap(t, transfers[1], "repo-clone-main-workspace", "main_workspace_materialization", "source_main_current_state", "target_main_workspace", "final", "execution")
}

func TestRepoCloneDryRunDoesNotCreateTargetAndUsesExpectedTransferRecords(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "source")
	target := filepath.Join(t.TempDir(), "target")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "clone", target, "--dry-run")
	require.NoError(t, err, stdout)
	assert.NoDirExists(t, target)

	_, data := decodeFacadeDataMap(t, stdout)
	assert.Equal(t, true, data["dry_run"])
	transfers := requireRepoCloneTransferMaps(t, data, 2)
	for _, record := range transfers {
		assert.Equal(t, "repo_clone", record["operation"])
		assert.Equal(t, "expected", record["result_kind"])
		assert.Equal(t, "preview_only", record["permission_scope"])
	}
}

func TestRepoCloneRejectsBadArgumentsAndRemoteLikeTargetsWithoutWrites(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "source")

	stdout, err := executeCommand(createTestRootCmd(), "clone", filepath.Join(t.TempDir(), "target"))
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unknown command")

	stdout, err = executeCommand(createTestRootCmd(), "repo", "clone")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "requires a target folder")

	stdout, err = executeCommand(createTestRootCmd(), "repo", "clone", filepath.Join(t.TempDir(), "target"), filepath.Join(t.TempDir(), "extra"))
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "accepts exactly one target folder")

	for _, target := range []string{"https://example.com/repo", "ssh://host/path", "git@host:org/repo", "user@host:path", "github.com:org/repo", "host:org/repo"} {
		t.Run(target, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "repo", "clone", target)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), "only copies a local or mounted JVS project")
		})
	}

	existing := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.WriteFile(existing, []byte("not a dir"), 0644))
	stdout, err = executeCommand(createTestRootCmd(), "repo", "clone", existing)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "target folder already exists")
	content, readErr := os.ReadFile(existing)
	require.NoError(t, readErr)
	assert.Equal(t, "not a dir", string(content))
}

func TestRepoCloneCommandRejectsTargetInsideCurrentWorkspaceWithClearError(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "repo", "clone", "target")
	require.Error(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "target cannot be inside a source workspace")
	assert.Contains(t, stderr, "Choose a folder outside the source project/workspaces")
	assert.NotContains(t, stderr, "nested")
	assert.NotContains(t, stderr, "staging")
	assert.NoDirExists(t, filepath.Join(repoRoot, "target"))
	assertNoRepoCloneStaging(t, repoRoot)
}

func TestRepoCloneRepoFlagRejectsTargetInsideSourceProjectWithClearError(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "source")
	_, err := repo.Init(repoRoot, "source")
	require.NoError(t, err)
	mainWorkspace := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainWorkspace, "app.txt"), []byte("v1"), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "source", nil)
	require.NoError(t, err)

	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWD)) })

	target := filepath.Join(repoRoot, "clone-target")
	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--repo", repoRoot, "repo", "clone", target)
	require.Error(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "target cannot be inside the source project")
	assert.Contains(t, stderr, "Choose a folder outside the source project/workspaces")
	assert.NotContains(t, stderr, "nested")
	assert.NotContains(t, stderr, "staging")
	assert.NoDirExists(t, target)
	assertNoRepoCloneStaging(t, repoRoot)
}

func TestRepoCloneRepoFlagSelectsSource(t *testing.T) {
	source := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("from source"), 0644))
	sourceID := savePointIDFromCLI(t, "source")

	workDir := t.TempDir()
	require.NoError(t, os.Chdir(workDir))
	target := filepath.Join(workDir, "target")
	stdout, err := executeCommand(createTestRootCmd(), "--repo", source, "--json", "repo", "clone", target, "--save-points", "main")
	require.NoError(t, err, stdout)
	data := decodeContractDataMap(t, stdout)
	assert.Equal(t, source, data["source_repo_root"])
	assert.Equal(t, target, data["target_repo_root"])
	assert.Equal(t, sourceID, data["newest_save_point"])
	assertFileContent(t, filepath.Join(target, "app.txt"), "from source")
}

func TestRepoCloneRepoFlagSelectsSourceFromAnotherRepoCWD(t *testing.T) {
	source := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("from source"), 0644))
	sourceID := savePointIDFromCLI(t, "source")

	otherRepo := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(otherRepo, "app.txt"), []byte("from other"), 0644))
	_ = savePointIDFromCLI(t, "other")
	require.NoError(t, os.Chdir(otherRepo))

	target := filepath.Join(filepath.Dir(otherRepo), "target")
	stdout, err := executeCommand(createTestRootCmd(), "--repo", source, "--json", "repo", "clone", target, "--save-points", "main")
	require.NoError(t, err, stdout)
	data := decodeContractDataMap(t, stdout)
	assert.Equal(t, source, data["source_repo_root"])
	assert.Equal(t, target, data["target_repo_root"])
	assert.Equal(t, sourceID, data["newest_save_point"])
	assertFileContent(t, filepath.Join(target, "app.txt"), "from source")
}

func TestRepoCloneAllModeCleanupPreviewProtectsImportedNonMainHistory(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	featureOut, err := executeCommand(createTestRootCmd(), "workspace", "new", "../feature", "--from", sourceID)
	require.NoError(t, err, featureOut)
	featurePath := filepath.Join(filepath.Dir(repoRoot), "feature")
	require.NoError(t, os.Remove(filepath.Join(featurePath, "app.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	featureID := savePointIDFromCLI(t, "feature")
	require.NoError(t, os.Chdir(repoRoot))

	target := filepath.Join(t.TempDir(), "target")
	cloneOut, err := executeCommand(createTestRootCmd(), "--json", "repo", "clone", target, "--save-points", "all")
	require.NoError(t, err, cloneOut)

	require.NoError(t, os.Chdir(target))
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, previewOut)
	previewData := decodeContractDataMap(t, previewOut)
	protectedSet, err := json.Marshal(previewData["protected_save_points"])
	require.NoError(t, err)
	assert.Contains(t, string(protectedSet), featureID)
	groups, err := json.Marshal(previewData["protection_groups"])
	require.NoError(t, err)
	assert.Contains(t, string(groups), "imported_clone_history")

	savePoints, err := snapshot.ListAll(target)
	require.NoError(t, err)
	require.Len(t, savePoints, 2)
}

func requireRepoCloneTransferMaps(t *testing.T, data map[string]any, expected int) []map[string]any {
	t.Helper()

	raw, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, raw, expected)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		require.True(t, ok, "transfer should be an object: %#v", item)
		out = append(out, record)
	}
	return out
}

func assertRepoCloneTransferMap(t *testing.T, record map[string]any, id, phase, sourceRole, destinationRole, resultKind, permission string) {
	t.Helper()

	assert.Equal(t, id, record["transfer_id"])
	assert.Equal(t, "repo_clone", record["operation"])
	assert.Equal(t, phase, record["phase"])
	assert.Equal(t, true, record["primary"])
	assert.Equal(t, resultKind, record["result_kind"])
	assert.Equal(t, permission, record["permission_scope"])
	assert.Equal(t, sourceRole, record["source_role"])
	assert.Equal(t, destinationRole, record["destination_role"])
	assert.Equal(t, true, record["checked_for_this_operation"])
	assert.NotEmpty(t, record["source_path"])
	assert.NotEmpty(t, record["materialization_destination"])
	assert.NotEmpty(t, record["published_destination"])
	assert.Contains(t, []any{"fast_copy", "normal_copy"}, record["performance_class"])
}

func assertNoRepoCloneStaging(t *testing.T, dir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.clone-staging-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
