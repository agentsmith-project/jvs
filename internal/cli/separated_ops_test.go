package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/pkg/errclass"
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
	assertSeparatedSaveTransfer(t, saveData, controlRoot, payloadRoot)

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
	assertSeparatedViewTransfer(t, viewData, controlRoot, viewPath)
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
	assertSeparatedRestorePreviewTransfer(t, restorePreview, controlRoot, payloadRoot)

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
	assertSeparatedRestoreRunTransfers(t, restoreRun, controlRoot, payloadRoot)
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

func TestSeparatedControlSaveAndCleanupBlockActiveRecoveryPlan(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "before recovery",
	)
	require.NoError(t, err, saveOut)

	cleanupPlanOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "preview",
	)
	require.NoError(t, err, cleanupPlanOut)
	_, cleanupPlan := decodeSeparatedControlDataMap(t, cleanupPlanOut)
	cleanupPlanID, _ := cleanupPlan["plan_id"].(string)
	require.NotEmpty(t, cleanupPlanID)

	seedSeparatedControlRecoveryPlanFixture(t, base, controlRoot, payloadRoot)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "save", args: []string{"save", "-m", "blocked by recovery"}},
		{name: "cleanup preview", args: []string{"cleanup", "preview"}},
		{name: "cleanup run", args: []string{"cleanup", "run", "--plan-id", cleanupPlanID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
			assert.Contains(t, env.Error.Message, "recovery plan")
		})
	}
}

func TestSeparatedControlSaveBlocksPendingRestorePlan(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "restore source",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)

	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v2\n"), 0644))
	secondOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "current payload",
	)
	require.NoError(t, err, secondOut)

	restorePreviewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", savePointID,
	)
	require.NoError(t, err, restorePreviewOut)
	_, restorePreview := decodeSeparatedControlDataMap(t, restorePreviewOut)
	require.NotEmpty(t, restorePreview["plan_id"])

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "blocked by restore",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "restore plan")
}

func TestSeparatedControlSaveAndCleanupBlockCorruptRestorePlan(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "before corrupt restore plan",
	)
	require.NoError(t, err, saveOut)

	cleanupPlanOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "preview",
	)
	require.NoError(t, err, cleanupPlanOut)
	_, cleanupPlan := decodeSeparatedControlDataMap(t, cleanupPlanOut)
	cleanupPlanID, _ := cleanupPlan["plan_id"].(string)
	require.NotEmpty(t, cleanupPlanID)

	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "corrupt-pending.json"), []byte("{not-json\n"), 0644))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "save", args: []string{"save", "-m", "blocked by corrupt restore plan"}},
		{name: "cleanup preview", args: []string{"cleanup", "preview"}},
		{name: "cleanup run", args: []string{"cleanup", "run", "--plan-id", cleanupPlanID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
			assert.Contains(t, env.Error.Message, "restore plans")
			assert.Contains(t, env.Error.Message, "not valid JSON")
		})
	}
}

func TestSeparatedControlSaveAndCleanupBlockStaleRestorePlanEvidence(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "restore source",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)

	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v2\n"), 0644))
	currentOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "current payload",
	)
	require.NoError(t, err, currentOut)

	cleanupPlanOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "preview",
	)
	require.NoError(t, err, cleanupPlanOut)
	_, cleanupPlan := decodeSeparatedControlDataMap(t, cleanupPlanOut)
	cleanupPlanID, _ := cleanupPlan["plan_id"].(string)
	require.NotEmpty(t, cleanupPlanID)

	restorePreviewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", savePointID,
		"--discard-unsaved",
	)
	require.NoError(t, err, restorePreviewOut)
	_, restorePreview := decodeSeparatedControlDataMap(t, restorePreviewOut)
	planID, _ := restorePreview["plan_id"].(string)
	require.NotEmpty(t, planID)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v3 unsaved\n"), 0644))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "save", args: []string{"save", "-m", "blocked by stale restore plan"}},
		{name: "cleanup preview", args: []string{"cleanup", "preview"}},
		{name: "cleanup run", args: []string{"cleanup", "run", "--plan-id", cleanupPlanID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, base, args...)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
			assert.Contains(t, env.Error.Message, "restore plan")
			assert.Contains(t, env.Error.Message, planID)
			assert.Equal(t, "v3 unsaved\n", separatedOpsReadFile(t, filepath.Join(payloadRoot, "app.txt")))
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

func TestSeparatedControlRestoreRunRejectsRegistryDriftAfterPreviewBeforeMutation(t *testing.T) {
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
		"save", "-m", "restore source",
	)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	savePointID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, savePointID)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v2\n"), 0644))
	previewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", savePointID, "--discard-unsaved",
	)
	require.NoError(t, err, previewOut)
	_, preview := decodeSeparatedControlDataMap(t, previewOut)
	planID, _ := preview["plan_id"].(string)
	require.NotEmpty(t, planID)

	driftPayloadRoot := filepath.Join(base, "payload-drift")
	require.NoError(t, os.MkdirAll(driftPayloadRoot, 0755))
	cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
	require.NoError(t, err)
	cfg.RealPath = driftPayloadRoot
	require.NoError(t, repo.WriteWorktreeConfig(controlRoot, "main", cfg))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", "--run", planID,
	)
	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, "E_PATH_BOUNDARY_ESCAPE")
	assert.NotContains(t, env.Error.Message, "payload root")
	assert.Contains(t, env.Error.Message, "workspace folder")
	assert.Contains(t, env.Error.Message, "control data")
	assert.Equal(t, "v2\n", separatedOpsReadFile(t, filepath.Join(payloadRoot, "app.txt")))
	assert.NoFileExists(t, filepath.Join(driftPayloadRoot, "app.txt"))
	assertSeparatedControlSentinelsIntact(t, controlRoot)
}

