package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/recoverystate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

var recoveryCmd = &cobra.Command{
	Use:   "recovery",
	Short: "Recover an interrupted restore",
}

var recoveryStatusCmd = &cobra.Command{
	Use:   "status [recovery-plan]",
	Short: "Show restore recovery status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
			return recoveryError(err)
		}
		if len(args) == 0 {
			result, err := recoveryStatusList(ctx.Repo.Root, ctx.Separated)
			if err != nil {
				return recoveryError(err)
			}
			if jsonOutput {
				return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
			}
			printRecoveryStatusList(result)
			return nil
		}
		result, err := recoveryStatusDetail(ctx.Repo.Root, args[0], ctx.Separated)
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printRecoveryStatusDetail(result)
		return nil
	},
}

var recoveryResumeCmd = &cobra.Command{
	Use:   "resume <recovery-plan>",
	Short: "Resume an interrupted restore",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundary(ctx.Separated); err != nil {
			return recoveryError(err)
		}
		result, err := runRecoveryResume(ctx.Repo.Root, args[0], ctx.Separated)
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printRecoveryResumeResult(result)
		return nil
	},
}

var recoveryRollbackCmd = &cobra.Command{
	Use:   "rollback <recovery-plan>",
	Short: "Rollback an interrupted restore",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundary(ctx.Separated); err != nil {
			return recoveryError(err)
		}
		result, err := runRecoveryRollback(ctx.Repo.Root, args[0], ctx.Separated)
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printRecoveryRollbackResult(result)
		return nil
	},
}

type publicRecoveryPlan struct {
	PlanID                 string            `json:"plan_id"`
	Status                 string            `json:"status"`
	Operation              string            `json:"operation"`
	RestorePlanID          string            `json:"restore_plan_id"`
	Folder                 string            `json:"folder"`
	Workspace              string            `json:"workspace"`
	SourceSavePoint        string            `json:"source_save_point"`
	Path                   string            `json:"path,omitempty"`
	RecommendedNextCommand string            `json:"recommended_next_command,omitempty"`
	LastError              string            `json:"last_error,omitempty"`
	BackupAvailable        bool              `json:"backup_available"`
	ResolvedAt             *string           `json:"resolved_at,omitempty"`
	Transfers              []transfer.Record `json:"transfers,omitempty"`
}

type publicRecoveryStatusList struct {
	Mode         string               `json:"mode"`
	Plans        []publicRecoveryPlan `json:"plans"`
	RestoreState *publicRecoveryState `json:"restore_state,omitempty"`
}

type publicRecoveryState struct {
	State                  string `json:"state"`
	Blocking               bool   `json:"blocking"`
	PlanID                 string `json:"plan_id,omitempty"`
	RecoveryPlanID         string `json:"recovery_plan_id,omitempty"`
	Message                string `json:"message,omitempty"`
	RecommendedNextCommand string `json:"recommended_next_command,omitempty"`
}

type publicRecoveryActionResult struct {
	Mode                   string            `json:"mode"`
	Status                 string            `json:"status"`
	PlanID                 string            `json:"plan_id"`
	Operation              string            `json:"operation"`
	Folder                 string            `json:"folder"`
	Workspace              string            `json:"workspace"`
	SourceSavePoint        string            `json:"source_save_point"`
	Path                   string            `json:"path,omitempty"`
	RestoredSavePoint      string            `json:"restored_save_point,omitempty"`
	RestoredPath           string            `json:"restored_path,omitempty"`
	FromSavePoint          string            `json:"from_save_point,omitempty"`
	HistoryChanged         bool              `json:"history_changed"`
	BackupRemoved          bool              `json:"backup_removed"`
	ProtectionReleased     bool              `json:"protection_released"`
	NewestSavePoint        *string           `json:"newest_save_point,omitempty"`
	NoWorkspaceHistoryMove bool              `json:"no_workspace_history_move"`
	Transfers              []transfer.Record `json:"transfers,omitempty"`
}

func recoveryStatusList(repoRoot string, separated *repo.SeparatedContext) (publicRecoveryStatusList, error) {
	plans, err := recovery.NewManager(repoRoot).List()
	if err != nil {
		if separated != nil {
			return publicRecoveryStatusList{}, separatedRecoveryStateStatusInspectError(separated, err)
		}
		return publicRecoveryStatusList{}, err
	}
	result := publicRecoveryStatusList{Mode: "status", Plans: []publicRecoveryPlan{}}
	if separated != nil {
		state, err := recoverystate.Inspect(repoRoot, separated.Workspace, separated)
		if err != nil {
			return publicRecoveryStatusList{}, err
		}
		if state.Kind == recoverystate.KindMalformedBlocking {
			return publicRecoveryStatusList{}, separatedRecoveryStateStatusError(separated, state)
		}
		if state.Kind == recoverystate.KindPendingRestorePreview || state.Kind == recoverystate.KindStaleRestorePreview {
			result.RestoreState = publicRecoveryStateFromState(state, separated)
		}
	}
	for _, plan := range plans {
		if plan.Status != recovery.StatusActive {
			continue
		}
		if err := validateSeparatedRecoveryStatusPlan(repoRoot, &plan, separated); err != nil {
			return publicRecoveryStatusList{}, err
		}
		result.Plans = append(result.Plans, publicRecoveryPlanFromPlan(repoRoot, &plan, separated))
	}
	return result, nil
}

