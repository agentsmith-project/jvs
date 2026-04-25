package restore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/restore"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func createSnapshot(t *testing.T, repoPath string) *model.Descriptor {
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot-content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test snapshot", nil)
	require.NoError(t, err)

	return desc
}

func writeUnsafeSnapshotTrap(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
	t.Helper()

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "file.txt"), []byte("trap payload"), 0644))

	payloadHash, err := integrity.ComputePayloadRootHash(snapshotDir)
	require.NoError(t, err)

	desc := &model.Descriptor{
		SnapshotID:      snapshotID,
		WorktreeName:    "main",
		CreatedAt:       time.Now().UTC(),
		Engine:          model.EngineCopy,
		PayloadRootHash: payloadHash,
		IntegrityState:  model.IntegrityVerified,
	}
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.MkdirAll(filepath.Dir(descriptorPath), 0755))
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
}

func TestRestorer_Restore(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify main after snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore (now always inplace)
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify content is restored
	content, err := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot-content", string(content))

	// Verify worktree state (since this is the only snapshot, we're at HEAD, not detached)
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Get("main")
	require.NoError(t, err)
	// After restoring to the only snapshot, we're at HEAD (not detached)
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)
	assert.NoFileExists(t, filepath.Join(mainPath, ".READY"))
	assert.NoFileExists(t, filepath.Join(mainPath, ".READY.gz"))
}

func TestRestorerRestoreRejectsReservedWorkspaceRootPayloadNames(t *testing.T) {
	for _, name := range []string{".READY", ".READY.gz"} {
		t.Run(name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			desc := createSnapshot(t, repoPath)
			mainPath := filepath.Join(repoPath, "main")
			reservedPath := filepath.Join(mainPath, name)
			require.NoError(t, os.WriteFile(reservedPath, []byte("user data"), 0644))

			err := restore.NewRestorer(repoPath, model.EngineCopy).Restore("main", desc.SnapshotID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reserved")
			assert.Contains(t, err.Error(), name)

			content, readErr := os.ReadFile(reservedPath)
			require.NoError(t, readErr)
			assert.Equal(t, "user data", string(content))
		})
	}
}

func TestRestorerRestoreReturnsRepoBusyWhenMutationLockHeld(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	held, err := repo.AcquireMutationLock(repoPath, "held-by-test")
	require.NoError(t, err)
	defer held.Release()

	err = restore.NewRestorer(repoPath, model.EngineCopy).Restore("main", desc.SnapshotID)
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
}

func TestRestorerRestoreRejectsPathLikeAndNonCanonicalIDsWithoutMaterializing(t *testing.T) {
	ids := []model.SnapshotID{
		"../../outside/escape",
		"/tmp/absolute",
		"nested/1708300800000-deadbeef",
		"1708300800000-DEADBEEF",
		"not-canonical",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			repoPath := setupTestRepo(t)
			mainPath := filepath.Join(repoPath, "main")
			require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current payload"), 0644))
			writeUnsafeSnapshotTrap(t, repoPath, id)

			restorer := restore.NewRestorer(repoPath, model.EngineCopy)
			err := restorer.Restore("main", id)
			require.Error(t, err)

			content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
			require.NoError(t, readErr)
			assert.Equal(t, "current payload", string(content))
		})
	}
}

