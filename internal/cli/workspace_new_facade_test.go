package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceNewCreatesRelativeExplicitFolderFromSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	targetFolder := filepath.Join(filepath.Dir(repoRoot), "exp")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID[:8])
	require.NoError(t, err)
	assertLineOrder(t, stdout, []string{
		"Folder: " + targetFolder,
		"Workspace: exp",
		"Started from save point: " + sourceID,
		"Newest save point: none",
		"Original workspace unchanged.",
	})
	assert.Contains(t, stdout, "Copy method: ")
	assert.Contains(t, stdout, "Checked for this operation")

	assertFileContent(t, filepath.Join(targetFolder, "app.txt"), "v1")
	cfg, err := worktree.NewManager(repoRoot).Get("exp")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.StartedFromSnapshotID)
	path, err := worktree.NewManager(repoRoot).Path("exp")
	require.NoError(t, err)
	assert.Equal(t, targetFolder, path)
}

func TestWorkspaceNewJSONIncludesPrimaryTransferForExplicitFolder(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	targetFolder := filepath.Join(t.TempDir(), "client-review-files")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "new", targetFolder, "--from", sourceID, "--name", "review")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "workspace new", env.Command)
	require.Equal(t, "review", data["workspace"])
	require.Equal(t, targetFolder, data["folder"])
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, transfers, 1)
	primary, ok := transfers[0].(map[string]any)
	require.True(t, ok, "primary transfer should be an object: %#v", transfers[0])
	require.Equal(t, "workspace-new-primary", primary["transfer_id"])
	require.Equal(t, "workspace_new", primary["operation"])
	require.Equal(t, "materialization", primary["phase"])
	require.Equal(t, true, primary["primary"])
	require.Equal(t, "final", primary["result_kind"])
	require.Equal(t, "execution", primary["permission_scope"])
	require.Equal(t, "save_point_payload", primary["source_role"])
	require.Equal(t, "workspace_folder", primary["destination_role"])
	require.NotEmpty(t, primary["source_path"])
	require.NotEmpty(t, primary["materialization_destination"])
	require.NotEmpty(t, primary["capability_probe_path"])
	require.Equal(t, targetFolder, primary["published_destination"])
	require.NotEqual(t, targetFolder, primary["materialization_destination"])
	require.Equal(t, filepath.Dir(targetFolder), primary["capability_probe_path"])
	require.Equal(t, true, primary["checked_for_this_operation"])
	require.Equal(t, "auto", primary["requested_engine"])
	require.NotEmpty(t, primary["effective_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	assertFileContent(t, filepath.Join(targetFolder, "app.txt"), "v1")
}

func TestWorkspaceNewCreatesAbsoluteExplicitFolderWithName(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	targetFolder := filepath.Join(t.TempDir(), "client-review-files")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", targetFolder, "--from", sourceID, "--name", "review")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Folder: "+targetFolder)
	assert.Contains(t, stdout, "Workspace: review")

	assertFileContent(t, filepath.Join(targetFolder, "app.txt"), "v1")
	cfg, err := worktree.NewManager(repoRoot).Get("review")
	require.NoError(t, err)
	assert.Equal(t, targetFolder, cfg.RealPath)
	path, err := worktree.NewManager(repoRoot).Path("review")
	require.NoError(t, err)
	assert.Equal(t, targetFolder, path)
}

func TestWorkspaceNewRejectsFolderInsideExistingWorkspace(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "./inside", "--from", sourceID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "inside an existing workspace")
	assert.NoDirExists(t, filepath.Join(repoRoot, "inside"))
}

func TestWorkspaceNewRejectsExistingFolder(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	targetFolder := filepath.Join(filepath.Dir(repoRoot), "existing")
	require.NoError(t, os.Mkdir(targetFolder, 0755))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", targetFolder, "--from", sourceID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "already exists")
}

