package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repoDetachPreviewData struct {
	Mode               string `json:"mode"`
	PlanID             string `json:"plan_id"`
	OperationID        string `json:"operation_id"`
	RepoRoot           string `json:"repo_root"`
	RepoID             string `json:"repo_id"`
	ArchivePath        string `json:"archive_path"`
	ExternalWorkspaces int    `json:"external_workspaces"`
	RunCommand         string `json:"run_command"`
}

type repoDetachRunData struct {
	Mode                      string `json:"mode"`
	PlanID                    string `json:"plan_id"`
	OperationID               string `json:"operation_id"`
	Status                    string `json:"status"`
	RepoRoot                  string `json:"repo_root"`
	RepoID                    string `json:"repo_id"`
	ArchivePath               string `json:"archive_path"`
	WorkingFilesPreserved     bool   `json:"working_files_preserved"`
	ActiveRepoDetached        bool   `json:"active_repo_detached"`
	SavePointStorageRemoved   bool   `json:"save_point_storage_removed"`
	ExternalWorkspacesUpdated int    `json:"external_workspaces_updated"`
	RecommendedNextCommand    string `json:"recommended_next_command"`
}

func TestRepoDetachPreviewJSONWritesPlanOnlyAndDoesNotArchive(t *testing.T) {
	fixture := setupRepoDetachFixture(t, "repodetachpreview")
	require.NoError(t, os.Chdir(fixture.repoRoot))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "detach")
	require.NoError(t, err, stdout)
	env := decodeRootJSONData(t, stdout, &repoDetachPreviewData{})
	assert.Equal(t, "repo detach", env.Command)
	preview := decodeRepoDetachPreview(t, stdout)

	require.NotEmpty(t, preview.PlanID)
	require.NotEmpty(t, preview.OperationID)
	assert.NotEqual(t, preview.PlanID, preview.OperationID)
	assert.Equal(t, "preview", preview.Mode)
	assert.Equal(t, fixture.repoRoot, preview.RepoRoot)
	assert.Equal(t, fixture.repoID, preview.RepoID)
	assert.Equal(t, 1, preview.ExternalWorkspaces)
	assert.Equal(t, "jvs repo detach --run "+preview.PlanID, preview.RunCommand)
	assert.Contains(t, preview.ArchivePath, filepath.Join(".jvs-detached", fixture.repoID+"-"+preview.OperationID+"-"))
	assert.NotContains(t, preview.ArchivePath, preview.PlanID)
	assert.Regexp(t, regexp.MustCompile(regexp.QuoteMeta(fixture.repoID+"-"+preview.OperationID+"-")+`\d{8}T\d{6}Z$`), filepath.Base(preview.ArchivePath))

	assert.DirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, ".jvs-detached"))
	assert.FileExists(t, filepath.Join(fixture.repoRoot, "app.txt"))
	assertSavePointStorageExists(t, fixture.repoRoot, fixture.savePointID)
	pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)

	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, fixture.repoRoot, status.Folder)
	assert.Equal(t, "main", status.Workspace)
}

func TestRepoDetachRunArchivesControlDataKeepsFilesAndDisablesExternalLocators(t *testing.T) {
	fixture := setupRepoDetachFixture(t, "repodetachrun")
	preview := previewRepoDetach(t, fixture.repoRoot)
	require.NoError(t, os.Chdir(fixture.repoRoot))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "detach", "--run", preview.PlanID)
	require.NoError(t, err, stdout)
	run := decodeRepoDetachRun(t, stdout)

	assert.Equal(t, "run", run.Mode)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, preview.OperationID, run.OperationID)
	assert.Equal(t, "detached", run.Status)
	assert.Equal(t, fixture.repoRoot, run.RepoRoot)
	assert.Equal(t, fixture.repoID, run.RepoID)
	assert.Equal(t, preview.ArchivePath, run.ArchivePath)
	assert.True(t, run.WorkingFilesPreserved)
	assert.True(t, run.ActiveRepoDetached)
	assert.False(t, run.SavePointStorageRemoved)
	assert.Equal(t, 1, run.ExternalWorkspacesUpdated)
	assert.Equal(t, "jvs repo detach --run "+preview.PlanID, run.RecommendedNextCommand)

	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
	assert.DirExists(t, filepath.Join(run.ArchivePath, repo.JVSDirName))
	assert.FileExists(t, filepath.Join(run.ArchivePath, "DETACHING"))
	assert.FileExists(t, filepath.Join(run.ArchivePath, "DETACHED"))
	assert.FileExists(t, filepath.Join(fixture.repoRoot, "app.txt"))
	assertSavePointStorageArchived(t, run.ArchivePath, fixture.savePointID)

	detaching := readRepoDetachMarker(t, filepath.Join(run.ArchivePath, "DETACHING"))
	assert.Equal(t, run.OperationID, detaching["operation_id"])
	assert.Equal(t, run.RepoID, detaching["repo_id"])
	assert.Equal(t, run.RepoRoot, detaching["old_repo_root"])
	assert.Equal(t, run.ArchivePath, detaching["archive_path"])
	assert.Equal(t, run.RecommendedNextCommand, detaching["recommended_next_command"])

	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.Error(t, err)
	assert.Empty(t, statusOut)
	assert.ErrorIs(t, err, errclass.ErrNotRepo)

	require.NoError(t, os.Chdir(fixture.externalWorkspace))
	externalStatusOut, err := executeCommand(createTestRootCmd(), "status")
	require.Error(t, err)
	assert.Empty(t, externalStatusOut)
	assert.Contains(t, strings.ToLower(err.Error()), "detached")
	assert.Contains(t, strings.ToLower(err.Error()), "orphaned")
	_, inspectErr := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         fixture.externalWorkspace,
		ExpectedRepoRoot:      fixture.repoRoot,
		ExpectedRepoID:        fixture.repoID,
		ExpectedWorkspaceName: "feature",
	})
	require.Error(t, inspectErr)
	assert.Contains(t, strings.ToLower(inspectErr.Error()), "detached")
}

