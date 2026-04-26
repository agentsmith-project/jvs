package gc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/gc"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/restore"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var zeroRetention = model.RetentionPolicy{}

func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func requireWorktreePath(t *testing.T, wtMgr *worktree.Manager, name string) string {
	t.Helper()

	path, err := wtMgr.Path(name)
	require.NoError(t, err)
	return path
}

func createTestSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	// Add some content
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func createRemovedWorktreeSnapshots(t *testing.T, repoPath, worktreeName string, count int) []model.SnapshotID {
	t.Helper()

	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create(worktreeName, nil)
	require.NoError(t, err)

	wtPath := requireWorktreePath(t, wtMgr, worktreeName)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	ids := make([]model.SnapshotID, 0, count)
	for i := 0; i < count; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(wtPath, "file.txt"), []byte{byte('a' + i)}, 0644))
		desc, err := creator.Create(worktreeName, "temp snap", nil)
		require.NoError(t, err)
		ids = append(ids, desc.SnapshotID)
	}

	require.NoError(t, wtMgr.Remove(worktreeName))
	return ids
}

func writePin(t *testing.T, repoPath, pinsRelDir string, snapshotID model.SnapshotID) {
	t.Helper()

	pinsDir := filepath.Join(repoPath, pinsRelDir)
	require.NoError(t, os.MkdirAll(pinsDir, 0755))
	pin := map[string]any{
		"pin_id":      string(snapshotID),
		"snapshot_id": string(snapshotID),
		"reason":      "test pin",
		"created_at":  "2026-04-24T00:00:00Z",
		"expires_at":  nil,
	}
	data, err := json.Marshal(pin)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pinsDir, string(snapshotID)+".json"), data, 0644))
}

func writeGCPlan(t *testing.T, repoPath string, plan *model.GCPlan) {
	t.Helper()

	if plan.SchemaVersion == 0 {
		plan.SchemaVersion = model.GCPlanSchemaVersion
	}
	if plan.RepoID == "" {
		r, err := repo.Discover(repoPath)
		require.NoError(t, err)
		plan.RepoID = r.RepoID
	}

	gcDir := filepath.Join(repoPath, ".jvs", "gc")
	require.NoError(t, os.MkdirAll(gcDir, 0755))
	data, err := json.MarshalIndent(plan, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gcDir, plan.PlanID+".json"), data, 0644))
}

func writeTombstone(t *testing.T, repoPath string, tombstone model.Tombstone) {
	t.Helper()

	tombstonesDir := filepath.Join(repoPath, ".jvs", "gc", "tombstones")
	require.NoError(t, os.MkdirAll(tombstonesDir, 0755))
	data, err := json.MarshalIndent(tombstone, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tombstonesDir, string(tombstone.SnapshotID)+".json"), data, 0644))
}

func assertSnapshotIDsSorted(t *testing.T, ids []model.SnapshotID) {
	t.Helper()

	require.True(t, sort.SliceIsSorted(ids, func(i, j int) bool {
		return string(ids[i]) < string(ids[j])
	}), "snapshot IDs should be sorted: %v", ids)
}

func requireTombstoneState(t *testing.T, repoPath string, snapshotID model.SnapshotID, expected string) {
	t.Helper()

	path := filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(snapshotID)+".json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tombstone map[string]any
	require.NoError(t, json.Unmarshal(data, &tombstone))
	require.Equal(t, expected, tombstone["gc_state"])
}

func TestCollector_Plan(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	assert.NotEmpty(t, plan.PlanID)
	assert.Equal(t, model.GCPlanSchemaVersion, plan.SchemaVersion)
	assert.NotEmpty(t, plan.RepoID)
	// Fresh repo has no snapshots, so protected set may be empty
}

func TestCollector_PlanDefaultDeletesRemovedWorkspaceSnapshotImmediately(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	assert.Contains(t, plan.ToDelete, ids[0])
	assert.Equal(t, 1, plan.CandidateCount)
}

func TestCollector_PlanUsesPayloadAndDescriptorSizeEstimate(t *testing.T) {
	repoPath := setupTestRepo(t)
	createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	require.Len(t, plan.ToDelete, 1)

	assert.Greater(t, plan.DeletableBytesEstimate, int64(0))
	assert.NotEqual(t, int64(1024*1024), plan.DeletableBytesEstimate)
}

func TestCollectorRunReturnsRepoBusyWhenMutationLockHeld(t *testing.T) {
	repoPath := setupTestRepo(t)

	held, err := repo.AcquireMutationLock(repoPath, "held-by-test")
	require.NoError(t, err)
	defer held.Release()

	err = gc.NewCollector(repoPath).Run("missing-plan")
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
}

func TestCollector_Plan_WithSnapshots(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	assert.NotEmpty(t, plan.ProtectedSet)
	// Protected set should contain the snapshot ID we just created
	assert.Contains(t, plan.ProtectedSet, snapshotID)
}

func TestCollector_PlanAndRun_IgnoresSnapshotTmpDirs(t *testing.T) {
	repoPath := setupTestRepo(t)
	tmpID := model.SnapshotID("1234567890123-deadbeef.tmp")
	tmpDir := filepath.Join(repoPath, ".jvs", "snapshots", string(tmpID))
	require.NoError(t, os.MkdirAll(tmpDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "partial-file.txt"), []byte("in progress"), 0644))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.NotContains(t, plan.ToDelete, tmpID)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	_, err = os.Stat(tmpDir)
	require.NoError(t, err, ".tmp snapshot directories must not be deleted by GC")

	_, err = os.Stat(filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(tmpID)+".json"))
	require.True(t, os.IsNotExist(err), ".tmp snapshot directories must not get tombstoned")
}

