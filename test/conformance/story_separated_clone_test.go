//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorySeparatedCloneMainOnlySplitTarget(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initSeparatedControlRepo(t, base, sourceControl, sourcePayload, "main")
	seedSeparatedCloneSourceControlFiles(t, sourceControl)
	seedSeparatedCloneExtraRuntimeFiles(t, sourceControl)
	createFiles(t, sourcePayload, map[string]string{"app.txt": "source v1\n"})

	save := separatedJSON(t, base, sourceControl, "main", "save", "-m", "source baseline")
	baseline := save["save_point_id"].(string)

	targetControl := filepath.Join(base, "target-control")
	targetPayload := filepath.Join(base, "target-payload")
	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", sourceControl,
		"--workspace", "main",
		"repo", "clone",
		targetPayload,
		"--target-control-root", targetControl,
		"--save-points", "main",
	)
	if code != 0 {
		t.Fatalf("separated clone failed: stdout=%s stderr=%s", stdout, stderr)
	}
	clone := requireSeparatedCloneExternalControlRootJSON(t, stdout, stderr, targetControl, targetPayload)
	if clone["source_repo_id"] == clone["target_repo_id"] {
		t.Fatalf("clone reused source repo_id: %#v", clone)
	}
	if clone["newest_save_point"] != baseline {
		t.Fatalf("clone newest save point = %#v, want %s in %#v", clone["newest_save_point"], baseline, clone)
	}
	requireSeparatedCloneTransfers(t, clone, sourceControl, sourcePayload, targetControl, targetPayload)

	requireAbsolutePathPresent(t, filepath.Join(targetControl, ".jvs", "repo_id"))
	requireAbsolutePathAbsent(t, filepath.Join(targetPayload, ".jvs"))
	if got := readAbsoluteFile(t, filepath.Join(targetPayload, "app.txt")); got != "source v1\n" {
		t.Fatalf("target payload app.txt = %q", got)
	}
	for _, path := range []string{
		filepath.Join(targetControl, ".jvs", "audit", "platform.log"),
		filepath.Join(targetControl, ".jvs", "locks", "platform.lock"),
		filepath.Join(targetControl, ".jvs", "runtime", "platform.tmp"),
		filepath.Join(targetControl, ".jvs", "views", "source-view-state"),
		filepath.Join(targetControl, ".jvs", "tmp", "source.tmp"),
	} {
		requireAbsolutePathAbsent(t, path)
	}

	doctor := separatedDoctorJSON(t, base, targetControl, targetPayload, "main", "--strict")
	if doctor["healthy"] != true {
		t.Fatalf("target doctor should be healthy: %#v", doctor)
	}

	status := separatedJSON(t, base, targetControl, "main", "status")
	requireSeparatedControlAuthoritativeData(t, status, targetControl, targetPayload, "main")
	if status["folder"] != targetPayload || status["unsaved_changes"] != false {
		t.Fatalf("target status has wrong separated payload state: %#v", status)
	}

	history := separatedJSON(t, base, targetControl, "main", "history")
	if got := savePointIDsFromHistory(t, history); !sameStringSlice(got, []string{baseline}) {
		t.Fatalf("target clone history IDs = %v, want [%s]", got, baseline)
	}

	view := separatedJSON(t, base, targetControl, "main", "view", baseline)
	viewPath, _ := view["view_path"].(string)
	if viewPath == "" {
		t.Fatalf("clone target view missing view_path: %#v", view)
	}
	if got := readAbsoluteFile(t, filepath.Join(viewPath, "app.txt")); got != "source v1\n" {
		t.Fatalf("target view app.txt = %q", got)
	}
	requireAbsolutePathMissing(t, filepath.Join(viewPath, ".jvs"))
	separatedJSON(t, base, targetControl, "main", "view", "close", view["view_id"].(string))

	createFiles(t, targetPayload, map[string]string{"app.txt": "target v2\n"})
	targetSave := separatedJSON(t, base, targetControl, "main", "save", "-m", "target update")
	targetSaveID, _ := targetSave["save_point_id"].(string)
	if targetSaveID == "" || targetSaveID == baseline {
		t.Fatalf("target save did not create an independent save point: %#v", targetSave)
	}
	sourceHistory := separatedJSON(t, base, sourceControl, "main", "history")
	if got := savePointIDsFromHistory(t, sourceHistory); !sameStringSlice(got, []string{baseline}) {
		t.Fatalf("source history changed after target save: %v", got)
	}
	if got := readAbsoluteFile(t, filepath.Join(sourcePayload, "app.txt")); got != "source v1\n" {
		t.Fatalf("source payload changed after target save: %q", got)
	}
}

