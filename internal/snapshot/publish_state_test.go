package snapshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectPublishStateMatrix(t *testing.T) {
	tests := []struct {
		name     string
		wantCode string
		mutate   func(t *testing.T, repoPath string, snapshotID model.SnapshotID)
	}{
		{
			name:     "missing READY",
			wantCode: "E_READY_MISSING",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")))
			},
		},
		{
			name:     "invalid READY leaf",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")
				require.NoError(t, os.Remove(readyPath))
				require.NoError(t, os.Mkdir(readyPath, 0755))
			},
		},
		{
			name:     "malformed READY json",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY"), []byte("{not json"), 0644))
			},
		},
		{
			name:     "READY snapshot id mismatch",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				mutatePublishStateReady(t, repoPath, snapshotID, func(marker map[string]any) {
					marker["snapshot_id"] = "1708300800000-deadbeef"
				})
			},
		},
		{
			name:     "READY payload hash mismatch",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				mutatePublishStateReady(t, repoPath, snapshotID, func(marker map[string]any) {
					marker["payload_root_hash"] = "bad-ready-payload-hash"
				})
			},
		},
		{
			name:     "READY engine mismatch",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				mutatePublishStateReady(t, repoPath, snapshotID, func(marker map[string]any) {
					marker["engine"] = string(model.EngineReflinkCopy)
				})
			},
		},
		{
			name:     "READY gzip engine mismatch",
			wantCode: "E_READY_INVALID",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")
				readyGzipPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY.gz")
				data, err := os.ReadFile(readyPath)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(readyGzipPath, data, 0644))
				require.NoError(t, os.Remove(readyPath))
				mutatePublishStateReadyFile(t, readyGzipPath, func(marker map[string]any) {
					marker["engine"] = string(model.EngineReflinkCopy)
				})
			},
		},
		{
			name:     "descriptor invalid json",
			wantCode: "E_DESCRIPTOR_CORRUPT",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json"), []byte("{not json"), 0644))
			},
		},
		{
			name:     "descriptor checksum mismatch",
			wantCode: "E_DESCRIPTOR_CHECKSUM_MISMATCH",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				mutatePublishStateDescriptor(t, repoPath, snapshotID, func(desc *model.Descriptor) {
					desc.DescriptorChecksum = "bad-descriptor-checksum"
				})
				mutatePublishStateReady(t, repoPath, snapshotID, func(marker map[string]any) {
					marker["descriptor_checksum"] = "bad-descriptor-checksum"
				})
			},
		},
		{
			name:     "READY without descriptor",
			wantCode: "E_READY_DESCRIPTOR_MISSING",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")))
			},
		},
		{
			name:     "descriptor without payload",
			wantCode: "E_PAYLOAD_MISSING",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))))
			},
		},
		{
			name:     "payload mismatch",
			wantCode: "E_PAYLOAD_HASH_MISMATCH",
			mutate: func(t *testing.T, repoPath string, snapshotID model.SnapshotID) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), "tampered.txt"), []byte("tampered"), 0644))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath, snapshotID := createPublishStateSnapshot(t)
			tt.mutate(t, repoPath, snapshotID)

			_, issue := snapshot.InspectPublishState(repoPath, snapshotID, snapshot.PublishStateOptions{
				RequireReady:             true,
				RequirePayload:           true,
				VerifyDescriptorChecksum: true,
				VerifyPayloadHash:        true,
			})

			require.NotNil(t, issue)
			assert.Equal(t, tt.wantCode, issue.Code)
		})
	}
}

func TestInspectPublishStateReadyGzipAliasDoesNotAffectPayloadHash(t *testing.T) {
	repoPath, snapshotID := createPublishStateSnapshot(t)
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	readyPath := filepath.Join(snapshotDir, ".READY")
	readyGzipPath := filepath.Join(snapshotDir, ".READY.gz")

	data, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readyGzipPath, data, 0644))
	require.NoError(t, os.Remove(readyPath))

	state, issue := snapshot.InspectPublishState(repoPath, snapshotID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})

	require.Nil(t, issue)
	require.NotNil(t, state)
	assert.Equal(t, readyGzipPath, state.ReadyPath)
}

func TestInspectPublishStateDescriptorErrorDoesNotClassifyByMessageText(t *testing.T) {
	repoPath, snapshotID := createPublishStateSnapshot(t)
	mutatePublishStateDescriptor(t, repoPath, snapshotID, func(desc *model.Descriptor) {
		desc.SnapshotID = "not found"
	})

	_, issue := snapshot.InspectPublishState(repoPath, snapshotID, snapshot.PublishStateOptions{
		RequireReady: true,
	})

	require.NotNil(t, issue)
	assert.Equal(t, "E_DESCRIPTOR_CORRUPT", issue.Code)
}

func createPublishStateSnapshot(t *testing.T) (string, model.SnapshotID) {
	t.Helper()
	repoPath := t.TempDir()
	_, err := repo.Init(repoPath, "test")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "main", "file.txt"), []byte("content"), 0644))

	desc, err := snapshot.NewCreator(repoPath, model.EngineCopy).Create("main", "publish-state", nil)
	require.NoError(t, err)
	return repoPath, desc.SnapshotID
}

func mutatePublishStateReady(t *testing.T, repoPath string, snapshotID model.SnapshotID, mutate func(map[string]any)) {
	t.Helper()
	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")
	mutatePublishStateReadyFile(t, readyPath, mutate)
}

func mutatePublishStateReadyFile(t *testing.T, readyPath string, mutate func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	var marker map[string]any
	require.NoError(t, json.Unmarshal(data, &marker))
	mutate(marker)
	data, err = json.Marshal(marker)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readyPath, data, 0644))
}

func mutatePublishStateDescriptor(t *testing.T, repoPath string, snapshotID model.SnapshotID, mutate func(*model.Descriptor)) {
	t.Helper()
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)
	var desc model.Descriptor
	require.NoError(t, json.Unmarshal(data, &desc))
	mutate(&desc)
	data, err = json.MarshalIndent(&desc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
}

func rewritePublishStateDescriptorChecksum(t *testing.T, repoPath string, snapshotID model.SnapshotID) model.HashValue {
	t.Helper()
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)
	var desc model.Descriptor
	require.NoError(t, json.Unmarshal(data, &desc))
	checksum, err := integrity.ComputeDescriptorChecksum(&desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum
	data, err = json.MarshalIndent(&desc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
	return checksum
}
