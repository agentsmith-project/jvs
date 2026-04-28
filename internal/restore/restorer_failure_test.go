package restore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
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

func TestRestorePartialRejectsDestinationSymlinkParentBeforeMutation(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "target"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "safe.txt"), []byte("snapshot safe"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "target", "secret.txt"), []byte("snapshot secret"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	partialDesc, err := creator.CreatePartial("main", "partial", nil, []string{"safe.txt", "target/secret.txt"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "safe.txt"), []byte("later safe"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)

	outsidePath := filepath.Join(repoPath, "outside")
	require.NoError(t, os.MkdirAll(outsidePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsidePath, "secret.txt"), []byte("outside original"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "safe.txt"), []byte("current safe"), 0644))
	require.NoError(t, os.RemoveAll(filepath.Join(mainPath, "target")))
	if err := os.Symlink(outsidePath, filepath.Join(mainPath, "target")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", partialDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	content, readErr := os.ReadFile(filepath.Join(outsidePath, "secret.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside original", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "safe.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current safe", string(content), "preflight must reject before changing earlier partial paths")

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, laterDesc.SnapshotID, cfg.HeadSnapshotID)
}

func TestRestorePartialRenameFsyncFailureRollsBackPayloadAndHead(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	partialDesc, err := creator.CreatePartial("main", "partial", nil, []string{"file.txt"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("later"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current original"), 0644))

	injected := false
	restoreFsyncDir = func(dir string) error {
		if !injected && strings.Contains(dir, ".restore-backup-") {
			injected = true
			return errors.New("injected backup fsync failure")
		}
		return fsutil.FsyncDir(dir)
	}
	t.Cleanup(func() {
		restoreFsyncDir = fsutil.FsyncDir
	})

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", partialDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup partial path")
	assert.NotContains(t, err.Error(), "backup retained")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current original", string(content))
	requireNoBackupDir(t, mainPath)
	assertHeadSnapshot(t, repoPath, laterDesc.SnapshotID)
}

func TestRestoreFullBackupStageFailureAfterFirstMoveRollsBackPayloadAndHead(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("snapshot a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "b.txt"), []byte("snapshot b"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	firstDesc, err := creator.Create("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("later a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "b.txt"), []byte("later b"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("current original a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "b.txt"), []byte("current original b"), 0644))

	injected := false
	restoreFsyncDir = func(dir string) error {
		if !injected && strings.Contains(dir, ".restore-backup-") {
			injected = true
			return errors.New("injected backup fsync failure")
		}
		return fsutil.FsyncDir(dir)
	}
	t.Cleanup(func() {
		restoreFsyncDir = fsutil.FsyncDir
	})

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", firstDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup current contents")
	assert.NotContains(t, err.Error(), "backup retained")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "a.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current original a", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "b.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current original b", string(content))
	requireNoBackupDir(t, mainPath)
	assertHeadSnapshot(t, repoPath, laterDesc.SnapshotID)
}

func TestRestoreFullBackupFailureRetainsBackupWhenRollbackFails(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	firstDesc, err := creator.Create("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("later"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current original"), 0644))

	injected := false
	restoreFsyncDir = func(dir string) error {
		if !injected && strings.Contains(dir, ".restore-backup-") {
			injected = true
			return errors.New("injected backup fsync failure")
		}
		return fsutil.FsyncDir(dir)
	}
	restoreRename = func(src, dst string) error {
		if strings.Contains(src, ".restore-backup-") && dst == filepath.Join(mainPath, "file.txt") {
			return errors.New("injected rollback rename failure")
		}
		return os.Rename(src, dst)
	}
	t.Cleanup(func() {
		restoreFsyncDir = fsutil.FsyncDir
		restoreRename = os.Rename
	})

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", firstDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback failed")
	assert.Contains(t, err.Error(), "backup retained")

	backupFile := requireSingleBackupFile(t, mainPath, "file.txt")
	content, readErr := os.ReadFile(backupFile)
	require.NoError(t, readErr)
	assert.Equal(t, "current original", string(content))
	assertHeadSnapshot(t, repoPath, laterDesc.SnapshotID)
}

