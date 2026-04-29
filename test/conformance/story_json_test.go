//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoryJSON_ManagedFolderSaveRestorePreviewFirst(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"README.md": "managed workspace\n",
	})
	setup := savePoint(t, repoPath, "project setup")

	createFiles(t, repoPath, map[string]string{
		"managed/config.txt": "version=baseline\n",
		"managed/report.txt": "baseline report\n",
		"src/app.txt":        "behavior=baseline\n",
	})
	statusBefore := jvsJSONData(t, repoPath, "status")
	if statusBefore["workspace"] != "main" || statusBefore["unsaved_changes"] != true {
		t.Fatalf("status before managed baseline should show main with unsaved work: %#v", statusBefore)
	}

	baseline := savePoint(t, repoPath, "managed report baseline")
	createFiles(t, repoPath, map[string]string{
		"managed/config.txt": "version=update\n",
		"managed/report.txt": "updated report\n",
		"src/app.txt":        "behavior=update\n",
	})
	updated := savePoint(t, repoPath, "managed report update")
	requireDifferentSavePoints(t, baseline, updated)
	requireHistoryIDs(t, repoPath, []string{updated, baseline, setup})
	requireHistoryGrepIDs(t, repoPath, "managed report", []string{updated, baseline})

	viewOut := jvsJSON(t, repoPath, "view", baseline, "managed/report.txt")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewData["read_only"] != true {
		t.Fatalf("view should be read-only: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != "baseline report\n" {
		t.Fatalf("view baseline report = %q", got)
	}
	closeView(t, repoPath, viewOut)

	preview := jvsJSONData(t, repoPath, "restore", baseline)
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || planID == "" {
		t.Fatalf("restore must be preview-first with a plan id: %#v", preview)
	}
	if preview["source_save_point"] != baseline || preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("restore preview mutated public state: %#v", preview)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "updated report\n" {
		t.Fatalf("preview changed report.txt: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{updated, baseline, setup})

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_save_point"] != baseline || restored["history_changed"] != false {
		t.Fatalf("restore run JSON mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "baseline report\n" {
		t.Fatalf("restore run report.txt = %q", got)
	}
	if got := readFile(t, repoPath, "managed/config.txt"); got != "version=baseline\n" {
		t.Fatalf("restore run config.txt = %q", got)
	}
	statusAfter := jvsJSONData(t, repoPath, "status")
	if statusAfter["newest_save_point"] != updated || statusAfter["history_head"] != updated || statusAfter["content_source"] != baseline {
		t.Fatalf("restore should leave history head at update and file source at baseline: %#v", statusAfter)
	}
	requireHistoryIDs(t, repoPath, []string{updated, baseline, setup})
}

func TestStoryJSON_ManagedPathRecoveryRestoresOnlyTargetPath(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "report v1\n",
		"managed/notes.txt":  "notes v1\n",
		"cache/tmp.bin":      "cache v1\n",
	})
	goodReport := savePoint(t, repoPath, "managed report before bad edit")
	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "bad report edit\n",
		"managed/notes.txt":  "notes v2\n",
		"cache/tmp.bin":      "cache v2\n",
	})
	badEdit := savePoint(t, repoPath, "bad managed report edit")

	historyPath := jvsJSONData(t, repoPath, "history", "--path", "managed/report.txt")
	if historyPath["path"] != "managed/report.txt" {
		t.Fatalf("history --path normalized path mismatch: %#v", historyPath)
	}
	requireCandidateSavePoint(t, historyPath["candidates"], goodReport)
	requireCandidateSavePoint(t, historyPath["candidates"], badEdit)

	viewOut := jvsJSON(t, repoPath, "view", goodReport, "managed/report.txt")
	viewPath, _ := decodeContractDataMap(t, viewOut)["view_path"].(string)
	if got := readAbsoluteFile(t, viewPath); got != "report v1\n" {
		t.Fatalf("view saved report = %q", got)
	}
	closeView(t, repoPath, viewOut)

	preview := jvsJSONData(t, repoPath, "restore", goodReport, "--path", "managed/report.txt")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["scope"] != "path" || preview["path"] != "managed/report.txt" || planID == "" {
		t.Fatalf("path restore preview mismatch: %#v", preview)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "bad report edit\n" {
		t.Fatalf("path restore preview changed target: %q", got)
	}
	if got := readFile(t, repoPath, "managed/notes.txt"); got != "notes v2\n" {
		t.Fatalf("path restore preview changed neighboring file: %q", got)
	}

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_path"] != "managed/report.txt" || restored["history_changed"] != false {
		t.Fatalf("path restore run mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "report v1\n" {
		t.Fatalf("restored report = %q", got)
	}
	if got := readFile(t, repoPath, "managed/notes.txt"); got != "notes v2\n" {
		t.Fatalf("path restore should not change notes.txt: %q", got)
	}
	if got := readFile(t, repoPath, "cache/tmp.bin"); got != "cache v2\n" {
		t.Fatalf("path restore should not change cache file: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{badEdit, goodReport})
	status := jvsJSONData(t, repoPath, "status")
	requirePublicPathSource(t, status["path_sources"], "managed/report.txt", goodReport)
}

func TestStoryJSON_ManagedFolderRetryRestoresWholeFolderWithSafetyChoice(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "baseline report\n",
		"managed/state.txt":  "baseline complete\n",
	})
	baseline := savePoint(t, repoPath, "managed baseline state")
	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "reviewed report\n",
		"managed/state.txt":  "reviewed complete\n",
	})
	reviewed := savePoint(t, repoPath, "managed reviewed state")
	historyBeforeRetry := []string{reviewed, baseline}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt":    "broken local edit\n",
		"managed/generated.tmp": "partial local output\n",
		"managed/state.txt":     "failed local edit\n",
	})

	preview := jvsJSONData(t, repoPath, "restore", reviewed, "--discard-unsaved")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != reviewed || planID == "" {
		t.Fatalf("managed retry restore preview mismatch: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("managed retry preview should not mutate files or history: %#v", preview)
	}
	requireDiscardUnsavedOption(t, preview["options"])
	if got := readFile(t, repoPath, "managed/state.txt"); got != "failed local edit\n" {
		t.Fatalf("preview changed failed local state: %q", got)
	}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_save_point"] != reviewed || restored["history_changed"] != false {
		t.Fatalf("managed retry restore run mismatch: %#v", restored)
	}
	if restored["content_source"] != reviewed {
		t.Fatalf("restore run content_source = %#v, want %s", restored["content_source"], reviewed)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "reviewed report\n" {
		t.Fatalf("restore should recover reviewed report: %q", got)
	}
	if got := readFile(t, repoPath, "managed/state.txt"); got != "reviewed complete\n" {
		t.Fatalf("restore should recover reviewed state: %q", got)
	}
	requirePathMissing(t, repoPath, "managed/generated.tmp")

	status := jvsJSONData(t, repoPath, "status")
	if status["newest_save_point"] != reviewed || status["history_head"] != reviewed || status["content_source"] != reviewed {
		t.Fatalf("status after restore should point at reviewed state: %#v", status)
	}
	if status["unsaved_changes"] != false || status["files_state"] != "matches_save_point" {
		t.Fatalf("status after restore should be clean: %#v", status)
	}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	createFiles(t, repoPath, map[string]string{
		"managed/generated.txt": "recomputed output\n",
		"managed/state.txt":     "retry complete\n",
	})
	retry := savePoint(t, repoPath, "managed retry complete")
	requireDifferentSavePoints(t, reviewed, retry)
	requireHistoryIDs(t, repoPath, []string{retry, reviewed, baseline})
	requireHistoryGrepIDs(t, repoPath, "managed", []string{retry, reviewed, baseline})

	retryView := jvsJSON(t, repoPath, "view", retry, "managed/generated.txt")
	retryViewPath, _ := decodeContractDataMap(t, retryView)["view_path"].(string)
	if got := readAbsoluteFile(t, retryViewPath); got != "recomputed output\n" {
		t.Fatalf("saved retry output = %q", got)
	}
	closeView(t, repoPath, retryView)

	statusAfterRetry := jvsJSONData(t, repoPath, "status")
	if statusAfterRetry["newest_save_point"] != retry || statusAfterRetry["history_head"] != retry || statusAfterRetry["content_source"] != retry {
		t.Fatalf("status after retry save should point at retry result: %#v", statusAfterRetry)
	}
	if statusAfterRetry["unsaved_changes"] != false || statusAfterRetry["files_state"] != "matches_save_point" {
		t.Fatalf("status after retry save should be clean: %#v", statusAfterRetry)
	}
}

