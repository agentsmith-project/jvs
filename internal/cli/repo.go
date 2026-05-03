package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	jvsrepo "github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/repoclone"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

var (
	repoCloneSavePoints        string
	repoCloneDryRun            bool
	repoCloneTargetControlRoot string
	repoCloneTargetPayloadRoot string
	repoMoveRunID              string
	repoRenameRunID            string
	repoDetachRunID            string
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage JVS projects",
}

var repoCloneCmd = &cobra.Command{
	Use:   "clone [target-folder]",
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
		separatedSource := ctx.Separated != nil || ctx.Repo.Mode == jvsrepo.RepoModeSeparatedControl
		if repoCloneSplitTargetRequested() && ctx.Separated == nil {
			return errclass.ErrUsage.WithMessage("--target-control-root requires an explicit source selected with --control-root and --workspace")
		}
		if separatedSource && !repoCloneSplitTargetRequested() {
			return separatedCloneExplicitTargetRequiredError()
		}
		targetPath := ""
		if len(args) == 1 {
			targetPath = args[0]
		}
		savePointsMode := repoclone.SavePointsMode(repoCloneSavePoints)
		if ctx.Separated != nil && !cmd.Flags().Changed("save-points") {
			savePointsMode = repoclone.SavePointsModeMain
		}
		result, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot:    ctx.Repo.Root,
			TargetPath:        targetPath,
			TargetControlRoot: repoCloneTargetControlRoot,
			TargetPayloadRoot: repoCloneTargetPayloadRoot,
			SavePointsMode:    savePointsMode,
			DryRun:            repoCloneDryRun,
			RequestedEngine:   requestedTransferEngine(ctx.Repo.Root),
		})
		if err != nil {
			return err
		}
		targetRepoRoot := result.TargetRepoRoot
		if result.TargetControlRoot != "" {
			targetRepoRoot = result.TargetControlRoot
		}
		recordResolvedTarget(targetRepoRoot, "main")
		if jsonOutput {
			return outputJSON(result)
		}
		printRepoCloneResult(result)
		return nil
	},
}

var repoMoveCmd = &cobra.Command{
	Use:   "move <new-folder> | move --run <plan-id>",
	Short: "Move a JVS project folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "repo move"); err != nil {
			return err
		}
		runPlanID, runRequested, err := workspaceRunFlag(cmd, "repo move")
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("repo move --run accepts only a plan ID")
			}
			result, err := executeRepoMovePlan(ctx.Repo.Root, runPlanID, "move")
			if err != nil {
				return err
			}
			recordResolvedTarget(result.RepoRoot, "main")
			if jsonOutput {
				return outputJSON(result)
			}
			printRepoMoveRunResult(result)
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("repo move requires a new folder")
		}
		plan, err := createRepoMovePlan(ctx.Repo.Root, args[0], repoMoveCommandOperation("move"), "move")
		if err != nil {
			return err
		}
		recordResolvedTarget(plan.SourceRepoRoot, "main")
		result := publicRepoMovePreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printRepoMovePreviewResult(result)
		return nil
	},
}

var repoRenameCmd = &cobra.Command{
	Use:   "rename <new-folder-name> | rename --run <plan-id>",
	Short: "Rename a JVS project folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "repo rename"); err != nil {
			return err
		}
		runPlanID, runRequested, err := workspaceRunFlag(cmd, "repo rename")
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("repo rename --run accepts only a plan ID")
			}
			result, err := executeRepoMovePlan(ctx.Repo.Root, runPlanID, "rename")
			if err != nil {
				return err
			}
			recordResolvedTarget(result.RepoRoot, "main")
			if jsonOutput {
				return outputJSON(result)
			}
			printRepoMoveRunResult(result)
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("repo rename requires a new folder name")
		}
		target, err := repoRenameTarget(ctx.Repo.Root, args[0])
		if err != nil {
			return err
		}
		plan, err := createRepoMovePlan(ctx.Repo.Root, target, repoMoveCommandOperation("rename"), "rename")
		if err != nil {
			return err
		}
		recordResolvedTarget(plan.SourceRepoRoot, "main")
		result := publicRepoMovePreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printRepoMovePreviewResult(result)
		return nil
	},
}

