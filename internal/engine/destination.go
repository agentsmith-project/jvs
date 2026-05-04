package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

type cloneDestinationMode int

const (
	cloneDestinationExisting cloneDestinationMode = iota
	cloneDestinationNew
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
	if err := revalidateCloneToNewDestination(dst); err != nil {
		return nil, err
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
	if err := ensureCloneDestinationParent(parent); err != nil {
		return fmt.Errorf("create clone destination parent: %w", err)
	}
	return revalidateCloneToNewDestination(dst)
}

func revalidateCloneToNewDestination(dst string) error {
	if dst == "" {
		return fmt.Errorf("clone destination is required")
	}
	if err := validateCloneDestinationParent(dst); err != nil {
		return fmt.Errorf("clone destination parent is not safe: %w", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("clone destination already exists: %s", dst)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat clone destination: %w", err)
	}
	return nil
}

func ensureCloneNewPath(dstRoot, rel, dstPath string) error {
	if rel == "." {
		return revalidateCloneToNewDestination(dstPath)
	}
	if err := pathutil.ValidateNoSymlinkParents(dstRoot, rel); err != nil {
		return fmt.Errorf("clone destination parent is not safe: %w", err)
	}
	if _, err := os.Lstat(dstPath); err == nil {
		return fmt.Errorf("clone destination already exists: %s", dstPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat clone destination: %w", err)
	}
	return nil
}

func validateCloneDestinationParent(dst string) error {
	return ensureDirNoSymlink(filepath.Dir(dst), 0755, false)
}

func ensureCloneDestinationParent(parent string) error {
	return ensureDirNoSymlink(parent, 0755, true)
}

func ensureDirNoSymlink(path string, perm os.FileMode, create bool) error {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("absolute path: %w", err)
	}
	clean := filepath.Clean(abs)
	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	current := volume
	if strings.HasPrefix(rest, string(os.PathSeparator)) {
		current += string(os.PathSeparator)
		rest = strings.TrimPrefix(rest, string(os.PathSeparator))
	}
	if current == "" {
		current = "."
	}
	if rest == "" {
		return validateExistingDirNoSymlink(current)
	}

	for _, component := range strings.Split(rest, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		next := filepath.Join(current, component)
		info, err := os.Lstat(next)
		if err != nil {
			if !os.IsNotExist(err) || !create {
				return fmt.Errorf("stat parent %s: %w", next, err)
			}
			if err := os.Mkdir(next, perm); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create parent %s: %w", next, err)
			}
			info, err = os.Lstat(next)
			if err != nil {
				return fmt.Errorf("stat parent %s: %w", next, err)
			}
		}
		if err := validateDirInfoNoSymlink(next, info); err != nil {
			return err
		}
		current = next
	}
	return nil
}

func validateExistingDirNoSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat parent %s: %w", path, err)
	}
	return validateDirInfoNoSymlink(path, info)
}

func validateDirInfoNoSymlink(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("parent is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent is not directory: %s", path)
	}
	return nil
}
