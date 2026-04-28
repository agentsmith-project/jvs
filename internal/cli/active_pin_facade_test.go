package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type restorePinObservingMeter struct {
	t         testing.TB
	repoRoot  string
	available int64
	maxPins   int
}

func (m *restorePinObservingMeter) AvailableBytes(path string) (int64, error) {
	pins := documentedPinCount(m.t, m.repoRoot)
	if pins > m.maxPins {
		m.maxPins = pins
	}
	return m.available, nil
}

func (m *restorePinObservingMeter) DeviceID(path string) (string, error) {
	return "test-fs", nil
}

func TestRestorePreviewOperationPinsAreReleasedOnSuccessAndFailure(t *testing.T) {
	t.Run("whole success", func(t *testing.T) {
		repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
		before := documentedPinCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Preview only")
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("whole failure", func(t *testing.T) {
		repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
		before := documentedPinCount(t, repoRoot)
		installFailingCapacityGate(t)

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("path success", func(t *testing.T) {
		repoRoot, firstID := setupRestorePathPinRepo(t)
		before := documentedPinCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Scope: path")
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("path failure", func(t *testing.T) {
		repoRoot, firstID := setupRestorePathPinRepo(t)
		before := documentedPinCount(t, repoRoot)
		installFailingCapacityGate(t)

		stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("path candidates do not pin without save ID", func(t *testing.T) {
		repoRoot, _ := setupRestorePathPinRepo(t)
		before := documentedPinCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--path", "app.txt")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Candidates for path: app.txt")
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})
}

func TestRestoreRunOperationPinsAreReleasedOnSuccessAndFailure(t *testing.T) {
	t.Run("whole success", func(t *testing.T) {
		repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := documentedPinCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Restored save point")
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("whole failure", func(t *testing.T) {
		repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := documentedPinCount(t, repoRoot)
		installFailingCapacityGate(t)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("path success", func(t *testing.T) {
		repoRoot, firstID := setupRestorePathPinRepo(t)
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := documentedPinCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.NoError(t, err)
		assert.Contains(t, stdout, "Restored path: app.txt")
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})

	t.Run("path failure", func(t *testing.T) {
		repoRoot, firstID := setupRestorePathPinRepo(t)
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		before := documentedPinCount(t, repoRoot)
		installFailingCapacityGate(t)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Equal(t, before, documentedPinCount(t, repoRoot))
	})
}

func TestRestorePreviewOperationPinExistsDuringCapacityGate(t *testing.T) {
	repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
	before := documentedPinCount(t, repoRoot)
	meter := &restorePinObservingMeter{t: t, repoRoot: repoRoot, available: 1 << 60}
	restore := installCapacityGateHooks(capacitygate.Gate{
		Meter:             meter,
		SafetyMarginBytes: 0,
	})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Preview only")
	assert.Equal(t, before+1, meter.maxPins)
	assert.Equal(t, before, documentedPinCount(t, repoRoot))
}

func TestReadOnlyViewPinProtectsSourceFromGCUntilClose(t *testing.T) {
	repoRoot, sourceID := setupLegacyRepoWithRemovedWorkspaceSavePoint(t)

	viewOut, err := executeCommand(createTestRootCmd(), "view", string(sourceID))
	require.NoError(t, err)
	viewID := viewIDFromHumanOutput(t, viewOut)
	viewPath := viewPathFromHumanOutput(t, viewOut)
	require.DirExists(t, viewPath)

	collector := gc.NewCollector(repoRoot)
	plan, err := collector.PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.Contains(t, plan.ProtectedSet, sourceID)
	assert.NotContains(t, plan.ToDelete, sourceID)
	require.NoError(t, collector.Run(plan.PlanID))
	assert.DirExists(t, filepath.Join(repoRoot, ".jvs", "snapshots", string(sourceID)))
	assert.FileExists(t, filepath.Join(repoRoot, ".jvs", "descriptors", string(sourceID)+".json"))

	closeOut, err := executeCommand(createTestRootCmd(), "view", "close", viewID)
	require.NoError(t, err)
	assert.Contains(t, closeOut, "Closed read-only view.")

	plan, err = gc.NewCollector(repoRoot).PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.NotContains(t, plan.ProtectedSet, sourceID)
	assert.Contains(t, plan.ToDelete, sourceID)
}

func setupRestorePathPinRepo(t *testing.T) (repoRoot, firstID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID = savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	return repoRoot, firstID
}

func setupLegacyRepoWithRemovedWorkspaceSavePoint(t *testing.T) (repoRoot string, sourceID model.SnapshotID) {
	t.Helper()
	repoRoot = t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	_, err = repo.Init(repoRoot, "test")
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))

	mgr := worktree.NewManager(repoRoot)
	_, err = mgr.Create("temp", nil)
	require.NoError(t, err)
	tempPath, err := mgr.Path("temp")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("source"), 0644))
	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("temp", "source", nil)
	require.NoError(t, err)
	require.NoError(t, mgr.Remove("temp"))
	return repoRoot, desc.SnapshotID
}
