package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/diff"
	"github.com/jvs-project/jvs/internal/snapshot"
)

var (
	diffStatOnly bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <from> <to>",
	Short: "Show differences between checkpoints",
	Long: `Show differences between two checkpoints.

Refs can be current, latest, a full checkpoint ID, a unique short ID, or an exact tag.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("diff requires two checkpoint refs")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		r, wtName, err := discoverOptionalWorktree()
		if err != nil {
			return err
		}

		fromID, err := resolveCheckpointRef(r.Root, wtName, args[0])
		if err != nil {
			return fmt.Errorf("resolve from checkpoint: %w", err)
		}
		toID, err := resolveCheckpointRef(r.Root, wtName, args[1])
		if err != nil {
			return fmt.Errorf("resolve to checkpoint: %w", err)
		}

		// Load descriptors for timestamps
		var fromTime, toTime time.Time
		if fromID != "" {
			fromDesc, err := snapshot.LoadDescriptor(r.Root, fromID)
			if err == nil {
				fromTime = fromDesc.CreatedAt
			}
		}
		if toID != "" {
			toDesc, err := snapshot.LoadDescriptor(r.Root, toID)
			if err == nil {
				toTime = toDesc.CreatedAt
			}
		}

		// Compute diff
		differ := diff.NewDiffer(r.Root)
		result, err := differ.Diff(fromID, toID)
		if err != nil {
			return fmt.Errorf("compute diff: %w", err)
		}

		// Set timestamps
		result.SetTimes(fromTime, toTime)

		if jsonOutput {
			return outputJSON(publicDiff(result))
		}

		if diffStatOnly {
			// Print summary only
			fmt.Printf("Added: %d, Removed: %d, Modified: %d\n",
				result.TotalAdded, result.TotalRemoved, result.TotalModified)
		} else {
			// Print full diff
			fmt.Print(result.FormatHuman())
		}
		return nil
	},
}

func init() {
	diffCmd.Flags().BoolVar(&diffStatOnly, "stat", false, "show summary only")
	rootCmd.AddCommand(diffCmd)
}
