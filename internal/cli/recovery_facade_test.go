package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWholeRestoreFailureCreatesRecoveryPlanStatusAndProtectsSource(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "Restore did not finish safely.")
	assert.Contains(t, err.Error(), "Recovery plan:")
	assert.Contains(t, err.Error(), "jvs recovery status")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	statusOut, err := executeCommand(createTestRootCmd(), "recovery", "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, recoveryPlanID)
	assert.Contains(t, statusOut, "active")
	assertRecoveryOutputOmitsInternalVocabulary(t, statusOut)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, recoveryPlanID, data["plan_id"])
	assert.Equal(t, "active", data["status"])
	assert.Equal(t, "restore", data["operation"])
	assert.Equal(t, sourceID, data["source_save_point"])
	assert.Contains(t, data["recommended_next_command"], "jvs recovery")
	assertRecoveryStatusPlanTransfers(t, data, []string{"restore-run-source-validation", "restore-run-primary"})
	assertRecoveryOutputOmitsInternalVocabulary(t, jsonOut)

	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assertRecoveryPlanTransfers(t, plan.Transfers, []string{"restore-run-source-validation", "restore-run-primary"})
	rawTransfers, ok := readRecoveryPlanFileMap(t, repoRoot, recoveryPlanID)["transfers"].([]any)
	require.True(t, ok, "recovery plan file should persist restore run transfers")
	assertRecoveryTransferIDs(t, rawTransfers, []string{"restore-run-source-validation", "restore-run-primary"})

	gcPlan, err := gc.NewCollector(repoRoot).PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.Contains(t, gcPlan.ProtectedSet, model.SnapshotID(sourceID))

	anotherPreview, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	anotherRestorePlanID := restorePlanIDFromHumanOutput(t, anotherPreview)
	stdout, err = executeCommand(createTestRootCmd(), "restore", "--run", anotherRestorePlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "active recovery plan")
	assert.Contains(t, err.Error(), "jvs recovery status")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
}

func TestSeparatedRestoreFailureGuidanceUsesSelectedCommands(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control root")
	payloadRoot := filepath.Join(base, "payload")
	cleanCWD := filepath.Join(base, "clean-cwd")
	require.NoError(t, os.MkdirAll(cleanCWD, 0755))
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "original.txt"), []byte("original"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.Remove(filepath.Join(payloadRoot, "original.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "source.txt"), []byte("source"), 0644))
	_ = saveSeparatedControlPoint(t, controlRoot, "current")

	previewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", sourceID,
		"--discard-unsaved",
	)
	require.NoError(t, err, previewOut)
	_, preview := decodeSeparatedControlDataMap(t, previewOut)
	restorePlanID, _ := preview["plan_id"].(string)
	require.NotEmpty(t, restorePlanID)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)

	stdout, err := executeCommand(createTestRootCmd(),
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", "--run", restorePlanID,
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	selectedStatus := "jvs --control-root " + shellQuoteArg(controlRoot) + " --workspace main recovery status " + recoveryPlanID
	assert.Contains(t, err.Error(), "Run: "+selectedStatus)
	assert.NotContains(t, err.Error(), "Run: jvs recovery")
	assert.NotContains(t, err.Error(), "jvs recovery resume")
	assert.NotContains(t, err.Error(), "jvs recovery rollback")

	plan, loadErr := recovery.NewManager(controlRoot).Load(recoveryPlanID)
	require.NoError(t, loadErr)
	assert.Equal(t, "jvs --control-root "+shellQuoteArg(controlRoot)+" --workspace main recovery resume "+recoveryPlanID, plan.RecommendedNextCommand)

	statusOut, statusErr := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status", recoveryPlanID,
	)
	require.NoError(t, statusErr, statusOut)
	_, data := decodeSeparatedControlDataMap(t, statusOut)
	assert.Equal(t, "jvs --control-root "+shellQuoteArg(controlRoot)+" --workspace main recovery resume "+recoveryPlanID, data["recommended_next_command"])
}

func TestRestoreFailurePersistsExecutedValidationSafetyAndPrimaryTransfers(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRestoreImpactRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID, "--save-first")
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	expectedTransfers := []string{"restore-run-source-validation", "save-primary", "restore-run-primary"}

	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assertRecoveryPlanTransfers(t, plan.Transfers, expectedTransfers)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assertRecoveryStatusPlanTransfers(t, data, expectedTransfers)
}

func TestRecoveryStatusShowsActivePlanSafetySummary(t *testing.T) {
	repoRoot, sourceID, _ := setupPathRecoveryRepo(t)
	recoveryPlanID := createPathRecoveryFailure(t, sourceID)

	listOut, err := executeCommand(createTestRootCmd(), "recovery", "status")
	require.NoError(t, err)
	assert.Contains(t, listOut, "Active recovery plans:")
	assert.Contains(t, listOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, listOut, "Status: active")
	assert.Contains(t, listOut, "Folder: "+repoRoot)
	assert.Contains(t, listOut, "Workspace: main")
	assert.Contains(t, listOut, "Source save point: "+sourceID)
	assert.Contains(t, listOut, "Path: app.txt")
	assert.Contains(t, listOut, "Recovery backup: available")
	assert.Contains(t, listOut, "Recommended next command: jvs recovery resume "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, listOut)

	detailOut, err := executeCommand(createTestRootCmd(), "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, detailOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, detailOut, "Status: active")
	assert.Contains(t, detailOut, "Folder: "+repoRoot)
	assert.Contains(t, detailOut, "Workspace: main")
	assert.Contains(t, detailOut, "Source save point: "+sourceID)
	assert.Contains(t, detailOut, "Path: app.txt")
	assert.Contains(t, detailOut, "Recovery backup: available")
	assert.Contains(t, detailOut, "Recommended next command: jvs recovery resume "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, detailOut)

	listJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status")
	require.NoError(t, err)
	env, listData := decodeFacadeDataMap(t, listJSON)
	require.True(t, env.OK, listJSON)
	assert.Equal(t, "recovery status", env.Command)
	plans, ok := listData["plans"].([]any)
	require.True(t, ok, "recovery status plans should be a list: %#v", listData)
	require.Len(t, plans, 1)
	listPlan, ok := plans[0].(map[string]any)
	require.True(t, ok, "recovery plan should be an object: %#v", plans[0])
	assertRecoveryJSONPlanSummary(t, listPlan, recoveryPlanID, repoRoot, sourceID, "app.txt")

	detailJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, detailData := decodeFacadeDataMap(t, detailJSON)
	require.True(t, env.OK, detailJSON)
	assert.Equal(t, "recovery status", env.Command)
	assertRecoveryJSONPlanSummary(t, detailData, recoveryPlanID, repoRoot, sourceID, "app.txt")
}

func TestRecoveryStatusJSONSurfacesSeparatedPendingRestorePreview(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	_ = saveSeparatedControlPoint(t, controlRoot, "current")
	plan, err := restoreplan.Create(controlRoot, "main", model.SnapshotID(sourceID), model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)
	require.NoError(t, err, stdout)
	env, data := decodeSeparatedControlDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "recovery status", env.Command)
	plans, ok := data["plans"].([]any)
	require.True(t, ok, "plans should remain a list: %#v", data)
	assert.Empty(t, plans)
	restoreState, ok := data["restore_state"].(map[string]any)
	require.True(t, ok, "restore_state should diagnose blocking restore preview: %#v", data)
	assert.Equal(t, "pending_restore_preview", restoreState["state"])
	assert.Equal(t, true, restoreState["blocking"])
	assert.Equal(t, plan.PlanID, restoreState["plan_id"])
	assert.Equal(t, "jvs --control-root "+shellQuoteArg(controlRoot)+" --workspace main restore --run "+plan.PlanID, restoreState["recommended_next_command"])
	assert.NotContains(t, stdout, ".jvs/restore-plans")
}

