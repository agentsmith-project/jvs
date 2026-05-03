package repo_test

import (
	"encoding/json"
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

	// Verify main workspace is the repo root, without a legacy main/ payload.
	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, repoPath, cfg.RealPath)
	assert.NoDirExists(t, filepath.Join(repoPath, "main"))

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
	assert.Equal(t, repoPath, cfg.RealPath)
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

func TestDiscover_NotFoundIsTyped(t *testing.T) {
	_, err := repo.Discover(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.ErrControlRepoNotFound)
}

func TestDiscover_MalformedLocatorIsNotTypedNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, repo.JVSDirName), []byte("{not-json"), 0644))

	_, err := repo.Discover(dir)
	require.Error(t, err)
	assert.NotErrorIs(t, err, repo.ErrControlRepoNotFound)
	assert.Contains(t, err.Error(), "parse JVS workspace locator")
}

func TestDiscoverControlRepo_NotFoundIsTyped(t *testing.T) {
	_, err := repo.DiscoverControlRepo(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, repo.ErrControlRepoNotFound)
}

func TestDiscoverControlRepo_PropagatesControlStatError(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "outer")
	_, err := repo.Init(repoPath, "outer")
	require.NoError(t, err)

	child := filepath.Join(repoPath, "main", "child")
	require.NoError(t, os.MkdirAll(child, 0755))
	loop := filepath.Join(child, repo.JVSDirName)
	require.NoError(t, os.Symlink(repo.JVSDirName, loop))

	_, err = repo.DiscoverControlRepo(child)
	require.Error(t, err)
	assert.NotErrorIs(t, err, repo.ErrControlRepoNotFound)
	assert.Contains(t, err.Error(), "stat JVS control directory")
}

func TestDiscoverWorktree_MainWorktree(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	// From repo root, which is the main workspace folder.
	r, wtName, err := repo.DiscoverWorktree(repoPath)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "main", wtName)

	// From nested path in the repo root.
	nested := filepath.Join(repoPath, "deep", "path")
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

	// Create a named external workspace.
	wtPath := filepath.Join(dir, "feature")
	require.NoError(t, os.MkdirAll(wtPath, 0755))
	require.NoError(t, repo.WriteWorkspaceLocator(wtPath, repoPath, "feature"))

	// Create config for worktree
	cfgDir := filepath.Join(repoPath, ".jvs", "worktrees", "feature")
	require.NoError(t, os.MkdirAll(cfgDir, 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{Name: "feature", RealPath: wtPath}))

	// Discover from named worktree
	r, wtName, err := repo.DiscoverWorktree(wtPath)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "feature", wtName)
}

func TestWorktreeManagedPayloadBoundaryEmbeddedMainExcludesControlDir(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	boundary, err := repo.WorktreeManagedPayloadBoundary(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, repoPath, boundary.Root)
	assert.Equal(t, []string{repo.JVSDirName}, boundary.ExcludedRootNames)
}

func TestWorktreeManagedPayloadBoundaryEmbeddedExternalLocatorStillExcluded(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	wtPath := filepath.Join(dir, "feature")
	require.NoError(t, os.MkdirAll(wtPath, 0755))
	require.NoError(t, repo.WriteWorkspaceLocator(wtPath, repoPath, "feature"))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{Name: "feature", RealPath: wtPath}))

	boundary, err := repo.WorktreeManagedPayloadBoundary(repoPath, "feature")
	require.NoError(t, err)
	assert.Equal(t, wtPath, boundary.Root)
	assert.Equal(t, []string{repo.JVSDirName}, boundary.ExcludedRootNames)
}

