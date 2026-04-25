//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 1: New User Onboarding Flow
// User Story: New user initializes repo, creates files, takes snapshot, views history

// TestE2E_Onboarding_NewUserFlow tests the complete onboarding experience
func TestE2E_Onboarding_NewUserFlow(t *testing.T) {
	// Create temp directory for the new user's project
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "myproject")

	// Step 1: Initialize repository
	t.Run("init_repo", func(t *testing.T) {
		stdout, stderr, code := runJVS(t, dir, "init", "myproject")
		if code != 0 {
			t.Fatalf("jvs init failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Initialized") && !strings.Contains(stdout, "created") {
			t.Errorf("expected init success message, got: %s", stdout)
		}

		// Verify .jvs/ directory exists
		jvsPath := filepath.Join(projectPath, ".jvs")
		if _, err := os.Stat(jvsPath); os.IsNotExist(err) {
			t.Error(".jvs/ directory should exist")
		}

		// Verify main/ directory exists
		mainPath := filepath.Join(projectPath, "main")
		if _, err := os.Stat(mainPath); os.IsNotExist(err) {
			t.Error("main/ directory should exist")
		}
	})

	// Step 2: Create initial files
	t.Run("create_files", func(t *testing.T) {
		mainPath := filepath.Join(projectPath, "main")

		// Create README.md
		createFiles(t, mainPath, map[string]string{
			"README.md": "Hello JVS\n",
		})

		// Create src directory and file
		createFiles(t, mainPath, map[string]string{
			"src/main.go": "package main\n",
		})

		// Verify files exist
		if !fileExists(t, filepath.Join(mainPath, "README.md")) {
			t.Error("README.md should exist")
		}
		if !fileExists(t, filepath.Join(mainPath, "src", "main.go")) {
			t.Error("src/main.go should exist")
		}
	})

	// Step 3: Take initial snapshot
	var snapshotID string
	t.Run("create_snapshot", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, projectPath, "checkpoint", "initial commit")
		if code != 0 {
			t.Fatalf("snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected 'Created checkpoint' in output, got: %s", stdout)
		}

		// Extract snapshot ID for later use
		snapshotID = extractSnapshotIDFromOutput(stdout)
		if snapshotID == "" {
			t.Log("Warning: could not extract snapshot ID from output")
		}
	})

	// Step 4: View history
	t.Run("view_history", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, projectPath, "checkpoint", "list")
		if code != 0 {
			t.Fatalf("history failed: %s", stderr)
		}
		if !strings.Contains(stdout, "initial commit") {
			t.Errorf("expected 'initial commit' in history, got: %s", stdout)
		}

		// Count snapshots - should be exactly one
		jsonOut, _, _ := runJVSInRepo(t, projectPath, "checkpoint", "list", "--json")
		count := getSnapshotCount(jsonOut)
		if count != 1 {
			t.Errorf("expected exactly 1 snapshot, got %d", count)
		}
	})

	// Step 5: View info
	t.Run("view_info", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, projectPath, "info")
		if code != 0 {
			t.Fatalf("info failed: %s", stderr)
		}
		// Info should show repository information
		if !strings.Contains(stdout, "Repository") && !strings.Contains(stdout, "repo") {
			t.Errorf("expected repository info, got: %s", stdout)
		}

		// Check JSON output for total_snapshots
		jsonOut, _, _ := runJVSInRepo(t, projectPath, "info", "--json")
		if !strings.Contains(jsonOut, "total_snapshots") && !strings.Contains(jsonOut, "checkpoint") {
			t.Logf("Warning: info JSON may not contain snapshot count: %s", jsonOut)
		}
	})

	// Step 6: Run doctor
	t.Run("run_doctor", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, projectPath, "doctor")
		if code != 0 {
			t.Fatalf("doctor failed: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected 'healthy' in doctor output, got: %s", stdout)
		}
	})
}

// extractSnapshotIDFromOutput extracts snapshot ID from command output
func extractSnapshotIDFromOutput(output string) string {
	// Look for patterns like "Created checkpoint abc123" or "checkpoint_id": "abc123"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created checkpoint") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "checkpoint" && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}
	return ""
}
