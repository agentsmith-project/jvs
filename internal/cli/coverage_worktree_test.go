package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspacePathCommand tests the public workspace path command.
func TestWorkspacePathCommand(t *testing.T) {
	repoPath, mainPath := setupCoverageRepo(t, "wspathrepo")

	t.Run("Workspace path with name", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "path", "main")
		assert.NoError(t, err)
		assert.Contains(t, stdout, mainPath)
	})

	assert.NoError(t, os.WriteFile("path-base.txt", []byte("path"), 0644))
	savePointID := createRootTestSavePoint(t, "path base")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "path-feature", "--from", savePointID)
	require.NoError(t, err, stdout)

	featurePath := filepath.Join(repoPath, "worktrees", "path-feature")
	assert.NoError(t, os.Chdir(featurePath))

	t.Run("Workspace path from inside workspace", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "path")
		assert.NoError(t, err)
		assert.Contains(t, stdout, featurePath)
	})
}

// TestWorkspaceRenameCommand tests the public workspace rename command.
func TestWorkspaceRenameCommand(t *testing.T) {
	setupCoverageRepo(t, "wsrename")
	assert.NoError(t, os.WriteFile("rename-base.txt", []byte("rename"), 0644))
	savePointID := createRootTestSavePoint(t, "rename base")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "oldname", "--from", savePointID)
	require.NoError(t, err, stdout)

	t.Run("Rename workspace", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "rename", "oldname", "newname")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Renamed workspace")
	})

	t.Run("Rename with JSON output", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "oldname2", "--from", savePointID)
		require.NoError(t, err, stdout)

		stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "rename", "oldname2", "newname2")
		assert.NoError(t, err)
		assert.Contains(t, stdout, `"workspace": "newname2"`)
		assert.Contains(t, stdout, `"status": "renamed"`)
	})
}

// TestWorkspaceNewCommand tests creating public workspaces from a save point.
func TestWorkspaceNewCommand(t *testing.T) {
	setupCoverageRepo(t, "wsnewrepo")
	assert.NoError(t, os.WriteFile("newfile.txt", []byte("content"), 0644))
	savePointID := createRootTestSavePoint(t, "workspace base")

	t.Run("New workspace from save point", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "custom-workspace", "--from", savePointID)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "custom-workspace")
		assert.Contains(t, stdout, "Started from save point")
	})

	t.Run("New workspace with JSON output", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "new", "json-workspace", "--from", savePointID)
		assert.NoError(t, err)
		assert.Contains(t, stdout, `"workspace": "json-workspace"`)
		assert.Contains(t, stdout, `"started_from_save_point": "`)
		assert.NotContains(t, stdout, "snapshot_id")
	})
}

// TestWorkspaceListCommand tests the public workspace list command.
func TestWorkspaceListCommand(t *testing.T) {
	setupCoverageRepo(t, "wslistrepo")
	assert.NoError(t, os.WriteFile("list-base.txt", []byte("list"), 0644))
	savePointID := createRootTestSavePoint(t, "list base")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "list-feature", "--from", savePointID)
	require.NoError(t, err, stdout)

	t.Run("List workspaces", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "workspace", "list")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "main")
		assert.Contains(t, stdout, "list-feature")
	})

	t.Run("List workspaces with JSON", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "list")
		assert.NoError(t, err)
		assert.Contains(t, stdout, `"workspace": "main"`)
		assert.Contains(t, stdout, `"workspace": "list-feature"`)
		assert.NotContains(t, stdout, "snapshot_id")
	})
}

// TestInitCommandJSON tests init command with JSON output.
func TestInitCommandJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
		createTestRootCmd()
	})

	assert.NoError(t, os.Chdir(dir))

	t.Run("Init with JSON output", func(t *testing.T) {
		assert.NoError(t, os.Mkdir("jsonrepo", 0755))
		stdout, err := executeCommand(createTestRootCmd(), "--json", "init", "jsonrepo")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "repo_root")
		assert.Contains(t, stdout, "repo_id")
		assert.Contains(t, stdout, "folder")
		assert.Contains(t, stdout, "workspace")
	})
}

// TestWorkspaceRemoveForce tests that force creates a reviewed remove plan.
func TestWorkspaceRemoveForce(t *testing.T) {
	repoPath, _ := setupCoverageRepo(t, "wsforceremove")
	assert.NoError(t, os.WriteFile("remove-base.txt", []byte("remove"), 0644))
	savePointID := createRootTestSavePoint(t, "remove base")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "toberemoved", "--from", savePointID)
	require.NoError(t, err, stdout)

	t.Run("Force previews before removing workspace", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "remove", "--force", "toberemoved")
		assert.NoError(t, err)
		preview := decodeWorkspaceRemovePreview(t, stdout)
		assert.Equal(t, "preview", preview.Mode)
		assert.DirExists(t, filepath.Join(repoPath, "worktrees", "toberemoved"))

		stdout, err = executeCommand(createTestRootCmd(), "workspace", "remove", "--run", preview.PlanID)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Removed workspace")
		assert.NoDirExists(t, filepath.Join(repoPath, "worktrees", "toberemoved"))
	})
}
