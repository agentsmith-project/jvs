package diff

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffer_Diff_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	// Create identical snapshots
	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap1 := createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add same file to both
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(filepath.Join(snap1, "file.txt"), content, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(snap2, "file.txt"), content, 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 0, result.TotalModified)
}

func TestDiffer_Diff_AddedFile(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add file only to snap2
	require.NoError(t, os.WriteFile(filepath.Join(snap2, "newfile.txt"), []byte("new"), 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 1, result.TotalAdded)
	assert.Equal(t, "newfile.txt", result.Added[0].Path)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 0, result.TotalModified)
}

func TestDiffer_Diff_RemovedFile(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap1 := createDiffSnapshot(t, tmpDir, fromID, false)
	createDiffSnapshot(t, tmpDir, toID, false)

	// Add file only to snap1
	require.NoError(t, os.WriteFile(filepath.Join(snap1, "removed.txt"), []byte("gone"), 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 1, result.TotalRemoved)
	assert.Equal(t, "removed.txt", result.Removed[0].Path)
	assert.Equal(t, 0, result.TotalModified)
}

func TestDiffer_Diff_ModifiedFile(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap1 := createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add same path with different content
	require.NoError(t, os.WriteFile(filepath.Join(snap1, "file.txt"), []byte("old"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(snap2, "file.txt"), []byte("new"), 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 1, result.TotalModified)
	assert.Equal(t, "file.txt", result.Modified[0].Path)
	assert.Equal(t, int64(3), result.Modified[0].OldSize)
	assert.Equal(t, int64(3), result.Modified[0].Size)
}

func TestDiffer_Diff_Symlink(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap1 := createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add symlink to snap1
	require.NoError(t, os.Symlink("target.txt", filepath.Join(snap1, "link")))

	// Add symlink to snap2 with different target
	require.NoError(t, os.Symlink("othertarget.txt", filepath.Join(snap2, "link")))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 1, result.TotalModified)
	assert.True(t, result.Modified[0].IsSymlink)
}

func TestDiffer_Diff_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap1 := createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add nested files
	require.NoError(t, os.MkdirAll(filepath.Join(snap1, "a", "b"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snap1, "a", "b", "file.txt"), []byte("nested"), 0644))

	require.NoError(t, os.MkdirAll(filepath.Join(snap2, "a", "b"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snap2, "a", "b", "file.txt"), []byte("modified"), 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 1, result.TotalModified)
	assert.Equal(t, filepath.Join("a", "b", "file.txt"), result.Modified[0].Path)
}

func TestDiffer_Diff_EmptyFrom(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add file to snap2
	require.NoError(t, os.WriteFile(filepath.Join(snap2, "file.txt"), []byte("content"), 0644))
	publishDiffSnapshot(t, tmpDir, toID, false)

	// Diff from empty (no fromID)
	result, err := differ.Diff("", toID)
	require.NoError(t, err)

	assert.Equal(t, 1, result.TotalAdded)
	assert.Equal(t, "file.txt", result.Added[0].Path)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 0, result.TotalModified)
}

func TestDiffer_Diff_SkipsReadyMarker(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800000-a3f7c1b2")
	toID := model.SnapshotID("1708300800001-b4c8d2e3")
	createDiffSnapshot(t, tmpDir, fromID, false)
	snap2 := createDiffSnapshot(t, tmpDir, toID, false)

	// Add .READY marker to snap2 (should be ignored)
	require.NoError(t, os.WriteFile(filepath.Join(snap2, ".READY"), []byte("{}"), 0644))
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff("", toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
}

func TestDiffer_Diff_CompressedAndUncompressedSameLogicalContentNoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)
	fromID := model.SnapshotID("1708300800010-aaaaaaaa")
	toID := model.SnapshotID("1708300800011-bbbbbbbb")

	fromSnap := createDiffSnapshot(t, tmpDir, fromID, false)
	toSnap := createDiffSnapshot(t, tmpDir, toID, true)

	require.NoError(t, os.WriteFile(filepath.Join(fromSnap, "file.txt"), []byte("logical content"), 0644))
	entry := writeCompressedLogicalFile(t, toSnap, "file.txt", []byte("logical content"), 0644)
	writeReadyWithCompressionManifest(t, toSnap, []compressionManifestEntry{entry})
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, true)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 0, result.TotalModified)
}

