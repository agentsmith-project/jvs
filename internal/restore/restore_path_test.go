package restore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestorerRestorePathReplacesOnlyRequestedPathAndDoesNotMoveHead(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := mainPayloadPath(t, repoPath)
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "src", "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v1"), 0644))
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	first, err := creator.CreateSavePoint("main", "first", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "src", "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v2"), 0644))
	second, err := creator.CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	err = restore.NewRestorer(repoPath, model.EngineCopy).RestorePath("main", first.SnapshotID, "src/app.txt")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(mainPath, "src", "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
	outside, err := os.ReadFile(filepath.Join(mainPath, "outside.txt"))
	require.NoError(t, err)
	assert.Equal(t, "outside v2", string(outside))

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, second.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, second.SnapshotID, cfg.LatestSnapshotID)
	entry, ok, err := cfg.PathSources.SourceForPath("src/app.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/app.txt", entry.TargetPath)
	assert.Equal(t, first.SnapshotID, entry.SourceSnapshotID)
}

func TestRestorerRestorePathMissingSourceDoesNotMutate(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := mainPayloadPath(t, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "present.txt"), []byte("v1"), 0644))
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	first, err := creator.CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "present.txt"), []byte("v2"), 0644))
	second, err := creator.CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	err = restore.NewRestorer(repoPath, model.EngineCopy).RestorePath("main", first.SnapshotID, "missing.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path does not exist")

	content, readErr := os.ReadFile(filepath.Join(mainPath, "present.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "v2", string(content))
	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, second.SnapshotID, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.PathSources)
}

func TestRestorerRestorePathRejectsSymlinkParentWithoutMutation(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := mainPayloadPath(t, repoPath)
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "safe"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "safe", "file.txt"), []byte("safe v1"), 0644))
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	first, err := creator.CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "safe", "file.txt"), []byte("safe v2"), 0644))
	second, err := creator.CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(mainPath, "safe")))
	require.NoError(t, os.Symlink(outside, filepath.Join(mainPath, "safe")))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside original"), 0644))

	err = restore.NewRestorer(repoPath, model.EngineCopy).RestorePath("main", first.SnapshotID, "safe/file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	outsideContent, readErr := os.ReadFile(filepath.Join(outside, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside original", string(outsideContent))
	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, second.SnapshotID, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.PathSources)
}