func recoveryStatusDetail(repoRoot, planID string, separated *repo.SeparatedContext) (publicRecoveryPlan, error) {
	if separated != nil {
		state, err := recoverystate.Inspect(repoRoot, separated.Workspace, separated)
		if err != nil {
			return publicRecoveryPlan{}, err
		}
		if state.Kind == recoverystate.KindMalformedBlocking {
			return publicRecoveryPlan{}, separatedRecoveryStateStatusError(separated, state)
		}
	}
	plan, err := recovery.NewManager(repoRoot).Load(planID)
	if err != nil {
		return publicRecoveryPlan{}, err
	}
	if err := validateSeparatedRecoveryStatusPlan(repoRoot, plan, separated); err != nil {
		return publicRecoveryPlan{}, err
	}
	return publicRecoveryPlanFromPlan(repoRoot, plan, separated), nil
}

func validateSeparatedRecoveryStatusPlan(repoRoot string, plan *recovery.Plan, separated *repo.SeparatedContext) error {
	if separated == nil || plan == nil {
		return nil
	}
	if state := recoverystate.ClassifyRecoveryPlanBinding(repoRoot, separated.Workspace, separated, plan); state.Kind == recoverystate.KindMalformedBlocking {
		return separatedRecoveryStateStatusError(separated, state)
	}
	if plan.Status != recovery.StatusActive {
		return nil
	}
	if err := validateSeparatedPayloadSymlinkBoundaryForRecoveryPlan(separated, plan); err != nil {
		return err
	}
	if err := recovery.NewManager(repoRoot).ValidateLiveBackup(plan); err != nil && !errors.Is(err, recovery.ErrBackupMissing) {
		return errclass.ErrRecoveryBlocking.
			WithMessagef("recovery plan %s cannot be inspected safely: %v", plan.PlanID, recoveryVocabulary(err.Error())).
			WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, "doctor --strict --json"))
	}
	if strings.TrimSpace(plan.RecoveryEvidence) == "" {
		return nil
	}
	if _, err := recovery.RecognizeCurrentState(repoRoot, plan); err != nil {
		return errclass.ErrRecoveryBlocking.
			WithMessagef("recovery plan %s current workspace folder evidence does not match the active external control root binding: %v", plan.PlanID, recoveryVocabulary(err.Error())).
			WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, "recovery status "+plan.PlanID))
	}
	return nil
}

func publicRecoveryPlanFromPlan(repoRoot string, plan *recovery.Plan, separated *repo.SeparatedContext) publicRecoveryPlan {
	var resolvedAt *string
	if plan.ResolvedAt != nil {
		value := plan.ResolvedAt.Format(time.RFC3339)
		resolvedAt = &value
	}
	backupStatus := recoveryBackupStatus(repoRoot, plan)
	return publicRecoveryPlan{
		PlanID:                 plan.PlanID,
		Status:                 string(plan.Status),
		Operation:              string(plan.Operation),
		RestorePlanID:          plan.RestorePlanID,
		Folder:                 plan.Folder,
		Workspace:              plan.Workspace,
		SourceSavePoint:        string(plan.SourceSavePoint),
		Path:                   plan.Path,
		RecommendedNextCommand: publicRecoveryRecommendedNextCommand(repoRoot, plan, backupStatus, separated),
		LastError:              recoveryVocabulary(plan.LastError),
		BackupAvailable:        backupStatus.Available,
		ResolvedAt:             resolvedAt,
		Transfers:              append([]transfer.Record(nil), plan.Transfers...),
	}
}

func publicRecoveryRecommendedNextCommand(repoRoot string, plan *recovery.Plan, backupStatus recoveryBackupSafetyStatus, separated *repo.SeparatedContext) string {
	if plan == nil || plan.Status != recovery.StatusActive {
		return ""
	}
	if !backupStatus.Available && !backupStatus.Missing {
		return ""
	}
	state, err := recovery.RecognizeCurrentState(repoRoot, plan)
	if err != nil {
		return ""
	}
	if backupStatus.Available {
		return selectedJVSCommand(separated, "recovery resume "+plan.PlanID)
	}
	if backupStatus.Missing && recovery.BackupMissingIsSafe(plan) && state.State == recovery.RecognizedPreMutation {
		return selectedJVSCommand(separated, "recovery rollback "+plan.PlanID)
	}
	return ""
}

