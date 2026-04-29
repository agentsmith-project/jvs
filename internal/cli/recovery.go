package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		if len(args) == 0 {
			result, err := recoveryStatusList(r.Root)
			if err != nil {
				return recoveryError(err)
			}
			if jsonOutput {
				return outputJSON(result)
			}
			printRecoveryStatusList(result)
			return nil
		}
		result, err := recoveryStatusDetail(r.Root, args[0])
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSON(result)
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		result, err := runRecoveryResume(r.Root, args[0])
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSON(result)
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		result, err := runRecoveryRollback(r.Root, args[0])
		if err != nil {
			return recoveryError(err)
		}
		if jsonOutput {
			return outputJSON(result)
		}
		printRecoveryRollbackResult(result)
		return nil
	},
}

type publicRecoveryPlan struct {
	PlanID                 string  `json:"plan_id"`
	Status                 string  `json:"status"`
	Operation              string  `json:"operation"`
	RestorePlanID          string  `json:"restore_plan_id"`
	Folder                 string  `json:"folder"`
	Workspace              string  `json:"workspace"`
	SourceSavePoint        string  `json:"source_save_point"`
	Path                   string  `json:"path,omitempty"`
	RecommendedNextCommand string  `json:"recommended_next_command,omitempty"`
	LastError              string  `json:"last_error,omitempty"`
	BackupAvailable        bool    `json:"backup_available"`
	ResolvedAt             *string `json:"resolved_at,omitempty"`
}

type publicRecoveryStatusList struct {
	Mode  string               `json:"mode"`
	Plans []publicRecoveryPlan `json:"plans"`
}

type publicRecoveryActionResult struct {
	Mode                   string  `json:"mode"`
	Status                 string  `json:"status"`
	PlanID                 string  `json:"plan_id"`
	Operation              string  `json:"operation"`
	Folder                 string  `json:"folder"`
	Workspace              string  `json:"workspace"`
	SourceSavePoint        string  `json:"source_save_point"`
	Path                   string  `json:"path,omitempty"`
	RestoredSavePoint      string  `json:"restored_save_point,omitempty"`
	RestoredPath           string  `json:"restored_path,omitempty"`
	FromSavePoint          string  `json:"from_save_point,omitempty"`
	HistoryChanged         bool    `json:"history_changed"`
	BackupRemoved          bool    `json:"backup_removed"`
	ProtectionReleased     bool    `json:"protection_released"`
	NewestSavePoint        *string `json:"newest_save_point,omitempty"`
	NoWorkspaceHistoryMove bool    `json:"no_workspace_history_move"`
}

func recoveryStatusList(repoRoot string) (publicRecoveryStatusList, error) {
	plans, err := recovery.NewManager(repoRoot).List()
	if err != nil {
		return publicRecoveryStatusList{}, err
	}
	result := publicRecoveryStatusList{Mode: "status", Plans: []publicRecoveryPlan{}}
	for _, plan := range plans {
		if plan.Status != recovery.StatusActive {
			continue
		}
		result.Plans = append(result.Plans, publicRecoveryPlanFromPlan(repoRoot, &plan))
	}
	return result, nil
}

func recoveryStatusDetail(repoRoot, planID string) (publicRecoveryPlan, error) {
	plan, err := recovery.NewManager(repoRoot).Load(planID)
	if err != nil {
		return publicRecoveryPlan{}, err
	}
	return publicRecoveryPlanFromPlan(repoRoot, plan), nil
}

func publicRecoveryPlanFromPlan(repoRoot string, plan *recovery.Plan) publicRecoveryPlan {
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
		RecommendedNextCommand: publicRecoveryRecommendedNextCommand(repoRoot, plan, backupStatus),
		LastError:              recoveryVocabulary(plan.LastError),
		BackupAvailable:        backupStatus.Available,
		ResolvedAt:             resolvedAt,
	}
}

func publicRecoveryRecommendedNextCommand(repoRoot string, plan *recovery.Plan, backupStatus recoveryBackupSafetyStatus) string {
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
		return "jvs recovery resume " + plan.PlanID
	}
	if backupStatus.Missing && recovery.BackupMissingIsSafe(plan) && state.State == recovery.RecognizedPreMutation {
		return "jvs recovery rollback " + plan.PlanID
	}
	return ""
}