func TestRecoveryStatusJSONSurfacesSeparatedStaleRestorePreviewWithDiscard(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control root")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	_ = saveSeparatedControlPoint(t, controlRoot, "current")

	previewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", sourceID,
		"--discard-unsaved",
	)
	require.NoError(t, err, previewOut)
	_, preview := decodeSeparatedControlDataMap(t, previewOut)
	planID, _ := preview["plan_id"].(string)
	require.NotEmpty(t, planID)
	require.Equal(t, "jvs --control-root "+shellQuoteArg(controlRoot)+" --workspace main restore --run "+planID, preview["run_command"])
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("new local work\n"), 0644))

	statusOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)
	require.NoError(t, err, statusOut)
	_, data := decodeSeparatedControlDataMap(t, statusOut)
	restoreState, ok := data["restore_state"].(map[string]any)
	require.True(t, ok, "restore_state should diagnose stale restore preview: %#v", data)
	assert.Equal(t, "stale_restore_preview", restoreState["state"])
	assert.Equal(t, true, restoreState["blocking"])
	assert.Equal(t, planID, restoreState["plan_id"])
	assert.Equal(t, "jvs --control-root "+shellQuoteArg(controlRoot)+" --workspace main restore discard "+planID, restoreState["recommended_next_command"])
	assert.NotContains(t, statusOut, ".jvs/restore-plans")
}

func TestRecoveryStatusJSONErrorsOnSeparatedRestorePlanWorkspaceIdentityMismatch(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	plan, err := restoreplan.Create(controlRoot, "main", model.SnapshotID(sourceID), model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	plan.Workspace = "feature"
	require.NoError(t, restoreplan.Write(controlRoot, plan))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "Restore plan "+plan.PlanID+" cannot be inspected safely")
	assert.Contains(t, env.Error.Message, "workspace identity mismatch")
	assert.Contains(t, env.Error.Hint, "doctor --strict --json")
	assert.NotContains(t, env.Error.Message, "stale_restore_preview")
	assert.NotContains(t, env.Error.Hint, "restore discard")
	assert.NotContains(t, strings.ToLower(env.Error.Message), "payload")
}

func TestRecoveryStatusJSONPrioritizesSeparatedActiveRecoveryOverRestoreResidue(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	_ = saveSeparatedControlPoint(t, controlRoot, "current")
	plan, err := restoreplan.Create(controlRoot, "main", model.SnapshotID(sourceID), model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	recoveryPlanID := "RP-" + plan.PlanID
	writeRecoveryStatusSeparatedActivePlan(t, controlRoot, recoveryPlanID, plan)

	stdout, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)
	require.NoError(t, err, stdout)
	env, data := decodeSeparatedControlDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "recovery status", env.Command)
	plans, ok := data["plans"].([]any)
	require.True(t, ok, "plans should remain a list: %#v", data)
	require.Len(t, plans, 1)
	active, ok := plans[0].(map[string]any)
	require.True(t, ok, "active plan should be an object: %#v", plans[0])
	assert.Equal(t, recoveryPlanID, active["plan_id"])
	assert.Equal(t, "active", active["status"])
	assert.NotContains(t, data, "restore_state")
	assert.NotContains(t, stdout, "restore --run")
}

func TestRecoveryStatusJSONErrorsOnMalformedSeparatedRestorePlan(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "corrupt-pending.json"), []byte("{not-json\n"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "Restore plan corrupt-pending")
	assert.Contains(t, env.Error.Message, "not valid JSON")
	assert.Contains(t, env.Error.Hint, "doctor --strict --json")
	assert.NotContains(t, env.Error.Message, ".jvs/restore-plans")
}

func TestRecoveryStatusJSONPrioritizesMalformedRestorePlanOverPendingPreview(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	plan, err := restoreplan.Create(controlRoot, "main", model.SnapshotID(sourceID), model.EngineCopy, restoreplan.Options{DiscardUnsaved: true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "zzzz-corrupt.json"), []byte("{not-json\n"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "Restore plan zzzz-corrupt")
	assert.Contains(t, env.Error.Message, "not valid JSON")
	assert.NotContains(t, env.Error.Message, "Restore plan "+plan.PlanID+" is pending")
	assert.Contains(t, env.Error.Hint, "doctor --strict --json")
}

func TestRecoveryStatusJSONErrorsOnMalformedSeparatedRecoveryState(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, controlRoot string)
		want []string
	}{
		{
			name: "malformed recovery plan JSON",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", "recovery-plans"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "recovery-plans", "RP-corrupt.json"), []byte("{not-json\n"), 0644))
			},
			want: []string{"Recovery state cannot be inspected safely", "recovery plan \"RP-corrupt\"", "not valid JSON"},
		},
		{
			name: "recovery plan symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", "recovery-plans"), 0755))
				if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "repo_id"), filepath.Join(controlRoot, ".jvs", "recovery-plans", "RP-link.json")); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Recovery state cannot be inspected safely", "recovery plan \"RP-link.json\"", "symlink"},
		},
		{
			name: "recovery plans directory symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				plansDir := filepath.Join(controlRoot, ".jvs", "recovery-plans")
				require.NoError(t, os.RemoveAll(plansDir))
				if err := os.Symlink(filepath.Join(controlRoot, ".jvs", "repo_id"), plansDir); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Recovery state cannot be inspected safely", "recovery plans", "symlink"},
		},
		{
			name: "restore plans directory symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				plansDir := filepath.Join(controlRoot, ".jvs", "restore-plans")
				require.NoError(t, os.RemoveAll(plansDir))
				outsideDir := filepath.Join(filepath.Dir(controlRoot), "outside-restore-plans")
				require.NoError(t, os.MkdirAll(outsideDir, 0755))
				if err := os.Symlink(outsideDir, plansDir); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Restore plans directory cannot be inspected safely", "symlink"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			tc.seed(t, controlRoot)

			stdout, stderr, exitCode := runContractSubprocess(t, base,
				"--json",
				"--control-root", controlRoot,
				"--workspace", "main",
				"recovery", "status",
			)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
			for _, want := range tc.want {
				assert.Contains(t, env.Error.Message, want)
			}
			assert.Contains(t, env.Error.Hint, "doctor --strict --json")
			assert.NotContains(t, env.Error.Message, ".jvs")
			assert.NotContains(t, strings.ToLower(env.Error.Message), "payload")
		})
	}
}

func TestRecoveryStatusJSONErrorsOnSeparatedActiveRecoveryIdentityMismatch(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 "RP-identity-mismatch",
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "feature",
		Folder:                 payloadRoot,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: payloadRoot},
		Backup:                 recovery.Backup{Path: filepath.Join(base, "backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		Phase:                  recovery.PhasePending,
		RecommendedNextCommand: "jvs recovery status RP-identity-mismatch",
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "Recovery plan RP-identity-mismatch cannot be inspected safely")
	assert.Contains(t, env.Error.Message, "workspace")
	assert.Contains(t, env.Error.Hint, "doctor --strict --json")
	assert.NotContains(t, strings.ToLower(env.Error.Message), "payload")
}

