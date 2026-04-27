package restore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreMaterializesSnapshotIntoMissingTempDestination(t *testing.T) {
	repoPath := setupFailureTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))

	desc, err := snapshot.NewCreator(repoPath, model.EngineCopy).Create("main", "base", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644))

	strict := &strictRestoreCloneEngine{}
	restorer := NewRestorer(repoPath, model.EngineCopy)
	restorer.engine = strict

	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)
	require.Len(t, strict.destinations, 1)
	assert.Contains(t, strict.destinations[0], ".restore-tmp-")
	assert.FileExists(t, filepath.Join(mainPath, "file.txt"))
	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "snapshot", string(content))
}

type strictRestoreCloneEngine struct {
	destinations []string
}

func (e *strictRestoreCloneEngine) Name() model.EngineType {
	return model.EngineCopy
}

func (e *strictRestoreCloneEngine) Clone(src, dst string) (*engine.CloneResult, error) {
	if _, err := os.Lstat(dst); err == nil {
		return nil, fmt.Errorf("restore materialize destination was precreated: %s", dst)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if _, err := os.Stat(filepath.Dir(dst)); err != nil {
		return nil, fmt.Errorf("restore materialize destination parent missing: %w", err)
	}
	e.destinations = append(e.destinations, dst)
	if !strings.Contains(dst, ".restore-tmp-") {
		return nil, fmt.Errorf("restore materialize destination should be temp path: %s", dst)
	}
	return engine.NewCopyEngine().Clone(src, dst)
}
