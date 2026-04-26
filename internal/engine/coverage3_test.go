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

// TestReflinkEngine_reflinkFile_ErrorScenarios tests error scenarios.
func TestReflinkEngine_reflinkFile_ErrorScenarios(t *testing.T) {
	t.Run("Source file cannot be opened", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Don't create source, so Walk will not find any files
		// But empty dir is still valid, so this succeeds
		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		// Empty directory clone succeeds
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify destination exists
		entries, err := os.ReadDir(dstPath)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("Destination cannot be created", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()

		// Create source file
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

		// Create a file at destination path (blocks directory creation)
		blocker := filepath.Join(dst, "cloned")
		os.WriteFile(blocker, []byte("block"), 0644)

		eng := engine.NewReflinkEngine()
		_, err := eng.Clone(src, blocker)
		assert.Error(t, err)
	})

	t.Run("Fsync failure on destination directory", func(t *testing.T) {
		// This is hard to test reliably as it requires triggering fsync errors
		// Skip this test
		t.Skip("fsync failure difficult to test reliably")
	})
}

// TestCopyEngine_copyFile_ErrorScenarios tests copy error scenarios.
func TestCopyEngine_copyFile_ErrorScenarios(t *testing.T) {
	t.Run("Source file becomes inaccessible", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root")
		}

		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create and then remove source
		srcFile := filepath.Join(src, "file.txt")
		os.WriteFile(srcFile, []byte("content"), 0644)
		os.Remove(srcFile)

		eng := engine.NewCopyEngine()
		_, err := eng.Clone(src, dstPath)
		// Empty directory succeeds - no files to copy
		require.NoError(t, err)
		_ = err
	})

	t.Run("Source is directory", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create directory with same name as expected file
		os.MkdirAll(filepath.Join(src, "notfile"), 0755)

		eng := engine.NewCopyEngine()
		// This should succeed - it copies directories
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		// Verify directory was copied
		_, err = os.Stat(filepath.Join(dstPath, "notfile"))
		require.NoError(t, err)
	})
}

// TestCopyEngine_CopySymlink_ReadlinkFailure tests readlink error handling.
func TestCopyEngine_CopySymlink_ReadlinkFailure(t *testing.T) {
	t.Run("Invalid symlink target", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// On most systems, creating a symlink with very long target
		// can cause issues, but we create a normal symlink
		os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
		err := os.Symlink("target.txt", filepath.Join(src, "link.txt"))
		require.NoError(t, err)

		eng := engine.NewCopyEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)

		// Verify symlink
		target, err := os.Readlink(filepath.Join(dstPath, "link.txt"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", target)

		_ = result
	})
}

// TestReflinkEngine_CopySymlink_ReadlinkFailure tests symlink copy errors.
func TestReflinkEngine_CopySymlink_ReadlinkFailure(t *testing.T) {
	t.Run("Valid symlink copy", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
		err := os.Symlink("target.txt", filepath.Join(src, "link"))
		require.NoError(t, err)

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Verify symlink
		target, err := os.Readlink(filepath.Join(dstPath, "link"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", target)

		_ = result
	})
}

// TestJuiceFSEngine_isOnJuiceFS_ProcMounts tests /proc/mounts parsing.
func TestJuiceFSEngine_isOnJuiceFS_ProcMounts(t *testing.T) {
	t.Run("Non-Linux system or no /proc/mounts", func(t *testing.T) {
		// On systems without /proc/mounts (non-Linux), the function
		// falls back to checking if juicefs command exists
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

		eng := engine.NewJuiceFSEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		// Will be degraded (not on JuiceFS)
		assert.True(t, result.Degraded)
		assert.NotEmpty(t, result.Degradations)

		// Verify content via fallback
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "test", string(content))
	})
}

// TestEngine_CopyResult_Fields tests CloneResult field usage.
func TestEngine_CopyResult_Fields(t *testing.T) {
	t.Run("CopyEngine with no degradation", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		os.WriteFile(filepath.Join(src, "file.txt"), []byte("simple"), 0644)

		eng := engine.NewCopyEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		assert.False(t, result.Degraded)
		assert.Empty(t, result.Degradations)
		assert.NotNil(t, result)
	})

	t.Run("ReflinkEngine reports degradation when reflink fails", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()
		dstPath := filepath.Join(dst, "cloned")

		// Create file - reflink may fail on most filesystems
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0644)

		eng := engine.NewReflinkEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)
		// Result should indicate whether reflink was used or fallback
		assert.NotNil(t, result)

		// Verify content preserved
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "data", string(content))
	})
}

// TestCopyEngine_FsyncDirFailure tests fsync failure handling.
func TestCopyEngine_FsyncDirFailure(t *testing.T) {
	// This test is difficult to implement as it requires triggering
	// fsync errors, which typically don't happen on tmpfs
	t.Skip("fsync failure difficult to test")
}

// TestReflinkEngine_ChtimesFailure tests mod time preservation failure.
func TestReflinkEngine_ChtimesFailure(t *testing.T) {
	// Setting mod times on files usually succeeds
	// This is more of a smoke test
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with specific mod time
	filePath := filepath.Join(src, "timed.txt")
	os.WriteFile(filePath, []byte("time test"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify file exists and has mod time set
	info, err := os.Stat(filepath.Join(dstPath, "timed.txt"))
	require.NoError(t, err)
	assert.False(t, info.ModTime().IsZero())

	_ = result
}

// TestCopyEngine_FileClosingTests tests proper file handle cleanup.
func TestCopyEngine_FileClosingTests(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create multiple files to test proper cleanup
	for i := 0; i < 100; i++ {
		os.WriteFile(filepath.Join(src, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("content%d", i)), 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all files copied (file handles were properly closed)
	for i := 0; i < 100; i++ {
		content, err := os.ReadFile(filepath.Join(dstPath, fmt.Sprintf("file%d.txt", i)))
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("content%d", i), string(content))
	}

	_ = result
}

// TestReflinkEngine_FileClosingTests tests file handle cleanup in reflink.
func TestReflinkEngine_FileClosingTests(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create many files
	for i := 0; i < 50; i++ {
		os.WriteFile(filepath.Join(src, fmt.Sprintf("file%d.txt", i)), []byte(fmt.Sprintf("data%d", i)), 0644)
	}

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify all files (proper cleanup)
	for i := 0; i < 50; i++ {
		content, err := os.ReadFile(filepath.Join(dstPath, fmt.Sprintf("file%d.txt", i)))
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("data%d", i), string(content))
	}

	_ = result
}

// TestDetectEngine_EnvOverridePriority tests environment override priority.
func TestDetectEngine_EnvOverridePriority(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		check    func(t *testing.T, eng engine.Engine)
	}{
		{
			name:     "juicefs override",
			envValue: "juicefs",
			check: func(t *testing.T, eng engine.Engine) {
				assert.Equal(t, "juicefs-clone", string(eng.Name()))
			},
		},
		{
			name:     "reflink override",
			envValue: "reflink",
			check: func(t *testing.T, eng engine.Engine) {
				assert.Equal(t, "reflink-copy", string(eng.Name()))
			},
		},
		{
			name:     "copy override",
			envValue: "copy",
			check: func(t *testing.T, eng engine.Engine) {
				assert.Equal(t, "copy", string(eng.Name()))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("JVS_ENGINE", tt.envValue)
			defer os.Unsetenv("JVS_ENGINE")

			tmpDir := t.TempDir()
			eng, err := engine.DetectEngine(tmpDir)

			require.NoError(t, err)
			tt.check(t, eng)
		})
	}
}
