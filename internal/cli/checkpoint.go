package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/config"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var (
	checkpointTags        []string
	checkpointPaths       []string
	checkpointCompression string
	checkpointNoteFile    string
)

type checkpointCreateOptions struct {
	Note        string
	Tags        []string
	Paths       []string
	Compression string
}

var checkpointCmd = &cobra.Command{
	Use:    "checkpoint [note]",
	Short:  "Create or list checkpoints",
	Hidden: true,
	Long: `Create a checkpoint of the current workspace.

Examples:
  jvs checkpoint "Before refactoring"
  jvs checkpoint "v1.0 release" --tag v1.0
  jvs checkpoint list`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		note, err := checkpointNote(args, checkpointNoteFile)
		if err != nil {
			return err
		}

		desc, err := createCheckpointDescriptor(r.Root, workspaceName, checkpointCreateOptions{
			Note:        note,
			Tags:        checkpointTags,
			Paths:       checkpointPaths,
			Compression: checkpointCompression,
		})
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(publicCheckpoint(desc))
		}

		if len(checkpointPaths) > 0 {
			fmt.Printf("Created checkpoint %s (%d paths)\n", color.SnapshotID(desc.SnapshotID.String()), len(checkpointPaths))
		} else {
			fmt.Printf("Created checkpoint %s\n", color.SnapshotID(desc.SnapshotID.String()))
		}
		if desc.Compression != nil {
			fmt.Printf("  (compressed: %s level %d)\n", desc.Compression.Type, desc.Compression.Level)
		}
		if len(desc.Tags) > 0 {
			tagColors := make([]string, len(desc.Tags))
			for i, tag := range desc.Tags {
				tagColors[i] = color.Tag(tag)
			}
			fmt.Printf("  Tags: %s\n", strings.Join(tagColors, ", "))
		}
		return nil
	},
}

var checkpointListCmd = &cobra.Command{
	Use:   "list",
	Short: "List checkpoints",
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		checkpoints, err := snapshot.Find(r.Root, snapshot.FilterOptions{WorktreeName: workspaceName})
		if err != nil {
			return err
		}
		if jsonOutput {
			return outputJSON(publicCheckpoints(checkpoints))
		}
		if len(checkpoints) == 0 {
			fmt.Println("No checkpoints found.")
			return nil
		}

		var current, latest model.SnapshotID
		if workspaceName != "" {
			if cfg, err := worktree.NewManager(r.Root).Get(workspaceName); err == nil {
				current = cfg.HeadSnapshotID
				latest = cfg.LatestSnapshotID
			}
		}

		for _, desc := range checkpoints {
			markers := checkpointMarkers(desc.SnapshotID, current, latest)
			note := desc.Note
			if note == "" {
				note = color.Dim("(no note)")
			}
			fmt.Printf("%s  %s  %s%s\n",
				color.SnapshotID(desc.SnapshotID.ShortID()),
				color.Dim(desc.CreatedAt.Format("2006-01-02 15:04")),
				note,
				markers,
			)
		}
		return nil
	},
}

func checkpointNote(args []string, noteFile string) (string, error) {
	if len(args) > 0 && args[0] == "-" {
		return readNoteFromStdin(), nil
	}
	if noteFile != "" {
		content, err := os.ReadFile(noteFile)
		if err != nil {
			return "", fmt.Errorf("read note file: %w", err)
		}
		return string(content), nil
	}
	if len(args) > 0 {
		return args[0], nil
	}
	return "", nil
}

func createCheckpointDescriptor(repoRoot, workspaceName string, opts checkpointCreateOptions) (*model.Descriptor, error) {
	wtMgr := worktree.NewManager(repoRoot)
	cfg, err := wtMgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	if !cfg.CanSnapshot() {
		return nil, fmt.Errorf("cannot checkpoint while current differs from latest; run jvs fork <name> or jvs restore latest")
	}

	allTags, err := checkpointTagsWithDefaults(repoRoot, opts.Tags)
	if err != nil {
		return nil, err
	}

	creator := snapshot.NewCreator(repoRoot, detectEngine(repoRoot))
	if opts.Compression != "" {
		comp, err := compression.NewCompressorFromString(opts.Compression)
		if err != nil {
			return nil, fmt.Errorf("invalid compression level: %w", err)
		}
		creator.SetCompression(comp.Level)
	}

	if len(opts.Paths) > 0 {
		return creator.CreatePartial(workspaceName, opts.Note, allTags, opts.Paths)
	}
	return creator.Create(workspaceName, opts.Note, allTags)
}

func checkpointTagsWithDefaults(repoRoot string, tags []string) ([]string, error) {
	allTags := append([]string{}, tags...)
	if jvsCfg, err := config.Load(repoRoot); err == nil {
		if defaultTags := jvsCfg.GetDefaultTags(); len(defaultTags) > 0 {
			seen := make(map[string]bool)
			for _, tag := range allTags {
				seen[tag] = true
			}
			for _, tag := range defaultTags {
				if !seen[tag] {
					allTags = append(allTags, tag)
					seen[tag] = true
				}
			}
		}
	}

	for _, tag := range allTags {
		if err := pathutil.ValidateTag(tag); err != nil {
			return nil, fmt.Errorf("invalid tag %q: %w", tag, err)
		}
		if err := validateCheckpointRefName(tag); err != nil {
			return nil, fmt.Errorf("invalid tag %q: %w", tag, err)
		}
	}
	return allTags, nil
}

func checkpointMarkers(id, current, latest model.SnapshotID) string {
	var markers []string
	if id == current {
		markers = append(markers, "current")
	}
	if id == latest {
		markers = append(markers, "latest")
	}
	if len(markers) == 0 {
		return ""
	}
	return "  [" + strings.Join(markers, ",") + "]"
}

func init() {
	checkpointCmd.Flags().StringSliceVar(&checkpointTags, "tag", []string{}, "tag for this checkpoint (can be repeated)")
	checkpointCmd.Flags().StringSliceVar(&checkpointPaths, "paths", []string{}, "paths to include in a partial checkpoint")
	checkpointCmd.Flags().StringVar(&checkpointCompression, "compress", "", "compression level (none, fast, default, max)")
	checkpointCmd.Flags().StringVarP(&checkpointNoteFile, "file", "F", "", "read note from file")
	checkpointCmd.Flags().Lookup("paths").Hidden = true
	checkpointCmd.Flags().Lookup("compress").Hidden = true
	checkpointCmd.AddCommand(checkpointListCmd)
	rootCmd.AddCommand(checkpointCmd)
}
