//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContract_TargetingInferredWorkspaceMustBeRegistered(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	ghostPath := filepath.Join(repoPath, "worktrees", "ghost", "nested")
	if err := os.MkdirAll(ghostPath, 0755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVS(t, ghostPath, "--json", "info")
	env := requireContractSingleJSONEnvelope(t, stdout, stderr, code)
	if code != 0 || !env.OK {
		t.Fatalf("info from unregistered payload failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if env.Workspace != nil {
		t.Fatalf("info inferred unregistered workspace: %s", stdout)
	}

	result := runTargetingContractJSONFromCWD(t, ghostPath, "status")
	requireContractJSONFailure(t, result, "status")
	requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
	if strings.Contains(result.stdout, "ghost") {
		t.Fatalf("unregistered workspace leaked into error envelope: %s", result.stdout)
	}
}

func TestContract_TargetingFakePayloadWithRegisteredNameIsNotWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	fakeMain := filepath.Join(repoPath, "worktrees", "main", "nested")
	if err := os.MkdirAll(fakeMain, 0755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVS(t, fakeMain, "--json", "info")
	env := requireContractSingleJSONEnvelope(t, stdout, stderr, code)
	if code != 0 || !env.OK {
		t.Fatalf("info from fake main payload failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if env.Workspace != nil {
		t.Fatalf("info inferred fake main workspace: %s", stdout)
	}

	result := runTargetingContractJSONFromCWD(t, fakeMain, "status")
	requireContractJSONFailure(t, result, "status")
	requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
	if result.env.Workspace != nil {
		t.Fatalf("status recorded fake main workspace: %s", result.stdout)
	}
}

func TestContract_TargetingMetadataDirRequiresExplicitWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "one")
	firstID := createContractCheckpoint(t, repoPath, "first")
	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "two")
	secondID := createContractCheckpoint(t, repoPath, "second")

	for _, cwd := range []string{
		filepath.Join(repoPath, ".jvs"),
		filepath.Join(repoPath, ".jvs", "worktrees", "main"),
	} {
		for _, tc := range []struct {
			name    string
			command string
			args    []string
		}{
			{name: "status", command: "status", args: []string{"status"}},
			{name: "workspace_path", command: "workspace path", args: []string{"workspace", "path"}},
			{name: "checkpoint_list", command: "checkpoint list", args: []string{"checkpoint", "list"}},
			{name: "diff", command: "diff", args: []string{"diff", firstID, secondID}},
		} {
			t.Run(filepath.Base(cwd)+"_"+tc.name, func(t *testing.T) {
				result := runTargetingContractJSONFromCWD(t, cwd, tc.args...)
				requireContractJSONFailure(t, result, tc.command)
				requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
				if result.env.Workspace != nil {
					t.Fatalf("metadata cwd inferred workspace: %s", result.stdout)
				}
			})
		}
	}

	for _, tc := range []struct {
		name    string
		command string
		args    []string
	}{
		{name: "status", command: "status", args: []string{"--workspace", "main", "status"}},
		{name: "workspace_path", command: "workspace path", args: []string{"--workspace", "main", "workspace", "path"}},
		{name: "checkpoint_list", command: "checkpoint list", args: []string{"--workspace", "main", "checkpoint", "list"}},
		{name: "diff", command: "diff", args: []string{"--workspace", "main", "diff", firstID, secondID}},
	} {
		t.Run("explicit_"+tc.name, func(t *testing.T) {
			result := runTargetingContractJSONFromCWD(t, filepath.Join(repoPath, ".jvs"), tc.args...)
			requireContractJSONSuccess(t, result, tc.command)
			if result.env.Workspace == nil || *result.env.Workspace != "main" {
				t.Fatalf("explicit --workspace did not select main: %s", result.stdout)
			}
		})
	}
}

func TestContract_TargetingWorkspaceFlagSelectsNamedWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "base")
	createContractCheckpoint(t, repoPath, "base")
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", "feature")
	requireContractRawJSONOK(t, stdout, stderr, code, "fork")

	result := runTargetingContractJSONFromCWD(t, mainPath, "--workspace", "feature", "status")
	requireContractJSONSuccess(t, result, "status")
	if result.env.Workspace == nil || *result.env.Workspace != "feature" {
		t.Fatalf("--workspace did not override CWD workspace: %s", result.stdout)
	}
	requireContractDataString(t, result, "workspace", "feature")
}

