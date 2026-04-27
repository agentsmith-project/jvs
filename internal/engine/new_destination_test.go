package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneToNewRequiresMissingDestinationButLegacyCloneAllowsEmptyDestination(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))

	legacyDst := filepath.Join(t.TempDir(), "legacy")
	require.NoError(t, os.MkdirAll(legacyDst, 0755))
	_, err := engine.NewCopyEngine().Clone(src, legacyDst)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(legacyDst, "file.txt"))

	newDst := filepath.Join(t.TempDir(), "owned-new")
	require.NoError(t, os.MkdirAll(newDst, 0755))
	_, err = engine.CloneToNew(engine.NewCopyEngine(), src, newDst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCloneToNewCreatesParentAndLeavesLeafForEngine(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("payload"), 0644))

	dst := filepath.Join(t.TempDir(), "parent", "owned-new")
	_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "file.txt"))
}
