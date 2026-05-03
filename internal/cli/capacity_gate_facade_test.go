package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCapacityAvailableEnv = "JVS_TEST_CAPACITY_AVAILABLE_BYTES"

type cliFakeCapacityMeter struct {
	available int64
	checks    int
}

type cliPathCapacityMeter struct {
	repoRoot          string
	tempRoot          string
	siblingPrefix     string
	siblingParent     string
	availableByDevice map[string]int64
	probes            []string
}

type cliSaveLockProbeMeter struct {
	repoRoot            string
	available           int64
	once                sync.Once
	acquiredDuringCheck atomic.Bool
}

func (m *cliFakeCapacityMeter) AvailableBytes(path string) (int64, error) {
	m.checks++
	return m.available, nil
}

func (m *cliFakeCapacityMeter) DeviceID(path string) (string, error) {
	return "test-fs", nil
}

func (m *cliSaveLockProbeMeter) AvailableBytes(path string) (int64, error) {
	m.once.Do(func() {
		result := make(chan bool, 1)
		go func() {
			acquired := false
			err := repo.WithMutationLock(m.repoRoot, "save capacity lock probe", func() error {
				acquired = true
				return nil
			})
			result <- err == nil && acquired
		}()
		select {
		case acquired := <-result:
			if acquired {
				m.acquiredDuringCheck.Store(true)
			}
		case <-time.After(250 * time.Millisecond):
		}
	})
	return m.available, nil
}

func (m *cliPathCapacityMeter) AvailableBytes(path string) (int64, error) {
	m.probes = append(m.probes, path)
	device, err := m.DeviceID(path)
	if err != nil {
		return 0, err
	}
	return m.availableByDevice[device], nil
}

func (m *cliPathCapacityMeter) DeviceID(path string) (string, error) {
	slashPath := filepath.ToSlash(path)
	if m.tempRoot != "" && pathHasPrefix(slashPath, filepath.ToSlash(m.tempRoot)) {
		return "temp-fs", nil
	}
	if m.siblingPrefix != "" && strings.HasPrefix(slashPath, filepath.ToSlash(m.siblingPrefix)) {
		return "sibling-fs", nil
	}
	if m.siblingParent != "" && slashPath == filepath.ToSlash(m.siblingParent) {
		return "sibling-fs", nil
	}
	if m.repoRoot != "" && pathHasPrefix(slashPath, filepath.ToSlash(m.repoRoot)) {
		return "repo-fs", nil
	}
	return slashPath, nil
}

func init() {
	raw := os.Getenv(testCapacityAvailableEnv)
	if raw == "" {
		return
	}
	available, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return
	}
	installCapacityGateHooks(capacitygate.Gate{
		Meter:             &cliFakeCapacityMeter{available: available},
		SafetyMarginBytes: 0,
	})
}

func TestViewCapacityFailDoesNotCreateViewOrMutate(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	meter := installFailingCapacityGate(t)
	useMissingTempDir(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "Folder: "+repoRoot)
	assert.Contains(t, err.Error(), "Workspace: main")
	assert.Contains(t, err.Error(), "No view was opened.")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	assert.Equal(t, 1, meter.checks)
	before.assertUnchanged(t, repoRoot)

	t.Setenv(testCapacityAvailableEnv, "0")
	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "view", firstID)
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "Not enough free space")
	assert.Contains(t, env.Error.Message, "No view was opened.")
	assertViewOutputOmitsLegacyVocabulary(t, env.Error.Message)
	before.assertUnchanged(t, repoRoot)
}