func TestStoryJSON_MistakenDeletionRecoveryRestoresOnlyDeletedPath(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "Report\n\nOpening draft.\n",
		"managed/notes.txt":  "Notes\n\nNext outline.\n",
	})
	reportDraft := savePoint(t, repoPath, "managed report draft")
	createFiles(t, repoPath, map[string]string{
		"managed/report.txt": "Report\n\nPolished draft.\n",
		"managed/notes.txt":  "Notes\n\nNext outline.\n",
	})
	latest := savePoint(t, repoPath, "managed report polished")
	requireHistoryIDs(t, repoPath, []string{latest, reportDraft})

	removePath(t, repoPath, "managed/report.txt")
	createFiles(t, repoPath, map[string]string{
		"managed/notes.txt": "Notes\n\nUnrelated work kept going.\n",
	})
	requirePathMissing(t, repoPath, "managed/report.txt")

	historyPath := jvsJSONData(t, repoPath, "history", "--path", "managed/report.txt")
	if historyPath["path"] != "managed/report.txt" {
		t.Fatalf("deletion recovery history path mismatch: %#v", historyPath)
	}
	requireCandidateSavePoint(t, historyPath["candidates"], reportDraft)
	if _, ok := historyPath["next_commands"].([]any); !ok {
		t.Fatalf("history --path should include next commands: %#v", historyPath)
	}
	requirePathMissing(t, repoPath, "managed/report.txt")
	if got := readFile(t, repoPath, "managed/notes.txt"); got != "Notes\n\nUnrelated work kept going.\n" {
		t.Fatalf("history --path changed unrelated work: %q", got)
	}

	viewOut := jvsJSON(t, repoPath, "view", reportDraft, "managed/report.txt")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewData["read_only"] != true {
		t.Fatalf("deletion recovery view should be read-only: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != "Report\n\nOpening draft.\n" {
		t.Fatalf("read-only view showed wrong report: %q", got)
	}
	closeView(t, repoPath, viewOut)

	preview := jvsJSONData(t, repoPath, "restore", reportDraft, "--path", "managed/report.txt", "--discard-unsaved")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["scope"] != "path" || preview["path"] != "managed/report.txt" || planID == "" {
		t.Fatalf("deletion recovery restore preview mismatch: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("deletion recovery preview should not mutate files or history: %#v", preview)
	}
	requirePathMissing(t, repoPath, "managed/report.txt")
	if got := readFile(t, repoPath, "managed/notes.txt"); got != "Notes\n\nUnrelated work kept going.\n" {
		t.Fatalf("path restore preview changed unrelated work: %q", got)
	}

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_path"] != "managed/report.txt" || restored["history_changed"] != false {
		t.Fatalf("deletion recovery restore run mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "managed/report.txt"); got != "Report\n\nOpening draft.\n" {
		t.Fatalf("restored deleted report = %q", got)
	}
	if got := readFile(t, repoPath, "managed/notes.txt"); got != "Notes\n\nUnrelated work kept going.\n" {
		t.Fatalf("path restore should keep unrelated work: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{latest, reportDraft})

	status := jvsJSONData(t, repoPath, "status")
	requirePublicPathSource(t, status["path_sources"], "managed/report.txt", reportDraft)

	recovered := savePoint(t, repoPath, "recover managed report")
	requireHistoryIDs(t, repoPath, []string{recovered, latest, reportDraft})
}

func TestStoryJSON_WorkspaceFromSavePointIsolation(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"managed/task.txt": "baseline task\n",
		"src/app.txt":      "main baseline\n",
	})
	base := savePoint(t, repoPath, "workspace baseline")
	createFiles(t, repoPath, map[string]string{"src/app.txt": "main local work\n"})

	created := jvsJSONData(t, repoPath, "workspace", "new", "copy-a", "--from", base)
	copyPath, _ := created["folder"].(string)
	if created["workspace"] != "copy-a" || created["started_from_save_point"] != base || copyPath == "" {
		t.Fatalf("workspace new JSON mismatch: %#v", created)
	}
	if sameCleanPath(copyPath, repoPath) {
		t.Fatalf("new workspace must be a distinct real folder, got %s", copyPath)
	}
	if info, err := os.Stat(copyPath); err != nil || !info.IsDir() {
		t.Fatalf("new workspace folder is not a real directory: info=%v err=%v", info, err)
	}
	if got := readFile(t, repoPath, "src/app.txt"); got != "main local work\n" {
		t.Fatalf("workspace new changed source main folder: %q", got)
	}
	if got := readFile(t, copyPath, "src/app.txt"); got != "main baseline\n" {
		t.Fatalf("new workspace did not start from save point: %q", got)
	}

	targetedStatusOut := jvsJSONFrom(t, copyPath, "--repo", repoPath, "--workspace", "copy-a", "status")
	targetedStatus := decodeContractDataMap(t, targetedStatusOut)
	if targetedStatus["workspace"] != "copy-a" || targetedStatus["folder"] != copyPath {
		t.Fatalf("new workspace folder with explicit --repo/--workspace should target itself: %#v", targetedStatus)
	}

	copyStatus := jvsJSONData(t, copyPath, "status")
	if copyStatus["workspace"] != "copy-a" || copyStatus["newest_save_point"] != nil || copyStatus["started_from_save_point"] != base {
		t.Fatalf("new workspace status mismatch: %#v", copyStatus)
	}
	copyHistory := jvsJSONData(t, copyPath, "history")
	if got := savePointIDsFromHistory(t, copyHistory); len(got) != 0 {
		t.Fatalf("new workspace should not inherit main history, got %v", got)
	}

	createFiles(t, copyPath, map[string]string{"src/app.txt": "copy workspace edit\n"})
	copyFirst := savePointFromCWD(t, copyPath, "copy first save")
	requireHistoryIDsInCWD(t, copyPath, []string{copyFirst})
	requireHistoryIDs(t, repoPath, []string{base})
	if got := readFile(t, repoPath, "src/app.txt"); got != "main local work\n" {
		t.Fatalf("workspace save changed source main folder: %q", got)
	}

	createFiles(t, copyPath, map[string]string{"src/app.txt": "copy workspace explicit target\n"})
	explicitSaveOut := jvsJSONFrom(t, copyPath, "--repo", repoPath, "--workspace", "copy-a", "save", "-m", "copy explicit target")
	explicitSave := decodeContractDataMap(t, explicitSaveOut)
	copySecond, _ := explicitSave["save_point_id"].(string)
	if explicitSave["workspace"] != "copy-a" || copySecond == "" {
		t.Fatalf("explicitly targeted save from workspace folder mismatch: %#v", explicitSave)
	}
	requireHistoryIDsInCWD(t, copyPath, []string{copySecond, copyFirst})
	requireHistoryIDs(t, repoPath, []string{base})
}

func TestStoryJSON_TargetingMainFromNonTargetCWD(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "main baseline\n"})
	mainBase := savePoint(t, repoPath, "main baseline")
	created := jvsJSONData(t, repoPath, "workspace", "new", "copy-b", "--from", mainBase)
	copyPath, _ := created["folder"].(string)
	createFiles(t, copyPath, map[string]string{"README.md": "copy workspace work\n"})

	otherCWD := filepath.Join(t.TempDir(), "not-the-target")
	if err := os.MkdirAll(otherCWD, 0755); err != nil {
		t.Fatalf("create outside cwd: %v", err)
	}

	mainHistoryOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "history")
	mainHistory := decodeContractDataMap(t, mainHistoryOut)
	if mainHistory["workspace"] != "main" {
		t.Fatalf("--repo history should default to main workspace, got %#v", mainHistory)
	}
	if got := savePointIDsFromHistory(t, mainHistory); !sameStringSlice(got, []string{mainBase}) {
		t.Fatalf("--repo main history IDs = %v, want [%s]", got, mainBase)
	}

	statusOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "--workspace", "main", "status")
	status := decodeContractDataMap(t, statusOut)
	if status["workspace"] != "main" || status["folder"] != repoPath || status["newest_save_point"] != mainBase {
		t.Fatalf("--repo --workspace main status mismatch: %#v", status)
	}

	copySaveOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "--workspace", "copy-b", "save", "-m", "copy workspace save")
	copySave := decodeContractDataMap(t, copySaveOut)
	copyRun, _ := copySave["save_point_id"].(string)
	if copySave["workspace"] != "copy-b" || copySave["started_from_save_point"] != mainBase || copyRun == "" {
		t.Fatalf("--repo --workspace copy save mismatch: %#v", copySave)
	}
	if got := readFile(t, repoPath, "README.md"); got != "main baseline\n" {
		t.Fatalf("targeted workspace save changed main folder: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{mainBase})

	copyHistoryOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "--workspace", "copy-b", "history")
	copyHistory := decodeContractDataMap(t, copyHistoryOut)
	if got := savePointIDsFromHistory(t, copyHistory); !sameStringSlice(got, []string{copyRun}) {
		t.Fatalf("--repo --workspace copy history IDs = %v, want [%s]", got, copyRun)
	}
}
