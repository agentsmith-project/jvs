package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	workspaceRemoveForce bool
	workspaceNewFromRef  string
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
	Use:   "new <name> --from <save>",
	Short: "Create a workspace from a save point",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		name := args[0]
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
		err = withActiveOperationSourcePin(r.Root, sourceID, "workspace new", func() error {
			if err := checkWorkspaceNewCapacity(r.Root, name, sourceID); err != nil {
				return err
			}
			var err error
			cfg, err = mgr.CreateStartedFromSnapshot(name, sourceID, func(src, dst string) error {
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
		recordResolvedTarget(r.Root, name)
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
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		list, err := worktree.NewManager(r.Root).List()
		if err != nil {
			return err
		}
		if jsonOutput {
			return outputJSON(publicWorkspaces(list))
		}
		for _, cfg := range list {
			head := string(cfg.HeadSnapshotID)
			if head == "" {
				head = color.Dim("(none)")
			} else if len(head) > 16 {
				head = color.SnapshotID(head[:16]) + color.Dim("...")
			} else {
				head = color.SnapshotID(head)
			}
			fmt.Printf("%-20s  %s\n", cfg.Name, head)
		}
		return nil
	},
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
	Use:   "remove <name>",
	Short: "Remove a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := discoverRequiredRepo()
		if err != nil {
			return err
		}
		name := args[0]
		mgr := worktree.NewManager(r.Root)
		_, err = validateWorkspaceRemoval(r.Root, name, workspaceRemoveForce)
		if err != nil {
			return err
		}
		if err := mgr.Remove(name); err != nil {
			return err
		}
		recordResolvedTarget(r.Root, name)
		if jsonOutput {
			return outputJSON(map[string]string{"workspace": name, "status": "removed"})
		}
		fmt.Printf("Removed workspace '%s'\n", name)
		return nil
	},
}

func validateWorkspaceRemoval(repoRoot, name string, force bool) (*model.WorktreeConfig, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(name)
	if err != nil {
		return nil, missingWorkspaceError(name)
	}
	if force {
		return cfg, nil
	}
	dirty, err := workspaceDirty(repoRoot, name)
	if err != nil {
		return nil, err
	}
	if dirty {
		return nil, fmt.Errorf("workspace %q has unsaved changes; use --force to remove", name)
	}
	if cfg.IsDetached() {
		return nil, workspaceRemoveCurrentDiffersError(name)
	}
	return cfg, nil
}

func workspaceRemoveCurrentDiffersError(name string) error {
	return fmt.Errorf("workspace %q is not at its newest save point; use --force to remove", name)
}

func init() {
	workspaceCmd.SetUsageTemplate(workspacePublicUsageTemplate)
	workspaceNewCmd.Flags().StringVar(&workspaceNewFromRef, "from", "", "save point ID to copy into the new workspace")
	workspaceRemoveCmd.Flags().BoolVarP(&workspaceRemoveForce, "force", "f", false, "force removal when folder files differ from the newest save point")
	workspaceCmd.AddCommand(workspaceNewCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspacePathCmd)
	workspaceCmd.AddCommand(workspaceRenameCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	rootCmd.AddCommand(workspaceCmd)
}