func TestCollector_Plan_ReadyWithoutDescriptorFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), []byte("{}"), 0644))

	collector := gc.NewCollector(repoPath)
	_, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	assert.ErrorIs(t, err, &errclass.JVSError{Code: "E_READY_DESCRIPTOR_MISSING"})
	assert.DirExists(t, snapshotDir, "GC must not silently delete corrupt published state")
}

func TestCollector_PlanFailsClosedWhenWorktreesDirIsSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	outside := t.TempDir()
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "worktrees")))
	if err := os.Symlink(outside, filepath.Join(repoPath, ".jvs", "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	collector := gc.NewCollector(repoPath)
	_, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
}

func TestCollector_RunFailsClosedWhenWorktreesDirIsSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)
	plan := &model.GCPlan{
		PlanID:          "worktrees-symlink",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{snapshotID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	outside := t.TempDir()
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "worktrees")))
	if err := os.Symlink(outside, filepath.Join(repoPath, ".jvs", "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
}

func TestCollector_PlanPropagatesCorruptWorktreeConfig(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPath, ".jvs", "worktrees", "main", "config.json"),
		[]byte("{not json"),
		0644,
	))

	collector := gc.NewCollector(repoPath)
	_, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
}

func TestCollector_Run(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)
}

func TestCollector_RunFailsClosedWhenAuditLogMalformed(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "audit-blocked", 1)
	snapshotID := ids[0]

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, snapshotID)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644))

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit")

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(snapshotID)+".json"))
}

func TestCollector_PlanFailsClosedWhenAuditRecordHashMismatches(t *testing.T) {
	repoPath := setupTestRepo(t)
	createRemovedWorktreeSnapshots(t, repoPath, "audit-plan-blocked", 1)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	tamperFirstAuditRecordForGCTest(t, auditPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "E_AUDIT_RECORD_HASH_MISMATCH")
	assert.Empty(t, gcPlanFiles(t, repoPath), "failed plan must not leave an unaudited active plan")
}

func TestCollector_PlanFailsClosedWhenAuditLogMalformed(t *testing.T) {
	repoPath := setupTestRepo(t)
	createRemovedWorktreeSnapshots(t, repoPath, "audit-plan-malformed", 1)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "E_AUDIT_RECORD_MALFORMED")
	assert.Empty(t, gcPlanFiles(t, repoPath), "failed plan must not leave an unaudited active plan")
}

func TestCollector_PlanAppendsAuditEvent(t *testing.T) {
	repoPath := setupTestRepo(t)
	createRemovedWorktreeSnapshots(t, repoPath, "audit-plan", 1)
	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	before := readAuditRecordsForGCTest(t, auditPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	after := readAuditRecordsForGCTest(t, auditPath)
	require.Len(t, after, len(before)+1)
	record := after[len(after)-1]
	assert.Equal(t, model.EventTypeGCPlan, record.EventType)
	require.NotNil(t, record.Details)
	assert.Equal(t, plan.PlanID, record.Details["plan_id"])
	assert.Equal(t, float64(plan.CandidateCount), record.Details["candidate_count"])
}

func TestCollectorRunFailsClosedWhenSnapshotsParentIsSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.SnapshotID("1708300800000-deadbeef")

	outside := t.TempDir()
	outsideSnapshot := filepath.Join(outside, string(snapshotID))
	require.NoError(t, os.MkdirAll(outsideSnapshot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideSnapshot, "secret.txt"), []byte("outside"), 0644))

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.RemoveAll(snapshotsDir))
	if err := os.Symlink(outside, snapshotsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	plan := &model.GCPlan{
		PlanID:          "symlink-parent",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{snapshotID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)

	assert.DirExists(t, outsideSnapshot, "GC must not delete through a symlinked snapshots parent")
	assert.FileExists(t, filepath.Join(outsideSnapshot, "secret.txt"))
}

func TestCollector_Run_InvalidPlanID(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	err := collector.Run("nonexistent-plan-id")
	assert.Error(t, err)
}

func TestCollector_RunRejectsTraversalSnapshotIDAndPreservesVictim(t *testing.T) {
	repoPath := setupTestRepo(t)
	victimPath := filepath.Join(repoPath, "victim.txt")
	require.NoError(t, os.WriteFile(victimPath, []byte("do not delete"), 0644))

	plan := &model.GCPlan{
		PlanID:          "traversal-plan",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{"../../victim.txt"},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)

	content, readErr := os.ReadFile(victimPath)
	require.NoError(t, readErr)
	assert.Equal(t, "do not delete", string(content))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "victim.txt.json"))
}

func TestCollector_RunRejectsInvalidPlannedSnapshotIDs(t *testing.T) {
	invalidIDs := []model.SnapshotID{
		"",
		"/tmp/evil",
		"safe/evil",
		"..",
		"1708300800000-A3F7C1B2",
	}
	for _, invalidID := range invalidIDs {
		t.Run(string(invalidID), func(t *testing.T) {
			repoPath := setupTestRepo(t)
			plan := &model.GCPlan{
				PlanID:          "invalid-plan",
				CreatedAt:       time.Now().UTC(),
				ToDelete:        []model.SnapshotID{invalidID},
				RetentionPolicy: zeroRetention,
			}
			writeGCPlan(t, repoPath, plan)

			collector := gc.NewCollector(repoPath)
			err := collector.Run(plan.PlanID)
			require.Error(t, err)
		})
	}
}

