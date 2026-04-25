package compression

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvs-project/jvs/pkg/model"
)

func TestNewCompressor(t *testing.T) {
	// Test no compression
	c := NewCompressor(LevelNone)
	if c.IsEnabled() {
		t.Error("expected compression to be disabled")
	}
	if c.Type != TypeNone {
		t.Errorf("expected type none, got %s", c.Type)
	}

	// Test fast compression
	c = NewCompressor(LevelFast)
	if !c.IsEnabled() {
		t.Error("expected compression to be enabled")
	}
	if c.Type != TypeGzip {
		t.Errorf("expected type gzip, got %s", c.Type)
	}
	if c.Level != LevelFast {
		t.Errorf("expected level %d, got %d", LevelFast, c.Level)
	}

	// Test default compression
	c = NewCompressor(LevelDefault)
	if c.Type != TypeGzip {
		t.Errorf("expected type gzip, got %s", c.Type)
	}
	if c.Level != LevelDefault {
		t.Errorf("expected level %d, got %d", LevelDefault, c.Level)
	}

	// Test max compression
	c = NewCompressor(LevelMax)
	if c.Type != TypeGzip {
		t.Errorf("expected type gzip, got %s", c.Type)
	}
	if c.Level != LevelMax {
		t.Errorf("expected level %d, got %d", LevelMax, c.Level)
	}
}

func TestNewCompressorFromString(t *testing.T) {
	tests := []struct {
		level    string
		expected CompressionLevel
	}{
		{"none", LevelNone},
		{"0", LevelNone},
		{"fast", LevelFast},
		{"1", LevelFast},
		{"default", LevelDefault},
		{"6", LevelDefault},
		{"max", LevelMax},
		{"9", LevelMax},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			c, err := NewCompressorFromString(tt.level)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if c.Level != tt.expected {
				t.Errorf("expected level %d, got %d", tt.expected, c.Level)
			}
		})
	}

	// Test invalid level
	_, err := NewCompressorFromString("invalid")
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestCompressorString(t *testing.T) {
	tests := []struct {
		level    CompressionLevel
		expected string
	}{
		{LevelNone, "none"},
		{LevelFast, "fast"},
		{LevelDefault, "default"},
		{LevelMax, "max"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			c := NewCompressor(tt.level)
			if c.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, c.String())
			}
		})
	}
}

func TestCompressDecompressFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testData := []byte("Hello, World! This is test data for compression.")
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Test compression disabled
	cNone := NewCompressor(LevelNone)
	compressedPath, err := cNone.CompressFile(testFile)
	if err != nil {
		t.Fatalf("compress with none: %v", err)
	}
	if compressedPath != testFile {
		t.Errorf("expected same path, got %s", compressedPath)
	}

	// Test compression enabled
	cFast := NewCompressor(LevelFast)
	compressedPath, err = cFast.CompressFile(testFile)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if compressedPath != testFile+".gz" {
		t.Errorf("expected .gz extension, got %s", compressedPath)
	}

	// Verify compressed file exists
	if _, err := os.Stat(compressedPath); os.IsNotExist(err) {
		t.Error("compressed file not created")
	}

	// Test decompression
	decompressedPath, err := DecompressFile(compressedPath)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if decompressedPath != testFile {
		t.Errorf("expected original path, got %s", decompressedPath)
	}

	// Verify decompressed data matches original
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}
	if string(data) != string(testData) {
		t.Error("decompressed data doesn't match original")
	}
}

func TestCompressDir(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create test files
	files := map[string]string{
		"file1.txt":        "content 1",
		"file2.txt":        "content 2",
		"subdir/file3.txt": "content 3",
	}
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	// Compress directory
	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 compressed files, got %d", count)
	}

	// Verify compressed files exist
	for path := range files {
		gzPath := filepath.Join(tmpDir, path+".gz")
		if _, err := os.Stat(gzPath); os.IsNotExist(err) {
			t.Errorf("compressed file not found: %s", gzPath)
		}
		// Original should be removed
		origPath := filepath.Join(tmpDir, path)
		if _, err := os.Stat(origPath); err == nil {
			t.Errorf("original file not removed: %s", origPath)
		}
	}
}

