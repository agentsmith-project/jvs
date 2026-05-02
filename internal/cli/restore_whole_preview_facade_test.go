package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreWholePreviewDoesNotMutateAndWritesPlan(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, "Preview only. No files were changed.", lines[0])
	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Plan: ")
	assert.Contains(t, stdout, "Source save point: "+firstID)
	assert.Contains(t, stdout, "Managed files to overwrite: 1")
	assert.Contains(t, stdout, "app.txt")
	assert.Contains(t, stdout, "Managed files to delete: 1")
	assert.Contains(t, stdout, "workspace-only.txt")
	assert.Contains(t, stdout, "Managed files to create: 1")
	assert.Contains(t, stdout, "only-source.txt")
	assert.Contains(t, stdout, "JVS control data and runtime state are not user files; restore leaves them alone.")
	assert.Contains(t, stdout, "History will not change.")
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "You can return to save point "+secondID+".")
	assert.Contains(t, stdout, "Expected newest save point: "+secondID)
	assert.Contains(t, stdout, "Expected folder evidence: ")
	planID := restorePlanIDFromHumanOutput(t, stdout)
	assert.Contains(t, stdout, "Run: `jvs restore --run "+planID+"`")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
	assertRestorePlanFileExists(t, repoRoot, planID)

	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "workspace-only.txt"), "workspace")
	require.NoFileExists(t, filepath.Join(repoRoot, "only-source.txt"))
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholePreviewJSONUsesCleanSchema(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "preview", data["mode"])
	require.NotEmpty(t, data["plan_id"])
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, firstID, data["source_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, secondID, data["expected_newest_save_point"])
	require.NotEmpty(t, data["expected_folder_evidence"])
	require.Equal(t, false, data["history_changed"])
	require.Equal(t, false, data["files_changed"])
	require.Equal(t, "jvs restore --run "+data["plan_id"].(string), data["run_command"])
	assertRestoreExpectedPreviewTransfer(t, data, repoRoot, firstID)
	assertRestorePreviewImpact(t, data, "overwrite", 1, "app.txt")
	assertRestorePreviewImpact(t, data, "delete", 1, "workspace-only.txt")
	assertRestorePreviewImpact(t, data, "create", 1, "only-source.txt")
	assertRestoreJSONOmitsLegacyFields(t, data)
	assert.NotContains(t, data, "restored_save_point")
	assert.NotContains(t, data, "content_source")
	assertRestorePlanFileExists(t, repoRoot, data["plan_id"].(string))
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholeRunExecutesBoundPlanAndKeepsHistory(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Plan: "+planID)
	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, stdout, "Managed files now match save point "+firstID+".")
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "History was not changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, stdout)
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
	assertFileContent(t, filepath.Join(repoRoot, "only-source.txt"), "source")
	require.NoFileExists(t, filepath.Join(repoRoot, "workspace-only.txt"))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)

	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	_, statusData := decodeFacadeDataMap(t, statusOut)
	require.Equal(t, firstID, statusData["content_source"])
	require.Equal(t, secondID, statusData["history_head"])
	require.Equal(t, false, statusData["unsaved_changes"])
}

func TestRestoreWholeRunJSONUsesCleanSchema(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, planID, data["plan_id"])
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, firstID, data["source_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["history_changed"])
	require.Equal(t, true, data["files_changed"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, "matches_save_point", data["files_state"])
	assertRestoreFinalRunTransfer(t, data, repoRoot, firstID)
	assertRestoreJSONOmitsLegacyFields(t, data)
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
}

func TestRestoreWholeRunReplansInsteadOfUsingPreviewTransfer(t *testing.T) {
	repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)

	plan, err := restoreplan.Load(repoRoot, planID)
	require.NoError(t, err)
	plan.Transfers = []transfer.Record{{
		TransferID:                 "restore-preview-validation-primary",
		Operation:                  "restore",
		Phase:                      "preview_validation",
		Primary:                    true,
		ResultKind:                 transfer.ResultKindExpected,
		PermissionScope:            transfer.PermissionScopePreviewOnly,
		SourceRole:                 "save_point_payload",
		SourcePath:                 filepath.Join(repoRoot, ".jvs", "snapshots", firstID),
		DestinationRole:            "restore_preview_validation",
		MaterializationDestination: "/preview-only-materialization",
		CapabilityProbePath:        "/preview-only-probe",
		PublishedDestination:       repoRoot,
		CheckedForThisOperation:    true,
		RequestedEngine:            engine.EngineAuto,
		EffectiveEngine:            model.EngineReflinkCopy,
		OptimizedTransfer:          true,
		PerformanceClass:           transfer.PerformanceClassFastCopy,
		DegradedReasons:            []string{},
		Warnings:                   []string{},
	}}
	require.NoError(t, restoreplan.Write(repoRoot, plan))

	t.Setenv("JVS_SNAPSHOT_ENGINE", "copy")
	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)

	_, data := decodeFacadeDataMap(t, stdout)
	_, primary := assertRestoreRunTransfersDestination(t, data, repoRoot, firstID, repoRoot, "restore-run-primary", ".restore-tmp-")
	require.Equal(t, "copy", primary["requested_engine"])
	require.Equal(t, "copy", primary["effective_engine"])
	require.Equal(t, "normal_copy", primary["performance_class"])
	require.NotEqual(t, "/preview-only-materialization", primary["materialization_destination"])
	require.NotEqual(t, "/preview-only-probe", primary["capability_probe_path"])
}

