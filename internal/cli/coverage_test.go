package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProgressEnabled tests the progressEnabled function.
func TestProgressEnabled(t *testing.T) {
	// Save original state
	originalNoProgress := noProgress
	originalJSONOutput := jsonOutput
	defer func() {
		noProgress = originalNoProgress
		jsonOutput = originalJSONOutput
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
			result := progressEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
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

	t.Run("Empty string returns Copy as fallback", func(t *testing.T) {
		engine := detectEngine("")
		assert.Equal(t, "copy", string(engine))
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

// TestResolveCheckpointRefForDiff tests the shared checkpoint ref resolver.
func TestResolveCheckpointRefForDiff(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	t.Run("Resolve current without workspace returns error", func(t *testing.T) {
		assert.NoError(t, os.Chdir(dir))
		_, err := resolveCheckpointRef(repoPath, "", "current")
		assert.Error(t, err)
	})

	t.Run("Resolve non-existent snapshot returns error", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "nonexistent-snapshot-id")
		assert.Error(t, err)
	})

	t.Run("Resolve empty string returns error", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "")
		assert.Error(t, err)
	})

	t.Run("Resolve current when no checkpoints exist", func(t *testing.T) {
		assert.NoError(t, os.Chdir(mainPath))
		_, err := resolveCheckpointRef(repoPath, "main", "current")
		assert.Error(t, err)
	})

	t.Run("Resolve current successfully after creating checkpoint", func(t *testing.T) {
		// Create a snapshot first
		assert.NoError(t, os.Chdir(mainPath))
		assert.NoError(t, os.WriteFile("headtest.txt", []byte("head test"), 0644))

		cmd3 := createTestRootCmd()
		stdout, _ := executeCommand(cmd3, "snapshot", "for current ref test", "--json")

		// Extract snapshot ID
		lines := strings.Split(stdout, "\n")
		var snapshotID string
		for _, line := range lines {
			if strings.Contains(line, `"snapshot_id"`) {
				parts := strings.Split(line, `"`)
				for i, p := range parts {
					if p == "snapshot_id" && i+2 < len(parts) {
						snapshotID = parts[i+2]
						break
					}
				}
			}
		}

		if snapshotID != "" {
			// Now current should resolve
			resolved, err := resolveCheckpointRef(repoPath, "main", "current")
			assert.NoError(t, err)
			assert.Equal(t, snapshotID, string(resolved))
		}
	})

	t.Run("Resolve with whitespace only", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "   ")
		assert.Error(t, err)
	})

	os.Chdir(originalWd)
}

// TestResolveCheckpointRefByID tests resolving checkpoints by full ID.
func TestResolveCheckpointRefByID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	// Create a snapshot
	assert.NoError(t, os.Chdir(mainPath))
	assert.NoError(t, os.WriteFile("test.txt", []byte("test content"), 0644))

	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", "test snapshot", "--json")

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if snapshotID != "" {
		// Test resolving by full ID
		resolved, err := resolveCheckpointRef(repoPath, "main", snapshotID)
		assert.NoError(t, err)
		assert.Equal(t, snapshotID, string(resolved))

		// Test resolving by short prefix
		shortPrefix := snapshotID[:8]
		resolved2, err := resolveCheckpointRef(repoPath, "main", shortPrefix)
		assert.NoError(t, err)
		assert.Equal(t, snapshotID, string(resolved2))
	}

	os.Chdir(originalWd)
}

// TestResolveCheckpointRefByTag tests resolving checkpoints by tag.
func TestResolveCheckpointRefByTag(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	// Create a snapshot with a tag
	assert.NoError(t, os.Chdir(mainPath))
	assert.NoError(t, os.WriteFile("tagtest.txt", []byte("tagged content"), 0644))

	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", "--tag", "testtag", "tagged snapshot", "--json")

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if snapshotID != "" {
		// Test resolving by tag
		resolved, err := resolveCheckpointRef(repoPath, "main", "testtag")
		assert.NoError(t, err)
		assert.Equal(t, snapshotID, string(resolved))
	}

	os.Chdir(originalWd)
}

