package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatorCreateSavePointRejectsManagedWorkspaceChangeBeforePublish(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mutate       func(t *testing.T, workspaceRoot string)
		assertResult func(t *testing.T, workspaceRoot string)
	}{
		{
			name: "content",
			mutate: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(workspaceRoot, "file.txt"), []byte("changed during save"), 0644))
			},
			assertResult: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(workspaceRoot, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, "changed during save", string(data))
			},
		},
		{
			name: "create",
			mutate: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(workspaceRoot, "new.txt"), []byte("new during save"), 0644))
			},
			assertResult: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				assert.FileExists(t, filepath.Join(workspaceRoot, "new.txt"))
			},
		},
		{
			name: "delete",
			mutate: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(workspaceRoot, "file.txt")))
			},
			assertResult: func(t *testing.T, workspaceRoot string) {
				t.Helper()
				assert.NoFileExists(t, filepath.Join(workspaceRoot, "file.txt"))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := setupCreatorFailureRepo(t)
			workspaceRoot := filepath.Join(repoPath, "main")
			var stagedSnapshotID model.SnapshotID
			restoreHook := SetAfterSnapshotPayloadStagedHookForTest(func(snapshotID model.SnapshotID, _ string) error {
				stagedSnapshotID = snapshotID
				tc.mutate(t, workspaceRoot)
				return nil
			})
			t.Cleanup(restoreHook)

			desc, err := NewCreator(repoPath, model.EngineCopy).CreateSavePoint("main", "racy save", nil)
			require.Error(t, err)
			assert.Nil(t, desc)
			assert.Contains(t, err.Error(), "workspace files changed while saving")
			require.NotEmpty(t, stagedSnapshotID)
			assertUnpublishedSaveAttempt(t, repoPath, stagedSnapshotID)
			tc.assertResult(t, workspaceRoot)

			cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
			require.NoError(t, cfgErr)
			assert.Empty(t, cfg.HeadSnapshotID)
			assert.Empty(t, cfg.LatestSnapshotID)
			assert.Empty(t, cfg.PathSources.RestoredPaths())
			assertPublishedSavePointCount(t, repoPath, 0)
		})
	}
}

func TestCreatorCreateSavePointIgnoresControlDirChangesBeforePublish(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "src", "app.txt"), []byte("v1"), 0644))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	restoreHook := SetAfterSnapshotPayloadStagedHookForTest(func(model.SnapshotID, string) error {
		return os.WriteFile(filepath.Join(folder, ".jvs", "control-during-save.txt"), []byte("control"), 0644)
	})
	t.Cleanup(restoreHook)

	desc, err := NewCreator(r.Root, model.EngineCopy).CreateSavePoint("main", "control change", nil)
	require.NoError(t, err)
	require.NotNil(t, desc)
	assert.FileExists(t, filepath.Join(folder, ".jvs", "control-during-save.txt"))
	assert.NoFileExists(t, filepath.Join(folder, ".jvs", "snapshots", string(desc.SnapshotID), ".jvs", "control-during-save.txt"))
	assertPublishedSavePointCount(t, r.Root, 1)

	cfg, cfgErr := repo.LoadWorktreeConfig(r.Root, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)
}

func TestCreatorCreateSavePointRejectsStagedPayloadDifferentFromPreSaveEvidence(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	var stagedSnapshotID model.SnapshotID
	restoreHook := SetAfterSnapshotPayloadStagedHookForTest(func(snapshotID model.SnapshotID, snapshotTmpDir string) error {
		stagedSnapshotID = snapshotID
		return os.WriteFile(filepath.Join(snapshotTmpDir, "file.txt"), []byte("staged content changed"), 0644)
	})
	t.Cleanup(restoreHook)

	desc, err := NewCreator(repoPath, model.EngineCopy).CreateSavePoint("main", "staged changed", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.Contains(t, err.Error(), "workspace files changed while saving")
	require.NotEmpty(t, stagedSnapshotID)
	assertUnpublishedSaveAttempt(t, repoPath, stagedSnapshotID)
	assertFileContentInSnapshotTest(t, filepath.Join(repoPath, "main", "file.txt"), "content")
	assertPublishedSavePointCount(t, repoPath, 0)
}

func TestCreatorPartialSnapshotDoesNotUseFullWorkspaceEvidence(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	workspaceRoot := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(workspaceRoot, "other.txt"), []byte("before"), 0644))
	restoreHook := SetAfterSnapshotPayloadStagedHookForTest(func(model.SnapshotID, string) error {
		return os.WriteFile(filepath.Join(workspaceRoot, "other.txt"), []byte("changed during partial"), 0644)
	})
	t.Cleanup(restoreHook)

	desc, err := NewCreator(repoPath, model.EngineCopy).CreatePartial("main", "partial", nil, []string{"file.txt"})
	require.NoError(t, err)
	require.NotNil(t, desc)
	assert.Equal(t, []string{"file.txt"}, desc.PartialPaths)
	data, readErr := os.ReadFile(filepath.Join(workspaceRoot, "other.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "changed during partial", string(data))
	assertPublishedSavePointCount(t, repoPath, 1)
}

func assertUnpublishedSaveAttempt(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
	t.Helper()
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)+".tmp"))
	assert.NoDirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"))
}

func assertPublishedSavePointCount(t *testing.T, repoPath string, expected int) {
	t.Helper()
	descriptors, err := ListAll(repoPath)
	require.NoError(t, err)
	assert.Len(t, descriptors, expected)
}

func assertFileContentInSnapshotTest(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}

func TestSaveEvidenceErrorVocabulary(t *testing.T) {
	err := saveEvidenceChangedError()
	lower := strings.ToLower(err.Error())
	assert.NotContains(t, lower, "checkpoint")
	assert.NotContains(t, lower, "snapshot")
	assert.NotContains(t, lower, "worktree")
	assert.NotContains(t, lower, "current")
	assert.NotContains(t, lower, "latest")
	assert.NotContains(t, lower, "head")
}
