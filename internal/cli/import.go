package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jvs-project/jvs/internal/engine"
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
		eng := newCloneEngine(r.Root)
		transferResult, transferEngine, err := cloneDirectory(source, mainWorkspace, eng)
		if err != nil {
			return fmt.Errorf("copy source into main workspace: %w", err)
		}

		note := fmt.Sprintf("initial import from %s", source)
		desc, err := createInitialCheckpoint(r.Root, note, []string{"import"})
		if err != nil {
			return fmt.Errorf("create initial checkpoint: %w", err)
		}
		capabilities, err := engine.ProbeCapabilities(r.Root, false)
		if err != nil {
			return fmt.Errorf("probe capabilities: %w", err)
		}

		output := map[string]any{
			"repo_root":          r.Root,
			"main_workspace":     mainWorkspace,
			"provenance":         source,
			"initial_checkpoint": desc.SnapshotID,
			"engine":             desc.Engine,
			"transfer_mode":      effectiveTransferMode(transferEngine, transferResult),
			"degraded_reasons":   degradedReasons(transferResult),
			"capabilities":       capabilities,
		}
		if jsonOutput {
			return outputJSON(output)
		}

		fmt.Printf("Imported directory into JVS repository\n")
		fmt.Printf("  Repo root: %s\n", r.Root)
		fmt.Printf("  Main workspace: %s\n", mainWorkspace)
		fmt.Printf("  Provenance: %s\n", source)
		fmt.Printf("  Initial checkpoint: %s\n", desc.SnapshotID)
		fmt.Printf("  Engine: %s\n", desc.Engine)
		fmt.Printf("  Transfer mode: %s\n", output["transfer_mode"])
		for _, reason := range degradedReasons(transferResult) {
			fmt.Printf("  Degraded: %s\n", reason)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
}
