package doctor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

type StrictCheck struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	ErrorCode *string `json:"error_code"`
	Message   string  `json:"message"`
}

type SeparatedStrictResult struct {
	RepoID               string        `json:"repo_id,omitempty"`
	WorkspaceName        string        `json:"workspace_name"`
	ControlRoot          string        `json:"control_root"`
	PayloadRoot          string        `json:"payload_root"`
	RepoMode             string        `json:"repo_mode"`
	SeparatedControl     bool          `json:"separated_control"`
	BoundaryValidated    bool          `json:"boundary_validated"`
	LocatorAuthoritative bool          `json:"locator_authoritative"`
	DoctorStrict         string        `json:"doctor_strict"`
	Healthy              bool          `json:"healthy"`
	Findings             []Finding     `json:"findings"`
	Checks               []StrictCheck `json:"checks"`
}

type separatedDoctorContext struct {
	Repo        *repo.Repo
	PayloadRoot string
	Workspace   string
}

const (
	separatedCheckRootOverlap      = "root_overlap"
	separatedCheckPayloadLocator   = "payload_locator"
	separatedCheckRepoIdentity     = "repo_identity"
	separatedCheckWorkspaceBinding = "workspace_binding"
	separatedCheckPathBoundary     = "path_boundary"
	separatedCheckPermissions      = "permissions"
	separatedCheckActiveOperation  = "active_operation"
	separatedCheckRecoveryState    = "recovery_state"
)

func CheckSeparatedStrict(req repo.SeparatedContextRequest) (*SeparatedStrictResult, error) {
	ctx, err := resolveSeparatedDoctorContext(req)
	if err != nil {
		return nil, err
	}

	result := &SeparatedStrictResult{
		RepoID:               ctx.Repo.RepoID,
		WorkspaceName:        ctx.Workspace,
		ControlRoot:          ctx.Repo.Root,
		PayloadRoot:          ctx.PayloadRoot,
		RepoMode:             ctx.Repo.Mode,
		SeparatedControl:     ctx.Repo.Mode == repo.RepoModeSeparatedControl,
		BoundaryValidated:    true,
		LocatorAuthoritative: false,
		DoctorStrict:         "passed",
		Healthy:              true,
		Findings:             []Finding{},
		Checks:               separatedStrictPassedChecks(),
	}

	checkSeparatedRootOverlap(result, ctx.Repo.Root, ctx.PayloadRoot)
	checkSeparatedPayloadLocator(result, ctx.PayloadRoot)
	checkSeparatedRepoIdentity(result, ctx.Repo)
	checkSeparatedWorkspaceBinding(result, req.Workspace, ctx.Workspace, ctx.PayloadRoot)
	checkSeparatedPathBoundary(result, ctx.Repo.Root, ctx.PayloadRoot)
	checkSeparatedPermissions(result, ctx.Repo.Root, ctx.PayloadRoot)
	checkSeparatedActiveOperation(result, ctx.Repo.Root)
	checkSeparatedRecoveryState(result, ctx.Repo.Root)

	if !separatedStrictChecksHealthy(result.Checks) {
		result.Healthy = false
		result.DoctorStrict = "failed"
	}
	return result, nil
}

func resolveSeparatedDoctorContext(req repo.SeparatedContextRequest) (*separatedDoctorContext, error) {
	ctx, err := repo.ResolveSeparatedContext(req)
	if err == nil {
		return &separatedDoctorContext{
			Repo:        ctx.Repo,
			PayloadRoot: ctx.PayloadRoot,
			Workspace:   ctx.Workspace,
		}, nil
	}
	if !separatedDoctorRecoverableContextError(err) {
		return nil, err
	}

	if err := pathutil.ValidateName(req.Workspace); err != nil {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("invalid workspace selector: %v", err)
	}
	r, openErr := repo.OpenControlRoot(req.ControlRoot)
	if openErr != nil {
		return nil, openErr
	}
	if r.Mode != repo.RepoModeSeparatedControl {
		return nil, errclass.ErrControlMalformed.WithMessagef("repo_mode is %q, want %q", r.Mode, repo.RepoModeSeparatedControl)
	}
	cfg, loadErr := repo.LoadWorktreeConfig(r.Root, req.Workspace)
	if loadErr != nil {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace %q is not registered", req.Workspace)
	}
	if cfg.Name != req.Workspace {
		return nil, errclass.ErrWorkspaceMismatch.WithMessagef("workspace selector %q does not match registry entry %q", req.Workspace, cfg.Name)
	}
	payloadRoot, cleanErr := cleanSeparatedDoctorRoot(cfg.RealPath, "payload root")
	if cleanErr != nil {
		return nil, errclass.ErrControlMalformed.WithMessagef("invalid payload root in workspace registry: %v", cleanErr)
	}
	return &separatedDoctorContext{
		Repo:        r,
		PayloadRoot: payloadRoot,
		Workspace:   req.Workspace,
	}, nil
}

