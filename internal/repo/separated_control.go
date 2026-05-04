package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

// SeparatedContextRequest explicitly selects a separated-control repository.
// It intentionally has no cwd field so callers cannot accidentally fall back to
// ambient discovery or a payload-side locator.
type SeparatedContextRequest struct {
	ControlRoot string
	Workspace   string
}

// SeparatedContextRevalidationRequest re-checks a previously resolved
// separated-control workspace binding before a mutation or re-read.
type SeparatedContextRevalidationRequest struct {
	ControlRoot string
	Workspace   string
	// ExpectedRepoID is the repo_id captured from the previously resolved
	// separated context.
	ExpectedRepoID string
	// ExpectedPayloadRoot is the payload root captured from the previously
	// resolved separated context.
	ExpectedPayloadRoot string
}

// SeparatedContext is the resolved authority and payload binding for a
// separated-control workspace.
type SeparatedContext struct {
	Repo                 *Repo
	ControlRoot          string
	PayloadRoot          string
	Workspace            string
	BoundaryValidated    bool
	LocatorAuthoritative bool
}

type separatedInitRoots struct {
	controlPath     string
	payloadPath     string
	controlPhysical string
	payloadPhysical string
}

// InitSeparatedControl creates a repo whose trusted control plane lives under
// controlRoot while the main workspace payload lives under payloadRoot.
func InitSeparatedControl(controlRoot, payloadRoot, workspaceName string) (*Repo, error) {
	if err := pathutil.ValidateName(workspaceName); err != nil {
		return nil, err
	}
	if workspaceName != "main" {
		return nil, errclass.ErrWorkspaceMismatch.WithMessage("phase 1 separated init only supports workspace \"main\"")
	}
	roots, err := validateSeparatedInitRoots(controlRoot, payloadRoot)
	if err != nil {
		return nil, err
	}
	if err := rejectPayloadLocatorPresent(roots.payloadPath); err != nil {
		return nil, err
	}
	controlExisted, err := validateSeparatedInitTarget(roots.controlPath, "control root")
	if err != nil {
		return nil, err
	}
	payloadExisted, err := validateSeparatedPayloadInitTarget(roots.payloadPath, roots.payloadPhysical)
	if err != nil {
		return nil, err
	}

	payloadCreated := false
	if !payloadExisted {
		if err := os.MkdirAll(roots.payloadPath, 0755); err != nil {
			return nil, permissionOrWrappedErr("create payload root", err)
		}
		payloadCreated = true
	}
	if err := validateSeparatedPayloadInitSymlinkBoundary(roots); err != nil {
		if payloadCreated {
			_ = os.RemoveAll(roots.payloadPath)
		}
		return nil, err
	}

	controlCreated := false
	if !controlExisted {
		controlCreated = true
	}
	repoID, err := createControlPlane(roots.controlPath, RepoModeSeparatedControl)
	if err != nil {
		rollbackSeparatedInit(roots.controlPath, controlCreated, roots.payloadPath, payloadCreated)
		return nil, permissionOrWrappedErr("create control plane", err)
	}

	cfg := &model.WorktreeConfig{
		Name:      workspaceName,
		RealPath:  roots.payloadPath,
		CreatedAt: time.Now().UTC(),
	}
	if err := WriteWorktreeConfig(roots.controlPath, workspaceName, cfg); err != nil {
		rollbackSeparatedInit(roots.controlPath, controlCreated, roots.payloadPath, payloadCreated)
		return nil, errclass.ErrControlMalformed.WithMessagef("write workspace registry: %v", err)
	}
	if err := fsutil.FsyncDir(roots.payloadPath); err != nil {
		rollbackSeparatedInit(roots.controlPath, controlCreated, roots.payloadPath, payloadCreated)
		return nil, permissionOrWrappedErr("fsync payload root", err)
	}
	if err := fsutil.FsyncDir(roots.controlPath); err != nil {
		rollbackSeparatedInit(roots.controlPath, controlCreated, roots.payloadPath, payloadCreated)
		return nil, permissionOrWrappedErr("fsync control root", err)
	}

	return &Repo{
		Root:          roots.controlPath,
		FormatVersion: FormatVersion,
		RepoID:        repoID,
		Mode:          RepoModeSeparatedControl,
	}, nil
}

