//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 2: Daily Development Cycle
// User Story: Developer creates checkpoints, restores to previous state, returns to latest

// TestE2E_Development_DailyCycle tests a typical development day workflow
func TestE2E_Development_DailyCycle(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "devproject")
	mainPath := filepath.Join(repoPath, "main")
	versionPath := filepath.Join(mainPath, "version.txt")

	// Initialize repository
	runJVS(t, dir, "init", "devproject")

	// Step 1: Create morning baseline with tag
	t.Run("morning_baseline", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("v1"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "morning baseline", "--tag", "daily")
		if code != 0 {
			t.Fatalf("checkpoint failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 2: Create second checkpoint (added feature)
	t.Run("added_feature", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("v2"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "added feature")
	})

	// Step 3: Create third checkpoint (version 3)
	t.Run("version_3", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("v3"), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "version 3")
	})

	// Get checkpoint IDs from the public list
	var snapshots []string
	t.Run("get_snapshot_ids", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		snapshots = extractAllSnapshotIDs(stdout)
		if len(snapshots) < 3 {
			t.Fatalf("expected at least 3 snapshots, got %d", len(snapshots))
		}
	})

	if len(snapshots) < 3 {
		t.Fatalf("cannot continue: need at least 3 snapshots, got %d", len(snapshots))
	}

	// Step 4: Restore to v2 checkpoint so current differs from latest.
	v2Snapshot := snapshots[len(snapshots)-2] // second oldest
	t.Run("restore_to_v2", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", v2Snapshot)
		if code != 0 {
			t.Fatalf("restore failed: %s", stderr)
		}

		// Verify file content is v2
		content := readFile(t, mainPath, "version.txt")
		if content != "v2" {
			t.Errorf("expected 'v2', got '%s'", content)
		}

		if !strings.Contains(stdout, "Workspace current differs from latest") {
			t.Errorf("expected historical status guidance, got stdout=%s stderr=%s", stdout, stderr)
		}
		requireWorkspaceCurrentLatest(t, repoPath, v2Snapshot, snapshots[0], false)
	})

	// Step 5: Try to create checkpoint while current differs from latest - must fail
	t.Run("checkpoint_fails_while_historical", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "checkpoint", "should fail")
		if code == 0 {
			t.Error("checkpoint should fail while current differs from latest")
		}
	})

	// Step 6: Restore latest to return to the newest checkpoint
	t.Run("restore_latest", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}

		// Verify file content is back to v3
		content := readFile(t, mainPath, "version.txt")
		if content != "v3" {
			t.Errorf("expected 'v3', got '%s'", content)
		}

		if !strings.Contains(stdout, "Workspace is at latest") {
			t.Errorf("expected latest status message, got: %s", stdout)
		}
		requireWorkspaceCurrentLatest(t, repoPath, snapshots[0], snapshots[0], true)
	})

	// Step 7: Can create checkpoints after restoring latest
	t.Run("continue_working", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("v4"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "continue working")
		if code != 0 {
			t.Fatalf("checkpoint failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})
}

// TestE2E_Development_LineageChain tests that snapshots form a proper lineage
func TestE2E_Development_LineageChain(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a series of snapshots
	for i := 1; i <= 5; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte(string(rune('a'+i-1))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "step")
	}

	// Get history and verify lineage
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	snapshots := extractAllSnapshotIDs(stdout)

	if len(snapshots) != 5 {
		t.Errorf("expected 5 snapshots, got %d", len(snapshots))
	}

	// Verify we can restore to each snapshot in the chain
	for i, snapID := range snapshots {
		_, _, code := runJVSInRepo(t, repoPath, "restore", snapID)
		if code != 0 {
			t.Errorf("failed to restore snapshot %d (%s)", i, snapID)
		}
	}
}

// TestE2E_Development_WorkflowWithTags tests daily workflow with tag-based navigation
func TestE2E_Development_WorkflowWithTags(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots with meaningful tags
	os.WriteFile(filepath.Join(mainPath, "status.txt"), []byte("started"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "start of day", "--tag", "morning")

	os.WriteFile(filepath.Join(mainPath, "status.txt"), []byte("feature-done"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "feature complete", "--tag", "feature")

	os.WriteFile(filepath.Join(mainPath, "status.txt"), []byte("end"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "end of day", "--tag", "eod")

	// Restore by tag
	t.Run("restore_by_tag", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", "feature")
		if code != 0 {
			t.Fatalf("restore by tag failed: %s", stderr)
		}

		content := readFile(t, mainPath, "status.txt")
		if content != "feature-done" {
			t.Errorf("expected 'feature-done', got '%s'", content)
		}
	})

	// Return to latest
	t.Run("return_to_latest", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Workspace is at latest") {
			t.Errorf("expected latest status message, got: %s", stdout)
		}
		content := readFile(t, mainPath, "status.txt")
		if content != "end" {
			t.Errorf("expected 'end', got '%s'", content)
		}
	})
}
