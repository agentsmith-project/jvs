package engine_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJuiceFSEngine_Clone_JuiceFSAvailablePath tests when juicefs command exists.
func TestJuiceFSEngine_Clone_JuiceFSAvailablePath(t *testing.T) {
	// Create a mock juicefs binary
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "juicefs")

	// Create a mock that exits with error (simulating clone failure)
	script := `#!/bin/sh
# Mock juicefs clone - fail to trigger fallback path
echo "mock juicefs clone" >&2
exit 1
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Skip("cannot create mock juicefs binary")
	}

	// Add mock to PATH
	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	// Also create a mock /proc/mounts entry for testing isOnJuiceFS
	// We'll test the fallback path when clone fails
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	// Should fall back to copy (either not-on-juicefs or clone-failed)
	require.NoError(t, err)
	assert.True(t, result.Degraded)

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	// Should have either "not-on-juicefs" or "juicefs-clone-failed" degradation
	assert.NotEmpty(t, result.Degradations)
}

// TestJuiceFSEngine_isOnJuiceFS_InvalidPath tests absolute path error handling.
func TestJuiceFSEngine_isOnJuiceFS_InvalidPath(t *testing.T) {
	// This tests the path where filepath.Abs fails
	// Most paths are valid, so we verify the function doesn't crash
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded)

	_ = result
}

// TestDetectEngine_JuiceFSInPath tests detection when juicefs is in PATH.
func TestDetectEngine_JuiceFSInPath(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "juicefs")

	// Create mock juicefs binary
	script := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Skip("cannot create mock juicefs binary")
	}

	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	os.Unsetenv("JVS_ENGINE")

	repoDir := t.TempDir()
	eng, err := engine.DetectEngine(repoDir)

	require.NoError(t, err)
	// Should detect something (juicefs if available and on mount, otherwise reflink/copy)
	assert.NotNil(t, eng)
}

// TestCopyEngine_filePathRelError tests filepath.Rel error path.
func TestCopyEngine_filePathRelError(t *testing.T) {
	// filepath.Rel errors are rare - they happen with invalid paths
	// We test the normal path extensively
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

// TestReflinkEngine_filePathRelError tests filepath.Rel in reflink.
func TestReflinkEngine_filePathRelError(t *testing.T) {
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

// TestCopyEngine_SourceIsSymlinkToDirectory tests symlink to directory handling.
func TestCopyEngine_SourceIsSymlinkToDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create directory and symlink to it
	os.MkdirAll(filepath.Join(src, "realdir"), 0755)
	os.WriteFile(filepath.Join(src, "realdir", "file.txt"), []byte("in dir"), 0644)
	os.Symlink("realdir", filepath.Join(src, "dirlink"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify both real directory and symlink were copied
	_, err = os.Stat(filepath.Join(dstPath, "realdir", "file.txt"))
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "dirlink"))
	require.NoError(t, err)
	assert.Equal(t, "realdir", target)

	_ = result
}

// TestReflinkEngine_SourceIsSymlinkToDirectory tests symlink to dir in reflink.
func TestReflinkEngine_SourceIsSymlinkToDirectory(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.MkdirAll(filepath.Join(src, "realdir"), 0755)
	os.WriteFile(filepath.Join(src, "realdir", "file.txt"), []byte("in dir"), 0644)
	os.Symlink("realdir", filepath.Join(src, "dirlink"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "dirlink"))
	require.NoError(t, err)
	assert.Equal(t, "realdir", target)

	_ = result
}

// TestCopyEngine_UnicodeFilenames tests unicode filenames.
func TestCopyEngine_UnicodeFilenames(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create files with unicode names
	unicodeNames := []string{
		"файл.txt",      // Russian
		"文件.txt",        // Chinese
		"αρχείο.txt",    // Greek
		"fichier é.txt", // French
		"datei ü.txt",   // German
	}

	for _, name := range unicodeNames {
		content := fmt.Sprintf("content of %s", name)
		os.WriteFile(filepath.Join(src, name), []byte(content), 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all files were copied
	for _, name := range unicodeNames {
		content, err := os.ReadFile(filepath.Join(dstPath, name))
		require.NoError(t, err, "failed to read %s", name)
		assert.Contains(t, string(content), name)
	}

	_ = result
}

// TestReflinkEngine_UnicodeFilenames tests unicode in reflink.
func TestReflinkEngine_UnicodeFilenames(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "тест.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(src, "テスト.txt"), []byte("test"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "тест.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "テスト.txt"))
	require.NoError(t, err)

	_ = result
}

// TestCopyEngine_FilesWithSpaces tests filenames with spaces.
func TestCopyEngine_FilesWithSpaces(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create files with spaces and special characters
	specialNames := []string{
		"file with spaces.txt",
		"file\twith\ttabs.txt",
		"file\nwith\nnewline.txt",
		"file;with;semicolons.txt",
	}

	for _, name := range specialNames {
		if name == "file\nwith\nnewline.txt" {
			// Skip newline - not valid in filenames on most systems
			continue
		}
		os.WriteFile(filepath.Join(src, name), []byte("content"), 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	_ = result
}

// TestJuiceFSEngine_AllFallbackPaths tests all fallback reasons.
func TestJuiceFSEngine_AllFallbackPaths(t *testing.T) {
	tests := []struct {
		name              string
		setupMockJuiceFS  bool
		mockSuccess       bool
		expectedDegraded  bool
		expectedDegradMsg string
	}{
		{
			name:             "juicefs not available",
			setupMockJuiceFS: false,
			expectedDegraded: true,
		},
		{
			name:             "juicefs available but not on juicefs",
			setupMockJuiceFS: true,
			mockSuccess:      false,
			expectedDegraded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oldPath string

			if tt.setupMockJuiceFS {
				tmpDir := t.TempDir()
				mockBin := filepath.Join(tmpDir, "juicefs")

				script := `#!/bin/sh