// OpenControlRoot opens exactly the supplied control root. It does not walk cwd
// and does not read workspace locators.
func OpenControlRoot(controlRoot string) (*Repo, error) {
	root, err := cleanSeparatedRoot(controlRoot, "control root")
	if err != nil {
		return nil, errclass.ErrControlMissing.WithMessage(err.Error())
	}
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errclass.ErrControlMissing.WithMessagef("control root does not exist: %s", root)
		}
		return nil, permissionOrWrappedErr("stat control root", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errclass.ErrPathBoundaryEscape.WithMessagef("control root must not be a symlink: %s", root)
	}
	if !info.IsDir() {
		return nil, errclass.ErrControlMalformed.WithMessagef("control root is not a directory: %s", root)
	}

	jvsDir := filepath.Join(root, JVSDirName)
	jvsInfo, err := os.Lstat(jvsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errclass.ErrControlMalformed.WithMessagef("control root is missing %s", JVSDirName)
		}
		return nil, permissionOrWrappedErr("stat control metadata", err)
	}
	if jvsInfo.Mode()&os.ModeSymlink != 0 || !jvsInfo.IsDir() {
		return nil, errclass.ErrControlMalformed.WithMessagef("control metadata is not a directory: %s", jvsDir)
	}
	version, err := readFormatVersion(jvsDir)
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessage(err.Error())
	}
	if version > FormatVersion {
		return nil, errclass.ErrFormatUnsupported.WithMessagef(
			"format version %d > supported %d", version, FormatVersion)
	}
	mode, err := readRepoMode(jvsDir)
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessage(err.Error())
	}
	repoID, err := readRepoID(jvsDir)
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessagef("read repo_id: %v", err)
	}
	if strings.TrimSpace(repoID) == "" {
		return nil, errclass.ErrControlMalformed.WithMessage("repo_id is required")
	}
	return &Repo{
		Root:          root,
		FormatVersion: version,
		RepoID:        repoID,
		Mode:          mode,
	}, nil
}

// ResolveSeparatedContext resolves a workspace from a separated control root
// using only the explicit control root and the workspace registry.
func ResolveSeparatedContext(req SeparatedContextRequest) (*SeparatedContext, error) {
	if err := pathutil.ValidateName(req.Workspace); err != nil {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("invalid workspace selector: %v", err)
	}
	r, err := OpenControlRoot(req.ControlRoot)
	if err != nil {
		return nil, err
	}
	if r.Mode != RepoModeSeparatedControl {
		return nil, errclass.ErrControlMalformed.WithMessagef("repo_mode is %q, want %q", r.Mode, RepoModeSeparatedControl)
	}

	cfg, err := loadSeparatedWorkspaceConfig(r.Root, req.Workspace)
	if err != nil {
		return nil, err
	}
	if cfg.Name != req.Workspace {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace selector %q does not match registry entry %q", req.Workspace, cfg.Name)
	}
	payloadRoot, err := cleanSeparatedRoot(cfg.RealPath, "payload root")
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessagef("invalid payload root in workspace registry: %v", err)
	}
	if err := validateSeparatedRegisteredPayloadRoot(r.Root, payloadRoot); err != nil {
		return nil, err
	}
	if err := rejectPayloadLocatorPresent(payloadRoot); err != nil {
		return nil, err
	}
	return &SeparatedContext{
		Repo:                 r,
		ControlRoot:          r.Root,
		PayloadRoot:          payloadRoot,
		Workspace:            req.Workspace,
		BoundaryValidated:    true,
		LocatorAuthoritative: false,
	}, nil
}

