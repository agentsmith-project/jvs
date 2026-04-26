package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/worktree"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show repository information",
	Run: func(cmd *cobra.Command, args []string) {
		r := requireRepo()

		// Count worktrees and snapshots
		wtMgr := worktree.NewManager(r.Root)
		wtList, _ := wtMgr.List()

		snapshotsDir := r.Root + "/.jvs/snapshots"
		entries, _ := os.ReadDir(snapshotsDir)
		snapshotCount := len(entries)

		snapshotEngine := string(detectEngineNoWrite(r.Root))

		info := map[string]any{
			"repo_root":        r.Root,
			"repo_id":          r.RepoID,
			"format_version":   r.FormatVersion,
			"engine":           snapshotEngine,
			"workspace_count":  len(wtList),
			"checkpoint_count": snapshotCount,
		}

		if jsonOutput {
			outputJSON(info)
			return
		}

		fmt.Printf("Repository: %s\n", r.Root)
		fmt.Printf("  Repo ID: %s\n", r.RepoID)
		fmt.Printf("  Format version: %d\n", r.FormatVersion)
		fmt.Printf("  Engine: %s\n", snapshotEngine)
		fmt.Printf("  Workspaces: %d\n", len(wtList))
		fmt.Printf("  Checkpoints: %d\n", snapshotCount)
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
