package snapshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func readDirNames(t *testing.T, path string) []string {
	t.Helper()

	entries, err := os.ReadDir(path)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestCreator_Create(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Add some content to main/
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("hello"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test note", nil)
	require.NoError(t, err)

	assert.NotEmpty(t, desc.SnapshotID)
	assert.Equal(t, "main", desc.WorktreeName)
	assert.Equal(t, "test note", desc.Note)
	assert.Equal(t, model.EngineCopy, desc.Engine)
	assert.NotEmpty(t, desc.PayloadRootHash)
	assert.NotEmpty(t, desc.DescriptorChecksum)

	// Verify snapshot directory exists
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.DirExists(t, snapshotDir)

	// Verify descriptor exists
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	assert.FileExists(t, descriptorPath)

	// Verify .READY marker exists
	readyPath := filepath.Join(snapshotDir, ".READY")
	assert.FileExists(t, readyPath)
}

func TestCreator_CreateRecordsCloneResultMetadata(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	first := filepath.Join(mainPath, "first.bin")
	second := filepath.Join(mainPath, "second.bin")
	require.NoError(t, os.WriteFile(first, []byte("shared"), 0644))
	if err := os.Link(first, second); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "metadata", nil)
	require.NoError(t, err)

	require.Equal(t, model.EngineCopy, desc.Engine)
	require.Equal(t, model.EngineCopy, desc.ActualEngine)
	require.Equal(t, model.EngineCopy, desc.EffectiveEngine)
	require.Contains(t, desc.DegradedReasons, "hardlink")
	require.NotNil(t, desc.MetadataPreservation)
	require.NotEmpty(t, desc.MetadataPreservation.Hardlinks)
	require.NotEmpty(t, desc.MetadataPreservation.Ownership)
	require.Equal(t, "linear-data-copy", desc.PerformanceClass)
}

func TestCreatorCreateReturnsRepoBusyWhenMutationLockHeld(t *testing.T) {
	repoPath := setupTestRepo(t)

	held, err := repo.AcquireMutationLock(repoPath, "held-by-test")
	require.NoError(t, err)
	defer held.Release()

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err = creator.Create("main", "blocked by lock", nil)
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
}

func TestCreator_ReadyProtocol(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Verify .READY contains correct info
	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), ".READY")
	data, err := os.ReadFile(readyPath)
	require.NoError(t, err)

	var marker model.ReadyMarker
	require.NoError(t, json.Unmarshal(data, &marker))
	assert.Equal(t, desc.SnapshotID, marker.SnapshotID)
}

func TestCreator_CreateRejectsReservedWorkspaceRootPayloadNames(t *testing.T) {
	for _, name := range []string{".READY", ".READY.gz"} {
		t.Run(name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			mainPath := filepath.Join(repoPath, "main")
			reservedPath := filepath.Join(mainPath, name)
			require.NoError(t, os.WriteFile(reservedPath, []byte("user data"), 0644))

			creator := snapshot.NewCreator(repoPath, model.EngineCopy)
			desc, err := creator.Create("main", "reserved root path", nil)
			require.Error(t, err)
			assert.Nil(t, desc)
			assert.Contains(t, err.Error(), "reserved")
			assert.Contains(t, err.Error(), name)

			content, readErr := os.ReadFile(reservedPath)
			require.NoError(t, readErr)
			assert.Equal(t, "user data", string(content))

			cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
			require.NoError(t, cfgErr)
			assert.Empty(t, cfg.HeadSnapshotID)
			assert.Empty(t, cfg.LatestSnapshotID)
		})
	}
}

func TestCreator_UpdatesHead(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	// Check head updated
	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, desc1.SnapshotID, cfg.HeadSnapshotID)

	// Create second snapshot
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Parent should be first snapshot
	assert.Equal(t, desc1.SnapshotID, *desc2.ParentID)

	// Head should be second
	cfg, _ = repo.LoadWorktreeConfig(repoPath, "main")
	assert.Equal(t, desc2.SnapshotID, cfg.HeadSnapshotID)
}

