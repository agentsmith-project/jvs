//go:build !windows

package capacitygate

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func (StatfsMeter) AvailableBytes(path string) (int64, error) {
	probe := existingParent(path)
	var st unix.Statfs_t
	if err := unix.Statfs(probe, &st); err != nil {
		return 0, err
	}
	availableBlocks, err := statfsUint64(st.Bavail, "available blocks")
	if err != nil {
		return 0, fmt.Errorf("statfs available bytes for %s: %w", probe, err)
	}
	blockSize, err := statfsInt64(st.Bsize, "block size")
	if err != nil {
		return 0, fmt.Errorf("statfs available bytes for %s: %w", probe, err)
	}
	available, err := availableBytesFromStatfs(availableBlocks, blockSize)
	if err != nil {
		return 0, fmt.Errorf("statfs available bytes for %s: %w", probe, err)
	}
	return available, nil
}

func (StatfsMeter) DeviceID(path string) (string, error) {
	probe := existingParent(path)
	var st unix.Stat_t
	if err := unix.Stat(probe, &st); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", st.Dev), nil
}

func statfsUint64[T ~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64](value T, name string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid statfs %s %d", name, value)
	}
	return uint64(value), nil
}

func statfsInt64[T ~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64](value T, name string) (int64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid statfs %s %d", name, value)
	}
	unsigned := uint64(value)
	if unsigned > uint64(maxInt64) {
		return 0, fmt.Errorf("invalid statfs %s %d exceeds int64", name, value)
	}
	return int64(unsigned), nil
}
