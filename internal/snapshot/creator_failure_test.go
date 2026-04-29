package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCreatorFailureRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main", "file.txt"), []byte("content"), 0644))
	return dir
}

func TestCreator_DescriptorWriteFailureDoesNotPublishReadyAndRemovesIntent(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)
	descriptorLanded := false
	var snapshotID model.SnapshotID
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		snapshotID = desc.SnapshotID
		if err := writeDescriptorFile(path, desc); err != nil {
			return err
		}
		descriptorLanded = true
		return errors.New("injected descriptor post-write failure")
	}

	desc, err := creator.Create("main", "descriptor failure", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.True(t, descriptorLanded, "test must exercise descriptor cleanup after a landed write")
	require.NotEmpty(t, snapshotID)

	assertNoIntents(t, repoPath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_PublishFailureAfterDescriptorWriteUnpublishesPayloadAndRemovesIntent(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	descriptorWritten := false
	var snapshotID model.SnapshotID
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		snapshotID = desc.SnapshotID
		descriptorWritten = true
		return writeDescriptorFile(path, desc)
	}
	creator.snapshotRenamer = func(oldpath, newpath string) error {
		if err := fsutil.RenameAndSync(oldpath, newpath); err != nil {
			return err
		}
		return errors.New("injected post-rename failure")
	}

	desc, err := creator.Create("main", "publish failure", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.True(t, descriptorWritten, "test must exercise failure after descriptor write")
	require.NotEmpty(t, snapshotID)

	assertNoIntents(t, repoPath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_PublishRenameDefiniteFailurePreservesExistingFinalSnapshot(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	var snapshotID model.SnapshotID
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		snapshotID = desc.SnapshotID
		if err := writeDescriptorFile(path, desc); err != nil {
			return err
		}

		finalDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
		require.NoError(t, os.MkdirAll(finalDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(finalDir, ".READY"), []byte("existing ready"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(finalDir, "existing.txt"), []byte("existing content"), 0644))
		return nil
	}
	creator.snapshotRenamer = func(oldpath, newpath string) error {
		assert.DirExists(t, oldpath)
		assert.DirExists(t, newpath)
		return errors.New("injected definite rename failure before commit")
	}

	desc, err := creator.Create("main", "publish final exists", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	require.NotEmpty(t, snapshotID)

	finalDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	assert.DirExists(t, finalDir)
	assert.FileExists(t, filepath.Join(finalDir, ".READY"))
	content, readErr := os.ReadFile(filepath.Join(finalDir, "existing.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "existing content", string(content))
	assertPublishAttemptArtifactsCleaned(t, repoPath, snapshotID)
	assertNoIntents(t, repoPath)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_PublishRenameDefiniteFailurePreservesFinalSnapshotSymlinkTarget(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, ".READY"), []byte("outside ready"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "outside.txt"), []byte("outside content"), 0644))

	var snapshotID model.SnapshotID
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		snapshotID = desc.SnapshotID
		if err := writeDescriptorFile(path, desc); err != nil {
			return err
		}

		finalDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
		if err := os.Symlink(outsideDir, finalDir); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}
		return nil
	}
	creator.snapshotRenamer = func(oldpath, newpath string) error {
		assert.DirExists(t, oldpath)
		info, statErr := os.Lstat(newpath)
		require.NoError(t, statErr)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "expected final leaf to be a symlink")
		return errors.New("injected definite rename failure before commit")
	}

	desc, err := creator.Create("main", "publish final symlink", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	require.NotEmpty(t, snapshotID)

	finalDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	info, statErr := os.Lstat(finalDir)
	require.NoError(t, statErr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "cleanup must not remove a preexisting final symlink")
	assert.FileExists(t, filepath.Join(outsideDir, ".READY"))
	content, readErr := os.ReadFile(filepath.Join(outsideDir, "outside.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside content", string(content))
	assertPublishAttemptArtifactsCleaned(t, repoPath, snapshotID)
	assertNoIntents(t, repoPath)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_PublishRenameCommitUncertainRetainsSnapshotDescriptorAndIntent(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	descriptorWritten := false
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		descriptorWritten = true
		return writeDescriptorFile(path, desc)
	}
	creator.snapshotRenamer = func(oldpath, newpath string) error {
		if err := fsutil.RenameAndSync(oldpath, newpath); err != nil {
			return err
		}
		return &fsutil.CommitUncertainError{
			Op:   "rename",
			Path: newpath,
			Err:  errors.New("injected post-rename fsync failure"),
		}
	}

	desc, err := creator.Create("main", "publish uncertain", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	require.True(t, fsutil.IsCommitUncertain(err), "creator should preserve uncertain publish semantics")
	assert.True(t, descriptorWritten, "test must exercise failure after descriptor write")

	intent := requireSingleIntent(t, repoPath)
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(intent.SnapshotID))
	assert.DirExists(t, snapshotDir)
	assert.FileExists(t, filepath.Join(snapshotDir, ".READY"))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(intent.SnapshotID)+".json"))
	require.NoError(t, VerifySnapshot(repoPath, intent.SnapshotID, true))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_HeadUpdateUncertainCommitRetainsPublishedSnapshotAndIntent(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	var committedSnapshotID model.SnapshotID
	creator.latestUpdater = func(_ *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
		cfg, err := repo.LoadWorktreeConfig(repoPath, worktreeName)
		require.NoError(t, err)
		cfg.HeadSnapshotID = snapshotID
		cfg.LatestSnapshotID = snapshotID
		require.NoError(t, repo.WriteWorktreeConfig(repoPath, worktreeName, cfg))

		committedSnapshotID = snapshotID
		return &fsutil.CommitUncertainError{
			Op:   "worktree config update",
			Path: filepath.Join(repoPath, ".jvs", "worktrees", worktreeName, "config.json"),
			Err:  errors.New("injected post-rename fsync failure"),
		}
	}

	desc, err := creator.Create("main", "head uncertain", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	require.True(t, fsutil.IsCommitUncertain(err), "creator should preserve uncertain metadata commit semantics")
	require.NotEmpty(t, committedSnapshotID)

	intent := requireSingleIntent(t, repoPath)
	assert.Equal(t, committedSnapshotID, intent.SnapshotID)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(committedSnapshotID))
	assert.DirExists(t, snapshotDir)
	assert.FileExists(t, filepath.Join(snapshotDir, ".READY"))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(committedSnapshotID)+".json"))
	require.NoError(t, VerifySnapshot(repoPath, committedSnapshotID, true))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, committedSnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, committedSnapshotID, cfg.LatestSnapshotID)
}

