package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/recoverystate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var (
	restoreInteractive    bool
	restoreDiscardDirty   bool
	restoreIncludeWorking bool
	restorePath           string
	restoreRunPlanID      string
)

var restoreCmd = &cobra.Command{
	Use:   "restore [save-point] [--path path]",
	Short: "Restore managed files to a save point",
	Long: `Restore managed files in the active folder to a save point.

The workspace history is not changed by restore. If the folder has unsaved
changes, save them first or discard them explicitly.

Restore creates a preview plan first. Run the listed plan ID to change files.
Use --path without a save point to list candidate save points for that path.

Examples:
  jvs restore 1771589abc
  jvs restore --run <plan-id>
  jvs restore discard <plan-id>
  jvs restore --path src/config.json
  jvs restore 1771589abc --path src/config.json
  jvs restore 1771589abc --save-first
  jvs restore 1771589abc --discard-unsaved`,
	Args: validateRestoreArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveWorkspaceScoped()
		if err != nil {
			return err
		}

		if restorePathFlagChanged(cmd) {
			return runRestorePath(cmd, args, ctx)
		}

		if restoreRunFlagChanged(cmd) {
			if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
				return restorePointError(err)
			}
			return runRestorePlan(ctx.Repo.Root, ctx.Workspace, restoreRunPlanID, ctx.Separated)
		}

		if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
			return restorePointError(err)
		}
		if restoreDiscardDirty && restoreIncludeWorking {
			return restorePointError(fmt.Errorf("--discard-unsaved and --save-first cannot be used together"))
		}
		if err := enforceSeparatedRestorePreviewMutationGuard(ctx.Repo.Root, ctx.Workspace, ctx.Separated); err != nil {
			return restorePointError(err)
		}
		expectedSeparated := restoreExpectedSeparatedContext(ctx.Separated)

		targetID, err := resolvePublicSavePointID(ctx.Repo.Root, args[0])
		if err != nil {
			return restorePointError(err)
		}

		var plan *restoreplan.Plan
		var decisionReason string
		err = withActiveOperationSourcePin(ctx.Repo.Root, targetID, "restore preview", func() error {
			engineType := requestedTransferEngine(ctx.Repo.Root)
			if !restoreDiscardDirty && !restoreIncludeWorking {
				if err := checkRestorePreviewPreDirtyCapacity(ctx.Repo.Root, ctx.Workspace, targetID, ""); err != nil {
					return err
				}
				unsavedChanges, err := workspaceDirty(ctx.Repo.Root, ctx.Workspace)
				if err != nil {
					return err
				}
				if unsavedChanges {
					decisionReason = "folder has unsaved changes"
					var err error
					plan, err = restoreplan.CreateDecisionPreviewWithExpectedSeparatedContext(ctx.Repo.Root, ctx.Workspace, targetID, engineType, expectedSeparated)
					return err
				}
			}
			var err error
			plan, err = restoreplan.CreateWithExpectedSeparatedContext(ctx.Repo.Root, ctx.Workspace, targetID, engineType, restoreplan.Options{
				DiscardUnsaved: restoreDiscardDirty,
				SaveFirst:      restoreIncludeWorking,
			}, expectedSeparated)
			return err
		})
		if err != nil {
			return restorePointError(err)
		}
		result := publicRestorePreviewFromPlan(plan)
		if decisionReason != "" {
			result.DecisionReason = decisionReason
			result.NextCommands = restoreDecisionNextCommands(targetID, "", ctx.Separated)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}

		printRestorePreviewResult(result)
		return nil
	},
}

var restoreDiscardCmd = &cobra.Command{
	Use:   "discard <restore-plan-id>",
	Short: "Discard a restore preview plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveWorkspaceScoped()
		if err != nil {
			return err
		}
		if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
			return restorePointError(err)
		}
		return runRestoreDiscardPlan(ctx.Repo.Root, ctx.Workspace, args[0], ctx.Separated)
	},
}