func runRecoveryRollback(repoRoot, planID string, separated *repo.SeparatedContext) (publicRecoveryActionResult, error) {
	var result publicRecoveryActionResult
	err := repo.WithMutationLock(repoRoot, "recovery rollback", func() error {
		mgr := recovery.NewManager(repoRoot)
		plan, err := mgr.Load(planID)
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundaryForRecoveryPlan(separated, plan); err != nil {
			return err
		}
		if plan.Status != recovery.StatusActive {
			return fmt.Errorf("recovery plan is already resolved")
		}
		state, err := verifyRecoveryEvidence(repoRoot, plan)
		if err != nil {
			return err
		}
		if state.State == recovery.RecognizedPreMutation {
			if err := mgr.ValidateLiveBackup(plan); err != nil {
				if errors.Is(err, recovery.ErrBackupMissing) {
					if verifyErr := recovery.VerifyMissingBackupRecoveryPoint(repoRoot, plan); verifyErr != nil {
						return fmt.Errorf("recovery rollback cannot be completed safely: %w", verifyErr)
					}
				} else {
					return fmt.Errorf("recovery rollback cannot be completed safely: %w", err)
				}
			}
			backupRemoved, err := resolveRecoveryPlanAndMaybeRemoveBackup(repoRoot, mgr, plan)
			if err != nil {
				return err
			}
			result = publicRecoveryActionResult{
				Mode:                   "rollback",
				Status:                 "resolved",
				PlanID:                 plan.PlanID,
				Operation:              string(plan.Operation),
				Folder:                 plan.Folder,
				Workspace:              plan.Workspace,
				SourceSavePoint:        string(plan.SourceSavePoint),
				Path:                   plan.Path,
				HistoryChanged:         false,
				BackupRemoved:          backupRemoved,
				ProtectionReleased:     true,
				NoWorkspaceHistoryMove: true,
			}
			return nil
		}
		if err := mgr.RestoreBackupWithOptions(plan, recovery.RestoreBackupOptions{TransferOperation: "recovery_rollback"}); err != nil {
			if errors.Is(err, recovery.ErrBackupMissing) {
				if verifyErr := recovery.VerifyMissingBackupRecoveryPoint(repoRoot, plan); verifyErr != nil {
					return fmt.Errorf("recovery rollback cannot be completed safely: %w", verifyErr)
				}
				backupRemoved, err := resolveRecoveryPlanAndMaybeRemoveBackup(repoRoot, mgr, plan)
				if err != nil {
					return err
				}
				result = publicRecoveryActionResult{
					Mode:                   "rollback",
					Status:                 "resolved",
					PlanID:                 plan.PlanID,
					Operation:              string(plan.Operation),
					Folder:                 plan.Folder,
					Workspace:              plan.Workspace,
					SourceSavePoint:        string(plan.SourceSavePoint),
					Path:                   plan.Path,
					HistoryChanged:         false,
					BackupRemoved:          backupRemoved,
					ProtectionReleased:     true,
					NoWorkspaceHistoryMove: true,
				}
				return nil
			}
			return fmt.Errorf("recovery rollback cannot be completed safely: %w", err)
		}
		transfers := mgr.LastTransferRecords()
		appendRecoveryCopyPointTransfers(plan, transfers)
		if err := markRecoveryBackupRestored(repoRoot, plan); err != nil {
			return err
		}
		backupRemoved, err := resolveRecoveryPlanAndMaybeRemoveBackup(repoRoot, mgr, plan)
		if err != nil {
			return err
		}
		result = publicRecoveryActionResult{
			Mode:                   "rollback",
			Status:                 "resolved",
			PlanID:                 plan.PlanID,
			Operation:              string(plan.Operation),
			Folder:                 plan.Folder,
			Workspace:              plan.Workspace,
			SourceSavePoint:        string(plan.SourceSavePoint),
			Path:                   plan.Path,
			HistoryChanged:         false,
			BackupRemoved:          backupRemoved,
			ProtectionReleased:     true,
			NoWorkspaceHistoryMove: true,
			Transfers:              transfers,
		}
		return nil
	})
	return result, err
}

