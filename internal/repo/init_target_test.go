package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitTarget_AllowsAbsoluteMultiLevelMissingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "parent", "child", "repo")

	r, err := InitTarget(target)
	require.NoError(t, err)
	require.Equal(t, target, r.Root)
	require.DirExists(t, filepath.Join(target, ".jvs"))
	require.DirExists(t, filepath.Join(target, "main"))
	require.DirExists(t, filepath.Join(target, "worktrees"))
}

func TestInitTarget_AllowsEmptyExistingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.MkdirAll(target, 0755))

	r, err := InitTarget(target)
	require.NoError(t, err)
	require.Equal(t, target, r.Root)
	require.DirExists(t, filepath.Join(target, ".jvs"))
}

func TestInitTarget_RejectsNonEmptyTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(target, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "file.txt"), []byte("data"), 0644))

	_, err := InitTarget(target)
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(target, ".jvs"))
}

func TestInitTarget_RejectsExistingJVSMetadata(t *testing.T) {
	target := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(target, ".jvs"), 0755))

	_, err := InitTarget(target)
	require.Error(t, err)
}

func TestInitTarget_RejectsNestedRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "outer")
	_, err := InitTarget(root)
	require.NoError(t, err)

	nested := filepath.Join(root, "main", "nested")
	_, err = InitTarget(nested)
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(nested, ".jvs"))
}

func TestInitTarget_RejectsPhysicalNestedRepositoryViaSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}

	base := t.TempDir()
	root := filepath.Join(base, "outer")
	_, err := InitTarget(root)
	require.NoError(t, err)

	linkParent := filepath.Join(base, "link-to-main")
	if err := os.Symlink(filepath.Join(root, "main"), linkParent); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	target := filepath.Join(linkParent, "nested")
	_, err = InitTarget(target)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nested repository")
	require.NoDirExists(t, filepath.Join(root, "main", "nested", ".jvs"))
}
