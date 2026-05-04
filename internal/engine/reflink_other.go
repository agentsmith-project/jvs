//go:build !linux

package engine

import (
	"fmt"
	"os"
)

// reflinkFile is unsupported on non-Linux platforms (FICLONE is a Linux ioctl).
func reflinkFile(_, _ string, _ os.FileInfo) error {
	return fmt.Errorf("reflink not supported on this platform")
}

func reflinkFileToNew(_, _ string, _ os.FileInfo) error {
	return fmt.Errorf("reflink not supported on this platform")
}
