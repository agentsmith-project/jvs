//go:build conformance

package conformance

import (
	"encoding/json"
	"strings"
	"testing"
)

type contractSmokeEnvelope struct {
	SchemaVersion int                 `json:"schema_version"`
	Command       string              `json:"command"`
	OK            bool                `json:"ok"`
	RepoRoot      *string             `json:"repo_root"`
	Workspace     *string             `json:"workspace"`
	Data          json.RawMessage     `json:"data"`
	Error         *contractSmokeError `json:"error"`
}

type contractSmokeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func TestContract_JSONStdoutPurityForCurrentPublicCommands(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "baseline\n"})
	first := savePoint(t, repoPath, "baseline")

	cases := [][]string{
		{"--json", "status"},
		{"--json", "history"},
		{"--json", "view", first},
		{"--json", "view", first, "README.md"},
		{"--json", "workspace", "list"},
		{"--json", "workspace", "path"},
		{"--json", "doctor", "--strict"},
		{"--json", "recovery", "status"},
		{"--json", "cleanup", "preview"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			stdout, stderr, code := runJVSInRepo(t, repoPath, args...)
			if code != 0 {
				t.Fatalf("command failed: stdout=%s stderr=%s", stdout, stderr)
			}
			requirePureJSONEnvelope(t, stdout, stderr, true)
			if len(args) > 1 && args[1] == "view" {
				closeView(t, repoPath, stdout)
			}
		})
	}
}

func TestContract_SaveHistoryRestoreAndViewUseSavePointVocabulary(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"app.txt": "v1\n"})
	first := savePoint(t, repoPath, "baseline")

	createFiles(t, repoPath, map[string]string{"app.txt": "v2\n"})
	second := savePoint(t, repoPath, "feature update")
	if first == second {
		t.Fatalf("two saves returned the same save point ID %q", first)
	}

	historyOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "history")
	if code != 0 {
		t.Fatalf("history failed: stdout=%s stderr=%s", historyOut, stderr)
	}
	history := decodeContractDataMap(t, historyOut)
	if history["newest_save_point"] != second {
		t.Fatalf("history newest_save_point = %#v, want %s\n%s", history["newest_save_point"], second, historyOut)
	}
	savePoints, ok := history["save_points"].([]any)
	if !ok || len(savePoints) != 2 {
		t.Fatalf("history save_points = %#v, want two entries\n%s", history["save_points"], historyOut)
	}
	assertNoLegacyPublicJSONFields(t, historyOut)

	viewOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "view", first, "app.txt")
	if code != 0 {
		t.Fatalf("view failed: stdout=%s stderr=%s", viewOut, stderr)
	}
	viewData := decodeContractDataMap(t, viewOut)
	if viewData["save_point"] != first || viewData["read_only"] != true {
		t.Fatalf("view data mismatch: %#v", viewData)
	}
	closeView(t, repoPath, viewOut)

	previewOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", first)
	if code != 0 {
		t.Fatalf("restore preview failed: stdout=%s stderr=%s", previewOut, stderr)
	}
	preview := decodeContractDataMap(t, previewOut)
	planID, _ := preview["plan_id"].(string)
	if planID == "" || preview["source_save_point"] != first || preview["mode"] != "preview" {
		t.Fatalf("restore preview data mismatch: %#v", preview)
	}

	runOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", "--run", planID)
	if code != 0 {
		t.Fatalf("restore run failed: stdout=%s stderr=%s", runOut, stderr)
	}
	restored := decodeContractDataMap(t, runOut)
	if restored["restored_save_point"] != first || restored["history_changed"] != false {
		t.Fatalf("restore run data mismatch: %#v", restored)
	}
	if got := readFile(t, repoPath, "app.txt"); got != "v1\n" {
		t.Fatalf("restore run file content = %q, want v1", got)
	}
	assertNoLegacyPublicJSONFields(t, runOut)
}

