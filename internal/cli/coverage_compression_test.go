package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSnapshotWithMaxCompression tests snapshot with max compression.
func TestSnapshotWithMaxCompression(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "compressrepo")

	// Change into main worktree
	mainPath := filepath.Join(repoPath, "main")
	assert.NoError(t, os.Chdir(mainPath))

	// Create a file with compressible content
	data := make([]byte, 50000)
	for i := range data {
		data[i] = byte(i % 10)
	}
	assert.NoError(t, os.WriteFile("compressible.dat", data, 0644))

	t.Run("Snapshot with max compression", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "snapshot", "--compress", "max", "compressed max")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})
}

// TestSnapshotWithFastCompression tests snapshot with fast compression.
func TestSnapshotWithFastCompression(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "compressrepo2")

	// Change into main worktree
	mainPath := filepath.Join(repoPath, "main")
	assert.NoError(t, os.Chdir(mainPath))

	assert.NoError(t, os.WriteFile("fast.txt", []byte("fast compress test"), 0644))

	t.Run("Snapshot with fast compression", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "snapshot", "--compress", "fast", "compressed fast")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})
}

// TestSnapshotWithNoneCompression tests snapshot with no compression.
func TestSnapshotWithNoneCompression(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "compressrepo3")

	// Change into main worktree
	mainPath := filepath.Join(repoPath, "main")
	assert.NoError(t, os.Chdir(mainPath))

	assert.NoError(t, os.WriteFile("none.txt", []byte("no compression"), 0644))

	t.Run("Snapshot with no compression", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "snapshot", "--compress", "none", "no compression")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})
}
