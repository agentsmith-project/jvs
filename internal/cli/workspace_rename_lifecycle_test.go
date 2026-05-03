package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceRenameFreshRunFailsClosedWhenExternalLocatorMissing(t *testing.T) {
	repoPath, _, _, folder := setupWorkspaceLifecycleExternalRepo(t, "wsrenamemissinglocator", "old-feature")
	locatorPath := filepath.Join(folder, repo.JVSDirName)
	require.NoError(t, os.Remove(locatorPath))
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "rename", "old-feature", "new-feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "workspace locator missing")
	assertWorkspaceRenameFreshRunUnchanged(t, repoPath, folder, "old-feature", "new-feature")
	assert.NoFileExists(t, locatorPath)
}

func TestWorkspaceRenameFreshRunFailsClosedWhenExternalLocatorCannotBeRewritten(t *testing.T) {
	repoPath, _, _, folder := setupWorkspaceLifecycleExternalRepo(t, "wsrenameunwritablelocator", "old-feature")
	require.NoError(t, os.Chmod(folder, 0555))
	t.Cleanup(func() {
		_ = os.Chmod(folder, 0755)
	})
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "rename", "old-feature", "new-feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "workspace locator")
	assertWorkspaceRenameFreshRunUnchanged(t, repoPath, folder, "old-feature", "new-feature")

	require.NoError(t, os.Chmod(folder, 0755))
	locator, ok, readErr := repo.ReadWorkspaceLocator(folder)
	require.NoError(t, readErr)
	require.True(t, ok)
	assert.Equal(t, "old-feature", locator.WorkspaceName)
}

func TestWorkspaceRenameRerunRepairsConfigDirRenamedBeforeConfigRewrite(t *testing.T) {
	repoPath, _, _, folder := setupWorkspaceLifecycleExternalRepo(t, "wsrenamerepairconfigname", "old-feature")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-feature")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-feature")
	require.NoError(t, os.Rename(oldConfigDir, newConfigDir))
	cfgBefore, err := repo.LoadWorktreeConfig(repoPath, "new-feature")
	require.NoError(t, err)
	require.Equal(t, "old-feature", cfgBefore.Name)

	repoID, err := workspaceCurrentRepoID(repoPath)
	require.NoError(t, err)
	record := workspaceLifecycleOperationRecord(repoID, workspaceRenameOperationID("old-feature", "new-feature"), "workspace rename", "started", "jvs workspace rename old-feature new-feature", map[string]any{
		"old_workspace":    "old-feature",
		"new_workspace":    "new-feature",
		"folder":           folder,
		"locator_present":  true,
		"locator_required": true,
	})
	require.NoError(t, lifecycle.WriteOperation(repoPath, record))
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "rename", "old-feature", "new-feature")
	require.NoError(t, err, stdout)
	var data publicWorkspaceRenameResult
	decodeRootJSONData(t, stdout, &data)
	assert.Equal(t, "renamed", data.Status)

	cfgAfter, err := repo.LoadWorktreeConfig(repoPath, "new-feature")
	require.NoError(t, err)
	assert.Equal(t, "new-feature", cfgAfter.Name)
	assert.Equal(t, folder, cfgAfter.RealPath)
	assert.NoDirExists(t, oldConfigDir)
	assert.DirExists(t, newConfigDir)
	locator, ok, err := repo.ReadWorkspaceLocator(folder)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "new-feature", locator.WorkspaceName)
	pending, err := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func assertWorkspaceRenameFreshRunUnchanged(t *testing.T, repoPath, folder, oldName, newName string) {
	t.Helper()

	assert.DirExists(t, folder)
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", oldName, "config.json"))
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "worktrees", newName))
	oldCfg, err := repo.LoadWorktreeConfig(repoPath, oldName)
	require.NoError(t, err)
	assert.Equal(t, oldName, oldCfg.Name)
	assert.Equal(t, folder, oldCfg.RealPath)
	_, err = repo.LoadWorktreeConfig(repoPath, newName)
	assert.Error(t, err)
	pending, err := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, err)
	assert.Empty(t, pending)
}
