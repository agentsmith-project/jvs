package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWorktreePathCommand tests the worktree path command.
func TestWorktreePathCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "wtpathrepo")

	// Change to repo directory
	assert.NoError(t, os.Chdir(repoPath))

	t.Run("Worktree path with name", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "worktree", "path", "main")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "main")
	})

	// Change to main worktree and test path without args
	mainPath := filepath.Join(repoPath, "main")
	assert.NoError(t, os.Chdir(mainPath))

	t.Run("Worktree path from inside worktree", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "path")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})
}

// TestWorktreeRenameCommand tests the worktree rename command.
func TestWorktreeRenameCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "wtrename")

	// Change to repo directory
	assert.NoError(t, os.Chdir(repoPath))

	// Create a worktree first
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "worktree", "create", "oldname")
	assert.NoError(t, err)

	t.Run("Rename worktree", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "rename", "oldname", "newname")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Renamed")
	})

	t.Run("Rename with JSON output", func(t *testing.T) {
		cmd4 := createTestRootCmd()
		_, err = executeCommand(cmd4, "worktree", "create", "oldname2")
		assert.NoError(t, err)

		// Note: rename doesn't output JSON even with --json flag
		cmd5 := createTestRootCmd()
		stdout, err := executeCommand(cmd5, "worktree", "rename", "oldname2", "newname2")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})
}

// TestWorktreeForkCommand tests the worktree fork command.
func TestWorktreeForkCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "wtforkrepo")

	// Change into main worktree
	mainPath := filepath.Join(repoPath, "main")
	assert.NoError(t, os.Chdir(mainPath))

	// Create a snapshot
	assert.NoError(t, os.WriteFile("forkfile.txt", []byte("fork content"), 0644))
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "snapshot", "fork base")
	assert.NoError(t, err)

	t.Run("Fork from current position (auto-name)", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "fork")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Created worktree")
	})

	t.Run("Fork with custom name", func(t *testing.T) {
		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "worktree", "fork", "custom-fork")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "custom-fork")
	})

	t.Run("Fork with JSON output", func(t *testing.T) {
		cmd5 := createTestRootCmd()
		stdout, err := executeCommand(cmd5, "worktree", "fork", "json-fork", "--json")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})
}

// TestWorktreeListCommand tests the worktree list command.
func TestWorktreeListCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "wtlistrepo")

	// Change to repo directory
	assert.NoError(t, os.Chdir(repoPath))

	t.Run("List worktrees", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "worktree", "list")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "main")
	})

	t.Run("List worktrees with JSON", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "list", "--json")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "[")
	})
}

// TestInitCommandJSON tests init command with JSON output.
func TestInitCommandJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))

	t.Run("Init with JSON output", func(t *testing.T) {
		assert.NoError(t, os.Mkdir("jsonrepo", 0755))
		cmd := createTestRootCmd()
		stdout, err := executeCommand(cmd, "init", "jsonrepo", "--json")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "repo_root")
		assert.Contains(t, stdout, "repo_id")
		assert.Contains(t, stdout, "folder")
		assert.Contains(t, stdout, "workspace")
	})
}

// TestWorktreeCreateForce tests worktree remove with force flag.
func TestWorktreeCreateForce(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "wtforceremove")

	// Change to repo directory
	assert.NoError(t, os.Chdir(repoPath))

	// Create a worktree
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "worktree", "create", "toberemoved")
	assert.NoError(t, err)

	t.Run("Remove worktree", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "remove", "toberemoved")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Removed")
	})
}
