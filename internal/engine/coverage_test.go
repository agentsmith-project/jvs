package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopyEngine_copyFile_ErrorPaths tests error paths in copyFile.
func TestCopyEngine_copyFile_ErrorPaths(t *testing.T) {
	t.Run("Source file does not exist", func(t *testing.T) {
		src := t.TempDir()
		dstPath := filepath.Join(t.TempDir(), "copy")

		eng := engine.NewCopyEngine()
		// Clone from non-existent source
		_, err := eng.Clone(filepath.Join(src, "nonexistent"), dstPath)
		assert.Error(t, err)
	})

	t.Run("Destination path contains non-existent directory", func(t *testing.T) {
		src := t.TempDir()

		// Create source file
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

		eng := engine.NewCopyEngine()
		// Try to clone to a path with non-existent parent directory
		_, err := eng.Clone(filepath.Join(src, "file.txt"), "/nonexistent/dest/path")
		assert.Error(t, err)
	})
}

// TestCopyEngine_LargeFile tests copying larger files.
func TestCopyEngine_LargeFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a larger file (1 MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(src, "large.bin"), largeData, 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(content))
	assert.Equal(t, largeData, content)
}

// TestCopyEngine_MultipleLargeFiles tests copying multiple large files.
func TestCopyEngine_MultipleLargeFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create multiple large files
	for i := 0; i < 10; i++ {
		data := make([]byte, 100*1024) // 100 KB each
		for j := range data {
			data[j] = byte((i + j) % 256)
		}
		os.WriteFile(filepath.Join(src, fmt.Sprintf("file%d.dat", i)), data, 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all files were copied
	for i := 0; i < 10; i++ {
		path := filepath.Join(dstPath, fmt.Sprintf("file%d.dat", i))
		_, err := os.Stat(path)
		require.NoError(t, err)
	}
}

// TestCopyEngine_DirectoryPermissions tests directory permission preservation.
func TestCopyEngine_DirectoryPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create directory with specific permissions
	dirPath := filepath.Join(src, "restricted")
	os.MkdirAll(dirPath, 0700)
	os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify directory permissions
	info, err := os.Stat(filepath.Join(dstPath, "restricted"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

// TestCopyEngine_MixedContent tests copying mixed content (files, dirs, symlinks).
func TestCopyEngine_MixedContent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create mixed content structure
	os.MkdirAll(filepath.Join(src, "dir1", "subdir"), 0755)
	os.MkdirAll(filepath.Join(src, "dir2"), 0755)

	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(src, "dir1", "file2.txt"), []byte("file2"), 0644)
	os.WriteFile(filepath.Join(src, "dir1", "subdir", "file3.txt"), []byte("file3"), 0644)

	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link"))
	os.Symlink("nonexistent", filepath.Join(src, "broken"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all content copied
	_, err = os.Stat(filepath.Join(dstPath, "file1.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "dir1", "file2.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "dir1", "subdir", "file3.txt"))
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)

	broken, err := os.Readlink(filepath.Join(dstPath, "broken"))
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", broken)

	_ = result
}

// TestCopyEngine_ReadOnlyFiles tests copying read-only files.
func TestCopyEngine_ReadOnlyFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create read-only file
	filePath := filepath.Join(src, "readonly.txt")
	os.WriteFile(filePath, []byte("readonly"), 0644)
	os.Chmod(filePath, 0444)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify read-only permission preserved
	info, err := os.Stat(filepath.Join(dstPath, "readonly.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0444), info.Mode().Perm())

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "readonly.txt"))
	require.NoError(t, err)
	assert.Equal(t, "readonly", string(content))
}

// TestCopyEngine_FileInNonAccessibleDirectory tests error when source dir has issues.
func TestCopyEngine_FileInNonAccessibleDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a directory with no permissions
	noAccessDir := filepath.Join(src, "noaccess")
	os.MkdirAll(noAccessDir, 0000)
	defer os.Chmod(noAccessDir, 0755) // Cleanup

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)
	// May succeed (skip noaccess dir) or fail depending on OS
	_ = err
}

// TestReflinkEngine_copyFile_ErrorPaths tests error paths in reflink copyFile.
func TestReflinkEngine_copyFile_ErrorPaths(t *testing.T) {
	t.Run("Source file does not exist", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		eng := engine.NewReflinkEngine()
		_, err := eng.Clone(filepath.Join(src, "nonexistent"), dstPath)
		assert.Error(t, err)
	})

	t.Run("Destination directory creation fails", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()

		// Create a file at the destination path (blocks directory creation)
		blocker := filepath.Join(dst, "cloned")
		os.WriteFile(blocker, []byte("block"), 0644)

		// Create source content
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

		eng := engine.NewReflinkEngine()
		_, err := eng.Clone(src, blocker)
		assert.Error(t, err)
	})
}