// RevalidateSeparatedContext resolves the separated-control binding again and
// verifies the control repo identity and registry payload root still match the
// previously resolved separated context.
func RevalidateSeparatedContext(req SeparatedContextRevalidationRequest) (*SeparatedContext, error) {
	if err := pathutil.ValidateName(req.Workspace); err != nil {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("invalid workspace selector: %v", err)
	}
	expectedRepoID := strings.TrimSpace(req.ExpectedRepoID)
	if expectedRepoID == "" {
		return nil, errclass.ErrRepoIDMismatch.WithMessage("expected repo_id is required")
	}
	expectedPayloadRoot, err := cleanSeparatedRoot(req.ExpectedPayloadRoot, "expected payload root")
	if err != nil {
		return nil, errclass.ErrPathBoundaryEscape.WithMessagef("invalid expected payload root: %v", err)
	}

	r, err := OpenControlRoot(req.ControlRoot)
	if err != nil {
		return nil, err
	}
	if r.Mode != RepoModeSeparatedControl {
		return nil, errclass.ErrControlMalformed.WithMessagef("repo_mode is %q, want %q", r.Mode, RepoModeSeparatedControl)
	}
	if r.RepoID != expectedRepoID {
		return nil, errclass.ErrRepoIDMismatch.WithMessagef("repo_id mismatch: control has %s, expected %s", r.RepoID, expectedRepoID)
	}

	cfg, err := loadSeparatedWorkspaceConfig(r.Root, req.Workspace)
	if err != nil {
		return nil, err
	}
	if cfg.Name != req.Workspace {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace selector %q does not match registry entry %q", req.Workspace, cfg.Name)
	}
	payloadRoot, err := cleanSeparatedRoot(cfg.RealPath, "payload root")
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessagef("invalid payload root in workspace registry: %v", err)
	}
	if payloadRoot != expectedPayloadRoot {
		return nil, errclass.ErrPathBoundaryEscape.WithMessagef(
			"workspace %q payload root changed: registry has %s, expected %s",
			req.Workspace, payloadRoot, expectedPayloadRoot,
		)
	}
	if err := validateSeparatedRegisteredPayloadRoot(r.Root, payloadRoot); err != nil {
		return nil, err
	}
	if err := rejectPayloadLocatorPresent(payloadRoot); err != nil {
		return nil, err
	}
	return &SeparatedContext{
		Repo:                 r,
		ControlRoot:          r.Root,
		PayloadRoot:          payloadRoot,
		Workspace:            req.Workspace,
		BoundaryValidated:    true,
		LocatorAuthoritative: false,
	}, nil
}

// ValidateSeparatedPayloadSymlinkBoundary walks a separated payload and rejects
// symlinks that resolve outside the payload or into the control root.
func ValidateSeparatedPayloadSymlinkBoundary(ctx *SeparatedContext) error {
	if ctx == nil {
		return nil
	}
	controlRoot, err := cleanSeparatedRoot(ctx.ControlRoot, "control root")
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("invalid control root: %v", err)
	}
	payloadRoot, err := cleanSeparatedRoot(ctx.PayloadRoot, "payload root")
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("invalid payload root: %v", err)
	}
	return validateSeparatedPayloadSymlinkBoundary(controlRoot, payloadRoot)
}

