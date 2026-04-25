//go:build conformance

package conformance

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jvs-project/jvs/internal/engine"
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

func TestContract_GCPlanJSONUsesPublicSpecFields(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
	if code != 0 {
		t.Fatalf("gc plan failed: stdout=%s stderr=%s", stdout, stderr)
	}
	data := decodeContractSmokeDataMap(t, stdout)

	for _, field := range []string{
		"plan_id",
		"protected_by_pin",
		"protected_by_lineage",
		"to_delete",
		"deletable_bytes_estimate",
	} {
		if _, ok := data[field]; !ok {
			t.Fatalf("gc plan JSON missing required field %q: %s", field, stdout)
		}
	}
	if _, ok := data["to_delete"].([]any); !ok {
		t.Fatalf("gc plan to_delete must be an array: %#v\n%s", data["to_delete"], stdout)
	}
	if _, ok := data["delete_checkpoints"]; ok {
		t.Fatalf("gc plan JSON exposed non-spec field delete_checkpoints: %s", stdout)
	}
}

func TestContract_TargetingRepoFlagStatusUsesRealCWDWorkspace(t *testing.T) {
	dir := t.TempDir()
	if stdout, stderr, code := runJVS(t, dir, "init", "repoA"); code != 0 {
		t.Fatalf("init repoA failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if stdout, stderr, code := runJVS(t, dir, "init", "repoB"); code != 0 {
		t.Fatalf("init repoB failed: stdout=%s stderr=%s", stdout, stderr)
	}

	repoA := filepath.Join(dir, "repoA")
	repoB := filepath.Join(dir, "repoB")

	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatal(err)
	}
	outsideCases := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "info", command: "info", args: []string{"--json", "--repo", repoA, "info"}},
		{name: "status", command: "status", args: []string{"--json", "--repo", repoA, "status"}},
		{name: "workspace_list", command: "workspace list", args: []string{"--json", "--repo", repoA, "workspace", "list"}},
	}
	for _, tc := range outsideCases {
		t.Run("outside_"+tc.name, func(t *testing.T) {
			stdout, stderr, code := runJVS(t, outside, tc.args...)
			if code == 0 {
				t.Fatalf("outside repo %s unexpectedly succeeded: stdout=%s stderr=%s", tc.name, stdout, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("outside repo JSON error wrote stderr: %q", stderr)
			}
			env := decodeContractSmokeEnvelope(t, stdout)
			if env.OK || env.Command != tc.command {
				t.Fatalf("unexpected outside repo envelope: %s", stdout)
			}
			errData, ok := env.Error.(map[string]any)
			if !ok {
				t.Fatalf("outside repo error was not an object: %#v\n%s", env.Error, stdout)
			}
			if errData["code"] != "E_NOT_REPO" {
				t.Fatalf("outside repo used wrong code: %#v", errData)
			}
			assertContractPublicErrorVocabulary(t, env.Error, stdout)
		})
	}

	stdout, stderr, code := runJVS(t, filepath.Join(repoA, "main"), "--json", "--repo", repoA, "status")
	if code != 0 {
		t.Fatalf("status with matching --repo failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if !env.OK || env.Workspace == nil || *env.Workspace != "main" {
		t.Fatalf("status did not infer main workspace from real CWD: %s", stdout)
	}

	stdout, stderr, code = runJVS(t, repoA, "--json", "--workspace", "main", "status")
	if code != 0 {
		t.Fatalf("status with --workspace from repo root failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if !env.OK || env.Workspace == nil || *env.Workspace != "main" {
		t.Fatalf("status did not accept --workspace from repo root: %s", stdout)
	}

	subdir := filepath.Join(repoA, "main", "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runJVS(t, subdir, "--json", "--repo", filepath.Join(repoA, "main"), "status")
	if code != 0 {
		t.Fatalf("status with --repo path inside same repo failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if !env.OK || env.Workspace == nil || *env.Workspace != "main" {
		t.Fatalf("status did not accept --repo path inside same repo: %s", stdout)
	}

	stdout, stderr, code = runJVS(t, filepath.Join(repoB, "main"), "--json", "--repo", repoA, "status")
	if code == 0 {
		t.Fatalf("status with mismatched --repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("targeting mismatch wrote stderr in JSON mode: %q", stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("targeting mismatch returned ok envelope: %s", stdout)
	}
	errData, ok := env.Error.(map[string]any)
	if !ok {
		t.Fatalf("targeting mismatch error was not an object: %#v\n%s", env.Error, stdout)
	}
	if errData["code"] != "E_TARGET_MISMATCH" {
		t.Fatalf("targeting mismatch used wrong code: %#v", errData)
	}
	assertContractPublicErrorVocabulary(t, env.Error, stdout)

	stdout, stderr, code = runJVS(t, filepath.Join(repoB, "main"), "--json", "--repo", repoA, "--workspace", "main", "status")
	if code == 0 {
		t.Fatalf("status with mismatched --repo and --workspace unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("targeting mismatch with --workspace wrote stderr in JSON mode: %q", stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("targeting mismatch with --workspace returned ok envelope: %s", stdout)
	}
	errData, ok = env.Error.(map[string]any)
	if !ok {
		t.Fatalf("targeting mismatch with --workspace error was not an object: %#v\n%s", env.Error, stdout)
	}
	if errData["code"] != "E_TARGET_MISMATCH" {
		t.Fatalf("targeting mismatch with --workspace used wrong code: %#v", errData)
	}
	assertContractPublicErrorVocabulary(t, env.Error, stdout)
}

func TestContract_DoctorAndGCJSONDoNotExposeInternalFields(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	emptyPlanOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
	if code != 0 {
		t.Fatalf("empty gc plan failed: stdout=%s stderr=%s", emptyPlanOut, stderr)
	}
	assertContractDataOmitsInternalFields(t, emptyPlanOut)

	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("main"), 0644); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "main"); code != 0 {
		t.Fatalf("main checkpoint failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "old-feature"); code != 0 {
		t.Fatalf("fork old-feature failed: stdout=%s stderr=%s", stdout, stderr)
	}
	featurePath := filepath.Join(repoPath, "worktrees", "old-feature")
	if err := os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr, code := runJVSInWorktree(t, repoPath, "old-feature", "checkpoint", "feature"); code != 0 {
		t.Fatalf("feature checkpoint failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if stdout, stderr, code := runJVSInRepo(t, repoPath, "workspace", "remove", "old-feature", "--force"); code != 0 {
		t.Fatalf("remove old-feature failed: stdout=%s stderr=%s", stdout, stderr)
	}
	makeDescriptorsOldForContract(t, repoPath)

	nonEmptyPlanOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
	if code != 0 {
		t.Fatalf("non-empty gc plan failed: stdout=%s stderr=%s", nonEmptyPlanOut, stderr)
	}
	assertContractDataOmitsInternalFields(t, nonEmptyPlanOut)
	planData := decodeContractSmokeDataMap(t, nonEmptyPlanOut)
	if planData["candidate_count"] == float64(0) {
		t.Fatalf("expected non-empty gc plan, got: %s", nonEmptyPlanOut)
	}

	if err := os.RemoveAll(mainPath); err != nil {
		t.Fatal(err)
	}
	doctorOut, stderr, code := runJVS(t, repoPath, "--json", "doctor")
	if code == 0 {
		t.Fatalf("unhealthy doctor unexpectedly succeeded: stdout=%s stderr=%s", doctorOut, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("doctor --json wrote stderr: %q", stderr)
	}
	assertContractDataOmitsInternalFields(t, doctorOut)
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

func TestContract_CloneCurrentRejectsWorkspacePathSource(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	dest := filepath.Join(t.TempDir(), "current-copy")
	stdout, stderr, code := runJVS(t, t.TempDir(), "--json", "clone", filepath.Join(repoPath, "main"), dest, "--scope", "current")
	if code == 0 {
		t.Fatalf("clone current with workspace source unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("clone current workspace-source JSON error wrote stderr: %q", stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("clone current workspace-source returned ok envelope: %s", stdout)
	}
	errData, ok := env.Error.(map[string]any)
	if !ok {
		t.Fatalf("clone current workspace-source error was not an object: %#v\n%s", env.Error, stdout)
	}
	message, _ := errData["message"].(string)
	hint, _ := errData["hint"].(string)
	if !strings.Contains(message, "repository root") || !strings.Contains(message+hint, "source-workspace") {
		t.Fatalf("clone current workspace-source error lacked guidance: %#v\n%s", errData, stdout)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("clone current workspace-source created destination: %v", err)
	}
}

func TestContract_CloneCurrentJSONSeparatesTransferFromMaterializationEngine(t *testing.T) {
	dir := t.TempDir()
	report, err := engine.ProbeCapabilities(dir, true)
	if err != nil {
		t.Fatalf("probe temp capabilities: %v", err)
	}
	if report.JuiceFS.Supported {
		t.Skip("test requires a non-JuiceFS temp directory")
	}

	if stdout, stderr, code := runJVS(t, dir, "init", "source"); code != 0 {
		t.Fatalf("init source failed: stdout=%s stderr=%s", stdout, stderr)
	}
	repoPath := filepath.Join(dir, "source")
	if err := os.WriteFile(filepath.Join(repoPath, "main", "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JVS_SNAPSHOT_ENGINE", "juicefs-clone")
	t.Setenv("JVS_ENGINE", "")
	dest := filepath.Join(dir, "dest")
	stdout, stderr, code := runJVS(t, dir, "--json", "clone", repoPath, dest, "--scope", "current")
	if code != 0 {
		t.Fatalf("clone current failed: stdout=%s stderr=%s", stdout, stderr)
	}
	data := decodeContractSmokeDataMap(t, stdout)
	if data["effective_engine"] != "juicefs-clone" {
		t.Fatalf("effective_engine must describe future materialization: %#v\n%s", data["effective_engine"], stdout)
	}
	if data["transfer_engine"] != "copy" {
		t.Fatalf("transfer_engine must describe this transfer: %#v\n%s", data["transfer_engine"], stdout)
	}
	if data["transfer_engine"] == data["effective_engine"] {
		t.Fatalf("transfer_engine and effective_engine were not separated: %s", stdout)
	}
	if _, ok := data["degraded_reasons"].([]any); !ok {
		t.Fatalf("clone current degraded_reasons must be an array: %s", stdout)
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

func TestContract_DoctorVerifyIntegrityContracts(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "doctor", "--repair-list")
	if code != 0 {
		t.Fatalf("doctor --repair-list failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	var actions []map[string]any
	if err := json.Unmarshal(env.Data, &actions); err != nil {
		t.Fatalf("decode repair list: %v\n%s", err, stdout)
	}
	var ids []string
	for _, action := range actions {
		id, _ := action["id"].(string)
		ids = append(ids, id)
	}
	wantIDs := []string{"clean_locks", "clean_runtime_tmp", "clean_runtime_operations"}
	if strings.Join(ids, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("unexpected repair list ids %v in %s", ids, stdout)
	}
	for _, forbidden := range []string{"rebuild", "audit_repair", "advance", "clean_tmp", "clean_intents"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("repair list exposed non-public action %q:\n%s", forbidden, stdout)
		}
	}

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "doctor", "--repair-runtime", "clean_runtime_tmp")
	if code == 0 {
		t.Fatalf("doctor --repair-runtime with arg unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("doctor usage error wrote stderr in JSON mode: %q", stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if env.OK || env.Command != "doctor" {
		t.Fatalf("unexpected doctor usage envelope: %s", stdout)
	}
	errData, ok := env.Error.(map[string]any)
	if !ok || errData["code"] != "E_USAGE" {
		t.Fatalf("unexpected doctor usage error: %#v\n%s", env.Error, stdout)
	}

	if err := os.WriteFile(filepath.Join(repoPath, ".jvs-tmp-contract"), []byte("tmp"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "doctor", "--repair-runtime")
	if code != 0 {
		t.Fatalf("doctor --repair-runtime failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	var doctorData map[string]any
	if err := json.Unmarshal(env.Data, &doctorData); err != nil {
		t.Fatalf("decode doctor repair data: %v\n%s", err, stdout)
	}
	repairs, ok := doctorData["repairs"].([]any)
	if !ok || len(repairs) == 0 {
		t.Fatalf("doctor repair JSON missing repairs: %s", stdout)
	}
	assertContractDataOmitsInternalFields(t, stdout)

	stdout, stderr, code = runJVSInRepo(t, repoPath, "--json", "verify", "--all", "1708300800000-deadbeef")
	if code == 0 {
		t.Fatalf("verify --all with checkpoint unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("verify usage error wrote stderr in JSON mode: %q", stderr)
	}
	env = decodeContractSmokeEnvelope(t, stdout)
	if env.OK || env.Command != "verify" {
		t.Fatalf("unexpected verify usage envelope: %s", stdout)
	}
	errData, ok = env.Error.(map[string]any)
	if !ok || errData["code"] != "E_USAGE" {
		t.Fatalf("unexpected verify usage error: %#v\n%s", env.Error, stdout)
	}
}

func TestContract_SetupJSONContract_InitAndCapability(t *testing.T) {
	dir := t.TempDir()

	initTarget := filepath.Join(dir, "initrepo")
	stdout, stderr, code := runJVS(t, dir, "--json", "init", initTarget)
	if code != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("init --json wrote stderr: %q", stderr)
	}
	initData := assertContractSetupJSONData(t, stdout)
	if initData["repo_root"] != initTarget {
		t.Fatalf("init repo_root mismatch: %#v\n%s", initData["repo_root"], stdout)
	}

	capabilityTarget := filepath.Join(dir, "capability-target")
	if err := os.Mkdir(capabilityTarget, 0755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runJVS(t, dir, "--json", "capability", capabilityTarget)
	if code != 0 {
		t.Fatalf("capability failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("capability --json wrote stderr: %q", stderr)
	}
	capabilityData := assertContractSetupJSONData(t, stdout)
	if capabilityData["target_path"] != capabilityTarget {
		t.Fatalf("capability target_path mismatch: %#v\n%s", capabilityData["target_path"], stdout)
	}
	if _, ok := capabilityData["write_probe"].(bool); !ok {
		t.Fatalf("capability write_probe must be a bool: %s", stdout)
	}
}

func TestContract_SetupRejectsPhysicalOverlapViaSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	sourceData := filepath.Join(source, "data")
	if err := os.MkdirAll(sourceData, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceData, "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(dir, "link-to-data")
	requireContractSymlink(t, sourceData, linkParent)
	dest := filepath.Join(linkParent, "repo")

	stdout, stderr, code := runJVS(t, dir, "--json", "import", source, dest)
	if code == 0 {
		t.Fatalf("overlapping import unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("overlapping import JSON error wrote stderr: %q", stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("overlapping import returned ok envelope: %s", stdout)
	}
	errData, ok := env.Error.(map[string]any)
	if !ok {
		t.Fatalf("overlapping import error was not an object: %#v\n%s", env.Error, stdout)
	}
	if !strings.Contains(strings.ToLower(errData["message"].(string)), "physical path overlap") {
		t.Fatalf("overlapping import error did not mention physical overlap: %#v\n%s", errData, stdout)
	}
	if _, err := os.Stat(filepath.Join(sourceData, "repo", ".jvs")); !os.IsNotExist(err) {
		t.Fatalf("overlapping import created repo metadata: %v", err)
	}
}

func TestContract_CapabilityJSONIncludesMetadataPreservationAndPerformanceClass(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "capability-target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVS(t, dir, "--json", "capability", target)
	if code != 0 {
		t.Fatalf("capability failed: stdout=%s stderr=%s", stdout, stderr)
	}
	data := decodeContractSmokeDataMap(t, stdout)
	if class, ok := data["performance_class"].(string); !ok || class == "" {
		t.Fatalf("capability performance_class must be a non-empty string: %s", stdout)
	}
	metadata, ok := data["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("capability metadata_preservation must be an object: %s", stdout)
	}
	for _, field := range []string{"symlinks", "hardlinks", "mode", "timestamps", "xattrs", "acls"} {
		if value, _ := metadata[field].(string); value == "" {
			t.Fatalf("capability metadata_preservation.%s must be non-empty: %s", field, stdout)
		}
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
	dec := json.NewDecoder(strings.NewReader(stdout))
	var env contractSmokeEnvelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("decode JSON envelope: %v\n%s", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout must contain exactly one JSON value: %s", stdout)
	}
	if len(env.Data) == 0 {
		t.Fatalf("JSON output missing data field: %s", stdout)
	}
	return env
}

func assertContractPublicErrorVocabulary(t *testing.T, errValue any, stdout string) {
	t.Helper()
	errData, ok := errValue.(map[string]any)
	if !ok {
		t.Fatalf("error was not an object: %#v\n%s", errValue, stdout)
	}
	for _, field := range []string{"code", "message", "hint"} {
		value, _ := errData[field].(string)
		lower := strings.ToLower(value)
		for _, legacy := range []string{"worktree", "snapshot", "history"} {
			if strings.Contains(lower, legacy) {
				t.Fatalf("public error leaked %q in %s: %s", legacy, field, stdout)
			}
		}
	}
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

func assertContractSetupJSONData(t *testing.T, stdout string) map[string]any {
	t.Helper()
	data := decodeContractSmokeDataMap(t, stdout)

	capabilities, ok := data["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("setup JSON data.capabilities must be an object: %s", stdout)
	}
	for _, field := range []string{"write", "juicefs", "reflink", "copy"} {
		if _, ok := capabilities[field].(map[string]any); !ok {
			t.Fatalf("setup JSON data.capabilities.%s must be an object: %s", field, stdout)
		}
	}

	effectiveEngine, ok := data["effective_engine"].(string)
	if !ok || effectiveEngine == "" {
		t.Fatalf("setup JSON data.effective_engine must be a non-empty string: %s", stdout)
	}
	if _, ok := data["warnings"].([]any); !ok {
		t.Fatalf("setup JSON data.warnings must be an array: %s", stdout)
	}

	return data
}

func requireContractSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func assertContractDataOmitsInternalFields(t *testing.T, stdout string) {
	t.Helper()
	env := decodeContractSmokeEnvelope(t, stdout)
	data := string(env.Data)
	for _, forbidden := range []string{
		"snapshot_id",
		"worktree",
		"head_snapshot",
		"latest_snapshot",
		"keep_min_snapshots",
	} {
		if strings.Contains(data, forbidden) {
			t.Fatalf("public JSON leaked %q:\n%s", forbidden, stdout)
		}
	}
}

func makeDescriptorsOldForContract(t *testing.T, repoPath string) {
	t.Helper()
	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	entries, err := os.ReadDir(descriptorsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(descriptorsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var desc map[string]any
		if err := json.Unmarshal(data, &desc); err != nil {
			t.Fatal(err)
		}
		desc["created_at"] = "2000-01-01T00:00:00Z"
		data, err = json.MarshalIndent(desc, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
