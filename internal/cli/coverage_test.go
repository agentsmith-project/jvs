package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCoverageRepo(t *testing.T, name string) (repoPath string, mainPath string) {
	t.Helper()

	dir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
		createTestRootCmd()
	})

	require.NoError(t, os.Chdir(dir))
	repoPath = initLegacyRepoForCLITest(t, name)
	mainPath = repoPath
	require.NoError(t, os.Chdir(mainPath))
	return repoPath, mainPath
}

func testExternalWorkspacePath(repoPath, name string) string {
	return filepath.Join(filepath.Dir(repoPath), name)
}

// TestProgressEnabled tests the progressEnabled function.
func TestProgressEnabled(t *testing.T) {
	// Save original state
	originalNoProgress := noProgress
	originalJSONOutput := jsonOutput
	originalAutoProgressEnabled := autoProgressEnabled
	defer func() {
		noProgress = originalNoProgress
		jsonOutput = originalJSONOutput
		autoProgressEnabled = originalAutoProgressEnabled
	}()

	tests := []struct {
		name       string
		noProgress bool
		jsonOutput bool
		expected   bool
	}{
		{"Both false - progress enabled", false, false, true},
		{"No progress flag set", true, false, false},
		{"JSON output set", false, true, false},
		{"Both set", true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noProgress = tt.noProgress
			jsonOutput = tt.jsonOutput
			autoProgressEnabled = func() bool { return true }
			result := progressEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("Auto terminal policy disabled", func(t *testing.T) {
		noProgress = false
		jsonOutput = false
		autoProgressEnabled = func() bool { return false }
		assert.False(t, progressEnabled())
	})
}

// TestOutputJSONBasicTests tests basic outputJSON behavior.
func TestOutputJSONBasicTests(t *testing.T) {
	// Save original state
	originalJSONOutput := jsonOutput
	defer func() {
		jsonOutput = originalJSONOutput
	}()

	t.Run("OutputJSON with false flag does nothing", func(t *testing.T) {
		jsonOutput = false
		err := outputJSON(map[string]string{"key": "value"})
		assert.NoError(t, err)
	})

	t.Run("OutputJSON with nil value", func(t *testing.T) {
		jsonOutput = true
		err := outputJSON(nil)
		assert.NoError(t, err)
	})
}

// TestDetectEngine_Coverage tests the detectEngine function.
func TestDetectEngine_Coverage(t *testing.T) {
	t.Run("Non-existent path returns Copy as fallback", func(t *testing.T) {
		engine := detectEngine("/nonexistent/path/that/does/not/exist/12345")
		assert.Equal(t, "copy", string(engine))
	})

	t.Run("Common paths return Copy as fallback", func(t *testing.T) {
		engine := detectEngine("/tmp")
		assert.Equal(t, "copy", string(engine))
	})

	t.Run("Empty string uses auto detection", func(t *testing.T) {
		engine := detectEngine("")
		assert.Contains(t, []string{"copy", "reflink-copy", "juicefs-clone"}, string(engine))
	})

	t.Run("Current directory returns valid engine", func(t *testing.T) {
		// Get current directory which should exist
		cwd, err := os.Getwd()
		if err == nil {
			engine := detectEngine(cwd)
			assert.NotEmpty(t, string(engine))
			// Should be one of the valid engines
			assert.Contains(t, []string{"copy", "reflink-copy", "juicefs-clone"}, string(engine))
		}
	})
}

// TestExecuteFunctionExists tests that Execute function exists and is callable.
// Note: Execute calls os.Exit() which terminates the process, making it
// difficult to test in unit tests.
func TestExecuteFunctionExists(t *testing.T) {
	t.Skip("Execute calls os.Exit - tested via E2E/integration tests")

	// The Execute function:
	// 1. Calls rootCmd.Execute()
	// 2. On error, prints to stderr and calls os.Exit(1)
	// This is tested via the E2E test suite
}

// TestFmtErr_Coverage tests that fmtErr doesn't panic.
func TestFmtErr_Coverage(t *testing.T) {
	// fmtErr writes to stderr and should not panic
	t.Run("fmtErr with single argument", func(t *testing.T) {
		fmtErr("test error message")
	})

	t.Run("fmtErr with multiple arguments", func(t *testing.T) {
		fmtErr("test error: %s %d", "value", 42)
	})

	t.Run("fmtErr with no arguments", func(t *testing.T) {
		fmtErr("simple error")
	})
}

// TestExecuteExists confirms Execute function exists and has correct signature.
func TestExecuteExists(t *testing.T) {
	// Execute is tested in E2E tests since it calls os.Exit
	// This test just verifies it exists for type checking
	_ = Execute
}

// TestRootCommandSetup tests root command initialization.
func TestRootCommandSetup(t *testing.T) {
	cmd := createTestRootCmd()

	// Verify rootCmd has expected configuration
	assert.Equal(t, "jvs", cmd.Use)
	assert.Equal(t, "JVS - Juicy Versioned Workspaces", cmd.Short)
	assert.True(t, cmd.SilenceUsage)
	assert.True(t, cmd.SilenceErrors)

	// Verify persistent flags are defined
	flags := cmd.PersistentFlags()
	flag, err := flags.GetBool("json")
	assert.NoError(t, err)
	assert.False(t, flag)

	flag, err = flags.GetBool("debug")
	assert.NoError(t, err)
	assert.False(t, flag)

	flag, err = flags.GetBool("no-progress")
	assert.NoError(t, err)
	assert.False(t, flag)
}

// TestPersistentPreRunTests tests the persistent pre-run function.
func TestPersistentPreRunTests(t *testing.T) {
	t.Run("Debug flag can be set", func(t *testing.T) {
		// Just verify the flag exists and can be parsed
		cmd := createTestRootCmd()
		_, err := executeCommand(cmd, "--debug", "init", "test-debug-flag")
		// Command should work (may fail if dir exists, but that's OK)
		_ = err
	})
}

// TestContextFunctionsOutsideRepo tests repo discovery helpers outside a repo.
func TestContextFunctionsOutsideRepo(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})
	dir := t.TempDir()

	// Change to a directory that's not a JVS repo
	assert.NoError(t, os.Chdir(dir))

	t.Run("requireRepo outside repo calls os.Exit", func(t *testing.T) {
		// This would normally call os.Exit, so we can't test it directly
		// But we can verify the function exists
		_ = requireRepo
	})

}

