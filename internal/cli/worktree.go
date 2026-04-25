package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/model"
)

var (
	worktreeCreateFrom string
	worktreeForce      bool
)

var worktreeCmd = &cobra.Command{
	Use:     "worktree",
	Short:   "Manage workspaces (legacy alias)",
	Aliases: []string{"wt"},
	Hidden:  true,
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new workspace (legacy alias)",
	Long: `Create a new workspace.

If --from is specified, the workspace is created from an existing checkpoint,
otherwise an empty workspace is created. Prefer jvs fork for new workspaces
from checkpoints.

Examples:
  jvs fork feature-x                               # Create from current checkpoint
  jvs fork v1.0 hotfix                             # Create from tag
  jvs worktree create scratch                      # Legacy empty workspace`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		name := args[0]

		mgr := worktree.NewManager(r.Root)

		// If --from is specified, create from checkpoint.
		if worktreeCreateFrom != "" {
			snapshotID := resolveSnapshotIDOrExit(r.Root, worktreeCreateFrom)

			// Verify snapshot exists and is valid
			if err := verifySnapshotStrong(r.Root, snapshotID); err != nil {
				fmtErr("verify snapshot: %v", err)
				os.Exit(1)
			}

			eng := newCloneEngine(r.Root)

			cfg, err := mgr.CreateFromSnapshot(name, snapshotID, func(src, dst string) error {
				_, err := eng.Clone(src, dst)
				return err
			})
			if err != nil {
				fmtErr("create workspace from checkpoint: %v", err)
				os.Exit(1)
			}

			if jsonOutput {
				outputJSON(cfg)
			} else {
				path, err := mgr.Path(name)
				if err != nil {
					fmtErr("resolve worktree path: %v", err)
					os.Exit(1)
				}
				fmt.Printf("Created worktree '%s' from checkpoint %s\n", color.Success(name), color.SnapshotID(snapshotID.String()))
				fmt.Printf("Path: %s\n", color.Dim(path))
			}
			return
		}

		// Create empty worktree
		cfg, err := mgr.Create(name, nil)
		if err != nil {
			fmtErr("create worktree: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(cfg)
		} else {
			path, err := mgr.Path(name)
			if err != nil {
				fmtErr("resolve worktree path: %v", err)
				os.Exit(1)
			}
			fmt.Printf("Created worktree '%s' at %s\n", color.Success(name), color.Dim(path))
		}
	},
}

// resolveSnapshotID resolves a snapshot reference to a full snapshot ID.
// Returns an error if the snapshot cannot be resolved.
func resolveSnapshotID(repoRoot, ref string) (model.SnapshotID, error) {
	// Try exact match first
	testID := model.SnapshotID(ref)
	_, err := snapshot.LoadDescriptor(repoRoot, testID)
	if err == nil {
		return testID, nil
	}

	// Try fuzzy match
	desc, err := snapshot.FindOne(repoRoot, ref)
	if err != nil {
		return "", fmt.Errorf("snapshot not found: %s", ref)
	}
	return desc.SnapshotID, nil
}

// resolveSnapshotIDOrExit resolves a snapshot reference to a full snapshot ID.
// Prints enhanced error messages and exits on failure (for CLI use).
func resolveSnapshotIDOrExit(repoRoot, ref string) model.SnapshotID {
	id, err := resolveSnapshotID(repoRoot, ref)
	if err != nil {
		// Print enhanced error message with suggestions
		fmt.Fprintln(os.Stderr, formatSnapshotNotFoundError(ref, repoRoot))
		os.Exit(1)
	}
	return id
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workspaces (legacy alias)",
	Long: `List all workspaces in the repository.

Shows each workspace name and its current checkpoint. Prefer jvs workspace list.`,
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		mgr := worktree.NewManager(r.Root)
		list, err := mgr.List()
		if err != nil {
			fmtErr("list worktrees: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(list)
			return
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
	},
}

var worktreePathCmd = &cobra.Command{
	Use:   "path [<name>]",
	Short: "Print the path to a workspace (legacy alias)",
	Long: `Print the path to a workspace.

If no name is specified, prints the path of the current workspace.
Prefer jvs workspace path.

Examples:
  jvs workspace path             # Path of current workspace
  jvs workspace path main        # Path of named workspace`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			_, name = requireWorktree()
		}

		mgr := worktree.NewManager(r.Root)

		// Check if worktree exists for better error message
		if name != "" {
			_, err := mgr.Get(name)
			if err != nil {
				// Worktree doesn't exist - show helpful error
				fmt.Fprintln(os.Stderr, formatWorktreeNotFoundError(name, r.Root))
				os.Exit(1)
			}
		}

		path, err := mgr.Path(name)
		if err != nil {
			fmtErr("resolve worktree path: %v", err)
			os.Exit(1)
		}
		fmt.Println(path)
	},
}

var worktreeRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a workspace (legacy alias)",
	Long: `Rename a workspace.

Changes the workspace name without affecting its content or checkpoints.
Prefer jvs workspace rename.

Examples:
  jvs workspace rename feature-1 feature-branch`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		oldName := args[0]
		newName := args[1]

		mgr := worktree.NewManager(r.Root)

		// Check if source worktree exists for better error message
		_, err := mgr.Get(oldName)
		if err != nil {
			// Worktree doesn't exist - show helpful error
			fmt.Fprintln(os.Stderr, formatWorktreeNotFoundError(oldName, r.Root))
			os.Exit(1)
		}

		if err := mgr.Rename(oldName, newName); err != nil {
			fmtErr("rename worktree: %v", err)
			os.Exit(1)
		}

		if !jsonOutput {
			fmt.Printf("Renamed worktree '%s' to '%s'\n", oldName, newName)
		}
	},
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a workspace (legacy alias)",
	Long: `Remove a workspace.

The workspace payload and metadata are deleted, but all checkpoints remain.
Use --force when the workspace current differs from latest. Prefer
jvs workspace remove.

Examples:
  jvs workspace remove feature-x      # Remove workspace
  jvs workspace remove --force old    # Force remove when current differs from latest`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		name := args[0]

		mgr := worktree.NewManager(r.Root)

		if _, err := validateWorkspaceRemoval(r.Root, name, worktreeForce); err != nil {
			fmtErr("%v", err)
			os.Exit(1)
		}

		if err := mgr.Remove(name); err != nil {
			fmtErr("remove worktree: %v", err)
			os.Exit(1)
		}

		if !jsonOutput {
			fmt.Printf("Removed worktree '%s'\n", name)
		}
	},
}

