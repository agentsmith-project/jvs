//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContract_SetupCommandsPathScopedIgnoreRepoWorkspaceAndCWD(t *testing.T) {
	dir := t.TempDir()
	repoA := filepath.Join(dir, "repoA")
	if stdout, stderr, code := runJVS(t, dir, "init", repoA); code != 0 {
		t.Fatalf("init repoA failed: stdout=%s stderr=%s", stdout, stderr)
	}

	unusedRepoFlag := filepath.Join(dir, "missing-repo")
	unusedWorkspaceFlag := "missing-workspace"
	repoCWD := filepath.Join(repoA, "main")

	initTarget := filepath.Join(dir, "targets", "initrepo")
	stdout, stderr, code := runJVS(t, repoCWD, "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "init", initTarget)
	requireContractSetupSuccess(t, stdout, stderr, code, "init")
	if data := decodeContractSmokeDataMap(t, stdout); data["repo_root"] != initTarget {
		t.Fatalf("init used non-explicit target: %#v\n%s", data["repo_root"], stdout)
	}

	source := filepath.Join(dir, "import-source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("import"), 0644); err != nil {
		t.Fatal(err)
	}
	importTarget := filepath.Join(dir, "targets", "importrepo")
	stdout, stderr, code = runJVS(t, repoCWD, "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "import", source, importTarget)
	requireContractSetupSuccess(t, stdout, stderr, code, "import")
	if data := decodeContractSmokeDataMap(t, stdout); data["repo_root"] != importTarget {
		t.Fatalf("import used non-explicit target: %#v\n%s", data["repo_root"], stdout)
	}

	cloneTarget := filepath.Join(dir, "targets", "clonerepo")
	stdout, stderr, code = runJVS(t, repoCWD, "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "clone", initTarget, cloneTarget, "--scope", "current")
	requireContractSetupSuccess(t, stdout, stderr, code, "clone")
	if data := decodeContractSmokeDataMap(t, stdout); data["repo_root"] != cloneTarget {
		t.Fatalf("clone used non-explicit target: %#v\n%s", data["repo_root"], stdout)
	}

	capabilityTarget := filepath.Join(dir, "missing", "a", "b")
	stdout, stderr, code = runJVS(t, repoCWD, "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "capability", capabilityTarget, "--write-probe")
	requireContractSetupSuccess(t, stdout, stderr, code, "capability")
	capabilityData := decodeContractSmokeDataMap(t, stdout)
	if capabilityData["target_path"] != capabilityTarget || capabilityData["probe_path"] != dir {
		t.Fatalf("capability did not probe explicit missing target parent: %#v\n%s", capabilityData, stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("capability created missing target path: %v", err)
	}

	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatal(err)
	}
	outsideTarget := filepath.Join(dir, "targets", "outside-init")
	stdout, stderr, code = runJVS(t, outside, "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "init", outsideTarget)
	requireContractSetupSuccess(t, stdout, stderr, code, "init from non-repo cwd")
}

func TestContract_SetupCommandsRequireExplicitPositionalPaths(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "init", args: []string{"--json", "--repo", filepath.Join(dir, "target"), "init"}},
		{name: "import", args: []string{"--json", "--repo", filepath.Join(dir, "source"), "--workspace", filepath.Join(dir, "target"), "import"}},
		{name: "clone", args: []string{"--json", "--repo", filepath.Join(dir, "source"), "--workspace", filepath.Join(dir, "target"), "clone"}},
		{name: "capability", args: []string{"--json", "--repo", filepath.Join(dir, "target"), "capability"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runJVS(t, dir, tc.args...)
			if code == 0 {
				t.Fatalf("%s unexpectedly accepted flags as positional args: stdout=%s stderr=%s", tc.name, stdout, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("%s JSON error wrote stderr: %q", tc.name, stderr)
			}
			env := decodeContractSmokeEnvelope(t, stdout)
			if env.OK {
				t.Fatalf("%s returned ok envelope for missing args: %s", tc.name, stdout)
			}
		})
	}
}

func TestContract_SetupDestinationTargetSemantics(t *testing.T) {
	for _, op := range []string{"init", "import", "clone-current", "clone-full"} {
		t.Run(op+"_relative_multi_level", func(t *testing.T) {
			base := t.TempDir()
			cwd := filepath.Join(base, "cwd")
			if err := os.Mkdir(cwd, 0755); err != nil {
				t.Fatal(err)
			}
			destArg := filepath.Join("rel", "multi", "repo")
			wantRoot := filepath.Join(cwd, "rel", "multi", "repo")

			stdout, stderr, code := runContractSetupOperation(t, cwd, op, base, destArg)
			requireContractSetupSuccess(t, stdout, stderr, code, op)
			data := decodeContractSmokeDataMap(t, stdout)
			if data["repo_root"] != wantRoot {
				t.Fatalf("%s repo_root mismatch: %#v want %s\n%s", op, data["repo_root"], wantRoot, stdout)
			}
			if !fileExists(t, filepath.Join(wantRoot, ".jvs", "repo_id")) {
				t.Fatalf("%s did not create repository at explicit destination", op)
			}
		})

		t.Run(op+"_existing_empty_dir", func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "empty", "repo")
			if err := os.MkdirAll(dest, 0755); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, code := runContractSetupOperation(t, base, op, base, dest)
			requireContractSetupSuccess(t, stdout, stderr, code, op)
			data := decodeContractSmokeDataMap(t, stdout)
			if data["repo_root"] != dest {
				t.Fatalf("%s existing empty dir repo_root mismatch: %#v\n%s", op, data["repo_root"], stdout)
			}
		})

		t.Run(op+"_reject_file_dest", func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "file-dest")
			if err := os.WriteFile(dest, []byte("user data"), 0644); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, code := runContractSetupOperation(t, base, op, base, dest)
			requireContractSetupFailure(t, stdout, stderr, code, op)
			if !fileExists(t, dest) {
				t.Fatalf("%s removed user file destination", op)
			}
		})

		t.Run(op+"_reject_non_empty_dest", func(t *testing.T) {
			base := t.TempDir()
			dest := filepath.Join(base, "non-empty")
			if err := os.MkdirAll(dest, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, code := runContractSetupOperation(t, base, op, base, dest)
			requireContractSetupFailure(t, stdout, stderr, code, op)
			if !fileExists(t, filepath.Join(dest, "keep.txt")) {
				t.Fatalf("%s removed user non-empty destination", op)
			}
			if _, err := os.Stat(filepath.Join(dest, ".jvs")); !os.IsNotExist(err) {
				t.Fatalf("%s created metadata in rejected non-empty destination: %v", op, err)
			}
		})

		t.Run(op+"_reject_parent_component_file", func(t *testing.T) {
			base := t.TempDir()
			parentFile := filepath.Join(base, "not-a-directory")
			if err := os.WriteFile(parentFile, []byte("user data"), 0644); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(parentFile, "child", "repo")

			stdout, stderr, code := runContractSetupOperation(t, base, op, base, dest)
			requireContractSetupFailure(t, stdout, stderr, code, op)
			if !fileExists(t, parentFile) {
				t.Fatalf("%s removed parent component file", op)
			}
		})
	}

	t.Run("import_reject_overlap", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "source")
		dest := filepath.Join(source, "precreated-empty")
		if err := os.MkdirAll(dest, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("keep"), 0644); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, code := runJVS(t, base, "--json", "import", source, dest)
		requireContractSetupFailure(t, stdout, stderr, code, "import overlap")
		if !fileExists(t, filepath.Join(source, "keep.txt")) || !fileExists(t, dest) {
			t.Fatalf("import overlap removed user source or precreated destination")
		}
	})

	for _, scope := range []string{"current", "full"} {
		t.Run("clone_"+scope+"_reject_overlap", func(t *testing.T) {
			base := t.TempDir()
			source := createContractSetupSourceRepo(t, base, "source")
			dest := filepath.Join(source, "main", "precreated-empty")
			if err := os.MkdirAll(dest, 0755); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, code := runJVS(t, base, "--json", "clone", source, dest, "--scope", scope)
			requireContractSetupFailure(t, stdout, stderr, code, "clone overlap")
			if !fileExists(t, dest) {
				t.Fatalf("clone %s overlap removed precreated destination", scope)
			}
			if _, err := os.Stat(filepath.Join(dest, ".jvs")); !os.IsNotExist(err) {
				t.Fatalf("clone %s overlap created metadata in rejected destination: %v", scope, err)
			}
		})
	}
}