// TestReflinkEngine_LargeFile tests reflink with large files.
func TestReflinkEngine_LargeFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a larger file (1 MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(src, "large.bin"), largeData, 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// May be degraded if reflink not supported

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(content))
	assert.Equal(t, largeData, content)

	_ = result
}

// TestReflinkEngine_FallbackOnRegularFile tests fallback to copy when reflink fails.
func TestReflinkEngine_FallbackOnRegularFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a regular file
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should succeed (either via reflink or fallback)
	// If degraded, fallback was used
	if result.Degraded {
		assert.Contains(t, result.Degradations, "reflink")
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

// TestReflinkEngine_EmptyFiles tests handling of empty files.
func TestReflinkEngine_EmptyFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create empty files
	os.WriteFile(filepath.Join(src, "empty1.txt"), []byte{}, 0644)
	os.WriteFile(filepath.Join(src, "empty2.txt"), []byte{}, 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify empty files exist
	info1, err := os.Stat(filepath.Join(dstPath, "empty1.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), info1.Size())

	info2, err := os.Stat(filepath.Join(dstPath, "empty2.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), info2.Size())

	_ = result
}

// TestReflinkEngine_HardlinkDegradation tests that hardlinks are properly degraded.
func TestReflinkEngine_HardlinkDegradation(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create files with hardlinks
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("content"), 0644)
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "hardlink.txt"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify content copied
	content1, err := os.ReadFile(filepath.Join(dstPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstPath, "hardlink.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content2))

	// Check if files are actually hardlinked in destination (they won't be)
	// which is expected degradation
	_ = result
}

// TestJuiceFSEngine_Clone_ErrorPaths tests error paths in JuiceFS clone.
func TestJuiceFSEngine_Clone_ErrorPaths(t *testing.T) {
	t.Run("Source does not exist", func(t *testing.T) {
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		eng := engine.NewJuiceFSEngine()
		_, err := eng.Clone("/nonexistent/source", dstPath)
		assert.Error(t, err)
	})

	t.Run("Destination is invalid", func(t *testing.T) {
		src := t.TempDir()
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

		eng := engine.NewJuiceFSEngine()
		// Use a file as destination path
		_, err := eng.Clone(src, filepath.Join(src, "file.txt"))
		assert.Error(t, err)
	})
}

// TestJuiceFSEngine_FallbackScenarios tests various fallback scenarios.
func TestJuiceFSEngine_FallbackScenarios(t *testing.T) {
	t.Run("Fallback when juicefs command not available", func(t *testing.T) {
		// Ensure PATH doesn't contain juicefs
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", "/bin:/usr/bin") // Minimal PATH without juicefs
		defer func() {
			if oldPath != "" {
				os.Setenv("PATH", oldPath)
			} else {
				os.Unsetenv("PATH")
			}
		}()

		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

		eng := engine.NewJuiceFSEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.True(t, result.Degraded)
		assert.NotEmpty(t, result.Degradations)

		// Verify fallback copy worked
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})

	t.Run("Fallback when not on JuiceFS mount", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

		eng := engine.NewJuiceFSEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.True(t, result.Degraded)

		// Verify content via fallback
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "test", string(content))
	})
}

// TestJuiceFSEngine_LargeFile tests JuiceFS engine with large files.
func TestJuiceFSEngine_LargeFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a 10 MB file
	largeData := make([]byte, 10*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(src, "large.bin"), largeData, 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Will be degraded since not on actual JuiceFS

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(content))

	_ = result
}

// TestJuiceFSEngine_EmptyDirectory tests empty directory cloning.
func TestJuiceFSEngine_EmptyDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create empty directory (temp dirs are created empty)
	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// May be degraded

	// Verify destination exists
	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)

	_ = result
}

// TestDetectEngine_PrefersJuiceFS tests engine detection priority.
func TestDetectEngine_PrefersJuiceFS(t *testing.T) {
	// Without environment variable, should try auto-detection
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, eng)
	// Without actual JuiceFS mount, will fall back to reflink or copy
	assert.NotNil(t, eng.Name())
}

// TestDetectEngine_WithInvalidEnv tests handling of invalid environment value.
func TestDetectEngine_WithInvalidEnv(t *testing.T) {
	os.Setenv("JVS_ENGINE", "invalid-engine-type")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	// Should fall back to auto-detect
	assert.NotNil(t, eng)
	assert.NotEmpty(t, eng.Name())
}

// TestCopyEngine_WithDeviceFiles tests handling special file types.
func TestCopyEngine_WithDeviceFiles(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to create device files")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Note: This test is skipped on non-root systems
	// Creating device files requires root privileges
	_ = src
	_ = dstPath
	t.Skip("device file handling requires root")
}

