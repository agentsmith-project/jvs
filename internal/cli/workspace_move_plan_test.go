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

type workspaceMovePreviewData struct {
	Mode                    string  `json:"mode"`
	PlanID                  string  `json:"plan_id"`
	Workspace               string  `json:"workspace"`
	SourceFolder            string  `json:"source_folder"`
	TargetFolder            string  `json:"target_folder"`
	NewestSavePoint         *string `json:"newest_save_point"`
	ContentSource           *string `json:"content_source"`
	ExpectedFolderEvidence  string  `json:"expected_folder_evidence"`
	UnsavedChanges          bool    `json:"unsaved_changes"`
	FilesState              string  `json:"files_state"`
	FolderMoved             bool    `json:"folder_moved"`
	FilesChanged            bool    `json:"files_changed"`
	WorkspaceNameChanged    bool    `json:"workspace_name_changed"`
	SavePointHistoryChanged bool    `json:"save_point_history_changed"`
	MoveMethod              string  `json:"move_method"`
	RunCommand              string  `json:"run_command"`
	SafeRunCommand          string  `json:"safe_run_command"`
	CWDInsideAffectedTree   bool    `json:"cwd_inside_affected_tree"`
}

type workspaceMoveRunData struct {
	Mode                    string `json:"mode"`
	PlanID                  string `json:"plan_id"`
	Status                  string `json:"status"`
	Workspace               string `json:"workspace"`
	SourceFolder            string `json:"source_folder"`
	TargetFolder            string `json:"target_folder"`
	Folder                  string `json:"folder"`
	FolderMoved             bool   `json:"folder_moved"`
	FilesChanged            bool   `json:"files_changed"`
	WorkspaceNameChanged    bool   `json:"workspace_name_changed"`
	SavePointHistoryChanged bool   `json:"save_point_history_changed"`
}

func TestWorkspaceMovePreviewRunMovesFolderOnlyAndPreservesHistory(t *testing.T) {
	repoPath, _, _, featurePath := setupWorkspaceDeleteRepo(t, "wsmoverun", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "result.txt"), []byte("move me"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	featureSave := createRootTestSavePoint(t, "feature before move")
	require.NoError(t, os.Chdir(repoPath))
	targetPath := filepath.Join(filepath.Dir(repoPath), "moved-feature")

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "feature", targetPath)
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceMovePreview(t, previewOut)
	assert.Equal(t, "preview", preview.Mode)
	require.NotEmpty(t, preview.PlanID)
	assert.Equal(t, "feature", preview.Workspace)
	assert.Equal(t, featurePath, preview.SourceFolder)
	assert.Equal(t, targetPath, preview.TargetFolder)
	assert.False(t, preview.FolderMoved)
	assert.False(t, preview.FilesChanged)
	assert.False(t, preview.WorkspaceNameChanged)
	assert.False(t, preview.SavePointHistoryChanged)
	assert.Equal(t, "atomic rename required", preview.MoveMethod)
	assert.Equal(t, "jvs workspace move --run "+preview.PlanID, preview.RunCommand)
	assert.DirExists(t, featurePath)
	assert.NoDirExists(t, targetPath)

	runOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeWorkspaceMoveRun(t, runOut)
	assert.Equal(t, "run", run.Mode)
	assert.Equal(t, "moved", run.Status)
	assert.Equal(t, "feature", run.Workspace)
	assert.Equal(t, featurePath, run.SourceFolder)
	assert.Equal(t, targetPath, run.TargetFolder)
	assert.Equal(t, targetPath, run.Folder)
	assert.True(t, run.FolderMoved)
	assert.True(t, run.FilesChanged)
	assert.False(t, run.WorkspaceNameChanged)
	assert.False(t, run.SavePointHistoryChanged)
	assert.NoDirExists(t, featurePath)
	assert.DirExists(t, targetPath)

	pathOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "path", "feature")
	require.NoError(t, err, pathOut)
	var pathData map[string]string
	decodeRootJSONData(t, pathOut, &pathData)
	assert.Equal(t, targetPath, pathData["path"])

	require.NoError(t, os.Chdir(targetPath))
	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, "feature", status.Workspace)
	assert.Equal(t, targetPath, status.Folder)
	require.NotNil(t, status.NewestSavePoint)
	assert.Equal(t, featureSave, *status.NewestSavePoint)

	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err, historyOut)
	assert.Contains(t, historyOut, featureSave)
}

func TestWorkspaceMoveRunFromSourceSubtreeFailsBeforeJournalOrMutation(t *testing.T) {
	repoPath, _, savePointID, featurePath := setupWorkspaceDeleteRepo(t, "wsmoveunsafecwd", "feature")
	targetPath := filepath.Join(filepath.Dir(repoPath), "moved-feature")
	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "feature", targetPath)
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceMovePreview(t, previewOut)
	planPath, err := workspaceMovePlanPath(repoPath, preview.PlanID, false)
	require.NoError(t, err)
	require.FileExists(t, planPath)
	subdir := filepath.Join(featurePath, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.Chdir(subdir))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.ErrorIs(t, err, errclass.ErrLifecycleUnsafeCWD)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr))
	assert.Equal(t, "E_LIFECYCLE_UNSAFE_CWD", jvsErr.Code)
	assert.Contains(t, err.Error(), preview.SafeRunCommand)
	assert.DirExists(t, featurePath)
	assert.NoDirExists(t, targetPath)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "feature", "config.json"))
	assert.FileExists(t, planPath)
	pending, pendingErr := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
	assertSavePointStorageExists(t, repoPath, savePointID)
}

