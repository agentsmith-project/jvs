package worktree

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFailureTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func createFailureTestSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	t.Helper()

	snapshotID := model.SnapshotID("1708300800000-a3f7c1b2")
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "snapshot.txt"), []byte("snapshot"), 0644))

	payloadHash, err := integrity.ComputePayloadRootHash(snapshotDir)
	require.NoError(t, err)
	desc := model.Descriptor{
		SnapshotID:      snapshotID,
		WorktreeName:    "main",
		CreatedAt:       time.Now().UTC(),
		Engine:          model.EngineCopy,
		PayloadRootHash: payloadHash,
		IntegrityState:  model.IntegrityVerified,
	}
	checksum, err := integrity.ComputeDescriptorChecksum(&desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum
	data, err := json.Marshal(desc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"), data, 0644))
	readyData, err := json.Marshal(model.ReadyMarker{
		SnapshotID:         snapshotID,
		CompletedAt:        time.Now().UTC(),
		PayloadHash:        payloadHash,
		Engine:             model.EngineCopy,
		DescriptorChecksum: checksum,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), readyData, 0644))
	return snapshotID
}

func copyFailureTestSnapshotTree(src, dst string) error {
	_, err := engine.NewCopyEngine().Clone(src, dst)
	return err
}

func assertNoFailureTestStagingPayloads(t *testing.T, repoPath, name string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(repoPath, "worktrees", "."+name+".staging-*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "staged payloads should be cleaned up")
}

func TestManager_CreateLikeConfigWriteFailureRemovesFinalPayload(t *testing.T) {
	for _, op := range []string{"create", "create-from", "fork"} {
		t.Run(op, func(t *testing.T) {
			repoPath := setupFailureTestRepo(t)
			mgr := NewManager(repoPath)
			snapshotID := createFailureTestSnapshot(t, repoPath)

			oldWrite := writeWorktreeConfig
			writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
				return errors.New("injected config write failure")
			}
			t.Cleanup(func() {
				writeWorktreeConfig = oldWrite
			})

			var err error
			switch op {
			case "create":
				_, err = mgr.Create("cfg-fail", nil)
			case "create-from":
				_, err = mgr.CreateFromSnapshot("cfg-fail", snapshotID, copyFailureTestSnapshotTree)
			case "fork":
				_, err = mgr.Fork(snapshotID, "cfg-fail", copyFailureTestSnapshotTree)
			}

			require.Error(t, err)
			_, statErr := os.Lstat(filepath.Join(repoPath, "worktrees", "cfg-fail"))
			require.True(t, os.IsNotExist(statErr), "final payload must not remain after config write failure")
			assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "cfg-fail", "config.json"))
			assertNoFailureTestStagingPayloads(t, repoPath, "cfg-fail")
		})
	}
}

func TestManager_CreateLikeUncertainConfigWriteKeepsPayloadAndVisibleConfig(t *testing.T) {
	for _, op := range []string{"create", "create-from", "fork"} {
		t.Run(op, func(t *testing.T) {
			repoPath := setupFailureTestRepo(t)
			mgr := NewManager(repoPath)
			snapshotID := createFailureTestSnapshot(t, repoPath)
			name := "cfg-uncertain"

			oldWrite := writeWorktreeConfig
			writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
				require.NoError(t, repo.WriteWorktreeConfig(repoRoot, name, cfg))
				return &fsutil.CommitUncertainError{
					Op:   "worktree config write",
					Path: filepath.Join(repoRoot, ".jvs", "worktrees", name, "config.json"),
					Err:  errors.New("injected post-rename fsync failure"),
				}
			}
			t.Cleanup(func() {
				writeWorktreeConfig = oldWrite
			})

			var err error
			switch op {
			case "create":
				_, err = mgr.Create(name, nil)
			case "create-from":
				_, err = mgr.CreateFromSnapshot(name, snapshotID, copyFailureTestSnapshotTree)
			case "fork":
				_, err = mgr.Fork(snapshotID, name, copyFailureTestSnapshotTree)
			}

			require.Error(t, err)
			assert.True(t, fsutil.IsCommitUncertain(err), "uncertain config write must remain detectable")

			payloadPath := filepath.Join(repoPath, "worktrees", name)
			assert.DirExists(t, payloadPath, "visible config may point at this payload")
			if op != "create" {
				assert.FileExists(t, filepath.Join(payloadPath, "snapshot.txt"))
			}

			loaded, loadErr := repo.LoadWorktreeConfig(repoPath, name)
			require.NoError(t, loadErr)
			assert.Equal(t, name, loaded.Name)
			if op != "create" {
				assert.Equal(t, snapshotID, loaded.HeadSnapshotID)
				assert.Equal(t, snapshotID, loaded.LatestSnapshotID)
			}

			path, pathErr := mgr.Path(name)
			require.NoError(t, pathErr)
			assert.Equal(t, payloadPath, path)
			assertNoFailureTestStagingPayloads(t, repoPath, name)
		})
	}
}

