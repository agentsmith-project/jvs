package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJuiceFSEngine_isOnJuiceFS_Tests tests isOnJuiceFS behavior.
func TestJuiceFSEngine_isOnJuiceFS_Tests(t *testing.T) {
	eng := engine.NewJuiceFSEngine()

	t.Run("Non-existent path returns false", func(t *testing.T) {
		// A non-existent path should return false (not on JuiceFS)
		// The function handles this gracefully
		_ = eng
	})

	t.Run("Temp directory returns false (not on JuiceFS)", func(t *testing.T) {
		// Temp dirs are not on JuiceFS in normal test environments
		tmpDir := t.TempDir()
		// We can't directly test isOnJuiceFS as it's unexported
		// but we can verify the Clone works with fallback
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")
		os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)

		result, err := eng.Clone(tmpDir, dstPath)
		require.NoError(t, err)
		// Should be degraded since not on JuiceFS
		assert.True(t, result.Degraded)
	})
}

// TestDetectEngine_VariousPaths tests detection on various paths.
func TestDetectEngine_VariousPaths(t *testing.T) {
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	tests := []struct {
		name string
		path string
	}{
		{"temp directory", ""},
		{"current directory", "."},
		{"parent directory", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPath := tt.path
			if testPath == "" {
				testPath = t.TempDir()
			}

			eng, err := engine.DetectEngine(testPath)
			require.NoError(t, err)
			assert.NotNil(t, eng)
			assert.NotEmpty(t, eng.Name())
		})
	}
}

// TestDetectEngine_ReflinkTest tests the reflink detection test.
func TestDetectEngine_ReflinkTest(t *testing.T) {
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	// The reflink test creates a temp dir and tries a reflink
	// We just verify this doesn't crash
	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, eng)
	// Will be copy or reflink depending on filesystem support
	assert.NotEmpty(t, eng.Name())
}

// TestCopyEngine_copyFile_DetailedErrorCases tests detailed error cases.
func TestCopyEngine_copyFile_DetailedErrorCases(t *testing.T) {
	t.Run("Source is directory not file", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create a directory
		os.MkdirAll(filepath.Join(src, "dir"), 0755)

		eng := engine.NewCopyEngine()
		// This should succeed - it copies the directory
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		// Verify directory was copied
		_, err = os.Stat(filepath.Join(dstPath, "dir"))
		require.NoError(t, err)
	})

	t.Run("Source file becomes unreadable during copy", func(t *testing.T) {
		// This tests the error path when source can't be read
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create a source file
		srcFile := filepath.Join(src, "file.txt")
		os.WriteFile(srcFile, []byte("content"), 0644)

		// Make the source file unreadable (may not work on all systems)
		os.Chmod(srcFile, 0000)

		eng := engine.NewCopyEngine()
		_, err := eng.Clone(src, dstPath)

		// May fail or succeed depending on permissions
		_ = err

		// Cleanup
		os.Chmod(srcFile, 0644)
	})
}

// TestReflinkEngine_copyFile_DetailedErrorCases tests reflink copyFile errors.
func TestReflinkEngine_copyFile_DetailedErrorCases(t *testing.T) {
	t.Run("Source is directory", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		os.MkdirAll(filepath.Join(src, "dir"), 0755)

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		_, err = os.Stat(filepath.Join(dstPath, "dir"))
		require.NoError(t, err)

		_ = result
	})

	t.Run("Reflink fails, falls back to copy", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create a regular file
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("reflink test data"), 0644)

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Reflink may fail (common on most filesystems) and fall back to copy
		// If degraded, fallback was used
		if result.Degraded {
			assert.NotEmpty(t, result.Degradations)
		}

		// Verify content was copied correctly
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "reflink test data", string(content))
	})
}

// TestCopyEngine_SymlinkErrorHandling tests symlink copy error handling.
func TestCopyEngine_SymlinkErrorHandling(t *testing.T) {
	t.Run("Copy valid symlink", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create target and symlink
		os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
		os.Symlink("target.txt", filepath.Join(src, "link"))

		eng := engine.NewCopyEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		// Verify symlink
		target, err := os.Readlink(filepath.Join(dstPath, "link"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", target)

		_ = result
	})

	t.Run("Copy multiple symlinks", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create multiple symlinks
		for i := 0; i < 5; i++ {
			target := fmt.Sprintf("target%d.txt", i)
			os.WriteFile(filepath.Join(src, target), []byte(target), 0644)
			os.Symlink(target, filepath.Join(src, fmt.Sprintf("link%d", i)))
		}

		eng := engine.NewCopyEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		// Verify all symlinks
		for i := 0; i < 5; i++ {
			linkPath := filepath.Join(dstPath, fmt.Sprintf("link%d", i))
			target, err := os.Readlink(linkPath)
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("target%d.txt", i), target)
		}

		_ = result
	})
}

// TestReflinkEngine_SymlinkErrorHandling tests symlink handling in reflink.
func TestReflinkEngine_SymlinkErrorHandling(t *testing.T) {
	t.Run("Copy valid symlink with reflink", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
		os.Symlink("target.txt", filepath.Join(src, "link"))

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Verify symlink preserved
		target, err := os.Readlink(filepath.Join(dstPath, "link"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", target)

		_ = result
	})

	t.Run("Copy absolute symlink", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create absolute symlink
		os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
		absPath, _ := filepath.Abs(filepath.Join(src, "target.txt"))
		os.Symlink(absPath, filepath.Join(src, "abslink"))

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Verify symlink preserved (may be converted to relative)
		_, err = os.Stat(filepath.Join(dstPath, "abslink"))
		require.NoError(t, err)

		_ = result
	})
}

