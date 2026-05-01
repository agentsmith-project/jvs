//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
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

func TestStoryJSON_DirtyRestoreDecisionPreviewShowsImpactBeforeSafetyChoice(t *testing.T) {
	t.Run("whole folder", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()

		createFiles(t, repoPath, map[string]string{
			"app.txt":  "v1\n",
			"keep.txt": "keep v1\n",
		})
		first := savePoint(t, repoPath, "first")
		createFiles(t, repoPath, map[string]string{
			"app.txt":       "v2\n",
			"generated.txt": "generated v2\n",
			"keep.txt":      "keep v1\n",
		})
		second := savePoint(t, repoPath, "second")
		historyBefore := []string{second, first}

		createFiles(t, repoPath, map[string]string{
			"app.txt":       "unsaved local edit\n",
			"generated.txt": "generated local edit\n",
		})

		stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", first)
		if got := readFile(t, repoPath, "app.txt"); got != "unsaved local edit\n" {
			t.Fatalf("dirty restore decision preview changed app.txt: %q", got)
		}
		if got := readFile(t, repoPath, "generated.txt"); got != "generated local edit\n" {
			t.Fatalf("dirty restore decision preview changed generated.txt: %q", got)
		}
		requireHistoryIDs(t, repoPath, historyBefore)
		if code != 0 {
			t.Fatalf("dirty restore should show a non-mutating decision preview before safety choice: stdout=%s stderr=%s", stdout, stderr)
		}
		requirePureJSONEnvelope(t, stdout, stderr, true)
		decision := decodeContractDataMap(t, stdout)
		if decision["mode"] != "decision_preview" || decision["source_save_point"] != first {
			t.Fatalf("dirty restore decision preview mismatch: %#v", decision)
		}
		if decision["history_changed"] != false || decision["files_changed"] != false {
			t.Fatalf("dirty restore decision preview must not mutate files or history: %#v", decision)
		}
		requireNoRunnableRestorePlan(t, decision)
		requireManagedFilesImpactAtLeast(t, decision, "overwrite", 1)
		requireManagedFilesImpactAtLeast(t, decision, "delete", 1)
		if !strings.Contains(stdout, "--discard-unsaved") || !strings.Contains(stdout, "--save-first") {
			t.Fatalf("dirty restore decision preview should show explicit safety choices: %s", stdout)
		}
	})

	t.Run("path", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()

		createFiles(t, repoPath, map[string]string{
			"app.txt":   "v1\n",
			"notes.txt": "notes v1\n",
		})
		first := savePoint(t, repoPath, "first")
		createFiles(t, repoPath, map[string]string{
			"app.txt":   "v2\n",
			"notes.txt": "notes v2\n",
		})
		second := savePoint(t, repoPath, "second")
		historyBefore := []string{second, first}

		createFiles(t, repoPath, map[string]string{
			"app.txt":   "unsaved app edit\n",
			"notes.txt": "unsaved notes edit\n",
		})

		stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", first, "--path", "app.txt")
		if got := readFile(t, repoPath, "app.txt"); got != "unsaved app edit\n" {
			t.Fatalf("dirty path restore decision preview changed target: %q", got)
		}
		if got := readFile(t, repoPath, "notes.txt"); got != "unsaved notes edit\n" {
			t.Fatalf("dirty path restore decision preview changed neighboring file: %q", got)
		}
		requireHistoryIDs(t, repoPath, historyBefore)
		if code != 0 {
			t.Fatalf("dirty path restore should show a non-mutating decision preview before safety choice: stdout=%s stderr=%s", stdout, stderr)
		}
		requirePureJSONEnvelope(t, stdout, stderr, true)
		decision := decodeContractDataMap(t, stdout)
		if decision["mode"] != "decision_preview" || decision["scope"] != "path" || decision["path"] != "app.txt" || decision["source_save_point"] != first {
			t.Fatalf("dirty path restore decision preview mismatch: %#v", decision)
		}
		if decision["history_changed"] != false || decision["files_changed"] != false {
			t.Fatalf("dirty path restore decision preview must not mutate files or history: %#v", decision)
		}
		requireNoRunnableRestorePlan(t, decision)
		requireManagedFilesImpactAtLeast(t, decision, "overwrite", 1)
		if !strings.Contains(stdout, "--discard-unsaved") || !strings.Contains(stdout, "--save-first") {
			t.Fatalf("dirty path restore decision preview should show explicit safety choices: %s", stdout)
		}
	})
}