func TestManager_CreateStartedFromPublishFailureLeavesNoWorkspaceOrMetadataResidue(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	snapshotID := createFailureTestSnapshot(t, repoPath)
	name := "started-publish-fail"
	payloadPath := filepath.Join(repoPath, "worktrees", name)
	configDir := filepath.Join(repoPath, ".jvs", "worktrees", name)

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if newpath == payloadPath {
			return errors.New("injected publish failure")
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	_, err := mgr.CreateStartedFromSnapshot(name, snapshotID, copyFailureTestSnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish workspace folder")
	assert.NoDirExists(t, payloadPath)
	assert.NoDirExists(t, configDir)
	assertNoFailureTestStagingPayloads(t, repoPath, name)
}

func TestManager_CreateStartedFromPublishCommitUncertainLeavesNoOrphanWorkspace(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	snapshotID := createFailureTestSnapshot(t, repoPath)
	name := "started-publish-uncertain"
	payloadPath := filepath.Join(repoPath, "worktrees", name)
	configDir := filepath.Join(repoPath, ".jvs", "worktrees", name)

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if newpath == payloadPath {
			require.NoError(t, oldRename(oldpath, newpath))
			return &fsutil.CommitUncertainError{
				Op:   "publish workspace folder",
				Path: newpath,
				Err:  errors.New("injected post-rename fsync failure"),
			}
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	_, err := mgr.CreateStartedFromSnapshot(name, snapshotID, copyFailureTestSnapshotTree)
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "publish commit-uncertain state must remain detectable")
	assertNoFailureTestStagingPayloads(t, repoPath, name)

	info, statErr := os.Lstat(payloadPath)
	if statErr == nil && info.IsDir() {
		loaded, loadErr := repo.LoadWorktreeConfig(repoPath, name)
		require.NoError(t, loadErr, "visible workspace folder must not be orphaned")
		assert.Equal(t, name, loaded.Name)
		assert.Equal(t, snapshotID, loaded.HeadSnapshotID)
		assert.Equal(t, snapshotID, loaded.StartedFromSnapshotID)
		return
	}
	require.True(t, os.IsNotExist(statErr), "workspace folder should either be removed or registered")
	assert.NoDirExists(t, configDir)
}

func TestManager_CreateStartedFromConfigWriteFailureRemovesWorkspaceAndMetadataResidue(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	snapshotID := createFailureTestSnapshot(t, repoPath)
	name := "started-config-fail"
	payloadPath := filepath.Join(repoPath, "worktrees", name)
	configDir := filepath.Join(repoPath, ".jvs", "worktrees", name)

	oldWrite := writeWorktreeConfig
	writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		return errors.New("injected config write failure")
	}
	t.Cleanup(func() {
		writeWorktreeConfig = oldWrite
	})

	_, err := mgr.CreateStartedFromSnapshot(name, snapshotID, copyFailureTestSnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write config")
	assert.NoDirExists(t, payloadPath)
	assert.NoDirExists(t, configDir)
	assertNoFailureTestStagingPayloads(t, repoPath, name)
}

func TestManager_CreateStartedFromUncertainConfigWriteKeepsVisibleWorkspace(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	snapshotID := createFailureTestSnapshot(t, repoPath)
	name := "started-config-uncertain"
	payloadPath := filepath.Join(repoPath, "worktrees", name)

	oldWrite := writeWorktreeConfig
	writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		require.NoError(t, repo.WriteWorktreeConfig(repoRoot, name, cfg))
		return &fsutil.CommitUncertainError{
			Op:   "worktree config write",
			Path: filepath.Join(repoRoot, ".jvs", "worktrees", name, "config.json"),
			Err:  errors.New("injected post-rename fsync failure"),
		}
	}
	t.Cleanup(func() {
		writeWorktreeConfig = oldWrite
	})

	_, err := mgr.CreateStartedFromSnapshot(name, snapshotID, copyFailureTestSnapshotTree)
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "uncertain config write must remain detectable")
	assert.DirExists(t, payloadPath)
	assert.FileExists(t, filepath.Join(payloadPath, "snapshot.txt"))
	loaded, loadErr := repo.LoadWorktreeConfig(repoPath, name)
	require.NoError(t, loadErr)
	assert.Equal(t, name, loaded.Name)
	assert.Equal(t, snapshotID, loaded.HeadSnapshotID)
	assert.Equal(t, snapshotID, loaded.StartedFromSnapshotID)
	assertNoFailureTestStagingPayloads(t, repoPath, name)
}

