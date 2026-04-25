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
	Short:   "Manage worktrees",
	Aliases: []string{"wt"},
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new worktree",
	Long: `Create a new worktree.

If --from is specified, the worktree is created from an existing snapshot,
otherwise an empty worktree is created.

Examples:
  jvs worktree create feature-x                    # Create empty worktree
  jvs worktree create hotfix --from v1.0           # Create from tag
  jvs worktree create feature-y --from 1771589-abc # Create from snapshot`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		name := args[0]

		mgr := worktree.NewManager(r.Root)

		// If --from is specified, create from snapshot
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
				fmtErr("create worktree from snapshot: %v", err)
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
				fmt.Printf("Created worktree '%s' from snapshot %s\n", color.Success(name), color.SnapshotID(snapshotID.String()))
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
	Short: "List all worktrees",
	Long: `List all worktrees in the repository.

Shows each worktree name and its current HEAD snapshot.`,
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
	Short: "Print the path to a worktree",
	Long: `Print the path to a worktree.

If no name is specified, prints the path of the current worktree.

Examples:
  jvs worktree path              # Path of current worktree
  jvs worktree path main         # Path of named worktree`,
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
	Short: "Rename a worktree",
	Long: `Rename a worktree.

Changes the worktree name without affecting its content or snapshots.

Examples:
  jvs worktree rename feature-1 feature-branch`,
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
	Short: "Remove a worktree",
	Long: `Remove a worktree.

The worktree payload and metadata are deleted, but all snapshots remain.
Use --force to remove a worktree that is in detached state.

Examples:
  jvs worktree remove feature-x      # Remove worktree
  jvs worktree remove --force old    # Force remove detached worktree`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()
		name := args[0]

		mgr := worktree.NewManager(r.Root)

		// First check if worktree exists for better error message
		_, err := mgr.Get(name)
		if err != nil {
			// Worktree doesn't exist - show helpful error
			fmt.Fprintln(os.Stderr, formatWorktreeNotFoundError(name, r.Root))
			os.Exit(1)
		}

		// Check for detached state unless --force
		if !worktreeForce {
			cfg, err := mgr.Get(name)
			if err == nil && cfg.IsDetached() {
				fmtErr("worktree '%s' is in detached state", name)
				fmt.Println()
				fmt.Printf("Current position: %s\n", cfg.HeadSnapshotID)
				fmt.Printf("Latest snapshot: %s\n", cfg.LatestSnapshotID)
				fmt.Println()
				fmt.Println("To remove anyway, use: jvs worktree remove --force " + name)
				os.Exit(1)
			}
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
	Use:   "fork [snapshot-id] [name]",
	Short: "Create a new worktree from a snapshot",
	Long: `Create a new worktree from a snapshot.

The new worktree will be at HEAD state - you can create snapshots immediately.

If snapshot-id is omitted, uses the current worktree's position.
If name is omitted, auto-generates a name.

The snapshot-id can be:
  - A full snapshot ID
  - A short ID prefix
  - A tag name
  - A note prefix (fuzzy match)

Examples:
  jvs worktree fork                           # Fork from current position, auto-name
  jvs worktree fork feature-x                 # Fork from current position with name
  jvs worktree fork v1.0 hotfix               # Fork from tag v1.0, name hotfix
  jvs worktree fork 1771589-abc feature-y     # Fork from specific snapshot`,
	Args: cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		r, wtName := requireWorktree()

		var snapshotID model.SnapshotID
		var name string

		// Parse arguments
		switch len(args) {
		case 0:
			// No args: use current position, auto-generate name
			mgr := worktree.NewManager(r.Root)
			cfg, err := mgr.Get(wtName)
			if err != nil {
				fmtErr("get current worktree: %v", err)
				os.Exit(1)
			}
			if cfg.HeadSnapshotID == "" {
				fmtErr("current worktree has no snapshots to fork from")
				os.Exit(1)
			}
			snapshotID = cfg.HeadSnapshotID
			name = "" // auto-generate

		case 1:
			// One arg: could be snapshot-id or name
			// Try to interpret as snapshot-id first
			arg := args[0]

			// Try to resolve as snapshot
			id, err := resolveSnapshotID(r.Root, arg)
			if err == nil {
				// Successfully resolved as snapshot
				snapshotID = id
				name = "" // auto-generate
			} else {
				// Not a snapshot, treat as name, use current position
				mgr := worktree.NewManager(r.Root)
				cfg, err := mgr.Get(wtName)
				if err != nil {
					fmtErr("get current worktree: %v", err)
					os.Exit(1)
				}
				if cfg.HeadSnapshotID == "" {
					fmtErr("current worktree has no snapshots to fork from")
					os.Exit(1)
				}
				snapshotID = cfg.HeadSnapshotID
				name = arg
			}

		case 2:
			// Two args: snapshot-id and name
			snapshotArg := args[0]
			name = args[1]

			snapshotID = resolveSnapshotIDOrExit(r.Root, snapshotArg)
		}

		// Auto-generate name if not provided
		if name == "" {
			name = fmt.Sprintf("fork-%s", snapshotID.ShortID())
		}

		// Verify snapshot exists and is valid
		if err := verifySnapshotStrong(r.Root, snapshotID); err != nil {
			fmtErr("verify snapshot: %v", err)
			os.Exit(1)
		}

		eng := newCloneEngine(r.Root)

		// Fork the worktree
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
			fmt.Printf("Created worktree '%s' from snapshot %s\n", color.Success(name), color.SnapshotID(snapshotID.String()))
			fmt.Printf("Path: %s\n", color.Dim(path))
			fmt.Println(color.Success("Worktree is at HEAD state - you can create snapshots."))
		}
	},
}

func init() {
	worktreeCreateCmd.Flags().StringVar(&worktreeCreateFrom, "from", "", "create from snapshot (ID, tag, or note prefix)")
	worktreeRemoveCmd.Flags().BoolVarP(&worktreeForce, "force", "f", false, "force removal even if in detached state")
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
