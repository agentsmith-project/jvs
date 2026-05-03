package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	workspaceMovePlanSchemaVersion = 1
	workspaceMovePlansDirName      = "workspace-move-plans"
	workspaceMoveMethod            = "atomic rename required"
)

type workspaceMovePlanRunHooks struct {
	afterMoveLocked func() error
}

var workspaceMoveRunHooks workspaceMovePlanRunHooks

type workspaceMovePlan struct {
	SchemaVersion           int       `json:"schema_version"`
	RepoID                  string    `json:"repo_id"`
	PlanID                  string    `json:"plan_id"`
	CreatedAt               time.Time `json:"created_at"`
	Workspace               string    `json:"workspace"`
	SourceFolder            string    `json:"source_folder"`
	TargetFolder            string    `json:"target_folder"`
	NewestSavePoint         *string   `json:"newest_save_point"`
	ContentSource           *string   `json:"content_source"`
	ExpectedNewestSavePoint *string   `json:"expected_newest_save_point"`
	ExpectedContentSource   *string   `json:"expected_content_source"`
	ExpectedFolderEvidence  string    `json:"expected_folder_evidence"`
	UnsavedChanges          bool      `json:"unsaved_changes"`
	FilesState              string    `json:"files_state"`
	MoveMethod              string    `json:"move_method"`
	RunCommand              string    `json:"run_command"`
	SafeRunCommand          string    `json:"safe_run_command"`
	CWDInsideAffectedTree   bool      `json:"cwd_inside_affected_tree"`
}

type publicWorkspaceMovePreviewResult struct {
	Mode                    string  `json:"mode"`
	PlanID                  string  `json:"plan_id"`
	Workspace               string  `json:"workspace"`
	SourceFolder            string  `json:"source_folder"`
	TargetFolder            string  `json:"target_folder"`
	NewestSavePoint         *string `json:"newest_save_point"`
	ContentSource           *string `json:"content_source"`
	ExpectedNewestSavePoint *string `json:"expected_newest_save_point"`
	ExpectedContentSource   *string `json:"expected_content_source"`
	ExpectedFolderEvidence  string  `json:"expected_folder_evidence"`
	UnsavedChanges          bool    `json:"unsaved_changes"`
	FilesState              string  `json:"files_state"`
	FolderMoved             bool    `json:"folder_moved"`
	FilesChanged            bool    `json:"files_changed"`
	WorkspaceNameChanged    bool    `json:"workspace_name_changed"`
	SavePointHistoryChanged bool    `json:"save_point_history_changed"`
	MoveMethod              string  `json:"move_method"`
	RunCommand              string  `json:"run_command"`
	SafeRunCommand          string  `json:"safe_run_command"`
	CWDInsideAffectedTree   bool    `json:"cwd_inside_affected_tree"`
}

type publicWorkspaceMoveRunResult struct {
	Mode                    string `json:"mode"`
	PlanID                  string `json:"plan_id"`
	Status                  string `json:"status"`
	Workspace               string `json:"workspace"`
	SourceFolder            string `json:"source_folder"`
	TargetFolder            string `json:"target_folder"`
	Folder                  string `json:"folder"`
	FolderMoved             bool   `json:"folder_moved"`
	FilesChanged            bool   `json:"files_changed"`
	WorkspaceNameChanged    bool   `json:"workspace_name_changed"`
	SavePointHistoryChanged bool   `json:"save_point_history_changed"`
}

