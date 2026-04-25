package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
)

// CopyEngine performs a full recursive copy of directories.
// This is the fallback engine that works on any filesystem but does not
// preserve hardlinks (they become separate copies).
type CopyEngine struct{}

// NewCopyEngine creates a new CopyEngine.
func NewCopyEngine() *CopyEngine {
	return &CopyEngine{}
}

// Name returns the engine type.
func (e *CopyEngine) Name() model.EngineType {
	return model.EngineCopy
}

// Clone recursively copies src to dst.
// Returns a degraded result if hardlinks were detected (they become separate copies).
func (e *CopyEngine) Clone(src, dst string) (*CloneResult, error) {
	result := NewCloneResult(model.EngineCopy)

	seenInodes := make(map[uint64]string)
	var dirs []dirMode

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		dstPath := filepath.Join(dst, rel)

		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if ino, ok := fileInode(info); ok {
				if seenInodes[ino] != "" {
					result.AddDegradation("hardlink", model.EngineCopy)
				} else {
					seenInodes[ino] = path
				}
			}
		}

		switch {
		case info.IsDir():
			if err := e.copyDir(path, dstPath, info); err != nil {
				return err
			}
			dirs = append(dirs, dirMode{path: dstPath, mode: info.Mode().Perm()})
			return nil

		case info.Mode()&os.ModeSymlink != 0:
			return e.copySymlink(path, dstPath, info)

		default:
			return e.copyFile(path, dstPath, info)
		}
	})

	if err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}

	if err := chmodDirs(dirs); err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}

	if err := fsutil.FsyncDir(dst); err != nil {
		return nil, fmt.Errorf("fsync dst: %w", err)
	}

	return result, nil
}

func (e *CopyEngine) copyDir(src, dst string, info os.FileInfo) error {
	if err := os.MkdirAll(dst, writableDirMode(info.Mode().Perm())); err != nil {
		return fmt.Errorf("mkdir %s: %w", dst, err)
	}
	return nil
}

func (e *CopyEngine) copyFile(src, dst string, info os.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer srcFile.Close()

	mode := info.Mode().Perm()
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create dst %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}

	// Sync file content
	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dst, err)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}

	// Preserve mod time
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func (e *CopyEngine) copySymlink(src, dst string, info os.FileInfo) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("readlink %s: %w", src, err)
	}
	return os.Symlink(target, dst)
}

type dirMode struct {
	path string
	mode os.FileMode
}

func writableDirMode(mode os.FileMode) os.FileMode {
	return mode | 0700
}

func chmodDirs(dirs []dirMode) error {
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return fmt.Errorf("chmod dir %s: %w", dirs[i].path, err)
		}
	}
	return nil
}
