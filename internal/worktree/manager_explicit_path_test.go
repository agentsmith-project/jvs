package worktree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerCreateStartedFromSnapshotAtUsesExplicitAbsoluteFolderAndDefaultName(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	target := filepath.Join(t.TempDir(), "experiment")

	cfg, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     target,
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.NoError(t, err)

	canonical := canonicalWorktreeTestPath(t, target)
	assert.Equal(t, "experiment", cfg.Name)
	assert.Equal(t, canonical, cfg.RealPath)
	assert.Equal(t, snapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, snapshotID, cfg.StartedFromSnapshotID)
	assert.FileExists(t, filepath.Join(target, "snapshot.txt"))
	assert.NoDirExists(t, filepath.Join(repoPath, "worktrees", "experiment"))

	require.NoError(t, os.MkdirAll(filepath.Join(target, "nested"), 0755))
	discovered, workspace, err := repo.DiscoverWorktree(filepath.Join(target, "nested"))
	require.NoError(t, err)
	assert.Equal(t, repoPath, discovered.Root)
	assert.Equal(t, "experiment", workspace)
}

func TestManagerCreateStartedFromSnapshotAtResolvesRelativeFolderFromCurrentDirectory(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	parent := t.TempDir()
	t.Chdir(parent)

	cfg, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     "relative-exp",
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.NoError(t, err)

	target := filepath.Join(parent, "relative-exp")
	assert.Equal(t, "relative-exp", cfg.Name)
	assert.Equal(t, canonicalWorktreeTestPath(t, target), cfg.RealPath)
	assert.FileExists(t, filepath.Join(target, "snapshot.txt"))
}

func TestManagerCreateStartedFromSnapshotAtDefaultNameUsesBasenameAndExplicitNameOverridesIt(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	parent := t.TempDir()

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     filepath.Join(parent, "main"),
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.NoDirExists(t, filepath.Join(parent, "main"))

	cfg, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Name:       "explicit-name",
		Folder:     filepath.Join(parent, "main"),
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.NoError(t, err)
	assert.Equal(t, "explicit-name", cfg.Name)
	assert.Equal(t, canonicalWorktreeTestPath(t, filepath.Join(parent, "main")), cfg.RealPath)
}

func TestManagerCreateStartedFromSnapshotAtRejectsInvalidBasenameBeforeCreatingFolder(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	target := filepath.Join(t.TempDir(), "bad name")

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     target,
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
	assert.NoDirExists(t, target)
}

func TestManagerCreateStartedFromSnapshotAtRejectsExistingTargetFolder(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	target := filepath.Join(t.TempDir(), "exists")
	require.NoError(t, os.MkdirAll(target, 0755))

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     target,
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestManagerCreateStartedFromSnapshotAtRejectsTargetInsideExistingWorkspace(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	target := filepath.Join(repoPath, "main", "nested-workspace")

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     target,
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
	assert.NoDirExists(t, target)
}

func TestManagerCreateStartedFromSnapshotAtRejectsTargetInsideRepoControlDirectory(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	target := filepath.Join(repoPath, ".jvs", "not-a-workspace")

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Folder:     target,
		SnapshotID: snapshotID,
	}, copySnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control")
	assert.NoDirExists(t, target)
}

func TestManagerCreateStartedFromSnapshotRequiresExplicitFolder(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)

	_, err := worktree.NewManager(repoPath).CreateStartedFromSnapshot("implicit", snapshotID, copySnapshotTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace folder is required")
	assert.True(t, errors.Is(err, errclass.ErrUsage), "expected a user-correctable API usage error")
}

func canonicalWorktreeTestPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(abs)
	require.NoError(t, err)
	return filepath.Clean(canonical)
}
