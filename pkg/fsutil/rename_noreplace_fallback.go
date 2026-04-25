//go:build !linux

package fsutil

import "os"

func renameNoReplace(oldpath, newpath string) error {
	return &os.LinkError{
		Op:  "rename no-replace",
		Old: oldpath,
		New: newpath,
		Err: ErrRenameNoReplaceUnsupported,
	}
}
