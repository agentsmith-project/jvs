//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoryJSON_MLExperimentBaselineRunRestorePreviewFirst(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"README.md": "experiment workspace\n",
	})
	setup := savePoint(t, repoPath, "project setup")

	createFiles(t, repoPath, map[string]string{
		"configs/baseline.yaml":       "run_id: run-42\nlearning_rate: 0.01\n",
		"experiments/model.py":        "LEARNING_RATE = 0.01\n",
		"outputs/run-42/metrics.json": `{"run_id":"run-42","accuracy":0.72}` + "\n",
	})
	statusBefore := jvsJSONData(t, repoPath, "status")
	if statusBefore["workspace"] != "main" || statusBefore["unsaved_changes"] != true {
		t.Fatalf("status before baseline result should show main with unsaved work: %#v", statusBefore)
	}

	baseline := savePoint(t, repoPath, "baseline result run-42")
	createFiles(t, repoPath, map[string]string{
		"configs/risky.yaml":          "run_id: run-42\nlearning_rate: 0.03\n",
		"experiments/model.py":        "LEARNING_RATE = 0.03\n",
		"outputs/run-42/metrics.json": `{"run_id":"run-42","accuracy":0.81}` + "\n",
	})
	run := savePoint(t, repoPath, "risky result run-42")
	requireDifferentSavePoints(t, baseline, run)
	requireHistoryIDs(t, repoPath, []string{run, baseline, setup})
	requireHistoryGrepIDs(t, repoPath, "run-42", []string{run, baseline})

	viewOut := jvsJSON(t, repoPath, "view", baseline, "outputs/run-42/metrics.json")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewData["read_only"] != true {
		t.Fatalf("view should be read-only: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != `{"run_id":"run-42","accuracy":0.72}`+"\n" {
		t.Fatalf("view baseline metrics = %q", got)
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
	if got := readFile(t, repoPath, "outputs/run-42/metrics.json"); got != `{"run_id":"run-42","accuracy":0.81}`+"\n" {
		t.Fatalf("preview changed metrics.json: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{run, baseline, setup})

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_save_point"] != baseline || restored["history_changed"] != false {
		t.Fatalf("restore run JSON mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "outputs/run-42/metrics.json"); got != `{"run_id":"run-42","accuracy":0.72}`+"\n" {
		t.Fatalf("restore run metrics.json = %q", got)
	}
	requirePathMissing(t, repoPath, "configs/risky.yaml")
	statusAfter := jvsJSONData(t, repoPath, "status")
	if statusAfter["newest_save_point"] != run || statusAfter["history_head"] != run || statusAfter["content_source"] != baseline {
		t.Fatalf("restore should leave history head at run and file source at baseline: %#v", statusAfter)
	}
	requireHistoryIDs(t, repoPath, []string{run, baseline, setup})
}

func TestStoryJSON_DataETLPathRecoveryRestoresOnlyTargetPath(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"data/raw/orders.csv": "id,total\n1,42\n",
		"etl/config.yml":      "source: v1\n",
		"reports/summary.txt": "baseline\n",
	})
	goodRaw := savePoint(t, repoPath, "raw data before bad ETL")
	createFiles(t, repoPath, map[string]string{
		"data/raw/orders.csv": "id,total\n1,0\n",
		"etl/config.yml":      "source: v2\n",
		"reports/summary.txt": "bad run summary\n",
	})
	badRun := savePoint(t, repoPath, "bad ETL run")

	historyPath := jvsJSONData(t, repoPath, "history", "--path", "data/raw/orders.csv")
	if historyPath["path"] != "data/raw/orders.csv" {
		t.Fatalf("history --path normalized path mismatch: %#v", historyPath)
	}
	requireCandidateSavePoint(t, historyPath["candidates"], goodRaw)
	requireCandidateSavePoint(t, historyPath["candidates"], badRun)

	viewOut := jvsJSON(t, repoPath, "view", goodRaw, "data/raw/orders.csv")
	viewPath, _ := decodeContractDataMap(t, viewOut)["view_path"].(string)
	if got := readAbsoluteFile(t, viewPath); got != "id,total\n1,42\n" {
		t.Fatalf("view old raw data = %q", got)
	}
	closeView(t, repoPath, viewOut)

	preview := jvsJSONData(t, repoPath, "restore", goodRaw, "--path", "data/raw/orders.csv")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["scope"] != "path" || preview["path"] != "data/raw/orders.csv" || planID == "" {
		t.Fatalf("path restore preview mismatch: %#v", preview)
	}
	if got := readFile(t, repoPath, "data/raw/orders.csv"); got != "id,total\n1,0\n" {
		t.Fatalf("path restore preview changed target: %q", got)
	}
	if got := readFile(t, repoPath, "etl/config.yml"); got != "source: v2\n" {
		t.Fatalf("path restore preview changed neighboring path: %q", got)
	}

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_path"] != "data/raw/orders.csv" || restored["history_changed"] != false {
		t.Fatalf("path restore run mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "data/raw/orders.csv"); got != "id,total\n1,42\n" {
		t.Fatalf("restored raw data = %q", got)
	}
	if got := readFile(t, repoPath, "etl/config.yml"); got != "source: v2\n" {
		t.Fatalf("path restore should not change config.yml: %q", got)
	}
	if got := readFile(t, repoPath, "reports/summary.txt"); got != "bad run summary\n" {
		t.Fatalf("path restore should not change summary: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{badRun, goodRaw})
	status := jvsJSONData(t, repoPath, "status")
	requirePublicPathSource(t, status["path_sources"], "data/raw/orders.csv", goodRaw)
}

func TestStoryJSON_DataETLStageRetryRestoresWholeFolderWithSafetyChoice(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"data/raw/orders.csv":  "id,total\n1,42\n",
		"etl/pipeline.yml":     "stage: raw\n",
		"etl/stage-status.txt": "raw complete\n",
	})
	raw := savePoint(t, repoPath, "raw ingestion run-77")
	createFiles(t, repoPath, map[string]string{
		"data/processed/orders.csv": "id,total,tax\n1,42,4\n",
		"etl/pipeline.yml":          "stage: processed\n",
		"etl/stage-status.txt":      "processed complete\n",
	})
	processed := savePoint(t, repoPath, "processed data run-77")
	historyBeforeRetry := []string{processed, raw}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	createFiles(t, repoPath, map[string]string{
		"data/processed/orders.csv": "id,total,tax\n1,0,0\n",
		"features/customers.csv":    "id,feature\n1,partial\n",
		"etl/stage-status.txt":      "features failed after partial write\n",
	})

	preview := jvsJSONData(t, repoPath, "restore", processed, "--discard-unsaved")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != processed || planID == "" {
		t.Fatalf("ETL stage retry restore preview mismatch: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("ETL stage retry preview should not mutate files or history: %#v", preview)
	}
	requireDiscardUnsavedOption(t, preview["options"])
	if got := readFile(t, repoPath, "etl/stage-status.txt"); got != "features failed after partial write\n" {
		t.Fatalf("ETL preview changed failed stage status: %q", got)
	}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_save_point"] != processed || restored["history_changed"] != false {
		t.Fatalf("ETL stage retry restore run mismatch: %#v", restored)
	}
	if restored["content_source"] != processed {
		t.Fatalf("ETL restore run content_source = %#v, want %s", restored["content_source"], processed)
	}
	if got := readFile(t, repoPath, "data/processed/orders.csv"); got != "id,total,tax\n1,42,4\n" {
		t.Fatalf("ETL restore should recover processed data: %q", got)
	}
	if got := readFile(t, repoPath, "etl/stage-status.txt"); got != "processed complete\n" {
		t.Fatalf("ETL restore should recover stage status: %q", got)
	}
	requirePathMissing(t, repoPath, "features/customers.csv")

	status := jvsJSONData(t, repoPath, "status")
	if status["newest_save_point"] != processed || status["history_head"] != processed || status["content_source"] != processed {
		t.Fatalf("ETL status after restore should point at processed stage: %#v", status)
	}
	if status["unsaved_changes"] != false || status["files_state"] != "matches_save_point" {
		t.Fatalf("ETL status after restore should be clean: %#v", status)
	}
	requireHistoryIDs(t, repoPath, historyBeforeRetry)

	createFiles(t, repoPath, map[string]string{
		"features/customers.csv": "id,feature\n1,recomputed\n",
		"etl/stage-status.txt":   "features retry complete\n",
	})
	retry := savePoint(t, repoPath, "features retry run-77")
	requireDifferentSavePoints(t, processed, retry)
	requireHistoryIDs(t, repoPath, []string{retry, processed, raw})
	requireHistoryGrepIDs(t, repoPath, "run-77", []string{retry, processed, raw})

	retryView := jvsJSON(t, repoPath, "view", retry, "features/customers.csv")
	retryViewPath, _ := decodeContractDataMap(t, retryView)["view_path"].(string)
	if got := readAbsoluteFile(t, retryViewPath); got != "id,feature\n1,recomputed\n" {
		t.Fatalf("saved ETL retry output = %q", got)
	}
	closeView(t, repoPath, retryView)

	statusAfterRetry := jvsJSONData(t, repoPath, "status")
	if statusAfterRetry["newest_save_point"] != retry || statusAfterRetry["history_head"] != retry || statusAfterRetry["content_source"] != retry {
		t.Fatalf("ETL status after retry save should point at retry result: %#v", statusAfterRetry)
	}
	if statusAfterRetry["unsaved_changes"] != false || statusAfterRetry["files_state"] != "matches_save_point" {
		t.Fatalf("ETL status after retry save should be clean: %#v", statusAfterRetry)
	}
}

func TestStoryJSON_MistakenDeletionRecoveryRestoresOnlyDeletedPath(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"content/chapter-03.md": "Chapter 03\n\nOpening draft.\n",
		"content/chapter-04.md": "Chapter 04\n\nNext outline.\n",
	})
	chapterDraft := savePoint(t, repoPath, "chapter 03 draft")
	createFiles(t, repoPath, map[string]string{
		"content/chapter-03.md": "Chapter 03\n\nPolished draft.\n",
		"content/chapter-04.md": "Chapter 04\n\nNext outline.\n",
	})
	latest := savePoint(t, repoPath, "chapter 03 polished")
	requireHistoryIDs(t, repoPath, []string{latest, chapterDraft})

	removePath(t, repoPath, "content/chapter-03.md")
	createFiles(t, repoPath, map[string]string{
		"content/chapter-04.md": "Chapter 04\n\nUnrelated work kept going.\n",
	})
	requirePathMissing(t, repoPath, "content/chapter-03.md")

	historyPath := jvsJSONData(t, repoPath, "history", "--path", "content/chapter-03.md")
	if historyPath["path"] != "content/chapter-03.md" {
		t.Fatalf("deletion recovery history path mismatch: %#v", historyPath)
	}
	requireCandidateSavePoint(t, historyPath["candidates"], chapterDraft)
	if _, ok := historyPath["next_commands"].([]any); !ok {
		t.Fatalf("history --path should include next commands: %#v", historyPath)
	}
	requirePathMissing(t, repoPath, "content/chapter-03.md")
	if got := readFile(t, repoPath, "content/chapter-04.md"); got != "Chapter 04\n\nUnrelated work kept going.\n" {
		t.Fatalf("history --path changed unrelated work: %q", got)
	}

	viewOut := jvsJSON(t, repoPath, "view", chapterDraft, "content/chapter-03.md")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewData["read_only"] != true {
		t.Fatalf("deletion recovery view should be read-only: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != "Chapter 03\n\nOpening draft.\n" {
		t.Fatalf("read-only view showed wrong chapter: %q", got)
	}
	closeView(t, repoPath, viewOut)

	preview := jvsJSONData(t, repoPath, "restore", chapterDraft, "--path", "content/chapter-03.md", "--discard-unsaved")
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["scope"] != "path" || preview["path"] != "content/chapter-03.md" || planID == "" {
		t.Fatalf("deletion recovery restore preview mismatch: %#v", preview)
	}
	if preview["history_changed"] != false || preview["files_changed"] != false {
		t.Fatalf("deletion recovery preview should not mutate files or history: %#v", preview)
	}
	requirePathMissing(t, repoPath, "content/chapter-03.md")
	if got := readFile(t, repoPath, "content/chapter-04.md"); got != "Chapter 04\n\nUnrelated work kept going.\n" {
		t.Fatalf("path restore preview changed unrelated work: %q", got)
	}

	restored := jvsJSONData(t, repoPath, "restore", "--run", planID)
	if restored["mode"] != "run" || restored["restored_path"] != "content/chapter-03.md" || restored["history_changed"] != false {
		t.Fatalf("deletion recovery restore run mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "content/chapter-03.md"); got != "Chapter 03\n\nOpening draft.\n" {
		t.Fatalf("restored deleted chapter = %q", got)
	}
	if got := readFile(t, repoPath, "content/chapter-04.md"); got != "Chapter 04\n\nUnrelated work kept going.\n" {
		t.Fatalf("path restore should keep unrelated work: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{latest, chapterDraft})

	status := jvsJSONData(t, repoPath, "status")
	requirePublicPathSource(t, status["path_sources"], "content/chapter-03.md", chapterDraft)

	recovered := savePoint(t, repoPath, "recover chapter 03")
	requireHistoryIDs(t, repoPath, []string{recovered, latest, chapterDraft})
}

func TestStoryJSON_AgentSandboxWorkspaceIsolation(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{
		"agent/task.md": "baseline prompt\n",
		"src/app.txt":   "main baseline\n",
	})
	base := savePoint(t, repoPath, "agent baseline")
	createFiles(t, repoPath, map[string]string{"src/app.txt": "main local work\n"})

	created := jvsJSONData(t, repoPath, "workspace", "new", "agent-a", "--from", base)
	agentPath, _ := created["folder"].(string)
	if created["workspace"] != "agent-a" || created["started_from_save_point"] != base || agentPath == "" {
		t.Fatalf("workspace new JSON mismatch: %#v", created)
	}
	if sameCleanPath(agentPath, repoPath) {
		t.Fatalf("agent workspace must be a distinct real folder, got %s", agentPath)
	}
	if info, err := os.Stat(agentPath); err != nil || !info.IsDir() {
		t.Fatalf("agent workspace folder is not a real directory: info=%v err=%v", info, err)
	}
	if got := readFile(t, repoPath, "src/app.txt"); got != "main local work\n" {
		t.Fatalf("workspace new changed source main folder: %q", got)
	}
	if got := readFile(t, agentPath, "src/app.txt"); got != "main baseline\n" {
		t.Fatalf("agent workspace did not start from save point: %q", got)
	}

	targetedStatusOut := jvsJSONFrom(t, agentPath, "--repo", repoPath, "--workspace", "agent-a", "status")
	targetedStatus := decodeContractDataMap(t, targetedStatusOut)
	if targetedStatus["workspace"] != "agent-a" || targetedStatus["folder"] != agentPath {
		t.Fatalf("new workspace folder with explicit --repo/--workspace should target itself: %#v", targetedStatus)
	}

	agentStatus := jvsJSONData(t, agentPath, "status")
	if agentStatus["workspace"] != "agent-a" || agentStatus["newest_save_point"] != nil || agentStatus["started_from_save_point"] != base {
		t.Fatalf("new agent workspace status mismatch: %#v", agentStatus)
	}
	agentHistory := jvsJSONData(t, agentPath, "history")
	if got := savePointIDsFromHistory(t, agentHistory); len(got) != 0 {
		t.Fatalf("new agent workspace should not inherit main history, got %v", got)
	}

	createFiles(t, agentPath, map[string]string{"src/app.txt": "agent experiment\n"})
	agentFirst := savePointFromCWD(t, agentPath, "agent first run")
	requireHistoryIDsInCWD(t, agentPath, []string{agentFirst})
	requireHistoryIDs(t, repoPath, []string{base})
	if got := readFile(t, repoPath, "src/app.txt"); got != "main local work\n" {
		t.Fatalf("agent save changed source main folder: %q", got)
	}

	createFiles(t, agentPath, map[string]string{"src/app.txt": "agent experiment via explicit target\n"})
	explicitSaveOut := jvsJSONFrom(t, agentPath, "--repo", repoPath, "--workspace", "agent-a", "save", "-m", "agent explicit target")
	explicitSave := decodeContractDataMap(t, explicitSaveOut)
	agentSecond, _ := explicitSave["save_point_id"].(string)
	if explicitSave["workspace"] != "agent-a" || agentSecond == "" {
		t.Fatalf("explicitly targeted save from workspace folder mismatch: %#v", explicitSave)
	}
	requireHistoryIDsInCWD(t, agentPath, []string{agentSecond, agentFirst})
	requireHistoryIDs(t, repoPath, []string{base})
}

func TestStoryJSON_TargetingMainFromNonTargetCWD(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "main baseline\n"})
	mainBase := savePoint(t, repoPath, "main baseline")
	created := jvsJSONData(t, repoPath, "workspace", "new", "agent-b", "--from", mainBase)
	agentPath, _ := created["folder"].(string)
	createFiles(t, agentPath, map[string]string{"README.md": "agent work\n"})

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

	agentSaveOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "--workspace", "agent-b", "save", "-m", "agent run")
	agentSave := decodeContractDataMap(t, agentSaveOut)
	agentRun, _ := agentSave["save_point_id"].(string)
	if agentSave["workspace"] != "agent-b" || agentSave["started_from_save_point"] != mainBase || agentRun == "" {
		t.Fatalf("--repo --workspace agent save mismatch: %#v", agentSave)
	}
	if got := readFile(t, repoPath, "README.md"); got != "main baseline\n" {
		t.Fatalf("targeted agent save changed main folder: %q", got)
	}
	requireHistoryIDs(t, repoPath, []string{mainBase})

	agentHistoryOut := jvsJSONFrom(t, otherCWD, "--repo", repoPath, "--workspace", "agent-b", "history")
	agentHistory := decodeContractDataMap(t, agentHistoryOut)
	if got := savePointIDsFromHistory(t, agentHistory); !sameStringSlice(got, []string{agentRun}) {
		t.Fatalf("--repo --workspace agent history IDs = %v, want [%s]", got, agentRun)
	}
}