func runRecoveryResume(repoRoot, planID string, separated *repo.SeparatedContext) (publicRecoveryActionResult, error) {
	var result publicRecoveryActionResult
	err := repo.WithMutationLock(repoRoot, "recovery resume", func() error {
		mgr := recovery.NewManager(repoRoot)
		plan, err := mgr.Load(planID)
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundaryForRecoveryPlan(separated, plan); err != nil {
			return err
		}
		if plan.Status != recovery.StatusActive {
			return fmt.Errorf("recovery plan is already resolved")
		}
		state, err := verifyRecoveryEvidence(repoRoot, plan)
		if err != nil {
			return err
		}
		if plan.Phase == recovery.PhaseRestoreApplied || state.State == recovery.RecognizedRestoreTarget {
			if err := mgr.MarkResolved(plan.PlanID); err != nil {
				return err
			}
			result, err = publicRecoveryResultAfterResume(repoRoot, plan)
			return err
		}
		restoredBackupPath := ""
		var backupRestoreTransfers []transfer.Record
		if state.State == recovery.RecognizedPreMutation {
			if recoveryBackupStatus(repoRoot, plan).Available {
				restoredBackupPath = plan.Backup.Path
			}
		} else {
			restoreBackupErr := mgr.RestoreBackupWithOptions(plan, recovery.RestoreBackupOptions{TransferOperation: "recovery_resume"})
			backupRestoreTransfers = mgr.LastTransferRecords()
			appendRecoveryCopyPointTransfers(plan, backupRestoreTransfers)
			if restoreBackupErr != nil {
				if errors.Is(restoreBackupErr, recovery.ErrBackupMissing) {
					if verifyErr := recovery.VerifyMissingBackupRecoveryPoint(repoRoot, plan); verifyErr != nil {
						return fmt.Errorf("recovery resume cannot return to the saved recovery point safely: %w", verifyErr)
					}
				} else {
					if len(backupRestoreTransfers) > 0 {
						restoreBackupErr = keepRecoveryPlanActiveAfterBackupRestoreFailure(repoRoot, plan, restoreBackupErr, separated)
					}
					return fmt.Errorf("recovery resume cannot return to the saved recovery point safely: %w", restoreBackupErr)
				}
			} else {
				restoredBackupPath = plan.Backup.Path
			}
		}
		evidence, err := currentRecoveryEvidence(repoRoot, plan)
		if err != nil {
			return err
		}
		plan.Backup.Path = restoreRecoveryBackupPath(plan.Folder)
		plan.Backup.State = recovery.BackupStatePending
		plan.Backup.PayloadRolledBack = false
		plan.Backup.Entries = nil
		plan.Backup.PayloadEvidence = ""
		plan.RecoveryEvidence = evidence
		plan.LastError = ""
		plan.UpdatedAt = time.Now().UTC()
		if err := mgr.Write(plan); err != nil {
			return err
		}
		if restoredBackupPath != "" {
			if err := mgr.RemoveBackupPath(plan, restoredBackupPath); err != nil {
				return err
			}
		}

		restorer := restore.NewRestorer(repoRoot, requestedTransferEngine(repoRoot))
		switch plan.Operation {
		case recovery.OperationRestore:
			err = restorer.RestoreLockedWithOptions(plan.Workspace, plan.SourceSavePoint, restore.RunOptions{BackupPath: plan.Backup.Path})
		case recovery.OperationRestorePath:
			err = restorer.RestorePathLockedWithOptions(plan.Workspace, plan.SourceSavePoint, plan.Path, restore.RunOptions{BackupPath: plan.Backup.Path})
		default:
			return fmt.Errorf("recovery operation is not supported")
		}
		restoreTransfers := restoreTransferRecords(restorer)
		appendRecoveryCopyPointTransfers(plan, restoreTransfers)
		if err != nil {
			if _, ok := restore.AsIncompleteError(err); ok {
				return keepRecoveryPlanActiveAfterRestoreFailure(repoRoot, plan, err, separated)
			}
			return keepRecoveryPlanActiveAfterNonIncompleteFailure(repoRoot, plan, err, separated)
		}
		if err := markRecoveryRestoreApplied(repoRoot, plan); err != nil {
			return err
		}
		if err := mgr.MarkResolved(plan.PlanID); err != nil {
			return err
		}
		result, err = publicRecoveryResultAfterResume(repoRoot, plan)
		if err != nil {
			return err
		}
		result.Transfers = append([]transfer.Record(nil), backupRestoreTransfers...)
		result.Transfers = append(result.Transfers, restoreTransfers...)
		return nil
	})
	return result, err
}

func validateSeparatedPayloadSymlinkBoundaryForRecoveryPlan(separated *repo.SeparatedContext, plan *recovery.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	_, err := validateSeparatedPayloadSymlinkBoundaryForExpectedRoot(separated, plan.Folder)
	return err
}

func publicRecoveryResultAfterResume(repoRoot string, plan *recovery.Plan) (publicRecoveryActionResult, error) {
	result := publicRecoveryActionResult{
		Mode:                   "resume",
		Status:                 "resolved",
		PlanID:                 plan.PlanID,
		Operation:              string(plan.Operation),
		Folder:                 plan.Folder,
		Workspace:              plan.Workspace,
		SourceSavePoint:        string(plan.SourceSavePoint),
		Path:                   plan.Path,
		HistoryChanged:         false,
		BackupRemoved:          true,
		ProtectionReleased:     true,
		NoWorkspaceHistoryMove: true,
	}
	switch plan.Operation {
	case recovery.OperationRestore:
		status, err := buildWorkspaceStatus(repoRoot, plan.Workspace)
		if err != nil {
			return publicRecoveryActionResult{}, err
		}
		result.RestoredSavePoint = string(plan.SourceSavePoint)
		result.NewestSavePoint = status.NewestSavePoint
	case recovery.OperationRestorePath:
		result.RestoredPath = plan.Path
		result.FromSavePoint = string(plan.SourceSavePoint)
	}
	return result, nil
}

func verifyRecoveryEvidence(repoRoot string, plan *recovery.Plan) (recovery.CurrentState, error) {
	if strings.TrimSpace(plan.RecoveryEvidence) == "" {
		return recovery.CurrentState{}, fmt.Errorf("recovery plan cannot confirm the current folder state; no files were changed")
	}
	state, err := recovery.RecognizeCurrentState(repoRoot, plan)
	if err != nil {
		return recovery.CurrentState{}, err
	}
	return state, nil
}

func currentRecoveryEvidence(repoRoot string, plan *recovery.Plan) (string, error) {
	var evidence string
	var err error
	switch plan.Operation {
	case recovery.OperationRestore:
		evidence, err = restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
	case recovery.OperationRestorePath:
		evidence, err = restoreplan.PathEvidence(repoRoot, plan.Workspace, plan.Path)
	default:
		return "", fmt.Errorf("recovery operation is not supported")
	}
	if err != nil {
		return "", err
	}
	return evidence, nil
}

