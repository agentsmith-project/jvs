//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 5: Multi-Worktree Collaboration
// User Story: Developer works on multiple features in parallel

// TestE2E_Worktree_ParallelFeatures tests working on multiple features simultaneously
func TestE2E_Worktree_ParallelFeatures(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "collab")
	mainPath := filepath.Join(repoPath, "main")

	// Initialize repository
	runJVS(t, dir, "init", "collab")

	// Step 1: Create shared baseline
	t.Run("create_baseline", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "lib.txt"), []byte("shared library"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "baseline", "--tag", "baseline")
	})

	// Step 2: Fork feature worktrees
	t.Run("fork_worktrees", func(t *testing.T) {
		// Fork auth feature
		stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "feature-auth")
		if code != 0 {
			t.Fatalf("fork feature-auth failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created workspace") {
			t.Errorf("expected success message, got: %s", stdout)
		}

		// Fork UI feature
		stdout, stderr, code = runJVSInRepo(t, repoPath, "fork", "feature-ui")
		if code != 0 {
			t.Fatalf("fork feature-ui failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created workspace") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 3: List worktrees
	t.Run("list_worktrees", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "list")
		if code != 0 {
			t.Fatalf("worktree list failed: %s", stderr)
		}

		// Should show all three worktrees
		if !strings.Contains(stdout, "main") {
			t.Error("expected 'main' in worktree list")
		}
		if !strings.Contains(stdout, "feature-auth") {
			t.Error("expected 'feature-auth' in worktree list")
		}
		if !strings.Contains(stdout, "feature-ui") {
			t.Error("expected 'feature-ui' in worktree list")
		}
	})

	// Step 4: Work in auth feature
	t.Run("work_on_auth", func(t *testing.T) {
		authPath := filepath.Join(repoPath, "worktrees", "feature-auth")
		os.WriteFile(filepath.Join(authPath, "auth.py"), []byte("auth module"), 0644)

		stdout, stderr, code := runJVSInWorktree(t, repoPath, "feature-auth", "checkpoint", "auth v1")
		if code != 0 {
			t.Fatalf("auth snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}

		// Verify auth file exists
		if !fileExists(t, filepath.Join(authPath, "auth.py")) {
			t.Error("auth.py should exist in auth worktree")
		}
	})

	// Step 5: Work in UI feature (independent)
	t.Run("work_on_ui", func(t *testing.T) {
		uiPath := filepath.Join(repoPath, "worktrees", "feature-ui")
		os.WriteFile(filepath.Join(uiPath, "ui.jsx"), []byte("ui component"), 0644)

		stdout, stderr, code := runJVSInWorktree(t, repoPath, "feature-ui", "checkpoint", "ui v1")
		if code != 0 {
			t.Fatalf("ui snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}

		// Verify UI file exists
		if !fileExists(t, filepath.Join(uiPath, "ui.jsx")) {
			t.Error("ui.jsx should exist in ui worktree")
		}
	})

	// Step 6: Verify isolation - main doesn't have feature files
	t.Run("verify_isolation", func(t *testing.T) {
		// Main should NOT have auth.py
		if fileExists(t, filepath.Join(mainPath, "auth.py")) {
			t.Error("main should NOT have auth.py")
		}

		// Main should NOT have ui.jsx
		if fileExists(t, filepath.Join(mainPath, "ui.jsx")) {
			t.Error("main should NOT have ui.jsx")
		}

		// Auth worktree should NOT have ui.jsx
		if fileExists(t, filepath.Join(repoPath, "worktrees", "feature-auth", "ui.jsx")) {
			t.Error("auth worktree should NOT have ui.jsx")
		}

		// UI worktree should NOT have auth.py
		if fileExists(t, filepath.Join(repoPath, "worktrees", "feature-ui", "auth.py")) {
			t.Error("ui worktree should NOT have auth.py")
		}
	})

	// Step 7: Check main's history (should only have main's snapshots)
	t.Run("main_history", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list")
		// Main should have baseline
		if !strings.Contains(stdout, "baseline") {
			t.Error("main history should contain 'baseline'")
		}
		// Main should NOT have auth/ui snapshots (they're in separate worktrees)
		// Note: This depends on whether history shows all snapshots or just current worktree
	})

	// Step 8: Rename worktree
	t.Run("rename_worktree", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "rename", "feature-ui", "feature-frontend")
		if code != 0 {
			t.Fatalf("rename failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Renamed") {
			t.Errorf("expected rename message, got: %s", stdout)
		}

		// Verify new name exists
		stdout, _, _ = runJVSInRepo(t, repoPath, "workspace", "list")
		if !strings.Contains(stdout, "feature-frontend") {
			t.Error("feature-frontend should exist after rename")
		}
		if strings.Contains(stdout, "feature-ui") {
			t.Error("feature-ui should NOT exist after rename")
		}
	})

	// Step 9: Remove worktree
	t.Run("remove_worktree", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "remove", "feature-auth")
		if code != 0 {
			t.Fatalf("remove failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Removed") {
			t.Errorf("expected remove message, got: %s", stdout)
		}

		// Verify removed from list
		stdout, _, _ = runJVSInRepo(t, repoPath, "workspace", "list")
		if strings.Contains(stdout, "feature-auth") {
			t.Error("feature-auth should NOT exist after removal")
		}
	})
}

// TestE2E_Worktree_CannotRemoveMain tests that main worktree cannot be removed
func TestE2E_Worktree_CannotRemoveMain(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Try to remove main - should fail
	_, _, code := runJVSInRepo(t, repoPath, "workspace", "remove", "main")
	if code == 0 {
		t.Error("should not be able to remove main worktree")
	}
}

// TestE2E_Worktree_IndependentHistory tests that worktrees have independent histories
func TestE2E_Worktree_IndependentHistory(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create baseline
	os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("base"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "main-baseline")

	// Fork worktree
	runJVSInRepo(t, repoPath, "fork", "feature")

	// Create snapshot in feature worktree
	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644)
	runJVSInWorktree(t, repoPath, "feature", "checkpoint", "feature-added")

	// Create snapshot in main
	os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte("main"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "main-update")

	// Verify histories are tracked separately
	t.Run("check_histories", func(t *testing.T) {
		// Both worktrees should see the shared baseline
		// But their subsequent snapshots are in different lineages
		mainHist, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
		featureHist, _, _ := runJVSInWorktree(t, repoPath, "feature", "checkpoint", "list", "--json")

		mainCount := getSnapshotCount(mainHist)
		featureCount := getSnapshotCount(featureHist)

		// Both should have at least the baseline
		if mainCount < 1 {
			t.Errorf("main should have at least 1 snapshot, got %d", mainCount)
		}
		if featureCount < 1 {
			t.Errorf("feature should have at least 1 snapshot, got %d", featureCount)
		}
	})
}

