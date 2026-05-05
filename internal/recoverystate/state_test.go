package recoverystate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/recoverystate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectTreatsMatchingResolvedRestoreResidueAsCompletedBeforeTargetValidation(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	plan := createStateRestorePreview(t, controlRoot, payloadRoot, "source\n")
	writeStateResolvedRecovery(t, controlRoot, plan)
	ctx := separatedStateContext(t, controlRoot)

	state, err := recoverystate.Inspect(controlRoot, "main", ctx)

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindCompletedRestoreResidue, state.Kind)
	assert.False(t, state.Blocking())
	assert.Equal(t, plan.PlanID, state.PlanID)
}

func TestInspectClassifiesForgedResolvedRecoveryResidueAsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(plan *recovery.Plan, base, payloadRoot string)
		want []string
	}{
		{
			name: "pre recovery workspace name",
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.PreWorktreeState.Name = "feature"
			},
			want: []string{"Recovery plan RP-", "cannot be inspected safely", "workspace name", "feature", "main"},
		},
		{
			name: "pre recovery workspace root",
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.PreWorktreeState.RealPath = filepath.Join(base, "other-payload")
			},
			want: []string{"Recovery plan RP-", "cannot be inspected safely", "workspace root identity mismatch"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlRoot, payloadRoot := setupSeparatedStateRepo(t)
			plan := createStateRestorePreview(t, controlRoot, payloadRoot, "source\n")
			resolved := newStateRecoveryPlan(t, controlRoot, "RP-"+plan.PlanID, recovery.StatusResolved, plan.PlanID, plan.SourceSavePoint, plan.Folder)
			tc.edit(resolved, filepath.Dir(controlRoot), payloadRoot)
			require.NoError(t, recovery.NewManager(controlRoot).Write(resolved))

			state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

			require.NoError(t, err)
			assert.Equal(t, recoverystate.KindMalformedBlocking, state.Kind)
			assert.True(t, state.Blocking())
			assert.Equal(t, "RP-"+plan.PlanID, state.RecoveryPlanID)
			assert.Empty(t, state.PlanID)
			assert.Equal(t, "doctor --strict --json", state.NextCommand)
			for _, want := range tc.want {
				assert.Contains(t, state.Message, want)
			}
			assert.NotContains(t, state.Message, "completed")
			assert.NotContains(t, strings.ToLower(state.Message), "payload")
		})
	}
}

func TestInspectPrioritizesMatchingActiveRecoveryOverRestoreResidue(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	plan := createStateRestorePreview(t, controlRoot, payloadRoot, "source\n")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	createStateSavePoint(t, controlRoot, "current")
	writeStateRecovery(t, controlRoot, "RP-"+plan.PlanID, recovery.StatusActive, plan.PlanID, plan.SourceSavePoint, plan.Folder)

	state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindActiveRecovery, state.Kind)
	assert.True(t, state.Blocking())
	assert.Equal(t, "RP-"+plan.PlanID, state.RecoveryPlanID)
	assert.Empty(t, state.PlanID)
	assert.Equal(t, "recovery status RP-"+plan.PlanID, state.NextCommand)
	assert.NotContains(t, state.Message, "restore --run")
}

func TestInspectClassifiesPendingRestorePreview(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	source := createStateSavePoint(t, controlRoot, "source")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("current\n"), 0644))
	createStateSavePoint(t, controlRoot, "current")
	plan, err := restoreplan.Create(controlRoot, "main", source, model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)

	state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindPendingRestorePreview, state.Kind)
	assert.True(t, state.Blocking())
	assert.Equal(t, plan.PlanID, state.PlanID)
	assert.Equal(t, "restore --run "+plan.PlanID, state.NextCommand)
	assert.NotContains(t, state.Message, "Run: jvs")
	assert.NotContains(t, state.Message, "restore --run "+plan.PlanID)
}

func TestInspectClassifiesStaleRestorePreviewWithDiscardCommand(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	plan := createStateRestorePreview(t, controlRoot, payloadRoot, "source\n")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("stale local change\n"), 0644))

	state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindStaleRestorePreview, state.Kind)
	assert.True(t, state.Blocking())
	assert.Equal(t, plan.PlanID, state.PlanID)
	assert.Equal(t, "restore discard "+plan.PlanID, state.NextCommand)
	assert.NotContains(t, state.Message, "Run: jvs")
	assert.NotContains(t, state.Message, "restore discard "+plan.PlanID)
	assert.NotContains(t, state.Message, ".jvs/restore-plans")
}

