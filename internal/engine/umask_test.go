//go:build !windows

package engine_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyEngine_ClonePreservesModesDespiteRestrictiveUmask(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "cloned")

	writeModeFixture(t, src)

	oldUmask := syscall.Umask(0077)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dst)
	require.NoError(t, err)

	assertMode(t, filepath.Join(dst, "public-dir"), 0755)
	assertMode(t, filepath.Join(dst, "public-dir", "file.txt"), 0644)
	assertMode(t, filepath.Join(dst, "script.sh"), 0755)
}

func TestReflinkEngine_ClonePreservesModesDespiteRestrictiveUmask(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "cloned")

	writeModeFixture(t, src)

	oldUmask := syscall.Umask(0077)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dst)
	require.NoError(t, err)

	assertMode(t, filepath.Join(dst, "public-dir"), 0755)
	assertMode(t, filepath.Join(dst, "public-dir", "file.txt"), 0644)
	assertMode(t, filepath.Join(dst, "script.sh"), 0755)
}

func writeModeFixture(t *testing.T, root string) {
	t.Helper()

	dir := filepath.Join(root, "public-dir")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.Chmod(dir, 0755))

	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	require.NoError(t, os.Chmod(file, 0644))

	script := filepath.Join(root, "script.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0755))
	require.NoError(t, os.Chmod(script, 0755))
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm())
}