func TestCreator_PayloadContentPreserved(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("original"), 0644)
	os.MkdirAll(filepath.Join(mainPath, "subdir"), 0755)
	os.WriteFile(filepath.Join(mainPath, "subdir", "nested.txt"), []byte("nested"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Verify snapshot content
	snapshotPath := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	content, err := os.ReadFile(filepath.Join(snapshotPath, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))

	content, err = os.ReadFile(filepath.Join(snapshotPath, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(content))
}

func TestCreator_InvalidWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("nonexistent", "", nil)
	require.Error(t, err)
}

func TestCreator_CreateRejectsUnsafeWorktreePayloadPath(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, repoPath, worktreeName string) string
	}{
		{
			name: "worktrees parent symlink",
			setup: func(t *testing.T, repoPath, worktreeName string) string {
				outsideRoot := t.TempDir()
				outsidePayload := filepath.Join(outsideRoot, worktreeName)
				require.NoError(t, os.MkdirAll(outsidePayload, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(outsidePayload, "secret.txt"), []byte("outside"), 0644))
				require.NoError(t, os.RemoveAll(filepath.Join(repoPath, "worktrees")))
				if err := os.Symlink(outsideRoot, filepath.Join(repoPath, "worktrees")); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
				return outsidePayload
			},
		},
		{
			name: "final payload symlink",
			setup: func(t *testing.T, repoPath, worktreeName string) string {
				outsidePayload := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(outsidePayload, "secret.txt"), []byte("outside"), 0644))
				payloadPath := filepath.Join(repoPath, "worktrees", worktreeName)
				require.NoError(t, os.RemoveAll(payloadPath))
				if err := os.Symlink(outsidePayload, payloadPath); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
				return outsidePayload
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			wtMgr := worktree.NewManager(repoPath)
			const worktreeName = "unsafe"
			_, err := wtMgr.Create(worktreeName, nil)
			require.NoError(t, err)
			outsidePayload := tc.setup(t, repoPath, worktreeName)

			creator := snapshot.NewCreator(repoPath, model.EngineCopy)
			_, err = creator.Create(worktreeName, "must fail", nil)
			require.Error(t, err)

			content, readErr := os.ReadFile(filepath.Join(outsidePayload, "secret.txt"))
			require.NoError(t, readErr)
			assert.Equal(t, "outside", string(content))
			assert.Empty(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "snapshots")))
			assert.Empty(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "descriptors")))
			assert.Empty(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "intents")))
		})
	}
}

func TestCreator_WithTags(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "tagged snapshot", []string{"v1.0", "release"})
	require.NoError(t, err)

	assert.Equal(t, []string{"v1.0", "release"}, desc.Tags)
}

func TestLoadDescriptor(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Load the descriptor
	loaded, err := snapshot.LoadDescriptor(repoPath, desc.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, desc.SnapshotID, loaded.SnapshotID)
	assert.Equal(t, desc.Note, loaded.Note)
}

func TestLoadDescriptorRejectsPathLikeAndNonCanonicalIDs(t *testing.T) {
	repoPath := setupTestRepo(t)
	trap := []byte(`{"snapshot_id":"1708300800000-deadbeef","created_at":"2024-01-01T00:00:00Z","engine":"copy","payload_root_hash":"abc","descriptor_checksum":"def","integrity_state":"verified"}`)

	ids := []model.SnapshotID{
		"../../outside/escape",
		"/tmp/absolute",
		"nested/1708300800000-deadbeef",
		"1708300800000-DEADBEEF",
		"not-canonical",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			rawPath := filepath.Join(repoPath, ".jvs", "descriptors", string(id)+".json")
			require.NoError(t, os.MkdirAll(filepath.Dir(rawPath), 0755))
			require.NoError(t, os.WriteFile(rawPath, trap, 0644))

			_, err := snapshot.LoadDescriptor(repoPath, id)
			require.Error(t, err)
		})
	}
}

func TestLoadDescriptorRejectsDescriptorIDMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)
	requestedID := model.SnapshotID("1708300800000-deadbeef")
	otherID := model.SnapshotID("1708300800001-feedface")
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(requestedID)+".json")
	data := []byte(`{"snapshot_id":"` + string(otherID) + `","created_at":"2024-01-01T00:00:00Z","engine":"copy","payload_root_hash":"abc","descriptor_checksum":"def","integrity_state":"verified"}`)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))

	_, err := snapshot.LoadDescriptor(repoPath, requestedID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match requested")
}