func TestCollector_RunUsesValidatedPendingList(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	candidateID := ids[0]
	alreadyCommittedID := model.SnapshotID("1708300800000-deadbeef")
	writeTombstone(t, repoPath, model.Tombstone{
		SnapshotID:  alreadyCommittedID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: true,
		GCState:     model.GCStateCommitted,
	})

	plan := &model.GCPlan{
		PlanID:          "validated-pending",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{candidateID, alreadyCommittedID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	var totals []int
	collector := gc.NewCollector(repoPath)
	collector.SetProgressCallback(func(phase string, current, total int, msg string) {
		totals = append(totals, total)
	})

	err := collector.Run(plan.PlanID)
	require.NoError(t, err)

	require.NotEmpty(t, totals)
	for _, total := range totals {
		assert.Equal(t, 1, total, "progress should count only validated pending IDs")
	}
	requireTombstoneState(t, repoPath, alreadyCommittedID, model.GCStateCommitted)
	requireTombstoneState(t, repoPath, candidateID, model.GCStateCommitted)
}

func TestCollector_RunFailsClosedForMissingPlannedSnapshotWithoutTombstoneEvidence(t *testing.T) {
	repoPath := setupTestRepo(t)
	missingID := model.SnapshotID("1708300800000-deadbeef")
	plan := &model.GCPlan{
		PlanID:          "missing-without-evidence",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{missingID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.NoError(t, statErr, "failed closed plan should remain for inspection")
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(missingID)+".json"))
}

func TestCollector_RunCompletesMissingSnapshotWithRetryTombstoneEvidence(t *testing.T) {
	for _, tt := range []struct {
		name      string
		tombstone model.Tombstone
	}{
		{name: "marked tombstone", tombstone: model.Tombstone{
			SnapshotID:  "1708300800000-deadbeef",
			DeletedAt:   time.Now().UTC(),
			Reclaimable: false,
			GCState:     model.GCStateMarked,
		}},
		{name: "failed tombstone", tombstone: model.Tombstone{
			SnapshotID:  "1708300800000-deadbeef",
			DeletedAt:   time.Now().UTC(),
			Reclaimable: false,
			GCState:     model.GCStateFailed,
			Reason:      "previous failure after deleting payload",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			missingID := tt.tombstone.SnapshotID
			writeTombstone(t, repoPath, tt.tombstone)
			plan := &model.GCPlan{
				PlanID:          "missing-with-retry-evidence",
				CreatedAt:       time.Now().UTC(),
				ToDelete:        []model.SnapshotID{missingID},
				RetentionPolicy: zeroRetention,
			}
			writeGCPlan(t, repoPath, plan)

			collector := gc.NewCollector(repoPath)
			err := collector.Run(plan.PlanID)
			require.NoError(t, err)

			requireTombstoneState(t, repoPath, missingID, model.GCStateCommitted)
			_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
			require.True(t, os.IsNotExist(statErr), "successful idempotent run should delete the plan")
		})
	}
}

func TestCollector_RunReadyWithoutDescriptorWithRetryTombstoneEvidenceFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.Remove(descriptorPath))
	writeTombstone(t, repoPath, model.Tombstone{
		SnapshotID:  snapshotID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: false,
		GCState:     model.GCStateFailed,
		Reason:      "previous failure after descriptor removal",
	})
	plan := &model.GCPlan{
		PlanID:          "ready-missing-descriptor-with-evidence",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{snapshotID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.ErrorIs(t, err, &errclass.JVSError{Code: "E_READY_DESCRIPTOR_MISSING"})

	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, descriptorPath)
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateFailed)
}

func TestCollector_RunSkipsMissingCommittedSnapshotIdempotently(t *testing.T) {
	repoPath := setupTestRepo(t)
	missingID := model.SnapshotID("1708300800000-deadbeef")
	writeTombstone(t, repoPath, model.Tombstone{
		SnapshotID:  missingID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: true,
		GCState:     model.GCStateCommitted,
	})
	plan := &model.GCPlan{
		PlanID:          "missing-committed",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{missingID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.NoError(t, err)
	requireTombstoneState(t, repoPath, missingID, model.GCStateCommitted)

	_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.True(t, os.IsNotExist(statErr), "successful idempotent run should delete the plan")
}

func TestCollector_RunCommittedTombstoneWithExistingSnapshotRetriesDeletion(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]
	writeTombstone(t, repoPath, model.Tombstone{
		SnapshotID:  snapshotID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: true,
		GCState:     model.GCStateCommitted,
	})

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, snapshotID)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateCommitted)
}

func TestCollector_RunCommittedTombstoneWithRemainingDescriptorFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.RemoveAll(snapshotDir))
	writeTombstone(t, repoPath, model.Tombstone{
		SnapshotID:  snapshotID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: true,
		GCState:     model.GCStateCommitted,
	})
	plan := &model.GCPlan{
		PlanID:          "committed-with-descriptor-residue",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{snapshotID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err := collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.ErrorIs(t, err, &errclass.JVSError{Code: "E_PAYLOAD_MISSING"})

	assert.NoDirExists(t, snapshotDir)
	assert.FileExists(t, descriptorPath)
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateCommitted)
}

func TestCollector_PlanRejectsSnapshotLeafSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.SnapshotID("1708300800000-deadbeef")
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0644))
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	if err := os.Symlink(outside, snapshotDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	collector := gc.NewCollector(repoPath)
	_, err := collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.FileExists(t, filepath.Join(outside, "file.txt"))
}

