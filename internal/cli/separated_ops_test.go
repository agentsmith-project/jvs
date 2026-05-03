package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedControlOpsJSONFieldsAndPayloadBoundary(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	seedSeparatedControlMetadataSentinels(t, controlRoot)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "payload only",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	assertSeparatedControlOpsData(t, saveData, controlRoot, payloadRoot, "main")
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)
	assertSeparatedTransferSource(t, saveData, payloadRoot)

	historyOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"history",
	)
	require.NoError(t, err, historyOut)
	_, historyData := decodeSeparatedControlDataMap(t, historyOut)
	assertSeparatedControlOpsData(t, historyData, controlRoot, payloadRoot, "main")
	require.Equal(t, []string{savePointID}, historySavePointIDsForSeparatedOpsTest(t, historyData, "save_points"))

	viewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"view", savePointID,
	)
	require.NoError(t, err, viewOut)
	_, viewData := decodeSeparatedControlDataMap(t, viewOut)
	assertSeparatedControlOpsData(t, viewData, controlRoot, payloadRoot, "main")
	viewPath, _ := viewData["view_path"].(string)
	require.NotEmpty(t, viewPath)
	assert.FileExists(t, filepath.Join(viewPath, "app.txt"))
	assert.NoFileExists(t, filepath.Join(viewPath, "audit"))
	assert.NoFileExists(t, filepath.Join(viewPath, "locks"))
	assert.NoFileExists(t, filepath.Join(viewPath, "restore-plans"))
	assert.NoFileExists(t, filepath.Join(viewPath, "runtime"))
	_, err = executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"view", "close", viewData["view_id"].(string),
	)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v2\n"), 0644))
	_, err = executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "payload v2",
	)
	require.NoError(t, err)

	restorePreviewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", savePointID,
	)
	require.NoError(t, err, restorePreviewOut)
	_, restorePreview := decodeSeparatedControlDataMap(t, restorePreviewOut)
	assertSeparatedControlOpsData(t, restorePreview, controlRoot, payloadRoot, "main")
	planID, _ := restorePreview["plan_id"].(string)
	require.NotEmpty(t, planID)

	restoreRunOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", "--run", planID,
	)
	require.NoError(t, err, restoreRunOut)
	_, restoreRun := decodeSeparatedControlDataMap(t, restoreRunOut)
	assertSeparatedControlOpsData(t, restoreRun, controlRoot, payloadRoot, "main")
	assert.Equal(t, savePointID, restoreRun["restored_save_point"])
	assert.Equal(t, "v1\n", separatedOpsReadFile(t, filepath.Join(payloadRoot, "app.txt")))
	assertSeparatedControlSentinelsIntact(t, controlRoot)

	recoveryOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)
	require.NoError(t, err, recoveryOut)
	_, recoveryData := decodeSeparatedControlDataMap(t, recoveryOut)
	assertSeparatedControlOpsData(t, recoveryData, controlRoot, payloadRoot, "main")

	cleanupPreviewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "preview",
	)
	require.NoError(t, err, cleanupPreviewOut)
	_, cleanupPreview := decodeSeparatedControlDataMap(t, cleanupPreviewOut)
	assertSeparatedControlOpsData(t, cleanupPreview, controlRoot, payloadRoot, "main")
	cleanupPlanID, _ := cleanupPreview["plan_id"].(string)
	require.NotEmpty(t, cleanupPlanID)

	cleanupRunOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "run", "--plan-id", cleanupPlanID,
	)
	require.NoError(t, err, cleanupRunOut)
	_, cleanupRun := decodeSeparatedControlDataMap(t, cleanupRunOut)
	assertSeparatedControlOpsData(t, cleanupRun, controlRoot, payloadRoot, "main")
	assert.Equal(t, "completed", cleanupRun["status"])
	assertSeparatedControlRootsIntact(t, controlRoot, payloadRoot)
}

func TestSeparatedControlOpsPayloadLocatorPresentFailsClosedBeforeMutation(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))
	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "before locator",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID := saveData["save_point_id"].(string)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, repo.JVSDirName), []byte("untrusted\n"), 0644))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "save", args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "save", "-m", "blocked"}},
		{name: "restore preview", args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "restore", savePointID}},
		{name: "cleanup preview", args: []string{"--json", "--control-root", controlRoot, "--workspace", "main", "cleanup", "preview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args...)
			require.Equal(t, 1, exitCode, "%s unexpectedly succeeded: stdout=%s stderr=%s", tc.name, stdout, stderr)
			require.Empty(t, strings.TrimSpace(stderr))
			env := decodeContractEnvelope(t, stdout)
			require.False(t, env.OK, stdout)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_PAYLOAD_LOCATOR_PRESENT", env.Error.Code)
			assert.JSONEq(t, `null`, string(env.Data))
		})
	}
}