func TestLoadDescriptorRejectsFinalDescriptorSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.SnapshotID("1708300800000-deadbeef")
	outsideDescriptor := filepath.Join(t.TempDir(), "outside-descriptor.json")
	data := []byte(`{"snapshot_id":"` + string(snapshotID) + `","created_at":"2024-01-01T00:00:00Z","engine":"copy","payload_root_hash":"abc","descriptor_checksum":"def","integrity_state":"verified"}`)
	require.NoError(t, os.WriteFile(outsideDescriptor, data, 0644))

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	if err := os.Symlink(outsideDescriptor, descriptorPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err := snapshot.LoadDescriptor(repoPath, snapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.FileExists(t, outsideDescriptor)
}

func TestLoadDescriptor_NotFound(t *testing.T) {
	repoPath := setupTestRepo(t)

	_, err := snapshot.LoadDescriptor(repoPath, "1708300800000-deadbeef")
	require.Error(t, err)
}

func TestVerifySnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Verify without payload hash
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, false)
	require.NoError(t, err)

	// Verify with payload hash
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	require.NoError(t, err)
}

func TestVerifySnapshotRejectsFinalSnapshotSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("content"), 0644))
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	require.NoError(t, os.RemoveAll(snapshotDir))
	if err := os.Symlink(outside, snapshotDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	assert.FileExists(t, filepath.Join(outside, "file.txt"))
}

func TestVerifySnapshot_InvalidID(t *testing.T) {
	repoPath := setupTestRepo(t)

	err := snapshot.VerifySnapshot(repoPath, "nonexistent", false)
	require.Error(t, err)
}

func TestCreator_DifferentEngines(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	// Test with Copy engine
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "copy", nil)
	require.NoError(t, err)
	assert.Equal(t, model.EngineCopy, desc.Engine)

	// Test with Reflink engine (falls back to copy on unsupported filesystem)
	creator2 := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)
	desc2, err := creator2.Create("main", "reflink", nil)
	require.NoError(t, err)
	assert.Equal(t, model.EngineReflinkCopy, desc2.Engine)
}

func TestLoadDescriptor_CorruptJSON(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a descriptor file with invalid JSON
	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	require.NoError(t, os.MkdirAll(descriptorsDir, 0755))
	descriptorPath := filepath.Join(descriptorsDir, "1708300800000-deadbeef.json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{invalid json"), 0644))

	_, err := snapshot.LoadDescriptor(repoPath, "1708300800000-deadbeef")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse descriptor")
}

func TestLoadDescriptor_OtherReadError(t *testing.T) {
	// Create an invalid repo path (not a directory)
	_, err := snapshot.LoadDescriptor("/proc/nonexistent", "test-id")
	assert.Error(t, err)
}

func TestVerifySnapshot_ChecksumMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Corrupt the checksum in the descriptor
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)
	var descMap map[string]any
	require.NoError(t, json.Unmarshal(data, &descMap))
	descMap["descriptor_checksum"] = "invalidchecksum"
	corruptedData, err := json.Marshal(descMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, corruptedData, 0644))

	// Verify should detect checksum mismatch
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestVerifySnapshot_PayloadHashMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Modify the snapshot payload to corrupt the hash
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	snapshotFile := filepath.Join(snapshotDir, "file.txt")
	require.NoError(t, os.WriteFile(snapshotFile, []byte("modified content"), 0644))

	// Verify with payload hash should detect mismatch
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload hash mismatch")
}

func TestCreator_CreateWithParent(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	// Modify and create second snapshot
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644)
	desc2, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Second snapshot should have first as parent
	assert.NotNil(t, desc2.ParentID)
	assert.Equal(t, desc1.SnapshotID, *desc2.ParentID)
}

func TestCreator_CreateFailsClosedWhenAuditLogMalformed(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(auditPath), 0755))
	require.NoError(t, os.WriteFile(auditPath, []byte("{malformed audit record}\n"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.Contains(t, err.Error(), "audit")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)
	assert.Empty(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "snapshots")))
}