func TestManager_SetLatestPropagatesUncertainConfigWrite(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	snapshotID := model.SnapshotID("1708300800000-a3f7c1b2")

	oldWrite := writeWorktreeConfig
	writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		require.Equal(t, "main", name)
		require.Equal(t, snapshotID, cfg.HeadSnapshotID)
		require.Equal(t, snapshotID, cfg.LatestSnapshotID)
		require.NoError(t, repo.WriteWorktreeConfig(repoRoot, name, cfg))
		return &fsutil.CommitUncertainError{
			Op:   "worktree config update",
			Path: filepath.Join(repoRoot, ".jvs", "worktrees", name, "config.json"),
			Err:  errors.New("injected post-rename fsync failure"),
		}
	}
	t.Cleanup(func() {
		writeWorktreeConfig = oldWrite
	})

	err := mgr.SetLatest("main", snapshotID)
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "SetLatest must preserve uncertain commit errors")

	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, loadErr)
	assert.Equal(t, snapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, snapshotID, cfg.LatestSnapshotID)
}

func TestManager_RemoveFailsClosedWhenAuditLogMalformed(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("audit-blocked", nil)
	require.NoError(t, err)

	payloadPath := filepath.Join(repoPath, "worktrees", "audit-blocked")
	require.NoError(t, os.WriteFile(filepath.Join(payloadPath, "payload.txt"), []byte("keep"), 0644))
	configPath := filepath.Join(repoPath, ".jvs", "worktrees", "audit-blocked", "config.json")
	require.FileExists(t, configPath)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(auditPath), 0755))
	require.NoError(t, os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644))

	err = mgr.Remove("audit-blocked")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit")

	assert.FileExists(t, filepath.Join(payloadPath, "payload.txt"))
	assert.FileExists(t, configPath)
}

func TestManager_RenameDefaultRenamePathIsDurableNoReplace(t *testing.T) {
	fn := runtime.FuncForPC(reflect.ValueOf(renamePath).Pointer())
	require.NotNil(t, fn)
	assert.Contains(t, fn.Name(), "pkg/fsutil.RenameNoReplaceAndSync")
}

func TestManager_RenameRollsBackPayloadWhenConfigDirRenameFails(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldConfigDir && newpath == newConfigDir {
			return errors.New("injected config rename failure")
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename config directory")

	assert.FileExists(t, filepath.Join(oldPayload, "payload.txt"))
	assert.NoDirExists(t, newPayload)
	assert.FileExists(t, filepath.Join(oldConfigDir, "config.json"))
	assert.NoFileExists(t, filepath.Join(newConfigDir, "config.json"))

	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "old-name")
	require.NoError(t, loadErr)
	assert.Equal(t, "old-name", cfg.Name)
}

func TestManager_RenamePayloadRenameIsNoReplaceAfterPrecheck(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldPayload && newpath == newPayload {
			require.NoError(t, os.Mkdir(newPayload, 0755))
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename payload")

	assert.FileExists(t, filepath.Join(oldPayload, "payload.txt"))
	assert.NoFileExists(t, filepath.Join(newPayload, "payload.txt"))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "old-name", "config.json"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "new-name", "config.json"))
}

func TestManager_RenameConfigDirRenameIsNoReplaceAfterPrecheckAndRollsBackPayload(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldConfigDir && newpath == newConfigDir {
			require.NoError(t, os.Mkdir(newConfigDir, 0755))
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename config directory")

	assert.FileExists(t, filepath.Join(oldPayload, "payload.txt"))
	assert.NoDirExists(t, newPayload)
	assert.FileExists(t, filepath.Join(oldConfigDir, "config.json"))
	assert.NoFileExists(t, filepath.Join(newConfigDir, "config.json"))

	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "old-name")
	require.NoError(t, loadErr)
	assert.Equal(t, "old-name", cfg.Name)
}

