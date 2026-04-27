package snapshotpayload_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializeToNewClonesIntoMissingDestination(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".READY"), []byte("ready"), 0644))

	dst := filepath.Join(t.TempDir(), "materialized")
	called := false
	err := snapshotpayload.MaterializeToNew(src, dst, snapshotpayload.Options{}, func(src, dst string) error {
		called = true
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("materialize destination was precreated: %s", dst)
		} else if !os.IsNotExist(err) {
			return err
		}
		return copyTree(src, dst)
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.FileExists(t, filepath.Join(dst, "file.txt"))
	assert.NoFileExists(t, filepath.Join(dst, ".READY"))
}

func TestMaterializeToNewRejectsExistingEmptyDestination(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "materialized")
	require.NoError(t, os.MkdirAll(dst, 0755))

	err := snapshotpayload.MaterializeToNew(src, dst, snapshotpayload.Options{}, copyTree)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}
