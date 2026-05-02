package gc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorPlanProtectsImportedCloneHistoryManifestOnlySavePoints(t *testing.T) {
	repoPath := setupTestRepo(t)
	importedIDs := createRemovedWorktreeSnapshots(t, repoPath, "imported", 2)
	writeImportedCloneHistoryManifestForGCTest(t, repoPath, importedIDs)

	plan, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	for _, id := range importedIDs {
		assert.Contains(t, plan.ProtectedSet, id)
		assert.NotContains(t, plan.ToDelete, id)
		assertProtectionGroupContains(t, plan, model.GCProtectionReasonImportedCloneHistory, id)
	}
}

func TestCollectorPlanFailsClosedForInvalidImportedCloneHistoryManifest(t *testing.T) {
	repoPath := setupTestRepo(t)
	path := clonehistory.ManifestPath(repoPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0644))

	_, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.Error(t, err)

	var jvsErr *errclass.JVSError
	require.ErrorAs(t, err, &jvsErr)
	assert.Equal(t, errclass.ErrGCPlanMismatch.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "imported clone history")
	assertNoCleanupPreviewPlanForGCTest(t, repoPath)
}

func TestCollectorPlanFailsClosedForImportedCloneHistoryMissingSavePoint(t *testing.T) {
	repoPath := setupTestRepo(t)
	missingID := model.SnapshotID("1708300800000-deadbeef")
	writeImportedCloneHistoryManifestRawForGCTest(t, repoPath, []model.SnapshotID{missingID})

	_, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.Error(t, err)

	var jvsErr *errclass.JVSError
	require.ErrorAs(t, err, &jvsErr)
	assert.Equal(t, errclass.ErrGCPlanMismatch.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "imported clone history")
	assertNoCleanupPreviewPlanForGCTest(t, repoPath)
}

func TestCollectorRunRevalidatesImportedCloneHistoryManifestChanges(t *testing.T) {
	repoPath := setupTestRepo(t)
	importedID := createRemovedWorktreeSnapshots(t, repoPath, "imported", 1)[0]
	writeImportedCloneHistoryManifestForGCTest(t, repoPath, []model.SnapshotID{importedID})

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	require.Contains(t, plan.ProtectedSet, importedID)
	require.NotContains(t, plan.ToDelete, importedID)

	require.NoError(t, os.Remove(clonehistory.ManifestPath(repoPath)))

	err = collector.Run(plan.PlanID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "imported clone history")
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(importedID)))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(importedID)+".json"))
}

func TestCollectorPlanProtectsDescriptorAndWorkspaceProvenanceReferences(t *testing.T) {
	repoPath := setupTestRepo(t)
	provenanceIDs := createRemovedWorktreeSnapshots(t, repoPath, "sources", 3)
	restoredFromID := provenanceIDs[0]
	restoredPathID := provenanceIDs[1]
	pathSourceID := provenanceIDs[2]
	headID := createTestSnapshot(t, repoPath)

	rewriteDescriptorProvenanceForGCTest(t, repoPath, headID, restoredFromID, restoredPathID)
	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.PathSources = model.NewPathSources()
	require.NoError(t, cfg.PathSources.Restore("restored/path.txt", pathSourceID))
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	plan, err := gc.NewCollector(repoPath).PlanWithPolicy(zeroRetention)
	require.NoError(t, err)

	for _, id := range provenanceIDs {
		assert.Contains(t, plan.ProtectedSet, id)
		assert.NotContains(t, plan.ToDelete, id)
		assertProtectionGroupContains(t, plan, model.GCProtectionReasonHistory, id)
	}
}

func writeImportedCloneHistoryManifestForGCTest(t *testing.T, repoPath string, ids []model.SnapshotID) {
	t.Helper()

	manifest := importedCloneHistoryManifestForGCTest(t, repoPath, ids)
	require.NoError(t, clonehistory.WriteManifest(repoPath, manifest))
}

func writeImportedCloneHistoryManifestRawForGCTest(t *testing.T, repoPath string, ids []model.SnapshotID) {
	t.Helper()

	manifest := importedCloneHistoryManifestForGCTest(t, repoPath, ids)
	manifest.ImportedSavePointsCount, manifest.ImportedSavePointsChecksum = clonehistory.ComputeImportedSavePointsEvidence(ids)
	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	path := clonehistory.ManifestPath(repoPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func importedCloneHistoryManifestForGCTest(t *testing.T, repoPath string, ids []model.SnapshotID) clonehistory.Manifest {
	t.Helper()

	r, err := repo.Discover(repoPath)
	require.NoError(t, err)
	return clonehistory.Manifest{
		SchemaVersion:      clonehistory.ManifestSchemaVersion,
		Operation:          clonehistory.OperationRepoClone,
		SourceRepoID:       "source-repo-id",
		TargetRepoID:       r.RepoID,
		SavePointsMode:     clonehistory.SavePointsModeAll,
		RuntimeStateCopied: false,
		ProtectionReason:   model.GCProtectionReasonImportedCloneHistory,
		ImportedSavePoints: ids,
	}
}

func rewriteDescriptorProvenanceForGCTest(t *testing.T, repoPath string, snapshotID, restoredFromID, restoredPathID model.SnapshotID) {
	t.Helper()

	desc, err := snapshot.LoadDescriptor(repoPath, snapshotID)
	require.NoError(t, err)
	desc.RestoredFrom = &restoredFromID
	desc.RestoredPaths = []model.RestoredPathSource{
		{
			TargetPath:       "restored/path.txt",
			SourceSnapshotID: restoredPathID,
			SourcePath:       "source/path.txt",
			Status:           model.PathSourceExact,
		},
	}
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(repoPath, snapshotID)
	require.NoError(t, err)
	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))

	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")
	readyData, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	var marker model.ReadyMarker
	require.NoError(t, json.Unmarshal(readyData, &marker))
	marker.DescriptorChecksum = checksum
	readyData, err = json.MarshalIndent(marker, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readyPath, readyData, 0644))
}

func assertNoCleanupPreviewPlanForGCTest(t *testing.T, repoPath string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(repoPath, ".jvs", "gc", "*.json"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}