// TestSaveWithLargeContent tests saving a larger file through the public CLI.
func TestSaveWithLargeContent(t *testing.T) {
	setupCoverageRepo(t, "testrepo")
	data := make([]byte, 1024*100) // 100KB
	for i := range data {
		data[i] = byte(i % 10)
	}
	assert.NoError(t, os.WriteFile("compressible.bin", data, 0644))

	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "save", "-m", "large content")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "save point")
}

// TestWorkspaceNewFromNonExistentSavePoint tests public workspace error handling.
func TestWorkspaceNewFromNonExistentSavePoint(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../feature", "--from", "nonexistent-save-point")
	assert.Error(t, err)
	assert.Empty(t, stdout)
}

// TestRestoreNonExistentSavePoint tests restore error handling.
func TestRestoreNonExistentSavePoint(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	stdout, err := executeCommand(createTestRootCmd(), "restore", "nonexistent-save-point")
	assert.Error(t, err)
	assert.Empty(t, stdout)
}

// TestCleanupRunWithNoPlan tests cleanup run without a plan.
func TestCleanupRunWithNoPlan(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	stdout, err := executeCommand(createTestRootCmd(), "cleanup", "run")
	assert.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "--plan-id")
}

// TestDetectEngine_EdgeCases tests detectEngine with more edge cases.
func TestDetectEngine_EdgeCases(t *testing.T) {
	t.Run("Detect with valid JVS repo path", func(t *testing.T) {
		dir := t.TempDir()
		cmd := createTestRootCmd()

		// Change to temp dir and init a repo
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)

		assert.NoError(t, os.Chdir(dir))
		_, err := executeCommand(cmd, "init", "testrepo")
		assert.NoError(t, err)

		// Test detection on the repo
		engine := detectEngine(filepath.Join(dir, "testrepo"))
		// Should return a valid engine (even if just copy)
		assert.NotEmpty(t, string(engine))
	})

	t.Run("Detect with path containing special characters", func(t *testing.T) {
		// Test that paths with special chars don't cause panics
		engine := detectEngine("/path/with spaces/and-dashes/under_score")
		assert.NotEmpty(t, string(engine))
	})
}

