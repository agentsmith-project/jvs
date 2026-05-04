//go:build linux

package engine

import (
	"fmt"
	"os"
	"syscall"
)

var (
	reflinkFileClone = func(dstFD, srcFD uintptr) syscall.Errno {
		const FICLONE = 0x40049409
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, dstFD, FICLONE, srcFD)
		return errno
	}
	reflinkFileToNewAfterCreateHook func(dst string) error
)

// reflinkFile attempts FICLONE ioctl to create a CoW copy.
func reflinkFile(src, dst string, info os.FileInfo) error {
	return reflinkFileWithFlags(src, dst, info, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
}

func reflinkFileToNew(src, dst string, info os.FileInfo) error {
	return reflinkFileWithFlags(src, dst, info, os.O_CREATE|os.O_WRONLY|os.O_EXCL)
}

func reflinkFileWithFlags(src, dst string, info os.FileInfo, flags int) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer srcFile.Close()

	mode := info.Mode().Perm()
	dstFile, err := os.OpenFile(dst, flags, mode)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer dstFile.Close()
	createdInfo, err := dstFile.Stat()
	if err != nil {
		return fmt.Errorf("stat created dst: %w", err)
	}

	if flags&os.O_EXCL != 0 && reflinkFileToNewAfterCreateHook != nil {
		if err := reflinkFileToNewAfterCreateHook(dst); err != nil {
			return fmt.Errorf("after create dst: %w", err)
		}
	}

	errno := reflinkFileClone(dstFile.Fd(), srcFile.Fd())
	if errno != 0 {
		if flags&os.O_EXCL != 0 {
			if err := cleanupReflinkFileToNewDestination(dst, createdInfo); err != nil {
				return fmt.Errorf("ficlone failed: %v; cleanup destination: %w", errno, err)
			}
		} else {
			dstFile.Close()
			os.Remove(dst)
		}
		return fmt.Errorf("ficlone failed: %v", errno)
	}

	if err := dstFile.Chmod(mode); err != nil {
		return fmt.Errorf("chmod dst: %w", err)
	}

	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func cleanupReflinkFileToNewDestination(dst string, createdInfo os.FileInfo) error {
	if createdInfo == nil {
		return fmt.Errorf("%w: created destination identity unavailable; no files were removed", errReflinkFallbackBlocked)
	}
	currentInfo, err := os.Lstat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: destination changed before cleanup; no files were removed", errReflinkFallbackBlocked)
		}
		return fmt.Errorf("%w: stat destination before cleanup: %v", errReflinkFallbackBlocked, err)
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(createdInfo, currentInfo) {
		return fmt.Errorf("%w: destination changed before cleanup; no files were removed", errReflinkFallbackBlocked)
	}
	if err := os.Remove(dst); err != nil {
		return fmt.Errorf("%w: remove created destination: %v", errReflinkFallbackBlocked, err)
	}
	return nil
}
