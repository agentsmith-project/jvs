package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicWrite_PostRenameFsyncFailureIsCommitUncertain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	oldFsyncDir := fsyncDir
	fsyncDir = func(string) error {
		return errors.New("injected directory fsync failure")
	}
	t.Cleanup(func() {
		fsyncDir = oldFsyncDir
	})

	err := AtomicWrite(path, []byte("new config"), 0644)
	require.Error(t, err)
	require.True(t, IsCommitUncertain(err), "post-rename fsync failure must be classified as an uncertain commit")

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, []byte("new config"), data)
}
