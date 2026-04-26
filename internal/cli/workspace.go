package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/model"
)

var (
	workspaceRemoveForce bool
	forkFromRef          string
	forkDiscardDirty     bool
	forkIncludeWorking   bool
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
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
		return nil, fmt.Errorf("workspace %q has dirty changes; run jvs checkpoint or use --force to remove", name)
	}
	if cfg.IsDetached() {
		return nil, workspaceRemoveCurrentDiffersError(name)
	}
	return cfg, nil
}

func workspaceRemoveCurrentDiffersError(name string) error {
	return fmt.Errorf("workspace %q current differs from latest; use --force to remove\n\nTo continue from the current checkpoint, use: jvs fork <new-workspace-name>\nTo return this workspace to latest, use: jvs restore latest\nTo remove anyway, use: jvs workspace remove --force %s", name, name)
}

var forkCmd = &cobra.Command{
	Use:   "fork [<ref> <name>|<name>]",
	Short: "Create a workspace from a checkpoint",
	Args:  cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		targetRef, name, defaultCurrent, err := parseForkArgs(r.Root, workspaceName, args)
		if err != nil {
			return err
		}

		var targetID model.SnapshotID
		if forkIncludeWorking && !defaultCurrent {
			targetID, err = resolveCheckpointRef(r.Root, workspaceName, targetRef)
			if err != nil {
				return err
			}
		}

		if err := rejectDirtyWorkspace(r.Root, workspaceName, "fork", forkDiscardDirty, forkIncludeWorking); err != nil {
			return err
		}

		if forkIncludeWorking {
			desc, err := createCheckpointDescriptor(r.Root, workspaceName, checkpointCreateOptions{
				Note: "include working before fork",
			})
			if err != nil {
				return err
			}
			if defaultCurrent {
				targetID = desc.SnapshotID
			}
		}

		if targetID == "" {
			targetID, err = resolveCheckpointRef(r.Root, workspaceName, targetRef)
			if err != nil {
				return err
			}
		}

		if name == "" {
			name = fmt.Sprintf("workspace-%s", targetID.ShortID())
		}

		if err := verifySnapshotStrong(r.Root, targetID); err != nil {
			return fmt.Errorf("verify checkpoint: %w", err)
		}

		mgr := worktree.NewManager(r.Root)
		eng := newCloneEngine(r.Root)
		cfg, err := mgr.Fork(targetID, name, func(src, dst string) error {
			_, err := eng.Clone(src, dst)
			return err
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(publicWorkspace(cfg))
		}

		path, err := mgr.Path(name)
		if err != nil {
			return err
		}
		fmt.Printf("Created workspace '%s' from checkpoint %s\n", color.Success(name), color.SnapshotID(targetID.String()))
		fmt.Printf("Path: %s\n", color.Dim(path))
		return nil
	},
}

func parseForkArgs(repoRoot, workspaceName string, args []string) (targetRef string, name string, defaultCurrent bool, err error) {
	if forkFromRef != "" {
		if len(args) != 1 {
			return "", "", false, fmt.Errorf("fork --from requires exactly one workspace name")
		}
		if err := validatePublicWorkspaceName(args[0]); err != nil {
			return "", "", false, err
		}
		return forkFromRef, args[0], false, nil
	}

	switch len(args) {
	case 0:
		return "current", "", true, nil
	case 1:
		if err := validatePublicWorkspaceName(args[0]); err != nil {
			return "", "", false, err
		}
		if _, err := resolveCheckpointRef(repoRoot, workspaceName, args[0]); err == nil {
			return "", "", false, fmt.Errorf("ambiguous fork argument %q: provide a workspace name, or use 'jvs fork %s <name>'", args[0], args[0])
		} else if !checkpointRefNotFound(err) {
			return "", "", false, err
		}
		return "current", args[0], true, nil
	case 2:
		if err := validatePublicWorkspaceName(args[1]); err != nil {
			return "", "", false, err
		}
		return args[0], args[1], false, nil
	default:
		return "", "", false, fmt.Errorf("too many arguments")
	}
}

func checkpointRefNotFound(err error) bool {
	return errors.Is(err, errRefNotFound)
}

func init() {
	workspaceRemoveCmd.Flags().BoolVarP(&workspaceRemoveForce, "force", "f", false, "force removal when current differs from latest")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspacePathCmd)
	workspaceCmd.AddCommand(workspaceRenameCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	rootCmd.AddCommand(workspaceCmd)

	forkCmd.Flags().StringVar(&forkFromRef, "from", "", "checkpoint ref to fork from")
	forkCmd.Flags().BoolVar(&forkDiscardDirty, "discard-dirty", false, "discard dirty workspace changes for this operation")
	forkCmd.Flags().BoolVar(&forkIncludeWorking, "include-working", false, "checkpoint dirty workspace changes before this operation")
	rootCmd.AddCommand(forkCmd)
}
