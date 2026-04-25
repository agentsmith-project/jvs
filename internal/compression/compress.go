// Package compression provides compression support for JVS snapshots.
// It supports gzip compression at configurable levels for snapshot data.
package compression

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

// CompressionLevel represents the compression level.
type CompressionLevel int

const (
	// LevelNone disables compression.
	LevelNone CompressionLevel = 0
	// LevelFast uses fastest compression (gzip level 1).
	LevelFast CompressionLevel = 1
	// LevelDefault uses default compression (gzip level 6).
	LevelDefault CompressionLevel = 6
	// LevelMax uses maximum compression (gzip level 9).
	LevelMax CompressionLevel = 9
)

// CompressionType represents the compression algorithm.
type CompressionType string

const (
	// TypeGzip uses gzip compression.
	TypeGzip CompressionType = "gzip"
	// TypeNone indicates no compression.
	TypeNone CompressionType = "none"
)

const (
	readyMarkerName             = ".READY"
	compressionManifestJSONName = "compression_manifest"
	compressionManifestVersion  = 1
)

var renameFile = os.Rename

func int64Ptr(v int64) *int64 {
	return &v
}

type compressionManifest struct {
	Version int                       `json:"version"`
	Type    CompressionType           `json:"type"`
	Files   []compressionManifestFile `json:"files"`
}

type compressionManifestFile struct {
	Path           string `json:"path"`
	CompressedPath string `json:"compressed_path"`
	OriginalSize   *int64 `json:"original_size"`
}

// Compressor handles compression operations.
type Compressor struct {
	Type  CompressionType
	Level CompressionLevel
}

// NewCompressor creates a new compressor with the specified level.
// Level 0 means no compression.
func NewCompressor(level CompressionLevel) *Compressor {
	if level <= LevelNone {
		return &Compressor{Type: TypeNone, Level: LevelNone}
	}
	return &Compressor{Type: TypeGzip, Level: level}
}

// NewCompressorFromString creates a compressor from a string level.
// Valid values: "none", "fast", "default", "max"
func NewCompressorFromString(level string) (*Compressor, error) {
	switch strings.ToLower(level) {
	case "none", "0":
		return NewCompressor(LevelNone), nil
	case "fast", "1":
		return NewCompressor(LevelFast), nil
	case "default", "6":
		return NewCompressor(LevelDefault), nil
	case "max", "9":
		return NewCompressor(LevelMax), nil
	default:
		return nil, fmt.Errorf("invalid compression level: %s (must be none, fast, default, or max)", level)
	}
}

// IsEnabled returns true if compression is enabled.
func (c *Compressor) IsEnabled() bool {
	return c.Type != TypeNone
}

// String returns the string representation of the compressor.
func (c *Compressor) String() string {
	switch c.Level {
	case LevelNone:
		return "none"
	case LevelFast:
		return "fast"
	case LevelDefault:
		return "default"
	case LevelMax:
		return "max"
	default:
		return fmt.Sprintf("level-%d", c.Level)
	}
}

// CompressFile compresses a file and returns the compressed path.
// The compressed file has a .gz extension added.
// If compression is disabled, returns the original path.
func (c *Compressor) CompressFile(path string) (string, error) {
	if !c.IsEnabled() {
		return path, nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refuse to compress symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refuse to compress non-regular file: %s", path)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0600
	}

	// Read original file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Compress
	compressed, err := c.compressBytes(data)
	if err != nil {
		return "", fmt.Errorf("compress: %w", err)
	}

	// Write compressed file
	compressedPath := path + ".gz"
	if err := os.WriteFile(compressedPath, compressed, mode); err != nil {
		return "", fmt.Errorf("write compressed file: %w", err)
	}
	if err := os.Chmod(compressedPath, mode); err != nil {
		return "", fmt.Errorf("chmod compressed file: %w", err)
	}

	return compressedPath, nil
}