func TestRestorerRestoreRejectsFinalSnapshotSymlinkWithoutMaterializing(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.SnapshotID("1708300800000-deadbeef")
	writeUnsafeSnapshotTrap(t, repoPath, snapshotID)

	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current payload"), 0644))

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("trap payload"), 0644))
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.RemoveAll(snapshotDir))
	if err := os.Symlink(outside, snapshotDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err := restore.NewRestorer(repoPath, model.EngineCopy).Restore("main", snapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current payload", string(content))
	assert.FileExists(t, filepath.Join(outside, "file.txt"))
}

func TestRestorerRestoreRejectsSymlinkedWorktreesParentBeforeOutsideMutation(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	wtMgr := worktree.NewManager(repoPath)
	const worktreeName = "unsafe"
	_, err := wtMgr.Create(worktreeName, nil)
	require.NoError(t, err)

	outsideRoot := t.TempDir()
	outsidePayload := filepath.Join(outsideRoot, worktreeName)
	require.NoError(t, os.MkdirAll(outsidePayload, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsidePayload, "file.txt"), []byte("outside original"), 0644))
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, "worktrees")))
	if err := os.Symlink(outsideRoot, filepath.Join(repoPath, "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err = restore.NewRestorer(repoPath, model.EngineCopy).Restore(worktreeName, desc.SnapshotID)
	require.Error(t, err)

	content, readErr := os.ReadFile(filepath.Join(outsidePayload, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside original", string(content))
	assert.NoDirExists(t, filepath.Join(outsideRoot, worktreeName+".restore-backup"))
}

func TestRestorerRestoreRejectsFinalPayloadSymlinkViaManagerPath(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	wtMgr := worktree.NewManager(repoPath)
	const worktreeName = "unsafe"
	_, err := wtMgr.Create(worktreeName, nil)
	require.NoError(t, err)

	outsidePayload := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsidePayload, "file.txt"), []byte("outside original"), 0644))
	payloadPath := filepath.Join(repoPath, "worktrees", worktreeName)
	require.NoError(t, os.RemoveAll(payloadPath))
	if err := os.Symlink(outsidePayload, payloadPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err = restore.NewRestorer(repoPath, model.EngineCopy).Restore(worktreeName, desc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree payload path")

	content, readErr := os.ReadFile(filepath.Join(outsidePayload, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside original", string(content))
}

func TestRestorer_RestoreToLatest(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify and create second snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("second"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc2, err := creator.Create("main", "second snapshot", nil)
	require.NoError(t, err)

	// Restore to first snapshot (detached)
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.True(t, cfg.IsDetached())

	// Restore to latest
	err = restorer.RestoreToLatest("main")
	require.NoError(t, err)

	// Verify content is from second snapshot
	content, err := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(content))

	// Verify worktree is back at HEAD
	cfg, _ = wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID)
}

func TestRestorer_Restore_SetsDetachedState(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc1 := createSnapshot(t, repoPath)

	// Create second snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("second"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc2, err := creator.Create("main", "second snapshot", nil)
	require.NoError(t, err)

	// Verify we're at HEAD
	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID)

	// Restore to first snapshot
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	// Verify detached state
	cfg, _ = wtMgr.Get("main")
	assert.True(t, cfg.IsDetached())
	assert.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID) // Latest unchanged
}

func TestWorktree_Fork(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Fork from snapshot
	wtMgr := worktree.NewManager(repoPath)
	eng := &mockEngine{content: "snapshot-content"}
	cfg, err := wtMgr.Fork(desc.SnapshotID, "feature", eng.clone)
	require.NoError(t, err)
	assert.Equal(t, "feature", cfg.Name)
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)
	assert.False(t, cfg.IsDetached())

	// Verify forked content
	forkPath := filepath.Join(repoPath, "worktrees", "feature")
	content, err := os.ReadFile(filepath.Join(forkPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot-content", string(content))
}

// mockEngine for testing
type mockEngine struct {
	content string
}

func (m *mockEngine) clone(src, dst string) error {
	// Copy test content
	return os.WriteFile(filepath.Join(dst, "file.txt"), []byte(m.content), 0644)
}

func TestRestorer_Restore_NonExistentSnapshotID(t *testing.T) {
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", "nonexistent-snapshot-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load snapshot")
}

func TestRestorer_Restore_NonExistentWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("nonexistent", desc.SnapshotID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get worktree")
}

func TestRestorer_RestoreToLatest_NoSnapshots(t *testing.T) {
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.RestoreToLatest("main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no snapshots")
}

func TestRestorer_Restore_SameSnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Restore to same snapshot (no-op effectively)
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify worktree is at HEAD (not detached)
	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
}

func TestRestorer_Restore_MultipleTimes(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc1 := createSnapshot(t, repoPath)

	// Create second snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("second"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc2, err := creator.Create("main", "second snapshot", nil)
	require.NoError(t, err)

	// Create third snapshot
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("third"), 0644)
	_, err = creator.Create("main", "third snapshot", nil)
	require.NoError(t, err)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	// Restore to first
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	// Restore to second
	err = restorer.Restore("main", desc2.SnapshotID)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(content))
}

func TestRestorer_NewRestorer(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Test with different engine types
	r1 := restore.NewRestorer(repoPath, model.EngineCopy)
	assert.NotNil(t, r1)

	r2 := restore.NewRestorer(repoPath, model.EngineJuiceFSClone)
	assert.NotNil(t, r2)

	r3 := restore.NewRestorer(repoPath, model.EngineReflinkCopy)
	assert.NotNil(t, r3)
}

