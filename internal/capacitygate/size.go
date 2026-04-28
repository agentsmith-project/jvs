package capacitygate

import (
	"fmt"
	"os"
	"path/filepath"
)

func TreeSize(root string, exclude func(rel string) bool) (int64, error) {
	return treeSize(root, root, exclude)
}

func TreeSizeWithin(baseRoot, relPath string, exclude func(rel string) bool) (int64, error) {
	target := filepath.Join(baseRoot, filepath.FromSlash(relPath))
	return treeSize(baseRoot, target, exclude)
}

func treeSize(baseRoot, root string, exclude func(rel string) bool) (int64, error) {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat size root: %w", err)
	}
	if !info.IsDir() {
		rel, err := filepath.Rel(baseRoot, root)
		if err != nil {
			return 0, fmt.Errorf("relative size path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if exclude != nil && exclude(rel) {
			return 0, nil
		}
		return logicalEntrySize(info), nil
	}

	var total int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(baseRoot, path)
		if err != nil {
			return fmt.Errorf("relative size path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if rel != "." && exclude != nil && exclude(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat size path: %w", err)
		}
		total = saturatingAdd(total, logicalEntrySize(info))
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func logicalEntrySize(info os.FileInfo) int64 {
	if info == nil || info.IsDir() {
		return 0
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return info.Size()
	}
	if info.Size() < 0 {
		return 0
	}
	return info.Size()
}
