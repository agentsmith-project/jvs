package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

var (
	targetRepoPath      string
	targetControlRoot   string
	targetWorkspaceName string
)

type cliDiscoveryContext struct {
	Repo      *repo.Repo
	Workspace string
	Separated *repo.SeparatedContext
}

func discoverRequiredRepo() (*repo.Repo, error) {
	ctx, err := resolveRepoScoped()
	if err != nil {
		return nil, err
	}
	return ctx.Repo, nil
}

func discoverRequiredRepoForDoctor() (*repo.Repo, error) {
	ctx, err := resolveDoctorScoped()
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

// resolveRepoScoped resolves commands that only require a repository.
func resolveRepoScoped() (*cliDiscoveryContext, error) {
	if targetControlRoot != "" {
		return resolveSeparatedScoped()
	}

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
	if err := enforceLifecyclePendingGuard(r.Root); err != nil {
		return nil, err
	}
	return &cliDiscoveryContext{Repo: r, Workspace: workspace}, nil
}

// resolveDoctorScoped resolves doctor against an explicit repository even when
// the current external workspace locator is stale enough to need doctor repair.
func resolveDoctorScoped() (*cliDiscoveryContext, error) {
	if targetControlRoot != "" {
		return resolveSeparatedScoped()
	}

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
		if targetRepoPath == "" {
			return nil, err
		}
		workspace = ""
	}

	recordResolvedTarget(r.Root, workspace)
	if err := enforceLifecyclePendingGuard(r.Root); err != nil {
		return nil, err
	}
	return &cliDiscoveryContext{Repo: r, Workspace: workspace}, nil
}

// resolveWorkspaceScoped resolves commands that require both repository and workspace.
func resolveWorkspaceScoped() (*cliDiscoveryContext, error) {
	ctx, err := resolveRepoScoped()
	if err != nil {
		return nil, err
	}
	if ctx.Workspace == "" {
		if viewErr := readOnlyViewWorkspaceError(ctx.Repo.Root); viewErr != nil {
			return nil, viewErr
		}
		return nil, notInsideWorkspaceError()
	}
	workspace, err := resolveNamedWorkspace(ctx.Repo.Root, ctx.Workspace)
	if err != nil {
		return nil, err
	}
	ctx.Workspace = workspace
	recordResolvedTarget(ctx.Repo.Root, ctx.Workspace)
	return ctx, nil
}

func resolveSeparatedScoped() (*cliDiscoveryContext, error) {
	if targetRepoPath != "" {
		return nil, errclass.ErrUsage.WithMessage("--control-root cannot be combined with --repo")
	}
	if strings.TrimSpace(targetWorkspaceName) == "" {
		return nil, errclass.ErrExplicitTargetRequired.WithMessage("--control-root requires --workspace <name>")
	}
	ctx, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
		ControlRoot: targetControlRoot,
		Workspace:   targetWorkspaceName,
	})
	if err != nil {
		return nil, err
	}
	recordResolvedTarget(ctx.ControlRoot, ctx.Workspace)
	if err := enforceLifecyclePendingGuard(ctx.ControlRoot); err != nil {
		return nil, err
	}
	return &cliDiscoveryContext{Repo: ctx.Repo, Workspace: ctx.Workspace, Separated: ctx}, nil
}

func discoveryStarts() (repoStart string, workspaceStart string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", errclass.ErrUsage.WithMessagef("cannot get current directory: %v", err)
	}

	workspaceStart = cwd
	if targetRepoPath != "" {
		targetRepo, err := resolveRepoFlagTarget(targetRepoPath)
		if err != nil {
			return "", "", err
		}
		recordResolvedTarget(targetRepo.Root, "")
		currentRepo, err := repo.DiscoverControlRepo(cwd)
		if err == nil {
			if err := canonicalizeRepoRoot(currentRepo); err != nil {
				return "", "", err
			}
			same, err := sameRepo(currentRepo, targetRepo)
			if err != nil {
				return "", "", err
			}
			if !same {
				return "", "", errclass.ErrTargetMismatch.WithMessagef(
					"targeting mismatch: --repo resolves to %s, but current directory belongs to %s",
					cleanRepoRoot(targetRepo.Root), cleanRepoRoot(currentRepo.Root),
				)
			}
		} else if !errors.Is(err, repo.ErrControlRepoNotFound) {
			return "", "", err
		}
		return targetRepo.Root, workspaceStart, nil
	}

	currentRepo, err := repo.Discover(cwd)
	if err != nil {
		var jvsErr *errclass.JVSError
		if errors.As(err, &jvsErr) {
			switch jvsErr.Code {
			case errclass.ErrNotRepo.Code, errclass.ErrLifecyclePending.Code:
				return "", "", err
			}
		}
		return "", "", errclass.ErrNotRepo.
			WithMessage("not a JVS repository (or any parent)").
			WithHint(suggestInit())
	}
	if err := canonicalizeRepoRoot(currentRepo); err != nil {
		return "", "", err
	}

	repoStart = currentRepo.Root

	return repoStart, workspaceStart, nil
}

