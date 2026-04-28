package snapshotpayload_test

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterialize_CompressedSnapshotDecodesPayloadAndRemovesControlMarkers(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")

	writeGzipFile(t, filepath.Join(src, "file.txt.gz"), []byte("logical content"), 0644)
	writeReadyWithCompressionManifest(t, src, []string{"file.txt"})

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{Compressed: true}, copyTree)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dst, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "logical content", string(content))
	assert.NoFileExists(t, filepath.Join(dst, "file.txt.gz"))
	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
}

func TestMaterialize_UncompressedSnapshotKeepsUserGzipFile(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")

	require.NoError(t, os.WriteFile(filepath.Join(src, "archive.gz"), []byte("not actually gzip"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".READY"), []byte("ready"), 0644))

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{Compressed: false}, copyTree)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dst, "archive.gz"))
	require.NoError(t, err)
	assert.Equal(t, "not actually gzip", string(content))
	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
}

func TestMaterializedLogicalSizeUncompressedExcludesOnlyRootControlMarkers(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY"), []byte("root control"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY.gz"), []byte("root control gzip"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("payload"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", ".READY"), []byte("user ready"), 0644))

	size, err := snapshotpayload.MaterializedLogicalSize(root, snapshotpayload.Options{})
	require.NoError(t, err)
	assert.EqualValues(t, len("payload")+len("user ready"), size)
}

func TestMaterializedLogicalSizeCompressedUsesManifestOriginalSize(t *testing.T) {
	root := t.TempDir()
	writeGzipFile(t, filepath.Join(root, "large.txt.gz"), []byte("tiny storage"), 0644)
	require.NoError(t, os.WriteFile(filepath.Join(root, "user.gz"), []byte("user gzip file"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", ".READY"), []byte("user ready"), 0644))
	writeReadyWithCompressionManifestEntries(t, root, []map[string]any{
		{
			"path":            "large.txt",
			"compressed_path": "large.txt.gz",
			"original_size":   int64(10_000_000),
		},
	})

	size, err := snapshotpayload.MaterializedLogicalSize(root, snapshotpayload.Options{Compressed: true})
	require.NoError(t, err)
	assert.EqualValues(t, 10_000_000+len("user gzip file")+len("user ready"), size)
}

func TestMaterializedLogicalSizeCompressedFailsClosedOnInvalidManifest(t *testing.T) {
	root := t.TempDir()
	writeGzipFile(t, filepath.Join(root, "large.txt.gz"), []byte("tiny storage"), 0644)
	writeReadyWithCompressionManifestEntries(t, root, []map[string]any{
		{
			"path":            "large.txt",
			"compressed_path": "large.txt.gz",
		},
	})

	_, err := snapshotpayload.MaterializedLogicalSize(root, snapshotpayload.Options{Compressed: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing original size")
}

func TestMaterializationCapacityEstimateCompressedPeakIncludesStorageCloneAndStaging(t *testing.T) {
	root := t.TempDir()
	writeGzipFile(t, filepath.Join(root, "tiny.txt.gz"), []byte("x"), 0644)
	require.NoError(t, os.WriteFile(filepath.Join(root, "user.txt"), []byte("plain"), 0644))
	writeReadyWithCompressionManifest(t, root, []string{"tiny.txt"})
	storageBytes := logicalFixtureTreeSize(t, root)

	estimate, err := snapshotpayload.EstimateMaterializationCapacity(root, snapshotpayload.Options{Compressed: true})
	require.NoError(t, err)

	assert.EqualValues(t, len("x")+len("plain"), estimate.FinalBytes)
	assert.EqualValues(t, storageBytes+int64(len("x")), estimate.PeakBytes)
	assert.Greater(t, estimate.PeakBytes, estimate.FinalBytes)
}

func TestMaterializationCapacityEstimateUncompressedPeakIncludesStorageControlMarkers(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY"), []byte("root control"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("payload"), 0644))

	estimate, err := snapshotpayload.EstimateMaterializationCapacity(root, snapshotpayload.Options{})
	require.NoError(t, err)

	assert.EqualValues(t, len("payload"), estimate.FinalBytes)
	assert.EqualValues(t, len("root control")+len("payload"), estimate.PeakBytes)
}

func TestMaterialize_CompressedSnapshotRemovesRootReadyGzipControlMarker(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")

	writeGzipFile(t, filepath.Join(src, "data.txt.gz"), []byte("logical data"), 0644)
	writeReadyWithCompressionManifest(t, src, []string{"data.txt"})
	require.NoError(t, os.WriteFile(filepath.Join(src, ".READY.gz"), []byte("user gzip data"), 0644))

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{Compressed: true}, copyTree)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dst, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "logical data", string(data))
	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
	assert.NoFileExists(t, filepath.Join(dst, ".READY.gz"))
}

func TestMaterialize_CompressedSnapshotUsesManifestForGzipFiles(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")

	writeGzipFile(t, filepath.Join(src, "data.txt.gz"), []byte("logical data"), 0644)
	userGzipContent := []byte("user-owned .gz content")
	require.NoError(t, os.WriteFile(filepath.Join(src, "archive.gz"), userGzipContent, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(src, "link.gz")))
	writeReadyWithCompressionManifest(t, src, []string{"data.txt"})

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{Compressed: true}, copyTree)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dst, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "logical data", string(content))
	assert.NoFileExists(t, filepath.Join(dst, "data.txt.gz"))

	archive, err := os.ReadFile(filepath.Join(dst, "archive.gz"))
	require.NoError(t, err)
	assert.Equal(t, userGzipContent, archive)

	info, err := os.Lstat(filepath.Join(dst, "link.gz"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, err := os.Readlink(filepath.Join(dst, "link.gz"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
}

func TestMaterialize_OnlyStripsRootReadyMarker(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")

	require.NoError(t, os.WriteFile(filepath.Join(src, ".READY"), []byte("control"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "subdir", ".READY"), []byte("user data"), 0644))

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{}, copyTree)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
	content, err := os.ReadFile(filepath.Join(dst, "subdir", ".READY"))
	require.NoError(t, err)
	assert.Equal(t, "user data", string(content))
}

func TestMaterialize_RejectsExistingNonEmptyDestination(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")
	require.NoError(t, os.MkdirAll(dst, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0644))

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{}, copyTree)
	require.Error(t, err)

	content, readErr := os.ReadFile(filepath.Join(dst, "old.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(content))
	assert.NoFileExists(t, filepath.Join(dst, "new.txt"))
}

func TestMaterialize_RejectsDestinationFileOrSymlink(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0644))

	t.Run("file", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "materialized")
		require.NoError(t, os.WriteFile(dst, []byte("old"), 0644))

		err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{}, copyTree)
		require.Error(t, err)

		content, readErr := os.ReadFile(dst)
		require.NoError(t, readErr)
		assert.Equal(t, "old", string(content))
	})

	t.Run("symlink", func(t *testing.T) {
		parent := t.TempDir()
		outside := t.TempDir()
		dst := filepath.Join(parent, "materialized")
		if err := os.Symlink(outside, dst); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}

		err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{}, copyTree)
		require.Error(t, err)
		assert.NoFileExists(t, filepath.Join(outside, "new.txt"))
	})
}