func TestSaveCapacityFailFirstSaveDoesNotCreateSavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "first")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Contains(t, err.Error(), "History was not changed.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertNoOldSavePointVocabulary(t, err.Error())
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, descriptorFileCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Empty(t, cfg.HeadSnapshotID)
	require.Empty(t, cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestSaveCapacityFailLaterSaveKeepsNewest(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "second")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(firstID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestSaveCapacityFailJSONKeepsErrorCode(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	t.Setenv(testCapacityAvailableEnv, "0")

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "save", "-m", "first")
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "Not enough free space")
	assert.Contains(t, env.Error.Message, "No save point was created.")
	assertNoOldSavePointVocabulary(t, env.Error.Message)
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
}

func TestSaveCapacityPreflightRunsInsideMutationLock(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	meter := &cliSaveLockProbeMeter{
		repoRoot:  repoRoot,
		available: 1 << 40,
	}
	restore := installCapacityGateHooks(capacitygate.Gate{
		Meter:             meter,
		SafetyMarginBytes: 0,
	})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "locked capacity")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Saved save point")
	assert.False(t, meter.acquiredDuringCheck.Load(), "save capacity preflight must run while the mutation lock is already held")
	assert.Equal(t, 1, savePointCatalogCount(t, repoRoot))
}

func TestSaveCapacityProbesControlAndPathSourceTempFilesystems(t *testing.T) {
	repoRoot, firstID, secondID := setupActivePathSourceRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	tempRoot := useMissingTempDir(t)
	meter := &cliPathCapacityMeter{
		repoRoot: repoRoot,
		tempRoot: tempRoot,
		availableByDevice: map[string]int64{
			"repo-fs": 1 << 40,
			"temp-fs": 0,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{
		Meter:             meter,
		SafetyMarginBytes: 0,
	})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "after path restore")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assertProbeUnder(t, meter.probes, filepath.Join(repoRoot, ".jvs"))
	assertProbeUnder(t, meter.probes, tempRoot)
	assertNoTempPrefix(t, filepath.Dir(tempRoot), "jvs-path-source-reconcile-")

	t.Setenv("TMPDIR", t.TempDir())
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	assertPublicPathSourcesFromConfig(t, cfg, "app.txt", firstID)
	before.assertUnchanged(t, repoRoot)
}

func TestSaveCapacityFailAfterWholeRestoreKeepsRestoredState(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)
	installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "after restore")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
	assertFileContent(t, filepath.Join(repoRoot, "only-source.txt"), "source")
	require.NoFileExists(t, filepath.Join(repoRoot, "workspace-only.txt"))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestSaveCapacityFailAfterPathRestoreKeepsPathSourcesAndAvoidsTempBypass(t *testing.T) {
	repoRoot, firstID, secondID := setupActivePathSourceRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)
	tempRoot := useMissingTempDir(t)
	installFailingCapacityGate(t)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "after path restore")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.NotContains(t, err.Error(), "path source reconciliation")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v1")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside v2")
	assertNoTempPrefix(t, filepath.Dir(tempRoot), "jvs-path-source-reconcile-")

	t.Setenv("TMPDIR", t.TempDir())
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	assertPublicPathSourcesFromConfig(t, cfg, "app.txt", firstID)
	before.assertUnchanged(t, repoRoot)
}

