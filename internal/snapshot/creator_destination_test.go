package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatorFullSnapshotClonesIntoOwnedNewTmpDestination(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "main", "full.txt"), []byte("full"), 0644))

	strict := &strictNewDestinationEngine{requireParent: true}
	creator := NewCreator(repoPath, model.EngineCopy)
	creator.engine = strict

	desc, err := creator.Create("main", "strict full", nil)
	require.NoError(t, err)
	require.NotNil(t, desc)
	require.Len(t, strict.destinations, 1)
	assert.True(t, strings.HasSuffix(strict.destinations[0], ".tmp"), "clone destination should be unpublished snapshot tmp: %s", strict.destinations[0])
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), "full.txt"))
}

func TestCreatorPartialSnapshotCreatesOnlyCloneParentForNestedDirectoryPath(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "main", "nested", "dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "main", "nested", "dir", "file.txt"), []byte("partial"), 0644))

	strict := &strictNewDestinationEngine{requireParent: true}
	creator := NewCreator(repoPath, model.EngineCopy)
	creator.engine = strict

	desc, err := creator.CreatePartial("main", "strict partial", nil, []string{"nested/dir"})
	require.NoError(t, err)
	require.NotNil(t, desc)
	require.Len(t, strict.destinations, 1)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(strict.destinations[0]), ".tmp/nested/dir"), "clone destination should be nested partial leaf: %s", strict.destinations[0])
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), "nested", "dir", "file.txt"))
}

type strictNewDestinationEngine struct {
	requireParent bool
	destinations  []string
}

func (e *strictNewDestinationEngine) Name() model.EngineType {
	return model.EngineCopy
}

func (e *strictNewDestinationEngine) Clone(src, dst string) (*engine.CloneResult, error) {
	if _, err := os.Lstat(dst); err == nil {
		return nil, fmt.Errorf("clone destination was precreated: %s", dst)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat clone destination: %w", err)
	}
	if e.requireParent {
		info, err := os.Stat(filepath.Dir(dst))
		if err != nil {
			return nil, fmt.Errorf("clone destination parent missing: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("clone destination parent is not a directory: %s", filepath.Dir(dst))
		}
	}
	e.destinations = append(e.destinations, dst)
	return engine.NewCopyEngine().Clone(src, dst)
}