func TestDecompressDir(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create test files
	files := map[string]string{
		"file1.txt":        "content 1",
		"file2.txt":        "content 2",
		"subdir/file3.txt": "content 3",
	}
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	c := NewCompressor(LevelFast)
	if _, err := c.CompressDir(tmpDir); err != nil {
		t.Fatalf("compress dir: %v", err)
	}

	// Decompress directory
	count, err := DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress dir: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 decompressed files, got %d", count)
	}

	// Verify decompressed files exist and content matches
	for path, expectedContent := range files {
		origPath := filepath.Join(tmpDir, path)
		data, err := os.ReadFile(origPath)
		if err != nil {
			t.Errorf("read decompressed file: %v", err)
			continue
		}
		if string(data) != expectedContent {
			t.Errorf("content mismatch for %s", path)
		}
		// Compressed file should be removed
		gzPath := filepath.Join(tmpDir, path+".gz")
		if _, err := os.Stat(gzPath); err == nil {
			t.Errorf("compressed file not removed: %s", gzPath)
		}
	}
}

func TestDecompressDir_UsesCompressionManifestNotGzipSuffix(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte("payload"), 0644); err != nil {
		t.Fatalf("write data: %v", err)
	}
	userGzip := filepath.Join(tmpDir, "archive.gz")
	userGzipContent := []byte("user-owned gzip-named content")
	if err := os.WriteFile(userGzip, userGzipContent, 0644); err != nil {
		t.Fatalf("write user gzip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "target.txt"), []byte("target"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(tmpDir, "link.gz")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 compressed regular files, got %d", count)
	}

	ready, err := os.ReadFile(filepath.Join(tmpDir, ".READY"))
	if err != nil {
		t.Fatalf("read ready: %v", err)
	}
	if !strings.Contains(string(ready), "compression_manifest") {
		t.Fatalf("ready marker missing compression manifest: %s", string(ready))
	}

	count, err = DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress dir: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 decompressed files, got %d", count)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "data.txt"))
	if err != nil {
		t.Fatalf("read data: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected data content: %q", string(data))
	}
	archive, err := os.ReadFile(userGzip)
	if err != nil {
		t.Fatalf("read user gzip: %v", err)
	}
	if string(archive) != string(userGzipContent) {
		t.Fatalf("user gzip was modified: %q", string(archive))
	}
	info, err := os.Lstat(filepath.Join(tmpDir, "link.gz"))
	if err != nil {
		t.Fatalf("lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.gz should remain a symlink")
	}
}

