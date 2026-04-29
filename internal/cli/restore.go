package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
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
  jvs restore --path src/config.json
  jvs restore 1771589abc --path src/config.json
  jvs restore 1771589abc --save-first
  jvs restore 1771589abc --discard-unsaved`,
	Args: validateRestoreArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		if restorePathFlagChanged(cmd) {
			return runRestorePath(cmd, args, r.Root, workspaceName)
		}

		if restoreRunFlagChanged(cmd) {
			return runRestorePlan(r.Root, workspaceName, restoreRunPlanID)
		}

		targetID, err := resolvePublicSavePointID(r.Root, args[0])
		if err != nil {
			return restorePointError(err)
		}

		var plan *restoreplan.Plan
		var decisionReason string
		err = withActiveOperationSourcePin(r.Root, targetID, "restore preview", func() error {
			if restoreDiscardDirty && restoreIncludeWorking {
				return fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
			}
			engineType := detectEngine(r.Root)
			if !restoreDiscardDirty && !restoreIncludeWorking {
				if err := checkRestorePreviewPreDirtyCapacity(r.Root, workspaceName, targetID, ""); err != nil {
					return err
				}
				unsavedChanges, err := workspaceDirty(r.Root, workspaceName)
				if err != nil {
					return err
				}
				if unsavedChanges {
					decisionReason = "folder has unsaved changes"
					var err error
					plan, err = restoreplan.CreateDecisionPreview(r.Root, workspaceName, targetID, engineType)
					return err
				}
			}
			var err error
			plan, err = restoreplan.Create(r.Root, workspaceName, targetID, engineType, restoreplan.Options{
				DiscardUnsaved: restoreDiscardDirty,
				SaveFirst:      restoreIncludeWorking,
			})
			return err
		})
		if err != nil {
			return restorePointError(err)
		}
		result := publicRestorePreviewFromPlan(plan)
		if decisionReason != "" {
			result.DecisionReason = decisionReason
			result.NextCommands = restoreDecisionNextCommands(targetID, "")
		}
		if jsonOutput {
			return outputJSON(result)
		}

		printRestorePreviewResult(result)
		return nil
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
	Mode              string  `json:"mode,omitempty"`
	PlanID            string  `json:"plan_id,omitempty"`
	Folder            string  `json:"folder"`
	Workspace         string  `json:"workspace"`
	RestoredSavePoint string  `json:"restored_save_point"`
	SourceSavePoint   string  `json:"source_save_point,omitempty"`
	NewestSavePoint   *string `json:"newest_save_point"`
	HistoryHead       *string `json:"history_head"`
	ContentSource     *string `json:"content_source"`
	UnsavedChanges    bool    `json:"unsaved_changes"`
	FilesState        string  `json:"files_state"`
	HistoryChanged    bool    `json:"history_changed"`
	FilesChanged      bool    `json:"files_changed"`
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
	Scope         string
	ProtectedPath string
	FilesChanged  bool
	Whole         publicRestoreResult
	Path          publicRestorePathResult
}

type restoreRunScopeOps struct {
	SaveFirstMessage string
	ValidateTarget   func() error
	ValidateSource   func(model.EngineType) error
	ApplyRestore     func(*recovery.Plan, model.EngineType) error
	RecordResult     func() error
}

func runRestorePlan(repoRoot, workspaceName, planID string) error {
	result, err := executeRestorePlanRun(repoRoot, workspaceName, planID)
	if err != nil {
		err = restoreRunErrorWithMutationState(err, result)
		if result.ProtectedPath != "" {
			return restorePathErrorWithFilesChanged(err, result.FilesChanged, result.ProtectedPath)
		}
		return restorePointError(err)
	}
	return outputRestoreRunResult(result)
}

func executeRestorePlanRun(repoRoot, workspaceName, planID string) (restoreRunResult, error) {
	result := restoreRunResult{Scope: restoreplan.ScopeWhole}
	err := repo.WithMutationLock(repoRoot, "restore run", func() error {
		plan, err := restoreplan.Load(repoRoot, planID)
		if err != nil {
			return err
		}
		if !plan.IsRunnable() {
			return fmt.Errorf("restore preview requires a safety decision; rerun preview with --save-first or --discard-unsaved. No files were changed.")
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
			return runLoadedRestorePlan(repoRoot, workspaceName, plan, &result)
		})
	})
	return result, err
}

func runLoadedRestorePlan(repoRoot, workspaceName string, plan *restoreplan.Plan, result *restoreRunResult) error {
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
	engineType := detectEngine(repoRoot)
	if err := ops.ValidateSource(engineType); err != nil {
		return err
	}
	if err := createRestoreSafetySave(repoRoot, workspaceName, engineType, plan.Options.SaveFirst, ops.SaveFirstMessage); err != nil {
		return err
	}
	recoveryPlan, err := recovery.NewManager(repoRoot).CreateActiveForRestore(plan, restoreRecoveryBackupPath(plan.Folder))
	if err != nil {
		return err
	}
	if err := ops.ApplyRestore(recoveryPlan, engineType); err != nil {
		result.FilesChanged = restoreApplyErrorChangedFiles(err)
		return handleRestoreApplyError(repoRoot, recoveryPlan, err)
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
			ValidateSource: func(engineType model.EngineType) error {
				return restoreplan.ValidateSource(repoRoot, workspaceName, plan, engineType)
			},
			ApplyRestore: func(recoveryPlan *recovery.Plan, engineType model.EngineType) error {
				return restore.NewRestorer(repoRoot, engineType).RestoreLockedWithOptions(workspaceName, plan.SourceSavePoint, restore.RunOptions{BackupPath: recoveryPlan.Backup.Path})
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
			ValidateSource: func(engineType model.EngineType) error {
				return restoreplan.ValidateSourcePath(repoRoot, workspaceName, plan, engineType)
			},
			ApplyRestore: func(recoveryPlan *recovery.Plan, engineType model.EngineType) error {
				return restore.NewRestorer(repoRoot, engineType).RestorePathLockedWithOptions(workspaceName, plan.SourceSavePoint, plan.Path, restore.RunOptions{BackupPath: recoveryPlan.Backup.Path})
			},
			RecordResult: func() error {
				status, err := publicRestorePathStatus(repoRoot, workspaceName, plan.Path, plan.SourceSavePoint)
				if err != nil {
					return err
				}
				status.Mode = "run"
				status.PlanID = plan.PlanID
				result.Path = status
				return nil
			},
		}, nil
	default:
		return restoreRunScopeOps{}, fmt.Errorf("restore plan scope is not supported")
	}
}

func createRestoreSafetySave(repoRoot, workspaceName string, engineType model.EngineType, saveFirst bool, message string) error {
	if !saveFirst {
		return nil
	}
	_, err := snapshot.NewCreator(repoRoot, engineType).CreateSavePointLocked(workspaceName, message, nil)
	return err
}

func handleRestoreApplyError(repoRoot string, recoveryPlan *recovery.Plan, restoreErr error) error {
	if _, ok := restore.AsIncompleteError(restoreErr); ok {
		return keepRecoveryPlanActiveAfterRestoreFailure(repoRoot, recoveryPlan, restoreErr)
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

func outputRestoreRunResult(result restoreRunResult) error {
	if jsonOutput {
		if result.Scope == restoreplan.ScopePath {
			return outputJSON(result.Path)
		}
		return outputJSON(result.Whole)
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
	err error
}

func (e *restoreRunFilesChangedError) Error() string {
	return fmt.Sprintf("%v. Files were changed; run jvs recovery status before continuing", e.err)
}

func (e *restoreRunFilesChangedError) Unwrap() error {
	return e.err
}

func restoreRunErrorWithMutationState(err error, result restoreRunResult) error {
	if err == nil || !result.FilesChanged {
		return err
	}
	return &restoreRunFilesChangedError{err: err}
}

func runRestorePath(cmd *cobra.Command, args []string, repoRoot, workspaceName string) error {
	if len(args) == 0 {
		path, err := normalizeRestorePathFlag(repoRoot, workspaceName, restorePath)
		if err != nil {
			return restorePathError(err, restorePath)
		}
		result, err := restorePathCandidates(repoRoot, workspaceName, path)
		if err != nil {
			return restorePathError(err, path)
		}
		if jsonOutput {
			return outputJSON(result)
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
		engineType := detectEngine(repoRoot)
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
				plan, err = restoreplan.CreatePathDecisionPreview(repoRoot, workspaceName, targetID, path, engineType)
				return err
			}
		}
		var err error
		plan, err = restoreplan.CreatePath(repoRoot, workspaceName, targetID, path, engineType, restoreplan.Options{
			DiscardUnsaved: restoreDiscardDirty,
			SaveFirst:      restoreIncludeWorking,
		})
		return err
	})
	if err != nil {
		return restorePathError(err, restorePath, path)
	}
	result := publicRestorePreviewFromPlan(plan)
	if decisionReason != "" {
		result.DecisionReason = decisionReason
		result.NextCommands = restoreDecisionNextCommands(targetID, path)
	}
	if jsonOutput {
		return outputJSON(result)
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

func restorePathCandidates(repoRoot, workspaceName, path string) (publicRestorePathCandidatesResult, error) {
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
		NextCommands: restorePathNextCommands(path, historyResult.Candidates),
		FilesChanged: false,
	}, nil
}

func restorePathNextCommands(path string, candidates []publicHistoryPathCandidate) []string {
	if len(candidates) == 0 {
		return []string{}
	}
	return []string{genericRestorePathCommand(path)}
}

func restoreDecisionNextCommands(sourceID model.SnapshotID, path string) []string {
	return []string{
		restoreDecisionCommand(sourceID, path, "--save-first"),
		restoreDecisionCommand(sourceID, path, "--discard-unsaved"),
	}
}

func restoreDecisionCommand(sourceID model.SnapshotID, path, option string) string {
	source := shellQuoteArg(string(sourceID))
	if path == "" {
		return fmt.Sprintf("jvs restore %s %s", source, option)
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Sprintf("jvs restore %s --path=%s %s", source, shellQuoteArg(path), option)
	}
	return fmt.Sprintf("jvs restore %s --path %s %s", source, shellQuoteArg(path), option)
}

func genericRestorePathCommand(path string) string {
	if strings.HasPrefix(path, "-") {
		return fmt.Sprintf("jvs restore <save> --path=%s", shellQuoteArg(path))
	}
	return fmt.Sprintf("jvs restore <save> --path %s", shellQuoteArg(path))
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
	printRestorePreviewImpact("overwrite", result.ManagedFiles.Overwrite)
	printRestorePreviewImpact("delete", result.ManagedFiles.Delete)
	printRestorePreviewImpact("create", result.ManagedFiles.Create)
	printRestorePreviewOptions(result.Options)
	fmt.Println("Ignored/unmanaged files will be kept.")
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
		commands = []string{genericRestorePathCommand(result.Path)}
	}
	for _, command := range commands {
		fmt.Printf("  %s\n", command)
	}
	fmt.Println("No files were changed.")
}

func printRestorePathResult(result publicRestorePathResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	if result.PlanID != "" {
		fmt.Printf("Plan: %s\n", result.PlanID)
	}
	fmt.Printf("Restored path: %s\n", result.RestoredPath)
	fmt.Printf("From save point: %s\n", color.SnapshotID(result.FromSavePoint))
	newest := formatStatusSavePoint(result.NewestSavePoint)
	fmt.Printf("Newest save point is still %s.\n", newest)
	fmt.Println("History was not changed.")
	fmt.Printf("Next save creates a new save point after %s and records this restored path.\n", newest)
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
	if err == nil {
		return nil
	}
	message := restorePointVocabulary(err.Error())
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: restorePointVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

func restorePathError(err error, protectedValues ...string) error {
	return restorePathErrorWithFilesChanged(err, false, protectedValues...)
}

func restorePathErrorWithFilesChanged(err error, filesChanged bool, protectedValues ...string) error {
	if err == nil {
		return nil
	}
	message := restorePathVocabulary(err.Error(), protectedValues...)
	if !filesChanged && !strings.Contains(message, "No files were changed.") {
		message += ". No files were changed."
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: restorePathVocabulary(jvsErr.Hint, protectedValues...)}
	}
	return fmt.Errorf("%s", message)
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
	rootCmd.AddCommand(restoreCmd)
}