func TestRecoveryStatusDetailJSONErrorsOnSeparatedActiveRecoveryIdentityMismatch(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 "RP-detail-identity-mismatch",
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "feature",
		Folder:                 payloadRoot,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: payloadRoot},
		Backup:                 recovery.Backup{Path: filepath.Join(base, "backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		Phase:                  recovery.PhasePending,
		RecommendedNextCommand: "jvs recovery status RP-detail-identity-mismatch",
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"recovery", "status", "RP-detail-identity-mismatch",
	)

	env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
	assert.Contains(t, env.Error.Message, "Recovery plan RP-detail-identity-mismatch cannot be inspected safely")
	assert.Contains(t, env.Error.Message, "workspace")
	assert.Contains(t, env.Error.Hint, "doctor --strict --json")
	assert.NotContains(t, strings.ToLower(env.Error.Message), "payload")
}

func TestRecoveryStatusDetailJSONRejectsResolvedPlanOutsideSelectedSeparatedBinding(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, plan *recovery.Plan, base, payloadRoot string)
		code string
		want []string
	}{
		{
			name: "workspace mismatch",
			edit: func(t *testing.T, plan *recovery.Plan, base, payloadRoot string) {
				featureRoot := filepath.Join(base, "feature-payload")
				require.NoError(t, os.MkdirAll(featureRoot, 0755))
				plan.Workspace = "feature"
				plan.Folder = featureRoot
				plan.PreWorktreeState = recovery.WorktreeState{Name: "feature", RealPath: featureRoot}
			},
			code: errclass.ErrRecoveryBlocking.Code,
			want: []string{"workspace identity mismatch", "feature", "main"},
		},
		{
			name: "folder boundary mismatch",
			edit: func(t *testing.T, plan *recovery.Plan, base, payloadRoot string) {
				otherRoot := filepath.Join(base, "other-payload")
				require.NoError(t, os.MkdirAll(otherRoot, 0755))
				plan.Folder = otherRoot
				plan.PreWorktreeState = recovery.WorktreeState{Name: "main", RealPath: otherRoot}
			},
			code: errclass.ErrPathBoundaryEscape.Code,
			want: []string{"workspace folder changed", "expected"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := setupSeparatedControlCLICWD(t)
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
			require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
			sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
			r, err := repo.OpenControlRoot(controlRoot)
			require.NoError(t, err)
			now := time.Now().UTC()
			plan := &recovery.Plan{
				SchemaVersion:          recovery.SchemaVersion,
				RepoID:                 r.RepoID,
				PlanID:                 "RP-resolved-detail-" + strings.ReplaceAll(tc.name, " ", "-"),
				Status:                 recovery.StatusResolved,
				Operation:              recovery.OperationRestore,
				RestorePlanID:          "restore-preview",
				Workspace:              "main",
				Folder:                 payloadRoot,
				SourceSavePoint:        model.SnapshotID(sourceID),
				CreatedAt:              now,
				UpdatedAt:              now,
				ResolvedAt:             &now,
				PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: payloadRoot},
				Backup:                 recovery.Backup{Path: filepath.Join(base, "backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRolledBack},
				Phase:                  recovery.PhaseRestoreApplied,
				RecommendedNextCommand: "jvs recovery status",
			}
			tc.edit(t, plan, base, payloadRoot)
			require.NoError(t, recovery.NewManager(controlRoot).Write(plan))

			stdout, stderr, exitCode := runContractSubprocess(t, base,
				"--json",
				"--control-root", controlRoot,
				"--workspace", "main",
				"recovery", "status", plan.PlanID,
			)

			env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, tc.code)
			if tc.code == errclass.ErrRecoveryBlocking.Code {
				assert.Contains(t, env.Error.Message, "Recovery plan "+plan.PlanID+" cannot be inspected safely")
			}
			for _, want := range tc.want {
				assert.Contains(t, env.Error.Message, want)
			}
			assert.Contains(t, env.Error.Hint, "doctor --strict --json")
			assert.NotContains(t, stdout, `"workspace":"feature"`)
		})
	}
}

func TestSeparatedRecoveryResumeRollbackRejectPlansOutsideSelectedBindingBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status recovery.Status
		edit   func(plan *recovery.Plan, base, payloadRoot string)
		want   []string
	}{
		{
			name:   "active pre worktree mismatch",
			status: recovery.StatusActive,
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.PreWorktreeState.Name = "feature"
			},
			want: []string{"workspace name identity mismatch", "feature", "main"},
		},
		{
			name:   "resolved workspace mismatch",
			status: recovery.StatusResolved,
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.Workspace = "feature"
				plan.PreWorktreeState.Name = "feature"
			},
			want: []string{"workspace identity mismatch", "feature", "main"},
		},
	} {
		for _, action := range []string{"resume", "rollback"} {
			t.Run(tc.name+" "+action, func(t *testing.T) {
				base, controlRoot, payloadRoot, planID := setupSeparatedRecoveryActionBindingFixture(t, tc.status, tc.edit)
				pinsBefore := documentedPinCount(t, controlRoot)

				stdout, stderr, exitCode := runContractSubprocess(t, base,
					"--json",
					"--control-root", controlRoot,
					"--workspace", "main",
					"recovery", action, planID,
				)

				env := requireSeparatedControlCLIJSONError(t, stdout, stderr, exitCode, errclass.ErrRecoveryBlocking.Code)
				assert.Contains(t, env.Error.Message, "Recovery plan "+planID+" cannot be inspected safely")
				for _, want := range tc.want {
					assert.Contains(t, env.Error.Message, want)
				}
				assert.Contains(t, env.Error.Hint, "doctor --strict --json")
				assert.Equal(t, "interrupted\n", separatedOpsReadFile(t, filepath.Join(payloadRoot, "app.txt")))
				plan, err := recovery.NewManager(controlRoot).Load(planID)
				require.NoError(t, err)
				assert.Equal(t, tc.status, plan.Status)
				assert.Equal(t, pinsBefore, documentedPinCount(t, controlRoot))
			})
		}
	}
}

func writeRecoveryStatusSeparatedActivePlan(t *testing.T, controlRoot, recoveryPlanID string, restorePlan *restoreplan.Plan) {
	t.Helper()

	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 recoveryPlanID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          restorePlan.PlanID,
		Workspace:              restorePlan.Workspace,
		Folder:                 restorePlan.Folder,
		SourceSavePoint:        restorePlan.SourceSavePoint,
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: restorePlan.Workspace, RealPath: restorePlan.Folder},
		Backup:                 recovery.Backup{Path: restorePlan.Folder + ".restore-backup-test", Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		Phase:                  recovery.PhasePending,
		RecommendedNextCommand: "jvs recovery status " + recoveryPlanID,
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))
}