func TestWorkspaceListShowsFoldersPointersWithoutDirtyScan(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.NoError(t, err)
	expFolder := filepath.Join(filepath.Dir(repoRoot), "exp")
	require.NoError(t, os.WriteFile(filepath.Join(expFolder, "app.txt"), []byte("unsaved edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "list")
	require.NoError(t, err)
	records := decodeWorkspaceListRecords(t, stdout)

	main := workspaceListRecordByName(t, records, "main")
	assert.Equal(t, true, main["current"])
	assert.Equal(t, repoRoot, main["folder"])
	assert.Equal(t, sourceID, main["content_source"])
	assert.Equal(t, sourceID, main["newest_save_point"])
	assert.Equal(t, sourceID, main["history_head"])
	assert.NotContains(t, main, "unsaved_changes")

	exp := workspaceListRecordByName(t, records, "exp")
	assert.Equal(t, false, exp["current"])
	assert.Equal(t, expFolder, exp["folder"])
	assert.Equal(t, sourceID, exp["content_source"])
	assert.Equal(t, sourceID, exp["started_from_save_point"])
	assert.NotContains(t, exp, "unsaved_changes")
}

func TestWorkspaceListStatusShowsUnsavedChanges(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.NoError(t, err)
	expFolder := filepath.Join(filepath.Dir(repoRoot), "exp")
	require.NoError(t, os.WriteFile(filepath.Join(expFolder, "app.txt"), []byte("unsaved edit"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "list", "--status")
	require.NoError(t, err)
	records := decodeWorkspaceListRecords(t, stdout)

	main := workspaceListRecordByName(t, records, "main")
	assert.Equal(t, false, main["unsaved_changes"])
	exp := workspaceListRecordByName(t, records, "exp")
	assert.Equal(t, true, exp["unsaved_changes"])
}

func TestWorkspaceNewFromSavePointCreatesIndependentStartedWorkspace(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("main changed"), 0644))
	sourcePrefix := sourceID[:8]

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourcePrefix)
	require.NoError(t, err)
	assertLineOrder(t, stdout, []string{
		"Folder: ",
		"Workspace: exp",
		"Started from save point: " + sourceID,
		"Newest save point: none",
		"Original workspace unchanged.",
	})
	assertWorkspaceNewOutputOmitsOldVocabulary(t, stdout)

	mainContent, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "main changed", string(mainContent))
	mainCfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), mainCfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(sourceID), mainCfg.LatestSnapshotID)

	cfg, err := worktree.NewManager(repoRoot).Get("exp")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
	assert.Empty(t, cfg.BaseSnapshotID)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.StartedFromSnapshotID)
	assert.Empty(t, cfg.PathSources)

	expPath, err := worktree.NewManager(repoRoot).Path("exp")
	require.NoError(t, err)
	expContent, err := os.ReadFile(filepath.Join(expPath, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(expContent))
}

