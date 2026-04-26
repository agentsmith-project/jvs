package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/color"
)

var initCmd = &cobra.Command{
	Use:   "init <repo-path>",
	Short: "Initialize a new JVS repository",
	Long: `Initialize a new JVS repository at <repo-path>.

This creates:
  - .jvs/ directory with all metadata structures
  - main/ workspace as the primary payload directory
  - format_version file (version 1)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := repo.InitTarget(args[0])
		if err != nil {
			return fmt.Errorf("failed to initialize repository: %w", err)
		}
		mainWorkspace := filepath.Join(r.Root, "main")
		capabilities, err := engine.ProbeCapabilities(r.Root, true)
		if err != nil {
			return fmt.Errorf("probe capabilities: %w", err)
		}

		if jsonOutput {
			output := map[string]any{
				"repo_root":      r.Root,
				"main_workspace": mainWorkspace,
				"format_version": r.FormatVersion,
				"repo_id":        r.RepoID,
			}
			applySetupJSONFields(output, capabilities, capabilities.RecommendedEngine, capabilities.Warnings)
			return outputJSON(output)
		}

		fmt.Printf("Initialized JVS repository\n")
		fmt.Printf("  Repo root: %s\n", color.Success(r.Root))
		fmt.Printf("  Main workspace: %s\n", color.Highlight(mainWorkspace))
		fmt.Printf("  Capabilities: write=%s juicefs=%t reflink=%s copy=%t recommended=%s\n",
			capabilities.Write.Confidence,
			capabilities.JuiceFS.Supported,
			capabilities.Reflink.Confidence,
			capabilities.Copy.Supported,
			capabilities.RecommendedEngine,
		)
		for _, warning := range capabilities.Warnings {
			fmt.Printf("  Warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
