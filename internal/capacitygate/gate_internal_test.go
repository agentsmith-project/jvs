package capacitygate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableBytesFromStatfsAllowsLargeRepresentableValue(t *testing.T) {
	const blockSize = int64(4096)
	blocks := uint64(maxInt64 / blockSize)

	got, err := availableBytesFromStatfs(blocks, blockSize)

	require.NoError(t, err)
	assert.Equal(t, (maxInt64/blockSize)*blockSize, got)
}

func TestAvailableBytesFromStatfsRejectsInt64Overflow(t *testing.T) {
	const blockSize = int64(4096)
	blocks := uint64(maxInt64/blockSize) + 1

	got, err := availableBytesFromStatfs(blocks, blockSize)

	require.Error(t, err)
	assert.Zero(t, got)
	assert.Contains(t, err.Error(), "available space")
}

func TestAvailableBytesFromStatfsRejectsNegativeBlockSize(t *testing.T) {
	got, err := availableBytesFromStatfs(1, -4096)

	require.Error(t, err)
	assert.Zero(t, got)
	assert.Contains(t, err.Error(), "block size")
}
