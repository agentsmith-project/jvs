package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/compression"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/config"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
)

var (
	snapshotTags        []string
	snapshotPaths       []string
	snapshotCompression string
	snapshotNoteFile    string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [note] [-- <paths>...]",
	Short: "Create a snapshot of the current worktree",
	Long: `Create a snapshot of the current worktree.

Captures the current state of the worktree at a point in time.

Examples:
  # Basic snapshot with note
  jvs snapshot "Before refactoring"

  # Snapshot with tags
  jvs snapshot "v1.0 release" --tag v1.0 --tag release

  # Partial snapshot of specific paths
  jvs snapshot "Assets only" -- paths/Assets/

  # Compressed snapshot
  jvs snapshot "checkpoint" --compress fast

  # Multi-line note via stdin
  jvs snapshot - < <<EOF
  ML Experiment: ResNet50 v2
  Result: 92.3% accuracy
  EOF

Compression levels: none, fast, default, max

NOTE: Cannot create snapshots in detached state. Use 'jvs worktree fork'
to create a new worktree from the current position first.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r, wtName := requireWorktree()

		// Check if worktree is in detached state
		wtMgr := worktree.NewManager(r.Root)
		cfg, err := wtMgr.Get(wtName)
		if err != nil {
			fmtErr("get worktree: %v", err)
			os.Exit(1)
		}

		if cfg.IsDetached() {
			fmtErr("cannot create snapshot in detached state")
			fmt.Println()
			fmt.Printf("You are currently at snapshot '%s' (historical).\n", cfg.HeadSnapshotID)
			fmt.Println("To continue working from this point:")
			fmt.Println()
			fmt.Printf("    jvs worktree fork %s <new-worktree-name>\n", cfg.HeadSnapshotID.ShortID())
			fmt.Println()
			fmt.Println("Or return to the latest state:")
			fmt.Println()
			fmt.Println("    jvs restore HEAD")
			os.Exit(1)
		}

		// Get note from args, stdin, or file
		var note string
		if len(args) > 0 && args[0] == "-" {
			// Read from stdin
			note = readNoteFromStdin()
		} else if snapshotNoteFile != "" {
			// Read from file
			content, err := os.ReadFile(snapshotNoteFile)
			if err != nil {
				fmtErr("read note file: %v", err)
				os.Exit(1)
			}
			note = string(content)
		} else if len(args) > 0 {
			note = args[0]
		}

		// Load config for default tags
		jvsCfg, _ := config.Load(r.Root)

		// Validate tags
		for _, tag := range snapshotTags {
			if err := pathutil.ValidateTag(tag); err != nil {
				fmtErr("invalid tag %q: %v", tag, err)
				os.Exit(1)
			}
		}

		// Combine command-line tags with default tags from config
		allTags := snapshotTags
		if defaultTags := jvsCfg.GetDefaultTags(); len(defaultTags) > 0 {
			// Add default tags that aren't already specified
			tagMap := make(map[string]bool)
			for _, tag := range allTags {
				tagMap[tag] = true
			}
			for _, defaultTag := range defaultTags {
				if !tagMap[defaultTag] {
					allTags = append(allTags, defaultTag)
				}
			}
		}

		// Create creator with compression if specified
		creator := snapshot.NewCreator(r.Root, detectEngine(r.Root))
		if snapshotCompression != "" {
			comp, err := compression.NewCompressorFromString(snapshotCompression)
			if err != nil {
				fmtErr("invalid compression level: %v", err)
				os.Exit(1)
			}
			creator.SetCompression(comp.Level)
		}

		var desc *model.Descriptor

		if len(snapshotPaths) > 0 {
			// Partial snapshot
			desc, err = creator.CreatePartial(wtName, note, allTags, snapshotPaths)
		} else {
			// Full snapshot
			desc, err = creator.Create(wtName, note, allTags)
		}

		if err != nil {
			fmtErr("create snapshot: %v", err)
			os.Exit(1)
		}

		if jsonOutput {
			outputJSON(desc)
		} else {
			if len(snapshotPaths) > 0 {
				fmt.Printf("Created partial snapshot %s (%d paths)\n", color.SnapshotID(desc.SnapshotID.String()), len(snapshotPaths))
			} else {
				fmt.Printf("Created snapshot %s\n", color.SnapshotID(desc.SnapshotID.String()))
			}
			if desc.Compression != nil {
				fmt.Printf("  (compressed: %s level %d)\n", desc.Compression.Type, desc.Compression.Level)
			}
			if len(allTags) > 0 {
				tagColors := make([]string, len(allTags))
				for i, tag := range allTags {
					tagColors[i] = color.Tag(tag)
				}
				fmt.Printf("  Tags: %s\n", strings.Join(tagColors, ", "))
			}
		}
	},
}

// readNoteFromStdin reads a multi-line note from stdin.
// Reads until EOF and returns the trimmed content.
func readNoteFromStdin() string {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmtErr("read stdin: %v", err)
		os.Exit(1)
	}
	// Trim trailing whitespace while preserving internal newlines
	note := strings.TrimRight(strings.Join(lines, "\n"), "\n\r ")
	// Also trim leading whitespace
	note = strings.TrimLeft(note, "\n\r ")
	return note
}

func init() {
	snapshotCmd.Flags().StringSliceVar(&snapshotTags, "tag", []string{}, "tag for this snapshot (can be repeated)")
	snapshotCmd.Flags().StringSliceVar(&snapshotPaths, "paths", []string{}, "paths to include in partial snapshot")
	snapshotCmd.Flags().StringVar(&snapshotCompression, "compress", "", "compression level (none, fast, default, max)")
	snapshotCmd.Flags().StringVarP(&snapshotNoteFile, "file", "F", "", "read note from file")
	rootCmd.AddCommand(snapshotCmd)
}