func TestRepoDetachExternalLocatorPreflightFailsClosedBeforeArchive(t *testing.T) {
	t.Run("malformed locator blocks preview", func(t *testing.T) {
		fixture := setupRepoDetachFixture(t, "repodetachmalformedlocator")
		require.NoError(t, os.WriteFile(filepath.Join(fixture.externalWorkspace, repo.JVSDirName), []byte("{not-json"), 0644))
		require.NoError(t, os.Chdir(fixture.repoRoot))

		stdout, err := executeCommand(createTestRootCmd(), "repo", "detach")
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.DirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
		assert.NoDirExists(t, filepath.Join(fixture.repoRoot, ".jvs-detached"))
	})

	t.Run("wrong workspace name blocks run before archive", func(t *testing.T) {
		fixture := setupRepoDetachFixture(t, "repodetachwronglocator")
		preview := previewRepoDetach(t, fixture.repoRoot)
		writeRepoMoveTestWorkspaceLocator(t, fixture.externalWorkspace, fixture.repoRoot, fixture.repoID, "wrong-name")
		require.NoError(t, os.Chdir(fixture.repoRoot))

		stdout, err := executeCommand(createTestRootCmd(), "repo", "detach", "--run", preview.PlanID)
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, err.Error(), "workspace_name mismatch")
		assert.DirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
		assert.NoDirExists(t, filepath.Join(fixture.repoRoot, ".jvs-detached"))
		pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
		require.NoError(t, pendingErr)
		assert.Empty(t, pending)
	})

	t.Run("unwritable locator blocks run before archive", func(t *testing.T) {
		fixture := setupRepoDetachFixture(t, "repodetachunwritablelocator")
		preview := previewRepoDetach(t, fixture.repoRoot)
		locatorPath := filepath.Join(fixture.externalWorkspace, repo.JVSDirName)
		require.NoError(t, os.Chmod(locatorPath, 0444))
		t.Cleanup(func() { _ = os.Chmod(locatorPath, 0644) })
		require.NoError(t, os.Chdir(fixture.repoRoot))

		stdout, err := executeCommand(createTestRootCmd(), "repo", "detach", "--run", preview.PlanID)
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, err.Error(), "not writable")
		assert.DirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
		assert.NoDirExists(t, filepath.Join(fixture.repoRoot, ".jvs-detached"))
	})
}

