package repo_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_CreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")

	r, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)
	require.NotNil(t, r)

	// Verify .jvs/ structure
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "format_version"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "main"))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "worktrees", "main", "config.json"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "descriptors"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "intents"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "audit"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "gc"))

	// Verify main/ payload directory
	assert.DirExists(t, filepath.Join(repoPath, "main"))

	// Verify format_version content
	content, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "format_version"))
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(content))

	// Verify repo_id exists and is non-empty
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "repo_id"))
	repoIDContent, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "repo_id"))
	require.NoError(t, err)
	assert.NotEmpty(t, string(repoIDContent))

	// Verify returned repo struct
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, 1, r.FormatVersion)
	assert.NotEmpty(t, r.RepoID)
}

func TestMutationLockNoWaitReturnsStableBusyError(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := repo.Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(repoPath, "test-holder")
	require.NoError(t, err)
	defer held.Release()

	_, err = repo.AcquireMutationLock(repoPath, "test-contender")
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
	assert.True(t, errors.Is(err, errclass.ErrRepoBusy))
	assert.Contains(t, err.Error(), "E_REPO_BUSY")
}

func TestWithMutationLockReleasesAfterCallbackError(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "lockedrepo")
	_, err := repo.Init(repoPath, "lockedrepo")
	require.NoError(t, err)

	sentinel := assert.AnError
	err = repo.WithMutationLock(repoPath, "failing-op", func() error {
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	lock, err := repo.AcquireMutationLock(repoPath, "after-error")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
}

func TestInit_MainWorktreeConfig(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")

	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Name)
	assert.NotZero(t, cfg.CreatedAt)
}

func TestInit_InvalidName(t *testing.T) {
	dir := t.TempDir()

	_, err := repo.Init(dir, "../evil")
	assert.Error(t, err)

	_, err = repo.Init(dir, "name/with/slash")
	assert.Error(t, err)

	_, err = repo.Init(dir, "")
	assert.Error(t, err)
}

func TestInit_ExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "existing")

	// Create directory first
	require.NoError(t, os.MkdirAll(repoPath, 0755))

	// Init should still work
	_, err := repo.Init(repoPath, "existing")
	require.NoError(t, err)
}

func TestDiscover_FindsRepo(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// Discover from repo root
	r, err := repo.Discover(repoPath)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)

	// Discover from nested path
	nested := filepath.Join(repoPath, "main", "subdir")
	require.NoError(t, os.MkdirAll(nested, 0755))
	r, err = repo.Discover(nested)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
}

func TestDiscover_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := repo.Discover(dir)
	assert.Error(t, err)
}

func TestDiscoverWorktree_MainWorktree(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// From main/ directory
	r, wtName, err := repo.DiscoverWorktree(filepath.Join(repoPath, "main"))
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "main", wtName)

	// From nested path in main/
	nested := filepath.Join(repoPath, "main", "deep", "path")
	require.NoError(t, os.MkdirAll(nested, 0755))
	r, wtName, err = repo.DiscoverWorktree(nested)
	require.NoError(t, err)
	assert.Equal(t, "main", wtName)
}

func TestDiscoverWorktree_NamedWorktree(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// Create a named worktree
	wtPath := filepath.Join(repoPath, "worktrees", "feature")
	require.NoError(t, os.MkdirAll(wtPath, 0755))

	// Create config for worktree
	cfgDir := filepath.Join(repoPath, ".jvs", "worktrees", "feature")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{Name: "feature"}))

	// Discover from named worktree
	r, wtName, err := repo.DiscoverWorktree(wtPath)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "feature", wtName)
}

func TestDiscoverWorktreeRejectsFakePayloadWithRegisteredName(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	fakeMain := filepath.Join(repoPath, "worktrees", "main", "nested")
	require.NoError(t, os.MkdirAll(fakeMain, 0755))

	r, wtName, err := repo.DiscoverWorktree(fakeMain)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "", wtName)
}

