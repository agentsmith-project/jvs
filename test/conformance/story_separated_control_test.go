//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorySeparatedControlInitCreatesMissingRootsAndReportsAuthoritativeJSON(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, stderr, code := runJVS(t, base, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated init failed: stdout=%s stderr=%s", stdout, stderr)
	}

	requireAbsolutePathExists(t, controlRoot)
	requireAbsolutePathExists(t, payloadRoot)
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
	initData := requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, "main")
	requireSeparatedControlSetupFields(t, initData, payloadRoot)
}

func TestStorySeparatedControlExplicitStatusIgnoresCleanCWD(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")

	cleanCWD := filepath.Join(base, "clean-cwd")
	if err := os.MkdirAll(cleanCWD, 0755); err != nil {
		t.Fatalf("create clean cwd: %v", err)
	}

	stdout, stderr, code := runJVS(t, cleanCWD,
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated status from clean cwd failed: stdout=%s stderr=%s", stdout, stderr)
	}

	requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, "main")

	humanStdout, humanStderr, humanCode := runJVS(t, cleanCWD,
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	if humanCode != 0 {
		t.Fatalf("separated human status from clean cwd failed: stdout=%s stderr=%s", humanStdout, humanStderr)
	}
	if !strings.Contains(humanStdout, "Control data: "+controlRoot) {
		t.Fatalf("separated human status should label external control root as control data:\n%s", humanStdout)
	}
	if strings.Contains(humanStdout, "Repo: "+controlRoot) {
		t.Fatalf("separated human status should not label control root as repo:\n%s", humanStdout)
	}
}

func TestStorySeparatedControlRejectsRepoFlagAndAmbientSelectors(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")

	for _, tc := range []struct {
		name    string
		args    []string
		command string
	}{
		{
			name:    "repo flag status",
			args:    []string{"--json", "--repo", controlRoot, "status"},
			command: "status",
		},
		{
			name:    "repo flag status with workspace",
			args:    []string{"--json", "--repo", controlRoot, "--workspace", "main", "status"},
			command: "status",
		},
		{
			name:    "repo flag save",
			args:    []string{"--json", "--repo", controlRoot, "save", "-m", "blocked"},
			command: "save",
		},
		{
			name:    "repo flag doctor",
			args:    []string{"--json", "--repo", controlRoot, "doctor", "--strict"},
			command: "doctor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runJVS(t, base, tc.args...)
			env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXPLICIT_TARGET_REQUIRED")
			requireSeparatedSelectorHint(t, env, controlRoot, tc.command)
		})
	}

	nestedCWD := filepath.Join(controlRoot, "nested")
	if err := os.MkdirAll(nestedCWD, 0755); err != nil {
		t.Fatalf("create nested control cwd: %v", err)
	}
	stdout, stderr, code := runJVS(t, nestedCWD, "--json", "status")
	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXPLICIT_TARGET_REQUIRED")
	requireSeparatedSelectorHint(t, env, controlRoot, "status")
}

func TestStorySeparatedControlWorkspaceFolderNakedStatusHintDoesNotSuggestInit(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")

	stdout, stderr, code := runJVS(t, payloadRoot, "--json", "status")

	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_NOT_REPO")
	if strings.Contains(env.Error.Hint, "jvs init") || strings.Contains(env.Error.Message, "jvs init") {
		t.Fatalf("workspace-folder naked status must not suggest init: %#v", env.Error)
	}
}

func TestStorySeparatedControlMissingWorkspaceHintIsCopyable(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")

	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"status",
	)

	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXPLICIT_TARGET_REQUIRED")
	requireSeparatedSelectorHint(t, env, controlRoot, "status")
}

func TestStorySeparatedControlInitRejectsNonMainWorkspaceWithoutMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, stderr, code := runJVS(t, base, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "feature",
		"--json",
	)

	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_WORKSPACE_MISMATCH")
	if !strings.Contains(env.Error.Hint, "--workspace main") {
		t.Fatalf("non-main init hint should point at --workspace main: %#v", env.Error)
	}
	requireAbsolutePathAbsent(t, controlRoot)
	requireAbsolutePathAbsent(t, payloadRoot)
}

func TestStorySeparatedControlPayloadLocatorPresentFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker func(t *testing.T, payloadRoot string)
	}{
		{
			name: "file",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("untrusted locator\n"), 0644); err != nil {
					t.Fatalf("write payload .jvs file: %v", err)
				}
			},
		},
		{
			name: "directory",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(payloadRoot, ".jvs"), 0755); err != nil {
					t.Fatalf("create payload .jvs directory: %v", err)
				}
			},
		},
		{
			name: "symlink",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.Symlink("elsewhere", filepath.Join(payloadRoot, ".jvs")); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			if err := os.MkdirAll(payloadRoot, 0755); err != nil {
				t.Fatalf("create payload root: %v", err)
			}
			tc.marker(t, payloadRoot)

			stdout, stderr, code := runJVS(t, base, "init",
				payloadRoot,
				"--control-root", controlRoot,
				"--workspace", "main",
				"--json",
			)

			requireSeparatedControlJSONError(t, stdout, stderr, code, "E_WORKSPACE_CONTROL_MARKER_PRESENT")
			requireAbsolutePathPresent(t, filepath.Join(payloadRoot, ".jvs"))
			requireAbsolutePathAbsent(t, controlRoot)
		})
	}
}