func createWorkspaceMovePlan(repoRoot, name, targetFolder string) (*workspaceMovePlan, error) {
	source, target, err := worktree.NewManager(repoRoot).PlanMove(name, targetFolder)
	if err != nil {
		return nil, err
	}
	status, err := buildWorkspaceStatus(repoRoot, name)
	if err != nil {
		return nil, err
	}
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, name)
	if err != nil {
		return nil, err
	}
	repoID, err := workspaceCurrentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	planID := uuidutil.NewV4()
	runCommand := "jvs workspace move --run " + planID
	safeRunCommand := workspaceSafeRunCommand(repoRoot, source, runCommand)
	cwdInsideAffectedTree, err := workspaceCWDInsideAffectedTree(source)
	if err != nil {
		return nil, err
	}
	plan := &workspaceMovePlan{
		SchemaVersion:           workspaceMovePlanSchemaVersion,
		RepoID:                  repoID,
		PlanID:                  planID,
		CreatedAt:               time.Now().UTC(),
		Workspace:               status.Workspace,
		SourceFolder:            source,
		TargetFolder:            target,
		NewestSavePoint:         cloneStringPtr(status.NewestSavePoint),
		ContentSource:           cloneStringPtr(status.ContentSource),
		ExpectedNewestSavePoint: cloneStringPtr(status.NewestSavePoint),
		ExpectedContentSource:   cloneStringPtr(status.ContentSource),
		ExpectedFolderEvidence:  evidence,
		UnsavedChanges:          status.UnsavedChanges,
		FilesState:              status.FilesState,
		MoveMethod:              workspaceMoveMethod,
		RunCommand:              runCommand,
		SafeRunCommand:          safeRunCommand,
		CWDInsideAffectedTree:   cwdInsideAffectedTree,
	}
	if err := writeWorkspaceMovePlan(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func executeWorkspaceMovePlan(repoRoot, planID string) (publicWorkspaceMoveRunResult, error) {
	var result publicWorkspaceMoveRunResult
	err := repo.WithMutationLock(repoRoot, "workspace move run", func() error {
		plan, err := loadWorkspaceMovePlan(repoRoot, planID)
		if err != nil {
			return err
		}
		record, pending, err := pendingWorkspaceMovePlanRecord(repoRoot, plan)
		if err != nil {
			return err
		}
		if pending {
			result, err = resumeWorkspaceMovePlan(repoRoot, plan, record)
			return err
		}
		if err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
			AffectedRoot:    plan.SourceFolder,
			SafeNextCommand: workspaceMovePlanSafeRunCommand(repoRoot, plan),
		}); err != nil {
			return err
		}
		if err := validateWorkspaceMovePlanTarget(repoRoot, plan); err != nil {
			return err
		}
		record = workspaceMoveOperationRecord(plan, "prepared")
		if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
			return err
		}
		if err := worktree.NewManager(repoRoot).MoveLocked(plan.Workspace, plan.TargetFolder); err != nil {
			return err
		}
		if workspaceMoveRunHooks.afterMoveLocked != nil {
			if err := workspaceMoveRunHooks.afterMoveLocked(); err != nil {
				return err
			}
		}
		if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_moved"); err != nil {
			return err
		}
		result, err = finishWorkspaceMovePlan(repoRoot, plan, record)
		return err
	})
	return result, err
}

func pendingWorkspaceMovePlanRecord(repoRoot string, plan *workspaceMovePlan) (lifecycle.OperationRecord, bool, error) {
	record, pending, err := pendingLifecycleRecordForPlan(repoRoot, plan.PlanID)
	if err != nil || !pending {
		return record, pending, err
	}
	if record.OperationType != "workspace move" || record.RepoID != plan.RepoID {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match workspace move plan", plan.PlanID)
	}
	if !workspaceLifecycleMetadataMatches(record, map[string]string{
		"plan_id":       plan.PlanID,
		"workspace":     plan.Workspace,
		"source_folder": plan.SourceFolder,
		"target_folder": plan.TargetFolder,
	}) {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match workspace move plan", plan.PlanID)
	}
	return record, true, nil
}