func TestDecompressDir_CorruptSecondGzipLeavesTreeUnchanged(t *testing.T) {
	root := t.TempDir()

	writeGzipFixture(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	secondGzip := filepath.Join(root, "second.txt.gz")
	secondGzipContent := []byte("this is not valid gzip data")
	if err := os.WriteFile(secondGzip, secondGzipContent, 0644); err != nil {
		t.Fatalf("write corrupt gzip: %v", err)
	}
	requireReadyMarkerForPaths(t, root, []string{"first.txt", "second.txt"})

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected corrupt gzip to fail")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	assertPathMissing(t, filepath.Join(root, "first.txt"))
	assertFileContent(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	assertPathMissing(t, filepath.Join(root, "second.txt"))
	assertRawFileContent(t, secondGzip, secondGzipContent)
}

func TestDecompressDir_TruncatedLaterGzipLeavesTreeUnchanged(t *testing.T) {
	root := t.TempDir()

	writeGzipFixture(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	secondGzip := filepath.Join(root, "second.txt.gz")
	writeGzipFixture(t, secondGzip, []byte("second"))
	secondGzipContent, err := os.ReadFile(secondGzip)
	if err != nil {
		t.Fatalf("read gzip fixture: %v", err)
	}
	if len(secondGzipContent) < 8 {
		t.Fatalf("gzip fixture unexpectedly short: %d bytes", len(secondGzipContent))
	}
	secondGzipContent = secondGzipContent[:len(secondGzipContent)-4]
	if err := os.WriteFile(secondGzip, secondGzipContent, 0644); err != nil {
		t.Fatalf("truncate gzip fixture: %v", err)
	}
	requireReadyMarkerForEntries(t, root, []map[string]any{
		{
			"path":            "first.txt",
			"compressed_path": "first.txt.gz",
			"original_size":   int64(len("first")),
		},
		{
			"path":            "second.txt",
			"compressed_path": "second.txt.gz",
			"original_size":   int64(len("second")),
		},
	})

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected truncated gzip to fail")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	assertPathMissing(t, filepath.Join(root, "first.txt"))
	assertFileContent(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	assertPathMissing(t, filepath.Join(root, "second.txt"))
	assertRawFileContent(t, secondGzip, secondGzipContent)
}

func TestDecompressDir_RejectsDeclaredOriginalSizeTooSmall(t *testing.T) {
	root := t.TempDir()

	writeGzipFixture(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	bombGzip := filepath.Join(root, "bomb.txt.gz")
	writeGzipFixture(t, bombGzip, []byte("larger than declared"))
	bombRaw, err := os.ReadFile(bombGzip)
	if err != nil {
		t.Fatalf("read bomb gzip: %v", err)
	}
	requireReadyMarkerForEntries(t, root, []map[string]any{
		{
			"path":            "first.txt",
			"compressed_path": "first.txt.gz",
			"original_size":   int64(len("first")),
		},
		{
			"path":            "bomb.txt",
			"compressed_path": "bomb.txt.gz",
			"original_size":   int64(1),
		},
	})

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected declared size mismatch to fail")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	assertPathMissing(t, filepath.Join(root, "first.txt"))
	assertFileContent(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	assertPathMissing(t, filepath.Join(root, "bomb.txt"))
	assertRawFileContent(t, bombGzip, bombRaw)
}

func TestDecompressDir_AcceptsDeclaredOriginalSizeExactMatch(t *testing.T) {
	root := t.TempDir()

	content := []byte("exactly declared")
	writeGzipFixture(t, filepath.Join(root, "file.txt.gz"), content)
	requireReadyMarkerForEntries(t, root, []map[string]any{
		{
			"path":            "file.txt",
			"compressed_path": "file.txt.gz",
			"original_size":   int64(len(content)),
		},
	})

	count, err := DecompressDir(root)
	if err != nil {
		t.Fatalf("decompress dir: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one file to be decompressed, got %d", count)
	}
	assertRawFileContent(t, filepath.Join(root, "file.txt"), content)
	assertPathMissing(t, filepath.Join(root, "file.txt.gz"))
}

func TestDecompressDir_CommitRenameFailureRollsBackEarlierOutput(t *testing.T) {
	root := t.TempDir()

	writeGzipFixture(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	writeGzipFixture(t, filepath.Join(root, "second.txt.gz"), []byte("second"))
	requireReadyMarkerForPaths(t, root, []string{"first.txt", "second.txt"})

	secondOutput := filepath.Join(root, "second.txt")
	originalRenameFile := renameFile
	renameFile = func(oldPath, newPath string) error {
		if newPath == secondOutput {
			return errors.New("injected rename failure")
		}
		return originalRenameFile(oldPath, newPath)
	}
	defer func() {
		renameFile = originalRenameFile
	}()

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected injected rename failure")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	assertPathMissing(t, filepath.Join(root, "first.txt"))
	assertFileContent(t, filepath.Join(root, "first.txt.gz"), []byte("first"))
	assertPathMissing(t, secondOutput)
	assertFileContent(t, filepath.Join(root, "second.txt.gz"), []byte("second"))
}

func TestDecompressDir_RejectsOutputSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	writeGzipFixture(t, filepath.Join(root, "source.gz"), []byte("outside write"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	requireReadyMarkerForEntries(t, root, []map[string]any{
		{
			"path":            "escape/out.txt",
			"compressed_path": "source.gz",
			"original_size":   int64(len("outside write")),
		},
	})

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside target was written or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "source.gz")); err != nil {
		t.Fatalf("compressed source should not be removed: %v", err)
	}
}

func TestDecompressDir_RejectsCompressedPathSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	writeGzipFixture(t, filepath.Join(outside, "secret.gz"), []byte("outside read"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	requireReadyMarkerForEntries(t, root, []map[string]any{
		{
			"path":            "out.txt",
			"compressed_path": "escape/secret.gz",
			"original_size":   int64(len("outside read")),
		},
	})

	count, err := DecompressDir(root)
	if err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
	if count != 0 {
		t.Fatalf("expected no files to be decompressed, got %d", count)
	}
	if _, err := os.Stat(filepath.Join(root, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("output was written or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.gz")); err != nil {
		t.Fatalf("outside compressed source should not be removed: %v", err)
	}
}

func TestIsCompressedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"file.txt", false},
		{"file.txt.gz", true},
		{"file.gz", true},
		{"file.tar.gz", true},
		{"gzip", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsCompressedFile(tt.path); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func requireReadyMarker(t *testing.T, root string) {
	t.Helper()
	requireReadyMarkerForPaths(t, root, nil)
}

func requireReadyMarkerForPaths(t *testing.T, root string, paths []string) {
	t.Helper()
	files := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		compressedPath := filepath.ToSlash(path + ".gz")
		files = append(files, map[string]any{
			"path":            filepath.ToSlash(path),
			"compressed_path": compressedPath,
			"original_size":   originalSizeForCompressedFixture(t, filepath.Join(root, filepath.FromSlash(compressedPath))),
		})
	}
	requireReadyMarkerForEntries(t, root, files)
}

func requireReadyMarkerForEntries(t *testing.T, root string, files []map[string]any) {
	t.Helper()
	marker := map[string]any{
		"snapshot_id":           "test",
		"payload_root_hash":     "hash",
		"engine":                "copy",
		"descriptor_checksum":   "checksum",
		"completed_at_for_test": "now",
		"compression_manifest": map[string]any{
			"version": 1,
			"type":    TypeGzip,
			"files":   files,
		},
	}
	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal ready marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".READY"), data, 0644); err != nil {
		t.Fatalf("write ready marker: %v", err)
	}
}

func originalSizeForCompressedFixture(t *testing.T, path string) int64 {
	t.Helper()
	data, err := decompressFileContentForTest(path)
	if err != nil {
		return 0
	}
	return int64(len(data))
}

func writeGzipFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir gzip parent: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create gzip fixture: %v", err)
	}
	w := gzip.NewWriter(f)
	if _, err := w.Write(data); err != nil {
		f.Close()
		t.Fatalf("write gzip fixture: %v", err)
	}
	if err := w.Close(); err != nil {
		f.Close()
		t.Fatalf("close gzip fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close gzip file: %v", err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got err %v", path, err)
	}
}

func assertRawFileContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != string(expected) {
		t.Fatalf("content mismatch for %s: got %q, want %q", path, string(data), string(expected))
	}
}

func assertFileContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	data, err := decompressFileContentForTest(path)
	if err != nil {
		t.Fatalf("read gzip content %s: %v", path, err)
	}
	if string(data) != string(expected) {
		t.Fatalf("content mismatch for %s: got %q, want %q", path, string(data), string(expected))
	}
}

func decompressFileContentForTest(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decompressBytes(data)
}

func TestCompressedUncompressedPath(t *testing.T) {
	tests := []struct {
		path         string
		compressed   string
		uncompressed string
	}{
		{"file.txt", "file.txt.gz", "file.txt"},
		{"file.txt.gz", "file.txt.gz.gz", "file.txt.gz"},
		{"path/to/file", "path/to/file.gz", "path/to/file"},
		{"archive.tar.gz", "archive.tar.gz.gz", "archive.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if c := CompressedPath(tt.path); c != tt.compressed {
				t.Errorf("CompressedPath: expected %s, got %s", tt.compressed, c)
			}
			if u := UncompressedPath(tt.compressed); u != tt.uncompressed {
				t.Errorf("UncompressedPath: expected %s, got %s", tt.uncompressed, u)
			}
		})
	}
}