func TestDiscoverWorktree_FromJvsDir(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// From .jvs/ directory - should map to "main" as default
	r, wtName, err := repo.DiscoverWorktree(filepath.Join(repoPath, ".jvs"))
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	// .jvs is not a worktree, should return empty or error
	assert.Equal(t, "", wtName)
}

func TestWorktreeConfigPath(t *testing.T) {
	path, err := repo.WorktreeConfigPath("/repo", "main")
	require.NoError(t, err)
	assert.Equal(t, "/repo/.jvs/worktrees/main/config.json", path)

	path, err = repo.WorktreeConfigPath("/repo", "feature")
	require.NoError(t, err)
	assert.Equal(t, "/repo/.jvs/worktrees/feature/config.json", path)
}

func TestWorktreePayloadPath(t *testing.T) {
	path, err := repo.WorktreePayloadPath("/repo", "main")
	require.NoError(t, err)
	assert.Equal(t, "/repo/main", path)

	path, err = repo.WorktreePayloadPath("/repo", "feature")
	require.NoError(t, err)
	assert.Equal(t, "/repo/worktrees/feature", path)
}

func TestWorktreePathHelpersRejectInvalidNames(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	for _, name := range []string{
		"",
		"../victim",
		"nested/victim",
		filepath.Join(string(os.PathSeparator), "abs-victim"),
		"..",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := repo.WorktreeConfigPath(repoPath, name)
			require.ErrorIs(t, err, errclass.ErrNameInvalid)

			_, err = repo.WorktreeConfigDirPath(repoPath, name)
			require.ErrorIs(t, err, errclass.ErrNameInvalid)

			_, err = repo.WorktreePayloadPath(repoPath, name)
			require.ErrorIs(t, err, errclass.ErrNameInvalid)
		})
	}
}

func TestLoadWorktreeConfigRejectsInvalidNameBeforeTraversal(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	traversedConfig := filepath.Join(repoPath, ".jvs", "victim", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(traversedConfig), 0755))
	require.NoError(t, os.WriteFile(traversedConfig, []byte(`{"name":"../victim"}`), 0644))

	_, err = repo.LoadWorktreeConfig(repoPath, "../victim")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestLoadWorktreeConfigRejectsSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "feature"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "feature", "config.json"), []byte(`{"name":"feature"}`), 0644))
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "worktrees")))
	if err := os.Symlink(outside, filepath.Join(repoPath, ".jvs", "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err = repo.LoadWorktreeConfig(repoPath, "feature")
	require.Error(t, err)
}

func TestLoadWorktreeConfigRejectsFinalConfigSymlink(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	outsideConfig := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(outsideConfig, []byte(`{"name":"feature"}`), 0644))
	cfgDir := filepath.Join(repoPath, ".jvs", "worktrees", "feature")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	if err := os.Symlink(outsideConfig, filepath.Join(cfgDir, "config.json")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err = repo.LoadWorktreeConfig(repoPath, "feature")
	require.Error(t, err)
}

func TestWriteWorktreeConfigRejectsSymlinkParentWithoutOutsideWrite(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "feature"), 0755))
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "worktrees")))
	if err := os.Symlink(outside, filepath.Join(repoPath, ".jvs", "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err = repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{Name: "feature"})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(outside, "feature", "config.json"))
}

func TestSnapshotPathHelpersRejectInvalidSnapshotIDs(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	for _, id := range []model.SnapshotID{
		"",
		"/tmp/evil",
		"../../victim",
		"1708300800000/nothex",
		"1708300800000-A3F7C1B2",
	} {
		t.Run(string(id), func(t *testing.T) {
			_, err := repo.SnapshotPath(repoPath, id)
			require.Error(t, err)
			_, err = repo.SnapshotDescriptorPath(repoPath, id)
			require.Error(t, err)
			_, err = repo.GCTombstonePath(repoPath, id)
			require.Error(t, err)
		})
	}
}

func TestSnapshotPathHelpersReturnCanonicalPaths(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	_, err := repo.Init(repoPath, "repo")
	require.NoError(t, err)
	id := model.SnapshotID("1708300800000-a3f7c1b2")

	snapshotPath, err := repo.SnapshotPath(repoPath, id)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".jvs", "snapshots", string(id)), snapshotPath)

	descriptorPath, err := repo.SnapshotDescriptorPath(repoPath, id)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".jvs", "descriptors", string(id)+".json"), descriptorPath)

	tombstonePath, err := repo.GCTombstonePath(repoPath, id)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(id)+".json"), tombstonePath)
}

