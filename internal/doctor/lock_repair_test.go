package doctor_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorRepairCleanLocksRemovesSameHostDeadPIDAndAllowsMutation(t *testing.T) {
	repoPath := setupTestRepo(t)

	writeDoctorRepoLockOwner(t, repoPath, staleSameHostOwner(t, "crashed checkpoint"))

	results, err := doctor.NewDoctor(repoPath).Repair([]string{"clean_locks"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "clean_locks", results[0].Action)
	assert.True(t, results[0].Success)
	assert.Equal(t, 1, results[0].Cleaned)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "locks", "repo.lock"))

	lock, err := repo.AcquireMutationLock(repoPath, "post-repair mutation")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
}

func TestDoctorRepairCleanLocksPreservesUnsafeLocks(t *testing.T) {
	repoPath := setupTestRepo(t)
	hostname, err := os.Hostname()
	require.NoError(t, err)
	deadPID := exitedDoctorChildPID(t)
	old := time.Now().UTC().Add(-2 * time.Hour)

	for _, tt := range []struct {
		name  string
		owner map[string]any
	}{
		{
			name: "live same-host pid",
			owner: map[string]any{
				"operation":  "live checkpoint",
				"pid":        os.Getpid(),
				"hostname":   hostname,
				"created_at": old,
			},
		},
		{
			name: "dead pid on different host",
			owner: map[string]any{
				"operation":  "remote checkpoint",
				"pid":        deadPID,
				"hostname":   "different-host.example",
				"created_at": old,
			},
		},
		{
			name: "dead pid with unknown host",
			owner: map[string]any{
				"operation":  "unknown checkpoint",
				"pid":        deadPID,
				"created_at": old,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "locks", "repo.lock")))
			writeDoctorRepoLockOwner(t, repoPath, tt.owner)

			results, err := doctor.NewDoctor(repoPath).Repair([]string{"clean_locks"})
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.True(t, results[0].Success)
			assert.Zero(t, results[0].Cleaned)
			assert.DirExists(t, filepath.Join(repoPath, ".jvs", "locks", "repo.lock"))

			_, err = repo.AcquireMutationLock(repoPath, "blocked mutation")
			require.ErrorIs(t, err, errclass.ErrRepoBusy)
		})
	}
}

func TestDoctorMalformedLockOwnerReportsFindingAndFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	require.NoError(t, os.MkdirAll(lockDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "owner.json"), []byte("{malformed"), 0600))

	result, err := doctor.NewDoctor(repoPath).Check(false)
	require.NoError(t, err)
	assertFindingCode(t, result, "lock", "E_REPO_LOCK_OWNER_INVALID")

	results, err := doctor.NewDoctor(repoPath).Repair([]string{"clean_locks"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "clean_locks", results[0].Action)
	assert.False(t, results[0].Success)
	assert.Zero(t, results[0].Cleaned)
	assert.DirExists(t, lockDir)

	_, err = repo.AcquireMutationLock(repoPath, "blocked mutation")
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
}

func staleSameHostOwner(t *testing.T, operation string) map[string]any {
	t.Helper()

	hostname, err := os.Hostname()
	require.NoError(t, err)
	return map[string]any{
		"operation":  operation,
		"pid":        exitedDoctorChildPID(t),
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	}
}

func writeDoctorRepoLockOwner(t *testing.T, repoPath string, owner any) {
	t.Helper()

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	require.NoError(t, os.MkdirAll(lockDir, 0700))
	data, err := json.MarshalIndent(owner, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "owner.json"), data, 0600))
}

func exitedDoctorChildPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}
