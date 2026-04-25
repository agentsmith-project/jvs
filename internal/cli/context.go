package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/color"
	"github.com/jvs-project/jvs/pkg/errclass"
)

var (
	targetRepoPath      string
	targetWorkspaceName string
)

type cliDiscoveryContext struct {
	Repo      *repo.Repo
	Workspace string
}

// requireRepo discovers the repo from CWD and returns it, or exits with error.
func requireRepo() *repo.Repo {
	r, err := discoverRequiredRepo()
	if err != nil {
		exitWithCLIError(err)
	}
	return r
}

// requireWorktree discovers the repo and worktree from CWD, or exits with error.
func requireWorktree() (*repo.Repo, string) {
	r, wtName, err := discoverRequiredWorktree()
	if err != nil {
		exitWithCLIError(err)
	}
	return r, wtName
}

func discoverRequiredRepo() (*repo.Repo, error) {
	ctx, err := resolveRepoScoped()
	if err != nil {
		return nil, err
	}
	return ctx.Repo, nil
}

func discoverRequiredWorktree() (*repo.Repo, string, error) {
	ctx, err := resolveWorkspaceScoped()
	if err != nil {
		return nil, "", err
	}
	return ctx.Repo, ctx.Workspace, nil
}

func discoverOptionalWorktree() (*repo.Repo, string, error) {
	ctx, err := resolveRepoScoped()
	if err != nil {
		return nil, "", err
	}
	return ctx.Repo, ctx.Workspace, nil
}

// resolveRepoScoped resolves commands that only require a repository.
func resolveRepoScoped() (*cliDiscoveryContext, error) {
	repoStart, workspaceStart, err := discoveryStarts()
	if err != nil {
		return nil, err
	}

	r, err := repo.Discover(repoStart)
	if err != nil {
		return nil, errclass.ErrNotRepo.
			WithMessage("not a JVS repository (or any parent)").
			WithHint(suggestInit())
	}
	recordResolvedTarget(r.Root, "")

	workspace, err := resolveOptionalWorkspace(r, workspaceStart)
	if err != nil {
		return nil, err
	}

	recordResolvedTarget(r.Root, workspace)
	return &cliDiscoveryContext{Repo: r, Workspace: workspace}, nil
}

// resolveWorkspaceScoped resolves commands that require both repository and workspace.
func resolveWorkspaceScoped() (*cliDiscoveryContext, error) {
	ctx, err := resolveRepoScoped()
	if err != nil {
		return nil, err
	}
	if ctx.Workspace == "" {
		return nil, errclass.ErrNotWorkspace.
			WithMessage("not inside a workspace").
			WithHint("Run from main/ or worktrees/<name>, or pass --workspace <name>.")
	}
	workspace, err := resolveNamedWorkspace(ctx.Repo.Root, ctx.Workspace)
	if err != nil {
		return nil, err
	}
	ctx.Workspace = workspace
	recordResolvedTarget(ctx.Repo.Root, ctx.Workspace)
	return ctx, nil
}

func discoveryStarts() (repoStart string, workspaceStart string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", errclass.ErrUsage.WithMessagef("cannot get current directory: %v", err)
	}

	workspaceStart = cwd
	repoStart = cwd
	if targetRepoPath != "" {
		path, err := filepath.Abs(targetRepoPath)
		if err != nil {
			return "", "", errclass.ErrUsage.WithMessagef("resolve --repo: %v", err)
		}
		if info, err := os.Stat(path); err != nil {
			return "", "", errclass.ErrNotRepo.
				WithMessagef("not a JVS repository: %s", targetRepoPath).
				WithHint(suggestInit())
		} else if !info.IsDir() {
			return "", "", errclass.ErrNotRepo.
				WithMessagef("--repo must be a directory: %s", targetRepoPath).
				WithHint(suggestInit())
		}
		repoStart = path
	}

	return repoStart, workspaceStart, nil
}

func resolveOptionalWorkspace(r *repo.Repo, start string) (string, error) {
	if targetWorkspaceName != "" {
		return resolveNamedWorkspace(r.Root, targetWorkspaceName)
	}

	return workspaceFromPath(r.Root, start)
}

func workspaceFromPath(repoRoot, path string) (string, error) {
	r, workspace, err := repo.DiscoverWorktree(path)
	if err != nil || workspace == "" {
		return "", nil
	}
	if filepath.Clean(r.Root) != filepath.Clean(repoRoot) {
		return "", errclass.ErrTargetMismatch.WithMessagef(
			"targeting mismatch: --repo resolves to %s, but current workspace %q belongs to %s",
			filepath.Clean(repoRoot), workspace, filepath.Clean(r.Root),
		)
	}
	return workspace, nil
}

func resolveNamedWorkspace(repoRoot, name string) (string, error) {
	if _, err := repo.LoadWorktreeConfig(repoRoot, name); err != nil {
		return "", errclass.ErrNotWorkspace.
			WithMessagef("workspace %q not found", name).
			WithHint("Run jvs workspace list to see available workspaces.")
	}
	return name, nil
}

func fmtErr(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if jsonOutput {
		emitJSONErrorOnce(errclass.ErrUsage.WithMessage(message))
		return
	}
	printHumanError(message, "")
}

func exitWithCLIError(err error) {
	if jsonOutput {
		emitJSONErrorOnce(err)
		os.Exit(1)
	}
	printCLIError(err)
	os.Exit(1)
}

func emitJSONErrorOnce(err error) {
	if jsonErrorEmitted {
		return
	}
	_ = outputJSONError(err)
	jsonErrorEmitted = true
}

func printCLIError(err error) {
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		if jvsErr.Code == errclass.ErrNotRepo.Code {
			fmt.Fprintln(os.Stderr, formatNotInRepositoryError())
			return
		}
		message := jvsErr.Message
		if message == "" {
			message = jvsErr.Code
		}
		printHumanError(message, jvsErr.Hint)
		return
	}
	printHumanError(err.Error(), "")
}

func printHumanError(message, hint string) {
	// Colorize the error prefix
	prefix := "jvs: "
	if color.Enabled() {
		prefix = color.Error("jvs:") + " "
	}
	fmt.Fprintln(os.Stderr, prefix+message)
	if hint != "" {
		fmt.Fprintln(os.Stderr, color.Dim("  hint: "+hint))
	}
}