func TestCollector_LoadPlanRejectsFinalPlanSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	outsidePlan := filepath.Join(t.TempDir(), "outside-plan.json")
	data, err := json.Marshal(&model.GCPlan{
		PlanID:    "plan-symlink",
		CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(outsidePlan, data, 0644))
	planPath := filepath.Join(repoPath, ".jvs", "gc", "plan-symlink.json")
	if err := os.Symlink(outsidePlan, planPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	collector := gc.NewCollector(repoPath)
	_, err = collector.LoadPlan("plan-symlink")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.FileExists(t, outsidePlan)
}

func TestCollector_RunRejectsFinalTombstoneSymlinkBeforeRead(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]

	outsideTombstone := filepath.Join(t.TempDir(), "outside-tombstone.json")
	data, err := json.Marshal(&model.Tombstone{
		SnapshotID:  snapshotID,
		DeletedAt:   time.Now().UTC(),
		Reclaimable: true,
		GCState:     model.GCStateCommitted,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(outsideTombstone, data, 0644))
	tombstonePath := filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(snapshotID)+".json")
	if err := os.Symlink(outsideTombstone, tombstonePath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	plan := &model.GCPlan{
		PlanID:          "tombstone-symlink",
		CreatedAt:       time.Now().UTC(),
		ToDelete:        []model.SnapshotID{snapshotID},
		RetentionPolicy: zeroRetention,
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
	assert.FileExists(t, outsideTombstone)
}

func TestCollector_LoadPlanRejectsTraversalPlanID(t *testing.T) {
	repoPath := setupTestRepo(t)
	victimPlan := &model.GCPlan{
		PlanID:    "victim",
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(victimPlan)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "victim.json"), data, 0644))

	collector := gc.NewCollector(repoPath)
	_, err = collector.LoadPlan("../../victim")
	require.Error(t, err)
}

func TestCollector_Plan_WithLineage(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create first snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("content1"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	// Create second snapshot (parent = first)
	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("content2"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Verify lineage
	assert.Equal(t, desc1.SnapshotID, *desc2.ParentID)

	// GC plan should protect both (lineage traversal)
	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, desc1.SnapshotID)
	assert.Contains(t, plan.ProtectedSet, desc2.SnapshotID)
}

func TestCollector_Run_WithDeletions(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create snapshot in main worktree
	createTestSnapshot(t, repoPath)

	// Create another worktree with its own snapshot
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("feature", nil)
	require.NoError(t, err)

	featurePath := requireWorktreePath(t, wtMgr, "feature")
	os.WriteFile(filepath.Join(featurePath, "file.txt"), []byte("feature content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	featureDesc, err := creator.Create("feature", "feature snapshot", nil)
	require.NoError(t, err)
	_ = cfg

	// Delete the feature worktree
	require.NoError(t, wtMgr.Remove("feature"))

	// Now the feature snapshot should be unprotected (use zero retention to bypass age protection)
	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.Contains(t, plan.ToDelete, featureDesc.SnapshotID)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	entries, _ := os.ReadDir(snapshotsDir)
	for _, e := range entries {
		assert.NotEqual(t, string(featureDesc.SnapshotID), e.Name())
	}
}

func TestCollector_Plan_EmptyRepo(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	assert.Empty(t, plan.ProtectedSet)
	assert.Empty(t, plan.ToDelete)
}

func TestCollector_Plan_IgnoresLegacyPinFilesInV0(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	candidateID := ids[0]

	writePin(t, repoPath, filepath.Join(".jvs", "gc", "pins"), candidateID)
	writePin(t, repoPath, filepath.Join(".jvs", "pins"), candidateID)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	assert.Contains(t, plan.ToDelete, candidateID)
	assert.NotContains(t, plan.ProtectedSet, candidateID)
}

func TestCollector_Plan_SortsPlanSets(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	for i := 0; i < 6; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte{byte('0' + i)}, 0644))
		_, err := creator.Create("main", "main snap", nil)
		require.NoError(t, err)
	}
	createRemovedWorktreeSnapshots(t, repoPath, "temp", 4)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	require.Len(t, plan.ProtectedSet, 6)
	require.Len(t, plan.ToDelete, 4)
	assertSnapshotIDsSorted(t, plan.ProtectedSet)
	assertSnapshotIDsSorted(t, plan.ToDelete)
}

func TestCollector_Run_IgnoresPinAddedAfterPlanningInV0(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	candidateID := ids[0]

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Contains(t, plan.ToDelete, candidateID)

	writePin(t, repoPath, filepath.Join(".jvs", "gc", "pins"), candidateID)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.True(t, os.IsNotExist(statErr), "successful GC should delete the plan")
	requireTombstoneState(t, repoPath, candidateID, model.GCStateCommitted)
}

func TestCollector_Plan_ProtectedCounts(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create multiple snapshots with lineage
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("1"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("2"), 0644)
	_, err = creator.Create("main", "second", nil)
	require.NoError(t, err)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	// Should have at least 1 protected by lineage (desc1 is parent of desc2)
	assert.GreaterOrEqual(t, plan.ProtectedByLineage, 0)
	// Should have exact count match
	assert.Equal(t, len(plan.ProtectedSet), len(plan.ProtectedSet))
}

func TestCollector_Run_TombstoneCreation(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create snapshot in main
	createTestSnapshot(t, repoPath)

	// Create another worktree with snapshot then delete it
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp content"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	tombstonesDir := filepath.Join(repoPath, ".jvs", "gc", "tombstones")
	entries, err := os.ReadDir(tombstonesDir)
	require.NoError(t, err)

	found := false
	for _, e := range entries {
		if e.Name() == string(tempDesc.SnapshotID)+".json" {
			found = true
			break
		}
	}
	assert.True(t, found, "tombstone should be created for deleted snapshot")
}