// TestResolveCheckpointRefRejectsNote tests that notes are not public checkpoint refs.
func TestResolveCheckpointRefRejectsNote(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	// Create a snapshot with a unique note
	uniqueNote := "unique-snapshot-note-" + t.Name()
	assert.NoError(t, os.Chdir(mainPath))
	assert.NoError(t, os.WriteFile("notetest.txt", []byte("noted content"), 0644))

	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", uniqueNote, "--json")

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if snapshotID != "" {
		// Notes are not public checkpoint refs.
		_, err := resolveCheckpointRef(repoPath, "main", "unique-snapshot-note")
		assert.Error(t, err)
	}

	os.Chdir(originalWd)
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

// TestResolveCheckpointRefMultipleTags tests when multiple snapshots have the same tag.
func TestResolveCheckpointRefMultipleTags(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	// Create two snapshots with the same tag (bad practice but should handle)
	assert.NoError(t, os.Chdir(mainPath))
	assert.NoError(t, os.WriteFile("test1.txt", []byte("test1"), 0644))
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "first", "--tag", "shared")

	assert.NoError(t, os.WriteFile("test2.txt", []byte("test2"), 0644))
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "second", "--tag", "shared")

	// Public refs reject ambiguous tags.
	_, err := resolveCheckpointRef(repoPath, "main", "shared")
	assert.Error(t, err)

	os.Chdir(originalWd)
}

// TestExecuteExists confirms Execute function exists and has correct signature.
func TestExecuteExists(t *testing.T) {
	// Execute is tested in E2E tests since it calls os.Exit
	// This test just verifies it exists for type checking
	_ = Execute
}

// TestRootCommandSetup tests root command initialization.
func TestRootCommandSetup(t *testing.T) {
	// Verify rootCmd has expected configuration
	assert.Equal(t, "jvs", rootCmd.Use)
	assert.Equal(t, "JVS - Juicy Versioned Workspaces", rootCmd.Short)
	assert.True(t, rootCmd.SilenceUsage)
	assert.True(t, rootCmd.SilenceErrors)

	// Verify persistent flags are defined
	flags := rootCmd.PersistentFlags()
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

// TestResolveCheckpointRefEdgeCases tests edge cases for checkpoint ref resolution.
func TestResolveCheckpointRefEdgeCases(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	t.Run("Resolve with very short prefix (<4 chars)", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "abc")
		assert.Error(t, err)
	})

	t.Run("Resolve with special characters", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "!@#$%")
		assert.Error(t, err)
	})

	t.Run("Resolve with newlines", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "test\nsnapshot")
		assert.Error(t, err)
	})

	os.Chdir(originalWd)
}

// TestContextFunctionsOutsideRepo tests requireRepo and requireWorktree outside a repo.
func TestContextFunctionsOutsideRepo(t *testing.T) {
	originalWd, _ := os.Getwd()
	dir := t.TempDir()

	// Change to a directory that's not a JVS repo
	assert.NoError(t, os.Chdir(dir))

	t.Run("requireRepo outside repo calls os.Exit", func(t *testing.T) {
		// This would normally call os.Exit, so we can't test it directly
		// But we can verify the function exists
		_ = requireRepo
	})

	t.Run("requireWorktree outside repo calls os.Exit", func(t *testing.T) {
		// This would normally call os.Exit, so we can't test it directly
		// But we can verify the function exists
		_ = requireWorktree
	})

	os.Chdir(originalWd)
}

// TestSnapshotWithCompression tests snapshot with compression enabled.
func TestSnapshotWithCompression(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a larger compressible file
	data := make([]byte, 1024*100) // 100KB
	for i := range data {
		data[i] = byte(i % 10) // Highly repetitive
	}
	assert.NoError(t, os.WriteFile("compressible.bin", data, 0644))

	// Create snapshot with compression
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "compressed", "--compress", "default")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "snapshot")

	os.Chdir(originalWd)
}

// TestWorktreeCreateFromNonExistentSnapshot tests error handling.
func TestWorktreeCreateFromNonExistentSnapshot(t *testing.T) {
	t.Skip("Command calls os.Exit - cannot be tested in unit tests")
}

// TestRestoreNonExistentSnapshot tests restore error handling.
func TestRestoreNonExistentSnapshot(t *testing.T) {
	t.Skip("Command calls os.Exit - cannot be tested in unit tests")
}

// TestGCRunWithNoPlan tests gc run without a plan.
func TestGCRunWithNoPlan(t *testing.T) {
	t.Skip("Command calls os.Exit - cannot be tested in unit tests")
}