func TestManager_RenamePayloadCommitUncertainDoesNotRollback(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldPayload && newpath == newPayload {
			require.NoError(t, oldRename(oldpath, newpath))
			return &fsutil.CommitUncertainError{
				Op:   "rename no-replace",
				Path: newpath,
				Err:  errors.New("injected parent fsync failure"),
			}
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "payload rename fsync failure must fail closed")

	assert.NoDirExists(t, oldPayload)
	assert.FileExists(t, filepath.Join(newPayload, "payload.txt"))
	assert.FileExists(t, filepath.Join(oldConfigDir, "config.json"))
	assert.NoDirExists(t, newConfigDir)
}

func TestManager_RenameConfigDirCommitUncertainDoesNotRollbackPayload(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldConfigDir && newpath == newConfigDir {
			require.NoError(t, oldRename(oldpath, newpath))
			return &fsutil.CommitUncertainError{
				Op:   "rename no-replace",
				Path: newpath,
				Err:  errors.New("injected parent fsync failure"),
			}
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "config directory rename fsync failure must fail closed")

	assert.NoDirExists(t, oldPayload)
	assert.FileExists(t, filepath.Join(newPayload, "payload.txt"))
	assert.NoDirExists(t, oldConfigDir)
	assert.FileExists(t, filepath.Join(newConfigDir, "config.json"))
}

func TestManager_RenameRollbackPayloadRenameIsNoReplace(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldRename := renamePath
	renamePath = func(oldpath, newpath string) error {
		if oldpath == oldConfigDir && newpath == newConfigDir {
			require.NoError(t, os.Mkdir(oldPayload, 0755))
			return errors.New("injected config rename failure")
		}
		return oldRename(oldpath, newpath)
	}
	t.Cleanup(func() {
		renamePath = oldRename
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback payload rename failed")

	assert.NoFileExists(t, filepath.Join(oldPayload, "payload.txt"))
	assert.FileExists(t, filepath.Join(newPayload, "payload.txt"))
	assert.FileExists(t, filepath.Join(oldConfigDir, "config.json"))
	assert.NoDirExists(t, newConfigDir)
}

func TestManager_RenameRollsBackConfigDirAndPayloadWhenConfigRewriteFails(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldWrite := writeWorktreeConfig
	writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		require.Equal(t, repoPath, repoRoot)
		require.Equal(t, "new-name", name)
		require.Equal(t, "new-name", cfg.Name)
		return errors.New("injected config rewrite failure")
	}
	t.Cleanup(func() {
		writeWorktreeConfig = oldWrite
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write config")

	assert.FileExists(t, filepath.Join(oldPayload, "payload.txt"))
	assert.NoDirExists(t, newPayload)
	assert.FileExists(t, filepath.Join(oldConfigDir, "config.json"))
	assert.NoFileExists(t, filepath.Join(newConfigDir, "config.json"))

	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "old-name")
	require.NoError(t, loadErr)
	assert.Equal(t, "old-name", cfg.Name)
}

func TestManager_RenameConfigRewriteCommitUncertainLeavesRenamedMapping(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mgr := NewManager(repoPath)
	_, err := mgr.Create("old-name", nil)
	require.NoError(t, err)

	oldPayload := filepath.Join(repoPath, "worktrees", "old-name")
	newPayload := filepath.Join(repoPath, "worktrees", "new-name")
	oldConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "old-name")
	newConfigDir := filepath.Join(repoPath, ".jvs", "worktrees", "new-name")
	require.NoError(t, os.WriteFile(filepath.Join(oldPayload, "payload.txt"), []byte("payload"), 0644))

	oldWrite := writeWorktreeConfig
	writeWorktreeConfig = func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		require.NoError(t, repo.WriteWorktreeConfig(repoRoot, name, cfg))
		return &fsutil.CommitUncertainError{
			Op:   "worktree config rename rewrite",
			Path: filepath.Join(repoRoot, ".jvs", "worktrees", name, "config.json"),
			Err:  errors.New("injected post-rename fsync failure"),
		}
	}
	t.Cleanup(func() {
		writeWorktreeConfig = oldWrite
	})

	err = mgr.Rename("old-name", "new-name")
	require.Error(t, err)
	assert.True(t, fsutil.IsCommitUncertain(err), "uncertain rewrite should remain detectable")

	assert.NoDirExists(t, oldPayload)
	assert.FileExists(t, filepath.Join(newPayload, "payload.txt"))
	assert.NoDirExists(t, oldConfigDir)
	assert.FileExists(t, filepath.Join(newConfigDir, "config.json"))

	cfg, loadErr := repo.LoadWorktreeConfig(repoPath, "new-name")
	require.NoError(t, loadErr)
	assert.Equal(t, "new-name", cfg.Name)
}
