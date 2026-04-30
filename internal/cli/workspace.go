package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	workspaceRemoveForce bool
	workspaceRemoveRunID string
	workspaceNewFromRef  string
	workspaceNewName     string
	workspaceListStatus  bool
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
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
		eng := newCloneEngine(r.Root)
		var cfg *model.WorktreeConfig
		req := worktree.StartedFromSnapshotRequest{
			Name:       name,
			Folder:     folder,
			SnapshotID: sourceID,
		}
		err = withActiveOperationSourcePin(r.Root, sourceID, "workspace new", func() error {
			if err := checkWorkspaceNewCapacity(r.Root, req); err != nil {
				return err
			}
			var err error
			cfg, err = mgr.CreateStartedFromSnapshotAt(req, func(src, dst string) error {
				_, err := engine.CloneToNew(eng, src, dst)
				return err
			})
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
		result := publicWorkspaceNewResult{
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
			return outputJSON(records)
		}
		printWorkspaceList(records, workspaceListStatus)
		return nil
	},
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		name := ""
		if len(args) > 0 {
			var err error
			name, err = resolveNamedWorkspace(r.Root, args[0])
			if err != nil {
				return err
			}
		} else {
			_, current, err := discoverRequiredWorktree()
			if err != nil {
				return err
			}
			name = current
		}

		mgr := worktree.NewManager(r.Root)
		path, err := mgr.Path(name)
		if err != nil {
			return err
		}
		recordResolvedTarget(r.Root, name)
		if jsonOutput {
			return outputJSON(map[string]string{"workspace": name, "path": path})
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		oldName, err := resolveNamedWorkspace(r.Root, args[0])
		if err != nil {
			return err
		}
		if err := validatePublicWorkspaceName(args[1]); err != nil {
			return err
		}
		if err := worktree.NewManager(r.Root).Rename(oldName, args[1]); err != nil {
			return err
		}
		recordResolvedTarget(r.Root, args[1])
		if jsonOutput {
			return outputJSON(map[string]string{"old_workspace": oldName, "workspace": args[1], "status": "renamed"})
		}
		fmt.Printf("Renamed workspace '%s' to '%s'\n", oldName, args[1])
		return nil
	},
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove <name> | remove --run <plan-id>",
	Short: "Remove a workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}

		runPlanID, runRequested, err := workspaceRemoveRunFlag(cmd)
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("workspace remove --run accepts only a plan ID")
			}
			if workspaceRemoveForceFlagChanged(cmd) {
				return fmt.Errorf("workspace remove --run options are fixed by the preview plan; run preview again to change --force. No workspace was removed.")
			}
			result, err := executeWorkspaceRemovePlan(r.Root, runPlanID)
			if err != nil {
				return err
			}
			recordResolvedTarget(r.Root, result.Workspace)
			if jsonOutput {
				return outputJSON(result)
			}
			printWorkspaceRemoveRunResult(result)
			return nil
		}

		if len(args) != 1 {
			return fmt.Errorf("workspace name is required")
		}
		plan, err := createWorkspaceRemovePlan(r.Root, args[0], workspaceRemoveForce)
		if err != nil {
			return err
		}
		recordResolvedTarget(r.Root, plan.Workspace)
		result := publicWorkspaceRemovePreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printWorkspaceRemovePreviewResult(result)
		return nil
	},
}

func validateWorkspaceRemoval(repoRoot, name string, force bool) (*model.WorktreeConfig, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(name)
	if err != nil {
		return nil, missingWorkspaceError(name)
	}
	if cfg.Name == "main" {
		return nil, fmt.Errorf("cannot remove main workspace")
	}
	if force {
		return cfg, nil
	}
	dirty, err := workspaceDirty(repoRoot, name)
	if err != nil {
		return nil, err
	}
	if dirty {
		return nil, fmt.Errorf("workspace %q has unsaved changes; use --force to preview a remove plan that discards them", name)
	}
	if cfg.IsDetached() {
		return nil, workspaceRemoveCurrentDiffersError(name)
	}
	return cfg, nil
}

func workspaceRemoveCurrentDiffersError(name string) error {
	return fmt.Errorf("workspace %q is not at its newest save point; use --force to preview a remove plan that discards it", name)
}

func workspaceRemoveRunFlag(cmd *cobra.Command) (string, bool, error) {
	flag := cmd.Flags().Lookup("run")
	if flag == nil || !flag.Changed {
		return "", false, nil
	}
	planID := strings.TrimSpace(flag.Value.String())
	if planID == "" {
		return "", true, fmt.Errorf("--run requires a workspace remove plan ID")
	}
	return planID, true, nil
}

func workspaceRemoveForceFlagChanged(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("force")
	return flag != nil && flag.Changed
}

func printWorkspaceRemovePreviewResult(result publicWorkspaceRemovePreviewResult) {
	fmt.Println("Preview only. No workspace folder was removed.")
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
	if result.Options.RemovesUnsavedWork {
		fmt.Println("Run will discard unsaved changes.")
	}
	fmt.Println("Workspace folder will be removed.")
	fmt.Println("Workspace metadata will be removed.")
	fmt.Println("Save point storage will not be removed.")
	fmt.Printf("Cleanup: %s\n", result.CleanupPreviewRun)
	fmt.Printf("Run: `%s`\n", result.RunCommand)
}

func printWorkspaceRemoveRunResult(result publicWorkspaceRemoveRunResult) {
	fmt.Printf("Removed workspace '%s'\n", result.Workspace)
	fmt.Println("Workspace folder removed: yes")
	fmt.Println("Workspace metadata removed: yes")
	fmt.Println("Save point storage removed: no")
	fmt.Printf("Cleanup: %s\n", result.CleanupPreviewRun)
}

func init() {
	workspaceCmd.SetUsageTemplate(workspacePublicUsageTemplate)
	workspaceNewCmd.Flags().StringVar(&workspaceNewFromRef, "from", "", "save point ID to copy into the new workspace")
	workspaceNewCmd.Flags().StringVar(&workspaceNewName, "name", "", "workspace name (defaults to the folder name)")
	workspaceListCmd.Flags().BoolVar(&workspaceListStatus, "status", false, "include unsaved change status")
	workspaceRemoveCmd.Flags().BoolVarP(&workspaceRemoveForce, "force", "f", false, "preview removal even when folder files differ from the newest save point")
	workspaceRemoveCmd.Flags().StringVar(&workspaceRemoveRunID, "run", "", "execute a workspace remove preview plan")
	workspaceCmd.AddCommand(workspaceNewCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspacePathCmd)
	workspaceCmd.AddCommand(workspaceRenameCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	rootCmd.AddCommand(workspaceCmd)
}
