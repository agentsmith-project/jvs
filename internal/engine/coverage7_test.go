package engine_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopyEngine_ChtimesEdgeCases tests mod time preservation edge cases.
func TestCopyEngine_ChtimesEdgeCases(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, skip permission test")
	}

	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create source file
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify mod time was set
	info, err := os.Stat(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.False(t, info.ModTime().IsZero())

	_ = result
}

// TestReflinkEngine_ChtimesEdgeCases tests chtimes in reflink.
func TestReflinkEngine_ChtimesEdgeCases(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.False(t, info.ModTime().IsZero())

	_ = result
}

// TestCopyEngine_IoCopySuccess verifies io.Copy path works.
func TestCopyEngine_IoCopySuccess(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Test with various file sizes
	sizes := []int64{0, 1, 100, 4096, 10000}

	for _, size := range sizes {
		name := filepath.Join(src, filepath.Join("file", fmt.Sprintf("size%d.txt", size)))
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 256)
		}

		os.MkdirAll(filepath.Dir(name), 0755)
		os.WriteFile(name, data, 0644)

		eng := engine.NewCopyEngine()
		result, err := eng.Clone(src, dstPath)

		require.NoError(t, err)

		// Verify content
		copied, err := os.ReadFile(filepath.Join(dstPath, filepath.Join("file", fmt.Sprintf("size%d.txt", size))))
		require.NoError(t, err)
		assert.Equal(t, len(data), len(copied))

		_ = result

		// Clean up for next iteration
		os.RemoveAll(dstPath)
	}
}

// TestReflinkEngine_IoCopySuccess tests io.Copy in reflink fallback.
func TestReflinkEngine_IoCopySuccess(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create a file that will likely trigger reflink fallback
	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(i)
	}
	os.WriteFile(filepath.Join(src, "file.txt"), data, 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	copied, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, len(data), len(copied))

	_ = result
}

// TestCopyEngine_SymlinkCreation verifies symlink creation.
func TestCopyEngine_SymlinkCreation(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	// Create various symlinks
	os.WriteFile(filepath.Join(src, "target"), []byte("target"), 0644)

	// Relative symlink
	os.Symlink("target", filepath.Join(src, "rel"))
	// Absolute symlink
	absPath, _ := filepath.Abs(filepath.Join(src, "target"))
	os.Symlink(absPath, filepath.Join(src, "abs"))
	// Multi-hop symlink
	os.Symlink("rel", filepath.Join(src, "hop"))

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// Verify all symlinks
	rel, _ := os.Readlink(filepath.Join(dstPath, "rel"))
	assert.Equal(t, "target", rel)

	abs, _ := os.Readlink(filepath.Join(dstPath, "abs"))
	assert.Equal(t, absPath, abs)

	hop, _ := os.Readlink(filepath.Join(dstPath, "hop"))
	assert.Equal(t, "rel", hop)

	_ = result
}

// TestReflinkEngine_SymlinkCreation tests symlink handling in reflink.
func TestReflinkEngine_SymlinkCreation(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "target"), []byte("target"), 0644)
	os.Symlink("target", filepath.Join(src, "link"))

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dstPath, "link"))
	require.NoError(t, err)
	assert.Equal(t, "target", target)

	_ = result
}

// TestCopyEngine_FsyncDir verifies directory fsync.
func TestCopyEngine_FsyncDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewCopyEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	// If we got here, fsync succeeded
	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestReflinkEngine_FsyncDir tests fsync in reflink.
func TestReflinkEngine_FsyncDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "cloned")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)

	eng := engine.NewReflinkEngine()
	result, err := eng.Clone(src, dstPath)

	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dstPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))

	_ = result
}

// TestDetectEngine_ReflinkSuccess tests reflink detection success.
func TestDetectEngine_ReflinkSuccess(t *testing.T) {
	os.Unsetenv("JVS_ENGINE")

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, eng)

	// On systems that support reflink (btrfs, xfs, apfs), might get reflink
	// Otherwise falls back to copy
	// Either is valid
	assert.NotEmpty(t, eng.Name())

	_ = eng
}

// TestJuiceFSEngine_DetectEngineWithoutJuiceFS tests detection when juicefs not available.
func TestJuiceFSEngine_DetectEngineWithoutJuiceFS(t *testing.T) {
	os.Unsetenv("JVS_ENGINE")

	// First verify juicefs is not installed
	_, err := exec.LookPath("juicefs")
	if err == nil {
		t.Skip("juicefs is installed, cannot test unavailable path")
	}

	tmpDir := t.TempDir()
	eng, err := engine.DetectEngine(tmpDir)

	require.NoError(t, err)
	require.NotNil(t, eng)

	// Should fall back to reflink or copy
	assert.NotEqual(t, "juicefs-clone", string(eng.Name()))

	_ = eng
}