func TestSnapshotPathHelpersRejectSymlinkControlParents(t *testing.T) {
	id := model.SnapshotID("1708300800000-a3f7c1b2")

	tests := []struct {
		name       string
		controlDir string
		call       func(string) (string, error)
	}{
		{
			name:       "worktrees",
			controlDir: filepath.Join(".jvs", "worktrees"),
			call: func(repoPath string) (string, error) {
				return repo.WorktreesDirPath(repoPath)
			},
		},
		{
			name:       "snapshots",
			controlDir: filepath.Join(".jvs", "snapshots"),
			call: func(repoPath string) (string, error) {
				return repo.SnapshotPath(repoPath, id)
			},
		},
		{
			name:       "intents",
			controlDir: filepath.Join(".jvs", "intents"),
			call: func(repoPath string) (string, error) {
				return repo.IntentsDirPath(repoPath)
			},
		},
		{
			name:       "descriptors",
			controlDir: filepath.Join(".jvs", "descriptors"),
			call: func(repoPath string) (string, error) {
				return repo.SnapshotDescriptorPath(repoPath, id)
			},
		},
		{
			name:       "tombstones",
			controlDir: filepath.Join(".jvs", "gc", "tombstones"),
			call: func(repoPath string) (string, error) {
				return repo.GCTombstonePath(repoPath, id)
			},
		},
		{
			name:       "gc pins",
			controlDir: filepath.Join(".jvs", "gc", "pins"),
			call: func(repoPath string) (string, error) {
				return repo.GCPinsDirPath(repoPath)
			},
		},
		{
			name:       "legacy pins",
			controlDir: filepath.Join(".jvs", "pins"),
			call: func(repoPath string) (string, error) {
				return repo.LegacyPinsDirPath(repoPath)
			},
		},
		{
			name:       "gc",
			controlDir: filepath.Join(".jvs", "gc"),
			call: func(repoPath string) (string, error) {
				return repo.GCPlanPath(repoPath, "plan-123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := filepath.Join(t.TempDir(), "repo")
			_, err := repo.Init(repoPath, "repo")
			require.NoError(t, err)

			outside := t.TempDir()
			controlPath := filepath.Join(repoPath, tt.controlDir)
			require.NoError(t, os.RemoveAll(controlPath))
			if err := os.Symlink(outside, controlPath); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}

			_, err = tt.call(repoPath)
			require.Error(t, err)
		})
	}
}