func TestContract_CheckpointListIsWorkspaceScoped(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "main base")
	mainID := createContractCheckpoint(t, repoPath, "main only")
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", "feature")
	requireContractRawJSONOK(t, stdout, stderr, code, "fork")

	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	writeContractFile(t, filepath.Join(featurePath, "data.txt"), "feature change")
	featureID := createTargetingContractCheckpointFromCWD(t, featurePath, "feature only")

	mainResult := runTargetingContractJSONFromCWD(t, mainPath, "checkpoint", "list")
	requireContractJSONSuccess(t, mainResult, "checkpoint list")
	mainRecords := targetingContractCheckpointRecords(t, mainResult)
	if len(mainRecords) != 1 || mainRecords[0].CheckpointID != mainID || mainRecords[0].Workspace != "main" {
		t.Fatalf("main checkpoint list was not workspace-scoped: %#v\n%s", mainRecords, mainResult.stdout)
	}

	featureResult := runTargetingContractJSONFromCWD(t, featurePath, "checkpoint", "list")
	requireContractJSONSuccess(t, featureResult, "checkpoint list")
	featureRecords := targetingContractCheckpointRecords(t, featureResult)
	if len(featureRecords) != 1 || featureRecords[0].CheckpointID != featureID || featureRecords[0].Workspace != "feature" {
		t.Fatalf("feature checkpoint list was not workspace-scoped: %#v\n%s", featureRecords, featureResult.stdout)
	}

	writeContractFile(t, filepath.Join(repoPath, ".jvs", "snapshots", featureID, "tampered.txt"), "tampered")
	mainResult = runTargetingContractJSONFromCWD(t, mainPath, "checkpoint", "list")
	requireContractJSONSuccess(t, mainResult, "checkpoint list")
	mainRecords = targetingContractCheckpointRecords(t, mainResult)
	if len(mainRecords) != 1 || mainRecords[0].CheckpointID != mainID || mainRecords[0].Workspace != "main" {
		t.Fatalf("main checkpoint list exposed damaged other-workspace checkpoint: %#v\n%s", mainRecords, mainResult.stdout)
	}

	featureResult = runTargetingContractJSONFromCWD(t, featurePath, "checkpoint", "list")
	requireContractJSONSuccess(t, featureResult, "checkpoint list")
	featureRecords = targetingContractCheckpointRecords(t, featureResult)
	if len(featureRecords) != 1 || featureRecords[0].CheckpointID != featureID || featureRecords[0].Workspace != "feature" {
		t.Fatalf("feature checkpoint list hid current-workspace damaged checkpoint: %#v\n%s", featureRecords, featureResult.stdout)
	}
}

func TestContract_DiffRejectsCrossWorkspaceCheckpointRefs(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "main base")
	mainID := createContractCheckpoint(t, repoPath, "main base")
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", "feature")
	requireContractRawJSONOK(t, stdout, stderr, code, "fork")

	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	writeContractFile(t, filepath.Join(featurePath, "data.txt"), "feature change")
	featureID := createTargetingContractCheckpointFromCWD(t, featurePath, "feature tagged", "--tag", "feature-tag")

	for _, tc := range []struct {
		name string
		ref  string
	}{
		{name: "full_id", ref: featureID},
		{name: "short_id", ref: featureID[:12]},
		{name: "tag", ref: "feature-tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runTargetingContractJSONFromCWD(t, mainPath, "diff", mainID, tc.ref)
			requireContractJSONFailure(t, result, "diff")
			requireContractErrorCode(t, result, "E_REF_NOT_FOUND")
			if result.env.Workspace == nil || *result.env.Workspace != "main" {
				t.Fatalf("diff did not record main workspace: %s", result.stdout)
			}
			errData := contractErrorMap(t, result)
			message, _ := errData["message"].(string)
			if strings.Contains(message, ".jvs") || strings.Contains(message, "worktrees") {
				t.Fatalf("cross-workspace diff error leaked internals: %#v\n%s", errData, result.stdout)
			}
		})
	}

	featureResult := runTargetingContractJSONFromCWD(t, featurePath, "diff", featureID, "feature-tag")
	requireContractJSONSuccess(t, featureResult, "diff")
}

