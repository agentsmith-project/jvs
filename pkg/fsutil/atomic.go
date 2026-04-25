// Package fsutil provides filesystem utilities for atomic operations and syncing.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CommitUncertainError reports that an operation reached the point where its
// filesystem mutation may be visible, but a post-commit durability step failed.
type CommitUncertainError struct {
	Op   string
	Path string
	Err  error
}

func (e *CommitUncertainError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Path == "" {
		return fmt.Sprintf("%s commit uncertain: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("%s commit uncertain for %s: %v", e.Op, e.Path, e.Err)
}

func (e *CommitUncertainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsCommitUncertain reports whether err means the filesystem mutation may have
// committed even though a later durability/sync step failed.
func IsCommitUncertain(err error) bool {
	var uncertain *CommitUncertainError
	return errors.As(err, &uncertain)
}

// ErrRenameNoReplaceUnsupported reports that the current platform does not
// provide an atomic no-replace rename primitive.
var ErrRenameNoReplaceUnsupported = errors.New("atomic rename no-replace unsupported")

var (
	fsyncDir          = FsyncDir
	renameNoReplaceOp = renameNoReplace
)

// AtomicWrite writes data to a temporary file, fsyncs, then renames to target path.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jvs-tmp-*")
	if err != nil {
		return fmt.Errorf("atomic write create tmp: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up on failure
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("atomic write: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("atomic write chmod: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("atomic write fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write close: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic write rename: %w", err)
	}
	if err := fsyncDir(dir); err != nil {
		return &CommitUncertainError{
			Op:   "atomic write",
			Path: path,
			Err:  fmt.Errorf("fsync directory after rename: %w", err),
		}
	}

	success = true
	return nil
}

// RenameAndSync renames old to new and fsyncs the parent directory.
func RenameAndSync(oldpath, newpath string) error {
	if err := os.Rename(oldpath, newpath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if err := fsyncDir(filepath.Dir(newpath)); err != nil {
		return &CommitUncertainError{
			Op:   "rename",
			Path: newpath,
			Err:  fmt.Errorf("fsync directory after rename: %w", err),
		}
	}
	return nil
}

// RenameNoReplaceAndSync renames old to new without replacing an existing
// destination and fsyncs the parent directory.
func RenameNoReplaceAndSync(oldpath, newpath string) error {
	if err := renameNoReplaceOp(oldpath, newpath); err != nil {
		return fmt.Errorf("rename no-replace: %w", err)
	}
	if err := fsyncDir(filepath.Dir(newpath)); err != nil {
		return &CommitUncertainError{
			Op:   "rename no-replace",
			Path: newpath,
			Err:  fmt.Errorf("fsync directory after rename: %w", err),
		}
	}
	return nil
}

// FsyncDir fsyncs a directory to ensure rename visibility is durable.
func FsyncDir(dirPath string) error {
	d, err := os.Open(dirPath)
	if err != nil {
		return fmt.Errorf("fsync dir open: %w", err)
	}
	defer d.Close()
	return d.Sync()
}

// FsyncTree recursively fsyncs all files under the given root directory.
// This ensures all data is durably written to disk before marking an operation complete.
func FsyncTree(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Only sync regular files. Directories are synced via FsyncDir, and
		// symlink targets may be outside the tree or intentionally dangling.
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open %s for fsync: %w", path, err)
			}
			err = f.Sync()
			f.Close()
			if err != nil {
				return fmt.Errorf("fsync %s: %w", path, err)
			}
		}
		return nil
	})
}
