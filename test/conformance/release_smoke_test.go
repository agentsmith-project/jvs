//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
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

type releaseSmokeCheckpointRecord struct {
	CheckpointID string   `json:"checkpoint_id"`
	Workspace    string   `json:"workspace"`
	Tags         []string `json:"tags"`
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
	requireReleaseSmokeCheckpointExists(t, repoPath, gcTempCheckpointID)

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
	requireReleaseSmokeCheckpointCollected(t, repoPath, gcTempCheckpointID)

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

func TestReleaseSmoke_ImportCloneRestoreForkPayloadPurity(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source")
	if err := os.MkdirAll(filepath.Join(sourcePath, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "data.txt"), []byte("imported v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "nested", "config.txt"), []byte("nested v1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	importedRepo := filepath.Join(dir, "imported")
	importOut, stderr, code := runJVS(t, dir, "--json", "import", sourcePath, importedRepo)
	requireReleaseSmokeJSONOK(t, importOut, stderr, code)
	initialCheckpoint, _ := decodeContractSmokeDataMap(t, importOut)["initial_checkpoint"].(string)
	requireReleaseSmokeCheckpointExists(t, importedRepo, initialCheckpoint)
	importedMain := filepath.Join(importedRepo, "main")
	requireReleaseSmokePayloadRootClean(t, importedMain)
	requireReleaseSmokeV1Payload(t, importedMain)

	if err := os.WriteFile(filepath.Join(importedMain, "data.txt"), []byte("imported v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(importedMain, "nested", "config.txt"), []byte("nested v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(importedMain, "latest-only.txt"), []byte("latest only\n"), 0644); err != nil {
		t.Fatal(err)
	}
	v2Out, stderr, code := runJVSInRepo(t, importedRepo, "--json", "checkpoint", "release-smoke v2", "--tag", "v2")
	requireReleaseSmokeJSONOK(t, v2Out, stderr, code)
	v2Checkpoint := releaseSmokeCheckpointID(t, v2Out)

	checkpoints := readReleaseSmokeCheckpointList(t, importedRepo)
	if len(checkpoints) != 2 {
		t.Fatalf("imported repo checkpoint count = %d, want 2: %#v", len(checkpoints), checkpoints)
	}
	latest := checkpoints[0].CheckpointID
	oldest := checkpoints[len(checkpoints)-1].CheckpointID
	if latest != v2Checkpoint {
		t.Fatalf("latest checkpoint = %s, want v2 checkpoint %s", latest, v2Checkpoint)
	}
	if oldest != initialCheckpoint {
		t.Fatalf("oldest checkpoint = %s, want import checkpoint %s", oldest, initialCheckpoint)
	}
	if tagged := releaseSmokeCheckpointIDByTag(t, checkpoints, "v2"); tagged != v2Checkpoint {
		t.Fatalf("tag v2 resolved to %s, want %s", tagged, v2Checkpoint)
	}

	restoreOut, stderr, code := runJVSInRepo(t, importedRepo, "--json", "restore", oldest)
	requireReleaseSmokeJSONOK(t, restoreOut, stderr, code)
	requireReleaseSmokeV1Payload(t, importedMain)

	restoreOut, stderr, code = runJVSInRepo(t, importedRepo, "--json", "restore", "latest")
	requireReleaseSmokeJSONOK(t, restoreOut, stderr, code)
	requireReleaseSmokeV2Payload(t, importedMain)

	stdout, stderr, code := runJVSInRepo(t, importedRepo, "--json", "fork", oldest, "from-ref")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)
	fromRefPath := filepath.Join(importedRepo, "worktrees", "from-ref")
	requireReleaseSmokeV1Payload(t, fromRefPath)

	stdout, stderr, code = runJVSInRepo(t, importedRepo, "--json", "fork", "v2", "from-tag")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)
	fromTagPath := filepath.Join(importedRepo, "worktrees", "from-tag")
	requireReleaseSmokeV2Payload(t, fromTagPath)
	requireReleaseSmokePayloadRootClean(t, importedMain)
	requireReleaseSmokePayloadRootClean(t, fromRefPath)
	requireReleaseSmokePayloadRootClean(t, fromTagPath)

	currentClone := filepath.Join(dir, "current-clone")
	stdout, stderr, code = runJVS(t, dir, "--json", "clone", importedRepo, currentClone, "--scope", "current")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)
	currentCloneCheckpoints := readReleaseSmokeCheckpointList(t, currentClone)
	if len(currentCloneCheckpoints) != 1 {
		t.Fatalf("current clone checkpoint count = %d, want 1: %#v", len(currentCloneCheckpoints), currentCloneCheckpoints)
	}
	currentCloneMain := filepath.Join(currentClone, "main")
	requireReleaseSmokeV2Payload(t, currentCloneMain)
	requireReleaseSmokePayloadRootClean(t, currentCloneMain)

	fullClone := filepath.Join(dir, "full-clone")
	stdout, stderr, code = runJVS(t, dir, "--json", "clone", importedRepo, fullClone, "--scope", "full")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)
	fullWorkspaceOut, stderr, code := runJVSInRepo(t, fullClone, "--json", "workspace", "list")
	requireReleaseSmokeJSONOK(t, fullWorkspaceOut, stderr, code)
	requireReleaseSmokeWorkspaceListed(t, fullWorkspaceOut, "from-ref")
	requireReleaseSmokeWorkspaceListed(t, fullWorkspaceOut, "from-tag")

	fullMain := filepath.Join(fullClone, "main")
	fullFromRef := filepath.Join(fullClone, "worktrees", "from-ref")
	fullFromTag := filepath.Join(fullClone, "worktrees", "from-tag")
	requireReleaseSmokeV2Payload(t, fullMain)
	requireReleaseSmokeV1Payload(t, fullFromRef)
	requireReleaseSmokeV2Payload(t, fullFromTag)
	requireReleaseSmokePayloadRootClean(t, fullMain)
	requireReleaseSmokePayloadRootClean(t, fullFromRef)
	requireReleaseSmokePayloadRootClean(t, fullFromTag)

	stdout, stderr, code = runJVSInRepo(t, fullClone, "--json", "fork", "gc-temp")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)
	gcTempPath := filepath.Join(fullClone, "worktrees", "gc-temp")
	if err := os.WriteFile(filepath.Join(gcTempPath, "temp.txt"), []byte("temporary full clone workspace\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tempCheckpointOut, stderr, code := runJVSInWorktree(t, fullClone, "gc-temp", "--json", "checkpoint", "release-smoke full clone gc-temp")
	requireReleaseSmokeJSONOK(t, tempCheckpointOut, stderr, code)
	tempCheckpoint := releaseSmokeCheckpointID(t, tempCheckpointOut)
	requireReleaseSmokeCheckpointExists(t, fullClone, tempCheckpoint)

	stdout, stderr, code = runJVSInRepo(t, fullClone, "--json", "workspace", "remove", "gc-temp")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	planOut, stderr, code := runJVSInRepo(t, fullClone, "--json", "gc", "plan")
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
	if !releaseSmokeStringArrayContains(toDelete, tempCheckpoint) {
		t.Fatalf("gc plan did not include removed gc-temp checkpoint %s: %s", tempCheckpoint, planOut)
	}

	runOut, stderr, code := runJVSInRepo(t, fullClone, "--json", "gc", "run", "--plan-id", planID)
	requireReleaseSmokeJSONOK(t, runOut, stderr, code)
	runData := decodeContractSmokeDataMap(t, runOut)
	if runData["status"] != "completed" {
		t.Fatalf("gc run status = %#v, want completed: %s", runData["status"], runOut)
	}
	requireReleaseSmokeCheckpointCollected(t, fullClone, tempCheckpoint)

	stdout, stderr, code = runJVSInRepo(t, fullClone, "doctor", "--strict")
	if code != 0 {
		t.Fatalf("full clone doctor --strict failed: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, code = runJVSInRepo(t, fullClone, "verify", "--all")
	if code != 0 {
		t.Fatalf("full clone verify --all failed: stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestReleaseSmokePathEntryMissingRejectsDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	linkPath := filepath.Join(dir, ".jvs")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), linkPath); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}
	if err := releaseSmokePathEntryMissing(linkPath); err == nil {
		t.Fatalf("dangling symlink %s was treated as missing", linkPath)
	}

	if err := os.Remove(linkPath); err != nil {
		t.Fatalf("remove dangling symlink: %v", err)
	}
	if err := releaseSmokePathEntryMissing(linkPath); err != nil {
		t.Fatalf("removed path should be missing: %v", err)
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

func requireReleaseSmokeCheckpointExists(t *testing.T, repoPath, checkpointID string) {
	t.Helper()
	if checkpointID == "" {
		t.Fatalf("checkpoint ID is empty")
	}
	for _, path := range []string{
		filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"),
		filepath.Join(repoPath, ".jvs", "snapshots", checkpointID),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("checkpoint %s missing %s: %v", checkpointID, path, err)
		}
	}
	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY")
	readyGzipPath := filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY.gz")
	if _, err := os.Stat(readyPath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat checkpoint %s ready marker %s: %v", checkpointID, readyPath, err)
	}
	if _, err := os.Stat(readyGzipPath); err != nil {
		t.Fatalf("checkpoint %s missing READY marker: %v", checkpointID, err)
	}
}

func requireReleaseSmokeCheckpointCollected(t *testing.T, repoPath, checkpointID string) {
	t.Helper()
	if checkpointID == "" {
		t.Fatalf("checkpoint ID is empty")
	}
	for _, record := range readReleaseSmokeCheckpointList(t, repoPath) {
		if record.CheckpointID == checkpointID {
			t.Fatalf("checkpoint list still includes GC candidate %s: %#v", checkpointID, record)
		}
	}
	requireReleaseSmokePathMissing(t, filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"))
	requireReleaseSmokePathMissing(t, filepath.Join(repoPath, ".jvs", "snapshots", checkpointID))
}

func readReleaseSmokeCheckpointList(t *testing.T, repoPath string) []releaseSmokeCheckpointRecord {
	t.Helper()
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	requireReleaseSmokeJSONOK(t, stdout, stderr, code)

	env := decodeContractSmokeEnvelope(t, stdout)
	var records []releaseSmokeCheckpointRecord
	if err := json.Unmarshal(env.Data, &records); err != nil {
		t.Fatalf("decode checkpoint list data: %v\n%s", err, stdout)
	}
	return records
}

func releaseSmokeCheckpointIDByTag(t *testing.T, records []releaseSmokeCheckpointRecord, tag string) string {
	t.Helper()
	for _, record := range records {
		for _, got := range record.Tags {
			if got == tag {
				return record.CheckpointID
			}
		}
	}
	t.Fatalf("checkpoint list did not include tag %q: %#v", tag, records)
	return ""
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

func requireReleaseSmokeV1Payload(t *testing.T, root string) {
	t.Helper()
	requireReleaseSmokeFileContent(t, root, "data.txt", "imported v1\n")
	requireReleaseSmokeFileContent(t, root, filepath.Join("nested", "config.txt"), "nested v1\n")
	requireReleaseSmokePathMissing(t, filepath.Join(root, "latest-only.txt"))
	requireReleaseSmokePayloadRootClean(t, root)
}

func requireReleaseSmokeV2Payload(t *testing.T, root string) {
	t.Helper()
	requireReleaseSmokeFileContent(t, root, "data.txt", "imported v2\n")
	requireReleaseSmokeFileContent(t, root, filepath.Join("nested", "config.txt"), "nested v2\n")
	requireReleaseSmokeFileContent(t, root, "latest-only.txt", "latest only\n")
	requireReleaseSmokePayloadRootClean(t, root)
}

func requireReleaseSmokeFileContent(t *testing.T, root, rel, want string) {
	t.Helper()
	path := filepath.Join(root, rel)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func requireReleaseSmokePathMissing(t *testing.T, path string) {
	t.Helper()
	if err := releaseSmokePathEntryMissing(path); err != nil {
		t.Fatal(err)
	}
}

func releaseSmokePathEntryMissing(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s should not exist", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", path, err)
	}
	return nil
}

func requireReleaseSmokePayloadRootClean(t *testing.T, root string) {
	t.Helper()
	requireReleaseSmokePathMissing(t, filepath.Join(root, ".jvs"))
}

func releaseSmokeStringArrayContains(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}
	return false
}