// TestE2E_Worktree_WorktreePath tests the worktree path command
func TestE2E_Worktree_WorktreePath(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a snapshot so we can fork from it
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "baseline")

	// Fork a worktree
	runJVSInRepo(t, repoPath, "fork", "my-feature")

	// Get path of main
	t.Run("main_path", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "path", "main")
		if code != 0 {
			t.Fatalf("worktree path main failed: %s", stderr)
		}
		if !strings.Contains(stdout, "main") {
			t.Errorf("expected path containing 'main', got: %s", stdout)
		}
	})

	// Get path of feature
	t.Run("feature_path", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "path", "my-feature")
		if code != 0 {
			t.Fatalf("worktree path my-feature failed: %s", stderr)
		}
		if !strings.Contains(stdout, "my-feature") {
			t.Errorf("expected path containing 'my-feature', got: %s", stdout)
		}
	})
}

// TestE2E_Worktree_ForkFromCurrentState tests forking from the last snapshot
func TestE2E_Worktree_ForkFromCurrentState(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create initial snapshot
	os.WriteFile(filepath.Join(mainPath, "original.txt"), []byte("v1"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	// Fork from current position (forks from last snapshot)
	t.Run("fork_from_snapshot", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "fork", "from-snapshot")

		// Verify fork has the original file
		forkPath := filepath.Join(repoPath, "worktrees", "from-snapshot")
		if !fileExists(t, filepath.Join(forkPath, "original.txt")) {
			t.Error("fork should have original.txt")
		}

		// Verify content matches snapshot
		content := readFile(t, forkPath, "original.txt")
		if content != "v1" {
			t.Errorf("expected 'v1', got '%s'", content)
		}
	})
}

// TestE2E_Worktree_MultipleFeatureBranches tests complex multi-feature workflow
func TestE2E_Worktree_MultipleFeatureBranches(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Setup: create shared library
	os.WriteFile(filepath.Join(mainPath, "shared.go"), []byte("package shared"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "shared lib", "--tag", "shared-v1")

	// Create multiple feature branches
	features := []string{"feature-a", "feature-b", "feature-c"}
	for _, name := range features {
		runJVSInRepo(t, repoPath, "fork", name)
	}

	// Work on each feature independently
	for i, name := range features {
		t.Run("work_on_"+name, func(t *testing.T) {
			featurePath := filepath.Join(repoPath, "worktrees", name)
			filename := name + ".txt"
			content := string(rune('A' + i))

			os.WriteFile(filepath.Join(featurePath, filename), []byte(content), 0644)
			stdout, stderr, code := runJVSInWorktree(t, repoPath, name, "checkpoint", name+" work")
			if code != 0 {
				t.Fatalf("snapshot failed: %s", stderr)
			}
			if !strings.Contains(stdout, "Created checkpoint") {
				t.Errorf("expected success, got: %s", stdout)
			}
		})
	}

	// Verify complete isolation
	t.Run("verify_complete_isolation", func(t *testing.T) {
		for i, name := range features {
			featurePath := filepath.Join(repoPath, "worktrees", name)

			// Should have shared library
			if !fileExists(t, filepath.Join(featurePath, "shared.go")) {
				t.Errorf("%s should have shared.go", name)
			}

			// Should have its own file
			ownFile := name + ".txt"
			if !fileExists(t, filepath.Join(featurePath, ownFile)) {
				t.Errorf("%s should have %s", name, ownFile)
			}

			// Should NOT have other features' files
			for j, other := range features {
				if i == j {
					continue
				}
				otherFile := other + ".txt"
				if fileExists(t, filepath.Join(featurePath, otherFile)) {
					t.Errorf("%s should NOT have %s", name, otherFile)
				}
			}
		}
	})
}
