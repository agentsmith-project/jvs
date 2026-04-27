package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/pkg/color"
)

var (
	restoreInteractive    bool
	restoreDiscardDirty   bool
	restoreIncludeWorking bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore <ref|latest>",
	Short: "Restore a workspace to a save point",
	Long: `Restore the current workspace to a checkpoint.

Refs can be current, latest, a full checkpoint ID, a unique short ID, or an exact tag.

Examples:
  jvs restore latest
  jvs restore 1771589abc
  jvs restore v1.0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		targetID, err := resolveCheckpointRef(r.Root, workspaceName, args[0])
		if err != nil {
			return err
		}

		if err := rejectDirtyWorkspace(r.Root, workspaceName, "restore", restoreDiscardDirty, restoreIncludeWorking); err != nil {
			return err
		}
		if restoreIncludeWorking {
			if _, err := createCheckpointDescriptor(r.Root, workspaceName, checkpointCreateOptions{
				Note: "include working before restore",
			}); err != nil {
				return err
			}
		}

		if err := restore.NewRestorer(r.Root, detectEngine(r.Root)).Restore(workspaceName, targetID); err != nil {
			return err
		}

		if jsonOutput {
			result, err := statusForRestore(r.Root, workspaceName)
			if err != nil {
				return err
			}
			result["checkpoint_id"] = string(targetID)
			return outputJSON(result)
		}

		status, err := buildWorkspaceStatus(r.Root, workspaceName)
		if err != nil {
			return err
		}
		fmt.Printf("Restored to checkpoint %s\n", color.SnapshotID(targetID.String()))
		if status.AtLatest {
			fmt.Println(color.Success("Workspace is at latest."))
		} else {
			fmt.Println(color.Warning("Workspace current differs from latest."))
			fmt.Println(color.Dim("To continue from here: jvs fork <name>"))
			fmt.Println(color.Dim("To return to latest: jvs restore latest"))
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreInteractive, "interactive", "i", false, "interactive confirmation")
	restoreCmd.Flags().Lookup("interactive").Hidden = true
	restoreCmd.Flags().BoolVar(&restoreDiscardDirty, "discard-dirty", false, "discard dirty workspace changes for this operation")
	restoreCmd.Flags().BoolVar(&restoreIncludeWorking, "include-working", false, "checkpoint dirty workspace changes before this operation")
	rootCmd.AddCommand(restoreCmd)
}
