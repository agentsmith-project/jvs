package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferIntoMainWorkspaceClonesToOwnedStagingBeforePublishing(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("payload"), 0644))

	r, err := repo.InitTarget(filepath.Join(base, "repo"))
	require.NoError(t, err)
	mainWorkspace := filepath.Join(r.Root, "main")

	strict := &strictTransferEngine{}
	oldNewTransferEngine := newTransferEngine
	newTransferEngine = func(model.EngineType) engine.Engine {
		return strict
	}
	t.Cleanup(func() {
		newTransferEngine = oldNewTransferEngine
	})
	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineCopy))

	plan, err := transferIntoMainWorkspace(source, mainWorkspace, r.Root)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, strict.destinations, 1)
	assert.NotEqual(t, mainWorkspace, strict.destinations[0], "clone should target staging, not precreated main")
	assert.True(t, strings.HasPrefix(filepath.Base(strict.destinations[0]), ".main.transfer-"), "unexpected staging path: %s", strict.destinations[0])
	assert.FileExists(t, filepath.Join(mainWorkspace, "file.txt"))
	assert.NoDirExists(t, strict.destinations[0])
}

type strictTransferEngine struct {
	destinations []string
}

func (e *strictTransferEngine) Name() model.EngineType {
	return model.EngineCopy
}

func (e *strictTransferEngine) Clone(src, dst string) (*engine.CloneResult, error) {
	if _, err := os.Lstat(dst); err == nil {
		return nil, fmt.Errorf("transfer destination was precreated: %s", dst)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if _, err := os.Stat(filepath.Dir(dst)); err != nil {
		return nil, fmt.Errorf("transfer destination parent missing: %w", err)
	}
	e.destinations = append(e.destinations, dst)
	return engine.NewCopyEngine().Clone(src, dst)
}
