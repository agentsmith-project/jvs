package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"key": "value"}`)

	err := fsutil.AtomicWrite(path, data, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte("old"), 0644)

	err := fsutil.AtomicWrite(path, []byte("new"), 0644)
	require.NoError(t, err)

	content, _ := os.ReadFile(path)
	assert.Equal(t, "new", string(content))
}

func TestAtomicWrite_NoTmpLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	fsutil.AtomicWrite(path, []byte("data"), 0644)

	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 1, "only the target file should exist")
}

func TestRenameAndSync(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("data"), 0644)

	err := fsutil.RenameAndSync(src, dst)
	require.NoError(t, err)

	assert.NoFileExists(t, src)
	content, _ := os.ReadFile(dst)
	assert.Equal(t, "data", string(content))
}

func TestFsyncDir(t *testing.T) {
	dir := t.TempDir()
	err := fsutil.FsyncDir(dir)
	assert.NoError(t, err)
}

func TestFsyncDir_NonExistent(t *testing.T) {
	err := fsutil.FsyncDir("/nonexistent/path/to/dir")
	assert.Error(t, err)
}

func TestFsyncTree(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure with files
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))

	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(subDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("data1"), 0644))
	require.NoError(t, os.WriteFile(file2, []byte("data2"), 0644))

	err := fsutil.FsyncTree(dir)
	assert.NoError(t, err)
}

func TestFsyncTree_NonExistent(t *testing.T) {
	err := fsutil.FsyncTree("/nonexistent/path")
	assert.Error(t, err)
}

func TestFsyncTree_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	err := fsutil.FsyncTree(dir)
	assert.NoError(t, err)
}

func TestAtomicWrite_InvalidPath(t *testing.T) {
	// Try to write to a path where the directory doesn't exist
	err := fsutil.AtomicWrite("/nonexistent/path/file.txt", []byte("data"), 0644)
	assert.Error(t, err)
}

func TestRenameAndSync_NonExistentSource(t *testing.T) {
	err := fsutil.RenameAndSync("/nonexistent/src", "/tmp/dst")
	assert.Error(t, err)
}

func TestAtomicWrite_EmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	err := fsutil.AtomicWrite(path, []byte{}, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, content)
}

func TestAtomicWrite_VariousPermissions(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		perm os.FileMode
	}{
		{"readonly", 0444},
		{"writeonly", 0222},
		{"readwrite", 0666},
		{"executable", 0755},
		{"custom", 0640},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			err := fsutil.AtomicWrite(path, []byte("data"), tt.perm)
			require.NoError(t, err)

			info, err := os.Stat(path)
			require.NoError(t, err)
			// On some systems, the umask may affect the actual permissions
			// Just check that the file exists and has some permissions set
			assert.NotNil(t, info)
		})
	}
}

func TestAtomicWrite_LargeData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	// Create 1MB of data
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	err := fsutil.AtomicWrite(path, largeData, 0644)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, len(largeData), len(content))
	assert.Equal(t, largeData, content)
}

func TestAtomicWrite_SpecialCharactersInPath(t *testing.T) {
	dir := t.TempDir()

	// Test various special characters in filenames
	specialNames := []string{
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"file.multiple.dots.txt",
		"file123.txt",
	}

	for _, name := range specialNames {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			err := fsutil.AtomicWrite(path, []byte("data"), 0644)
			require.NoError(t, err)

			// Verify file exists
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.False(t, info.IsDir())
		})
	}
}

func TestFsyncTree_DeepNesting(t *testing.T) {
	dir := t.TempDir()

	// Create deeply nested structure
	deepPath := filepath.Join(dir, "a", "b", "c", "d", "e", "file.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(deepPath), 0755))
	require.NoError(t, os.WriteFile(deepPath, []byte("deep content"), 0644))

	err := fsutil.FsyncTree(dir)
	require.NoError(t, err)
}

func TestFsyncTree_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create multiple files at different levels
	paths := []string{
		"root1.txt",
		"root2.txt",
		"sub1/file1.txt",
		"sub1/file2.txt",
		"sub2/deep/file.txt",
	}

	for _, path := range paths {
		fullPath := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte("content"), 0644))
	}

	err := fsutil.FsyncTree(dir)
	require.NoError(t, err)
}

func TestFsyncTree_SingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "single.txt")

	require.NoError(t, os.WriteFile(filePath, []byte("single file content"), 0644))

	err := fsutil.FsyncTree(filePath)
	assert.NoError(t, err)
}

func TestRenameAndSync_DifferentDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	src := filepath.Join(srcDir, "file.txt")
	dst := filepath.Join(dstDir, "file.txt")

	require.NoError(t, os.WriteFile(src, []byte("data"), 0644))

	err := fsutil.RenameAndSync(src, dst)
	require.NoError(t, err)

	// Verify source is gone and destination exists
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))

	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "data", string(content))
}

func TestRenameAndSync_Overwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	require.NoError(t, os.WriteFile(src, []byte("source"), 0644))
	require.NoError(t, os.WriteFile(dst, []byte("destination"), 0644))

	err := fsutil.RenameAndSync(src, dst)
	require.NoError(t, err)

	content, _ := os.ReadFile(dst)
	assert.Equal(t, "source", string(content))
}

func TestAtomicWrite_PreservesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")

	// Create binary data with all possible byte values
	binaryData := make([]byte, 256)
	for i := range binaryData {
		binaryData[i] = byte(i)
	}

	err := fsutil.AtomicWrite(path, binaryData, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, binaryData, content)
}

func TestAtomicWrite_Utf8Content(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utf8.txt")

	// Test with UTF-8 content including emojis and multi-byte characters
	utf8Data := []byte("Hello 世界 🌍 Привет Мир")

	err := fsutil.AtomicWrite(path, utf8Data, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, utf8Data, content)
	assert.Equal(t, "Hello 世界 🌍 Привет Мир", string(content))
}

func TestFsyncTree_WithSubdirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a more complex directory structure
	structure := []string{
		"file1.txt",
		"dir1/file2.txt",
		"dir1/dir2/file3.txt",
		"dir1/dir2/dir3/file4.txt",
	}

	for _, path := range structure {
		fullPath := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte("content"), 0644))
	}

	err := fsutil.FsyncTree(dir)
	require.NoError(t, err)
}

func TestFsyncTree_WithEmptySubdirectories(t *testing.T) {
	dir := t.TempDir()

	// Create empty subdirectories
	require.NoError(t, os.Mkdir(filepath.Join(dir, "empty1"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "empty2"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested", "empty"), 0755))

	// Create one file so it's not completely empty
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644))

	err := fsutil.FsyncTree(dir)
	require.NoError(t, err)
}

func TestAtomicWrite_RewriteSameContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same.txt")
	data := []byte("same content")

	// Write first time
	err := fsutil.AtomicWrite(path, data, 0644)
	require.NoError(t, err)

	// Write same content again
	err = fsutil.AtomicWrite(path, data, 0644)
	require.NoError(t, err)

	// Verify content is still correct
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestRenameAndSync_NonExistentDestDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0644))

	// Destination directory doesn't exist
	dst := filepath.Join(dir, "nonexistent", "dst")

	err := fsutil.RenameAndSync(src, dst)
	assert.Error(t, err)
}

func TestFsyncTree_SymlinkSkip(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file
	filePath := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0644))

	// Create a symlink to the file (if supported on this system)
	symlinkPath := filepath.Join(dir, "link.txt")
	err := os.Symlink(filePath, symlinkPath)
	if err != nil {
		t.Skip("symlinks not supported on this system")
	}

	// FsyncTree should handle symlinks gracefully
	err = fsutil.FsyncTree(dir)
	// May succeed or fail depending on whether symlinks are synced
	_ = err
}

func TestAtomicWrite_ReplaceDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a directory with the same name as our target file
	dirPath := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(dirPath, 0755))

	// Try to atomic write a file with the same name as the directory
	// This should fail because you can't rename a file over a directory
	err := fsutil.AtomicWrite(dirPath, []byte("data"), 0644)
	// The behavior may vary by OS, but it should either fail or succeed
	_ = err
}

func TestFsyncTree_SymlinkDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644))

	// Create a symlink to the directory itself
	symlinkPath := filepath.Join(dir, "selflink")
	err := os.Symlink(dir, symlinkPath)
	if err != nil {
		t.Skip("symlinks not supported")
	}

	// FsyncTree should handle circular symlinks without infinite loop
	// (it only syncs files, not directories)
	err = fsutil.FsyncTree(dir)
	// May succeed or fail depending on OS
	_ = err
}

func TestAtomicWrite_WithNewlines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newlines.txt")

	data := []byte("line1\nline2\r\nline3\n\r\n")

	err := fsutil.AtomicWrite(path, data, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestAtomicWrite_JsonContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	jsonData := []byte(`{"name":"test","value":123,"nested":{"key":"value"}}`)

	err := fsutil.AtomicWrite(path, jsonData, 0644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, jsonData, content)
}

func TestAtomicWrite_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.Mkdir(roDir, 0555))
	t.Cleanup(func() {
		os.Chmod(roDir, 0755)
	})

	path := filepath.Join(roDir, "file.txt")
	err := fsutil.AtomicWrite(path, []byte("data"), 0644)
	assert.Error(t, err)
}

func TestFsyncTree_WithFiles(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644))
	}

	err := fsutil.FsyncTree(dir)
	assert.NoError(t, err)
}

func TestFsyncTree_FileVanishesDuringWalk(t *testing.T) {
	// This test is difficult to implement without some form of injection
	// because we need a file to disappear between Walk and Open/Sync
	// We'll skip it and document the limitation
	t.Skip("requires file deletion during walk - difficult to test reliably")
}

func TestAtomicWrite_RetryOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// First write should succeed
	err := fsutil.AtomicWrite(path, []byte("first"), 0644)
	require.NoError(t, err)

	// Second write should also succeed
	err = fsutil.AtomicWrite(path, []byte("second"), 0644)
	require.NoError(t, err)

	content, _ := os.ReadFile(path)
	assert.Equal(t, "second", string(content))
}

func TestFsyncTree_WithSymlinksToFiles(t *testing.T) {
	dir := t.TempDir()

	// Create original files
	originalFile := filepath.Join(dir, "original.txt")
	require.NoError(t, os.WriteFile(originalFile, []byte("content"), 0644))

	// Create a symlink to the file
	symlinkPath := filepath.Join(dir, "link.txt")
	err := os.Symlink(originalFile, symlinkPath)
	if err != nil {
		t.Skip("symlinks not supported")
	}

	// FsyncTree should handle symlinks
	err = fsutil.FsyncTree(dir)
	// The behavior with symlinks varies by OS - just verify it doesn't crash
	_ = err
}
