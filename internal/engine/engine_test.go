package engine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyEngine_ClonePreservesFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source content
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(src, "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "subdir", "nested.txt"), []byte("world"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)
	require.NoError(t, err)
	assert.False(t, result.Degraded)
	assert.Empty(t, result.Degradations)

	// Verify content preserved
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))

	content, err = os.ReadFile(filepath.Join(dstPath, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(content))
}

func TestCopyEngine_ClonePreservesSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("target.txt", filepath.Join(src, "link")))

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestCopyEngine_ClonePreservesPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "script.sh"), []byte("#!/bin/bash"), 0755)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dstPath, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestCopyEngine_ReportsHardlinkDegradation(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file and hardlink to it
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("content"), 0644)
	require.NoError(t, os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "hardlink.txt")))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)
	require.NoError(t, err)
	// Copy engine cannot preserve hardlinks, should report degradation
	assert.True(t, result.Degraded)
	assert.Contains(t, result.Degradations, "hardlink")
}

func TestCopyEngine_Name(t *testing.T) {
	eng := engine.NewCopyEngine()
	assert.Equal(t, model.EngineCopy, eng.Name())
}

func TestCopyEngine_EmptyDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)
	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify dst exists and is empty
	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCopyEngine_SourceNotFound(t *testing.T) {
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewCopyEngine()
	_, err := eng.Clone("/nonexistent/source", dstPath)
	require.Error(t, err)
}

func TestCopyEngine_NestedDirectories(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create deeply nested structure
	os.MkdirAll(filepath.Join(src, "a", "b", "c", "d"), 0755)
	os.WriteFile(filepath.Join(src, "a", "b", "c", "d", "deep.txt"), []byte("deep"), 0644)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	require.NoError(t, err)

	// Verify deep file exists
	content, err := os.ReadFile(filepath.Join(dstPath, "a", "b", "c", "d", "deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep", string(content))
}

func TestCopyEngine_BrokenSymlink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a broken symlink
	require.NoError(t, os.Symlink("nonexistent", filepath.Join(src, "broken-link")))

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	require.NoError(t, err)

	// Symlink should be copied even if broken
	target, err := os.Readlink(filepath.Join(dstPath, "broken-link"))
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", target)
}

func TestNewEngine_Copy(t *testing.T) {
	eng := engine.NewEngine(model.EngineCopy)
	assert.Equal(t, model.EngineCopy, eng.Name())
}

func TestNewEngine_Reflink(t *testing.T) {
	eng := engine.NewEngine(model.EngineReflinkCopy)
	assert.Equal(t, model.EngineReflinkCopy, eng.Name())
}

func TestNewEngine_JuiceFS(t *testing.T) {
	eng := engine.NewEngine(model.EngineJuiceFSClone)
	assert.Equal(t, model.EngineJuiceFSClone, eng.Name())
}

func TestNewEngine_UnknownFallback(t *testing.T) {
	// Unknown engine types should fall back to Copy
	eng := engine.NewEngine(model.EngineType("unknown"))
	assert.Equal(t, model.EngineCopy, eng.Name())
}

func TestNewEngine_InvalidType(t *testing.T) {
	// Empty string should also fall back to Copy
	eng := engine.NewEngine("")
	assert.Equal(t, model.EngineCopy, eng.Name())
}

func TestCopyEngine_DestinationCreationError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file where destination directory should be
	dstPath := filepath.Join(dst, "file-blocker")
	require.NoError(t, os.WriteFile(dstPath, []byte("block"), 0644))

	// Try to clone into a path that includes a file as a directory component
	cloneTo := filepath.Join(dstPath, "subdir", "cloned")

	eng := engine.NewCopyEngine()
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	_, err := eng.Clone(src, cloneTo)
	// Should error because can't create directory inside a file
	assert.Error(t, err)
}

func TestCopyEngine_SymlinkReadError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a file (not a symlink) with link-like name
	os.WriteFile(filepath.Join(src, "fake-link"), []byte("not a link"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	// Should succeed - regular files are copied normally
	require.NoError(t, err)
	assert.False(t, result.Degraded)
}

func TestCopyEngine_PreservesModTime(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a file with specific mod time
	filePath := filepath.Join(src, "timestamp.txt")
	os.WriteFile(filePath, []byte("time test"), 0644)

	// Set a specific mod time
	pastTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	os.Chtimes(filePath, pastTime, pastTime)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	require.NoError(t, err)

	// Verify mod time was preserved
	info, err := os.Stat(filepath.Join(dstPath, "timestamp.txt"))
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(pastTime) || info.ModTime().Sub(pastTime) < time.Second)
}