func TestWorkspaceNewStatusHistoryAndFirstSaveUseStartedFromLineage(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	_, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.NoError(t, err)

	statusOut, err := executeCommand(createTestRootCmd(), "--workspace", "exp", "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, "Workspace: exp")
	assert.Contains(t, statusOut, "Newest save point: none")
	assert.Contains(t, statusOut, "Started from save point: "+sourceID)
	assert.Contains(t, statusOut, "Unsaved changes: no")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, statusOut)

	statusJSON, err := executeCommand(createTestRootCmd(), "--json", "--workspace", "exp", "status")
	require.NoError(t, err)
	_, statusData := decodeFacadeDataMap(t, statusJSON)
	assert.Nil(t, statusData["newest_save_point"])
	assert.Nil(t, statusData["history_head"])
	assert.Equal(t, sourceID, statusData["content_source"])
	assert.Equal(t, sourceID, statusData["started_from_save_point"])
	assert.Equal(t, false, statusData["unsaved_changes"])
	assert.Equal(t, "started_from_save_point", statusData["files_state"])

	historyOut, err := executeCommand(createTestRootCmd(), "--workspace", "exp", "history")
	require.NoError(t, err)
	assert.NotContains(t, historyOut, "No save points yet.")
	assert.Contains(t, historyOut, "Workspace started from "+sourceID)
	assert.Contains(t, historyOut, "Current pointer: "+sourceID)
	assert.Contains(t, historyOut, "Workspace has not created its own save point yet.")
	assert.Contains(t, historyOut, "source")
	assertNoCheckpointSnapshotWorktreeVocabulary(t, historyOut)

	saveOut, err := executeCommand(createTestRootCmd(), "--workspace", "exp", "--json", "save", "-m", "exp first")
	require.NoError(t, err)
	_, saveData := decodeFacadeDataMap(t, saveOut)
	firstID := model.SnapshotID(saveData["save_point_id"].(string))
	assert.Equal(t, sourceID, saveData["started_from_save_point"])
	desc, err := snapshot.LoadDescriptor(repoRoot, firstID)
	require.NoError(t, err)
	assert.Nil(t, desc.ParentID)
	require.NotNil(t, desc.StartedFrom)
	assert.Equal(t, model.SnapshotID(sourceID), *desc.StartedFrom)
	assert.Nil(t, desc.RestoredFrom)

	historyJSON, err := executeCommand(createTestRootCmd(), "--json", "--workspace", "exp", "history")
	require.NoError(t, err)
	_, historyData := decodeFacadeDataMap(t, historyJSON)
	assert.Equal(t, string(firstID), historyData["newest_save_point"])
	points := historyData["save_points"].([]any)
	require.Len(t, points, 1)
	assert.Equal(t, string(firstID), points[0].(map[string]any)["save_point_id"])

	historyAfterSave, err := executeCommand(createTestRootCmd(), "--workspace", "exp", "history")
	require.NoError(t, err)
	assert.Contains(t, historyAfterSave, "Workspace started from "+sourceID)
	assert.Contains(t, historyAfterSave, "exp first")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, historyAfterSave)

	humanRepoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(humanRepoRoot, "app.txt"), []byte("v1"), 0644))
	humanSourceID := savePointIDFromCLI(t, "source")
	_, err = executeCommand(createTestRootCmd(), "workspace", "new", "../humanexp", "--from", humanSourceID)
	require.NoError(t, err)
	humanSaveOut, err := executeCommand(createTestRootCmd(), "--workspace", "humanexp", "save", "-m", "exp first")
	require.NoError(t, err)
	assert.Contains(t, humanSaveOut, "Started from save point "+humanSourceID)
	assertWorkspaceNewOutputOmitsOldVocabulary(t, humanSaveOut)
}

func TestWorkspaceNewFromSavePointFailureDoesNotCreateWorkspace(t *testing.T) {
	t.Run("existing workspace", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		sourceID := savePointIDFromCLI(t, "source")

		stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../main", "--from", sourceID)
		require.Error(t, err)
		require.Empty(t, stdout)
		assertWorkspaceNewOutputOmitsOldVocabulary(t, err.Error())
		_, loadErr := worktree.NewManager(repoRoot).Get("main")
		require.NoError(t, loadErr)
	})

	t.Run("invalid save point ref", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)

		stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", "notasave")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "save point ID")
		assertWorkspaceNewOutputOmitsOldVocabulary(t, err.Error())
		assertWorkspaceMissing(t, repoRoot, "exp")
	})

	t.Run("damaged source", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		sourceID := savePointIDFromCLI(t, "source")
		require.NoError(t, os.Remove(filepath.Join(repoRoot, repo.JVSDirName, "snapshots", sourceID, ".READY")))

		stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
		require.Error(t, err)
		require.Empty(t, stdout)
		assertWorkspaceNewOutputOmitsOldVocabulary(t, err.Error())
		assertWorkspaceMissing(t, repoRoot, "exp")
	})
}