func TestRecoveryStatusJSONSurfacesPersistedPlanTransfers(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	recoveryPlanID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	plan.Transfers = []transfer.Record{testRecoveryStatusTransfer(repoRoot, sourceID)}
	require.NoError(t, recovery.NewManager(repoRoot).Write(plan))

	listJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status")
	require.NoError(t, err)
	env, listData := decodeFacadeDataMap(t, listJSON)
	require.True(t, env.OK, listJSON)
	assert.Equal(t, "recovery status", env.Command)
	plans, ok := listData["plans"].([]any)
	require.True(t, ok, "recovery status plans should be a list: %#v", listData)
	require.Len(t, plans, 1)
	listPlan, ok := plans[0].(map[string]any)
	require.True(t, ok, "recovery plan should be an object: %#v", plans[0])
	assertRecoveryStatusPlanTransfers(t, listPlan, []string{"persisted-restore-transfer"})

	detailJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, detailData := decodeFacadeDataMap(t, detailJSON)
	require.True(t, env.OK, detailJSON)
	assert.Equal(t, "recovery status", env.Command)
	assertRecoveryStatusPlanTransfers(t, detailData, []string{"persisted-restore-transfer"})
}

func TestRecoveryPlanTransfersFilterActualExecutionMaterializationsWithoutDedupe(t *testing.T) {
	actual := testRecoveryStatusTransfer("/repo", "source")
	expectedPreview := actual
	expectedPreview.TransferID = "restore-preview-validation-primary"
	expectedPreview.ResultKind = transfer.ResultKindExpected
	expectedPreview.PermissionScope = transfer.PermissionScopePreviewOnly
	previewOnly := actual
	previewOnly.TransferID = "preview-only-transfer"
	previewOnly.PermissionScope = transfer.PermissionScopePreviewOnly
	emptyDestination := actual
	emptyDestination.TransferID = "empty-destination-transfer"
	emptyDestination.MaterializationDestination = ""

	plan := &recovery.Plan{}
	appendRecoveryCopyPointTransfers(plan, []transfer.Record{actual, expectedPreview, previewOnly, emptyDestination})
	appendRecoveryCopyPointTransfers(plan, []transfer.Record{actual})

	require.Len(t, plan.Transfers, 2)
	assert.Equal(t, "persisted-restore-transfer", plan.Transfers[0].TransferID)
	assert.Equal(t, "persisted-restore-transfer", plan.Transfers[1].TransferID)
}

func TestRecoveryStatusReportsUnavailableBackup(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	recoveryPlanID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)

	detailOut, err := executeCommand(createTestRootCmd(), "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, detailOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, detailOut, "Recovery backup: unavailable")
	assert.NotContains(t, detailOut, "Recommended next command: jvs recovery resume "+recoveryPlanID)
	assert.Contains(t, detailOut, "Recommended next command: jvs recovery rollback "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, detailOut)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, false, data["backup_available"])
	assert.Equal(t, "jvs recovery rollback "+recoveryPlanID, data["recommended_next_command"])
}

func TestRecoveryStatusOmitsRecommendationWhenBackupUnavailableAndEvidenceUnsafe(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	recoveryPlanID := createRequiredMissingBackupPlanAfterPayloadMutation(t, repoRoot, sourceID)

	detailOut, err := executeCommand(createTestRootCmd(), "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, detailOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, detailOut, "Recovery backup: unavailable")
	assert.NotContains(t, detailOut, "Recommended next command:")
	assert.NotContains(t, detailOut, "jvs recovery resume "+recoveryPlanID)
	assert.NotContains(t, detailOut, "jvs recovery rollback "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, detailOut)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, false, data["backup_available"])
	assert.NotContains(t, data, "recommended_next_command")
}

func TestRecoveryStatusTreatsSemanticallyUnsafeExistingBackupAsUnavailable(t *testing.T) {
	repoRoot, sourceID, _ := setupPathRecoveryRepo(t)
	recoveryPlanID := createPathRecoveryPlanWithUnsafeBackupSemantics(t, repoRoot, sourceID)

	detailOut, err := executeCommand(createTestRootCmd(), "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, detailOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, detailOut, "Recovery backup: unavailable")
	assert.NotContains(t, detailOut, "Recommended next command:")
	assert.NotContains(t, detailOut, "jvs recovery resume "+recoveryPlanID)
	assert.NotContains(t, detailOut, "jvs recovery rollback "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, detailOut)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, false, data["backup_available"])
	assert.NotContains(t, data, "recommended_next_command")
}

func TestRecoveryStatusTreatsPathBackupMissingRequiredEntryAsUnavailable(t *testing.T) {
	repoRoot, sourceID, _ := setupPathRecoveryRepo(t)
	recoveryPlanID := createPathRecoveryPlanWithMissingRequiredBackupEntry(t, repoRoot, sourceID)

	listOut, err := executeCommand(createTestRootCmd(), "recovery", "status")
	require.NoError(t, err)
	assert.Contains(t, listOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, listOut, "Recovery backup: unavailable")
	assert.NotContains(t, listOut, "Recommended next command:")
	assert.NotContains(t, listOut, "jvs recovery resume "+recoveryPlanID)
	assert.NotContains(t, listOut, "jvs recovery rollback "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, listOut)

	detailOut, err := executeCommand(createTestRootCmd(), "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, detailOut, "Recovery plan: "+recoveryPlanID)
	assert.Contains(t, detailOut, "Recovery backup: unavailable")
	assert.NotContains(t, detailOut, "Recommended next command:")
	assert.NotContains(t, detailOut, "jvs recovery resume "+recoveryPlanID)
	assert.NotContains(t, detailOut, "jvs recovery rollback "+recoveryPlanID)
	assertRecoveryOutputOmitsInternalVocabulary(t, detailOut)

	listJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status")
	require.NoError(t, err)
	env, listData := decodeFacadeDataMap(t, listJSON)
	require.True(t, env.OK, listJSON)
	assert.Equal(t, "recovery status", env.Command)
	plans, ok := listData["plans"].([]any)
	require.True(t, ok, "recovery status plans should be a list: %#v", listData)
	require.Len(t, plans, 1)
	listPlan, ok := plans[0].(map[string]any)
	require.True(t, ok, "recovery plan should be an object: %#v", plans[0])
	assert.Equal(t, false, listPlan["backup_available"])
	assert.NotContains(t, listPlan, "recommended_next_command")

	detailJSON, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, detailData := decodeFacadeDataMap(t, detailJSON)
	require.True(t, env.OK, detailJSON)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, false, detailData["backup_available"])
	assert.NotContains(t, detailData, "recommended_next_command")
}

func TestRestoreRunCommitUncertainVisibleRecoveryPlanStopsBeforeMutationAndProtectsSource(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		require.NoError(t, os.WriteFile(path, data, perm))
		return &fsutil.CommitUncertainError{Op: "atomic write", Path: path, Err: errors.New("injected post-commit fsync failure")}
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "jvs recovery status")
	assert.Contains(t, err.Error(), "jvs recovery resume")
	assert.Contains(t, err.Error(), "jvs recovery rollback")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())

	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, recovery.StatusActive, plans[0].Status)
	assert.Equal(t, model.SnapshotID(sourceID), plans[0].SourceSavePoint)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)

	gcPlan, err := gc.NewCollector(repoRoot).PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.Contains(t, gcPlan.ProtectedSet, model.SnapshotID(sourceID))
}

