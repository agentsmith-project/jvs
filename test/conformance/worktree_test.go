//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test 10: Worktree create succeeds
func TestWorktree_Create(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	createWorktreeTestBaseline(t, repoPath)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "feature")
	if code != 0 {
		t.Fatalf("worktree create failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Created workspace") {
		t.Errorf("expected success message, got: %s", stdout)
	}
}

// Test 11: Worktree list shows all worktrees
func TestWorktree_List(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	createWorktreeTestBaseline(t, repoPath)

	// Create additional worktree
	runJVSInRepo(t, repoPath, "fork", "feature")

	stdout, _, code := runJVSInRepo(t, repoPath, "workspace", "list")
	if code != 0 {
		t.Fatal("worktree list failed")
	}
	if !strings.Contains(stdout, "main") || !strings.Contains(stdout, "feature") {
		t.Errorf("expected both worktrees, got: %s", stdout)
	}
}

// Test 12: Worktree rename succeeds
func TestWorktree_Rename(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	createWorktreeTestBaseline(t, repoPath)

	runJVSInRepo(t, repoPath, "fork", "old-name")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "rename", "old-name", "new-name")
	if code != 0 {
		t.Fatalf("worktree rename failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Renamed") {
		t.Errorf("expected success message, got: %s", stdout)
	}
}

// Test 13: Worktree remove succeeds
func TestWorktree_Remove(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	createWorktreeTestBaseline(t, repoPath)

	runJVSInRepo(t, repoPath, "fork", "to-delete")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "remove", "to-delete")
	if code != 0 {
		t.Fatalf("worktree remove failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Removed") {
		t.Errorf("expected success message, got: %s", stdout)
	}
}

// Test 14: Worktree cannot remove main
func TestWorktree_CannotRemoveMain(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	_, _, code := runJVSInRepo(t, repoPath, "workspace", "remove", "main")
	if code == 0 {
		t.Error("should not be able to remove main worktree")
	}
}

// Test 15: Worktree path returns correct path
func TestWorktree_Path(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	stdout, _, code := runJVSInRepo(t, repoPath, "workspace", "path", "main")
	if code != 0 {
		t.Fatal("worktree path failed")
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("expected path containing 'main', got: %s", stdout)
	}
}

// Test 16: Forked workspace materializes the selected checkpoint
func TestWorktree_CreateIsEmpty(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create content in main
	dataPath := filepath.Join(repoPath, "main", "data.txt")
	os.WriteFile(dataPath, []byte("test content"), 0644)

	// Create snapshot
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	// Create worktree
	runJVSInRepo(t, repoPath, "fork", "feature")

	// Feature workspace should contain the forked checkpoint contents.
	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	if _, err := os.Stat(filepath.Join(featurePath, "data.txt")); err != nil {
		t.Fatalf("forked workspace should contain data.txt: %v", err)
	}
}

func createWorktreeTestBaseline(t *testing.T, repoPath string) {
	t.Helper()
	dataPath := filepath.Join(repoPath, "main", "baseline.txt")
	if err := os.WriteFile(dataPath, []byte("baseline"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "baseline"); code != 0 {
		t.Fatalf("baseline checkpoint failed: %s", stderr)
	}
}