func TestViewCapacityChecksPayloadHashTempFilesystemBeforeMaterialization(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	missingTemp := filepath.Join(t.TempDir(), "missing-temp")
	t.Setenv("TMPDIR", missingTemp)
	meter := &cliPathCapacityMeter{
		repoRoot: repoRoot,
		tempRoot: missingTemp,
		availableByDevice: map[string]int64{
			"repo-fs": 100 << 20,
			"temp-fs": 0,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No view was opened.")
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(missingTemp))
	assert.Equal(t, 0, viewDirCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestViewCapacityWithPathSourcesDoesNotMaterializeStatusBeforeGate(t *testing.T) {
	repoRoot, firstID, _ := setupActivePathSourceRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	missingTemp := useMissingTempDir(t)
	installFailingCapacityGate(t)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No view was opened.")
	assert.NotContains(t, err.Error(), "expected workspace")
	assertNoTempPrefix(t, filepath.Dir(missingTemp), "jvs-expected-workspace-")
	t.Setenv("TMPDIR", t.TempDir())
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholePreviewCapacityFailDoesNotWritePlanOrTemp(t *testing.T) {
	repoRoot, firstID, _ := setupWholeRestoreImpactRepo(t)
	installFailingCapacityGate(t)
	useMissingTempDir(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)
	tempsBefore := restorePreviewTempCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No restore plan was created.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	assert.Equal(t, tempsBefore, restorePreviewTempCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)

	t.Setenv(testCapacityAvailableEnv, "0")
	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "restore", firstID)
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "Not enough free space")
	assert.Contains(t, env.Error.Message, "No restore plan was created.")
	assertRestoreOutputOmitsLegacyVocabulary(t, env.Error.Message)
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	assert.Equal(t, tempsBefore, restorePreviewTempCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholePreviewCapacityWithPathSourcesDoesNotMaterializeDirtyBeforeGate(t *testing.T) {
	repoRoot, firstID, _ := setupActivePathSourceRepo(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)
	missingTemp := useMissingTempDir(t)
	installFailingCapacityGate(t)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No restore plan was created.")
	assert.NotContains(t, err.Error(), "expected workspace")
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	assertNoTempPrefix(t, filepath.Dir(missingTemp), "jvs-expected-workspace-")
	t.Setenv("TMPDIR", t.TempDir())
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholePreviewBehaviorFlagsSkipPathSourceDirtyMaterialization(t *testing.T) {
	for _, flag := range []string{"--save-first", "--discard-unsaved"} {
		t.Run(flag, func(t *testing.T) {
			repoRoot, firstID, _ := setupActivePathSourceRepo(t)
			before := captureViewMutationSnapshot(t, repoRoot)
			plansBefore := restorePlanFileCount(t, repoRoot)
			missingTemp := useMissingTempDir(t)
			installFailingCapacityGate(t)

			stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, flag)
			require.Error(t, err)
			require.Empty(t, strings.TrimSpace(stdout))
			assert.Contains(t, err.Error(), "Not enough free space")
			assert.Contains(t, err.Error(), "No restore plan was created.")
			assert.NotContains(t, err.Error(), "expected workspace")
			assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
			assertNoTempPrefix(t, filepath.Dir(missingTemp), "jvs-expected-workspace-")

			t.Setenv("TMPDIR", t.TempDir())
			before.assertUnchanged(t, repoRoot)
		})
	}
}

func TestRestorePathPreviewCapacityFailDoesNotWritePlanOrTempAndCandidateSkipsGate(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	meter := installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)
	tempsBefore := restorePreviewTempCount(t, repoRoot)

	candidatesOut, err := executeCommand(createTestRootCmd(), "restore", "--path", "app.txt")
	require.NoError(t, err)
	assert.Contains(t, candidatesOut, "Candidates for path: app.txt")
	assert.Equal(t, 0, meter.checks)
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No restore plan was created.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	assert.Equal(t, tempsBefore, restorePreviewTempCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePathPreviewCapacityDoesNotMaterializeDirtyBeforeGate(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	_ = savePointIDFromCLI(t, "second")
	missingTemp := useMissingTempDir(t)
	installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No restore plan was created.")
	assert.NotContains(t, err.Error(), "expected workspace")
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertNoTempPrefix(t, filepath.Dir(missingTemp), "jvs-expected-workspace-")
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholeRunCapacityFailBeforeWorkspaceMutation(t *testing.T) {
	repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	installFailingCapacityGate(t)
	useMissingTempDir(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Contains(t, err.Error(), "History was not changed.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "workspace-only.txt"), "workspace")
	require.NoFileExists(t, filepath.Join(repoRoot, "only-source.txt"))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreWholeRunCapacityChecksRestorePayloadSiblingFilesystem(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("source"), 0644))
	firstID := savePointIDFromCLI(t, "source")
	require.NoError(t, os.Remove(filepath.Join(repoRoot, "app.txt")))
	secondID := savePointIDFromCLI(t, "empty")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	tempRoot := filepath.Join(t.TempDir(), "temp")
	require.NoError(t, os.MkdirAll(tempRoot, 0755))
	t.Setenv("TMPDIR", tempRoot)
	meter := &cliPathCapacityMeter{
		repoRoot:      repoRoot,
		tempRoot:      tempRoot,
		siblingPrefix: repoRoot + ".restore-",
		siblingParent: filepath.Dir(repoRoot),
		availableByDevice: map[string]int64{
			"repo-fs":    100 << 20,
			"temp-fs":    100 << 20,
			"sibling-fs": 0,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No files were changed.")
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(filepath.Dir(repoRoot)))
	require.NoFileExists(t, filepath.Join(repoRoot, "app.txt"))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestSeparatedRestoreWholeRunCapacityProbesPayloadSiblingFilesystem(t *testing.T) {
	base := setupSeparatedControlCLICWD(t)
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("source"), 0644))

	sourceOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "source",
	)
	require.NoError(t, err, sourceOut)
	sourceID, _ := decodeContractDataMap(t, sourceOut)["save_point_id"].(string)
	require.NotEmpty(t, sourceID)
	require.NoError(t, os.Remove(filepath.Join(payloadRoot, "app.txt")))
	_, err = executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"save", "-m", "empty",
	)
	require.NoError(t, err)
	previewOut, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", sourceID,
	)
	require.NoError(t, err, previewOut)
	planID, _ := decodeContractDataMap(t, previewOut)["plan_id"].(string)
	require.NotEmpty(t, planID)

	tempRoot := filepath.Join(t.TempDir(), "temp")
	require.NoError(t, os.MkdirAll(tempRoot, 0755))
	t.Setenv("TMPDIR", tempRoot)
	meter := &cliPathCapacityMeter{
		repoRoot:      controlRoot,
		tempRoot:      tempRoot,
		siblingParent: filepath.Dir(payloadRoot),
		availableByDevice: map[string]int64{
			"repo-fs":    100 << 20,
			"temp-fs":    100 << 20,
			"sibling-fs": 0,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)
	before := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)

	stdout, err := executeCommand(createTestRootCmd(),
		"--json",
		"--control-root", controlRoot,
		"--workspace", "main",
		"restore", "--run", planID,
	)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(filepath.Dir(payloadRoot)))
	require.NoFileExists(t, filepath.Join(payloadRoot, "app.txt"))
	assertSeparatedLifecycleRootsUnchanged(t, before, controlRoot, payloadRoot)
}

func TestRestoreWholeRunCapacityDoesNotBudgetRenameBackupAsCopy(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("source"), 0644))
	firstID := savePointIDFromCLI(t, "source")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "workspace-only.bin"), []byte(strings.Repeat("x", 64<<10)), 0644))
	secondID := savePointIDFromCLI(t, "workspace")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	sourcePeak := materializationPeakForSavePoint(t, repoRoot, model.SnapshotID(firstID))
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, "main")
	require.NoError(t, err)
	workspaceBytes, err := capacitygate.TreeSize(boundary.Root, boundary.ExcludesRelativePath)
	require.NoError(t, err)
	require.Greater(t, workspaceBytes, sourcePeak)

	tempRoot := filepath.Join(t.TempDir(), "temp")
	require.NoError(t, os.MkdirAll(tempRoot, 0755))
	t.Setenv("TMPDIR", tempRoot)
	meter := &cliPathCapacityMeter{
		repoRoot:      repoRoot,
		tempRoot:      tempRoot,
		siblingPrefix: repoRoot + ".restore-",
		siblingParent: filepath.Dir(repoRoot),
		availableByDevice: map[string]int64{
			"repo-fs":    100 << 20,
			"temp-fs":    2 * sourcePeak,
			"sibling-fs": sourcePeak,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored save point: "+firstID)
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(filepath.Dir(repoRoot)))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "source")
	require.NoFileExists(t, filepath.Join(repoRoot, "workspace-only.bin"))

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Equal(t, 0, restoreBackupCount(t, repoRoot))
}

