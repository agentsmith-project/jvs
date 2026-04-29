package gc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/pkg/errclass"
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

func TestCollectorPlanProtectionGroupsDoNotExposeUnpublishedIntent(t *testing.T) {
	repoPath := setupTestRepo(t)
	unpublishedID := model.NewSnapshotID()
	writeSnapshotIntent(t, repoPath, unpublishedID)

	plan, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.NotContains(t, plan.ProtectedSet, unpublishedID)
	for _, group := range plan.ProtectionGroups {
		assert.NotContains(t, group.SavePoints, unpublishedID)
	}
}

func TestCollectorPlanActiveOperationScanFailureUsesPublicCleanupLanguage(t *testing.T) {
	repoPath := setupTestRepo(t)
	blockIntentDirectory(t, repoPath)

	_, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.Error(t, err)

	var jvsErr *errclass.JVSError
	require.ErrorAs(t, err, &jvsErr)
	assert.Equal(t, errclass.ErrGCPlanMismatch.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "active operations")
	assert.Contains(t, jvsErr.Message, "doctor --strict")
	assertCleanupActiveOperationErrorOmitsInternalVocabulary(t, jvsErr.Code)
	assertCleanupActiveOperationErrorOmitsInternalVocabulary(t, jvsErr.Message)
}

func TestCollectorRunRejectsPlanWhenPublishedIntentChangesCandidates(t *testing.T) {
	repoPath := setupTestRepo(t)
	intentID := createRemovedWorktreeSnapshots(t, repoPath, "intent-after-preview", 1)[0]

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, intentID)

	writeSnapshotIntent(t, repoPath, intentID)

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "candidate set changed")
	assert.Contains(t, err.Error(), "cleanup preview")
}

func blockIntentDirectory(t *testing.T, repoPath string) {
	t.Helper()

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	require.NoError(t, os.RemoveAll(intentsDir))
	require.NoError(t, os.WriteFile(intentsDir, []byte("not a directory"), 0644))
}

func assertCleanupActiveOperationErrorOmitsInternalVocabulary(t *testing.T, value string) {
	t.Helper()

	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"checkpoint",
		"publish state",
		"ready marker",
		"intents",
		"intent",
		".jvs",
		"control path",
		"control directory",
		"stat ",
	} {
		assert.NotContains(t, lower, forbidden)
	}
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