func TestRecoveryResumeResolvesStalePlanAfterSuccessfulRestorePlanWriteFailure(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected post-mutation recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "Files were changed")
	assert.NotContains(t, err.Error(), "No files were changed.")
	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	recoveryPlanID := plans[0].PlanID
	assert.Equal(t, recovery.StatusActive, plans[0].Status)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestWholeRecoveryRollbackRestoresFilesMetadataAndResolvesPlan(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)
	originalBackupCount := restoreBackupCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	cleanupWhileActive := cleanupPreviewData(t)
	assertCleanupFieldContains(t, cleanupWhileActive, "protected_save_points", sourceID)
	assertCleanupFieldOmits(t, cleanupWhileActive, "reclaimable_save_points", sourceID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assert.Contains(t, rollbackOut, "History was restored to the pre-restore state.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Empty(t, cfg.PathSources)
	assert.Equal(t, originalBackupCount, restoreBackupCount(t, repoRoot))
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
	cleanupAfterRollback := cleanupPreviewData(t)
	assertCleanupFieldOmits(t, cleanupAfterRollback, "protected_save_points", sourceID)
	assertCleanupFieldContains(t, cleanupAfterRollback, "reclaimable_save_points", sourceID)
}

func TestRecoveryRollbackBackupRestoreSurfacesTransferInHumanAndJSON(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		_, sourceID, _ := setupWholeRecoveryRepo(t)
		recoveryPlanID := createWholeRecoveryFailure(t, sourceID)

		rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
		require.NoError(t, err)
		assert.Contains(t, rollbackOut, "Recovery rollback completed.")
		assert.Contains(t, rollbackOut, "Copy method:")
		assert.Contains(t, rollbackOut, "Checked for this operation")
		assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)
	})

	t.Run("json", func(t *testing.T) {
		repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
		recoveryPlanID := createWholeRecoveryFailure(t, sourceID)

		jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "rollback", recoveryPlanID)
		require.NoError(t, err)
		env, data := decodeFacadeDataMap(t, jsonOut)
		require.True(t, env.OK, jsonOut)
		assert.Equal(t, "recovery rollback", env.Command)
		transfers, ok := data["transfers"].([]any)
		require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
		require.Len(t, transfers, 1)
		primary, ok := transfers[0].(map[string]any)
		require.True(t, ok, "primary transfer should be an object: %#v", transfers[0])
		assert.Equal(t, "recovery_rollback", primary["operation"])
		assert.Equal(t, "backup_restore", primary["phase"])
		assert.Equal(t, true, primary["primary"])
		assert.Equal(t, "final", primary["result_kind"])
		assert.Equal(t, "execution", primary["permission_scope"])
		assert.Equal(t, "recovery_backup_content", primary["source_role"])
		assert.Contains(t, primary["source_path"], ".restore-backup-")
		assert.Equal(t, "temporary_folder", primary["destination_role"])
		assert.Equal(t, "temporary_folder", primary["materialization_destination"])
		assert.Equal(t, repoRoot, primary["published_destination"])
		assert.Equal(t, true, primary["checked_for_this_operation"])
		assert.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	})
}

func TestRecoveryRollbackBackupRestorePersistsTransferInRecoveryPlan(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	recoveryPlanID := createWholeRecoveryFailure(t, sourceID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err, rollbackOut)

	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	require.Len(t, plan.Transfers, 3)
	assert.Equal(t, "restore-run-source-validation", plan.Transfers[0].TransferID)
	assert.Equal(t, "restore-run-primary", plan.Transfers[1].TransferID)
	assert.Equal(t, "recovery-backup-restore-primary", plan.Transfers[2].TransferID)
	assert.Equal(t, "recovery_rollback", plan.Transfers[2].Operation)
	assert.Equal(t, "backup_restore", plan.Transfers[2].Phase)
	assert.NotEmpty(t, plan.Transfers[2].MaterializationDestination)

	rawPlan := readRecoveryPlanFileMap(t, repoRoot, recoveryPlanID)
	rawTransfers, ok := rawPlan["transfers"].([]any)
	require.True(t, ok, "recovery plan file should persist transfers: %#v", rawPlan)
	require.Len(t, rawTransfers, 3)
}

func TestRecoveryRollbackResolvesAfterBackupRestoreAppliedButPlanWriteFailed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			return errors.New("injected metadata confirmation failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())

	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 1 {
			return errors.New("injected rollback recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})
	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
}

func TestWholeRecoveryResumeCompletesRestoreAndResolvesPlan(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	restoreHooks()

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Empty(t, cfg.PathSources)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
}

func TestPathRecoveryResumeResolvesAfterSuccessfulRestorePlanWriteFailure(t *testing.T) {
	repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID, "--path", "app.txt")
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected path post-mutation recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "Files were changed")
	assert.NotContains(t, err.Error(), "No files were changed.")
	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	recoveryPlanID := plans[0].PlanID
	assert.Equal(t, recovery.StatusActive, plans[0].Status)

	app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(app))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
	entry, ok, err := cfg.PathSources.SourceForPath("app.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.SnapshotID(sourceID), entry.SourceSnapshotID)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored path: app.txt")
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestPathRecoveryRollbackIsScopedAndResumeRecordsPathSource(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
		recoveryPlanID := createPathRecoveryFailure(t, sourceID)

		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside changed after failure"), 0644))
		rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
		require.NoError(t, err)
		assert.Contains(t, rollbackOut, "Recovery rollback completed.")
		assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

		app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
		require.NoError(t, err)
		assert.Equal(t, "v2", string(app))
		outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
		require.NoError(t, err)
		assert.Equal(t, "outside changed after failure", string(outside))
		cfg, err := worktree.NewManager(repoRoot).Get("main")
		require.NoError(t, err)
		assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
		assert.Equal(t, model.SnapshotID(latestID), cfg.LatestSnapshotID)
		assert.Empty(t, cfg.PathSources)
		assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	})

	t.Run("resume", func(t *testing.T) {
		repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
		recoveryPlanID := createPathRecoveryFailure(t, sourceID)

		resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
		require.NoError(t, err)
		assert.Contains(t, resumeOut, "Recovery resume completed.")
		assert.Contains(t, resumeOut, "Restored path: app.txt")
		assert.Contains(t, resumeOut, "From save point: "+sourceID)
		assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)

		app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
		require.NoError(t, err)
		assert.Equal(t, "v1", string(app))
		outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
		require.NoError(t, err)
		assert.Equal(t, "outside v2", string(outside))
		cfg, err := worktree.NewManager(repoRoot).Get("main")
		require.NoError(t, err)
		assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
		assert.Equal(t, model.SnapshotID(latestID), cfg.LatestSnapshotID)
		entry, ok, err := cfg.PathSources.SourceForPath("app.txt")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, model.SnapshotID(sourceID), entry.SourceSnapshotID)
		assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	})
}

func TestRecoveryRollbackCompletesAfterBackupPayloadRestoredButMetadataWriteFailed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			return errors.New("injected metadata confirmation failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())

	restoreMetadata := recovery.SetWriteWorktreeConfigHookForTest(func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		return errors.New("injected recovery metadata write failure")
	})
	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	restoreMetadata()
	require.Error(t, err)
	require.Empty(t, stdout)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	cfg, err = worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
}