func TestStoryJSON_DirtyRestoreExplicitSafetyChoicesCreateRunnablePlans(t *testing.T) {
	t.Run("discard unsaved whole folder", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()

		createFiles(t, repoPath, map[string]string{
			"app.txt":       "v1\n",
			"generated.txt": "generated v1\n",
		})
		first := savePoint(t, repoPath, "first")
		createFiles(t, repoPath, map[string]string{
			"app.txt":       "v2\n",
			"generated.txt": "generated v2\n",
		})
		second := savePoint(t, repoPath, "second")
		historyBefore := []string{second, first}
		createFiles(t, repoPath, map[string]string{
			"app.txt":       "throw away me\n",
			"generated.txt": "throw away generated\n",
		})

		preview := jvsJSONData(t, repoPath, "restore", first, "--discard-unsaved")
		planID, _ := preview["plan_id"].(string)
		if preview["mode"] != "preview" || preview["source_save_point"] != first || planID == "" {
			t.Fatalf("discard-unsaved restore preview mismatch: %#v", preview)
		}
		if preview["history_changed"] != false || preview["files_changed"] != false {
			t.Fatalf("discard-unsaved preview must not mutate files or history: %#v", preview)
		}
		requireDiscardUnsavedOption(t, preview["options"])
		if got := readFile(t, repoPath, "app.txt"); got != "throw away me\n" {
			t.Fatalf("discard-unsaved preview changed app.txt: %q", got)
		}
		requireHistoryIDs(t, repoPath, historyBefore)

		run := jvsJSONData(t, repoPath, "restore", "--run", planID)
		if run["mode"] != "run" || run["restored_save_point"] != first || run["content_source"] != first {
			t.Fatalf("discard-unsaved restore run mismatch: %#v", run)
		}
		if got := readFile(t, repoPath, "app.txt"); got != "v1\n" {
			t.Fatalf("discard-unsaved restore run app.txt = %q", got)
		}
		requireHistoryIDs(t, repoPath, historyBefore)
	})

	t.Run("save first path", func(t *testing.T) {
		repoPath, cleanup := initTestRepo(t)
		defer cleanup()

		createFiles(t, repoPath, map[string]string{
			"app.txt":   "v1\n",
			"notes.txt": "notes v1\n",
		})
		first := savePoint(t, repoPath, "first")
		createFiles(t, repoPath, map[string]string{
			"app.txt":   "v2\n",
			"notes.txt": "notes v2\n",
		})
		second := savePoint(t, repoPath, "second")
		createFiles(t, repoPath, map[string]string{
			"app.txt":   "local app before path restore\n",
			"notes.txt": "local notes kept outside path restore\n",
		})

		preview := jvsJSONData(t, repoPath, "restore", first, "--path", "app.txt", "--save-first")
		planID, _ := preview["plan_id"].(string)
		if preview["mode"] != "preview" || preview["scope"] != "path" || preview["path"] != "app.txt" || planID == "" {
			t.Fatalf("save-first path restore preview mismatch: %#v", preview)
		}
		if preview["history_changed"] != false || preview["files_changed"] != false {
			t.Fatalf("save-first path preview must not mutate files or history: %#v", preview)
		}
		requireSaveFirstOption(t, preview["options"])
		requireHistoryIDs(t, repoPath, []string{second, first})
		if got := readFile(t, repoPath, "app.txt"); got != "local app before path restore\n" {
			t.Fatalf("save-first path preview changed app.txt: %q", got)
		}

		run := jvsJSONData(t, repoPath, "restore", "--run", planID)
		safetySave, _ := run["newest_save_point"].(string)
		if run["mode"] != "run" || run["restored_path"] != "app.txt" || run["source_save_point"] != first || safetySave == "" {
			t.Fatalf("save-first path restore run mismatch: %#v", run)
		}
		requireDifferentSavePoints(t, safetySave, second)
		requireHistoryIDs(t, repoPath, []string{safetySave, second, first})
		if got := readFile(t, repoPath, "app.txt"); got != "v1\n" {
			t.Fatalf("save-first path restore run app.txt = %q", got)
		}
		if got := readFile(t, repoPath, "notes.txt"); got != "local notes kept outside path restore\n" {
			t.Fatalf("path restore should keep neighboring dirty file: %q", got)
		}

		viewOut := jvsJSON(t, repoPath, "view", safetySave, "app.txt")
		viewPath, _ := decodeContractDataMap(t, viewOut)["view_path"].(string)
		if got := readAbsoluteFile(t, viewPath); got != "local app before path restore\n" {
			t.Fatalf("save-first safety save did not capture dirty path content: %q", got)
		}
		closeView(t, repoPath, viewOut)
	})
}

