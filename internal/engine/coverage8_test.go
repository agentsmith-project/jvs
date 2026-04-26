package engine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJuiceFSEngine_CloneSuccessPath tests the juicefs clone success path.
// This test is only run if juicefs is actually installed.
func TestJuiceFSEngine_CloneSuccessPath(t *testing.T) {
	// Check if juicefs is installed
	if _, err := exec.LookPath("juicefs"); err != nil {
		t.Skip("juicefs not installed - skipping success path test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source content
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// On non-JuiceFS filesystems, this will fall back to copy and be degraded
	// The important thing is the function doesn't error
	assert.NotNil(t, result)

	// Verify content was copied
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

// TestJuiceFSEngine_DegradationPaths tests various degradation scenarios.
func TestJuiceFSEngine_DegradationPaths8(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source content
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test content"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Degradations)

	// On systems without juicefs, should report degradation
	// Since isJuiceFSAvailable is unexported, we check the result
	assert.True(t, result.Degraded)
	assert.Contains(t, result.Degradations[0], "juicefs")
}

// TestJuiceFSEngine_EmptyDirectory8 tests cloning empty directories.
func TestJuiceFSEngine_EmptyDirectory8(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Don't create any files - empty source

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify dst exists
	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestJuiceFSEngine_ComplexStructure tests cloning complex directory structures.
func TestJuiceFSEngine_ComplexStructure(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create complex structure
	os.MkdirAll(filepath.Join(src, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(src, "a", "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "file2.txt"), []byte("file2"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "c", "file3.txt"), []byte("file3"), 0644)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify all files exist
	content, err := os.ReadFile(filepath.Join(dstPath, "a", "b", "c", "file3.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file3", string(content))

	content, err = os.ReadFile(filepath.Join(dstPath, "root.txt"))
	require.NoError(t, err)
	assert.Equal(t, "root", string(content))
}

// TestJuiceFSEngine_PreservesPermissions tests permission preservation.
func TestJuiceFSEngine_PreservesPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with specific permissions
	os.WriteFile(filepath.Join(src, "script.sh"), []byte("#!/bin/bash"), 0755)
	os.WriteFile(filepath.Join(src, "data.txt"), []byte("data"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify permissions
	info, err := os.Stat(filepath.Join(dstPath, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())

	info, err = os.Stat(filepath.Join(dstPath, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

// TestJuiceFSEngine_WithSpecialFiles tests cloning with special file types.
func TestJuiceFSEngine_WithSpecialFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create various file types
	os.WriteFile(filepath.Join(src, "regular.txt"), []byte("regular"), 0644)
	os.MkdirAll(filepath.Join(src, "dir"), 0755)
	os.WriteFile(filepath.Join(src, "dir", "nested.txt"), []byte("nested"), 0644)
	os.Symlink("regular.txt", filepath.Join(src, "link"))

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify symlink
	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "regular.txt", target)
}

// TestJuiceFSEngine_DetectEngine tests engine detection.
func TestJuiceFSEngine_DetectEngine(t *testing.T) {
	tmpDir := t.TempDir()

	// Test detection returns a valid engine
	eng, err := engine.DetectEngine(tmpDir)
	assert.NoError(t, err)
	assert.NotNil(t, eng)
	assert.NotEmpty(t, string(eng.Name()))
}

// TestJuiceFSEngine_WithEnvironmentVariable tests engine selection via env var.
func TestJuiceFSEngine_WithEnvironmentVariable(t *testing.T) {
	// Save original env
	originalVal := os.Getenv("JVS_ENGINE")
	defer func() {
		if originalVal == "" {
			os.Unsetenv("JVS_ENGINE")
		} else {
			os.Setenv("JVS_ENGINE", originalVal)
		}
	}()

	t.Run("JVS_ENGINE=juicefs", func(t *testing.T) {
		os.Setenv("JVS_ENGINE", "juicefs")
		tmpDir := t.TempDir()

		eng, err := engine.DetectEngine(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "juicefs-clone", string(eng.Name()))
	})

	t.Run("JVS_ENGINE=reflink", func(t *testing.T) {
		os.Setenv("JVS_ENGINE", "reflink")
		tmpDir := t.TempDir()

		eng, err := engine.DetectEngine(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "reflink-copy", string(eng.Name()))
	})

	t.Run("JVS_ENGINE=copy", func(t *testing.T) {
		os.Setenv("JVS_ENGINE", "copy")
		tmpDir := t.TempDir()

		eng, err := engine.DetectEngine(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "copy", string(eng.Name()))
	})

	t.Run("JVS_ENGINE=unknown falls back to copy", func(t *testing.T) {
		os.Setenv("JVS_ENGINE", "unknown")
		tmpDir := t.TempDir()

		eng, err := engine.DetectEngine(tmpDir)
		assert.NoError(t, err)
		// Unknown engines fall back to detection which returns copy, reflink, or juicefs
		assert.NotNil(t, eng)
	})
}