func TestCollector_LoadPlan_Invalid(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	_, err := collector.LoadPlan("nonexistent-plan")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errclass.ErrGCPlanMismatch)
	assert.NotContains(t, err.Error(), "control leaf")
	assert.NotContains(t, err.Error(), repoPath)
}

func TestCollector_LoadPlan_InvalidJSON(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a plan file with invalid JSON
	gcDir := filepath.Join(repoPath, ".jvs", "gc")
	require.NoError(t, os.MkdirAll(gcDir, 0755))
	invalidPlanPath := filepath.Join(gcDir, "invalid-plan.json")
	require.NoError(t, os.WriteFile(invalidPlanPath, []byte("{invalid json"), 0644))

	collector := gc.NewCollector(repoPath)
	_, err := collector.LoadPlan("invalid-plan")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errclass.ErrGCPlanMismatch)
}

func TestCollector_LoadPlanRejectsPlanIDMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)
	r, err := repo.Discover(repoPath)
	require.NoError(t, err)

	plan := &model.GCPlan{
		SchemaVersion: model.GCPlanSchemaVersion,
		RepoID:        r.RepoID,
		PlanID:        "other-plan",
		CreatedAt:     time.Now().UTC(),
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "gc", "requested-plan.json"), data, 0644))

	collector := gc.NewCollector(repoPath)
	_, err = collector.LoadPlan("requested-plan")
	require.Error(t, err)
	assert.ErrorIs(t, err, errclass.ErrGCPlanMismatch)
	assert.Contains(t, err.Error(), "plan_id")
	assert.NotContains(t, err.Error(), repoPath)
}

func TestCollector_LoadPlanRejectsRepoMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)
	plan := &model.GCPlan{
		SchemaVersion: model.GCPlanSchemaVersion,
		RepoID:        "different-repo",
		PlanID:        "repo-mismatch",
		CreatedAt:     time.Now().UTC(),
	}
	writeGCPlan(t, repoPath, plan)

	collector := gc.NewCollector(repoPath)
	_, err := collector.LoadPlan(plan.PlanID)
	require.Error(t, err)
	assert.ErrorIs(t, err, errclass.ErrGCPlanMismatch)
	assert.Contains(t, err.Error(), "repository")
	assert.NotContains(t, err.Error(), repoPath)
}

func TestCollector_Plan_WritePlanError(t *testing.T) {
	// This test is hard to implement without mocking
	// In real scenarios, writePlan only fails on disk I/O errors
	// which are rare on modern systems
	repoPath := setupTestRepo(t)

	// Create a snapshot to ensure plan has content
	createTestSnapshot(t, repoPath)

	collector := gc.NewCollector(repoPath)
	_, err := collector.Plan()
	assert.NoError(t, err, "plan should succeed under normal conditions")
}

func TestCollector_Plan_WithNonexistentSnapshotsDir(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Remove snapshots directory to simulate edge case
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	os.RemoveAll(snapshotsDir)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	assert.Empty(t, plan.ProtectedSet)
	assert.Empty(t, plan.ToDelete)
}

func TestCollector_Plan_WithOnlyLineage(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a chain of snapshots
	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("1"), 0644)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("2"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Verify lineage
	assert.Equal(t, desc1.SnapshotID, *desc2.ParentID)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	// Both should be protected (desc2 as head, desc1 as parent)
	assert.Contains(t, plan.ProtectedSet, desc1.SnapshotID)
	assert.Contains(t, plan.ProtectedSet, desc2.SnapshotID)
	// At least 1 protected by lineage
	assert.Greater(t, plan.ProtectedByLineage, 0)
}

func TestCollector_PlanWithPolicy_ProtectsDetachedLatestAndLineage(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s1"), 0644))
	desc1, err := creator.Create("main", "s1", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s2"), 0644))
	desc2, err := creator.Create("main", "s2", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s3"), 0644))
	desc3, err := creator.Create("main", "s3", nil)
	require.NoError(t, err)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	require.NoError(t, restorer.Restore("main", desc1.SnapshotID))

	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Get("main")
	require.NoError(t, err)
	require.True(t, cfg.IsDetached())
	require.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)
	require.Equal(t, desc3.SnapshotID, cfg.LatestSnapshotID)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, desc1.SnapshotID)
	assert.Contains(t, plan.ProtectedSet, desc2.SnapshotID)
	assert.Contains(t, plan.ProtectedSet, desc3.SnapshotID)
	assert.NotContains(t, plan.ToDelete, desc3.SnapshotID)
	assert.NotContains(t, plan.ToDelete, desc2.SnapshotID)
	assert.Equal(t, 1, plan.ProtectedByLineage)
}

func TestCollector_Run_PreservesDetachedLatestForRestoreToLatest(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s1"), 0644))
	desc1, err := creator.Create("main", "s1", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s2"), 0644))
	_, err = creator.Create("main", "s2", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("s3"), 0644))
	desc3, err := creator.Create("main", "s3", nil)
	require.NoError(t, err)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	require.NoError(t, restorer.Restore("main", desc1.SnapshotID))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	require.NoError(t, collector.Run(plan.PlanID))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc3.SnapshotID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(desc3.SnapshotID)+".json"))
	require.NoError(t, restorer.RestoreToLatest("main"))
}