func TestRestorePathRunCapacityFailBeforeWorkspaceMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	installFailingCapacityGate(t)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Contains(t, err.Error(), "History was not changed.")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside v2")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	require.Empty(t, cfg.PathSources)
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreSaveFirstRunCapacityCoversPathSourceReconcileBeforeSafetySave(t *testing.T) {
	repoRoot, firstID, secondID := setupActivePathSourceRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--save-first")
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	sourcePeak := materializationPeakForSavePoint(t, repoRoot, model.SnapshotID(firstID))
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)
	tempRoot := filepath.Join(t.TempDir(), "missing-temp")
	t.Setenv("TMPDIR", tempRoot)
	meter := &cliPathCapacityMeter{
		repoRoot:      repoRoot,
		tempRoot:      tempRoot,
		siblingPrefix: repoRoot + ".restore-",
		siblingParent: filepath.Dir(repoRoot),
		availableByDevice: map[string]int64{
			"repo-fs":    100 << 20,
			"temp-fs":    2 * sourcePeak,
			"sibling-fs": 100 << 20,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.NotContains(t, err.Error(), "path source reconciliation")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(tempRoot))
	assertNoTempPrefix(t, filepath.Dir(tempRoot), "jvs-expected-workspace-")
	assertNoTempPrefix(t, filepath.Dir(tempRoot), "jvs-path-source-reconcile-")

	t.Setenv("TMPDIR", t.TempDir())
	before.assertUnchanged(t, repoRoot)
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	assertPublicPathSourcesFromConfig(t, cfg, "app.txt", firstID)
}

