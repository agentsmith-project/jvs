package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workspaceDeletePreviewData struct {
	Mode                  string                      `json:"mode"`
	PlanID                string                      `json:"plan_id"`
	Workspace             string                      `json:"workspace"`
	Folder                string                      `json:"folder"`
	UnsavedChanges        bool                        `json:"unsaved_changes"`
	FilesState            string                      `json:"files_state"`
	NewestSavePoint       *string                     `json:"newest_save_point"`
	ContentSource         *string                     `json:"content_source"`
	FolderRemoved         bool                        `json:"folder_removed"`
	FilesChanged          bool                        `json:"files_changed"`
	RunCommand            string                      `json:"run_command"`
	SafeRunCommand        string                      `json:"safe_run_command"`
	CWDInsideAffectedTree bool                        `json:"cwd_inside_affected_tree"`
	Options               workspaceDeletePreviewFlags `json:"options"`
	CleanupPreviewRun     string                      `json:"cleanup_preview_run"`
}

type workspaceDeletePreviewFlags struct {
	DiscardUnsaved     bool `json:"discard_unsaved,omitempty"`
	RemovesUnsavedWork bool `json:"removes_unsaved_work,omitempty"`
}

type workspaceDeleteRunData struct {
	Mode                     string `json:"mode"`
	PlanID                   string `json:"plan_id"`
	Status                   string `json:"status"`
	Workspace                string `json:"workspace"`
	Folder                   string `json:"folder"`
	FolderRemoved            bool   `json:"folder_removed"`
	WorkspaceMetadataRemoved bool   `json:"workspace_metadata_removed"`
	SavePointStorageRemoved  bool   `json:"save_point_storage_removed"`
	FilesChanged             bool   `json:"files_changed"`
	CleanupCommand           string `json:"cleanup_command"`
}

func TestWorkspaceDeletePreviewFirstDoesNotDeleteFolder(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wspreviewfirst", "feature")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, stdout)
	preview := decodeWorkspaceDeletePreview(t, stdout)

	assert.Equal(t, "preview", preview.Mode)
	require.NotEmpty(t, preview.PlanID)
	assert.Equal(t, "feature", preview.Workspace)
	assert.Equal(t, featurePath, preview.Folder)
	assert.False(t, preview.UnsavedChanges)
	assert.Equal(t, "started_from_save_point", preview.FilesState)
	assert.Nil(t, preview.NewestSavePoint)
	require.NotNil(t, preview.ContentSource)
	assert.Equal(t, savePointID, *preview.ContentSource)
	assert.False(t, preview.FolderRemoved)
	assert.False(t, preview.FilesChanged)
	assert.Equal(t, "jvs workspace delete --run "+preview.PlanID, preview.RunCommand)
	assert.Contains(t, preview.CleanupPreviewRun, "jvs cleanup preview")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeleteRunDeletesOnlyWorkspaceFolderAndMetadata(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeleterun", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "result.txt"), []byte("kept in save point"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	workspaceSavePointID := createRootTestSavePoint(t, "feature result")
	require.NoError(t, os.Chdir(repoPath))

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	planPath := requireWorkspaceDeletePlanFile(t, repoPath, preview.PlanID)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "--run", preview.PlanID)
	require.NoError(t, err, stdout)
	run := decodeWorkspaceDeleteRun(t, stdout)

	assert.Equal(t, "run", run.Mode)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, "deleted", run.Status)
	assert.Equal(t, "feature", run.Workspace)
	assert.Equal(t, featurePath, run.Folder)
	assert.True(t, run.FolderRemoved)
	assert.True(t, run.WorkspaceMetadataRemoved)
	assert.False(t, run.SavePointStorageRemoved)
	assert.True(t, run.FilesChanged)
	assert.Contains(t, run.CleanupCommand, "jvs cleanup preview")
	assert.NoDirExists(t, featurePath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature"))
	assertSavePointStorageExists(t, repoPath, savePointID)
	assert.NoFileExists(t, planPath)

	viewOut, err := executeCommand(createTestRootCmd(), "--json", "view", workspaceSavePointID, "result.txt")
	require.NoError(t, err, viewOut)
	var viewResult publicViewResult
	decodeRootJSONData(t, viewOut, &viewResult)
	closeOut, err := executeCommand(createTestRootCmd(), "--json", "view", "close", viewResult.ViewID)
	require.NoError(t, err, closeOut)

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "delete", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "not found")
}

func TestWorkspaceDeleteDirtyFailsClosedAndHasNoForceCompatibility(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeletedirty", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("dirty"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unsaved changes")
	assert.NotContains(t, err.Error(), "--force")
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "delete", "--force", "feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unknown flag: --force")
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeleteMainUsesWorkspaceVocabulary(t *testing.T) {
	_, _, _, _ = setupWorkspaceDeleteRepo(t, "wsdeletemain", "feature")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "main")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot delete main workspace")
	assert.NotContains(t, err.Error(), "worktree")
}