func TestContract_WorkspaceMissingUsesNotWorkspaceCode(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	for _, tc := range []struct {
		name    string
		command string
		args    []string
	}{
		{name: "path", command: "workspace path", args: []string{"workspace", "path", "missing"}},
		{name: "remove", command: "workspace remove", args: []string{"workspace", "remove", "missing"}},
		{name: "rename", command: "workspace rename", args: []string{"workspace", "rename", "missing", "renamed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runContractJSON(t, repoPath, tc.args...)
			requireContractJSONFailure(t, result, tc.command)
			requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
			errData := contractErrorMap(t, result)
			message, _ := errData["message"].(string)
			if strings.Contains(message, ".jvs") || strings.Contains(message, "config") {
				t.Fatalf("missing workspace error leaked internals: %#v\n%s", errData, result.stdout)
			}
		})
	}
}

func TestContract_CheckpointListRequiresWorkspace(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	writeContractFile(t, filepath.Join(repoPath, "main", "data.txt"), "base")
	createContractCheckpoint(t, repoPath, "base")

	result := runTargetingContractJSONFromCWD(t, repoPath, "checkpoint", "list")
	requireContractJSONFailure(t, result, "checkpoint list")
	requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
	if result.env.Workspace != nil {
		t.Fatalf("checkpoint list from repo root inferred workspace: %s", result.stdout)
	}

	result = runTargetingContractJSONFromCWD(t, repoPath, "--workspace", "main", "checkpoint", "list")
	requireContractJSONSuccess(t, result, "checkpoint list")
	if result.env.Workspace == nil || *result.env.Workspace != "main" {
		t.Fatalf("checkpoint list did not record explicit workspace: %s", result.stdout)
	}
}

func TestContract_DiffRequiresWorkspaceBeforeFullIDResolution(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()
	mainPath := filepath.Join(repoPath, "main")

	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "one")
	firstID := createContractCheckpoint(t, repoPath, "first")
	writeContractFile(t, filepath.Join(mainPath, "data.txt"), "two")
	secondID := createContractCheckpoint(t, repoPath, "second")

	result := runTargetingContractJSONFromCWD(t, repoPath, "diff", firstID, secondID)
	requireContractJSONFailure(t, result, "diff")
	requireContractErrorCode(t, result, "E_NOT_WORKSPACE")
	if result.env.Workspace != nil {
		t.Fatalf("diff from repo root inferred workspace: %s", result.stdout)
	}

	result = runTargetingContractJSONFromCWD(t, mainPath, "diff", firstID, secondID)
	requireContractJSONSuccess(t, result, "diff")

	result = runTargetingContractJSONFromCWD(t, repoPath, "--workspace", "main", "diff", firstID, secondID)
	requireContractJSONSuccess(t, result, "diff")
}

func runTargetingContractJSONFromCWD(t *testing.T, cwd string, args ...string) contractCommandResult {
	t.Helper()
	allArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVS(t, cwd, allArgs...)
	env := requireContractSingleJSONEnvelope(t, stdout, stderr, code)
	return contractCommandResult{stdout: stdout, stderr: stderr, code: code, env: env}
}

type targetingContractCheckpointRecord struct {
	CheckpointID string `json:"checkpoint_id"`
	Workspace    string `json:"workspace"`
}

func createTargetingContractCheckpointFromCWD(t *testing.T, cwd, note string, args ...string) string {
	t.Helper()
	allArgs := append([]string{"checkpoint", note}, args...)
	result := runTargetingContractJSONFromCWD(t, cwd, allArgs...)
	requireContractJSONSuccess(t, result, "checkpoint")
	data := contractDataMap(t, result)
	id, _ := data["checkpoint_id"].(string)
	if id == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", result.stdout)
	}
	return id
}

func targetingContractCheckpointRecords(t *testing.T, result contractCommandResult) []targetingContractCheckpointRecord {
	t.Helper()
	var records []targetingContractCheckpointRecord
	if err := json.Unmarshal(result.env.Data, &records); err != nil {
		t.Fatalf("decode checkpoint list data: %v\n%s", err, result.stdout)
	}
	return records
}
