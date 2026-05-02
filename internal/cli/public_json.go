package cli

import (
	"fmt"
	"strings"
	"time"

	clidoctor "github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/transfer"
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
		model.GCProtectionReasonActiveOperation:
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
	code = strings.ReplaceAll(code, "WORKTREE", "WORKSPACE")
	code = strings.ReplaceAll(code, "SNAPSHOT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "CHECKPOINT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "GC", "CLEANUP")
	code = strings.ReplaceAll(code, "_HEAD_", "_SOURCE_")
	return code
}

func publicContractVocabulary(value string) string {
	replacer := strings.NewReplacer(
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
	)
	return replacer.Replace(value)
}