func TestSeparatedControlRestorePreviewRejectsRegistryDriftBeforePlanWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(savePointID string) []string
	}{
		{
			name: "whole",
			args: func(savePointID string) []string {
				return []string{"restore", savePointID, "--discard-unsaved"}
			},
		},
		{
			name: "path",
			args: func(savePointID string) []string {
				return []string{"restore", savePointID, "--path", "app.txt", "--discard-unsaved"}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
				"save", "-m", "restore source",
			)
			require.NoError(t, err, saveOut)
			_, saveData := decodeSeparatedControlDataMap(t, saveOut)
			savePointID, _ := saveData["save_point_id"].(string)
			require.NotEmpty(t, savePointID)
			require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v2\n"), 0644))

			driftPayloadRoot := filepath.Join(base, "payload-drift")
			require.NoError(t, os.MkdirAll(driftPayloadRoot, 0755))
			drifted := false
			restorePlanner := restoreplan.SetTransferPlannerForTest(separatedOpsTransferPlannerFunc(func(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
				if !drifted {
					cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
					require.NoError(t, err)
					cfg.RealPath = driftPayloadRoot
					require.NoError(t, repo.WriteWorktreeConfig(controlRoot, "main", cfg))
					drifted = true
				}
				return (engine.TransferPlanner{}).PlanTransfer(req)
			}))
			t.Cleanup(restorePlanner)
			plansBefore := separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "restore-plans"))

			args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
			args = append(args, tc.args(savePointID)...)
			stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), args...)

			require.Error(t, err, "restore preview unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, 1, errclass.ErrPathBoundaryEscape.Code)
			assert.NotContains(t, env.Error.Message, "payload root")
			assert.Contains(t, env.Error.Message, "workspace folder")
			assert.Contains(t, env.Error.Message, "control data")
			assert.True(t, drifted, "test planner should trigger registry drift")
			assert.Equal(t, plansBefore, separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "restore-plans")))
			assert.Equal(t, "v2\n", separatedOpsReadFile(t, filepath.Join(payloadRoot, "app.txt")))
			assert.NoFileExists(t, filepath.Join(driftPayloadRoot, "app.txt"))
			assertSeparatedControlSentinelsIntact(t, controlRoot)
		})
	}
}

func TestSeparatedControlCleanupPreviewRejectsPayloadSymlinkEscapeBeforePlanMutation(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	seedSeparatedControlMetadataSentinels(t, controlRoot)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("v1\n"), 0644))
	_, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "cleanup source",
	)
	require.NoError(t, err)
	if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), filepath.Join(payloadRoot, "control-link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	plansBefore := separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "gc"))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"cleanup", "preview",
	)
	requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, "E_PATH_BOUNDARY_ESCAPE")
	assert.Equal(t, plansBefore, separatedOpsReadDirNames(t, filepath.Join(controlRoot, ".jvs", "gc")))
	assertSeparatedControlSentinelsIntact(t, controlRoot)
}

type separatedOpsTransferPlannerFunc func(engine.TransferPlanRequest) (*engine.TransferPlan, error)

func (fn separatedOpsTransferPlannerFunc) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	return fn(req)
}

func assertSeparatedControlOpsData(t *testing.T, data map[string]any, controlRoot, payloadRoot, workspace string) {
	t.Helper()

	assertExternalControlDataShape(t, data, controlRoot, payloadRoot, workspace)
}

func seedSeparatedControlMetadataSentinels(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"audit", "locks", "restore-plans", "runtime"} {
		require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), []byte("audit sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), []byte("lock sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-state.tmp"), []byte("{}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644))
}