func TestInspectClassifiesRestorePreviewAfterSaveDriftAsStale(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	plan := createStateRestorePreview(t, controlRoot, payloadRoot, "source\n")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("saved after preview\n"), 0644))
	createStateSavePoint(t, controlRoot, "saved after preview")

	state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindStaleRestorePreview, state.Kind)
	assert.True(t, state.Blocking())
	assert.Equal(t, plan.PlanID, state.PlanID)
	assert.Equal(t, "restore discard "+plan.PlanID, state.NextCommand)
	assert.NotContains(t, state.NextCommand, "restore --run")
}

func TestInspectClassifiesActiveRecovery(t *testing.T) {
	controlRoot, payloadRoot := setupSeparatedStateRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
	source := createStateSavePoint(t, controlRoot, "source")
	writeStateRecovery(t, controlRoot, "RP-active", recovery.StatusActive, "restore-preview", source, payloadRoot)

	state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

	require.NoError(t, err)
	assert.Equal(t, recoverystate.KindActiveRecovery, state.Kind)
	assert.True(t, state.Blocking())
	assert.Equal(t, "RP-active", state.RecoveryPlanID)
	assert.Equal(t, "recovery status RP-active", state.NextCommand)
	assert.NotContains(t, state.Message, "Run: jvs")
	assert.NotContains(t, state.Message, "recovery status RP-active")
}

func TestInspectClassifiesActiveRecoveryIdentityMismatchAsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(plan *recovery.Plan, base, payloadRoot string)
		want []string
	}{
		{
			name: "workspace selector",
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.Workspace = "feature"
			},
			want: []string{"Recovery plan RP-active cannot be inspected safely", "workspace", "feature", "main"},
		},
		{
			name: "pre recovery workspace name",
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.PreWorktreeState.Name = "feature"
			},
			want: []string{"Recovery plan RP-active cannot be inspected safely", "workspace name", "feature", "main"},
		},
		{
			name: "workspace root",
			edit: func(plan *recovery.Plan, base, payloadRoot string) {
				plan.Folder = filepath.Join(base, "other-payload")
				plan.PreWorktreeState.RealPath = plan.Folder
			},
			want: []string{"Recovery plan RP-active cannot be inspected safely", "workspace folder", "changed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlRoot, payloadRoot := setupSeparatedStateRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source\n"), 0644))
			source := createStateSavePoint(t, controlRoot, "source")
			plan := newStateRecoveryPlan(t, controlRoot, "RP-active", recovery.StatusActive, "restore-preview", source, payloadRoot)
			tc.edit(plan, filepath.Dir(controlRoot), payloadRoot)
			require.NoError(t, recovery.NewManager(controlRoot).Write(plan))

			state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

			require.NoError(t, err)
			assert.Equal(t, recoverystate.KindMalformedBlocking, state.Kind)
			assert.True(t, state.Blocking())
			assert.Equal(t, "RP-active", state.RecoveryPlanID)
			assert.Equal(t, "doctor --strict --json", state.NextCommand)
			for _, want := range tc.want {
				assert.Contains(t, state.Message, want)
			}
			assert.NotContains(t, strings.ToLower(state.Message), "payload")
		})
	}
}

func TestInspectClassifiesMalformedRecoveryPlans(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, controlRoot string)
		want []string
	}{
		{
			name: "malformed",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", "recovery-plans"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "recovery-plans", "RP-corrupt.json"), []byte("{not-json\n"), 0644))
			},
			want: []string{"Recovery state cannot be inspected safely", "recovery plan \"RP-corrupt\"", "not valid JSON"},
		},
		{
			name: "symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", "recovery-plans"), 0755))
				target := filepath.Join(controlRoot, ".jvs", "repo_id")
				if err := os.Symlink(target, filepath.Join(controlRoot, ".jvs", "recovery-plans", "RP-link.json")); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Recovery state cannot be inspected safely", "recovery plan \"RP-link.json\"", "symlink"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlRoot, _ := setupSeparatedStateRepo(t)
			tc.seed(t, controlRoot)

			state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

			require.NoError(t, err)
			assert.Equal(t, recoverystate.KindMalformedBlocking, state.Kind)
			assert.True(t, state.Blocking())
			assert.Equal(t, "doctor --strict --json", state.NextCommand)
			assert.NotContains(t, state.Message, "Run: jvs")
			for _, want := range tc.want {
				assert.Contains(t, state.Message, want)
			}
			assert.NotContains(t, state.Message, ".jvs/recovery-plans")
			assert.NotContains(t, strings.ToLower(state.Message), "payload")
		})
	}
}