func separatedDoctorRecoverableContextError(err error) bool {
	for _, target := range []error{
		errclass.ErrPayloadLocatorPresent,
		errclass.ErrControlPayloadOverlap,
		errclass.ErrPayloadInsideControl,
		errclass.ErrControlInsidePayload,
		errclass.ErrPathBoundaryEscape,
		errclass.ErrPayloadMissing,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func separatedStrictPassedChecks() []StrictCheck {
	return []StrictCheck{
		{Name: separatedCheckRootOverlap, Status: "passed", Message: "Control and payload roots are separate."},
		{Name: separatedCheckPayloadLocator, Status: "passed", Message: "No payload-root control marker is present."},
		{Name: separatedCheckRepoIdentity, Status: "passed", Message: "Repo identity matches the control root."},
		{Name: separatedCheckWorkspaceBinding, Status: "passed", Message: "Workspace selector matches the control registry."},
		{Name: separatedCheckPathBoundary, Status: "passed", Message: "Canonical paths stay within declared boundaries."},
		{Name: separatedCheckPermissions, Status: "passed", Message: "Required read/write/fsync permissions are available."},
		{Name: separatedCheckActiveOperation, Status: "passed", Message: "No active operation blocks this command."},
		{Name: separatedCheckRecoveryState, Status: "passed", Message: "No recovery state blocks this command."},
	}
}

func checkSeparatedRootOverlap(result *SeparatedStrictResult, controlRoot, payloadRoot string) {
	if code, message := separatedRootOverlapCode(controlRoot, payloadRoot); code != "" {
		failSeparatedCheck(result, separatedCheckRootOverlap, code, message)
		result.BoundaryValidated = false
	}
}

func checkSeparatedPayloadLocator(result *SeparatedStrictResult, payloadRoot string) {
	locatorPath := filepath.Join(payloadRoot, repo.JVSDirName)
	if _, err := os.Lstat(locatorPath); err == nil {
		failSeparatedCheck(
			result,
			separatedCheckPayloadLocator,
			errclass.ErrPayloadLocatorPresent.Code,
			fmt.Sprintf("Payload root contains root-level %s path: %s", repo.JVSDirName, locatorPath),
		)
		return
	} else if os.IsNotExist(err) {
		return
	} else if errors.Is(err, os.ErrPermission) {
		failSeparatedCheck(result, separatedCheckPermissions, errclass.ErrPermissionDenied.Code, fmt.Sprintf("Cannot inspect payload locator: %v", err))
		return
	} else {
		failSeparatedCheck(result, separatedCheckPayloadLocator, errclass.ErrPayloadLocatorPresent.Code, fmt.Sprintf("Cannot inspect payload locator: %v", err))
	}
}

func checkSeparatedRepoIdentity(result *SeparatedStrictResult, r *repo.Repo) {
	if r == nil || strings.TrimSpace(r.RepoID) == "" {
		failSeparatedCheck(result, separatedCheckRepoIdentity, errclass.ErrControlMalformed.Code, "Repo identity is missing from the control root.")
		return
	}
	if r.Mode != repo.RepoModeSeparatedControl {
		failSeparatedCheck(result, separatedCheckRepoIdentity, errclass.ErrControlMalformed.Code, "Repo mode is not separated_control.")
	}
}

func checkSeparatedWorkspaceBinding(result *SeparatedStrictResult, requested, resolved, payloadRoot string) {
	if requested != resolved {
		failSeparatedCheck(
			result,
			separatedCheckWorkspaceBinding,
			errclass.ErrWorkspaceMismatch.Code,
			fmt.Sprintf("Workspace selector %q resolved to %q.", requested, resolved),
		)
		return
	}
	if strings.TrimSpace(payloadRoot) == "" {
		failSeparatedCheck(result, separatedCheckWorkspaceBinding, errclass.ErrWorkspaceMismatch.Code, "Workspace payload binding is empty.")
	}
}

func checkSeparatedPathBoundary(result *SeparatedStrictResult, controlRoot, payloadRoot string) {
	if _, err := os.Lstat(controlRoot); err != nil {
		failSeparatedCheck(result, separatedCheckPathBoundary, errclass.ErrControlMissing.Code, fmt.Sprintf("Cannot inspect control root: %v", err))
		result.BoundaryValidated = false
		return
	}
	info, err := os.Lstat(payloadRoot)
	if err != nil {
		code := errclass.ErrPayloadMissing.Code
		if errors.Is(err, os.ErrPermission) {
			code = errclass.ErrPermissionDenied.Code
		}
		failSeparatedCheck(result, separatedCheckPathBoundary, code, fmt.Sprintf("Cannot inspect payload root: %v", err))
		result.BoundaryValidated = false
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		failSeparatedCheck(result, separatedCheckPathBoundary, errclass.ErrPathBoundaryEscape.Code, fmt.Sprintf("Payload root must not be a symlink: %s", payloadRoot))
		result.BoundaryValidated = false
		return
	}
	if !info.IsDir() {
		failSeparatedCheck(result, separatedCheckPathBoundary, errclass.ErrPayloadMissing.Code, fmt.Sprintf("Payload root is not a directory: %s", payloadRoot))
		result.BoundaryValidated = false
	}
}

func checkSeparatedPermissions(result *SeparatedStrictResult, controlRoot, payloadRoot string) {
	for _, target := range []struct {
		role string
		path string
	}{
		{role: "control metadata", path: filepath.Join(controlRoot, repo.JVSDirName)},
		{role: "payload root", path: payloadRoot},
	} {
		if err := separatedPermissionSmoke(target.path); err != nil {
			code := errclass.ErrPermissionDenied.Code
			if os.IsNotExist(err) {
				code = errclass.ErrPayloadMissing.Code
			}
			failSeparatedCheck(result, separatedCheckPermissions, code, fmt.Sprintf("Cannot verify %s permissions at %s: %v", target.role, target.path, err))
			return
		}
	}
}

func separatedPermissionSmoke(dir string) (err error) {
	if _, err := os.ReadDir(dir); err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	probePath := filepath.Join(dir, fmt.Sprintf(".jvs-doctor-permission-probe-%d-%d.tmp", os.Getpid(), time.Now().UnixNano()))
	probe, err := os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create no-overwrite probe: %w", err)
	}
	probeCreated := true
	defer func() {
		if probeCreated {
			_ = os.Remove(probePath)
		}
	}()
	if _, err := probe.Write([]byte("jvs doctor permission probe\n")); err != nil {
		_ = probe.Close()
		return fmt.Errorf("write probe: %w", err)
	}
	if err := probe.Sync(); err != nil {
		_ = probe.Close()
		return fmt.Errorf("fsync probe: %w", err)
	}
	if err := probe.Close(); err != nil {
		return fmt.Errorf("close probe: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("delete probe: %w", err)
	}
	probeCreated = false
	if err := fsutil.FsyncDir(dir); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}

func checkSeparatedActiveOperation(result *SeparatedStrictResult, controlRoot string) {
	inspection, err := repo.InspectMutationLock(controlRoot)
	if err != nil {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Cannot inspect active operation lock: %v", err))
		return
	}
	if inspection.Status != repo.MutationLockAbsent {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Repository mutation lock is %s.", inspection.Status))
		return
	}
	pending, err := lifecycle.ListPendingOperations(controlRoot)
	if err != nil {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Cannot inspect lifecycle operations: %v", err))
		return
	}
	if len(pending) > 0 {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Lifecycle operation %s is pending.", pending[0].OperationID))
		return
	}
	if message, err := separatedIntentState(controlRoot); err != nil {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Cannot inspect operation intents: %v", err))
		return
	} else if message != "" {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, message)
		return
	}
	if message, err := separatedCleanupPlanState(controlRoot); err != nil {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, fmt.Sprintf("Cannot inspect cleanup plans: %v", err))
		return
	} else if message != "" {
		failSeparatedCheck(result, separatedCheckActiveOperation, errclass.ErrActiveOperationBlocking.Code, message)
	}
}