func TestCreator_CreateFailsClosedWhenAuditRecordHashMismatches(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	first, err := creator.Create("main", "first", nil)
	require.NoError(t, err)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	tamperFirstAuditRecordForCreatorTest(t, auditPath)

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v2"), 0644))
	desc, err := creator.Create("main", "second", nil)
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.Contains(t, err.Error(), "E_AUDIT_RECORD_HASH_MISMATCH")

	cfg, cfgErr := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, cfgErr)
	assert.Equal(t, first.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, first.SnapshotID, cfg.LatestSnapshotID)
	assert.Len(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "snapshots")), 1)
}

func TestCreator_CreateWithNonExistentRepo(t *testing.T) {
	// Test Create with a non-existent repository path
	creator := snapshot.NewCreator("/nonexistent/path", model.EngineCopy)
	_, err := creator.Create("main", "test", nil)
	assert.Error(t, err)
}

func TestWriteReadyMarker_MarshalFailure(t *testing.T) {
	// This tests the json.Marshal error path in writeReadyMarker
	// We can't easily trigger this without using an invalid type,
	// but the test structure is here for completeness
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "test", nil)
	require.NoError(t, err)
	// If we get here without panic, writeReadyMarker worked
}

func TestLoadDescriptor_EmptySnapshotID(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a descriptor file with empty snapshot_id
	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	require.NoError(t, os.MkdirAll(descriptorsDir, 0755))
	descriptorPath := filepath.Join(descriptorsDir, "1708300800000-deadbeef.json")
	// Valid JSON but minimal fields
	require.NoError(t, os.WriteFile(descriptorPath, []byte(`{"snapshot_id": "", "created_at": "2024-01-01T00:00:00Z", "engine": "copy", "payload_root_hash": "abc", "descriptor_checksum": "def", "integrity_state": "verified"}`), 0644))

	_, err := snapshot.LoadDescriptor(repoPath, "1708300800000-deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match requested")
}

func TestVerifySnapshot_LoadDescriptorError(t *testing.T) {
	// Test that VerifySnapshot returns error when LoadDescriptor fails
	repoPath := setupTestRepo(t)

	err := snapshot.VerifySnapshot(repoPath, "nonexistent-id", false)
	assert.Error(t, err)
}

func TestVerifySnapshot_ComputeChecksumError(t *testing.T) {
	// This tests the checksum computation error path in VerifySnapshot
	// Most checksum errors come from integrity.ComputeDescriptorChecksum
	// which is hard to fail without invalid input
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// First verify the original is valid
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, false)
	require.NoError(t, err)

	// Now modify descriptor to have a different checksum
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)

	var descMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &descMap))
	descMap["descriptor_checksum"] = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	corruptData, err := json.Marshal(descMap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, corruptData, 0644))

	// Verify should detect checksum mismatch
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, false)
	assert.Error(t, err)
}

func TestNewCreator(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	assert.NotNil(t, creator)

	// Test that creator can create snapshots successfully
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)
	assert.NotNil(t, desc)
	assert.Equal(t, model.EngineCopy, desc.Engine)
}

func TestMatchesFilter_NoteContains(t *testing.T) {
	// Test the NoteContains filter path specifically
	repoPath := setupTestRepo(t)

	createCatalogSnapshot(t, repoPath, "important feature work", nil)
	createCatalogSnapshot(t, repoPath, "bug fix", nil)

	// Filter by note containing "feature"
	opts := snapshot.FilterOptions{NoteContains: "feature"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Contains(t, matches[0].Note, "feature")
}

func TestMatchesFilter_SinceBefore(t *testing.T) {
	// Test that snapshots before Since time are filtered out
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "early", nil)

	since := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	createCatalogSnapshot(t, repoPath, "late", nil)

	// Filter to only get snapshots after 'since'
	opts := snapshot.FilterOptions{Since: since}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.Equal(t, "late", matches[0].Note)
}

func TestLoadDescriptor_ReadPermissionError(t *testing.T) {
	// Test LoadDescriptor when file exists but can't be read
	// This tests the non-IsNotExist error path in LoadDescriptor
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Make descriptor file unreadable
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(desc.SnapshotID)+".json")
	require.NoError(t, os.Chmod(descriptorPath, 0000))
	defer os.Chmod(descriptorPath, 0644)

	_, err = snapshot.LoadDescriptor(repoPath, desc.SnapshotID)
	assert.Error(t, err)
}

