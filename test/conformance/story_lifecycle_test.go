//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoryJSON_InitExistingFolderAdoptsWithoutMovingOrRewritingFiles(t *testing.T) {
	parent := t.TempDir()
	folder := filepath.Join(parent, "existing-project")
	if err := os.MkdirAll(filepath.Join(folder, "notes"), 0755); err != nil {
		t.Fatalf("create existing folder: %v", err)
	}
	userFile := filepath.Join(folder, "notes", "plan.md")
	if err := os.WriteFile(userFile, []byte("draft plan\n"), 0400); err != nil {
		t.Fatalf("write user file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.Chmod(userFile, 0400); err != nil {
		t.Fatalf("chmod user file: %v", err)
	}
	beforeInfo, err := os.Stat(userFile)
	if err != nil {
		t.Fatalf("stat user file before init: %v", err)
	}
	beforeContent := readAbsoluteFile(t, userFile)
	time.Sleep(20 * time.Millisecond)

	initData := jvsJSONData(t, parent, "init", folder)
	if initData["folder"] != folder || initData["workspace"] != "main" {
		t.Fatalf("init existing folder JSON mismatch: %#v", initData)
	}

	afterInfo, err := os.Stat(userFile)
	if err != nil {
		t.Fatalf("stat user file after init: %v", err)
	}
	if got := readAbsoluteFile(t, userFile); got != beforeContent {
		t.Fatalf("init rewrote user file content: got %q want %q", got, beforeContent)
	}
	if afterInfo.Mode().Perm() != beforeInfo.Mode().Perm() || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) || afterInfo.Size() != beforeInfo.Size() {
		t.Fatalf("init should not rewrite user file metadata: before mode=%s mtime=%s size=%d after mode=%s mtime=%s size=%d",
			beforeInfo.Mode().Perm(), beforeInfo.ModTime(), beforeInfo.Size(),
			afterInfo.Mode().Perm(), afterInfo.ModTime(), afterInfo.Size())
	}
	if got := readFile(t, folder, "README.md"); got != "hello\n" {
		t.Fatalf("init moved or rewrote README.md: %q", got)
	}

	statusBeforeSave := jvsJSONData(t, folder, "status")
	if statusBeforeSave["workspace"] != "main" || statusBeforeSave["folder"] != folder {
		t.Fatalf("status after init should target adopted main folder: %#v", statusBeforeSave)
	}
	if statusBeforeSave["unsaved_changes"] != true || statusBeforeSave["files_state"] != "not_saved" {
		t.Fatalf("status after init should show adopted files as not saved/unsaved: %#v", statusBeforeSave)
	}
	if statusBeforeSave["newest_save_point"] != nil || statusBeforeSave["history_head"] != nil || statusBeforeSave["content_source"] != nil {
		t.Fatalf("status before first save should not invent save point state: %#v", statusBeforeSave)
	}

	first := savePointFromCWD(t, folder, "first save from existing folder")
	requireHistoryIDsInCWD(t, folder, []string{first})
	statusAfterSave := jvsJSONData(t, folder, "status")
	if statusAfterSave["unsaved_changes"] != false || statusAfterSave["files_state"] != "matches_save_point" {
		t.Fatalf("status after first save should be clean: %#v", statusAfterSave)
	}
	if statusAfterSave["newest_save_point"] != first || statusAfterSave["history_head"] != first || statusAfterSave["content_source"] != first {
		t.Fatalf("status after first save should point at first save point: %#v", statusAfterSave)
	}
}

func TestStoryJSON_WorkspaceRemoveIsPreviewFirstAndKeepsSavePointStorageForCleanupReview(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "main\n"})
	base := savePoint(t, repoPath, "main base")
	created := jvsJSONData(t, repoPath, "workspace", "new", "../experiment", "--from", base)
	workspacePath, _ := created["folder"].(string)
	if workspacePath == "" {
		t.Fatalf("workspace new missing folder: %#v", created)
	}
	createFiles(t, workspacePath, map[string]string{"result.txt": "experiment result\n"})
	experimentSave := savePointFromCWD(t, workspacePath, "experiment result")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "experiment")
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace remove preview must not delete workspace folder before run: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	pathAfterPreview := jvsJSONData(t, repoPath, "workspace", "path", "experiment")
	if pathAfterPreview["path"] != workspacePath {
		t.Fatalf("workspace remove preview must keep workspace metadata: %#v", pathAfterPreview)
	}
	if code != 0 {
		t.Fatalf("workspace remove preview failed: stdout=%s stderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, true)
	preview := decodeContractDataMap(t, stdout)
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["workspace"] != "experiment" || planID == "" {
		t.Fatalf("workspace remove should return a preview with a runnable plan id: %#v", preview)
	}
	if preview["folder"] != workspacePath || preview["folder_removed"] != false || preview["files_changed"] != false {
		t.Fatalf("workspace remove preview should report no file mutation: %#v", preview)
	}
	if preview["run_command"] != "jvs workspace remove --run "+planID {
		t.Fatalf("workspace remove preview should provide the bound run command: %#v", preview)
	}
	if cleanupCommand, _ := preview["cleanup_preview_run"].(string); !strings.Contains(cleanupCommand, "jvs cleanup preview") {
		t.Fatalf("workspace remove preview should point cleanup to a later reviewed preview: %#v", preview)
	}

	run := jvsJSONData(t, repoPath, "workspace", "remove", "--run", planID)
	if run["mode"] != "run" || run["workspace"] != "experiment" || run["status"] != "removed" {
		t.Fatalf("workspace remove run mismatch: %#v", run)
	}
	if run["folder"] != workspacePath || run["folder_removed"] != true || run["workspace_metadata_removed"] != true || run["save_point_storage_removed"] != false {
		t.Fatalf("workspace remove run should delete only workspace folder/metadata: %#v", run)
	}
	if cleanupCommand, _ := run["cleanup_command"].(string); !strings.Contains(cleanupCommand, "jvs cleanup preview") {
		t.Fatalf("workspace remove run should leave cleanup as a later reviewed step: %#v", run)
	}
	requireAbsolutePathMissing(t, workspacePath)
	pathOut, pathErr, pathCode := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "experiment")
	if pathCode == 0 {
		t.Fatalf("workspace metadata should be removed after run: stdout=%s stderr=%s", pathOut, pathErr)
	}
	requirePureJSONEnvelope(t, pathOut, pathErr, false)

	viewOut := jvsJSON(t, repoPath, "view", experimentSave, "result.txt")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if got := readAbsoluteFile(t, viewPath); got != "experiment result\n" {
		t.Fatalf("removed workspace save point storage should still be viewable before cleanup: %q", got)
	}
	closeView(t, repoPath, viewOut)

	cleanupPreview := jvsJSONData(t, repoPath, "cleanup", "preview")
	cleanupPlanID, _ := cleanupPreview["plan_id"].(string)
	if cleanupPlanID == "" {
		t.Fatalf("cleanup remains a later reviewed preview step: %#v", cleanupPreview)
	}
	requireJSONStringArrayContains(t, cleanupPreview["reclaimable_save_points"], experimentSave)
	requireHistoryIDs(t, repoPath, []string{base})
}