func TestRecoveryRollbackRejectsReplacedBackupBeforeMutation(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	recoveryPlanID := createWholeRecoveryFailure(t, sourceID)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	require.Equal(t, recovery.BackupStateRequired, plan.Backup.State)
	require.NotEmpty(t, plan.PreRecoveryEvidence)
	require.Equal(t, plan.PreRecoveryEvidence, plan.Backup.PayloadEvidence)
	require.DirExists(t, plan.Backup.Path)

	require.NoError(t, os.RemoveAll(plan.Backup.Path))
	require.NoError(t, os.MkdirAll(plan.Backup.Path, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(plan.Backup.Path, "replaced.txt"), []byte("replaced backup"), 0644))
	beforePins := documentedPinCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "recovery rollback cannot be completed safely")
	assert.Contains(t, err.Error(), "recovery backup")
	assert.Contains(t, err.Error(), "no files were changed")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())

	plan, err = recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	assert.FileExists(t, filepath.Join(plan.Backup.Path, "replaced.txt"))

	content, err := os.ReadFile(filepath.Join(repoRoot, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(repoRoot, "original.txt"))
}

func TestRecoveryRollbackWithEvidenceAndMissingBackupResolvesWithoutMutation(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assert.Contains(t, rollbackOut, "No recovery backup was present.")
	assert.NotContains(t, rollbackOut, "Recovery backup removed.")
	assert.NotContains(t, rollbackOut, "Copy method:")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	plan, err := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestRecoveryRollbackNoCopyJSONOmitsTransfers(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "rollback", planID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery rollback", env.Command)
	assert.NotContains(t, data, "transfers")

	plan, err := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	assert.Empty(t, plan.Transfers)
	assert.NotContains(t, readRecoveryPlanFileMap(t, repoRoot, planID), "transfers")
}

func TestRecoveryRollbackPreMutationWithBackupJSONAndPlanOmitTransfers(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	plan, err := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(plan.Backup.Path, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(plan.Backup.Path, "original.txt"), []byte("original"), 0644))

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "rollback", planID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery rollback", env.Command)
	assert.Equal(t, true, data["backup_removed"])
	assert.NotContains(t, data, "transfers")
	assert.NoDirExists(t, plan.Backup.Path)

	plan, err = recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	assert.Empty(t, plan.Transfers)
	assert.NotContains(t, readRecoveryPlanFileMap(t, repoRoot, planID), "transfers")
}

func TestRecoveryResumeNoBackupCopySurfacesRestoreTransfer(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
		planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)

		resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
		require.NoError(t, err)
		assert.Contains(t, resumeOut, "Recovery resume completed.")
		assert.Contains(t, resumeOut, "Copy method:")
		assert.Contains(t, resumeOut, "Checked for this operation")
		assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
	})

	t.Run("json", func(t *testing.T) {
		t.Setenv("JVS_SNAPSHOT_ENGINE", "")
		t.Setenv("JVS_ENGINE", "")
		repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
		planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)

		jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "resume", planID)
		require.NoError(t, err)
		env, data := decodeFacadeDataMap(t, jsonOut)
		require.True(t, env.OK, jsonOut)
		assert.Equal(t, "recovery resume", env.Command)
		transfers := requireRecoveryTransfers(t, data, 1)
		restoreTransfer := requireTransferMap(t, transfers[0])
		assertRecoveryResumeRestoreTransfer(t, restoreTransfer, repoRoot, sourceID)
	})
}

func TestRecoveryResumeWithBackupRestoreSurfacesBothTransfersInJSON(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createWholeResumePlanRequiringBackupRestore(t, repoRoot, sourceID)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "resume", planID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery resume", env.Command)
	transfers := requireRecoveryTransfers(t, data, 2)
	backupTransfer := requireTransferMap(t, transfers[0])
	assert.Equal(t, "recovery-backup-restore-primary", backupTransfer["transfer_id"])
	assert.Equal(t, "recovery_resume", backupTransfer["operation"])
	assert.Equal(t, "backup_restore", backupTransfer["phase"])
	assert.Equal(t, true, backupTransfer["primary"])
	assert.Equal(t, "final", backupTransfer["result_kind"])
	assert.Equal(t, "execution", backupTransfer["permission_scope"])
	assert.Equal(t, "recovery_backup_content", backupTransfer["source_role"])
	assert.Contains(t, backupTransfer["source_path"], ".restore-backup-")
	assert.Equal(t, "temporary_folder", backupTransfer["destination_role"])
	assert.Equal(t, "temporary_folder", backupTransfer["materialization_destination"])
	assert.Equal(t, repoRoot, backupTransfer["published_destination"])
	assert.Equal(t, true, backupTransfer["checked_for_this_operation"])
	assert.Equal(t, "auto", backupTransfer["requested_engine"])
	assert.Contains(t, []any{"fast_copy", "normal_copy"}, backupTransfer["performance_class"])

	restoreTransfer := requireTransferMap(t, transfers[1])
	assertRecoveryResumeRestoreTransfer(t, restoreTransfer, repoRoot, sourceID)

	plan, err := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	require.Len(t, plan.Transfers, 2)
	assert.Equal(t, "recovery-backup-restore-primary", plan.Transfers[0].TransferID)
	assert.Equal(t, "restore-run-primary", plan.Transfers[1].TransferID)
	rawTransfers, ok := readRecoveryPlanFileMap(t, repoRoot, planID)["transfers"].([]any)
	require.True(t, ok, "recovery plan file should persist transfers")
	require.Len(t, rawTransfers, 2)
}

func TestRecoveryResumePersistsBackupRestoreTransferWhenBackupRestoreFailsAfterCopy(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createWholeResumePlanRequiringBackupRestore(t, repoRoot, sourceID)

	restoreMetadata := recovery.SetWriteWorktreeConfigHookForTest(func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		return errors.New("injected recovery metadata write failure")
	})
	stdout, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	restoreMetadata()
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected recovery metadata write failure")

	expectedTransfers := []string{"recovery-backup-restore-primary"}
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assertRecoveryPlanTransfers(t, plan.Transfers, expectedTransfers)

	rawTransfers, ok := readRecoveryPlanFileMap(t, repoRoot, planID)["transfers"].([]any)
	require.True(t, ok, "recovery plan file should persist backup restore transfer")
	assertRecoveryTransferIDs(t, rawTransfers, expectedTransfers)

	jsonOut, statusErr := executeCommand(createTestRootCmd(), "--json", "recovery", "status", planID)
	require.NoError(t, statusErr)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, "active", data["status"])
	assertRecoveryStatusPlanTransfers(t, data, expectedTransfers)
}

func TestRecoveryResumePersistsPrimaryTransferWhenRestoreFailsAfterMaterialization(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	updateHeadCalled := false
	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			updateHeadCalled = true
			return errors.New("injected update metadata failure after materialization")
		},
	})
	t.Cleanup(restoreHooks)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.True(t, updateHeadCalled, "restore should reach metadata update after materialization")

	expectedTransfers := []string{"restore-run-primary"}
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assertRecoveryPlanTransfers(t, plan.Transfers, expectedTransfers)

	rawTransfers, ok := readRecoveryPlanFileMap(t, repoRoot, planID)["transfers"].([]any)
	require.True(t, ok, "recovery plan file should persist restore materialization transfer")
	assertRecoveryTransferIDs(t, rawTransfers, expectedTransfers)

	jsonOut, statusErr := executeCommand(createTestRootCmd(), "--json", "recovery", "status", planID)
	require.NoError(t, statusErr)
	require.NotContains(t, jsonOut, "restore-preview-validation-primary")
	require.NotContains(t, jsonOut, `"result_kind":"expected"`)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, "active", data["status"])
	assertRecoveryStatusPlanTransfers(t, data, expectedTransfers)
}