func keepRecoveryPlanActiveAfterRestoreFailure(repoRoot string, plan *recovery.Plan, restoreErr error, separated *repo.SeparatedContext) error {
	lastError := restoreErr.Error()
	if incomplete, ok := restore.AsIncompleteError(restoreErr); ok {
		plan.Backup.Path = incomplete.BackupPath
		plan.Backup.PayloadRolledBack = incomplete.PayloadRolledBack
		if incomplete.PayloadRolledBack {
			plan.Backup.State = recovery.BackupStateRolledBack
			plan.Phase = recovery.PhasePending
		} else {
			plan.Backup.State = recovery.BackupStateRequired
			plan.Phase = recovery.PhaseBackupRequired
		}
		plan.Backup.Entries = recoveryBackupEntriesFromRestore(incomplete.PathEntries)
		plan.Backup.PayloadEvidence = ""
		if plan.Backup.State == recovery.BackupStateRequired {
			if evidence, err := recovery.NewManager(repoRoot).BackupPayloadEvidence(plan); err == nil {
				plan.Backup.PayloadEvidence = evidence
			} else {
				lastError = fmt.Sprintf("%s (recovery backup identity could not be recorded: %v)", lastError, recoveryVocabulary(err.Error()))
			}
		}
	}
	switch plan.Operation {
	case recovery.OperationRestore:
		evidence, err := restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
		if err == nil {
			plan.RecoveryEvidence = evidence
		}
	case recovery.OperationRestorePath:
		evidence, err := restoreplan.PathEvidence(repoRoot, plan.Workspace, plan.Path)
		if err == nil {
			plan.RecoveryEvidence = evidence
		}
	}
	plan.LastError = lastError
	plan.CompletedSteps = []string{"restore attempted", "recovery backup retained"}
	plan.PendingSteps = []string{"resume restore or rollback"}
	plan.RecommendedNextCommand = selectedJVSCommand(separated, "recovery resume "+plan.PlanID)
	plan.UpdatedAt = time.Now().UTC()
	if err := recovery.NewManager(repoRoot).Write(plan); err != nil {
		return err
	}
	return restoreDidNotFinishError(plan, separated)
}

func recoveryBackupEntriesFromRestore(entries []restore.PathBackupEntry) []recovery.BackupEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]recovery.BackupEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, recovery.BackupEntry{Path: entry.Path, HadOriginal: entry.HadOriginal})
	}
	return out
}

func keepRecoveryPlanActiveAfterNonIncompleteFailure(repoRoot string, plan *recovery.Plan, restoreErr error, separated *repo.SeparatedContext) error {
	if evidence, err := currentRecoveryEvidence(repoRoot, plan); err == nil {
		plan.RecoveryEvidence = evidence
	}
	plan.LastError = restoreErr.Error()
	plan.CompletedSteps = []string{"restore retried", "recovery point retained"}
	plan.PendingSteps = []string{"resume restore or rollback"}
	plan.RecommendedNextCommand = selectedJVSCommand(separated, "recovery resume "+plan.PlanID)
	plan.UpdatedAt = time.Now().UTC()
	if err := recovery.NewManager(repoRoot).Write(plan); err != nil {
		return err
	}
	return restoreErr
}

func keepRecoveryPlanActiveAfterBackupRestoreFailure(repoRoot string, plan *recovery.Plan, restoreErr error, separated *repo.SeparatedContext) error {
	if evidence, err := currentRecoveryEvidence(repoRoot, plan); err == nil {
		plan.RecoveryEvidence = evidence
	}
	plan.LastError = restoreErr.Error()
	plan.PendingSteps = []string{"resume restore or rollback"}
	plan.RecommendedNextCommand = selectedJVSCommand(separated, "recovery resume "+plan.PlanID)
	plan.UpdatedAt = time.Now().UTC()
	if err := recovery.NewManager(repoRoot).Write(plan); err != nil {
		return fmt.Errorf("%w (persist active recovery plan: %v)", restoreErr, err)
	}
	return restoreErr
}

func restoreTransferRecords(restorer *restore.Restorer) []transfer.Record {
	if restorer == nil {
		return nil
	}
	record, ok := restorer.LastTransferRecord()
	if !ok {
		return nil
	}
	return []transfer.Record{record}
}

func appendRecoveryCopyPointTransfers(plan *recovery.Plan, records []transfer.Record) {
	if plan == nil || len(records) == 0 {
		return
	}
	for _, record := range records {
		if !isRecoveryPlanTransfer(record) {
			continue
		}
		plan.Transfers = append(plan.Transfers, record)
	}
}

func isRecoveryPlanTransfer(record transfer.Record) bool {
	return record.ResultKind == transfer.ResultKindFinal &&
		record.PermissionScope == transfer.PermissionScopeExecution &&
		strings.TrimSpace(record.MaterializationDestination) != ""
}

func markRecoveryRestoreApplied(repoRoot string, plan *recovery.Plan) error {
	evidence, err := recovery.CurrentEvidence(repoRoot, plan)
	if err != nil {
		return err
	}
	plan.Phase = recovery.PhaseRestoreApplied
	plan.RecoveryEvidence = evidence
	plan.LastError = ""
	plan.CompletedSteps = []string{"restore applied"}
	plan.PendingSteps = []string{"resolve recovery plan"}
	plan.UpdatedAt = time.Now().UTC()
	return recovery.NewManager(repoRoot).Write(plan)
}