func TestStoryJSON_CleanupPreviewProtectsOpenReadOnlyViewUntilClosed(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "main\n"})
	base := savePoint(t, repoPath, "main base")
	created := jvsJSONData(t, repoPath, "workspace", "new", "../view-check", "--from", base)
	workspacePath, _ := created["folder"].(string)
	if workspacePath == "" {
		t.Fatalf("workspace new missing folder: %#v", created)
	}
	createFiles(t, workspacePath, map[string]string{"artifact.txt": "view protected\n"})
	viewedSave := savePointFromCWD(t, workspacePath, "view protected artifact")

	viewOut := jvsJSON(t, workspacePath, "view", viewedSave, "artifact.txt")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewData["read_only"] != true {
		t.Fatalf("view should be read-only: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != "view protected\n" {
		t.Fatalf("view content mismatch before cleanup preview: %q", got)
	}

	removeWorkspaceForStorySetup(t, repoPath, "view-check")
	requireAbsolutePathMissing(t, workspacePath)

	protectedPreview := jvsJSONData(t, repoPath, "cleanup", "preview")
	protectedPlanID, _ := protectedPreview["plan_id"].(string)
	if protectedPlanID == "" {
		t.Fatalf("cleanup preview missing plan id while view is open: %#v", protectedPreview)
	}
	requireCleanupProtectionGroups(t, protectedPreview, "history", "open_view")
	requireCleanupProtectionContainsSavePoint(t, protectedPreview, "open_view", viewedSave)
	requireJSONStringArrayContains(t, protectedPreview["protected_save_points"], viewedSave)
	requireJSONStringArrayNotContains(t, protectedPreview["reclaimable_save_points"], viewedSave)
	if got := readAbsoluteFile(t, viewPath); got != "view protected\n" {
		t.Fatalf("cleanup preview should not disturb open view content: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{base})

	closeView(t, repoPath, viewOut)
	freshPreview := jvsJSONData(t, repoPath, "cleanup", "preview")
	freshPlanID, _ := freshPreview["plan_id"].(string)
	if freshPlanID == "" || freshPlanID == protectedPlanID {
		t.Fatalf("fresh cleanup preview should have a new plan id after closing view: before=%q after=%#v", protectedPlanID, freshPreview)
	}
	requireCleanupProtectionOmitsSavePoint(t, freshPreview, "open_view", viewedSave)
	requireJSONStringArrayNotContains(t, freshPreview["protected_save_points"], viewedSave)
	requireJSONStringArrayContains(t, freshPreview["reclaimable_save_points"], viewedSave)
	requireHistoryIDs(t, repoPath, []string{base})

	listOut := jvsJSON(t, repoPath, "workspace", "list")
	if strings.Contains(listOut, "view-check") {
		t.Fatalf("removed workspace should not reappear during cleanup previews: %s", listOut)
	}
}

func removeWorkspaceForStorySetup(t *testing.T, repoPath, workspaceName string) {
	t.Helper()
	remove := jvsJSONData(t, repoPath, "workspace", "remove", workspaceName, "--force")
	if remove["status"] == "removed" {
		return
	}
	if remove["mode"] != "preview" {
		t.Fatalf("workspace remove setup should remove or preview removal: %#v", remove)
	}
	planID, _ := remove["plan_id"].(string)
	if planID == "" {
		t.Fatalf("workspace remove setup preview missing plan id: %#v", remove)
	}
	run := jvsJSONData(t, repoPath, "workspace", "remove", "--run", planID)
	if run["status"] != "removed" {
		t.Fatalf("workspace remove setup run failed: %#v", run)
	}
}