func TestStorySeparatedCloneRejectsPositionalTarget(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initSeparatedControlRepo(t, base, sourceControl, sourcePayload, "main")
	createFiles(t, sourcePayload, map[string]string{"app.txt": "source v1\n"})
	separatedJSON(t, base, sourceControl, "main", "save", "-m", "source baseline")

	target := filepath.Join(base, "positional-target")
	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", sourceControl,
		"--workspace", "main",
		"repo", "clone",
		target,
		"--save-points", "main",
	)
	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXPLICIT_TARGET_REQUIRED")
	if !strings.Contains(env.Error.Message, "--target-control-root") {
		t.Fatalf("positional target error should require a target control root: %#v", env.Error)
	}
	requireAbsolutePathAbsent(t, target)
}

func TestStorySeparatedCloneRepoFlagRequiresControlRootSelector(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initSeparatedControlRepo(t, base, sourceControl, sourcePayload, "main")
	createFiles(t, sourcePayload, map[string]string{"app.txt": "source v1\n"})
	separatedJSON(t, base, sourceControl, "main", "save", "-m", "source baseline")

	target := filepath.Join(base, "repo-flag-target")
	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--repo", sourceControl,
		"repo", "clone",
		target,
		"--save-points", "main",
	)
	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXPLICIT_TARGET_REQUIRED")
	requireSeparatedSelectorHint(t, env, sourceControl, "repo clone")
	requireAbsolutePathAbsent(t, target)
}

func TestStorySeparatedCloneTargetErrorsFailClosed(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initSeparatedControlRepo(t, base, sourceControl, sourcePayload, "main")
	createFiles(t, sourcePayload, map[string]string{"app.txt": "source v1\n"})
	separatedJSON(t, base, sourceControl, "main", "save", "-m", "source baseline")

	for _, tc := range []struct {
		name  string
		setup func(root string) (controlRoot, payloadRoot string)
		mode  string
		code  string
	}{
		{
			name: "occupied control",
			setup: func(root string) (string, string) {
				controlRoot := filepath.Join(root, "occupied-control")
				payloadRoot := filepath.Join(root, "occupied-payload")
				createFiles(t, controlRoot, map[string]string{"user.txt": "keep\n"})
				return controlRoot, payloadRoot
			},
			mode: "main",
			code: "E_TARGET_ROOT_OCCUPIED",
		},
		{
			name: "occupied payload",
			setup: func(root string) (string, string) {
				controlRoot := filepath.Join(root, "occupied-control")
				payloadRoot := filepath.Join(root, "occupied-payload")
				createFiles(t, payloadRoot, map[string]string{"user.txt": "keep\n"})
				return controlRoot, payloadRoot
			},
			mode: "main",
			code: "E_TARGET_ROOT_OCCUPIED",
		},
		{
			name: "same target roots",
			setup: func(root string) (string, string) {
				targetRoot := filepath.Join(root, "same")
				return targetRoot, targetRoot
			},
			mode: "main",
			code: "E_CONTROL_PAYLOAD_OVERLAP",
		},
		{
			name: "payload inside control",
			setup: func(root string) (string, string) {
				controlRoot := filepath.Join(root, "target-control")
				return controlRoot, filepath.Join(controlRoot, "payload")
			},
			mode: "main",
			code: "E_PAYLOAD_INSIDE_CONTROL",
		},
		{
			name: "control inside payload",
			setup: func(root string) (string, string) {
				payloadRoot := filepath.Join(root, "target-payload")
				return filepath.Join(payloadRoot, "control"), payloadRoot
			},
			mode: "main",
			code: "E_CONTROL_INSIDE_PAYLOAD",
		},
		{
			name: "all protection missing",
			setup: func(root string) (string, string) {
				return filepath.Join(root, "all-control"), filepath.Join(root, "all-payload")
			},
			mode: "all",
			code: "E_IMPORTED_HISTORY_PROTECTION_MISSING",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			targetRoot := filepath.Join(base, "case-"+tc.name)
			controlRoot, payloadRoot := tc.setup(targetRoot)
			stdout, stderr, code := runJVS(t, base,
				"--json",
				"--control-root", sourceControl,
				"--workspace", "main",
				"repo", "clone",
				payloadRoot,
				"--target-control-root", controlRoot,
				"--save-points", tc.mode,
			)
			env := requireSeparatedControlJSONError(t, stdout, stderr, code, tc.code)
			if tc.code == "E_IMPORTED_HISTORY_PROTECTION_MISSING" && env.Error.Hint == "" {
				t.Fatalf("all protection error should hint main-only/upgrade: %#v", env.Error)
			}
			requireAbsolutePathAbsent(t, filepath.Join(controlRoot, ".jvs"))
			requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
		})
	}
}