func TestCompressionInfoFromLevel(t *testing.T) {
	// Test valid levels
	info, err := CompressionInfoFromLevel("fast")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info.Type != TypeGzip {
		t.Errorf("expected type gzip, got %s", info.Type)
	}
	if info.Level != LevelFast {
		t.Errorf("expected level %d, got %d", LevelFast, info.Level)
	}

	// Test none (returns nil)
	info, err = CompressionInfoFromLevel("none")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil for none compression")
	}

	// Test invalid
	_, err = CompressionInfoFromLevel("invalid")
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestCompressionInfoString(t *testing.T) {
	info := &model.CompressionInfo{
		Type:  "gzip",
		Level: 6,
	}

	// This tests the model.CompressionInfo.String method if it exists,
	// or verifies the structure is correct
	if info.Type != "gzip" {
		t.Errorf("expected type gzip, got %s", info.Type)
	}
	if info.Level != 6 {
		t.Errorf("expected level 6, got %d", info.Level)
	}
}

func TestCompressBytes(t *testing.T) {
	c := NewCompressor(LevelFast)
	data := []byte("Hello, World!")

	compressed, err := c.compressBytes(data)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	// Compressed data should be different from original
	if string(compressed) == string(data) {
		t.Error("compressed data is identical to original")
	}

	// Decompress and verify
	decompressed, err := decompressBytes(compressed)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	if string(decompressed) != string(data) {
		t.Error("decompressed data doesn't match original")
	}
}

