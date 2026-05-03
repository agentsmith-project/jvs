package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	repoMovePlanSchemaVersion = 1
	repoMovePlansDirName      = "repo-move-plans"
	repoMoveMethod            = "atomic rename required"
)

type repoMovePlanRunHooks struct {
	afterRootMoved func() error
}

var repoMoveRunHooks repoMovePlanRunHooks

type repoMovePlan struct {
	SchemaVersion      int                         `json:"schema_version"`
	RepoID             string                      `json:"repo_id"`
	PlanID             string                      `json:"plan_id"`
	CreatedAt          time.Time                   `json:"created_at"`
	Operation          string                      `json:"operation"`
	Command            string                      `json:"command"`
	SourceRepoRoot     string                      `json:"source_repo_root"`
	TargetRepoRoot     string                      `json:"target_repo_root"`
	ExternalWorkspaces []repoMoveExternalWorkspace `json:"external_workspaces"`
	MoveMethod         string                      `json:"move_method"`
	RunCommand         string                      `json:"run_command"`
	SafeRunCommand     string                      `json:"safe_run_command"`
}

type repoMoveExternalWorkspace struct {
	Workspace string `json:"workspace"`
	Folder    string `json:"folder"`
}

type publicRepoMovePreviewResult struct {
	Mode                    string `json:"mode"`
	Operation               string `json:"operation"`
	PlanID                  string `json:"plan_id"`
	SourceRepoRoot          string `json:"source_repo_root"`
	TargetRepoRoot          string `json:"target_repo_root"`
	RepoID                  string `json:"repo_id"`
	MoveMethod              string `json:"move_method"`
	FolderMoved             bool   `json:"folder_moved"`
	RepoIDChanged           bool   `json:"repo_id_changed"`
	SavePointHistoryChanged bool   `json:"save_point_history_changed"`
	MainWorkspaceUpdated    bool   `json:"main_workspace_updated"`
	ExternalWorkspaces      int    `json:"external_workspaces"`
	RunCommand              string `json:"run_command"`
	SafeRunCommand          string `json:"safe_run_command"`
}

type publicRepoMoveRunResult struct {
	Mode                      string `json:"mode"`
	Operation                 string `json:"operation"`
	PlanID                    string `json:"plan_id"`
	Status                    string `json:"status"`
	SourceRepoRoot            string `json:"source_repo_root"`
	TargetRepoRoot            string `json:"target_repo_root"`
	RepoRoot                  string `json:"repo_root"`
	RepoID                    string `json:"repo_id"`
	FolderMoved               bool   `json:"folder_moved"`
	RepoIDChanged             bool   `json:"repo_id_changed"`
	SavePointHistoryChanged   bool   `json:"save_point_history_changed"`
	MainWorkspaceUpdated      bool   `json:"main_workspace_updated"`
	ExternalWorkspacesUpdated int    `json:"external_workspaces_updated"`
}

