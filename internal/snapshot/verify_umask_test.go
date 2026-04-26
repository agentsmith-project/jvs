//go:build !windows

package snapshot_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifySnapshot_CompressedPayloadUsesLogicalHashDespiteRestrictiveUmask(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	publicDir := filepath.Join(mainPath, "public")
	require.NoError(t, os.MkdirAll(publicDir, 0755))
	require.NoError(t, os.Chmod(publicDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "file.txt"), []byte("compressed content"), 0644))
	require.NoError(t, os.Chmod(filepath.Join(publicDir, "file.txt"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "archive.gz"), []byte("user-owned gzip payload"), 0644))
	require.NoError(t, os.Chmod(filepath.Join(mainPath, "archive.gz"), 0644))

	oldUmask := syscall.Umask(0077)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)
	desc, err := creator.Create("main", "compressed under restrictive umask", nil)
	require.NoError(t, err)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "public", "file.txt.gz"))
	assert.FileExists(t, filepath.Join(snapshotDir, "archive.gz"))

	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	require.NoError(t, err)
}
