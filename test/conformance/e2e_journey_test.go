//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 9: Complete User Journey (Integration)
// User Story: Comprehensive test exercising all major features

// TestE2E_Journey_CompleteWorkflow tests a multi-day development workflow
func TestE2E_Journey_CompleteWorkflow(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "myproject")
	mainPath := filepath.Join(repoPath, "main")

	// ===== Day 1: Project Initialization =====
	t.Run("day1_initialization", func(t *testing.T) {
		// Initialize project
		stdout, stderr, code := runJVS(t, dir, "init", "myproject")
		if code != 0 {
			t.Fatalf("init failed: %s", stderr)
		}

		// Verify structure
		if !fileExists(t, filepath.Join(repoPath, ".jvs")) {
			t.Error(".jvs should exist")
		}
		if !fileExists(t, mainPath) {
			t.Error("main should exist")
		}

		// Create initial files
		createFiles(t, mainPath, map[string]string{
			"README.md":         "# My Project\n",
			"src/main.go":       "package main\n\nfunc main() {}\n",
			"src/lib/helper.go": "package lib\n",
		})

		// Initial snapshot
		stdout, _, code = runJVSInRepo(t, repoPath, "checkpoint", "initial commit", "--tag", "v0.1.0")
		if code != 0 {
			t.Fatal("initial snapshot failed")
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success: %s", stdout)
		}

		// Doctor should be healthy
		stdout, _, _ = runJVSInRepo(t, repoPath, "doctor")
		if !strings.Contains(stdout, "healthy") {
			t.Error("should be healthy after init")
		}
	})

	// ===== Day 2-3: Feature Development =====
	t.Run("day2_feature_development", func(t *testing.T) {
		// Fork feature branch
		_, stderr, code := runJVSInRepo(t, repoPath, "fork", "feature-auth")
		if code != 0 {
			t.Fatalf("fork failed: %s", stderr)
		}

		authPath := filepath.Join(repoPath, "worktrees", "feature-auth")

		// Work on auth feature
		createFiles(t, authPath, map[string]string{
			"src/auth/login.go":  "package auth\n\nfunc Login() {}\n",
			"src/auth/logout.go": "package auth\n\nfunc Logout() {}\n",
		})

		// Snapshot auth progress
		_, _, code = runJVSInWorktree(t, repoPath, "feature-auth", "checkpoint", "auth module", "--tag", "auth")
		if code != 0 {
			t.Fatal("auth snapshot failed")
		}

		// Fork another feature
		runJVSInRepo(t, repoPath, "fork", "feature-api")
		apiPath := filepath.Join(repoPath, "worktrees", "feature-api")

		createFiles(t, apiPath, map[string]string{
			"src/api/handler.go": "package api\n\nfunc Handle() {}\n",
		})

		runJVSInWorktree(t, repoPath, "feature-api", "checkpoint", "api module", "--tag", "api")
	})

	// ===== Day 4: Bug Fix in Main =====
	t.Run("day4_bugfix_in_main", func(t *testing.T) {
		// Fix bug in main
		os.WriteFile(filepath.Join(mainPath, "src/lib/helper.go"), []byte("package lib\n\nfunc Help() string { return \"fixed\" }\n"), 0644)

		_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "bugfix: helper function", "--tag", "bugfix")
		if code != 0 {
			t.Fatalf("bugfix snapshot failed: %s", stderr)
		}

		// Continue development
		os.WriteFile(filepath.Join(mainPath, "VERSION"), []byte("0.2.0"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "version bump", "--tag", "v0.2.0")
	})

	// ===== Day 5: Release Branching =====
	t.Run("day5_release_branching", func(t *testing.T) {
		// Create release from current main
		os.WriteFile(filepath.Join(mainPath, "VERSION"), []byte("1.0.0-rc1"), 0644)
		_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "release candidate 1",
			"--tag", "v1.0.0-rc1", "--tag", "rc", "--tag", "release")
		if code != 0 {
			t.Fatalf("release snapshot failed: %s", stderr)
		}

		// Fork release maintenance branch
		runJVSInRepo(t, repoPath, "fork", "release-1.x")
		releasePath := filepath.Join(repoPath, "worktrees", "release-1.x")

		// Final release
		os.WriteFile(filepath.Join(releasePath, "VERSION"), []byte("1.0.0"), 0644)
		runJVSInWorktree(t, repoPath, "release-1.x", "checkpoint", "release 1.0.0",
			"--tag", "v1.0.0", "--tag", "stable", "--tag", "release")
	})

	// ===== Day 6: Hotfix from Release =====
	t.Run("day6_hotfix_from_release", func(t *testing.T) {
		// Simulate production issue - need hotfix from stable
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", "stable")
		if code != 0 {
			t.Fatalf("restore stable failed: %s", stderr)
		}

		// Fork hotfix branch
		_, stderr, code = runJVSInRepo(t, repoPath, "fork", "hotfix-1.0.1")
		if code != 0 {
			t.Fatalf("fork hotfix failed: %s", stderr)
		}

		hotfixPath := filepath.Join(repoPath, "worktrees", "hotfix-1.0.1")

		// Apply hotfix
		os.WriteFile(filepath.Join(hotfixPath, "VERSION"), []byte("1.0.1"), 0644)
		os.WriteFile(filepath.Join(hotfixPath, "src/lib/helper.go"), []byte("package lib\n\nfunc Help() string { return \"hotfixed\" }\n"), 0644)

		_, _, code = runJVSInWorktree(t, repoPath, "hotfix-1.0.1", "checkpoint", "hotfix 1.0.1",
			"--tag", "v1.0.1", "--tag", "hotfix")
		if code != 0 {
			t.Fatal("hotfix snapshot failed")
		}

		// Return main to latest
		_, stderr, code = runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}
	})

	// ===== Day 7: Verification and Cleanup =====
	t.Run("day7_verification_and_cleanup", func(t *testing.T) {
		// List all worktrees
		stdout, _, _ := runJVSInRepo(t, repoPath, "workspace", "list")
		t.Logf("Worktrees: %s", stdout)

		// Complete feature-auth and remove
		runJVSInWorktree(t, repoPath, "feature-auth", "checkpoint", "auth complete", "--tag", "auth", "--tag", "complete")
		runJVSInRepo(t, repoPath, "workspace", "remove", "feature-auth")

		// Complete feature-api and remove
		runJVSInWorktree(t, repoPath, "feature-api", "checkpoint", "api complete", "--tag", "api", "--tag", "complete")
		runJVSInRepo(t, repoPath, "workspace", "remove", "feature-api")

		// Run GC to clean up orphaned snapshots
		planOut, _, _ := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
		planID := extractPlanID(planOut)
		if planID == "" {
			planID = extractPlanIDFromText(planOut)
		}
		if planID != "" {
			runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", planID)
		}
	})

	// ===== Final Verification =====
	t.Run("final_verification", func(t *testing.T) {
		// Verify all remaining snapshots
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify failed: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK in verify: %s", stdout)
		}

		// Doctor should report healthy
		stdout, stderr, code = runJVSInRepo(t, repoPath, "doctor", "--strict")
		if code != 0 {
			t.Fatalf("doctor failed: stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected healthy: %s", stdout)
		}

		// History should show our work
		stdout, _, _ = runJVSInRepo(t, repoPath, "checkpoint", "list")
		t.Logf("Final history:\n%s", stdout)
	})
}

