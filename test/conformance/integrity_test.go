//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
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

func TestDoctorStrict_MalformedAuditIsUnhealthy(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	runJVSInRepo(t, repoPath, "checkpoint", "audited")

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	if err := os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
	if code == 0 {
		t.Fatalf("doctor --strict accepted malformed audit: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "E_AUDIT_RECORD_MALFORMED") {
		t.Fatalf("doctor --strict missing audit error code: %s", stdout)
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