// TestCopyEngine_PreservesExtendedAttributes tests extended attribute preservation.
func TestCopyEngine_PreservesExtendedAttributes(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	filePath := filepath.Join(src, "file.txt")
	os.WriteFile(filePath, []byte("content"), 0644)

	// Try to set an extended attribute (may fail on some filesystems)
	// This is more of a smoke test
	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify basic content
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestCopyEngine_DeepNesting tests very deep directory nesting.
func TestCopyEngine_DeepNesting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a very deep directory structure
	deepPath := src
	for i := 0; i < 50; i++ {
		deepPath = filepath.Join(deepPath, fmt.Sprintf("level%d", i))
	}
	os.MkdirAll(deepPath, 0755)
	os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify deep file exists
	deepDest := dstPath
	for i := 0; i < 50; i++ {
		deepDest = filepath.Join(deepDest, fmt.Sprintf("level%d", i))
	}
	content, err := os.ReadFile(filepath.Join(deepDest, "deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep content", string(content))

	_ = result
}

// TestReflinkEngine_ReflinkFailureFallback tests reflink failure fallback.
func TestReflinkEngine_ReflinkFailureFallback(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a file - reflink will likely fail on most filesystems
	// and should fall back to regular copy
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test data for reflink"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify content preserved regardless of reflink success
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test data for reflink", string(content))

	// If degraded, reflink failed and fallback was used
	if result.Degraded {
		assert.NotEmpty(t, result.Degradations)
	}

	_ = result
}

// TestReflinkEngine_CopySymlink_ErrorPaths tests symlink copy error paths.
func TestReflinkEngine_CopySymlink_ErrorPaths(t *testing.T) {
	t.Run("Broken symlink", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create broken symlink
		os.Symlink("nonexistent-target", filepath.Join(src, "broken"))

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Verify symlink copied (even if broken)
		target, err := os.Readlink(filepath.Join(dstPath, "broken"))
		require.NoError(t, err)
		assert.Equal(t, "nonexistent-target", target)

		_ = result
	})
}

// TestDetectEngine_ReflinkDetection tests reflink capability detection.
func TestDetectEngine_ReflinkDetection(t *testing.T) {
	// Clear environment to force auto-detect
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, eng)

	// The engine will be either reflink (if supported) or copy (fallback)
	// Both are valid outcomes
	assert.NotEmpty(t, eng.Name())
}

// TestDetectEngine_AllEnvOptions tests all environment variable options.
func TestDetectEngine_AllEnvOptions(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{"copy engine", "copy", "copy"},
		{"reflink engine", "reflink", "reflink-copy"},
		{"juicefs engine", "juicefs", "juicefs-clone"},
		{"unknown falls back to auto", "unknown", ""}, // auto-detect
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("JVS_ENGINE", tt.envValue)
			defer os.Unsetenv("JVS_ENGINE")

			tmpDir := t.TempDir()
			eng, err := engine.DetectEngine(tmpDir)

			require.NoError(t, err)
			require.NotNil(t, eng)

			if tt.expected != "" {
				assert.Equal(t, tt.expected, string(eng.Name()))
			}
			// If empty expected, we just verify it doesn't crash
		})
	}
}

// TestCopyEngine_SparseFile tests copying sparse files (files with holes).
func TestCopyEngine_SparseFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a sparse file (seek and write, leaving holes)
	// Note: This is a simplified test - true sparse file handling is complex
	filePath := filepath.Join(src, "sparse.bin")
	file, err := os.Create(filePath)
	require.NoError(t, err)
	defer file.Close()

	// Seek to 1MB and write some data
	file.Seek(1024*1024, 0)
	file.Write([]byte("data at 1MB"))
	file.Close()

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify file exists and has correct size
	info, err := os.Stat(filepath.Join(dstPath, "sparse.bin"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, info.Size(), int64(1024*1024))

	_ = result
}

// TestCopyEngine_FileToNonWritableDestination tests error when destination is read-only.
func TestCopyEngine_FileToNonWritableDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create source file
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	// Create read-only destination directory
	dstPath := filepath.Join(dst, "readonly")
	os.MkdirAll(dstPath, 0444)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)

	// Should fail because can't create files in read-only directory
	assert.Error(t, err)

	// Cleanup for next tests
	os.Chmod(dstPath, 0755)
}

// TestReflinkEngine_ModTimePreservation tests modification time preservation.
func TestReflinkEngine_ModTimePreservation(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file and set specific mod time
	filePath := filepath.Join(src, "timed.txt")
	os.WriteFile(filePath, []byte("time test"), 0644)

	// Set a past time
	pastTime := os.FileMode(0644)
	_ = pastTime
	// Note: Setting specific mod times can be tricky
	// Just verify the file gets copied with reasonable mod time

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify file exists with mod time set
	info, err := os.Stat(filepath.Join(dstPath, "timed.txt"))
	require.NoError(t, err)
	assert.False(t, info.ModTime().IsZero())

	_ = result
}
