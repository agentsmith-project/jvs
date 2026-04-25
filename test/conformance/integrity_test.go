//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

// Test 7: Verify passes for valid snapshots
func TestVerify_ValidSnapshots(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	// Verify
	stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
	if code != 0 {
		t.Fatalf("verify failed: %s", stderr)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("expected OK in output, got: %s", stdout)
	}
}

// Test 8: Doctor reports healthy
func TestDoctor_Healthy(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
	if code != 0 {
		t.Fatalf("doctor failed: %s", stderr)
	}
	if !strings.Contains(stdout, "healthy") {
		t.Errorf("expected 'healthy' in output, got: %s", stdout)
	}
}

// Test 9: History shows snapshots
func TestHistory_ShowsSnapshots(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshots
	runJVSInRepo(t, repoPath, "checkpoint", "first")
	runJVSInRepo(t, repoPath, "checkpoint", "second")

	// Check history
	stdout, _, code := runJVSInRepo(t, repoPath, "checkpoint", "list")
	if code != 0 {
		t.Fatalf("history failed")
	}
	if !strings.Contains(stdout, "first") || !strings.Contains(stdout, "second") {
		t.Errorf("expected both snapshots in history, got: %s", stdout)
	}
}