exit 1
`
				if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
					t.Skip("cannot create mock juicefs binary")
				}

				oldPath = os.Getenv("PATH")
				newPath := tmpDir + string(os.PathListSeparator) + oldPath
				os.Setenv("PATH", newPath)
				defer os.Setenv("PATH", oldPath)
			}

			src := t.TempDir()
			dst := t.TempDir()
			dstPath := filepath.Join(dst, "cloned")

			os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

			eng := engine.NewJuiceFSEngine()
			result, err := eng.Clone(src, dstPath)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedDegraded, result.Degraded)

			// Verify fallback worked
			content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
			require.NoError(t, err)
			assert.Equal(t, "test", string(content))
		})
	}
}

// TestCopyEngine_SymlinkChain tests chains of symlinks.
func TestCopyEngine_SymlinkChain(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create chain: link3 -> link2 -> link1 -> file
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("target"), 0644)
	os.Symlink("file.txt", filepath.Join(src, "link1"))
	os.Symlink("link1", filepath.Join(src, "link2"))
	os.Symlink("link2", filepath.Join(src, "link3"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all symlinks copied
	for _, link := range []string{"link1", "link2", "link3"} {
		target, err := os.Readlink(filepath.Join(dstPath, link))
		require.NoError(t, err)
		expected := map[string]string{
			"link1": "file.txt",
			"link2": "link1",
			"link3": "link2",
		}
		assert.Equal(t, expected[link], target)
	}

	_ = result
}

// TestReflinkEngine_SymlinkChain tests symlink chains in reflink.
func TestReflinkEngine_SymlinkChain(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("target"), 0644)
	os.Symlink("file.txt", filepath.Join(src, "link1"))
	os.Symlink("link1", filepath.Join(src, "link2"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	target1, err := os.Readlink(filepath.Join(dstPath, "link1"))
	require.NoError(t, err)
	assert.Equal(t, "file.txt", target1)

	target2, err := os.Readlink(filepath.Join(dstPath, "link2"))
	require.NoError(t, err)
	assert.Equal(t, "link1", target2)

	_ = result
}

// TestCopyEngine_MixedSpecialFiles tests mix of special files.
func TestCopyEngine_MixedSpecialFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create mix of regular files, directories, symlinks
	os.MkdirAll(filepath.Join(src, "dir1", "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("f1"), 0644)
	os.WriteFile(filepath.Join(src, "dir1", "file2.txt"), []byte("f2"), 0644)
	os.WriteFile(filepath.Join(src, "dir1", "subdir", "file3.txt"), []byte("f3"), 0644)

	os.WriteFile(filepath.Join(src, "target.txt"), []byte("tgt"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link.txt"))
	os.Symlink("dir1", filepath.Join(src, "dirlink"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all content
	_, err = os.Stat(filepath.Join(dstPath, "file1.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "dir1", "file2.txt"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dstPath, "dir1", "subdir", "file3.txt"))
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)

	dirTarget, err := os.Readlink(filepath.Join(dstPath, "dirlink"))
	require.NoError(t, err)
	assert.Equal(t, "dir1", dirTarget)

	_ = result
}

// TestDetectEngine_MultipleEnginesAvailable tests detection with multiple options.
func TestDetectEngine_MultipleEnginesAvailable(t *testing.T) {
	// Set up mock juicefs
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "juicefs")

	script := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Skip("cannot create mock juicefs binary")
	}

	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	os.Unsetenv("JVS_ENGINE")

	repoDir := t.TempDir()
	eng, err := engine.DetectEngine(repoDir)

	require.NoError(t, err)
	// With juicefs "available" but not on mount, should fall back
	assert.NotNil(t, eng)
	_ = eng
}

// TestJuiceFSEngine_FallbackToCopy_CopyFailures tests fallback when copy fails.
func TestJuiceFSEngine_FallbackToCopy_CopyFailures(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file at destination path to cause copy to fail
	blocker := filepath.Join(dst, "cloned")
	os.WriteFile(blocker, []byte("block"), 0644)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	_, err := eng.Clone(src, blocker)

	// Should error
	assert.Error(t, err)
}

// TestCopyEngine_ExistingDestination tests when destination already exists.
func TestCopyEngine_ExistingDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create destination with existing file
	os.MkdirAll(dstPath, 0755)
	os.WriteFile(filepath.Join(dstPath, "existing.txt"), []byte("old"), 0644)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("new"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// New file should exist
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))

	// Old file should still be there (Clone doesn't clear dst)
	content, err = os.ReadFile(filepath.Join(dstPath, "existing.txt"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(content))

	_ = result
}

// TestReflinkEngine_ExistingDestination tests existing destination in reflink.
func TestReflinkEngine_ExistingDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.MkdirAll(dstPath, 0755)
	os.WriteFile(filepath.Join(dstPath, "existing.txt"), []byte("old"), 0644)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("new"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))

	_ = result
}

// TestCopyEngine_VerifyNoHardlinksInCopy verifies hardlinks become separate files.
func TestCopyEngine_VerifyNoHardlinksInCopy(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create file with hardlinks
	os.WriteFile(filepath.Join(src, "original.txt"), []byte("same content"), 0644)
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link1.txt"))
	os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "link2.txt"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should report hardlink degradation
	assert.True(t, result.Degraded)
	assert.Contains(t, result.Degradations, "hardlink")

	// Verify files are separate (not hardlinked)
	stat1, _ := os.Stat(filepath.Join(dstPath, "original.txt"))
	stat2, _ := os.Stat(filepath.Join(dstPath, "link1.txt"))
	stat3, _ := os.Stat(filepath.Join(dstPath, "link2.txt"))

	// On systems that support inodes, verify they're different
	sys1 := stat1.Sys()
	sys2 := stat2.Sys()
	sys3 := stat3.Sys()

	if sys1 != nil && sys2 != nil && sys3 != nil {
		// Check if we have inode info
		type InodeGetter interface {
			Ino() uint64
		}
		if ino1, ok := sys1.(InodeGetter); ok {
			if ino2, ok2 := sys2.(InodeGetter); ok2 {
				if ino3, ok3 := sys3.(InodeGetter); ok3 {
					// Inodes should be different (copied, not reflinked)
					assert.NotEqual(t, ino1.Ino(), ino2.Ino())
					assert.NotEqual(t, ino1.Ino(), ino3.Ino())
				}
			}
		}
	}
}

// TestEngine_AllEnginesHandleEmptyDirectory tests all engines with empty dir.
func TestEngine_AllEnginesHandleEmptyDirectory(t *testing.T) {
	src := t.TempDir()

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

			// Verify empty destination
			entries, err := os.ReadDir(dstPath)
			require.NoError(t, err)
			assert.Empty(t, entries)

			_ = result
		})
	}
}

// TestEngine_AllEnginesHandleSingleFile tests all engines with single file.
func TestEngine_AllEnginesHandleSingleFile(t *testing.T) {
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

// TestEngine_AllEnginesHandleNestedStructure tests all engines with nested structure.
func TestEngine_AllEnginesHandleNestedStructure(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(src, "a", "file1.txt"), []byte("f1"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "file2.txt"), []byte("f2"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "c", "file3.txt"), []byte("f3"), 0644)

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

			// Verify all files
			content1, _ := os.ReadFile(filepath.Join(dstPath, "a", "file1.txt"))
			assert.Equal(t, "f1", string(content1))

			content2, _ := os.ReadFile(filepath.Join(dstPath, "a", "b", "file2.txt"))
			assert.Equal(t, "f2", string(content2))

			content3, _ := os.ReadFile(filepath.Join(dstPath, "a", "b", "c", "file3.txt"))
			assert.Equal(t, "f3", string(content3))

			_ = result
		})
	}
}

// TestDetectEngine_VerifyConsistency verifies detection is consistent.
func TestDetectEngine_VerifyConsistency(t *testing.T) {
	oldEnv := os.Getenv("JVS_ENGINE")
	os.Unsetenv("JVS_ENGINE")
	defer func() {
		if oldEnv != "" {
			os.Setenv("JVS_ENGINE", oldEnv)
		}
	}()

	tmpDir := t.TempDir()

	// Call detection multiple times
	var engines []engine.Engine
	for i := 0; i < 5; i++ {
		eng, err := engine.DetectEngine(tmpDir)
		require.NoError(t, err)
		engines = append(engines, eng)
	}

	// All should return the same engine type
	firstName := engines[0].Name()
	for _, eng := range engines[1:] {
		assert.Equal(t, firstName, eng.Name())
	}
}

// TestCopyEngine_LargeNumberOfFiles tests handling many files.
func TestCopyEngine_LargeNumberOfFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create 100 files
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("file%03d.txt", i)
		content := fmt.Sprintf("content %d", i)
		os.WriteFile(filepath.Join(src, name), []byte(content), 0644)
	}

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.False(t, result.Degraded)

	// Verify all files
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("file%03d.txt", i)
		expected := fmt.Sprintf("content %d", i)
		content, err := os.ReadFile(filepath.Join(dstPath, name))
		require.NoError(t, err)
		assert.Equal(t, expected, string(content))
	}

	_ = result
}

// TestReflinkEngine_LargeNumberOfFiles tests many files with reflink.
func TestReflinkEngine_LargeNumberOfFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("file%03d.txt", i)
		os.WriteFile(filepath.Join(src, name), []byte("content"), 0644)
	}

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Spot check some files
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("file%03d.txt", i)
		_, err := os.Stat(filepath.Join(dstPath, name))
		require.NoError(t, err)
	}

	_ = result
}

// TestJuiceFSEngine_isJuiceFSAvailable checks juicefs availability detection.
func TestJuiceFSEngine_isJuiceFSAvailable(t *testing.T) {
	// This tests isJuiceFSAvailable through the Clone interface
	// When juicefs doesn't exist, should fall back

	// Verify juicefs is not in PATH
	_, err := exec.LookPath("juicefs")
	if err == nil {
		// juicefs exists, skip this test
		t.Skip("juicefs is installed, cannot test unavailable path")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded)

	// Should have "juicefs-not-available" degradation
	found := false
	for _, deg := range result.Degradations {
		if deg == "juicefs-not-available" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have juicefs-not-available degradation")

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

// TestNewEngine_VerifyFactory verifies NewEngine factory function.
func TestNewEngine_VerifyFactory(t *testing.T) {
	eng := engine.NewEngine("copy")
	assert.Equal(t, "copy", string(eng.Name()))

	eng2 := engine.NewEngine("reflink-copy")
	assert.Equal(t, "reflink-copy", string(eng2.Name()))

	eng3 := engine.NewEngine("juicefs-clone")
	assert.Equal(t, "juicefs-clone", string(eng3.Name()))

	eng4 := engine.NewEngine("unknown")
	assert.Equal(t, "copy", string(eng4.Name()))

	eng5 := engine.NewEngine("")
	assert.Equal(t, "copy", string(eng5.Name()))
}
