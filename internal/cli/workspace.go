package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	workspaceRenameDryRun bool
	workspaceDeleteRunID  string
	workspaceMoveRunID    string
	workspaceNewFromRef   string
	workspaceNewName      string
	workspaceListStatus   bool
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return cmd.Help()
	},
}

const workspacePublicUsageTemplate = `Usage:
  {{.CommandPath}} [command]

Available Commands:
{{- range .Commands}}{{if .IsAvailableCommand}}
 {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}

Use "{{.CommandPath}} [command] --help" for more information about a command.
`

var workspaceNewCmd = &cobra.Command{
	Use:   "new <folder> --from <save>",
	Short: "Create a workspace from a save point",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "workspace new"); err != nil {
			return err
		}
		r := ctx.Repo
		folder, err := filepath.Abs(args[0])
		if err != nil {
			return workspaceNewError(fmt.Errorf("resolve workspace folder: %w", err))
		}
		folder = filepath.Clean(folder)
		name := workspaceNewName
		if name == "" {
			name = filepath.Base(folder)
		}
		if err := validatePublicWorkspaceName(name); err != nil {
			return workspaceNewError(err)
		}
		if workspaceNewFromRef == "" {
			return workspaceNewError(fmt.Errorf("--from save point ID is required"))
		}
		sourceID, err := resolvePublicSavePointID(r.Root, workspaceNewFromRef)
		if err != nil {
			return workspaceNewError(err)
		}

		mgr := worktree.NewManager(r.Root)
		var cfg *model.WorktreeConfig
		req := worktree.StartedFromSnapshotRequest{
			Name:            name,
			Folder:          folder,
			SnapshotID:      sourceID,
			RequestedEngine: requestedTransferEngine(r.Root),
		}
		err = withActiveOperationSourcePin(r.Root, sourceID, "workspace new", func() error {
			if err := checkWorkspaceNewCapacity(r.Root, req); err != nil {
				return err
			}
			var err error
			cfg, err = mgr.CreateStartedFromSnapshotAt(req, nil)
			return err
		})
		if err != nil {
			return workspaceNewError(err)
		}

		path, err := mgr.Path(name)
		if err != nil {
			return workspaceNewError(err)
		}
		recordResolvedTarget(r.Root, cfg.Name)
		var transferRecord *transfer.Record
		if record, ok := mgr.LastTransferRecord(); ok {
			transferRecord = &record
		}
		result := publicWorkspaceNewResult{
			Data:                 transferDataFromRecord(transferRecord),
			Mode:                 "new",
			Status:               "created",
			Workspace:            cfg.Name,
			Folder:               path,
			StartedFromSavePoint: string(sourceID),
			ContentSource:        string(cfg.HeadSnapshotID),
			OriginalUnchanged:    true,
			UnsavedChanges:       false,
		}
		if jsonOutput {
			return outputJSON(result)
		}
		printWorkspaceNewResult(result)
		return nil
	},
}

type publicWorkspaceNewResult struct {
	transfer.Data
	Mode                 string  `json:"mode"`
	Status               string  `json:"status"`
	Workspace            string  `json:"workspace"`
	Folder               string  `json:"folder"`
	StartedFromSavePoint string  `json:"started_from_save_point"`
	ContentSource        string  `json:"content_source"`
	NewestSavePoint      *string `json:"newest_save_point"`
	HistoryHead          *string `json:"history_head"`
	OriginalUnchanged    bool    `json:"original_workspace_unchanged"`
	UnsavedChanges       bool    `json:"unsaved_changes"`
}

func printWorkspaceNewResult(result publicWorkspaceNewResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Started from save point: %s\n", color.SnapshotID(result.StartedFromSavePoint))
	if len(result.Transfers) > 0 {
		printPrimaryTransferSummary(&result.Transfers[0])
	}
	fmt.Println("Newest save point: none")
	if result.OriginalUnchanged {
		fmt.Println("Original workspace unchanged.")
	}
	fmt.Println("Unsaved changes: no")
}

