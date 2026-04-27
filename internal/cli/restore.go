package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/color"
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

		if err := rejectUnsavedChangesForRestore(r.Root, workspaceName); err != nil {
			return restorePointError(err)
		}

		plan, err := restoreplan.Create(r.Root, workspaceName, targetID, detectEngine(r.Root), restoreplan.Options{
			DiscardUnsaved: restoreDiscardDirty,
			SaveFirst:      restoreIncludeWorking,
		})
		if err != nil {
			return restorePointError(err)
		}
		result := publicRestorePreviewFromPlan(plan)
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
	PlanID                  string                         `json:"plan_id"`
	Scope                   string                         `json:"scope,omitempty"`
	Folder                  string                         `json:"folder"`
	Workspace               string                         `json:"workspace"`
	SourceSavePoint         string                         `json:"source_save_point"`
	Path                    string                         `json:"path,omitempty"`
	NewestSavePoint         *string                        `json:"newest_save_point"`
	HistoryHead             *string                        `json:"history_head"`
	ExpectedNewestSavePoint *string                        `json:"expected_newest_save_point"`
	ExpectedFolderEvidence  string                         `json:"expected_folder_evidence,omitempty"`
	ExpectedPathEvidence    string                         `json:"expected_path_evidence,omitempty"`
	ManagedFiles            restoreplan.ManagedFilesImpact `json:"managed_files"`
	Options                 restoreplan.Options            `json:"options,omitempty"`
	HistoryChanged          bool                           `json:"history_changed"`
	FilesChanged            bool                           `json:"files_changed"`
	RunCommand              string                         `json:"run_command"`
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
		{name: "include-working", publicName: "--save-first"},
		{name: "discard-unsaved", publicName: "--discard-unsaved"},
		{name: "discard-dirty", publicName: "--discard-unsaved"},
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

func runRestorePlan(repoRoot, workspaceName, planID string) error {
	var result publicRestoreResult
	var pathResult publicRestorePathResult
	resultScope := restoreplan.ScopeWhole
	protectedRunPath := ""
	err := repo.WithMutationLock(repoRoot, "restore run", func() error {
		plan, err := restoreplan.Load(repoRoot, planID)
		if err != nil {
			return err
		}
		resultScope = plan.EffectiveScope()
		if resultScope == restoreplan.ScopePath {
			protectedRunPath = plan.Path
		}
		switch resultScope {
		case restoreplan.ScopeWhole:
			if err := restoreplan.ValidateTarget(repoRoot, workspaceName, plan); err != nil {
				return err
			}
			if err := restoreplan.ValidateSource(repoRoot, workspaceName, plan, detectEngine(repoRoot)); err != nil {
				return err
			}
			if plan.Options.SaveFirst {
				if _, err := snapshot.NewCreator(repoRoot, detectEngine(repoRoot)).CreateSavePointLocked(workspaceName, "save before restore", nil); err != nil {
					return err
				}
			}
			if err := restore.NewRestorer(repoRoot, detectEngine(repoRoot)).RestoreLocked(workspaceName, plan.SourceSavePoint); err != nil {
				return err
			}
			result, err = publicRestoreStatus(repoRoot, workspaceName, plan.SourceSavePoint)
			if err != nil {
				return err
			}
			result.Mode = "run"
			result.PlanID = plan.PlanID
			result.SourceSavePoint = string(plan.SourceSavePoint)
			result.FilesChanged = true
			return nil
		case restoreplan.ScopePath:
			if err := restoreplan.ValidatePathTarget(repoRoot, workspaceName, plan); err != nil {
				return err
			}
			if err := restoreplan.ValidateSourcePath(repoRoot, workspaceName, plan, detectEngine(repoRoot)); err != nil {
				return err
			}
			if plan.Options.SaveFirst {
				if _, err := snapshot.NewCreator(repoRoot, detectEngine(repoRoot)).CreateSavePointLocked(workspaceName, "save before restore path", nil); err != nil {
					return err
				}
			}
			if err := restore.NewRestorer(repoRoot, detectEngine(repoRoot)).RestorePathLocked(workspaceName, plan.SourceSavePoint, plan.Path); err != nil {
				return err
			}
			pathResult, err = publicRestorePathStatus(repoRoot, workspaceName, plan.Path, plan.SourceSavePoint)
			if err != nil {
				return err
			}
			pathResult.Mode = "run"
			pathResult.PlanID = plan.PlanID
			return nil
		default:
			return fmt.Errorf("restore plan scope is not supported")
		}
	})
	if err != nil {
		if protectedRunPath != "" {
			return restorePathError(err, protectedRunPath)
		}
		return restorePointError(err)
	}
	if jsonOutput {
		if resultScope == restoreplan.ScopePath {
			return outputJSON(pathResult)
		}
		return outputJSON(result)
	}
	if resultScope == restoreplan.ScopePath {
		printRestorePathResult(pathResult)
		return nil
	}
	printRestoreResult(result)
	return nil
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
	if !restoreIncludeWorking && !restoreDiscardDirty {
		pathDirty, err := workspacePathDirty(repoRoot, workspaceName, path)
		if err != nil {
			return restorePathError(err, restorePath, path)
		}
		if pathDirty {
			return restorePathError(fmt.Errorf("path has unsaved changes in %s; use --save-first to save them before restore or --discard-unsaved to discard changes in this path", path), restorePath, path)
		}
	}

	plan, err := restoreplan.CreatePath(repoRoot, workspaceName, targetID, path, detectEngine(repoRoot), restoreplan.Options{
		DiscardUnsaved: restoreDiscardDirty,
		SaveFirst:      restoreIncludeWorking,
	})
	if err != nil {
		return restorePathError(err, restorePath, path)
	}
	result := publicRestorePreviewFromPlan(plan)
	if jsonOutput {
		return outputJSON(result)
	}
	printRestorePreviewResult(result)
	return nil
}