func validateSeparatedInitRoots(controlRoot, payloadRoot string) (separatedInitRoots, error) {
	controlPath, err := cleanSeparatedRoot(controlRoot, "control root")
	if err != nil {
		return separatedInitRoots{}, err
	}
	payloadPath, err := cleanSeparatedRoot(payloadRoot, "payload root")
	if err != nil {
		return separatedInitRoots{}, err
	}
	controlPhysical, err := resolvePhysicalInitTarget(controlPath)
	if err != nil {
		return separatedInitRoots{}, errclass.ErrPathBoundaryEscape.WithMessagef("resolve control root: %v", err)
	}
	payloadPhysical, err := resolvePhysicalInitTarget(payloadPath)
	if err != nil {
		return separatedInitRoots{}, errclass.ErrPathBoundaryEscape.WithMessagef("resolve payload root: %v", err)
	}
	if err := validateSeparatedRootBoundary(controlPath, payloadPath, controlPhysical, payloadPhysical); err != nil {
		return separatedInitRoots{}, err
	}
	if same, known, err := sameExistingPath(controlPath, payloadPath); err != nil {
		return separatedInitRoots{}, permissionOrWrappedErr("compare control and payload identity", err)
	} else if known && same {
		return separatedInitRoots{}, errclass.ErrControlPayloadOverlap.WithMessage("control root and payload root refer to the same filesystem object")
	}
	return separatedInitRoots{
		controlPath:     controlPath,
		payloadPath:     payloadPath,
		controlPhysical: controlPhysical,
		payloadPhysical: payloadPhysical,
	}, nil
}

func validateSeparatedRootBoundary(controlPath, payloadPath, controlPhysical, payloadPhysical string) error {
	if controlPath == payloadPath || controlPhysical == payloadPhysical {
		return errclass.ErrControlPayloadOverlap.WithMessage("control root and payload root must be distinct")
	}
	if absPathContains(controlPath, payloadPath) || absPathContains(controlPhysical, payloadPhysical) {
		return errclass.ErrPayloadInsideControl.WithMessage("payload root must not be inside control root")
	}
	if absPathContains(payloadPath, controlPath) || absPathContains(payloadPhysical, controlPhysical) {
		return errclass.ErrControlInsidePayload.WithMessage("control root must not be inside payload root")
	}
	return nil
}

func validateSeparatedInitTarget(path, role string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, permissionOrWrappedErr("stat "+role, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, errclass.ErrTargetRootOccupied.WithMessagef("%s must not be a symlink: %s", role, path)
	}
	if !info.IsDir() {
		return true, errclass.ErrTargetRootOccupied.WithMessagef("%s exists and is not a directory: %s", role, path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return true, permissionOrWrappedErr("read "+role, err)
	}
	if len(entries) > 0 {
		return true, errclass.ErrTargetRootOccupied.WithMessagef("%s must be empty or missing: %s", role, path)
	}
	return true, nil
}

func validateSeparatedPayloadInitTarget(path, physicalPath string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := rejectSeparatedNestedPayload(path, physicalPath); err != nil {
				return false, err
			}
			return false, nil
		}
		return false, permissionOrWrappedErr("stat payload root", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, errclass.ErrTargetRootOccupied.WithMessagef("payload root must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return true, errclass.ErrTargetRootOccupied.WithMessagef("payload root exists and is not a directory: %s", path)
	}
	if err := rejectSeparatedNestedPayload(path, physicalPath); err != nil {
		return true, err
	}
	return true, nil
}

func rejectSeparatedNestedPayload(path, physicalPath string) error {
	if err := rejectNestedInitTarget(path); err != nil {
		return errclass.ErrTargetRootOccupied.WithMessagef("payload root is inside existing JVS repository: %s", path)
	}
	if physicalPath != "" && physicalPath != path {
		if err := rejectNestedInitTarget(physicalPath); err != nil {
			return errclass.ErrTargetRootOccupied.WithMessagef("payload root is inside existing JVS repository: %s (physical target: %s)", path, physicalPath)
		}
	}
	return nil
}

func loadSeparatedWorkspaceConfig(repoRoot, workspace string) (*model.WorktreeConfig, error) {
	worktreesDir, err := WorktreesDirPath(repoRoot)
	if err != nil {
		return nil, errclass.ErrControlMalformed.WithMessagef("workspace registry missing or malformed: %v", err)
	}
	cfgDir, err := WorktreeConfigDirPath(repoRoot, workspace)
	if err != nil {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("invalid workspace selector: %v", err)
	}
	info, err := os.Lstat(cfgDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace %q is not registered", workspace)
		}
		return nil, permissionOrWrappedErr("stat workspace registry entry", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errclass.ErrControlMalformed.WithMessagef("workspace registry entry is malformed: %s", cfgDir)
	}
	if !absPathContains(worktreesDir, cfgDir) {
		return nil, errclass.ErrPathBoundaryEscape.WithMessage("workspace registry path escapes worktrees directory")
	}
	cfg, err := LoadWorktreeConfig(repoRoot, workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace %q config is missing", workspace)
		}
		return nil, errclass.ErrControlMalformed.WithMessagef("load workspace registry: %v", err)
	}
	return cfg, nil
}

