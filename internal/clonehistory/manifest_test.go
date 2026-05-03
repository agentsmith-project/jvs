package clonehistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestWriteLoadAndValidate(t *testing.T) {
	repoPath, repoID, savePointID := setupCloneHistoryRepo(t)

	manifest := Manifest{
		SchemaVersion:      ManifestSchemaVersion,
		Operation:          OperationRepoClone,
		SourceRepoID:       "source-repo-id",
		TargetRepoID:       repoID,
		SavePointsMode:     SavePointsModeAll,
		RuntimeStateCopied: false,
		ProtectionReason:   model.GCProtectionReasonImportedCloneHistory,
		ImportedSavePoints: []model.SnapshotID{savePointID},
	}

	require.NoError(t, WriteManifest(repoPath, manifest))
	loaded, ok, err := LoadManifest(repoPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, manifest.SourceRepoID, loaded.SourceRepoID)
	assert.Equal(t, manifest.TargetRepoID, loaded.TargetRepoID)
	assert.Equal(t, manifest.ImportedSavePoints, loaded.ImportedSavePoints)
	assert.Equal(t, 1, loaded.ImportedSavePointsCount)
	assert.NotEmpty(t, loaded.ImportedSavePointsChecksum)
	require.NoError(t, ValidateManifest(repoPath, loaded))
}

func TestManifestValidationFailsClosed(t *testing.T) {
	repoPath, repoID, savePointID := setupCloneHistoryRepo(t)

	t.Run("metadata directory without manifest", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Dir(ManifestPath(repoPath))))
		require.NoError(t, os.MkdirAll(filepath.Dir(ManifestPath(repoPath)), 0755))

		_, ok, err := LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("target repo mismatch", func(t *testing.T) {
		manifest := validCloneHistoryManifest(repoID, []model.SnapshotID{savePointID})
		manifest.TargetRepoID = "different-target"
		writeRawManifest(t, repoPath, manifest)

		_, ok, err := LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		require.ErrorIs(t, err, errclass.ErrRepoIDMismatch)
		var jvsErr *errclass.JVSError
		require.True(t, errors.As(err, &jvsErr), "expected JVS error, got %T: %v", err, err)
		assert.Equal(t, errclass.ErrRepoIDMismatch.Code, jvsErr.Code)
		assert.Contains(t, err.Error(), "target repo")
	})

	t.Run("missing save point", func(t *testing.T) {
		manifest := validCloneHistoryManifest(repoID, []model.SnapshotID{"1708300800000-deadbeef"})
		writeRawManifest(t, repoPath, manifest)

		_, ok, err := LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "imported save point")
	})

	t.Run("truncated imported save point list without evidence update", func(t *testing.T) {
		otherSavePointID := createCloneHistorySavePoint(t, repoPath, "other.txt", "other")
		manifest := validCloneHistoryManifest(repoID, []model.SnapshotID{savePointID, otherSavePointID})
		require.NoError(t, WriteManifest(repoPath, manifest))
		loaded, ok, err := LoadManifest(repoPath)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, loaded.ImportedSavePoints, 2)
		loaded.ImportedSavePoints = loaded.ImportedSavePoints[:1]
		writeManifestObject(t, repoPath, *loaded)

		_, ok, err = LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "imported_save_points")
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		manifest := validCloneHistoryManifest(repoID, []model.SnapshotID{savePointID})
		require.NoError(t, WriteManifest(repoPath, manifest))
		loaded, ok, err := LoadManifest(repoPath)
		require.NoError(t, err)
		require.True(t, ok)
		loaded.ImportedSavePointsChecksum = "sha256:bad"
		writeManifestObject(t, repoPath, *loaded)

		_, ok, err = LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "checksum")
	})

	t.Run("runtime state copied", func(t *testing.T) {
		manifest := validCloneHistoryManifest(repoID, []model.SnapshotID{savePointID})
		manifest.RuntimeStateCopied = true
		writeRawManifest(t, repoPath, manifest)

		_, ok, err := LoadValidatedManifest(repoPath)
		require.Error(t, err)
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "runtime_state_copied")
	})
}

func setupCloneHistoryRepo(t *testing.T) (string, string, model.SnapshotID) {
	t.Helper()

	repoPath := t.TempDir()
	r, err := repo.Init(repoPath, "test")
	require.NoError(t, err)
	mainPath := requireMainWorktreePath(t, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644))
	desc, err := snapshot.NewCreator(repoPath, model.EngineCopy).Create("main", "test", nil)
	require.NoError(t, err)
	return repoPath, r.RepoID, desc.SnapshotID
}

func createCloneHistorySavePoint(t *testing.T, repoPath, fileName, content string) model.SnapshotID {
	t.Helper()

	mainPath := requireMainWorktreePath(t, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, fileName), []byte(content), 0644))
	desc, err := snapshot.NewCreator(repoPath, model.EngineCopy).Create("main", content, nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func requireMainWorktreePath(t testing.TB, repoPath string) string {
	t.Helper()

	path, err := repo.WorktreePayloadPath(repoPath, "main")
	require.NoError(t, err)
	return path
}

func validCloneHistoryManifest(repoID string, ids []model.SnapshotID) Manifest {
	return Manifest{
		SchemaVersion:      ManifestSchemaVersion,
		Operation:          OperationRepoClone,
		SourceRepoID:       "source-repo-id",
		TargetRepoID:       repoID,
		SavePointsMode:     SavePointsModeAll,
		RuntimeStateCopied: false,
		ProtectionReason:   model.GCProtectionReasonImportedCloneHistory,
		ImportedSavePoints: ids,
	}
}

func writeRawManifest(t *testing.T, repoPath string, manifest Manifest) {
	t.Helper()

	manifest.ImportedSavePointsCount, manifest.ImportedSavePointsChecksum = ComputeImportedSavePointsEvidence(manifest.ImportedSavePoints)
	writeManifestObject(t, repoPath, manifest)
}

func writeManifestObject(t *testing.T, repoPath string, manifest Manifest) {
	t.Helper()

	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	path := ManifestPath(repoPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, data, 0644))
}
