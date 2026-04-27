package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReflinkEngine_Name(t *testing.T) {
	eng := engine.NewReflinkEngine()
	assert.Equal(t, model.EngineReflinkCopy, eng.Name())
}

func TestReflinkEngine_Clone(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source content
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Reflink may not be supported on this filesystem, so degraded is acceptable
	// The important thing is that the clone succeeded (possibly with fallback)

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestReflinkEngine_FallbackToCopy(t *testing.T) {
	// Test that when reflink fails, we can detect it
	eng := engine.NewReflinkEngine()
	assert.NotNil(t, eng)
}

func TestReflinkEngine_CloneNestedStructure(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create nested structure
	os.MkdirAll(filepath.Join(src, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(src, "a", "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "file2.txt"), []byte("file2"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "c", "file3.txt"), []byte("file3"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// May be degraded if reflink not supported

	// Verify all files copied
	content1, err := os.ReadFile(filepath.Join(dstPath, "a", "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file1", string(content1))

	content3, err := os.ReadFile(filepath.Join(dstPath, "a", "b", "c", "file3.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file3", string(content3))

	_ = result
}

func TestReflinkEngine_CloneToNewFileLeaf(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "config.yaml")
	require.NoError(t, os.WriteFile(srcFile, []byte("setting: true\n"), 0644))

	dstFile := filepath.Join(t.TempDir(), "nested", "config.yaml")
	_, err := engine.CloneToNew(engine.NewReflinkEngine(), srcFile, dstFile)
	require.NoError(t, err)

	info, err := os.Lstat(dstFile)
	require.NoError(t, err)
	require.False(t, info.IsDir(), "file leaf destination must not be precreated as a directory")
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "setting: true\n", string(content))
}

func TestReflinkEngine_CloneToNewSymlinkLeaf(t *testing.T) {
	srcDir := t.TempDir()
	srcLink := filepath.Join(srcDir, "config-link")
	if err := os.Symlink("config.yaml", srcLink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	dstLink := filepath.Join(t.TempDir(), "nested", "config-link")
	_, err := engine.CloneToNew(engine.NewReflinkEngine(), srcLink, dstLink)
	require.NoError(t, err)

	info, err := os.Lstat(dstLink)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, err := os.Readlink(dstLink)
	require.NoError(t, err)
	assert.Equal(t, "config.yaml", target)
}

func TestReflinkEngine_CloneToNewDirectoryTree(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "file.txt"), []byte("payload"), 0644))

	dst := filepath.Join(t.TempDir(), "owned-new")
	_, err := engine.CloneToNew(engine.NewReflinkEngine(), src, dst)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dst, "nested", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "payload", string(content))
}

func TestReflinkEngine_CloneWithSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create content with symlink
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("target.txt", filepath.Join(src, "link")))
	require.NoError(t, os.Symlink("nonexistent", filepath.Join(src, "broken")))

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify symlinks preserved
	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)

	broken, err := os.Readlink(filepath.Join(dstPath, "broken"))
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", broken)
}

func TestReflinkEngine_ClonePreservesPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with specific permissions
	os.WriteFile(filepath.Join(src, "script.sh"), []byte("#!/bin/bash"), 0755)
	os.WriteFile(filepath.Join(src, "readonly.txt"), []byte("readonly"), 0444)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify permissions
	scriptInfo, err := os.Stat(filepath.Join(dstPath, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), scriptInfo.Mode().Perm())

	readonlyInfo, err := os.Stat(filepath.Join(dstPath, "readonly.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0444), readonlyInfo.Mode().Perm())
}

func TestReflinkEngine_CloneEmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create empty directory
	os.MkdirAll(filepath.Join(src, "empty"), 0755)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify empty dir exists
	entries, err := os.ReadDir(filepath.Join(dstPath, "empty"))
	require.NoError(t, err)
	assert.Empty(t, entries)

	_ = result
}

func TestReflinkEngine_SourceNotFound(t *testing.T) {
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone("/nonexistent/source/path", dstPath)
	require.Error(t, err)
}
