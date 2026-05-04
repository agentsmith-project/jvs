package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	clidoctor "github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type publicSavePointRecord struct {
	SavePointID string    `json:"save_point_id"`
	Workspace   string    `json:"workspace"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type publicSavePointCreatedRecord struct {
	transfer.Data
	SavePointID          string                     `json:"save_point_id"`
	Workspace            string                     `json:"workspace"`
	Message              string                     `json:"message"`
	CreatedAt            time.Time                  `json:"created_at"`
	NewestSavePoint      string                     `json:"newest_save_point"`
	StartedFromSavePoint string                     `json:"started_from_save_point,omitempty"`
	RestoredFrom         string                     `json:"restored_from,omitempty"`
	RestoredPaths        []publicRestoredPathSource `json:"restored_paths,omitempty"`
	UnsavedChanges       bool                       `json:"unsaved_changes"`
}

type publicDoctorResult struct {
	Healthy  bool                  `json:"healthy"`
	Findings []publicDoctorFinding `json:"findings"`
	Repairs  []publicDoctorRepair  `json:"repairs,omitempty"`
}

type publicDoctorFinding struct {
	Category               string `json:"category"`
	Description            string `json:"description"`
	Severity               string `json:"severity"`
	ErrorCode              string `json:"error_code,omitempty"`
	RecommendedNextCommand string `json:"recommended_next_command,omitempty"`
}

type publicDoctorRepair struct {
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Cleaned int    `json:"cleaned,omitempty"`
}

type publicCleanupPlan struct {
	PlanID                   string                         `json:"plan_id"`
	CreatedAt                time.Time                      `json:"created_at"`
	ProtectedSavePoints      []string                       `json:"protected_save_points"`
	ProtectionGroups         []publicCleanupProtectionGroup `json:"protection_groups"`
	ProtectedByHistory       int                            `json:"protected_by_history"`
	CandidateCount           int                            `json:"candidate_count"`
	ReclaimableSavePoints    []string                       `json:"reclaimable_save_points"`
	ReclaimableBytesEstimate int64                          `json:"reclaimable_bytes_estimate"`
}

type publicCleanupProtectionGroup struct {
	Reason     string   `json:"reason"`
	Count      int      `json:"count"`
	SavePoints []string `json:"save_points"`
}

type publicRestoredPathSource struct {
	TargetPath      string                 `json:"target_path"`
	SourceSavePoint string                 `json:"source_save_point"`
	SourcePath      string                 `json:"source_path"`
	Status          model.PathSourceStatus `json:"status"`
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

func publicSavePointCreated(desc *model.Descriptor, unsavedChanges bool, transferData transfer.Data) publicSavePointCreatedRecord {
	record := publicSavePointCreatedRecord{
		Data:            transferData,
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
	if desc.StartedFrom != nil {
		record.StartedFromSavePoint = string(*desc.StartedFrom)
	}
	record.RestoredPaths = publicRestoredPathSources(desc.RestoredPaths)
	return record
}

func publicJSONData(data any) (any, error) {
	if data == nil {
		return nil, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encode public JSON data: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode public JSON data: %w", err)
	}
	return publicJSONValue(decoded)
}

func publicJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return publicJSONMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			publicItem, err := publicJSONValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, publicItem)
		}
		return out, nil
	default:
		return value, nil
	}
}

func publicJSONMap(in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for key, value := range in {
		if key == "transfers" {
			transfers, err := publicJSONTransfers(value)
			if err != nil {
				return nil, err
			}
			out[key] = transfers
			continue
		}
		publicValue, err := publicJSONValue(value)
		if err != nil {
			return nil, err
		}
		out[key] = publicValue
	}
	return out, nil
}

func publicJSONTransfers(value any) ([]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("public JSON transfers must be an array")
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		recordMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("public JSON transfer must be an object")
		}
		publicRecord := transfer.PublicRecordFromRecord(transferRecordFromPublicJSONMap(recordMap))
		publicMap, err := publicTransferRecordMap(publicRecord)
		if err != nil {
			return nil, err
		}
		out = append(out, publicMap)
	}
	return out, nil
}

func publicTransferRecordMap(record transfer.PublicRecord) (map[string]any, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode public transfer record: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode public transfer record: %w", err)
	}
	return out, nil
}

func transferRecordFromPublicJSONMap(in map[string]any) transfer.Record {
	return transfer.Record{
		TransferID:                 publicJSONString(in, "transfer_id"),
		Operation:                  publicJSONString(in, "operation"),
		Phase:                      publicJSONString(in, "phase"),
		Primary:                    publicJSONBool(in, "primary"),
		ResultKind:                 transfer.ResultKind(publicJSONString(in, "result_kind")),
		PermissionScope:            transfer.PermissionScope(publicJSONString(in, "permission_scope")),
		SourceRole:                 publicJSONString(in, "source_role"),
		SourcePath:                 publicJSONString(in, "source_path"),
		DestinationRole:            publicJSONString(in, "destination_role"),
		MaterializationDestination: publicJSONString(in, "materialization_destination"),
		CapabilityProbePath:        publicJSONString(in, "capability_probe_path"),
		PublishedDestination:       publicJSONString(in, "published_destination"),
		CheckedForThisOperation:    publicJSONBool(in, "checked_for_this_operation"),
		RequestedEngine:            model.EngineType(publicJSONString(in, "requested_engine")),
		EffectiveEngine:            model.EngineType(publicJSONString(in, "effective_engine")),
		OptimizedTransfer:          publicJSONBool(in, "optimized_transfer"),
		PerformanceClass:           transfer.PerformanceClass(publicJSONString(in, "performance_class")),
		DegradedReasons:            publicJSONStringSlice(in, "degraded_reasons"),
		Warnings:                   publicJSONStringSlice(in, "warnings"),
	}
}

func publicJSONString(in map[string]any, key string) string {
	value, _ := in[key].(string)
	return value
}

func publicJSONBool(in map[string]any, key string) bool {
	value, _ := in[key].(bool)
	return value
}

func publicJSONStringSlice(in map[string]any, key string) []string {
	raw, ok := in[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
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

func publicDoctor(result *clidoctor.Result) publicDoctorResult {
	findings := make([]publicDoctorFinding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, publicDoctorFinding{
			Category:               publicContractVocabulary(finding.Category),
			Description:            publicContractVocabulary(finding.Description),
			Severity:               finding.Severity,
			ErrorCode:              publicErrorCodeVocabulary(finding.ErrorCode),
			RecommendedNextCommand: finding.RecommendedNextCommand,
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

func publicCleanup(plan *model.GCPlan) (publicCleanupPlan, error) {
	protectionGroups, err := publicCleanupProtectionGroups(plan.ProtectionGroups)
	if err != nil {
		return publicCleanupPlan{}, err
	}
	return publicCleanupPlan{
		PlanID:                   plan.PlanID,
		CreatedAt:                plan.CreatedAt,
		ProtectedSavePoints:      publicSnapshotIDs(plan.ProtectedSet),
		ProtectionGroups:         protectionGroups,
		ProtectedByHistory:       cleanupProtectionGroupCount(protectionGroups, model.GCProtectionReasonHistory, plan.ProtectedByLineage),
		CandidateCount:           plan.CandidateCount,
		ReclaimableSavePoints:    publicSnapshotIDs(plan.ToDelete),
		ReclaimableBytesEstimate: plan.DeletableBytesEstimate,
	}, nil
}

func publicCleanupProtectionGroups(groups []model.GCProtectionGroup) ([]publicCleanupProtectionGroup, error) {
	out := make([]publicCleanupProtectionGroup, 0, len(groups))
	for _, group := range groups {
		reason, err := publicCleanupProtectionReason(group.Reason)
		if err != nil {
			return nil, err
		}
		out = append(out, publicCleanupProtectionGroup{
			Reason:     reason,
			Count:      group.Count,
			SavePoints: publicSnapshotIDs(group.SavePoints),
		})
	}
	return out, nil
}

func publicCleanupProtectionReason(reason string) (string, error) {
	switch reason {
	case model.GCProtectionReasonHistory,
		model.GCProtectionReasonOpenView,
		model.GCProtectionReasonActiveRecovery,
		model.GCProtectionReasonActiveOperation,
		model.GCProtectionReasonImportedCloneHistory:
		return reason, nil
	default:
		return "", fmt.Errorf("cleanup plan contains unsupported cleanup protection reason")
	}
}

func cleanupProtectionGroupCount(groups []publicCleanupProtectionGroup, reason string, fallback int) int {
	for _, group := range groups {
		if group.Reason == reason {
			return group.Count
		}
	}
	return fallback
}

func publicSnapshotIDs(ids []model.SnapshotID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func publicErrorCodeVocabulary(code string) string {
	switch code {
	case "E_WORKTREE_PAYLOAD_INVALID":
		return "E_WORKSPACE_PATH_BINDING_INVALID"
	case "E_WORKTREE_PAYLOAD_MISSING":
		return errclass.ErrWorkspaceMissing.Code
	case "E_CONTROL_PAYLOAD_OVERLAP":
		return errclass.ErrControlWorkspaceOverlap.Code
	case "E_PAYLOAD_INSIDE_CONTROL":
		return errclass.ErrWorkspaceInsideControl.Code
	case "E_CONTROL_INSIDE_PAYLOAD":
		return errclass.ErrControlInsideWorkspace.Code
	case "E_PAYLOAD_LOCATOR_PRESENT":
		return errclass.ErrWorkspaceControlMarkerPresent.Code
	case "E_PAYLOAD_MISSING":
		return "E_SAVE_POINT_MISSING"
	case "E_PAYLOAD_INVALID":
		return "E_SAVE_POINT_INVALID"
	case "E_PAYLOAD_HASH_MISMATCH":
		return errclass.ErrSavePointHashMismatch.Code
	}
	code = strings.ReplaceAll(code, "WORKTREE", "WORKSPACE")
	code = strings.ReplaceAll(code, "SNAPSHOT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "CHECKPOINT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "GC", "CLEANUP")
	code = strings.ReplaceAll(code, "_HEAD_", "_SOURCE_")
	return code
}

func publicContractVocabulary(value string) string {
	value = strings.NewReplacer(
		"head_snapshot_id", "content_source",
		"latest_snapshot_id", "newest_save_point",
		"base_snapshot_id", "started_from_save_point",
		"orphan intent files", "stale operation records",
		"intent files", "operation records",
		"intents directory", "operations directory",
		"intents", "operations",
		"intent", "operation",
		"head snapshot", "content source save point",
		"latest snapshot", "newest save point",
		"base snapshot", "started from save point",
		"snapshot_id", "save_point_id",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"Checkpoints", "Save points",
		"Checkpoint", "Save point",
		"Snapshots", "Save points",
		"Snapshot", "Save point",
		"GC", "Cleanup",
		"gc", "cleanup",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"Worktrees", "Workspaces",
		"Worktree", "Workspace",
	).Replace(value)
	return strings.NewReplacer(
		"payload_root_hash", "save_point_hash",
		"payload path is bound", "folder path is bound",
		"payload path invalid", "folder path invalid",
		"payload directory missing", "folder missing",
		"compute payload hash", "compute save point hash",
		"payload hash mismatch", "save point hash mismatch",
		"payload missing", "save point storage missing",
		"READY payload_root_hash", "READY save_point_hash",
		"Payload path is bound", "Folder path is bound",
		"Payload path invalid", "Folder path invalid",
		"Payload directory missing", "Folder missing",
		"Compute payload hash", "Compute save point hash",
		"Payload hash mismatch", "Save point hash mismatch",
		"Payload missing", "Save point storage missing",
	).Replace(value)
}