func TestRecoveryResumeWithBackupRestoreHumanShowsPrimaryAndAdditionalTransfers(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createWholeResumePlanRequiringBackupRestore(t, repoRoot, sourceID)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.NoError(t, err, resumeOut)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Copy method:")
	assert.Equal(t, 1, strings.Count(resumeOut, "Copy method:"), resumeOut)
	assert.Contains(t, resumeOut, "Additional transfers: 1 backup restore; see JSON for details")
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
}

func TestRecoveryResumePrimaryTransferPrefersRestoreMaterialization(t *testing.T) {
	transfers := []transfer.Record{
		{
			TransferID: "recovery-backup-restore-primary",
			Operation:  "recovery_resume",
			Phase:      "backup_restore",
			Primary:    true,
		},
		{
			TransferID: "restore-run-primary",
			Operation:  "restore",
			Phase:      "materialization",
			Primary:    true,
		},
	}

	primary := recoveryResumePrimaryTransfer(transfers)
	require.NotNil(t, primary)
	assert.Equal(t, "restore-run-primary", primary.TransferID)
}

func TestRecoveryResumeWithEvidenceAndMissingBackupRetriesRestore(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
}

func TestRecoveryMissingBackupWithChangedWorkspaceMetadataFailsClosed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = model.SnapshotID(sourceID)
	require.NoError(t, repo.WriteWorktreeConfig(repoRoot, "main", cfg))
	beforePins := documentedPinCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "current workspace state")
	assert.Contains(t, err.Error(), "no files were changed")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	stdout, err = executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "current workspace state")
	assert.Contains(t, err.Error(), "no files were changed")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr = recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := repoRoot
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err = repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
}

func TestRecoveryMissingRequiredBackupFailsClosedAndPlanStaysActive(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createRequiredMissingBackupPlanAfterPayloadMutation(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot be completed safely")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := repoRoot
	content, readErr := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))

	stdout, err = executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot return to the saved recovery point safely")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr = recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
}

func TestRecoveryResumeNonIncompleteFailureKeepsPlanActiveAndResumable(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.Remove(auditPath))
	require.NoError(t, os.Mkdir(auditPath, 0755))

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.NotEmpty(t, plan.RecoveryEvidence)
	assert.NotEmpty(t, plan.LastError)

	require.NoError(t, os.RemoveAll(auditPath))
	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
}

func setupWholeRecoveryRepo(t *testing.T) (repoRoot, sourceID, originalID string) {
	t.Helper()
	repoRoot = t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	_, err = repo.Init(repoRoot, "test")
	require.NoError(t, err)
	mainPath := repoRoot
	require.NoError(t, os.Chdir(mainPath))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "original.txt"), []byte("original"), 0644))
	originalID = savePointIDFromCLI(t, "original")

	mgr := worktree.NewManager(repoRoot)
	_, err = mgr.Create("source", nil)
	require.NoError(t, err)
	sourcePath, err := mgr.Path("source")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePath, "source.txt"), []byte("source"), 0644))
	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("source", "source", nil)
	require.NoError(t, err)
	require.NoError(t, mgr.Remove("source"))
	require.NoError(t, os.Chdir(mainPath))
	return repoRoot, string(desc.SnapshotID), originalID
}

func createCrashRecoveryPlanWithMissingBackup(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: folder + ".restore-backup-missing", Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func createRequiredMissingBackupPlanAfterPayloadMutation(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(folder, "original.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "source.txt"), []byte("source"), 0644))
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: folder + ".restore-backup-missing", Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func createPathRecoveryPlanWithUnsafeBackupSemantics(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	backupPath := folder + ".restore-backup-unsafe"
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	evidence, err := restoreplan.PathEvidence(repoRoot, "main", "app.txt")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestorePath,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		Path:                   "app.txt",
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func createPathRecoveryPlanWithMissingRequiredBackupEntry(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	backupPath := folder + ".restore-backup-missing-entry"
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	evidence, err := restoreplan.PathEvidence(repoRoot, "main", "app.txt")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestorePath,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		Path:                   "app.txt",
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: backupPath, Scope: recovery.BackupScopePath, State: recovery.BackupStateRequired, Entries: []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}}},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func setupPathRecoveryRepo(t *testing.T) (repoRoot, sourceID, latestID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	sourceID = savePointIDFromCLI(t, "source")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	latestID = savePointIDFromCLI(t, "latest")
	return repoRoot, sourceID, latestID
}

func createPathRecoveryFailure(t *testing.T, sourceID string) string {
	t.Helper()
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID, "--path", "app.txt")
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		RecordPathSource: func(string, string, string, model.SnapshotID) error {
			return errors.New("injected record path source failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Recovery plan:")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	return recoveryPlanIDFromText(t, err.Error())
}

func createWholeRecoveryFailure(t *testing.T, sourceID string) string {
	t.Helper()
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			return errors.New("injected update metadata failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Recovery plan:")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	return recoveryPlanIDFromText(t, err.Error())
}

func createWholeResumePlanRequiringBackupRestore(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	mainPath := repoRoot
	preEvidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	backupPath := mainPath + ".restore-backup-transfer"
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "original.txt"), []byte("original"), 0644))
	require.NoError(t, os.Remove(filepath.Join(mainPath, "original.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "interrupted.txt"), []byte("interrupted"), 0644))
	recoveryEvidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)

	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 mainPath,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
		Phase:                  recovery.PhaseBackupRequired,
		PreRecoveryEvidence:    preEvidence,
		RecoveryEvidence:       recoveryEvidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	recordCLIRecoveryBackupPayloadEvidence(t, repoRoot, &plan)
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func setupSeparatedRecoveryActionBindingFixture(t *testing.T, status recovery.Status, edit func(*recovery.Plan, string, string)) (base, controlRoot, payloadRoot, planID string) {
	t.Helper()

	base = setupSeparatedControlCLICWD(t)
	controlRoot = filepath.Join(base, "control")
	payloadRoot = filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	sourceID := saveSeparatedControlPoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("original\n"), 0644))
	_ = saveSeparatedControlPoint(t, controlRoot, "original")

	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
	require.NoError(t, err)
	preEvidence, err := restoreplan.WorkspaceEvidence(controlRoot, "main")
	require.NoError(t, err)
	backupPath := payloadRoot + ".restore-backup-binding"
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("original\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("interrupted\n"), 0644))
	recoveryEvidence, err := restoreplan.WorkspaceEvidence(controlRoot, "main")
	require.NoError(t, err)

	now := time.Now().UTC()
	planID = "RP-binding-" + strings.ReplaceAll(string(status), "_", "-") + "-" + string(model.NewSnapshotID())
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 status,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 payloadRoot,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
		Phase:                  recovery.PhaseBackupRequired,
		PreRecoveryEvidence:    preEvidence,
		RecoveryEvidence:       recoveryEvidence,
		RecommendedNextCommand: "jvs recovery resume " + planID,
	}
	recordCLIRecoveryBackupPayloadEvidence(t, controlRoot, plan)
	if status == recovery.StatusResolved {
		plan.ResolvedAt = &now
	}
	if edit != nil {
		edit(plan, base, payloadRoot)
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))
	return base, controlRoot, payloadRoot, planID
}

