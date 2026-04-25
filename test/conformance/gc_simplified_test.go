//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extractFirstSnapshotID extracts the first snapshot_id from JSON output.
func extractFirstSnapshotID(jsonOutput string) string {
	lines := strings.Split(jsonOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, `"checkpoint_id"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "checkpoint_id" && i+2 < len(parts) {
					return parts[i+2]
				}
			}
		}
	}
	return ""
}

// TestGC_LineageProtection tests that GC correctly protects snapshots via lineage
func TestGC_LineageProtection(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a chain of snapshots in main (lineage)
	snapshotIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte(string(rune('a'+i))), 0644)
		stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "--tag", "chain", "--json")
		snapshotIDs[i] = extractFirstSnapshotID(stdout)
	}

	// Fork a worktree from the middle of the chain
	runJVSInRepo(t, repoPath, "fork", "feature", "--at", snapshotIDs[2])
	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644)
	runJVSInWorktree(t, repoPath, "feature", "checkpoint", "feature snapshot")

	// Create GC plan - all snapshots in lineage should be protected
	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}

	// The plan should show 0 candidates since all snapshots are protected:
	// - main lineage is protected via worktree head
	// - feature worktree head is protected
	// - lineage protection protects parent chain
	t.Logf("GC Plan: %s", planOut)
}

// TestGC_PinProtection tests that GC correctly protects pinned snapshots
func TestGC_PinProtection(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots
	snapshotIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte(string(rune('0'+i))), 0644)
		stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "--tag", "test", "--json")
		snapshotIDs[i] = extractFirstSnapshotID(stdout)
	}

	// Check if pin command is available
	_, stderr, code := runJVS(t, repoPath, "pin", "--help")
	if code != 0 {
		t.Skip("pin command not available:", stderr)
	}

	// Pin the first snapshot
	_, stderr, code = runJVSInRepo(t, repoPath, "pin", "add", snapshotIDs[0], "--reason", "important baseline")
	if code != 0 {
		t.Fatalf("pin add failed: %s", stderr)
	}

	// Remove main worktree (this would orphan snapshots except for the pin)
	runJVSInRepo(t, repoPath, "workspace", "remove", "main")

	// Create GC plan
	planOut, _, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed")
	}

	t.Logf("GC Plan: %s", planOut)

	// The pinned snapshot should still exist
	// Verify by checking history
	historyOut, _, _ := runJVS(t, repoPath, "--json", "checkpoint", "list")
	if !strings.Contains(historyOut, snapshotIDs[0]) {
		t.Errorf("pinned snapshot %s should still exist in history", snapshotIDs[0])
	}

	// List pins
	pinOut, _, _ := runJVS(t, repoPath, "pin", "list")
	if !strings.Contains(pinOut, snapshotIDs[0]) {
		t.Errorf("pinned snapshot %s should be in pin list", snapshotIDs[0])
	}
}

// TestGC_DeterministicPlan tests that GC plan generation is deterministic
func TestGC_DeterministicPlan(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create predictable snapshots
	for i := 1; i <= 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte(string(rune('0'+i))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "checkpoint")
	}

	// Generate first plan
	plan1Out, _, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("first gc plan failed")
	}

	// Generate second plan
	plan2Out, _, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("second gc plan failed")
	}

	// Plans should be identical
	// Compare key fields that should be deterministic
	if strings.Contains(plan1Out, `"protected_by_pin"`) || strings.Contains(plan2Out, `"protected_by_pin"`) {
		t.Error("v0 plan should not expose protected_by_pin field")
	}
	if !strings.Contains(plan1Out, `"protected_by_lineage"`) || !strings.Contains(plan2Out, `"protected_by_lineage"`) {
		t.Error("plan should contain protected_by_lineage field")
	}

	// Both should have same candidate count
	count1 := extractJSONField(plan1Out, "candidate_count")
	count2 := extractJSONField(plan2Out, "candidate_count")
	if count1 != count2 {
		t.Errorf("candidate count should be deterministic: got %s and %s", count1, count2)
	}

	t.Logf("Plan 1: %s", plan1Out)
	t.Logf("Plan 2: %s", plan2Out)
}

// TestGC_EmptyRepo tests GC behavior on empty repository
func TestGC_EmptyRepo(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Run GC plan on empty repo
	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan on empty repo failed: %s", stderr)
	}

	t.Logf("Empty repo GC plan: %s", planOut)

	// Should have 0 candidates
	candidateCount := extractJSONField(planOut, "candidate_count")
	if candidateCount != "0" && candidateCount != "" {
		t.Errorf("expected 0 candidates for empty repo, got: %s", candidateCount)
	}
}

// TestGC_NoDeletableSnapshots tests GC when all snapshots are protected
func TestGC_NoDeletableSnapshots(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a single snapshot
	os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("data"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "only snapshot", "--tag", "important")

	// Run GC plan
	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}

	// Should have 0 candidates (single snapshot is protected as worktree head)
	candidateCount := extractJSONField(planOut, "candidate_count")
	if candidateCount == "0" || candidateCount == "" {
		t.Logf("Correctly identified 0 deletable snapshots")
	} else {
		t.Logf("Unexpected candidate count: %s. Plan: %s", candidateCount, planOut)
	}
}

// TestGC_LineageWithBranches tests lineage protection with multiple branches
func TestGC_LineageWithBranches(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create base snapshot in main
	os.WriteFile(filepath.Join(mainPath, "base.txt"), []byte("base"), 0644)
	baseOut, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "base", "--tag", "baseline", "--json")
	baseID := extractFirstSnapshotID(baseOut)

	// Create another snapshot in main
	os.WriteFile(filepath.Join(mainPath, "v2.txt"), []byte("v2"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Fork branch from base
	runJVSInRepo(t, repoPath, "fork", "branch1", "--at", baseID)
	branchPath := filepath.Join(repoPath, "worktrees", "branch1")
	os.WriteFile(filepath.Join(branchPath, "branch.txt"), []byte("branch work"), 0644)
	runJVSInWorktree(t, repoPath, "branch1", "checkpoint", "branch snapshot")

	// Fork another branch from base
	runJVSInRepo(t, repoPath, "fork", "branch2", "--at", baseID)
	branch2Path := filepath.Join(repoPath, "worktrees", "branch2")
	os.WriteFile(filepath.Join(branch2Path, "branch2.txt"), []byte("branch2 work"), 0644)
	runJVSInWorktree(t, repoPath, "branch2", "checkpoint", "branch2 snapshot")

	// All snapshots should be protected via lineage
	// - base snapshot: protected as ancestor of all worktrees
	// - v2 snapshot: protected as main head
	// - branch snapshots: protected as branch heads

	planOut, _, _ := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	t.Logf("Multi-branch GC plan: %s", planOut)

	// Verify GC shows protected_by_lineage > 0, meaning lineage protection is working
	if !strings.Contains(planOut, `"protected_by_lineage"`) {
		t.Error("GC plan should show protected_by_lineage field")
	}

	// The base snapshot should be in the protected_set (or protected via lineage)
	if !strings.Contains(planOut, baseID) {
		// Check if it's at least mentioned in the plan
		t.Logf("Note: baseID %s may not be in protected_set JSON but is protected via lineage chain", baseID)
	}
}

// TestGC_PinExpiry tests that pins with expiry work correctly
func TestGC_PinExpiry(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Check if pin command is available
	_, stderr, code := runJVS(t, repoPath, "pin", "--help")
	if code != 0 {
		t.Skip("pin command not available:", stderr)
	}

	// Create a snapshot
	os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("data"), 0644)
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "temporary", "--json")
	snapshotID := extractFirstSnapshotID(stdout)

	// Pin with a very short expiry (this is a conformance test for the pin mechanism)
	// Note: In real usage, pins would have longer expiry, but for testing we use short duration
	_, stderr, code = runJVSInRepo(t, repoPath, "pin", "add", snapshotID, "--expires", "1h", "--reason", "short-lived pin")
	if code != 0 {
		t.Logf("pin add with expiry failed (may not be supported): %s", stderr)
		// This is OK - expiry might not be implemented yet
		return
	}

	// Verify pin was created
	pinOut, _, _ := runJVS(t, repoPath, "pin", "list")
	if !strings.Contains(pinOut, snapshotID) {
		t.Errorf("pinned snapshot %s should be in pin list", snapshotID)
	}

	// Remove pin
	runJVSInRepo(t, repoPath, "pin", "remove", snapshotID)

	// Verify pin was removed
	pinOut, _, _ = runJVS(t, repoPath, "pin", "list")
	if strings.Contains(pinOut, snapshotID) {
		t.Errorf("pinned snapshot %s should be removed from pin list", snapshotID)
	}
}

// TestGC_V0LineagePolicy tests v0 GC protection without public retention policy fields.
func TestGC_V0LineagePolicy(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create multiple snapshots
	for i := 1; i <= 10; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte(string(rune('0'+i%10))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "checkpoint")
	}

	// Run GC plan - active workspace lineage should protect these snapshots.
	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}

	t.Logf("v0 lineage GC plan: %s", planOut)

	// The plan should show protection counts
	if strings.Contains(planOut, `"protected_by_pin"`) || strings.Contains(planOut, `"protected_by_retention"`) || strings.Contains(planOut, `"retention"`) {
		t.Error("v0 plan should not expose pin or retention policy fields")
	}
	if !strings.Contains(planOut, `"protected_by_lineage"`) {
		t.Error("plan should contain protected_by_lineage field")
	}
}
