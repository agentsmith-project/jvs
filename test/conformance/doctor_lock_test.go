//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorRepairRuntimeCleansStaleRepoLock(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	writeConformanceRepoLockOwner(t, repoPath, staleConformanceSameHostOwner(t, "crashed save"))

	stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor --repair-runtime failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Repair clean_locks") {
		t.Fatalf("doctor --repair-runtime did not report clean_locks: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".jvs", "locks", "repo.lock")); !os.IsNotExist(err) {
		t.Fatalf("stale repo lock was not removed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "after.txt"), []byte("mutation works"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runJVSInRepo(t, repoPath, "save", "-m", "after stale lock recovery")
	if code != 0 {
		t.Fatalf("save after stale lock repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
}

func staleConformanceSameHostOwner(t *testing.T, operation string) map[string]any {
	t.Helper()

	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"operation":  operation,
		"pid":        exitedConformanceChildPID(t),
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	}
}

func writeConformanceRepoLockOwner(t *testing.T, repoPath string, owner any) {
	t.Helper()

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(owner, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "owner.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func exitedConformanceChildPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return pid
}
