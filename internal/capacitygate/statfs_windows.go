//go:build windows

package capacitygate

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func (StatfsMeter) AvailableBytes(path string) (int64, error) {
	probe := existingParent(path)
	full, pathp, err := windowsFullPathPointer(probe)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pathp, &available, nil, nil); err != nil {
		return 0, err
	}
	if available > uint64(maxInt64) {
		return 0, fmt.Errorf("disk free space for %s exceeds int64: bytes=%d", full, available)
	}
	return int64(available), nil
}

func (StatfsMeter) DeviceID(path string) (string, error) {
	probe := existingParent(path)
	root, err := windowsVolumeRoot(probe)
	if err != nil {
		return "", err
	}
	return root, nil
}

func windowsVolumeRoot(path string) (string, error) {
	full, pathp, err := windowsFullPathPointer(path)
	if err != nil {
		return "", err
	}
	bufferLen := uint32(len(full) + 1)
	if bufferLen < windows.MAX_PATH {
		bufferLen = windows.MAX_PATH
	}
	for {
		buf := make([]uint16, bufferLen)
		err := windows.GetVolumePathName(pathp, &buf[0], bufferLen)
		if err == nil {
			root := windows.UTF16ToString(buf)
			if root == "" {
				return "", fmt.Errorf("empty volume root for %s", full)
			}
			return strings.ToLower(filepath.Clean(root)), nil
		}
		if err != windows.ERROR_INSUFFICIENT_BUFFER && err != windows.ERROR_FILENAME_EXCED_RANGE {
			return "", err
		}
		if bufferLen >= 32768 {
			return "", err
		}
		bufferLen *= 2
	}
}

func windowsFullPathPointer(path string) (string, *uint16, error) {
	full, err := windows.FullPath(path)
	if err != nil {
		return "", nil, err
	}
	pathp, err := windows.UTF16PtrFromString(full)
	if err != nil {
		return "", nil, err
	}
	return full, pathp, nil
}
