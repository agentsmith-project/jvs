package snapshot_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test matchesFilter directly for more coverage
func TestMatchesFilter_Direct(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	desc := createCatalogSnapshot(t, repoPath, "test snapshot", []string{"tag1", "tag2"})

	// Test that matching worktree name works
	opts := snapshot.FilterOptions{WorktreeName: "main"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, desc.SnapshotID, matches[0].SnapshotID)

	// Test that non-matching worktree name filters out
	opts = snapshot.FilterOptions{WorktreeName: "other"}
	matches, err = snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func setupCatalogTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func createCatalogSnapshot(t *testing.T, repoPath, note string, tags []string) *model.Descriptor {
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", note, tags)
	require.NoError(t, err)
	return desc
}

func TestListAll(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// Empty repo
	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Empty(t, all)

	// Create two snapshots
	desc1 := createCatalogSnapshot(t, repoPath, "first", nil)
	desc2 := createCatalogSnapshot(t, repoPath, "second", nil)

	all, err = snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// Newest first
	assert.Equal(t, desc2.SnapshotID, all[0].SnapshotID)
	assert.Equal(t, desc1.SnapshotID, all[1].SnapshotID)
}

func TestListAllRequiresReadyMarker(t *testing.T) {
	type markerSetup func(t *testing.T, snapshotDir string)

	regularMarker := func(name string) markerSetup {
		return func(t *testing.T, snapshotDir string) {
			t.Helper()
			writeCatalogReadyMarker(t, snapshotDir, name)
		}
	}
	symlinkMarker := func(name string) markerSetup {
		return func(t *testing.T, snapshotDir string) {
			t.Helper()
			targetPath := filepath.Join(snapshotDir, "ready-target")
			require.NoError(t, os.WriteFile(targetPath, []byte("ready"), 0644))
			if err := os.Symlink("ready-target", filepath.Join(snapshotDir, name)); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}
		}
	}
	directoryMarker := func(name string) markerSetup {
		return func(t *testing.T, snapshotDir string) {
			t.Helper()
			require.NoError(t, os.Mkdir(filepath.Join(snapshotDir, name), 0755))
		}
	}

	tests := []struct {
		name      string
		setup     markerSetup
		wantErr   bool
		wantCount int
	}{
		{
			name:      ".READY regular file is visible",
			setup:     regularMarker(".READY"),
			wantCount: 1,
		},
		{
			name:      ".READY.gz regular file is visible",
			setup:     regularMarker(".READY.gz"),
			wantCount: 1,
		},
		{
			name:      "missing marker is invisible",
			wantCount: 0,
		},
		{
			name:    ".READY symlink fails closed",
			setup:   symlinkMarker(".READY"),
			wantErr: true,
		},
		{
			name:    ".READY directory fails closed",
			setup:   directoryMarker(".READY"),
			wantErr: true,
		},
		{
			name:    ".READY.gz symlink fails closed",
			setup:   symlinkMarker(".READY.gz"),
			wantErr: true,
		},
		{
			name:    ".READY.gz directory fails closed",
			setup:   directoryMarker(".READY.gz"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := setupCatalogTestRepo(t)
			desc := createCatalogSnapshot(t, repoPath, "ready marker boundary", nil)
			snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
			for _, markerName := range []string{".READY", ".READY.gz"} {
				require.NoError(t, os.RemoveAll(filepath.Join(snapshotDir, markerName)))
			}
			if tt.setup != nil {
				tt.setup(t, snapshotDir)
			}

			all, err := snapshot.ListAll(repoPath)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, all)
				return
			}
			require.NoError(t, err)
			require.Len(t, all, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, desc.SnapshotID, all[0].SnapshotID)
			}
		})
	}
}

func writeCatalogReadyMarker(t *testing.T, snapshotDir, name string) {
	t.Helper()
	repoPath := filepath.Dir(filepath.Dir(filepath.Dir(snapshotDir)))
	snapshotID := model.SnapshotID(filepath.Base(snapshotDir))
	desc, err := snapshot.LoadDescriptor(repoPath, snapshotID)
	require.NoError(t, err)
	data, err := json.Marshal(model.ReadyMarker{
		SnapshotID:         desc.SnapshotID,
		CompletedAt:        time.Now().UTC(),
		PayloadHash:        desc.PayloadRootHash,
		Engine:             desc.Engine,
		DescriptorChecksum: desc.DescriptorChecksum,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, name), data, 0644))
}

