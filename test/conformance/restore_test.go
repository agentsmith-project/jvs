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
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	// Create second checkpoint so the first checkpoint becomes historical.
	os.WriteFile(dataPath, []byte("modified"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Get first checkpoint ID from the public checkpoint list.
	stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest
	latestSnapshot := snapshots[0]

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

	if !strings.Contains(stdout, "Workspace current differs from latest") {
		t.Errorf("expected historical status guidance, got: %s", stdout)
	}
	requireWorkspaceCurrentLatest(t, repoPath, firstSnapshot, latestSnapshot, false)
}

func TestRestore_InplaceKeepsCallerCwdUsable(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	dataPath := filepath.Join(mainPath, "data.txt")

	os.WriteFile(dataPath, []byte("original"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v1")
	os.WriteFile(dataPath, []byte("modified"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1]

	beforeRoot, err := os.Stat(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(mainPath); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)

	_, stderr, code := runJVS(t, mainPath, "restore", firstSnapshot)
	if code != 0 {
		t.Fatalf("restore failed: %s", stderr)
	}

	content, err := os.ReadFile("data.txt")
	if err != nil {
		t.Fatalf("read restored file from caller cwd: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("expected 'original', got '%s'", string(content))
	}
	cwdRoot, err := os.Stat(".")
	if err != nil {
		t.Fatal(err)
	}
	afterRoot, err := os.Stat(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(beforeRoot, afterRoot) {
		t.Fatal("restore replaced the workspace root directory")
	}
	if !os.SameFile(cwdRoot, afterRoot) {
		t.Fatal("caller cwd no longer points at the restored workspace root")
	}
}

// Test 7: Restore latest returns to the newest checkpoint.
func TestRestore_Latest(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two snapshots
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("first"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	os.WriteFile(dataPath, []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Get first snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest
	latestSnapshot := snapshots[0]

	// Restore to first so current differs from latest.
	runJVSInRepo(t, repoPath, "restore", firstSnapshot)

	// Verify content
	content, _ := os.ReadFile(dataPath)
	if string(content) != "first" {
		t.Errorf("expected 'first', got '%s'", string(content))
	}

	// Restore latest
	stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
	if code != 0 {
		t.Fatalf("restore latest failed: %s", stderr)
	}

	// Verify content is back to latest
	content, _ = os.ReadFile(dataPath)
	if string(content) != "second" {
		t.Errorf("expected 'second', got '%s'", string(content))
	}

	if !strings.Contains(stdout, "Workspace is at latest") {
		t.Errorf("expected latest status message, got: %s", stdout)
	}
	requireWorkspaceCurrentLatest(t, repoPath, latestSnapshot, latestSnapshot, true)
}

// Test 8: Worktree fork creates new worktree
func TestWorktree_Fork(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create content and snapshot
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("original"), 0644)

	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	// Fork from current position
	stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "feature")
	if code != 0 {
		t.Fatalf("fork failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Created workspace") {
		t.Errorf("expected success message, got: %s", stdout)
	}

	// Verify new worktree exists
	stdout, _, _ = runJVSInRepo(t, repoPath, "workspace", "list")
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
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	os.WriteFile(dataPath, []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Get first snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	snapshots := extractAllSnapshotIDs(stdout)
	if len(snapshots) < 2 {
		t.Fatal("expected at least 2 snapshots")
	}
	firstSnapshot := snapshots[len(snapshots)-1] // oldest

	// Fork from first snapshot
	_, stderr, code := runJVSInRepo(t, repoPath, "fork", firstSnapshot, "from-first")
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

// Test 10: Cannot checkpoint while current differs from latest.
func TestCheckpoint_HistoricalFails(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two checkpoints
	runJVSInRepo(t, repoPath, "checkpoint", "v1")
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Get first checkpoint ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	snapshots := extractAllSnapshotIDs(stdout)
	firstSnapshot := snapshots[len(snapshots)-1]

	// Restore to first so current differs from latest.
	runJVSInRepo(t, repoPath, "restore", firstSnapshot)
	requireWorkspaceCurrentLatest(t, repoPath, firstSnapshot, snapshots[0], false)

	// Try to create checkpoint - should fail.
	_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "should fail")
	if code == 0 {
		t.Error("checkpoint should fail while current differs from latest")
	}
	if !strings.Contains(stderr, "current differs from latest") {
		t.Errorf("expected historical-position error, got: %s", stderr)
	}
}

// Test 11: Fork by tag
func TestWorktree_ForkByTag(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create content and snapshot with tag
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("tagged content"), 0644)

	runJVSInRepo(t, repoPath, "checkpoint", "release v1", "--tag", "v1.0")

	// Fork by tag
	_, stderr, code := runJVSInRepo(t, repoPath, "fork", "v1.0", "hotfix")
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
		const field = "checkpoint_id"
		if !strings.Contains(line, `"`+field+`"`) {
			continue
		}
		parts := strings.Split(line, `"`)
		for i, p := range parts {
			if p == field && i+2 < len(parts) {
				ids = append(ids, parts[i+2])
			}
		}
	}
	return ids
}
