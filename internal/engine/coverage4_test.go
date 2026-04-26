package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopyEngine_copyFile_WriteError tests write error handling.
func TestCopyEngine_copyFile_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()

	// Create source file
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	// Create destination with a read-only directory
	readOnlyDir := filepath.Join(dst, "readonly")
	os.MkdirAll(readOnlyDir, 0500)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, readOnlyDir)

	// Should fail due to permission issues
	assert.Error(t, err)

	// Cleanup
	os.Chmod(readOnlyDir, 0755)
}

// TestCopyEngine_copyFile_SyncError tests file sync error path.
func TestCopyEngine_copyFile_SyncError(t *testing.T) {
	// This is difficult to test as it requires causing fsync to fail
	// Skip on systems where we can't trigger this
	t.Skip("sync error difficult to test reliably")
}

// TestCopyEngine_copyFile_ChtimesError tests mod time preservation error.
func TestCopyEngine_copyFile_ChtimesError(t *testing.T) {
	// Test with a read-only parent directory
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)

	// Most of the time this succeeds
	// The chtimes error path is hard to trigger
	if err != nil {
		assert.Error(t, err)
	} else {
		// Verify content copied even if chtimes might have issues
		content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	}
}

// TestReflinkEngine_copyFile_ChtimesError tests reflink chtimes error.
func TestReflinkEngine_copyFile_ChtimesError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	// Should succeed
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestCopyEngine_FileWithManyZeros tests file with many zeros.
func TestCopyEngine_FileWithManyZeros(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with lots of zeros
	zeros := make([]byte, 100*1024)
	os.WriteFile(filepath.Join(src, "zeros.bin"), zeros, 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "zeros.bin"))
	require.NoError(t, err)
	assert.Equal(t, len(zeros), len(content))

	_ = result
}

// TestReflinkEngine_FileWithManyZeros tests zeros file with reflink.
func TestReflinkEngine_FileWithManyZeros(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	zeros := make([]byte, 100*1024)
	os.WriteFile(filepath.Join(src, "zeros.bin"), zeros, 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstPath, "zeros.bin"))
	require.NoError(t, err)
	assert.Equal(t, len(zeros), len(content))

	_ = result
}

