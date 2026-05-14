package integrity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/model"
)

type PayloadRootHashStats struct {
	Entries     int64
	Files       int64
	Directories int64
	Symlinks    int64
	Bytes       int64
}

// ComputePayloadRootHash computes a deterministic hash of the entire payload tree.
// Algorithm: walk in byte-order sorted path order, compute per-entry hash,
// concatenate all lines, hash the result.
func ComputePayloadRootHash(root string) (model.HashValue, error) {
	return ComputePayloadRootHashWithExclusions(root, nil)
}

// ComputePayloadRootHashWithExclusions computes a payload hash while skipping
// any workspace-relative paths reported as excluded.
func ComputePayloadRootHashWithExclusions(root string, excluded func(rel string) bool) (model.HashValue, error) {
	hash, _, err := ComputePayloadRootHashWithExclusionsAndStats(root, excluded)
	return hash, err
}

func ComputePayloadRootHashWithStats(root string) (model.HashValue, PayloadRootHashStats, error) {
	return ComputePayloadRootHashWithExclusionsAndStats(root, nil)
}

func ComputePayloadRootHashWithExclusionsAndStats(root string, excluded func(rel string) bool) (model.HashValue, PayloadRootHashStats, error) {
	var lines []string
	var stats PayloadRootHashStats

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip root itself
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		if filepath.ToSlash(rel) == ".READY" {
			return nil
		}
		if excluded != nil && excluded(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		stats.add(info)

		entryHash, err := computeEntryHash(path, info)
		if err != nil {
			return fmt.Errorf("hash entry %s: %w", rel, err)
		}

		// Format: <type>:<path>:<metadata>:<hash>
		// path uses forward slashes for portability
		pathPortable := filepath.ToSlash(rel)
		meta := formatMetadata(info)
		line := fmt.Sprintf("%s:%s:%s:%s", entryType(info), pathPortable, meta, entryHash)
		lines = append(lines, line)

		return nil
	})
	if err != nil {
		return "", PayloadRootHashStats{}, fmt.Errorf("walk payload: %w", err)
	}

	// Sort lines by path (byte order)
	sort.Strings(lines)

	// Concatenate and hash
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	hash := sha256.Sum256([]byte(buf.String()))
	return model.HashValue(hex.EncodeToString(hash[:])), stats, nil
}

func (s *PayloadRootHashStats) add(info os.FileInfo) {
	if s == nil || info == nil {
		return
	}
	s.Entries++
	switch {
	case info.IsDir():
		s.Directories++
	case info.Mode()&os.ModeSymlink != 0:
		s.Symlinks++
		s.Bytes += nonNegativeSize(info)
	default:
		s.Files++
		s.Bytes += nonNegativeSize(info)
	}
}

func nonNegativeSize(info os.FileInfo) int64 {
	if info == nil || info.Size() < 0 {
		return 0
	}
	return info.Size()
}

func entryType(info os.FileInfo) string {
	if info.IsDir() {
		return "dir"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	return "file"
}

func formatMetadata(info os.FileInfo) string {
	switch {
	case info.IsDir():
		return fmt.Sprintf("mode=%04o", info.Mode().Perm())
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Sprintf("mode=%04o", info.Mode().Perm())
	default:
		return fmt.Sprintf("mode=%04o,size=%d",
			info.Mode().Perm(),
			info.Size())
	}
}

func computeEntryHash(path string, info os.FileInfo) (string, error) {
	h := sha256.New()

	switch {
	case info.IsDir():
		// Directory hash is hash of its name
		h.Write([]byte(info.Name()))

	case info.Mode()&os.ModeSymlink != 0:
		// Symlink hash is hash of target
		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("read symlink: %w", err)
		}
		h.Write([]byte(target))

	default:
		// File hash is hash of content
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open file: %w", err)
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