func TestStoryJSON_SaveFirstProtectsCurrentWorkBeforeOlderRestore(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"app.txt":  "v1\n",
		"note.txt": "note v1\n",
	})
	first := savePoint(t, repoPath, "first")
	createFiles(t, repoPath, map[string]string{
		"app.txt":  "v2\n",
		"note.txt": "note v2\n",
	})
	second := savePoint(t, repoPath, "second")
	createFiles(t, repoPath, map[string]string{
		"app.txt":  "local app before older restore\n",
		"note.txt": "local note before older restore\n",
	})

	preview := jvsJSONData(t, repoPath, "restore", first, "--save-first")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != first || planID == "" {
		t.Fatalf("save-first restore preview mismatch: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false || preview["expected_newest_save_point"] != second {
		t.Fatalf("save-first preview should not mutate and should keep expected newest at current head: %#v", preview)
	}
	requireSaveFirstOption(t, preview["options"])
	requireHistoryIDs(t, repoPath, []string{second, first})

	run := jvsJSONData(t, repoPath, "restore", "--run", planID)
	safetySave, _ := run["newest_save_point"].(string)
	if run["mode"] != "run" || run["restored_save_point"] != first || run["content_source"] != first {
		t.Fatalf("save-first restore run mismatch: %#v", run)
	}
	requireDifferentSavePoints(t, safetySave, second)
	if run["history_head"] != safetySave {
		t.Fatalf("save-first restore should explain safety save as history head: %#v", run)
	}
	if run["history_changed"] != false || run["unsaved_changes"] != false || run["files_state"] != "matches_save_point" {
		t.Fatalf("save-first restore should finish clean without treating restore as history rewrite: %#v", run)
	}
	if got := readFile(t, repoPath, "app.txt"); got != "v1\n" {
		t.Fatalf("save-first restore app.txt = %q", got)
	}
	if got := readFile(t, repoPath, "note.txt"); got != "note v1\n" {
		t.Fatalf("save-first restore note.txt = %q", got)
	}

	history := jvsJSONData(t, repoPath, "history", "to", safetySave)
	requireHistoryIDs(t, repoPath, []string{safetySave, second, first})
	requireHistoryRecordMessage(t, history, safetySave, "save before restore")

	status := jvsJSONData(t, repoPath, "status")
	if status["newest_save_point"] != safetySave || status["history_head"] != safetySave || status["content_source"] != first {
		t.Fatalf("status after save-first restore should distinguish history head from restored content source: %#v", status)
	}
	if status["unsaved_changes"] != false || status["files_state"] != "matches_save_point" {
		t.Fatalf("status after save-first restore should be clean: %#v", status)
	}

	appView := jvsJSON(t, repoPath, "view", safetySave, "app.txt")
	appViewPath, _ := decodeContractDataMap(t, appView)["view_path"].(string)
	if got := readAbsoluteFile(t, appViewPath); got != "local app before older restore\n" {
		t.Fatalf("safety save should preserve local app work: %q", got)
	}
	closeView(t, repoPath, appView)

	noteView := jvsJSON(t, repoPath, "view", safetySave, "note.txt")
	noteViewPath, _ := decodeContractDataMap(t, noteView)["view_path"].(string)
	if got := readAbsoluteFile(t, noteViewPath); got != "local note before older restore\n" {
		t.Fatalf("safety save should preserve local note work: %q", got)
	}
	closeView(t, repoPath, noteView)
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

	created := jvsJSONData(t, repoPath, "workspace", "new", "../copy-a", "--from", base)
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
	if got := savePointIDsFromHistory(t, copyHistory); !sameStringSlice(got, []string{base}) {
		t.Fatalf("new workspace current pointer history = %v, want started-from source [%s]", got, base)
	}
	if copyHistory["newest_save_point"] != nil || copyHistory["started_from_save_point"] != base {
		t.Fatalf("new workspace should keep an empty own history head while pointing at its source: %#v", copyHistory)
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

func TestStoryJSON_WorkspaceListStatusAcrossMultipleExplicitFolders(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"brief.md":    "baseline brief\n",
		"src/app.txt": "main baseline\n",
	})
	base := savePoint(t, repoPath, "multi workspace baseline")
	reviewPath := filepath.Join(filepath.Dir(repoPath), "review-a")
	cleanPath := filepath.Join(filepath.Dir(repoPath), "clean-b")

	reviewCreated := jvsJSONData(t, repoPath, "workspace", "new", "../review-a", "--from", base)
	cleanCreated := jvsJSONData(t, repoPath, "workspace", "new", "../clean-b", "--from", base)
	if reviewCreated["workspace"] != "review-a" || reviewCreated["folder"] != reviewPath || reviewCreated["started_from_save_point"] != base {
		t.Fatalf("review workspace creation mismatch: %#v", reviewCreated)
	}
	if cleanCreated["workspace"] != "clean-b" || cleanCreated["folder"] != cleanPath || cleanCreated["started_from_save_point"] != base {
		t.Fatalf("clean workspace creation mismatch: %#v", cleanCreated)
	}

	createFiles(t, reviewPath, map[string]string{
		"analysis/notes.txt": "unsaved review notes\n",
		"src/app.txt":        "review workspace dirty edit\n",
	})

	reviewPathOut := jvsJSONData(t, repoPath, "workspace", "path", "review-a")
	cleanPathOut := jvsJSONData(t, repoPath, "workspace", "path", "clean-b")
	if reviewPathOut["path"] != reviewPath || cleanPathOut["path"] != cleanPath {
		t.Fatalf("workspace path should return cd-able explicit folders: review=%#v clean=%#v", reviewPathOut, cleanPathOut)
	}
	reviewStatus := jvsJSONData(t, reviewPath, "status")
	if reviewStatus["workspace"] != "review-a" || reviewStatus["folder"] != reviewPath || reviewStatus["unsaved_changes"] != true {
		t.Fatalf("cd into dirty review workspace should target itself and show unsaved work: %#v", reviewStatus)
	}
	cleanStatus := jvsJSONData(t, cleanPath, "status")
	if cleanStatus["workspace"] != "clean-b" || cleanStatus["folder"] != cleanPath || cleanStatus["unsaved_changes"] != false {
		t.Fatalf("cd into clean workspace should target itself and remain clean: %#v", cleanStatus)
	}

	listDefault := jvsJSONValue(t, repoPath, "workspace", "list")
	mainDefault := workspaceListRecord(t, listDefault, "main")
	reviewDefault := workspaceListRecord(t, listDefault, "review-a")
	cleanDefault := workspaceListRecord(t, listDefault, "clean-b")
	if mainDefault["current"] != true || mainDefault["folder"] != repoPath || mainDefault["content_source"] != base || mainDefault["newest_save_point"] != base || mainDefault["history_head"] != base {
		t.Fatalf("default list should show main folder and pointers without unrelated fields: %#v", mainDefault)
	}
	if reviewDefault["current"] != false || reviewDefault["folder"] != reviewPath || reviewDefault["content_source"] != base || reviewDefault["newest_save_point"] != nil || reviewDefault["history_head"] != nil || reviewDefault["started_from_save_point"] != base {
		t.Fatalf("default list should show review folder/source without own history: %#v", reviewDefault)
	}
	if cleanDefault["current"] != false || cleanDefault["folder"] != cleanPath || cleanDefault["content_source"] != base || cleanDefault["newest_save_point"] != nil || cleanDefault["history_head"] != nil || cleanDefault["started_from_save_point"] != base {
		t.Fatalf("default list should show clean folder/source without own history: %#v", cleanDefault)
	}
	for _, record := range []map[string]any{mainDefault, reviewDefault, cleanDefault} {
		if _, ok := record["unsaved_changes"]; ok {
			t.Fatalf("default workspace list should not force dirty status: %#v", record)
		}
	}

	listWithStatus := jvsJSONValue(t, repoPath, "workspace", "list", "--status")
	mainWithStatus := workspaceListRecord(t, listWithStatus, "main")
	reviewWithStatus := workspaceListRecord(t, listWithStatus, "review-a")
	cleanWithStatus := workspaceListRecord(t, listWithStatus, "clean-b")
	if mainWithStatus["unsaved_changes"] != false || reviewWithStatus["unsaved_changes"] != true || cleanWithStatus["unsaved_changes"] != false {
		t.Fatalf("workspace list --status should distinguish dirty and clean folders: main=%#v review=%#v clean=%#v", mainWithStatus, reviewWithStatus, cleanWithStatus)
	}
}

