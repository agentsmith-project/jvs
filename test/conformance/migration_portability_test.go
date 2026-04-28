//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationPhysicalCopyRepairRuntimeRebindsAdoptedMainWorkspace(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	first := savePoint(t, sourceRepo, "baseline before copy")
	createFiles(t, sourceRepo, map[string]string{"app.txt": "v2\n"})
	second := savePoint(t, sourceRepo, "second before copy")

	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copyMigrationTree(t, sourceRepo, copiedRepo)
	if err := os.Rename(sourceRepo, sourceRepo+".offline"); err != nil {
		t.Fatalf("take source path offline: %v", err)
	}

	stdout, stderr, code := runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict")
	if code == 0 {
		t.Fatalf("strict doctor should fail before repair when copied real_path points offline\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PAYLOAD_INVALID")

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor repair-runtime failed after physical copy: stdout=%s stderr=%s", stdout, stderr)
	}
	doctorData := decodeContractDataMap(t, stdout)
	if doctorData["healthy"] != true {
		t.Fatalf("doctor repair-runtime should leave destination healthy: %#v\n%s", doctorData, stdout)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict")
	if code != 0 {
		t.Fatalf("strict doctor after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	doctorData = decodeContractDataMap(t, stdout)
	if doctorData["healthy"] != true {
		t.Fatalf("strict doctor after repair should be healthy: %#v", doctorData)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "history")
	if code != 0 {
		t.Fatalf("history after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	history := decodeContractDataMap(t, stdout)
	if history["newest_save_point"] != second {
		t.Fatalf("history newest_save_point = %#v, want %s", history["newest_save_point"], second)
	}
	savePoints, ok := history["save_points"].([]any)
	if !ok || len(savePoints) != 2 {
		t.Fatalf("history save_points = %#v, want two save points", history["save_points"])
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "status")
	if code != 0 {
		t.Fatalf("status after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	status := decodeContractDataMap(t, stdout)
	if status["folder"] != copiedRepo || status["workspace"] != "main" {
		t.Fatalf("status targets wrong workspace after repair: %#v", status)
	}
	if status["newest_save_point"] != second || status["content_source"] != second || status["unsaved_changes"] != false {
		t.Fatalf("status save point state mismatch after repair: %#v", status)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "workspace", "path")
	if code != 0 {
		t.Fatalf("workspace path after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	workspacePath := decodeContractDataMap(t, stdout)
	if workspacePath["path"] != copiedRepo {
		t.Fatalf("workspace path = %#v, want %s", workspacePath["path"], copiedRepo)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "restore", first, "--path", "app.txt")
	if code != 0 {
		t.Fatalf("path restore preview after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	preview := decodeContractDataMap(t, stdout)
	planID, _ := preview["plan_id"].(string)
	if planID == "" {
		t.Fatalf("restore preview missing plan_id: %#v", preview)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "restore", "--run", planID)
	if code != 0 {
		t.Fatalf("path restore run after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	restored := decodeContractDataMap(t, stdout)
	if restored["source_save_point"] != first || restored["history_changed"] != false {
		t.Fatalf("restore result mismatch after repair: %#v", restored)
	}
	if got := readFile(t, copiedRepo, "app.txt"); got != "v1\n" {
		t.Fatalf("restored app.txt = %q, want v1", got)
	}
}

func TestMigrationPhysicalCopyRepairRuntimeRebindsAdoptedMainWorkspaceWhenSourceStillExists(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	first := savePoint(t, sourceRepo, "baseline before copy")
	createFiles(t, sourceRepo, map[string]string{"app.txt": "v2\n"})
	second := savePoint(t, sourceRepo, "second before copy")

	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copyMigrationTree(t, sourceRepo, copiedRepo)

	stdout, stderr, code := runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict")
	if code == 0 {
		t.Fatalf("strict doctor should fail before repair when copied real_path points at online source\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PAYLOAD_INVALID")

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor repair-runtime failed after physical copy with source still online: stdout=%s stderr=%s", stdout, stderr)
	}
	doctorData := decodeContractDataMap(t, stdout)
	if doctorData["healthy"] != true {
		t.Fatalf("doctor repair-runtime should leave destination healthy: %#v\n%s", doctorData, stdout)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "status")
	if code != 0 {
		t.Fatalf("status after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	status := decodeContractDataMap(t, stdout)
	if status["folder"] != copiedRepo || status["workspace"] != "main" {
		t.Fatalf("status targets wrong workspace after repair: %#v", status)
	}
	if status["newest_save_point"] != second || status["content_source"] != second || status["unsaved_changes"] != false {
		t.Fatalf("status save point state mismatch after repair: %#v", status)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "workspace", "path")
	if code != 0 {
		t.Fatalf("workspace path after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	workspacePath := decodeContractDataMap(t, stdout)
	if workspacePath["path"] != copiedRepo {
		t.Fatalf("workspace path = %#v, want %s", workspacePath["path"], copiedRepo)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "restore", first, "--path", "app.txt")
	if code != 0 {
		t.Fatalf("path restore preview after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	preview := decodeContractDataMap(t, stdout)
	planID, _ := preview["plan_id"].(string)
	if planID == "" {
		t.Fatalf("restore preview missing plan_id: %#v", preview)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "restore", "--run", planID)
	if code != 0 {
		t.Fatalf("path restore run after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	restored := decodeContractDataMap(t, stdout)
	if restored["source_save_point"] != first || restored["history_changed"] != false {
		t.Fatalf("restore result mismatch after repair: %#v", restored)
	}
	if got := readFile(t, copiedRepo, "app.txt"); got != "v1\n" {
		t.Fatalf("restored copied app.txt = %q, want v1", got)
	}
	if got := readFile(t, sourceRepo, "app.txt"); got != "v2\n" {
		t.Fatalf("source app.txt changed through copied restore: got %q, want v2", got)
	}
}

func TestMigrationPhysicalCopyRepairRuntimeRebindsExternalWorkspaceWhenSourceStillExistsAndContentMatches(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	base := savePoint(t, sourceRepo, "baseline before workspace")
	stdout, stderr, code := runJVSInRepo(t, sourceRepo, "--json", "workspace", "new", "feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}

	sourceFeature := filepath.Join(filepath.Dir(sourceRepo), "feature")
	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copiedFeature := filepath.Join(filepath.Dir(copiedRepo), "feature")
	copyMigrationTree(t, sourceRepo, copiedRepo)
	copyMigrationTree(t, sourceFeature, copiedFeature)

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor repair-runtime failed after physical copy with external workspace: stdout=%s stderr=%s", stdout, stderr)
	}
	doctorData := decodeContractDataMap(t, stdout)
	if doctorData["healthy"] != true {
		t.Fatalf("doctor repair-runtime should leave copied external workspace healthy: %#v\n%s", doctorData, stdout)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "workspace", "path", "feature")
	if code != 0 {
		t.Fatalf("workspace path feature after repair failed: stdout=%s stderr=%s", stdout, stderr)
	}
	workspacePath := decodeContractDataMap(t, stdout)
	if workspacePath["path"] != copiedFeature {
		t.Fatalf("workspace path feature = %#v, want copied sibling %s", workspacePath["path"], copiedFeature)
	}
	if workspacePath["path"] == sourceFeature {
		t.Fatalf("workspace path feature still points at source sibling %s", sourceFeature)
	}
}

func TestMigrationPhysicalCopyRepairRuntimeRewritesCopiedExternalWorkspaceLocatorWhenSourceOffline(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	base := savePoint(t, sourceRepo, "baseline before workspace")
	stdout, stderr, code := runJVSInRepo(t, sourceRepo, "--json", "workspace", "new", "feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}

	sourceFeature := filepath.Join(filepath.Dir(sourceRepo), "feature")
	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copiedFeature := filepath.Join(filepath.Dir(copiedRepo), "feature")
	copyMigrationTree(t, sourceRepo, copiedRepo)
	copyMigrationTree(t, sourceFeature, copiedFeature)
	if err := os.Rename(sourceRepo, sourceRepo+".offline"); err != nil {
		t.Fatalf("take source repo offline: %v", err)
	}
	if err := os.Rename(sourceFeature, sourceFeature+".offline"); err != nil {
		t.Fatalf("take source workspace offline: %v", err)
	}

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor repair-runtime failed after physical copy with offline external workspace: stdout=%s stderr=%s", stdout, stderr)
	}
	doctorData := decodeContractDataMap(t, stdout)
	if doctorData["healthy"] != true {
		t.Fatalf("doctor repair-runtime should leave copied external workspace healthy: %#v\n%s", doctorData, stdout)
	}

	statusOut := jvsJSON(t, copiedFeature, "status")
	statusEnv := decodeContractEnvelope(t, statusOut)
	if statusEnv.RepoRoot == nil || *statusEnv.RepoRoot != copiedRepo {
		t.Fatalf("copied external workspace should self-discover copied repo: %#v", statusEnv.RepoRoot)
	}
	status := decodeContractDataMap(t, statusOut)
	if status["workspace"] != "feature" || status["folder"] != copiedFeature || status["started_from_save_point"] != base {
		t.Fatalf("copied external workspace status mismatch: %#v", status)
	}

	createFiles(t, copiedFeature, map[string]string{"feature.txt": "offline copy work\n"})
	saveOut := jvsJSON(t, copiedFeature, "save", "-m", "offline copied feature")
	saveEnv := decodeContractEnvelope(t, saveOut)
	if saveEnv.RepoRoot == nil || *saveEnv.RepoRoot != copiedRepo || saveEnv.Workspace == nil || *saveEnv.Workspace != "feature" {
		t.Fatalf("copied external workspace save targeted wrong repo/workspace: repo=%#v workspace=%#v", saveEnv.RepoRoot, saveEnv.Workspace)
	}
	saveData := decodeContractDataMap(t, saveOut)
	featureSave, _ := saveData["save_point_id"].(string)
	if featureSave == "" {
		t.Fatalf("copied external workspace save missing save_point_id: %#v", saveData)
	}
	requireHistoryIDsInCWD(t, copiedFeature, []string{featureSave})
	requireHistoryIDs(t, copiedRepo, []string{base})
}

func TestMigrationPhysicalCopyRepairRuntimeKeepsExternalWorkspaceUnhealthyWhenSourceStillExistsAndDestinationMissing(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	base := savePoint(t, sourceRepo, "baseline before workspace")
	stdout, stderr, code := runJVSInRepo(t, sourceRepo, "--json", "workspace", "new", "feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}

	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copyMigrationTree(t, sourceRepo, copiedRepo)

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code == 0 {
		t.Fatalf("doctor repair-runtime should fail when external workspace destination sibling is missing: stdout=%s stderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PATH_BINDING_INVALID")

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict")
	if code == 0 {
		t.Fatalf("strict doctor should stay unhealthy after skipped external workspace rebind: stdout=%s stderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PATH_BINDING_INVALID")
}

func TestMigrationPhysicalCopyRepairRuntimeKeepsExternalWorkspaceUnhealthyWhenSourceStillExistsAndDestinationContentDiffers(t *testing.T) {
	sourceRepo, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n", "keep.txt": "kept\n"})
	base := savePoint(t, sourceRepo, "baseline before workspace")
	stdout, stderr, code := runJVSInRepo(t, sourceRepo, "--json", "workspace", "new", "feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}

	sourceFeature := filepath.Join(filepath.Dir(sourceRepo), "feature")
	copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
	copiedFeature := filepath.Join(filepath.Dir(copiedRepo), "feature")
	copyMigrationTree(t, sourceRepo, copiedRepo)
	copyMigrationTree(t, sourceFeature, copiedFeature)
	createFiles(t, copiedFeature, map[string]string{"app.txt": "destination differs\n"})

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
	if code == 0 {
		t.Fatalf("doctor repair-runtime should fail when external workspace destination content differs: stdout=%s stderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PATH_BINDING_INVALID")

	stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict")
	if code == 0 {
		t.Fatalf("strict doctor should stay unhealthy after skipped external workspace content rebind: stdout=%s stderr=%s", stdout, stderr)
	}
	assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PATH_BINDING_INVALID")
}

func copyMigrationTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy migration tree: %v", err)
	}
}

func assertMigrationDoctorFinding(t *testing.T, stdout, stderr, code string) {
	t.Helper()

	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	var data struct {
		Healthy  bool `json:"healthy"`
		Findings []struct {
			ErrorCode string `json:"error_code"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode doctor data: %v\n%s", err, stdout)
	}
	if data.Healthy {
		t.Fatalf("doctor should be unhealthy before repair: %s", stdout)
	}
	for _, finding := range data.Findings {
		if finding.ErrorCode == code {
			return
		}
	}
	t.Fatalf("doctor output missing finding %s: %s", code, stdout)
}