func TestRestoreWholePreviewAndRunHumanOutputShowTransferMethod(t *testing.T) {
	repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	assert.Contains(t, previewOut, "Expected copy method: ")
	assert.Contains(t, previewOut, "Checked for this preview")
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	runOut, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)
	copyMethodIndex := strings.Index(runOut, "Copy method: ")
	additionalIndex := strings.Index(runOut, "Additional transfers: 1 source validation; see JSON for details")
	require.NotEqual(t, -1, copyMethodIndex, runOut)
	require.NotEqual(t, -1, additionalIndex, runOut)
	require.Less(t, copyMethodIndex, additionalIndex, runOut)
	assert.Contains(t, runOut, "Checked for this operation")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
}

func TestRestoreRunPrimaryTransferPrefersRestoreMaterialization(t *testing.T) {
	transfers := []transfer.Record{
		{
			TransferID:       "save-primary",
			Operation:        "save",
			Phase:            "materialization",
			Primary:          true,
			PerformanceClass: transfer.PerformanceClassFastCopy,
		},
		{
			TransferID:       "restore-run-primary",
			Operation:        "restore",
			Phase:            "materialization",
			Primary:          true,
			PerformanceClass: transfer.PerformanceClassNormalCopy,
		},
	}

	primary := restoreRunPrimaryTransfer(transfers)

	require.NotNil(t, primary)
	require.Equal(t, "restore-run-primary", primary.TransferID)
	require.Equal(t, transfer.PerformanceClassNormalCopy, primary.PerformanceClass)
}

func TestRestoreWholeRunRejectsChangedNewestWithoutMutation(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	before := captureViewMutationSnapshot(t, repoRoot)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v3"), 0644))
	thirdID := savePointIDFromCLI(t, "third")

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "folder changed since preview")
	assert.Contains(t, err.Error(), "run preview again")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v3")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(thirdID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(thirdID), cfg.LatestSnapshotID)
	require.NotEqual(t, secondID, thirdID)
	_ = before
}

func TestRestoreWholeRunRejectsChangedFolderEvidenceWithoutMutation(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "folder changed since preview")
	assert.Contains(t, err.Error(), "run preview again")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreDirtyFolderShowsDecisionPreviewWithoutPlan(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	plansBefore := restorePlanFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.NotEmpty(t, lines)
	assert.Equal(t, "Preview only. No files were changed.", lines[0])
	assert.Contains(t, stdout, "Decision needed: folder has unsaved changes.")
	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.NotContains(t, stdout, "Plan: ")
	assert.Contains(t, stdout, "Source save point: "+firstID)
	assert.Contains(t, stdout, "Managed files to overwrite: 1")
	assert.Contains(t, stdout, "app.txt")
	assert.Contains(t, stdout, "Managed files to delete: 1")
	assert.Contains(t, stdout, "workspace-only.txt")
	assert.Contains(t, stdout, "Managed files to create: 1")
	assert.Contains(t, stdout, "only-source.txt")
	assert.Contains(t, stdout, "History will not change.")
	assert.Contains(t, stdout, "Newest save point is still "+secondID+".")
	assert.Contains(t, stdout, "Expected folder evidence: ")
	assert.Contains(t, stdout, "Rerun preview with one safety option:")
	assert.Contains(t, stdout, "jvs restore "+firstID+" --save-first")
	assert.Contains(t, stdout, "jvs restore "+firstID+" --discard-unsaved")
	assert.NotContains(t, stdout, "Run: `jvs restore --run")
	assertRestoreOutputOmitsLegacyVocabulary(t, strings.ReplaceAll(stdout, repoRoot, ""))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
}

