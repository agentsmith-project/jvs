//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 4: Hotfix/Emergency Flow
// User Story: On-call engineer restores to old version, creates hotfix workspace

// TestE2E_Hotfix_EmergencyWorkflow tests the complete hotfix workflow
func TestE2E_Hotfix_EmergencyWorkflow(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "prod")
	mainPath := filepath.Join(repoPath, "main")
	versionPath := filepath.Join(mainPath, "VERSION")

	// Initialize repository
	runJVS(t, dir, "init", "prod")

	// Step 1: Create production versions
	t.Run("create_versions", func(t *testing.T) {
		// v1.0 - initial stable release
		os.WriteFile(versionPath, []byte("1.0"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "v1.0", "--tag", "v1.0", "--tag", "stable")

		// v1.1 - minor update
		os.WriteFile(versionPath, []byte("1.1"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "v1.1", "--tag", "v1.1")

		// v2.0 - major update
		os.WriteFile(versionPath, []byte("2.0"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "v2.0", "--tag", "v2.0")
	})

	// Step 2: Emergency - restore to v1.1
	t.Run("emergency_restore", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "v1.1")
		if code != 0 {
			t.Fatalf("restore to v1.1 failed: %s", stderr)
		}

		// Verify content
		content := readFile(t, mainPath, "VERSION")
		if content != "1.1" {
			t.Errorf("expected '1.1', got '%s'", content)
		}

		if !strings.Contains(stdout, "Workspace current differs from latest") {
			t.Errorf("expected historical status guidance, got stdout=%s stderr=%s", stdout, stderr)
		}
		status := readWorkspaceStatus(t, repoPath)
		if status.AtLatest || status.Current == status.Latest {
			t.Fatalf("expected current to differ from latest, got current=%s latest=%s", status.Current, status.Latest)
		}
	})

	// Step 3: Try to create checkpoint - must fail while current differs from latest
	t.Run("cannot_checkpoint_while_historical", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("1.1.1"), 0644)
		_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "try")
		if code == 0 {
			t.Error("checkpoint should fail while current differs from latest")
		}
		if !strings.Contains(stderr, "current differs from latest") {
			t.Logf("expected historical-position error, got: %s", stderr)
		}
	})

	// Step 4: Fork hotfix workspace from the historical current checkpoint
	t.Run("fork_hotfix_branch", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "hotfix-v1.1", "--discard-dirty")
		if code != 0 {
			t.Fatalf("fork failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created workspace") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 5: Work on hotfix in forked worktree
	t.Run("create_hotfix", func(t *testing.T) {
		hotfixPath := filepath.Join(repoPath, "worktrees", "hotfix-v1.1")
		hotfixVersionPath := filepath.Join(hotfixPath, "VERSION")

		// Verify hotfix starts with v1.1 content
		content := readFile(t, hotfixPath, "VERSION")
		if content != "1.1" {
			t.Errorf("expected '1.1', got '%s'", content)
		}

		// Create hotfix version
		os.WriteFile(hotfixVersionPath, []byte("1.1.1"), 0644)
		stdout, stderr, code := runJVSInWorktree(t, repoPath, "hotfix-v1.1",
			"checkpoint", "hotfix v1.1.1", "--tag", "v1.1.1", "--tag", "hotfix")
		if code != 0 {
			t.Fatalf("hotfix checkpoint failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 6: Return main to latest
	t.Run("restore_main_latest", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest", "--discard-dirty")
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}

		// Verify main is back to v2.0
		content := readFile(t, mainPath, "VERSION")
		if content != "2.0" {
			t.Errorf("expected '2.0', got '%s'", content)
		}

		if !strings.Contains(stdout, "Workspace is at latest") {
			t.Errorf("expected latest status message, got: %s", stdout)
		}
	})

	// Step 7: Verify worktrees are independent
	t.Run("verify_independence", func(t *testing.T) {
		// Main should be at v2.0
		mainContent := readFile(t, mainPath, "VERSION")
		if mainContent != "2.0" {
			t.Errorf("main should be at v2.0, got '%s'", mainContent)
		}

		// Hotfix should be at v1.1.1
		hotfixContent := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-v1.1"), "VERSION")
		if hotfixContent != "1.1.1" {
			t.Errorf("hotfix should be at v1.1.1, got '%s'", hotfixContent)
		}
	})
}

// TestE2E_Hotfix_FromStableTag tests creating hotfix from stable tag
func TestE2E_Hotfix_FromStableTag(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	versionPath := filepath.Join(mainPath, "VERSION")

	// Create stable release
	os.WriteFile(versionPath, []byte("3.0.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "stable 3.0", "--tag", "v3.0.0", "--tag", "stable")

	// Create unstable development
	os.WriteFile(versionPath, []byte("4.0.0-dev"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "dev 4.0", "--tag", "dev")

	// Emergency: need to fix stable
	t.Run("hotfix_from_stable", func(t *testing.T) {
		// Fork directly from stable tag
		_, stderr, code := runJVSInRepo(t, repoPath, "fork", "stable", "hotfix-3.0")
		if code != 0 {
			t.Fatalf("fork from stable failed: %s", stderr)
		}

		// Verify hotfix has stable content
		content := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-3.0"), "VERSION")
		if content != "3.0.0" {
			t.Errorf("expected stable content, got: %s", content)
		}

		// Apply hotfix
		hotfixVersionPath := filepath.Join(repoPath, "worktrees", "hotfix-3.0", "VERSION")
		os.WriteFile(hotfixVersionPath, []byte("3.0.1"), 0644)
		runJVSInWorktree(t, repoPath, "hotfix-3.0", "checkpoint", "hotfix 3.0.1",
			"--tag", "v3.0.1", "--tag", "hotfix")
	})

	// Verify main still has dev content
	t.Run("main_unchanged", func(t *testing.T) {
		content := readFile(t, mainPath, "VERSION")
		if content != "4.0.0-dev" {
			t.Errorf("main should still have dev content, got: %s", content)
		}
	})
}

// TestE2E_Hotfix_MultipleHotfixBranches tests multiple concurrent hotfix branches
func TestE2E_Hotfix_MultipleHotfixBranches(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create multiple releases
	for _, ver := range []string{"1.0", "2.0", "3.0"} {
		os.WriteFile(filepath.Join(mainPath, "VERSION"), []byte(ver), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "v"+ver, "--tag", "v"+ver)
	}

	// Create hotfix branches from different versions
	t.Run("create_multiple_hotfixes", func(t *testing.T) {
		// Hotfix for v1.0
		_, _, code := runJVSInRepo(t, repoPath, "fork", "v1.0", "hotfix-1.x")
		if code != 0 {
			t.Fatal("failed to fork hotfix-1.x")
		}

		// Hotfix for v2.0
		_, _, code = runJVSInRepo(t, repoPath, "fork", "v2.0", "hotfix-2.x")
		if code != 0 {
			t.Fatal("failed to fork hotfix-2.x")
		}

		// Verify both exist and have correct content
		ver1 := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-1.x"), "VERSION")
		if ver1 != "1.0" {
			t.Errorf("hotfix-1.x should have v1.0, got: %s", ver1)
		}

		ver2 := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-2.x"), "VERSION")
		if ver2 != "2.0" {
			t.Errorf("hotfix-2.x should have v2.0, got: %s", ver2)
		}
	})

	// Apply different hotfixes
	t.Run("apply_hotfixes", func(t *testing.T) {
		// Hotfix 1.x -> 1.0.1
		os.WriteFile(filepath.Join(repoPath, "worktrees", "hotfix-1.x", "VERSION"), []byte("1.0.1"), 0644)
		runJVSInWorktree(t, repoPath, "hotfix-1.x", "checkpoint", "hotfix 1.0.1", "--tag", "v1.0.1")

		// Hotfix 2.x -> 2.0.1
		os.WriteFile(filepath.Join(repoPath, "worktrees", "hotfix-2.x", "VERSION"), []byte("2.0.1"), 0644)
		runJVSInWorktree(t, repoPath, "hotfix-2.x", "checkpoint", "hotfix 2.0.1", "--tag", "v2.0.1")

		// Verify independence
		ver1 := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-1.x"), "VERSION")
		ver2 := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-2.x"), "VERSION")

		if ver1 != "1.0.1" || ver2 != "2.0.1" {
			t.Errorf("hotfixes should be independent: 1.x=%s, 2.x=%s", ver1, ver2)
		}
	})
}

// TestE2E_Hotfix_RestoreBySnapshotID tests restoring by snapshot ID for hotfix
func TestE2E_Hotfix_RestoreBySnapshotID(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create versions
	for _, ver := range []string{"A", "B", "C"} {
		os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte(ver), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "state "+ver)
	}

	// Get snapshot ID for state B
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 3 {
		t.Fatal("expected at least 3 snapshots")
	}
	stateBID := snapshots[len(snapshots)-2] // second oldest

	// Restore by ID and fork
	t.Run("restore_and_fork_by_id", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "restore", stateBID)
		if code != 0 {
			t.Fatal("restore by ID failed")
		}

		content := readFile(t, mainPath, "state.txt")
		if content != "B" {
			t.Errorf("expected state B, got: %s", content)
		}

		// Fork from this state
		_, _, code = runJVSInRepo(t, repoPath, "fork", "from-state-b")
		if code != 0 {
			t.Fatal("fork failed")
		}

		// Verify forked worktree has state B
		forkContent := readFile(t, filepath.Join(repoPath, "worktrees", "from-state-b"), "state.txt")
		if forkContent != "B" {
			t.Errorf("forked worktree should have state B, got: %s", forkContent)
		}
	})
}