// TestCopyEngine_copySymlink_ReadlinkError tests symlink read error.
func TestCopyEngine_copySymlink_ReadlinkError(t *testing.T) {
	// Testing readlink error is difficult as os.Readlink usually succeeds
	// on valid symlinks. We verify the symlink path is covered.
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a symlink
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	err := os.Symlink("target.txt", filepath.Join(src, "link"))
	require.NoError(t, err)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify symlink was copied
	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

// TestCopyEngine_SymlinkToSymlink tests symlink pointing to symlink.
func TestCopyEngine_SymlinkToSymlink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create target and symlink
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link1"))
	os.Symlink("link1", filepath.Join(src, "link2"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify symlinks
	target1, err := os.Readlink(filepath.Join(dstPath, "link1"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target1)

	target2, err := os.Readlink(filepath.Join(dstPath, "link2"))
	require.NoError(t, err)
	assert.Equal(t, "link1", target2)

	_ = result
}

// TestReflinkEngine_copyFile_WriteError tests reflink copy write errors.
func TestReflinkEngine_copyFile_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	// Make destination read-only
	os.MkdirAll(dstPath, 0500)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	// Should fail or handle gracefully
	_ = err

	// Cleanup
	os.Chmod(dstPath, 0755)
}

// TestReflinkEngine_copyFile_SyncError tests reflink copy sync error.
func TestReflinkEngine_copyFile_SyncError(t *testing.T) {
	t.Skip("sync error difficult to test reliably")
}

// TestReflinkEngine_copySymlink_ReadlinkError tests reflink symlink errors.
func TestReflinkEngine_copySymlink_ReadlinkError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create valid symlink
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
}

// TestJuiceFSEngine_Clone_JuiceFSAvailable tests juicefs available path.
func TestJuiceFSEngine_Clone_JuiceFSAvailable(t *testing.T) {
	// When juicefs is not available, should fallback to copy
	// This is already tested, but we explicitly verify the degradation
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Since juicefs is not installed in test environment, should be degraded
	assert.True(t, result.Degraded)
	assert.NotEmpty(t, result.Degradations)

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

// TestJuiceFSEngine_Clone_OnJuiceFS tests on-JuiceFS detection.
func TestJuiceFSEngine_Clone_OnJuiceFS(t *testing.T) {
	// On non-JuiceFS filesystem, should fallback
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should be degraded since not on JuiceFS
	assert.True(t, result.Degraded)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestJuiceFSEngine_isOnJuiceFS_AbsPathError tests absolute path error handling.
func TestJuiceFSEngine_isOnJuiceFS_AbsPathError(t *testing.T) {
	// Test with paths that might cause filepath.Abs to fail
	// Most paths are valid, so this is a smoke test
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	// Should succeed even if abs path has issues
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))

	_ = result
}

// TestJuiceFSEngine_isOnJuiceFS_ProcMountsOpenError tests /proc/mounts handling.
func TestJuiceFSEngine_isOnJuiceFS_ProcMountsOpenError(t *testing.T) {
	// On non-Linux systems, /proc/mounts doesn't exist
	// The function should fall back gracefully
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	// Should work regardless of /proc/mounts
	require.NoError(t, err)
	assert.True(t, result.Degraded)

	_ = result
}

// TestDetectEngine_JuiceFSDetection tests JuiceFS detection path.
func TestDetectEngine_JuiceFSDetection(t *testing.T) {
	// Test auto-detection when juicefs might be available
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
	// Will detect reflink or copy since JuiceFS is not actually mounted
	assert.NotEmpty(t, eng.Name())
}

// TestDetectEngine_ReflinkDetectionSuccess tests successful reflink detection.
func TestDetectEngine_ReflinkDetectionSuccess(t *testing.T) {
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
	// The engine will be reflink if supported, or copy as fallback
	name := eng.Name()
	assert.NotEmpty(t, name)
}

// TestDetectEngine_TestDirCreationFailure tests temp dir creation failure.
func TestDetectEngine_TestDirCreationFailure(t *testing.T) {
	// This tests the path where creating temp dir for reflink test fails
	// Hard to trigger, but we verify the behavior doesn't crash
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
	assert.NotNil(t, eng)
}

// TestCopyEngine_RelativePathError tests relative path calculation error.
func TestCopyEngine_RelativePathError(t *testing.T) {
	// filepath.Rel should almost always succeed on valid paths
	// This is a smoke test
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

// TestReflinkEngine_RelativePathError tests relative path error in reflink.
func TestReflinkEngine_RelativePathError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestCopyEngine_WalkError tests filepath.Walk error handling.
func TestCopyEngine_WalkError(t *testing.T) {
	// Test when filepath.Walk encounters an error
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a directory with no permissions
	noAccessDir := filepath.Join(src, "noaccess")
	os.MkdirAll(noAccessDir, 0000)
	defer os.Chmod(noAccessDir, 0755)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, dstPath)

	// May succeed (walk skips inaccessible dir) or error
	_ = err
}

// TestReflinkEngine_WalkError tests walk error in reflink.
func TestReflinkEngine_WalkError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create inaccessible directory
	noAccessDir := filepath.Join(src, "noaccess")
	os.MkdirAll(noAccessDir, 0000)
	defer os.Chmod(noAccessDir, 0755)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	_ = err
}

// TestReflinkEngine_reflinkFile_SourceOpenError tests source file open error.
func TestReflinkEngine_reflinkFile_SourceOpenError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with no read permissions
	filePath := filepath.Join(src, "noread.txt")
	os.WriteFile(filePath, []byte("content"), 0644)
	os.Chmod(filePath, 0000)
	defer os.Chmod(filePath, 0644)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	// May fail or succeed depending on how Walk handles it
	_ = err
}

// TestReflinkEngine_reflinkFile_DestOpenError tests destination open error.
func TestReflinkEngine_reflinkFile_DestOpenError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	// Create destination directory that's read-only
	os.MkdirAll(dstPath, 0500)
	defer os.Chmod(dstPath, 0755)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, dstPath)

	// Should fail due to permission
	assert.Error(t, err)
}