func createRepoMovePlan(repoRoot, targetFolder, operation, command string) (*repoMovePlan, error) {
	sourceRoot, err := canonicalPhysicalRepoRoot(repoRoot)
	if err != nil {
		return nil, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	targetRoot, err := normalizeRepoMoveTarget(targetFolder)
	if err != nil {
		return nil, err
	}
	if err := validateRepoMoveTarget(sourceRoot, targetRoot); err != nil {
		return nil, err
	}
	repoID, err := workspaceCurrentRepoID(sourceRoot)
	if err != nil {
		return nil, err
	}
	external, err := repoMoveExternalWorkspacePlans(sourceRoot, repoID)
	if err != nil {
		return nil, err
	}
	planID := uuidutil.NewV4()
	runCommand := repoMoveExplicitRunCommand(command, sourceRoot, planID)
	plan := &repoMovePlan{
		SchemaVersion:      repoMovePlanSchemaVersion,
		RepoID:             repoID,
		PlanID:             planID,
		CreatedAt:          time.Now().UTC(),
		Operation:          operation,
		Command:            command,
		SourceRepoRoot:     sourceRoot,
		TargetRepoRoot:     targetRoot,
		ExternalWorkspaces: external,
		MoveMethod:         repoMoveMethod,
		RunCommand:         runCommand,
		SafeRunCommand:     repoMoveSafeRunCommand(sourceRoot, runCommand),
	}
	if err := writeRepoMovePlan(sourceRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func executeRepoMovePlan(repoRoot, planID, command string) (publicRepoMoveRunResult, error) {
	currentRoot, err := canonicalPhysicalRepoRoot(repoRoot)
	if err != nil {
		return publicRepoMoveRunResult{}, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	plan, err := loadRepoMovePlan(currentRoot, planID)
	if err != nil {
		return publicRepoMoveRunResult{}, err
	}
	if plan.Command != command {
		return publicRepoMoveRunResult{}, fmt.Errorf("%s plan %q cannot be run with repo %s", plan.Command, planID, command)
	}
	record, pending, err := pendingRepoMovePlanRecord(currentRoot, plan)
	if err != nil {
		return publicRepoMoveRunResult{}, err
	}
	if pending {
		return resumeRepoMovePlan(currentRoot, plan, record)
	}
	if err := prepareFreshRepoMoveRun(plan); err != nil {
		return publicRepoMoveRunResult{}, err
	}
	record = repoMoveOperationRecord(plan, "prepared")
	if err := repoMoveWithPremoveLock(plan.SourceRepoRoot, "repo "+command+" run", func() error {
		return lifecycle.WriteOperation(plan.SourceRepoRoot, record)
	}); err != nil {
		return publicRepoMoveRunResult{}, err
	}
	return commitRepoMoveFromPrepared(plan, record)
}

func prepareFreshRepoMoveRun(plan *repoMovePlan) error {
	sourceIdentity, err := repoMoveRootIdentity(plan.SourceRepoRoot, plan.RepoID)
	if err != nil {
		return err
	}
	targetIdentity, err := repoMoveRootIdentity(plan.TargetRepoRoot, plan.RepoID)
	if err != nil {
		return err
	}
	if targetIdentity.State == workspaceLifecycleIdentityDifferent {
		return repoMoveCannotResumeError("destination repo root identity changed")
	}
	if sourceIdentity.State != workspaceLifecycleIdentityExpected || targetIdentity.State != workspaceLifecycleIdentityMissing {
		return repoMoveCannotResumeError(repoMoveIdentityDecisionReason(sourceIdentity, targetIdentity))
	}
	if err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
		AffectedRoot:    plan.SourceRepoRoot,
		SafeNextCommand: plan.SafeRunCommand,
	}); err != nil {
		return err
	}
	if err := validateRepoMoveTarget(plan.SourceRepoRoot, plan.TargetRepoRoot); err != nil {
		return err
	}
	if err := validateRepoMoveMainAtSource(plan.SourceRepoRoot); err != nil {
		return err
	}
	return validateRepoMoveExternalWorkspaces(plan, plan.SourceRepoRoot)
}

func pendingRepoMovePlanRecord(repoRoot string, plan *repoMovePlan) (lifecycle.OperationRecord, bool, error) {
	record, pending, err := pendingLifecycleRecordForPlan(repoRoot, plan.PlanID)
	if err != nil || !pending {
		return record, pending, err
	}
	if record.OperationType != repoMoveCommandJournalType(plan.Command) || record.RepoID != plan.RepoID {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match repo %s plan", plan.PlanID, plan.Command)
	}
	if !workspaceLifecycleMetadataMatches(record, map[string]string{
		"plan_id":          plan.PlanID,
		"operation":        plan.Operation,
		"source_repo_root": plan.SourceRepoRoot,
		"target_repo_root": plan.TargetRepoRoot,
	}) {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match repo %s plan", plan.PlanID, plan.Command)
	}
	return record, true, nil
}

func resumeRepoMovePlan(currentRepoRoot string, plan *repoMovePlan, record lifecycle.OperationRecord) (publicRepoMoveRunResult, error) {
	sourceIdentity, err := repoMoveRootIdentity(plan.SourceRepoRoot, plan.RepoID)
	if err != nil {
		return publicRepoMoveRunResult{}, err
	}
	targetIdentity, err := repoMoveRootIdentity(plan.TargetRepoRoot, plan.RepoID)
	if err != nil {
		return publicRepoMoveRunResult{}, err
	}
	if targetIdentity.State == workspaceLifecycleIdentityDifferent {
		return publicRepoMoveRunResult{}, repoMoveCannotResumeError("destination repo root identity changed")
	}
	if sourceIdentity.State == workspaceLifecycleIdentityDifferent && targetIdentity.State == workspaceLifecycleIdentityExpected {
		return publicRepoMoveRunResult{}, repoMoveCannotResumeError("source repo root identity changed")
	}

	switch record.Phase {
	case "prepared":
		if sourceIdentity.State == workspaceLifecycleIdentityExpected && targetIdentity.State == workspaceLifecycleIdentityMissing {
			if err := prepareFreshRepoMoveRun(plan); err != nil {
				return publicRepoMoveRunResult{}, err
			}
			return commitRepoMoveFromPrepared(plan, record)
		}
		if sourceIdentity.State == workspaceLifecycleIdentityMissing && targetIdentity.State == workspaceLifecycleIdentityExpected {
			return finishRepoMovePlan(plan.TargetRepoRoot, plan, record)
		}
		return publicRepoMoveRunResult{}, repoMoveCannotResumeError(repoMoveIdentityDecisionReason(sourceIdentity, targetIdentity))
	case "repo_moved", "main_updated", "locators_rewritten":
		if sourceIdentity.State == workspaceLifecycleIdentityMissing && targetIdentity.State == workspaceLifecycleIdentityExpected {
			return finishRepoMovePlan(plan.TargetRepoRoot, plan, record)
		}
		return publicRepoMoveRunResult{}, repoMoveCannotResumeError(repoMoveIdentityDecisionReason(sourceIdentity, targetIdentity))
	default:
		return publicRepoMoveRunResult{}, fmt.Errorf("repo %s is pending in unsupported phase %q", plan.Command, record.Phase)
	}
}

func commitRepoMoveFromPrepared(plan *repoMovePlan, record lifecycle.OperationRecord) (publicRepoMoveRunResult, error) {
	if err := markRepoMoveExternalLocatorsPending(plan, record); err != nil {
		return publicRepoMoveRunResult{}, err
	}
	if err := lifecycle.MoveSameFilesystemNoOverwrite(plan.SourceRepoRoot, plan.TargetRepoRoot); err != nil {
		return publicRepoMoveRunResult{}, fmt.Errorf("move repo root: %w", err)
	}
	if repoMoveRunHooks.afterRootMoved != nil {
		if err := repoMoveRunHooks.afterRootMoved(); err != nil {
			return publicRepoMoveRunResult{}, err
		}
	}
	return finishRepoMovePlan(plan.TargetRepoRoot, plan, record)
}

func markRepoMoveExternalLocatorsPending(plan *repoMovePlan, record lifecycle.OperationRecord) error {
	for _, external := range plan.ExternalWorkspaces {
		if err := repo.MarkWorkspaceLocatorPendingLifecycle(repo.MarkWorkspaceLocatorPendingLifecycleRequest{
			WorkspaceRoot:          external.Folder,
			ExpectedRepoID:         plan.RepoID,
			ExpectedRepoRoot:       plan.SourceRepoRoot,
			ExpectedWorkspaceName:  external.Workspace,
			OperationID:            plan.PlanID,
			OperationType:          repoMoveCommandJournalType(plan.Command),
			Phase:                  record.Phase,
			SourceRepoRoot:         plan.SourceRepoRoot,
			TargetRepoRoot:         plan.TargetRepoRoot,
			RecommendedNextCommand: plan.RunCommand,
		}); err != nil {
			return fmt.Errorf("mark external workspace %q pending repo %s: %w", external.Workspace, plan.Command, err)
		}
	}
	return nil
}

func finishRepoMovePlan(repoRoot string, plan *repoMovePlan, record lifecycle.OperationRecord) (publicRepoMoveRunResult, error) {
	if err := repo.WithMutationLock(repoRoot, "repo "+plan.Command+" finish", func() error {
		if record.Phase == "prepared" {
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "repo_moved"); err != nil {
				return err
			}
		}
		if record.Phase == "repo_moved" {
			if err := updateRepoMoveMainRealPath(repoRoot); err != nil {
				return err
			}
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "main_updated"); err != nil {
				return err
			}
		}
		if record.Phase == "main_updated" {
			for _, external := range plan.ExternalWorkspaces {
				if err := rewriteRepoMoveExternalLocator(plan, external); err != nil {
					return err
				}
			}
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "locators_rewritten"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return publicRepoMoveRunResult{}, err
	}

	result := publicRepoMoveRunResult{
		Mode:                      "run",
		Operation:                 plan.Operation,
		PlanID:                    plan.PlanID,
		Status:                    "moved",
		SourceRepoRoot:            plan.SourceRepoRoot,
		TargetRepoRoot:            plan.TargetRepoRoot,
		RepoRoot:                  plan.TargetRepoRoot,
		RepoID:                    plan.RepoID,
		FolderMoved:               true,
		RepoIDChanged:             false,
		SavePointHistoryChanged:   false,
		MainWorkspaceUpdated:      true,
		ExternalWorkspacesUpdated: len(plan.ExternalWorkspaces),
	}
	if err := lifecycle.ConsumeOperation(repoRoot, record.OperationID); err != nil {
		return publicRepoMoveRunResult{}, err
	}
	deleteRepoMovePlan(repoRoot, plan.PlanID)
	return result, nil
}

func repoMoveWithPremoveLock(repoRoot, operation string, fn func() error) error {
	lock, err := repo.AcquireMutationLock(repoRoot, operation)
	if err != nil {
		return err
	}
	err = fn()
	releaseErr := lock.Release()
	if err != nil {
		return err
	}
	return releaseErr
}

func validateRepoMoveMainAtSource(repoRoot string) error {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	if err != nil {
		return fmt.Errorf("load main workspace registry: %w", err)
	}
	if cfg.Name != "main" {
		return fmt.Errorf("main workspace registry name changed")
	}
	if cfg.RealPath == "" {
		return fmt.Errorf("main workspace must be the repo root before repo move")
	}
	if !workspaceLifecycleSamePath(cfg.RealPath, repoRoot) {
		return fmt.Errorf("main workspace registry path changed")
	}
	return nil
}

func updateRepoMoveMainRealPath(repoRoot string) error {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	if err != nil {
		return fmt.Errorf("load main workspace registry: %w", err)
	}
	if cfg.Name != "main" {
		return fmt.Errorf("main workspace registry name changed")
	}
	cfg.RealPath = repoRoot
	if err := repo.WriteWorktreeConfig(repoRoot, "main", cfg); err != nil {
		return fmt.Errorf("update main workspace registry: %w", err)
	}
	return nil
}

func validateRepoMoveExternalWorkspaces(plan *repoMovePlan, expectedRepoRoot string) error {
	for _, external := range plan.ExternalWorkspaces {
		if err := validateRepoMoveExternalWorkspace(plan, external, expectedRepoRoot); err != nil {
			return err
		}
	}
	return nil
}

func validateRepoMoveExternalWorkspace(plan *repoMovePlan, external repoMoveExternalWorkspace, expectedRepoRoot string) error {
	if err := pathutil.ValidateName(external.Workspace); err != nil {
		return err
	}
	info, err := os.Lstat(external.Folder)
	if err != nil {
		return fmt.Errorf("external workspace %q is not reachable: %w", external.Workspace, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("external workspace %q folder is symlink: %s", external.Workspace, external.Folder)
	}
	if !info.IsDir() {
		return fmt.Errorf("external workspace %q folder is not a directory: %s", external.Workspace, external.Folder)
	}
	if !modeAllowsWrite(info.Mode()) {
		return fmt.Errorf("external workspace %q folder is not writable: %s", external.Workspace, external.Folder)
	}
	locatorInfo, err := os.Lstat(filepath.Join(external.Folder, repo.JVSDirName))
	if err != nil {
		return fmt.Errorf("external workspace %q locator is not reachable: %w", external.Workspace, err)
	}
	if !modeAllowsWrite(locatorInfo.Mode()) {
		return fmt.Errorf("external workspace %q locator is not writable: %s", external.Workspace, filepath.Join(external.Folder, repo.JVSDirName))
	}
	diagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         external.Folder,
		ExpectedRepoRoot:      expectedRepoRoot,
		ExpectedRepoID:        plan.RepoID,
		ExpectedWorkspaceName: external.Workspace,
	})
	if err != nil {
		return fmt.Errorf("external workspace %q locator is not well-formed: %w", external.Workspace, err)
	}
	if !diagnostic.Matches {
		return fmt.Errorf("external workspace %q locator freshness mismatch: %s", external.Workspace, diagnostic.Reason)
	}
	return nil
}

func modeAllowsWrite(mode os.FileMode) bool {
	return mode.Perm()&0222 != 0
}

func rewriteRepoMoveExternalLocator(plan *repoMovePlan, external repoMoveExternalWorkspace) error {
	err := repo.RewriteWorkspaceLocator(repo.RewriteWorkspaceLocatorRequest{
		WorkspaceRoot:         external.Folder,
		ExpectedRepoID:        plan.RepoID,
		ExpectedRepoRoot:      plan.SourceRepoRoot,
		ExpectedWorkspaceName: external.Workspace,
		NewRepoRoot:           plan.TargetRepoRoot,
		NewWorkspaceName:      external.Workspace,
	})
	if err == nil {
		return nil
	}
	diagnostic, inspectErr := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         external.Folder,
		ExpectedRepoRoot:      plan.TargetRepoRoot,
		ExpectedRepoID:        plan.RepoID,
		ExpectedWorkspaceName: external.Workspace,
	})
	if inspectErr == nil && diagnostic.Matches {
		return nil
	}
	return err
}