func TestContract_WorkspaceAndCleanupUseCurrentPublicCommands(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"notes.txt": "base\n"})
	base := savePoint(t, repoPath, "base")

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "new", "feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}
	created := decodeContractDataMap(t, stdout)
	if created["workspace"] != "feature" || created["started_from_save_point"] != base {
		t.Fatalf("workspace new data mismatch: %#v", created)
	}

	listOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "list")
	if code != 0 {
		t.Fatalf("workspace list failed: stdout=%s stderr=%s", listOut, stderr)
	}
	if !strings.Contains(listOut, `"workspace": "main"`) || !strings.Contains(listOut, `"workspace": "feature"`) {
		t.Fatalf("workspace list missing main or feature: %s", listOut)
	}

	pathOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "feature")
	if code != 0 {
		t.Fatalf("workspace path failed: stdout=%s stderr=%s", pathOut, stderr)
	}
	featurePath, _ := decodeContractDataMap(t, pathOut)["path"].(string)
	if featurePath == "" {
		t.Fatalf("workspace path missing path: %s", pathOut)
	}

	renameOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "rename", "feature", "renamed-feature")
	if code != 0 {
		t.Fatalf("workspace rename failed: stdout=%s stderr=%s", renameOut, stderr)
	}
	if decodeContractDataMap(t, renameOut)["workspace"] != "renamed-feature" {
		t.Fatalf("workspace rename data mismatch: %s", renameOut)
	}

	removeOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "renamed-feature", "--force")
	if code != 0 {
		t.Fatalf("workspace remove failed: stdout=%s stderr=%s", removeOut, stderr)
	}
	if decodeContractDataMap(t, removeOut)["status"] != "removed" {
		t.Fatalf("workspace remove data mismatch: %s", removeOut)
	}

	previewOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "cleanup", "preview")
	if code != 0 {
		t.Fatalf("cleanup preview failed: stdout=%s stderr=%s", previewOut, stderr)
	}
	preview := decodeContractDataMap(t, previewOut)
	planID, _ := preview["plan_id"].(string)
	if planID == "" {
		t.Fatalf("cleanup preview missing plan_id: %s", previewOut)
	}
	for _, field := range []string{"protected_save_points", "protected_by_history", "reclaimable_save_points", "reclaimable_bytes_estimate"} {
		if _, ok := preview[field]; !ok {
			t.Fatalf("cleanup preview missing public field %q: %#v", field, preview)
		}
	}

	runOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "cleanup", "run", "--plan-id", planID)
	if code != 0 {
		t.Fatalf("cleanup run failed: stdout=%s stderr=%s", runOut, stderr)
	}
	if data := decodeContractDataMap(t, runOut); data["status"] != "completed" || data["plan_id"] != planID {
		t.Fatalf("cleanup run data mismatch: %#v", data)
	}
}

func TestContract_PublicHelpOnlyAdvertisesGACommands(t *testing.T) {
	rootHelp, stderr, code := runJVS(t, conformanceRepoRoot, "--help")
	if code != 0 {
		t.Fatalf("root help failed: stdout=%s stderr=%s", rootHelp, stderr)
	}
	for _, required := range []string{"save", "history", "view", "restore", "workspace", "cleanup", "doctor", "recovery"} {
		if !strings.Contains(rootHelp, required) {
			t.Fatalf("root help missing public command %q:\n%s", required, rootHelp)
		}
	}
	for _, legacy := range []string{"checkpoint", "fork", "gc", "verify", "worktree", "snapshot"} {
		if strings.Contains(rootHelp, legacy) {
			t.Fatalf("root help exposes legacy public command %q:\n%s", legacy, rootHelp)
		}
	}

	workspaceHelp, stderr, code := runJVS(t, conformanceRepoRoot, "workspace", "--help")
	if code != 0 {
		t.Fatalf("workspace help failed: stdout=%s stderr=%s", workspaceHelp, stderr)
	}
	for _, required := range []string{"new", "list", "path", "rename", "remove"} {
		if !strings.Contains(workspaceHelp, required) {
			t.Fatalf("workspace help missing public subcommand %q:\n%s", required, workspaceHelp)
		}
	}

	cleanupHelp, stderr, code := runJVS(t, conformanceRepoRoot, "cleanup", "--help")
	if code != 0 {
		t.Fatalf("cleanup help failed: stdout=%s stderr=%s", cleanupHelp, stderr)
	}
	for _, required := range []string{"preview", "run"} {
		if !strings.Contains(cleanupHelp, required) {
			t.Fatalf("cleanup help missing public subcommand %q:\n%s", required, cleanupHelp)
		}
	}

	doctorHelp, stderr, code := runJVS(t, conformanceRepoRoot, "doctor", "--help")
	if code != 0 {
		t.Fatalf("doctor help failed: stdout=%s stderr=%s", doctorHelp, stderr)
	}
	if !strings.Contains(doctorHelp, "--strict") {
		t.Fatalf("doctor help must advertise --strict:\n%s", doctorHelp)
	}
	if !strings.Contains(strings.ToLower(doctorHelp), "save point integrity") {
		t.Fatalf("doctor help must describe strict checks as save point integrity:\n%s", doctorHelp)
	}

	recoveryHelp, stderr, code := runJVS(t, conformanceRepoRoot, "recovery", "--help")
	if code != 0 {
		t.Fatalf("recovery help failed: stdout=%s stderr=%s", recoveryHelp, stderr)
	}
	for _, required := range []string{"status", "resume", "rollback"} {
		if !strings.Contains(recoveryHelp, required) {
			t.Fatalf("recovery help missing public subcommand %q:\n%s", required, recoveryHelp)
		}
	}

	for _, surface := range []struct {
		name string
		help string
	}{
		{"root help", rootHelp},
		{"workspace help", workspaceHelp},
		{"cleanup help", cleanupHelp},
		{"doctor help", doctorHelp},
		{"recovery help", recoveryHelp},
	} {
		assertContractHelpOmitsLegacyPublicVocabulary(t, surface.name, surface.help)
	}
}