func TestRestoreSaveFirstRunCapacityFailBeforeSafetySave(t *testing.T) {
	t.Run("whole", func(t *testing.T) {
		repoRoot, firstID, secondID := setupWholeRestoreImpactRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--save-first")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		installFailingCapacityGate(t)
		descriptorCount := descriptorFileCount(t, repoRoot)
		catalogCount := savePointCatalogCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Contains(t, err.Error(), "Not enough free space")
		assert.Contains(t, err.Error(), "No save point was created.")
		assert.Contains(t, err.Error(), "No files were changed.")
		assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
		assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
		assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))

		cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
		require.NoError(t, err)
		require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
		require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "local edit")
	})

	t.Run("path", func(t *testing.T) {
		repoRoot := setupAdoptedSaveFacadeRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
		firstID := savePointIDFromCLI(t, "first")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
		secondID := savePointIDFromCLI(t, "second")
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("path local edit"), 0644))
		previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt", "--save-first")
		require.NoError(t, err)
		planID := restorePlanIDFromHumanOutput(t, previewOut)
		installFailingCapacityGate(t)
		descriptorCount := descriptorFileCount(t, repoRoot)
		catalogCount := savePointCatalogCount(t, repoRoot)

		stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
		require.Error(t, err)
		require.Empty(t, strings.TrimSpace(stdout))
		assert.Contains(t, err.Error(), "Not enough free space")
		assert.Contains(t, err.Error(), "No save point was created.")
		assert.Contains(t, err.Error(), "No files were changed.")
		assertRestoreOutputOmitsLegacyVocabulary(t, err.Error())
		assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
		assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))

		cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
		require.NoError(t, err)
		require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
		require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
		require.Empty(t, cfg.PathSources)
		assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "path local edit")
	})
}