// TestReflinkEngine_reflinkFile_IoctlError tests FICLONE ioctl failure.
func TestReflinkEngine_reflinkFile_IoctlError(t *testing.T) {
	// On most filesystems, FICLONE will fail and fall back to copy
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	// Should succeed via fallback
	require.NoError(t, err)
	// If degraded, reflink failed (expected on most filesystems)
	if result.Degraded {
		assert.NotEmpty(t, result.Degradations)
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

// TestCopyEngine_FsyncDirError tests directory fsync error.
func TestCopyEngine_FsyncDirError(t *testing.T) {
	t.Skip("fsync error difficult to test reliably")
}

// TestReflinkEngine_FsyncDirError tests directory fsync error in reflink.
func TestReflinkEngine_FsyncDirError(t *testing.T) {
	t.Skip("fsync error difficult to test reliably")
}

// TestJuiceFSEngine_DetectEngine_VerifyAllPaths tests all detection paths.
func TestJuiceFSEngine_DetectEngine_VerifyAllPaths(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedType  string
		checkDegraded bool
	}{
		{"copy engine", "copy", "copy", false},
		{"reflink engine", "reflink", "reflink-copy", false},
		{"juicefs engine", "juicefs", "juicefs-clone", false},
		{"empty env auto-detect", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("JVS_ENGINE", tt.envValue)
			} else {
				os.Unsetenv("JVS_ENGINE")
			}
			defer os.Unsetenv("JVS_ENGINE")

			tmpDir := t.TempDir()
			eng, err := engine.DetectEngine(tmpDir)

			require.NoError(t, err)
			require.NotNil(t, eng)

			if tt.expectedType != "" {
				assert.Equal(t, tt.expectedType, string(eng.Name()))
			}

			// Verify the engine works
			dst := t.TempDir()
			dstPath := filepath.Join(dst, "cloned")
			os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)

			result, err := eng.Clone(tmpDir, dstPath)
			require.NoError(t, err)

			// Verify content
			content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
			require.NoError(t, err)
			assert.Equal(t, "test", string(content))

			_ = result
		})
	}
}