func TestDiffer_Diff_CompressedChangedLogicalFileUsesUserPath(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)
	fromID := model.SnapshotID("1708300800012-cccccccc")
	toID := model.SnapshotID("1708300800013-dddddddd")

	fromSnap := createDiffSnapshot(t, tmpDir, fromID, true)
	toSnap := createDiffSnapshot(t, tmpDir, toID, true)

	fromEntry := writeCompressedLogicalFile(t, fromSnap, "file.txt", []byte("old"), 0644)
	toEntry := writeCompressedLogicalFile(t, toSnap, "file.txt", []byte("new content"), 0600)
	writeReadyWithCompressionManifest(t, fromSnap, []compressionManifestEntry{fromEntry})
	writeReadyWithCompressionManifest(t, toSnap, []compressionManifestEntry{toEntry})
	publishDiffSnapshot(t, tmpDir, fromID, true)
	publishDiffSnapshot(t, tmpDir, toID, true)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	require.Equal(t, 0, result.TotalAdded)
	require.Equal(t, 0, result.TotalRemoved)
	require.Equal(t, 1, result.TotalModified)
	change := result.Modified[0]
	assert.Equal(t, "file.txt", change.Path)
	assert.Equal(t, int64(len("old")), change.OldSize)
	assert.Equal(t, int64(len("new content")), change.Size)
	assert.Equal(t, os.FileMode(0600), change.Mode.Perm())
	assert.NotEmpty(t, change.OldHash)
	assert.NotEmpty(t, change.NewHash)
	assert.NotEqual(t, change.OldHash, change.NewHash)
	assertNoGzipChangePaths(t, result)
}

func TestDiffer_Diff_CompressedSnapshotKeepsUserOwnedGzipPath(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)
	fromID := model.SnapshotID("1708300800014-eeeeeeee")
	toID := model.SnapshotID("1708300800015-ffffffff")

	fromSnap := createDiffSnapshot(t, tmpDir, fromID, true)
	toSnap := createDiffSnapshot(t, tmpDir, toID, true)

	fromEntry := writeCompressedLogicalFile(t, fromSnap, "data.txt", []byte("same logical file"), 0644)
	toEntry := writeCompressedLogicalFile(t, toSnap, "data.txt", []byte("same logical file"), 0644)
	require.NoError(t, os.WriteFile(filepath.Join(fromSnap, "archive.gz"), []byte("old user gzip bytes"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(toSnap, "archive.gz"), []byte("new user gzip bytes"), 0644))
	writeReadyWithCompressionManifest(t, fromSnap, []compressionManifestEntry{fromEntry})
	writeReadyWithCompressionManifest(t, toSnap, []compressionManifestEntry{toEntry})
	publishDiffSnapshot(t, tmpDir, fromID, true)
	publishDiffSnapshot(t, tmpDir, toID, true)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	require.Equal(t, 0, result.TotalAdded)
	require.Equal(t, 0, result.TotalRemoved)
	require.Equal(t, 1, result.TotalModified)
	assert.Equal(t, "archive.gz", result.Modified[0].Path)
	assert.Equal(t, int64(len("old user gzip bytes")), result.Modified[0].OldSize)
	assert.Equal(t, int64(len("new user gzip bytes")), result.Modified[0].Size)
}

func TestDiffResult_FormatHuman(t *testing.T) {
	result := &DiffResult{
		FromSnapshotID: "1708300800000-a3f7c1b2",
		ToSnapshotID:   "1708300800001-b4c8d2e3",
		Added: []*Change{
			{Path: "newfile.txt", Type: ChangeAdded},
		},
		Removed: []*Change{
			{Path: "oldfile.txt", Type: ChangeRemoved},
		},
		Modified: []*Change{
			{Path: "changed.txt", Type: ChangeModified, OldSize: 100, Size: 200},
		},
		TotalAdded:    1,
		TotalRemoved:  1,
		TotalModified: 1,
	}

	output := result.FormatHuman()
	assert.Contains(t, output, "Added (1):")
	assert.Contains(t, output, "+ newfile.txt")
	assert.Contains(t, output, "Removed (1):")
	assert.Contains(t, output, "- oldfile.txt")
	assert.Contains(t, output, "Modified (1):")
	assert.Contains(t, output, "~ changed.txt")
	assert.Contains(t, output, "(100 -> 200 bytes)")
}

func TestDiff_NonExistentSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".jvs", "snapshots"), 0755))

	// Both snapshots don't exist.
	_, err := differ.Diff("1708300800004-feedface", "1708300800003-deadbeef")
	require.Error(t, err)
	assert.True(t, errors.Is(err, &errclass.JVSError{Code: "E_DESCRIPTOR_MISSING"}), "got %v", err)

	// Only fromID exists, toID missing
	createDiffSnapshot(t, tmpDir, "1708300800002-cafebabe", false)
	publishDiffSnapshot(t, tmpDir, "1708300800002-cafebabe", false)

	_, err = differ.Diff("1708300800002-cafebabe", "1708300800003-deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspect to publish state")
	assert.True(t, errors.Is(err, &errclass.JVSError{Code: "E_DESCRIPTOR_MISSING"}), "got %v", err)

	// Only toID exists, fromID missing
	_, err = differ.Diff("1708300800004-feedface", "1708300800002-cafebabe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspect from publish state")
	assert.True(t, errors.Is(err, &errclass.JVSError{Code: "E_DESCRIPTOR_MISSING"}), "got %v", err)
}