func TestCreator_HeadUpdateFailureUnpublishesDescriptorAndPayloadAndRemovesIntent(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	descriptorWritten := false
	var snapshotID model.SnapshotID
	creator.descriptorWriter = func(path string, desc *model.Descriptor) error {
		snapshotID = desc.SnapshotID
		descriptorWritten = true
		return writeDescriptorFile(path, desc)
	}
	creator.latestUpdater = func(*worktree.Manager, string, model.SnapshotID) error {
		return errors.New("injected head update failure")
	}

	desc, err := creator.Create("main", "head failure", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.True(t, descriptorWritten, "test must exercise failure after descriptor write")
	require.NotEmpty(t, snapshotID)

	assertNoIntents(t, repoPath)
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY"))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
}

func TestCreator_SavePointAuditAppendabilityFailureBeforeHistoryUpdateUnpublishesDescriptorAndPayload(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)

	var stagedSnapshotID model.SnapshotID
	restoreHook := SetAfterSnapshotPayloadStagedHookForTest(func(snapshotID model.SnapshotID, _ string) error {
		stagedSnapshotID = snapshotID
		auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
		if err := os.Remove(auditPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Mkdir(auditPath, 0755)
	})
	t.Cleanup(restoreHook)

	desc, err := creator.CreateSavePoint("main", "audit blocked", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.Contains(t, err.Error(), "audit")
	require.NotEmpty(t, stagedSnapshotID)
	assertUnpublishedSaveAttempt(t, repoPath, stagedSnapshotID)
	assertNoIntents(t, repoPath)

	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
	assertPublishedSavePointCount(t, repoPath, 0)
}

func TestCreator_SavePointLateAuditAppendFailureWarnsAfterHistoryUpdate(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	creator := NewCreator(repoPath, model.EngineCopy)
	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")

	historyUpdated := false
	baseLatestUpdater := creator.latestUpdater
	creator.latestUpdater = func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
		if err := baseLatestUpdater(wtMgr, worktreeName, snapshotID); err != nil {
			return err
		}
		historyUpdated = true
		if err := os.Remove(auditPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Mkdir(auditPath, 0755)
	}

	var desc *model.Descriptor
	var err error
	stderr := captureCreatorFailureStderr(t, func() {
		desc, err = creator.CreateSavePoint("main", "late audit warning", nil)
	})

	require.NoError(t, err)
	require.NotNil(t, desc)
	assert.True(t, historyUpdated, "test must fail audit only after history update succeeds")
	assert.Contains(t, stderr, "warning: saved save point "+string(desc.SnapshotID))
	assert.Contains(t, stderr, "could not write audit log")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)
	assertPublishedSavePointCount(t, repoPath, 1)
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json"))
	assert.DirExists(t, auditPath)
}

func requireSingleIntent(t *testing.T, repoPath string) model.IntentRecord {
	t.Helper()

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	entries, err := os.ReadDir(intentsDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	data, err := os.ReadFile(filepath.Join(intentsDir, entries[0].Name()))
	require.NoError(t, err)
	var intent model.IntentRecord
	require.NoError(t, json.Unmarshal(data, &intent))
	require.NotEmpty(t, intent.SnapshotID)
	return intent
}

func assertNoIntents(t *testing.T, repoPath string) {
	t.Helper()

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	entries, err := os.ReadDir(intentsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func captureCreatorFailureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	fn()

	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return buf.String()
}

func assertPublishAttemptArtifactsCleaned(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
	t.Helper()

	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)+".tmp"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
}