func markRecoveryBackupRestored(repoRoot string, plan *recovery.Plan) error {
	evidence, err := recovery.CurrentEvidence(repoRoot, plan)
	if err != nil {
		return err
	}
	state, err := recovery.RecognizeCurrentState(repoRoot, plan)
	if err != nil {
		return fmt.Errorf("recovery backup restored but the saved state could not be verified: %w", err)
	}
	if state.State != recovery.RecognizedPreMutation {
		return fmt.Errorf("recovery backup restored but the saved state could not be verified")
	}
	plan.Phase = recovery.PhaseBackupRestored
	plan.Backup.State = recovery.BackupStateRolledBack
	plan.Backup.PayloadRolledBack = true
	plan.RecoveryEvidence = evidence
	plan.LastError = ""
	plan.CompletedSteps = []string{"recovery backup restored"}
	plan.PendingSteps = []string{"resolve recovery plan", "cleanup recovery backup"}
	plan.UpdatedAt = time.Now().UTC()
	return recovery.NewManager(repoRoot).Write(plan)
}

func resolveRecoveryPlanAndMaybeRemoveBackup(repoRoot string, mgr *recovery.Manager, plan *recovery.Plan) (bool, error) {
	backupAvailable := recoveryBackupStatus(repoRoot, plan).Available
	if err := mgr.MarkResolved(plan.PlanID); err != nil {
		return false, err
	}
	if !backupAvailable {
		return false, nil
	}
	if err := mgr.RemoveBackup(plan); err != nil {
		return false, err
	}
	return true, nil
}

func restoreDidNotFinishError(plan *recovery.Plan, separated *repo.SeparatedContext) error {
	var b strings.Builder
	if plan.Operation == recovery.OperationRestorePath {
		b.WriteString("Path restore did not finish safely.\n")
	} else {
		b.WriteString("Restore did not finish safely.\n")
	}
	fmt.Fprintf(&b, "Folder: %s\n", plan.Folder)
	fmt.Fprintf(&b, "Workspace: %s\n", plan.Workspace)
	fmt.Fprintf(&b, "Recovery plan: %s\n", plan.PlanID)
	if plan.Operation == recovery.OperationRestorePath && plan.Path != "" {
		fmt.Fprintf(&b, "Path: %s\n", plan.Path)
	}
	b.WriteString("No history was changed after the last confirmed step.\n")
	fmt.Fprintf(&b, "Run: %s", selectedJVSCommand(separated, "recovery status "+plan.PlanID))
	return fmt.Errorf("%s", b.String())
}

func activeRecoveryBlocksRestoreError(plan recovery.Plan) error {
	return errclass.ErrRecoveryBlocking.
		WithMessagef("workspace has active recovery plan %s; run jvs recovery status %s, jvs recovery resume %s, or jvs recovery rollback %s before another restore", plan.PlanID, plan.PlanID, plan.PlanID, plan.PlanID).
		WithHint("Run jvs recovery status " + plan.PlanID + ".")
}

func enforceSeparatedRecoveryMutationGuard(repoRoot, workspaceName string, separated *repo.SeparatedContext, operation string) error {
	if separated == nil {
		return nil
	}
	state, err := recoverystate.Inspect(repoRoot, workspaceName, separated)
	if err != nil {
		return separatedRecoveryMutationInspectError(separated, operation, "recovery state", err)
	}
	if !state.Blocking() {
		return nil
	}
	switch state.Kind {
	case recoverystate.KindPendingRestorePreview:
		return separatedRecoveryMutationBlockingError(separated, operation, "restore plan", state.PlanID, "pending", state.NextCommand)
	case recoverystate.KindStaleRestorePreview:
		return separatedRecoveryMutationBlockingError(separated, operation, "restore plan", state.PlanID, "stale", state.NextCommand)
	case recoverystate.KindActiveRecovery:
		return separatedRecoveryMutationBlockingError(separated, operation, "recovery plan", state.RecoveryPlanID, "active", state.NextCommand)
	case recoverystate.KindMalformedBlocking:
		return separatedRecoveryMutationStateError(separated, operation, state)
	}
	return nil
}

func publicRecoveryStateFromState(state recoverystate.State, separated *repo.SeparatedContext) *publicRecoveryState {
	result := &publicRecoveryState{
		State:          string(state.Kind),
		Blocking:       state.Blocking(),
		PlanID:         state.PlanID,
		RecoveryPlanID: state.RecoveryPlanID,
		Message:        recoveryVocabulary(state.Message),
	}
	if command := state.RecommendedJVSCommandFor(separated); command != "" {
		result.RecommendedNextCommand = command
	}
	return result
}

func separatedRecoveryMutationInspectError(separated *repo.SeparatedContext, operation, state string, err error) error {
	return errclass.ErrRecoveryBlocking.
		WithMessagef("cannot %s while external control root recovery state cannot be inspected: %s: %v", operation, state, recoveryVocabulary(err.Error())).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, "doctor --strict --json"))
}

func separatedRecoveryMutationStateError(separated *repo.SeparatedContext, operation string, state recoverystate.State) error {
	return errclass.ErrRecoveryBlocking.
		WithMessagef("cannot %s while external control root recovery state cannot be inspected: %s: %s", operation, recoveryStateCollection(state), recoveryVocabulary(state.Message)).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, state.NextCommand))
}