func requireRecoveryTransfers(t *testing.T, data map[string]any, expected int) []any {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, transfers, expected)
	return transfers
}

func requireTransferMap(t *testing.T, transfer any) map[string]any {
	t.Helper()
	record, ok := transfer.(map[string]any)
	require.True(t, ok, "transfer should be an object: %#v", transfer)
	return record
}

func assertRecoveryResumeRestoreTransfer(t *testing.T, record map[string]any, repoRoot, sourceID string) {
	t.Helper()
	mainPath := repoRoot
	assert.Equal(t, "restore-run-primary", record["transfer_id"])
	assert.Equal(t, "restore", record["operation"])
	assert.Equal(t, "materialization", record["phase"])
	assert.Equal(t, true, record["primary"])
	assert.Equal(t, "final", record["result_kind"])
	assert.Equal(t, "execution", record["permission_scope"])
	assert.Equal(t, "save_point_content", record["source_role"])
	assert.Equal(t, "save_point:"+sourceID, record["source_path"])
	assert.Equal(t, "temporary_folder", record["destination_role"])
	assert.Equal(t, "temporary_folder", record["materialization_destination"])
	assert.Equal(t, filepath.Dir(repoRoot), record["capability_probe_path"])
	assert.Equal(t, mainPath, record["published_destination"])
	assert.Equal(t, true, record["checked_for_this_operation"])
	assert.Equal(t, "auto", record["requested_engine"])
	assert.Contains(t, []any{"fast_copy", "normal_copy"}, record["performance_class"])
	assert.IsType(t, []any{}, record["degraded_reasons"])
	assert.IsType(t, []any{}, record["warnings"])
}

func testRecoveryStatusTransfer(repoRoot, sourceID string) transfer.Record {
	mainPath := repoRoot
	return transfer.Record{
		TransferID:                 "persisted-restore-transfer",
		Operation:                  "restore",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 transfer.ResultKindFinal,
		PermissionScope:            transfer.PermissionScopeExecution,
		SourceRole:                 "save_point_payload",
		SourcePath:                 filepath.Join(repoRoot, ".jvs", "snapshots", sourceID),
		DestinationRole:            "restore_staging",
		MaterializationDestination: mainPath + ".restore-tmp-status",
		CapabilityProbePath:        repoRoot,
		PublishedDestination:       mainPath,
		CheckedForThisOperation:    true,
		RequestedEngine:            engine.EngineAuto,
		EffectiveEngine:            model.EngineCopy,
		PerformanceClass:           transfer.PerformanceClassNormalCopy,
		DegradedReasons:            []string{},
		Warnings:                   []string{},
	}
}

func assertRecoveryStatusPlanTransfers(t *testing.T, data map[string]any, expectedIDs []string) {
	t.Helper()
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "recovery status plan should include transfers: %#v", data)
	assertRecoveryTransferIDs(t, transfers, expectedIDs)
}

func assertRecoveryPlanTransfers(t *testing.T, transfers []transfer.Record, expectedIDs []string) {
	t.Helper()
	require.Len(t, transfers, len(expectedIDs))
	for i, expectedID := range expectedIDs {
		assert.Equal(t, expectedID, transfers[i].TransferID)
		assert.Equal(t, transfer.ResultKindFinal, transfers[i].ResultKind)
		assert.Equal(t, transfer.PermissionScopeExecution, transfers[i].PermissionScope)
		assert.NotEqual(t, "restore-preview-validation-primary", transfers[i].TransferID)
		assert.NotEmpty(t, transfers[i].MaterializationDestination)
	}
}

func assertRecoveryTransferIDs(t *testing.T, transfers []any, expectedIDs []string) {
	t.Helper()
	require.Len(t, transfers, len(expectedIDs))
	for i, expectedID := range expectedIDs {
		record := requireTransferMap(t, transfers[i])
		assert.Equal(t, expectedID, record["transfer_id"])
		assert.Equal(t, "final", record["result_kind"])
		assert.Equal(t, "execution", record["permission_scope"])
		assert.NotEqual(t, "restore-preview-validation-primary", record["transfer_id"])
		assert.NotEmpty(t, record["materialization_destination"])
	}
}

func readRecoveryPlanFileMap(t *testing.T, repoRoot, planID string) map[string]any {
	t.Helper()
	path, err := repo.RecoveryPlanPath(repoRoot, planID)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	return raw
}

func recordCLIRecoveryBackupPayloadEvidence(t *testing.T, repoRoot string, plan *recovery.Plan) {
	t.Helper()
	evidence, err := recovery.NewManager(repoRoot).BackupPayloadEvidence(plan)
	require.NoError(t, err)
	plan.Backup.PayloadEvidence = evidence
	if strings.TrimSpace(plan.PreRecoveryEvidence) == "" {
		plan.PreRecoveryEvidence = evidence
	}
}

func recoveryPlanIDFromText(t *testing.T, value string) string {
	t.Helper()
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(line, "Recovery plan: ") {
			planID := strings.TrimSpace(strings.TrimPrefix(line, "Recovery plan: "))
			require.NotEmpty(t, planID)
			return planID
		}
	}
	t.Fatalf("recovery plan ID not found in:\n%s", value)
	return ""
}

func restoreBackupCount(t *testing.T, repoRoot string) int {
	t.Helper()
	matches, err := filepath.Glob(repoRoot + ".restore-backup-*")
	require.NoError(t, err)
	nested, err := filepath.Glob(filepath.Join(repoRoot, "*.restore-backup-*"))
	require.NoError(t, err)
	return len(matches) + len(nested)
}

func assertRecoveryOutputOmitsInternalVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree", "pin", "gc", "internal"} {
		assert.False(t, regexp.MustCompile(`\b`+regexp.QuoteMeta(word)+`\b`).MatchString(lower), "output should not expose %q:\n%s", word, value)
	}
}

func assertRecoveryJSONPlanSummary(t *testing.T, data map[string]any, planID, folder, sourceID, path string) {
	t.Helper()
	assert.Equal(t, planID, data["plan_id"])
	assert.Equal(t, "active", data["status"])
	assert.Equal(t, "restore_path", data["operation"])
	assert.Equal(t, "main", data["workspace"])
	assert.Equal(t, folder, data["folder"])
	assert.Equal(t, sourceID, data["source_save_point"])
	assert.Equal(t, path, data["path"])
	assert.Equal(t, true, data["backup_available"])
	assert.Equal(t, "jvs recovery resume "+planID, data["recommended_next_command"])
}

func cleanupPreviewData(t *testing.T) map[string]any {
	t.Helper()
	stdout, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)
	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "cleanup preview", env.Command)
	return data
}

func assertCleanupFieldContains(t *testing.T, data map[string]any, field, value string) {
	t.Helper()
	values, ok := data[field].([]any)
	require.True(t, ok, "cleanup field %q should be a list: %#v", field, data[field])
	assert.Contains(t, values, value)
}

func assertCleanupFieldOmits(t *testing.T, data map[string]any, field, value string) {
	t.Helper()
	values, ok := data[field].([]any)
	require.True(t, ok, "cleanup field %q should be a list: %#v", field, data[field])
	assert.NotContains(t, values, value)
}
