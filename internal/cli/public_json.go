package cli

import (
	"strings"
	"time"

	clidiff "github.com/agentsmith-project/jvs/internal/diff"
	clidoctor "github.com/agentsmith-project/jvs/internal/doctor"
	cliverify "github.com/agentsmith-project/jvs/internal/verify"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type publicCheckpointRecord struct {
	CheckpointID         string                      `json:"checkpoint_id"`
	ParentCheckpointID   string                      `json:"parent_checkpoint_id,omitempty"`
	Workspace            string                      `json:"workspace"`
	CreatedAt            time.Time                   `json:"created_at"`
	Note                 string                      `json:"note,omitempty"`
	Tags                 []string                    `json:"tags,omitempty"`
	Engine               model.EngineType            `json:"engine"`
	ActualEngine         model.EngineType            `json:"actual_engine,omitempty"`
	EffectiveEngine      model.EngineType            `json:"effective_engine,omitempty"`
	DegradedReasons      []string                    `json:"degraded_reasons,omitempty"`
	MetadataPreservation *model.MetadataPreservation `json:"metadata_preservation,omitempty"`
	PerformanceClass     string                      `json:"performance_class,omitempty"`
	PayloadRootHash      model.HashValue             `json:"payload_root_hash"`
	DescriptorChecksum   model.HashValue             `json:"descriptor_checksum"`
	IntegrityState       model.IntegrityState        `json:"integrity_state"`
}

type publicSavePointRecord struct {
	SavePointID string    `json:"save_point_id"`
	Workspace   string    `json:"workspace"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type publicSavePointCreatedRecord struct {
	SavePointID     string                     `json:"save_point_id"`
	Workspace       string                     `json:"workspace"`
	Message         string                     `json:"message"`
	CreatedAt       time.Time                  `json:"created_at"`
	NewestSavePoint string                     `json:"newest_save_point"`
	RestoredFrom    string                     `json:"restored_from,omitempty"`
	RestoredPaths   []publicRestoredPathSource `json:"restored_paths,omitempty"`
	UnsavedChanges  bool                       `json:"unsaved_changes"`
}

type publicSavePointHistoryRecord struct {
	Workspace       string                  `json:"workspace"`
	SavePoints      []publicSavePointRecord `json:"save_points"`
	NewestSavePoint string                  `json:"newest_save_point,omitempty"`
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

type publicDoctorResult struct {
	Healthy  bool                  `json:"healthy"`
	Findings []publicDoctorFinding `json:"findings"`
	Repairs  []publicDoctorRepair  `json:"repairs,omitempty"`
}

type publicDoctorFinding struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	ErrorCode   string `json:"error_code,omitempty"`
}

type publicDoctorRepair struct {
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Cleaned int    `json:"cleaned,omitempty"`
}

type publicGCPlan struct {
	PlanID                 string    `json:"plan_id"`
	CreatedAt              time.Time `json:"created_at"`
	ProtectedCheckpoints   []string  `json:"protected_checkpoints"`
	ProtectedByLineage     int       `json:"protected_by_lineage"`
	CandidateCount         int       `json:"candidate_count"`
	ToDelete               []string  `json:"to_delete"`
	DeletableBytesEstimate int64     `json:"deletable_bytes_estimate"`
}

type publicRestoredPathSource struct {
	TargetPath      string                 `json:"target_path"`
	SourceSavePoint string                 `json:"source_save_point"`
	SourcePath      string                 `json:"source_path"`
	Status          model.PathSourceStatus `json:"status"`
}

func publicCheckpoint(desc *model.Descriptor) publicCheckpointRecord {
	record := publicCheckpointRecord{
		CheckpointID:         string(desc.SnapshotID),
		Workspace:            desc.WorktreeName,
		CreatedAt:            desc.CreatedAt,
		Note:                 desc.Note,
		Tags:                 desc.Tags,
		Engine:               desc.Engine,
		ActualEngine:         desc.ActualEngine,
		EffectiveEngine:      desc.EffectiveEngine,
		DegradedReasons:      desc.DegradedReasons,
		MetadataPreservation: desc.MetadataPreservation,
		PerformanceClass:     desc.PerformanceClass,
		PayloadRootHash:      desc.PayloadRootHash,
		DescriptorChecksum:   desc.DescriptorChecksum,
		IntegrityState:       desc.IntegrityState,
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

func publicSavePoint(desc *model.Descriptor) publicSavePointRecord {
	return publicSavePointRecord{
		SavePointID: string(desc.SnapshotID),
		Workspace:   desc.WorktreeName,
		Message:     desc.Note,
		CreatedAt:   desc.CreatedAt,
	}
}

func publicSavePoints(descs []*model.Descriptor) []publicSavePointRecord {
	records := make([]publicSavePointRecord, 0, len(descs))
	for _, desc := range descs {
		records = append(records, publicSavePoint(desc))
	}
	return records
}

func publicSavePointCreated(desc *model.Descriptor, unsavedChanges bool) publicSavePointCreatedRecord {
	record := publicSavePointCreatedRecord{
		SavePointID:     string(desc.SnapshotID),
		Workspace:       desc.WorktreeName,
		Message:         desc.Note,
		CreatedAt:       desc.CreatedAt,
		NewestSavePoint: string(desc.SnapshotID),
		UnsavedChanges:  unsavedChanges,
	}
	if desc.RestoredFrom != nil {
		record.RestoredFrom = string(*desc.RestoredFrom)
	}
	record.RestoredPaths = publicRestoredPathSources(desc.RestoredPaths)
	return record
}

func publicRestoredPathSources(sources []model.RestoredPathSource) []publicRestoredPathSource {
	if len(sources) == 0 {
		return nil
	}
	records := make([]publicRestoredPathSource, 0, len(sources))
	for _, source := range sources {
		records = append(records, publicRestoredPathSource{
			TargetPath:      source.TargetPath,
			SourceSavePoint: string(source.SourceSnapshotID),
			SourcePath:      source.SourcePath,
			Status:          source.Status,
		})
	}
	return records
}

func publicSavePointHistory(workspace string, descs []*model.Descriptor, newest model.SnapshotID) publicSavePointHistoryRecord {
	return publicSavePointHistoryRecord{
		Workspace:       workspace,
		SavePoints:      publicSavePoints(descs),
		NewestSavePoint: string(newest),
	}
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
		Error:            publicContractVocabulary(result.Error),
		ErrorCode:        publicErrorCodeVocabulary(result.ErrorCode),
	}
}

func publicVerifyResults(results []*cliverify.Result) []publicVerifyResult {
	records := make([]publicVerifyResult, 0, len(results))
	for _, result := range results {
		records = append(records, publicVerify(result))
	}
	return records
}

func publicDoctor(result *clidoctor.Result) publicDoctorResult {
	findings := make([]publicDoctorFinding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, publicDoctorFinding{
			Category:    publicContractVocabulary(finding.Category),
			Description: publicContractVocabulary(finding.Description),
			Severity:    finding.Severity,
			ErrorCode:   publicErrorCodeVocabulary(finding.ErrorCode),
		})
	}
	return publicDoctorResult{
		Healthy:  result.Healthy,
		Findings: findings,
	}
}

func publicDoctorWithRepairs(result *clidoctor.Result, repairs []clidoctor.RepairResult) publicDoctorResult {
	record := publicDoctor(result)
	if repairs == nil {
		return record
	}
	record.Repairs = make([]publicDoctorRepair, 0, len(repairs))
	for _, repair := range repairs {
		record.Repairs = append(record.Repairs, publicDoctorRepair{
			Action:  repair.Action,
			Success: repair.Success,
			Message: publicContractVocabulary(repair.Message),
			Cleaned: repair.Cleaned,
		})
	}
	return record
}

func publicGC(plan *model.GCPlan) publicGCPlan {
	return publicGCPlan{
		PlanID:                 plan.PlanID,
		CreatedAt:              plan.CreatedAt,
		ProtectedCheckpoints:   publicCheckpointIDs(plan.ProtectedSet),
		ProtectedByLineage:     plan.ProtectedByLineage,
		CandidateCount:         plan.CandidateCount,
		ToDelete:               publicCheckpointIDs(plan.ToDelete),
		DeletableBytesEstimate: plan.DeletableBytesEstimate,
	}
}

func publicCheckpointIDs(ids []model.SnapshotID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func publicErrorCodeVocabulary(code string) string {
	code = strings.ReplaceAll(code, "WORKTREE", "WORKSPACE")
	code = strings.ReplaceAll(code, "SNAPSHOT", "CHECKPOINT")
	code = strings.ReplaceAll(code, "_HEAD_", "_CURRENT_")
	return code
}

func publicContractVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"head_snapshot_id", "current_checkpoint",
		"latest_snapshot_id", "latest_checkpoint",
		"base_snapshot_id", "base_checkpoint",
		"orphan intent files", "stale operation records",
		"intent files", "operation records",
		"intents directory", "operations directory",
		"intents", "operations",
		"intent", "operation",
		"head snapshot", "current checkpoint",
		"latest snapshot", "latest checkpoint",
		"base snapshot", "base checkpoint",
		"snapshot_id", "checkpoint_id",
		"snapshot", "checkpoint",
		"Snapshot", "Checkpoint",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"Worktrees", "Workspaces",
		"Worktree", "Workspace",
	)
	return replacer.Replace(value)
}