func separatedRecoveryStateStatusError(separated *repo.SeparatedContext, state recoverystate.State) error {
	if err := separatedRecoveryStateBoundaryError(separated, state); err != nil {
		return err
	}
	return errclass.ErrRecoveryBlocking.
		WithMessage(recoveryVocabulary(state.Message)).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, state.NextCommand))
}

func separatedRecoveryStateBoundaryError(separated *repo.SeparatedContext, state recoverystate.State) error {
	var jvsErr *errclass.JVSError
	if !errors.As(state.Cause, &jvsErr) || jvsErr.Code != errclass.ErrPathBoundaryEscape.Code {
		return nil
	}
	message := strings.TrimSpace(jvsErr.Message)
	if message == "" {
		message = state.Cause.Error()
	}
	return errclass.ErrPathBoundaryEscape.
		WithMessage(recoveryVocabulary(message)).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, "doctor --strict --json"))
}

func separatedRecoveryStateStatusInspectError(separated *repo.SeparatedContext, err error) error {
	return errclass.ErrRecoveryBlocking.
		WithMessagef("Recovery state cannot be inspected safely: %s.", recoveryVocabulary(err.Error())).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, "doctor --strict --json"))
}

func recoveryStateCollection(state recoverystate.State) string {
	if state.Collection != "" {
		return state.Collection
	}
	if state.PlanID != "" {
		return "restore plans"
	}
	return "recovery plans"
}

func separatedRecoveryMutationBlockingError(separated *repo.SeparatedContext, operation, kind, planID, stateLabel, nextCommand string) error {
	return errclass.ErrRecoveryBlocking.
		WithMessagef("cannot %s while %s %s is %s", operation, kind, planID, stateLabel).
		WithHint(separatedSelectorHint(separated.ControlRoot, separated.Workspace, nextCommand))
}

func restoreRecoveryBackupPath(folder string) string {
	return folder + ".restore-backup-" + uuidutil.NewV4()[:8]
}

type recoveryBackupSafetyStatus struct {
	Available bool
	Missing   bool
}

func recoveryBackupStatus(repoRoot string, plan *recovery.Plan) recoveryBackupSafetyStatus {
	err := recovery.NewManager(repoRoot).ValidateLiveBackup(plan)
	if err == nil {
		return recoveryBackupSafetyStatus{Available: true}
	}
	if errors.Is(err, recovery.ErrBackupMissing) {
		return recoveryBackupSafetyStatus{Missing: true}
	}
	return recoveryBackupSafetyStatus{}
}

func printRecoveryStatusList(result publicRecoveryStatusList) {
	if len(result.Plans) == 0 {
		if result.RestoreState != nil && result.RestoreState.Blocking {
			fmt.Println("Pending restore state:")
			fmt.Printf("  Status: %s\n", result.RestoreState.State)
			if result.RestoreState.PlanID != "" {
				fmt.Printf("  Restore plan: %s\n", result.RestoreState.PlanID)
			}
			if result.RestoreState.RecommendedNextCommand != "" {
				fmt.Printf("  Recommended next command: %s\n", result.RestoreState.RecommendedNextCommand)
			}
			return
		}
		fmt.Println("No active recovery plans.")
		return
	}
	fmt.Println("Active recovery plans:")
	for i, plan := range result.Plans {
		if i > 0 {
			fmt.Println()
		}
		printRecoveryPlanSummary(plan, "  ")
	}
}

func printRecoveryStatusDetail(result publicRecoveryPlan) {
	printRecoveryPlanSummary(result, "")
}

func printRecoveryPlanSummary(result publicRecoveryPlan, indent string) {
	fmt.Printf("%sRecovery plan: %s\n", indent, result.PlanID)
	fmt.Printf("%sStatus: %s\n", indent, result.Status)
	fmt.Printf("%sOperation: %s\n", indent, recoveryOperationLabel(result.Operation))
	fmt.Printf("%sFolder: %s\n", indent, result.Folder)
	fmt.Printf("%sWorkspace: %s\n", indent, result.Workspace)
	fmt.Printf("%sSource save point: %s\n", indent, color.SnapshotID(result.SourceSavePoint))
	if result.Path != "" {
		fmt.Printf("%sPath: %s\n", indent, result.Path)
	}
	fmt.Printf("%sRecovery backup: %s\n", indent, recoveryBackupAvailabilityLabel(result.BackupAvailable))
	if result.LastError != "" {
		fmt.Printf("%sLast error: %s\n", indent, result.LastError)
	}
	if result.RecommendedNextCommand != "" {
		fmt.Printf("%sRecommended next command: %s\n", indent, result.RecommendedNextCommand)
	}
}