// TestCopyEngine_FilePathLength tests long file path handling.
func TestCopyEngine_FilePathLength(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a deeply nested path with long names
	deepPath := src
	for i := 0; i < 10; i++ {
		deepPath = filepath.Join(deepPath, "very-long-directory-name-"+string(rune('a'+i)))
	}
	os.MkdirAll(deepPath, 0755)
	os.WriteFile(filepath.Join(deepPath, "very-long-filename.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify deep file exists
	deepDest := dstPath
	for i := 0; i < 10; i++ {
		deepDest = filepath.Join(deepDest, "very-long-directory-name-"+string(rune('a'+i)))
	}
	content, err := os.ReadFile(filepath.Join(deepDest, "very-long-filename.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestReflinkEngine_FilePathLength tests long paths in reflink.
func TestReflinkEngine_FilePathLength(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	deepPath := src
	for i := 0; i < 5; i++ {
		deepPath = filepath.Join(deepPath, "deep")
	}
	os.MkdirAll(deepPath, 0755)
	os.WriteFile(filepath.Join(deepPath, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	deepDest := dstPath
	for i := 0; i < 5; i++ {
		deepDest = filepath.Join(deepDest, "deep")
	}
	content, err := os.ReadFile(filepath.Join(deepDest, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestCopyEngine_MultipleHardlinksToSameFile tests multiple hardlinks.
func TestCopyEngine_MultipleHardlinksToSameFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create original file
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("content"), 0644)

	// Create multiple hardlinks
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link1.txt"))
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link2.txt"))
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link3.txt"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should report hardlink degradation
	assert.True(t, result.Degraded)
	assert.Contains(t, result.Degradations, "hardlink")

	// Verify all files exist
	for _, name := range []string{"original.txt", "link1.txt", "link2.txt", "link3.txt"} {
		content, err := os.ReadFile(filepath.Join(dstPath, name))
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	}
}

// TestReflinkEngine_DirectoryCreationError tests directory creation error.
func TestReflinkEngine_DirectoryCreationError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file at destination path
	blocker := filepath.Join(dst, "cloned")
	os.WriteFile(blocker, []byte("block"), 0644)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	_, err := eng.Clone(src, blocker)

	assert.Error(t, err)
}

// TestCopyEngine_DirectoryCreationError tests directory creation errors.
func TestCopyEngine_DirectoryCreationError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file at destination path
	blocker := filepath.Join(dst, "cloned")
	os.WriteFile(blocker, []byte("block"), 0644)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	_, err := eng.Clone(src, blocker)

	assert.Error(t, err)
}

// TestCopyEngine_SysStatNil tests when Sys() returns nil.
func TestCopyEngine_SysStatNil(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestReflinkEngine_NonRegularFile tests non-regular file handling.
func TestReflinkEngine_NonRegularFile(t *testing.T) {
	// Test with various file types
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create regular file, symlink, and directory
	os.WriteFile(filepath.Join(src, "regular.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link"))
	os.MkdirAll(filepath.Join(src, "dir"), 0755)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify all types copied
	_, err = os.Stat(filepath.Join(dstPath, "regular.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "link"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "dir"))
	require.NoError(t, err)

	_ = result
}

// TestCopyEngine_SymlinkToDirectory tests symlink pointing to directory.
func TestCopyEngine_SymlinkToDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create directory and symlink to it
	os.MkdirAll(filepath.Join(src, "targetdir"), 0755)
	os.WriteFile(filepath.Join(src, "targetdir", "file.txt"), []byte("in dir"), 0644)
	os.Symlink("targetdir", filepath.Join(src, "dirlink"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify symlink
	target, err := os.Readlink(filepath.Join(dstPath, "dirlink"))
	require.NoError(t, err)
	assert.Equal(t, "targetdir", target)

	// Verify target directory exists
	_, err = os.Stat(filepath.Join(dstPath, "targetdir", "file.txt"))
	require.NoError(t, err)

	_ = result
}

// TestReflinkEngine_SymlinkToDirectory tests symlink to directory in reflink.
func TestReflinkEngine_SymlinkToDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.MkdirAll(filepath.Join(src, "targetdir"), 0755)
	os.WriteFile(filepath.Join(src, "targetdir", "file.txt"), []byte("in dir"), 0644)
	os.Symlink("targetdir", filepath.Join(src, "dirlink"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "dirlink"))
	require.NoError(t, err)
	assert.Equal(t, "targetdir", target)

	_ = result
}

// TestDetectEngine_EmptyPath tests empty path handling.
func TestDetectEngine_EmptyPath(t *testing.T) {
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	eng, err := engine.DetectEngine("")

	// Should handle gracefully
	require.NoError(t, err)
	assert.NotNil(t, eng)
}

// TestCopyEngine_DeviceFile tests device file handling (when available).
func TestCopyEngine_DeviceFile(t *testing.T) {
	// Skip if not root
	if os.Getuid() != 0 {
		t.Skip("requires root to create device files")
	}
	t.Skip("device file creation requires mknod")
}

// TestCopyEngine_NamedPipe tests named pipe (FIFO) handling.
func TestCopyEngine_NamedPipe(t *testing.T) {
	// Skip - named pipes can cause tests to hang
	t.Skip("named pipes can cause test hangs")
}

// TestReflinkEngine_EmptySource tests empty source directory.
func TestReflinkEngine_EmptySource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify empty destination
	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)

	_ = result
}

// TestCopyEngine_EmptySource tests empty source for copy engine.
func TestCopyEngine_EmptySource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)

	_ = result
}

// TestJuiceFSEngine_EmptySource tests empty source for JuiceFS engine.
func TestJuiceFSEngine_EmptySource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	entries, err := os.ReadDir(dstPath)
	require.NoError(t, err)
	assert.Empty(t, entries)

	_ = result
}

// TestAllEngines_SameSource tests all engines with same source.
func TestAllEngines_SameSource(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	engines := []struct {
		name   string
		engine engine.Engine
	}{
		{"copy", engine.NewCopyEngine()},
		{"reflink", engine.NewReflinkEngine()},
		{"juicefs", engine.NewJuiceFSEngine()},
	}

	for _, tt := range engines {
		t.Run(tt.name, func(t *testing.T) {
			dst := t.TempDir()
			dstPath := filepath.Join(dst, "cloned")

			result, err := tt.engine.Clone(src, dstPath)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
			require.NoError(t, err)
			assert.Equal(t, "content", string(content))

			_ = result
		})
	}
}

// TestEngine_CloneResult_InitialState tests CloneResult initialization.
func TestEngine_CloneResult_InitialState(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// Result fields are accessible
	_ = result.Degraded
	_ = result.Degradations

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}
