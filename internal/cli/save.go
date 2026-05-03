package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
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
		ctx, err := resolveWorkspaceScoped()
		if err != nil {
			return err
		}

		message, err := savePointMessage(args)
		if err != nil {
			return err
		}

		if err := validateSeparatedPayloadSymlinkBoundary(ctx.Separated); err != nil {
			return savePointError(err)
		}

		desc, transferRecord, err := createSavePointDescriptor(ctx.Repo.Root, ctx.Workspace, message, ctx.Separated)
		if err != nil {
			return savePointError(err)
		}

		unsavedChanges, err := workspaceDirty(ctx.Repo.Root, ctx.Workspace)
		if err != nil {
			return err
		}

		if jsonOutput {
			return outputJSONWithSeparatedControl(
				publicSavePointCreated(desc, unsavedChanges, transferDataFromRecord(transferRecord)),
				ctx.Separated,
				separatedDoctorStrictNotRun,
			)
		}

		fmt.Printf("Saved save point %s\n", color.SnapshotID(desc.SnapshotID.String()))
		fmt.Printf("Workspace: %s\n", ctx.Workspace)
		if message != "" {
			fmt.Printf("Message: %s\n", message)
		}
		printPrimaryTransferSummary(transferRecord)
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

func createSavePointDescriptor(repoRoot, workspaceName, message string, separated *repo.SeparatedContext) (*model.Descriptor, *transfer.Record, error) {
	var desc *model.Descriptor
	var transferRecord *transfer.Record
	err := repo.WithMutationLock(repoRoot, "save", func() error {
		if err := validateSeparatedPayloadSymlinkBoundary(separated); err != nil {
			return err
		}
		if err := enforceSeparatedRecoveryMutationGuard(repoRoot, workspaceName, separated, "save"); err != nil {
			return err
		}
		if err := checkSaveCapacity(repoRoot, workspaceName); err != nil {
			return err
		}
		if err := validateSeparatedPayloadSymlinkBoundary(separated); err != nil {
			return err
		}
		var err error
		creator := snapshot.NewCreator(repoRoot, requestedTransferEngine(repoRoot))
		desc, err = creator.CreateSavePointLocked(workspaceName, message, nil)
		if err != nil {
			return err
		}
		if record, ok := creator.LastTransferRecord(); ok {
			transferRecord = &record
		}
		return err
	})
	return desc, transferRecord, err
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