// TestE2E_Journey_RestoreScenarios tests various restore scenarios
func TestE2E_Journey_RestoreScenarios(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create timeline of snapshots
	versions := []struct {
		content string
		note    string
		tag     string
	}{
		{"v1.0", "initial", "v1.0"},
		{"v1.1", "minor update", "v1.1"},
		{"v2.0", "major update", "v2.0"},
		{"v2.1", "patch", "v2.1"},
		{"v3.0", "next major", "v3.0"},
	}

	for _, v := range versions {
		os.WriteFile(filepath.Join(mainPath, "version.txt"), []byte(v.content), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", v.note, "--tag", v.tag)
	}

	// Get snapshot IDs
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	ids := extractAllSnapshotIDs(stdout)

	// Test restore by tag
	t.Run("restore_by_tag", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "restore", "v2.0")
		if code != 0 {
			t.Fatal("restore by tag failed")
		}
		content := readFile(t, mainPath, "version.txt")
		if content != "v2.0" {
			t.Errorf("expected v2.0, got: %s", content)
		}
	})

	// Test restore by ID
	t.Run("restore_by_id", func(t *testing.T) {
		if len(ids) < 3 {
			t.Skip("not enough snapshots")
		}
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest") // Reset first
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}

		_, _, code = runJVSInRepo(t, repoPath, "restore", ids[2])
		if code != 0 {
			t.Fatal("restore by ID failed")
		}
	})

	// Test restore latest
	t.Run("restore_latest", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Fatal("restore latest failed")
		}
		content := readFile(t, mainPath, "version.txt")
		if content != "v3.0" {
			t.Errorf("expected v3.0, got: %s", content)
		}
	})
}

