//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStorySeparatedControlInitCreatesMissingRootsAndReportsAuthoritativeJSON(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	stdout, stderr, code := runJVS(t, base, "init",
		"--control-root", controlRoot,
		"--payload-root", payloadRoot,
		"--workspace", "main",
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated init failed: stdout=%s stderr=%s", stdout, stderr)
	}

	requireAbsolutePathExists(t, controlRoot)
	requireAbsolutePathExists(t, payloadRoot)
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
	requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, "main")
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
				"--control-root", controlRoot,
				"--payload-root", payloadRoot,
				"--workspace", "main",
				"--json",
			)

			requireSeparatedControlJSONError(t, stdout, stderr, code, "E_PAYLOAD_LOCATOR_PRESENT")
			requireAbsolutePathPresent(t, filepath.Join(payloadRoot, ".jvs"))
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
			wantCode:    "E_CONTROL_PAYLOAD_OVERLAP",
			wantMissing: []func(base string) string{
				func(base string) string { return filepath.Join(base, "repo") },
			},
		},
		{
			name:        "payload inside control",
			controlRoot: func(base string) string { return filepath.Join(base, "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "control", "payload") },
			wantCode:    "E_PAYLOAD_INSIDE_CONTROL",
			wantMissing: []func(base string) string{
				func(base string) string { return filepath.Join(base, "control") },
			},
		},
		{
			name:        "control inside payload",
			controlRoot: func(base string) string { return filepath.Join(base, "payload", "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "payload") },
			wantCode:    "E_CONTROL_INSIDE_PAYLOAD",
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
				"--control-root", controlRoot,
				"--payload-root", payloadRoot,
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

func TestStorySeparatedControlTargetOccupancyFailsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		occupiedTarget string
	}{
		{name: "control root non-empty", occupiedTarget: "control"},
		{name: "payload root non-empty", occupiedTarget: "payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			occupiedRoot := controlRoot
			otherRoot := payloadRoot
			if tc.occupiedTarget == "payload" {
				occupiedRoot = payloadRoot
				otherRoot = controlRoot
			}
			if err := os.MkdirAll(occupiedRoot, 0755); err != nil {
				t.Fatalf("create occupied root: %v", err)
			}
			sentinel := filepath.Join(occupiedRoot, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("do not touch\n"), 0644); err != nil {
				t.Fatalf("write occupancy sentinel: %v", err)
			}

			stdout, stderr, code := runJVS(t, base, "init",
				"--control-root", controlRoot,
				"--payload-root", payloadRoot,
				"--workspace", "main",
				"--json",
			)

			requireSeparatedControlJSONError(t, stdout, stderr, code, "E_TARGET_ROOT_OCCUPIED")
			if got := readAbsoluteFile(t, sentinel); got != "do not touch\n" {
				t.Fatalf("occupancy sentinel mutated: %q", got)
			}
			requireAbsolutePathMissing(t, otherRoot)
			requireDirectoryEntries(t, occupiedRoot, []string{"sentinel.txt"})
		})
	}
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
		"--control-root", controlRoot,
		"--payload-root", payloadRoot,
		"--workspace", workspace,
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated init setup failed: stdout=%s stderr=%s", stdout, stderr)
	}
	requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, workspace)
}

func requireSeparatedControlAuthoritativeJSON(t *testing.T, stdout, stderr, controlRoot, payloadRoot, workspace string) map[string]any {
	t.Helper()
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
		t.Fatalf("separated JSON repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
	}

	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode separated JSON data object: %v\n%s", err, stdout)
	}
	requireSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, workspace)
	return data
}

func requireSeparatedControlAuthoritativeData(t *testing.T, data map[string]any, controlRoot, payloadRoot, workspace string) {
	t.Helper()
	if data["control_root"] != controlRoot {
		t.Fatalf("data.control_root = %#v, want %q in %#v", data["control_root"], controlRoot, data)
	}
	if data["payload_root"] != payloadRoot {
		t.Fatalf("data.payload_root = %#v, want %q in %#v", data["payload_root"], payloadRoot, data)
	}
	if data["repo_mode"] != "separated_control" {
		t.Fatalf("data.repo_mode = %#v, want separated_control in %#v", data["repo_mode"], data)
	}
	if data["workspace_name"] != workspace {
		t.Fatalf("data.workspace_name = %#v, want %q in %#v", data["workspace_name"], workspace, data)
	}
	if data["separated_control"] != true {
		t.Fatalf("data.separated_control = %#v, want true in %#v", data["separated_control"], data)
	}
	if data["boundary_validated"] != true {
		t.Fatalf("data.boundary_validated = %#v, want true in %#v", data["boundary_validated"], data)
	}
	if data["locator_authoritative"] != false {
		t.Fatalf("data.locator_authoritative = %#v, want false in %#v", data["locator_authoritative"], data)
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
