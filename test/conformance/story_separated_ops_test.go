//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStorySeparatedOpsPayloadBoundaryAndAuthoritativeJSON(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")
	seedSeparatedStoryControlFiles(t, controlRoot)
	createFiles(t, payloadRoot, map[string]string{"app.txt": "v1\n"})

	firstOut := separatedJSON(t, base, controlRoot, "main", "save", "-m", "payload only")
	first := firstOut["save_point_id"].(string)
	requireSeparatedControlAuthoritativeData(t, firstOut, controlRoot, payloadRoot, "main")
	requireSeparatedStoryTransferSource(t, firstOut, controlRoot, payloadRoot)

	history := separatedJSON(t, base, controlRoot, "main", "history")
	requireSeparatedControlAuthoritativeData(t, history, controlRoot, payloadRoot, "main")
	if got := savePointIDsFromHistory(t, history); !sameStringSlice(got, []string{first}) {
		t.Fatalf("separated history IDs = %v, want [%s]", got, first)
	}

	workspaceList := separatedJSON(t, base, controlRoot, "main", "workspace", "list")
	requireSeparatedControlAuthoritativeData(t, workspaceList, controlRoot, payloadRoot, "main")
	workspaces, ok := workspaceList["workspaces"].([]any)
	if !ok || len(workspaces) != 1 {
		t.Fatalf("separated workspace list should expose one workspace: %#v", workspaceList)
	}
	workspacePath := separatedJSON(t, base, controlRoot, "main", "workspace", "path", "main")
	requireSeparatedControlAuthoritativeData(t, workspacePath, controlRoot, payloadRoot, "main")
	if workspacePath["path"] != payloadRoot {
		t.Fatalf("separated workspace path = %#v, want %q in %#v", workspacePath["path"], payloadRoot, workspacePath)
	}

	newWorkspaceTarget := filepath.Join(base, "feature-payload")
	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"workspace", "new", newWorkspaceTarget, "--from", first, "--name", "feature",
	)
	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXTERNAL_LIFECYCLE_UNSUPPORTED")
	requireAbsolutePathMissing(t, newWorkspaceTarget)

	view := separatedJSON(t, base, controlRoot, "main", "view", first)
	requireSeparatedControlAuthoritativeData(t, view, controlRoot, payloadRoot, "main")
	viewPath, _ := view["view_path"].(string)
	if viewPath == "" {
		t.Fatalf("view missing view_path: %#v", view)
	}
	requireSeparatedStoryViewTransfer(t, view, controlRoot, viewPath)
	requireAbsolutePathExists(t, filepath.Join(viewPath, "app.txt"))
	requireAbsolutePathMissing(t, filepath.Join(viewPath, "audit"))
	requireAbsolutePathMissing(t, filepath.Join(viewPath, "locks"))
	requireAbsolutePathMissing(t, filepath.Join(viewPath, "restore-plans"))
	requireAbsolutePathMissing(t, filepath.Join(viewPath, "runtime"))
	separatedJSON(t, base, controlRoot, "main", "view", "close", view["view_id"].(string))

	createFiles(t, payloadRoot, map[string]string{"app.txt": "v2\n"})
	separatedJSON(t, base, controlRoot, "main", "save", "-m", "payload v2")

	preview := separatedJSON(t, base, controlRoot, "main", "restore", first)
	requireSeparatedControlAuthoritativeData(t, preview, controlRoot, payloadRoot, "main")
	planID, _ := preview["plan_id"].(string)
	if planID == "" {
		t.Fatalf("restore preview missing plan_id: %#v", preview)
	}
	requireSeparatedStoryRestorePreviewTransfer(t, preview, controlRoot, payloadRoot)
	run := separatedJSON(t, base, controlRoot, "main", "restore", "--run", planID)
	requireSeparatedControlAuthoritativeData(t, run, controlRoot, payloadRoot, "main")
	requireSeparatedStoryRestoreRunTransfers(t, run, controlRoot, payloadRoot)
	if got := readAbsoluteFile(t, filepath.Join(payloadRoot, "app.txt")); got != "v1\n" {
		t.Fatalf("restore should update payload only, app.txt=%q", got)
	}
	requireSeparatedStoryControlFiles(t, controlRoot)

	recovery := separatedJSON(t, base, controlRoot, "main", "recovery", "status")
	requireSeparatedControlAuthoritativeData(t, recovery, controlRoot, payloadRoot, "main")

	cleanupPreview := separatedJSON(t, base, controlRoot, "main", "cleanup", "preview")
	requireSeparatedControlAuthoritativeData(t, cleanupPreview, controlRoot, payloadRoot, "main")
	cleanupPlanID, _ := cleanupPreview["plan_id"].(string)
	if cleanupPlanID == "" {
		t.Fatalf("cleanup preview missing plan_id: %#v", cleanupPreview)
	}
	cleanupRun := separatedJSON(t, base, controlRoot, "main", "cleanup", "run", "--plan-id", cleanupPlanID)
	requireSeparatedControlAuthoritativeData(t, cleanupRun, controlRoot, payloadRoot, "main")
	requireAbsolutePathExists(t, controlRoot)
	requireAbsolutePathExists(t, payloadRoot)
	requireSeparatedStoryControlFiles(t, controlRoot)
}

