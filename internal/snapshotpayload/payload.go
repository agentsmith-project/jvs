// Package snapshotpayload materializes and hashes the logical user payload
// stored inside a snapshot directory.
package snapshotpayload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jvs-project/jvs/internal/compression"
	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/model"
)

// CloneFunc copies a snapshot storage tree to a destination directory.
type CloneFunc func(src, dst string) error

// Options describes how a snapshot payload is encoded on disk.
type Options struct {
	Compressed bool
}

// OptionsFromDescriptor derives payload materialization options from a descriptor.
func OptionsFromDescriptor(desc *model.Descriptor) Options {
	return Options{Compressed: desc != nil && desc.Compression != nil}
}

// OptionsForSnapshot reads the snapshot descriptor and derives payload options.
func OptionsForSnapshot(repoRoot string, snapshotID model.SnapshotID) (Options, error) {
	path, err := repo.SnapshotDescriptorPathForRead(repoRoot, snapshotID)
	if err != nil {
		return Options{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Options{}, fmt.Errorf("read descriptor: %w", err)
	}

	var desc model.Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return Options{}, fmt.Errorf("parse descriptor: %w", err)
	}
	if desc.SnapshotID != snapshotID {
		return Options{}, fmt.Errorf("descriptor snapshot ID %q does not match requested %q", desc.SnapshotID, snapshotID)
	}
	return OptionsFromDescriptor(&desc), nil
}

// Materialize clones a snapshot storage tree and normalizes it into the logical
// user payload: control markers are removed and compressed payload files are
// decoded when the descriptor says the snapshot is compressed.
func Materialize(src, dst string, opts Options, clone CloneFunc) error {
	if clone == nil {
		return fmt.Errorf("clone function is required")
	}
	if err := validateMaterializeSource(src); err != nil {
		return err
	}
	if err := prepareMaterializeDestination(dst); err != nil {
		return err
	}
	if err := clone(src, dst); err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	if err := Clean(dst, opts); err != nil {
		return err
	}
	return nil
}

func validateMaterializeSource(src string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat materialize source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("materialize source is a symlink: %s", src)
	}
	if !info.IsDir() {
		return fmt.Errorf("materialize source is not a directory: %s", src)
	}
	return nil
}

func prepareMaterializeDestination(dst string) error {
	if dst == "" {
		return fmt.Errorf("materialize destination is required")
	}
	if err := validateNoSymlinkParents(dst); err != nil {
		return fmt.Errorf("invalid materialize destination parent: %w", err)
	}

	info, err := os.Lstat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat materialize destination: %w", err)
		}
		if err := os.Mkdir(dst, 0755); err != nil {
			return fmt.Errorf("create materialize destination: %w", err)
		}
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("materialize destination is a symlink: %s", dst)
	}
	if !info.IsDir() {
		return fmt.Errorf("materialize destination is not a directory: %s", dst)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return fmt.Errorf("read materialize destination: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("materialize destination must be empty: %s", dst)
	}
	return nil
}

func validateNoSymlinkParents(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("absolute path: %w", err)
	}
	clean := filepath.Clean(abs)
	parent := filepath.Dir(clean)
	if parent == clean {
		return nil
	}

	volume := filepath.VolumeName(parent)
	rest := strings.TrimPrefix(parent, volume)
	current := volume
	if strings.HasPrefix(rest, string(os.PathSeparator)) {
		current += string(os.PathSeparator)
		rest = strings.TrimPrefix(rest, string(os.PathSeparator))
	}
	if current == "" {
		current = "."
	}
	if rest == "" {
		return nil
	}

	for _, component := range strings.Split(rest, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("stat parent %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent is symlink: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent is not directory: %s", current)
		}
	}
	return nil
}

// Clean normalizes an already-copied payload tree in place.
func Clean(root string, opts Options) error {
	if opts.Compressed {
		if _, err := compression.DecompressDir(root); err != nil {
			return fmt.Errorf("decompress payload: %w", err)
		}
	}
	if err := removeControlMarkers(root); err != nil {
		return err
	}
	return nil
}

// ComputeHash computes the logical payload hash without mutating the snapshot
// storage tree.
func ComputeHash(root string, opts Options) (model.HashValue, error) {
	tmpParent, err := os.MkdirTemp("", "jvs-payload-hash-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpParent)

	tmpPayload := filepath.Join(tmpParent, "payload")
	if err := Materialize(root, tmpPayload, opts, copyTree); err != nil {
		return "", err
	}

	hash, err := integrity.ComputePayloadRootHash(tmpPayload)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func copyTree(src, dst string) error {
	_, err := engine.NewCopyEngine().Clone(src, dst)
	return err
}

func removeControlMarkers(root string) error {
	readyPath := filepath.Join(root, storageReadyMarkerName)
	info, err := os.Lstat(readyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat control marker %s: %w", readyPath, err)
	}
	if err := os.RemoveAll(readyPath); err != nil {
		return fmt.Errorf("remove control marker %s: %w", readyPath, err)
	}
	if info.IsDir() {
		return nil
	}
	return nil
}