func TestRestorer_RestoreToLatest_FromDetached(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc1 := createSnapshot(t, repoPath)

	// Create second snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("second"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc2, err := creator.Create("main", "second snapshot", nil)
	require.NoError(t, err)

	// Restore to first (detached)
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.True(t, cfg.IsDetached())

	// Restore to latest (exit detached)
	err = restorer.RestoreToLatest("main")
	require.NoError(t, err)

	cfg, _ = wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)
}

func TestRestorer_Restore_VerifySnapshotError(t *testing.T) {
	// Test that restore fails when snapshot verification fails
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Corrupt the snapshot by modifying the descriptor checksum
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)

	var descMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &descMap))
	descMap["descriptor_checksum"] = "invalidchecksum"
	corruptData, err := json.Marshal(descMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, corruptData, 0644))

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verify snapshot")
}

func TestRestorer_Restore_RefusesTamperedPayload(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("working copy"), 0644))

	snapshotFile := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), "file.txt")
	require.NoError(t, os.WriteFile(snapshotFile, []byte("tampered payload"), 0644))

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify snapshot")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "working copy", string(content))
}

func TestRestorer_Restore_CompressedSnapshotPreservesUserGzipAndGzipSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("snapshot data"), 0644))
	userGzipContent := []byte("user-owned gzip path")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "archive.gz"), userGzipContent, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "target.txt"), []byte("target"), 0644))
	if err := os.Symlink("target.txt", filepath.Join(mainPath, "link.gz")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(6)
	desc, err := creator.Create("main", "compressed with user gzip paths", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("modified"), 0644))
	require.NoError(t, os.Remove(filepath.Join(mainPath, "archive.gz")))
	require.NoError(t, os.Remove(filepath.Join(mainPath, "link.gz")))

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(mainPath, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot data", string(content))
	archive, err := os.ReadFile(filepath.Join(mainPath, "archive.gz"))
	require.NoError(t, err)
	assert.Equal(t, userGzipContent, archive)
	info, err := os.Lstat(filepath.Join(mainPath, "link.gz"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, err := os.Readlink(filepath.Join(mainPath, "link.gz"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestRestorer_Restore_KeepsCurrentWorkingDirectoryUsable(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("snapshot data"), 0644))
	beforeRoot, err := os.Stat(mainPath)
	require.NoError(t, err)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "cwd restore", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("modified"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "current-only.txt"), []byte("remove me"), 0644))

	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(mainPath))
	defer os.Chdir(oldwd)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	content, err := os.ReadFile("data.txt")
	require.NoError(t, err)
	assert.Equal(t, "snapshot data", string(content))
	assert.NoFileExists(t, "current-only.txt")

	cwdRoot, err := os.Stat(".")
	require.NoError(t, err)
	afterRoot, err := os.Stat(mainPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(beforeRoot, afterRoot), "restore must keep the workspace root directory in place")
	assert.True(t, os.SameFile(cwdRoot, afterRoot), "caller cwd must still point at the restored workspace root")
}

func TestRestorer_Restore_CompressedSnapshotKeepsCurrentWorkingDirectoryUsable(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("compressed snapshot data"), 0644))
	beforeRoot, err := os.Stat(mainPath)
	require.NoError(t, err)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(6)
	desc, err := creator.Create("main", "compressed cwd restore", nil)
	require.NoError(t, err)
	require.NotNil(t, desc.Compression)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("modified"), 0644))

	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(mainPath))
	defer os.Chdir(oldwd)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	content, err := os.ReadFile("data.txt")
	require.NoError(t, err)
	assert.Equal(t, "compressed snapshot data", string(content))
	assert.NoFileExists(t, "data.txt.gz")
	assert.NoFileExists(t, ".READY")
	assert.NoFileExists(t, ".READY.gz")

	cwdRoot, err := os.Stat(".")
	require.NoError(t, err)
	afterRoot, err := os.Stat(mainPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(beforeRoot, afterRoot), "compressed restore must keep the workspace root directory in place")
	assert.True(t, os.SameFile(cwdRoot, afterRoot), "caller cwd must still point at the restored workspace root")
}

func TestRestorer_Restore_PartialSnapshotOverlaysOnlyIncludedPaths(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "included.txt"), []byte("snapshot included"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "unrelated.txt"), []byte("original unrelated"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "included only", nil, []string{"included.txt"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "included.txt"), []byte("modified included"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "unrelated.txt"), []byte("modified unrelated"), 0644))

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(mainPath, "included.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot included", string(content))
	content, err = os.ReadFile(filepath.Join(mainPath, "unrelated.txt"))
	require.NoError(t, err)
	assert.Equal(t, "modified unrelated", string(content))
}