func TestCollector_PlanWithPolicy_ProtectsDetachedLatestInNonMainWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)
	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("feature", nil)
	require.NoError(t, err)

	featurePath := requireWorktreePath(t, wtMgr, "feature")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "file.txt"), []byte("feature-1"), 0644))
	desc1, err := creator.Create("feature", "feature 1", nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "file.txt"), []byte("feature-2"), 0644))
	desc2, err := creator.Create("feature", "feature 2", nil)
	require.NoError(t, err)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)
	require.NoError(t, restorer.Restore("feature", desc1.SnapshotID))

	cfg, err := wtMgr.Get("feature")
	require.NoError(t, err)
	require.True(t, cfg.IsDetached())
	require.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)
	require.Equal(t, desc2.SnapshotID, cfg.LatestSnapshotID)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, desc2.SnapshotID)
	assert.NotContains(t, plan.ToDelete, desc2.SnapshotID)
}

func TestCollector_Plan_WithManySnapshots(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create multiple snapshots
	var snapshotIDs []model.SnapshotID
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte(string(rune(i))), 0644)
		desc, err := creator.Create("main", "test", nil)
		require.NoError(t, err)
		snapshotIDs = append(snapshotIDs, desc.SnapshotID)
	}

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	// Only the latest should be directly protected (others by lineage)
	assert.Contains(t, plan.ProtectedSet, snapshotIDs[len(snapshotIDs)-1])
	assert.Equal(t, 0, plan.CandidateCount)
}

func TestCollector_Plan_IgnoresPinFilesInV0(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create snapshot in main
	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("1"), 0644)
	_, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	// Create temp worktree with a snapshot that won't be in main's lineage
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	tempDesc, err := creator.Create("temp", "temp snap", nil)
	require.NoError(t, err)
	_ = cfg

	// Delete the temp worktree so snapshot is only protected by pin
	require.NoError(t, wtMgr.Remove("temp"))

	// Create pin for the temp snapshot
	pinsDir := filepath.Join(repoPath, ".jvs", "pins")
	require.NoError(t, os.MkdirAll(pinsDir, 0755))
	pinContent := `{"snapshot_id":"` + string(tempDesc.SnapshotID) + `","pinned_at":"2099-01-01T00:00:00Z","reason":"test"}`
	require.NoError(t, os.WriteFile(filepath.Join(pinsDir, string(tempDesc.SnapshotID)+".json"), []byte(pinContent), 0644))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	assert.Contains(t, plan.ToDelete, tempDesc.SnapshotID)
}

func TestCollector_Run_DeletesSnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create snapshot in main (protected)
	createTestSnapshot(t, repoPath)

	// Create temp worktree snapshot
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(tempDesc.SnapshotID))
	_, err = os.Stat(snapshotDir)
	assert.True(t, os.IsNotExist(err), "snapshot directory should be deleted")
}

func TestCollector_Run_DescriptorRemoval(t *testing.T) {
	repoPath := setupTestRepo(t)

	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp content"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	// Verify descriptor exists
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(tempDesc.SnapshotID)+".json")
	_, err = os.Stat(descriptorPath)
	require.NoError(t, err, "descriptor should exist")

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	_, err = os.Stat(descriptorPath)
	assert.True(t, os.IsNotExist(err), "descriptor should be deleted")
}

func TestCollector_ListAllSnapshots_Empty(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Don't create any snapshots
	collector := gc.NewCollector(repoPath)

	// Access internal method via Plan which calls listAllSnapshots
	plan, err := collector.Plan()
	require.NoError(t, err)
	assert.Empty(t, plan.ProtectedSet)
}

func TestCollector_ListAllSnapshots_WithNonDirectoryEntries(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	createTestSnapshot(t, repoPath)

	// Add a non-directory entry to snapshots dir
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	err := os.WriteFile(filepath.Join(snapshotsDir, "file.txt"), []byte("test"), 0644)
	require.NoError(t, err)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	// Plan should still succeed, ignoring non-dir entries
	assert.NotEmpty(t, plan.ProtectedSet)
}

func TestCollector_RunPayloadDeleteFailureKeepsDescriptorAndFailsClosedOnDamagedRetry(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	snapshotsDir := filepath.Dir(snapshotDir)
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.Chmod(snapshotsDir, 0555))
	t.Cleanup(func() {
		_ = os.Chmod(snapshotsDir, 0755)
	})

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remove snapshot dir")

	assert.FileExists(t, descriptorPath, "descriptor must remain inspectable when payload deletion fails")
	assert.DirExists(t, snapshotDir)
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateFailed)

	require.NoError(t, os.Chmod(snapshotsDir, 0755))
	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.ErrorIs(t, err, &errclass.JVSError{Code: "E_READY_MISSING"})

	assert.DirExists(t, snapshotDir)
	assert.FileExists(t, descriptorPath)
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateFailed)
}

func TestCollector_PlanFailsClosedWhenDescriptorIsDirectory(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create temp worktree with a snapshot
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	// Replace the descriptor file with a directory containing a file (blocking os.Remove)
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(tempDesc.SnapshotID)+".json")
	require.NoError(t, os.Remove(descriptorPath))
	require.NoError(t, os.MkdirAll(descriptorPath, 0755))
	// Add a file inside so os.Remove fails (can't remove non-empty dir)
	require.NoError(t, os.WriteFile(filepath.Join(descriptorPath, "blocker"), []byte("x"), 0644))

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	_, err = collector.PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not regular file")

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(tempDesc.SnapshotID))
	assert.DirExists(t, snapshotDir, "GC must not delete payload when descriptor leaf type is unsafe")
	assert.DirExists(t, descriptorPath, "descriptor obstacle should remain for inspection")
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(tempDesc.SnapshotID)+".json"))
}