func TestContract_CapabilityMissingNestedTargetReportsProbeParent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "a", "b")

	stdout, stderr, code := runJVS(t, dir, "--json", "--repo", filepath.Join(dir, "unused"), "--workspace", "missing", "capability", target, "--write-probe")
	requireContractSetupSuccess(t, stdout, stderr, code, "capability")

	data := decodeContractSmokeDataMap(t, stdout)
	if data["target_path"] != target || data["probe_path"] != dir {
		t.Fatalf("capability missing target probe mismatch: %#v\n%s", data, stdout)
	}
	warnings, ok := data["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("capability missing target must include warnings: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("capability created missing target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".jvs")); !os.IsNotExist(err) {
		t.Fatalf("capability created repository metadata: %v", err)
	}
}

func TestContract_CloneScopesRequireRepoRootSource(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	for _, scope := range []string{"current", "full"} {
		t.Run(scope, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "clone-dest")
			stdout, stderr, code := runJVS(t, t.TempDir(), "--json", "clone", filepath.Join(repoPath, "main"), dest, "--scope", scope)
			requireContractSetupFailure(t, stdout, stderr, code, "clone "+scope)
			env := decodeContractSmokeEnvelope(t, stdout)
			errData, ok := env.Error.(map[string]any)
			if !ok {
				t.Fatalf("clone %s error was not an object: %#v\n%s", scope, env.Error, stdout)
			}
			message, _ := errData["message"].(string)
			hint, _ := errData["hint"].(string)
			if !strings.Contains(message, "repository root") || !strings.Contains(message+hint, "source-workspace") {
				t.Fatalf("clone %s source path error lacked guidance: %#v\n%s", scope, errData, stdout)
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Fatalf("clone %s created destination for workspace source: %v", scope, err)
			}
		})
	}
}

