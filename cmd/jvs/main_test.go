package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getProjectRoot returns the absolute path to the project root.
func getProjectRoot(t *testing.T) string {
	dir, err := os.Getwd()
	require.NoError(t, err)
	// Walk up to find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}

// TestExecute verifies that main() executes correctly.
func TestExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs-test")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Test that binary exists and is executable
	info, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "binary should be executable")
}

// TestMainHelpFlag tests that the help flag works.
func TestMainHelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs-test")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Run with --help
	cmd := exec.Command(binPath, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "JVS")
	assert.Contains(t, string(out), "save points")
}

// TestMainVersion tests that version/help output works.
func TestMainVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs-test")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Run with --help to see version info
	cmd := exec.Command(binPath, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.NotEmpty(t, string(out))
}

// TestMainUnknownCommand tests error handling for unknown commands.
func TestMainUnknownCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs-test")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Run with unknown command
	cmd := exec.Command(binPath, "unknown-command-xyz")
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(string(out)), "unknown")
}

// TestMainEntryPoints tests that the main function is properly defined.
func TestMainEntryPoints(t *testing.T) {
	// This is a compile-time test to ensure main() exists
	_ = main
}

// TestBinaryExecutionIntegration is an integration test.
func TestBinaryExecutionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Test init command
	cmd := exec.Command(binPath, "init", "test")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "init failed: %s", string(out))
	assert.Contains(t, string(out), "JVS is ready for this folder.")

	// Verify repo was created
	repoPath := filepath.Join(tmpDir, "test")
	_, err = os.Stat(filepath.Join(repoPath, ".jvs"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(repoPath, "main"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Test status command
	cmd = exec.Command(binPath, "status")
	cmd.Dir = repoPath
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "Folder:")
	assert.Contains(t, string(out), "Workspace: main")
	assert.Contains(t, string(out), "Newest save point: none")
}

// TestBinaryJSONOutput tests JSON output format.
func TestBinaryJSONOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Init repo
	cmd := exec.Command(binPath, "init", "test")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "init failed: %s", string(out))
	require.Contains(t, string(out), "JVS is ready for this folder.")

	// Test JSON output
	cmd = exec.Command(binPath, "--json", "status")
	cmd.Dir = filepath.Join(tmpDir, "test")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "{")
	assert.Contains(t, string(out), `"command": "status"`)
	assert.Contains(t, string(out), "repo_root")
	assert.Contains(t, string(out), "files_state")
}

// TestBinaryErrorHandling tests error messages.
func TestBinaryErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Run a repo-scoped command outside of repo (should fail)
	cmd := exec.Command(binPath, "status")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(string(out)), "not a jvs repository")
}

// TestBinarySaveHistoryFlow tests a complete save point workflow.
func TestBinarySaveHistoryFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "jvs")
	jvsDir := filepath.Join(getProjectRoot(t), "cmd", "jvs")

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Dir = jvsDir
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(output))

	// Init repo
	cmd := exec.Command(binPath, "init", "test")
	cmd.Dir = tmpDir
	_, err = cmd.CombinedOutput()
	require.NoError(t, err)

	repoPath := filepath.Join(tmpDir, "test")

	// Create a file
	testFile := filepath.Join(repoPath, "test.txt")
	err = os.WriteFile(testFile, []byte("hello world"), 0644)
	require.NoError(t, err)

	// Create save point
	cmd = exec.Command(binPath, "save", "-m", "test save point")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "Saved save point")
	assert.Contains(t, string(out), "Message: test save point")
	assert.Contains(t, string(out), "Newest save point:")

	// Check save point history
	cmd = exec.Command(binPath, "history")
	cmd.Dir = repoPath
	out, err = cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "Save points")
	assert.Contains(t, string(out), "test save point")
	assert.NotContains(t, strings.ToLower(string(out)), "checkpoint")
	assert.NotContains(t, strings.ToLower(string(out)), "snapshot")
}
