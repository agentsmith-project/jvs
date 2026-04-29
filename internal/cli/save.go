package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var saveMessage string

var saveCmd = &cobra.Command{
	Use:   "save [-m message]",
	Short: "Create a save point",
	Long: `Create a save point for the active workspace.

Examples:
  jvs save -m "baseline"
  jvs save --message "before cleanup"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}

		message, err := savePointMessage(args)
		if err != nil {
			return err
		}

		desc, err := createSavePointDescriptor(r.Root, workspaceName, message)
		if err != nil {
			return savePointError(err)
		}

		unsavedChanges, err := workspaceDirty(r.Root, workspaceName)
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSON(publicSavePointCreated(desc, unsavedChanges))
		}

		fmt.Printf("Saved save point %s\n", color.SnapshotID(desc.SnapshotID.String()))
		fmt.Printf("Workspace: %s\n", workspaceName)
		if message != "" {
			fmt.Printf("Message: %s\n", message)
		}
		if desc.RestoredFrom != nil {
			fmt.Printf("Created from restored save point %s\n", color.SnapshotID(desc.RestoredFrom.String()))
		}
		if desc.StartedFrom != nil {
			fmt.Printf("Started from save point %s\n", color.SnapshotID(desc.StartedFrom.String()))
		}
		for _, restoredPath := range desc.RestoredPaths {
			fmt.Printf("Includes restored path %s from save point %s.\n",
				restoredPath.TargetPath,
				color.SnapshotID(restoredPath.SourceSnapshotID.String()),
			)
		}
		fmt.Printf("Newest save point: %s\n", color.SnapshotID(desc.SnapshotID.String()))
		if unsavedChanges {
			fmt.Println("Unsaved changes: yes")
		} else {
			fmt.Println("Unsaved changes: no")
		}
		return nil
	},
}

func createSavePointDescriptor(repoRoot, workspaceName, message string) (*model.Descriptor, error) {
	var desc *model.Descriptor
	err := repo.WithMutationLock(repoRoot, "save", func() error {
		if err := checkSaveCapacity(repoRoot, workspaceName); err != nil {
			return err
		}
		var err error
		desc, err = snapshot.NewCreator(repoRoot, detectEngine(repoRoot)).CreateSavePointLocked(workspaceName, message, nil)
		return err
	})
	return desc, err
}

func savePointMessage(args []string) (string, error) {
	messageFromFlag := strings.TrimSpace(saveMessage)
	if messageFromFlag != "" && len(args) > 0 {
		return "", fmt.Errorf("provide a save point message with either -m/--message or a positional message, not both")
	}
	if messageFromFlag != "" {
		return messageFromFlag, nil
	}
	if len(args) > 0 {
		return strings.TrimSpace(args[0]), nil
	}
	return "", fmt.Errorf("save point message is required; use -m \"baseline\"")
}

func savePointError(err error) error {
	if err == nil {
		return nil
	}
	message := publicSavePointVocabulary(err.Error())
	if !fsutil.IsCommitUncertain(err) {
		message = appendSaveFailureHistoryUnchangedMessage(message)
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return &errclass.JVSError{Code: jvsErr.Code, Message: message, Hint: publicSavePointVocabulary(jvsErr.Hint)}
	}
	return fmt.Errorf("%s", message)
}

func appendSaveFailureHistoryUnchangedMessage(message string) string {
	if !strings.Contains(message, "No save point was created.") {
		message = appendSaveFailureSentence(message, "No save point was created.")
	}
	if !strings.Contains(message, "History was not changed.") {
		message = appendSaveFailureSentence(message, "History was not changed.")
	}
	return message
}

func appendSaveFailureSentence(message, sentence string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return sentence
	}
	if strings.HasSuffix(message, ".") || strings.HasSuffix(message, "!") || strings.HasSuffix(message, "?") {
		return message + " " + sentence
	}
	return message + ". " + sentence
}

func publicSavePointVocabulary(value string) string {
	if value == "" {
		return value
	}
	replacer := strings.NewReplacer(
		"current differs from latest",
		"is not at the newest save point",
		"checkpointing",
		"saving",
		"checkpoints",
		"save points",
		"checkpoint",
		"save point",
		"snapshots",
		"save points",
		"snapshot",
		"save point",
		"worktrees",
		"workspaces",
		"worktree",
		"workspace",
		"detached",
		"not at the newest save point",
		"latest",
		"newest save point",
		"current",
		"selected save point",
		"head",
		"selected save point",
		"fork",
		"workspace new",
		"commit",
		"save",
	)
	return replacer.Replace(value)
}

func init() {
	saveCmd.Flags().StringVarP(&saveMessage, "message", "m", "", "message for this save point")
	rootCmd.AddCommand(saveCmd)
}
