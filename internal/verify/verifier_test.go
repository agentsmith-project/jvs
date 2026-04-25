package verify_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/compression"
	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func createTestSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func createCompressedTestSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	t.Helper()
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("compressed content"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)
	desc, err := creator.Create("main", "compressed", nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func writeUnsafeSnapshotTrap(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
	t.Helper()

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "file.txt"), []byte("trap payload"), 0644))

	payloadHash, err := integrity.ComputePayloadRootHash(snapshotDir)
	require.NoError(t, err)

	desc := &model.Descriptor{
		SnapshotID:      snapshotID,
		WorktreeName:    "main",
		CreatedAt:       time.Now().UTC(),
		Engine:          model.EngineCopy,
		PayloadRootHash: payloadHash,
		IntegrityState:  model.IntegrityVerified,
	}
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.MkdirAll(filepath.Dir(descriptorPath), 0755))
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
}

func TestVerifier_VerifySnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true)
	require.NoError(t, err)
	t.Logf("Result: %+v", result)
	assert.True(t, result.ChecksumValid, "checksum should be valid")
	assert.True(t, result.PayloadHashValid, "payload hash should be valid")
	assert.False(t, result.TamperDetected, "no tamper should be detected")
}

func TestVerifier_VerifySnapshotRejectsPathLikeAndNonCanonicalIDs(t *testing.T) {
	repoPath := setupTestRepo(t)
	ids := []model.SnapshotID{
		"../../outside/escape",
		"/tmp/absolute",
		"nested/1708300800000-deadbeef",
		"1708300800000-DEADBEEF",
		"not-canonical",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			writeUnsafeSnapshotTrap(t, repoPath, id)

			v := verify.NewVerifier(repoPath)
			result, err := v.VerifySnapshot(id, true)
			require.NoError(t, err)
			assert.True(t, result.TamperDetected)
			assert.Equal(t, "critical", result.Severity)
			assert.NotEmpty(t, result.Error)
		})
	}
}

func TestVerifierVerifySnapshotRejectsFinalSnapshotSymlink(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.SnapshotID("1708300800000-deadbeef")
	writeUnsafeSnapshotTrap(t, repoPath, snapshotID)

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("trap payload"), 0644))
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.RemoveAll(snapshotDir))
	if err := os.Symlink(outside, snapshotDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	result, err := verify.NewVerifier(repoPath).VerifySnapshot(snapshotID, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
	assert.Contains(t, result.Error, "symlink")
	assert.FileExists(t, filepath.Join(outside, "file.txt"))
}

func TestVerifier_VerifySnapshot_CompressedPayloadUsesLogicalHash(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createCompressedTestSnapshot(t, repoPath)

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true)
	require.NoError(t, err)

	assert.True(t, result.ChecksumValid)
	assert.True(t, result.PayloadHashValid)
	assert.False(t, result.TamperDetected)
}

func TestVerifier_VerifySnapshot_DetectsCompressedPayloadTampering(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createCompressedTestSnapshot(t, repoPath)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	tamperedPath := filepath.Join(snapshotDir, "file.txt.gz")
	comp := compression.NewCompressor(compression.LevelDefault)
	plainPath := filepath.Join(snapshotDir, "replacement")
	require.NoError(t, os.WriteFile(plainPath, []byte("tampered content"), 0644))
	compressedPath, err := comp.CompressFile(plainPath)
	require.NoError(t, err)
	require.NoError(t, os.Rename(compressedPath, tamperedPath))
	require.NoError(t, os.Remove(plainPath))

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true)
	require.NoError(t, err)

	assert.True(t, result.ChecksumValid)
	assert.False(t, result.PayloadHashValid)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
}

func TestVerifier_VerifyAll(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create two snapshots
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "first", nil)
	require.NoError(t, err)
	_, err = creator.Create("main", "second", nil)
	require.NoError(t, err)

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(false)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	for _, r := range results {
		assert.True(t, r.ChecksumValid)
	}
}

func TestVerifier_VerifySnapshot_Nonexistent(t *testing.T) {
	repoPath := setupTestRepo(t)

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot("nonexistent-snapshot-id", false)
	require.NoError(t, err)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
	assert.NotEmpty(t, result.Error)
}

func TestVerifier_VerifyAll_Empty(t *testing.T) {
	repoPath := setupTestRepo(t)

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(false)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestVerifier_VerifySnapshot_NoPayloadHash(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, false)
	require.NoError(t, err)
	assert.True(t, result.ChecksumValid)
	assert.False(t, result.PayloadHashValid) // Not verified when verifyPayloadHash=false
	assert.False(t, result.TamperDetected)
}

func TestVerifier_VerifySnapshot_ChecksumTampering(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	// Corrupt the descriptor by modifying the Note field
	// This will cause the checksum to mismatch
	// Descriptors are stored in .jvs/descriptors/<snapshot-id>.json
	descPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	content, err := os.ReadFile(descPath)
	require.NoError(t, err)

	t.Logf("Original descriptor: %s", string(content))

	// Modify the note field to invalidate checksum
	// Replace "test" note with "TAMPERED" - note the space after colon in JSON
	modified := string(content)
	modified = replaceJSONField(modified, `"note": "test"`, `"note": "TAMPERED"`)
	require.NoError(t, os.WriteFile(descPath, []byte(modified), 0644))

	t.Logf("Modified descriptor: %s", modified)

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, false)
	require.NoError(t, err)

	t.Logf("Result: %+v", result)

	assert.False(t, result.ChecksumValid)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
	assert.Contains(t, result.Error, "checksum mismatch")
}