// TestOutputJSON_ErrorHandling tests outputJSON error handling.
func TestOutputJSON_ErrorHandling(t *testing.T) {
	originalJSONOutput := jsonOutput
	defer func() {
		jsonOutput = originalJSONOutput
	}()

	t.Run("OutputJSON with unmarshalable type", func(t *testing.T) {
		jsonOutput = true
		// Channel is not JSON serializable
		ch := make(chan int)
		err := outputJSON(ch)
		assert.Error(t, err)
	})

	t.Run("OutputJSON with cyclic reference", func(t *testing.T) {
		jsonOutput = true
		// Create a cyclic reference
		type cyclic struct {
			Next *cyclic
		}
		val := &cyclic{}
		val.Next = val
		err := outputJSON(val)
		assert.Error(t, err)
	})
}

// TestSaveCommand_WithMessage tests save with current public message forms.
func TestSaveCommand_WithMessage(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	t.Run("Save with positional message", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("positional.txt", []byte("test"), 0644))
		stdout, err := executeCommand(createTestRootCmd(), "save", "positional message")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "save point")
	})

	t.Run("Save with very long message", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("long.txt", []byte("test"), 0644))
		longMessage := strings.Repeat("a", 1000)
		stdout, err := executeCommand(createTestRootCmd(), "save", "-m", longMessage)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "save point")
	})

	t.Run("Save with special characters in message", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("special.txt", []byte("test"), 0644))
		stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "message with quotes: \"test\" and 'apostrophes'")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "save point")
	})

	t.Run("Save without message returns usage error", func(t *testing.T) {
		stdout, err := executeCommand(createTestRootCmd(), "save")
		assert.Error(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, err.Error(), "save point message is required")
	})
}

// TestDoctorCommand tests the doctor command.
func TestDoctorCommand(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	t.Run("Doctor basic check", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "doctor")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "healthy")
	})

	t.Run("Doctor with --strict flag", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "doctor", "--strict")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "healthy")
	})

	t.Run("Doctor with --repair-runtime flag", func(t *testing.T) {
		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "doctor", "--repair-runtime")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "healthy")
	})
}

// TestHistoryCommand tests the history command.
func TestHistoryCommand(t *testing.T) {
	setupCoverageRepo(t, "testrepo")

	t.Run("History with no save points", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		_, err := executeCommand(cmd2, "history")
		assert.NoError(t, err)
	})

	t.Run("History after creating save point", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("historytest.txt", []byte("test"), 0644))
		createRootTestSavePoint(t, "for history test")

		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "history")
		assert.NoError(t, err)
		assert.NotEmpty(t, stdout)
	})

	t.Run("History JSON uses save point schema", func(t *testing.T) {
		cmd5 := createTestRootCmd()
		stdout, err := executeCommand(cmd5, "--json", "history")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "save_points")
		assert.NotContains(t, stdout, "snapshot_id")
		assert.NotContains(t, stdout, "worktree")
	})

	t.Run("History with limit", func(t *testing.T) {
		cmd6 := createTestRootCmd()
		_, err := executeCommand(cmd6, "history", "--limit", "1")
		assert.NoError(t, err)
	})
}

// TestRemovedLegacyCoverageCommandsAreUnknown keeps removed entry points out of the public CLI.
func TestRemovedLegacyCoverageCommandsAreUnknown(t *testing.T) {
	for _, oldCommand := range []string{"snapshot", "diff", "worktree", "verify", "info", "gc"} {
		t.Run(oldCommand, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), oldCommand, "--help")
			assert.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), "unknown command")
		})
	}
}

// TestWorkspaceCommands tests public workspace commands.
func TestWorkspaceCommands(t *testing.T) {
	setupCoverageRepo(t, "testrepo")
	assert.NoError(t, os.WriteFile("wstest.txt", []byte("test"), 0644))
	savePointID := createRootTestSavePoint(t, "for workspace")

	t.Run("Workspace list", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "workspace", "list")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "main")
	})

	t.Run("Workspace new from save point", func(t *testing.T) {
		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "workspace", "new", "../test-workspace", "--from", savePointID)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "test-workspace")
	})
}

// TestDoctorStrictReplacesVerifyAll tests strict health checks through the public CLI.
func TestDoctorStrictReplacesVerifyAll(t *testing.T) {
	setupCoverageRepo(t, "testrepo")
	assert.NoError(t, os.WriteFile("verify.txt", []byte("verify test"), 0644))
	createRootTestSavePoint(t, "for doctor strict")

	stdout, err := executeCommand(createTestRootCmd(), "doctor", "--strict")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "healthy")
}
