//go:build linux

package fsutil

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldpath, newpath string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, uint(unix.RENAME_NOREPLACE)); err != nil {
		if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) {
			return &os.LinkError{Op: "renameat2", Old: oldpath, New: newpath, Err: ErrRenameNoReplaceUnsupported}
		}
		return &os.LinkError{Op: "renameat2", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}
