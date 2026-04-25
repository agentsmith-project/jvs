//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type releaseSmokeWorkspaceStatus struct {
	Workspace string `json:"workspace"`
	Dirty     bool   `json:"dirty"`
}

type releaseSmokeWorkspaceRecord struct {
	Workspace string `json:"workspace"`
}

func TestReleaseSmoke_RepresentativeRepoDoctorStrictVerifyAllAndGC(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("release smoke base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	baseOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "release-smoke base")
	requireReleaseSmokeJSONOK(t, baseOut, stderr, code)
	if checkpointID := releaseSmokeCheckpointID(t, baseOut); checkpointID == "" {
		t.Fatalf("base checkpoint output missing checkpoint_id: %s", baseOut)
	}

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", "active-feature")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	activePath := filepath.Join(repoPath, "worktrees", "active-feature")
	if err := os.WriteFile(filepath.Join(activePath, "feature.txt"), []byte("dirty feature work\n"), 0644); err != nil {
		t.Fatal(err)
	}
	activeStatus := readReleaseSmokeWorkspaceStatus(t, repoPath, "active-feature")
	if !activeStatus.Dirty {
		t.Fatalf("active-feature should be dirty after workspace edit")
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "active-feature")
	if code == 0 {
		t.Fatalf("dirty workspace remove unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("dirty workspace remove JSON command wrote stderr: %q", stderr)
	}
	dirtyRemoveEnvelope := decodeContractSmokeEnvelope(t, stdout)
	if dirtyRemoveEnvelope.OK {
		t.Fatalf("dirty workspace remove returned ok JSON envelope: %s", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "dirty") {
		t.Fatalf("dirty workspace remove error did not mention dirty state: %s", stdout)
	}

	activeCheckpointOut, stderr, code := runJVSInWorktree(t, repoPath, "active-feature", "--json", "checkpoint", "release-smoke active-feature")
	requireReleaseSmokeJSONOK(t, activeCheckpointOut, stderr, code)
	if checkpointID := releaseSmokeCheckpointID(t, activeCheckpointOut); checkpointID == "" {
		t.Fatalf("active-feature checkpoint output missing checkpoint_id: %s", activeCheckpointOut)
	}
	activeStatus = readReleaseSmokeWorkspaceStatus(t, repoPath, "active-feature")
	if activeStatus.Dirty {
		t.Fatalf("active-feature should be clean after checkpoint")
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "fork", "gc-temp")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	gcTempPath := filepath.Join(repoPath, "worktrees", "gc-temp")
	if err := os.WriteFile(filepath.Join(gcTempPath, "temp.txt"), []byte("temporary workspace checkpoint\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gcTempCheckpointOut, stderr, code := runJVSInWorktree(t, repoPath, "gc-temp", "--json", "checkpoint", "release-smoke gc-temp")
	requireReleaseSmokeJSONOK(t, gcTempCheckpointOut, stderr, code)
	gcTempCheckpointID := releaseSmokeCheckpointID(t, gcTempCheckpointOut)

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "gc-temp")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	planOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
	requireReleaseSmokeJSONOK(t, planOut, stderr, code)
	planData := decodeContractSmokeDataMap(t, planOut)
	planID, _ := planData["plan_id"].(string)
	if planID == "" {
		t.Fatalf("gc plan missing plan_id: %s", planOut)
	}
	candidateCount, ok := planData["candidate_count"].(float64)
	if !ok || candidateCount < 1 {
		t.Fatalf("gc plan candidate_count must be >= 1: %#v\n%s", planData["candidate_count"], planOut)
	}
	toDelete, ok := planData["to_delete"].([]any)
	if !ok {
		t.Fatalf("gc plan to_delete must be an array: %#v\n%s", planData["to_delete"], planOut)
	}
	if !releaseSmokeStringArrayContains(toDelete, gcTempCheckpointID) || !strings.Contains(planOut, gcTempCheckpointID) {
		t.Fatalf("gc plan did not include removed gc-temp checkpoint %s: %s", gcTempCheckpointID, planOut)
	}

	runOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "run", "--plan-id", planID)
	requireReleaseSmokeJSONOK(t, runOut, stderr, code)
	runData := decodeContractSmokeDataMap(t, runOut)
	if runData["status"] != "completed" {
		t.Fatalf("gc run status = %#v, want completed: %s", runData["status"], runOut)
	}

	workspaceListOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "list")
	requireReleaseSmokeJSONOK(t, workspaceListOut, stderr, code)
	requireReleaseSmokeWorkspaceListed(t, workspaceListOut, "active-feature")

	stdout, stderr, code = runJVSInRepo(t, repoPath, "doctor", "--strict")
	if code != 0 {
		t.Fatalf("doctor --strict failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "verify", "--all")
	if code != 0 {
		t.Fatalf("verify --all failed: stdout=%s stderr=%s", stdout, stderr)
	}
}

func requireReleaseSmokeJSONOK(t *testing.T, stdout, stderr string, code int) {
	t.Helper()
	if code != 0 {
		t.Fatalf("JSON command failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("JSON command wrote stderr: %q", stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("JSON envelope was not ok: %s", stdout)
	}
}

func releaseSmokeCheckpointID(t *testing.T, stdout string) string {
	t.Helper()
	data := decodeContractSmokeDataMap(t, stdout)
	checkpointID, _ := data["checkpoint_id"].(string)
	if checkpointID == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", stdout)
	}
	return checkpointID
}

func readReleaseSmokeWorkspaceStatus(t *testing.T, repoPath, workspace string) releaseSmokeWorkspaceStatus {
	t.Helper()
	stdout, stderr, code := runJVSInWorktree(t, repoPath, workspace, "--json", "status")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	env := decodeContractSmokeEnvelope(t, stdout)
	var status releaseSmokeWorkspaceStatus
	if err := json.Unmarshal(env.Data, &status); err != nil {
		t.Fatalf("decode status data: %v\n%s", err, stdout)
	}
	if status.Workspace != workspace {
		t.Fatalf("status workspace = %q, want %q: %s", status.Workspace, workspace, stdout)
	}
	return status
}

func requireReleaseSmokeWorkspaceListed(t *testing.T, stdout, workspace string) {
	t.Helper()
	env := decodeContractSmokeEnvelope(t, stdout)
	var records []releaseSmokeWorkspaceRecord
	if err := json.Unmarshal(env.Data, &records); err != nil {
		t.Fatalf("decode workspace list data: %v\n%s", err, stdout)
	}
	for _, record := range records {
		if record.Workspace == workspace {
			return
		}
	}
	t.Fatalf("workspace list did not include %q: %s", workspace, stdout)
}

func releaseSmokeStringArrayContains(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}
	return false
}