func TestDiscoverWorktreeTreatsLegacyPayloadFolderUnderRepoRootAsMain(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myrepo")
	_, err := repo.Init(repoPath, "myrepo")
	require.NoError(t, err)

	legacyPathInsideMain := filepath.Join(repoPath, "worktrees", "main", "nested")
	require.NoError(t, os.MkdirAll(legacyPathInsideMain, 0755))

	r, wtName, err := repo.DiscoverWorktree(legacyPathInsideMain)
	require.NoError(t, err)
	assert.Equal(t, repoPath, r.Root)
	assert.Equal(t, "main", wtName)
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
	repoPath := filepath.Join(t.TempDir(), "repo")
	_, err := repo.Init(repoPath, "repo")
	require.NoError(t, err)

	path, err := repo.WorktreePayloadPath(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, repoPath, path)

	featurePath := filepath.Join(filepath.Dir(repoPath), "feature")
	require.NoError(t, os.MkdirAll(featurePath, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{Name: "feature", RealPath: featurePath}))

	path, err = repo.WorktreePayloadPath(repoPath, "feature")
	require.NoError(t, err)
	assert.Equal(t, featurePath, path)
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

func TestWriteWorkspaceLocatorRejectsMalformedExistingRepoRoot(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))

	for _, tc := range []struct {
		name     string
		repoRoot string
	}{
		{name: "blank", repoRoot: ""},
		{name: "relative", repoRoot: "relative/repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := writeRepoTestWorkspaceLocator(t, workspace, tc.repoRoot)

			err := repo.WriteWorkspaceLocator(workspace, repoPath, "feature")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "repo_root")

			after, readErr := os.ReadFile(filepath.Join(workspace, repo.JVSDirName))
			require.NoError(t, readErr)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func TestWriteWorkspaceLocatorIncludesRepoIdentityAndWorkspaceName(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	r, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))

	require.NoError(t, repo.WriteWorkspaceLocator(workspace, repoPath, "feature"))

	data, err := os.ReadFile(filepath.Join(workspace, repo.JVSDirName))
	require.NoError(t, err)
	var locator map[string]any
	require.NoError(t, json.Unmarshal(data, &locator))
	assert.Equal(t, "jvs-workspace", locator["type"])
	assert.Equal(t, float64(repo.FormatVersion), locator["format_version"])
	assert.Equal(t, repoPath, locator["repo_root"])
	assert.Equal(t, r.RepoID, locator["repo_id"])
	assert.Equal(t, "feature", locator["workspace_name"])
}

func TestDiscoverWorktreeFromExternalLocatorUsesLocatorWorkspaceName(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	r, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	nested := filepath.Join(workspace, "nested")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{
		Name:     "feature",
		RealPath: workspace,
	}))
	require.NoError(t, repo.WriteWorkspaceLocator(workspace, repoPath, "feature"))

	discovered, workspaceName, err := repo.DiscoverWorktree(nested)
	require.NoError(t, err)
	assert.Equal(t, r.Root, discovered.Root)
	assert.Equal(t, "feature", workspaceName)
}

func TestDiscoverWorktreeLocatorWorkspaceMismatchFailsClearly(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	r, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "feature", &model.WorktreeConfig{
		Name:     "feature",
		RealPath: workspace,
	}))
	writeRepoTestWorkspaceLocator(t, workspace, repoPath, r.RepoID, "missing")

	_, _, err = repo.DiscoverWorktree(workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locator")
	assert.Contains(t, err.Error(), "workspace")
	assert.Contains(t, err.Error(), "registry")
}

func TestDiscoverWorktreeLocatorRepoIDMismatchFailsClearly(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	writeRepoTestWorkspaceLocator(t, workspace, repoPath, "wrong-repo-id", "feature")

	_, _, err = repo.DiscoverWorktree(workspace)
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrRepoIDMismatch)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected JVS error, got %T: %v", err, err)
	assert.Equal(t, errclass.ErrRepoIDMismatch.Code, jvsErr.Code)
	assert.Contains(t, err.Error(), "locator")
	assert.Contains(t, err.Error(), "repo_id")
}

func TestWorkspaceLocatorMatchesRepoWorkspaceRejectsWrongWorkspaceName(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	r, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	writeRepoTestWorkspaceLocator(t, workspace, repoPath, r.RepoID, "wrong-name")

	matches, err := repo.WorkspaceLocatorMatchesRepoWorkspace(workspace, repoPath, "feature")
	require.NoError(t, err)
	assert.False(t, matches)

	diagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         workspace,
		ExpectedRepoRoot:      repoPath,
		ExpectedRepoID:        r.RepoID,
		ExpectedWorkspaceName: "feature",
	})
	require.NoError(t, err)
	assert.True(t, diagnostic.Present)
	assert.False(t, diagnostic.Matches)
	assert.Contains(t, diagnostic.Reason, "workspace_name")
}