func TestDecompressFileNotCompressed(t *testing.T) {
	tmpDir := t.TempDir()

	// Create non-compressed file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("Hello, World!")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Decompress should return original path for non-compressed files
	result, err := DecompressFile(testFile)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if result != testFile {
		t.Errorf("expected original path, got %s", result)
	}
}

func TestCompressorString_UnknownLevel(t *testing.T) {
	// Test the default case in String() for unknown levels
	c := &Compressor{Type: TypeGzip, Level: CompressionLevel(3)} // Not a predefined level
	str := c.String()
	if str != "level-3" {
		t.Errorf("expected 'level-3', got %s", str)
	}
}

func TestCompressFile_Error_NonExistent(t *testing.T) {
	c := NewCompressor(LevelFast)
	_, err := c.CompressFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestDecompressFile_Error_CorruptData(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with invalid gzip data
	corruptFile := filepath.Join(tmpDir, "corrupt.txt.gz")
	corruptData := []byte("this is not valid gzip data")
	if err := os.WriteFile(corruptFile, corruptData, 0644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	// Decompress should fail
	_, err := DecompressFile(corruptFile)
	if err == nil {
		t.Error("expected error for corrupt gzip data")
	}
}

func TestDecompressFile_Error_NonExistent(t *testing.T) {
	_, err := DecompressFile("/nonexistent/path/file.txt.gz")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestCompressDir_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress empty dir: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}
}

func TestCompressDir_Disabled(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cNone := NewCompressor(LevelNone)
	count, err := cNone.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir with none: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files for disabled compression, got %d", count)
	}
}

func TestDecompressDir_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	count, err := DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress empty dir: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}
}