// TestE2E_Journey_TagOperations tests comprehensive tag operations
func TestE2E_Journey_TagOperations(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots with various tag patterns
	os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("1.0.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "release 1.0", "--tag", "v1.0.0", "--tag", "v1", "--tag", "release")

	os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("1.1.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "release 1.1", "--tag", "v1.1.0", "--tag", "v1", "--tag", "release")

	os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("2.0.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "release 2.0", "--tag", "v2.0.0", "--tag", "v2", "--tag", "release", "--tag", "current-release")

	// Filter by different tags
	t.Run("filter_by_release", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "release")
		if count != 3 {
			t.Errorf("expected 3 releases, got %d", count)
		}
	})

	t.Run("filter_by_v1", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "v1")
		if count != 2 {
			t.Errorf("expected 2 v1 releases, got %d", count)
		}
	})

	t.Run("filter_by_current_release", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "current-release")
		if count != 1 {
			t.Errorf("expected 1 current-release, got %d", count)
		}
	})

	// Fork from tag
	t.Run("fork_from_tag", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "fork", "v1.1.0", "maintain-1.x")
		if code != 0 {
			t.Fatal("fork from tag failed")
		}

		content := readFile(t, filepath.Join(repoPath, "worktrees", "maintain-1.x"), "app.txt")
		if content != "1.1.0" {
			t.Errorf("expected 1.1.0, got: %s", content)
		}
	})
}