func validateRestoreArgs(cmd *cobra.Command, args []string) error {
	if restoreRunFlagChanged(cmd) {
		if restorePathFlagChanged(cmd) {
			return fmt.Errorf("--run cannot be used with --path")
		}
		if flags := changedRestoreRunBehaviorFlags(cmd); len(flags) > 0 {
			return fmt.Errorf("restore --run options are fixed by the preview plan; run preview again to change %s. No files were changed.", strings.Join(flags, ", "))
		}
		if len(args) != 0 {
			return fmt.Errorf("restore --run accepts only a plan ID")
		}
		if strings.TrimSpace(restoreRunPlanID) == "" {
			return fmt.Errorf("--run requires a restore plan ID")
		}
		return nil
	}
	if restorePathFlagChanged(cmd) {
		if len(args) > 1 {
			return fmt.Errorf("restore --path accepts at most one save point ID")
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("save point ID is required. Choose a save point ID, then run the command again")
	}
	return nil
}

type publicRestoreResult struct {
	Mode              string            `json:"mode,omitempty"`
	PlanID            string            `json:"plan_id,omitempty"`
	Folder            string            `json:"folder"`
	Workspace         string            `json:"workspace"`
	RestoredSavePoint string            `json:"restored_save_point"`
	SourceSavePoint   string            `json:"source_save_point,omitempty"`
	NewestSavePoint   *string           `json:"newest_save_point"`
	HistoryHead       *string           `json:"history_head"`
	ContentSource     *string           `json:"content_source"`
	UnsavedChanges    bool              `json:"unsaved_changes"`
	FilesState        string            `json:"files_state"`
	HistoryChanged    bool              `json:"history_changed"`
	FilesChanged      bool              `json:"files_changed"`
	Transfers         []transfer.Record `json:"transfers,omitempty"`
}

type publicRestorePreviewResult struct {
	Mode                    string                         `json:"mode"`
	PlanID                  string                         `json:"plan_id,omitempty"`
	Scope                   string                         `json:"scope,omitempty"`
	Folder                  string                         `json:"folder"`
	Workspace               string                         `json:"workspace"`
	SourceSavePoint         string                         `json:"source_save_point"`
	Path                    string                         `json:"path,omitempty"`
	DecisionReason          string                         `json:"decision_reason,omitempty"`
	NewestSavePoint         *string                        `json:"newest_save_point"`
	HistoryHead             *string                        `json:"history_head"`
	ExpectedNewestSavePoint *string                        `json:"expected_newest_save_point"`
	ExpectedFolderEvidence  string                         `json:"expected_folder_evidence,omitempty"`
	ExpectedPathEvidence    string                         `json:"expected_path_evidence,omitempty"`
	ManagedFiles            restoreplan.ManagedFilesImpact `json:"managed_files"`
	Transfers               []transfer.Record              `json:"transfers,omitempty"`
	Options                 restoreplan.Options            `json:"options,omitempty"`
	HistoryChanged          bool                           `json:"history_changed"`
	FilesChanged            bool                           `json:"files_changed"`
	RunCommand              string                         `json:"run_command,omitempty"`
	NextCommands            []string                       `json:"next_commands,omitempty"`
}

type publicRestorePathCandidatesResult struct {
	Mode         string                       `json:"mode"`
	Folder       string                       `json:"folder"`
	Workspace    string                       `json:"workspace"`
	Path         string                       `json:"path"`
	Candidates   []publicHistoryPathCandidate `json:"candidates"`
	NextCommands []string                     `json:"next_commands"`
	FilesChanged bool                         `json:"files_changed"`
}

type publicRestoreDiscardResult struct {
	Mode                   string `json:"mode"`
	PlanID                 string `json:"plan_id"`
	Folder                 string `json:"folder"`
	Workspace              string `json:"workspace"`
	SourceSavePoint        string `json:"source_save_point"`
	Path                   string `json:"path,omitempty"`
	PlanDiscarded          bool   `json:"plan_discarded"`
	FilesChanged           bool   `json:"files_changed"`
	HistoryChanged         bool   `json:"history_changed"`
	RecommendedNextCommand string `json:"recommended_next_command,omitempty"`
}

type publicRestorePathResult struct {
	Mode               string                     `json:"mode,omitempty"`
	PlanID             string                     `json:"plan_id,omitempty"`
	Folder             string                     `json:"folder"`
	Workspace          string                     `json:"workspace"`
	RestoredPath       string                     `json:"restored_path"`
	FromSavePoint      string                     `json:"from_save_point"`
	SourceSavePoint    string                     `json:"source_save_point"`
	NewestSavePoint    *string                    `json:"newest_save_point"`
	HistoryHead        *string                    `json:"history_head"`
	ContentSource      *string                    `json:"content_source"`
	HistoryChanged     bool                       `json:"history_changed"`
	FilesChanged       bool                       `json:"files_changed"`
	PathSourceRecorded bool                       `json:"path_source_recorded"`
	PathSources        []publicRestoredPathSource `json:"path_sources,omitempty"`
	UnsavedChanges     bool                       `json:"unsaved_changes"`
	FilesState         string                     `json:"files_state"`
	Transfers          []transfer.Record          `json:"transfers,omitempty"`
}

func restorePathFlagChanged(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("path")
	return flag != nil && flag.Changed
}

func restoreRunFlagChanged(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("run")
	return flag != nil && flag.Changed
}

func changedRestoreRunBehaviorFlags(cmd *cobra.Command) []string {
	flags := []struct {
		name       string
		publicName string
	}{
		{name: "save-first", publicName: "--save-first"},
		{name: "discard-unsaved", publicName: "--discard-unsaved"},
		{name: "interactive", publicName: "run-time restore options"},
	}
	var changed []string
	seen := map[string]bool{}
	for _, item := range flags {
		flag := cmd.Flags().Lookup(item.name)
		if flag != nil && flag.Changed {
			if seen[item.publicName] {
				continue
			}
			seen[item.publicName] = true
			changed = append(changed, item.publicName)
		}
	}
	return changed
}

type restoreRunResult struct {
	Scope          string
	ProtectedPath  string
	FilesChanged   bool
	TransferRecord *transfer.Record
	Transfers      []transfer.Record
	Whole          publicRestoreResult
	Path           publicRestorePathResult
}

type restoreRunScopeOps struct {
	SaveFirstMessage string
	ValidateTarget   func() error
	ValidateSource   func(model.EngineType) (*transfer.Record, error)
	ApplyRestore     func(*recovery.Plan, model.EngineType) (restoreApplyOutcome, error)
	RecordResult     func() error
}

type restoreApplyOutcome struct {
	Transfer    transfer.Record
	HasTransfer bool
}

func runRestorePlan(repoRoot, workspaceName, planID string, separated *repo.SeparatedContext) error {
	result, err := executeRestorePlanRun(repoRoot, workspaceName, planID, separated)
	if err != nil {
		err = restoreRunErrorWithMutationState(err, result, separated)
		if result.ProtectedPath != "" {
			return restorePathErrorWithFilesChangedForSeparated(err, result.FilesChanged, separated, result.ProtectedPath)
		}
		return restorePointErrorForSeparated(err, separated)
	}
	return outputRestoreRunResult(result, separated)
}

func runRestoreDiscardPlan(repoRoot, workspaceName, planID string, separated *repo.SeparatedContext) error {
	result, err := executeRestoreDiscardPlan(repoRoot, workspaceName, planID, separated)
	if err != nil {
		return restorePointError(err)
	}
	if jsonOutput {
		return outputJSONWithSeparatedControl(result, separated, separatedDoctorStrictNotRun)
	}
	printRestoreDiscardResult(result)
	return nil
}

func executeRestoreDiscardPlan(repoRoot, workspaceName, planID string, separated *repo.SeparatedContext) (publicRestoreDiscardResult, error) {
	var result publicRestoreDiscardResult
	err := repo.WithMutationLock(repoRoot, "restore discard", func() error {
		plan, err := restoreplan.Load(repoRoot, planID)
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundaryForRestorePlan(separated, plan); err != nil {
			return err
		}
		if err := enforceSeparatedRestoreDiscardMutationGuard(repoRoot, workspaceName, separated, planID); err != nil {
			return err
		}
		activeRecovery, err := recovery.NewManager(repoRoot).ActiveForWorkspace(workspaceName)
		if err != nil {
			return err
		}
		if len(activeRecovery) > 0 {
			return activeRecoveryBlocksRestoreError(activeRecovery[0])
		}
		discarded, err := restoreplan.Discard(repoRoot, workspaceName, planID)
		if err != nil {
			return err
		}
		result = publicRestoreDiscardResult{
			Mode:                   "discard",
			PlanID:                 discarded.PlanID,
			Folder:                 discarded.Folder,
			Workspace:              discarded.Workspace,
			SourceSavePoint:        string(discarded.SourceSavePoint),
			Path:                   discarded.Path,
			PlanDiscarded:          true,
			FilesChanged:           false,
			HistoryChanged:         false,
			RecommendedNextCommand: selectedJVSCommand(separated, "recovery status"),
		}
		return nil
	})
	return result, err
}

func executeRestorePlanRun(repoRoot, workspaceName, planID string, separated *repo.SeparatedContext) (restoreRunResult, error) {
	result := restoreRunResult{Scope: restoreplan.ScopeWhole}
	err := repo.WithMutationLock(repoRoot, "restore run", func() error {
		plan, err := restoreplan.Load(repoRoot, planID)
		if err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundaryForRestorePlan(separated, plan); err != nil {
			return err
		}
		if !plan.IsRunnable() {
			return fmt.Errorf("restore preview requires a safety decision; rerun preview with --save-first or --discard-unsaved. No files were changed.")
		}
		if err := enforceSeparatedRestoreRunMutationGuard(repoRoot, workspaceName, separated, planID); err != nil {
			return err
		}
		result.Scope = plan.EffectiveScope()
		if result.Scope == restoreplan.ScopePath {
			result.ProtectedPath = plan.Path
		}
		activeRecovery, err := recovery.NewManager(repoRoot).ActiveForWorkspace(workspaceName)
		if err != nil {
			return err
		}
		if len(activeRecovery) > 0 {
			return activeRecoveryBlocksRestoreError(activeRecovery[0])
		}
		return withActiveOperationSourcePin(repoRoot, plan.SourceSavePoint, "restore run", func() error {
			return runLoadedRestorePlan(repoRoot, workspaceName, plan, &result, separated)
		})
	})
	return result, err
}

func enforceSeparatedRestorePreviewMutationGuard(repoRoot, workspaceName string, separated *repo.SeparatedContext) error {
	return enforceSeparatedRecoveryMutationGuard(repoRoot, workspaceName, separated, "restore preview")
}

func enforceSeparatedRestoreRunMutationGuard(repoRoot, workspaceName string, separated *repo.SeparatedContext, planID string) error {
	return enforceSeparatedRestoreMutationGuard(repoRoot, workspaceName, separated, "restore --run", planID, func(state recoverystate.State) bool {
		return state.Kind == recoverystate.KindPendingRestorePreview && state.PlanID == planID
	})
}

func enforceSeparatedRestoreDiscardMutationGuard(repoRoot, workspaceName string, separated *repo.SeparatedContext, planID string) error {
	return enforceSeparatedRestoreMutationGuard(repoRoot, workspaceName, separated, "restore discard", planID, func(state recoverystate.State) bool {
		switch state.Kind {
		case recoverystate.KindPendingRestorePreview, recoverystate.KindStaleRestorePreview:
			return state.PlanID == planID
		default:
			return false
		}
	})
}

func enforceSeparatedRestoreMutationGuard(repoRoot, workspaceName string, separated *repo.SeparatedContext, operation, planID string, allow func(recoverystate.State) bool) error {
	if separated == nil {
		return nil
	}
	state, err := recoverystate.Inspect(repoRoot, workspaceName, separated)
	if err != nil {
		return separatedRecoveryMutationInspectError(separated, operation, "recovery state", err)
	}
	if !state.Blocking() || allow(state) {
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
	default:
		return nil
	}
}

func validateSeparatedPayloadSymlinkBoundaryForRestorePlan(separated *repo.SeparatedContext, plan *restoreplan.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	_, err := validateSeparatedPayloadSymlinkBoundaryForExpectedRoot(separated, plan.Folder)
	return err
}

func restoreExpectedSeparatedContext(separated *repo.SeparatedContext) restoreplan.ExpectedSeparatedContext {
	if separated == nil || separated.Repo == nil {
		return restoreplan.ExpectedSeparatedContext{}
	}
	return restoreplan.ExpectedSeparatedContext{
		RepoID:      separated.Repo.RepoID,
		ControlRoot: separated.ControlRoot,
		PayloadRoot: separated.PayloadRoot,
		Workspace:   separated.Workspace,
	}
}

func runLoadedRestorePlan(repoRoot, workspaceName string, plan *restoreplan.Plan, result *restoreRunResult, separated *repo.SeparatedContext) error {
	ops, err := restoreRunOpsForScope(repoRoot, workspaceName, plan, result)
	if err != nil {
		return err
	}
	if err := ops.ValidateTarget(); err != nil {
		return err
	}
	sourceState, err := restoreplan.InspectSourceReadOnly(repoRoot, plan.SourceSavePoint)
	if err != nil {
		return err
	}
	if err := checkRestoreRunCapacity(repoRoot, workspaceName, plan, sourceState.SnapshotDir, sourceState.Descriptor); err != nil {
		return err
	}
	engineType := requestedTransferEngine(repoRoot)
	validationTransfer, err := ops.ValidateSource(engineType)
	if err != nil {
		return err
	}
	if validationTransfer != nil {
		result.Transfers = append(result.Transfers, *validationTransfer)
	}
	safetyTransfer, err := createRestoreSafetySave(repoRoot, workspaceName, engineType, plan.Options.SaveFirst, ops.SaveFirstMessage)
	if err != nil {
		return err
	}
	if safetyTransfer != nil {
		result.Transfers = append(result.Transfers, *safetyTransfer)
	}
	recoveryPlan, err := recovery.NewManager(repoRoot).CreateActiveForRestore(plan, restoreRecoveryBackupPath(plan.Folder))
	if err != nil {
		return err
	}
	appendRecoveryCopyPointTransfers(recoveryPlan, result.Transfers)
	applyOutcome, err := ops.ApplyRestore(recoveryPlan, engineType)
	if applyOutcome.HasTransfer {
		record := applyOutcome.Transfer
		result.TransferRecord = &record
		result.Transfers = append(result.Transfers, record)
		appendRecoveryCopyPointTransfers(recoveryPlan, []transfer.Record{record})
	}
	if err != nil {
		result.FilesChanged = restoreApplyErrorChangedFiles(err)
		return handleRestoreApplyError(repoRoot, recoveryPlan, err, separated)
	}
	result.FilesChanged = true
	if err := resolveAppliedRestoreRecovery(repoRoot, recoveryPlan); err != nil {
		return err
	}
	return ops.RecordResult()
}

func restoreRunOpsForScope(repoRoot, workspaceName string, plan *restoreplan.Plan, result *restoreRunResult) (restoreRunScopeOps, error) {
	switch result.Scope {
	case restoreplan.ScopeWhole:
		return restoreRunScopeOps{
			SaveFirstMessage: "save before restore",
			ValidateTarget: func() error {
				return restoreplan.ValidateTarget(repoRoot, workspaceName, plan)
			},
			ValidateSource: func(engineType model.EngineType) (*transfer.Record, error) {
				return restoreplan.ValidateSource(repoRoot, workspaceName, plan, engineType)
			},
			ApplyRestore: func(recoveryPlan *recovery.Plan, engineType model.EngineType) (restoreApplyOutcome, error) {
				restorer := restore.NewRestorer(repoRoot, engineType)
				err := restorer.RestoreLockedWithOptions(workspaceName, plan.SourceSavePoint, restore.RunOptions{BackupPath: recoveryPlan.Backup.Path})
				return restoreApplyOutcomeFromRestorer(restorer), err
			},
			RecordResult: func() error {
				status, err := publicRestoreStatus(repoRoot, workspaceName, plan.SourceSavePoint)
				if err != nil {
					return err
				}
				status.Mode = "run"
				status.PlanID = plan.PlanID
				status.SourceSavePoint = string(plan.SourceSavePoint)
				status.FilesChanged = true
				status.Transfers = append([]transfer.Record(nil), result.Transfers...)
				result.Whole = status
				return nil
			},
		}, nil
	case restoreplan.ScopePath:
		return restoreRunScopeOps{
			SaveFirstMessage: "save before restore path",
			ValidateTarget: func() error {
				return restoreplan.ValidatePathTarget(repoRoot, workspaceName, plan)
			},
			ValidateSource: func(engineType model.EngineType) (*transfer.Record, error) {
				return restoreplan.ValidateSourcePath(repoRoot, workspaceName, plan, engineType)
			},
			ApplyRestore: func(recoveryPlan *recovery.Plan, engineType model.EngineType) (restoreApplyOutcome, error) {
				restorer := restore.NewRestorer(repoRoot, engineType)
				err := restorer.RestorePathLockedWithOptions(workspaceName, plan.SourceSavePoint, plan.Path, restore.RunOptions{BackupPath: recoveryPlan.Backup.Path})
				return restoreApplyOutcomeFromRestorer(restorer), err
			},
			RecordResult: func() error {
				status, err := publicRestorePathStatus(repoRoot, workspaceName, plan.Path, plan.SourceSavePoint)
				if err != nil {
					return err
				}
				status.Mode = "run"
				status.PlanID = plan.PlanID
				status.Transfers = append([]transfer.Record(nil), result.Transfers...)
				result.Path = status
				return nil
			},
		}, nil
	default:
		return restoreRunScopeOps{}, fmt.Errorf("restore plan scope is not supported")
	}
}

func createRestoreSafetySave(repoRoot, workspaceName string, engineType model.EngineType, saveFirst bool, message string) (*transfer.Record, error) {
	if !saveFirst {
		return nil, nil
	}
	creator := snapshot.NewCreator(repoRoot, engineType)
	_, err := creator.CreateSavePointLocked(workspaceName, message, nil)
	if err != nil {
		return nil, err
	}
	record, ok := creator.LastTransferRecord()
	if !ok {
		return nil, nil
	}
	return &record, nil
}

func restoreApplyOutcomeFromRestorer(restorer *restore.Restorer) restoreApplyOutcome {
	if restorer == nil {
		return restoreApplyOutcome{}
	}
	record, ok := restorer.LastTransferRecord()
	if !ok {
		return restoreApplyOutcome{}
	}
	return restoreApplyOutcome{Transfer: record, HasTransfer: true}
}

func handleRestoreApplyError(repoRoot string, recoveryPlan *recovery.Plan, restoreErr error, separated *repo.SeparatedContext) error {
	if _, ok := restore.AsIncompleteError(restoreErr); ok {
		return keepRecoveryPlanActiveAfterRestoreFailure(repoRoot, recoveryPlan, restoreErr, separated)
	}
	if resolveErr := recovery.NewManager(repoRoot).MarkResolved(recoveryPlan.PlanID); resolveErr != nil {
		return fmt.Errorf("%w; additionally failed to resolve recovery plan: %v", restoreErr, resolveErr)
	}
	return restoreErr
}

func resolveAppliedRestoreRecovery(repoRoot string, recoveryPlan *recovery.Plan) error {
	if err := markRecoveryRestoreApplied(repoRoot, recoveryPlan); err != nil {
		return err
	}
	return recovery.NewManager(repoRoot).MarkResolved(recoveryPlan.PlanID)
}

func outputRestoreRunResult(result restoreRunResult, separated *repo.SeparatedContext) error {
	if jsonOutput {
		if result.Scope == restoreplan.ScopePath {
			return outputJSONWithSeparatedControl(result.Path, separated, separatedDoctorStrictNotRun)
		}
		return outputJSONWithSeparatedControl(result.Whole, separated, separatedDoctorStrictNotRun)
	}
	if result.Scope == restoreplan.ScopePath {
		printRestorePathResult(result.Path)
		return nil
	}
	printRestoreResult(result.Whole)
	return nil
}

func restoreApplyErrorChangedFiles(err error) bool {
	incomplete, ok := restore.AsIncompleteError(err)
	return ok && !incomplete.PayloadRolledBack
}

type restoreRunFilesChangedError struct {
	err                   error
	recoveryStatusCommand string
}

func (e *restoreRunFilesChangedError) Error() string {
	command := strings.TrimSpace(e.recoveryStatusCommand)
	if command == "" {
		command = "jvs recovery status"
	}
	return fmt.Sprintf("%v. Files were changed; run %s before continuing", e.err, command)
}

func (e *restoreRunFilesChangedError) Unwrap() error {
	return e.err
}

func restoreRunErrorWithMutationState(err error, result restoreRunResult, separated *repo.SeparatedContext) error {
	if err == nil || !result.FilesChanged {
		return err
	}
	return &restoreRunFilesChangedError{err: err, recoveryStatusCommand: selectedJVSCommand(separated, "recovery status")}
}

func runRestorePath(cmd *cobra.Command, args []string, ctx *cliDiscoveryContext) error {
	repoRoot := ctx.Repo.Root
	workspaceName := ctx.Workspace
	if len(args) == 0 {
		if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
			return restorePathError(err, restorePath)
		}
		repoRoot = ctx.Repo.Root
		workspaceName = ctx.Workspace
		path, err := normalizeRestorePathFlag(repoRoot, workspaceName, restorePath)
		if err != nil {
			return restorePathError(err, restorePath)
		}
		result, err := restorePathCandidates(repoRoot, workspaceName, path, ctx.Separated)
		if err != nil {
			return restorePathError(err, path)
		}
		if jsonOutput {
			return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printRestorePathCandidates(result)
		return nil
	}

	targetID, err := resolvePublicSavePointID(repoRoot, args[0])
	if err != nil {
		return restorePathError(err, restorePath)
	}
	if restoreDiscardDirty && restoreIncludeWorking {
		return restorePathError(fmt.Errorf("--discard-unsaved and --save-first cannot be used together"), restorePath)
	}

	if err := validateAndRefreshSeparatedPayloadBoundary(ctx); err != nil {
		return restorePathError(err, restorePath)
	}
	repoRoot = ctx.Repo.Root
	workspaceName = ctx.Workspace
	if err := enforceSeparatedRestorePreviewMutationGuard(repoRoot, workspaceName, ctx.Separated); err != nil {
		return restorePathError(err, restorePath)
	}
	expectedSeparated := restoreExpectedSeparatedContext(ctx.Separated)

	path, err := normalizeRestorePathFlag(repoRoot, workspaceName, restorePath)
	if err != nil {
		return restorePathError(err, restorePath)
	}

	var plan *restoreplan.Plan
	var decisionReason string
	err = withActiveOperationSourcePin(repoRoot, targetID, "restore path preview", func() error {
		if restoreDiscardDirty && restoreIncludeWorking {
			return fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
		}
		engineType := requestedTransferEngine(repoRoot)
		if !restoreIncludeWorking && !restoreDiscardDirty {
			if err := checkRestorePreviewPreDirtyCapacity(repoRoot, workspaceName, targetID, path); err != nil {
				return err
			}
			pathDirty, err := workspacePathDirty(repoRoot, workspaceName, path)
			if err != nil {
				return err
			}
			if pathDirty {
				decisionReason = fmt.Sprintf("path has unsaved changes in %s", path)
				var err error
				plan, err = restoreplan.CreatePathDecisionPreviewWithExpectedSeparatedContext(repoRoot, workspaceName, targetID, path, engineType, expectedSeparated)
				return err
			}
		}
		var err error
		plan, err = restoreplan.CreatePathWithExpectedSeparatedContext(repoRoot, workspaceName, targetID, path, engineType, restoreplan.Options{
			DiscardUnsaved: restoreDiscardDirty,
			SaveFirst:      restoreIncludeWorking,
		}, expectedSeparated)
		return err
	})
	if err != nil {
		return restorePathError(err, restorePath, path)
	}
	result := publicRestorePreviewFromPlan(plan)
	if decisionReason != "" {
		result.DecisionReason = decisionReason
		result.NextCommands = restoreDecisionNextCommands(targetID, path, ctx.Separated)
	}
	if jsonOutput {
		return outputJSONWithSeparatedControl(result, ctx.Separated, separatedDoctorStrictNotRun)
	}
	printRestorePreviewResult(result)
	return nil
}

func publicRestoreStatus(repoRoot, workspaceName string, restoredID model.SnapshotID) (publicRestoreResult, error) {
	status, err := buildWorkspaceStatus(repoRoot, workspaceName)
	if err != nil {
		return publicRestoreResult{}, err
	}
	return publicRestoreResult{
		Folder:            status.Folder,
		Workspace:         status.Workspace,
		RestoredSavePoint: string(restoredID),
		NewestSavePoint:   status.NewestSavePoint,
		HistoryHead:       status.HistoryHead,
		ContentSource:     status.ContentSource,
		UnsavedChanges:    status.UnsavedChanges,
		FilesState:        status.FilesState,
		HistoryChanged:    false,
		FilesChanged:      true,
	}, nil
}

func publicRestorePreviewFromPlan(plan *restoreplan.Plan) publicRestorePreviewResult {
	mode := "preview"
	if plan.DecisionOnly {
		mode = "decision_preview"
	}
	return publicRestorePreviewResult{
		Mode:                    mode,
		PlanID:                  plan.PlanID,
		Scope:                   plan.EffectiveScope(),
		Folder:                  plan.Folder,
		Workspace:               plan.Workspace,
		SourceSavePoint:         string(plan.SourceSavePoint),
		Path:                    plan.Path,
		NewestSavePoint:         publicSnapshotIDPtr(plan.NewestSavePoint),
		HistoryHead:             publicSnapshotIDPtr(plan.HistoryHead),
		ExpectedNewestSavePoint: publicSnapshotIDPtr(plan.ExpectedNewestSavePoint),
		ExpectedFolderEvidence:  plan.ExpectedFolderEvidence,
		ExpectedPathEvidence:    plan.ExpectedPathEvidence,
		ManagedFiles:            plan.ManagedFiles,
		Transfers:               append([]transfer.Record(nil), plan.Transfers...),
		Options:                 plan.Options,
		HistoryChanged:          false,
		FilesChanged:            false,
		RunCommand:              plan.RunCommand,
	}
}

func publicSnapshotIDPtr(id *model.SnapshotID) *string {
	if id == nil || *id == "" {
		return nil
	}
	value := string(*id)
	return &value
}

func normalizeRestorePathFlag(repoRoot, workspaceName, raw string) (string, error) {
	path, err := normalizeViewPath(raw)
	if err != nil {
		if strings.Contains(err.Error(), "JVS control data is not managed") {
			return "", fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
		}
		return "", fmt.Errorf("path must be a workspace-relative path")
	}
	if path == "" {
		return "", fmt.Errorf("path must be a workspace-relative path")
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if boundary.ExcludesRelativePath(path) {
		return "", fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	if err := pathutil.ValidateNoSymlinkParents(boundary.Root, path); err != nil {
		return "", fmt.Errorf("path must be a workspace-relative path: %w", err)
	}
	return path, nil
}

func restorePathCandidates(repoRoot, workspaceName, path string, separated *repo.SeparatedContext) (publicRestorePathCandidatesResult, error) {
	historyResult, err := findHistoryPathCandidates(repoRoot, workspaceName, path)
	if err != nil {
		return publicRestorePathCandidatesResult{}, err
	}
	return publicRestorePathCandidatesResult{
		Mode:         "candidates",
		Folder:       historyResult.Folder,
		Workspace:    historyResult.Workspace,
		Path:         historyResult.Path,
		Candidates:   historyResult.Candidates,
		NextCommands: restorePathNextCommands(path, historyResult.Candidates, separated),
		FilesChanged: false,
	}, nil
}

func restorePathNextCommands(path string, candidates []publicHistoryPathCandidate, separated *repo.SeparatedContext) []string {
	if len(candidates) == 0 {
		return []string{}
	}
	return []string{genericRestorePathCommand(path, separated)}
}

func restoreDecisionNextCommands(sourceID model.SnapshotID, path string, separated *repo.SeparatedContext) []string {
	return []string{
		restoreDecisionCommand(sourceID, path, "--save-first", separated),
		restoreDecisionCommand(sourceID, path, "--discard-unsaved", separated),
	}
}

func restoreDecisionCommand(sourceID model.SnapshotID, path, option string, separated *repo.SeparatedContext) string {
	source := shellQuoteArg(string(sourceID))
	if path == "" {
		return fmt.Sprintf("%s restore %s %s", selectedJVSCommandPrefix(separated), source, option)
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Sprintf("%s restore %s --path=%s %s", selectedJVSCommandPrefix(separated), source, shellQuoteArg(path), option)
	}
	return fmt.Sprintf("%s restore %s --path %s %s", selectedJVSCommandPrefix(separated), source, shellQuoteArg(path), option)
}

func genericRestorePathCommand(path string, separated *repo.SeparatedContext) string {
	if strings.HasPrefix(path, "-") {
		return fmt.Sprintf("%s restore <save> --path=%s", selectedJVSCommandPrefix(separated), shellQuoteArg(path))
	}
	return fmt.Sprintf("%s restore <save> --path %s", selectedJVSCommandPrefix(separated), shellQuoteArg(path))
}

func selectedJVSCommand(separated *repo.SeparatedContext, command string) string {
	return selectedJVSCommandPrefix(separated) + " " + strings.TrimSpace(command)
}

func selectedJVSCommandPrefix(separated *repo.SeparatedContext) string {
	if separated == nil {
		return "jvs"
	}
	workspace := strings.TrimSpace(separated.Workspace)
	if workspace == "" {
		workspace = "main"
	}
	return "jvs --control-root " + shellQuoteArg(separated.ControlRoot) + " --workspace " + shellQuoteArg(workspace)
}

func publicRestorePathStatus(repoRoot, workspaceName, path string, sourceID model.SnapshotID) (publicRestorePathResult, error) {
	status, err := buildWorkspaceStatus(repoRoot, workspaceName)
	if err != nil {
		return publicRestorePathResult{}, err
	}
	return publicRestorePathResult{
		Folder:             status.Folder,
		Workspace:          status.Workspace,
		RestoredPath:       path,
		FromSavePoint:      string(sourceID),
		SourceSavePoint:    string(sourceID),
		NewestSavePoint:    status.NewestSavePoint,
		HistoryHead:        status.HistoryHead,
		ContentSource:      status.ContentSource,
		HistoryChanged:     false,
		FilesChanged:       true,
		PathSourceRecorded: pathSourceRecorded(status.PathSources, path, sourceID),
		PathSources:        status.PathSources,
		UnsavedChanges:     status.UnsavedChanges,
		FilesState:         status.FilesState,
	}, nil
}

func pathSourceRecorded(sources []publicRestoredPathSource, path string, sourceID model.SnapshotID) bool {
	for _, source := range sources {
		if source.TargetPath == path && source.SourceSavePoint == string(sourceID) {
			return true
		}
	}
	return false
}

func printRestoreResult(result publicRestoreResult) {
	restored := color.SnapshotID(result.RestoredSavePoint)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.PlanID != "" {
		fmt.Printf("Plan: %s\n", result.PlanID)
	}
	fmt.Printf("Restored save point: %s\n", restored)
	fmt.Printf("Managed files now match save point %s.\n", restored)
	printRestoreRunTransferSummary(result.Transfers)
	if result.NewestSavePoint != nil && *result.NewestSavePoint != result.RestoredSavePoint {
		newest := color.SnapshotID(*result.NewestSavePoint)
		fmt.Printf("Newest save point is still %s.\n", newest)
		fmt.Println("History was not changed.")
		fmt.Printf("Next save creates a new save point after %s.\n", newest)
		return
	}
	fmt.Printf("Newest save point: %s\n", formatStatusSavePoint(result.NewestSavePoint))
	fmt.Println("History was not changed.")
}

func printRestorePreviewResult(result publicRestorePreviewResult) {
	fmt.Println("Preview only. No files were changed.")
	if result.DecisionReason != "" {
		fmt.Printf("Decision needed: %s.\n", result.DecisionReason)
	}
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.PlanID != "" {
		fmt.Printf("Plan: %s\n", result.PlanID)
	}
	if result.Scope == restoreplan.ScopePath {
		fmt.Println("Scope: path")
		fmt.Printf("Path: %s\n", result.Path)
	}
	fmt.Printf("Source save point: %s\n", color.SnapshotID(result.SourceSavePoint))
	if len(result.Transfers) > 0 {
		printPrimaryExpectedTransferSummary(&result.Transfers[0])
	}
	printRestorePreviewImpact("overwrite", result.ManagedFiles.Overwrite)
	printRestorePreviewImpact("delete", result.ManagedFiles.Delete)
	printRestorePreviewImpact("create", result.ManagedFiles.Create)
	printRestorePreviewOptions(result.Options)
	fmt.Println("JVS control data and runtime state are not user files; restore leaves them alone.")
	fmt.Println("History will not change.")
	newest := formatStatusSavePoint(result.NewestSavePoint)
	fmt.Printf("Newest save point is still %s.\n", newest)
	if result.NewestSavePoint != nil {
		fmt.Printf("You can return to save point %s.\n", color.SnapshotID(*result.NewestSavePoint))
	}
	fmt.Printf("Expected newest save point: %s\n", formatStatusSavePoint(result.ExpectedNewestSavePoint))
	if result.ExpectedPathEvidence != "" {
		fmt.Printf("Expected path evidence: %s\n", result.ExpectedPathEvidence)
	} else {
		fmt.Printf("Expected folder evidence: %s\n", result.ExpectedFolderEvidence)
	}
	if result.RunCommand != "" {
		fmt.Printf("Run: `%s`\n", result.RunCommand)
		return
	}
	if len(result.NextCommands) > 0 {
		fmt.Println("Rerun preview with one safety option:")
		for _, command := range result.NextCommands {
			fmt.Printf("  %s\n", command)
		}
	}
}

func printRestorePreviewImpact(label string, summary restoreplan.ChangeSummary) {
	fmt.Printf("Managed files to %s: %d\n", label, summary.Count)
	for _, sample := range summary.Samples {
		fmt.Printf("  %s\n", sample)
	}
}

func printRestorePreviewOptions(options restoreplan.Options) {
	switch {
	case options.SaveFirst:
		fmt.Println("Run options: save unsaved changes first")
	case options.DiscardUnsaved:
		fmt.Println("Run options: discard unsaved changes")
	}
}

func printRestorePathCandidates(result publicRestorePathCandidatesResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Println("No save point ID was provided.")
	fmt.Printf("Candidates for path: %s\n", result.Path)
	if len(result.Candidates) == 0 {
		fmt.Println("No candidates found.")
	} else {
		for _, candidate := range result.Candidates {
			message := candidate.Message
			if message == "" {
				message = color.Dim("(no message)")
			}
			fmt.Printf("%s  %s  %s\n",
				color.SnapshotID(candidate.SavePointID),
				color.Dim(candidate.CreatedAt.Format("2006-01-02 15:04")),
				message,
			)
		}
	}
	fmt.Println("Choose a save point ID, then run:")
	commands := result.NextCommands
	if len(commands) == 0 {
		commands = []string{genericRestorePathCommand(result.Path, nil)}
	}
	for _, command := range commands {
		fmt.Printf("  %s\n", command)
	}
	fmt.Println("No files were changed.")
}

func printRestoreDiscardResult(result publicRestoreDiscardResult) {
	fmt.Println("Restore preview discarded.")
	fmt.Printf("Plan: %s\n", result.PlanID)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.Path != "" {
		fmt.Printf("Path: %s\n", result.Path)
	}
	fmt.Println("No files were changed.")
	if result.RecommendedNextCommand != "" {
		fmt.Printf("Recommended next command: %s\n", result.RecommendedNextCommand)
	}
}

func printRestorePathResult(result publicRestorePathResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.PlanID != "" {
		fmt.Printf("Plan: %s\n", result.PlanID)
	}
	fmt.Printf("Restored path: %s\n", result.RestoredPath)
	fmt.Printf("From save point: %s\n", color.SnapshotID(result.FromSavePoint))
	printRestoreRunTransferSummary(result.Transfers)
	newest := formatStatusSavePoint(result.NewestSavePoint)
	fmt.Printf("Newest save point is still %s.\n", newest)
	fmt.Println("History was not changed.")
	fmt.Printf("Next save creates a new save point after %s and records this restored path.\n", newest)
}

func printRestoreRunTransferSummary(transfers []transfer.Record) {
	primaryIndex := restoreRunPrimaryTransferIndex(transfers)
	if primaryIndex < 0 {
		return
	}
	printPrimaryTransferSummary(&transfers[primaryIndex])
	printRestoreAdditionalTransferSummary(transfers, primaryIndex)
}

func restoreRunPrimaryTransferIndex(transfers []transfer.Record) int {
	for i := range transfers {
		if transfers[i].Primary && transfers[i].Operation == "restore" && transfers[i].Phase == "materialization" {
			return i
		}
	}
	if len(transfers) == 0 {
		return -1
	}
	return 0
}

func printRestoreAdditionalTransferSummary(transfers []transfer.Record, primaryIndex int) {
	sourceValidationCount := 0
	safetySaveCount := 0
	otherCount := 0
	for i, record := range transfers {
		if i == primaryIndex {
			continue
		}
		switch {
		case record.Operation == "restore" && record.Phase == "source_validation":
			sourceValidationCount++
		case record.Operation == "save":
			safetySaveCount++
		default:
			otherCount++
		}
	}
	parts := []string{}
	if sourceValidationCount > 0 {
		parts = append(parts, restoreAdditionalTransferCount(sourceValidationCount, "source validation", "source validations"))
	}
	if safetySaveCount > 0 {
		parts = append(parts, restoreAdditionalTransferCount(safetySaveCount, "safety save", "safety saves"))
	}
	if otherCount > 0 {
		parts = append(parts, restoreAdditionalTransferCount(otherCount, "other transfer", "other transfers"))
	}
	if len(parts) == 0 {
		return
	}
	fmt.Printf("Additional transfers: %s; see JSON for details\n", strings.Join(parts, ", "))
}

func restoreAdditionalTransferCount(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func withActiveOperationSourcePin(repoRoot string, sourceID model.SnapshotID, reason string, fn func() error) error {
	handle, err := sourcepin.NewManager(repoRoot).Create(sourceID, reason)
	if err != nil {
		return err
	}
	err = fn()
	if releaseErr := handle.Release(); releaseErr != nil {
		if err != nil {
			return fmt.Errorf("%w; additionally failed to release restore source protection: %v", err, releaseErr)
		}
		return fmt.Errorf("failed to release restore source protection: %w", releaseErr)
	}
	return err
}

func restorePointError(err error) error {
	return restorePointErrorForSeparated(err, nil)
}

func restorePointErrorForSeparated(err error, separated *repo.SeparatedContext) error {
	if err == nil {
		return nil
	}
	message := restorePointVocabulary(rewriteBareRecoveryCommandsForSeparated(err.Error(), separated))
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{
			Code:    jvsErr.Code,
			Message: message,
			Hint:    restorePointVocabulary(rewriteBareRecoveryCommandsForSeparated(jvsErr.Hint, separated)),
		}
	}
	return fmt.Errorf("%s", message)
}

func restorePathError(err error, protectedValues ...string) error {
	return restorePathErrorWithFilesChanged(err, false, protectedValues...)
}

func restorePathErrorWithFilesChanged(err error, filesChanged bool, protectedValues ...string) error {
	return restorePathErrorWithFilesChangedForSeparated(err, filesChanged, nil, protectedValues...)
}

func restorePathErrorWithFilesChangedForSeparated(err error, filesChanged bool, separated *repo.SeparatedContext, protectedValues ...string) error {
	if err == nil {
		return nil
	}
	message := restorePathVocabulary(rewriteBareRecoveryCommandsForSeparated(err.Error(), separated), protectedValues...)
	if !filesChanged && !strings.Contains(message, "No files were changed.") {
		message += ". No files were changed."
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{
			Code:    jvsErr.Code,
			Message: message,
			Hint:    restorePathVocabulary(rewriteBareRecoveryCommandsForSeparated(jvsErr.Hint, separated), protectedValues...),
		}
	}
	return fmt.Errorf("%s", message)
}

func rewriteBareRecoveryCommandsForSeparated(value string, separated *repo.SeparatedContext) string {
	if separated == nil || strings.TrimSpace(value) == "" {
		return value
	}
	replacer := strings.NewReplacer(
		"jvs recovery status", selectedJVSCommand(separated, "recovery status"),
		"jvs recovery resume", selectedJVSCommand(separated, "recovery resume"),
		"jvs recovery rollback", selectedJVSCommand(separated, "recovery rollback"),
	)
	return replacer.Replace(value)
}

func restorePathVocabulary(value string, protectedValues ...string) string {
	protected := compactProtectedValues(protectedValues)
	if len(protected) == 0 {
		return restorePointVocabulary(value)
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		if match := protectedValueAt(value, i, protected); match != "" {
			out.WriteString(match)
			i += len(match)
			continue
		}
		next := len(value)
		if idx := nextProtectedValueOffset(value, i, protected); idx >= 0 {
			next = idx
		}
		out.WriteString(restorePointVocabulary(value[i:next]))
		i = next
	}
	return out.String()
}

func compactProtectedValues(values []string) []string {
	var protected []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		protected = append(protected, value)
	}
	return protected
}

func protectedValueAt(value string, offset int, protected []string) string {
	for _, candidate := range protected {
		if strings.HasPrefix(value[offset:], candidate) && protectedValueHasDelimiters(value, offset, len(candidate)) {
			return candidate
		}
	}
	return ""
}

func nextProtectedValueOffset(value string, start int, protected []string) int {
	for i := start; i < len(value); i++ {
		if protectedValueAt(value, i, protected) != "" {
			return i
		}
	}
	return -1
}

func protectedValueHasDelimiters(value string, start, length int) bool {
	return protectedValueStartDelimited(value, start) && protectedValueEndDelimited(value, start+length)
}

func protectedValueStartDelimited(value string, start int) bool {
	if start == 0 {
		return true
	}
	return isProtectedValueDelimiter(value[start-1])
}

func protectedValueEndDelimited(value string, end int) bool {
	if end >= len(value) {
		return true
	}
	return isProtectedValueDelimiter(value[end])
}

func isProtectedValueDelimiter(b byte) bool {
	if isASCIISpace(b) {
		return true
	}
	switch b {
	case '"', '\'', '`', ':', ';', ',', '(', ')', '[', ']', '{', '}', '<', '>', '=', '.':
		return true
	default:
		return false
	}
}

func restorePointVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"dirty changes", "unsaved changes",
		"dirty", "unsaved",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"active source pins", "save point protections",
		"active source pin", "save point protection",
		"gc control data", "JVS control data",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"current", "source",
		"latest", "newest",
		"HEAD", "source",
		"head", "source",
		"detached", "restored",
		"fork", "save",
	)
	return replacer.Replace(value)
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreInteractive, "interactive", "i", false, "interactive confirmation")
	restoreCmd.Flags().Lookup("interactive").Hidden = true
	restoreCmd.Flags().BoolVar(&restoreDiscardDirty, "discard-unsaved", false, "discard unsaved folder changes for this operation")
	restoreCmd.Flags().BoolVar(&restoreIncludeWorking, "save-first", false, "create a save point for unsaved changes before restore")
	restoreCmd.Flags().StringVar(&restorePath, "path", "", "restore only this workspace-relative path")
	restoreCmd.Flags().StringVar(&restoreRunPlanID, "run", "", "execute a restore preview plan")
	restoreCmd.AddCommand(restoreDiscardCmd)
	rootCmd.AddCommand(restoreCmd)
}