func resumeWorkspaceMovePlan(repoRoot string, plan *workspaceMovePlan, record lifecycle.OperationRecord) (publicWorkspaceMoveRunResult, error) {
	sourceIdentity, err := workspaceLifecycleFolderIdentity(repoRoot, plan.RepoID, plan.Workspace, plan.SourceFolder, plan.ExpectedFolderEvidence)
	if err != nil {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(err.Error())
	}
	targetIdentity, err := workspaceLifecycleFolderIdentity(repoRoot, plan.RepoID, plan.Workspace, plan.TargetFolder, plan.ExpectedFolderEvidence)
	if err != nil {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(err.Error())
	}
	configIdentity, err := workspaceMoveConfigIdentity(repoRoot, plan)
	if err != nil {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(err.Error())
	}
	if sourceIdentity.State == workspaceLifecycleIdentityDifferent {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError("source folder identity changed")
	}
	if targetIdentity.State == workspaceLifecycleIdentityDifferent {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError("target folder identity changed")
	}
	if configIdentity.State != workspaceLifecycleIdentityExpected {
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(configIdentity.Reason)
	}

	switch record.Phase {
	case "validated", "prepared":
		if sourceIdentity.State == workspaceLifecycleIdentityExpected && targetIdentity.State == workspaceLifecycleIdentityMissing {
			if !workspaceLifecycleSamePath(configIdentity.Path, plan.SourceFolder) {
				return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError("workspace registry no longer points at the source folder")
			}
			if err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
				AffectedRoot:    plan.SourceFolder,
				SafeNextCommand: workspaceMovePlanSafeRunCommand(repoRoot, plan),
			}); err != nil {
				return publicWorkspaceMoveRunResult{}, err
			}
			if err := validateWorkspaceMovePlanTarget(repoRoot, plan); err != nil {
				return publicWorkspaceMoveRunResult{}, err
			}
			if err := worktree.NewManager(repoRoot).MoveLocked(plan.Workspace, plan.TargetFolder); err != nil {
				return publicWorkspaceMoveRunResult{}, err
			}
			if workspaceMoveRunHooks.afterMoveLocked != nil {
				if err := workspaceMoveRunHooks.afterMoveLocked(); err != nil {
					return publicWorkspaceMoveRunResult{}, err
				}
			}
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_moved"); err != nil {
				return publicWorkspaceMoveRunResult{}, err
			}
			return finishWorkspaceMovePlan(repoRoot, plan, record)
		}
		if sourceIdentity.State == workspaceLifecycleIdentityMissing && targetIdentity.State == workspaceLifecycleIdentityExpected {
			return finishWorkspaceMovePlan(repoRoot, plan, record)
		}
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(workspaceMoveIdentityDecisionReason(sourceIdentity, targetIdentity))
	case "workspace_moved", "locator_rewritten":
		if sourceIdentity.State == workspaceLifecycleIdentityMissing && targetIdentity.State == workspaceLifecycleIdentityExpected {
			return finishWorkspaceMovePlan(repoRoot, plan, record)
		}
		return publicWorkspaceMoveRunResult{}, workspaceMoveCannotResumeError(workspaceMoveIdentityDecisionReason(sourceIdentity, targetIdentity))
	default:
		return publicWorkspaceMoveRunResult{}, fmt.Errorf("workspace move is pending in unsupported phase %q", record.Phase)
	}
}

func finishWorkspaceMovePlan(repoRoot string, plan *workspaceMovePlan, record lifecycle.OperationRecord) (publicWorkspaceMoveRunResult, error) {
	if err := ensureWorkspaceMoveConfigAtTarget(repoRoot, plan); err != nil {
		return publicWorkspaceMoveRunResult{}, err
	}
	if err := repo.WriteWorkspaceLocator(plan.TargetFolder, repoRoot, plan.Workspace); err != nil {
		return publicWorkspaceMoveRunResult{}, err
	}
	if err := workspaceLifecycleWritePhase(repoRoot, &record, "locator_rewritten"); err != nil {
		return publicWorkspaceMoveRunResult{}, err
	}
	result := publicWorkspaceMoveRunResult{
		Mode:                    "run",
		PlanID:                  plan.PlanID,
		Status:                  "moved",
		Workspace:               plan.Workspace,
		SourceFolder:            plan.SourceFolder,
		TargetFolder:            plan.TargetFolder,
		Folder:                  plan.TargetFolder,
		FolderMoved:             true,
		FilesChanged:            true,
		WorkspaceNameChanged:    false,
		SavePointHistoryChanged: false,
	}
	if err := lifecycle.ConsumeOperation(repoRoot, record.OperationID); err != nil {
		return publicWorkspaceMoveRunResult{}, err
	}
	deleteWorkspaceMovePlan(repoRoot, plan.PlanID)
	return result, nil
}

