//go:build conformance

package conformance

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoryRepoCloneEmbeddedProjectKeepsIdentityHistoryAndMainWorkspaceUsable(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source-project")
	stdout, stderr, code := runJVS(t, base, "init", source)
	if code != 0 {
		t.Fatalf("init source repo failed: stdout=%s stderr=%s", stdout, stderr)
	}

	createFiles(t, source, map[string]string{
		"README.md":   "source project\n",
		"src/app.txt": "main v1\n",
	})
	mainV1 := savePoint(t, source, "main baseline")

	createFiles(t, source, map[string]string{"src/app.txt": "main v2\n"})
	mainV2 := savePoint(t, source, "main update")

	reviewCreated := jvsJSONData(t, source, "workspace", "new", "../review-workspace", "--from", mainV1)
	reviewPath, _ := reviewCreated["folder"].(string)
	if reviewCreated["workspace"] != "review-workspace" || reviewPath == "" {
		t.Fatalf("review workspace setup mismatch: %#v", reviewCreated)
	}
	createFiles(t, reviewPath, map[string]string{
		"review/notes.txt": "review notes\n",
		"src/app.txt":      "reviewed main v1\n",
	})
	reviewSave := savePointFromCWD(t, reviewPath, "review notes")

	sourceRepoID := strings.TrimSpace(readAbsoluteFile(t, filepath.Join(source, ".jvs", "repo_id")))
	requireHistoryIDs(t, source, []string{mainV2, mainV1})
	requireHistoryIDsInCWD(t, reviewPath, []string{reviewSave})

	for _, tc := range []struct {
		name                 string
		mode                 string
		wantCopiedCount      float64
		wantCopiedSavePoints []string
		reviewSavePointView  bool
		wantMainHistoryAfter []string
	}{
		{
			name:                 "all save points",
			mode:                 "all",
			wantCopiedCount:      3,
			wantCopiedSavePoints: []string{mainV1, mainV2, reviewSave},
			reviewSavePointView:  true,
			wantMainHistoryAfter: []string{mainV2, mainV1},
		},
		{
			name:                 "main save points",
			mode:                 "main",
			wantCopiedCount:      2,
			wantCopiedSavePoints: []string{mainV1, mainV2},
			reviewSavePointView:  false,
			wantMainHistoryAfter: []string{mainV2, mainV1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(base, "target-"+strings.ReplaceAll(tc.mode, " ", "-"))
			stdout, stderr, code := runJVS(t, base,
				"--json",
				"--repo", source,
				"repo", "clone",
				target,
				"--save-points", tc.mode,
			)
			if code != 0 {
				t.Fatalf("repo clone --save-points %s failed: stdout=%s stderr=%s", tc.mode, stdout, stderr)
			}
			env := requirePureJSONEnvelope(t, stdout, stderr, true)
			if env.RepoRoot == nil || *env.RepoRoot != target {
				t.Fatalf("clone JSON repo_root = %#v, want target %q\n%s", env.RepoRoot, target, stdout)
			}
			if env.Workspace == nil || *env.Workspace != "main" {
				t.Fatalf("clone JSON workspace = %#v, want main\n%s", env.Workspace, stdout)
			}

			clone := decodeContractDataMap(t, stdout)
			sourceRepoIDFromJSON, _ := clone["source_repo_id"].(string)
			if sourceRepoIDFromJSON != sourceRepoID {
				t.Fatalf("clone source_repo_id = %q, want source disk repo_id %q in %#v", sourceRepoIDFromJSON, sourceRepoID, clone)
			}
			targetRepoID, _ := clone["target_repo_id"].(string)
			if targetRepoID == "" || targetRepoID == sourceRepoID {
				t.Fatalf("clone target_repo_id should be a new repo identity: source=%q clone=%#v", sourceRepoID, clone)
			}
			if targetDiskRepoID := strings.TrimSpace(readAbsoluteFile(t, filepath.Join(target, ".jvs", "repo_id"))); targetDiskRepoID != targetRepoID {
				t.Fatalf("target repo id was not written as clone identity: %#v", clone)
			}
			if clone["source_repo_root"] != source || clone["target_repo_root"] != target || clone["save_points_mode"] != tc.mode {
				t.Fatalf("clone JSON should describe the user's source, target, and save point choice: %#v", clone)
			}
			if clone["newest_save_point"] != mainV2 || clone["save_points_copied_count"] != tc.wantCopiedCount {
				t.Fatalf("clone copied wrong save point set: %#v", clone)
			}
			requireRepoCloneSameStringSet(t, jsonStringArray(t, clone["save_points_copied"]), tc.wantCopiedSavePoints)
			if got := jsonStringArray(t, clone["workspaces_created"]); !sameStringSlice(got, []string{"main"}) {
				t.Fatalf("clone should only create target main workspace, got %v in %#v", got, clone)
			}
			requireJSONStringArrayContains(t, clone["source_workspaces_not_created"], "review-workspace")
			requireRepoClonePublicTransfers(t, clone, source, target)

			if got := readFile(t, target, "src/app.txt"); got != "main v2\n" {
				t.Fatalf("target main workspace content = %q", got)
			}
			workspaceList := jvsJSONValue(t, target, "workspace", "list")
			records, ok := workspaceList.([]any)
			if !ok || len(records) != 1 {
				t.Fatalf("target workspace list should only contain main: %#v", workspaceList)
			}
			mainRecord := workspaceListRecord(t, workspaceList, "main")
			if mainRecord["folder"] != target || mainRecord["newest_save_point"] != mainV2 {
				t.Fatalf("target main workspace should be naturally usable: %#v", mainRecord)
			}
			stdout, stderr, code = runJVSInRepo(t, target, "--json", "workspace", "path", "review-workspace")
			if code == 0 {
				t.Fatalf("clone should not create source review workspace in target: stdout=%s stderr=%s", stdout, stderr)
			}
			requirePureJSONEnvelope(t, stdout, stderr, false)

			status := jvsJSONData(t, target, "status")
			if status["workspace"] != "main" || status["folder"] != target || status["unsaved_changes"] != false {
				t.Fatalf("target status mismatch: %#v", status)
			}
			history := jvsJSONData(t, target, "history")
			if got := savePointIDsFromHistory(t, history); !sameStringSlice(got, tc.wantMainHistoryAfter) {
				t.Fatalf("target main history IDs = %v, want %v", got, tc.wantMainHistoryAfter)
			}

			view := jvsJSONData(t, target, "view", mainV1, "src/app.txt")
			viewPath, _ := view["view_path"].(string)
			if got := readAbsoluteFile(t, viewPath); got != "main v1\n" {
				t.Fatalf("target view of main baseline = %q", got)
			}
			jvsJSONData(t, target, "view", "close", view["view_id"].(string))

			stdout, stderr, code = runJVSInRepo(t, target, "--json", "view", reviewSave, "review/notes.txt")
			if tc.reviewSavePointView {
				if code != 0 {
					t.Fatalf("all clone should keep review save point viewable: stdout=%s stderr=%s", stdout, stderr)
				}
				viewData := decodeContractDataMap(t, stdout)
				viewPath, _ := viewData["view_path"].(string)
				if got := readAbsoluteFile(t, viewPath); got != "review notes\n" {
					t.Fatalf("all clone review save point content = %q", got)
				}
				jvsJSONData(t, target, "view", "close", viewData["view_id"].(string))
			} else {
				requirePureJSONEnvelope(t, stdout, stderr, false)
			}

			restorePreview := jvsJSONData(t, target, "restore", mainV1)
			planID, _ := restorePreview["plan_id"].(string)
			if restorePreview["mode"] != "preview" || planID == "" {
				t.Fatalf("target restore should produce a runnable preview: %#v", restorePreview)
			}
			restored := jvsJSONData(t, target, "restore", "--run", planID)
			if restored["mode"] != "run" || restored["restored_save_point"] != mainV1 {
				t.Fatalf("target restore run mismatch: %#v", restored)
			}
			if got := readFile(t, target, "src/app.txt"); got != "main v1\n" {
				t.Fatalf("target restore content = %q", got)
			}

			createFiles(t, target, map[string]string{"src/app.txt": "target work after clone\n"})
			targetSave := jvsJSONData(t, target, "save", "-m", "target follow-up")
			targetSaveID, _ := targetSave["save_point_id"].(string)
			if targetSaveID == "" || targetSaveID == mainV1 || targetSaveID == mainV2 || targetSaveID == reviewSave {
				t.Fatalf("target save should create an independent save point: %#v", targetSave)
			}

			if got := strings.TrimSpace(readAbsoluteFile(t, filepath.Join(source, ".jvs", "repo_id"))); got != sourceRepoID {
				t.Fatalf("source repo identity changed after clone/save: got %q want %q", got, sourceRepoID)
			}
			requireHistoryIDs(t, source, []string{mainV2, mainV1})
			requireHistoryIDsInCWD(t, reviewPath, []string{reviewSave})
			if got := readFile(t, source, "src/app.txt"); got != "main v2\n" {
				t.Fatalf("source main workspace changed after target operations: %q", got)
			}
			if got := readFile(t, reviewPath, "review/notes.txt"); got != "review notes\n" {
				t.Fatalf("source review workspace changed after target operations: %q", got)
			}
		})
	}
}

