//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type contractSmokeEnvelope struct {
	Command   string          `json:"command"`
	OK        bool            `json:"ok"`
	RepoRoot  *string         `json:"repo_root"`
	Workspace *string         `json:"workspace"`
	Data      json.RawMessage `json:"data"`
	Error     any             `json:"error"`
}

func TestContract_JSONStdoutPurity(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	cases := [][]string{
		{"--json", "info"},
		{"--json", "workspace", "list"},
		{"--json", "workspace", "path"},
		{"--json", "doctor"},
		{"--json", "gc", "plan"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, stderr, code := runJVSInRepo(t, repoPath, args...)
			if code != 0 {
				t.Fatalf("command failed: stdout=%s stderr=%s", stdout, stderr)
			}
			if !json.Valid([]byte(stdout)) {
				t.Fatalf("stdout is not pure JSON: %q", stdout)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("JSON command wrote stderr: %q", stderr)
			}
			env := decodeContractSmokeEnvelope(t, stdout)
			if !env.OK {
				t.Fatalf("JSON envelope was not ok: %s", stdout)
			}
		})
	}
}

func TestContract_GCRunJSONEnvelope(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	planOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
	if code != 0 {
		t.Fatalf("gc plan failed: stdout=%s stderr=%s", planOut, stderr)
	}

	planData := decodeContractSmokeDataMap(t, planOut)
	planID, _ := planData["plan_id"].(string)
	if planID == "" {
		t.Fatalf("gc plan did not return plan_id: %s", planOut)
	}

	runOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "run", "--plan-id", planID)
	if code != 0 {
		t.Fatalf("gc run failed: stdout=%s stderr=%s", runOut, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("gc run --json wrote stderr: %q", stderr)
	}

	env := decodeContractSmokeEnvelope(t, runOut)
	if !env.OK || env.Command != "gc run" {
		t.Fatalf("unexpected gc run envelope: %s", runOut)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode gc run data: %v\n%s", err, runOut)
	}
	if data["status"] != "completed" || data["plan_id"] != planID {
		t.Fatalf("unexpected gc run data: %#v", data)
	}
}