func validateSeparatedRegisteredPayloadRoot(controlRoot, payloadRoot string) error {
	info, err := os.Lstat(payloadRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return errclass.ErrPayloadMissing.WithMessagef("payload root does not exist: %s", payloadRoot)
		}
		return permissionOrWrappedErr("stat payload root", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errclass.ErrPathBoundaryEscape.WithMessagef("payload root must not be a symlink: %s", payloadRoot)
	}
	if !info.IsDir() {
		return errclass.ErrPayloadMissing.WithMessagef("payload root is not a directory: %s", payloadRoot)
	}
	controlPhysical, err := existingPhysicalPath(controlRoot)
	if err != nil {
		return errclass.ErrControlMalformed.WithMessagef("resolve control root: %v", err)
	}
	payloadPhysical, err := existingPhysicalPath(payloadRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return errclass.ErrPayloadMissing.WithMessagef("payload root does not exist: %s", payloadRoot)
		}
		return permissionOrWrappedErr("resolve payload root", err)
	}
	return validateSeparatedRootBoundary(controlRoot, payloadRoot, controlPhysical, payloadPhysical)
}

func rejectPayloadLocatorPresent(payloadRoot string) error {
	locatorPath := filepath.Join(payloadRoot, JVSDirName)
	if _, err := os.Lstat(locatorPath); err == nil {
		return errclass.ErrPayloadLocatorPresent.WithMessagef("payload root contains root-level %s path: %s", JVSDirName, locatorPath)
	} else if os.IsNotExist(err) {
		return nil
	} else if errors.Is(err, os.ErrPermission) {
		return errclass.ErrPermissionDenied.WithMessagef("stat payload locator: %v", err)
	} else {
		return fmt.Errorf("stat payload locator: %w", err)
	}
}

func validateSeparatedPayloadSymlinkBoundary(controlRoot, payloadRoot string) error {
	controlInfo, err := os.Lstat(controlRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return errclass.ErrControlMissing.WithMessagef("control root does not exist: %s", controlRoot)
		}
		return permissionOrWrappedErr("stat control root boundary", err)
	}
	if controlInfo.Mode()&os.ModeSymlink != 0 || !controlInfo.IsDir() {
		return errclass.ErrPathBoundaryEscape.WithMessagef("control root is not a real directory: %s", controlRoot)
	}
	payloadInfo, err := os.Lstat(payloadRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return errclass.ErrPayloadMissing.WithMessagef("payload root does not exist: %s", payloadRoot)
		}
		return permissionOrWrappedErr("stat payload root boundary", err)
	}
	if payloadInfo.Mode()&os.ModeSymlink != 0 || !payloadInfo.IsDir() {
		return errclass.ErrPathBoundaryEscape.WithMessagef("payload root is not a real directory: %s", payloadRoot)
	}

	controlReal, err := existingPhysicalPath(controlRoot)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("resolve control root boundary: %v", err)
	}
	payloadReal, err := existingPhysicalPath(payloadRoot)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("resolve payload root boundary: %v", err)
	}
	return walkSeparatedPayloadSymlinkBoundary(controlReal, payloadReal, payloadRoot)
}