func TestRestoreDirtyDecisionPreviewDoesNotMutateFilesOrHistory(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "restore", env.Command)
	require.Equal(t, "decision_preview", data["mode"])
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, firstID, data["source_save_point"])
	require.Equal(t, secondID, data["newest_save_point"])
	require.Equal(t, secondID, data["history_head"])
	require.Equal(t, secondID, data["expected_newest_save_point"])
	require.NotEmpty(t, data["expected_folder_evidence"])
	require.Equal(t, false, data["history_changed"])
	require.Equal(t, false, data["files_changed"])
	require.NotContains(t, data, "plan_id")
	require.NotContains(t, data, "run_command")
	nextCommands, ok := data["next_commands"].([]any)
	require.True(t, ok, "next_commands should be an array: %#v", data["next_commands"])
	assert.Contains(t, nextCommands, "jvs restore "+firstID+" --save-first")
	assert.Contains(t, nextCommands, "jvs restore "+firstID+" --discard-unsaved")
	assertRestorePreviewImpact(t, data, "overwrite", 1, "app.txt")
	assertRestoreJSONOmitsLegacyFields(t, data)
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreDirtyDecisionPreviewPlanCannotRun(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))

	decisionPlan, err := restoreplan.CreateDecisionPreview(repoRoot, "main", model.SnapshotID(firstID), detectEngine(repoRoot))
	require.NoError(t, err)
	decisionPlan.PlanID = "decision-preview"
	decisionPlan.RunCommand = ""
	require.NoError(t, restoreplan.Write(repoRoot, decisionPlan))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", "decision-preview")
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "requires a safety decision")
	assert.Contains(t, err.Error(), "--save-first")
	assert.Contains(t, err.Error(), "--discard-unsaved")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholeDiscardUnsavedPreviewThenRun(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))

	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-unsaved")
	require.NoError(t, err)
	assert.Contains(t, previewOut, "Preview only. No files were changed.")
	assert.Contains(t, previewOut, "Run options: discard unsaved changes")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)
	_, data := decodeFacadeDataMap(t, stdout)
	require.Equal(t, "run", data["mode"])
	require.Equal(t, firstID, data["restored_save_point"])
	require.Equal(t, firstID, data["content_source"])
	require.Equal(t, false, data["unsaved_changes"])
	require.Equal(t, false, data["history_changed"])
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
}

func TestRestoreWholeSaveFirstPreviewThenRunCreatesSafetySaveThenRestores(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	plansBefore := restorePlanFileCount(t, repoRoot)

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--save-first")
	require.NoError(t, err)
	_, previewData := decodeFacadeDataMap(t, previewOut)
	planID := previewData["plan_id"].(string)
	previewOptions, ok := previewData["options"].(map[string]any)
	require.True(t, ok, "preview options should be exposed: %#v", previewData["options"])
	require.Equal(t, true, previewOptions["save_first"])
	require.NotContains(t, previewOptions, "discard_unsaved")
	require.Equal(t, secondID, previewData["expected_newest_save_point"])
	require.Equal(t, plansBefore+1, restorePlanFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	historyBeforeRun, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	_, historyBeforeRunData := decodeFacadeDataMap(t, historyBeforeRun)
	require.Len(t, historyBeforeRunData["save_points"].([]any), 2)

	runOut, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err)
	_, runData := decodeFacadeDataMap(t, runOut)
	newest, ok := runData["newest_save_point"].(string)
	require.True(t, ok)
	require.NotEmpty(t, newest)
	require.NotEqual(t, secondID, newest)
	require.Equal(t, newest, runData["history_head"])
	require.Equal(t, firstID, runData["restored_save_point"])
	require.Equal(t, firstID, runData["content_source"])
	require.Equal(t, false, runData["history_changed"])
	require.Equal(t, false, runData["unsaved_changes"])
	assertSaveFirstRestoreRunTransfers(t, runData, repoRoot, firstID, repoRoot, "restore-run-primary", ".restore-tmp-")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")

	historyOut, err := executeCommand(createTestRootCmd(), "history", "to", newest)
	require.NoError(t, err)
	assert.Contains(t, historyOut, "save before restore")

	safetyDesc, err := snapshot.LoadDescriptor(repoRoot, model.SnapshotID(newest))
	require.NoError(t, err)
	require.NotNil(t, safetyDesc.ParentID)
	require.Equal(t, model.SnapshotID(secondID), *safetyDesc.ParentID)
}