func runContractSetupOperation(t *testing.T, cwd, op, base, destArg string) (stdout, stderr string, code int) {
	t.Helper()

	switch op {
	case "init":
		return runJVS(t, cwd, "--json", "init", destArg)
	case "import":
		source := filepath.Join(base, "source-import")
		if err := os.MkdirAll(source, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("import"), 0644); err != nil {
			t.Fatal(err)
		}
		return runJVS(t, cwd, "--json", "import", source, destArg)
	case "clone-current":
		source := createContractSetupSourceRepo(t, base, "source-clone-current")
		return runJVS(t, cwd, "--json", "clone", source, destArg, "--scope", "current")
	case "clone-full":
		source := createContractSetupSourceRepo(t, base, "source-clone-full")
		return runJVS(t, cwd, "--json", "clone", source, destArg, "--scope", "full")
	default:
		t.Fatalf("unknown setup operation %q", op)
		return "", "", 1
	}
}

func createContractSetupSourceRepo(t *testing.T, base, name string) string {
	t.Helper()

	source := filepath.Join(base, name)
	if stdout, stderr, code := runJVS(t, base, "init", source); code != 0 {
		t.Fatalf("init %s source failed: stdout=%s stderr=%s", name, stdout, stderr)
	}
	if err := os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("clone"), 0644); err != nil {
		t.Fatal(err)
	}
	return source
}

func requireContractSetupSuccess(t *testing.T, stdout, stderr string, code int, command string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("%s failed: stdout=%s stderr=%s", command, stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("%s --json wrote stderr: %q", command, stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if !env.OK {
		t.Fatalf("%s returned non-ok envelope: %s", command, stdout)
	}
}

func requireContractSetupFailure(t *testing.T, stdout, stderr string, code int, command string) {
	t.Helper()
	if code == 0 {
		t.Fatalf("%s unexpectedly succeeded: stdout=%s stderr=%s", command, stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("%s JSON error wrote stderr: %q", command, stderr)
	}
	env := decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("%s returned ok envelope for failure: %s", command, stdout)
	}
}
