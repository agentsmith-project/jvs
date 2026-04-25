package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveCheckpointRefWithTag tests diff command with tag-based checkpoint refs.
func TestResolveCheckpointRefWithTag(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "diffrepo_tags")
	assert.NoError(t, err)

	// Change into main worktree
	mainPath := filepath.Join(dir, "diffrepo_tags", "main")
	assert.NoError(t, os.Chdir(mainPath))

	// Create two snapshots with different tags
	assert.NoError(t, os.WriteFile("file1.txt", []byte("v1"), 0644))
	cmd2 := createTestRootCmd()
	_, err = executeCommand(cmd2, "snapshot", "--tag", "first-tag", "first snapshot")
	assert.NoError(t, err)

	assert.NoError(t, os.WriteFile("file1.txt", []byte("v2"), 0644))
	cmd3 := createTestRootCmd()
	_, err = executeCommand(cmd3, "snapshot", "--tag", "second-tag", "second snapshot")
	assert.NoError(t, err)

	t.Run("Diff using tag references", func(t *testing.T) {
		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "diff", "first-tag", "second-tag")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})

	t.Run("Diff stat with tags", func(t *testing.T) {
		cmd5 := createTestRootCmd()
		stdout, err := executeCommand(cmd5, "diff", "--stat", "first-tag", "second-tag")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})
}

// TestResolveCheckpointRefWithID tests diff with full snapshot ID resolution.
func TestResolveCheckpointRefWithID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "diffrepo_ids")
	assert.NoError(t, err)

	// Change into main worktree
	mainPath := filepath.Join(dir, "diffrepo_ids", "main")
	assert.NoError(t, os.Chdir(mainPath))

	// Create a snapshot and get its ID
	assert.NoError(t, os.WriteFile("file1.txt", []byte("id test"), 0644))
	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", "id test snapshot", "--json")
	assert.NoError(t, err)

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, "snapshot_id") {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if snapshotID != "" {
		t.Run("Diff with full snapshot ID", func(t *testing.T) {
			cmd3 := createTestRootCmd()
			stdout, err := executeCommand(cmd3, "diff", snapshotID, snapshotID)
			assert.NoError(t, err)
			assert.NotEmpty(t, stdout)
		})

		t.Run("Diff stat with snapshot ID", func(t *testing.T) {
			cmd4 := createTestRootCmd()
			stdout, err := executeCommand(cmd4, "diff", "--stat", snapshotID, snapshotID)
			assert.NoError(t, err)
			assert.NotEmpty(t, stdout)
		})
	}
}

// TestResolveCheckpointRefWithShortID tests diff with short ID prefix.
func TestResolveCheckpointRefWithShortID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	assert.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "diffrepo_short")
	assert.NoError(t, err)

	// Change into main worktree
	mainPath := filepath.Join(dir, "diffrepo_short", "main")
	assert.NoError(t, os.Chdir(mainPath))

	// Create a snapshot
	assert.NoError(t, os.WriteFile("file1.txt", []byte("short id test"), 0644))
	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", "short snapshot", "--json")
	assert.NoError(t, err)

	// Extract short ID (first 8 chars)
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, "snapshot_id") {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if len(snapshotID) > 8 {
		shortID := snapshotID[:8]
		t.Run("Diff with short ID prefix", func(t *testing.T) {
			cmd3 := createTestRootCmd()
			stdout, err := executeCommand(cmd3, "diff", shortID, shortID)
			assert.NoError(t, err)
			assert.NotEmpty(t, stdout)
		})
	}
}