func TestRestoreWholeSaveFirstRunValidatesSourceBeforeSafetySave(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--save-first")
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".jvs", "snapshots", firstID, "app.txt"), []byte("tainted source"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "source save point is not restorable")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	require.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholeRunRejectsRuntimeBehaviorFlagsWithoutMutation(t *testing.T) {
	t.Run("discard preview then save-first run", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-unsaved")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := captureViewMutationSnapshot(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID, "--save-first")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "options are fixed by the preview plan")
		assert.Contains(t, err.Error(), "No files were changed.")
		assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
		before.assertUnchanged(t, repoRoot)
	})

	t.Run("save-first preview then discard run json", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--save-first")
		require.NoError(t, err)
		_, previewData := decodeFacadeDataMap(t, previewOut)
		planID := previewData["plan_id"].(string)
		before := captureViewMutationSnapshot(t, repoRoot)

		jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", "--run", planID, "--discard-unsaved")
		require.NotZero(t, exitCode)
		require.Empty(t, strings.TrimSpace(stderr))
		env := decodeContractEnvelope(t, jsonOut)
		require.False(t, env.OK, jsonOut)
		require.NotNil(t, env.Error)
		assert.Contains(t, env.Error.Message, "options are fixed by the preview plan")
		assert.Contains(t, env.Error.Message, "No files were changed.")
		assertRestoreOutputOmitsLegacyVocabulary(t, env.Error.Message)
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
		before.assertUnchanged(t, repoRoot)
	})

	t.Run("removed legacy save flag is unknown at run time", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-unsaved")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := captureViewMutationSnapshot(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID, "--include-working")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "unknown flag: --include-working")
		assert.NotContains(t, err.Error(), "options are fixed by the preview plan")
		assert.NotContains(t, err.Error(), "--save-first")
		assert.NotContains(t, err.Error(), "deprecated")
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
		before.assertUnchanged(t, repoRoot)
	})

	t.Run("removed legacy discard flag is unknown at run time", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--discard-unsaved")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := captureViewMutationSnapshot(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID, "--discard-dirty")
		require.Error(t, err)
		require.Empty(t, stdout)
		assert.Contains(t, err.Error(), "unknown flag: --discard-dirty")
		assert.NotContains(t, err.Error(), "options are fixed by the preview plan")
		assert.NotContains(t, err.Error(), "--discard-unsaved")
		assert.NotContains(t, err.Error(), "deprecated")
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
		before.assertUnchanged(t, repoRoot)
	})

	t.Run("removed legacy save flag is unknown in json", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID, "--save-first")
		require.NoError(t, err)
		_, previewData := decodeFacadeDataMap(t, previewOut)
		planID := previewData["plan_id"].(string)
		before := captureViewMutationSnapshot(t, repoRoot)

		jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", "--run", planID, "--include-working")
		require.NotZero(t, exitCode)
		require.Empty(t, strings.TrimSpace(stderr))
		env := decodeContractEnvelope(t, jsonOut)
		require.False(t, env.OK, jsonOut)
		require.NotNil(t, env.Error)
		assert.Contains(t, env.Error.Message, "unknown flag: --include-working")
		assert.NotContains(t, env.Error.Message, "options are fixed by the preview plan")
		assert.NotContains(t, env.Error.Message, "--save-first")
		assert.NotContains(t, env.Error.Message, "deprecated")
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
		before.assertUnchanged(t, repoRoot)
	})
}

func TestRestorePathUsesPreviewBeforeRun(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)

	planID := assertPathRestorePreviewHuman(t, stdout, repoRoot, firstID, secondID, "app.txt")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")

	runOut, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)
	assert.Contains(t, runOut, "Plan: "+planID)
	assert.Contains(t, runOut, "Restored path: app.txt")
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
}

