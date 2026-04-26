package engine_test

import (
	"os"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEngine_Default(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("JVS_ENGINE")

	// Create a temp dir to test detection
	tmpDir := t.TempDir()

	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, eng)

	// Without JuiceFS or reflink support, should fall back to copy
	// The exact engine depends on the filesystem
	assert.Contains(t, []model.EngineType{
		model.EngineCopy,
		model.EngineReflinkCopy,
		model.EngineJuiceFSClone,
	}, eng.Name())
}

func TestDetectEngine_EnvCopy(t *testing.T) {
	os.Setenv("JVS_ENGINE", "copy")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, model.EngineCopy, eng.Name())
}

func TestDetectEngine_EnvReflink(t *testing.T) {
	os.Setenv("JVS_ENGINE", "reflink")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, model.EngineReflinkCopy, eng.Name())
}

func TestDetectEngine_EnvJuiceFS(t *testing.T) {
	os.Setenv("JVS_ENGINE", "juicefs")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, model.EngineJuiceFSClone, eng.Name())
}

func TestDetectEngine_EnvUnknown(t *testing.T) {
	os.Setenv("JVS_ENGINE", "unknown")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	// Unknown env value should fall back to auto-detect
	assert.Contains(t, []model.EngineType{
		model.EngineCopy,
		model.EngineReflinkCopy,
		model.EngineJuiceFSClone,
	}, eng.Name())
}