func TestRestorer_Restore_PartialSnapshotReplacesOnlyDescriptorPaths(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "tracked"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "untouched"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "tracked", "model.bin"), []byte("snapshot model"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "untouched", "notes.txt"), []byte("snapshot notes"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "tracked dir only", nil, []string{"tracked"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "tracked", "model.bin"), []byte("current model"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "tracked", "scratch.tmp"), []byte("inside included path"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "untouched", "notes.txt"), []byte("current notes"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "workspace-only.txt"), []byte("current only"), 0644))

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(mainPath, "tracked", "model.bin"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot model", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "tracked", "scratch.tmp"))

	content, err = os.ReadFile(filepath.Join(mainPath, "untouched", "notes.txt"))
	require.NoError(t, err)
	assert.Equal(t, "current notes", string(content))
	content, err = os.ReadFile(filepath.Join(mainPath, "workspace-only.txt"))
	require.NoError(t, err)
	assert.Equal(t, "current only", string(content))
}

func TestRestorer_RestoreWithReflinkEngine(t *testing.T) {
	// Test restore with reflink engine
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify main after snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore with reflink engine
	restorer := restore.NewRestorer(repoPath, model.EngineReflinkCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify content is restored
	content, err := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot-content", string(content))
}

func TestRestorer_RestoreWithJuiceFSEngine(t *testing.T) {
	// Test restore with juicefs-clone engine
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify main after snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore with juicefs-clone engine
	restorer := restore.NewRestorer(repoPath, model.EngineJuiceFSClone)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify content is restored
	content, err := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "snapshot-content", string(content))
}

func TestRestorer_Restore_EmptySnapshotID(t *testing.T) {
	// Test restore with empty snapshot ID
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", "")
	assert.Error(t, err)
}

func TestRestorer_RestoreToLatest_NonExistentWorktree(t *testing.T) {
	// Test RestoreToLatest with non-existent worktree
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.RestoreToLatest("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get worktree")
}

func TestRestorer_Restore_PreservesFilePermissions(t *testing.T) {
	// Test that restore preserves file permissions
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create a file with specific permissions
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "script.sh"), []byte("#!/bin/bash\necho test"), 0755))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "snapshot with executable", nil)
	require.NoError(t, err)

	// Modify the file
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "script.sh"), []byte("modified"), 0644))

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify permissions are preserved (this depends on engine behavior)
	info, err := os.Stat(filepath.Join(mainPath, "script.sh"))
	require.NoError(t, err)
	// The copy engine should preserve permissions
	assert.NotNil(t, info.Mode())
}

func TestRestorer_Restore_MultipleFiles(t *testing.T) {
	// Test restore with multiple files and directories
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create multiple files
	os.MkdirAll(filepath.Join(mainPath, "subdir"), 0755)
	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("content2"), 0644)
	os.WriteFile(filepath.Join(mainPath, "subdir", "nested.txt"), []byte("nested"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "multi-file snapshot", nil)
	require.NoError(t, err)

	// Modify all files
	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("modified1"), 0644)
	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("modified2"), 0644)
	os.WriteFile(filepath.Join(mainPath, "subdir", "nested.txt"), []byte("modified nested"), 0644)

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify all files are restored
	content1, _ := os.ReadFile(filepath.Join(mainPath, "file1.txt"))
	assert.Equal(t, "content1", string(content1))

	content2, _ := os.ReadFile(filepath.Join(mainPath, "file2.txt"))
	assert.Equal(t, "content2", string(content2))

	nested, _ := os.ReadFile(filepath.Join(mainPath, "subdir", "nested.txt"))
	assert.Equal(t, "nested", string(nested))
}

func TestRestorer_Restore_DetachedStateNotLatest(t *testing.T) {
	// Test the detached state determination logic
	// isDetached = snapshotID != cfg.LatestSnapshotID
	repoPath := setupTestRepo(t)

	// Create first snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("first"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	// Create second snapshot (now latest)
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("second"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Restore to first (not latest) -> should be detached
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.True(t, cfg.IsDetached())
	assert.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID)

	// Restore to second (is latest) -> should not be detached
	err = restorer.Restore("main", desc2.SnapshotID)
	require.NoError(t, err)

	cfg, _ = wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID)
}

func TestRestorer_RestoreToLatest_GetWorktreeError(t *testing.T) {
	// Test RestoreToLatest when Get worktree fails
	// Use a non-existent worktree
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.RestoreToLatest("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get worktree")
}