func TestStorySeparatedOpsLifecycleCommandsFailClosedWithoutMutatingRoots(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         func(base, controlRoot, payloadRoot, savePointID string) []string
		missingPaths func(base, controlRoot, payloadRoot string) []string
	}{
		{
			name: "repo move preview",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "move", filepath.Join(base, "control-moved"))
			},
			missingPaths: func(base, controlRoot, payloadRoot string) []string {
				return []string{filepath.Join(base, "control-moved")}
			},
		},
		{
			name: "repo move run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "move", "--run", "missing-separated-plan")
			},
		},
		{
			name: "repo rename preview",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "rename", "control-renamed")
			},
			missingPaths: func(base, controlRoot, payloadRoot string) []string {
				return []string{filepath.Join(base, "control-renamed")}
			},
		},
		{
			name: "repo rename run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "rename", "--run", "missing-separated-plan")
			},
		},
		{
			name: "repo detach preview",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "detach")
			},
		},
		{
			name: "repo detach run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "repo", "detach", "--run", "missing-separated-plan")
			},
		},
		{
			name: "workspace new",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "new", filepath.Join(base, "feature-payload"), "--from", savePointID, "--name", "feature")
			},
			missingPaths: func(base, controlRoot, payloadRoot string) []string {
				return []string{filepath.Join(base, "feature-payload")}
			},
		},
		{
			name: "workspace move preview",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "move", "main", filepath.Join(base, "payload-moved"))
			},
			missingPaths: func(base, controlRoot, payloadRoot string) []string {
				return []string{filepath.Join(base, "payload-moved")}
			},
		},
		{
			name: "workspace move run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "move", "--run", "missing-separated-plan")
			},
		},
		{
			name: "workspace rename",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "rename", "main", "main-renamed")
			},
		},
		{
			name: "workspace rename dry run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "rename", "--dry-run", "main", "main-renamed")
			},
		},
		{
			name: "workspace delete preview",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "delete", "main")
			},
		},
		{
			name: "workspace delete run",
			args: func(base, controlRoot, payloadRoot, savePointID string) []string {
				return separatedLifecycleStoryArgs(controlRoot, "workspace", "delete", "--run", "missing-separated-plan")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, controlRoot, payloadRoot, savePointID := setupSeparatedLifecycleStoryRepo(t)
			before := captureSeparatedLifecycleStoryRoots(t, controlRoot, payloadRoot)

			stdout, stderr, code := runJVS(t, base, tc.args(base, controlRoot, payloadRoot, savePointID)...)

			env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_EXTERNAL_LIFECYCLE_UNSUPPORTED")
			if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
				t.Fatalf("unsupported lifecycle repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
			}
			if env.Workspace == nil || *env.Workspace != "main" {
				t.Fatalf("unsupported lifecycle workspace = %#v, want main\n%s", env.Workspace, stdout)
			}
			if !strings.Contains(env.Error.Message, "No files changed") {
				t.Fatalf("unsupported lifecycle message should promise no mutation: %#v", env.Error)
			}

			requireSeparatedLifecycleStoryRootsUnchanged(t, before, controlRoot, payloadRoot)
			requireSeparatedStoryControlFiles(t, controlRoot)
			if got := readAbsoluteFile(t, filepath.Join(payloadRoot, "app.txt")); got != "lifecycle sentinel\n" {
				t.Fatalf("payload sentinel mutated: %q", got)
			}
			if tc.missingPaths != nil {
				for _, path := range tc.missingPaths(base, controlRoot, payloadRoot) {
					requireAbsolutePathAbsent(t, path)
				}
			}
		})
	}
}

