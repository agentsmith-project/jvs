package gc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCollectorInternalTestRepo(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	_, err := repo.Init(repoPath, "test")
	require.NoError(t, err)
	return repoPath
}

func createCollectorInternalDeletedWorktreeSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	t.Helper()

	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)
	tempPath, err := wtMgr.Path("temp")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	require.NoError(t, wtMgr.Remove("temp"))
	return desc.SnapshotID
}

func requireTombstoneStateForInternalTest(t *testing.T, repoPath string, snapshotID model.SnapshotID, expected string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(snapshotID)+".json"))
	require.NoError(t, err)
	var tombstone model.Tombstone
	require.NoError(t, json.Unmarshal(data, &tombstone))
	require.Equal(t, expected, tombstone.GCState)
}

func TestCollectorRunDoesNotCommitWhenSnapshotParentFsyncFails(t *testing.T) {
	repoPath := setupCollectorInternalTestRepo(t)
	snapshotID := createCollectorInternalDeletedWorktreeSnapshot(t, repoPath)
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")

	originalFsyncDir := collectorFsyncDir
	collectorFsyncDir = func(dir string) error {
		if dir == snapshotsDir {
			return errors.New("injected snapshot parent fsync failure")
		}
		return fsutil.FsyncDir(dir)
	}
	t.Cleanup(func() {
		collectorFsyncDir = originalFsyncDir
	})

	collector := NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, snapshotID)

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fsync snapshot parent")
	requireTombstoneStateForInternalTest(t, repoPath, snapshotID, model.GCStateFailed)
}

func TestCollectorRunDoesNotCommitWhenDescriptorParentFsyncFails(t *testing.T) {
	repoPath := setupCollectorInternalTestRepo(t)
	snapshotID := createCollectorInternalDeletedWorktreeSnapshot(t, repoPath)
	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")

	originalFsyncDir := collectorFsyncDir
	collectorFsyncDir = func(dir string) error {
		if dir == descriptorsDir {
			return errors.New("injected descriptor parent fsync failure")
		}
		return fsutil.FsyncDir(dir)
	}
	t.Cleanup(func() {
		collectorFsyncDir = originalFsyncDir
	})

	collector := NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, snapshotID)

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fsync descriptor parent")
	requireTombstoneStateForInternalTest(t, repoPath, snapshotID, model.GCStateFailed)
}