func TestRestoreFinalPayloadFsyncFailureRetainsBackupAndHead(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	firstDesc, err := creator.Create("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("later"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current original"), 0644))

	payloadSyncs := 0
	restoreFsyncDir = func(dir string) error {
		if dir == mainPath {
			payloadSyncs++
			if payloadSyncs == 3 {
				return errors.New("injected final payload fsync failure")
			}
		}
		return fsutil.FsyncDir(dir)
	}
	t.Cleanup(func() {
		restoreFsyncDir = fsutil.FsyncDir
	})

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", firstDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup retained")

	backupFile := requireSingleBackupFile(t, mainPath, "file.txt")
	content, readErr := os.ReadFile(backupFile)
	require.NoError(t, readErr)
	assert.Equal(t, "current original", string(content))
	assertHeadSnapshot(t, repoPath, laterDesc.SnapshotID)
}

func TestRestoreFullUpdateHeadFailureRollsBackPayloadAndRetainsBackupPath(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	firstDesc, err := creator.Create("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("later"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current original"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "current-only.txt"), []byte("keep me"), 0644))

	restorer := NewRestorer(repoPath, model.EngineCopy)
	restorer.updateHead = func(*worktree.Manager, string, model.SnapshotID) error {
		return errors.New("injected update head failure")
	}

	err = restorer.Restore("main", firstDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update head")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current original", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "current-only.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "keep me", string(content))
	requireSingleBackupDir(t, mainPath)
	assertWorktreeConfig(t, repoPath, laterDesc.SnapshotID, laterDesc.SnapshotID)
}

func TestRestoreAuditAppendFailureAfterPayloadSuccessReturnsSuccess(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("source"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	firstDesc, err := creator.CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("newer"), 0644))
	laterDesc, err := creator.CreateSavePoint("main", "later", nil)
	require.NoError(t, err)

	restoreHooks := SetHooksForTest(Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
			if err := os.Remove(auditPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			return os.Mkdir(auditPath, 0755)
		},
	})
	t.Cleanup(restoreHooks)

	err = NewRestorer(repoPath, model.EngineCopy).Restore("main", firstDesc.SnapshotID)
	require.NoError(t, err)
	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "source", string(content))
	requireNoBackupDir(t, mainPath)
	assertWorktreeConfig(t, repoPath, firstDesc.SnapshotID, laterDesc.SnapshotID)
}

func TestRestorePartialUpdateHeadFailureRollsBackTouchedPathsOnly(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("snapshot a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "dir", "b.txt"), []byte("snapshot b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "untouched.txt"), []byte("snapshot untouched"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	partialDesc, err := creator.CreatePartial("main", "partial", nil, []string{"a.txt", "dir/b.txt"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("later a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "dir", "b.txt"), []byte("later b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "untouched.txt"), []byte("later untouched"), 0644))
	laterDesc, err := creator.Create("main", "later", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "a.txt"), []byte("current a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "dir", "b.txt"), []byte("current b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "untouched.txt"), []byte("current untouched"), 0644))

	restorer := NewRestorer(repoPath, model.EngineCopy)
	restorer.updateHead = func(*worktree.Manager, string, model.SnapshotID) error {
		return errors.New("injected update head failure")
	}

	err = restorer.Restore("main", partialDesc.SnapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update head")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "a.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current a", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "dir", "b.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current b", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "untouched.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current untouched", string(content))
	requireSingleBackupDir(t, mainPath)
	assertWorktreeConfig(t, repoPath, laterDesc.SnapshotID, laterDesc.SnapshotID)
}

func requireSingleBackupFile(t *testing.T, payloadPath, rel string) string {
	t.Helper()

	matches, err := filepath.Glob(payloadPath + ".restore-backup-*")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	return filepath.Join(matches[0], rel)
}

func requireSingleBackupDir(t *testing.T, payloadPath string) string {
	t.Helper()

	matches, err := filepath.Glob(payloadPath + ".restore-backup-*")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	return matches[0]
}

func requireNoBackupDir(t *testing.T, payloadPath string) {
	t.Helper()

	matches, err := filepath.Glob(payloadPath + ".restore-backup-*")
	require.NoError(t, err)
	require.Empty(t, matches)
}

func assertHeadSnapshot(t *testing.T, repoPath string, want model.SnapshotID) {
	t.Helper()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, want, cfg.HeadSnapshotID)
}

func assertWorktreeConfig(t *testing.T, repoPath string, wantHead, wantLatest model.SnapshotID) {
	t.Helper()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, wantHead, cfg.HeadSnapshotID)
	assert.Equal(t, wantLatest, cfg.LatestSnapshotID)
}