// DecompressFile decompresses a .gz file and returns the decompressed path.
// If the file is not compressed, returns the original path.
func DecompressFile(path string) (string, error) {
	// Check if file is compressed
	if !strings.HasSuffix(path, ".gz") {
		return path, nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat compressed file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refuse to decompress symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refuse to decompress non-regular file: %s", path)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0600
	}

	// Read compressed file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read compressed file: %w", err)
	}

	// Decompress
	decompressed, err := decompressBytes(data)
	if err != nil {
		return "", fmt.Errorf("decompress: %w", err)
	}

	// Write decompressed file (remove .gz extension)
	decompressedPath := strings.TrimSuffix(path, ".gz")
	if err := os.WriteFile(decompressedPath, decompressed, mode); err != nil {
		return "", fmt.Errorf("write decompressed file: %w", err)
	}
	if err := os.Chmod(decompressedPath, mode); err != nil {
		return "", fmt.Errorf("chmod decompressed file: %w", err)
	}

	return decompressedPath, nil
}

// CompressDir compresses eligible regular files in a snapshot directory tree
// and records the compressed paths in the root .READY marker.
// Returns the count of compressed files and any error.
func (c *Compressor) CompressDir(root string) (int, error) {
	if !c.IsEnabled() {
		return 0, nil
	}

	if _, err := os.Lstat(filepath.Join(root, readyMarkerName)); err != nil {
		return 0, fmt.Errorf("read ready marker: %w", err)
	}

	count := 0
	manifest := compressionManifest{
		Version: compressionManifestVersion,
		Type:    c.Type,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)

		// Skip directories, symlinks, the root control marker, already-gzip
		// user files, and non-regular filesystem entries.
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || rel == readyMarkerName || strings.HasSuffix(path, ".gz") || !info.Mode().IsRegular() {
			return nil
		}

		compressedPath := path + ".gz"
		if _, err := os.Lstat(compressedPath); err == nil {
			// Do not overwrite a user-owned sibling such as "file.gz".
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check compressed path %s: %w", compressedPath, err)
		}

		// Compress file
		_, err = c.CompressFile(path)
		if err != nil {
			return fmt.Errorf("compress %s: %w", path, err)
		}

		// Remove original file after successful compression
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove original %s: %w", path, err)
		}

		count++
		manifest.Files = append(manifest.Files, compressionManifestFile{
			Path:           rel,
			CompressedPath: rel + ".gz",
			OriginalSize:   int64Ptr(info.Size()),
		})
		return nil
	})
	if err != nil {
		return count, err
	}

	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})
	if err := writeCompressionManifest(root, manifest); err != nil {
		return count, err
	}

	return count, nil
}

// DecompressDir decompresses only files listed in the root .READY compression
// manifest. User-owned .gz files and symlinks are left untouched.
// Returns the count of decompressed files and any error.
func DecompressDir(root string) (int, error) {
	if err := validateManifestRoot(root); err != nil {
		return 0, err
	}

	manifest, err := readCompressionManifest(root)
	if err != nil {
		return 0, err
	}

	plan, err := planManifestDecompression(root, manifest)
	if err != nil {
		return 0, err
	}

	if len(plan) == 0 {
		return 0, nil
	}

	stagingDir, err := makeDecompressionSiblingTempDir(root, "staging")
	if err != nil {
		return 0, fmt.Errorf("create decompression staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	staged, err := stageManifestDecompression(plan, stagingDir)
	if err != nil {
		return 0, err
	}

	trashDir, err := makeDecompressionSiblingTempDir(root, "trash")
	if err != nil {
		return 0, fmt.Errorf("create decompression trash dir: %w", err)
	}

	if err := commitStagedDecompression(staged, trashDir); err != nil {
		return 0, err
	}

	_ = os.RemoveAll(trashDir)
	return len(plan), nil
}

func writeCompressionManifest(root string, manifest compressionManifest) error {
	readyPath := filepath.Join(root, readyMarkerName)
	info, err := os.Lstat(readyPath)
	if err != nil {
		return fmt.Errorf("stat ready marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("ready marker is not a regular file")
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0600
	}

	data, err := os.ReadFile(readyPath)
	if err != nil {
		return fmt.Errorf("read ready marker: %w", err)
	}
	var marker map[string]any
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &marker); err != nil {
			return fmt.Errorf("parse ready marker: %w", err)
		}
	}
	if marker == nil {
		marker = make(map[string]any)
	}
	marker[compressionManifestJSONName] = manifest

	data, err = json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ready marker: %w", err)
	}
	if err := os.WriteFile(readyPath, data, mode); err != nil {
		return fmt.Errorf("write ready marker: %w", err)
	}
	return os.Chmod(readyPath, mode)
}

