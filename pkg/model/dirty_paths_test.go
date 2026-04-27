package model_test

import (
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathDirtyAncestorDirtyRestoreChildLeavesChildCleanAndWorkspaceDirty(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	restoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(head)
	require.NoError(t, state.MarkPathModified("src"))

	require.NoError(t, state.RestorePath("src/pkg", restoreSource))

	assert.True(t, state.HasUnsavedChanges)
	assert.False(t, state.DirtyPaths.Contains("src/pkg/file.go"))
	assert.True(t, state.DirtyPaths.Contains("src/other.go"))
}

func TestPathDirtySiblingDirtyRestoreChildLeavesSiblingDirty(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	restoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(head)
	require.NoError(t, state.MarkPathModified("src/other.go"))

	require.NoError(t, state.RestorePath("src/pkg", restoreSource))

	assert.True(t, state.HasUnsavedChanges)
	assert.False(t, state.DirtyPaths.Contains("src/pkg/file.go"))
	assert.True(t, state.DirtyPaths.Contains("src/other.go"))
}

func TestWorkspaceRelativePathKeyNormalizationIsNotFilesystemSafetyValidation(t *testing.T) {
	key, err := model.NormalizeWorkspaceRelativePathKey("./.jvs//foo")

	require.NoError(t, err)
	assert.Equal(t, ".jvs/foo", key)
}
