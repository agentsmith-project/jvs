package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyEngineCloneToNewRefusesLeafCreatedAfterPrepare(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "payload.txt")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0644))

	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside original"), 0644))

	dst := filepath.Join(t.TempDir(), "late-leaf")
	require.NoError(t, PrepareCloneToNewDestination(dst))
	require.NoError(t, os.Symlink(outside, dst))

	_, _, err := NewCopyEngine().cloneInto(src, dst, cloneDestinationNew)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	content, readErr := os.ReadFile(outside)
	require.NoError(t, readErr)
	assert.Equal(t, "outside original", string(content))
}