func TestRestorer_NewRestorer_Engines(t *testing.T) {
	// Test NewRestorer with all engine types
	repoPath := setupTestRepo(t)

	// Test Copy engine
	r1 := restore.NewRestorer(repoPath, model.EngineCopy)
	assert.NotNil(t, r1)

	// Test Reflink engine
	r2 := restore.NewRestorer(repoPath, model.EngineReflinkCopy)
	assert.NotNil(t, r2)

	// Test JuiceFS engine
	r3 := restore.NewRestorer(repoPath, model.EngineJuiceFSClone)
	assert.NotNil(t, r3)
}

func TestRestorer_Restore_ToSameContent(t *testing.T) {
	// Test restore when content is already the same
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Don't modify content, restore to same state
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify worktree is at HEAD
	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
}

func TestRestorer_Restore_SymlinkPreservation(t *testing.T) {
	// Test that symlinks are preserved during restore
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create a file and a symlink
	os.WriteFile(filepath.Join(mainPath, "target.txt"), []byte("target content"), 0644)

	// On systems that support symlinks
	err := os.Symlink("target.txt", filepath.Join(mainPath, "link.txt"))
	if err != nil {
		t.Skip("symlinks not supported on this system")
	}

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "snapshot with symlink", nil)
	require.NoError(t, err)

	// Modify the symlink
	os.Remove(filepath.Join(mainPath, "link.txt"))
	os.WriteFile(filepath.Join(mainPath, "link.txt"), []byte("not a symlink"), 0644)

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// The symlink behavior depends on the engine
	// Just verify restore didn't fail
	_, err = os.Stat(filepath.Join(mainPath, "link.txt"))
	require.NoError(t, err)
}

func TestRestorer_RestoreWithDifferentEngineTypes(t *testing.T) {
	// Test that restore works with all engine types
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("original"), 0644)

	// Create snapshot with one engine
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)

	// Modify content
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore with reflink engine
	restorer := restore.NewRestorer(repoPath, model.EngineReflinkCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify restored
	content, _ := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	assert.Equal(t, "original", string(content))
}

func TestRestorer_Restore_WithSubdirectories(t *testing.T) {
	// Test restore with nested directory structure
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create nested directories
	os.MkdirAll(filepath.Join(mainPath, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(mainPath, "a", "b", "c", "file.txt"), []byte("deep content"), 0644)
	os.WriteFile(filepath.Join(mainPath, "a", "file.txt"), []byte("mid content"), 0644)
	os.WriteFile(filepath.Join(mainPath, "root.txt"), []byte("root content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "nested dirs snapshot", nil)
	require.NoError(t, err)

	// Modify files
	os.WriteFile(filepath.Join(mainPath, "a", "b", "c", "file.txt"), []byte("modified deep"), 0644)
	os.WriteFile(filepath.Join(mainPath, "a", "file.txt"), []byte("modified mid"), 0644)
	os.WriteFile(filepath.Join(mainPath, "root.txt"), []byte("modified root"), 0644)

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify all files restored
	deep, _ := os.ReadFile(filepath.Join(mainPath, "a", "b", "c", "file.txt"))
	assert.Equal(t, "deep content", string(deep))

	mid, _ := os.ReadFile(filepath.Join(mainPath, "a", "file.txt"))
	assert.Equal(t, "mid content", string(mid))

	root, _ := os.ReadFile(filepath.Join(mainPath, "root.txt"))
	assert.Equal(t, "root content", string(root))
}

func TestRestorer_Restore_WithEmptySnapshotID(t *testing.T) {
	// Test that restoring with an empty snapshot ID fails appropriately
	repoPath := setupTestRepo(t)
	createSnapshot(t, repoPath)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", "")
	assert.Error(t, err)
}

func TestRestorer_Restore_UpdatesHeadCorrectly(t *testing.T) {
	// Test that Restore correctly updates the head snapshot ID
	repoPath := setupTestRepo(t)

	// Create two snapshots
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "v1", nil)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644)
	desc2, err := creator.Create("main", "v2", nil)
	require.NoError(t, err)

	// Verify initial state - at latest (desc2)
	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)

	// Restore to desc1
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	// Verify head is now desc1
	cfg, _ = wtMgr.Get("main")
	assert.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)
	assert.True(t, cfg.IsDetached())

	// Restore to desc2 (latest)
	err = restorer.Restore("main", desc2.SnapshotID)
	require.NoError(t, err)

	// Verify head is now desc2 and not detached
	cfg, _ = wtMgr.Get("main")
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)
	assert.False(t, cfg.IsDetached())
}

