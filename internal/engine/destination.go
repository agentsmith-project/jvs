package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

// CloneToNew clones src into an owned destination path whose leaf must not
// already exist. This is the mode required by engines such as `juicefs clone`.
func CloneToNew(eng Engine, src, dst string) (*CloneResult, error) {
	if eng == nil {
		return nil, fmt.Errorf("clone engine is required")
	}
	if err := PrepareCloneToNewDestination(dst); err != nil {
		return nil, err
	}
	if newDestinationEngine, ok := eng.(interface {
		CloneToNew(src, dst string) (*CloneResult, error)
	}); ok {
		return newDestinationEngine.CloneToNew(src, dst)
	}
	return eng.Clone(src, dst)
}

// PrepareCloneToNewDestination creates the destination parent if necessary and
// verifies that the destination leaf is absent.
func PrepareCloneToNewDestination(dst string) error {
	if dst == "" {
		return fmt.Errorf("clone destination is required")
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("create clone destination parent: %w", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("clone destination already exists: %s", dst)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat clone destination: %w", err)
	}
	return nil
}
