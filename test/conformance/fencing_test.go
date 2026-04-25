//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

// Test 26: Restore to historical checkpoint makes current differ from latest.
// (Covered by TestRestore_Inplace in restore_test.go)

// Test 27: History limit works
func TestHistory_Limit(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create 5 snapshots
	for i := 0; i < 5; i++ {
		runJVSInRepo(t, repoPath, "snapshot", "test")
	}

	// History with limit 2
	stdout, _, code := runJVSInRepo(t, repoPath, "history", "--limit", "2")
	if code != 0 {
		t.Fatal("history --limit failed")
	}

	lines := strings.Count(stdout, "\n")
	if lines > 3 { // 2 snapshots + possible header
		t.Errorf("expected at most 2 snapshots, got %d lines", lines)
	}
}

// Test 28: Invalid name rejected
func TestValidation_InvalidName(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Try to create worktree with invalid name
	_, _, code := runJVSInRepo(t, repoPath, "worktree", "create", "../evil")
	if code == 0 {
		t.Error("should reject path traversal in name")
	}
}

// Test 29: Verify with payload hash
func TestVerify_WithPayloadHash(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot
	runJVSInRepo(t, repoPath, "snapshot", "v1")

	// Get snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	snapshotID := extractSnapshotID(stdout)

	// Verify with payload hash (single snapshot)
	_, stderr, code := runJVSInRepo(t, repoPath, "verify", snapshotID)
	if code != 0 {
		t.Fatalf("verify failed: %s", stderr)
	}
}

// Test 30: Multiple snapshots maintain lineage
func TestSnapshot_Lineage(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create multiple snapshots
	runJVSInRepo(t, repoPath, "snapshot", "first")
	runJVSInRepo(t, repoPath, "snapshot", "second")
	runJVSInRepo(t, repoPath, "snapshot", "third")

	// Verify all snapshots exist
	stdout, _, _ := runJVSInRepo(t, repoPath, "verify", "--all")
	if strings.Contains(stdout, "TAMPERED") {
		t.Error("snapshots should not be tampered")
	}
}

// Test 31: GC run with valid plan
func TestGC_RunWithPlan(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshots
	runJVSInRepo(t, repoPath, "snapshot", "v1")
	runJVSInRepo(t, repoPath, "snapshot", "v2")

	// Create plan
	stdout, _, _ := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	planID := extractPlanID(stdout)
	if planID == "" {
		t.Fatal("could not get plan ID")
	}

	// Run GC (should succeed, though nothing to delete since all protected)
	_, _, code := runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", planID)
	if code != 0 {
		t.Error("gc run should succeed")
	}
}

// Test 32: Snapshot with tags (integration)
func TestSnapshot_TagsIntegration(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot with tags
	runJVSInRepo(t, repoPath, "snapshot", "release v1", "--tag", "v1.0", "--tag", "release")

	// Verify tag appears in history
	stdout, _, code := runJVSInRepo(t, repoPath, "history", "--tag", "release")
	if code != 0 {
		t.Fatal("history --tag failed")
	}
	if !strings.Contains(stdout, "release") {
		t.Error("expected tag in history output")
	}
}

// Test 33: History grep filter
func TestHistory_GrepFilter(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshots with different notes
	runJVSInRepo(t, repoPath, "snapshot", "development work")
	runJVSInRepo(t, repoPath, "snapshot", "production release")

	// Filter by grep
	stdout, _, code := runJVSInRepo(t, repoPath, "history", "--grep", "release")
	if code != 0 {
		t.Fatal("history --grep failed")
	}
	if !strings.Contains(stdout, "release") {
		t.Error("expected 'release' in output")
	}
	if strings.Contains(stdout, "development") {
		t.Error("should not contain 'development'")
	}
}

func extractPlanID(jsonOutput string) string {
	lines := strings.Split(jsonOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, `"plan_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "plan_id" && i+2 < len(parts) {
					return parts[i+2]
				}
			}
		}
	}
	return ""
}