func TestDifferRejectsDamagedPublishState(t *testing.T) {
	tests := []struct {
		name     string
		wantCode string
		mutate   func(t *testing.T, repoRoot string, id model.SnapshotID)
	}{
		{
			name:     "missing READY",
			wantCode: "E_READY_MISSING",
			mutate: func(t *testing.T, repoRoot string, id model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(repoRoot, ".jvs", "snapshots", string(id), ".READY")))
			},
		},
		{
			name:     "malformed READY",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoRoot string, id model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".jvs", "snapshots", string(id), ".READY"), []byte("{not json"), 0644))
			},
		},
		{
			name:     "descriptor checksum mismatch with matching READY checksum",
			wantCode: "E_DESCRIPTOR_CHECKSUM_MISMATCH",
			mutate: func(t *testing.T, repoRoot string, id model.SnapshotID) {
				t.Helper()
				const badChecksum = "bad-descriptor-checksum"
				mutateDiffDescriptor(t, repoRoot, id, func(doc map[string]any) {
					doc["descriptor_checksum"] = badChecksum
				})
				mutateDiffReady(t, repoRoot, id, func(doc map[string]any) {
					doc["descriptor_checksum"] = badChecksum
				})
			},
		},
		{
			name:     "payload hash mismatch",
			wantCode: "E_PAYLOAD_HASH_MISMATCH",
			mutate: func(t *testing.T, repoRoot string, id model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".jvs", "snapshots", string(id), "tampered.txt"), []byte("tampered"), 0644))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			id := model.SnapshotID("1708300800000-deadbeef")
			snap := createDiffSnapshot(t, tmpDir, id, false)
			require.NoError(t, os.WriteFile(filepath.Join(snap, "file.txt"), []byte("content"), 0644))
			publishDiffSnapshot(t, tmpDir, id, false)
			tt.mutate(t, tmpDir, id)

			_, err := NewDiffer(tmpDir).Diff(id, id)
			require.Error(t, err)
			require.True(t, errors.Is(err, &errclass.JVSError{Code: tt.wantCode}), "got %v", err)
		})
	}
}

func TestDifferRejectsPathLikeAndNonCanonicalIDs(t *testing.T) {
	ids := []model.SnapshotID{
		"../../outside/escape",
		"/tmp/absolute",
		"nested/1708300800000-deadbeef",
		"1708300800000-DEADBEEF",
		"not-canonical",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			tmpDir := t.TempDir()
			differ := NewDiffer(tmpDir)
			toID := model.SnapshotID("1708300800000-deadbeef")

			fromPath := filepath.Join(tmpDir, ".jvs", "snapshots", string(id))
			toPath := filepath.Join(tmpDir, ".jvs", "snapshots", string(toID))
			require.NoError(t, os.MkdirAll(fromPath, 0755))
			require.NoError(t, os.MkdirAll(toPath, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(fromPath, "file.txt"), []byte("from"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(toPath, "file.txt"), []byte("to"), 0644))

			_, err := differ.Diff(id, toID)
			require.Error(t, err)
		})
	}
}

func TestDifferRejectsFinalSnapshotSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotsDir := filepath.Join(tmpDir, ".jvs", "snapshots")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0755))
	snapshotID := model.SnapshotID("1708300800000-deadbeef")

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0644))
	if err := os.Symlink(outside, filepath.Join(snapshotsDir, string(snapshotID))); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err := NewDiffer(tmpDir).Diff("", snapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.FileExists(t, filepath.Join(outside, "file.txt"))
}

func TestDiff_EmptySnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	differ := NewDiffer(tmpDir)

	fromID := model.SnapshotID("1708300800005-aaaaaaaa")
	toID := model.SnapshotID("1708300800006-bbbbbbbb")
	createDiffSnapshot(t, tmpDir, fromID, false)
	createDiffSnapshot(t, tmpDir, toID, false)
	publishDiffSnapshot(t, tmpDir, fromID, false)
	publishDiffSnapshot(t, tmpDir, toID, false)

	result, err := differ.Diff(fromID, toID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalAdded)
	assert.Equal(t, 0, result.TotalRemoved)
	assert.Equal(t, 0, result.TotalModified)
	assert.Empty(t, result.Added)
	assert.Empty(t, result.Removed)
	assert.Empty(t, result.Modified)
}