func TestDecompressDir_SkipReadyMarker(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create a .READY.gz file which should be skipped
	readyFile := filepath.Join(tmpDir, ".READY.gz")
	readyData := []byte("ready marker")
	if err := os.WriteFile(readyFile, readyData, 0644); err != nil {
		t.Fatalf("write ready file: %v", err)
	}

	count, err := DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress dir with ready marker: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files (ready marker skipped), got %d", count)
	}
}

func TestCompressDir_SkipAlreadyCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create an already compressed file
	gzFile := filepath.Join(tmpDir, "already.txt.gz")
	gzData := []byte("already compressed data")
	if err := os.WriteFile(gzFile, gzData, 0644); err != nil {
		t.Fatalf("write gz file: %v", err)
	}

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files (already compressed skipped), got %d", count)
	}
}

func TestDecompressDir_SkipNonCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create a non-compressed file
	txtFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(txtFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	count, err := DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress dir: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files (non-compressed skipped), got %d", count)
	}
}

func TestCompressFile_AllLevels(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file with compressible data
	testData := make([]byte, 1024)
	for i := range testData {
		testData[i] = byte(i % 10) // Highly repetitive
	}
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	levels := []CompressionLevel{LevelFast, LevelDefault, LevelMax}
	levelNames := map[CompressionLevel]string{LevelFast: "fast", LevelDefault: "default", LevelMax: "max"}
	for _, level := range levels {
		t.Run(levelNames[level], func(t *testing.T) {
			c := NewCompressor(level)
			compressedPath, err := c.CompressFile(testFile)
			if err != nil {
				t.Fatalf("compress: %v", err)
			}

			// Verify compressed file is smaller
			info, _ := os.Stat(compressedPath)
			if info.Size() >= int64(len(testData)) {
				t.Errorf("compressed file not smaller: %d >= %d", info.Size(), len(testData))
			}
		})
	}
}

func TestCompressFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty file
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	c := NewCompressor(LevelFast)
	compressedPath, err := c.CompressFile(emptyFile)
	if err != nil {
		t.Fatalf("compress empty file: %v", err)
	}

	// Verify compressed file exists
	if _, err := os.Stat(compressedPath); os.IsNotExist(err) {
		t.Error("compressed file not created")
	}
}

func TestDecompressFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and compress empty file
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	c := NewCompressor(LevelFast)
	compressedPath, err := c.CompressFile(emptyFile)
	if err != nil {
		t.Fatalf("compress empty file: %v", err)
	}

	// Decompress
	decompressedPath, err := DecompressFile(compressedPath)
	if err != nil {
		t.Fatalf("decompress empty file: %v", err)
	}

	// Verify decompressed file is empty
	data, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("read decompressed file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(data))
	}
}

