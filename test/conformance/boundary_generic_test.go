//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundaryJSON_UserPayloadExcludesJVSControlData(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "version one\n",
	})
	first := savePoint(t, repoPath, "managed report v1")
	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "version two\n",
	})
	second := savePoint(t, repoPath, "managed report v2")

	firstPreview := jvsJSONData(t, repoPath, "restore", first)
	firstPlanID, _ := firstPreview["plan_id"].(string)
	if firstPlanID == "" {
		t.Fatalf("restore preview missing plan id: %#v", firstPreview)
	}
	jvsJSONData(t, repoPath, "restore", "--run", firstPlanID)
	recoveryPlanFiles := requireControlFilesWithSuffix(t, repoPath, ".jvs/recovery-plans", ".json")

	cleanupPlan := jvsJSONData(t, repoPath, "cleanup", "preview")
	cleanupPlanID, _ := cleanupPlan["plan_id"].(string)
	if cleanupPlanID == "" {
		t.Fatalf("cleanup preview missing plan id: %#v", cleanupPlan)
	}
	writeWorkspaceFile(t, repoPath, ".jvs/locks/runtime-lock.json", "runtime lock data\n")
	requirePathExists(t, repoPath, ".jvs/restore-plans/"+firstPlanID+".json")
	requirePathExists(t, repoPath, ".jvs/gc/"+cleanupPlanID+".json")

	controlDataSave := savePoint(t, repoPath, "after control data exists")
	viewOut := jvsJSON(t, repoPath, "view", controlDataSave)
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewPath == "" || viewData["read_only"] != true {
		t.Fatalf("view should expose a read-only payload path: %#v", viewData)
	}
	requirePathMissing(t, viewPath, ".jvs")
	closeView(t, repoPath, viewOut)

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "view", controlDataSave, ".jvs/recovery-plans")
	if code == 0 {
		t.Fatalf("view of JVS control data unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "JVS control data is not managed") {
		t.Fatalf("view control-data error should explain the boundary: stdout=%s stderr=%s", stdout, stderr)
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "restore", controlDataSave, "--path", ".jvs/recovery-plans")
	if code == 0 {
		t.Fatalf("restore of JVS control data unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "JVS control data is not managed") {
		t.Fatalf("restore control-data error should explain the boundary: stdout=%s stderr=%s", stdout, stderr)
	}

	secondPreview := jvsJSONData(t, repoPath, "restore", second)
	secondPlanID, _ := secondPreview["plan_id"].(string)
	if secondPlanID == "" {
		t.Fatalf("second restore preview missing plan id: %#v", secondPreview)
	}
	jvsJSONData(t, repoPath, "restore", "--run", secondPlanID)
	if got := readFile(t, repoPath, "managed/report.txt"); got != "version two\n" {
		t.Fatalf("restore should update managed payload: %q", got)
	}
	requirePathExists(t, repoPath, ".jvs/locks/runtime-lock.json")
	requirePathExists(t, repoPath, ".jvs/restore-plans/"+firstPlanID+".json")
	requirePathExists(t, repoPath, ".jvs/gc/"+cleanupPlanID+".json")
	for _, name := range recoveryPlanFiles {
		requirePathExists(t, repoPath, ".jvs/recovery-plans/"+name)
	}
}

func TestBoundaryJSON_WorkspaceLocatorStaysControlData(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "main baseline\n",
	})
	base := savePoint(t, repoPath, "main baseline")

	created := jvsJSONData(t, repoPath, "workspace", "new", "../generic-copy", "--from", base)
	workspacePath, _ := created["folder"].(string)
	if workspacePath == "" {
		t.Fatalf("workspace new missing folder: %#v", created)
	}
	locatorPath := filepath.Join(workspacePath, ".jvs")
	locatorBefore := readAbsoluteFile(t, locatorPath)

	createFiles(t, workspacePath, map[string]string{
		"managed/report.txt": "workspace version\n",
	})
	workspaceSave := savePointFromCWD(t, workspacePath, "workspace managed report")
	viewOut := jvsJSON(t, workspacePath, "view", workspaceSave)
	viewPath, _ := decodeContractDataMap(t, viewOut)["view_path"].(string)
	requirePathMissing(t, viewPath, ".jvs")
	closeView(t, repoPath, viewOut)

	createFiles(t, workspacePath, map[string]string{
		"managed/report.txt": "unsaved workspace change\n",
	})
	preview := jvsJSONData(t, workspacePath, "restore", workspaceSave, "--discard-unsaved")
	planID, _ := preview["plan_id"].(string)
	if planID == "" {
		t.Fatalf("workspace restore preview missing plan id: %#v", preview)
	}
	jvsJSONData(t, workspacePath, "restore", "--run", planID)
	if got := readFile(t, workspacePath, "managed/report.txt"); got != "workspace version\n" {
		t.Fatalf("workspace restore should recover managed report: %q", got)
	}
	if got := readAbsoluteFile(t, locatorPath); got != locatorBefore {
		t.Fatalf("workspace locator changed during save/view/restore:\n got %q\nwant %q", got, locatorBefore)
	}
}