var repoDetachCmd = &cobra.Command{
	Use:   "detach | detach --run <plan-id>",
	Short: "Stop JVS managing the current project folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		runPlanID, runRequested, err := workspaceRunFlag(cmd, "repo detach")
		if err != nil {
			return err
		}
		if runRequested {
			if len(args) != 0 {
				return fmt.Errorf("repo detach --run accepts only a plan ID")
			}
			if err := rejectSeparatedLifecycleRunTarget("repo detach"); err != nil {
				return err
			}
			result, err := executeRepoDetachRunFromCWD(runPlanID)
			if err != nil {
				return err
			}
			if jsonOutput {
				return outputJSON(result)
			}
			printRepoDetachRunResult(result)
			return nil
		}
		if len(args) != 0 {
			return fmt.Errorf("repo detach does not accept arguments")
		}
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		if err := rejectSeparatedLifecycleCommand(ctx, "repo detach"); err != nil {
			return err
		}
		plan, err := createRepoDetachPlan(ctx.Repo.Root)
		if err != nil {
			return err
		}
		recordResolvedTarget(plan.RepoRoot, "main")
		result := publicRepoDetachPreviewFromPlan(plan)
		if jsonOutput {
			return outputJSON(result)
		}
		printRepoDetachPreviewResult(result)
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
	if repoCloneSplitTargetRequested() {
		if len(args) > 1 {
			return errclass.ErrUsage.WithMessage("repo clone accepts exactly one target folder")
		}
		if strings.TrimSpace(repoCloneTargetControlRoot) == "" {
			return errclass.ErrUsage.WithMessage("--target-payload-root requires --target-control-root")
		}
		if len(args) == 1 && strings.TrimSpace(repoCloneTargetPayloadRoot) != "" {
			return errclass.ErrUsage.WithMessage("repo clone target folder cannot be combined with --target-payload-root")
		}
		if len(args) == 0 && strings.TrimSpace(repoCloneTargetPayloadRoot) == "" {
			return errclass.ErrUsage.WithMessage("repo clone with --target-control-root requires a target folder")
		}
		if repoclone.IsRemoteLikeInput(repoCloneTargetControlRoot) ||
			repoclone.IsRemoteLikeInput(repoCloneTargetPayloadRoot) ||
			(len(args) == 1 && repoclone.IsRemoteLikeInput(args[0])) {
			return repoCloneRemoteInputError()
		}
		return nil
	}
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

func repoCloneSplitTargetRequested() bool {
	return strings.TrimSpace(repoCloneTargetControlRoot) != "" || strings.TrimSpace(repoCloneTargetPayloadRoot) != ""
}

func separatedCloneExplicitTargetRequiredError() error {
	return errclass.ErrExplicitTargetRequired.WithMessage("control data is outside the folder; repo clone requires --target-control-root for the target")
}

func repoCloneRemoteInputError() error {
	return errclass.ErrUsage.
		WithMessage("JVS repo clone only copies a local or mounted JVS project.").
		WithHint("Remote URLs and git-style sources are not supported. Use a local path with --repo, then provide the target folder.")
}

func rejectSeparatedLifecycleCommand(ctx *cliDiscoveryContext, command string) error {
	if ctx == nil || ctx.Repo == nil {
		return nil
	}
	if ctx.Separated == nil && ctx.Repo.Mode != jvsrepo.RepoModeSeparatedControl {
		return nil
	}
	return separatedLifecycleUnsupportedError(command)
}

func rejectSeparatedLifecycleRunTarget(command string) error {
	if targetControlRoot != "" {
		ctx, err := resolveRepoScoped()
		if err != nil {
			return err
		}
		return rejectSeparatedLifecycleCommand(ctx, command)
	}
	if targetRepoPath != "" {
		r, err := resolveRepoFlagTarget(targetRepoPath)
		if err != nil {
			return err
		}
		return rejectSeparatedLifecycleCommand(&cliDiscoveryContext{Repo: r}, command)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return errclass.ErrUsage.WithMessagef("cannot get current directory: %v", err)
	}
	if r, err := jvsrepo.DiscoverControlRepo(cwd); err == nil && r.Mode == jvsrepo.RepoModeSeparatedControl {
		recordResolvedTarget(r.Root, "")
		return separatedLifecycleUnsupportedError(command)
	}
	return nil
}

func separatedLifecycleUnsupportedError(command string) error {
	return errclass.ErrSeparatedLifecycleUnsupported.
		WithMessagef("%s is not supported when control data is outside the folder yet. No files changed.", command).
		WithHint("Repo and workspace lifecycle commands are disabled for external control roots until control data and folder lifecycle semantics are redesigned.")
}

func printRepoCloneResult(result *repoclone.Result) {
	if result.DryRun {
		printRepoCloneDryRun(result)
		return
	}
	fmt.Println("Cloned JVS project")
	fmt.Printf("Source: %s\n", result.SourceRepoRoot)
	if result.TargetControlRoot != "" {
		fmt.Printf("Target folder: %s\n", result.TargetFolder)
		fmt.Printf("Target control data: %s\n", result.TargetControlRoot)
	} else {
		fmt.Printf("Target: %s\n", result.TargetRepoRoot)
	}
	fmt.Printf("Save points copied: %s (%d)\n", repoCloneModeLabel(result.SavePointsMode), result.SavePointsCopiedCount)
	fmt.Println("Workspaces created: main only")
	fmt.Printf("Source workspaces not created: %s\n", repoCloneSkippedWorkspaces(result.SourceWorkspacesNotCreated))
	printRepoCloneTransferSummary("Save point storage", repoCloneTransferByID(result.Transfers, "repo-clone-save-points"), false)
	printRepoCloneTransferSummary("Main workspace", repoCloneTransferByID(result.Transfers, "repo-clone-main-workspace"), false)
}

func printRepoCloneDryRun(result *repoclone.Result) {
	fmt.Println("Repo clone dry run")
	fmt.Printf("Source: %s\n", result.SourceRepoRoot)
	if result.TargetControlRoot != "" {
		fmt.Printf("Target folder: %s\n", result.TargetFolder)
		fmt.Printf("Target control data: %s\n", result.TargetControlRoot)
	} else {
		fmt.Printf("Target: %s\n", result.TargetRepoRoot)
	}
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

func printRepoMovePreviewResult(result publicRepoMovePreviewResult) {
	fmt.Printf("%s preview\n", publicRepoMovePrintLabel(result.Operation))
	fmt.Printf("Old folder: %s\n", result.SourceRepoRoot)
	fmt.Printf("New folder: %s\n", result.TargetRepoRoot)
	fmt.Printf("Repo ID unchanged: %s\n", result.RepoID)
	fmt.Printf("Move method: %s\n", result.MoveMethod)
	fmt.Printf("External workspace connections to update: %d\n", result.ExternalWorkspaces)
	fmt.Printf("Plan: %s\n", result.PlanID)
	fmt.Println("No files were moved.")
	fmt.Printf("Run: `%s`\n", result.RunCommand)
	fmt.Printf("Safe run: `%s`\n", result.SafeRunCommand)
}

func printRepoMoveRunResult(result publicRepoMoveRunResult) {
	fmt.Printf("Moved JVS project.\n")
	fmt.Printf("Old folder: %s\n", result.SourceRepoRoot)
	fmt.Printf("New folder: %s\n", result.TargetRepoRoot)
	fmt.Printf("Repo ID unchanged: %s\n", result.RepoID)
	fmt.Printf("Updated workspace connections: %d\n", result.ExternalWorkspacesUpdated)
}

func printRepoDetachPreviewResult(result publicRepoDetachPreviewResult) {
	fmt.Println("Repo detach preview")
	fmt.Printf("Folder: %s\n", result.RepoRoot)
	fmt.Printf("Repo ID: %s\n", result.RepoID)
	fmt.Printf("Archive: %s\n", result.ArchivePath)
	fmt.Printf("External workspace connections to detach: %d\n", result.ExternalWorkspaces)
	fmt.Printf("Plan: %s\n", result.PlanID)
	fmt.Println("No files were moved.")
	fmt.Println("No JVS metadata was archived.")
	fmt.Printf("Run: `%s`\n", result.RunCommand)
}

func printRepoDetachRunResult(result publicRepoDetachRunResult) {
	fmt.Println("Detached JVS project.")
	fmt.Printf("Folder: %s\n", result.RepoRoot)
	fmt.Printf("Archive: %s\n", result.ArchivePath)
	fmt.Printf("Detached workspace connections: %d\n", result.ExternalWorkspacesUpdated)
	fmt.Println("Working files preserved: yes")
	fmt.Println("Save point storage deleted: no")
	fmt.Println("Current folder is no longer an active JVS repo.")
}

func init() {
	repoCloneCmd.Flags().StringVar(&repoCloneSavePoints, "save-points", string(repoclone.SavePointsModeAll), "save points to copy: all or main; default all for ordinary clone; when control data is outside the target folder, external-control clone defaults to main; --save-points all fails closed for external control roots")
	repoCloneCmd.Flags().BoolVar(&repoCloneDryRun, "dry-run", false, "plan the clone without creating the target")
	repoCloneCmd.Flags().StringVar(&repoCloneTargetControlRoot, "target-control-root", "", "target external control data root")
	repoCloneCmd.Flags().StringVar(&repoCloneTargetPayloadRoot, "target-payload-root", "", "target folder for external-control clone")
	repoCloneCmd.Flags().Lookup("target-payload-root").Hidden = true
	repoMoveCmd.Flags().StringVar(&repoMoveRunID, "run", "", "execute a repo move preview plan")
	repoRenameCmd.Flags().StringVar(&repoRenameRunID, "run", "", "execute a repo rename preview plan")
	repoDetachCmd.Flags().StringVar(&repoDetachRunID, "run", "", "execute a repo detach preview plan")
	repoCmd.AddCommand(repoCloneCmd)
	repoCmd.AddCommand(repoMoveCmd)
	repoCmd.AddCommand(repoRenameCmd)
	repoCmd.AddCommand(repoDetachCmd)
	rootCmd.AddCommand(repoCmd)
}