func ensureWorkspaceMoveConfigAtTarget(repoRoot string, plan *workspaceMovePlan) error {
	configIdentity, err := workspaceMoveConfigIdentity(repoRoot, plan)
	if err != nil {
		return fmt.Errorf("verify workspace registry after move: %w", err)
	}
	if configIdentity.State != workspaceLifecycleIdentityExpected {
		return workspaceMoveCannotResumeError(configIdentity.Reason)
	}
	if workspaceLifecycleSamePath(configIdentity.Path, plan.TargetFolder) {
		return nil
	}
	if !workspaceLifecycleSamePath(configIdentity.Path, plan.SourceFolder) {
		return workspaceMoveCannotResumeError("workspace registry no longer points at the source or target folder")
	}
	if err := worktree.NewManager(repoRoot).RebindRealPath(plan.Workspace, plan.TargetFolder); err != nil {
		return fmt.Errorf("update workspace registry after move: %w", err)
	}
	updated, err := workspaceMoveConfigIdentity(repoRoot, plan)
	if err != nil {
		return fmt.Errorf("verify updated workspace registry after move: %w", err)
	}
	if updated.State != workspaceLifecycleIdentityExpected || !workspaceLifecycleSamePath(updated.Path, plan.TargetFolder) {
		return workspaceMoveCannotResumeError("workspace registry did not update to the target folder")
	}
	return nil
}

func workspaceMoveOperationRecord(plan *workspaceMovePlan, phase string) lifecycle.OperationRecord {
	return workspaceLifecycleOperationRecord(plan.RepoID, plan.PlanID, "workspace move", phase, plan.RunCommand, map[string]any{
		"workspace":                  plan.Workspace,
		"source_folder":              plan.SourceFolder,
		"target_folder":              plan.TargetFolder,
		"plan_id":                    plan.PlanID,
		"expected_folder_evidence":   plan.ExpectedFolderEvidence,
		"expected_newest_save_point": plan.ExpectedNewestSavePoint,
		"expected_content_source":    plan.ExpectedContentSource,
		"unsaved_changes":            plan.UnsavedChanges,
		"files_state":                plan.FilesState,
	})
}

func workspaceMoveIdentityDecisionReason(sourceIdentity, targetIdentity workspaceLifecycleIdentity) string {
	return fmt.Sprintf("source folder is %s and target folder is %s", sourceIdentity.State, targetIdentity.State)
}

func workspaceMoveCannotResumeError(reason string) error {
	if reason == "" {
		reason = "identity evidence is incomplete"
	}
	return fmt.Errorf("workspace move cannot resume: %s", reason)
}

func validateWorkspaceMovePlanTarget(repoRoot string, plan *workspaceMovePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace move plan is required")
	}
	if plan.Workspace == "main" {
		return fmt.Errorf("cannot move main workspace")
	}
	status, err := buildWorkspaceStatus(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceMoveChangedSincePreviewError()
	}
	if status.Workspace != plan.Workspace || status.Folder != plan.SourceFolder {
		return workspaceMoveChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.NewestSavePoint, plan.ExpectedNewestSavePoint) {
		return workspaceMoveChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.ContentSource, plan.ExpectedContentSource) {
		return workspaceMoveChangedSincePreviewError()
	}
	if status.UnsavedChanges != plan.UnsavedChanges || status.FilesState != plan.FilesState {
		return workspaceMoveChangedSincePreviewError()
	}
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceMoveChangedSincePreviewError()
	}
	if evidence != plan.ExpectedFolderEvidence {
		return workspaceMoveChangedSincePreviewError()
	}
	source, target, err := worktree.NewManager(repoRoot).PlanMove(plan.Workspace, plan.TargetFolder)
	if err != nil || source != plan.SourceFolder || target != plan.TargetFolder {
		return workspaceMoveChangedSincePreviewError()
	}
	return nil
}

