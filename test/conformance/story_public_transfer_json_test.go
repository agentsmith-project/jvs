//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStory_PublicTransferJSONUsesContentVocabulary(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	if err := os.WriteFile(filepath.Join(repoPath, "app.txt"), []byte("v1\n"), 0644); err != nil {
		t.Fatalf("write app: %v", err)
	}
	saveOut := jvsJSON(t, repoPath, "save", "-m", "baseline")
	saveData := decodeContractDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	if savePointID == "" {
		t.Fatalf("save output missing save_point_id: %#v", saveData)
	}
	requireStoryPublicTransfersClean(t, saveData)
	saveTransfer := requireStoryPublicTransferByID(t, saveData, "save-primary")
	requireStoryTransferString(t, saveTransfer, "source_role", "workspace_content")
	requireStoryTransferString(t, saveTransfer, "source_path", repoPath)
	requireStoryTransferString(t, saveTransfer, "destination_role", "save_point_content")
	requireStoryTransferString(t, saveTransfer, "published_destination", "save_point:"+savePointID)

	viewOut := jvsJSON(t, repoPath, "view", savePointID, "app.txt")
	viewData := decodeContractDataMap(t, viewOut)
	viewPath, _ := viewData["view_path"].(string)
	if viewPath == "" {
		t.Fatalf("view output missing view_path: %#v", viewData)
	}
	if strings.Contains(filepath.ToSlash(viewPath), "/payload") {
		t.Fatalf("view_path exposes old payload vocabulary: %#v", viewData)
	}
	if got := readAbsoluteFile(t, viewPath); got != "v1\n" {
		t.Fatalf("view content = %q", got)
	}
	requireStoryPublicTransfersClean(t, viewData)
	viewID, _ := viewData["view_id"].(string)
	viewTransfer := requireStoryPublicTransferByID(t, viewData, "view-primary")
	requireStoryTransferString(t, viewTransfer, "source_role", "save_point_content")
	requireStoryTransferString(t, viewTransfer, "source_path", "save_point:"+savePointID)
	requireStoryTransferString(t, viewTransfer, "destination_role", "content_view")
	requireStoryTransferString(t, viewTransfer, "published_destination", "content_view:"+viewID+"/app.txt")
	closeView(t, repoPath, viewOut)

	restoreOut := jvsJSON(t, repoPath, "restore", savePointID)
	restoreData := decodeContractDataMap(t, restoreOut)
	requireStoryPublicTransfersClean(t, restoreData)
	restoreTransfer := requireStoryPublicTransferByID(t, restoreData, "restore-preview-validation-primary")
	requireStoryTransferString(t, restoreTransfer, "source_role", "save_point_content")
	requireStoryTransferString(t, restoreTransfer, "source_path", "save_point:"+savePointID)
	requireStoryTransferString(t, restoreTransfer, "destination_role", "temporary_folder")
	requireStoryTransferString(t, restoreTransfer, "materialization_destination", "temporary_folder")
	requireStoryTransferString(t, restoreTransfer, "published_destination", repoPath)
}

func requireStoryPublicTransfersClean(t *testing.T, data map[string]any) {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	if !ok || len(transfers) == 0 {
		t.Fatalf("data.transfers should be a non-empty array: %#v", data["transfers"])
	}
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("transfer should be an object: %#v", item)
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal transfer: %v", err)
		}
		for _, forbidden := range []string{"payload", ".jvs/snapshots", "save_point_payload", "save_point_staging"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("public transfer exposes %q: %#v", forbidden, record)
			}
		}
	}
}

func requireStoryPublicTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	if !ok {
		t.Fatalf("data.transfers should be an array: %#v", data["transfers"])
	}
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("transfer should be an object: %#v", item)
		}
		if record["transfer_id"] == id {
			return record
		}
	}
	t.Fatalf("missing transfer %q in %#v", id, transfers)
	return nil
}

func requireStoryTransferString(t *testing.T, record map[string]any, key, want string) {
	t.Helper()
	got, _ := record[key].(string)
	if got != want {
		t.Fatalf("transfer %s = %#v, want %q in %#v", key, record[key], want, record)
	}
}