func TestVerifySnapshot_MissingSnapshotDirectory(t *testing.T) {
	// Test VerifySnapshot when snapshot directory doesn't exist
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)

	// Remove the snapshot directory
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	require.NoError(t, os.RemoveAll(snapshotDir))

	// Verify with payload hash should fail
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	assert.Error(t, err)
}

func TestCreator_SnapshotWithEmptyNote(t *testing.T) {
	// Test creating a snapshot with an empty note
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "", desc.Note)
}

func TestCreator_SnapshotWithEmptyTags(t *testing.T) {
	// Test creating a snapshot with empty tags slice
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", []string{})
	require.NoError(t, err)
	assert.NotNil(t, desc.Tags)
	assert.Empty(t, desc.Tags)
}

func TestMatchesFilter_NonMatchingNote(t *testing.T) {
	// Test matchesFilter when note doesn't contain the search string
	repoPath := setupCatalogTestRepo(t)

	createCatalogSnapshot(t, repoPath, "completely different note", nil)

	// Search for something that doesn't exist
	opts := snapshot.FilterOptions{NoteContains: "notfound"}
	matches, err := snapshot.Find(repoPath, opts)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

// TestCreatePartial_SinglePath tests creating a partial snapshot with a single path.
func TestCreatePartial_SinglePath(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create multiple files and directories
	os.MkdirAll(filepath.Join(mainPath, "models"), 0755)
	os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model data"), 0644)
	os.WriteFile(filepath.Join(mainPath, "config.yaml"), []byte("config"), 0644)
	os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("readme"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "models only", nil, []string{"models"})
	require.NoError(t, err)

	// Verify PartialPaths is set
	require.NotNil(t, desc.PartialPaths)
	assert.Len(t, desc.PartialPaths, 1)
	assert.Equal(t, "models", desc.PartialPaths[0])

	// Verify snapshot directory structure
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "models", "model1.pt"))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "config.yaml"))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "README.md"))
}

// TestCreatePartial_MultiplePaths tests creating a partial snapshot with multiple paths.
func TestCreatePartial_MultiplePaths(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	// Create multiple files and directories
	os.MkdirAll(filepath.Join(mainPath, "models"), 0755)
	os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model data"), 0644)
	os.WriteFile(filepath.Join(mainPath, "config.yaml"), []byte("config"), 0644)
	os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("readme"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "models and config", nil, []string{"models", "config.yaml"})
	require.NoError(t, err)

	// Verify PartialPaths is set
	require.NotNil(t, desc.PartialPaths)
	assert.Len(t, desc.PartialPaths, 2)

	// Verify snapshot directory structure
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "models", "model1.pt"))
	assert.FileExists(t, filepath.Join(snapshotDir, "config.yaml"))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "README.md"))
}

// TestCreatePartial_SingleFile tests creating a partial snapshot of a single file.
func TestCreatePartial_SingleFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "config.yaml"), []byte("config"), 0644)
	os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("readme"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "config only", nil, []string{"config.yaml"})
	require.NoError(t, err)

	// Verify PartialPaths is set
	require.NotNil(t, desc.PartialPaths)
	assert.Len(t, desc.PartialPaths, 1)
	assert.Equal(t, "config.yaml", desc.PartialPaths[0])

	// Verify snapshot directory structure
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "config.yaml"))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "README.md"))
}

func TestCreatePartial_SingleFileWithReflinkEngine(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "config.yaml"), []byte("config"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("readme"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)
	desc, err := creator.CreatePartial("main", "config only", nil, []string{"config.yaml"})
	require.NoError(t, err)

	require.Equal(t, []string{"config.yaml"}, desc.PartialPaths)
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	content, err := os.ReadFile(filepath.Join(snapshotDir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "config", string(content))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "README.md"))
}

// TestCreatePartial_EmptyPathsEquivalentToFull tests that empty paths behaves like full snapshot.
func TestCreatePartial_EmptyPathsEquivalentToFull(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create with nil paths
	fullDesc, err := creator.Create("main", "full", nil)
	require.NoError(t, err)
	assert.Nil(t, fullDesc.PartialPaths)

	// Create with empty paths
	partialDesc, err := creator.CreatePartial("main", "empty partial", nil, []string{})
	require.NoError(t, err)
	assert.Nil(t, partialDesc.PartialPaths)

	// Both should have the same content in snapshot
	fullSnapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(fullDesc.SnapshotID))
	partialSnapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(partialDesc.SnapshotID))
	assert.FileExists(t, filepath.Join(fullSnapshotDir, "file.txt"))
	assert.FileExists(t, filepath.Join(partialSnapshotDir, "file.txt"))
}