func TestViewCapacityUsesCompressedLogicalSize(t *testing.T) {
	repoRoot, firstID, _ := setupCompressedCapacityRepo(t)
	restoreTreeWriteBitsForCleanup(t, filepath.Join(repoRoot, ".jvs", "views"))
	installCapacityGateAvailable(t, 2<<20)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No view was opened.")
	assert.Equal(t, 0, viewDirCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestRestorePreviewCapacityUsesCompressedLogicalSize(t *testing.T) {
	repoRoot, firstID, _ := setupCompressedCapacityRepo(t)
	installCapacityGateAvailable(t, 2<<20)
	before := captureViewMutationSnapshot(t, repoRoot)
	plansBefore := restorePlanFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No restore plan was created.")
	assert.Equal(t, plansBefore, restorePlanFileCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestRestoreSaveFirstRunCapacityUsesCompressedLogicalSizeBeforeSafetySave(t *testing.T) {
	repoRoot, firstID, secondID := setupCompressedCapacityRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "huge.txt"), []byte("local edit"), 0644))
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--save-first")
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	installCapacityGateAvailable(t, 2<<20)
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "huge.txt"), "local edit")

	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	require.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func TestViewCapacityUsesCompressedPeakSize(t *testing.T) {
	repoRoot, firstID, _ := setupTinyCompressedCapacityRepo(t)
	restoreTreeWriteBitsForCleanup(t, filepath.Join(repoRoot, ".jvs", "views"))
	available := viewCapacityBetweenFinalAndPeak(t, repoRoot, model.SnapshotID(firstID))
	installCapacityGateAvailable(t, available)
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "No view was opened.")
	assert.Equal(t, 0, viewDirCount(t, repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestViewCapacityProbesTempFilesystemForEmptySavePoint(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	emptyID := savePointIDFromCLI(t, "empty")
	missingTemp := useMissingTempDir(t)
	meter := &cliPathCapacityMeter{
		repoRoot: repoRoot,
		tempRoot: missingTemp,
		availableByDevice: map[string]int64{
			"repo-fs": 100 << 20,
			"temp-fs": 0,
		},
	}
	restore := installCapacityGateHooks(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 1})
	t.Cleanup(restore)

	stdout, err := executeCommand(createTestRootCmd(), "view", emptyID)
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, slashPaths(meter.probes), filepath.ToSlash(missingTemp))
}

func installFailingCapacityGate(t *testing.T) *cliFakeCapacityMeter {
	t.Helper()
	meter := &cliFakeCapacityMeter{available: 0}
	restore := installCapacityGateHooks(capacitygate.Gate{
		Meter:             meter,
		SafetyMarginBytes: 0,
	})
	t.Cleanup(restore)
	return meter
}

func installCapacityGateAvailable(t *testing.T, available int64) *cliFakeCapacityMeter {
	t.Helper()
	meter := &cliFakeCapacityMeter{available: available}
	restore := installCapacityGateHooks(capacitygate.Gate{
		Meter:             meter,
		SafetyMarginBytes: 0,
	})
	t.Cleanup(restore)
	return meter
}

func installCapacityGateHooks(gate capacitygate.Gate) func() {
	oldViewGate := viewCapacityGate
	oldRunGate := restoreRunCapacityGate
	oldSaveGate := saveCapacityGate
	oldWorkspaceNewGate := workspaceNewCapacityGate
	restorePlanGate := restoreplan.SetCapacityGateForTest(gate)
	viewCapacityGate = gate
	restoreRunCapacityGate = gate
	saveCapacityGate = gate
	workspaceNewCapacityGate = gate
	return func() {
		viewCapacityGate = oldViewGate
		restoreRunCapacityGate = oldRunGate
		saveCapacityGate = oldSaveGate
		workspaceNewCapacityGate = oldWorkspaceNewGate
		restorePlanGate()
	}
}

func restorePreviewTempCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs"))
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "restore-preview-") {
			count++
		}
	}
	return count
}

func snapshotTempCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "snapshots"))
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			count++
		}
	}
	return count
}

func intentFileCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "intents"))
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func useMissingTempDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "missing-temp")
	t.Setenv("TMPDIR", path)
	return path
}

