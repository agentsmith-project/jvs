package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var (
	restoreInteractive    bool
	restoreDiscardDirty   bool
	restoreIncludeWorking bool
)

var restoreCmd = &cobra.Command{
	Use:   "restore <save-point>",
	Short: "Restore managed files to a save point",
	Long: `Restore managed files in the active folder to a save point.

The workspace history is not changed by restore. If the folder has unsaved
changes, save them first or discard them explicitly.

Examples:
  jvs restore 1771589abc
  jvs restore 1771589abc --save-first
  jvs restore 1771589abc --discard-unsaved`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		targetID, err := resolvePublicSavePointID(r.Root, args[0])
		if err != nil {
			return restorePointError(err)
		}

		if err := rejectUnsavedChangesForRestore(r.Root, workspaceName); err != nil {
			return restorePointError(err)
		}
		if restoreIncludeWorking {
			if _, err := createSavePointDescriptor(r.Root, workspaceName, "save before restore"); err != nil {
				return restorePointError(err)
			}
		}

		if err := restore.NewRestorer(r.Root, detectEngine(r.Root)).Restore(workspaceName, targetID); err != nil {
			return restorePointError(err)
		}

		result, err := publicRestoreStatus(r.Root, workspaceName, targetID)
		if err != nil {
			return restorePointError(err)
		}
		if jsonOutput {
			return outputJSON(result)
		}

		printRestoreResult(result)
		return nil
	},
}

type publicRestoreResult struct {
	Folder            string  `json:"folder"`
	Workspace         string  `json:"workspace"`
	RestoredSavePoint string  `json:"restored_save_point"`
	NewestSavePoint   *string `json:"newest_save_point"`
	HistoryHead       *string `json:"history_head"`
	ContentSource     *string `json:"content_source"`
	UnsavedChanges    bool    `json:"unsaved_changes"`
	FilesState        string  `json:"files_state"`
	HistoryChanged    bool    `json:"history_changed"`
}

func rejectUnsavedChangesForRestore(repoRoot, workspaceName string) error {
	if restoreDiscardDirty && restoreIncludeWorking {
		return fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
	}
	unsavedChanges, err := workspaceDirty(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if !unsavedChanges || restoreDiscardDirty || restoreIncludeWorking {
		return nil
	}
	return fmt.Errorf("folder has unsaved changes; use --save-first to save them before restore or --discard-unsaved to discard them. No files were changed.")
}

func publicRestoreStatus(repoRoot, workspaceName string, restoredID model.SnapshotID) (publicRestoreResult, error) {
	status, err := buildWorkspaceStatus(repoRoot, workspaceName)
	if err != nil {
		return publicRestoreResult{}, err
	}
	return publicRestoreResult{
		Folder:            status.Folder,
		Workspace:         status.Workspace,
		RestoredSavePoint: string(restoredID),
		NewestSavePoint:   status.NewestSavePoint,
		HistoryHead:       status.HistoryHead,
		ContentSource:     status.ContentSource,
		UnsavedChanges:    status.UnsavedChanges,
		FilesState:        status.FilesState,
		HistoryChanged:    false,
	}, nil
}

func printRestoreResult(result publicRestoreResult) {
	restored := color.SnapshotID(result.RestoredSavePoint)
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Restored save point: %s\n", restored)
	fmt.Printf("Managed files now match save point %s.\n", restored)
	if result.NewestSavePoint != nil && *result.NewestSavePoint != result.RestoredSavePoint {
		newest := color.SnapshotID(*result.NewestSavePoint)
		fmt.Printf("Newest save point is still %s.\n", newest)
		fmt.Println("History was not changed.")
		fmt.Printf("Next save creates a new save point after %s.\n", newest)
		return
	}
	fmt.Printf("Newest save point: %s\n", formatStatusSavePoint(result.NewestSavePoint))
	fmt.Println("History was not changed.")
}

func restorePointError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", restorePointVocabulary(err.Error()))
}

func restorePointVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"--discard-dirty", "--discard-unsaved",
		"--include-working", "--save-first",
		"dirty changes", "unsaved changes",
		"dirty", "unsaved",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"current", "source",
		"latest", "newest",
		"HEAD", "source",
		"head", "source",
		"detached", "restored",
		"fork", "save",
	)
	return replacer.Replace(value)
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreInteractive, "interactive", "i", false, "interactive confirmation")
	restoreCmd.Flags().Lookup("interactive").Hidden = true
	restoreCmd.Flags().BoolVar(&restoreDiscardDirty, "discard-unsaved", false, "discard unsaved folder changes for this operation")
	restoreCmd.Flags().BoolVar(&restoreIncludeWorking, "save-first", false, "create a save point for unsaved changes before restore")
	restoreCmd.Flags().BoolVar(&restoreDiscardDirty, "discard-dirty", false, "discard dirty workspace changes for this operation")
	restoreCmd.Flags().Lookup("discard-dirty").Hidden = true
	restoreCmd.Flags().BoolVar(&restoreIncludeWorking, "include-working", false, "checkpoint dirty workspace changes before this operation")
	restoreCmd.Flags().Lookup("include-working").Hidden = true
	rootCmd.AddCommand(restoreCmd)
}
