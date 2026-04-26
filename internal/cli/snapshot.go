package cli

import (
	"bufio"
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
	snapshotTags        []string
	snapshotPaths       []string
	snapshotCompression string
	snapshotNoteFile    string
)

var snapshotCmd = &cobra.Command{
	Use:    "snapshot [note] [-- <paths>...]",
	Short:  "Create a checkpoint of the current workspace (legacy alias)",
	Hidden: true,
	Long: `Create a checkpoint of the current workspace.

This hidden legacy alias remains for older scripts. Prefer jvs checkpoint.

Captures the current state of the workspace at a point in time.

Examples:
  # Basic checkpoint with note
  jvs checkpoint "Before refactoring"

  # Checkpoint with tags
  jvs checkpoint "v1.0 release" --tag v1.0 --tag release

  # Continue from a historical current checkpoint
  jvs fork hotfix

  # Return to latest before checkpointing
  jvs restore latest

Compression levels: none, fast, default, max

NOTE: Cannot create a checkpoint when current differs from latest. Use
jvs fork <name> to continue from the current checkpoint, or jvs restore latest
before running jvs checkpoint.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		r, wtName := requireWorktree()

		// Check whether current differs from latest.
		wtMgr := worktree.NewManager(r.Root)
		cfg, err := wtMgr.Get(wtName)
		if err != nil {
			fmtErr("get worktree: %v", err)
			os.Exit(1)
		}

		if cfg.IsDetached() {
			fmtErr("cannot create checkpoint: current differs from latest")
			fmt.Println()
			fmt.Printf("Current checkpoint: %s\n", cfg.HeadSnapshotID)
			fmt.Printf("Latest checkpoint: %s\n", cfg.LatestSnapshotID)
			fmt.Println()
			fmt.Println("To continue from the current checkpoint:")
			fmt.Println()
			fmt.Printf("    jvs fork %s <new-workspace-name>\n", cfg.HeadSnapshotID.ShortID())
			fmt.Println()
			fmt.Println("To return this workspace to latest before checkpointing:")
			fmt.Println()
			fmt.Println("    jvs restore latest")
			fmt.Println()
			fmt.Println("Then create a checkpoint with:")
			fmt.Println()
			fmt.Println("    jvs checkpoint <note>")
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
	snapshotCmd.Flags().StringSliceVar(&snapshotTags, "tag", []string{}, "tag for this checkpoint (can be repeated)")
	snapshotCmd.Flags().StringSliceVar(&snapshotPaths, "paths", []string{}, "paths to include in partial checkpoint")
	snapshotCmd.Flags().StringVar(&snapshotCompression, "compress", "", "compression level (none, fast, default, max)")
	snapshotCmd.Flags().StringVarP(&snapshotNoteFile, "file", "F", "", "read note from file")
	rootCmd.AddCommand(snapshotCmd)
}
