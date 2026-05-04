package gc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/repoclone"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorPlanProtectsSavePointsImportedByRepoCloneAll(t *testing.T) {
	source := setupTestRepo(t)
	mainID := createTestSnapshot(t, source)
	importedIDs := createRemovedWorktreeSnapshots(t, source, "imported", 2)
	target := filepath.Join(t.TempDir(), "target")

	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	plan, err := gc.NewCollector(target).PlanWithPolicy(zeroRetention)
	require.NoError(t, err)
	assert.Contains(t, plan.ProtectedSet, mainID)
	for _, id := range importedIDs {
		assert.Contains(t, plan.ProtectedSet, id)
		assert.NotContains(t, plan.ToDelete, id)
		assertProtectionGroupContains(t, plan, model.GCProtectionReasonImportedCloneHistory, id)
	}
}

func TestRepoCloneAllManifestTruncationFailsClosedForDoctorAndCleanupPreview(t *testing.T) {
	source := setupTestRepo(t)
	_ = createTestSnapshot(t, source)
	importedIDs := createRemovedWorktreeSnapshots(t, source, "imported", 2)
	target := filepath.Join(t.TempDir(), "target")

	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	manifest, ok, err := clonehistory.LoadManifest(target)
	require.NoError(t, err)
	require.True(t, ok)
	require.Subset(t, manifest.ImportedSavePoints, importedIDs)
	require.GreaterOrEqual(t, len(manifest.ImportedSavePoints), 2)
	manifest.ImportedSavePoints = manifest.ImportedSavePoints[:len(manifest.ImportedSavePoints)-1]
	writeRepoCloneIntegrationManifestObject(t, target, *manifest)

	doctorResult, err := doctor.NewDoctor(target).Check(true)
	require.NoError(t, err)
	assert.False(t, doctorResult.Healthy)
	assertDoctorFindingCodeForRepoCloneIntegration(t, doctorResult, "clone_history", doctor.ErrorCodeCloneHistoryInvalid)

	_, err = gc.NewCollector(target).PlanWithPolicy(zeroRetention)
	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.ErrorAs(t, err, &jvsErr)
	assert.Equal(t, errclass.ErrCleanupPlanMismatch.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "imported clone history")
}

func writeRepoCloneIntegrationManifestObject(t *testing.T, repoPath string, manifest clonehistory.Manifest) {
	t.Helper()

	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(clonehistory.ManifestPath(repoPath), data, 0644))
}

func assertDoctorFindingCodeForRepoCloneIntegration(t *testing.T, result *doctor.Result, category, code string) {
	t.Helper()

	for _, finding := range result.Findings {
		if finding.Category == category && finding.ErrorCode == code {
			return
		}
	}
	t.Fatalf("expected doctor finding %s/%s in %#v", category, code, result.Findings)
}
