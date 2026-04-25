//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

// Test 4: Snapshot creates successfully
func TestSnapshot_Basic(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot
	stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "test snapshot")
	if code != 0 {
		t.Fatalf("snapshot failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Created checkpoint") {
		t.Errorf("expected 'Created checkpoint' in output, got: %s", stdout)
	}
}

// Test 5: Snapshot with tags
func TestSnapshot_WithTags(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot with tags
	stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "release", "--tag", "v1.0", "--tag", "release")
	if code != 0 {
		t.Fatalf("snapshot with tags failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Created checkpoint") {
		t.Errorf("expected 'Created checkpoint' in output, got: %s", stdout)
	}

	// Verify tags appear in history
	historyOut, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list")
	if !strings.Contains(historyOut, "release") {
		t.Error("expected tag in history output")
	}
}