func setupWholeRestoreImpactRepo(t *testing.T) (repoRoot, firstID, secondID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "only-source.txt"), []byte("source"), 0644))
	firstID = savePointIDFromCLI(t, "first")

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.Remove(filepath.Join(repoRoot, "only-source.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "workspace-only.txt"), []byte("workspace"), 0644))
	secondID = savePointIDFromCLI(t, "second")
	return repoRoot, firstID, secondID
}

func restorePlanIDFromHumanOutput(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Plan: ") {
			planID := strings.TrimSpace(strings.TrimPrefix(line, "Plan: "))
			require.NotEmpty(t, planID)
			return planID
		}
	}
	t.Fatalf("restore preview output did not include Plan line:\n%s", stdout)
	return ""
}

func assertRestorePreviewImpact(t *testing.T, data map[string]any, kind string, count int, sample string) {
	t.Helper()
	managed, ok := data["managed_files"].(map[string]any)
	require.True(t, ok, "managed_files should be an object: %#v", data["managed_files"])
	rawImpact, ok := managed[kind].(map[string]any)
	require.True(t, ok, "%s impact should be an object: %#v", kind, managed[kind])
	require.EqualValues(t, count, rawImpact["count"])
	samples, ok := rawImpact["samples"].([]any)
	require.True(t, ok, "%s samples should be an array: %#v", kind, rawImpact["samples"])
	assert.Contains(t, samples, sample)
}

func assertRestoreExpectedPreviewTransfer(t *testing.T, data map[string]any, repoRoot, sourceID string) {
	t.Helper()
	assertRestoreExpectedPreviewTransferDestination(t, data, repoRoot, sourceID, repoRoot)
}