func recoveryBackupAvailabilityLabel(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func printRecoveryRollbackResult(result publicRecoveryActionResult) {
	fmt.Println("Recovery rollback completed.")
	fmt.Printf("Recovery plan: %s\n", result.PlanID)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.Path != "" {
		fmt.Printf("Path: %s\n", result.Path)
	}
	if primary := recoveryRollbackPrimaryTransfer(result.Transfers); primary != nil {
		printRecoveryTransferSummary(result.Transfers, primary)
	}
	fmt.Println("History was restored to the pre-restore state.")
	if result.BackupRemoved {
		fmt.Println("Recovery backup removed.")
	} else {
		fmt.Println("No recovery backup was present.")
	}
}

func printRecoveryResumeResult(result publicRecoveryActionResult) {
	fmt.Println("Recovery resume completed.")
	fmt.Printf("Recovery plan: %s\n", result.PlanID)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.RestoredPath != "" {
		fmt.Printf("Restored path: %s\n", result.RestoredPath)
		fmt.Printf("From save point: %s\n", color.SnapshotID(result.FromSavePoint))
	} else {
		fmt.Printf("Restored save point: %s\n", color.SnapshotID(result.RestoredSavePoint))
	}
	if primary := recoveryResumePrimaryTransfer(result.Transfers); primary != nil {
		printRecoveryTransferSummary(result.Transfers, primary)
	}
	fmt.Println("History was not changed.")
	fmt.Println("Recovery backup removed.")
}

func printRecoveryTransferSummary(transfers []transfer.Record, primary *transfer.Record) {
	if primary == nil {
		return
	}
	printPrimaryTransferSummary(primary)
	if additional := recoveryAdditionalTransfersLabel(transfers, primary); additional != "" {
		fmt.Printf("Additional transfers: %s; see JSON for details\n", additional)
	}
}

func recoveryRollbackPrimaryTransfer(transfers []transfer.Record) *transfer.Record {
	for i := range transfers {
		if transfers[i].Primary && transfers[i].Operation == "recovery_rollback" && transfers[i].Phase == "backup_restore" {
			return &transfers[i]
		}
	}
	return firstPrimaryRecoveryTransfer(transfers)
}

func recoveryResumePrimaryTransfer(transfers []transfer.Record) *transfer.Record {
	for i := range transfers {
		if transfers[i].Primary && transfers[i].Operation == "restore" && transfers[i].Phase == "materialization" {
			return &transfers[i]
		}
	}
	for i := range transfers {
		if transfers[i].Primary && transfers[i].Operation == "recovery_resume" && transfers[i].Phase == "backup_restore" {
			return &transfers[i]
		}
	}
	return firstPrimaryRecoveryTransfer(transfers)
}

func firstPrimaryRecoveryTransfer(transfers []transfer.Record) *transfer.Record {
	for i := range transfers {
		if transfers[i].Primary {
			return &transfers[i]
		}
	}
	if len(transfers) == 0 {
		return nil
	}
	return &transfers[0]
}

func recoveryAdditionalTransfersLabel(transfers []transfer.Record, primary *transfer.Record) string {
	counts := map[string]int{}
	order := []string{}
	total := 0
	for i := range transfers {
		if primary != nil && &transfers[i] == primary {
			continue
		}
		total++
		label := recoveryTransferLabel(transfers[i])
		if counts[label] == 0 {
			order = append(order, label)
		}
		counts[label]++
	}
	if total == 0 {
		return ""
	}
	if len(order) != 1 {
		return fmt.Sprintf("%d", total)
	}
	label := order[0]
	count := counts[label]
	return fmt.Sprintf("%d %s", count, pluralRecoveryTransferLabel(label, count))
}

func recoveryTransferLabel(record transfer.Record) string {
	switch {
	case record.Phase == "backup_restore":
		return "backup restore"
	case record.Operation == "restore" && record.Phase == "materialization":
		return "restore materialization"
	default:
		return "transfer"
	}
}

func pluralRecoveryTransferLabel(label string, count int) string {
	if count == 1 {
		return label
	}
	return label + "s"
}

func recoveryOperationLabel(operation string) string {
	switch operation {
	case string(recovery.OperationRestorePath):
		return "path restore"
	default:
		return "restore"
	}
}

func recoveryError(err error) error {
	if err == nil {
		return nil
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		message := recoveryVocabulary(jvsErr.Message)
		if strings.TrimSpace(message) == "" {
			message = recoveryVocabulary(err.Error())
		}
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: recoveryVocabulary(jvsErr.Hint)}
	}
	message := recoveryVocabulary(err.Error())
	if strings.Contains(message, "no files were changed") || strings.Contains(message, "No files were changed") {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s", message)
}

func recoveryVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"workspace registry", "control data",
		"Workspace registry", "Control data",
		"registry has", "control data records",
		"Registry has", "Control data records",
		"payload root", "workspace folder",
		"payload boundary", "workspace boundary",
		"payload symlink", "workspace folder symlink",
		"payload", "workspace folder",
		".jvs/restore-plans", "restore plans",
		".jvs/recovery-plans", "recovery plans",
		".jvs", "control data",
		"control leaf", "control data entry",
		"control directory", "control data directory",
		"control path", "control data path",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"active source pins", "save point protections",
		"active source pin", "save point protection",
		"pin", "protection",
		"gc", "cleanup",
		"internal", "JVS",
		"HEAD", "source",
		"head", "source",
	)
	return replacer.Replace(value)
}

func init() {
	recoveryCmd.AddCommand(recoveryStatusCmd)
	recoveryCmd.AddCommand(recoveryResumeCmd)
	recoveryCmd.AddCommand(recoveryRollbackCmd)
	rootCmd.AddCommand(recoveryCmd)
}
