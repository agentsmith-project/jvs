package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
)

// SameFilesystem reports whether sourcePath and destinationParent are on the
// same filesystem device.
func SameFilesystem(sourcePath, destinationParent string) (bool, error) {
	meter := capacitygate.StatfsMeter{}
	sourceDevice, err := meter.DeviceID(sourcePath)
	if err != nil {
		return false, fmt.Errorf("inspect source filesystem: %w", err)
	}
	destinationDevice, err := meter.DeviceID(destinationParent)
	if err != nil {
		return false, fmt.Errorf("inspect destination filesystem: %w", err)
	}
	return sourceDevice == destinationDevice, nil
}

// MoveSameFilesystemNoOverwrite moves sourcePath to destinationPath using the
// platform no-overwrite atomic rename primitive. Cross-filesystem moves fail
// closed so callers do not accidentally fall back to copy+delete semantics.
func MoveSameFilesystemNoOverwrite(sourcePath, destinationPath string) error {
	sourceAbs, err := cleanAbsPath(sourcePath)
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
	}
	destinationAbs, err := cleanAbsPath(destinationPath)
	if err != nil {
		return fmt.Errorf("resolve destination path: %w", err)
	}
	if _, err := os.Lstat(sourceAbs); err != nil {
		return fmt.Errorf("stat source path: %w", err)
	}
	destinationParent := filepath.Dir(destinationAbs)
	parentInfo, err := os.Lstat(destinationParent)
	if err != nil {
		return fmt.Errorf("stat destination parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination parent must not be a symlink: %s", destinationParent)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("destination parent is not a directory: %s", destinationParent)
	}

	same, err := SameFilesystem(sourceAbs, destinationParent)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("source and destination parent are on different filesystems")
	}

	sourceParent := filepath.Dir(sourceAbs)
	if err := fsutil.RenameNoReplaceAndSync(sourceAbs, destinationAbs); err != nil {
		return err
	}
	if sourceParent != destinationParent {
		if err := fsutil.FsyncDir(sourceParent); err != nil {
			return &fsutil.CommitUncertainError{
				Op:   "rename no-replace",
				Path: sourceAbs,
				Err:  fmt.Errorf("fsync source directory after rename: %w", err),
			}
		}
	}
	return nil
}