func assertRestoreExpectedPreviewTransferDestination(t *testing.T, data map[string]any, repoRoot, sourceID, publishedDestination string) {
	t.Helper()
	primary := assertSingleRestoreTransfer(t, data)
	require.Equal(t, "restore-preview-validation-primary", primary["transfer_id"])
	require.Equal(t, "restore", primary["operation"])
	require.Equal(t, "preview_validation", primary["phase"])
	require.Equal(t, true, primary["primary"])
	require.Equal(t, "expected", primary["result_kind"])
	require.Equal(t, "preview_only", primary["permission_scope"])
	require.Equal(t, "save_point_payload", primary["source_role"])
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", sourceID), primary["source_path"])
	require.Equal(t, "restore_preview_validation", primary["destination_role"])
	require.Contains(t, primary["materialization_destination"], filepath.Join(repoRoot, ".jvs", "restore-preview-"))
	require.NotEqual(t, repoRoot, primary["materialization_destination"])
	require.NotEmpty(t, primary["capability_probe_path"])
	require.Equal(t, publishedDestination, primary["published_destination"])
	require.Equal(t, true, primary["checked_for_this_operation"])
	require.Contains(t, []any{"auto", "copy"}, primary["requested_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	require.IsType(t, []any{}, primary["degraded_reasons"])
	require.IsType(t, []any{}, primary["warnings"])
}

func assertRestoreFinalRunTransfer(t *testing.T, data map[string]any, repoRoot, sourceID string) {
	t.Helper()
	assertRestoreRunTransfersDestination(t, data, repoRoot, sourceID, repoRoot, "restore-run-primary", ".restore-tmp-")
}

func assertRestorePathFinalRunTransfer(t *testing.T, data map[string]any, repoRoot, sourceID, path string) {
	t.Helper()
	assertRestoreRunTransfersDestination(t, data, repoRoot, sourceID, filepath.Join(repoRoot, filepath.FromSlash(path)), "restore-path-run-primary", ".restore-path-tmp-")
}

func assertRestoreRunTransfersDestination(t *testing.T, data map[string]any, repoRoot, sourceID, publishedDestination, transferID, tempMarker string) (map[string]any, map[string]any) {
	t.Helper()
	transfers := assertRestoreTransfers(t, data, 2)
	validation, ok := transfers[0].(map[string]any)
	require.True(t, ok, "source validation transfer should be an object: %#v", transfers[0])
	assertRestoreRunSourceValidationTransfer(t, validation, repoRoot, sourceID, publishedDestination)
	primary, ok := transfers[1].(map[string]any)
	require.True(t, ok, "restore transfer should be an object: %#v", transfers[1])
	assertRestoreFinalRunTransferRecord(t, primary, repoRoot, sourceID, publishedDestination, transferID, tempMarker)
	return validation, primary
}

func assertRestoreRunSourceValidationTransfer(t *testing.T, record map[string]any, repoRoot, sourceID, publishedDestination string) {
	t.Helper()
	require.Equal(t, "restore-run-source-validation", record["transfer_id"])
	require.Equal(t, "restore", record["operation"])
	require.Equal(t, "source_validation", record["phase"])
	require.Equal(t, false, record["primary"])
	require.Equal(t, "final", record["result_kind"])
	require.Equal(t, "execution", record["permission_scope"])
	require.Equal(t, "save_point_payload", record["source_role"])
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", sourceID), record["source_path"])
	require.Equal(t, "restore_source_validation", record["destination_role"])
	require.Contains(t, record["materialization_destination"], filepath.Join(repoRoot, ".jvs", "restore-run-validation-"))
	require.NotEqual(t, repoRoot, record["materialization_destination"])
	require.NotEmpty(t, record["capability_probe_path"])
	require.Equal(t, publishedDestination, record["published_destination"])
	require.Equal(t, true, record["checked_for_this_operation"])
	require.Contains(t, []any{"auto", "copy"}, record["requested_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, record["performance_class"])
	require.IsType(t, []any{}, record["degraded_reasons"])
	require.IsType(t, []any{}, record["warnings"])
}

func assertRestoreFinalRunTransferRecord(t *testing.T, primary map[string]any, repoRoot, sourceID, publishedDestination, transferID, tempMarker string) {
	t.Helper()
	require.Equal(t, transferID, primary["transfer_id"])
	require.Equal(t, "restore", primary["operation"])
	require.Equal(t, "materialization", primary["phase"])
	require.Equal(t, true, primary["primary"])
	require.Equal(t, "final", primary["result_kind"])
	require.Equal(t, "execution", primary["permission_scope"])
	require.Equal(t, "save_point_payload", primary["source_role"])
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", sourceID), primary["source_path"])
	require.Equal(t, "restore_staging", primary["destination_role"])
	require.Contains(t, primary["materialization_destination"], tempMarker)
	require.NotEqual(t, repoRoot, primary["materialization_destination"])
	require.Equal(t, filepath.Dir(repoRoot), primary["capability_probe_path"])
	require.Equal(t, publishedDestination, primary["published_destination"])
	require.Equal(t, true, primary["checked_for_this_operation"])
	require.Contains(t, []any{"auto", "copy"}, primary["requested_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	require.IsType(t, []any{}, primary["degraded_reasons"])
	require.IsType(t, []any{}, primary["warnings"])
}

func assertSaveFirstRestoreRunTransfers(t *testing.T, data map[string]any, repoRoot, sourceID, publishedDestination, restoreTransferID, tempMarker string) {
	t.Helper()
	transfers := assertRestoreTransfers(t, data, 3)
	validation, ok := transfers[0].(map[string]any)
	require.True(t, ok, "source validation transfer should be an object: %#v", transfers[0])
	assertRestoreRunSourceValidationTransfer(t, validation, repoRoot, sourceID, publishedDestination)

	safetySave, ok := transfers[1].(map[string]any)
	require.True(t, ok, "safety save transfer should be an object: %#v", transfers[1])
	require.Equal(t, "save-primary", safetySave["transfer_id"])
	require.Equal(t, "save", safetySave["operation"])
	require.Equal(t, true, safetySave["primary"])
	require.Equal(t, "final", safetySave["result_kind"])
	require.Equal(t, "execution", safetySave["permission_scope"])

	restoreTransfer, ok := transfers[2].(map[string]any)
	require.True(t, ok, "restore transfer should be an object: %#v", transfers[2])
	assertRestoreFinalRunTransferRecord(t, restoreTransfer, repoRoot, sourceID, publishedDestination, restoreTransferID, tempMarker)
}

func assertSingleRestoreTransfer(t *testing.T, data map[string]any) map[string]any {
	t.Helper()
	transfers := assertRestoreTransfers(t, data, 1)
	primary, ok := transfers[0].(map[string]any)
	require.True(t, ok, "primary transfer should be an object: %#v", transfers[0])
	return primary
}

func assertRestoreTransfers(t *testing.T, data map[string]any, expected int) []any {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, transfers, expected)
	return transfers
}

func assertRestorePlanFileExists(t *testing.T, repoRoot, planID string) {
	t.Helper()
	require.FileExists(t, filepath.Join(repoRoot, ".jvs", "restore-plans", planID+".json"))
}

func restorePlanFileCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "restore-plans"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, string(content))
}
