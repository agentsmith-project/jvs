package model_test

import (
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathSourcesParentRestoreClearsDescendantsAndChildRestoreIsMoreSpecific(t *testing.T) {
	parentSource := snapshotID("1708300700000-aaaaaaaa")
	childSource := snapshotID("1708300800000-bbbbbbbb")
	laterParentSource := snapshotID("1708300900000-cccccccc")

	sources := model.NewPathSources()
	require.NoError(t, sources.Restore("src/pkg/service", childSource))
	require.NoError(t, sources.Restore("src/pkg", parentSource))

	_, ok := sources["src/pkg/service"]
	assert.False(t, ok, "restoring a parent must clear descendant path sources")
	require.Len(t, sources, 1)

	require.NoError(t, sources.Restore("src/pkg/service", childSource))

	entry, ok, err := sources.SourceForPath("src/pkg/service/handler.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg/service", entry.TargetPath)
	assert.Equal(t, childSource, entry.SourceSnapshotID)

	entry, ok, err = sources.SourceForPath("src/pkg/other.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg", entry.TargetPath)
	assert.Equal(t, parentSource, entry.SourceSnapshotID)

	require.NoError(t, sources.Restore("src/pkg/./service//api", laterParentSource))
	entry, ok, err = sources.SourceForPath("src/pkg/service/api/routes.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg/service/api", entry.TargetPath)
	assert.Equal(t, laterParentSource, entry.SourceSnapshotID)
}

func TestPathSourcesNormalizeAndRejectUnsafePaths(t *testing.T) {
	source := snapshotID("1708300700000-aaaaaaaa")
	sources := model.NewPathSources()

	require.NoError(t, sources.Restore("./src//pkg/../pkg/service/", source))
	_, ok := sources["src/pkg/service"]
	assert.True(t, ok)

	unsafePaths := []string{
		"",
		".",
		"/abs/path",
		`C:\x`,
		"C:/x",
		"c:/x",
		"..",
		"../outside",
		"src/../../outside",
	}
	for _, path := range unsafePaths {
		t.Run(path, func(t *testing.T) {
			assert.Error(t, sources.Restore(path, source))
		})
	}
}

func TestPathSourcesEditAndDeleteInvalidateSources(t *testing.T) {
	parentSource := snapshotID("1708300700000-aaaaaaaa")
	childSource := snapshotID("1708300800000-bbbbbbbb")

	sources := model.NewPathSources()
	require.NoError(t, sources.Restore("src/pkg", parentSource))
	require.NoError(t, sources.Restore("src/pkg/service", childSource))

	require.NoError(t, sources.MarkModified("src/pkg/service/handler.go"))
	entry, ok, err := sources.SourceForPath("src/pkg/service/handler.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg/service", entry.TargetPath)
	assert.Equal(t, model.PathSourceModifiedAfterRestore, entry.Status)

	require.NoError(t, sources.MarkDeleted("src/pkg/service"))
	_, ok = sources["src/pkg/service"]
	assert.False(t, ok, "deleted restored path should not be recorded as restored provenance")

	entry, ok, err = sources.SourceForPath("src/pkg/other.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg", entry.TargetPath)
	assert.Equal(t, model.PathSourceModifiedAfterRestore, entry.Status)
}

func TestPathSourcesModifiedChildDoesNotUnnecessarilyDowngradeParent(t *testing.T) {
	parentSource := snapshotID("1708300700000-aaaaaaaa")
	childSource := snapshotID("1708300800000-bbbbbbbb")

	sources := model.NewPathSources()
	require.NoError(t, sources.Restore("src", parentSource))
	require.NoError(t, sources.Restore("src/pkg", childSource))

	require.NoError(t, sources.MarkModified("src/pkg/file.go"))

	parentEntry, ok, err := sources.SourceForPath("src/other.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src", parentEntry.TargetPath)
	assert.Equal(t, model.PathSourceExact, parentEntry.Status)

	childEntry, ok, err := sources.SourceForPath("src/pkg/file.go")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "src/pkg", childEntry.TargetPath)
	assert.Equal(t, model.PathSourceModifiedAfterRestore, childEntry.Status)
}

func TestPathSourcesEntriesAreStableCopies(t *testing.T) {
	firstSource := snapshotID("1708300700000-aaaaaaaa")
	secondSource := snapshotID("1708300800000-bbbbbbbb")

	sources := model.NewPathSources()
	require.NoError(t, sources.Restore("b", secondSource))
	require.NoError(t, sources.Restore("a", firstSource))

	entries := sources.RestoredPaths()

	require.Len(t, entries, 2)
	assert.Equal(t, "a", entries[0].TargetPath)
	assert.Equal(t, "b", entries[1].TargetPath)

	entries[0].SourceSnapshotID = secondSource
	assert.Equal(t, firstSource, sources["a"].SourceSnapshotID)
}
