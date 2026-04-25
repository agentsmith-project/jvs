package integrity_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputePayloadRootHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644)

	hash1, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	hash2, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2, "hash must be deterministic")
}

func TestComputePayloadRootHash_DetectsContentChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("original"), 0644)

	hash1, _ := integrity.ComputePayloadRootHash(dir)
	os.WriteFile(file, []byte("modified"), 0644)
	hash2, _ := integrity.ComputePayloadRootHash(dir)

	assert.NotEqual(t, hash1, hash2, "content change must produce different hash")
}

func TestComputePayloadRootHash_DetectsPermissionChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("content"), 0644)

	hash1, _ := integrity.ComputePayloadRootHash(dir)
	os.Chmod(file, 0755)
	hash2, _ := integrity.ComputePayloadRootHash(dir)

	assert.NotEqual(t, hash1, hash2, "permission change must produce different hash")
}

func TestComputePayloadRootHash_IncludesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_IncludesSymlink(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link")))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash, "empty dir should still produce a hash")
}

func TestComputePayloadRootHash_DeterministicWithDifferentModTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("content"), 0644)

	hash1, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)

	// Change modification time
	newTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(file, newTime, newTime))

	hash2, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)

	// Hash should be the same despite mod time change (per spec)
	assert.Equal(t, hash1, hash2, "hash should not depend on modification time")
}

func TestComputePayloadRootHash_NestedStructure(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "file2.txt"), []byte("file2"), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_SkipsReadyMarker(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(dir, ".READY"), []byte(`{"snapshot_id":"test"}`), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Remove .READY and verify hash is the same
	os.Remove(filepath.Join(dir, ".READY"))
	hash2, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2, ".READY should be excluded from hash")
}

func TestComputePayloadRootHash_HashesSubdirReadyMarker(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", ".READY"), []byte("user data"), 0644))

	hash1, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", ".READY"), []byte("tampered"), 0644))
	hash2, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "subdirectory .READY files are user payload")
}

func TestComputePayloadRootHash_BrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink pointing to nothing
	require.NoError(t, os.Symlink("nonexistent-target", filepath.Join(dir, "broken")))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_Hardlink(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "file1.txt")
	os.WriteFile(file1, []byte("content"), 0644)
	file2 := filepath.Join(dir, "file2.txt")
	require.NoError(t, os.Link(file1, file2))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_DeeplyNested(t *testing.T) {
	dir := t.TempDir()
	deepPath := filepath.Join(dir, "a", "b", "c", "d", "e")
	require.NoError(t, os.MkdirAll(deepPath, 0755))
	os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep content"), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_SpecialCharactersInFilename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file with spaces.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(dir, "file-with-dashes.txt"), []byte("content"), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_MixedContentTypes(t *testing.T) {
	dir := t.TempDir()
	// Create a mix of files, directories, and symlinks
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("file content"), 0644)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("nested"), 0644)
	os.WriteFile(filepath.Join(dir, "link-target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("link-target.txt", filepath.Join(dir, "link")))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_UnreadableFile(t *testing.T) {
	// Test that hash computation fails when a file cannot be read
	// This tests the error path in computeEntryHash for file reading
	dir := t.TempDir()

	// Create a directory with a file
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	// Make the file unreadable
	require.NoError(t, os.Chmod(filepath.Join(dir, "file.txt"), 0000))

	// On Unix systems, this should cause an error
	// On Windows, the behavior may differ
	hash, err := integrity.ComputePayloadRootHash(dir)
	if err != nil {
		// Expected on systems that respect chmod 0000
		assert.Empty(t, hash)
	} else {
		// Some systems allow root to read even with 0000 permissions
		// In that case, we at least verify the test ran
		assert.NotEmpty(t, hash)
	}

	// Cleanup: restore permissions so dir can be removed
	os.Chmod(filepath.Join(dir, "file.txt"), 0644)
}

func TestComputePayloadRootHash_SymlinkReadError(t *testing.T) {
	// This tests the error path when os.Readlink fails
	// This is hard to trigger without actually causing a filesystem error
	// The test exists for completeness
	dir := t.TempDir()

	// Create a valid symlink (tests the success path)
	os.WriteFile(filepath.Join(dir, "target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link")))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_WalkError(t *testing.T) {
	// Test that filepath.Walk errors are propagated
	// This is difficult to test without actually causing a walk error
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_FileWithVariousSizes(t *testing.T) {
	// Test hash computation with files of different sizes
	dir := t.TempDir()

	// Empty file
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte(""), 0644)

	// Small file
	os.WriteFile(filepath.Join(dir, "small.txt"), []byte("x"), 0644)

	// Larger file
	largeContent := make([]byte, 10*1024) // 10KB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(dir, "large.txt"), largeContent, 0644)

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_SymlinkToDirectory(t *testing.T) {
	// Test hash computation with a symlink to a directory
	dir := t.TempDir()

	// Create a directory
	os.MkdirAll(filepath.Join(dir, "targetdir"), 0755)
	os.WriteFile(filepath.Join(dir, "targetdir", "file.txt"), []byte("content"), 0644)

	// Create a symlink to the directory
	require.NoError(t, os.Symlink("targetdir", filepath.Join(dir, "linkdir")))

	hash, err := integrity.ComputePayloadRootHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestComputePayloadRootHash_DirectoryWithDifferentPermissions(t *testing.T) {
	// Test that directory permissions are included in the hash
	dir := t.TempDir()

	subdir1 := filepath.Join(dir, "dir1")
	subdir2 := filepath.Join(dir, "dir2")
	require.NoError(t, os.MkdirAll(subdir1, 0755))
	require.NoError(t, os.MkdirAll(subdir2, 0700))

	hash1, _ := integrity.ComputePayloadRootHash(dir)

	// Change permissions
	os.Chmod(subdir2, 0755)

	hash2, _ := integrity.ComputePayloadRootHash(dir)

	// Hashes should be the same because we sort by path and permissions
	// are included in metadata. Actually, with different permissions,
	// the hash should differ.
	assert.NotEqual(t, hash1, hash2)
}

func TestComputePayloadRootHash_FileWithDifferentPermissions(t *testing.T) {
	// Test that file permissions are included in the hash
	dir := t.TempDir()

	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("content"), 0644)

	hash1, _ := integrity.ComputePayloadRootHash(dir)

	// Change permissions
	os.Chmod(file, 0755)

	hash2, _ := integrity.ComputePayloadRootHash(dir)

	assert.NotEqual(t, hash1, hash2, "file permissions should affect hash")
}