func repoMoveExternalWorkspacePlans(repoRoot, repoID string) ([]repoMoveExternalWorkspace, error) {
	mgr := worktree.NewManager(repoRoot)
	configs, err := mgr.List()
	if err != nil {
		return nil, err
	}
	var external []repoMoveExternalWorkspace
	for _, cfg := range configs {
		if cfg.Name == "main" {
			if err := validateRepoMoveMainConfig(repoRoot, cfg); err != nil {
				return nil, err
			}
			continue
		}
		if cfg.RealPath == "" || workspaceLifecycleSamePath(cfg.RealPath, repoRoot) {
			continue
		}
		folder, err := mgr.Path(cfg.Name)
		if err != nil {
			return nil, err
		}
		if workspaceLifecycleSamePath(folder, repoRoot) {
			return nil, fmt.Errorf("workspace %q overlaps the repo root", cfg.Name)
		}
		record := repoMoveExternalWorkspace{Workspace: cfg.Name, Folder: folder}
		plan := &repoMovePlan{RepoID: repoID, SourceRepoRoot: repoRoot}
		if err := validateRepoMoveExternalWorkspace(plan, record, repoRoot); err != nil {
			return nil, err
		}
		external = append(external, record)
	}
	return external, nil
}

func validateRepoMoveMainConfig(repoRoot string, cfg *model.WorktreeConfig) error {
	if cfg == nil || cfg.Name != "main" {
		return fmt.Errorf("main workspace registry name changed")
	}
	if cfg.RealPath == "" {
		return fmt.Errorf("main workspace must be the repo root before repo move")
	}
	if !workspaceLifecycleSamePath(cfg.RealPath, repoRoot) {
		return fmt.Errorf("main workspace registry path changed")
	}
	return nil
}