// TestE2E_Journey_CompleteWorktreeLifecycle tests the complete worktree lifecycle
func TestE2E_Journey_CompleteWorktreeLifecycle(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Setup
	os.WriteFile(filepath.Join(mainPath, "base.txt"), []byte("base"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "base")

	// Create worktrees
	t.Run("create_worktrees", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "fork", "feature-a")
		runJVSInRepo(t, repoPath, "fork", "feature-b")
		runJVSInRepo(t, repoPath, "fork", "feature-c")

		// Verify list
		stdout, _, _ := runJVSInRepo(t, repoPath, "workspace", "list")
		if !strings.Contains(stdout, "feature-a") || !strings.Contains(stdout, "feature-b") || !strings.Contains(stdout, "feature-c") {
			t.Error("all worktrees should be listed")
		}
	})

	// Work in each
	t.Run("work_in_parallel", func(t *testing.T) {
		for _, name := range []string{"feature-a", "feature-b", "feature-c"} {
			path := filepath.Join(repoPath, "worktrees", name)
			os.WriteFile(filepath.Join(path, name+".txt"), []byte(name), 0644)
			runJVSInWorktree(t, repoPath, name, "checkpoint", name+" work")
		}
	})

	// Rename
	t.Run("rename_worktree", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "workspace", "rename", "feature-c", "feature-final")
		if code != 0 {
			t.Fatal("rename failed")
		}

		stdout, _, _ := runJVSInRepo(t, repoPath, "workspace", "list")
		if strings.Contains(stdout, "feature-c") {
			t.Error("old name should not exist")
		}
		if !strings.Contains(stdout, "feature-final") {
			t.Error("new name should exist")
		}
	})

	// Remove
	t.Run("remove_worktrees", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "workspace", "remove", "feature-a")
		runJVSInRepo(t, repoPath, "workspace", "remove", "feature-b")

		stdout, _, _ := runJVSInRepo(t, repoPath, "workspace", "list")
		if strings.Contains(stdout, "feature-a") || strings.Contains(stdout, "feature-b") {
			t.Error("removed worktrees should not appear")
		}
	})

	// Cannot remove main
	t.Run("cannot_remove_main", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "workspace", "remove", "main")
		if code == 0 {
			t.Error("should not be able to remove main")
		}
	})
}

// TestE2E_Journey_GCWithActiveWorktrees tests GC with active worktrees
func TestE2E_Journey_GCWithActiveWorktrees(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create main snapshots
	for i := 1; i <= 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte(string(rune('a'+i))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "main")
	}

	// Create worktree with snapshots
	runJVSInRepo(t, repoPath, "fork", "active-feature")
	featurePath := filepath.Join(repoPath, "worktrees", "active-feature")
	os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("active"), 0644)
	runJVSInWorktree(t, repoPath, "active-feature", "checkpoint", "active")

	// Create and remove worktree (creates orphan)
	runJVSInRepo(t, repoPath, "fork", "removed-feature")
	removedPath := filepath.Join(repoPath, "worktrees", "removed-feature")
	os.WriteFile(filepath.Join(removedPath, "removed.txt"), []byte("removed"), 0644)
	runJVSInWorktree(t, repoPath, "removed-feature", "checkpoint", "will-be-orphaned")
	runJVSInRepo(t, repoPath, "workspace", "remove", "removed-feature")

	// Run GC
	t.Run("gc_preserves_active", func(t *testing.T) {
		planOut, _, _ := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
		planID := extractPlanID(planOut)
		if planID == "" {
			planID = extractPlanIDFromText(planOut)
		}
		if planID != "" {
			runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", planID)
		}

		// Active worktree should still work
		_, _, code := runJVSInWorktree(t, repoPath, "active-feature", "checkpoint", "still active")
		if code != 0 {
			t.Error("active worktree should still work")
		}
	})

	// Verify all remaining snapshots
	t.Run("verify_integrity", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Error("verify should pass")
		}
	})
}

// TestE2E_Journey_DoctorRepairsIssues tests doctor repair functionality
func TestE2E_Journey_DoctorRepairsIssues(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create healthy state
	runJVSInRepo(t, repoPath, "checkpoint", "healthy")

	// Introduce issues
	t.Run("introduce_issues", func(t *testing.T) {
		// Orphan tmp
		os.MkdirAll(filepath.Join(jvsPath, "snapshots", "orphan.tmp"), 0755)

		// Orphan intent
		os.MkdirAll(filepath.Join(jvsPath, "intents"), 0755)
		os.WriteFile(filepath.Join(jvsPath, "intents", "stale.json"), []byte(`{"status":"incomplete"}`), 0644)
	})

	// Doctor detects
	t.Run("doctor_detects", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Doctor findings: %s", stdout)
	})

	// Repair
	t.Run("doctor_repairs", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
	})

	// Verify healthy
	t.Run("verify_healthy", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Fatalf("doctor failed: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected healthy: %s", stdout)
		}
	})
}