func TestInspectClassifiesMalformedAndSymlinkRestorePlans(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, controlRoot string)
		want []string
	}{
		{
			name: "malformed",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "corrupt-pending.json"), []byte("{not-json\n"), 0644))
			},
			want: []string{"Restore plan corrupt-pending", "not valid JSON"},
		},
		{
			name: "symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				target := filepath.Join(controlRoot, ".jvs", "restore-plans", "restore-target.json")
				require.NoError(t, os.WriteFile(target, []byte("{}\n"), 0644))
				if err := os.Symlink(target, filepath.Join(controlRoot, ".jvs", "restore-plans", "restore-link.json")); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Restore plan entry restore-link.json", "symlink"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlRoot, _ := setupSeparatedStateRepo(t)
			tc.seed(t, controlRoot)

			state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

			require.NoError(t, err)
			assert.Equal(t, recoverystate.KindMalformedBlocking, state.Kind)
			assert.True(t, state.Blocking())
			assert.NotContains(t, state.Message, "Run: jvs")
			for _, want := range tc.want {
				assert.Contains(t, state.Message, want)
			}
			assert.NotContains(t, state.Message, ".jvs/restore-plans")
		})
	}
}

func TestInspectClassifiesUnsafeRestorePlansDirectory(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T, controlRoot string)
		want []string
	}{
		{
			name: "directory symlink",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				restorePlansDir := filepath.Join(controlRoot, ".jvs", "restore-plans")
				require.NoError(t, os.RemoveAll(restorePlansDir))
				outsideDir := filepath.Join(filepath.Dir(controlRoot), "outside-restore-plans")
				require.NoError(t, os.MkdirAll(outsideDir, 0755))
				if err := os.Symlink(outsideDir, restorePlansDir); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			want: []string{"Restore plans directory cannot be inspected safely", "symlink"},
		},
		{
			name: "regular file",
			seed: func(t *testing.T, controlRoot string) {
				t.Helper()
				restorePlansDir := filepath.Join(controlRoot, ".jvs", "restore-plans")
				require.NoError(t, os.RemoveAll(restorePlansDir))
				require.NoError(t, os.WriteFile(restorePlansDir, []byte("not a directory\n"), 0644))
			},
			want: []string{"Restore plans directory cannot be inspected safely", "not a directory"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controlRoot, _ := setupSeparatedStateRepo(t)
			tc.seed(t, controlRoot)

			state, err := recoverystate.Inspect(controlRoot, "main", separatedStateContext(t, controlRoot))

			require.NoError(t, err)
			assert.Equal(t, recoverystate.KindMalformedBlocking, state.Kind)
			assert.True(t, state.Blocking())
			assert.Equal(t, "doctor --strict --json", state.NextCommand)
			for _, want := range tc.want {
				assert.Contains(t, state.Message, want)
			}
			assert.NotContains(t, state.Message, ".jvs/restore-plans")
		})
	}
}

func setupSeparatedStateRepo(t *testing.T) (controlRoot, payloadRoot string) {
	t.Helper()

	base := t.TempDir()
	controlRoot = filepath.Join(base, "control")
	payloadRoot = filepath.Join(base, "payload")
	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	return controlRoot, payloadRoot
}

func createStateRestorePreview(t *testing.T, controlRoot, payloadRoot, content string) *restoreplan.Plan {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte(content), 0644))
	source := createStateSavePoint(t, controlRoot, "source")
	plan, err := restoreplan.Create(controlRoot, "main", source, model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	return plan
}

func createStateSavePoint(t *testing.T, controlRoot, message string) model.SnapshotID {
	t.Helper()

	desc, err := snapshot.NewCreator(controlRoot, model.EngineCopy).CreateSavePoint("main", message, nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func writeStateResolvedRecovery(t *testing.T, controlRoot string, restorePlan *restoreplan.Plan) {
	t.Helper()
	writeStateRecovery(t, controlRoot, "RP-"+restorePlan.PlanID, recovery.StatusResolved, restorePlan.PlanID, restorePlan.SourceSavePoint, restorePlan.Folder)
}

func writeStateRecovery(t *testing.T, controlRoot, planID string, status recovery.Status, restorePlanID string, source model.SnapshotID, folder string) {
	t.Helper()

	plan := newStateRecoveryPlan(t, controlRoot, planID, status, restorePlanID, source, folder)
	require.NoError(t, recovery.NewManager(controlRoot).Write(plan))
}

func newStateRecoveryPlan(t *testing.T, controlRoot, planID string, status recovery.Status, restorePlanID string, source model.SnapshotID, folder string) *recovery.Plan {
	t.Helper()

	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 status,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          restorePlanID,
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        source,
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: folder},
		Backup:                 recovery.Backup{Path: filepath.Join(filepath.Dir(folder), "restore-backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRolledBack},
		Phase:                  recovery.PhaseRestoreApplied,
		RecommendedNextCommand: "jvs recovery status " + planID,
	}
	if status == recovery.StatusResolved {
		plan.ResolvedAt = &now
	}
	return plan
}

func separatedStateContext(t *testing.T, controlRoot string) *repo.SeparatedContext {
	t.Helper()

	ctx, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{ControlRoot: controlRoot, Workspace: "main"})
	require.NoError(t, err)
	return ctx
}
