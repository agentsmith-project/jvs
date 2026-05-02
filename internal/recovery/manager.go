// Package recovery manages durable recovery plans for restore operations.
package recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const SchemaVersion = 1

var (
	ErrBackupMissing                                    = errors.New("recovery backup is missing")
	writePlanFile                                       = fsutil.AtomicWrite
	restoreBackupClone                                  = cloneRecoveryBackupToNew
	restoreBackupCapacityGate                           = capacitygate.Default()
	restoreBackupTransferPlanner transfer.EnginePlanner = engine.TransferPlanner{}
	writeWorktreeCfg                                    = repo.WriteWorktreeConfig
)

func SetWriteHookForTest(hook func(string, []byte, os.FileMode) error) func() {
	previous := writePlanFile
	writePlanFile = hook
	return func() {
		writePlanFile = previous
	}
}

func SetRestoreBackupCloneHookForTest(hook func(src, dst string) error) func() {
	previous := restoreBackupClone
	restoreBackupClone = func(_ engine.Engine, src, dst string) (*engine.CloneResult, error) {
		return nil, hook(src, dst)
	}
	return func() {
		restoreBackupClone = previous
	}
}

func SetRestoreBackupCapacityGateForTest(gate capacitygate.Gate) func() {
	previous := restoreBackupCapacityGate
	restoreBackupCapacityGate = gate
	return func() {
		restoreBackupCapacityGate = previous
	}
}

func SetRestoreBackupTransferPlannerForTest(planner transfer.EnginePlanner) func() {
	previous := restoreBackupTransferPlanner
	restoreBackupTransferPlanner = planner
	return func() {
		restoreBackupTransferPlanner = previous
	}
}

func SetWriteWorktreeConfigHookForTest(hook func(repoRoot, name string, cfg *model.WorktreeConfig) error) func() {
	previous := writeWorktreeCfg
	writeWorktreeCfg = hook
	return func() {
		writeWorktreeCfg = previous
	}
}

type Status string

const (
	StatusActive   Status = "active"
	StatusResolved Status = "resolved"
)

type Operation string

const (
	OperationRestore     Operation = "restore"
	OperationRestorePath Operation = "restore_path"
)

type BackupScope string

const (
	BackupScopeWhole BackupScope = "whole"
	BackupScopePath  BackupScope = "path"
)

type BackupState string

const (
	BackupStatePending    BackupState = "pending"
	BackupStateRequired   BackupState = "required"
	BackupStateRolledBack BackupState = "rolled_back"
)

type Phase string

const (
	PhasePending        Phase = "pending"
	PhaseBackupRequired Phase = "backup_required"
	PhaseRestoreApplied Phase = "restore_applied"
	PhaseBackupRestored Phase = "backup_restored"
)

