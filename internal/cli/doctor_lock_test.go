package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorRepairRuntimeCleansStaleRepoLockAndAllowsCheckpoint(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "stale-lock-recovery")

	writeCLIRepoLockOwner(t, repoPath, staleCLISameHostOwner(t, "crashed checkpoint"))

	stdout, err := runPublicCLI(t, "doctor", "--repair-runtime")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, "Repair clean_locks")
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "locks", "repo.lock"))

	require.NoError(t, os.WriteFile("after.txt", []byte("mutation works"), 0644))
	stdout, err = runPublicCLI(t, "checkpoint", "after stale lock recovery")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, "Created checkpoint")
}

func staleCLISameHostOwner(t *testing.T, operation string) map[string]any {
	t.Helper()

	hostname, err := os.Hostname()
	require.NoError(t, err)
	return map[string]any{
		"operation":  operation,
		"pid":        exitedCLIChildPID(t),
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	}
}

func writeCLIRepoLockOwner(t *testing.T, repoPath string, owner any) {
	t.Helper()

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	require.NoError(t, os.MkdirAll(lockDir, 0700))
	data, err := json.MarshalIndent(owner, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "owner.json"), data, 0600))
}

func exitedCLIChildPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}
