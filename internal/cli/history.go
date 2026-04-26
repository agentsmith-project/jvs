package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	historyLimit      int
	historyNoteFilter string
	historyTagFilter  string
	historyAll        bool
)

var historyCmd = &cobra.Command{
	Use:    "history",
	Short:  "Show snapshot history for the current worktree",
	Hidden: true,
	Long: `Show snapshot history for the current worktree.

Traverses the lineage chain from head backwards.
Use --all to show all snapshots in the repository.

The output shows:
  - [HEAD] marker on the latest snapshot in the lineage
  - Current position indicator (you are here)

Examples:
  jvs history                    # Show current worktree history
  jvs history -n 10              # Show last 10 snapshots
  jvs history --grep "fix"       # Filter by note substring
  jvs history --tag v1.0         # Filter by tag
  jvs history --all              # Show all snapshots in repo`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			return errclass.ErrUsage.
				WithMessage("legacy command is not available with --json").
				WithHint("Use jvs checkpoint list --json.")
		}

		r, wtName := requireWorktree()

		var history []*model.Descriptor
		var latestSnapshotID model.SnapshotID
		var currentSnapshotID model.SnapshotID

		// Get worktree config to know current state
		wtMgr := worktree.NewManager(r.Root)
		cfg, err := wtMgr.Get(wtName)
		if err != nil {
			fmtErr("load worktree config: %v", err)
			os.Exit(1)
		}
		currentSnapshotID = cfg.HeadSnapshotID
		latestSnapshotID = cfg.LatestSnapshotID

		if historyAll {
			// Show all snapshots with optional filtering
			opts := snapshot.FilterOptions{
				NoteContains: historyNoteFilter,
				HasTag:       historyTagFilter,
			}
			var err error
			history, err = snapshot.Find(r.Root, opts)
			if err != nil {
				fmtErr("list snapshots: %v", err)
				os.Exit(1)
			}
		} else {
			// Show lineage for current worktree
			if cfg.HeadSnapshotID == "" {
				if jsonOutput {
					outputJSON([]any{})
				} else {
					fmt.Println("No snapshots yet.")
				}
				return nil
			}

			currentID := &cfg.HeadSnapshotID
			count := 0

			for currentID != nil && (historyLimit == 0 || count < historyLimit) {
				desc, err := snapshot.LoadDescriptor(r.Root, *currentID)
				if err != nil {
					break
				}

				// Apply filters
				if historyNoteFilter != "" && !strings.Contains(desc.Note, historyNoteFilter) {
					currentID = desc.ParentID
					continue
				}
				if historyTagFilter != "" && !hasTag(desc, historyTagFilter) {
					currentID = desc.ParentID
					continue
				}

				history = append(history, desc)
				currentID = desc.ParentID
				count++
			}
		}

		if jsonOutput {
			outputJSON(history)
			return nil
		}

		if len(history) == 0 {
			fmt.Println("No snapshots found.")
			return nil
		}

		// Print history with markers
		isDetached := cfg.IsDetached()
		for _, desc := range history {
			note := desc.Note
			if note == "" {
				note = color.Dim("(no note)")
			}
			tagsStr := ""
			if len(desc.Tags) > 0 {
				tagColors := make([]string, len(desc.Tags))
				for i, tag := range desc.Tags {
					tagColors[i] = color.Tag(tag)
				}
				tagsStr = "  [" + strings.Join(tagColors, ",") + "]"
			}

			// Build marker string
			marker := ""
			if !historyAll {
				// Mark HEAD (latest in lineage)
				if desc.SnapshotID == latestSnapshotID {
					marker = "  " + color.Header("[HEAD]")
				}
			}

			// Print the line with colored snapshot ID
			fmt.Printf("%s  %s  %s%s%s\n",
				color.SnapshotID(desc.SnapshotID.ShortID()),
				color.Dim(desc.CreatedAt.Format("2006-01-02 15:04")),
				note,
				tagsStr,
				marker,
			)

			// Show "you are here" marker after current position
			if desc.SnapshotID == currentSnapshotID {
				if isDetached {
					fmt.Println(color.Dim("◄── you are here (detached)"))
				} else if !historyAll {
					fmt.Println(color.Success("◄── you are here (HEAD)"))
				}
			}
		}
		return nil
	},
}

func hasTag(desc *model.Descriptor, tag string) bool {
	for _, t := range desc.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 0, "limit number of entries (0 = all)")
	historyCmd.Flags().StringVarP(&historyNoteFilter, "grep", "g", "", "filter by note substring")
	historyCmd.Flags().StringVar(&historyTagFilter, "tag", "", "filter by tag")
	historyCmd.Flags().BoolVar(&historyAll, "all", false, "show all snapshots (not just current worktree)")
	rootCmd.AddCommand(historyCmd)
}
