package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffCommand_TwoSnapshots(t *testing.T) {
	// This is an integration test that requires building the binary
	t.Skip("requires full build - manual testing only for now")
}

// TestDiff_SimpleIntegration is a manual integration test helper
func TestDiff_SimpleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	jvsBin := "/tmp/jvs"
	if _, err := os.Stat(jvsBin); os.IsNotExist(err) {
		t.Skip("jvs binary not found at /tmp/jvs; build first with: go build -o /tmp/jvs ./cmd/jvs")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "testrepo")

	cmd := exec.Command(jvsBin, "init", "testrepo")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "init failed: %s", output)

	mainPath := filepath.Join(repoPath, "main")

	// Create initial file
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("content1"), 0644))

	cmd = exec.Command(jvsBin, "snapshot", "first snapshot")
	cmd.Dir = mainPath
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "first snapshot failed: %s", output)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("content1-modified"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("new file"), 0644))

	cmd = exec.Command(jvsBin, "snapshot", "second snapshot")
	cmd.Dir = mainPath
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "second snapshot failed: %s", output)

	cmd = exec.Command(jvsBin, "--json", "checkpoint", "list")
	cmd.Dir = mainPath
	output, err = cmd.Output()
	require.NoError(t, err, "checkpoint list failed: %s", output)

	env := decodeContractEnvelope(t, string(output))
	require.True(t, env.OK, string(output))
	assert.Equal(t, "checkpoint list", env.Command)
	assert.NotContains(t, string(env.Data), "snapshot_id")
	assert.NotContains(t, string(env.Data), "worktree")

	var checkpoints []publicCheckpointRecord
	require.NoError(t, json.Unmarshal(env.Data, &checkpoints), string(output))
	require.GreaterOrEqual(t, len(checkpoints), 2)

	checkpointIDsByNote := make(map[string]string, len(checkpoints))
	for _, checkpoint := range checkpoints {
		require.NotEmpty(t, checkpoint.CheckpointID)
		checkpointIDsByNote[checkpoint.Note] = checkpoint.CheckpointID
	}
	firstID := checkpointIDsByNote["first snapshot"]
	secondID := checkpointIDsByNote["second snapshot"]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)

	cmd = exec.Command(jvsBin, "diff", "--stat", firstID, secondID)
	cmd.Dir = mainPath
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "diff failed: %s", output)

	diffOutput := string(output)
	// Should show added, removed, modified summary
	assert.Contains(t, diffOutput, "Added")
	assert.Contains(t, diffOutput, "Modified")
}
