package engine

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJuiceFSCloneCapturesCommandStderrDuringFallback(t *testing.T) {
	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	require.NoError(t, os.WriteFile(juicefsPath, []byte("#!/bin/sh\nprintf 'simulated juicefs failure\\n' >&2\nexit 7\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))
	dst := filepath.Join(t.TempDir(), "clone")

	eng := NewJuiceFSEngine()
	eng.isOnJuiceFSFunc = func(string) bool { return true }

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	result, cloneErr := eng.Clone(src, dst)
	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	leaked, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	require.NoError(t, cloneErr)
	assert.NotContains(t, string(leaked), "simulated juicefs failure")
	assert.Contains(t, result.Degradations, "juicefs-clone-failed")
	assert.True(t, containsDegradationFragment(result.Degradations, "simulated juicefs failure"), "degradations should retain captured stderr context: %#v", result.Degradations)
	assert.FileExists(t, filepath.Join(dst, "file.txt"))
}

func TestJuiceFSCloneToNewRemovesPartialDestinationBeforeFallback(t *testing.T) {
	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	require.NoError(t, os.WriteFile(juicefsPath, []byte("#!/bin/sh\nmkdir -p \"$3\"\nprintf stale > \"$3/stale.txt\"\nprintf 'partial clone\\n' >&2\nexit 7\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))
	dst := filepath.Join(t.TempDir(), "clone")

	eng := NewJuiceFSEngine()
	eng.isOnJuiceFSFunc = func(string) bool { return true }

	result, err := eng.CloneToNew(src, dst)
	require.NoError(t, err)
	assert.Contains(t, result.Degradations, "juicefs-clone-failed")
	assert.FileExists(t, filepath.Join(dst, "file.txt"))
	assert.NoFileExists(t, filepath.Join(dst, "stale.txt"))
}

func TestJuiceFSCloneToNewFailureCleanupPreservesLateReplacement(t *testing.T) {
	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	require.NoError(t, os.WriteFile(juicefsPath, []byte("#!/bin/sh\nmkdir -p \"$3\"\nprintf stale > \"$3/stale.txt\"\nrm -rf \"$JVS_TEST_REPLACEMENT_DST\"\nmkdir -p \"$JVS_TEST_REPLACEMENT_DST\"\nprintf user > \"$JVS_TEST_REPLACEMENT_DST/user.txt\"\nprintf 'partial clone\\n' >&2\nexit 7\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))
	dst := filepath.Join(t.TempDir(), "clone")
	t.Setenv("JVS_TEST_REPLACEMENT_DST", dst)

	eng := NewJuiceFSEngine()
	eng.isOnJuiceFSFunc = func(string) bool { return true }

	result, err := eng.CloneToNew(src, dst)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.FileExists(t, filepath.Join(dst, "user.txt"))
	assert.NoFileExists(t, filepath.Join(dst, "file.txt"))
}

func TestJuiceFSCloneToNewFailsClosedWhenPartialCleanupFails(t *testing.T) {
	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	require.NoError(t, os.WriteFile(juicefsPath, []byte("#!/bin/sh\nmkdir -p \"$3\"\nprintf stale > \"$3/stale.txt\"\nchmod a-w \"${3%/*}\"\nprintf 'partial clone\\n' >&2\nexit 7\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))
	dstParent := t.TempDir()
	dst := filepath.Join(dstParent, "clone")
	t.Cleanup(func() {
		_ = filepath.WalkDir(dstParent, func(path string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0755)
			}
			return nil
		})
		_ = os.Chmod(dstParent, 0755)
	})

	eng := NewJuiceFSEngine()
	eng.isOnJuiceFSFunc = func(string) bool { return true }

	result, err := eng.CloneToNew(src, dst)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "cleanup partial destination")
	assert.NoFileExists(t, filepath.Join(dst, "file.txt"))
}

func containsDegradationFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
