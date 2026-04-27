package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	historyLimit      int
	historyNoteFilter string
	historyTagFilter  string
	historyAll        bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show save point history",
	Long: `Show save points for the active workspace.

Examples:
  jvs history
  jvs history -n 10
  jvs history --grep "baseline"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		cfg, savePoints, err := loadSavePointHistory(r.Root, workspaceName)
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(publicSavePointHistory(workspaceName, savePoints, cfg.LatestSnapshotID))
		}

		if len(savePoints) == 0 {
			fmt.Println("No save points yet.")
			return nil
		}

		if historyAll {
			fmt.Println("Save points:")
		} else {
			fmt.Printf("Save points for workspace %s:\n", workspaceName)
		}
		for _, desc := range savePoints {
			message := desc.Note
			if message == "" {
				message = color.Dim("(no message)")
			}
			fmt.Printf("%s  %s  %s\n",
				color.SnapshotID(desc.SnapshotID.ShortID()),
				color.Dim(desc.CreatedAt.Format("2006-01-02 15:04")),
				message,
			)
		}
		return nil
	},
}

func loadSavePointHistory(repoRoot, workspaceName string) (*model.WorktreeConfig, []*model.Descriptor, error) {
	cfg, err := worktree.NewManager(repoRoot).Get(workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("load workspace: %w", err)
	}

	if historyAll {
		savePoints, err := snapshot.Find(repoRoot, snapshot.FilterOptions{
			NoteContains: historyNoteFilter,
			HasTag:       historyTagFilter,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list save points: %w", err)
		}
		return cfg, limitSavePoints(savePoints), nil
	}

	if cfg.LatestSnapshotID == "" {
		return cfg, []*model.Descriptor{}, nil
	}

	var savePoints []*model.Descriptor
	currentID := &cfg.LatestSnapshotID
	for currentID != nil && (historyLimit == 0 || len(savePoints) < historyLimit) {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return nil, nil, fmt.Errorf("load save point: %w", err)
		}
		if historyNoteFilter != "" && !strings.Contains(desc.Note, historyNoteFilter) {
			currentID = desc.ParentID
			continue
		}
		if historyTagFilter != "" && !hasTag(desc, historyTagFilter) {
			currentID = desc.ParentID
			continue
		}
		savePoints = append(savePoints, desc)
		currentID = desc.ParentID
	}
	return cfg, savePoints, nil
}

func limitSavePoints(savePoints []*model.Descriptor) []*model.Descriptor {
	if historyLimit <= 0 || len(savePoints) <= historyLimit {
		return savePoints
	}
	return savePoints[:historyLimit]
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
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 0, "limit number of save points (0 = all)")
	historyCmd.Flags().StringVarP(&historyNoteFilter, "grep", "g", "", "filter by message substring")
	historyCmd.Flags().StringVar(&historyTagFilter, "tag", "", "filter by tag")
	historyCmd.Flags().BoolVar(&historyAll, "all", false, "show save points from all workspaces")
	rootCmd.AddCommand(historyCmd)
}