func TestMaterialize_AllowsExistingEmptyRealDirectory(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")
	require.NoError(t, os.MkdirAll(dst, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0644))

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{}, copyTree)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(dst, "new.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "new", string(content))
}

func TestMaterialize_CompressedSnapshotRejectsManifestOutputSymlinkParent(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")
	outside := t.TempDir()

	writeGzipFile(t, filepath.Join(src, "source.gz"), []byte("outside write"), 0644)
	require.NoError(t, os.Symlink(outside, filepath.Join(src, "escape")))
	writeReadyWithCompressionManifestEntries(t, src, []map[string]any{
		{
			"path":            "escape/out.txt",
			"compressed_path": "source.gz",
			"original_size":   int64(len("outside write")),
		},
	})

	err := snapshotpayload.Materialize(src, dst, snapshotpayload.Options{Compressed: true}, copyTree)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(outside, "out.txt"))
}

func TestClean_CompressedSnapshotDecompressFailureLeavesStorageTree(t *testing.T) {
	root := t.TempDir()

	writeGzipFile(t, filepath.Join(root, "first.txt.gz"), []byte("first"), 0644)
	secondGzip := filepath.Join(root, "second.txt.gz")
	secondGzipContent := []byte("this is not valid gzip data")
	require.NoError(t, os.WriteFile(secondGzip, secondGzipContent, 0644))
	writeReadyWithCompressionManifest(t, root, []string{"first.txt", "second.txt"})

	err := snapshotpayload.Clean(root, snapshotpayload.Options{Compressed: true})
	require.Error(t, err)

	assert.NoFileExists(t, filepath.Join(root, "first.txt"))
	assert.FileExists(t, filepath.Join(root, "first.txt.gz"))
	assert.NoFileExists(t, filepath.Join(root, "second.txt"))
	content, readErr := os.ReadFile(secondGzip)
	require.NoError(t, readErr)
	assert.Equal(t, secondGzipContent, content)
	assert.FileExists(t, filepath.Join(root, ".READY"))
}

func TestComputeHash_IgnoresRootReadyGzipControlMarker(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "payload.txt"), []byte("payload"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY"), []byte("ready"), 0644))

	hashWithoutReadyGzip, err := snapshotpayload.ComputeHash(root, snapshotpayload.Options{})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY.gz"), []byte("ready alias"), 0644))
	hashWithReadyGzip, err := snapshotpayload.ComputeHash(root, snapshotpayload.Options{})
	require.NoError(t, err)

	assert.Equal(t, hashWithoutReadyGzip, hashWithReadyGzip)
}

func copyTree(src, dst string) error {
	_, err := engine.NewCopyEngine().Clone(src, dst)
	return err
}

func writeGzipFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	require.NoError(t, err)
	defer f.Close()

	w := gzip.NewWriter(f)
	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Chmod(mode))
}

func writeReadyWithCompressionManifest(t *testing.T, root string, paths []string) {
	t.Helper()

	files := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		compressedPath := filepath.ToSlash(path + ".gz")
		files = append(files, map[string]any{
			"path":            filepath.ToSlash(path),
			"compressed_path": compressedPath,
			"original_size":   gzipFixtureOriginalSize(t, filepath.Join(root, filepath.FromSlash(compressedPath))),
		})
	}
	writeReadyWithCompressionManifestEntries(t, root, files)
}

func writeReadyWithCompressionManifestEntries(t *testing.T, root string, files []map[string]any) {
	t.Helper()

	marker := map[string]any{
		"snapshot_id": "test",
		"compression_manifest": map[string]any{
			"version": 1,
			"type":    "gzip",
			"files":   files,
		},
	}
	data, err := json.Marshal(marker)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".READY"), data, 0644))
}

func gzipFixtureOriginalSize(t *testing.T, path string) int64 {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	r, err := gzip.NewReader(f)
	if err != nil {
		return 0
	}
	defer r.Close()

	buf := make([]byte, 1024)
	var total int64
	for {
		n, err := r.Read(buf)
		total += int64(n)
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return total
		}
		return 0
	}
}

func logicalFixtureTreeSize(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := entry.Info()
		require.NoError(t, err)
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	}))
	return total
}