func TestRepoDetachRunResumesAfterArchiveBeforeDetachedMetadata(t *testing.T) {
	fixture := setupRepoDetachFixture(t, "repodetachresume")
	preview := previewRepoDetach(t, fixture.repoRoot)
	oldHooks := repoDetachRunHooks
	repoDetachRunHooks.afterArchive = func() error {
		return errors.New("injected crash after .jvs archive")
	}
	t.Cleanup(func() { repoDetachRunHooks = oldHooks })
	require.NoError(t, os.Chdir(fixture.repoRoot))

	stdout, err := executeCommand(createTestRootCmd(), "repo", "detach", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected crash")
	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
	assert.DirExists(t, filepath.Join(preview.ArchivePath, repo.JVSDirName))
	assert.FileExists(t, filepath.Join(preview.ArchivePath, "DETACHING"))
	assert.NoFileExists(t, filepath.Join(preview.ArchivePath, "DETACHED"))
	assert.FileExists(t, filepath.Join(preview.ArchivePath, repo.JVSDirName, "lifecycle", "operations", preview.OperationID+".json"))
	assert.NoFileExists(t, filepath.Join(preview.ArchivePath, repo.JVSDirName, "lifecycle", "operations", preview.PlanID+".json"))

	repoDetachRunHooks = oldHooks
	require.NoError(t, os.Chdir(fixture.repoRoot))
	runOut, err := executeCommand(createTestRootCmd(), "--json", "repo", "detach", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeRepoDetachRun(t, runOut)
	assert.Equal(t, "detached", run.Status)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, preview.OperationID, run.OperationID)
	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
	assert.FileExists(t, filepath.Join(preview.ArchivePath, "DETACHED"))
	assert.NoFileExists(t, filepath.Join(preview.ArchivePath, repo.JVSDirName, "lifecycle", "operations", preview.OperationID+".json"))

	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.Error(t, err)
	assert.Empty(t, statusOut)
}

func TestRepoDetachRunWithDetachedMarkerConsumesPendingLifecycle(t *testing.T) {
	fixture := setupRepoDetachFixture(t, "repodetachdetachedpending")
	preview := previewRepoDetach(t, fixture.repoRoot)
	oldHooks := repoDetachRunHooks
	repoDetachRunHooks.afterDetached = func() error {
		return errors.New("injected crash after DETACHED marker")
	}
	t.Cleanup(func() { repoDetachRunHooks = oldHooks })
	require.NoError(t, os.Chdir(fixture.repoRoot))

	stdout, err := executeCommand(createTestRootCmd(), "repo", "detach", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected crash")
	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, repo.JVSDirName))
	assert.FileExists(t, filepath.Join(preview.ArchivePath, "DETACHED"))
	pending, pendingErr := lifecycle.ListPendingOperations(preview.ArchivePath)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.OperationID, pending[0].OperationID)

	repoDetachRunHooks = oldHooks
	require.NoError(t, os.Chdir(fixture.repoRoot))
	runOut, err := executeCommand(createTestRootCmd(), "--json", "repo", "detach", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeRepoDetachRun(t, runOut)
	assert.Equal(t, "detached", run.Status)
	assert.Equal(t, preview.PlanID, run.PlanID)
	assert.Equal(t, preview.OperationID, run.OperationID)

	pending, pendingErr = lifecycle.ListPendingOperations(preview.ArchivePath)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
	assert.FileExists(t, filepath.Join(preview.ArchivePath, "DETACHED"))
}

type repoDetachFixture struct {
	repoRoot          string
	repoID            string
	savePointID       string
	externalWorkspace string
}

func setupRepoDetachFixture(t *testing.T, name string) repoDetachFixture {
	t.Helper()
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte(name), 0644))
	savePointID := createRootTestSavePoint(t, "baseline")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../feature", "--from", savePointID)
	require.NoError(t, err, stdout)
	return repoDetachFixture{
		repoRoot:          repoRoot,
		repoID:            readRepoMoveTestRepoID(t, repoRoot),
		savePointID:       savePointID,
		externalWorkspace: filepath.Join(filepath.Dir(repoRoot), "feature"),
	}
}

func previewRepoDetach(t *testing.T, repoRoot string) repoDetachPreviewData {
	t.Helper()
	require.NoError(t, os.Chdir(repoRoot))
	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "detach")
	require.NoError(t, err, stdout)
	return decodeRepoDetachPreview(t, stdout)
}

func decodeRepoDetachPreview(t *testing.T, stdout string) repoDetachPreviewData {
	t.Helper()
	var preview repoDetachPreviewData
	decodeRootJSONData(t, stdout, &preview)
	return preview
}

func decodeRepoDetachRun(t *testing.T, stdout string) repoDetachRunData {
	t.Helper()
	var run repoDetachRunData
	decodeRootJSONData(t, stdout, &run)
	return run
}

func readRepoDetachMarker(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var marker map[string]any
	require.NoError(t, json.Unmarshal(data, &marker))
	return marker
}

func assertSavePointStorageArchived(t *testing.T, archivePath, savePointID string) {
	t.Helper()
	assert.DirExists(t, filepath.Join(archivePath, repo.JVSDirName, "snapshots", savePointID))
	assert.FileExists(t, filepath.Join(archivePath, repo.JVSDirName, "descriptors", savePointID+".json"))
}
