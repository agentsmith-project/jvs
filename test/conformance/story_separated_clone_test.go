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
		"--target-control-root", targetControl,
		"--target-payload-root", targetPayload,
		"--save-points", "main",
	)
	if code != 0 {
		t.Fatalf("separated clone failed: stdout=%s stderr=%s", stdout, stderr)
	}
	clone := requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, targetControl, targetPayload, "main")
	if clone["source_repo_id"] == clone["target_repo_id"] {
		t.Fatalf("clone reused source repo_id: %#v", clone)
	}
	if clone["newest_save_point"] != baseline {
		t.Fatalf("clone newest save point = %#v, want %s in %#v", clone["newest_save_point"], baseline, clone)
	}
	if clone["doctor_strict"] != "passed" {
		t.Fatalf("clone doctor_strict = %#v, want passed in %#v", clone["doctor_strict"], clone)
	}

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

	doctor := separatedJSON(t, base, targetControl, "main", "doctor", "--strict")
	requireSeparatedControlAuthoritativeData(t, doctor, targetControl, targetPayload, "main")
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
	if !strings.Contains(env.Error.Message, "--target-control-root") || !strings.Contains(env.Error.Message, "--target-payload-root") {
		t.Fatalf("positional target error should require split target roots: %#v", env.Error)
	}
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
				"--target-control-root", controlRoot,
				"--target-payload-root", payloadRoot,
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