func requireRepoClonePublicTransfers(t *testing.T, data map[string]any, source, target string) {
	t.Helper()
	save := requireStoryPublicTransferByID(t, data, "repo-clone-save-points")
	requireStoryTransferString(t, save, "source_role", "save_point_storage")
	requireStoryTransferString(t, save, "source_path", "control_data")
	requireStoryTransferString(t, save, "destination_role", "target_save_point_storage")
	requireStoryTransferString(t, save, "materialization_destination", "temporary_folder")
	requireStoryTransferString(t, save, "published_destination", "control_data")
	requireStoryTransferString(t, save, "result_kind", "final")
	requireStoryTransferString(t, save, "permission_scope", "execution")

	main := requireStoryPublicTransferByID(t, data, "repo-clone-main-workspace")
	requireStoryTransferString(t, main, "source_role", "source_main_current_state")
	requireStoryTransferString(t, main, "source_path", source)
	requireStoryTransferString(t, main, "destination_role", "target_main_workspace")
	requireStoryTransferString(t, main, "materialization_destination", "temporary_folder")
	requireStoryTransferString(t, main, "published_destination", target)
	requireStoryTransferString(t, main, "capability_probe_path", filepath.Dir(target))
	requireStoryTransferString(t, main, "result_kind", "final")
	requireStoryTransferString(t, main, "permission_scope", "execution")

	transfers, ok := data["transfers"].([]any)
	if !ok {
		t.Fatalf("clone transfers should be an array: %#v", data["transfers"])
	}
	if len(transfers) < 2 {
		t.Fatalf("clone transfers should include at least the save point and main workspace records: %#v", transfers)
	}
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("clone transfer should be an object: %#v", item)
		}
		if record["checked_for_this_operation"] != true {
			t.Fatalf("clone transfer should be checked for this operation: %#v", record)
		}
		if record["performance_class"] != "fast_copy" && record["performance_class"] != "normal_copy" {
			t.Fatalf("clone transfer performance_class = %#v, want fast_copy or normal_copy in %#v", record["performance_class"], record)
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal clone transfer: %v", err)
		}
		for _, forbidden := range []string{".jvs", ".jvs/", "/snapshots", "payload", "worktree", "snapshot"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("clone transfer leaks internal control-plane vocabulary %q: %#v", forbidden, record)
			}
		}
	}
}

func requireRepoCloneSameStringSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("string set = %v, want %v", got, want)
	}
	seen := make(map[string]int, len(got))
	for _, value := range got {
		seen[value]++
	}
	for _, value := range want {
		if seen[value] == 0 {
			t.Fatalf("string set = %v, want %v", got, want)
		}
		seen[value]--
	}
	for _, count := range seen {
		if count != 0 {
			t.Fatalf("string set = %v, want %v", got, want)
		}
	}
}
