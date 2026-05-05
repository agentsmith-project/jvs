//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStoryE2EGate_CoversRegularUserStories(t *testing.T) {
	cmd := exec.Command("make", "-n", "story-e2e")
	cmd.Dir = conformanceRepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n story-e2e failed: %v\n%s", err, string(out))
	}

	patterns := storyE2EDryRunPatterns(t, string(out))
	tests := listConformanceStoryTests(t)
	var missing []string
	for _, name := range tests {
		if !storyE2EMatchesAny(patterns, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("make story-e2e does not cover regular user story tests: %s\nmake -n output:\n%s", strings.Join(missing, ", "), string(out))
	}
}

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

func TestStory_PublicTransferJSONDegradedSaveKeepsPublicWarningsClean(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	if err := os.WriteFile(filepath.Join(repoPath, "app.txt"), []byte("v1\n"), 0644); err != nil {
		t.Fatalf("write app: %v", err)
	}

	t.Setenv("JVS_SNAPSHOT_ENGINE", "juicefs-clone")
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "save", "-m", "baseline with fallback")
	if code != 0 {
		t.Fatalf("degraded save should still succeed by copying normally\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, true)
	requireStoryPublicJSONTextClean(t, stdout)
	saveData := decodeContractDataMap(t, stdout)

	savePointID, _ := saveData["save_point_id"].(string)
	if savePointID == "" {
		t.Fatalf("save output missing save_point_id: %#v", saveData)
	}
	if got := readAbsoluteFile(t, filepath.Join(repoPath, "app.txt")); got != "v1\n" {
		t.Fatalf("workspace content changed during fallback save: %q", got)
	}

	requireStoryPublicTransfersClean(t, saveData)
	saveTransfer := requireStoryPublicTransferByID(t, saveData, "save-primary")
	requireStoryTransferString(t, saveTransfer, "source_role", "workspace_content")
	requireStoryTransferString(t, saveTransfer, "source_path", repoPath)
	requireStoryTransferString(t, saveTransfer, "destination_role", "save_point_content")
	requireStoryTransferString(t, saveTransfer, "materialization_destination", "temporary_folder")
	requireStoryTransferString(t, saveTransfer, "published_destination", "save_point:"+savePointID)
	requireStoryTransferString(t, saveTransfer, "requested_engine", "juicefs-clone")
	requireStoryTransferString(t, saveTransfer, "effective_engine", "copy")
	requireStoryTransferString(t, saveTransfer, "performance_class", "normal_copy")
	if optimized, _ := saveTransfer["optimized_transfer"].(bool); optimized {
		t.Fatalf("fallback save should report a normal copy transfer: %#v", saveTransfer)
	}

	degradedReasons := requireStoryTransferStringList(t, saveTransfer, "degraded_reasons")
	warnings := requireStoryTransferStringList(t, saveTransfer, "warnings")
	if len(degradedReasons) == 0 || len(warnings) == 0 {
		t.Fatalf("fallback save should explain the degraded transfer: %#v", saveTransfer)
	}
	requireStoryTransferListContains(t, degradedReasons, "juicefs-clone unavailable at destination")
	requireStoryTransferListContains(t, warnings, "juicefs command not found")
}

func storyE2EDryRunPatterns(t *testing.T, dryRun string) []*regexp.Regexp {
	t.Helper()
	matches := regexp.MustCompile(`(?:^|\s)-run(?:=|\s+)['"]?([^'"\s]+)['"]?`).FindAllStringSubmatch(dryRun, -1)
	if len(matches) == 0 {
		t.Fatalf("make story-e2e dry-run should include at least one go test -run pattern:\n%s", dryRun)
	}
	patterns := make([]*regexp.Regexp, 0, len(matches))
	for _, match := range matches {
		pattern, err := regexp.Compile(match[1])
		if err != nil {
			t.Fatalf("story-e2e -run pattern %q should compile: %v", match[1], err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns
}

func listConformanceStoryTests(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("go", "test", "-tags", "conformance", "-list", "^TestStory", "./test/conformance/...")
	cmd.Dir = conformanceRepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list conformance story tests failed: %v\n%s", err, string(out))
	}

	var tests []string
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "TestStory") {
			tests = append(tests, name)
		}
	}
	if len(tests) == 0 {
		t.Fatalf("no conformance story tests listed:\n%s", string(out))
	}
	return tests
}

func storyE2EMatchesAny(patterns []*regexp.Regexp, name string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
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

func requireStoryPublicJSONTextClean(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{".jvs", "payload", "snapshot", "stderr:", "stdout:"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("public JSON output exposes %q:\n%s", forbidden, output)
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

func requireStoryTransferStringList(t *testing.T, record map[string]any, key string) []string {
	t.Helper()
	raw, ok := record[key].([]any)
	if !ok {
		t.Fatalf("transfer %s should be an array: %#v in %#v", key, record[key], record)
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("transfer %s should contain strings: %#v in %#v", key, raw, record)
		}
		values = append(values, value)
	}
	return values
}

func requireStoryTransferListContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("transfer list %v missing %q", values, want)
}
