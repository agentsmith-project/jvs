package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceStateWholeRestorePreservesHistoryHeadAndRecordsRestoredFrom(t *testing.T) {
	previousHead := snapshotID("1708300800000-bbbbbbbb")
	restoredFrom := snapshotID("1708300700000-aaaaaaaa")
	newSave := snapshotID("1708300900000-cccccccc")

	state := model.WorkspaceStateAtSavePoint(previousHead)

	state.RestoreWhole(restoredFrom)

	assertSnapshotPtrEqual(t, previousHead, state.HistoryHead)
	assertSnapshotPtrEqual(t, restoredFrom, state.ContentSource)
	assert.False(t, state.HasUnsavedChanges)
	assert.Empty(t, state.PathSources)

	lineage := state.Save(newSave)

	assertSnapshotPtrEqual(t, previousHead, lineage.ParentID)
	assertSnapshotPtrEqual(t, restoredFrom, lineage.RestoredFrom)
	assert.Nil(t, lineage.StartedFrom)
	assert.Empty(t, lineage.RestoredPaths)

	desc := model.Descriptor{SnapshotID: newSave}
	lineage.ApplyToDescriptor(&desc)

	assertSnapshotPtrEqual(t, previousHead, desc.ParentID)
	assertSnapshotPtrEqual(t, restoredFrom, desc.RestoredFrom)
	assert.Nil(t, desc.StartedFrom)
	assert.Empty(t, desc.RestoredPaths)
}

func TestWorkspaceStateStartedFromFirstSaveHasNoParentAndRecordsStartedFrom(t *testing.T) {
	startedFrom := snapshotID("1708300700000-aaaaaaaa")
	firstSave := snapshotID("1708300800000-bbbbbbbb")

	state := model.WorkspaceStateStartedFrom(startedFrom)

	assert.Nil(t, state.HistoryHead)
	assertSnapshotPtrEqual(t, startedFrom, state.ContentSource)
	assertSnapshotPtrEqual(t, startedFrom, state.StartedFrom)
	assert.False(t, state.HasUnsavedChanges)

	lineage := state.Save(firstSave)

	assert.Nil(t, lineage.ParentID)
	assertSnapshotPtrEqual(t, startedFrom, lineage.StartedFrom)
	assert.Nil(t, lineage.RestoredFrom)

	desc := model.Descriptor{SnapshotID: firstSave}
	lineage.ApplyToDescriptor(&desc)

	assert.Nil(t, desc.ParentID)
	assertSnapshotPtrEqual(t, startedFrom, desc.StartedFrom)
	assert.Nil(t, desc.RestoredFrom)
}

func TestWorkspaceStateSaveConvergesToNewSavePoint(t *testing.T) {
	previousHead := snapshotID("1708300800000-bbbbbbbb")
	wholeRestoreSource := snapshotID("1708300700000-aaaaaaaa")
	pathRestoreSource := snapshotID("1708300750000-dddddddd")
	newSave := snapshotID("1708300900000-cccccccc")

	state := model.WorkspaceStateAtSavePoint(previousHead)
	state.RestoreWhole(wholeRestoreSource)
	require.NoError(t, state.RestorePath("src/pkg", pathRestoreSource))
	state.MarkUnsaved()

	lineage := state.Save(newSave)

	assertSnapshotPtrEqual(t, previousHead, lineage.ParentID)
	assertSnapshotPtrEqual(t, wholeRestoreSource, lineage.RestoredFrom)
	require.Len(t, lineage.RestoredPaths, 1)
	assert.Equal(t, "src/pkg", lineage.RestoredPaths[0].TargetPath)
	assert.Equal(t, pathRestoreSource, lineage.RestoredPaths[0].SourceSnapshotID)

	assertSnapshotPtrEqual(t, newSave, state.HistoryHead)
	assertSnapshotPtrEqual(t, newSave, state.ContentSource)
	assert.Empty(t, state.PathSources)
	assert.False(t, state.HasUnsavedChanges)
}

func TestWorkspaceStatePathRestoreDoesNotMoveHistoryHead(t *testing.T) {
	previousHead := snapshotID("1708300800000-bbbbbbbb")
	pathRestoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(previousHead)

	require.NoError(t, state.RestorePath("src/pkg", pathRestoreSource))

	assertSnapshotPtrEqual(t, previousHead, state.HistoryHead)
	assertSnapshotPtrEqual(t, previousHead, state.ContentSource)
	assert.False(t, state.HasUnsavedChanges)
	require.Len(t, state.PathSources, 1)
	entry, ok, err := state.PathSources.SourceForPath("src/pkg/file.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, pathRestoreSource, entry.SourceSnapshotID)
	assert.Equal(t, model.PathSourceExact, entry.Status)
}

func TestWorkspaceStatePathRestoreClearsDirtyOnlyInsideRestoredPath(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	restoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(head)
	require.NoError(t, state.MarkPathModified("src/pkg/file.go"))
	require.NoError(t, state.MarkPathModified("README.md"))

	require.NoError(t, state.RestorePath("src/pkg", restoreSource))

	assert.True(t, state.HasUnsavedChanges)
	assert.False(t, state.HasUnknownUnsavedChanges)
	assert.False(t, state.DirtyPaths.Contains("src/pkg/file.go"))
	assert.True(t, state.DirtyPaths.Contains("README.md"))
}

func TestWorkspaceStatePathRestoreClearsOnlyDirtyRestoredPath(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	restoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(head)
	require.NoError(t, state.MarkPathModified("src/pkg/file.go"))

	require.NoError(t, state.RestorePath("src/pkg", restoreSource))

	assert.False(t, state.HasUnsavedChanges)
	assert.False(t, state.HasUnknownUnsavedChanges)
	assert.Empty(t, state.DirtyPaths)
}

func TestWorkspaceStatePathRestoreKeepsUnknownDirty(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	restoreSource := snapshotID("1708300700000-aaaaaaaa")

	state := model.WorkspaceStateAtSavePoint(head)
	state.MarkUnsaved()

	require.NoError(t, state.RestorePath("src/pkg", restoreSource))

	assert.True(t, state.HasUnsavedChanges)
	assert.True(t, state.HasUnknownUnsavedChanges)
	assert.Empty(t, state.DirtyPaths)
}

func TestWorkspaceStateSaveLineageDoesNotExposeUnsavedSaveFlag(t *testing.T) {
	head := snapshotID("1708300800000-bbbbbbbb")
	newSave := snapshotID("1708300900000-cccccccc")

	state := model.WorkspaceStateAtSavePoint(head)
	state.MarkUnsaved()

	lineage := state.NextSaveLineage(newSave)
	encoded, err := json.Marshal(lineage)
	require.NoError(t, err)

	assert.NotContains(t, strings.ToLower(string(encoded)), "unsaved")
}

func snapshotID(raw string) model.SnapshotID {
	return model.SnapshotID(raw)
}

func assertSnapshotPtrEqual(t *testing.T, expected model.SnapshotID, actual *model.SnapshotID) {
	t.Helper()
	require.NotNil(t, actual)
	assert.Equal(t, expected, *actual)
}