func TestDiffResult_FormatHuman_NoChanges(t *testing.T) {
	result := &DiffResult{
		FromSnapshotID: "1708300800000-a3f7c1b2",
		ToSnapshotID:   "1708300800001-b4c8d2e3",
		TotalAdded:     0,
		TotalRemoved:   0,
		TotalModified:  0,
	}

	output := result.FormatHuman()
	assert.Contains(t, output, "No changes.")
}

type compressionManifestEntry struct {
	path         string
	originalSize int64
}

func createDiffSnapshot(t *testing.T, repoRoot string, snapshotID model.SnapshotID, compressed bool) string {
	t.Helper()

	snapshotPath := filepath.Join(repoRoot, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotPath, 0755))
	return snapshotPath
}

func publishDiffSnapshot(t *testing.T, repoRoot string, snapshotID model.SnapshotID, compressed bool) {
	t.Helper()

	snapshotPath := filepath.Join(repoRoot, ".jvs", "snapshots", string(snapshotID))
	payloadHash, err := snapshotpayload.ComputeHash(snapshotPath, snapshotpayload.Options{Compressed: compressed})
	require.NoError(t, err)

	desc := model.Descriptor{
		SnapshotID:      snapshotID,
		WorktreeName:    "main",
		CreatedAt:       time.Unix(1708300800, 0).UTC(),
		Engine:          model.EngineCopy,
		PayloadRootHash: payloadHash,
		IntegrityState:  model.IntegrityVerified,
	}
	if compressed {
		desc.Compression = &model.CompressionInfo{
			Type:  "gzip",
			Level: 6,
		}
	}
	checksum, err := integrity.ComputeDescriptorChecksum(&desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	descriptorDir := filepath.Join(repoRoot, ".jvs", "descriptors")
	require.NoError(t, os.MkdirAll(descriptorDir, 0755))
	data, err := json.Marshal(desc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(descriptorDir, string(snapshotID)+".json"), data, 0644))

	writeDiffReadyMarker(t, snapshotPath, snapshotID, payloadHash, checksum)
}

func writeCompressedLogicalFile(t *testing.T, root, rel string, data []byte, mode os.FileMode) compressionManifestEntry {
	t.Helper()

	writeGzipFile(t, filepath.Join(root, rel+".gz"), data, mode)
	return compressionManifestEntry{
		path:         filepath.ToSlash(rel),
		originalSize: int64(len(data)),
	}
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

func writeReadyWithCompressionManifest(t *testing.T, root string, entries []compressionManifestEntry) {
	t.Helper()

	files := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		files = append(files, map[string]any{
			"path":            entry.path,
			"compressed_path": entry.path + ".gz",
			"original_size":   entry.originalSize,
		})
	}

	marker := map[string]any{
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

func writeDiffReadyMarker(t *testing.T, root string, snapshotID model.SnapshotID, payloadHash, checksum model.HashValue) {
	t.Helper()

	marker := map[string]any{}
	readyPath := filepath.Join(root, ".READY")
	if data, err := os.ReadFile(readyPath); err == nil && len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, &marker))
	} else if err != nil && !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	marker["snapshot_id"] = string(snapshotID)
	marker["completed_at"] = time.Unix(1708300800, 0).UTC()
	marker["payload_root_hash"] = string(payloadHash)
	marker["engine"] = string(model.EngineCopy)
	marker["descriptor_checksum"] = string(checksum)

	data, err := json.Marshal(marker)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readyPath, data, 0644))
}

func mutateDiffDescriptor(t *testing.T, repoRoot string, snapshotID model.SnapshotID, mutate func(map[string]any)) {
	t.Helper()
	mutateDiffJSONFile(t, filepath.Join(repoRoot, ".jvs", "descriptors", string(snapshotID)+".json"), mutate)
}

func mutateDiffReady(t *testing.T, repoRoot string, snapshotID model.SnapshotID, mutate func(map[string]any)) {
	t.Helper()
	mutateDiffJSONFile(t, filepath.Join(repoRoot, ".jvs", "snapshots", string(snapshotID), ".READY"), mutate)
}

func mutateDiffJSONFile(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	mutate(doc)
	data, err = json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func assertNoGzipChangePaths(t *testing.T, result *DiffResult) {
	t.Helper()

	for _, change := range result.Added {
		assert.NotContains(t, change.Path, ".gz")
	}
	for _, change := range result.Removed {
		assert.NotContains(t, change.Path, ".gz")
	}
	for _, change := range result.Modified {
		assert.NotContains(t, change.Path, ".gz")
	}
}