func TestRewriteWorkspaceLocatorRequiresExpectedIdentity(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	r, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	t.Run("rewrites workspace name after all expected identity matches", func(t *testing.T) {
		workspace := filepath.Join(dir, "workspace-success")
		require.NoError(t, os.MkdirAll(workspace, 0755))
		require.NoError(t, repo.WriteWorkspaceLocator(workspace, repoPath, "feature"))

		err := repo.RewriteWorkspaceLocator(repo.RewriteWorkspaceLocatorRequest{
			WorkspaceRoot:         workspace,
			ExpectedRepoID:        r.RepoID,
			ExpectedRepoRoot:      repoPath,
			ExpectedWorkspaceName: "feature",
			NewRepoRoot:           repoPath,
			NewWorkspaceName:      "review",
		})
		require.NoError(t, err)

		diagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
			WorkspaceRoot:         workspace,
			ExpectedRepoRoot:      repoPath,
			ExpectedRepoID:        r.RepoID,
			ExpectedWorkspaceName: "review",
		})
		require.NoError(t, err)
		assert.True(t, diagnostic.Matches, diagnostic.Reason)
	})

	for _, tc := range []struct {
		name   string
		mutate func(repo.RewriteWorkspaceLocatorRequest) repo.RewriteWorkspaceLocatorRequest
	}{
		{
			name: "repo id mismatch",
			mutate: func(req repo.RewriteWorkspaceLocatorRequest) repo.RewriteWorkspaceLocatorRequest {
				req.ExpectedRepoID = "wrong-repo-id"
				return req
			},
		},
		{
			name: "old repo root mismatch",
			mutate: func(req repo.RewriteWorkspaceLocatorRequest) repo.RewriteWorkspaceLocatorRequest {
				req.ExpectedRepoRoot = filepath.Join(dir, "different-old-root")
				return req
			},
		},
		{
			name: "workspace name mismatch",
			mutate: func(req repo.RewriteWorkspaceLocatorRequest) repo.RewriteWorkspaceLocatorRequest {
				req.ExpectedWorkspaceName = "wrong-name"
				return req
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := filepath.Join(dir, "workspace-"+tc.name)
			require.NoError(t, os.MkdirAll(workspace, 0755))
			require.NoError(t, repo.WriteWorkspaceLocator(workspace, repoPath, "feature"))
			before, err := os.ReadFile(filepath.Join(workspace, repo.JVSDirName))
			require.NoError(t, err)

			req := repo.RewriteWorkspaceLocatorRequest{
				WorkspaceRoot:         workspace,
				ExpectedRepoID:        r.RepoID,
				ExpectedRepoRoot:      repoPath,
				ExpectedWorkspaceName: "feature",
				NewRepoRoot:           repoPath,
				NewWorkspaceName:      "review",
			}
			err = repo.RewriteWorkspaceLocator(tc.mutate(req))
			require.ErrorIs(t, err, errclass.ErrLifecycleIdentityMismatch)

			after, readErr := os.ReadFile(filepath.Join(workspace, repo.JVSDirName))
			require.NoError(t, readErr)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func writeRepoTestWorkspaceLocator(t *testing.T, dir, repoRoot string, fields ...string) []byte {
	t.Helper()

	locator := map[string]any{
		"type":           "jvs-workspace",
		"format_version": repo.FormatVersion,
		"repo_root":      repoRoot,
	}
	if len(fields) > 0 {
		locator["repo_id"] = fields[0]
	}
	if len(fields) > 1 {
		locator["workspace_name"] = fields[1]
	}
	data, err := json.Marshal(locator)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, repo.JVSDirName), data, 0644))
	return data
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

func TestWorktreeConfigPersistsPathSources(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "testrepo")
	_, err := repo.Init(repoPath, "testrepo")
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	source := model.SnapshotID("1708300800000-abc12345")
	cfg.PathSources = model.NewPathSources()
	require.NoError(t, cfg.PathSources.Restore("src/app.txt", source))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	loaded, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	entry, ok, err := loaded.PathSources.SourceForPath("src/app.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/app.txt", entry.TargetPath)
	assert.Equal(t, source, entry.SourceSnapshotID)
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