func TestStorySeparatedOpsSymlinkEscapeFailsClosed(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")
	seedSeparatedStoryControlFiles(t, controlRoot)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "blocked symlink",
	)
	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_PATH_BOUNDARY_ESCAPE")
}

func TestStorySeparatedOpsStatusSymlinkEscapeFailsClosed(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")
	seedSeparatedStoryControlFiles(t, controlRoot)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"status",
	)
	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_PATH_BOUNDARY_ESCAPE")
}

func TestStorySeparatedOpsViewSymlinkEscapeFailsBeforeRuntimeMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")
	seedSeparatedStoryControlFiles(t, controlRoot)
	createFiles(t, payloadRoot, map[string]string{"app.txt": "v1\n"})

	firstOut := separatedJSON(t, base, controlRoot, "main", "save", "-m", "before symlink")
	first := firstOut["save_point_id"].(string)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	requireDirectoryEntries(t, filepath.Join(controlRoot, ".jvs", "gc", "pins"), []string{})

	stdout, stderr, code := runJVS(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"view", first,
	)
	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_PATH_BOUNDARY_ESCAPE")
	requireAbsolutePathMissing(t, filepath.Join(controlRoot, ".jvs", "views"))
	requireDirectoryEntries(t, filepath.Join(controlRoot, ".jvs", "gc", "pins"), []string{})
}

func separatedJSON(t *testing.T, cwd, controlRoot, workspace string, args ...string) map[string]any {
	t.Helper()
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", workspace}
	fullArgs = append(fullArgs, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs %v failed\nstdout=%s\nstderr=%s", fullArgs, stdout, stderr)
	}
	return requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, separatedWorkspaceFolderFromControlOutput(t, stdout), workspace)
}

func separatedDoctorJSON(t *testing.T, cwd, controlRoot, workspaceFolder, workspace string, args ...string) map[string]any {
	t.Helper()
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", workspace, "doctor"}
	fullArgs = append(fullArgs, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs %v failed\nstdout=%s\nstderr=%s", fullArgs, stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
		t.Fatalf("separated doctor JSON repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
	}
	data := decodeContractDataMap(t, stdout)
	requireSeparatedControlDoctorData(t, data, controlRoot, workspaceFolder, workspace)
	return data
}

func setupSeparatedLifecycleStoryRepo(t *testing.T) (base, controlRoot, payloadRoot, savePointID string) {
	t.Helper()
	base = t.TempDir()
	controlRoot = filepath.Join(base, "control")
	payloadRoot = filepath.Join(base, "payload")
	initSeparatedControlRepo(t, base, controlRoot, payloadRoot, "main")
	seedSeparatedStoryControlFiles(t, controlRoot)
	createFiles(t, payloadRoot, map[string]string{"app.txt": "lifecycle sentinel\n"})
	saveOut := separatedJSON(t, base, controlRoot, "main", "save", "-m", "lifecycle unsupported source")
	savePointID, _ = saveOut["save_point_id"].(string)
	if savePointID == "" {
		t.Fatalf("separated lifecycle setup missing save point ID: %#v", saveOut)
	}
	requireSeparatedStoryControlFiles(t, controlRoot)
	return base, controlRoot, payloadRoot, savePointID
}

func separatedLifecycleStoryArgs(controlRoot string, args ...string) []string {
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
	return append(fullArgs, args...)
}

type separatedLifecycleStoryRootSnapshot map[string]separatedLifecycleStoryNode

type separatedLifecycleStoryNode struct {
	Mode       os.FileMode
	Content    string
	LinkTarget string
}

func captureSeparatedLifecycleStoryRoots(t *testing.T, controlRoot, payloadRoot string) map[string]separatedLifecycleStoryRootSnapshot {
	t.Helper()
	return map[string]separatedLifecycleStoryRootSnapshot{
		"control": captureSeparatedLifecycleStoryRoot(t, controlRoot),
		"payload": captureSeparatedLifecycleStoryRoot(t, payloadRoot),
	}
}

func captureSeparatedLifecycleStoryRoot(t *testing.T, root string) separatedLifecycleStoryRootSnapshot {
	t.Helper()
	snapshot := separatedLifecycleStoryRootSnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		node := separatedLifecycleStoryNode{Mode: info.Mode()}
		switch {
		case info.Mode().IsRegular():
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			node.Content = string(content)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			node.LinkTarget = target
		}
		snapshot[rel] = node
		return nil
	}); err != nil {
		t.Fatalf("capture separated lifecycle root %s: %v", root, err)
	}
	return snapshot
}