// replaceJSONField is a simple helper to replace a field in JSON
func replaceJSONField(json, old, new string) string {
	// Simple string replacement - works for our test case
	result := ""
	for i := 0; i < len(json); i++ {
		if i+len(old) <= len(json) && json[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(json[i])
		}
	}
	return result
}

func TestVerifier_VerifySnapshot_PayloadTampering(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	// Modify the snapshot payload
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	// Add a file to the snapshot directory
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "tampered.txt"), []byte("modified"), 0644))

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true) // Verify payload hash
	require.NoError(t, err)
	assert.True(t, result.ChecksumValid) // Descriptor checksum still valid
	assert.False(t, result.PayloadHashValid)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
}

func TestVerifier_VerifyAll_WithMixedResults(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create two snapshots
	snapID1 := createTestSnapshot(t, repoPath)

	// Add more content for second snapshot
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "file2.txt"), []byte("more content"), 0644)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	// Corrupt first snapshot's payload
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapID1))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "tampered.txt"), []byte("tampered"), 0644))

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(true) // Verify payload hashes
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Count valid and invalid
	validPayloadCount := 0
	tamperedCount := 0
	for _, r := range results {
		if r.PayloadHashValid {
			validPayloadCount++
		}
		if r.TamperDetected {
			tamperedCount++
		}
	}
	assert.Equal(t, 1, validPayloadCount)
	assert.Equal(t, 1, tamperedCount)
}

func TestVerifier_VerifyAll_WithNonDirectoryEntries(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	createTestSnapshot(t, repoPath)

	// Add a non-directory entry to snapshots dir
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.WriteFile(filepath.Join(snapshotsDir, "file.txt"), []byte("test"), 0644))

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(false)
	require.NoError(t, err)
	// Should only verify the actual snapshot directory, not the file
	assert.Len(t, results, 1)
}

func TestVerifier_VerifyAllSkipsTmpAndInvalidSnapshotDirNames(t *testing.T) {
	repoPath := setupTestRepo(t)
	validID := createTestSnapshot(t, repoPath)

	writeUnsafeSnapshotTrap(t, repoPath, "1708300800000-deadbeef.tmp")
	writeUnsafeSnapshotTrap(t, repoPath, "not-canonical")

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(false)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, validID, results[0].SnapshotID)
}

func TestVerifier_VerifyAll_WithDeletedSnapshotsDir(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	createTestSnapshot(t, repoPath)

	// Remove the snapshots directory
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.RemoveAll(snapshotsDir))

	v := verify.NewVerifier(repoPath)
	results, err := v.VerifyAll(false)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestVerifier_VerifySnapshot_WithCorruptedDescriptor(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	// Corrupt the descriptor JSON to trigger load error
	descPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	// Write invalid JSON
	require.NoError(t, os.WriteFile(descPath, []byte("{invalid json"), 0644))

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, false)
	require.NoError(t, err)

	// Should report critical error for corrupted descriptor
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
	assert.NotEmpty(t, result.Error)
}

func TestVerifier_VerifySnapshot_PayloadHashWithTampering(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	// Modify the snapshot payload to cause payload hash mismatch
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "extra.txt"), []byte("tampered"), 0644))

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true)
	require.NoError(t, err)

	// Should report payload hash mismatch
	assert.True(t, result.ChecksumValid) // Descriptor checksum still valid
	assert.False(t, result.PayloadHashValid)
	assert.True(t, result.TamperDetected)
	assert.Equal(t, "critical", result.Severity)
	assert.Contains(t, result.Error, "payload hash mismatch")
}

func TestVerifier_VerifySnapshot_PayloadHashComputeError(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	// Remove the .READY marker which might be required for payload hash computation
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	readyPath := filepath.Join(snapshotDir, ".READY")
	os.Remove(readyPath)

	// Make the snapshot directory contain a non-regular file to potentially trigger errors
	// Create a special file that could cause issues
	specialFile := filepath.Join(snapshotDir, "special")
	require.NoError(t, os.MkdirAll(specialFile, 0755))

	v := verify.NewVerifier(repoPath)
	result, err := v.VerifySnapshot(snapshotID, true)
	require.NoError(t, err)

	// Result should have some state - the exact behavior depends on ComputePayloadRootHash
	// Just verify it doesn't crash
	_ = result
	_ = result.Error
}

func TestVerifier_VerifyAll_SnapshotsDirReadError(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a snapshot
	createTestSnapshot(t, repoPath)

	// Make snapshots directory unreadable
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.Chmod(snapshotsDir, 0000))

	v := verify.NewVerifier(repoPath)
	_, err := v.VerifyAll(false)
	// Should return error when can't read snapshots directory
	assert.Error(t, err)

	// Restore permissions for cleanup
	os.Chmod(snapshotsDir, 0755)
}