func TestContract_DirtyWorkspaceRemoveRejectedByDefault(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("clean"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "base"); code != 0 {
		t.Fatalf("checkpoint failed: %s", stderr)
	}
	if _, stderr, code := runJVSInRepo(t, repoPath, "fork", "feature"); code != 0 {
		t.Fatalf("fork failed: %s", stderr)
	}

	featureFile := filepath.Join(repoPath, "worktrees", "feature", "file.txt")
	if err := os.WriteFile(featureFile, []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "remove", "feature")
	if code == 0 {
		t.Fatalf("dirty workspace remove unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "dirty") {
		t.Fatalf("dirty remove error did not mention dirty state: stdout=%s stderr=%s", stdout, stderr)
	}
	if _, err := os.Stat(featureFile); err != nil {
		t.Fatalf("dirty workspace file was removed despite rejection: %v", err)
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "worktree", "remove", "feature")
	if code == 0 {
		t.Fatalf("legacy dirty workspace remove unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "dirty") {
		t.Fatalf("legacy dirty remove error did not mention dirty state: stdout=%s stderr=%s", stdout, stderr)
	}
	if _, err := os.Stat(featureFile); err != nil {
		t.Fatalf("legacy dirty workspace file was removed despite rejection: %v", err)
	}
}

func TestContract_PublicHelpHidesInternalCommandsAndUnpublishedFlags(t *testing.T) {
	stdout, stderr, code := runJVS(t, t.TempDir(), "--help")
	if code != 0 {
		t.Fatalf("root help failed: stdout=%s stderr=%s", stdout, stderr)
	}
	for _, hidden := range []string{"config", "conformance", "snapshot", "worktree"} {
		if strings.Contains(stdout, hidden) {
			t.Fatalf("root help leaked %q:\n%s", hidden, stdout)
		}
	}

	stdout, stderr, code = runJVS(t, t.TempDir(), "checkpoint", "--help")
	if code != 0 {
		t.Fatalf("checkpoint help failed: stdout=%s stderr=%s", stdout, stderr)
	}
	for _, hiddenFlag := range []string{"--paths", "--compress"} {
		if strings.Contains(stdout, hiddenFlag) {
			t.Fatalf("checkpoint help leaked %q:\n%s", hiddenFlag, stdout)
		}
	}

	for _, args := range [][]string{
		{"init", "--help"},
		{"doctor", "--help"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, stderr, code := runJVS(t, t.TempDir(), args...)
			if code != 0 {
				t.Fatalf("help command failed: stdout=%s stderr=%s", stdout, stderr)
			}
			for _, legacy := range []string{"worktree", "snapshot"} {
				if strings.Contains(stdout, legacy) {
					t.Fatalf("public help leaked %q in %s:\n%s", legacy, strings.Join(args, " "), stdout)
				}
			}
		})
	}
}

func TestContract_CloneCurrentUsesWorkspaceVocabulary(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	dest := filepath.Join(t.TempDir(), "current-copy")
	stdout, stderr, code := runJVS(t, t.TempDir(), "--json", "clone", repoPath, dest, "--scope", "current")
	if code != 0 {
		t.Fatalf("clone current failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.Contains(stdout, "source_worktree") {
		t.Fatalf("clone current JSON leaked source_worktree:\n%s", stdout)
	}
	data := decodeContractSmokeDataMap(t, stdout)
	provenance, ok := data["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("clone current missing provenance object: %s", stdout)
	}
	if provenance["source_workspace"] != "main" {
		t.Fatalf("clone current missing source_workspace: %#v", provenance)
	}

	humanDest := filepath.Join(t.TempDir(), "current-human-copy")
	stdout, stderr, code = runJVS(t, t.TempDir(), "clone", repoPath, humanDest, "--scope", "current")
	if code != 0 {
		t.Fatalf("clone current human failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "Source workspace:") || strings.Contains(stdout, "Source worktree:") {
		t.Fatalf("clone current human leaked old vocabulary:\n%s", stdout)
	}
}

func TestContract_JSONArgValidationReportsCommand(t *testing.T) {
	stdout, stderr, code := runJVS(t, t.TempDir(), "--json", "diff")
	if code == 0 {
		t.Fatalf("diff without args unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("JSON arg validation wrote stderr: %q", stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if env.OK || env.Command != "diff" {
		t.Fatalf("unexpected JSON error envelope: %s", stdout)
	}
}

func TestContract_PublicJSONVocabularyUsesCheckpointsAndWorkspaces(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	firstOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "first")
	if code != 0 {
		t.Fatalf("first checkpoint failed: stdout=%s stderr=%s", firstOut, stderr)
	}
	first, ok := decodeContractSmokeDataMap(t, firstOut)["checkpoint_id"].(string)
	if !ok || first == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", firstOut)
	}

	if err := os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	secondOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "second")
	if code != 0 {
		t.Fatalf("second checkpoint failed: stdout=%s stderr=%s", secondOut, stderr)
	}
	second, ok := decodeContractSmokeDataMap(t, secondOut)["checkpoint_id"].(string)
	if !ok || second == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", secondOut)
	}

	cases := [][]string{
		{"--json", "checkpoint", "list"},
		{"--json", "status"},
		{"--json", "workspace", "list"},
		{"--json", "fork", "contract-branch"},
		{"--json", "diff", first, second},
		{"--json", "verify", second},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, stderr, code := runJVSInRepo(t, repoPath, args...)
			if code != 0 {
				t.Fatalf("command failed: stdout=%s stderr=%s", stdout, stderr)
			}
			env := decodeContractSmokeEnvelope(t, stdout)
			if !env.OK {
				t.Fatalf("envelope was not ok: %s", stdout)
			}
			data := string(env.Data)
			for _, legacy := range []string{"snapshot_id", "worktree", "head_snapshot", "latest_snapshot", "from_snapshot", "to_snapshot"} {
				if strings.Contains(data, legacy) {
					t.Fatalf("public JSON leaked %q in %s:\n%s", legacy, strings.Join(args, " "), stdout)
				}
			}
		})
	}
}

func TestContract_DocsCommandSmoke(t *testing.T) {
	cases := [][]string{
		{"--help"},
		{"checkpoint", "--help"},
		{"checkpoint", "list", "--help"},
		{"status", "--help"},
		{"restore", "--help"},
		{"fork", "--help"},
		{"workspace", "--help"},
		{"workspace", "list", "--help"},
		{"capability", "--help"},
		{"import", "--help"},
		{"clone", "--help"},
		{"gc", "--help"},
		{"doctor", "--help"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, stderr, code := runJVS(t, t.TempDir(), args...)
			if code != 0 {
				t.Fatalf("help command failed: stdout=%s stderr=%s", stdout, stderr)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("help command produced empty stdout")
			}
		})
	}
}

func decodeContractSmokeEnvelope(t *testing.T, stdout string) contractSmokeEnvelope {
	t.Helper()
	var env contractSmokeEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode JSON envelope: %v\n%s", err, stdout)
	}
	if len(env.Data) == 0 {
		t.Fatalf("JSON output missing data field: %s", stdout)
	}
	return env
}

func decodeContractSmokeDataMap(t *testing.T, stdout string) map[string]any {
	t.Helper()
	env := decodeContractSmokeEnvelope(t, stdout)
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode JSON data: %v\n%s", err, stdout)
	}
	return data
}
