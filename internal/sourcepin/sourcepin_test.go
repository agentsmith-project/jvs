package sourcepin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerCreateWritesDocumentedPinAndReleaseRemovesIt(t *testing.T) {
	repoRoot := setupSourcePinRepo(t)
	snapshotID := model.NewSnapshotID()

	handle, err := sourcepin.NewManager(repoRoot).CreateWithID(snapshotID, "view-"+string(snapshotID), "active read-only view")
	require.NoError(t, err)
	require.NotNil(t, handle)

	pinPath := filepath.Join(repoRoot, ".jvs", "gc", "pins", "view-"+string(snapshotID)+".json")
	data, err := os.ReadFile(pinPath)
	require.NoError(t, err)
	var pin model.Pin
	require.NoError(t, json.Unmarshal(data, &pin))
	assert.Equal(t, "view-"+string(snapshotID), pin.PinID)
	assert.Equal(t, snapshotID, pin.SnapshotID)
	assert.Equal(t, "active read-only view", pin.Reason)
	assert.False(t, pin.CreatedAt.IsZero())
	assert.False(t, pin.PinnedAt.IsZero())

	protected, err := sourcepin.NewManager(repoRoot).ProtectedSnapshotIDs()
	require.NoError(t, err)
	assert.Equal(t, []model.SnapshotID{snapshotID}, protected)

	require.NoError(t, handle.Release())
	assert.NoFileExists(t, pinPath)
}

func TestManagerRejectsUnsafePinIDAndInvalidSnapshotID(t *testing.T) {
	repoRoot := setupSourcePinRepo(t)
	mgr := sourcepin.NewManager(repoRoot)

	_, err := mgr.CreateWithID(model.NewSnapshotID(), "../bad", "bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pin ID")

	_, err = mgr.CreateWithID(model.SnapshotID("not-valid"), "valid-pin", "bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot ID")
}

func TestCreateWithIDFailsWithoutReplacingExistingRegularPin(t *testing.T) {
	repoRoot := setupSourcePinRepo(t)
	mgr := sourcepin.NewManager(repoRoot)
	originalID := model.NewSnapshotID()
	replacementID := model.NewSnapshotID()

	original, err := mgr.CreateWithID(originalID, "source-existing", "original protection")
	require.NoError(t, err)
	originalPath := filepath.Join(repoRoot, ".jvs", "gc", "pins", "source-existing.json")
	originalData, err := os.ReadFile(originalPath)
	require.NoError(t, err)

	replacement, err := mgr.CreateWithID(replacementID, "source-existing", "replacement protection")
	require.Error(t, err)
	require.Nil(t, replacement)
	assert.Contains(t, err.Error(), "already exists")

	afterData, err := os.ReadFile(originalPath)
	require.NoError(t, err)
	assert.JSONEq(t, string(originalData), string(afterData))

	require.NoError(t, original.Release())
}

func TestHandleReleaseFailsClosedWhenPinWasReplaced(t *testing.T) {
	repoRoot := setupSourcePinRepo(t)
	mgr := sourcepin.NewManager(repoRoot)
	originalID := model.NewSnapshotID()
	replacementID := model.NewSnapshotID()

	handle, err := mgr.CreateWithID(originalID, "source-racy", "original operation")
	require.NoError(t, err)
	pinPath := filepath.Join(repoRoot, ".jvs", "gc", "pins", "source-racy.json")
	writeSourcePinForTest(t, pinPath, model.Pin{
		PinID:      "source-racy",
		SnapshotID: replacementID,
		CreatedAt:  handle.Pin.CreatedAt.Add(time.Second),
		PinnedAt:   handle.Pin.PinnedAt.Add(time.Second),
		Reason:     "replacement operation",
	})

	err = handle.Release()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed")

	pin, err := mgr.Read("source-racy")
	require.NoError(t, err)
	assert.Equal(t, replacementID, pin.SnapshotID)
	assert.Equal(t, "replacement operation", pin.Reason)

	require.NoError(t, mgr.RemoveIfMatches(*pin))
}

func TestManagerRemoveByIDFailsClosedWithoutDeleting(t *testing.T) {
	repoRoot := setupSourcePinRepo(t)
	mgr := sourcepin.NewManager(repoRoot)
	snapshotID := model.NewSnapshotID()

	handle, err := mgr.CreateWithID(snapshotID, "source-remove", "operation")
	require.NoError(t, err)
	pinPath := filepath.Join(repoRoot, ".jvs", "gc", "pins", "source-remove.json")

	err = mgr.Remove("source-remove")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matching pin identity")
	assert.FileExists(t, pinPath)

	require.NoError(t, handle.Release())
}

func setupSourcePinRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func writeSourcePinForTest(t *testing.T, path string, pin model.Pin) {
	t.Helper()
	data, err := json.MarshalIndent(pin, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}
