//go:build linux

package engine

import (
	"fmt"
	"os"
	"syscall"
)

// reflinkFile attempts FICLONE ioctl to create a CoW copy.
func reflinkFile(src, dst string, info os.FileInfo) error {
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

	const FICLONE = 0x40049409
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, dstFile.Fd(), FICLONE, srcFile.Fd())
	if errno != 0 {
		dstFile.Close()
		os.Remove(dst)
		return fmt.Errorf("ficlone failed: %v", errno)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod dst: %w", err)
	}

	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
