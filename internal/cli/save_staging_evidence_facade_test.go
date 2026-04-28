package cli

import (
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
	assertNoOldSavePointVocabulary(t, err.Error())
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, descriptorFileCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
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
	assertNoOldSavePointVocabulary(t, env.Error.Message)
	assert.Equal(t, 0, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, 0, descriptorFileCount(t, repoRoot))
	assert.Equal(t, 0, snapshotTempCount(t, repoRoot))
	assertFileContent(t, filepath.Join(repoRoot, "app.txt"), "changed during save")
}
