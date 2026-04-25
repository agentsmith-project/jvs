package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/config"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestDetectEngine_UsesSnapshotEngineEnvFullValue(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineJuiceFSClone))
	t.Setenv("JVS_ENGINE", "copy")

	require.Equal(t, model.EngineJuiceFSClone, detectEngine(t.TempDir()))
}

func TestDetectEngine_UsesLegacySimpleEngineEnv(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "reflink")

	require.Equal(t, model.EngineReflinkCopy, detectEngine(t.TempDir()))
}

func TestDetectEngine_UsesConfigDefaultEngine(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	repoRoot := initEngineResolverRepo(t)

	cfg := config.Default()
	cfg.DefaultEngine = model.EngineJuiceFSClone
	require.NoError(t, config.Save(repoRoot, cfg))

	require.Equal(t, model.EngineJuiceFSClone, detectEngine(repoRoot))
}

func TestWorktreeCloneEngine_UsesEffectiveResolver(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineReflinkCopy))
	t.Setenv("JVS_ENGINE", "")
	repoRoot := initEngineResolverRepo(t)

	cfg := config.Default()
	cfg.DefaultEngine = model.EngineCopy
	require.NoError(t, config.Save(repoRoot, cfg))

	require.Equal(t, model.EngineReflinkCopy, newCloneEngine(repoRoot).Name())
}

func initEngineResolverRepo(t *testing.T) string {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoRoot, 0755))
	_, err := repo.Init(repoRoot, "repo")
	require.NoError(t, err)
	return repoRoot
}