func assertSeparatedControlSentinelsIntact(t *testing.T, controlRoot string) {
	t.Helper()
	assert.Equal(t, "audit sentinel\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "audit", "platform.log")))
	assert.Equal(t, "lock sentinel\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "locks", "platform.lock")))
	assert.Equal(t, "{}\n", separatedOpsReadFile(t, filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-state.tmp")))
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

func assertSeparatedSaveTransfer(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()

	primary := requireSeparatedTransferByID(t, data, "save-primary")
	assert.Equal(t, "save", primary["operation"])
	assert.Equal(t, "materialization", primary["phase"])
	assert.Equal(t, "workspace_content", primary["source_role"])
	assert.Equal(t, payloadRoot, primary["source_path"])
	assert.Equal(t, "save_point_staging", primary["destination_role"])
	assertPathUnder(t, primary["materialization_destination"], filepath.Join(controlRoot, ".jvs"))
	assertPathUnder(t, primary["published_destination"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	assertPathUnder(t, primary["capability_probe_path"], filepath.Join(controlRoot, ".jvs"))
	assertSeparatedTransferCopyEvidence(t, primary)
}

func assertSeparatedViewTransfer(t *testing.T, data map[string]any, controlRoot, viewPath string) {
	t.Helper()

	primary := requireSeparatedTransferByID(t, data, "view-primary")
	assert.Equal(t, "view", primary["operation"])
	assert.Equal(t, "view_materialization", primary["phase"])
	assert.Equal(t, "save_point_payload", primary["source_role"])
	assertPathUnder(t, primary["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	assert.Equal(t, "view_directory", primary["destination_role"])
	assertPathUnder(t, primary["materialization_destination"], filepath.Join(controlRoot, ".jvs", "views"))
	assertPathUnder(t, primary["capability_probe_path"], filepath.Join(controlRoot, ".jvs", "views"))
	assert.Equal(t, viewPath, primary["published_destination"])
	assertSeparatedTransferCopyEvidence(t, primary)
}

func assertSeparatedRestorePreviewTransfer(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()

	primary := requireSeparatedTransferByID(t, data, "restore-preview-validation-primary")
	assert.Equal(t, "restore", primary["operation"])
	assert.Equal(t, "preview_validation", primary["phase"])
	assert.Equal(t, "expected", primary["result_kind"])
	assert.Equal(t, "preview_only", primary["permission_scope"])
	assert.Equal(t, "save_point_payload", primary["source_role"])
	assertPathUnder(t, primary["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	assert.Equal(t, "restore_preview_validation", primary["destination_role"])
	assertPathUnder(t, primary["materialization_destination"], filepath.Join(controlRoot, ".jvs"))
	assert.Equal(t, payloadRoot, primary["published_destination"])
	assertSeparatedTransferCopyEvidence(t, primary)
}

func assertSeparatedRestoreRunTransfers(t *testing.T, data map[string]any, controlRoot, payloadRoot string) {
	t.Helper()

	validation := requireSeparatedTransferByID(t, data, "restore-run-source-validation")
	assert.Equal(t, "restore", validation["operation"])
	assert.Equal(t, "source_validation", validation["phase"])
	assert.Equal(t, "final", validation["result_kind"])
	assert.Equal(t, "execution", validation["permission_scope"])
	assert.Equal(t, "save_point_payload", validation["source_role"])
	assertPathUnder(t, validation["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	assert.Equal(t, "restore_source_validation", validation["destination_role"])
	assertPathUnder(t, validation["materialization_destination"], filepath.Join(controlRoot, ".jvs"))
	assert.Equal(t, payloadRoot, validation["published_destination"])
	assertSeparatedTransferCopyEvidence(t, validation)

	primary := requireSeparatedTransferByID(t, data, "restore-run-primary")
	assert.Equal(t, "restore", primary["operation"])
	assert.Equal(t, "materialization", primary["phase"])
	assert.Equal(t, "save_point_payload", primary["source_role"])
	assertPathUnder(t, primary["source_path"], filepath.Join(controlRoot, ".jvs", "snapshots"))
	assert.Equal(t, "restore_staging", primary["destination_role"])
	assertPathUnder(t, primary["materialization_destination"], filepath.Dir(payloadRoot))
	assert.NotContains(t, filepath.ToSlash(primary["materialization_destination"].(string)), filepath.ToSlash(controlRoot))
	assert.Equal(t, payloadRoot, primary["published_destination"])
	assertSeparatedTransferCopyEvidence(t, primary)
}

func requireSeparatedTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		require.True(t, ok, "transfer should be an object: %#v", item)
		if record["transfer_id"] == id {
			return record
		}
	}
	t.Fatalf("missing transfer %q in %#v", id, transfers)
	return nil
}

func assertSeparatedTransferCopyEvidence(t *testing.T, record map[string]any) {
	t.Helper()

	assert.Equal(t, true, record["checked_for_this_operation"])
	assert.Contains(t, []any{"auto", "copy"}, record["requested_engine"])
	assert.Contains(t, []any{"copy", "juicefs_clone", "reflink_copy"}, record["effective_engine"])
	assert.Contains(t, []any{"fast_copy", "normal_copy"}, record["performance_class"])
	require.NotEmpty(t, record["capability_probe_path"])
}

func assertPathUnder(t *testing.T, got any, root string) {
	t.Helper()

	path, ok := got.(string)
	require.True(t, ok, "path should be a string: %#v", got)
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	require.NoError(t, err)
	assert.True(t, rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)), "%s should be under %s", path, root)
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