func TestStoryJSON_WorkspaceFolderRestoreLoopKeepsMainIsolated(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"app.txt":  "base app\n",
		"note.txt": "base note\n",
	})
	base := savePoint(t, repoPath, "workspace restore base")
	createFiles(t, repoPath, map[string]string{
		"app.txt": "main local work after base\n",
	})
	workspacePath := filepath.Join(filepath.Dir(repoPath), "restore-loop")
	created := jvsJSONData(t, repoPath, "workspace", "new", "../restore-loop", "--from", base)
	if created["workspace"] != "restore-loop" || created["folder"] != workspacePath || created["started_from_save_point"] != base {
		t.Fatalf("workspace creation mismatch: %#v", created)
	}

	createFiles(t, workspacePath, map[string]string{
		"app.txt":              "workspace saved edit\n",
		"workspace/result.txt": "first workspace result\n",
	})
	workspaceSave := savePointFromCWD(t, workspacePath, "workspace first result")
	workspaceHistory := jvsJSONData(t, workspacePath, "history")
	if got := savePointIDsFromHistory(t, workspaceHistory); !sameStringSlice(got, []string{workspaceSave}) {
		t.Fatalf("workspace history IDs = %v, want [%s]", got, workspaceSave)
	}
	mainHistory := jvsJSONData(t, repoPath, "history")
	if got := savePointIDsFromHistory(t, mainHistory); !sameStringSlice(got, []string{base}) {
		t.Fatalf("main history IDs = %v, want [%s]", got, base)
	}
	if got := readFile(t, repoPath, "app.txt"); got != "main local work after base\n" {
		t.Fatalf("workspace save changed main folder: %q", got)
	}

	preview := jvsJSONData(t, workspacePath, "restore", base)
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["workspace"] != "restore-loop" || preview["folder"] != workspacePath || preview["source_save_point"] != base || planID == "" {
		t.Fatalf("workspace restore preview should target the current workspace folder: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("workspace restore preview should not mutate files or history: %#v", preview)
	}
	if got := readFile(t, workspacePath, "app.txt"); got != "workspace saved edit\n" {
		t.Fatalf("workspace restore preview changed file: %q", got)
	}

	restored := jvsJSONData(t, workspacePath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["workspace"] != "restore-loop" || restored["folder"] != workspacePath || restored["restored_save_point"] != base || restored["content_source"] != base {
		t.Fatalf("workspace restore run should target workspace and restore base: %#v", restored)
	}
	if restored["newest_save_point"] != workspaceSave || restored["history_head"] != workspaceSave || restored["history_changed"] != false || restored["files_changed"] != true {
		t.Fatalf("workspace restore run should keep workspace history head while moving content source: %#v", restored)
	}
	if got := readFile(t, workspacePath, "app.txt"); got != "base app\n" {
		t.Fatalf("workspace restore did not recover base app: %q", got)
	}
	requirePathMissing(t, workspacePath, "workspace/result.txt")
	if got := readFile(t, repoPath, "app.txt"); got != "main local work after base\n" {
		t.Fatalf("workspace restore changed main folder: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{base})

	statusAfterRestore := jvsJSONData(t, workspacePath, "status")
	if statusAfterRestore["workspace"] != "restore-loop" || statusAfterRestore["folder"] != workspacePath || statusAfterRestore["content_source"] != base || statusAfterRestore["newest_save_point"] != workspaceSave || statusAfterRestore["history_head"] != workspaceSave {
		t.Fatalf("workspace status after restore should distinguish current source from history head: %#v", statusAfterRestore)
	}

	createFiles(t, workspacePath, map[string]string{
		"app.txt":              "workspace work after restore\n",
		"workspace/result.txt": "second workspace result\n",
	})
	afterRestoreSave := savePointFromCWD(t, workspacePath, "workspace after restore")
	workspaceHistory = jvsJSONData(t, workspacePath, "history")
	if got := savePointIDsFromHistory(t, workspaceHistory); !sameStringSlice(got, []string{afterRestoreSave, workspaceSave}) {
		t.Fatalf("workspace history IDs after restore save = %v, want [%s %s]", got, afterRestoreSave, workspaceSave)
	}
	mainHistory = jvsJSONData(t, repoPath, "history")
	if got := savePointIDsFromHistory(t, mainHistory); !sameStringSlice(got, []string{base}) {
		t.Fatalf("main history IDs after workspace save = %v, want [%s]", got, base)
	}
	if got := readFile(t, repoPath, "app.txt"); got != "main local work after base\n" {
		t.Fatalf("workspace save after restore changed main folder: %q", got)
	}

	finalStatus := jvsJSONData(t, workspacePath, "status")
	if finalStatus["workspace"] != "restore-loop" || finalStatus["content_source"] != afterRestoreSave || finalStatus["newest_save_point"] != afterRestoreSave || finalStatus["history_head"] != afterRestoreSave || finalStatus["started_from_save_point"] != base {
		t.Fatalf("save after workspace restore should stay in workspace history: %#v", finalStatus)
	}
}

func TestStoryJSON_WorkspaceExplicitFolderTrackingAndCurrentPointer(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"brief.md":    "baseline brief\n",
		"src/app.txt": "main baseline\n",
	})
	base := savePoint(t, repoPath, "analysis baseline")
	createFiles(t, repoPath, map[string]string{"src/app.txt": "main continues independently\n"})

	created := jvsJSONData(t, repoPath, "workspace", "new", "../analysis-run", "--from", base)
	analysisPath := filepath.Join(filepath.Dir(repoPath), "analysis-run")
	if created["workspace"] != "analysis-run" || created["folder"] != analysisPath || created["started_from_save_point"] != base {
		t.Fatalf("workspace new should report explicit folder and source save point: %#v", created)
	}
	if got := readFile(t, analysisPath, "src/app.txt"); got != "main baseline\n" {
		t.Fatalf("new workspace content = %q, want baseline", got)
	}
	if got := readFile(t, repoPath, "src/app.txt"); got != "main continues independently\n" {
		t.Fatalf("workspace creation changed main folder: %q", got)
	}

	path := jvsJSONData(t, repoPath, "workspace", "path", "analysis-run")
	if path["path"] != analysisPath {
		t.Fatalf("workspace path should return explicit folder: %#v", path)
	}

	statusBeforeSave := jvsJSONData(t, analysisPath, "status")
	if statusBeforeSave["workspace"] != "analysis-run" || statusBeforeSave["folder"] != analysisPath {
		t.Fatalf("cd into workspace should naturally target that workspace: %#v", statusBeforeSave)
	}
	if statusBeforeSave["content_source"] != base || statusBeforeSave["newest_save_point"] != nil || statusBeforeSave["history_head"] != nil || statusBeforeSave["started_from_save_point"] != base {
		t.Fatalf("new workspace should point at source before first save without owning history: %#v", statusBeforeSave)
	}

	listBeforeSave := jvsJSONValue(t, repoPath, "workspace", "list")
	mainBefore := workspaceListRecord(t, listBeforeSave, "main")
	analysisBefore := workspaceListRecord(t, listBeforeSave, "analysis-run")
	if mainBefore["folder"] != repoPath || mainBefore["content_source"] != base || mainBefore["newest_save_point"] != base {
		t.Fatalf("workspace list should show main folder and pointer: %#v", mainBefore)
	}
	if analysisBefore["folder"] != analysisPath || analysisBefore["content_source"] != base || analysisBefore["newest_save_point"] != nil || analysisBefore["started_from_save_point"] != base {
		t.Fatalf("workspace list should show new workspace source and empty own history: %#v", analysisBefore)
	}

	historyBeforeSave := jvsJSONData(t, repoPath, "history", "from", base)
	if historyBeforeSave["direction"] != "from" || historyBeforeSave["start_save_point"] != base {
		t.Fatalf("history from base should describe descendants from that save point: %#v", historyBeforeSave)
	}
	requireHistoryWorkspacePointer(t, historyBeforeSave, "main", base)
	requireHistoryWorkspacePointer(t, historyBeforeSave, "analysis-run", base)

	createFiles(t, analysisPath, map[string]string{
		"analysis/result.txt": "analysis result\n",
		"src/app.txt":         "analysis edit\n",
	})
	analysisSave := savePointFromCWD(t, analysisPath, "analysis result")
	analysisHistory := jvsJSONData(t, analysisPath, "history")
	if got := savePointIDsFromHistory(t, analysisHistory); !sameStringSlice(got, []string{analysisSave}) {
		t.Fatalf("analysis workspace history IDs = %v, want [%s]", got, analysisSave)
	}
	mainHistory := jvsJSONData(t, repoPath, "history")
	if got := savePointIDsFromHistory(t, mainHistory); !sameStringSlice(got, []string{base}) {
		t.Fatalf("main history IDs = %v, want [%s]", got, base)
	}
	if got := readFile(t, repoPath, "src/app.txt"); got != "main continues independently\n" {
		t.Fatalf("workspace save changed main folder: %q", got)
	}

	statusAfterSave := jvsJSONData(t, analysisPath, "status")
	if statusAfterSave["content_source"] != analysisSave || statusAfterSave["newest_save_point"] != analysisSave || statusAfterSave["history_head"] != analysisSave || statusAfterSave["started_from_save_point"] != base {
		t.Fatalf("workspace first save should move current pointer to its new save point: %#v", statusAfterSave)
	}

	listAfterSave := jvsJSONValue(t, analysisPath, "workspace", "list")
	mainAfter := workspaceListRecord(t, listAfterSave, "main")
	analysisAfter := workspaceListRecord(t, listAfterSave, "analysis-run")
	if mainAfter["folder"] != repoPath || mainAfter["content_source"] != base || mainAfter["newest_save_point"] != base {
		t.Fatalf("main list record should stay on base after workspace save: %#v", mainAfter)
	}
	if analysisAfter["folder"] != analysisPath || analysisAfter["content_source"] != analysisSave || analysisAfter["newest_save_point"] != analysisSave || analysisAfter["history_head"] != analysisSave {
		t.Fatalf("analysis list record should move pointer to first save: %#v", analysisAfter)
	}

	historyAfterSave := jvsJSONData(t, repoPath, "history", "from", base)
	if got := savePointIDsFromHistory(t, historyAfterSave); !sameStringSlice(got, []string{base, analysisSave}) {
		t.Fatalf("history from base IDs = %v, want base then analysis save", got)
	}
	requireHistoryEdge(t, historyAfterSave, base, analysisSave, "started_from")
	requireHistoryWorkspacePointer(t, historyAfterSave, "main", base)
	requireHistoryWorkspacePointer(t, historyAfterSave, "analysis-run", analysisSave)

	workspaceHistoryFromSource := jvsJSONData(t, analysisPath, "history", "from")
	if workspaceHistoryFromSource["direction"] != "from" || workspaceHistoryFromSource["start_save_point"] != base {
		t.Fatalf("history from inside workspace should start from its source when no save point is provided: %#v", workspaceHistoryFromSource)
	}
	if got := savePointIDsFromHistory(t, workspaceHistoryFromSource); !sameStringSlice(got, []string{base, analysisSave}) {
		t.Fatalf("workspace history from source IDs = %v, want base then analysis save", got)
	}
	requireHistoryEdge(t, workspaceHistoryFromSource, base, analysisSave, "started_from")
	requireHistoryWorkspacePointer(t, workspaceHistoryFromSource, "analysis-run", analysisSave)
}

func TestStoryJSON_WorkspaceNewRequiresExplicitFolderOutsideExistingWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"notes.txt": "baseline\n"})
	base := savePoint(t, repoPath, "explicit folder baseline")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "new", "analysis-run", "--from", base)
	if code == 0 {
		t.Fatalf("bare name-shaped workspace target should be rejected, not treated as a safe default: stdout=%s stderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, false)
	if _, err := os.Stat(filepath.Join(repoPath, "analysis-run")); err == nil {
		t.Fatalf("rejected bare name-shaped workspace target created a folder inside main workspace")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat rejected workspace target: %v", err)
	}
	pathOut, pathErr, pathCode := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "analysis-run")
	if pathCode == 0 {
		t.Fatalf("rejected workspace should not appear in registry: stdout=%s stderr=%s", pathOut, pathErr)
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "workspace", "new", "nested/analysis-run", "--from", base)
	if code == 0 {
		t.Fatalf("workspace target inside existing workspace should be rejected: stdout=%s stderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, false)
	if _, err := os.Stat(filepath.Join(repoPath, "nested", "analysis-run")); err == nil {
		t.Fatalf("rejected nested workspace target created a folder inside main workspace")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat rejected nested workspace target: %v", err)
	}

	created := jvsJSONData(t, repoPath, "workspace", "new", "../analysis-run", "--from", base)
	analysisPath := filepath.Join(filepath.Dir(repoPath), "analysis-run")
	if created["workspace"] != "analysis-run" || created["folder"] != analysisPath {
		t.Fatalf("explicit sibling folder should create workspace naturally: %#v", created)
	}
	status := jvsJSONData(t, analysisPath, "status")
	if status["workspace"] != "analysis-run" || status["folder"] != analysisPath || status["started_from_save_point"] != base {
		t.Fatalf("explicit sibling workspace should be usable from its folder: %#v", status)
	}
}

func TestStoryJSON_WorkspaceNameCanDifferFromFolderBasename(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"review.md": "baseline\n"})
	base := savePoint(t, repoPath, "review baseline")
	reviewFolder := filepath.Join(filepath.Dir(repoPath), "scratch-space")

	created := jvsJSONData(t, repoPath, "workspace", "new", "../scratch-space", "--from", base, "--name", "review-copy")
	if created["workspace"] != "review-copy" || created["folder"] != reviewFolder || created["started_from_save_point"] != base {
		t.Fatalf("--name should decouple registry name from folder basename: %#v", created)
	}
	path := jvsJSONData(t, repoPath, "workspace", "path", "review-copy")
	if path["path"] != reviewFolder {
		t.Fatalf("workspace path should use registry name and return explicit folder: %#v", path)
	}
	status := jvsJSONData(t, reviewFolder, "status")
	if status["workspace"] != "review-copy" || status["folder"] != reviewFolder {
		t.Fatalf("cd into renamed workspace folder should discover registry name: %#v", status)
	}

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "scratch-space")
	if code == 0 {
		t.Fatalf("folder basename should not become a second workspace name when --name is used: stdout=%s stderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, false)
}

func TestStoryJSON_TargetingMainFromNonTargetCWD(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "main baseline\n"})
	mainBase := savePoint(t, repoPath, "main baseline")
	created := jvsJSONData(t, repoPath, "workspace", "new", "../copy-b", "--from", mainBase)
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