func validateRepoMoveTarget(sourceRoot, targetRoot string) error {
	sourceAbs, err := filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(targetRoot)
	if err != nil {
		return err
	}
	sourceAbs = filepath.Clean(sourceAbs)
	targetAbs = filepath.Clean(targetAbs)
	if sourceAbs == targetAbs {
		return fmt.Errorf("new repo folder must be different from the current repo folder")
	}
	inside, err := repoMovePathContains(sourceAbs, targetAbs)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("new repo folder must not be inside the current repo folder")
	}
	parent := filepath.Dir(targetAbs)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("stat destination parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination parent must not be a symlink: %s", parent)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("destination parent is not a directory: %s", parent)
	}
	if _, err := os.Lstat(targetAbs); err == nil {
		return fmt.Errorf("destination repo folder already exists: %s", targetAbs)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination repo folder: %w", err)
	}
	same, err := lifecycle.SameFilesystem(sourceAbs, parent)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("source and destination parent are on different filesystems")
	}
	return nil
}

func normalizeRepoMoveTarget(targetFolder string) (string, error) {
	if strings.TrimSpace(targetFolder) == "" {
		return "", errclass.ErrUsage.WithMessage("repo move requires a new folder")
	}
	target, err := filepath.Abs(targetFolder)
	if err != nil {
		return "", fmt.Errorf("resolve repo folder: %w", err)
	}
	return filepath.Clean(target), nil
}