func resolveRepoFlagTarget(path string) (*repo.Repo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errclass.ErrUsage.WithMessagef("resolve --repo: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if cwd, cwdErr := os.Getwd(); cwdErr == nil {
				pendingRepo, pendingErr := repo.DiscoverPendingLifecycleRepoFromWorkspace(cwd, abs)
				if pendingErr == nil {
					return pendingRepo, nil
				}
				if !errors.Is(pendingErr, repo.ErrControlRepoNotFound) {
					return nil, pendingErr
				}
			}
		}
		return nil, errclass.ErrNotRepo.
			WithMessagef("--repo is not inside a JVS repository: %s", path).
			WithHint("Pass --repo as this repository root or a path inside it.")
	}
	discoveryStart := abs
	if !info.IsDir() {
		discoveryStart = filepath.Dir(abs)
	}
	r, err := repo.DiscoverControlRepo(discoveryStart)
	if err == nil {
		if err := repo.ValidateWorkspaceLocatorEvidence(discoveryStart, r.Root); err != nil {
			return nil, err
		}
		if err := canonicalizeRepoRoot(r); err != nil {
			return nil, err
		}
		return r, nil
	}
	if !errors.Is(err, repo.ErrControlRepoNotFound) {
		return nil, err
	}
	r, err = repo.Discover(discoveryStart)
	if err != nil {
		if !errors.Is(err, repo.ErrControlRepoNotFound) {
			return nil, err
		}
		return nil, errclass.ErrNotRepo.
			WithMessagef("--repo is not inside a JVS repository: %s", path).
			WithHint("Pass --repo as this repository root or a path inside it.")
	}
	if err := canonicalizeRepoRoot(r); err != nil {
		return nil, err
	}
	return r, nil
}