func separatedIntentState(controlRoot string) (string, error) {
	intentsDir, err := repo.IntentsDirPath(controlRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Sprintf("Operation intent entry %s is a symlink.", name), nil
		}
		if entry.IsDir() {
			return fmt.Sprintf("Operation intent entry %s is a directory.", name), nil
		}
		return fmt.Sprintf("Operation intent %s is present.", name), nil
	}
	return "", nil
}

func separatedCleanupPlanState(controlRoot string) (string, error) {
	gcDir, err := repo.GCDirPath(controlRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	entries, err := os.ReadDir(gcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Sprintf("Cleanup plan entry %s is a symlink.", name), nil
		}
		if strings.HasSuffix(name, ".json") {
			return fmt.Sprintf("Cleanup plan %s is pending.", strings.TrimSuffix(name, ".json")), nil
		}
		if strings.HasPrefix(name, ".jvs-tmp-") {
			return fmt.Sprintf("Cleanup plan temporary state %s is pending.", name), nil
		}
	}
	return "", nil
}

func checkSeparatedRecoveryState(result *SeparatedStrictResult, controlRoot string) {
	if message, err := separatedRestorePlanState(controlRoot); err != nil {
		failSeparatedCheck(result, separatedCheckRecoveryState, errclass.ErrRecoveryBlocking.Code, fmt.Sprintf("Cannot inspect restore plans: %v", err))
		return
	} else if message != "" {
		failSeparatedCheck(result, separatedCheckRecoveryState, errclass.ErrRecoveryBlocking.Code, message)
		return
	}
	if message, err := separatedRecoveryPlanState(controlRoot); err != nil {
		failSeparatedCheck(result, separatedCheckRecoveryState, errclass.ErrRecoveryBlocking.Code, fmt.Sprintf("Cannot inspect recovery plans: %v", err))
		return
	} else if message != "" {
		failSeparatedCheck(result, separatedCheckRecoveryState, errclass.ErrRecoveryBlocking.Code, message)
	}
}

