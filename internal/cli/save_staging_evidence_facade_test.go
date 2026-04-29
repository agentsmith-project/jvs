package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSaveStagingMutationEnv = "JVS_TEST_SAVE_STAGING_MUTATION"

func init() {
	if rel := os.Getenv(testSaveStagingMutationEnv); rel != "" {
		snapshot.SetAfterSnapshotPayloadStagedHookForTest(func(model.SnapshotID, string) error {
			root := os.Getenv("JVS_CONTRACT_CWD")
			if root == "" {
				root, _ = os.Getwd()
			}
			return os.WriteFile(filepath.Join(root, rel), []byte("changed during save"), 0644)
		})
	}
}

func TestSaveConcurrentWorkspaceChangeHumanErrorUsesPublicVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	restoreHook := snapshot.SetAfterSnapshotPayloadStagedHookForTest(func(model.SnapshotID, string) error {
		return os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("changed during save"), 0644)
	})
	t.Cleanup(restoreHook)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "racy save")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "workspace files changed while saving")
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.NotContains(t, err.Error(), "..")
	assertNoOldSavePointVocabulary(t, err.Error())
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, descriptorFileCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "changed during save")
}

func TestSaveConcurrentWorkspaceChangeKeepsPathSources(t *testing.T) {
	repoRoot, firstID, secondID := setupActivePathSourceRepo(t)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)
	restoreHook := snapshot.SetAfterSnapshotPayloadStagedHookForTest(func(model.SnapshotID, string) error {
		return os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside changed during save"), 0644)
	})
	t.Cleanup(restoreHook)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "racy path source save")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "workspace files changed while saving")
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "outside.txt"), "outside changed during save")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, model.SnapshotID(secondID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(secondID), cfg.LatestSnapshotID)
	assertPublicPathSourcesFromConfig(t, cfg, "app.txt", firstID)
}

func TestSaveConcurrentWorkspaceChangeJSONErrorUsesPublicVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	t.Setenv(testSaveStagingMutationEnv, "app.txt")

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "save", "-m", "racy save")
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "workspace files changed while saving")
	assert.Contains(t, env.Error.Message, "No save point was created.")
	assert.NotContains(t, env.Error.Message, "..")
	assertNoOldSavePointVocabulary(t, env.Error.Message)
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, descriptorFileCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "changed during save")
}

func TestSaveDefiniteFailureLeavesHistoryUnchangedAndFailedSavePointUnavailable(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)
	descriptorCount := descriptorFileCount(t, repoRoot)
	catalogCount := savePointCatalogCount(t, repoRoot)

	var failedID model.SnapshotID
	restoreHook := installAuditAppendabilityFailureAfterSaveStagingHook(t, repoRoot, &failedID)
	t.Cleanup(restoreHook)

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "second")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "No save point was created.")
	assert.Contains(t, err.Error(), "History was not changed.")
	assertNoOldSavePointVocabulary(t, err.Error())
	require.NotEmpty(t, failedID)
	assert.Equal(t, descriptorCount, descriptorFileCount(t, repoRoot))
	assert.Equal(t, catalogCount, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(firstID), cfg.LatestSnapshotID)

	viewOut, viewErr := executeCommand(createTestRootCmd(), "view", string(failedID))
	if viewErr == nil && strings.TrimSpace(viewOut) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, viewOut))
	}
	require.Error(t, viewErr)
	require.Empty(t, strings.TrimSpace(viewOut))
	assert.Contains(t, viewErr.Error(), "save point")
	assert.Contains(t, viewErr.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, viewErr.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestSaveDefiniteFailureJSONReportsHistoryUnchanged(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	before := captureViewMutationSnapshot(t, repoRoot)

	var failedID model.SnapshotID
	restoreHook := installAuditAppendabilityFailureAfterSaveStagingHook(t, repoRoot, &failedID)
	t.Cleanup(restoreHook)

	jsonOut, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--json", "save", "-m", "second")
	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "No save point was created.")
	assert.Contains(t, env.Error.Message, "History was not changed.")
	assertNoOldSavePointVocabulary(t, env.Error.Message)
	require.NotEmpty(t, failedID)
	assert.Equal(t, 0, intentFileCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "v2")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, model.SnapshotID(firstID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(firstID), cfg.LatestSnapshotID)
	before.assertUnchanged(t, repoRoot)
}

func installAuditAppendabilityFailureAfterSaveStagingHook(t *testing.T, repoRoot string, failedID *model.SnapshotID) func() {
	t.Helper()
	return snapshot.SetAfterSnapshotPayloadStagedHookForTest(func(snapshotID model.SnapshotID, _ string) error {
		*failedID = snapshotID
		auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
		if err := os.Remove(auditPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Mkdir(auditPath, 0755)
	})
}
