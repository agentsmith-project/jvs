package gc_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorPlanProtectionGroupsExplainCommonReasons(t *testing.T) {
	repoPath := setupTestRepo(t)
	historyID := createTestSnapshot(t, repoPath)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 3)
	openViewID := ids[0]
	activeRecoveryID := ids[1]
	activeOperationID := ids[2]

	_, err := sourcepin.NewManager(repoPath).CreateWithID(openViewID, "view-"+string(openViewID), "active read-only view")
	require.NoError(t, err)
	writeActiveRecoveryPlan(t, repoPath, activeRecoveryID)
	writeSnapshotIntent(t, repoPath, activeOperationID)

	plan, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assertProtectionGroupContains(t, plan, "history", historyID)
	assertProtectionGroupContains(t, plan, "open_view", openViewID)
	assertProtectionGroupContains(t, plan, "active_recovery", activeRecoveryID)
	assertProtectionGroupContains(t, plan, "active_operation", activeOperationID)
	assert.Contains(t, plan.ProtectedSet, historyID)
	assert.Contains(t, plan.ProtectedSet, openViewID)
	assert.Contains(t, plan.ProtectedSet, activeRecoveryID)
	assert.Contains(t, plan.ProtectedSet, activeOperationID)
	assert.NotContains(t, plan.ToDelete, openViewID)
	assert.NotContains(t, plan.ToDelete, activeRecoveryID)
	assert.NotContains(t, plan.ToDelete, activeOperationID)
}

func TestCollectorRunRejectsPlanWhenProtectedSetChangedEvenIfCandidatesMatch(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Empty(t, plan.ToDelete)

	writeSnapshotIntent(t, repoPath, model.NewSnapshotID())

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protected set changed")
	assert.Contains(t, err.Error(), "cleanup preview")
}

func writeActiveRecoveryPlan(t *testing.T, repoPath string, sourceID model.SnapshotID) {
	t.Helper()

	r, err := repo.Discover(repoPath)
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 "RP-gc-protection-groups",
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 filepath.Join(repoPath, "main"),
		SourceSavePoint:        sourceID,
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: "main"},
		Backup:                 recovery.Backup{Path: filepath.Join(repoPath, "main.restore-backup-test"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		RecommendedNextCommand: "jvs recovery status RP-gc-protection-groups",
	}
	require.NoError(t, recovery.NewManager(repoPath).Write(&plan))
}