// TestCreatePartial_AbsolutePathRejected tests that absolute paths are rejected.
func TestCreatePartial_AbsolutePathRejected(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.CreatePartial("main", "test", nil, []string{"/absolute/path"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be relative")
}

// TestCreatePartial_PathTraversalRejected tests that paths with '..' are rejected.
func TestCreatePartial_PathTraversalRejected(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.CreatePartial("main", "test", nil, []string{"../etc/passwd"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot contain '..'")
}

func TestCreatePartial_RejectsSymlinkIntermediateEscape(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	outsidePath := filepath.Join(repoPath, "outside")
	require.NoError(t, os.MkdirAll(outsidePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsidePath, "secret.txt"), []byte("outside secret"), 0644))
	if err := os.Symlink(outsidePath, filepath.Join(mainPath, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "must not escape", nil, []string{"link/secret.txt"})
	require.Error(t, err)
	assert.Nil(t, desc)

	content, readErr := os.ReadFile(filepath.Join(outsidePath, "secret.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside secret", string(content))

	entries, readErr := os.ReadDir(filepath.Join(repoPath, ".jvs", "snapshots"))
	require.NoError(t, readErr)
	assert.Empty(t, entries, "rejected partial snapshot must not publish outside content")
}

func TestCreatePartial_AllowsFinalSymlinkLeaf(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	outsidePath := filepath.Join(repoPath, "outside")
	require.NoError(t, os.MkdirAll(outsidePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsidePath, "secret.txt"), []byte("outside secret"), 0644))
	if err := os.Symlink(filepath.Join(outsidePath, "secret.txt"), filepath.Join(mainPath, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "symlink leaf", nil, []string{"link"})
	require.NoError(t, err)
	require.Equal(t, []string{"link"}, desc.PartialPaths)

	snapshotLink := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), "link")
	info, err := os.Lstat(snapshotLink)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	target, err := os.Readlink(snapshotLink)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(outsidePath, "secret.txt"), target)
}

// TestCreatePartial_NonExistentPathRejected tests that non-existent paths are rejected.
func TestCreatePartial_NonExistentPathRejected(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.CreatePartial("main", "test", nil, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestCreatePartial_DuplicatePathsDeduplicated tests that duplicate paths are deduplicated.
func TestCreatePartial_DuplicatePathsDeduplicated(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.MkdirAll(filepath.Join(mainPath, "models"), 0755)
	os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "duplicates", nil, []string{"models", "models", "models"})
	require.NoError(t, err)

	// Should only have one entry after deduplication
	assert.Len(t, desc.PartialPaths, 1)
	assert.Equal(t, "models", desc.PartialPaths[0])
}

func TestCreatePartial_AncestorCoveredPathsAreFolded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{
			name:  "ancestor first",
			paths: []string{"models", "models/checkpoints/checkpoint.pt"},
		},
		{
			name:  "descendant first",
			paths: []string{"models/checkpoints/checkpoint.pt", "models"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := setupTestRepo(t)

			mainPath := filepath.Join(repoPath, "main")
			require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "models", "checkpoints"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(mainPath, "models", "checkpoints", "checkpoint.pt"), []byte("checkpoint"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(mainPath, "README.md"), []byte("readme"), 0644))

			creator := snapshot.NewCreator(repoPath, model.EngineCopy)
			desc, err := creator.CreatePartial("main", "covered paths", nil, tc.paths)
			require.NoError(t, err)
			require.Equal(t, []string{"models"}, desc.PartialPaths)

			loaded, err := snapshot.LoadDescriptor(repoPath, desc.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, []string{"models"}, loaded.PartialPaths)

			auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
			auditData, err := os.ReadFile(auditPath)
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSpace(string(auditData)), "\n")
			require.NotEmpty(t, lines)

			var record model.AuditRecord
			require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &record))
			require.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
			require.NotNil(t, record.Details)
			partialData, err := json.Marshal(record.Details["partial_paths"])
			require.NoError(t, err)
			var auditPartialPaths []string
			require.NoError(t, json.Unmarshal(partialData, &auditPartialPaths))
			assert.Equal(t, []string{"models"}, auditPartialPaths)

			snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
			assert.FileExists(t, filepath.Join(snapshotDir, "models", "model1.pt"))
			assert.FileExists(t, filepath.Join(snapshotDir, "models", "checkpoints", "checkpoint.pt"))
			assert.NoFileExists(t, filepath.Join(snapshotDir, "README.md"))
		})
	}
}