func requireSeparatedCloneExternalControlRootJSON(t *testing.T, stdout, stderr, targetControlRoot, targetFolder string) map[string]any {
	t.Helper()
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != targetControlRoot {
		t.Fatalf("clone JSON repo_root = %#v, want target control root %q\n%s", env.RepoRoot, targetControlRoot, stdout)
	}

	data := decodeContractDataMap(t, stdout)
	if data["target_control_root"] != targetControlRoot {
		t.Fatalf("clone target_control_root = %#v, want %q in %#v", data["target_control_root"], targetControlRoot, data)
	}
	if data["target_folder"] != targetFolder {
		t.Fatalf("clone target_folder = %#v, want %q in %#v", data["target_folder"], targetFolder, data)
	}
	if got := jsonStringArray(t, data["workspaces_created"]); !sameStringSlice(got, []string{"main"}) {
		t.Fatalf("clone workspaces_created = %v, want [main] in %#v", got, data)
	}
	requireSeparatedControlPublicModelFieldsAbsent(t, data)
	for _, field := range []string{"target_payload_root", "source_payload_root"} {
		if _, ok := data[field]; ok {
			t.Fatalf("external clone public JSON exposes old field data.%s in %#v", field, data)
		}
	}
	return data
}

func seedSeparatedCloneSourceControlFiles(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"audit", "locks", "runtime"} {
		if err := os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755); err != nil {
			t.Fatalf("create control metadata dir %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), []byte("audit sentinel\n"), 0644); err != nil {
		t.Fatalf("write audit sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), []byte("lock sentinel\n"), 0644); err != nil {
		t.Fatalf("write lock sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644); err != nil {
		t.Fatalf("write runtime sentinel: %v", err)
	}
}

func seedSeparatedCloneExtraRuntimeFiles(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"views", "tmp"} {
		if err := os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "views", "source-view-state"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write source view sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "tmp", "source.tmp"), []byte("tmp\n"), 0644); err != nil {
		t.Fatalf("write source tmp sentinel: %v", err)
	}
}

func requireSeparatedCloneTransfers(t *testing.T, data map[string]any, sourceControl, sourcePayload, targetControl, targetPayload string) {
	t.Helper()

	save := requireSeparatedCloneTransferByID(t, data, "repo-clone-save-points")
	if save["source_path"] != filepath.Join(sourceControl, ".jvs") {
		t.Fatalf("clone save-point transfer source_path = %#v, want source control .jvs in %#v", save["source_path"], save)
	}
	if save["published_destination"] != filepath.Join(targetControl, ".jvs") {
		t.Fatalf("clone save-point transfer published_destination = %#v, want target control .jvs in %#v", save["published_destination"], save)
	}
	if path, _ := save["materialization_destination"].(string); strings.Contains(filepath.ToSlash(path), filepath.ToSlash(targetPayload)) {
		t.Fatalf("clone save-point transfer materialized in target payload: %#v", save)
	}
	requireSeparatedCloneCopyEvidence(t, save)

	main := requireSeparatedCloneTransferByID(t, data, "repo-clone-main-workspace")
	if main["source_path"] != sourcePayload {
		t.Fatalf("clone main transfer source_path = %#v, want source payload %q in %#v", main["source_path"], sourcePayload, main)
	}
	if main["published_destination"] != targetPayload {
		t.Fatalf("clone main transfer published_destination = %#v, want target payload %q in %#v", main["published_destination"], targetPayload, main)
	}
	if main["capability_probe_path"] != filepath.Dir(targetPayload) {
		t.Fatalf("clone main transfer capability_probe_path = %#v, want payload parent in %#v", main["capability_probe_path"], main)
	}
	if path, _ := main["materialization_destination"].(string); strings.Contains(filepath.ToSlash(path), filepath.ToSlash(targetControl)) {
		t.Fatalf("clone main transfer materialized in target control: %#v", main)
	}
	requireSeparatedCloneCopyEvidence(t, main)
}

func requireSeparatedCloneTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	if !ok {
		t.Fatalf("clone transfers should be array: %#v", data["transfers"])
	}
	for _, item := range transfers {
		transfer, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("clone transfer should be object: %#v", item)
		}
		if transfer["transfer_id"] == id {
			return transfer
		}
	}
	t.Fatalf("missing clone transfer %q in %#v", id, transfers)
	return nil
}

func requireSeparatedCloneCopyEvidence(t *testing.T, transfer map[string]any) {
	t.Helper()
	if transfer["checked_for_this_operation"] != true {
		t.Fatalf("clone transfer was not checked for this operation: %#v", transfer)
	}
	if transfer["capability_probe_path"] == "" {
		t.Fatalf("clone transfer missing capability_probe_path: %#v", transfer)
	}
	if transfer["performance_class"] != "fast_copy" && transfer["performance_class"] != "normal_copy" {
		t.Fatalf("clone transfer performance_class = %#v, want fast_copy or normal_copy in %#v", transfer["performance_class"], transfer)
	}
}