func rejectUnsavedChangesForRestore(repoRoot, workspaceName string) error {
	if restoreDiscardDirty && restoreIncludeWorking {
		return fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
	}
	unsavedChanges, err := workspaceDirty(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if !unsavedChanges || restoreDiscardDirty || restoreIncludeWorking {
		return nil
	}
	return fmt.Errorf("folder has unsaved changes; use --save-first to save them before restore, --discard-unsaved to discard them, or cancel. No files were changed.")
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
	return publicRestorePreviewResult{
		Mode:                    "preview",
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
	if err != nil || path == "" {
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

func genericRestorePathCommand(path string) string {
	if strings.HasPrefix(path, "-") {
		return fmt.Sprintf("jvs restore <save> --path=%s", shellQuoteArg(path))
	}
	return fmt.Sprintf("jvs restore <save> --path %s", shellQuoteArg(path))
}

func ensureRestorePathSourceExists(repoRoot, workspaceName string, savePointID model.SnapshotID, path string) error {
	desc, err := snapshot.LoadDescriptor(repoRoot, savePointID)
	if err != nil {
		return fmt.Errorf("load save point: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return fmt.Errorf("workspace managed boundary: %w", err)
	}
	exists, err := savePointContainsHistoryPath(repoRoot, desc, path, boundary)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("path does not exist in save point: %s", path)
	}
	return nil
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
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Plan: %s\n", result.PlanID)
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
	fmt.Printf("Run: `%s`\n", result.RunCommand)
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

func restorePointError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", restorePointVocabulary(err.Error()))
}

func restorePathError(err error, protectedValues ...string) error {
	if err == nil {
		return nil
	}
	message := restorePathVocabulary(err.Error(), protectedValues...)
	if !strings.Contains(message, "No files were changed.") {
		message += ". No files were changed."
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
		"--discard-dirty", "--discard-unsaved",
		"--include-working", "--save-first",
		"dirty changes", "unsaved changes",
		"dirty", "unsaved",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
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
	restoreCmd.Flags().BoolVar(&restoreDiscardDirty, "discard-dirty", false, "discard dirty workspace changes for this operation")
	restoreCmd.Flags().Lookup("discard-dirty").Hidden = true
	restoreCmd.Flags().BoolVar(&restoreIncludeWorking, "include-working", false, "checkpoint dirty workspace changes before this operation")
	restoreCmd.Flags().Lookup("include-working").Hidden = true
	restoreCmd.Flags().StringVar(&restorePath, "path", "", "restore only this workspace-relative path")
	restoreCmd.Flags().StringVar(&restoreRunPlanID, "run", "", "execute a restore preview plan")
	rootCmd.AddCommand(restoreCmd)
}
