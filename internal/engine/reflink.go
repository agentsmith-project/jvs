package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
)

// ReflinkEngine performs reflink-based copy (O(1) CoW) on supported filesystems.
// Falls back to regular copy for files that cannot be reflinked.
type ReflinkEngine struct {
	CopyEngine *CopyEngine
}

// NewReflinkEngine creates a new ReflinkEngine.
func NewReflinkEngine() *ReflinkEngine {
	return &ReflinkEngine{
		CopyEngine: NewCopyEngine(),
	}
}

// Name returns the engine type.
func (e *ReflinkEngine) Name() model.EngineType {
	return model.EngineReflinkCopy
}

// Clone performs a reflink copy if supported, falls back to regular copy otherwise.
func (e *ReflinkEngine) Clone(src, dst string) (*CloneResult, error) {
	result := &CloneResult{}
	var dirs []dirMode

	if err := os.MkdirAll(dst, 0755); err != nil {
		return nil, fmt.Errorf("create dst directory: %w", err)
	}

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		dstPath := filepath.Join(dst, rel)

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
			if err := reflinkFile(path, dstPath, info); err != nil {
				result.Degraded = true
				result.Degradations = append(result.Degradations, "reflink")
				return e.copyFile(path, dstPath, info)
			}
			return nil
		}
	})

	if err != nil {
		return nil, fmt.Errorf("reflink clone: %w", err)
	}

	if err := chmodDirs(dirs); err != nil {
		return nil, fmt.Errorf("reflink clone: %w", err)
	}

	if err := fsutil.FsyncDir(dst); err != nil {
		return nil, fmt.Errorf("fsync dst: %w", err)
	}

	return result, nil
}

func (e *ReflinkEngine) copyDir(src, dst string, info os.FileInfo) error {
	return os.MkdirAll(dst, writableDirMode(info.Mode().Perm()))
}

func (e *ReflinkEngine) copySymlink(src, dst string, info os.FileInfo) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("readlink: %w", err)
	}
	return os.Symlink(target, dst)
}

func (e *ReflinkEngine) copyFile(src, dst string, info os.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer srcFile.Close()

	mode := info.Mode().Perm()
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