func TestWorkspaceMovePreviewFromInsideExternalWorkspaceIncludesExplicitSafeRun(t *testing.T) {
	repoPath, _, _, featurePath := setupWorkspaceLifecycleExternalRepo(t, "wsmovepreviewsafecwd", "feature")
	targetPath := filepath.Join(filepath.Dir(featurePath), "moved-feature")
	require.NoError(t, os.Chdir(featurePath))

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "feature", targetPath)
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceMovePreview(t, previewOut)
	require.NotEmpty(t, preview.PlanID)
	assert.True(t, preview.CWDInsideAffectedTree)
	assert.Equal(t, "jvs workspace move --run "+preview.PlanID, preview.RunCommand)
	assert.Equal(t, "cd "+filepath.Dir(featurePath)+" && jvs --repo "+repoPath+" workspace move --run "+preview.PlanID, preview.SafeRunCommand)
	assert.DirExists(t, featurePath)
	assert.NoDirExists(t, targetPath)
}

func TestWorkspaceMoveRunResumesAfterFolderAndConfigMovedBeforeLocator(t *testing.T) {
	repoPath, _, _, featurePath := setupWorkspaceDeleteRepo(t, "wsmoveresume", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "result.txt"), []byte("move me"), 0644))
	require.NoError(t, os.Chdir(repoPath))
	targetPath := filepath.Join(filepath.Dir(repoPath), "moved-feature")

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "feature", targetPath)
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceMovePreview(t, previewOut)
	planPath, err := workspaceMovePlanPath(repoPath, preview.PlanID, false)
	require.NoError(t, err)

	oldHooks := workspaceMoveRunHooks
	workspaceMoveRunHooks.afterMoveLocked = func() error {
		return errors.New("injected crash after workspace move")
	}
	t.Cleanup(func() {
		workspaceMoveRunHooks = oldHooks
	})

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected crash")
	assert.NoDirExists(t, featurePath)
	assert.DirExists(t, targetPath)
	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "feature")
	require.NoError(t, loadErr)
	assert.Equal(t, targetPath, cfg.RealPath)
	pending, pendingErr := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.PlanID, pending[0].OperationID)

	workspaceMoveRunHooks = oldHooks
	runOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeWorkspaceMoveRun(t, runOut)
	assert.Equal(t, "moved", run.Status)
	assert.Equal(t, targetPath, run.Folder)
	assert.NoDirExists(t, featurePath)
	assert.DirExists(t, targetPath)
	assert.NoFileExists(t, planPath)
	pending, pendingErr = lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)

	pathOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "path", "feature")
	require.NoError(t, err, pathOut)
	var pathData map[string]string
	decodeRootJSONData(t, pathOut, &pathData)
	assert.Equal(t, targetPath, pathData["path"])

	require.NoError(t, os.Chdir(targetPath))
	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, "feature", status.Workspace)
	assert.Equal(t, targetPath, status.Folder)
}

func TestWorkspaceMoveResumeFailsClosedWhenSourceDifferentAndDestinationExpected(t *testing.T) {
	repoPath, _, _, featurePath := setupWorkspaceDeleteRepo(t, "wsmoveresumefail", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "result.txt"), []byte("move me"), 0644))
	require.NoError(t, os.Chdir(repoPath))
	targetPath := filepath.Join(filepath.Dir(repoPath), "moved-feature")

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "move", "feature", targetPath)
	require.NoError(t, err, previewOut)
	preview := decodeWorkspaceMovePreview(t, previewOut)
	planPath, err := workspaceMovePlanPath(repoPath, preview.PlanID, false)
	require.NoError(t, err)

	oldHooks := workspaceMoveRunHooks
	workspaceMoveRunHooks.afterMoveLocked = func() error {
		return errors.New("injected crash after workspace move")
	}
	t.Cleanup(func() {
		workspaceMoveRunHooks = oldHooks
	})

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	workspaceMoveRunHooks = oldHooks

	require.NoError(t, os.MkdirAll(featurePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "different.txt"), []byte("different identity"), 0644))

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "workspace move cannot resume")
	assert.Contains(t, err.Error(), "source folder identity changed")
	assert.FileExists(t, planPath)
	pending, pendingErr := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.PlanID, pending[0].OperationID)
	assert.DirExists(t, featurePath)
	assert.DirExists(t, targetPath)
}

func decodeWorkspaceMovePreview(t *testing.T, stdout string) workspaceMovePreviewData {
	t.Helper()

	var preview workspaceMovePreviewData
	decodeRootJSONData(t, stdout, &preview)
	return preview
}

func decodeWorkspaceMoveRun(t *testing.T, stdout string) workspaceMoveRunData {
	t.Helper()

	var run workspaceMoveRunData
	decodeRootJSONData(t, stdout, &run)
	return run
}