func publicWorkspaceMovePreviewFromPlan(plan *workspaceMovePlan) publicWorkspaceMovePreviewResult {
	return publicWorkspaceMovePreviewResult{
		Mode:                    "preview",
		PlanID:                  plan.PlanID,
		Workspace:               plan.Workspace,
		SourceFolder:            plan.SourceFolder,
		TargetFolder:            plan.TargetFolder,
		NewestSavePoint:         cloneStringPtr(plan.NewestSavePoint),
		ContentSource:           cloneStringPtr(plan.ContentSource),
		ExpectedNewestSavePoint: cloneStringPtr(plan.ExpectedNewestSavePoint),
		ExpectedContentSource:   cloneStringPtr(plan.ExpectedContentSource),
		ExpectedFolderEvidence:  plan.ExpectedFolderEvidence,
		UnsavedChanges:          plan.UnsavedChanges,
		FilesState:              plan.FilesState,
		FolderMoved:             false,
		FilesChanged:            false,
		WorkspaceNameChanged:    false,
		SavePointHistoryChanged: false,
		MoveMethod:              plan.MoveMethod,
		RunCommand:              plan.RunCommand,
		SafeRunCommand:          workspaceMovePlanSafeRunCommand("", plan),
		CWDInsideAffectedTree:   plan.CWDInsideAffectedTree,
	}
}

func writeWorkspaceMovePlan(repoRoot string, plan *workspaceMovePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace move plan is required")
	}
	dir, err := workspaceMovePlansDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create workspace move plan directory: %w", err)
	}
	if err := validateWorkspaceMovePlanDir(dir); err != nil {
		return err
	}
	path, err := workspaceMovePlanPath(repoRoot, plan.PlanID, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace move plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func deleteWorkspaceMovePlan(repoRoot, planID string) {
	path, err := workspaceMovePlanPath(repoRoot, planID, true)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func loadWorkspaceMovePlan(repoRoot, planID string) (*workspaceMovePlan, error) {
	path, err := workspaceMovePlanPath(repoRoot, planID, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace move plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace move plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace move plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace move plan %q is not readable", planID)
	}
	var plan workspaceMovePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("workspace move plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != workspaceMovePlanSchemaVersion {
		return nil, fmt.Errorf("workspace move plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("workspace move plan %q plan_id does not match request", planID)
	}
	repoID, err := workspaceCurrentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("workspace move plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func workspaceMovePlanPath(repoRoot, planID string, missingOK bool) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	dir, err := workspaceMovePlansDir(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, planID+".json")
	if err := validateWorkspaceMovePlanLeaf(path, missingOK); err != nil {
		return "", err
	}
	return path, nil
}

func workspaceMovePlansDir(repoRoot string) (string, error) {
	gcDir, err := repo.GCDirPath(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(gcDir), workspaceMovePlansDirName), nil
}

func validateWorkspaceMovePlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat workspace move plan directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace move plan directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace move plan path is not directory: %s", path)
	}
	return nil
}

func validateWorkspaceMovePlanLeaf(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace move plan is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace move plan is not a regular file: %s", path)
	}
	return nil
}

func workspaceMoveChangedSincePreviewError() error {
	return fmt.Errorf("workspace changed since preview; run preview again. No workspace was moved.")
}