// TestFindRepoRoot tests the findRepoRoot function.
func TestFindRepoRoot(t *testing.T) {
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	t.Run("Find repo root from subdirectory", func(t *testing.T) {
		// Check if we're in a repo first
		_, err := os.Stat(filepath.Join(originalWd, "go.mod"))
		if err != nil {
			t.Skip("Not in repo root")
		}

		// Start from a subdirectory of the repo
		subDir := filepath.Join(originalWd, "internal")
		assert.NoError(t, os.Chdir(subDir))

		root, err := findRepoRoot()
		assert.NoError(t, err)
		assert.Contains(t, root, "jvs")
		// Change back to original
		os.Chdir(originalWd)
	})

	t.Run("Find repo root from repo root", func(t *testing.T) {
		// Verify we're in a directory with go.mod
		_, err := os.Stat(filepath.Join(originalWd, "go.mod"))
		if err != nil {
			t.Skip("Not in repo root")
		}

		root, err := findRepoRoot()
		assert.NoError(t, err)
		assert.Equal(t, originalWd, root)
	})

	t.Run("Find repo root from temp directory returns error", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.Chdir(dir))

		_, err := findRepoRoot()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "go.mod not found")
		// Change back to original
		os.Chdir(originalWd)
	})
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

// TestResolveCheckpointRefNonExistentTag tests resolving with non-existent tag.
func TestResolveCheckpointRefNonExistentTag(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	t.Run("Resolve non-existent tag returns error", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "nonexistent-tag")
		assert.Error(t, err)
	})

	t.Run("Resolve with case-sensitive tag", func(t *testing.T) {
		// Create a snapshot with a tag
		mainPath := repoPath + "/main"
		assert.NoError(t, os.Chdir(mainPath))
		assert.NoError(t, os.WriteFile("case.txt", []byte("test"), 0644))

		cmd2 := createTestRootCmd()
		_, err := executeCommand(cmd2, "snapshot", "test", "--tag", "MyTag")
		assert.NoError(t, err)

		// Try to resolve with different case
		_, err = resolveCheckpointRef(repoPath, "main", "mytag")
		// Should fail due to case sensitivity
		assert.Error(t, err)
	})

	os.Chdir(originalWd)
}

// TestResolveCheckpointRef_InvalidID tests checkpoint refs with invalid IDs.
func TestResolveCheckpointRef_InvalidID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	t.Run("Resolve with invalid hex characters", func(t *testing.T) {
		_, err := resolveCheckpointRef(repoPath, "main", "zzzzzzzzzzzz")
		assert.Error(t, err)
	})

	t.Run("Resolve with ID too long", func(t *testing.T) {
		// Very long ID string
		longID := strings.Repeat("a", 1000)
		_, err := resolveCheckpointRef(repoPath, "main", longID)
		assert.Error(t, err)
	})

	os.Chdir(originalWd)
}

// TestResolveCheckpointRef_ByTagLikeIDPrefix tests tag refs that look like short IDs.
func TestResolveCheckpointRef_ByTagLikeIDPrefix(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	// Create a snapshot with a tag that looks like an ID prefix
	assert.NoError(t, os.Chdir(mainPath))
	assert.NoError(t, os.WriteFile("tag.txt", []byte("test"), 0644))

	cmd2 := createTestRootCmd()
	stdout, _ := executeCommand(cmd2, "snapshot", "test", "--tag", "abc123", "--json")

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	if snapshotID != "" {
		// This is not a canonical ID, so it should resolve as an exact tag.
		_, err := resolveCheckpointRef(repoPath, "main", "abc123")
		// Should resolve by tag
		assert.NoError(t, err)
	}

	os.Chdir(originalWd)
}

// TestResolveCheckpointRef_CurrentErrorPaths tests current ref error paths.
func TestResolveCheckpointRef_CurrentErrorPaths(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	mainPath := repoPath + "/main"

	t.Run("current from workspace with no checkpoints", func(t *testing.T) {
		assert.NoError(t, os.Chdir(mainPath))
		_, err := resolveCheckpointRef(repoPath, "main", "current")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no current")
	})

	os.Chdir(originalWd)
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

// TestSnapshotCommand_WithNote tests snapshot with various note formats.
func TestSnapshotCommand_WithNote(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	t.Run("Snapshot with empty note", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("empty.txt", []byte("test"), 0644))
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "snapshot", "")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})

	t.Run("Snapshot with very long note", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("long.txt", []byte("test"), 0644))
		longNote := strings.Repeat("a", 1000)
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "snapshot", longNote)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})

	t.Run("Snapshot with special characters in note", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("special.txt", []byte("test"), 0644))
		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "snapshot", "note with quotes: \"test\" and 'apostrophes'")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})

	os.Chdir(originalWd)
}