func TestStorySeparatedControlBoundaryRootCasesFailWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		controlRoot func(base string) string
		payloadRoot func(base string) string
		wantCode    string
		wantMissing []func(base string) string
	}{
		{
			name:        "same root",
			controlRoot: func(base string) string { return filepath.Join(base, "repo") },
			payloadRoot: func(base string) string { return filepath.Join(base, "repo") },
			wantCode:    "E_CONTROL_WORKSPACE_OVERLAP",
			wantMissing: []func(base string) string{
				func(base string) string { return filepath.Join(base, "repo") },
			},
		},
		{
			name:        "payload inside control",
			controlRoot: func(base string) string { return filepath.Join(base, "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "control", "payload") },
			wantCode:    "E_WORKSPACE_INSIDE_CONTROL",
			wantMissing: []func(base string) string{
				func(base string) string { return filepath.Join(base, "control") },
			},
		},
		{
			name:        "control inside payload",
			controlRoot: func(base string) string { return filepath.Join(base, "payload", "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "payload") },
			wantCode:    "E_CONTROL_INSIDE_WORKSPACE",
			wantMissing: []func(base string) string{
				func(base string) string { return filepath.Join(base, "payload") },
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := tc.controlRoot(base)
			payloadRoot := tc.payloadRoot(base)

			stdout, stderr, code := runJVS(t, base, "init",
				payloadRoot,
				"--control-root", controlRoot,
				"--workspace", "main",
				"--json",
			)

			requireSeparatedControlJSONError(t, stdout, stderr, code, tc.wantCode)
			for _, missing := range tc.wantMissing {
				requireAbsolutePathMissing(t, missing(base))
			}
		})
	}
}

func TestStorySeparatedControlOccupiedControlRootFailsWithoutMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	if err := os.MkdirAll(controlRoot, 0755); err != nil {
		t.Fatalf("create occupied control root: %v", err)
	}
	sentinel := filepath.Join(controlRoot, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("do not touch\n"), 0644); err != nil {
		t.Fatalf("write control occupancy sentinel: %v", err)
	}

	stdout, stderr, code := runJVS(t, base, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)

	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_TARGET_ROOT_OCCUPIED")
	if got := readAbsoluteFile(t, sentinel); got != "do not touch\n" {
		t.Fatalf("control occupancy sentinel mutated: %q", got)
	}
	requireAbsolutePathMissing(t, payloadRoot)
	requireDirectoryEntries(t, controlRoot, []string{"sentinel.txt"})
}

func TestStorySeparatedControlInitRejectsWorkspaceSymlinkEscapeWithoutControlMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	outsideRoot := filepath.Join(base, "outside")
	if err := os.MkdirAll(payloadRoot, 0755); err != nil {
		t.Fatalf("create payload root: %v", err)
	}
	if err := os.MkdirAll(outsideRoot, 0755); err != nil {
		t.Fatalf("create outside root: %v", err)
	}
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(payloadRoot, "escape")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, code := runJVS(t, base, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)

	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_PATH_BOUNDARY_ESCAPE")
	requireAbsolutePathAbsent(t, filepath.Join(controlRoot, ".jvs"))
	requireAbsolutePathAbsent(t, controlRoot)
	if got := readAbsoluteFile(t, outsideFile); got != "outside\n" {
		t.Fatalf("outside symlink target mutated: %q", got)
	}
}

func TestStorySeparatedControlInitAdoptsExistingNonEmptyWorkspaceFolder(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	userFile := filepath.Join(payloadRoot, "src", "app.txt")
	if err := os.MkdirAll(filepath.Dir(userFile), 0755); err != nil {
		t.Fatalf("create existing workspace folder: %v", err)
	}
	if err := os.WriteFile(userFile, []byte("adopt me\n"), 0644); err != nil {
		t.Fatalf("write existing workspace file: %v", err)
	}
	if err := os.Chmod(userFile, 0640); err != nil {
		t.Fatalf("set existing workspace file mode: %v", err)
	}
	originalMTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(userFile, originalMTime, originalMTime); err != nil {
		t.Fatalf("set existing workspace file mtime: %v", err)
	}
	before, err := os.Stat(userFile)
	if err != nil {
		t.Fatalf("stat existing workspace file before init: %v", err)
	}

	stdout, stderr, code := runJVS(t, base, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", "main",
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated init should adopt existing workspace folder: stdout=%s stderr=%s", stdout, stderr)
	}
	initData := requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, "main")
	if initData["unsaved_changes"] != true || initData["newest_save_point"] != nil {
		t.Fatalf("adopted init should report unsaved, not-yet-saved files: %#v", initData)
	}
	requireSeparatedControlSetupFields(t, initData, payloadRoot)
	if got := readAbsoluteFile(t, userFile); got != "adopt me\n" {
		t.Fatalf("adopted workspace file mutated: %q", got)
	}
	after, err := os.Stat(userFile)
	if err != nil {
		t.Fatalf("stat adopted workspace file after init: %v", err)
	}
	if after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("adopted workspace file metadata changed: mode %v -> %v, mtime %s -> %s",
			before.Mode(), after.Mode(), before.ModTime(), after.ModTime())
	}
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
	requireSeparatedControlRegistryEntry(t, controlRoot, payloadRoot, "main")

	statusOut, statusErr, statusCode := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	if statusCode != 0 {
		t.Fatalf("status after adopted separated init failed: stdout=%s stderr=%s", statusOut, statusErr)
	}
	statusData := requireSeparatedControlAuthoritativeJSON(t, statusOut, statusErr, controlRoot, payloadRoot, "main")
	if statusData["unsaved_changes"] != true || statusData["files_state"] != "not_saved" {
		t.Fatalf("status after adopted separated init should see existing files as unsaved: %#v", statusData)
	}

	saveOut, saveErr, saveCode := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save",
		"-m", "adopt baseline",
	)
	if saveCode != 0 {
		t.Fatalf("save after adopted separated init failed: stdout=%s stderr=%s", saveOut, saveErr)
	}
	saveData := requireSeparatedControlAuthoritativeJSON(t, saveOut, saveErr, controlRoot, payloadRoot, "main")
	savePointID, _ := saveData["save_point_id"].(string)
	if savePointID == "" {
		t.Fatalf("save after adopt missing save point id: %#v", saveData)
	}
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "snapshots", savePointID, "src", "app.txt")); got != "adopt me\n" {
		t.Fatalf("save point did not capture adopted workspace file: %q", got)
	}
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
}