func TestSeparatedControlSaveSymlinkEscapeFailsClosedWithStableCode(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	seedSeparatedControlMetadataSentinels(t, controlRoot)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "blocked symlink",
	)
	require.Equal(t, 1, exitCode, "save unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.False(t, env.OK, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_PATH_BOUNDARY_ESCAPE", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestSeparatedControlViewSymlinkEscapeFailsBeforeControlMutation(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	seedSeparatedControlMetadataSentinels(t, controlRoot)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "before symlink",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	pinsBefore := separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "gc", "pins"))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"view", savePointID,
	)
	require.Equal(t, 1, exitCode, "view unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.False(t, env.OK, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_PATH_BOUNDARY_ESCAPE", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
	assert.NoDirExists(t, filepath.Join(controlRoot, ".jvs", "views"), "blocked view must not create control runtime")
	assert.Equal(t, pinsBefore, separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "gc", "pins")))
}

func TestSeparatedControlViewRejectsSymlinkedViewsRuntimeRoot(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "before views symlink",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)

	externalViewsRoot := filepath.Join(base, "external-views")
	require.NoError(t, os.MkdirAll(externalViewsRoot, 0755))
	viewsRoot := filepath.Join(controlRoot, ".jvs", "views")
	require.NoError(t, os.RemoveAll(viewsRoot))
	if err := os.Symlink(externalViewsRoot, viewsRoot); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"view", savePointID,
	)
	require.Equal(t, 1, exitCode, "view unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.False(t, env.OK, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_PATH_BOUNDARY_ESCAPE", env.Error.Code)
	assert.JSONEq(t, `null`, string(env.Data))
	assert.Empty(t, separatedOpsReadDirNames(t, externalViewsRoot), "blocked view must not create directories outside the control root")
}

func assertSeparatedControlOpsData(t *testing.T, data map[string]any, controlRoot, payloadRoot, workspace string) {
	t.Helper()

	assertSeparatedControlAuthoritativeData(t, data, controlRoot, payloadRoot, workspace)
	assert.Equal(t, "not_run", data["doctor_strict"])
}

func seedSeparatedControlMetadataSentinels(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"audit", "locks", "restore-plans", "runtime"} {
		require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), []byte("audit sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), []byte("lock sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-plan.json"), []byte("{}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644))
}

func assertSeparatedControlSentinelsIntact(t *testing.T, controlRoot string) {
	t.Helper()
	assert.Equal(t, "audit sentinel\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "audit", "platform.log")))
	assert.Equal(t, "lock sentinel\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "locks", "platform.lock")))
	assert.Equal(t, "{}\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-plan.json")))
	assert.Equal(t, "runtime sentinel\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp")))
}

func assertSeparatedControlRootsIntact(t *testing.T, controlRoot, payloadRoot string) {
	t.Helper()
	assert.DirExists(t, controlRoot)
	assert.DirExists(t, payloadRoot)
	assert.DirExists(t, filepath.Join(controlRoot, ".jvs", "audit"))
	assert.DirExists(t, filepath.Join(controlRoot, ".jvs", "locks"))
	assert.DirExists(t, filepath.Join(controlRoot, ".jvs", "restore-plans"))
	assert.DirExists(t, filepath.Join(controlRoot, ".jvs", "runtime"))
	assertSeparatedControlSentinelsIntact(t, controlRoot)
}

func assertSeparatedTransferSource(t *testing.T, data map[string]any, payloadRoot string) {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.NotEmpty(t, transfers)
	primary, ok := transfers[0].(map[string]any)
	require.True(t, ok, "transfer should be an object: %#v", transfers[0])
	assert.Equal(t, payloadRoot, primary["source_path"])
}

func historySavePointIDsForSeparatedOpsTest(t *testing.T, data map[string]any, key string) []string {
	t.Helper()
	raw, ok := data[key].([]any)
	require.True(t, ok, "%s should be an array: %#v", key, data[key])
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		require.True(t, ok, "save point record should be an object: %#v", item)
		id, _ := record["save_point_id"].(string)
		require.NotEmpty(t, id, "save point record missing id: %#v", record)
		ids = append(ids, id)
	}
	return ids
}

func separatedOpsReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func separatedOpsReadDirNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func decodeSeparatedOpsEnvelopeData(t *testing.T, stdout string) (contractEnvelope, map[string]any) {
	t.Helper()
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return env, data
}