func requireSeparatedLifecycleStoryRootsUnchanged(t *testing.T, before map[string]separatedLifecycleStoryRootSnapshot, controlRoot, payloadRoot string) {
	t.Helper()
	after := captureSeparatedLifecycleStoryRoots(t, controlRoot, payloadRoot)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("separated lifecycle command mutated control/payload roots\nbefore=%#v\nafter=%#v", before, after)
	}
}

func separatedWorkspaceFolderFromControlOutput(t *testing.T, stdout string) string {
	t.Helper()
	data := decodeContractDataMap(t, stdout)
	folder, _ := data["folder"].(string)
	if folder == "" {
		t.Fatalf("external control root JSON missing folder: %#v", data)
	}
	return folder
}

func seedSeparatedStoryControlFiles(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"audit", "locks", "restore-plans", "runtime"} {
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
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-state.tmp"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("write restore plan sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644); err != nil {
		t.Fatalf("write runtime sentinel: %v", err)
	}
}

func requireSeparatedStoryControlFiles(t *testing.T, controlRoot string) {
	t.Helper()
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "audit", "platform.log")); got != "audit sentinel\n" {
		t.Fatalf("audit sentinel mutated: %q", got)
	}
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "locks", "platform.lock")); got != "lock sentinel\n" {
		t.Fatalf("lock sentinel mutated: %q", got)
	}
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-state.tmp")); got != "{}\n" {
		t.Fatalf("restore plan sentinel mutated: %q", got)
	}
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp")); got != "runtime sentinel\n" {
		t.Fatalf("runtime sentinel mutated: %q", got)
	}
}

func requireSeparatedStoryTransferSource(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	if !ok || len(transfers) == 0 {
		t.Fatalf("separated save should report transfer: %#v", data["transfers"])
	}
	transfer, ok := transfers[0].(map[string]any)
	if !ok {
		t.Fatalf("transfer should be object: %#v", transfers[0])
	}
	if transfer["source_path"] != payloadRoot {
		t.Fatalf("transfer source_path = %#v, want payload root %q in %#v", transfer["source_path"], payloadRoot, transfer)
	}
	requireSeparatedStoryPathUnder(t, transfer["published_destination"], filepath.Join(controlRoot, ".jvs", "snapshots"))
}