func repoMovePathContains(baseAbs, pathAbs string) (bool, error) {
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)), nil
}

type repoMoveRootIdentityResult struct {
	State  string
	Reason string
}

func repoMoveRootIdentity(root, expectedRepoID string) (repoMoveRootIdentityResult, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityMissing}, nil
		}
		return repoMoveRootIdentityResult{}, fmt.Errorf("stat repo root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityDifferent, Reason: "repo root is symlink"}, nil
	}
	if !info.IsDir() {
		return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityDifferent, Reason: "repo root is not a directory"}, nil
	}
	repoID, err := workspaceCurrentRepoID(root)
	if err != nil {
		return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityDifferent, Reason: "repo_id missing or unreadable"}, nil
	}
	if repoID != expectedRepoID {
		return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityDifferent, Reason: "repo_id changed"}, nil
	}
	return repoMoveRootIdentityResult{State: workspaceLifecycleIdentityExpected}, nil
}

func repoMoveIdentityDecisionReason(sourceIdentity, targetIdentity repoMoveRootIdentityResult) string {
	return fmt.Sprintf("source repo root is %s and destination repo root is %s", sourceIdentity.State, targetIdentity.State)
}

func repoMoveCannotResumeError(reason string) error {
	if reason == "" {
		reason = "identity evidence is incomplete"
	}
	return fmt.Errorf("repo move cannot resume: %s", reason)
}