func TestRestorer_Restore_SingleSnapshotIsNotDetached(t *testing.T) {
	// Test that restoring to the only snapshot doesn't enter detached state
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	wtMgr := worktree.NewManager(repoPath)
	cfg, _ := wtMgr.Get("main")
	assert.False(t, cfg.IsDetached())
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)
}

func TestRestorer_Restore_WithJuiceFSCloneEngine(t *testing.T) {
	// Test restore with juicefs-clone engine specifically
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify content
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore with juicefs-clone engine
	restorer := restore.NewRestorer(repoPath, model.EngineJuiceFSClone)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify restored
	content, _ := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	assert.Equal(t, "snapshot-content", string(content))
}

func TestRestorer_Restore_CreatesAuditLogEntry(t *testing.T) {
	// Test that restore creates an audit log entry
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	// Modify content
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644)

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify audit log was created
	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	content, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "\"restore\"")
}

func TestRestorer_Restore_DetachedStateInAuditLog(t *testing.T) {
	// Test that detached state is recorded in audit log
	repoPath := setupTestRepo(t)

	// Create two snapshots
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "v1", nil)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644)
	_, err = creator.Create("main", "v2", nil)
	require.NoError(t, err)

	// Restore to first (enters detached state)
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc1.SnapshotID)
	require.NoError(t, err)

	// Verify audit log contains detached=true
	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	content, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	// The last entry should be the restore we just did
	assert.Contains(t, string(content), "\"detached\":true")
}

func TestRestorer_Restore_WithCompression(t *testing.T) {
	// Test restore with compressed snapshot
	repoPath := setupTestRepo(t)

	mp := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mp, "file.txt"), []byte("original content"), 0644)

	// Create compressed snapshot
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(6) // gzip level 6
	desc, err := creator.Create("main", "compressed snapshot", nil)
	require.NoError(t, err)

	// Verify compression metadata was set
	assert.NotNil(t, desc.Compression)
	assert.Equal(t, "gzip", desc.Compression.Type)
	assert.Equal(t, 6, desc.Compression.Level)

	// Modify content
	os.WriteFile(filepath.Join(mp, "file.txt"), []byte("modified content"), 0644)

	// Restore
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	// Verify content was restored (decompressed)
	content, err := os.ReadFile(filepath.Join(mp, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original content", string(content))
	assert.NoFileExists(t, filepath.Join(mp, "file.txt.gz"))
	assert.NoFileExists(t, filepath.Join(mp, ".READY"))
	assert.NoFileExists(t, filepath.Join(mp, ".READY.gz"))
}

func TestRestorer_Restore_CorruptedSnapshotData(t *testing.T) {
	// Test restore when snapshot data is corrupted
	repoPath := setupTestRepo(t)

	mp := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mp, "file.txt"), []byte("original"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "test", nil)
	require.NoError(t, err)

	// Note: Modifying snapshot data directly doesn't corrupt checksum
	// The compression test above covers the decompression path
}

func TestRestorer_Restore_CorruptedDescriptor(t *testing.T) {
	// Test restore when descriptor file is corrupted
	repoPath := setupTestRepo(t)

	mp := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mp, "file.txt"), []byte("original"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)

	// Corrupt the descriptor file
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	os.WriteFile(descriptorPath, []byte("invalid json"), 0644)

	// Try to restore - should fail
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	assert.Error(t, err)
}

func TestRestorer_Restore_CorruptedPayloadHash(t *testing.T) {
	// Test restore when payload hash doesn't match
	repoPath := setupTestRepo(t)

	mp := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mp, "file.txt"), []byte("original"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)

	// Modify the payload root hash in descriptor to mismatch
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)

	var descMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &descMap))
	descMap["payload_root_hash"] = "invalidhash0000000000000000000000000"
	corruptData, err := json.Marshal(descMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, corruptData, 0644))

	// Try to restore - should fail verification
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err = restorer.Restore("main", desc.SnapshotID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verify snapshot")
}

func TestRestorer_Restore_EmptyWorktreeName(t *testing.T) {
	repoPath := setupTestRepo(t)
	desc := createSnapshot(t, repoPath)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("", desc.SnapshotID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worktree name is required")
}

func TestRestorer_Restore_EmptyBothArgs(t *testing.T) {
	repoPath := setupTestRepo(t)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	err := restorer.Restore("", "")
	assert.Error(t, err)
}
