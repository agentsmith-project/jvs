//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
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
	requireSeparatedStoryTransferSource(t, firstOut, payloadRoot)

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
	requireSeparatedControlJSONError(t, stdout, stderr, code, "E_SEPARATED_LIFECYCLE_UNSUPPORTED")
	requireAbsolutePathMissing(t, newWorkspaceTarget)

	view := separatedJSON(t, base, controlRoot, "main", "view", first)
	requireSeparatedControlAuthoritativeData(t, view, controlRoot, payloadRoot, "main")
	viewPath, _ := view["view_path"].(string)
	if viewPath == "" {
		t.Fatalf("view missing view_path: %#v", view)
	}
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
	run := separatedJSON(t, base, controlRoot, "main", "restore", "--run", planID)
	requireSeparatedControlAuthoritativeData(t, run, controlRoot, payloadRoot, "main")
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
	return requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, separatedPayloadRootFromControlOutput(t, stdout), workspace)
}

func separatedPayloadRootFromControlOutput(t *testing.T, stdout string) string {
	t.Helper()
	data := decodeContractDataMap(t, stdout)
	payloadRoot, _ := data["payload_root"].(string)
	if payloadRoot == "" {
		t.Fatalf("separated JSON missing payload_root: %#v", data)
	}
	return payloadRoot
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
	if err := os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-plan.json"), []byte("{}\n"), 0644); err != nil {
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
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-plan.json")); got != "{}\n" {
		t.Fatalf("restore plan sentinel mutated: %q", got)
	}
	if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp")); got != "runtime sentinel\n" {
		t.Fatalf("runtime sentinel mutated: %q", got)
	}
}

func requireSeparatedStoryTransferSource(t *testing.T, data map[string]any, payloadRoot string) {
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
}