func validateSeparatedPayloadInitSymlinkBoundary(roots separatedInitRoots) error {
	controlReal := roots.controlPhysical
	controlInfo, err := os.Lstat(roots.controlPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return permissionOrWrappedErr("stat control root boundary", err)
		}
	} else {
		if controlInfo.Mode()&os.ModeSymlink != 0 || !controlInfo.IsDir() {
			return errclass.ErrPathBoundaryEscape.WithMessagef("control root is not a real directory: %s", roots.controlPath)
		}
		controlReal, err = existingPhysicalPath(roots.controlPath)
		if err != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("resolve control root boundary: %v", err)
		}
	}

	payloadInfo, err := os.Lstat(roots.payloadPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errclass.ErrPayloadMissing.WithMessagef("payload root does not exist: %s", roots.payloadPath)
		}
		return permissionOrWrappedErr("stat payload root boundary", err)
	}
	if payloadInfo.Mode()&os.ModeSymlink != 0 || !payloadInfo.IsDir() {
		return errclass.ErrPathBoundaryEscape.WithMessagef("payload root is not a real directory: %s", roots.payloadPath)
	}
	payloadReal, err := existingPhysicalPath(roots.payloadPath)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("resolve payload root boundary: %v", err)
	}
	return walkSeparatedPayloadSymlinkBoundary(controlReal, payloadReal, roots.payloadPath)
}

func walkSeparatedPayloadSymlinkBoundary(controlReal, payloadReal, payloadRoot string) error {
	return filepath.WalkDir(payloadRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("walk payload boundary: %v", walkErr)
		}
		if path == payloadRoot || entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		rel := separatedPayloadRelativePath(payloadRoot, path)
		targetReal, err := filepath.EvalSymlinks(path)
		if err != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("payload symlink cannot be resolved safely: %s: %v", rel, err)
		}
		targetReal = filepath.Clean(targetReal)
		if pathWithinCleanRoot(controlReal, targetReal) {
			return errclass.ErrPathBoundaryEscape.WithMessagef("payload symlink points into control root: %s", rel)
		}
		if !pathWithinCleanRoot(payloadReal, targetReal) {
			return errclass.ErrPathBoundaryEscape.WithMessagef("payload symlink escapes payload boundary: %s", rel)
		}
		return nil
	})
}

func separatedPayloadRelativePath(payloadRoot, path string) string {
	rel, err := filepath.Rel(payloadRoot, path)
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel)
}

func pathWithinCleanRoot(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relPathContained(rel)
}

func cleanSeparatedRoot(path, role string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s must not be empty", role)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", role, err)
	}
	return filepath.Clean(abs), nil
}

func absPathContains(base, path string) bool {
	contained, err := cleanAbsPathContains(base, path)
	return err == nil && contained
}

func sameExistingPath(left, right string) (bool, bool, error) {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil || rightErr != nil {
		if leftErr != nil && !os.IsNotExist(leftErr) {
			return false, false, leftErr
		}
		if rightErr != nil && !os.IsNotExist(rightErr) {
			return false, false, rightErr
		}
		return false, false, nil
	}
	return os.SameFile(leftInfo, rightInfo), true, nil
}

func permissionOrWrappedErr(context string, err error) error {
	if errors.Is(err, os.ErrPermission) {
		return errclass.ErrPermissionDenied.WithMessagef("%s: %v", context, err)
	}
	return fmt.Errorf("%s: %w", context, err)
}

func rollbackSeparatedInit(controlRoot string, controlCreated bool, payloadRoot string, payloadCreated bool) {
	if controlCreated {
		_ = os.RemoveAll(controlRoot)
	} else {
		_ = os.RemoveAll(filepath.Join(controlRoot, JVSDirName))
	}
	if payloadCreated {
		_ = os.RemoveAll(payloadRoot)
	}
}