func TestListAll_SortedByTime(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// Create snapshots with time gap
	desc1 := createCatalogSnapshot(t, repoPath, "first", nil)
	time.Sleep(10 * time.Millisecond)
	desc2 := createCatalogSnapshot(t, repoPath, "second", nil)
	time.Sleep(10 * time.Millisecond)
	desc3 := createCatalogSnapshot(t, repoPath, "third", nil)

	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Should be newest first
	assert.Equal(t, desc3.SnapshotID, all[0].SnapshotID)
	assert.Equal(t, desc2.SnapshotID, all[1].SnapshotID)
	assert.Equal(t, desc1.SnapshotID, all[2].SnapshotID)
}

func TestFind_ByNote(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release v1", nil)
	createCatalogSnapshot(t, repoPath, "release v2", nil)
	createCatalogSnapshot(t, repoPath, "wip feature", nil)

	opts := snapshot.FilterOptions{NoteContains: "release"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 2)
}

func TestFind_ByTag(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "first", []string{"v1.0"})
	createCatalogSnapshot(t, repoPath, "second", []string{"v1.1", "release"})
	createCatalogSnapshot(t, repoPath, "third", []string{"wip"})

	opts := snapshot.FilterOptions{HasTag: "release"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "release", matches[0].Tags[1])
}

func TestFind_ByWorktree(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// Create snapshots in main
	createCatalogSnapshot(t, repoPath, "main-snapshot", nil)

	// Create another worktree and snapshot
	wtMgr := worktree.NewManager(repoPath)
	cfg, err := wtMgr.Create("feature", nil)
	require.NoError(t, err)

	// Add content to feature worktree
	featurePath, err := wtMgr.Path("feature")
	require.NoError(t, err)
	os.WriteFile(filepath.Join(featurePath, "file.txt"), []byte("feature-content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err = creator.Create("feature", "feature-snapshot", nil)
	require.NoError(t, err)
	_ = cfg

	// Filter by main worktree
	opts := snapshot.FilterOptions{WorktreeName: "main"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "main", matches[0].WorktreeName)
}

func TestFind_ByTimeRange(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	before := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	createCatalogSnapshot(t, repoPath, "first", nil)
	middle := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	createCatalogSnapshot(t, repoPath, "second", nil)
	_ = time.Now().UTC() // after

	// Find snapshots between before and middle
	opts := snapshot.FilterOptions{Since: before, Until: middle}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "first", matches[0].Note)
}

func TestFind_CombinedFilters(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release v1", []string{"release"})
	createCatalogSnapshot(t, repoPath, "release v2", []string{"release", "stable"})
	createCatalogSnapshot(t, repoPath, "wip", []string{"wip"})

	// Filter by both note and tag
	opts := snapshot.FilterOptions{NoteContains: "release", HasTag: "stable"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "release v2", matches[0].Note)
}

func TestFind_EmptyResult(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "test", nil)

	opts := snapshot.FilterOptions{HasTag: "nonexistent"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestFindOne_ByNotePrefix(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release v1.0", nil)
	createCatalogSnapshot(t, repoPath, "wip feature", nil)

	desc, err := snapshot.FindOne(repoPath, "release")
	require.NoError(t, err)
	assert.Equal(t, "release v1.0", desc.Note)
}

func TestFindOne_ByTag(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "first", []string{"v1.0"})
	createCatalogSnapshot(t, repoPath, "second", []string{"wip"})

	desc, err := snapshot.FindOne(repoPath, "v1.0")
	require.NoError(t, err)
	assert.Equal(t, "first", desc.Note)
}

func TestFindOneFailsClosedWhenUnreadableDescriptorCouldHideAlias(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release target", []string{"release"})
	broken := createCatalogSnapshot(t, repoPath, "unreadable descriptor", []string{"other"})
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "descriptors", string(broken.SnapshotID)+".json"), []byte("{not json"), 0644))

	_, err := snapshot.FindOne(repoPath, "release")
	require.Error(t, err)
	require.True(t, errors.Is(err, errclass.ErrDescriptorCorrupt), "got %v", err)
}

func TestFindOneExactIDIgnoresUnrelatedUnreadableDescriptor(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	target := createCatalogSnapshot(t, repoPath, "exact target", []string{"release"})
	broken := createCatalogSnapshot(t, repoPath, "unreadable descriptor", []string{"other"})
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "descriptors", string(broken.SnapshotID)+".json"), []byte("{not json"), 0644))

	found, err := snapshot.FindOne(repoPath, string(target.SnapshotID))
	require.NoError(t, err)
	require.Equal(t, target.SnapshotID, found.SnapshotID)
}

