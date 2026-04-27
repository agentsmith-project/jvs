package restoreplan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
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
	err = restoreplan.ValidateSourcePath(repoRoot, "main", plan, model.EngineCopy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source save point is not restorable")
	assert.Contains(t, err.Error(), "path does not exist in save point: missing.txt")
}

func setupRestorePlanRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}