func setupCompressedCapacityRepo(t *testing.T) (repoRoot, firstID, secondID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	large := strings.Repeat("A", 3<<20)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "huge.txt"), []byte(large), 0644))
	creator := snapshot.NewCreator(repoRoot, model.EngineCopy)
	creator.SetCompression(compression.LevelMax)
	first, err := creator.CreateSavePoint("main", "compressed first", nil)
	require.NoError(t, err)
	require.NotNil(t, first.Compression)
	firstID = string(first.SnapshotID)

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "huge.txt"), []byte("v2"), 0644))
	secondID = savePointIDFromCLI(t, "second")
	return repoRoot, firstID, secondID
}

func setupTinyCompressedCapacityRepo(t *testing.T) (repoRoot, firstID, secondID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tiny.txt"), []byte("x"), 0644))
	creator := snapshot.NewCreator(repoRoot, model.EngineCopy)
	creator.SetCompression(compression.LevelMax)
	first, err := creator.CreateSavePoint("main", "compressed tiny", nil)
	require.NoError(t, err)
	require.NotNil(t, first.Compression)
	firstID = string(first.SnapshotID)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "tiny.txt"), []byte("y"), 0644))
	secondID = savePointIDFromCLI(t, "second")
	return repoRoot, firstID, secondID
}

func setupActivePathSourceRepo(t *testing.T) (repoRoot, firstID, secondID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	firstID = savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	secondID = savePointIDFromCLI(t, "second")
	previewOut, err := executeCommand(createTestRootCmd(), "restore", firstID, "--path", "app.txt")
	require.NoError(t, err)
	planID := restorePlanIDFromHumanOutput(t, previewOut)
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", planID)
	require.NoError(t, err)
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	require.Len(t, cfg.PathSources, 1)
	return repoRoot, firstID, secondID
}

func viewCapacityBetweenFinalAndPeak(t *testing.T, repoRoot string, sourceID model.SnapshotID) int64 {
	t.Helper()
	estimate := materializationEstimateForSavePoint(t, repoRoot, sourceID)
	require.Greater(t, estimate.PeakBytes, estimate.FinalBytes)
	finalRequired := 2*estimate.FinalBytes + metadataFloor
	peakRequired := 2*estimate.PeakBytes + metadataFloor
	require.Less(t, finalRequired, peakRequired)
	return finalRequired
}

func materializationPeakForSavePoint(t *testing.T, repoRoot string, sourceID model.SnapshotID) int64 {
	t.Helper()
	return materializationEstimateForSavePoint(t, repoRoot, sourceID).PeakBytes
}

func materializationEstimateForSavePoint(t *testing.T, repoRoot string, sourceID model.SnapshotID) snapshotpayload.MaterializationCapacityEstimate {
	t.Helper()
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, sourceID)
	require.NoError(t, err)
	opts, err := snapshotpayload.OptionsForSnapshot(repoRoot, sourceID)
	require.NoError(t, err)
	estimate, err := snapshotpayload.EstimateMaterializationCapacity(snapshotDir, opts)
	require.NoError(t, err)
	return estimate
}

func assertPublicPathSourcesFromConfig(t *testing.T, cfg *model.WorktreeConfig, targetPath, sourceSavePoint string) {
	t.Helper()
	entry, ok, err := cfg.PathSources.SourceForPath(targetPath)
	require.NoError(t, err)
	require.True(t, ok, "expected path source for %s", targetPath)
	require.Equal(t, model.SnapshotID(sourceSavePoint), entry.SourceSnapshotID)
}

func pathHasPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/")
}

func assertNoTempPrefix(t *testing.T, parent, prefix string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), prefix), "unexpected temp entry %s", entry.Name())
	}
}

func assertProbeUnder(t *testing.T, probes []string, prefix string) {
	t.Helper()
	slashPrefix := filepath.ToSlash(prefix)
	for _, probe := range slashPaths(probes) {
		if pathHasPrefix(probe, slashPrefix) {
			return
		}
	}
	assert.Failf(t, "missing capacity probe", "expected a probe under %s, got %v", slashPrefix, slashPaths(probes))
}

func slashPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, filepath.ToSlash(path))
	}
	return out
}