func TestFindOne_ByTagPrefix(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "test", []string{"v1.0.0-alpha"})

	// Should find by tag prefix
	desc, err := snapshot.FindOne(repoPath, "v1.0")
	require.NoError(t, err)
	assert.Equal(t, "test", desc.Note)
}

func TestFindOne_BySnapshotIDPrefix(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	desc := createCatalogSnapshot(t, repoPath, "test", nil)

	// Use short ID (first 8 chars)
	shortID := desc.SnapshotID.ShortID()
	found, err := snapshot.FindOne(repoPath, shortID)
	require.NoError(t, err)
	assert.Equal(t, desc.SnapshotID, found.SnapshotID)

	// Use full ID
	found2, err := snapshot.FindOne(repoPath, string(desc.SnapshotID))
	require.NoError(t, err)
	assert.Equal(t, desc.SnapshotID, found2.SnapshotID)
}

func TestFindOne_Ambiguous(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release v1", nil)
	createCatalogSnapshot(t, repoPath, "release v2", nil)

	// Both notes start with "release"
	_, err := snapshot.FindOne(repoPath, "release")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestFindOne_AmbiguousTags(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "first", []string{"v1", "release"})
	createCatalogSnapshot(t, repoPath, "second", []string{"v2", "release"})

	// Both have "release" tag
	_, err := snapshot.FindOne(repoPath, "release")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestFindOne_NotFound(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	_, err := snapshot.FindOne(repoPath, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no snapshot found")
}

func TestFindByTag(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "old", []string{"release"})
	createCatalogSnapshot(t, repoPath, "new", []string{"release"})

	// FindByTag returns the latest (newest first in ListAll)
	desc, err := snapshot.FindByTag(repoPath, "release")
	require.NoError(t, err)
	assert.Equal(t, "new", desc.Note)
}

func TestFindByTag_NotFound(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	_, err := snapshot.FindByTag(repoPath, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no snapshot found")
}

func TestListAll_EmptySnapshotsDir(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// No snapshots yet
	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestListAll_HandlesCorruptDescriptor(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// Create a valid snapshot
	desc := createCatalogSnapshot(t, repoPath, "valid", nil)

	// Create a corrupt snapshot directory (no descriptor)
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	corruptDir := filepath.Join(snapshotsDir, "0000000000000-corrupt")
	require.NoError(t, os.Mkdir(corruptDir, 0755))

	// ListAll should skip the corrupt one
	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, desc.SnapshotID, all[0].SnapshotID)
}

func TestListAll_NonexistentSnapshotsDir(t *testing.T) {
	// Create a repo without snapshots directory
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)

	// Remove snapshots directory
	snapshotsDir := filepath.Join(dir, ".jvs", "snapshots")
	os.RemoveAll(snapshotsDir)

	// ListAll should return empty, not error
	all, err := snapshot.ListAll(dir)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestListAll_ReadError(t *testing.T) {
	// Create a directory that's not readable
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)

	// Make .jvs not readable
	jvsDir := filepath.Join(dir, ".jvs")
	require.NoError(t, os.Chmod(jvsDir, 0000))
	defer os.Chmod(jvsDir, 0755) // cleanup

	// ListAll should error
	_, err = snapshot.ListAll(dir)
	assert.Error(t, err)
}

func TestListAll_SkipsNonDirectories(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	// Create a valid snapshot
	desc := createCatalogSnapshot(t, repoPath, "valid", nil)

	// Create a file in snapshots directory (not a directory)
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.WriteFile(filepath.Join(snapshotsDir, "not-a-dir.txt"), []byte("content"), 0644))

	// ListAll should skip the file
	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, desc.SnapshotID, all[0].SnapshotID)
}

func TestFind_UntilFilter(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "first", nil)

	// Get time after first snapshot
	middle := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	createCatalogSnapshot(t, repoPath, "second", nil)
	time.Sleep(10 * time.Millisecond)

	createCatalogSnapshot(t, repoPath, "third", nil)

	// Find snapshots up to middle (should only include first)
	opts := snapshot.FilterOptions{Until: middle}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "first", matches[0].Note)
}

func TestFind_ListAllErrorPropagation(t *testing.T) {
	// Create a directory that's not readable
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)

	// Make .jvs not readable
	jvsDir := filepath.Join(dir, ".jvs")
	require.NoError(t, os.Chmod(jvsDir, 0000))
	defer os.Chmod(jvsDir, 0755) // cleanup

	// Find should propagate the error from ListAll
	opts := snapshot.FilterOptions{WorktreeName: "main"}
	_, err = snapshot.Find(dir, opts)
	assert.Error(t, err)
}

