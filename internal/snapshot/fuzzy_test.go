package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSnapshotTestRepo(t *testing.T) string {
	tmpDir := t.TempDir()
	repoRoot := tmpDir

	// Create .jvs structure
	jvsDir := filepath.Join(repoRoot, ".jvs")
	snapshotsDir := filepath.Join(jvsDir, "snapshots")
	descriptorsDir := filepath.Join(jvsDir, "descriptors")
	require.NoError(t, os.MkdirAll(snapshotsDir, 0755))
	require.NoError(t, os.MkdirAll(descriptorsDir, 0755))

	// Create test snapshots
	now := time.Now().UTC()
	snapshots := []*model.Descriptor{
		{
			SnapshotID:      "1771589366482-abc12340",
			WorktreeName:    "main",
			CreatedAt:       now.Add(-3 * time.Hour),
			Note:            "initial setup",
			Tags:            []string{"v1.0"},
			Engine:          "copy",
			PayloadRootHash: "hash1",
		},
		{
			SnapshotID:      "1771589366482-def45670",
			WorktreeName:    "main",
			CreatedAt:       now.Add(-2 * time.Hour),
			Note:            "added features",
			Tags:            []string{"v1.1"},
			Engine:          "copy",
			PayloadRootHash: "hash2",
		},
		{
			SnapshotID:      "1771589366482-abc789a0",
			WorktreeName:    "main",
			CreatedAt:       now.Add(-1 * time.Hour),
			Note:            "bug fixes",
			Tags:            []string{"stable"},
			Engine:          "copy",
			PayloadRootHash: "hash3",
		},
	}

	for _, snap := range snapshots {
		// Create snapshot directory
		snapDir := filepath.Join(snapshotsDir, string(snap.SnapshotID))
		require.NoError(t, os.MkdirAll(snapDir, 0755))
		readyData := fmt.Sprintf(`{
			"snapshot_id": "%s",
			"completed_at": "%s",
			"payload_root_hash": "%s",
			"engine": "%s",
			"descriptor_checksum": "checksum"
		}`, snap.SnapshotID, snap.CreatedAt.Format(time.RFC3339), snap.PayloadRootHash, snap.Engine)
		require.NoError(t, os.WriteFile(filepath.Join(snapDir, ".READY"), []byte(readyData), 0644))

		// Write descriptor
		descPath := filepath.Join(descriptorsDir, string(snap.SnapshotID)+".json")
		data := fmt.Sprintf(`{
			"snapshot_id": "%s",
			"worktree_name": "%s",
			"created_at": "%s",
			"note": "%s",
			"tags": [%s],
			"engine": "%s",
			"payload_root_hash": "%s",
			"descriptor_checksum": "checksum",
			"integrity_state": "verified"
		}`, snap.SnapshotID, snap.WorktreeName, snap.CreatedAt.Format(time.RFC3339),
			snap.Note, formatTags(snap.Tags), snap.Engine, snap.PayloadRootHash)
		require.NoError(t, os.WriteFile(descPath, []byte(data), 0644))
	}

	return repoRoot
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var quoted []string
	for _, t := range tags {
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, ", ")
}

func TestFindMultiple_ByIDPrefix(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "1771589366482", 10)
	require.NoError(t, err)

	// Should match all three snapshots (same ID prefix)
	assert.Equal(t, 3, len(matches))
	// All should be id matches
	for _, m := range matches {
		assert.Equal(t, "id", m.MatchType)
	}
}

func TestFindMultiple_ByTag(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "v1", 10)
	require.NoError(t, err)

	// Should match v1.0 and v1.1
	assert.GreaterOrEqual(t, len(matches), 2)
}

func TestFindMultiple_ByNoteSubstring(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "fix", 10)
	require.NoError(t, err)

	// Should match "bug fixes"
	assert.GreaterOrEqual(t, len(matches), 1)
	found := false
	for _, m := range matches {
		if strings.Contains(m.Desc.Note, "fix") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find snapshot with 'fix' in note")
}

func TestFindMultiple_LimitsResults(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "1771589366482", 2)
	require.NoError(t, err)

	assert.Equal(t, 2, len(matches))
}

func TestFindMultiple_NoMatches(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "nonexistent", 10)
	require.NoError(t, err)

	assert.Equal(t, 0, len(matches))
}

func TestFormatMatchList(t *testing.T) {
	repoRoot := setupSnapshotTestRepo(t)

	matches, err := FindMultiple(repoRoot, "1771589366482", 3)
	require.NoError(t, err)

	output := FormatMatchList(matches)
	assert.Contains(t, output, "Matching snapshots:")
	assert.Contains(t, output, "initial setup")
	assert.Contains(t, output, "added features")
	assert.Contains(t, output, "bug fixes")
}