func TestWorkspaceNewCapacityFailureDoesNotCreateWorkspaceOrPin(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("main changed"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)
	beforeMainContent, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	beforeMainCfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	meter := &workspaceNewCapacityPinProbeMeter{t: t, repoRoot: repoRoot, beforePins: before.pins}
	restoreCapacity := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restoreCapacity)

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Greater(t, meter.checks, 0)
	assert.True(t, meter.observedActivePin, "workspace new capacity inspection should run while source is actively protected")
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No workspace was created.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, err.Error())
	assertWorkspaceMissing(t, repoRoot, "exp")
	assertWorkspaceNewNoStaging(t, repoRoot, "exp")
	assert.Equal(t, before.pins, documentedPinCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
	afterMainContent, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, beforeMainContent, afterMainContent)
	afterMainCfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, beforeMainCfg, afterMainCfg)

	t.Setenv(testCapacityAvailableEnv, "0")
	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "workspace", "new", "../jsonexp", "--from", sourceID)
	require.NotZero(t, exitCode, stderr)
	env := decodeContractEnvelope(t, jsonOut)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "No workspace was created.")
	assert.Contains(t, env.Error.Message, "No files were changed.")
	assertWorkspaceMissing(t, repoRoot, "jsonexp")
	assertWorkspaceNewNoStaging(t, repoRoot, "jsonexp")
	assert.Equal(t, before.pins, documentedPinCount(t, repoRoot))
}

func TestWorkspaceNewCapacityIncludesSourceHashTempFilesystem(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	sourceID := savePointIDFromCLI(t, "source")
	missingTemp := useMissingTempDir(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	meter := &workspaceNewPathCapacityPinProbeMeter{
		cliPathCapacityMeter: cliPathCapacityMeter{
			repoRoot:      repoRoot,
			tempRoot:      missingTemp,
			siblingPrefix: filepath.Dir(repoRoot),
			availableByDevice: map[string]int64{
				"repo-fs":    100 << 20,
				"sibling-fs": 100 << 20,
				"temp-fs":    0,
			},
		},
		t:          t,
		repoRoot:   repoRoot,
		beforePins: before.pins,
	}
	restoreCapacity := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 1})
	t.Cleanup(restoreCapacity)

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", sourceID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "capacity failure should preserve structured error: %v", err)
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", jvsErr.Code)
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No workspace was created.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(missingTemp))
	assert.True(t, meter.observedActivePin, "source hash capacity check should hold active source protection")
	assertWorkspaceMissing(t, repoRoot, "exp")
	assertWorkspaceNewNoStaging(t, repoRoot, "exp")
	assert.Equal(t, before.pins, documentedPinCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

type workspaceNewCapacityPinProbeMeter struct {
	t                 *testing.T
	repoRoot          string
	beforePins        int
	checks            int
	observedActivePin bool
}

func (m *workspaceNewCapacityPinProbeMeter) AvailableBytes(path string) (int64, error) {
	m.checks++
	if assert.Equal(m.t, m.beforePins+1, documentedPinCount(m.t, m.repoRoot), "capacity check must hold active source protection") {
		m.observedActivePin = true
	}
	return 0, nil
}

type workspaceNewPathCapacityPinProbeMeter struct {
	cliPathCapacityMeter
	t                 *testing.T
	repoRoot          string
	beforePins        int
	observedActivePin bool
}

func (m *workspaceNewPathCapacityPinProbeMeter) AvailableBytes(path string) (int64, error) {
	if assert.Equal(m.t, m.beforePins+1, documentedPinCount(m.t, m.repoRoot), "capacity check must hold active source protection") {
		m.observedActivePin = true
	}
	return m.cliPathCapacityMeter.AvailableBytes(path)
}

func TestWorkspaceNewRejectsTaintedSourcePayloadBeforePublish(t *testing.T) {
	repoRoot := setupContainerWorkspaceNewRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, repo.JVSDirName), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, repo.JVSDirName, "format_version"), []byte("tainted"), 0644))
	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "tainted source", nil)
	require.NoError(t, err)
	beforeCatalog := savePointCatalogCount(t, repoRoot)
	beforeDescriptors := descriptorFileCount(t, repoRoot)
	beforeMainCfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)

	require.NoError(t, os.Chdir(repoRoot))
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../exp", "--from", desc.SnapshotID.String())
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "control data")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, err.Error())
	assertWorkspaceMissing(t, repoRoot, "exp")
	assertWorkspaceNewNoStaging(t, repoRoot, "exp")
	assert.Equal(t, beforeCatalog, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, beforeDescriptors, descriptorFileCount(t, repoRoot))
	afterMainCfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, beforeMainCfg, afterMainCfg)
}