func repoMoveOperationRecord(plan *repoMovePlan, phase string) lifecycle.OperationRecord {
	return workspaceLifecycleOperationRecord(plan.RepoID, plan.PlanID, repoMoveCommandJournalType(plan.Command), phase, plan.RunCommand, map[string]any{
		"plan_id":          plan.PlanID,
		"operation":        plan.Operation,
		"source_repo_root": plan.SourceRepoRoot,
		"target_repo_root": plan.TargetRepoRoot,
	})
}

func publicRepoMovePreviewFromPlan(plan *repoMovePlan) publicRepoMovePreviewResult {
	return publicRepoMovePreviewResult{
		Mode:                    "preview",
		Operation:               plan.Operation,
		PlanID:                  plan.PlanID,
		SourceRepoRoot:          plan.SourceRepoRoot,
		TargetRepoRoot:          plan.TargetRepoRoot,
		RepoID:                  plan.RepoID,
		MoveMethod:              plan.MoveMethod,
		FolderMoved:             false,
		RepoIDChanged:           false,
		SavePointHistoryChanged: false,
		MainWorkspaceUpdated:    false,
		ExternalWorkspaces:      len(plan.ExternalWorkspaces),
		RunCommand:              plan.RunCommand,
		SafeRunCommand:          plan.SafeRunCommand,
	}
}

