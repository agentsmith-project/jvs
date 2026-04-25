package cli

import (
	"time"

	clidiff "github.com/jvs-project/jvs/internal/diff"
	cliverify "github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/pkg/model"
)

type publicCheckpointRecord struct {
	CheckpointID       string               `json:"checkpoint_id"`
	ParentCheckpointID string               `json:"parent_checkpoint_id,omitempty"`
	Workspace          string               `json:"workspace"`
	CreatedAt          time.Time            `json:"created_at"`
	Note               string               `json:"note,omitempty"`
	Tags               []string             `json:"tags,omitempty"`
	Engine             model.EngineType     `json:"engine"`
	PayloadRootHash    model.HashValue      `json:"payload_root_hash"`
	DescriptorChecksum model.HashValue      `json:"descriptor_checksum"`
	IntegrityState     model.IntegrityState `json:"integrity_state"`
}

type publicWorkspaceRecord struct {
	Workspace      string    `json:"workspace"`
	BaseCheckpoint string    `json:"base_checkpoint,omitempty"`
	Current        string    `json:"current,omitempty"`
	Latest         string    `json:"latest,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type publicDiffResult struct {
	FromCheckpoint string            `json:"from_checkpoint"`
	ToCheckpoint   string            `json:"to_checkpoint"`
	FromTime       time.Time         `json:"from_time"`
	ToTime         time.Time         `json:"to_time"`
	Added          []*clidiff.Change `json:"added"`
	Removed        []*clidiff.Change `json:"removed"`
	Modified       []*clidiff.Change `json:"modified"`
	TotalAdded     int               `json:"total_added"`
	TotalRemoved   int               `json:"total_removed"`
	TotalModified  int               `json:"total_modified"`
}

type publicVerifyResult struct {
	CheckpointID     string `json:"checkpoint_id"`
	ChecksumValid    bool   `json:"checksum_valid"`
	PayloadHashValid bool   `json:"payload_hash_valid"`
	TamperDetected   bool   `json:"tamper_detected"`
	Severity         string `json:"severity,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorCode        string `json:"error_code,omitempty"`
}

func publicCheckpoint(desc *model.Descriptor) publicCheckpointRecord {
	record := publicCheckpointRecord{
		CheckpointID:       string(desc.SnapshotID),
		Workspace:          desc.WorktreeName,
		CreatedAt:          desc.CreatedAt,
		Note:               desc.Note,
		Tags:               desc.Tags,
		Engine:             desc.Engine,
		PayloadRootHash:    desc.PayloadRootHash,
		DescriptorChecksum: desc.DescriptorChecksum,
		IntegrityState:     desc.IntegrityState,
	}
	if desc.ParentID != nil {
		record.ParentCheckpointID = string(*desc.ParentID)
	}
	return record
}

func publicCheckpoints(descs []*model.Descriptor) []publicCheckpointRecord {
	records := make([]publicCheckpointRecord, 0, len(descs))
	for _, desc := range descs {
		records = append(records, publicCheckpoint(desc))
	}
	return records
}

func publicWorkspace(cfg *model.WorktreeConfig) publicWorkspaceRecord {
	return publicWorkspaceRecord{
		Workspace:      cfg.Name,
		BaseCheckpoint: string(cfg.BaseSnapshotID),
		Current:        string(cfg.HeadSnapshotID),
		Latest:         string(cfg.LatestSnapshotID),
		CreatedAt:      cfg.CreatedAt,
	}
}

func publicWorkspaces(configs []*model.WorktreeConfig) []publicWorkspaceRecord {
	records := make([]publicWorkspaceRecord, 0, len(configs))
	for _, cfg := range configs {
		records = append(records, publicWorkspace(cfg))
	}
	return records
}

func publicDiff(result *clidiff.DiffResult) publicDiffResult {
	return publicDiffResult{
		FromCheckpoint: string(result.FromSnapshotID),
		ToCheckpoint:   string(result.ToSnapshotID),
		FromTime:       result.FromTime,
		ToTime:         result.ToTime,
		Added:          result.Added,
		Removed:        result.Removed,
		Modified:       result.Modified,
		TotalAdded:     result.TotalAdded,
		TotalRemoved:   result.TotalRemoved,
		TotalModified:  result.TotalModified,
	}
}

func publicVerify(result *cliverify.Result) publicVerifyResult {
	return publicVerifyResult{
		CheckpointID:     string(result.SnapshotID),
		ChecksumValid:    result.ChecksumValid,
		PayloadHashValid: result.PayloadHashValid,
		TamperDetected:   result.TamperDetected,
		Severity:         result.Severity,
		Error:            result.Error,
		ErrorCode:        result.ErrorCode,
	}
}

func publicVerifyResults(results []*cliverify.Result) []publicVerifyResult {
	records := make([]publicVerifyResult, 0, len(results))
	for _, result := range results {
		records = append(records, publicVerify(result))
	}
	return records
}
