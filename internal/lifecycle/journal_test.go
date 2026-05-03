package lifecycle_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalWriteListConsumePending(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".jvs"), 0755))

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	first := lifecycle.OperationRecord{
		SchemaVersion:          lifecycle.SchemaVersion,
		OperationID:            "op-2",
		OperationType:          "repo_move",
		RepoID:                 "repo-123",
		Phase:                  "repo_root_moved",
		RecommendedNextCommand: "jvs repo move --run plan-2",
		CreatedAt:              now.Add(time.Minute),
		UpdatedAt:              now.Add(time.Minute),
		Metadata: map[string]any{
			"old_repo_root": "/old",
			"new_repo_root": "/new",
		},
	}
	second := lifecycle.OperationRecord{
		SchemaVersion:          lifecycle.SchemaVersion,
		OperationID:            "op-1",
		OperationType:          "workspace_rename",
		RepoID:                 "repo-123",
		Phase:                  "prepared",
		RecommendedNextCommand: "jvs workspace rename old new",
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	require.NoError(t, lifecycle.WriteOperation(repoRoot, first))
	require.NoError(t, lifecycle.WriteOperation(repoRoot, second))

	pending, err := lifecycle.ListPendingOperations(repoRoot)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, "op-1", pending[0].OperationID, "pending list should be stable by creation time")
	assert.Equal(t, "jvs workspace rename old new", pending[0].RecommendedNextCommand)
	assert.Equal(t, map[string]any{"old_repo_root": "/old", "new_repo_root": "/new"}, pending[1].Metadata)

	raw, err := os.ReadFile(filepath.Join(repoRoot, ".jvs", "lifecycle", "operations", "op-1.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"recommended_next_command"`)
	assert.Contains(t, string(raw), `"schema_version"`)

	require.NoError(t, lifecycle.ConsumeOperation(repoRoot, "op-1"))
	pending, err = lifecycle.ListPendingOperations(repoRoot)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "op-2", pending[0].OperationID)
}

func TestJournalConsumedPhaseIsNotPending(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".jvs"), 0755))

	now := time.Now().UTC()
	require.NoError(t, lifecycle.WriteOperation(repoRoot, lifecycle.OperationRecord{
		SchemaVersion:          lifecycle.SchemaVersion,
		OperationID:            "op-consumed",
		OperationType:          "workspace_delete",
		RepoID:                 "repo-123",
		Phase:                  lifecycle.PhaseConsumed,
		RecommendedNextCommand: "jvs workspace delete --run plan-1",
		CreatedAt:              now,
		UpdatedAt:              now,
	}))

	pending, err := lifecycle.ListPendingOperations(repoRoot)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestCWDSafetyRejectsLexicalAndPhysicalContainment(t *testing.T) {
	base := t.TempDir()
	affected := filepath.Join(base, "project")
	nested := filepath.Join(affected, "subdir")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.NoError(t, os.MkdirAll(outside, 0755))

	for _, tc := range []struct {
		name string
		cwd  string
	}{
		{name: "lexical", cwd: nested},
		{name: "physical", cwd: symlinkedCWDInsideAffectedTree(t, base, affected)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
				CWD:             tc.cwd,
				AffectedRoot:    affected,
				SafeNextCommand: "cd " + outside + " && jvs repo move --run plan-1",
			})
			require.ErrorIs(t, err, errclass.ErrLifecycleUnsafeCWD)
			var jvsErr *errclass.JVSError
			require.ErrorAs(t, err, &jvsErr)
			assert.Equal(t, errclass.ErrLifecycleUnsafeCWD.Code, jvsErr.Code)
			assert.Contains(t, jvsErr.Message, "No files were changed")
			assert.Contains(t, jvsErr.Message, "jvs repo move --run plan-1")
			assert.Contains(t, err.Error(), "No files were changed")
			assert.Contains(t, err.Error(), "jvs repo move --run plan-1")
		})
	}

	require.NoError(t, lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
		CWD:             outside,
		AffectedRoot:    affected,
		SafeNextCommand: "cd " + outside,
	}))
}

func symlinkedCWDInsideAffectedTree(t *testing.T, base, affected string) string {
	t.Helper()

	link := filepath.Join(base, "project-link")
	if err := os.Symlink(affected, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	return filepath.Join(link, "subdir")
}

func TestMoveSameFilesystemNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0644))

	err := lifecycle.MoveSameFilesystemNoOverwrite(src, dst)
	if errors.Is(err, fsutil.ErrRenameNoReplaceUnsupported) {
		t.Skip("platform does not support atomic rename no-replace")
	}
	require.NoError(t, err)
	assert.NoFileExists(t, src)
	assert.FileExists(t, dst)
}

func TestMoveSameFilesystemNoOverwriteDoesNotReplaceDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0644))
	require.NoError(t, os.WriteFile(dst, []byte("destination"), 0644))

	err := lifecycle.MoveSameFilesystemNoOverwrite(src, dst)
	require.Error(t, err)
	if !errors.Is(err, fsutil.ErrRenameNoReplaceUnsupported) {
		require.ErrorIs(t, err, os.ErrExist)
	}
	assert.FileExists(t, src)
	data, readErr := os.ReadFile(dst)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("destination"), data)
}