func canonicalizeRepoRoot(r *repo.Repo) error {
	root, err := canonicalPhysicalRepoRoot(r.Root)
	if err != nil {
		return errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	r.Root = root
	return nil
}

func sameRepo(a, b *repo.Repo) (bool, error) {
	return sameRepoRoot(a.Root, b.Root)
}

func sameRepoRoot(a, b string) (bool, error) {
	aRoot, err := canonicalPhysicalRepoRoot(a)
	if err != nil {
		return false, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	bRoot, err := canonicalPhysicalRepoRoot(b)
	if err != nil {
		return false, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	return aRoot == bRoot, nil
}

func cleanRepoRoot(path string) string {
	root, err := canonicalPhysicalRepoRoot(path)
	if err == nil {
		return root
	}
	return filepath.Clean(path)
}

func canonicalPhysicalRepoRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(realPath), nil
}

func resolveOptionalWorkspace(r *repo.Repo, start string) (string, error) {
	currentWorkspace, err := workspaceFromPath(r.Root, start)
	if err != nil {
		return "", err
	}
	if targetWorkspaceName != "" {
		return resolveNamedWorkspace(r.Root, targetWorkspaceName)
	}

	if currentWorkspace != "" {
		return currentWorkspace, nil
	}
	if targetRepoPath != "" {
		if _, err := repo.LoadWorktreeConfig(r.Root, "main"); err == nil {
			return "main", nil
		}
	}
	return "", nil
}

func workspaceFromPath(repoRoot, path string) (string, error) {
	r, workspace, err := repo.DiscoverWorktree(path)
	if err != nil {
		if errors.Is(err, repo.ErrControlRepoNotFound) {
			return "", nil
		}
		var jvsErr *errclass.JVSError
		if targetRepoPath != "" && errors.As(err, &jvsErr) && jvsErr.Code == errclass.ErrLifecyclePending.Code {
			return "", nil
		}
		return "", err
	}
	if workspace == "" {
		return "", nil
	}
	same, err := sameRepoRoot(r.Root, repoRoot)
	if err != nil {
		return "", err
	}
	if !same {
		return "", errclass.ErrTargetMismatch.WithMessagef(
			"targeting mismatch: --repo resolves to %s, but current workspace %q belongs to %s",
			cleanRepoRoot(repoRoot), workspace, cleanRepoRoot(r.Root),
		)
	}
	if _, err := repo.LoadWorktreeConfig(repoRoot, workspace); err != nil {
		return "", nil
	}
	return workspace, nil
}

func readOnlyViewWorkspaceError(repoRoot string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	viewID, ok := readOnlyViewIDFromPath(repoRoot, cwd)
	if !ok {
		return nil
	}
	message := "This path is a read-only view"
	if pin, err := sourcepin.NewManager(repoRoot).Read(viewID); err == nil && pin != nil && pin.SnapshotID != "" {
		message += " of save point " + string(pin.SnapshotID)
	}
	message += ", not a workspace. No files or history were changed."
	return errclass.ErrNotWorkspace.
		WithMessage(message).
		WithHint("Run from a workspace folder, or close the view with jvs view close " + viewID + ".")
}

func readOnlyViewIDFromPath(repoRoot, path string) (string, bool) {
	viewsRoot := filepath.Join(repoRoot, repo.JVSDirName, "views")
	if viewID, ok := readOnlyViewIDFromContainedPath(viewsRoot, path); ok {
		return viewID, true
	}
	viewsPhysical, err := filepath.EvalSymlinks(viewsRoot)
	if err != nil {
		return "", false
	}
	pathPhysical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	return readOnlyViewIDFromContainedPath(viewsPhysical, pathPhysical)
}

func readOnlyViewIDFromContainedPath(viewsRoot, path string) (string, bool) {
	viewsAbs, err := filepath.Abs(viewsRoot)
	if err != nil {
		return "", false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(viewsAbs), filepath.Clean(pathAbs))
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	first := strings.Split(filepath.Clean(rel), string(filepath.Separator))[0]
	if _, err := normalizeViewID(first); err != nil {
		return "", false
	}
	return first, true
}

func resolveNamedWorkspace(repoRoot, name string) (string, error) {
	if _, err := repo.LoadWorktreeConfig(repoRoot, name); err != nil {
		return "", missingWorkspaceError(name)
	}
	return name, nil
}

func notInsideWorkspaceError() error {
	return errclass.ErrNotWorkspace.
		WithMessage("not inside a workspace").
		WithHint("Run from a workspace directory such as main/, or pass --workspace <name>.")
}

func missingWorkspaceError(name string) error {
	return errclass.ErrNotWorkspace.
		WithMessagef("workspace %q not found", name).
		WithHint("Run jvs workspace list to see available workspaces.")
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
		if jvsErr.Code == errclass.ErrNotRepo.Code && isGenericNotRepoMessage(jvsErr.Message) {
			fmt.Fprintln(os.Stderr, formatNotInRepositoryError())
			return
		}
		message := jvsErr.Message
		if message == "" {
			message = publicErrorCodeVocabulary(jvsErr.Code)
		}
		printHumanError(message, jvsErr.Hint)
		return
	}
	printHumanError(err.Error(), "")
}

func printHumanError(message, hint string) {
	message = publicCLIErrorMessageVocabulary(message)
	hint = publicCLIErrorMessageVocabulary(hint)

	// Colorize the error prefix
	prefix := "jvs: "
	if color.EnabledFor(os.Stderr) {
		prefix = color.ErrorFor(os.Stderr, "jvs:") + " "
	}
	fmt.Fprintln(os.Stderr, prefix+message)
	if hint != "" {
		fmt.Fprintln(os.Stderr, color.DimFor(os.Stderr, "  hint: "+hint))
	}
}