type Plan struct {
	SchemaVersion           int                 `json:"schema_version"`
	RepoID                  string              `json:"repo_id"`
	PlanID                  string              `json:"plan_id"`
	Status                  Status              `json:"status"`
	Operation               Operation           `json:"operation"`
	RestorePlanID           string              `json:"restore_plan_id"`
	Workspace               string              `json:"workspace"`
	Folder                  string              `json:"folder"`
	SourceSavePoint         model.SnapshotID    `json:"source_save_point"`
	Path                    string              `json:"path,omitempty"`
	CreatedAt               time.Time           `json:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
	ResolvedAt              *time.Time          `json:"resolved_at,omitempty"`
	PreWorktreeState        WorktreeState       `json:"pre_worktree_state"`
	Backup                  Backup              `json:"backup"`
	Phase                   Phase               `json:"phase,omitempty"`
	PreRecoveryEvidence     string              `json:"pre_recovery_evidence,omitempty"`
	RecoveryEvidence        string              `json:"recovery_evidence,omitempty"`
	LastError               string              `json:"last_error,omitempty"`
	CompletedSteps          []string            `json:"completed_steps,omitempty"`
	PendingSteps            []string            `json:"pending_steps,omitempty"`
	RecommendedNextCommand  string              `json:"recommended_next_command"`
	CleanupProtectionPinIDs []string            `json:"cleanup_protection_pin_ids,omitempty"`
	CleanupProtectionPins   []model.Pin         `json:"cleanup_protection_pins,omitempty"`
	RestoreOptions          restoreplan.Options `json:"restore_options,omitempty"`
	Transfers               []transfer.Record   `json:"transfers,omitempty"`
}

type WorktreeState struct {
	Name             string            `json:"name"`
	RealPath         string            `json:"real_path,omitempty"`
	BaseSnapshotID   model.SnapshotID  `json:"base_snapshot_id,omitempty"`
	HeadSnapshotID   model.SnapshotID  `json:"head_snapshot_id,omitempty"`
	LatestSnapshotID model.SnapshotID  `json:"latest_snapshot_id,omitempty"`
	PathSources      model.PathSources `json:"path_sources,omitempty"`
	CreatedAt        time.Time         `json:"created_at,omitempty"`
}

type Backup struct {
	Path              string        `json:"path"`
	Scope             BackupScope   `json:"scope"`
	State             BackupState   `json:"state,omitempty"`
	TouchedPaths      []string      `json:"touched_paths,omitempty"`
	Entries           []BackupEntry `json:"entries,omitempty"`
	PayloadRolledBack bool          `json:"payload_rolled_back,omitempty"`
}

type BackupEntry struct {
	Path        string `json:"path"`
	HadOriginal bool   `json:"had_original"`
}

type VisiblePlanWriteUncertainError struct {
	PlanID string
	Err    error
}

type RecognizedState string

const (
	RecognizedPlanEvidence          RecognizedState = "plan_evidence"
	RecognizedPreMutation           RecognizedState = "pre_mutation"
	RecognizedRestoreTarget         RecognizedState = "restore_target"
	RecognizedBackupPayloadRestored RecognizedState = "backup_payload_restored"
)

type CurrentState struct {
	State    RecognizedState
	Evidence string
}

type RestoreBackupOptions struct {
	TransferOperation string
}

func (e *VisiblePlanWriteUncertainError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Recovery plan: %s\nRecovery plan was created, but its save was not fully confirmed. No files were changed.\nRun: jvs recovery status %s\nOr run: jvs recovery resume %s\nOr run: jvs recovery rollback %s", e.PlanID, e.PlanID, e.PlanID, e.PlanID)
}

func (e *VisiblePlanWriteUncertainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Manager struct {
	repoRoot      string
	lastTransfers []transfer.Record
}

func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

func (m *Manager) LastTransferRecords() []transfer.Record {
	if m == nil || len(m.lastTransfers) == 0 {
		return nil
	}
	return append([]transfer.Record(nil), m.lastTransfers...)
}

func NewPlanID() string {
	return "RP-" + uuidutil.NewV4()
}

func (m *Manager) CreateActiveForRestore(preview *restoreplan.Plan, backupPath string) (*Plan, error) {
	if preview == nil {
		return nil, fmt.Errorf("restore plan is required")
	}
	if strings.TrimSpace(backupPath) == "" {
		return nil, fmt.Errorf("backup path is required")
	}
	repoID, err := currentRepoID(m.repoRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, preview.Workspace)
	if err != nil {
		return nil, fmt.Errorf("load workspace state: %w", err)
	}
	evidence, err := recoveryEvidenceForPreview(m.repoRoot, preview)
	if err != nil {
		return nil, err
	}

	planID := NewPlanID()
	op := OperationRestore
	scope := BackupScopeWhole
	var touched []string
	var entries []BackupEntry
	if preview.EffectiveScope() == restoreplan.ScopePath {
		op = OperationRestorePath
		scope = BackupScopePath
		touched = []string{preview.Path}
		entries = []BackupEntry{{Path: preview.Path, HadOriginal: false}}
	}

	now := time.Now().UTC()
	plan := &Plan{
		SchemaVersion:    SchemaVersion,
		RepoID:           repoID,
		PlanID:           planID,
		Status:           StatusActive,
		Operation:        op,
		RestorePlanID:    preview.PlanID,
		Workspace:        preview.Workspace,
		Folder:           preview.Folder,
		SourceSavePoint:  preview.SourceSavePoint,
		Path:             preview.Path,
		CreatedAt:        now,
		UpdatedAt:        now,
		PreWorktreeState: worktreeStateFromConfig(cfg),
		Backup: Backup{
			Path:         backupPath,
			Scope:        scope,
			State:        BackupStatePending,
			TouchedPaths: touched,
			Entries:      entries,
		},
		Phase:                  PhasePending,
		PreRecoveryEvidence:    evidence,
		RecoveryEvidence:       evidence,
		CompletedSteps:         []string{"recovery plan created"},
		PendingSteps:           []string{"restore files", "update workspace metadata", "cleanup recovery backup"},
		RecommendedNextCommand: "jvs recovery resume " + planID,
		RestoreOptions:         preview.Options,
	}
	if err := m.Write(plan); err != nil {
		if fsutil.IsCommitUncertain(err) {
			if loaded, loadErr := m.Load(planID); loadErr == nil {
				if loaded.Status == StatusActive {
					return nil, &VisiblePlanWriteUncertainError{PlanID: loaded.PlanID, Err: err}
				}
			}
		}
		return nil, err
	}
	return plan, nil
}

func (m *Manager) Write(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("recovery plan is required")
	}
	if err := validatePlan(plan); err != nil {
		return err
	}
	repoID, err := currentRepoID(m.repoRoot)
	if err != nil {
		return err
	}
	if plan.RepoID != repoID {
		return fmt.Errorf("recovery plan %q belongs to a different repository", plan.PlanID)
	}
	if err := os.MkdirAll(filepath.Join(m.repoRoot, repo.JVSDirName, "recovery-plans"), 0755); err != nil {
		return fmt.Errorf("create recovery plan directory: %w", err)
	}
	path, err := repo.RecoveryPlanPathForWrite(m.repoRoot, plan.PlanID)
	if err != nil {
		return fmt.Errorf("recovery plan path: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recovery plan: %w", err)
	}
	return writePlanFile(path, data, 0644)
}

func (m *Manager) Load(planID string) (*Plan, error) {
	if err := validatePlanID(planID); err != nil {
		return nil, err
	}
	path, err := repo.RecoveryPlanPathForRead(m.repoRoot, planID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("recovery plan %q not found", planID)
		}
		return nil, fmt.Errorf("recovery plan %q is not readable: %w", planID, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("recovery plan %q not found", planID)
		}
		return nil, fmt.Errorf("recovery plan %q is not readable: %w", planID, err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("recovery plan %q is not valid JSON", planID)
	}
	if err := validatePlan(&plan); err != nil {
		return nil, fmt.Errorf("recovery plan %q: %w", planID, err)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("recovery plan %q plan_id does not match request", planID)
	}
	repoID, err := currentRepoID(m.repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("recovery plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func (m *Manager) List() ([]Plan, error) {
	dir, err := repo.RecoveryPlansDirPath(m.repoRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("recovery plans: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read recovery plans: %w", err)
	}
	plans := make([]Plan, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("recovery plan %q is a symlink", name)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("recovery plan %q is not a regular file", name)
		}
		if !strings.HasSuffix(name, ".json") {
			return nil, fmt.Errorf("recovery plan %q must be a JSON file", name)
		}
		if err := pathutil.ValidateName(name); err != nil {
			return nil, fmt.Errorf("recovery plan file name is unsafe: %w", err)
		}
		planID := strings.TrimSuffix(name, ".json")
		plan, err := m.Load(planID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].PlanID < plans[j].PlanID })
	return plans, nil
}

func (m *Manager) ActiveForWorkspace(workspace string) ([]Plan, error) {
	plans, err := m.List()
	if err != nil {
		return nil, err
	}
	var active []Plan
	for _, plan := range plans {
		if plan.Status == StatusActive && plan.Workspace == workspace {
			active = append(active, plan)
		}
	}
	return active, nil
}

func (m *Manager) MarkResolved(planID string) error {
	plan, err := m.Load(planID)
	if err != nil {
		return err
	}
	if plan.Status == StatusResolved {
		return m.releaseCleanupProtections(plan)
	}
	now := time.Now().UTC()
	plan.Status = StatusResolved
	plan.ResolvedAt = &now
	plan.UpdatedAt = now
	plan.PendingSteps = nil
	plan.RecommendedNextCommand = ""
	plan.CompletedSteps = appendStep(plan.CompletedSteps, "recovery resolved")
	if err := m.Write(plan); err != nil {
		return err
	}
	return m.releaseCleanupProtections(plan)
}

func (m *Manager) releaseCleanupProtections(plan *Plan) error {
	if plan == nil {
		return nil
	}
	for _, pin := range plan.CleanupProtectionPins {
		if err := sourcepin.NewManager(m.repoRoot).RemoveIfMatches(pin); err != nil {
			return fmt.Errorf("release recovery save point protection: %w", err)
		}
	}
	return nil
}

func (m *Manager) RestoreBackup(plan *Plan) error {
	return m.RestoreBackupWithOptions(plan, RestoreBackupOptions{})
}

func (m *Manager) RestoreBackupWithOptions(plan *Plan, options RestoreBackupOptions) error {
	if m != nil {
		m.lastTransfers = nil
	}
	boundary, err := m.validateLiveBackup(plan)
	if err != nil {
		return err
	}
	transferOperation := recoveryBackupTransferOperation(options.TransferOperation)
	if backupPayloadRestoreRequired(plan.Backup) {
		switch plan.Backup.Scope {
		case BackupScopeWhole:
			if err := m.restoreWholeBackup(plan, boundary, plan.Backup.Path, transferOperation); err != nil {
				return err
			}
		case BackupScopePath:
			if err := m.restorePathBackup(plan, boundary.Root, plan.Backup.Path, pathBackupEntries(plan.Backup), transferOperation); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported backup scope")
		}
	}
	if err := writeWorktreeCfg(m.repoRoot, plan.Workspace, plan.PreWorktreeState.WorktreeConfig()); err != nil {
		return fmt.Errorf("restore workspace metadata: %w", err)
	}
	return nil
}

// ValidateLiveBackup checks that a recovery plan's backup exists and is safe to use.
func (m *Manager) ValidateLiveBackup(plan *Plan) error {
	_, err := m.validateLiveBackup(plan)
	return err
}

func (m *Manager) validateLiveBackup(plan *Plan) (repo.WorktreePayloadBoundary, error) {
	if plan == nil {
		return repo.WorktreePayloadBoundary{}, fmt.Errorf("recovery plan is required")
	}
	if err := validatePlan(plan); err != nil {
		return repo.WorktreePayloadBoundary{}, err
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(m.repoRoot, plan.Workspace)
	if err != nil {
		return repo.WorktreePayloadBoundary{}, fmt.Errorf("workspace folder: %w", err)
	}
	if err := validateBackupSemantics(boundary, plan); err != nil {
		return repo.WorktreePayloadBoundary{}, err
	}
	if err := validateBackupPath(boundary.Root, plan.Folder, plan.Backup.Path); err != nil {
		return repo.WorktreePayloadBoundary{}, err
	}
	if plan.Backup.Scope == BackupScopePath && backupPayloadRestoreRequired(plan.Backup) {
		if _, err := validatePathBackupRequiredEntries(boundary.Root, plan.Backup.Path, pathBackupEntries(plan.Backup)); err != nil {
			return repo.WorktreePayloadBoundary{}, err
		}
	}
	return boundary, nil
}

func CurrentEvidence(repoRoot string, plan *Plan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("recovery plan is required")
	}
	switch plan.Operation {
	case OperationRestore:
		return restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
	case OperationRestorePath:
		return restoreplan.PathEvidence(repoRoot, plan.Workspace, plan.Path)
	default:
		return "", fmt.Errorf("recovery operation is not supported")
	}
}

func RecognizeCurrentState(repoRoot string, plan *Plan) (CurrentState, error) {
	evidence, err := CurrentEvidence(repoRoot, plan)
	if err != nil {
		return CurrentState{}, err
	}
	if ok, err := currentMatchesPreMutation(repoRoot, plan, evidence); err != nil {
		return CurrentState{}, err
	} else if ok {
		return CurrentState{State: RecognizedPreMutation, Evidence: evidence}, nil
	}
	if strings.TrimSpace(plan.RecoveryEvidence) != "" && evidence == plan.RecoveryEvidence {
		return CurrentState{State: RecognizedPlanEvidence, Evidence: evidence}, nil
	}
	if ok, err := currentMatchesRestoreTarget(repoRoot, plan, evidence); err != nil {
		return CurrentState{}, err
	} else if ok {
		return CurrentState{State: RecognizedRestoreTarget, Evidence: evidence}, nil
	}
	if ok, err := currentMatchesBackupPayloadRestored(repoRoot, plan, evidence); err != nil {
		return CurrentState{}, err
	} else if ok {
		return CurrentState{State: RecognizedBackupPayloadRestored, Evidence: evidence}, nil
	}
	return CurrentState{}, fmt.Errorf("folder changed since the recovery plan was created; no files were changed")
}

func (m *Manager) RemoveBackup(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("recovery plan is required")
	}
	return m.RemoveBackupPath(plan, plan.Backup.Path)
}

func (m *Manager) RemoveBackupPath(plan *Plan, backupPath string) error {
	if plan == nil {
		return fmt.Errorf("recovery plan is required")
	}
	if err := validatePlan(plan); err != nil {
		return err
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(m.repoRoot, plan.Workspace)
	if err != nil {
		return fmt.Errorf("workspace folder: %w", err)
	}
	if err := validateBackupPath(boundary.Root, plan.Folder, backupPath); err != nil {
		return err
	}
	if err := os.RemoveAll(backupPath); err != nil {
		return fmt.Errorf("remove recovery backup: %w", err)
	}
	return nil
}

func worktreeStateFromConfig(cfg *model.WorktreeConfig) WorktreeState {
	if cfg == nil {
		return WorktreeState{}
	}
	return WorktreeState{
		Name:             cfg.Name,
		RealPath:         cfg.RealPath,
		BaseSnapshotID:   cfg.BaseSnapshotID,
		HeadSnapshotID:   cfg.HeadSnapshotID,
		LatestSnapshotID: cfg.LatestSnapshotID,
		PathSources:      cfg.PathSources.Clone(),
		CreatedAt:        cfg.CreatedAt,
	}
}

func recoveryEvidenceForPreview(repoRoot string, preview *restoreplan.Plan) (string, error) {
	if preview.EffectiveScope() == restoreplan.ScopePath {
		if strings.TrimSpace(preview.ExpectedPathEvidence) != "" {
			return preview.ExpectedPathEvidence, nil
		}
		return restoreplan.PathEvidence(repoRoot, preview.Workspace, preview.Path)
	}
	if strings.TrimSpace(preview.ExpectedFolderEvidence) != "" {
		return preview.ExpectedFolderEvidence, nil
	}
	return restoreplan.WorkspaceEvidence(repoRoot, preview.Workspace)
}

func validateBackupPath(boundaryRoot, planFolder, backupPath string) error {
	if strings.TrimSpace(backupPath) == "" {
		return fmt.Errorf("backup folder is required")
	}
	cleanBoundaryRoot := filepath.Clean(boundaryRoot)
	cleanPlanFolder := filepath.Clean(planFolder)
	if cleanPlanFolder != cleanBoundaryRoot {
		return fmt.Errorf("recovery plan folder does not match the workspace folder")
	}
	cleanBackup := filepath.Clean(backupPath)
	expectedPrefix := cleanBoundaryRoot + ".restore-backup-"
	if !strings.HasPrefix(cleanBackup, expectedPrefix) || len(cleanBackup) == len(expectedPrefix) {
		return fmt.Errorf("backup folder is not a recovery backup")
	}
	if filepath.Dir(cleanBackup) != filepath.Dir(cleanBoundaryRoot) {
		return fmt.Errorf("backup folder is not next to the workspace folder")
	}
	info, err := os.Lstat(cleanBackup)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrBackupMissing
		}
		return fmt.Errorf("check recovery backup: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("recovery backup is not a safe folder")
	}
	return nil
}

func (m *Manager) restoreWholeBackup(plan *Plan, boundary repo.WorktreePayloadBoundary, backupPath, transferOperation string) error {
	tempPath := boundary.Root + ".recovery-restore-tmp-" + uuidutil.NewV4()[:8]
	defer os.RemoveAll(tempPath)
	bytes, err := capacitygate.TreeSize(backupPath, nil)
	if err != nil {
		return fmt.Errorf("measure recovery backup: %w", err)
	}
	if err := checkRecoveryBackupRestoreCapacity(plan, []capacitygate.Component{{
		Name:  "recovery backup restore staging",
		Path:  tempPath,
		Bytes: bytes,
	}}); err != nil {
		return err
	}
	intent := recoveryBackupRestoreIntent(recoveryBackupRestoreIntentRequest{
		TransferID:                 "recovery-backup-restore-primary",
		Operation:                  transferOperation,
		SourcePath:                 backupPath,
		MaterializationDestination: tempPath,
		CapabilityProbePath:        filepath.Dir(tempPath),
		PublishedDestination:       boundary.Root,
		Primary:                    true,
	})
	if err := m.materializeRecoveryBackupCopy(intent); err != nil {
		return fmt.Errorf("copy recovery backup: %w", err)
	}
	if err := clearManagedDirectory(boundary); err != nil {
		return err
	}
	return moveManagedContents(tempPath, boundary.Root, boundary.ExcludesRelativePath)
}

func (m *Manager) restorePathBackup(plan *Plan, payloadRoot, backupPath string, entries []BackupEntry, transferOperation string) error {
	entries, err := validatePathBackupRequiredEntries(payloadRoot, backupPath, entries)
	if err != nil {
		return err
	}
	tempPath := payloadRoot + ".recovery-path-tmp-" + uuidutil.NewV4()[:8]
	defer os.RemoveAll(tempPath)
	copyEntries := pathBackupCopyEntries(entries)
	components := make([]capacitygate.Component, 0, len(copyEntries))
	for _, entry := range copyEntries {
		source := filepath.Join(backupPath, entry.Path)
		bytes, err := capacitygate.TreeSize(source, nil)
		if err != nil {
			return fmt.Errorf("measure recovery backup path %s: %w", entry.Path, err)
		}
		components = append(components, capacitygate.Component{
			Name:  "recovery backup path restore staging",
			Path:  filepath.Join(tempPath, entry.Path),
			Bytes: bytes,
		})
	}
	if err := checkRecoveryBackupRestoreCapacity(plan, components); err != nil {
		return err
	}
	copyIndex := 0
	for _, entry := range entries {
		clean := entry.Path
		if entry.HadOriginal {
			copyIndex++
			transferID := "recovery-path-backup-restore-primary"
			primary := true
			if copyIndex > 1 {
				transferID = fmt.Sprintf("recovery-path-backup-restore-%d", copyIndex)
				primary = false
			}
			intent := recoveryBackupRestoreIntent(recoveryBackupRestoreIntentRequest{
				TransferID:                 transferID,
				Operation:                  transferOperation,
				SourcePath:                 filepath.Join(backupPath, clean),
				MaterializationDestination: filepath.Join(tempPath, clean),
				CapabilityProbePath:        filepath.Dir(tempPath),
				PublishedDestination:       filepath.Join(payloadRoot, clean),
				Primary:                    primary,
			})
			if err := m.materializeRecoveryBackupCopy(intent); err != nil {
				return fmt.Errorf("copy recovery backup path %s: %w", clean, err)
			}
		}
	}
	for _, entry := range entries {
		clean := entry.Path
		dst := filepath.Join(payloadRoot, clean)
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("remove restored path %s: %w", clean, err)
		}
		if !entry.HadOriginal {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create parent for %s: %w", clean, err)
		}
		if err := os.Rename(filepath.Join(tempPath, clean), dst); err != nil {
			return fmt.Errorf("restore path %s: %w", clean, err)
		}
	}
	return nil
}

func (m *Manager) materializeRecoveryBackupCopy(intent transfer.Intent) error {
	plan, err := transfer.PlanIntent(restoreBackupTransferPlanner, intent)
	if err != nil {
		return fmt.Errorf("plan transfer: %w", err)
	}
	transferEngine := model.EngineCopy
	if plan != nil && plan.TransferEngine != "" {
		transferEngine = plan.TransferEngine
	}
	runtime, err := restoreBackupClone(engine.NewEngine(transferEngine), intent.SourcePath, intent.MaterializationDestination)
	if err != nil {
		return err
	}
	record := transfer.RecordFromPlanAndRuntime(intent, plan, runtime)
	m.lastTransfers = append(m.lastTransfers, record)
	return nil
}

func checkRecoveryBackupRestoreCapacity(plan *Plan, components []capacitygate.Component) error {
	if len(components) == 0 {
		return nil
	}
	folder := ""
	workspace := ""
	path := ""
	if plan != nil {
		folder = plan.Folder
		workspace = plan.Workspace
		path = plan.Path
	}
	_, err := restoreBackupCapacityGate.Check(capacitygate.Request{
		Operation:       "recovery backup restore",
		Folder:          folder,
		Workspace:       workspace,
		Path:            path,
		Components:      components,
		FailureMessages: []string{"No files were changed.", "Recovery backup was not changed."},
	})
	return err
}

type recoveryBackupRestoreIntentRequest struct {
	TransferID                 string
	Operation                  string
	SourcePath                 string
	MaterializationDestination string
	CapabilityProbePath        string
	PublishedDestination       string
	Primary                    bool
}

func recoveryBackupRestoreIntent(req recoveryBackupRestoreIntentRequest) transfer.Intent {
	return transfer.Intent{
		TransferID:                 req.TransferID,
		Operation:                  recoveryBackupTransferOperation(req.Operation),
		Phase:                      "backup_restore",
		Primary:                    req.Primary,
		ResultKind:                 transfer.ResultKindFinal,
		PermissionScope:            transfer.PermissionScopeExecution,
		SourceRole:                 "recovery_backup_payload",
		SourcePath:                 req.SourcePath,
		DestinationRole:            "recovery_restore_staging",
		MaterializationDestination: req.MaterializationDestination,
		CapabilityProbePath:        req.CapabilityProbePath,
		PublishedDestination:       req.PublishedDestination,
		RequestedEngine:            engine.EngineAuto,
	}
}

func recoveryBackupTransferOperation(operation string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return "recovery_backup_restore"
	}
	return operation
}

func pathBackupCopyEntries(entries []BackupEntry) []BackupEntry {
	out := make([]BackupEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.HadOriginal {
			out = append(out, entry)
		}
	}
	return out
}

func validatePathBackupRequiredEntries(payloadRoot, backupPath string, entries []BackupEntry) ([]BackupEntry, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("recovery path is required")
	}
	entries = append([]BackupEntry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	for i := range entries {
		clean, err := pathutil.CleanRel(entries[i].Path)
		if err != nil {
			return nil, fmt.Errorf("recovery path is not safe: %w", err)
		}
		entries[i].Path = clean
		if err := pathutil.ValidateNoSymlinkParents(payloadRoot, clean); err != nil {
			return nil, fmt.Errorf("recovery path parent is not safe: %w", err)
		}
		if entries[i].HadOriginal {
			if err := validatePathBackupRequiredEntry(backupPath, clean); err != nil {
				return nil, err
			}
		}
	}
	return entries, nil
}

func validatePathBackupRequiredEntry(backupPath, clean string) error {
	if _, err := os.Lstat(filepath.Join(backupPath, clean)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("recovery backup path %s is missing", clean)
		}
		return fmt.Errorf("check recovery backup path %s: %w", clean, err)
	}
	return nil
}

func clearManagedDirectory(boundary repo.WorktreePayloadBoundary) error {
	entries, err := os.ReadDir(boundary.Root)
	if err != nil {
		return fmt.Errorf("read workspace folder: %w", err)
	}
	for _, entry := range entries {
		rel := entry.Name()
		if boundary.ExcludesRelativePath(rel) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(boundary.Root, rel)); err != nil {
			return fmt.Errorf("remove workspace path %s: %w", rel, err)
		}
	}
	return nil
}

func moveManagedContents(srcRoot, dstRoot string, excluded func(rel string) bool) error {
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return fmt.Errorf("read recovery backup: %w", err)
	}
	if err := os.MkdirAll(dstRoot, 0755); err != nil {
		return fmt.Errorf("create workspace folder: %w", err)
	}
	for _, entry := range entries {
		rel := entry.Name()
		if excluded != nil && excluded(rel) {
			continue
		}
		if err := os.Rename(filepath.Join(srcRoot, rel), filepath.Join(dstRoot, rel)); err != nil {
			return fmt.Errorf("restore workspace path %s: %w", rel, err)
		}
	}
	return nil
}

func cloneRecoveryBackupToNew(eng engine.Engine, src, dst string) (*engine.CloneResult, error) {
	if eng == nil {
		eng = engine.NewCopyEngine()
	}
	return engine.CloneToNew(eng, src, dst)
}

func (s WorktreeState) WorktreeConfig() *model.WorktreeConfig {
	return &model.WorktreeConfig{
		Name:             s.Name,
		RealPath:         s.RealPath,
		BaseSnapshotID:   s.BaseSnapshotID,
		HeadSnapshotID:   s.HeadSnapshotID,
		LatestSnapshotID: s.LatestSnapshotID,
		PathSources:      s.PathSources.Clone(),
		CreatedAt:        s.CreatedAt,
	}
}

func validatePlan(plan *Plan) error {
	if plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version")
	}
	if err := validatePlanID(plan.PlanID); err != nil {
		return err
	}
	switch plan.Status {
	case StatusActive, StatusResolved:
	default:
		return fmt.Errorf("unsupported status")
	}
	switch plan.Operation {
	case OperationRestore, OperationRestorePath:
	default:
		return fmt.Errorf("unsupported operation")
	}
	if strings.TrimSpace(plan.RepoID) == "" {
		return fmt.Errorf("repo id is required")
	}
	if strings.TrimSpace(plan.Workspace) == "" {
		return fmt.Errorf("workspace is required")
	}
	if strings.TrimSpace(plan.Folder) == "" {
		return fmt.Errorf("folder is required")
	}
	if err := plan.SourceSavePoint.Validate(); err != nil {
		return fmt.Errorf("source save point: %w", err)
	}
	if plan.Operation == OperationRestorePath && strings.TrimSpace(plan.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if plan.Phase != "" {
		switch plan.Phase {
		case PhasePending, PhaseBackupRequired, PhaseRestoreApplied, PhaseBackupRestored:
		default:
			return fmt.Errorf("unsupported recovery phase")
		}
	}
	if strings.TrimSpace(plan.Backup.Path) == "" {
		return fmt.Errorf("backup path is required")
	}
	switch plan.Backup.Scope {
	case BackupScopeWhole, BackupScopePath:
	default:
		return fmt.Errorf("unsupported backup scope")
	}
	switch backupState(plan.Backup) {
	case BackupStatePending, BackupStateRequired, BackupStateRolledBack:
	default:
		return fmt.Errorf("unsupported backup state")
	}
	for _, entry := range plan.Backup.Entries {
		if _, err := pathutil.CleanRel(entry.Path); err != nil {
			return fmt.Errorf("backup entry path is unsafe: %w", err)
		}
	}
	for _, pinID := range plan.CleanupProtectionPinIDs {
		if err := pathutil.ValidateName(pinID); err != nil {
			return fmt.Errorf("cleanup protection id is unsafe: %w", err)
		}
	}
	return nil
}

func validateBackupSemantics(boundary repo.WorktreePayloadBoundary, plan *Plan) error {
	switch plan.Operation {
	case OperationRestore:
		if plan.Backup.Scope != BackupScopeWhole {
			return fmt.Errorf("restore recovery plan must use whole-folder backup")
		}
	case OperationRestorePath:
		if plan.Backup.Scope != BackupScopePath {
			return fmt.Errorf("path restore recovery plan must use path backup")
		}
		return validatePathBackupSemantics(boundary, plan)
	default:
		return fmt.Errorf("unsupported operation")
	}
	return nil
}

func validatePathBackupSemantics(boundary repo.WorktreePayloadBoundary, plan *Plan) error {
	planPath, err := pathutil.CleanRel(plan.Path)
	if err != nil {
		return fmt.Errorf("recovery path is not safe: %w", err)
	}
	if boundary.ExcludesRelativePath(planPath) {
		return fmt.Errorf("recovery path is repo control data and is not managed")
	}
	if err := pathutil.ValidateNoSymlinkParents(boundary.Root, planPath); err != nil {
		return fmt.Errorf("recovery path parent is not safe: %w", err)
	}
	checkPath := func(raw string) error {
		clean, err := pathutil.CleanRel(raw)
		if err != nil {
			return fmt.Errorf("recovery path is not safe: %w", err)
		}
		if boundary.ExcludesRelativePath(clean) {
			return fmt.Errorf("recovery path is repo control data and is not managed")
		}
		if clean != planPath {
			return fmt.Errorf("recovery path backup does not match plan path")
		}
		return nil
	}
	if len(plan.Backup.TouchedPaths) > 0 {
		for _, touched := range plan.Backup.TouchedPaths {
			if err := checkPath(touched); err != nil {
				return err
			}
		}
	}
	entries := pathBackupEntries(plan.Backup)
	if len(entries) == 0 {
		return fmt.Errorf("recovery path backup entry is required")
	}
	for _, entry := range entries {
		if err := checkPath(entry.Path); err != nil {
			return err
		}
	}
	return nil
}

func BackupMissingIsSafe(plan *Plan) bool {
	if plan == nil {
		return false
	}
	switch backupState(plan.Backup) {
	case BackupStatePending, BackupStateRolledBack:
		return true
	default:
		return false
	}
}

func VerifyMissingBackupRecoveryPoint(repoRoot string, plan *Plan) error {
	if !BackupMissingIsSafe(plan) {
		return ErrBackupMissing
	}
	state, err := RecognizeCurrentState(repoRoot, plan)
	if err != nil {
		return err
	}
	if state.State != RecognizedPreMutation {
		return fmt.Errorf("current workspace state does not match the recovery plan; no files were changed")
	}
	return nil
}

func currentMatchesPreMutation(repoRoot string, plan *Plan, evidence string) (bool, error) {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, plan.Workspace)
	if err != nil {
		return false, fmt.Errorf("confirm current workspace state: %w", err)
	}
	if !worktreeStateEqual(worktreeStateFromConfig(cfg), plan.PreWorktreeState) {
		return false, nil
	}
	preEvidence := plan.PreRecoveryEvidence
	if strings.TrimSpace(preEvidence) == "" {
		if backupState(plan.Backup) == BackupStateRequired {
			return false, nil
		}
		preEvidence = plan.RecoveryEvidence
	}
	if strings.TrimSpace(preEvidence) == "" || evidence != preEvidence {
		return false, nil
	}
	return true, nil
}

func currentMatchesRestoreTarget(repoRoot string, plan *Plan, evidence string) (bool, error) {
	switch plan.Operation {
	case OperationRestore:
		return currentMatchesWholeRestoreTarget(repoRoot, plan, evidence)
	case OperationRestorePath:
		return currentMatchesPathRestoreTarget(repoRoot, plan, evidence)
	default:
		return false, nil
	}
}

func currentMatchesWholeRestoreTarget(repoRoot string, plan *Plan, evidence string) (bool, error) {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, plan.Workspace)
	if err != nil {
		return false, fmt.Errorf("confirm current workspace state: %w", err)
	}
	if cfg.HeadSnapshotID != plan.SourceSavePoint {
		return false, nil
	}
	desc, err := snapshot.LoadDescriptor(repoRoot, plan.SourceSavePoint)
	if err != nil {
		return false, fmt.Errorf("load source save point: %w", err)
	}
	return evidence == string(desc.PayloadRootHash), nil
}

func currentMatchesPathRestoreTarget(repoRoot string, plan *Plan, evidence string) (bool, error) {
	planPath, err := pathutil.CleanRel(plan.Path)
	if err != nil {
		return false, fmt.Errorf("confirm path restore state: %w", err)
	}
	cfg, err := repo.LoadWorktreeConfig(repoRoot, plan.Workspace)
	if err != nil {
		return false, fmt.Errorf("confirm current workspace state: %w", err)
	}
	entry, ok, err := cfg.PathSources.SourceForPath(planPath)
	if err != nil {
		return false, fmt.Errorf("confirm restored path source: %w", err)
	}
	if !ok ||
		entry.TargetPath != planPath ||
		entry.SourceSnapshotID != plan.SourceSavePoint ||
		entry.SourcePath != planPath ||
		entry.Status != model.PathSourceExact {
		return false, nil
	}
	sourceEvidence, err := sourcePathEvidence(repoRoot, plan.Workspace, plan.SourceSavePoint, planPath)
	if err != nil {
		return false, err
	}
	return evidence == sourceEvidence, nil
}

func currentMatchesBackupPayloadRestored(repoRoot string, plan *Plan, evidence string) (bool, error) {
	if backupState(plan.Backup) != BackupStateRequired {
		return false, nil
	}
	preEvidence := strings.TrimSpace(plan.PreRecoveryEvidence)
	if preEvidence == "" {
		return false, nil
	}
	return evidence == preEvidence, nil
}

func sourcePathEvidence(repoRoot, workspaceName string, sourceID model.SnapshotID, path string) (string, error) {
	desc, err := snapshot.LoadDescriptor(repoRoot, sourceID)
	if err != nil {
		return "", fmt.Errorf("load source save point: %w", err)
	}
	if desc.SnapshotID != sourceID {
		return "", fmt.Errorf("source save point descriptor does not match request")
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace folder: %w", err)
	}
	if boundary.ExcludesRelativePath(path) {
		return "", fmt.Errorf("path is repo control data and is not managed")
	}
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, sourceID)
	if err != nil {
		return "", fmt.Errorf("source save point path: %w", err)
	}
	tmpRoot, err := os.MkdirTemp(filepath.Join(repoRoot, repo.JVSDirName), "recovery-source-")
	if err != nil {
		return "", fmt.Errorf("create source evidence workspace: %w", err)
	}
	defer os.RemoveAll(tmpRoot)
	sourceRoot := filepath.Join(tmpRoot, "source")
	if err := snapshotpayload.MaterializeToNew(snapshotDir, sourceRoot, snapshotpayload.OptionsFromDescriptor(desc), func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	}); err != nil {
		return "", fmt.Errorf("materialize source save point: %w", err)
	}
	return restoreplan.PathEvidenceFromRoot(sourceRoot, path, boundary.ExcludesRelativePath)
}

func backupState(backup Backup) BackupState {
	if backup.State != "" {
		return backup.State
	}
	if backup.PayloadRolledBack {
		return BackupStateRolledBack
	}
	return BackupStatePending
}

func backupPayloadAlreadyAtRecoveryPoint(backup Backup) bool {
	return backupState(backup) == BackupStateRolledBack || backup.PayloadRolledBack
}

func backupPayloadRestoreRequired(backup Backup) bool {
	return backupState(backup) != BackupStatePending && !backupPayloadAlreadyAtRecoveryPoint(backup)
}

func pathBackupEntries(backup Backup) []BackupEntry {
	if len(backup.Entries) > 0 {
		return append([]BackupEntry(nil), backup.Entries...)
	}
	entries := make([]BackupEntry, 0, len(backup.TouchedPaths))
	for _, rel := range backup.TouchedPaths {
		entries = append(entries, BackupEntry{Path: rel, HadOriginal: true})
	}
	return entries
}

func worktreeStateEqual(a, b WorktreeState) bool {
	if a.Name != b.Name ||
		a.RealPath != b.RealPath ||
		a.BaseSnapshotID != b.BaseSnapshotID ||
		a.HeadSnapshotID != b.HeadSnapshotID ||
		a.LatestSnapshotID != b.LatestSnapshotID ||
		!a.CreatedAt.Equal(b.CreatedAt) {
		return false
	}
	return pathSourcesEqual(a.PathSources, b.PathSources)
}

func pathSourcesEqual(a, b model.PathSources) bool {
	if len(a) != len(b) {
		return false
	}
	for key, av := range a {
		bv, ok := b[key]
		if !ok || !reflect.DeepEqual(av, bv) {
			return false
		}
	}
	return true
}

func validatePlanID(planID string) error {
	if strings.TrimSpace(planID) == "" {
		return fmt.Errorf("recovery plan ID is required")
	}
	if err := pathutil.ValidateName(planID); err != nil {
		return fmt.Errorf("recovery plan ID is unsafe: %w", err)
	}
	return nil
}

func currentRepoID(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, repo.JVSDirName, repo.RepoIDFile))
	if err != nil {
		return "", fmt.Errorf("read repository identity: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func appendStep(steps []string, step string) []string {
	for _, existing := range steps {
		if existing == step {
			return steps
		}
	}
	return append(steps, step)
}