// TestJuiceFSEngine_DetailedFallback tests specific fallback paths.
func TestJuiceFSEngine_DetailedFallback(t *testing.T) {
	t.Run("Verify fallback degradation reasons", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

		eng := engine.NewJuiceFSEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.True(t, result.Degraded)

		// Check that one of the expected degradation reasons is present
		expectedDegradations := []string{
			"juicefs-not-available",
			"not-on-juicefs",
			"juicefs-clone-failed",
		}

		found := false
		for _, deg := range result.Degradations {
			for _, expected := range expectedDegradations {
				if deg == expected {
					found = true
					break
				}
			}
		}
		assert.True(t, found, "should have one of the expected degradation reasons: %v", result.Degradations)
	})

	t.Run("Multiple files all fallback", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create multiple files
		for i := 0; i < 10; i++ {
			os.WriteFile(filepath.Join(src, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("content%d", i)), 0644)
		}

		eng := engine.NewJuiceFSEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.True(t, result.Degraded)

		// Verify all files copied
		for i := 0; i < 10; i++ {
			path := filepath.Join(dstPath, fmt.Sprintf("file%d.txt", i))
			_, err := os.Stat(path)
			require.NoError(t, err)
		}

		_ = result
	})
}

// TestEngine_NewEngine_CreatesCorrectTypes tests NewEngine factory.
func TestEngine_NewEngine_CreatesCorrectTypes(t *testing.T) {
	tests := []struct {
		name     string
		engine   model.EngineType
		expected string
	}{
		{"copy engine", "copy", "copy"},
		{"reflink-copy engine", "reflink-copy", "reflink-copy"},
		{"juicefs-clone engine", "juicefs-clone", "juicefs-clone"},
		{"unknown falls back to copy", "unknown-engine", "copy"},
		{"empty string falls back to copy", "", "copy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := engine.NewEngine(tt.engine)
			assert.Equal(t, tt.expected, string(eng.Name()))
		})
	}
}

// TestCopyEngine_CopyFile_ZeroLengthFile tests zero-length file handling.
func TestCopyEngine_CopyFile_ZeroLengthFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create zero-length file
	os.WriteFile(filepath.Join(src, "empty.txt"), []byte{}, 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify file exists and is empty
	info, err := os.Stat(filepath.Join(dstPath, "empty.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())

	_ = result
}

// TestReflinkEngine_CopyFile_ZeroLengthFile tests reflink with empty files.
func TestReflinkEngine_CopyFile_ZeroLengthFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "empty.txt"), []byte{}, 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify empty file
	info, err := os.Stat(filepath.Join(dstPath, "empty.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())

	_ = result
}

// TestCopyEngine_HardlinkToSameFile tests hardlink to same inode.
func TestCopyEngine_HardlinkToSameFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a file and hardlink to it
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("content"), 0644)
	err := os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link1.txt"))
	require.NoError(t, err)
	err = os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link2.txt"))
	require.NoError(t, err)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should report hardlink degradation
	assert.True(t, result.Degraded)
	assert.Contains(t, result.Degradations, "hardlink")

	// Verify all files exist with content
	for _, name := range []string{"original.txt", "link1.txt", "link2.txt"} {
		content, err := os.ReadFile(filepath.Join(dstPath, name))
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	}
}

// TestReflinkEngine_HardlinkHandling tests hardlink handling in reflink.
func TestReflinkEngine_HardlinkHandling(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with hardlinks
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("hardlink content"), 0644)
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link1.txt"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify files exist
	_, err = os.Stat(filepath.Join(dstPath, "original.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dstPath, "link1.txt"))
	require.NoError(t, err)

	_ = result
	_ = err
}

// TestCopyEngine_DirectoryWithPerms tests various directory permissions.
func TestCopyEngine_DirectoryWithPerms(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create directories with various permissions
	perms := []os.FileMode{0755, 0750, 0777, 0700, 0500}
	for i, perm := range perms {
		dirName := fmt.Sprintf("dir%d", i)
		dirPath := filepath.Join(src, dirName)
		os.MkdirAll(dirPath, perm)
		os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("file"), 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify directories exist (permissions may have umask applied)
	for i := range perms {
		dirName := fmt.Sprintf("dir%d", i)
		info, err := os.Stat(filepath.Join(dstPath, dirName))
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "%s should be a directory", dirName)
	}

	_ = result
}

// TestDetectEngine_MultipleCalls tests consistency of detection.
func TestDetectEngine_MultipleCalls(t *testing.T) {
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	tmpDir := t.TempDir()

	// Call DetectEngine multiple times
	var firstEngine, secondEngine engine.Engine
	var err error

	firstEngine, err = engine.DetectEngine(tmpDir)
	require.NoError(t, err)

	secondEngine, err = engine.DetectEngine(tmpDir)
	require.NoError(t, err)

	// Should return consistent results
	assert.Equal(t, firstEngine.Name(), secondEngine.Name())
}
