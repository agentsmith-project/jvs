package repo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectMutationLockOwnerRecordsProcessIdentity(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	lock, err := AcquireMutationLock(repoPath, "test-holder")
	require.NoError(t, err)
	defer lock.Release()

	inspection, err := InspectMutationLock(repoPath)
	require.NoError(t, err)
	require.Equal(t, MutationLockHeld, inspection.Status)
	require.NotNil(t, inspection.Owner)

	hostname, err := os.Hostname()
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), inspection.Owner.PID)
	assert.Equal(t, hostname, inspection.Owner.Hostname)
	assert.Equal(t, "test-holder", inspection.Owner.Operation)
	assert.NotZero(t, inspection.Owner.CreatedAt)
	assert.False(t, inspection.SafeToRemove)

	data, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "locks", "repo.lock", "owner.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "created_at")
}

func TestInspectMutationLockOnlyMarksSameHostDeadOldPIDStale(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	hostname, err := os.Hostname()
	require.NoError(t, err)
	deadPID := 424242
	old := time.Now().UTC().Add(-2 * time.Hour)
	withMutationLockProcessLiveness(t, func(pid int) (processLiveness, error) {
		if pid == deadPID {
			return processGone, nil
		}
		if pid == os.Getpid() {
			return processAlive, nil
		}
		return processLivenessUnknown, errors.New("unexpected pid")
	})

	writeRepoLockOwner(t, repoPath, map[string]any{
		"operation":  "crashed checkpoint",
		"pid":        deadPID,
		"hostname":   hostname,
		"created_at": old,
	})
	inspection, err := InspectMutationLock(repoPath)
	require.NoError(t, err)
	assert.Equal(t, MutationLockStale, inspection.Status)
	assert.True(t, inspection.SafeToRemove)

	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "locks", "repo.lock")))

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
			writeRepoLockOwner(t, repoPath, tt.owner)

			inspection, err := InspectMutationLock(repoPath)
			require.NoError(t, err)
			assert.False(t, inspection.SafeToRemove)
			assert.NotEqual(t, MutationLockStale, inspection.Status)
		})
	}
}

func TestInspectMutationLockFailsClosedWhenProcessLivenessUnknown(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	hostname, err := os.Hostname()
	require.NoError(t, err)
	pid := 424242
	withMutationLockProcessLiveness(t, func(int) (processLiveness, error) {
		return processLivenessUnknown, errors.New("process status unavailable")
	})

	writeRepoLockOwner(t, repoPath, map[string]any{
		"operation":  "uncertain checkpoint",
		"pid":        pid,
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	})

	inspection, err := InspectMutationLock(repoPath)
	require.NoError(t, err)
	assert.Equal(t, MutationLockHeld, inspection.Status)
	assert.False(t, inspection.SafeToRemove)
	assert.Contains(t, inspection.Reason, "pid status unknown")
}

func TestRemoveStaleMutationLockRemovesSameHostDeadOldLock(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	hostname, err := os.Hostname()
	require.NoError(t, err)
	deadPID := 424242
	withMutationLockProcessLiveness(t, func(pid int) (processLiveness, error) {
		if pid == deadPID {
			return processGone, nil
		}
		return processLivenessUnknown, errors.New("unexpected pid")
	})

	writeRepoLockOwner(t, repoPath, map[string]any{
		"operation":  "crashed checkpoint",
		"pid":        deadPID,
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	})

	inspection, removed, err := RemoveStaleMutationLock(repoPath)
	require.NoError(t, err)
	assert.True(t, removed)
	assert.Equal(t, MutationLockStale, inspection.Status)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "locks", "repo.lock"))

	lock, err := AcquireMutationLock(repoPath, "post-cleanup")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
}

func TestRemoveStaleMutationLockRevalidatesOwnerBeforeRemoval(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	hostname, err := os.Hostname()
	require.NoError(t, err)
	stalePID := 424242
	writeRepoLockOwner(t, repoPath, map[string]any{
		"operation":  "crashed checkpoint",
		"pid":        stalePID,
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	})

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	var reacquired *MutationLock
	swapped := false
	withMutationLockProcessLiveness(t, func(pid int) (processLiveness, error) {
		switch {
		case pid == stalePID && !swapped:
			swapped = true
			require.NoError(t, os.RemoveAll(lockDir))
			var err error
			reacquired, err = AcquireMutationLock(repoPath, "new mutation")
			require.NoError(t, err)
			return processGone, nil
		case pid == stalePID:
			return processGone, nil
		case pid == os.Getpid():
			return processAlive, nil
		default:
			return processLivenessUnknown, errors.New("unexpected pid")
		}
	})

	inspection, removed, err := RemoveStaleMutationLock(repoPath)
	require.NoError(t, err)
	assert.True(t, swapped)
	assert.False(t, removed)
	assert.Equal(t, MutationLockHeld, inspection.Status)
	require.NotNil(t, inspection.Owner)
	assert.Equal(t, "new mutation", inspection.Owner.Operation)
	assert.DirExists(t, lockDir)
	require.NotNil(t, reacquired)
	require.NoError(t, reacquired.Release())
}

func writeRepoLockOwner(t *testing.T, repoPath string, owner any) {
	t.Helper()

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	require.NoError(t, os.MkdirAll(lockDir, 0700))
	data, err := json.MarshalIndent(owner, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "owner.json"), data, 0600))
}

func withMutationLockProcessLiveness(t *testing.T, checker processLivenessChecker) {
	t.Helper()

	previous := mutationLockProcessLiveness
	mutationLockProcessLiveness = checker
	t.Cleanup(func() {
		mutationLockProcessLiveness = previous
	})
}