func TestBoundaryJSON_PathRestoreKeepsCacheLikeUnrelatedPath(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "report v1\n",
		"cache/tmp.bin":      "cache v1\n",
	})
	first := savePoint(t, repoPath, "managed report and cache v1")
	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "report v2\n",
		"cache/tmp.bin":      "cache current\n",
	})
	second := savePoint(t, repoPath, "managed report and cache v2")

	preview := jvsJSONData(t, repoPath, "restore", first, "--path", "managed/report.txt")
	planID, _ := preview["plan_id"].(string)
	if preview["scope"] != "path" || preview["path"] != "managed/report.txt" || planID == "" {
		t.Fatalf("path restore preview mismatch: %#v", preview)
	}
	if got := readFile(t, repoPath, "cache/tmp.bin"); got != "cache current\n" {
		t.Fatalf("path restore preview changed cache-like file: %q", got)
	}

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["restored_path"] != "managed/report.txt" || restored["history_changed"] != false {
		t.Fatalf("path restore run mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "report v1\n" {
		t.Fatalf("path restore did not restore managed report: %q", got)
	}
	if got := readFile(t, repoPath, "cache/tmp.bin"); got != "cache current\n" {
		t.Fatalf("path restore should not restore or delete cache-like unrelated file: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{second, first})
}

func TestViewJSON_LargeFileAndDirectoryViewsAreReadOnly(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt":   "small managed text\n",
		"large/blob.bin":       strings.Repeat("0123456789abcdef", 8192),
		"large/nested/info.md": "nested file\n",
	})
	save := savePoint(t, repoPath, "large managed files")

	fileViewOut := jvsJSON(t, repoPath, "view", save, "large/blob.bin")
	fileView := decodeContractDataMap(t, fileViewOut)
	fileViewPath, _ := fileView["view_path"].(string)
	if fileView["read_only"] != true || fileViewPath == "" {
		t.Fatalf("large file view should be read-only: %#v", fileView)
	}
	requireNoWriteBits(t, fileViewPath)
	closeView(t, repoPath, fileViewOut)

	dirViewOut := jvsJSON(t, repoPath, "view", save, "large")
	dirView := decodeContractDataMap(t, dirViewOut)
	dirViewPath, _ := dirView["view_path"].(string)
	if dirView["read_only"] != true || dirViewPath == "" {
		t.Fatalf("large directory view should be read-only: %#v", dirView)
	}
	requireNoWriteBits(t, dirViewPath)
	requireNoWriteBits(t, filepath.Join(dirViewPath, "blob.bin"))
	requireNoWriteBits(t, filepath.Join(dirViewPath, "nested"))
	requireNoWriteBits(t, filepath.Join(dirViewPath, "nested", "info.md"))
	closeView(t, repoPath, dirViewOut)
}

func writeWorkspaceFile(t *testing.T, root, filename, content string) {
	t.Helper()
	path := filepath.Join(root, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func requirePathExists(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
		t.Fatalf("%s should exist: %v", name, err)
	}
}

func requireControlFilesWithSuffix(t *testing.T, root, name, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), suffix) {
			files = append(files, entry.Name())
		}
	}
	if len(files) == 0 {
		t.Fatalf("%s should contain at least one %s file", name, suffix)
	}
	return files
}

func requireNoWriteBits(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm()&0222 != 0 {
		t.Fatalf("%s should have no write bits, mode is %s", path, info.Mode().Perm())
	}
}