func requireSeparatedStoryViewTransfer(t *testing.T, data map[string]any, controlRoot, viewPath string) {
	t.Helper()
	transfer := requireSeparatedStoryTransferByID(t, data, "view-primary")
	if transfer["operation"] != "view" || transfer["phase"] != "view_materialization" {
		t.Fatalf("view transfer has wrong operation/phase: %#v", transfer)
	}
	requireSeparatedStoryPathUnder(t, transfer["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	requireSeparatedStoryPathUnder(t, transfer["materialization_destination"], filepath.Join(controlRoot, ".jvs", "views"))
	requireSeparatedStoryPathUnder(t, transfer["capability_probe_path"], filepath.Join(controlRoot, ".jvs", "views"))
	if transfer["published_destination"] != viewPath {
		t.Fatalf("view published_destination = %#v, want %q in %#v", transfer["published_destination"], viewPath, transfer)
	}
	requireSeparatedStoryTransferCopyEvidence(t, transfer)
}

func requireSeparatedStoryRestorePreviewTransfer(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()
	transfer := requireSeparatedStoryTransferByID(t, data, "restore-preview-validation-primary")
	if transfer["result_kind"] != "expected" || transfer["permission_scope"] != "preview_only" {
		t.Fatalf("restore preview transfer must be preview-only expected: %#v", transfer)
	}
	requireSeparatedStoryPathUnder(t, transfer["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	requireSeparatedStoryPathUnder(t, transfer["materialization_destination"], filepath.Join(controlRoot, ".jvs"))
	if transfer["published_destination"] != payloadRoot {
		t.Fatalf("restore preview published_destination = %#v, want %q in %#v", transfer["published_destination"], payloadRoot, transfer)
	}
	requireSeparatedStoryTransferCopyEvidence(t, transfer)
}

func requireSeparatedStoryRestoreRunTransfers(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()
	validation := requireSeparatedStoryTransferByID(t, data, "restore-run-source-validation")
	if validation["result_kind"] != "final" || validation["permission_scope"] != "execution" {
		t.Fatalf("restore validation transfer must be final execution: %#v", validation)
	}
	requireSeparatedStoryPathUnder(t, validation["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	requireSeparatedStoryPathUnder(t, validation["materialization_destination"], filepath.Join(controlRoot, ".jvs"))
	if validation["published_destination"] != payloadRoot {
		t.Fatalf("restore validation published_destination = %#v, want %q in %#v", validation["published_destination"], payloadRoot, validation)
	}
	requireSeparatedStoryTransferCopyEvidence(t, validation)

	primary := requireSeparatedStoryTransferByID(t, data, "restore-run-primary")
	requireSeparatedStoryPathUnder(t, primary["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	requireSeparatedStoryPathUnder(t, primary["materialization_destination"], filepath.Dir(payloadRoot))
	if path, _ := primary["materialization_destination"].(string); strings.Contains(filepath.ToSlash(path), filepath.ToSlash(controlRoot)) {
		t.Fatalf("restore primary materialized inside control root: %#v", primary)
	}
	if primary["published_destination"] != payloadRoot {
		t.Fatalf("restore primary published_destination = %#v, want %q in %#v", primary["published_destination"], payloadRoot, primary)
	}
	requireSeparatedStoryTransferCopyEvidence(t, primary)
}

func requireSeparatedStoryTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	if !ok {
		t.Fatalf("transfers should be array: %#v", data["transfers"])
	}
	for _, item := range transfers {
		transfer, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("transfer should be object: %#v", item)
		}
		if transfer["transfer_id"] == id {
			return transfer
		}
	}
	t.Fatalf("missing transfer %q in %#v", id, transfers)
	return nil
}

func requireSeparatedStoryTransferCopyEvidence(t *testing.T, transfer map[string]any) {
	t.Helper()
	if transfer["checked_for_this_operation"] != true {
		t.Fatalf("transfer was not checked for this operation: %#v", transfer)
	}
	if transfer["capability_probe_path"] == "" {
		t.Fatalf("transfer missing capability_probe_path: %#v", transfer)
	}
	if transfer["performance_class"] != "fast_copy" && transfer["performance_class"] != "normal_copy" {
		t.Fatalf("transfer performance_class = %#v, want fast_copy or normal_copy in %#v", transfer["performance_class"], transfer)
	}
}

func requireSeparatedStoryPathUnder(t *testing.T, got any, root string) {
	t.Helper()
	path, ok := got.(string)
	if !ok || path == "" {
		t.Fatalf("path should be non-empty string under %s: %#v", root, got)
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		t.Fatalf("path %q should be under %q (rel=%q err=%v)", path, root, rel, err)
	}
}
