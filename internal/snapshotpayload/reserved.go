package snapshotpayload

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	storageReadyMarkerName     = ".READY"
	storageReadyGzipMarkerName = ".READY.gz"
)

var storageControlMarkerNames = [...]string{
	storageReadyMarkerName,
	storageReadyGzipMarkerName,
}

// ReservedWorkspacePayloadRootNames returns names that cannot appear as user
// files at the workspace payload root because they collide with storage control
// markers.
func ReservedWorkspacePayloadRootNames() []string {
	names := make([]string, 0, len(storageControlMarkerNames))
	return append(names, storageControlMarkerNames[:]...)
}

// ReservedWorkspacePayloadRootPathError reports a user payload root entry that
// collides with snapshot storage control paths.
type ReservedWorkspacePayloadRootPathError struct {
	Name string
	Path string
}

func (e *ReservedWorkspacePayloadRootPathError) Error() string {
	return fmt.Sprintf("reserved workspace payload root path %q exists at %s; move or remove it before continuing", e.Name, e.Path)
}

// CheckReservedWorkspacePayloadRoot fails closed when a workspace payload root
// contains names reserved by the snapshot storage control plane.
func CheckReservedWorkspacePayloadRoot(root string) error {
	if root == "" {
		return fmt.Errorf("workspace payload root is required")
	}
	for _, name := range storageControlMarkerNames {
		path := filepath.Join(root, name)
		if _, err := os.Lstat(path); err == nil {
			return &ReservedWorkspacePayloadRootPathError{Name: name, Path: path}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check reserved workspace payload root path %q: %w", name, err)
		}
	}
	return nil
}