func TestFindOne_ListAllErrorPropagation(t *testing.T) {
	// Create a directory that's not readable
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)

	// Make .jvs not readable
	jvsDir := filepath.Join(dir, ".jvs")
	require.NoError(t, os.Chmod(jvsDir, 0000))
	defer os.Chmod(jvsDir, 0755) // cleanup

	// FindOne should propagate the error from ListAll
	_, err = snapshot.FindOne(dir, "test")
	assert.Error(t, err)
}

func TestMatchesFilter_MissingWorktreeName(t *testing.T) {
	// Test matchesFilter when filter WorktreeName is empty
	// This exercises the code path where empty WorktreeName doesn't filter
	repoPath := setupCatalogTestRepo(t)

	desc := createCatalogSnapshot(t, repoPath, "test", nil)

	// Empty WorktreeName should match any snapshot
	opts := snapshot.FilterOptions{WorktreeName: ""}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, desc.SnapshotID, matches[0].SnapshotID)
}

func TestMatchesFilter_ZeroSinceTime(t *testing.T) {
	// Test matchesFilter when Since time is zero
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "test", nil)

	// Zero Since time should not filter
	opts := snapshot.FilterOptions{Since: time.Time{}}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}

func TestMatchesFilter_ZeroUntilTime(t *testing.T) {
	// Test matchesFilter when Until time is zero
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "test", nil)

	// Zero Until time should not filter
	opts := snapshot.FilterOptions{Until: time.Time{}}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}

func TestMatchesFilter_MissingNoteContains(t *testing.T) {
	// Test matchesFilter when filter NoteContains is empty
	repoPath := setupCatalogTestRepo(t)

	desc := createCatalogSnapshot(t, repoPath, "test note", nil)

	// Empty NoteContains should match any snapshot
	opts := snapshot.FilterOptions{NoteContains: ""}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, desc.SnapshotID, matches[0].SnapshotID)
}

func TestMatchesFilter_NoTagsInDescriptor(t *testing.T) {
	// Test hasTag when descriptor has no tags
	repoPath := setupCatalogTestRepo(t)

	_ = createCatalogSnapshot(t, repoPath, "test", nil)

	// HasTag filter should not match snapshot with no tags
	opts := snapshot.FilterOptions{HasTag: "sometag"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestFindOne_MatchesExactTag(t *testing.T) {
	// Test that FindOne matches exact tag, not just prefix
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "test", []string{"v1.0.0"})

	// Exact tag match should work
	desc, err := snapshot.FindOne(repoPath, "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", desc.Tags[0])
}

func TestMatchesQuery_NonMatchingNotePrefix(t *testing.T) {
	// Test matchesQuery when note doesn't match prefix
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "release", nil)

	// Query that doesn't match note prefix should not match
	_, err := snapshot.FindOne(repoPath, "feature")
	assert.Error(t, err)
}

func TestFindByTag_FindErrorPropagation(t *testing.T) {
	// Test that FindByTag propagates errors from Find
	// Create a directory that's not readable
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)

	// Make .jvs not readable
	jvsDir := filepath.Join(dir, ".jvs")
	require.NoError(t, os.Chmod(jvsDir, 0000))
	defer os.Chmod(jvsDir, 0755) // cleanup

	// FindByTag should propagate the error from Find
	_, err = snapshot.FindByTag(dir, "release")
	assert.Error(t, err)
}

func TestListAll_EmptyRepo(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	all, err := snapshot.ListAll(repoPath)
	require.NoError(t, err)
	assert.Empty(t, all)
	assert.Len(t, all, 0)
}

func TestFind_NoMatches(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "alpha release", []string{"alpha"})
	createCatalogSnapshot(t, repoPath, "beta release", []string{"beta"})

	opts := snapshot.FilterOptions{NoteContains: "gamma", HasTag: "nonexistent"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestLoadDescriptor_CorruptedJSON(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	require.NoError(t, os.MkdirAll(descriptorsDir, 0755))
	descriptorPath := filepath.Join(descriptorsDir, "1708300800000-deadbeef.json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("not valid json at all"), 0644))

	_, err := snapshot.LoadDescriptor(repoPath, "1708300800000-deadbeef")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse descriptor")
}

func TestLoadDescriptor_NonExistent(t *testing.T) {
	repoPath := setupCatalogTestRepo(t)

	_, err := snapshot.LoadDescriptor(repoPath, "1708300800000-deadbeef")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
