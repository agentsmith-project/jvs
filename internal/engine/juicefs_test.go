package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJuiceFSEngine_Name(t *testing.T) {
	eng := engine.NewJuiceFSEngine()
	assert.Equal(t, model.EngineJuiceFSClone, eng.Name())
}

func TestJuiceFSEngine_CloneWithFallback(t *testing.T) {
	// This test verifies the engine falls back to copy when juicefs clone is not available
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source content
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should be degraded since juicefs clone will fail or not be available
	// The degradation could be "juicefs-not-available", "not-on-juicefs", or "juicefs-clone-failed"
	assert.True(t, result.Degraded)
	// At least one degradation should be reported
	assert.NotEmpty(t, result.Degradations)

	// Verify content was still copied (via fallback)
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestJuiceFSEngine_CloneNestedStructure(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create nested structure
	os.MkdirAll(filepath.Join(src, "a", "b"), 0755)
	os.WriteFile(filepath.Join(src, "a", "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(src, "a", "b", "file2.txt"), []byte("file2"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded) // Falls back to copy

	// Verify nested content
	content, err := os.ReadFile(filepath.Join(dstPath, "a", "b", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "file2", string(content))
}

func TestJuiceFSEngine_CloneWithSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create content with symlink
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	require.NoError(t, os.Symlink("target.txt", filepath.Join(src, "link")))

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded) // Falls back to copy

	// Verify symlink preserved
	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target.txt", target)
}

func TestJuiceFSEngine_Clone_NotOnJuiceFS(t *testing.T) {
	// Verify fallback when not on JuiceFS
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded)
	// Should report "not-on-juicefs" or "juicefs-not-available" degradation
	assert.NotEmpty(t, result.Degradations)

	// Verify content was copied via fallback
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestJuiceFSEngine_Clone_JuiceFSNotAvailable(t *testing.T) {
	// Set environment to simulate juicefs not being available
	os.Setenv("JVS_ENGINE", "juicefs")
	defer os.Unsetenv("JVS_ENGINE")

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should fall back to copy since juicefs is not available
	assert.True(t, result.Degraded)
	assert.NotEmpty(t, result.Degradations)

	// Verify content was copied
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

func TestJuiceFSEngine_DetectEngine_JuiceFSAvailable(t *testing.T) {
	// When juicefs is available (simulated by setting env)
	os.Setenv("JVS_ENGINE", "juicefs")
	defer os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, model.EngineJuiceFSClone, eng.Name())
}

func TestJuiceFSEngine_DetectEngine_PrefersJuiceFS(t *testing.T) {
	// Without environment override, should try to detect JuiceFS first
	tmpDir := t.TempDir()

	// Clear env
	os.Unsetenv("JVS_ENGINE")

	eng, err := engine.DetectEngine(tmpDir)
	require.NoError(t, err)
	// Since JuiceFS isn't actually mounted, will fall back to reflink or copy
	assert.NotNil(t, eng)
}