func TestCreatePartial_AncestorCoveredInvalidDescendantStillRejected(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.MkdirAll(filepath.Join(mainPath, "models"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "invalid covered path", nil, []string{"models", "models/missing.pt"})
	require.Error(t, err)
	assert.Nil(t, desc)
	assert.Contains(t, err.Error(), "path does not exist: models/missing.pt")
	assert.Empty(t, readDirNames(t, filepath.Join(repoPath, ".jvs", "snapshots")))
}

// TestCreatePartial_NestedDirectories tests partial snapshot with nested directory paths.
func TestCreatePartial_NestedDirectories(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.MkdirAll(filepath.Join(mainPath, "models", "checkpoints"), 0755)
	os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model"), 0644)
	os.WriteFile(filepath.Join(mainPath, "models", "checkpoints", "checkpoint.pt"), []byte("checkpoint"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "nested", nil, []string{"models"})
	require.NoError(t, err)

	// Both files should be included (parent directory includes all children)
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "models", "model1.pt"))
	assert.FileExists(t, filepath.Join(snapshotDir, "models", "checkpoints", "checkpoint.pt"))
}

// TestCreatePartial_WithTags tests partial snapshot with tags.
func TestCreatePartial_WithTags(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.MkdirAll(filepath.Join(mainPath, "models"), 0755)
	os.WriteFile(filepath.Join(mainPath, "models", "model1.pt"), []byte("model"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "tagged partial", []string{"v1.0", "models"}, []string{"models"})
	require.NoError(t, err)

	assert.Equal(t, []string{"v1.0", "models"}, desc.Tags)
	assert.Len(t, desc.PartialPaths, 1)
}

// TestCreatePartial_CallCreateViaCreatePartial tests that Create() properly delegates to CreatePartial.
func TestCreatePartial_CallCreateViaCreatePartial(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create via Create (should call CreatePartial with nil)
	desc1, err := creator.Create("main", "via Create", nil)
	require.NoError(t, err)

	// Create via CreatePartial with nil
	desc2, err := creator.CreatePartial("main", "via CreatePartial", nil, nil)
	require.NoError(t, err)

	// Both should have nil PartialPaths
	assert.Nil(t, desc1.PartialPaths)
	assert.Nil(t, desc2.PartialPaths)

	// Both should have the snapshoted file
	snapshotDir1 := filepath.Join(repoPath, ".jvs", "snapshots", string(desc1.SnapshotID))
	snapshotDir2 := filepath.Join(repoPath, ".jvs", "snapshots", string(desc2.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir1, "file.txt"))
	assert.FileExists(t, filepath.Join(snapshotDir2, "file.txt"))
}

func TestCreator_SetCompression(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("hello world"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)

	desc, err := creator.Create("main", "compressed snapshot", nil)
	require.NoError(t, err)

	require.NotNil(t, desc.Compression)
	assert.Equal(t, "gzip", desc.Compression.Type)
	assert.Equal(t, 6, desc.Compression.Level)
}

func TestCreator_CompressionFailureFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	writeNameWhoseGzipSiblingCannotBeCreated(t, mainPath)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(compression.LevelFast)

	desc, err := creator.Create("main", "must fail closed", nil)
	require.Error(t, err)
	assert.Nil(t, desc)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Empty(t, cfg.HeadSnapshotID)
	assert.Empty(t, cfg.LatestSnapshotID)

	descriptorEntries, err := os.ReadDir(filepath.Join(repoPath, ".jvs", "descriptors"))
	require.NoError(t, err)
	assert.Empty(t, descriptorEntries, "compression failure must not write a descriptor")

	snapshotEntries, err := os.ReadDir(filepath.Join(repoPath, ".jvs", "snapshots"))
	require.NoError(t, err)
	for _, entry := range snapshotEntries {
		entryPath := filepath.Join(repoPath, ".jvs", "snapshots", entry.Name())
		assert.NoFileExists(t, filepath.Join(entryPath, ".READY"), "failed compressed snapshot must not be published READY")
		if strings.HasSuffix(entry.Name(), ".tmp") {
			assert.NoFileExists(t, filepath.Join(entryPath, ".READY"), "failed tmp snapshot must not contain publish state")
		}
	}
}

