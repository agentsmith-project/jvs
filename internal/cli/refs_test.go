package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestResolveCheckpointRefDescriptorCorruptionIsNotNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		ref  func(model.SnapshotID) string
		tags []string
	}{
		{
			name: "full_checkpoint_id",
			ref:  func(id model.SnapshotID) string { return string(id) },
		},
		{
			name: "short_checkpoint_id",
			ref:  func(id model.SnapshotID) string { return string(id)[:8] },
		},
		{
			name: "tag_alias",
			ref:  func(model.SnapshotID) string { return "broken-tag" },
			tags: []string{"broken-tag"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := setupRefResolutionRepo(t)
			id := createRefResolutionCheckpoint(t, repoRoot, "corrupt descriptor target", tc.tags)
			corruptRefResolutionDescriptor(t, repoRoot, id)

			_, err := resolveCheckpointRef(repoRoot, "main", tc.ref(id))
			require.Error(t, err)
			require.True(t, errors.Is(err, errclass.ErrDescriptorCorrupt), "got %v", err)
			require.False(t, errors.Is(err, errRefNotFound), "got %v", err)
		})
	}
}

func TestResolveCheckpointRefTagAliasFailsClosedWithUnreadableDescriptor(t *testing.T) {
	repoRoot := setupRefResolutionRepo(t)
	createRefResolutionCheckpoint(t, repoRoot, "release target", []string{"release"})
	broken := createRefResolutionCheckpoint(t, repoRoot, "unreadable descriptor", []string{"other"})
	corruptRefResolutionDescriptor(t, repoRoot, broken)

	_, err := resolveCheckpointRef(repoRoot, "main", "release")
	require.Error(t, err)
	require.True(t, errors.Is(err, errclass.ErrDescriptorCorrupt), "got %v", err)
}

func TestResolveCheckpointRefInWorkspaceTagAliasFailsClosedWithUnreadableDescriptor(t *testing.T) {
	repoRoot := setupRefResolutionRepo(t)
	createRefResolutionCheckpoint(t, repoRoot, "release target", []string{"release"})
	broken := createRefResolutionCheckpoint(t, repoRoot, "unreadable descriptor", []string{"other"})
	corruptRefResolutionDescriptor(t, repoRoot, broken)

	_, err := resolveCheckpointRefInWorkspace(repoRoot, "main", "release")
	require.Error(t, err)
	require.True(t, errors.Is(err, errclass.ErrDescriptorCorrupt), "got %v", err)
}

func TestCheckpointListMalformedReadyJSONUsesPublishStateCode(t *testing.T) {
	repoRoot := setupRefResolutionRepo(t)
	id := createRefResolutionCheckpoint(t, repoRoot, "malformed ready", nil)
	readyPath := filepath.Join(repoRoot, ".jvs", "snapshots", string(id), ".READY")
	require.NoError(t, os.WriteFile(readyPath, []byte("{not json"), 0644))

	stdout, stderr, code := runContractSubprocess(t, filepath.Join(repoRoot, "main"), "--json", "checkpoint", "list")
	require.NotZero(t, code, stdout)
	require.Empty(t, stderr)

	env := decodeContractEnvelope(t, stdout)
	require.False(t, env.OK, stdout)
	require.NotNil(t, env.Error, stdout)
	require.Equal(t, "E_READY_INVALID", env.Error.Code)
}

func TestResolveCheckpointRefMissingFullIDStaysRefNotFound(t *testing.T) {
	repoRoot := setupRefResolutionRepo(t)

	_, err := resolveCheckpointRef(repoRoot, "main", "1708300800000-deadbeef")
	require.Error(t, err)
	require.True(t, errors.Is(err, errRefNotFound), "got %v", err)
	require.False(t, errors.Is(err, errclass.ErrDescriptorCorrupt), "got %v", err)
}

func TestCheckpointRefNotFoundUsesTypedError(t *testing.T) {
	require.True(t, checkpointRefNotFound(fmt.Errorf("wrapped: %w", errRefNotFound.WithMessage("missing checkpoint"))))
	require.False(t, checkpointRefNotFound(errclass.ErrDescriptorCorrupt.WithMessage("descriptor not found on disk")))
}

func setupRefResolutionRepo(t *testing.T) string {
	t.Helper()
	repoRoot := filepath.Join(t.TempDir(), "repo")
	_, err := repo.Init(repoRoot, "repo")
	require.NoError(t, err)
	return repoRoot
}

func createRefResolutionCheckpoint(t *testing.T, repoRoot, note string, tags []string) model.SnapshotID {
	t.Helper()
	payloadPath := filepath.Join(repoRoot, "main", "data.txt")
	require.NoError(t, os.WriteFile(payloadPath, []byte(note), 0644))

	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).Create("main", note, tags)
	require.NoError(t, err)
	return desc.SnapshotID
}

func corruptRefResolutionDescriptor(t *testing.T, repoRoot string, id model.SnapshotID) {
	t.Helper()
	path := filepath.Join(repoRoot, ".jvs", "descriptors", string(id)+".json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0644))
}