func TestWorkspaceDeleteRunAcquiresMutationLockBeforeRevalidate(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeletelockboundary", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	planPath := requireWorkspaceDeletePlanFile(t, repoPath, preview.PlanID)

	lock, err := repo.AcquireMutationLock(repoPath, "test concurrent workspace mutation")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, lock.Release())
	}()

	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("changed while locked"), 0644))
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "repository mutation lock")
	assert.NotContains(t, err.Error(), "changed since preview")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, planPath)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeleteRunFailsIfWorkspaceChangedSincePreview(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeletechanged", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("changed after preview"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "changed since preview")
	assert.Contains(t, err.Error(), "No workspace was deleted")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeleteRunFromInsideTargetFailsBeforeJournalOrMutation(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeleteunsafecwd", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	planPath := requireWorkspaceDeletePlanFile(t, repoPath, preview.PlanID)

	require.NoError(t, os.Chdir(featurePath))
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.ErrorIs(t, err, errclass.ErrLifecycleUnsafeCWD)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr))
	assert.Equal(t, "E_LIFECYCLE_UNSAFE_CWD", jvsErr.Code)
	assert.Contains(t, err.Error(), preview.SafeRunCommand)
	assert.DirExists(t, featurePath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assert.FileExists(t, planPath)
	pending, pendingErr := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeletePreviewFromInsideExternalWorkspaceIncludesExplicitSafeRun(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceLifecycleExternalRepo(t, "wsdeletepreviewsafecwd", "feature")
	require.NoError(t, os.Chdir(featurePath))

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	require.NotEmpty(t, preview.PlanID)
	assert.True(t, preview.CWDInsideAffectedTree)
	assert.Equal(t, "jvs workspace delete --run "+preview.PlanID, preview.RunCommand)
	assert.Equal(t, "cd "+filepath.Dir(featurePath)+" && jvs --repo "+repoPath+" workspace delete --run "+preview.PlanID, preview.SafeRunCommand)
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceDeleteRunResumesAfterFolderAndConfigRemovedBeforePhase(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsdeleteresume", "feature")
	require.NoError(t, os.Chdir(repoPath))

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceDeletePreview(t, previewOut)
	planPath := requireWorkspaceDeletePlanFile(t, repoPath, preview.PlanID)

	oldHooks := workspaceDeleteRunHooks
	workspaceDeleteRunHooks.afterRemoveLocked = func() error {
		return errors.New("injected crash after workspace delete")
	}
	t.Cleanup(func() {
		workspaceDeleteRunHooks = oldHooks
	})

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "delete", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected crash")
	assert.NoDirExists(t, featurePath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature"))
	pending, pendingErr := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.PlanID, pending[0].OperationID)

	workspaceDeleteRunHooks = oldHooks
	runOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeWorkspaceDeleteRun(t, runOut)
	assert.Equal(t, "deleted", run.Status)
	assert.Equal(t, "feature", run.Workspace)
	assert.Equal(t, featurePath, run.Folder)
	assert.True(t, run.FolderRemoved)
	assert.True(t, run.WorkspaceMetadataRemoved)
	assert.NoDirExists(t, featurePath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature"))
	assertSavePointStorageExists(t, repoPath, savePointID)
	assert.NoFileExists(t, planPath)
	pending, pendingErr = lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
}

func setupWorkspaceDeleteRepo(t *testing.T, repoName, workspaceName string) (repoPath, mainPath, savePointID, workspacePath string) {
	t.Helper()

	repoPath, mainPath = setupCoverageRepo(t, repoName)
	require.NoError(t, os.WriteFile("remove-base.txt", []byte("remove"), 0644))
	savePointID = createRootTestSavePoint(t, "remove base")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../"+workspaceName, "--from", savePointID)
	require.NoError(t, err, stdout)
	workspacePath = testExternalWorkspacePath(repoPath, workspaceName)
	return repoPath, mainPath, savePointID, workspacePath
}

func decodeWorkspaceDeletePreview(t *testing.T, stdout string) workspaceDeletePreviewData {
	t.Helper()

	var preview workspaceDeletePreviewData
	decodeRootJSONData(t, stdout, &preview)
	return preview
}

func decodeWorkspaceDeleteRun(t *testing.T, stdout string) workspaceDeleteRunData {
	t.Helper()

	var run workspaceDeleteRunData
	decodeRootJSONData(t, stdout, &run)
	return run
}

func assertSavePointStorageExists(t *testing.T, repoPath, savePointID string) {
	t.Helper()

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", savePointID))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", savePointID+".json"))
}

func requireWorkspaceDeletePlanFile(t *testing.T, repoPath, planID string) string {
	t.Helper()

	path, err := workspaceDeletePlanPath(repoPath, planID, false)
	require.NoError(t, err)
	require.FileExists(t, path)
	return path
}