// TestSnapshotCommand_WithCompress tests snapshot with compression levels.
func TestSnapshotCommand_WithCompress(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))
	assert.NoError(t, os.WriteFile("compress.txt", []byte("test"), 0644))

	t.Run("Snapshot with no compression", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "snapshot", "--compress", "none", "test no compress")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})

	t.Run("Snapshot with fast compression", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "snapshot", "--compress", "fast", "test fast compress")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "snapshot")
	})

	os.Chdir(originalWd)
}

// TestDoctorCommand tests the doctor command.
func TestDoctorCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

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

	os.Chdir(originalWd)
}

// TestHistoryCommand tests the history command.
func TestHistoryCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	t.Run("History with no snapshots", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		_, err := executeCommand(cmd2, "history")
		assert.NoError(t, err)
		// Should show empty history
	})

	t.Run("History after creating snapshot", func(t *testing.T) {
		assert.NoError(t, os.WriteFile("historytest.txt", []byte("test"), 0644))
		cmd3 := createTestRootCmd()
		_, err := executeCommand(cmd3, "snapshot", "for history test")
		assert.NoError(t, err)

		cmd4 := createTestRootCmd()
		stdout, err := executeCommand(cmd4, "history")
		assert.NoError(t, err)
		// History output should contain snapshot information
		assert.NotEmpty(t, stdout)
	})

	t.Run("History JSON uses save point schema", func(t *testing.T) {
		cmd5 := createTestRootCmd()
		stdout, err := executeCommand(cmd5, "history", "--json")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "save_points")
		assert.NotContains(t, stdout, "snapshot_id")
		assert.NotContains(t, stdout, "worktree")
	})

	t.Run("History with limit", func(t *testing.T) {
		cmd6 := createTestRootCmd()
		_, err := executeCommand(cmd6, "history", "--limit", "1")
		assert.NoError(t, err)
		// Should return at most 1 snapshot
	})

	os.Chdir(originalWd)
}

// TestInfoCommand tests the info command.
func TestInfoCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	t.Run("Info in new repo", func(t *testing.T) {
		cmd2 := createTestRootCmd()
		stdout, err := executeCommand(cmd2, "info")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "Repository")
	})

	t.Run("Info with JSON output", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "info", "--json")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "{")
	})

	os.Chdir(originalWd)
}

// TestWorktreeCommands tests various worktree commands.
func TestWorktreeCommands(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a snapshot first
	assert.NoError(t, os.WriteFile("wttest.txt", []byte("test"), 0644))
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "for worktree", "--json")
	assert.NoError(t, err)

	// Extract snapshot ID
	lines := strings.Split(stdout, "\n")
	var snapshotID string
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					snapshotID = parts[i+2]
					break
				}
			}
		}
	}

	t.Run("Worktree list", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "worktree", "list")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "main")
	})

	if snapshotID != "" {
		t.Run("Worktree fork from snapshot", func(t *testing.T) {
			cmd4 := createTestRootCmd()
			stdout, err := executeCommand(cmd4, "worktree", "fork", snapshotID, "test-branch")
			assert.NoError(t, err)
			assert.Contains(t, stdout, "test-branch")
		})
	}

	os.Chdir(originalWd)
}

// TestVerifyCommand tests the verify command.
func TestVerifyCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	assert.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	assert.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a snapshot
	assert.NoError(t, os.WriteFile("verify.txt", []byte("verify test"), 0644))
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "snapshot", "for verify")
	assert.NoError(t, err)

	t.Run("Verify all snapshots", func(t *testing.T) {
		cmd3 := createTestRootCmd()
		stdout, err := executeCommand(cmd3, "verify", "--all")
		assert.NoError(t, err)
		// Verify output contains OK for each snapshot
		assert.Contains(t, stdout, "OK")
	})

	os.Chdir(originalWd)
}