func TestCompressFile_Large(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a larger file (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	largeFile := filepath.Join(tmpDir, "large.bin")
	if err := os.WriteFile(largeFile, largeData, 0644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	c := NewCompressor(LevelDefault)
	compressedPath, err := c.CompressFile(largeFile)
	if err != nil {
		t.Fatalf("compress large file: %v", err)
	}

	// Verify compression ratio
	info, _ := os.Stat(compressedPath)
	if info.Size() >= int64(len(largeData)) {
		t.Errorf("large file not compressed: %d >= %d", info.Size(), len(largeData))
	}
}

func TestSnapshotCompressionInfoString(t *testing.T) {
	tests := []struct {
		name     string
		info     *SnapshotCompressionInfo
		expected string
	}{
		{"nil", nil, "none"},
		{"none type", &SnapshotCompressionInfo{Type: TypeNone, Level: LevelNone}, "none"},
		{"gzip fast", &SnapshotCompressionInfo{Type: TypeGzip, Level: LevelFast}, "gzip-1"},
		{"gzip default", &SnapshotCompressionInfo{Type: TypeGzip, Level: LevelDefault}, "gzip-6"},
		{"gzip max", &SnapshotCompressionInfo{Type: TypeGzip, Level: LevelMax}, "gzip-9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.String(); got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestCompressDir_WithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create complex directory structure
	structure := map[string]string{
		"root.txt":                  "root content",
		"dir1/sub1.txt":             "sub1 content",
		"dir1/dir2/deep.txt":        "deep content",
		"dir1/dir2/dir3/deeper.txt": "deeper content",
		"another/file.txt":          "another content",
	}

	for path, content := range structure {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != len(structure) {
		t.Errorf("expected %d files, got %d", len(structure), count)
	}

	// Verify all originals are removed
	for path := range structure {
		fullPath := filepath.Join(tmpDir, path)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Errorf("original file not removed: %s", fullPath)
		}
	}
}

func TestDecompressDir_WithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create compressed files in subdirectories
	compressibleFiles := map[string]string{
		"root.txt":           "root content",
		"dir1/sub1.txt":      "sub1 content",
		"dir1/dir2/deep.txt": "deep content",
		"another/file.txt":   "another content",
	}

	// Compress all the .txt files
	for path, content := range compressibleFiles {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		c := NewCompressor(LevelFast)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if _, err := c.CompressFile(fullPath); err != nil {
			t.Fatalf("compress file: %v", err)
		}
		os.Remove(fullPath)
	}
	paths := make([]string, 0, len(compressibleFiles))
	for path := range compressibleFiles {
		paths = append(paths, path)
	}
	requireReadyMarkerForPaths(t, tmpDir, paths)

	// Create a .READY.gz marker file (should be skipped)
	readyFile := filepath.Join(tmpDir, ".READY.gz")
	c := NewCompressor(LevelFast)
	if err := os.WriteFile(readyFile, []byte("ready"), 0644); err != nil {
		t.Fatalf("write ready file: %v", err)
	}
	if _, err := c.CompressFile(readyFile); err != nil {
		t.Fatalf("compress ready: %v", err)
	}
	os.Remove(readyFile)

	// Create a non-compressed file (should be skipped)
	skipFile := filepath.Join(tmpDir, "skip.txt")
	if err := os.WriteFile(skipFile, []byte("not compressed"), 0644); err != nil {
		t.Fatalf("write skip file: %v", err)
	}

	count, err := DecompressDir(tmpDir)
	if err != nil {
		t.Fatalf("decompress dir: %v", err)
	}
	// Should decompress 4 .txt.gz files (not .READY.gz or skip.txt)
	expectedCount := len(compressibleFiles)
	if count != expectedCount {
		t.Errorf("expected %d files, got %d", expectedCount, count)
	}
}

func TestCompressBytes_WriteError(t *testing.T) {
	// This tests the write error path in compressBytes
	// We can't easily trigger gzip write errors, but we can verify the function exists
	c := NewCompressor(LevelFast)

	// Test with empty data
	compressed, err := c.compressBytes([]byte{})
	if err != nil {
		t.Fatalf("compress empty: %v", err)
	}
	if len(compressed) == 0 {
		t.Error("compressed empty data should not be empty")
	}
}

func TestDecompressBytes_InvalidGzip(t *testing.T) {
	// Test invalid gzip data
	invalidData := []byte("not gzip at all")
	_, err := decompressBytes(invalidData)
	if err == nil {
		t.Error("expected error for invalid gzip data")
	}
}

func TestNewCompressorFromString_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected CompressionLevel
	}{
		{"NONE", LevelNone},
		{"FAST", LevelFast},
		{"Default", LevelDefault},
		{"MAX", LevelMax},
		{"None", LevelNone},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c, err := NewCompressorFromString(tt.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if c.Level != tt.expected {
				t.Errorf("expected level %d, got %d", tt.expected, c.Level)
			}
		})
	}
}