func savePoint(t *testing.T, repoPath, message string) string {
	t.Helper()
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "save", "-m", message)
	if code != 0 {
		t.Fatalf("save failed: stdout=%s stderr=%s", stdout, stderr)
	}
	data := decodeContractDataMap(t, stdout)
	id, _ := data["save_point_id"].(string)
	if id == "" {
		t.Fatalf("save output missing save_point_id: %s", stdout)
	}
	assertNoLegacyPublicJSONFields(t, stdout)
	return id
}

func closeView(t *testing.T, repoPath, viewOut string) {
	t.Helper()
	viewID, _ := decodeContractDataMap(t, viewOut)["view_id"].(string)
	if viewID == "" {
		t.Fatalf("view output missing view_id: %s", viewOut)
	}
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "view", "close", viewID)
	if code != 0 {
		t.Fatalf("view close failed: stdout=%s stderr=%s", stdout, stderr)
	}
	requirePureJSONEnvelope(t, stdout, stderr, true)
}

func requirePureJSONEnvelope(t *testing.T, stdout, stderr string, wantOK bool) contractSmokeEnvelope {
	t.Helper()
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("JSON command wrote stderr: %q", stderr)
	}
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("stdout is not pure JSON: %q", stdout)
	}
	env := decodeContractEnvelope(t, stdout)
	if env.OK != wantOK {
		t.Fatalf("JSON envelope ok = %t, want %t: %s", env.OK, wantOK, stdout)
	}
	return env
}

func decodeContractEnvelope(t *testing.T, stdout string) contractSmokeEnvelope {
	t.Helper()
	var env contractSmokeEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode JSON envelope: %v\n%s", err, stdout)
	}
	if env.SchemaVersion == 0 {
		t.Fatalf("JSON envelope missing schema_version: %s", stdout)
	}
	return env
}

func decodeContractDataMap(t *testing.T, stdout string) map[string]any {
	t.Helper()
	env := requirePureJSONEnvelope(t, stdout, "", true)
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode JSON envelope data object: %v\n%s", err, stdout)
	}
	return data
}

func assertNoLegacyPublicJSONFields(t *testing.T, stdout string) {
	t.Helper()
	for _, legacy := range []string{
		"checkpoint_id",
		"parent_checkpoint_id",
		"worktree",
		"worktree_name",
		"head_snapshot",
		"latest_snapshot",
		"from_snapshot",
		"to_snapshot",
	} {
		if strings.Contains(stdout, `"`+legacy+`"`) {
			t.Fatalf("public JSON exposes legacy field %q:\n%s", legacy, stdout)
		}
	}
}

func assertContractHelpOmitsLegacyPublicVocabulary(t *testing.T, name, help string) {
	t.Helper()
	for _, legacy := range []string{"checkpoint", "snapshot", "worktree", "fork", "gc", "info"} {
		if containsContractHelpWord(help, legacy) {
			t.Fatalf("%s exposes legacy public word %q:\n%s", name, legacy, help)
		}
	}
}

func containsContractHelpWord(body, word string) bool {
	for _, field := range strings.FieldsFunc(strings.ToLower(body), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-')
	}) {
		if field == word {
			return true
		}
	}
	return false
}