func TestStorySeparatedControlStatusMissingControlFailsClosed(t *testing.T) {
	base := t.TempDir()
	missingControlRoot := filepath.Join(base, "missing-control")
	cleanCWD := filepath.Join(base, "clean-cwd")
	if err := os.MkdirAll(cleanCWD, 0755); err != nil {
		t.Fatalf("create clean cwd: %v", err)
	}

	stdout, stderr, code := runJVS(t, cleanCWD,
		"--control-root", missingControlRoot,
		"--workspace", "main",
		"status",
		"--json",
	)

	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_CONTROL_MISSING")
	requireAbsolutePathMissing(t, missingControlRoot)
}

func initSeparatedControlRepo(t *testing.T, cwd, controlRoot, payloadRoot, workspace string) {
	t.Helper()
	stdout, stderr, code := runJVS(t, cwd, "init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", workspace,
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated init setup failed: stdout=%s stderr=%s", stdout, stderr)
	}
	requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, workspace)
}

func requireSeparatedControlAuthoritativeJSON(t *testing.T, stdout, stderr, controlRoot, workspaceFolder, workspace string) map[string]any {
	t.Helper()
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
		t.Fatalf("external control root JSON repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
	}

	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode external control root JSON data object: %v\n%s", err, stdout)
	}
	requireSeparatedControlAuthoritativeData(t, data, controlRoot, workspaceFolder, workspace)
	return data
}

func requireSeparatedControlAuthoritativeData(t *testing.T, data map[string]any, controlRoot, workspaceFolder, workspace string) {
	t.Helper()
	requireSeparatedControlTargetData(t, data, controlRoot, workspaceFolder, workspace)
	requireSeparatedControlPublicModelFieldsAbsent(t, data)
}

func requireSeparatedControlDoctorData(t *testing.T, data map[string]any, controlRoot, workspaceFolder, workspace string) {
	t.Helper()
	requireSeparatedControlTargetData(t, data, controlRoot, workspaceFolder, workspace)
}

func requireSeparatedControlTargetData(t *testing.T, data map[string]any, controlRoot, workspaceFolder, workspace string) {
	t.Helper()
	if data["control_root"] != controlRoot {
		t.Fatalf("data.control_root = %#v, want %q in %#v", data["control_root"], controlRoot, data)
	}
	if data["folder"] != workspaceFolder {
		t.Fatalf("data.folder = %#v, want %q in %#v", data["folder"], workspaceFolder, data)
	}
	if data["workspace"] != workspace {
		t.Fatalf("data.workspace = %#v, want %q in %#v", data["workspace"], workspace, data)
	}
	if workspace != "main" {
		t.Fatalf("external control root conformance helper only supports workspace main, got %q", workspace)
	}
}