var worktreeForkCmd = &cobra.Command{
	Use:   "fork [checkpoint-ref] [name]",
	Short: "Create a workspace from a checkpoint (legacy alias)",
	Long: `Create a workspace from a checkpoint.

This hidden legacy alias remains for older scripts. Prefer jvs fork.

If checkpoint-ref is omitted, uses the current workspace checkpoint.
If name is omitted, auto-generates a name.

The checkpoint ref can be:
  - current
  - latest
  - A full checkpoint ID
  - A short ID prefix
  - A tag name
  - A note prefix (fuzzy match)

Examples:
  jvs fork                                    # Fork from current, auto-name
  jvs fork feature-x                          # Fork from current with name
  jvs fork v1.0 hotfix                        # Fork from tag v1.0, name hotfix
  jvs fork 1771589-abc feature-y              # Fork from specific checkpoint`,
	Args: cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		r, wtName := requireWorktree()
		snapshotID, name, defaultCurrent, err := resolveLegacyWorktreeForkArgs(r.Root, args)
		if err != nil {
			fmtErr("%v", err)
			os.Exit(1)
		}

		if err := rejectDirtyWorkspace(r.Root, wtName, "fork", forkDiscardDirty, forkIncludeWorking); err != nil {
			fmtErr("%v", err)
			os.Exit(1)
		}

		if forkIncludeWorking {
			desc, err := createCheckpointDescriptor(r.Root, wtName, checkpointCreateOptions{
				Note: "include working before fork",
			})
			if err != nil {
				fmtErr("checkpoint working changes: %v", err)
				os.Exit(1)
			}
			if defaultCurrent {
				snapshotID = desc.SnapshotID
			}
		}

		if defaultCurrent && snapshotID == "" {
			snapshotID, err = legacyCurrentForkSnapshot(r.Root, wtName)
			if err != nil {
				fmtErr("%v", err)
				os.Exit(1)
			}
		}

		// Auto-generate name if not provided
		if name == "" {
			name = fmt.Sprintf("fork-%s", snapshotID.ShortID())
		}

		// Verify checkpoint exists and is valid.
		if err := verifySnapshotStrong(r.Root, snapshotID); err != nil {
			fmtErr("verify snapshot: %v", err)
			os.Exit(1)
		}

		eng := newCloneEngine(r.Root)

		// Fork the workspace.
		mgr := worktree.NewManager(r.Root)
		cfg, err := mgr.Fork(snapshotID, name, func(src, dst string) error {
			_, err := eng.Clone(src, dst)
			return err
		})
		if err != nil {
			fmtErr("fork worktree: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(cfg)
		} else {
			path, err := mgr.Path(name)
			if err != nil {
				fmtErr("resolve worktree path: %v", err)
				os.Exit(1)
			}
			fmt.Printf("Created worktree '%s' from checkpoint %s\n", color.Success(name), color.SnapshotID(snapshotID.String()))
			fmt.Printf("Path: %s\n", color.Dim(path))
			fmt.Println(color.Success("Workspace is at latest - you can create checkpoints."))
		}
	},
}

func resolveLegacyWorktreeForkArgs(repoRoot string, args []string) (model.SnapshotID, string, bool, error) {
	switch len(args) {
	case 0:
		return "", "", true, nil
	case 1:
		arg := args[0]
		id, err := resolveSnapshotID(repoRoot, arg)
		if err == nil {
			return id, "", false, nil
		}
		return "", arg, true, nil
	case 2:
		id, err := resolveSnapshotID(repoRoot, args[0])
		if err != nil {
			return "", "", false, err
		}
		return id, args[1], false, nil
	default:
		return "", "", false, fmt.Errorf("too many arguments")
	}
}

func legacyCurrentForkSnapshot(repoRoot, wtName string) (model.SnapshotID, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(wtName)
	if err != nil {
		return "", fmt.Errorf("get current worktree: %w", err)
	}
	if cfg.HeadSnapshotID == "" {
		return "", fmt.Errorf("current workspace has no checkpoints to fork from")
	}
	return cfg.HeadSnapshotID, nil
}

func init() {
	worktreeCreateCmd.Flags().StringVar(&worktreeCreateFrom, "from", "", "create from checkpoint (ID, tag, or note prefix)")
	worktreeRemoveCmd.Flags().BoolVarP(&worktreeForce, "force", "f", false, "force removal when current differs from latest")
	worktreeForkCmd.Flags().BoolVar(&forkDiscardDirty, "discard-dirty", false, "discard dirty workspace changes for this operation")
	worktreeForkCmd.Flags().BoolVar(&forkIncludeWorking, "include-working", false, "checkpoint dirty workspace changes before this operation")
	worktreeCmd.AddCommand(worktreeCreateCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreePathCmd)
	worktreeCmd.AddCommand(worktreeRenameCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
	worktreeCmd.AddCommand(worktreeForkCmd)
	rootCmd.AddCommand(worktreeCmd)
}

func verifySnapshotStrong(repoRoot string, snapshotID model.SnapshotID) error {
	result, err := verify.NewVerifier(repoRoot).VerifySnapshot(snapshotID, true)
	if err != nil {
		return err
	}
	if result.TamperDetected {
		if result.Error != "" {
			return fmt.Errorf("%s", result.Error)
		}
		return fmt.Errorf("tamper detected")
	}
	return nil
}