func TestControlLeafHelpersRejectFinalSymlinks(t *testing.T) {
	id := model.SnapshotID("1708300800000-a3f7c1b2")

	tests := []struct {
		name     string
		leafPath func(string) string
		ops      []func(string) (string, error)
	}{
		{
			name: "snapshot dir",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "snapshots", string(id))
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.SnapshotPathForRead(repoPath, id) },
				func(repoPath string) (string, error) { return repo.SnapshotPathForDelete(repoPath, id) },
			},
		},
		{
			name: "descriptor",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "descriptors", string(id)+".json")
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.SnapshotDescriptorPathForRead(repoPath, id) },
				func(repoPath string) (string, error) { return repo.SnapshotDescriptorPathForWrite(repoPath, id) },
				func(repoPath string) (string, error) { return repo.SnapshotDescriptorPathForDelete(repoPath, id) },
			},
		},
		{
			name: "plan",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "gc", "plan-123.json")
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.GCPlanPathForRead(repoPath, "plan-123") },
				func(repoPath string) (string, error) { return repo.GCPlanPathForWrite(repoPath, "plan-123") },
				func(repoPath string) (string, error) { return repo.GCPlanPathForDelete(repoPath, "plan-123") },
			},
		},
		{
			name: "tombstone",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(id)+".json")
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.GCTombstonePathForRead(repoPath, id) },
				func(repoPath string) (string, error) { return repo.GCTombstonePathForWrite(repoPath, id) },
				func(repoPath string) (string, error) { return repo.GCTombstonePathForDelete(repoPath, id) },
			},
		},
		{
			name: "pin",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "gc", "pins", string(id)+".json")
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.GCPinPathForRead(repoPath, string(id)+".json") },
			},
		},
		{
			name: "legacy pin",
			leafPath: func(repoPath string) string {
				return filepath.Join(repoPath, ".jvs", "pins", string(id)+".json")
			},
			ops: []func(string) (string, error){
				func(repoPath string) (string, error) { return repo.LegacyPinPathForRead(repoPath, string(id)+".json") },
			},
		},
	}

	for _, tt := range tests {
		for i, op := range tt.ops {
			t.Run(tt.name, func(t *testing.T) {
				repoPath := filepath.Join(t.TempDir(), "repo")
				_, err := repo.Init(repoPath, "repo")
				require.NoError(t, err)

				outside := filepath.Join(t.TempDir(), "outside")
				require.NoError(t, os.MkdirAll(outside, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0644))

				leaf := tt.leafPath(repoPath)
				require.NoError(t, os.MkdirAll(filepath.Dir(leaf), 0755))
				if err := os.Symlink(outside, leaf); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}

				_, err = op(repoPath)
				require.Error(t, err, "operation %d should reject final symlink", i)
				assert.Contains(t, err.Error(), "symlink")
				assert.FileExists(t, filepath.Join(outside, "secret.txt"))
			})
		}
	}
}

func TestGCPlanPathRejectsTraversalPlanID(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	for _, planID := range []string{"", "../plan", "../../victim", "/tmp/plan", `a\b`} {
		t.Run(planID, func(t *testing.T) {
			_, err := repo.GCPlanPath(repoPath, planID)
			require.Error(t, err)
		})
	}
}

func TestGCPlanPathAllowsSafePlanID(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	_, err := repo.Init(repoPath, "repo")
	require.NoError(t, err)
	path, err := repo.GCPlanPath(repoPath, "plan-123")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".jvs", "gc", "plan-123.json"), path)
}

func TestWriteAndLoadWorktreeConfig(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	// Load existing config
	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Name)

	// Modify and write
	cfg.HeadSnapshotID = "1708300800000-abc12345"
	err = repo.WriteWorktreeConfig(repoPath, "main", cfg)
	require.NoError(t, err)

	// Load again
	cfg2, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID("1708300800000-abc12345"), cfg2.HeadSnapshotID)
}

func TestLoadWorktreeConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	_, err = repo.LoadWorktreeConfig(repoPath, "nonexistent")
	assert.Error(t, err)
}

func TestDiscover_WrongFormatVersion(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// Overwrite format_version with higher version
	formatFile := filepath.Join(repoPath, ".jvs", "format_version")
	err = os.WriteFile(formatFile, []byte("999\n"), 0644)
	require.NoError(t, err)

	_, err = repo.Discover(repoPath)
	assert.Error(t, err)
}

func TestDiscover_MissingFormatVersion(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// Remove format_version
	formatFile := filepath.Join(repoPath, ".jvs", "format_version")
	os.Remove(formatFile)

	_, err = repo.Discover(repoPath)
	assert.Error(t, err)
}
