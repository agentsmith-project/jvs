package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitAdoptedWorkspace_RegistersMainRealFolderWithoutMovingFiles(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "src", "app.txt"), []byte("user data"), 0644))

	canonical := canonicalPath(t, folder)
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	require.Equal(t, canonical, r.Root)
	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Name)
	assert.Equal(t, canonical, cfg.RealPath)
	assert.FileExists(t, filepath.Join(folder, "src", "app.txt"))
	assert.DirExists(t, filepath.Join(folder, ".jvs"))
	assert.NoDirExists(t, filepath.Join(folder, "main"))
}

func TestInitAdoptedWorkspace_AllowsNonEmptyFolderAndDoesNotCreateMainPayload(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "README.md"), []byte("existing"), 0644))

	_, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(folder, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "existing", string(content))
	assert.NoDirExists(t, filepath.Join(folder, "main"))
}

func TestWorktreePayloadPath_UsesConfiguredRealPathForAdoptedMain(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))

	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	payloadPath, err := repo.WorktreePayloadPath(r.Root, "main")
	require.NoError(t, err)
	assert.Equal(t, canonicalPath(t, folder), payloadPath)
}

func TestDiscoverWorktree_AdoptedMainFromChild(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	child := filepath.Join(folder, "src", "pkg")
	require.NoError(t, os.MkdirAll(child, 0755))

	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	discovered, workspace, err := repo.DiscoverWorktree(child)
	require.NoError(t, err)
	assert.Equal(t, r.Root, discovered.Root)
	assert.Equal(t, "main", workspace)
}

func TestDiscoverWorktree_AdoptedMainDoesNotTreatControlDirAsWorkspace(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))

	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	discovered, workspace, err := repo.DiscoverWorktree(filepath.Join(folder, ".jvs", "worktrees", "main"))
	require.NoError(t, err)
	assert.Equal(t, r.Root, discovered.Root)
	assert.Equal(t, "", workspace)
}

func TestDiscoverWorktree_AdoptedControlSymlinkLexicalPathDoesNotResolveToWorkspace(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	src := filepath.Join(folder, "src", "nested")
	require.NoError(t, os.MkdirAll(src, 0755))

	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	link := filepath.Join(folder, ".jvs", "link-to-src")
	if err := os.Symlink(filepath.Join("..", "src"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	discovered, workspace, err := repo.DiscoverWorktree(filepath.Join(link, "nested"))
	require.NoError(t, err)
	assert.Equal(t, r.Root, discovered.Root)
	assert.Equal(t, "", workspace)
}

func TestInitAdoptedWorkspace_StoresCanonicalRealPathForSymlinkInput(t *testing.T) {
	base := t.TempDir()
	realFolder := filepath.Join(base, "real-project")
	linkFolder := filepath.Join(base, "link-project")
	require.NoError(t, os.MkdirAll(realFolder, 0755))
	if err := os.Symlink(realFolder, linkFolder); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	r, err := repo.InitAdoptedWorkspace(linkFolder)
	require.NoError(t, err)

	canonical := canonicalPath(t, realFolder)
	assert.Equal(t, canonical, r.Root)
	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	assert.Equal(t, canonical, cfg.RealPath)
	assert.DirExists(t, filepath.Join(realFolder, ".jvs"))
}

func TestInitAdoptedWorkspace_RejectsNestedExistingRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(root, 0755))
	_, err := repo.InitAdoptedWorkspace(root)
	require.NoError(t, err)

	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0755))
	_, err = repo.InitAdoptedWorkspace(nested)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested repository")
	assert.NoDirExists(t, filepath.Join(nested, ".jvs"))
}

func TestWorktreePayloadPath_RejectsConfiguredRealPathNameMismatch(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	cfg.Name = "not-main"
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "main", cfg))

	_, err = repo.WorktreePayloadPath(r.Root, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config name mismatch")
}

func TestWorktreePayloadPath_RejectsConfiguredRealPathInsideControlDir(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	cfg.RealPath = filepath.Join(r.Root, ".jvs", "worktrees")
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "main", cfg))

	_, err = repo.WorktreePayloadPath(r.Root, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control")
}

func TestWorktreePayloadPath_RejectsNestedFallbackUnderAdoptedMain(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	_, err = repo.WorktreePayloadPath(r.Root, "feature")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func TestWorktreePayloadPath_RejectsMetadataEntryMissingConfig(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(r.Root, ".jvs", "worktrees", "feature"), 0755))

	_, err = repo.WorktreePayloadPath(r.Root, "feature")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config")

	err = repo.ValidateWorktreeRealPathRegistry(r.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config")
}

func TestWorktreePayloadPath_RejectsConfiguredRelativeRealPath(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	cfg.RealPath = "."
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "main", cfg))

	_, err = repo.WorktreePayloadPath(r.Root, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")

	err = repo.ValidateWorktreeRealPathRegistry(r.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestWorktreePayloadPath_RejectsConfiguredWhitespaceRealPath(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	cfg, err := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, err)
	cfg.RealPath = " "
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "main", cfg))

	_, err = repo.WorktreePayloadPath(r.Root, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")

	err = repo.ValidateWorktreeRealPathRegistry(r.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestWorktreeRealPathRegistry_RejectsEqualRealPaths(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(r.Root, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "feature", configWithRealPath("feature", r.Root)))

	err = repo.ValidateWorktreeRealPathRegistry(r.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func TestWorktreeRealPathRegistry_RejectsNestedConfiguredRealPath(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "src"), 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(r.Root, ".jvs", "worktrees", "feature"), 0755))
	require.NoError(t, repo.WriteWorktreeConfig(r.Root, "feature", configWithRealPath("feature", filepath.Join(r.Root, "src"))))

	err = repo.ValidateWorktreeRealPathRegistry(r.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overlap")
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	canonical, err := filepath.EvalSymlinks(abs)
	require.NoError(t, err)
	return filepath.Clean(canonical)
}

func configWithRealPath(name, realPath string) *model.WorktreeConfig {
	return &model.WorktreeConfig{
		Name:     name,
		RealPath: realPath,
	}
}
