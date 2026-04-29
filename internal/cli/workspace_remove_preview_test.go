package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workspaceRemovePreviewData struct {
	Mode              string                      `json:"mode"`
	PlanID            string                      `json:"plan_id"`
	Workspace         string                      `json:"workspace"`
	Folder            string                      `json:"folder"`
	UnsavedChanges    bool                        `json:"unsaved_changes"`
	FilesState        string                      `json:"files_state"`
	NewestSavePoint   *string                     `json:"newest_save_point"`
	ContentSource     *string                     `json:"content_source"`
	FolderRemoved     bool                        `json:"folder_removed"`
	FilesChanged      bool                        `json:"files_changed"`
	RunCommand        string                      `json:"run_command"`
	Options           workspaceRemovePreviewFlags `json:"options"`
	CleanupPreviewRun string                      `json:"cleanup_preview_run"`
}

type workspaceRemovePreviewFlags struct {
	DiscardUnsaved     bool `json:"discard_unsaved,omitempty"`
	RemovesUnsavedWork bool `json:"removes_unsaved_work,omitempty"`
}

type workspaceRemoveRunData struct {
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

func TestWorkspaceRemovePreviewFirstDoesNotDeleteFolder(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceRemoveRepo(t, "wspreviewfirst", "feature")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "feature")
	require.NoError(t, err, stdout)
	preview := decodeWorkspaceRemovePreview(t, stdout)

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
	assert.Equal(t, "jvs workspace remove --run "+preview.PlanID, preview.RunCommand)
	assert.Contains(t, preview.CleanupPreviewRun, "jvs cleanup preview")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceRemoveRunDeletesOnlyWorkspaceFolderAndMetadata(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceRemoveRepo(t, "wsremoverun", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceRemovePreview(t, previewOut)
	planPath := requireWorkspaceRemovePlanFile(t, repoPath, preview.PlanID)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "--run", preview.PlanID)
	require.NoError(t, err, stdout)
	run := decodeWorkspaceRemoveRun(t, stdout)

	assert.Equal(t, "run", run.Mode)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, "removed", run.Status)
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

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "remove", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "not found")
}

func TestWorkspaceRemoveDirtyRequiresForceBeforeRunnablePlan(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceRemoveRepo(t, "wsremovedirty", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("dirty"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "remove", "feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "unsaved changes")
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "--force", "feature")
	require.NoError(t, err, stdout)
	preview := decodeWorkspaceRemovePreview(t, stdout)

	assert.Equal(t, "preview", preview.Mode)
	assert.True(t, preview.UnsavedChanges)
	assert.True(t, preview.Options.DiscardUnsaved)
	assert.True(t, preview.Options.RemovesUnsavedWork)
	assert.False(t, preview.FolderRemoved)
	assert.False(t, preview.FilesChanged)
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "remove", "--run", preview.PlanID, "--force")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "options are fixed by the preview plan")
	assert.DirExists(t, featurePath)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceRemoveMainUsesWorkspaceVocabulary(t *testing.T) {
	_, _, _, _ = setupWorkspaceRemoveRepo(t, "wsremovemain", "feature")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "remove", "main")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot remove main workspace")
	assert.NotContains(t, err.Error(), "worktree")
}

func TestWorkspaceRemoveRunAcquiresMutationLockBeforeRevalidate(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceRemoveRepo(t, "wsremovelockboundary", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceRemovePreview(t, previewOut)
	planPath := requireWorkspaceRemovePlanFile(t, repoPath, preview.PlanID)

	lock, err := repo.AcquireMutationLock(repoPath, "test concurrent workspace mutation")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, lock.Release())
	}()

	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("changed while locked"), 0644))
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "remove", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "repository mutation lock")
	assert.NotContains(t, err.Error(), "changed since preview")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, planPath)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceRemoveRunFailsIfWorkspaceChangedSincePreview(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceRemoveRepo(t, "wsremovechanged", "feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "feature")
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceRemovePreview(t, previewOut)
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "remove-base.txt"), []byte("changed after preview"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "remove", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "changed since preview")
	assert.Contains(t, err.Error(), "No workspace was removed")
	assert.DirExists(t, featurePath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func setupWorkspaceRemoveRepo(t *testing.T, repoName, workspaceName string) (repoPath, mainPath, savePointID, workspacePath string) {
	t.Helper()

	repoPath, mainPath = setupCoverageRepo(t, repoName)
	require.NoError(t, os.WriteFile("remove-base.txt", []byte("remove"), 0644))
	savePointID = createRootTestSavePoint(t, "remove base")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", workspaceName, "--from", savePointID)
	require.NoError(t, err, stdout)
	workspacePath = filepath.Join(repoPath, "worktrees", workspaceName)
	return repoPath, mainPath, savePointID, workspacePath
}

func decodeWorkspaceRemovePreview(t *testing.T, stdout string) workspaceRemovePreviewData {
	t.Helper()

	var preview workspaceRemovePreviewData
	decodeRootJSONData(t, stdout, &preview)
	return preview
}

func decodeWorkspaceRemoveRun(t *testing.T, stdout string) workspaceRemoveRunData {
	t.Helper()

	var run workspaceRemoveRunData
	decodeRootJSONData(t, stdout, &run)
	return run
}

func assertSavePointStorageExists(t *testing.T, repoPath, savePointID string) {
	t.Helper()

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", savePointID))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", savePointID+".json"))
}

func requireWorkspaceRemovePlanFile(t *testing.T, repoPath, planID string) string {
	t.Helper()

	path, err := workspaceRemovePlanPath(repoPath, planID, false)
	require.NoError(t, err)
	require.FileExists(t, path)
	return path
}
