//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test 6: Restore places worktree at historical snapshot (inplace)
func TestRestore_Inplace(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create first snapshot
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("original"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v1")

	// Create second snapshot (so we can restore to first and be detached)
	os.WriteFile(dataPath, []byte("modified"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v2")

	// Get first snapshot ID from history
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest

	// Restore to first - this is inplace
	stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", firstSnapshot)
	if code != 0 {
		t.Fatalf("restore failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Restored") {
		t.Errorf("expected restore message, got: %s", stdout)
	}

	// Verify content is restored
	content, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Errorf("expected 'original', got '%s'", string(content))
	}
	if fileExists(t, filepath.Join(repoPath, "main", ".READY")) || fileExists(t, filepath.Join(repoPath, "main", ".READY.gz")) {
		t.Error("restore should not materialize snapshot control markers into the worktree")
	}

	// Verify worktree is in detached state
	if !strings.Contains(stdout, "DETACHED") {
		t.Error("expected detached state message")
	}
}

// Test 7: Restore HEAD returns to latest
func TestRestore_HEAD(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two snapshots
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("first"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v1")

	os.WriteFile(dataPath, []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v2")

	// Get first snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest

	// Restore to first (detached)
	runJVSInRepo(t, repoPath, "restore", firstSnapshot)

	// Verify content
	content, _ := os.ReadFile(dataPath)
	if string(content) != "first" {
		t.Errorf("expected 'first', got '%s'", string(content))
	}

	// Restore HEAD
	stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "HEAD")
	if code != 0 {
		t.Fatalf("restore HEAD failed: %s", stderr)
	}

	// Verify content is back to latest
	content, _ = os.ReadFile(dataPath)
	if string(content) != "second" {
		t.Errorf("expected 'second', got '%s'", string(content))
	}

	// Verify HEAD state
	if !strings.Contains(stdout, "HEAD state") {
		t.Error("expected HEAD state message")
	}
}

// Test 8: Worktree fork creates new worktree
func TestWorktree_Fork(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create content and snapshot
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("original"), 0644)

	runJVSInRepo(t, repoPath, "snapshot", "v1")

	// Fork from current position
	stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", "feature")
	if code != 0 {
		t.Fatalf("fork failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Created worktree") {
		t.Errorf("expected success message, got: %s", stdout)
	}

	// Verify new worktree exists
	stdout, _, _ = runJVSInRepo(t, repoPath, "worktree", "list")
	if !strings.Contains(stdout, "feature") {
		t.Error("feature worktree should exist")
	}

	// Verify forked worktree has content
	forkPath := filepath.Join(repoPath, "worktrees", "feature", "data.txt")
	content, err := os.ReadFile(forkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Errorf("expected 'original', got '%s'", string(content))
	}
}

// Test 9: Fork from specific snapshot
func TestWorktree_ForkFromSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two snapshots
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("first"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v1")

	os.WriteFile(dataPath, []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "v2")

	// Get first snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest

	// Fork from first snapshot
	_, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", firstSnapshot, "from-first")
	if code != 0 {
		t.Fatalf("fork from snapshot failed: %s", stderr)
	}

	// Verify forked worktree has first content
	forkPath := filepath.Join(repoPath, "worktrees", "from-first", "data.txt")
	content, err := os.ReadFile(forkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first" {
		t.Errorf("expected 'first', got '%s'", string(content))
	}
}

// Test 10: Cannot snapshot in detached state
func TestSnapshot_DetachedFails(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two snapshots
	runJVSInRepo(t, repoPath, "snapshot", "v1")
	runJVSInRepo(t, repoPath, "snapshot", "v2")

	// Get first snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	firstSnapshot := snapshots[len(snapshots)-1]

	// Restore to first (detached)
	runJVSInRepo(t, repoPath, "restore", firstSnapshot)

	// Try to create snapshot - should fail
	_, _, code := runJVSInRepo(t, repoPath, "snapshot", "should fail")
	if code == 0 {
		t.Error("snapshot should fail in detached state")
	}
}

// Test 11: Fork by tag
func TestWorktree_ForkByTag(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create content and snapshot with tag
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("tagged content"), 0644)

	runJVSInRepo(t, repoPath, "snapshot", "release v1", "--tag", "v1.0")

	// Fork by tag
	_, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", "v1.0", "hotfix")
	if code != 0 {
		t.Fatalf("fork by tag failed: %s", stderr)
	}

	// Verify forked worktree has content
	forkPath := filepath.Join(repoPath, "worktrees", "hotfix", "data.txt")
	content, err := os.ReadFile(forkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "tagged content" {
		t.Errorf("expected 'tagged content', got '%s'", string(content))
	}
}

func extractSnapshotID(historyJSON string) string {
	ids := extractAllSnapshotIDs(historyJSON)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func extractAllSnapshotIDs(historyJSON string) []string {
	var ids []string
	lines := strings.Split(historyJSON, "\n")
	for _, line := range lines {
		if strings.Contains(line, `"snapshot_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "snapshot_id" && i+2 < len(parts) {
					ids = append(ids, parts[i+2])
				}
			}
		}
	}
	return ids
}