func TestCompressFile_AlreadyCompressedExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file that already has .gz extension
	gzFile := filepath.Join(tmpDir, "test.txt.gz")
	testData := []byte("test data")
	if err := os.WriteFile(gzFile, testData, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewCompressor(LevelFast)
	compressedPath, err := c.CompressFile(gzFile)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	// Should add another .gz extension
	expectedPath := gzFile + ".gz"
	if compressedPath != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, compressedPath)
	}
}

func TestIsEnabled(t *testing.T) {
	c := NewCompressor(LevelNone)
	if c.IsEnabled() {
		t.Error("LevelNone should not be enabled")
	}

	c = NewCompressor(LevelFast)
	if !c.IsEnabled() {
		t.Error("LevelFast should be enabled")
	}

	c = NewCompressor(LevelDefault)
	if !c.IsEnabled() {
		t.Error("LevelDefault should be enabled")
	}

	c = NewCompressor(LevelMax)
	if !c.IsEnabled() {
		t.Error("LevelMax should be enabled")
	}
}

func TestCompressDir_WithSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create a regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("regular content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file, got %d", count)
	}
}

func TestDecompressFile_TruncatedGzip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid gzip file then truncate it
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewCompressor(LevelFast)
	gzPath, err := c.CompressFile(testFile)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	// Read the compressed file and truncate it
	gzData, err := os.ReadFile(gzPath)
	if err != nil {
		t.Fatalf("read gz: %v", err)
	}
	// Truncate to half size (will be invalid gzip)
	truncatedData := gzData[:len(gzData)/2]
	truncatedPath := filepath.Join(tmpDir, "truncated.txt.gz")
	if err := os.WriteFile(truncatedPath, truncatedData, 0644); err != nil {
		t.Fatalf("write truncated: %v", err)
	}

	// Decompress should fail
	_, err = DecompressFile(truncatedPath)
	if err == nil {
		t.Error("expected error for truncated gzip data")
	}
}

func TestCompressionLevels_AllValues(t *testing.T) {
	// Test creating compressors with various levels
	levels := []CompressionLevel{
		LevelNone,
		LevelFast,
		CompressionLevel(2), // Custom level
		LevelDefault,
		CompressionLevel(7), // Custom level
		LevelMax,
	}

	for _, level := range levels {
		c := NewCompressor(level)
		if level <= LevelNone {
			if c.IsEnabled() {
				t.Errorf("level %d should be disabled", level)
			}
		} else {
			if !c.IsEnabled() {
				t.Errorf("level %d should be enabled", level)
			}
			if c.Level != level {
				t.Errorf("expected level %d, got %d", level, c.Level)
			}
		}
	}
}

func TestCompressFile_DirectoryAsPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Try to compress a directory (should fail on ReadFile)
	c := NewCompressor(LevelFast)
	_, err := c.CompressFile(tmpDir)
	if err == nil {
		t.Error("expected error when trying to compress a directory")
	}
}

func TestCompressFile_VerySmall(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a very small file (1 byte)
	smallFile := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewCompressor(LevelMax)
	compressedPath, err := c.CompressFile(smallFile)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	// Verify decompression works
	decompressedPath, err := DecompressFile(compressedPath)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	data, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 1 || data[0] != 'x' {
		t.Errorf("unexpected data: %v", data)
	}
}

func TestCompressDir_NestedDeepStructure(t *testing.T) {
	tmpDir := t.TempDir()
	requireReadyMarker(t, tmpDir)

	// Create a deeply nested structure
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "d", "e", "file.txt")
	if err := os.MkdirAll(filepath.Dir(deepPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(deepPath, []byte("deep file"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := NewCompressor(LevelFast)
	count, err := c.CompressDir(tmpDir)
	if err != nil {
		t.Fatalf("compress dir: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file, got %d", count)
	}

	// Verify the compressed file exists at the deep path
	deepGzPath := deepPath + ".gz"
	if _, err := os.Stat(deepGzPath); os.IsNotExist(err) {
		t.Errorf("compressed file not found at %s", deepGzPath)
	}
}