func writeRepoMovePlan(repoRoot string, plan *repoMovePlan) error {
	if plan == nil {
		return fmt.Errorf("repo move plan is required")
	}
	dir, err := repoMovePlansDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create repo move plan directory: %w", err)
	}
	if err := validateRepoMovePlanDir(dir); err != nil {
		return err
	}
	path, err := repoMovePlanPath(repoRoot, plan.PlanID, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repo move plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func loadRepoMovePlan(repoRoot, planID string) (*repoMovePlan, error) {
	var plan repoMovePlan
	if err := loadRepoScopedPlan(repoRoot, planID, &plan, repoScopedPlanLoadOptions{
		name:          "repo move plan",
		schemaVersion: repoMovePlanSchemaVersion,
		path:          repoMovePlanPath,
	}); err != nil {
		return nil, err
	}
	return &plan, nil
}

func repoMovePlanPath(repoRoot, planID string, missingOK bool) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	dir, err := repoMovePlansDir(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, planID+".json")
	if err := validateRepoMovePlanLeaf(path, missingOK); err != nil {
		return "", err
	}
	return path, nil
}

func repoMovePlansDir(repoRoot string) (string, error) {
	gcDir, err := repo.GCDirPath(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(gcDir), repoMovePlansDirName), nil
}

func validateRepoMovePlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat repo move plan directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repo move plan directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo move plan path is not directory: %s", path)
	}
	return nil
}

func validateRepoMovePlanLeaf(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repo move plan is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("repo move plan is not a regular file: %s", path)
	}
	return nil
}

func deleteRepoMovePlan(repoRoot, planID string) {
	path, err := repoMovePlanPath(repoRoot, planID, true)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func repoMoveExplicitRunCommand(command, sourceRoot, planID string) string {
	return "jvs --repo " + sourceRoot + " repo " + command + " --run " + planID
}

func repoMoveSafeRunCommand(sourceRoot, runCommand string) string {
	return "cd " + filepath.Dir(sourceRoot) + " && " + runCommand
}

func validateRepoRenameFolderName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errclass.ErrUsage.WithMessage("repo rename requires a new folder name")
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		return errclass.ErrNameInvalid.WithMessagef("repo rename requires a folder name, not a path: %s", name)
	}
	if name == "." || name == ".." {
		return errclass.ErrNameInvalid.WithMessagef("repo rename requires a folder name, not %q", name)
	}
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	return nil
}

func repoRenameTarget(repoRoot, newName string) (string, error) {
	if err := validateRepoRenameFolderName(newName); err != nil {
		return "", err
	}
	sourceRoot, err := canonicalPhysicalRepoRoot(repoRoot)
	if err != nil {
		return "", errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	return filepath.Join(filepath.Dir(sourceRoot), newName), nil
}

func publicRepoMovePrintLabel(operation string) string {
	if operation == "repo_rename" {
		return "Repo rename"
	}
	return "Repo move"
}

func repoMoveCommandOperation(command string) string {
	if command == "rename" {
		return "repo_rename"
	}
	return "repo_move"
}

func repoMoveCommandJournalType(command string) string {
	return "repo " + command
}
