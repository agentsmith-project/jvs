package engine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJuiceFSEngine_MockBinaryWithSuccess tests with mock juicefs that succeeds.
func TestJuiceFSEngine_MockBinaryWithSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "juicefs")

	// Create a mock that "succeeds" - actually needs to do the copy
	// Since we can't actually implement juicefs clone, we make it fail
	// but the fact that it exists triggers the available path
	script := `#!/bin/sh
# Mock juicefs clone - we'll make it fail to test fallback
# But first, the code will check isOnJuiceFS which will fail
# (since we're not actually on a JuiceFS mount)
exit 1
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Skip("cannot create mock juicefs binary")
	}

	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	// Now juicefs is "available"
	// Verify that exec.LookPath finds it
	_, err := exec.LookPath("juicefs")
	require.NoError(t, err, "juicefs should be in PATH")

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	// Should be degraded because either:
	// 1. Not on JuiceFS (most likely)
	// 2. Clone failed
	assert.True(t, result.Degraded)

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

// TestJuiceFSEngine_VerifyDegradationReasons checks degradation reasons.
func TestJuiceFSEngine_VerifyDegradationReasons(t *testing.T) {
	// First verify juicefs is NOT installed
	_, err := exec.LookPath("juicefs")
	if err == nil {
		t.Skip("juicefs is installed, cannot test unavailable path")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded)

	// Should have "juicefs-not-available" since juicefs isn't installed
	found := false
	for _, deg := range result.Degradations {
		if deg == "juicefs-not-available" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have juicefs-not-available degradation when juicefs is not installed")

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestJuiceFSEngine_WithMockBinary tests with mock binary.
func TestJuiceFSEngine_WithMockBinary(t *testing.T) {
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "juicefs")

	// Create mock that exits with error
	script := `#!/bin/sh
exit 1
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Skip("cannot create mock juicefs binary")
	}

	oldPath := os.Getenv("PATH")
	newPath := tmpDir + string(os.PathListSeparator) + oldPath
	os.Setenv("PATH", newPath)
	defer os.Setenv("PATH", oldPath)

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("test"), 0644)

	eng := engine.NewJuiceFSEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)
	assert.True(t, result.Degraded)

	// With mock binary present but not on actual JuiceFS mount,
	// should get "not-on-juicefs" degradation
	// OR if the clone command runs and fails, "juicefs-clone-failed"
	validDegradations := []string{"not-on-juicefs", "juicefs-clone-failed"}
	found := false
	for _, deg := range result.Degradations {
		for _, valid := range validDegradations {
			if deg == valid {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "should have not-on-juicefs or juicefs-clone-failed degradation")

	// Verify fallback worked
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))

	_ = result
}