func requireSeparatedControlRegistryEntry(t *testing.T, controlRoot, workspaceFolder, workspace string) {
	t.Helper()

	configPath := filepath.Join(controlRoot, ".jvs", "worktrees", workspace, "config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read separated workspace registry %s: %v", configPath, err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode separated workspace registry %s: %v\n%s", configPath, err, raw)
	}
	if config["name"] != workspace {
		t.Fatalf("registry workspace name = %#v, want %q in %#v", config["name"], workspace, config)
	}
	if config["real_path"] != workspaceFolder {
		t.Fatalf("registry real_path = %#v, want %q in %#v", config["real_path"], workspaceFolder, config)
	}
}

func requireSeparatedControlSetupFields(t *testing.T, data map[string]any, workspaceFolder string) {
	t.Helper()

	capabilities, ok := data["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("external init JSON missing capabilities object: %#v", data)
	}
	if capabilities["target_path"] != workspaceFolder {
		t.Fatalf("capabilities.target_path = %#v, want %q in %#v", capabilities["target_path"], workspaceFolder, capabilities)
	}
	if capabilities["write_probe"] != true {
		t.Fatalf("capabilities.write_probe = %#v, want true in %#v", capabilities["write_probe"], capabilities)
	}
	effectiveEngine, ok := data["effective_engine"].(string)
	if !ok || effectiveEngine == "" {
		t.Fatalf("external init JSON missing effective_engine: %#v", data)
	}
	if capabilities["recommended_engine"] != effectiveEngine {
		t.Fatalf("effective_engine = %#v, want capabilities.recommended_engine %#v in %#v", effectiveEngine, capabilities["recommended_engine"], data)
	}
	metadata, ok := data["metadata_preservation"].(map[string]any)
	if !ok || len(metadata) == 0 {
		t.Fatalf("external init JSON missing metadata_preservation object: %#v", data["metadata_preservation"])
	}
	performanceClass, ok := data["performance_class"].(string)
	if !ok || performanceClass == "" {
		t.Fatalf("external init JSON missing performance_class: %#v", data)
	}
	if _, ok := data["warnings"].([]any); !ok {
		t.Fatalf("external init JSON warnings should be an array: %#v", data["warnings"])
	}
}

func requireSeparatedControlPublicModelFieldsAbsent(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{
		"repo",
		"payload_root",
		"repo_mode",
		"separated_control",
		"workspace_name",
		"locator_authoritative",
		"doctor_strict",
	} {
		if _, ok := data[field]; ok {
			t.Fatalf("external control root public JSON exposes old field data.%s in %#v", field, data)
		}
	}
}

func requireSeparatedControlJSONError(t *testing.T, stdout, stderr string, exitCode int, wantCode string) contractSmokeEnvelope {
	t.Helper()
	if exitCode == 0 {
		t.Fatalf("command unexpectedly succeeded, want %s: stdout=%s stderr=%s", wantCode, stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if !bytes.Equal(bytes.TrimSpace(env.Data), []byte("null")) {
		t.Fatalf("JSON error data = %s, want null\n%s", string(env.Data), stdout)
	}
	if env.Error == nil {
		t.Fatalf("JSON error envelope missing error, want %s\n%s", wantCode, stdout)
	}
	if env.Error.Code != wantCode {
		t.Fatalf("JSON error code = %q, want %q\n%s", env.Error.Code, wantCode, stdout)
	}
	return env
}

func requireSeparatedSelectorHint(t *testing.T, env contractSmokeEnvelope, controlRoot, command string) {
	t.Helper()
	if env.Error == nil {
		t.Fatalf("JSON error envelope missing error")
	}
	for _, want := range []string{"--control-root " + controlRoot, "--workspace main", command} {
		if !strings.Contains(env.Error.Hint, want) {
			t.Fatalf("selector hint %q missing %q in %#v", env.Error.Hint, want, env.Error)
		}
	}
}

func requireDirectoryEntries(t *testing.T, root string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read directory entries for %s: %v", root, err)
	}
	if len(entries) != len(want) {
		t.Fatalf("%s entries = %v, want %v", root, directoryEntryNames(entries), want)
	}
	for i, entry := range entries {
		if entry.Name() != want[i] {
			t.Fatalf("%s entries = %v, want %v", root, directoryEntryNames(entries), want)
		}
	}
}

func requireAbsolutePathPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}

func requireAbsolutePathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Fatalf("%s should not exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("lstat %s: %v", path, err)
	}
}

func directoryEntryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
