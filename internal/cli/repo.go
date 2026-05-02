package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/repoclone"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

var (
	repoCloneSavePoints string
	repoCloneDryRun     bool
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage JVS projects",
}

var repoCloneCmd = &cobra.Command{
	Use:   "clone <target-folder>",
	Short: "Clone a local JVS project",
	Args:  validateRepoCloneArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if repoclone.IsRemoteLikeInput(targetRepoPath) {
			return repoCloneRemoteInputError()
		}
		ctx, err := resolveRepoCloneSource()
		if err != nil {
			return err
		}
		result, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot:  ctx.Repo.Root,
			TargetPath:      args[0],
			SavePointsMode:  repoclone.SavePointsMode(repoCloneSavePoints),
			DryRun:          repoCloneDryRun,
			RequestedEngine: requestedTransferEngine(ctx.Repo.Root),
		})
		if err != nil {
			return err
		}
		recordResolvedTarget(result.TargetRepoRoot, "main")
		if jsonOutput {
			return outputJSON(result)
		}
		printRepoCloneResult(result)
		return nil
	},
}

func resolveRepoCloneSource() (*cliDiscoveryContext, error) {
	if targetRepoPath == "" {
		return resolveRepoScoped()
	}
	sourceRepo, err := resolveRepoFlagTarget(targetRepoPath)
	if err != nil {
		return nil, err
	}
	recordResolvedTarget(sourceRepo.Root, "")
	return &cliDiscoveryContext{Repo: sourceRepo}, nil
}

func validateRepoCloneArgs(cmd *cobra.Command, args []string) error {
	switch len(args) {
	case 0:
		return errclass.ErrUsage.WithMessage("repo clone requires a target folder")
	case 1:
		if repoclone.IsRemoteLikeInput(args[0]) {
			return repoCloneRemoteInputError()
		}
		return nil
	default:
		return errclass.ErrUsage.WithMessage("repo clone accepts exactly one target folder")
	}
}

func repoCloneRemoteInputError() error {
	return errclass.ErrUsage.
		WithMessage("JVS repo clone only copies a local or mounted JVS project.").
		WithHint("Remote URLs and git-style sources are not supported. Use a local path with --repo, then provide the target folder.")
}

func printRepoCloneResult(result *repoclone.Result) {
	if result.DryRun {
		printRepoCloneDryRun(result)
		return
	}
	fmt.Println("Cloned JVS project")
	fmt.Printf("Source: %s\n", result.SourceRepoRoot)
	fmt.Printf("Target: %s\n", result.TargetRepoRoot)
	fmt.Printf("Save points copied: %s (%d)\n", repoCloneModeLabel(result.SavePointsMode), result.SavePointsCopiedCount)
	fmt.Println("Workspaces created: main only")
	fmt.Printf("Source workspaces not created: %s\n", repoCloneSkippedWorkspaces(result.SourceWorkspacesNotCreated))
	printRepoCloneTransferSummary("Save point storage", repoCloneTransferByID(result.Transfers, "repo-clone-save-points"), false)
	printRepoCloneTransferSummary("Main workspace", repoCloneTransferByID(result.Transfers, "repo-clone-main-workspace"), false)
	if result.DoctorStrict != "" {
		fmt.Printf("Doctor strict: %s\n", result.DoctorStrict)
	}
}

func printRepoCloneDryRun(result *repoclone.Result) {
	fmt.Println("Repo clone dry run")
	fmt.Printf("Source: %s\n", result.SourceRepoRoot)
	fmt.Printf("Target: %s\n", result.TargetRepoRoot)
	fmt.Printf("Save points to copy: %s (%d)\n", repoCloneModeLabel(result.SavePointsMode), result.SavePointsCopiedCount)
	fmt.Println("Workspaces that would be created: main only")
	fmt.Printf("Source workspaces that would not be created: %s\n", repoCloneSkippedWorkspaces(result.SourceWorkspacesNotCreated))
	printRepoCloneTransferSummary("Expected save point storage copy", repoCloneTransferByID(result.Transfers, "repo-clone-save-points"), true)
	printRepoCloneTransferSummary("Expected main workspace copy", repoCloneTransferByID(result.Transfers, "repo-clone-main-workspace"), true)
	fmt.Println("No files were created.")
}

func repoCloneModeLabel(mode repoclone.SavePointsMode) string {
	if mode == repoclone.SavePointsModeMain {
		return "main history closure"
	}
	return "all"
}

func repoCloneSkippedWorkspaces(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func repoCloneTransferByID(records []transfer.Record, id string) *transfer.Record {
	for i := range records {
		if records[i].TransferID == id {
			return &records[i]
		}
	}
	return nil
}

func printRepoCloneTransferSummary(label string, record *transfer.Record, expected bool) {
	if record == nil {
		return
	}
	fmt.Printf("%s: Copy method: %s\n", label, publicCopyMethod(record.PerformanceClass))
	if why := publicTransferWhy(*record); why != "" {
		fmt.Printf("Why: %s\n", why)
	}
	if record.CheckedForThisOperation {
		if expected {
			fmt.Println("Checked for this preview")
			return
		}
		fmt.Println("Checked for this operation")
	}
}

func init() {
	repoCloneCmd.Flags().StringVar(&repoCloneSavePoints, "save-points", string(repoclone.SavePointsModeAll), "save points to copy: all or main")
	repoCloneCmd.Flags().BoolVar(&repoCloneDryRun, "dry-run", false, "plan the clone without creating the target")
	repoCmd.AddCommand(repoCloneCmd)
	rootCmd.AddCommand(repoCmd)
}