func workspaceNewError(err error) error {
	if err == nil {
		return nil
	}
	message := publicSavePointVocabulary(err.Error())
	if strings.Contains(message, "workspace real path overlap") {
		message = "workspace folder is inside an existing workspace or overlaps one"
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: publicSavePointVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		records, err := buildWorkspaceListRecords(ctx.Repo.Root, ctx.Workspace, workspaceListStatus)
		if err != nil {
			return err
		}
		if jsonOutput {
			data := any(records)
			if ctx.Separated != nil {
				data = publicWorkspaceListResult{Workspaces: records}
			}
			return outputJSONWithSeparatedControl(data, ctx.Separated, separatedDoctorStrictNotRun)
		}
		printWorkspaceList(records, workspaceListStatus)
		return nil
	},
}

type publicWorkspaceListResult struct {
	Workspaces []publicWorkspaceListRecord `json:"workspaces"`
}

type publicWorkspaceListRecord struct {
	Current              bool      `json:"current"`
	Workspace            string    `json:"workspace"`
	Folder               string    `json:"folder"`
	ContentSource        *string   `json:"content_source"`
	NewestSavePoint      *string   `json:"newest_save_point"`
	HistoryHead          *string   `json:"history_head"`
	StartedFromSavePoint *string   `json:"started_from_save_point,omitempty"`
	UnsavedChanges       *bool     `json:"unsaved_changes,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

func buildWorkspaceListRecords(repoRoot, currentWorkspace string, includeStatus bool) ([]publicWorkspaceListRecord, error) {
	mgr := worktree.NewManager(repoRoot)
	list, err := mgr.List()
	if err != nil {
		return nil, err
	}
	records := make([]publicWorkspaceListRecord, 0, len(list))
	for _, cfg := range list {
		folder, err := mgr.Path(cfg.Name)
		if err != nil {
			return nil, err
		}
		historyHead := statusStringPointer(cfg.LatestSnapshotID)
		record := publicWorkspaceListRecord{
			Current:              cfg.Name == currentWorkspace,
			Workspace:            cfg.Name,
			Folder:               folder,
			ContentSource:        statusStringPointer(cfg.HeadSnapshotID),
			NewestSavePoint:      historyHead,
			HistoryHead:          historyHead,
			StartedFromSavePoint: statusStringPointer(cfg.StartedFromSnapshotID),
			CreatedAt:            cfg.CreatedAt,
		}
		if includeStatus {
			dirty, err := workspaceDirty(repoRoot, cfg.Name)
			if err != nil {
				return nil, err
			}
			record.UnsavedChanges = &dirty
		}
		records = append(records, record)
	}
	return records, nil
}

func printWorkspaceList(records []publicWorkspaceListRecord, includeStatus bool) {
	for i, record := range records {
		if i > 0 {
			fmt.Println()
		}
		marker := " "
		if record.Current {
			marker = "*"
		}
		fmt.Printf("%s Workspace: %s\n", marker, record.Workspace)
		fmt.Printf("  Folder: %s\n", record.Folder)
		fmt.Printf("  Content source: %s\n", formatStatusSavePoint(record.ContentSource))
		fmt.Printf("  Newest save point: %s\n", formatStatusSavePoint(record.NewestSavePoint))
		fmt.Printf("  History head: %s\n", formatStatusSavePoint(record.HistoryHead))
		if record.StartedFromSavePoint != nil {
			fmt.Printf("  Started from save point: %s\n", formatStatusSavePoint(record.StartedFromSavePoint))
		}
		if includeStatus {
			if record.UnsavedChanges != nil && *record.UnsavedChanges {
				fmt.Println("  Unsaved changes: yes")
			} else {
				fmt.Println("  Unsaved changes: no")
			}
		}
	}
}

var workspacePathCmd = &cobra.Command{
	Use:   "path [<name>]",
	Short: "Print a workspace path",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		name := ""
		if len(args) > 0 {
			var err error
			name, err = resolveNamedWorkspace(ctx.Repo.Root, args[0])
			if err != nil {
				return err
			}
		} else if ctx.Workspace != "" {
			name = ctx.Workspace
		} else {
			_, current, err := discoverRequiredWorktree()
			if err != nil {
				return err
			}
			name = current
		}

		mgr := worktree.NewManager(ctx.Repo.Root)
		path, err := mgr.Path(name)
		if err != nil {
			return err
		}
		recordResolvedTarget(ctx.Repo.Root, name)
		if jsonOutput {
			return outputJSONWithSeparatedControl(map[string]string{"workspace": name, "path": path}, ctx.Separated, separatedDoctorStrictNotRun)
		}
		fmt.Println(path)
		return nil
	},
}

var workspaceRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a workspace",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "workspace rename"); err != nil {
			return err
		}
		if err := validatePublicWorkspaceName(args[0]); err != nil {
			return err
		}
		if err := validatePublicWorkspaceName(args[1]); err != nil {
			return err
		}
		r := ctx.Repo
		result, err := executeWorkspaceRename(r.Root, args[0], args[1], workspaceRenameDryRun)
		if err != nil {
			return err
		}
		recordResolvedTarget(r.Root, result.Workspace)
		if jsonOutput {
			return outputJSON(result)
		}
		printWorkspaceRenameResult(result)
		return nil
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <name> | delete --run <plan-id>",
	Short: "Delete a workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "workspace delete"); err != nil {
			return err
		}
		r := ctx.Repo

		runPlanID, runRequested, err := workspaceRunFlag(cmd, "workspace delete")
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("workspace delete --run accepts only a plan ID")
			}
			result, err := executeWorkspaceDeletePlan(r.Root, runPlanID)
			if err != nil {
				return err
			}
			recordResolvedTarget(r.Root, result.Workspace)
			if jsonOutput {
				return outputJSON(result)
			}
			printWorkspaceDeleteRunResult(result)
			return nil
		}

		if len(args) != 1 {
			return fmt.Errorf("workspace name is required")
		}
		plan, err := createWorkspaceDeletePlan(r.Root, args[0])
		if err != nil {
			return err
		}
		recordResolvedTarget(r.Root, plan.Workspace)
		result := publicWorkspaceDeletePreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printWorkspaceDeletePreviewResult(result)
		return nil
	},
}

var workspaceMoveCmd = &cobra.Command{
	Use:   "move <name> <new-folder> | move --run <plan-id>",
	Short: "Move a workspace folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "workspace move"); err != nil {
			return err
		}
		r := ctx.Repo
		runPlanID, runRequested, err := workspaceRunFlag(cmd, "workspace move")
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("workspace move --run accepts only a plan ID")
			}
			result, err := executeWorkspaceMovePlan(r.Root, runPlanID)
			if err != nil {
				return err
			}
			recordResolvedTarget(r.Root, result.Workspace)
			if jsonOutput {
				return outputJSON(result)
			}
			printWorkspaceMoveRunResult(result)
			return nil
		}
		if len(args) != 2 {
			return fmt.Errorf("workspace name and new folder are required")
		}
		plan, err := createWorkspaceMovePlan(r.Root, args[0], args[1])
		if err != nil {
			return err
		}
		recordResolvedTarget(r.Root, plan.Workspace)
		result := publicWorkspaceMovePreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printWorkspaceMovePreviewResult(result)
		return nil
	},
}

func validateWorkspaceDeletion(repoRoot, name string) (*model.WorktreeConfig, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(name)
	if err != nil {
		return nil, missingWorkspaceError(name)
	}
	if cfg.Name == "main" {
		return nil, fmt.Errorf("cannot delete main workspace")
	}
	dirty, err := workspaceDirty(repoRoot, name)
	if err != nil {
		return nil, err
	}
	if dirty {
		return nil, fmt.Errorf("workspace %q has unsaved changes; save or restore changes before deleting it", name)
	}
	if cfg.IsDetached() {
		return nil, workspaceDeleteCurrentDiffersError(name)
	}
	return cfg, nil
}

func workspaceDeleteCurrentDiffersError(name string) error {
	return fmt.Errorf("workspace %q is not at its newest save point; save or restore changes before deleting it", name)
}

func workspaceRunFlag(cmd *cobra.Command, command string) (string, bool, error) {
	flag := cmd.Flags().Lookup("run")
	if flag == nil || !flag.Changed {
		return "", false, nil
	}
	planID := strings.TrimSpace(flag.Value.String())
	if planID == "" {
		return "", true, fmt.Errorf("--run requires a %s plan ID", command)
	}
	return planID, true, nil
}

func printWorkspaceRenameResult(result publicWorkspaceRenameResult) {
	if result.Mode == "dry-run" {
		fmt.Println("Preview only. No workspace metadata was changed.")
		fmt.Printf("Workspace: %s -> %s\n", result.OldWorkspace, result.Workspace)
		fmt.Printf("Folder: %s\n", result.Folder)
		return
	}
	fmt.Printf("Renamed workspace %q to %q\n", result.OldWorkspace, result.Workspace)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Println("Workspace folder moved: no")
}

func printWorkspaceDeletePreviewResult(result publicWorkspaceDeletePreviewResult) {
	fmt.Println("Preview only. No workspace folder was deleted.")
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Plan: %s\n", result.PlanID)
	fmt.Printf("Files state: %s\n", result.FilesState)
	fmt.Printf("Newest save point: %s\n", formatStatusSavePoint(result.NewestSavePoint))
	fmt.Printf("Content source: %s\n", formatStatusSavePoint(result.ContentSource))
	if result.UnsavedChanges {
		fmt.Println("Unsaved changes: yes")
	} else {
		fmt.Println("Unsaved changes: no")
	}
	fmt.Println("Workspace folder will be deleted.")
	fmt.Println("Workspace metadata will be deleted.")
	fmt.Println("Save point storage will not be deleted.")
	fmt.Printf("Cleanup: %s\n", result.CleanupPreviewRun)
	fmt.Printf("Run: `%s`\n", result.RunCommand)
	fmt.Printf("Safe run: `%s`\n", result.SafeRunCommand)
}

func printWorkspaceDeleteRunResult(result publicWorkspaceDeleteRunResult) {
	fmt.Printf("Deleted workspace %q\n", result.Workspace)
	fmt.Println("Workspace folder deleted: yes")
	fmt.Println("Workspace metadata deleted: yes")
	fmt.Println("Save point storage deleted: no")
	fmt.Printf("Cleanup: %s\n", result.CleanupPreviewRun)
}

func printWorkspaceMovePreviewResult(result publicWorkspaceMovePreviewResult) {
	fmt.Println("Workspace move preview")
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Old folder: %s\n", result.SourceFolder)
	fmt.Printf("New folder: %s\n", result.TargetFolder)
	fmt.Println("Workspace name: unchanged")
	fmt.Printf("Move method: %s\n", result.MoveMethod)
	fmt.Printf("Plan: %s\n", result.PlanID)
	fmt.Println("No files were moved.")
	fmt.Printf("Run: `%s`\n", result.RunCommand)
	fmt.Printf("Safe run: `%s`\n", result.SafeRunCommand)
}

func printWorkspaceMoveRunResult(result publicWorkspaceMoveRunResult) {
	fmt.Printf("Moved workspace %q\n", result.Workspace)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Println("Workspace name changed: no")
}

func init() {
	workspaceCmd.SetUsageTemplate(workspacePublicUsageTemplate)
	workspaceNewCmd.Flags().StringVar(&workspaceNewFromRef, "from", "", "save point ID to copy into the new workspace")
	workspaceNewCmd.Flags().StringVar(&workspaceNewName, "name", "", "workspace name (defaults to the folder name)")
	workspaceListCmd.Flags().BoolVar(&workspaceListStatus, "status", false, "include unsaved change status")
	workspaceRenameCmd.Flags().BoolVar(&workspaceRenameDryRun, "dry-run", false, "preview the workspace rename without changing metadata")
	workspaceDeleteCmd.Flags().StringVar(&workspaceDeleteRunID, "run", "", "execute a workspace delete preview plan")
	workspaceMoveCmd.Flags().StringVar(&workspaceMoveRunID, "run", "", "execute a workspace move preview plan")
	workspaceCmd.AddCommand(workspaceNewCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspacePathCmd)
	workspaceCmd.AddCommand(workspaceRenameCmd)
	workspaceCmd.AddCommand(workspaceMoveCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	rootCmd.AddCommand(workspaceCmd)
}