func readCompressionManifest(root string) (*compressionManifest, error) {
	readyPath := filepath.Join(root, readyMarkerName)
	info, err := os.Lstat(readyPath)
	if err != nil {
		return nil, fmt.Errorf("read compression manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("read compression manifest: ready marker is not a regular file")
	}

	data, err := os.ReadFile(readyPath)
	if err != nil {
		return nil, fmt.Errorf("read compression manifest: %w", err)
	}
	var marker map[string]json.RawMessage
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, fmt.Errorf("parse ready marker: %w", err)
	}
	raw, ok := marker[compressionManifestJSONName]
	if !ok {
		return nil, fmt.Errorf("compression manifest missing")
	}

	var manifest compressionManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse compression manifest: %w", err)
	}
	if manifest.Version != compressionManifestVersion {
		return nil, fmt.Errorf("unsupported compression manifest version: %d", manifest.Version)
	}
	if manifest.Type != TypeGzip {
		return nil, fmt.Errorf("unsupported compression type: %s", manifest.Type)
	}
	return &manifest, nil
}

type decompressionPlan struct {
	compressedPath string
	outputPath     string
	mode           os.FileMode
	originalSize   int64
}

type stagedDecompression struct {
	decompressionPlan
	stagedPath string
	trashPath  string
}

type decompressionRollbackLedger struct {
	installedOutputs []string
	movedCompressed  []movedCompressedFile
}

type movedCompressedFile struct {
	trashPath      string
	compressedPath string
}

var errDecompressedSizeExceeded = errors.New("decompressed data exceeds manifest original size")

func planManifestDecompression(root string, manifest *compressionManifest) ([]decompressionPlan, error) {
	plan := make([]decompressionPlan, 0, len(manifest.Files))
	seenOutputs := make(map[string]struct{}, len(manifest.Files))
	seenCompressed := make(map[string]struct{}, len(manifest.Files))

	for _, file := range manifest.Files {
		rel, err := cleanManifestPath(file.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid manifest path %q: %w", file.Path, err)
		}

		compressedSource := file.CompressedPath
		if compressedSource == "" {
			compressedSource = filepath.ToSlash(rel) + ".gz"
		}
		compressedRel, err := cleanManifestPath(compressedSource)
		if err != nil {
			return nil, fmt.Errorf("invalid manifest compressed path %q: %w", file.CompressedPath, err)
		}

		if _, ok := seenOutputs[rel]; ok {
			return nil, fmt.Errorf("duplicate manifest path: %s", rel)
		}
		seenOutputs[rel] = struct{}{}
		if _, ok := seenCompressed[compressedRel]; ok {
			return nil, fmt.Errorf("duplicate manifest compressed path: %s", compressedRel)
		}
		seenCompressed[compressedRel] = struct{}{}

		if file.OriginalSize == nil {
			return nil, fmt.Errorf("manifest path %q missing original size", file.Path)
		}
		if *file.OriginalSize < 0 {
			return nil, fmt.Errorf("manifest path %q has negative original size: %d", file.Path, *file.OriginalSize)
		}

		if _, err := validateExistingParentDir(root, rel); err != nil {
			return nil, fmt.Errorf("invalid manifest path %q parent: %w", file.Path, err)
		}
		if _, err := validateExistingParentDir(root, compressedRel); err != nil {
			return nil, fmt.Errorf("invalid manifest compressed path %q parent: %w", file.CompressedPath, err)
		}

		outputPath := filepath.Join(root, rel)
		compressedPath := filepath.Join(root, compressedRel)
		info, err := os.Lstat(compressedPath)
		if err != nil {
			return nil, fmt.Errorf("stat compressed file: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refuse to decompress symlink")
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refuse to decompress non-regular file")
		}

		if _, err := os.Lstat(outputPath); err == nil {
			return nil, fmt.Errorf("decompressed target already exists: %s", outputPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat decompressed target: %w", err)
		}

		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0600
		}
		plan = append(plan, decompressionPlan{
			compressedPath: compressedPath,
			outputPath:     outputPath,
			mode:           mode,
			originalSize:   *file.OriginalSize,
		})
	}

	return plan, nil
}

func validateManifestRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat payload root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("payload root is a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("payload root is not a directory")
	}
	return nil
}

func cleanManifestPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	slashPath := filepath.ToSlash(path)
	if pathpkg.IsAbs(slashPath) {
		return "", fmt.Errorf("path escapes payload root")
	}
	localPath := filepath.FromSlash(slashPath)
	if filepath.IsAbs(localPath) || filepath.VolumeName(localPath) != "" {
		return "", fmt.Errorf("path escapes payload root")
	}

	parts := strings.Split(slashPath, "/")
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("empty path component")
		}
		if part == "." {
			return "", fmt.Errorf("dot path component")
		}
		if part == ".." {
			return "", fmt.Errorf("path escapes payload root")
		}
	}

	clean := pathpkg.Clean(slashPath)
	if clean == "." {
		return "", fmt.Errorf("path escapes payload root")
	}
	return filepath.FromSlash(clean), nil
}

func validateExistingParentDir(root, rel string) (string, error) {
	parentRel := filepath.Dir(rel)
	if parentRel == "." {
		return root, nil
	}

	current := root
	for _, component := range strings.Split(filepath.ToSlash(parentRel), "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("stat parent %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("parent directory is a symlink: %s", current)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("parent is not a directory: %s", current)
		}
	}
	return current, nil
}

func makeDecompressionSiblingTempDir(root, kind string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cleanRoot := filepath.Clean(absRoot)
	parent := filepath.Dir(cleanRoot)
	base := filepath.Base(cleanRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "payload"
	}
	return os.MkdirTemp(parent, ".jvs-"+base+"-decompress-"+kind+"-*")
}

func stageManifestDecompression(plan []decompressionPlan, stagingDir string) ([]stagedDecompression, error) {
	staged := make([]stagedDecompression, 0, len(plan))
	for i, file := range plan {
		stagedPath := filepath.Join(stagingDir, fmt.Sprintf("%06d", i))
		if err := decodeManifestFileToPath(file, stagedPath); err != nil {
			return nil, fmt.Errorf("decompress %s: %w", file.compressedPath, err)
		}
		staged = append(staged, stagedDecompression{
			decompressionPlan: file,
			stagedPath:        stagedPath,
		})
	}
	return staged, nil
}

func decodeManifestFileToPath(file decompressionPlan, stagedPath string) error {
	in, err := os.Open(file.compressedPath)
	if err != nil {
		return fmt.Errorf("read compressed file: %w", err)
	}
	defer in.Close()

	gzipReader, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}

	out, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, file.mode)
	if err != nil {
		gzipReader.Close()
		return fmt.Errorf("create staged decompressed file: %w", err)
	}

	removeStaged := true
	defer func() {
		if removeStaged {
			_ = os.Remove(stagedPath)
		}
	}()

	_, copyErr := copyExactDecompressedSize(out, gzipReader, file.originalSize)
	gzipCloseErr := gzipReader.Close()
	outCloseErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("decompress gzip stream: %w", copyErr)
	}
	if gzipCloseErr != nil {
		return fmt.Errorf("close gzip stream: %w", gzipCloseErr)
	}
	if outCloseErr != nil {
		return fmt.Errorf("write staged decompressed file: %w", outCloseErr)
	}
	if err := os.Chmod(stagedPath, file.mode); err != nil {
		return fmt.Errorf("chmod staged decompressed file: %w", err)
	}

	removeStaged = false
	return nil
}

type declaredSizeReader struct {
	r         io.Reader
	remaining int64
}