func runRecoveryRollback(repoRoot, planID string) (publicRecoveryActionResult, error) {
	var result publicRecoveryActionResult
	err := repo.WithMutationLock(repoRoot, "recovery rollback", func() error {
		mgr := recovery.NewManager(repoRoot)
		plan, err := mgr.Load(planID)
		if err != nil {
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
		if err := mgr.RestoreBackup(plan); err != nil {
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
		}
		return nil
	})
	return result, err
}

func runRecoveryResume(repoRoot, planID string) (publicRecoveryActionResult, error) {
	var result publicRecoveryActionResult
	err := repo.WithMutationLock(repoRoot, "recovery resume", func() error {
		mgr := recovery.NewManager(repoRoot)
		plan, err := mgr.Load(planID)
		if err != nil {
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
		if state.State == recovery.RecognizedPreMutation {
			if recoveryBackupStatus(repoRoot, plan).Available {
				restoredBackupPath = plan.Backup.Path
			}
		} else {
			if err := mgr.RestoreBackup(plan); err != nil {
				if errors.Is(err, recovery.ErrBackupMissing) {
					if verifyErr := recovery.VerifyMissingBackupRecoveryPoint(repoRoot, plan); verifyErr != nil {
						return fmt.Errorf("recovery resume cannot return to the saved recovery point safely: %w", verifyErr)
					}
				} else {
					return fmt.Errorf("recovery resume cannot return to the saved recovery point safely: %w", err)
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

		restorer := restore.NewRestorer(repoRoot, detectEngine(repoRoot))
		switch plan.Operation {
		case recovery.OperationRestore:
			err = restorer.RestoreLockedWithOptions(plan.Workspace, plan.SourceSavePoint, restore.RunOptions{BackupPath: plan.Backup.Path})
		case recovery.OperationRestorePath:
			err = restorer.RestorePathLockedWithOptions(plan.Workspace, plan.SourceSavePoint, plan.Path, restore.RunOptions{BackupPath: plan.Backup.Path})
		default:
			return fmt.Errorf("recovery operation is not supported")
		}
		if err != nil {
			if _, ok := restore.AsIncompleteError(err); ok {
				return keepRecoveryPlanActiveAfterRestoreFailure(repoRoot, plan, err)
			}
			return keepRecoveryPlanActiveAfterNonIncompleteFailure(repoRoot, plan, err)
		}
		if err := markRecoveryRestoreApplied(repoRoot, plan); err != nil {
			return err
		}
		if err := mgr.MarkResolved(plan.PlanID); err != nil {
			return err
		}
		result, err = publicRecoveryResultAfterResume(repoRoot, plan)
		return err
	})
	return result, err
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

func keepRecoveryPlanActiveAfterRestoreFailure(repoRoot string, plan *recovery.Plan, restoreErr error) error {
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
	plan.LastError = restoreErr.Error()
	plan.CompletedSteps = []string{"restore attempted", "recovery backup retained"}
	plan.PendingSteps = []string{"resume restore or rollback"}
	plan.RecommendedNextCommand = "jvs recovery resume " + plan.PlanID
	plan.UpdatedAt = time.Now().UTC()
	if err := recovery.NewManager(repoRoot).Write(plan); err != nil {
		return err
	}
	return restoreDidNotFinishError(plan)
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

func keepRecoveryPlanActiveAfterNonIncompleteFailure(repoRoot string, plan *recovery.Plan, restoreErr error) error {
	if evidence, err := currentRecoveryEvidence(repoRoot, plan); err == nil {
		plan.RecoveryEvidence = evidence
	}
	plan.LastError = restoreErr.Error()
	plan.CompletedSteps = []string{"restore retried", "recovery point retained"}
	plan.PendingSteps = []string{"resume restore or rollback"}
	plan.RecommendedNextCommand = "jvs recovery resume " + plan.PlanID
	plan.UpdatedAt = time.Now().UTC()
	if err := recovery.NewManager(repoRoot).Write(plan); err != nil {
		return err
	}
	return restoreErr
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

func restoreDidNotFinishError(plan *recovery.Plan) error {
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
	fmt.Fprintf(&b, "Run: jvs recovery status %s", plan.PlanID)
	return fmt.Errorf("%s", b.String())
}

func activeRecoveryBlocksRestoreError(plan recovery.Plan) error {
	return fmt.Errorf("workspace has active recovery plan %s; run jvs recovery status %s, jvs recovery resume %s, or jvs recovery rollback %s before another restore", plan.PlanID, plan.PlanID, plan.PlanID, plan.PlanID)
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
	fmt.Println("History was not changed.")
	fmt.Println("Recovery backup removed.")
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
	message := recoveryVocabulary(err.Error())
	var jvsErr *errclass.JVSError
	if strings.Contains(message, "no files were changed") || strings.Contains(message, "No files were changed") {
		return fmt.Errorf("%s", message)
	}
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: recoveryVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

func recoveryVocabulary(value string) string {
	replacer := strings.NewReplacer(
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
