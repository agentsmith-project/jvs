//go:build conformance

package conformance

import (
	"encoding/json"
	"path/filepath"
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

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "new", "../feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}
	created := decodeContractDataMap(t, stdout)
	featureFolder := filepath.Join(filepath.Dir(repoPath), "feature")
	if created["workspace"] != "feature" || created["folder"] != featureFolder || created["started_from_save_point"] != base {
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
	renamedPathOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "renamed-feature")
	if code != 0 {
		t.Fatalf("workspace path after rename failed: stdout=%s stderr=%s", renamedPathOut, stderr)
	}
	featurePath, _ = decodeContractDataMap(t, renamedPathOut)["path"].(string)
	if featurePath == "" {
		t.Fatalf("workspace path after rename missing path: %s", renamedPathOut)
	}

	removeOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "renamed-feature", "--force")
	if code != 0 {
		t.Fatalf("workspace remove preview failed: stdout=%s stderr=%s", removeOut, stderr)
	}
	removePreview := decodeContractDataMap(t, removeOut)
	removePlanID, _ := removePreview["plan_id"].(string)
	if removePreview["mode"] != "preview" ||
		removePlanID == "" ||
		removePreview["folder"] != featurePath ||
		removePreview["folder_removed"] != false ||
		removePreview["files_changed"] != false ||
		removePreview["run_command"] != "jvs workspace remove --run "+removePlanID ||
		removePreview["save_point_storage_removed"] != false {
		t.Fatalf("workspace remove preview data mismatch: %#v", removePreview)
	}

	removeRunOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "remove", "--run", removePlanID)
	if code != 0 {
		t.Fatalf("workspace remove run failed: stdout=%s stderr=%s", removeRunOut, stderr)
	}
	removeRun := decodeContractDataMap(t, removeRunOut)
	if removeRun["status"] != "removed" ||
		removeRun["folder_removed"] != true ||
		removeRun["workspace_metadata_removed"] != true ||
		removeRun["save_point_storage_removed"] != false {
		t.Fatalf("workspace remove run data mismatch: %#v", removeRun)
	}
	removedPathOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "path", "renamed-feature")
	if code == 0 {
		t.Fatalf("workspace remove run left workspace metadata: stdout=%s stderr=%s", removedPathOut, stderr)
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
	for _, field := range publicCleanupPlanJSONFields() {
		if _, ok := preview[field]; !ok {
			t.Fatalf("cleanup preview missing public field %q: %#v", field, preview)
		}
	}
	requireCleanupProtectionGroups(t, preview, "history")
	if _, ok := preview["protected_by_history"].(float64); !ok {
		t.Fatalf("cleanup preview protected_by_history must be numeric: %#v", preview["protected_by_history"])
	}
	if _, ok := preview["candidate_count"].(float64); !ok {
		t.Fatalf("cleanup preview candidate_count must be numeric: %#v", preview["candidate_count"])
	}

	runOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "cleanup", "run", "--plan-id", planID)
	if code != 0 {
		t.Fatalf("cleanup run failed: stdout=%s stderr=%s", runOut, stderr)
	}
	if data := decodeContractDataMap(t, runOut); data["status"] != "completed" || data["plan_id"] != planID {
		t.Fatalf("cleanup run data mismatch: %#v", data)
	}
}

func TestContract_CleanupPreviewUsesStableProtectionReasonTokens(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"notes.txt": "base\n"})
	base := savePoint(t, repoPath, "base")

	viewOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "view", base, "notes.txt")
	if code != 0 {
		t.Fatalf("view failed: stdout=%s stderr=%s", viewOut, stderr)
	}
	defer closeView(t, repoPath, viewOut)

	previewOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "cleanup", "preview")
	if code != 0 {
		t.Fatalf("cleanup preview failed: stdout=%s stderr=%s", previewOut, stderr)
	}
	preview := decodeContractDataMap(t, previewOut)
	requireCleanupProtectionGroups(t, preview, "history", "open_view")
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
	historyTagOut, historyTagErr, historyTagCode := runJVS(t, conformanceRepoRoot, "history", "--tag", "v1")
	if historyTagCode == 0 || !strings.Contains(historyTagErr+historyTagOut, "unknown flag: --tag") {
		t.Fatalf("history --tag must be unavailable public surface: stdout=%s stderr=%s code=%d", historyTagOut, historyTagErr, historyTagCode)
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

func TestHistoryHelpDoesNotAdvertiseTag(t *testing.T) {
	historyHelp, stderr, code := runJVS(t, conformanceRepoRoot, "history", "--help")
	if code != 0 {
		t.Fatalf("history help failed: stdout=%s stderr=%s", historyHelp, stderr)
	}
	for _, forbidden := range []string{"--tag", "history --tag"} {
		if strings.Contains(historyHelp, forbidden) {
			t.Fatalf("history help advertises non-GA tag surface %q:\n%s", forbidden, historyHelp)
		}
	}
}

func TestContract_ReleaseFacingDocsUsePublicRuntimeBoundaryVocabulary(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				assertNoPathShapedRuntimeMechanismLine(t, doc, lineNo, line)
				lowerLine := strings.ToLower(line)
				for _, internalTerm := range runtimeInternalFileVocabularyFragments() {
					if strings.Contains(lowerLine, internalTerm) {
						t.Fatalf("%s:%d leaks internal runtime/storage term %q; use JVS control data, runtime state, or active operations instead:\n%s", doc, lineNo, internalTerm, line)
					}
				}
			})
		})
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

func requireCleanupProtectionGroups(t *testing.T, preview map[string]any, requiredReasons ...string) {
	t.Helper()

	groups, ok := preview["protection_groups"].([]any)
	if !ok {
		t.Fatalf("cleanup preview protection_groups must be an array: %#v", preview["protection_groups"])
	}
	stableReasons := stableCleanupProtectionReasonSet()
	seen := map[string]bool{}
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			t.Fatalf("cleanup protection group must be an object: %#v", rawGroup)
		}
		reason, ok := group["reason"].(string)
		if !ok || reason == "" {
			t.Fatalf("cleanup protection group missing reason: %#v", group)
		}
		if !stableReasons[reason] {
			t.Fatalf("cleanup protection group reason %q is outside stable token set %v", reason, stableCleanupProtectionReasonTokens())
		}
		if _, ok := group["count"].(float64); !ok {
			t.Fatalf("cleanup protection group %q missing numeric count: %#v", reason, group)
		}
		if _, ok := group["save_points"].([]any); !ok {
			t.Fatalf("cleanup protection group %q missing save_points array: %#v", reason, group)
		}
		seen[reason] = true
	}
	for _, reason := range requiredReasons {
		if !seen[reason] {
			t.Fatalf("cleanup preview missing protection group reason %q: %#v", reason, groups)
		}
	}
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