func (r *declaredSizeReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.r.Read(extra[:])
		if n > 0 {
			return 0, errDecompressedSizeExceeded
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func copyExactDecompressedSize(dst io.Writer, src io.Reader, expected int64) (int64, error) {
	if expected < 0 {
		return 0, fmt.Errorf("negative manifest original size: %d", expected)
	}

	reader := &declaredSizeReader{
		r:         src,
		remaining: expected,
	}
	buf := make([]byte, 32*1024)
	var total int64

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if total != expected {
					return total, fmt.Errorf("decompressed size mismatch: got %d, want %d", total, expected)
				}
				return total, nil
			}
			if errors.Is(readErr, errDecompressedSizeExceeded) {
				return total, fmt.Errorf("decompressed size exceeds manifest original size: got more than %d bytes", expected)
			}
			return total, readErr
		}
	}
}

func commitStagedDecompression(staged []stagedDecompression, trashDir string) error {
	var ledger decompressionRollbackLedger

	for i := range staged {
		file := &staged[i]
		file.trashPath = filepath.Join(trashDir, fmt.Sprintf("%06d.gz", i))

		if err := renameFile(file.stagedPath, file.outputPath); err != nil {
			return rollbackDecompressionCommit(&ledger, trashDir, fmt.Errorf("install decompressed file %s: %w", file.outputPath, err))
		}
		ledger.installedOutputs = append(ledger.installedOutputs, file.outputPath)

		if err := renameFile(file.compressedPath, file.trashPath); err != nil {
			return rollbackDecompressionCommit(&ledger, trashDir, fmt.Errorf("move compressed file %s: %w", file.compressedPath, err))
		}
		ledger.movedCompressed = append(ledger.movedCompressed, movedCompressedFile{
			trashPath:      file.trashPath,
			compressedPath: file.compressedPath,
		})
	}

	return nil
}

func rollbackDecompressionCommit(ledger *decompressionRollbackLedger, trashDir string, commitErr error) error {
	rollbackErr := ledger.rollback()
	if rollbackErr == nil {
		_ = os.RemoveAll(trashDir)
		return fmt.Errorf("commit decompression: %w", commitErr)
	}
	return fmt.Errorf("commit decompression: %w; rollback failed: %w", commitErr, rollbackErr)
}

func (ledger *decompressionRollbackLedger) rollback() error {
	var rollbackErrs []error

	for i := len(ledger.movedCompressed) - 1; i >= 0; i-- {
		file := ledger.movedCompressed[i]
		if err := renameFile(file.trashPath, file.compressedPath); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore compressed file %s: %w", file.compressedPath, err))
		}
	}

	for i := len(ledger.installedOutputs) - 1; i >= 0; i-- {
		outputPath := ledger.installedOutputs[i]
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("remove decompressed file %s: %w", outputPath, err))
		}
	}

	return errors.Join(rollbackErrs...)
}

// compressBytes compresses a byte slice using gzip.
func (c *Compressor) compressBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, int(c.Level))
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, fmt.Errorf("write: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}

	return buf.Bytes(), nil
}

// decompressBytes decompresses a gzipped byte slice.
func decompressBytes(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer r.Close()

	result, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return result, nil
}

// IsCompressedFile returns true if the file path indicates a compressed file.
func IsCompressedFile(path string) bool {
	return strings.HasSuffix(path, ".gz")
}

// CompressedPath returns the compressed path for a file.
func CompressedPath(path string) string {
	return path + ".gz"
}

// UncompressedPath returns the uncompressed path for a file.
func UncompressedPath(path string) string {
	return strings.TrimSuffix(path, ".gz")
}

// SnapshotCompressionInfo stores compression metadata in the descriptor.
type SnapshotCompressionInfo struct {
	Type  CompressionType  `json:"type,omitempty"`
	Level CompressionLevel `json:"level,omitempty"`
}

// CompressionInfoFromLevel creates compression info from a level string.
func CompressionInfoFromLevel(level string) (*SnapshotCompressionInfo, error) {
	c, err := NewCompressorFromString(level)
	if err != nil {
		return nil, err
	}
	if !c.IsEnabled() {
		return nil, nil
	}
	return &SnapshotCompressionInfo{
		Type:  c.Type,
		Level: c.Level,
	}, nil
}

// String returns the string representation of the compression info.
func (ci *SnapshotCompressionInfo) String() string {
	if ci == nil || ci.Type == TypeNone {
		return "none"
	}
	return fmt.Sprintf("%s-%d", ci.Type, ci.Level)
}