func separatedRestorePlanState(controlRoot string) (string, error) {
	restorePlansDir := filepath.Join(controlRoot, repo.JVSDirName, "restore-plans")
	entries, err := os.ReadDir(restorePlansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Sprintf("Restore plan entry %s is a symlink.", name), nil
		}
		if entry.IsDir() {
			return fmt.Sprintf("Restore plan entry %s is a directory.", name), nil
		}
		if strings.HasSuffix(name, ".json") {
			return fmt.Sprintf("Restore plan %s is pending.", strings.TrimSuffix(name, ".json")), nil
		}
	}
	return "", nil
}

func separatedRecoveryPlanState(controlRoot string) (string, error) {
	plans, err := recovery.NewManager(controlRoot).List()
	if err != nil {
		return "", err
	}
	for _, plan := range plans {
		if plan.Status == recovery.StatusActive {
			return fmt.Sprintf("Recovery plan %s is active.", plan.PlanID), nil
		}
	}
	return "", nil
}

func failSeparatedCheck(result *SeparatedStrictResult, name, code, message string) {
	for i := range result.Checks {
		if result.Checks[i].Name != name {
			continue
		}
		result.Checks[i].Status = "failed"
		result.Checks[i].ErrorCode = stringPtr(code)
		result.Checks[i].Message = message
		return
	}
	result.Checks = append(result.Checks, StrictCheck{
		Name:      name,
		Status:    "failed",
		ErrorCode: stringPtr(code),
		Message:   message,
	})
}

func separatedStrictChecksHealthy(checks []StrictCheck) bool {
	for _, check := range checks {
		if check.Status == "failed" {
			return false
		}
	}
	return true
}

func separatedRootOverlapCode(controlRoot, payloadRoot string) (string, string) {
	control := filepath.Clean(controlRoot)
	payload := filepath.Clean(payloadRoot)
	if control == payload {
		return errclass.ErrControlPayloadOverlap.Code, "Control root and payload root must be distinct."
	}
	if pathContains(control, payload) {
		return errclass.ErrPayloadInsideControl.Code, "Payload root must not be inside control root."
	}
	if pathContains(payload, control) {
		return errclass.ErrControlInsidePayload.Code, "Control root must not be inside payload root."
	}
	controlPhysical, controlErr := filepath.EvalSymlinks(control)
	payloadPhysical, payloadErr := filepath.EvalSymlinks(payload)
	if controlErr == nil && payloadErr == nil {
		controlPhysical = filepath.Clean(controlPhysical)
		payloadPhysical = filepath.Clean(payloadPhysical)
		if controlPhysical == payloadPhysical {
			return errclass.ErrControlPayloadOverlap.Code, "Control root and payload root resolve to the same filesystem location."
		}
		if pathContains(controlPhysical, payloadPhysical) {
			return errclass.ErrPayloadInsideControl.Code, "Payload root resolves inside control root."
		}
		if pathContains(payloadPhysical, controlPhysical) {
			return errclass.ErrControlInsidePayload.Code, "Control root resolves inside payload root."
		}
	}
	return "", ""
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func cleanSeparatedDoctorRoot(path, role string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s must not be empty", role)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", role, err)
	}
	return filepath.Clean(abs), nil
}

func stringPtr(value string) *string {
	return &value
}
