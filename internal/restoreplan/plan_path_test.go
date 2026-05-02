package restoreplan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathEvidenceOnlyCoversTargetPath(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v1"), 0644))

	before, err := restoreplan.PathEvidence(repoRoot, "main", "app.txt")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v2"), 0644))
	afterOutside, err := restoreplan.PathEvidence(repoRoot, "main", "app.txt")
	require.NoError(t, err)
	require.Equal(t, before, afterOutside)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v2"), 0644))
	afterTarget, err := restoreplan.PathEvidence(repoRoot, "main", "app.txt")
	require.NoError(t, err)
	require.NotEqual(t, before, afterTarget)
}

func TestCreatePathPlanAndValidatePathTarget(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside v2"), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	plan, err := restoreplan.CreatePath(repoRoot, "main", first.SnapshotID, "app.txt", model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	require.Equal(t, restoreplan.ScopePath, plan.Scope)
	require.Equal(t, "app.txt", plan.Path)
	require.NotEmpty(t, plan.ExpectedPathEvidence)
	require.Empty(t, plan.ExpectedFolderEvidence)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside local"), 0644))
	require.NoError(t, restoreplan.ValidatePathTarget(repoRoot, "main", plan))

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("target local"), 0644))
	err = restoreplan.ValidatePathTarget(repoRoot, "main", plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "folder changed since preview")
}

func TestCreateWholePlanIncludesExpectedPreviewTransfer(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v2"), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	plan, err := restoreplan.Create(repoRoot, "main", first.SnapshotID, engine.EngineAuto, restoreplan.Options{})
	require.NoError(t, err)

	require.Len(t, plan.Transfers, 1)
	record := plan.Transfers[0]
	require.Equal(t, "restore-preview-validation-primary", record.TransferID)
	require.Equal(t, "restore", record.Operation)
	require.Equal(t, "preview_validation", record.Phase)
	require.True(t, record.Primary)
	require.Equal(t, transfer.ResultKindExpected, record.ResultKind)
	require.Equal(t, transfer.PermissionScopePreviewOnly, record.PermissionScope)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", string(first.SnapshotID)), record.SourcePath)
	require.Equal(t, "restore_preview_validation", record.DestinationRole)
	require.Contains(t, record.MaterializationDestination, filepath.Join(repoRoot, ".jvs", "restore-preview-"))
	require.Equal(t, mainPath, record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Contains(t, []transfer.PerformanceClass{transfer.PerformanceClassFastCopy, transfer.PerformanceClassNormalCopy}, record.PerformanceClass)
	assert.Equal(t, "v2", readRestorePlanTestFile(t, filepath.Join(mainPath, "app.txt")))
}

func TestCreatePathPlanIncludesExpectedPreviewTransfer(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "src", "app.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "src", "app.txt"), []byte("v2"), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	plan, err := restoreplan.CreatePath(repoRoot, "main", first.SnapshotID, "src/app.txt", engine.EngineAuto, restoreplan.Options{})
	require.NoError(t, err)

	require.Len(t, plan.Transfers, 1)
	record := plan.Transfers[0]
	require.Equal(t, "restore-preview-validation-primary", record.TransferID)
	require.Equal(t, "restore", record.Operation)
	require.Equal(t, "preview_validation", record.Phase)
	require.True(t, record.Primary)
	require.Equal(t, transfer.ResultKindExpected, record.ResultKind)
	require.Equal(t, transfer.PermissionScopePreviewOnly, record.PermissionScope)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", string(first.SnapshotID)), record.SourcePath)
	require.Equal(t, "restore_preview_validation", record.DestinationRole)
	require.Contains(t, record.MaterializationDestination, filepath.Join(repoRoot, ".jvs", "restore-preview-"))
	require.Equal(t, filepath.Join(mainPath, "src", "app.txt"), record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Contains(t, []transfer.PerformanceClass{transfer.PerformanceClassFastCopy, transfer.PerformanceClassNormalCopy}, record.PerformanceClass)
	assert.Equal(t, "v2", readRestorePlanTestFile(t, filepath.Join(mainPath, "src", "app.txt")))
}

func TestValidateSourceReturnsRunValidationTransfer(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)

	plan := &restoreplan.Plan{
		Scope:           restoreplan.ScopeWhole,
		Workspace:       "main",
		Folder:          mainPath,
		SourceSavePoint: first.SnapshotID,
	}
	record, err := restoreplan.ValidateSource(repoRoot, "main", plan, model.EngineCopy)
	require.NoError(t, err)

	assertRunValidationTransferRecord(t, record, repoRoot, string(first.SnapshotID), mainPath)
}

func TestValidateSourcePathReturnsRunValidationTransfer(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "src", "app.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)

	plan := &restoreplan.Plan{
		Scope:           restoreplan.ScopePath,
		Workspace:       "main",
		Folder:          mainPath,
		SourceSavePoint: first.SnapshotID,
		Path:            "src/app.txt",
	}
	record, err := restoreplan.ValidateSourcePath(repoRoot, "main", plan, model.EngineCopy)
	require.NoError(t, err)

	assertRunValidationTransferRecord(t, record, repoRoot, string(first.SnapshotID), filepath.Join(mainPath, "src", "app.txt"))
}

func TestCreatePlansReleaseOperationPins(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("v2"), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "second", nil)
	require.NoError(t, err)

	before := restorePlanDocumentedPinCount(t, repoRoot)
	_, err = restoreplan.Create(repoRoot, "main", first.SnapshotID, model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	require.Equal(t, before, restorePlanDocumentedPinCount(t, repoRoot))

	_, err = restoreplan.CreatePath(repoRoot, "main", first.SnapshotID, "app.txt", model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	require.Equal(t, before, restorePlanDocumentedPinCount(t, repoRoot))
}

func TestValidateSourcePathRejectsMissingSourcePath(t *testing.T) {
	repoRoot := setupRestorePlanRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "present.txt"), []byte("v1"), 0644))
	first, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "first", nil)
	require.NoError(t, err)

	plan := &restoreplan.Plan{
		Scope:           restoreplan.ScopePath,
		Workspace:       "main",
		SourceSavePoint: first.SnapshotID,
		Path:            "missing.txt",
	}
	_, err = restoreplan.ValidateSourcePath(repoRoot, "main", plan, model.EngineCopy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source save point is not restorable")
	assert.Contains(t, err.Error(), "path does not exist in save point: missing.txt")
}

func assertRunValidationTransferRecord(t *testing.T, record *transfer.Record, repoRoot, sourceID, publishedDestination string) {
	t.Helper()
	require.NotNil(t, record)
	require.Equal(t, "restore-run-source-validation", record.TransferID)
	require.Equal(t, "restore", record.Operation)
	require.Equal(t, "source_validation", record.Phase)
	require.False(t, record.Primary)
	require.Equal(t, transfer.ResultKindFinal, record.ResultKind)
	require.Equal(t, transfer.PermissionScopeExecution, record.PermissionScope)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, filepath.Join(repoRoot, ".jvs", "snapshots", sourceID), record.SourcePath)
	require.Equal(t, "restore_source_validation", record.DestinationRole)
	require.Contains(t, record.MaterializationDestination, filepath.Join(repoRoot, ".jvs", "restore-run-validation-"))
	require.Equal(t, publishedDestination, record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, model.EngineCopy, record.RequestedEngine)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.Equal(t, transfer.PerformanceClassNormalCopy, record.PerformanceClass)
}

func setupRestorePlanRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func restorePlanDocumentedPinCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "gc", "pins"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count
}

func readRestorePlanTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
