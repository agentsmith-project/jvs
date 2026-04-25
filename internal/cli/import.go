package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/repo"
)

var importCmd = &cobra.Command{
	Use:   "import <existing-dir> <repo-path>",
	Short: "Import an existing directory into a new JVS repository",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := existingDirectory(args[0])
		if err != nil {
			return fmt.Errorf("invalid import source: %w", err)
		}
		if err := rejectContainsJVS(source); err != nil {
			return err
		}
		if err := rejectDangerousOverlap("source", source, "repository target", args[1]); err != nil {
			return err
		}

		r, err := repo.InitTarget(args[1])
		if err != nil {
			return fmt.Errorf("initialize import repository: %w", err)
		}

		mainWorkspace := filepath.Join(r.Root, "main")
		transferPlan, err := planTransfer(source, mainWorkspace, r.Root)
		if err != nil {
			return fmt.Errorf("plan source transfer: %w", err)
		}
		if _, err := cloneDirectory(source, mainWorkspace, transferPlan); err != nil {
			return fmt.Errorf("copy source into main workspace: %w", err)
		}

		note := fmt.Sprintf("initial import from %s", source)
		desc, err := createInitialCheckpoint(r.Root, note, []string{"import"})
		if err != nil {
			return fmt.Errorf("create initial checkpoint: %w", err)
		}

		output := map[string]any{
			"scope":              "import",
			"requested_scope":    "import",
			"repo_root":          r.Root,
			"main_workspace":     mainWorkspace,
			"provenance":         source,
			"initial_checkpoint": desc.SnapshotID,
			"engine":             desc.Engine,
		}
		applyTransferJSONFields(output, transferPlan)
		if jsonOutput {
			return outputJSON(output)
		}

		fmt.Printf("Imported directory into JVS repository\n")
		fmt.Printf("  Scope: import\n")
		fmt.Printf("  Repo root: %s\n", r.Root)
		fmt.Printf("  Main workspace: %s\n", mainWorkspace)
		fmt.Printf("  Provenance: %s\n", source)
		fmt.Printf("  Initial checkpoint: %s\n", desc.SnapshotID)
		fmt.Printf("  Engine: %s\n", desc.Engine)
		fmt.Printf("  Requested engine: %s\n", transferPlan.RequestedEngine)
		fmt.Printf("  Transfer engine: %s\n", transferPlan.TransferEngine)
		fmt.Printf("  Effective engine: %s\n", transferPlan.EffectiveEngine)
		fmt.Printf("  Optimized transfer: %t\n", transferPlan.OptimizedTransfer)
		for _, reason := range transferPlan.DegradedReasons {
			fmt.Printf("  Degraded: %s\n", reason)
		}
		for _, warning := range transferPlan.Warnings {
			fmt.Printf("  Warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
}