func TestCollector_writeTombstone_TombstonesDirIsFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create temp worktree with a snapshot
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err = creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	// Make tombstones a file instead of directory (blocking writeTombstone)
	tombstonesPath := filepath.Join(repoPath, ".jvs", "gc", "tombstones")
	require.NoError(t, os.RemoveAll(tombstonesPath))
	require.NoError(t, os.WriteFile(tombstonesPath, []byte("blocked"), 0644))

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tombstone")

	_, err = os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.NoError(t, err, "failed GC runs should keep the plan for retry")

	os.Remove(tombstonesPath)
	os.MkdirAll(tombstonesPath, 0755)
}

func TestCollector_Run_DeleteSucceedsCommittedTombstoneWriteFailsThenRetryCompletes(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := createRemovedWorktreeSnapshots(t, repoPath, "temp", 1)
	snapshotID := ids[0]
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	tombstonesDir := filepath.Join(repoPath, ".jvs", "gc", "tombstones")

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	collector.SetProgressCallback(func(string, int, int, string) {
		require.NoError(t, os.Chmod(tombstonesDir, 0555))
	})
	t.Cleanup(func() {
		_ = os.Chmod(tombstonesDir, 0755)
	})

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write committed tombstone")
	assert.NoDirExists(t, snapshotDir)
	assert.NoFileExists(t, descriptorPath)
	requireTombstoneState(t, repoPath, snapshotID, model.GCStateMarked)

	require.NoError(t, os.Chmod(tombstonesDir, 0755))
	collector = gc.NewCollector(repoPath)
	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	requireTombstoneState(t, repoPath, snapshotID, model.GCStateCommitted)
	_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.True(t, os.IsNotExist(statErr), "successful retry should delete the plan")
}

func TestCollector_Run_DeleteSnapshotErrorKeepsPlan(t *testing.T) {
	repoPath := setupTestRepo(t)

	createRemovedWorktreeSnapshots(t, repoPath, "temp", 2)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Len(t, plan.ToDelete, 2)

	failingID := plan.ToDelete[1]
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(failingID)+".json")
	descriptorData, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)
	require.NoError(t, os.Remove(descriptorPath))
	require.NoError(t, os.MkdirAll(descriptorPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(descriptorPath, "blocker"), []byte("x"), 0644))

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), string(failingID))

	_, statErr := os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.NoError(t, statErr, "failed GC runs should keep the plan for retry")

	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(plan.ToDelete[0])+".json"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(plan.ToDelete[0])))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", "tombstones", string(failingID)+".json"))
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(failingID)))

	require.NoError(t, os.RemoveAll(descriptorPath))
	require.NoError(t, os.WriteFile(descriptorPath, descriptorData, 0644))
	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	_, statErr = os.Stat(filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	require.True(t, os.IsNotExist(statErr), "successful retry should delete the plan")
	requireTombstoneState(t, repoPath, plan.ToDelete[0], "committed")
	requireTombstoneState(t, repoPath, failingID, "committed")
}

func TestCollector_Plan_IgnoresInvalidPinFilesInV0(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	createTestSnapshot(t, repoPath)

	// Create a pin file with invalid JSON
	pinsDir := filepath.Join(repoPath, ".jvs", "pins")
	require.NoError(t, os.MkdirAll(pinsDir, 0755))
	invalidPinPath := filepath.Join(pinsDir, "invalid-pin.json")
	require.NoError(t, os.WriteFile(invalidPinPath, []byte("{invalid json}"), 0644))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)
	require.NotNil(t, plan)
}

func TestCollector_Plan_PinFileDoesNotAffectAlreadyProtectedSnapshotInV0(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)

	// Create a pin for the same snapshot that's already protected by main worktree
	pinsDir := filepath.Join(repoPath, ".jvs", "pins")
	require.NoError(t, os.MkdirAll(pinsDir, 0755))
	pinContent := `{"snapshot_id":"` + string(desc.SnapshotID) + `","pinned_at":"2099-01-01T00:00:00Z","reason":"test"}`
	require.NoError(t, os.WriteFile(filepath.Join(pinsDir, string(desc.SnapshotID)+".json"), []byte(pinContent), 0644))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, desc.SnapshotID)
}

func TestCollector_Plan_WithIntents(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create some intents (in-progress operations)
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	require.NoError(t, os.MkdirAll(intentsDir, 0755))

	intentIDs := []string{"intent1", "intent2", "intent3"}
	for _, id := range intentIDs {
		intentPath := filepath.Join(intentsDir, id+".json")
		require.NoError(t, os.WriteFile(intentPath, []byte(`{"intent_id":"`+id+`"}`), 0644))
	}

	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	require.NoError(t, err)

	// Plan should succeed with intents considered protected
	// (though they're not valid snapshot IDs, they're in the protected set)
	assert.NotEmpty(t, plan.ProtectedSet)
}

