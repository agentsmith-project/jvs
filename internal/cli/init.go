package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
)

var initCmd = &cobra.Command{
	Use:   "init [folder]",
	Short: "Initialize JVS for a folder",
	Long: `Initialize JVS control data for a folder.

When no folder is provided, the current directory is adopted. Existing files stay
in place; JVS stores control data in .jvs/ and registers the folder as the main
workspace.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder := "."
		if len(args) > 0 {
			folder = args[0]
		}

		createdFolders, err := ensureInitFolderExists(folder)
		if err != nil {
			return err
		}
		r, err := repo.InitAdoptedWorkspace(folder)
		if err != nil {
			cleanupCreatedInitFolders(createdFolders)
			return fmt.Errorf("failed to initialize folder: %w", err)
		}
		capabilities, err := engine.ProbeCapabilities(r.Root, true)
		if err != nil {
			return fmt.Errorf("probe capabilities: %w", err)
		}

		if jsonOutput {
			output := map[string]any{
				"folder":            r.Root,
				"workspace":         "main",
				"repo_root":         r.Root,
				"format_version":    r.FormatVersion,
				"repo_id":           r.RepoID,
				"newest_save_point": nil,
				"unsaved_changes":   true,
			}
			applySetupJSONFields(output, capabilities, capabilities.RecommendedEngine, capabilities.Warnings)
			return outputJSON(output)
		}

		fmt.Printf("Folder: %s\n", r.Root)
		fmt.Println("Workspace: main")
		fmt.Println("JVS is ready for this folder.")
		fmt.Println("Files were not moved or copied.")
		fmt.Println("Newest save point: none")
		fmt.Println("Unsaved changes: yes")
		fmt.Println("Next: jvs save -m \"baseline\"")
		fmt.Printf("Capabilities: write=%s juicefs=%t reflink=%s copy=%t recommended=%s\n",
			capabilities.Write.Confidence,
			capabilities.JuiceFS.Supported,
			capabilities.Reflink.Confidence,
			capabilities.Copy.Supported,
			capabilities.RecommendedEngine,
		)
		for _, warning := range capabilities.Warnings {
			fmt.Printf("Warning: %s\n", warning)
		}
		return nil
	},
}

func ensureInitFolderExists(folder string) ([]string, error) {
	target, err := filepath.Abs(folder)
	if err != nil {
		return nil, fmt.Errorf("resolve folder: %w", err)
	}
	target = filepath.Clean(target)

	var created []string
	for current := target; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat folder: %w", err)
		}
		created = append(created, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}

	if len(created) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		cleanupCreatedInitFolders(created)
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return created, nil
}

func cleanupCreatedInitFolders(created []string) {
	for i, dir := range created {
		if i == 0 {
			_ = os.RemoveAll(dir)
			continue
		}
		_ = os.Remove(dir)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
}