func TestWorkspacePublicDiscoveryUsesCleanVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)

	workspaceHelp, err := executeCommand(createTestRootCmd(), "workspace", "--help")
	require.NoError(t, err)
	assert.Contains(t, workspaceHelp, "new")
	assert.NotContains(t, workspaceHelp, "  list")
	assert.NotContains(t, workspaceHelp, "  path")
	assert.NotContains(t, workspaceHelp, "  rename")
	assert.NotContains(t, workspaceHelp, "  remove")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, workspaceHelp)

	removeHelp, err := executeCommand(createTestRootCmd(), "workspace", "remove", "--help")
	require.NoError(t, err)
	assertWorkspaceNewOutputOmitsOldVocabulary(t, removeHelp)

	listJSON, err := executeCommand(createTestRootCmd(), "--json", "workspace", "list")
	require.NoError(t, err)
	assert.NotContains(t, listJSON, "base_checkpoint")
	assert.NotContains(t, listJSON, "latest")
	assert.Contains(t, listJSON, "current")
	assert.Contains(t, listJSON, "history_head")

	_, err = worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
}

func TestWorkspaceNewHelpUsesSavePointVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "save point")
	assertWorkspaceNewOutputOmitsOldVocabulary(t, stdout)
}

func assertWorkspaceMissing(t *testing.T, repoRoot, name string) {
	t.Helper()
	_, err := worktree.NewManager(repoRoot).Get(name)
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(repoRoot, repo.JVSDirName, "worktrees", name))
	assert.NoDirExists(t, filepath.Join(repoRoot, name))
	assert.NoDirExists(t, filepath.Join(repoRoot, "worktrees", name))
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repoRoot), name))
}

func assertWorkspaceNewNoStaging(t *testing.T, repoRoot, name string) {
	t.Helper()
	for _, pattern := range []string{
		filepath.Join(repoRoot, "."+name+".staging-*"),
		filepath.Join(repoRoot, "worktrees", "."+name+".staging-*"),
		filepath.Join(filepath.Dir(repoRoot), "."+name+".staging-*"),
	} {
		matches, err := filepath.Glob(pattern)
		require.NoError(t, err)
		assert.Empty(t, matches, "unexpected staging matches for %s", pattern)
	}
}

func setupContainerWorkspaceNewRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	_, err = repo.Init(repoRoot, "test")
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))
	return repoRoot
}

func assertLineOrder(t *testing.T, value string, lines []string) {
	t.Helper()
	last := -1
	for _, line := range lines {
		idx := strings.Index(value, line)
		require.NotEqualf(t, -1, idx, "missing line %q in output:\n%s", line, value)
		assert.Greaterf(t, idx, last, "line %q appeared out of order in output:\n%s", line, value)
		last = idx
	}
}

func assertWorkspaceNewOutputOmitsOldVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree", "latest", "detached", "fork", "commit"} {
		assert.NotContains(t, lower, word)
	}
}

func decodeWorkspaceListRecords(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var records []map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &records), stdout)
	return records
}

func workspaceListRecordByName(t *testing.T, records []map[string]any, name string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["workspace"] == name {
			return record
		}
	}
	t.Fatalf("workspace %q not found in %#v", name, records)
	return nil
}