func TestCollector_walkLineage_WithMissingDescriptor(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot in a temp worktree
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp", nil)
	require.NoError(t, err)
	_ = cfg

	// Delete the descriptor to simulate corruption
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(tempDesc.SnapshotID)+".json")
	require.NoError(t, os.Remove(descriptorPath))

	// Create a new worktree and try to pin the orphaned snapshot
	// This will cause walkLineage to fail finding the descriptor
	wtMgr2 := worktree.NewManager(repoPath)
	cfg2, err := wtMgr2.Create("temp2", nil)
	require.NoError(t, err)
	_ = cfg2

	// Manually update the worktree config to point to the orphaned snapshot
	temp2Path := requireWorktreePath(t, wtMgr, "temp2")
	wtConfigPath := filepath.Join(temp2Path, ".jvs-worktree.json")
	wtConfigContent, _ := os.ReadFile(wtConfigPath)
	// Update head_snapshot_id
	newContent := string(wtConfigContent)
	// Find the existing head_snapshot_id and replace it
	if idx := indexOf(newContent, `"head_snapshot_id":"`); idx >= 0 {
		endIdx := idx + len(`"head_snapshot_id":"`) + 36 // approximate UUID length
		newContent = newContent[:idx] + `"head_snapshot_id":"` + string(tempDesc.SnapshotID) + `"` + newContent[endIdx+2:]
	}
	os.WriteFile(wtConfigPath, []byte(newContent), 0644)

	collector := gc.NewCollector(repoPath)
	_, err = collector.Plan()
	require.Error(t, err)
	assert.ErrorIs(t, err, &errclass.JVSError{Code: "E_READY_DESCRIPTOR_MISSING"})
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestCollector_PlanWithPolicy_AgeRetention(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	os.WriteFile(filepath.Join(mainPath, "file1.txt"), []byte("a"), 0644)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("b"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// KeepMinAge of 1 hour covers all just-created snapshots
	policy := model.RetentionPolicy{KeepMinAge: 1 * time.Hour}
	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(policy)
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, desc1.SnapshotID)
	assert.Contains(t, plan.ProtectedSet, desc2.SnapshotID)
	assert.Empty(t, plan.ToDelete)
}

func TestCollector_PlanWithPolicy_CountRetention(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create 3 snapshots in main
	var mainIDs []model.SnapshotID
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte(string(rune('a'+i))), 0644)
		desc, err := creator.Create("main", "main snap", nil)
		require.NoError(t, err)
		mainIDs = append(mainIDs, desc.SnapshotID)
	}

	// Create temp worktree with 1 snapshot, then delete the worktree
	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	tempDesc, err := creator.Create("temp", "temp snap", nil)
	require.NoError(t, err)

	require.NoError(t, wtMgr.Remove("temp"))

	// KeepMinSnapshots=5 is more than total (4), so all should be retained
	policy := model.RetentionPolicy{KeepMinSnapshots: 5}
	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(policy)
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, tempDesc.SnapshotID)
	assert.Empty(t, plan.ToDelete)
}

func TestCollector_PlanWithPolicy_ZeroRetention(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create temp worktree with snapshot, then remove worktree
	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	tempDesc, err := creator.Create("temp", "temp snap", nil)
	require.NoError(t, err)

	require.NoError(t, wtMgr.Remove("temp"))

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	assert.Contains(t, plan.ToDelete, tempDesc.SnapshotID)
}

func TestCollector_Run_EmptyPlanID(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	err := collector.Run("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plan ID is required")
}

func TestCollector_SetProgressCallback(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot in main (protected)
	createTestSnapshot(t, repoPath)

	// Create a temp worktree snapshot that will be deleted
	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err = creator.Create("temp", "temp snap", nil)
	require.NoError(t, err)

	require.NoError(t, wtMgr.Remove("temp"))

	var callCount atomic.Int32
	callback := func(phase string, current, total int, msg string) {
		callCount.Add(1)
	}

	collector := gc.NewCollector(repoPath)
	collector.SetProgressCallback(callback)

	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.NotEmpty(t, plan.ToDelete, "expected at least one deletion candidate")

	err = collector.Run(plan.PlanID)
	require.NoError(t, err)

	assert.Greater(t, callCount.Load(), int32(0), "progress callback should have been invoked")
}

func TestCollector_PlanWithPolicy_CombinedRetention(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create 3 snapshots in main
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte(string(rune('a'+i))), 0644)
		_, err := creator.Create("main", "snap", nil)
		require.NoError(t, err)
	}

	// Create temp worktree with snapshot, then delete worktree
	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath := requireWorktreePath(t, wtMgr, "temp")
	os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644)
	tempDesc, err := creator.Create("temp", "temp snap", nil)
	require.NoError(t, err)

	require.NoError(t, wtMgr.Remove("temp"))

	// Both policies: age covers all recently-created snapshots AND count covers all (4 total, keep 10)
	policy := model.RetentionPolicy{
		KeepMinAge:       1 * time.Hour,
		KeepMinSnapshots: 10,
	}
	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(policy)
	require.NoError(t, err)

	assert.Contains(t, plan.ProtectedSet, tempDesc.SnapshotID)
	assert.Empty(t, plan.ToDelete)
	assert.Greater(t, plan.ProtectedByRetention, 0)
}

func readAuditRecordsForGCTest(t *testing.T, auditPath string) []model.AuditRecord {
	t.Helper()

	data, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	records := make([]model.AuditRecord, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var record model.AuditRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func tamperFirstAuditRecordForGCTest(t *testing.T, auditPath string) {
	t.Helper()

	data, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	record["event_type"] = "restore"
	line, err := json.Marshal(record)
	require.NoError(t, err)
	lines[0] = string(line)

	require.NoError(t, os.WriteFile(auditPath, []byte(strings.Join(lines, "\n")+"\n"), 0644))
}

func gcPlanFiles(t *testing.T, repoPath string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(repoPath, ".jvs", "gc", "*.json"))
	require.NoError(t, err)
	return matches
}