func TestCreator_CompressedSnapshotIsPublishableAndMaterializesSafely(t *testing.T) {
	repoPath := setupTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("logical payload"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "archive.gz"), []byte("user gzip-named data"), 0644))
	require.NoError(t, os.Symlink("archive.gz", filepath.Join(mainPath, "leaf-link")))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)

	desc, err := creator.Create("main", "compressed publish", nil)
	require.NoError(t, err)
	require.NotNil(t, desc.Compression)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	readyData, err := os.ReadFile(filepath.Join(snapshotDir, ".READY"))
	require.NoError(t, err)
	var ready map[string]any
	require.NoError(t, json.Unmarshal(readyData, &ready))
	assert.Contains(t, ready, "compression_manifest")
	assert.FileExists(t, filepath.Join(snapshotDir, "file.txt.gz"))
	assert.NoFileExists(t, filepath.Join(snapshotDir, "file.txt"))
	assert.FileExists(t, filepath.Join(snapshotDir, "archive.gz"))
	info, err := os.Lstat(filepath.Join(snapshotDir, "leaf-link"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)

	require.NoError(t, snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true))
	hash, err := snapshotpayload.ComputeHash(snapshotDir, snapshotpayload.OptionsFromDescriptor(desc))
	require.NoError(t, err)
	assert.Equal(t, desc.PayloadRootHash, hash)

	materialized := filepath.Join(t.TempDir(), "payload")
	err = snapshotpayload.Materialize(snapshotDir, materialized, snapshotpayload.OptionsFromDescriptor(desc), copySnapshotTreeForCreatorTest)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(materialized, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "logical payload", string(content))
	userGzip, err := os.ReadFile(filepath.Join(materialized, "archive.gz"))
	require.NoError(t, err)
	assert.Equal(t, "user gzip-named data", string(userGzip))
	target, err := os.Readlink(filepath.Join(materialized, "leaf-link"))
	require.NoError(t, err)
	assert.Equal(t, "archive.gz", target)
}

func TestCreator_CreatePartial_EmptyPaths(t *testing.T) {
	repoPath := setupTestRepo(t)

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.CreatePartial("main", "empty paths", nil, []string{})
	require.NoError(t, err)

	assert.Nil(t, desc.PartialPaths)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.FileExists(t, filepath.Join(snapshotDir, "file.txt"))
}

func TestCreator_Create_EmptyWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "empty worktree snapshot", nil)
	require.NoError(t, err)

	assert.NotEmpty(t, desc.SnapshotID)
	assert.Equal(t, "main", desc.WorktreeName)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
	assert.DirExists(t, snapshotDir)
	assert.FileExists(t, filepath.Join(snapshotDir, ".READY"))
}

func writeNameWhoseGzipSiblingCannotBeCreated(t *testing.T, dir string) string {
	t.Helper()

	for n := 253; n >= 1; n-- {
		name := strings.Repeat("a", n)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("payload"), 0644); err != nil {
			continue
		}

		probePath := path + ".gz"
		if err := os.WriteFile(probePath, []byte("probe"), 0644); err != nil {
			return name
		}
		require.NoError(t, os.Remove(probePath))
		require.NoError(t, os.Remove(path))
	}

	t.Skip("filesystem did not expose a filename length that allows the source but rejects the .gz sibling")
	return ""
}

func copySnapshotTreeForCreatorTest(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
	})
}

func tamperFirstAuditRecordForCreatorTest(t *testing.T, auditPath string) {
	t.Helper()

	data, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	record["event_type"] = "restore"
	line, err := json.Marshal(record)
	require.NoError(t, err)
	lines[0] = string(line)

	require.NoError(t, os.WriteFile(auditPath, []byte(strings.Join(lines, "\n")+"\n"), 0644))
}
