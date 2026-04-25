//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

// Test 20: GC plan succeeds
func TestGC_Plan(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create some snapshots
	runJVSInRepo(t, repoPath, "checkpoint", "v1")
	runJVSInRepo(t, repoPath, "checkpoint", "v2")

	// Create GC plan
	stdout, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Plan ID:") {
		t.Errorf("expected plan output, got: %s", stdout)
	}
}

// Test 21: GC run requires plan ID
func TestGC_RunRequiresPlanID(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Try to run without plan ID
	_, _, code := runJVSInRepo(t, repoPath, "gc", "run")
	if code == 0 {
		t.Error("gc run should require --plan-id")
	}
}

// Test 22: Info shows repository info
func TestInfo_ShowsInfo(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "info")
	if code != 0 {
		t.Fatalf("info failed: %s", stderr)
	}
	if !strings.Contains(stdout, "Repository") {
		t.Errorf("expected repository info, got: %s", stdout)
	}
}

// Test 23: JSON output works
func TestJSON_Output(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	stdout, _, code := runJVSInRepo(t, repoPath, "info", "--json")
	if code != 0 {
		t.Fatal("info --json failed")
	}
	if !strings.Contains(stdout, `"repo_root"`) {
		t.Errorf("expected JSON output, got: %s", stdout)
	}
}

// Test 24: Doctor with --strict runs full checks
func TestDoctor_Strict(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot
	runJVSInRepo(t, repoPath, "checkpoint", "v1")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
	if code != 0 {
		t.Fatalf("doctor --strict failed: %s", stderr)
	}
	if !strings.Contains(stdout, "healthy") && !strings.Contains(stdout, "Findings") {
		t.Errorf("expected health output, got: %s", stdout)
	}
}
