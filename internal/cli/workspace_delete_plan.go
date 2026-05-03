package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	workspaceDeletePlanSchemaVersion = 1
	workspaceDeletePlansDirName      = "workspace-delete-plans"
	workspaceDeleteCleanupPreviewRun = "jvs cleanup preview, then jvs cleanup run --plan-id <cleanup-plan-id>"
)

type workspaceDeletePlanRunHooks struct {
	afterRemoveLocked func() error
}

var workspaceDeleteRunHooks workspaceDeletePlanRunHooks

type workspaceDeletePlan struct {
	SchemaVersion           int                        `json:"schema_version"`
	RepoID                  string                     `json:"repo_id"`
	PlanID                  string                     `json:"plan_id"`
	CreatedAt               time.Time                  `json:"created_at"`
	Workspace               string                     `json:"workspace"`
	Folder                  string                     `json:"folder"`
	NewestSavePoint         *string                    `json:"newest_save_point"`
	ContentSource           *string                    `json:"content_source"`
	ExpectedNewestSavePoint *string                    `json:"expected_newest_save_point"`
	ExpectedContentSource   *string                    `json:"expected_content_source"`
	ExpectedFolderEvidence  string                     `json:"expected_folder_evidence"`
	UnsavedChanges          bool                       `json:"unsaved_changes"`
	FilesState              string                     `json:"files_state"`
	Options                 workspaceDeletePlanOptions `json:"options,omitempty"`
	RunCommand              string                     `json:"run_command"`
	SafeRunCommand          string                     `json:"safe_run_command"`
	CWDInsideAffectedTree   bool                       `json:"cwd_inside_affected_tree"`
	CleanupPreviewRun       string                     `json:"cleanup_preview_run"`
}

type workspaceDeletePlanOptions struct {
	DiscardUnsaved     bool `json:"discard_unsaved,omitempty"`
	RemovesUnsavedWork bool `json:"deletes_unsaved_work,omitempty"`
}

type publicWorkspaceDeletePreviewResult struct {
	Mode                     string                     `json:"mode"`
	PlanID                   string                     `json:"plan_id"`
	Workspace                string                     `json:"workspace"`
	Folder                   string                     `json:"folder"`
	NewestSavePoint          *string                    `json:"newest_save_point"`
	ContentSource            *string                    `json:"content_source"`
	ExpectedNewestSavePoint  *string                    `json:"expected_newest_save_point"`
	ExpectedContentSource    *string                    `json:"expected_content_source"`
	ExpectedFolderEvidence   string                     `json:"expected_folder_evidence"`
	UnsavedChanges           bool                       `json:"unsaved_changes"`
	FilesState               string                     `json:"files_state"`
	Options                  workspaceDeletePlanOptions `json:"options,omitempty"`
	FolderRemoved            bool                       `json:"folder_removed"`
	FilesChanged             bool                       `json:"files_changed"`
	WorkspaceMetadataRemoved bool                       `json:"workspace_metadata_removed"`
	SavePointStorageRemoved  bool                       `json:"save_point_storage_removed"`
	RunCommand               string                     `json:"run_command"`
	SafeRunCommand           string                     `json:"safe_run_command"`
	CWDInsideAffectedTree    bool                       `json:"cwd_inside_affected_tree"`
	CleanupPreviewRun        string                     `json:"cleanup_preview_run"`
}

type publicWorkspaceDeleteRunResult struct {
	Mode                     string  `json:"mode"`
	PlanID                   string  `json:"plan_id"`
	Status                   string  `json:"status"`
	Workspace                string  `json:"workspace"`
	Folder                   string  `json:"folder"`
	NewestSavePoint          *string `json:"newest_save_point"`
	ContentSource            *string `json:"content_source"`
	UnsavedChanges           bool    `json:"unsaved_changes"`
	FilesState               string  `json:"files_state"`
	FolderRemoved            bool    `json:"folder_removed"`
	FilesChanged             bool    `json:"files_changed"`
	WorkspaceMetadataRemoved bool    `json:"workspace_metadata_removed"`
	SavePointStorageRemoved  bool    `json:"save_point_storage_removed"`
	CleanupCommand           string  `json:"cleanup_command"`
	CleanupPreviewRun        string  `json:"cleanup_preview_run"`
}

func createWorkspaceDeletePlan(repoRoot, name string) (*workspaceDeletePlan, error) {
	if _, err := validateWorkspaceDeletion(repoRoot, name); err != nil {
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
	runCommand := "jvs workspace delete --run " + planID
	safeRunCommand := workspaceSafeRunCommand(repoRoot, status.Folder, runCommand)
	cwdInsideAffectedTree, err := workspaceCWDInsideAffectedTree(status.Folder)
	if err != nil {
		return nil, err
	}

	plan := &workspaceDeletePlan{
		SchemaVersion:           workspaceDeletePlanSchemaVersion,
		RepoID:                  repoID,
		PlanID:                  planID,
		CreatedAt:               time.Now().UTC(),
		Workspace:               status.Workspace,
		Folder:                  status.Folder,
		NewestSavePoint:         cloneStringPtr(status.NewestSavePoint),
		ContentSource:           cloneStringPtr(status.ContentSource),
		ExpectedNewestSavePoint: cloneStringPtr(status.NewestSavePoint),
		ExpectedContentSource:   cloneStringPtr(status.ContentSource),
		ExpectedFolderEvidence:  evidence,
		UnsavedChanges:          status.UnsavedChanges,
		FilesState:              status.FilesState,
		RunCommand:              runCommand,
		SafeRunCommand:          safeRunCommand,
		CWDInsideAffectedTree:   cwdInsideAffectedTree,
		CleanupPreviewRun:       workspaceDeleteCleanupPreviewRun,
	}
	if err := writeWorkspaceDeletePlan(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func executeWorkspaceDeletePlan(repoRoot, planID string) (publicWorkspaceDeleteRunResult, error) {
	var result publicWorkspaceDeleteRunResult
	err := repo.WithMutationLock(repoRoot, "workspace delete run", func() error {
		plan, err := loadWorkspaceDeletePlan(repoRoot, planID)
		if err != nil {
			return err
		}
		record, pending, err := pendingWorkspaceDeletePlanRecord(repoRoot, plan)
		if err != nil {
			return err
		}
		if pending {
			result, err = resumeWorkspaceDeletePlan(repoRoot, plan, record)
			return err
		}
		if err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
			AffectedRoot:    plan.Folder,
			SafeNextCommand: workspaceDeletePlanSafeRunCommand(repoRoot, plan),
		}); err != nil {
			return err
		}
		if err := validateWorkspaceDeletePlanTarget(repoRoot, plan); err != nil {
			return err
		}
		record = workspaceDeleteOperationRecord(plan, "prepared")
		if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
			return err
		}
		if err := worktree.NewManager(repoRoot).RemoveLocked(plan.Workspace); err != nil {
			return err
		}
		if workspaceDeleteRunHooks.afterRemoveLocked != nil {
			if err := workspaceDeleteRunHooks.afterRemoveLocked(); err != nil {
				return err
			}
		}
		if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_deleted"); err != nil {
			return err
		}
		result, err = finishWorkspaceDeletePlan(repoRoot, plan, record)
		return err
	})
	return result, err
}

func pendingWorkspaceDeletePlanRecord(repoRoot string, plan *workspaceDeletePlan) (lifecycle.OperationRecord, bool, error) {
	record, pending, err := pendingLifecycleRecordForPlan(repoRoot, plan.PlanID)
	if err != nil || !pending {
		return record, pending, err
	}
	if record.OperationType != "workspace delete" || record.RepoID != plan.RepoID {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match workspace delete plan", plan.PlanID)
	}
	if !workspaceLifecycleMetadataMatches(record, map[string]string{
		"plan_id":   plan.PlanID,
		"workspace": plan.Workspace,
		"folder":    plan.Folder,
	}) {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match workspace delete plan", plan.PlanID)
	}
	return record, true, nil
}

func resumeWorkspaceDeletePlan(repoRoot string, plan *workspaceDeletePlan, record lifecycle.OperationRecord) (publicWorkspaceDeleteRunResult, error) {
	folderIdentity, err := workspaceLifecycleFolderIdentity(repoRoot, plan.RepoID, plan.Workspace, plan.Folder, plan.ExpectedFolderEvidence)
	if err != nil {
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError(err.Error())
	}
	configIdentity, err := workspaceDeleteConfigIdentity(repoRoot, plan)
	if err != nil {
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError(err.Error())
	}
	if folderIdentity.State == workspaceLifecycleIdentityDifferent {
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError("workspace folder identity changed")
	}
	if configIdentity.State == workspaceLifecycleIdentityDifferent {
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError(configIdentity.Reason)
	}

	switch record.Phase {
	case "validated", "prepared":
		if folderIdentity.State == workspaceLifecycleIdentityExpected && configIdentity.State == workspaceLifecycleIdentityExpected {
			if err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
				AffectedRoot:    plan.Folder,
				SafeNextCommand: workspaceDeletePlanSafeRunCommand(repoRoot, plan),
			}); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			if err := validateWorkspaceDeletePlanTarget(repoRoot, plan); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			if err := worktree.NewManager(repoRoot).RemoveLocked(plan.Workspace); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			if workspaceDeleteRunHooks.afterRemoveLocked != nil {
				if err := workspaceDeleteRunHooks.afterRemoveLocked(); err != nil {
					return publicWorkspaceDeleteRunResult{}, err
				}
			}
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_deleted"); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			return finishWorkspaceDeletePlan(repoRoot, plan, record)
		}
		if folderIdentity.State == workspaceLifecycleIdentityMissing && configIdentity.State == workspaceLifecycleIdentityMissing {
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_deleted"); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			return finishWorkspaceDeletePlan(repoRoot, plan, record)
		}
		if folderIdentity.State == workspaceLifecycleIdentityMissing && configIdentity.State == workspaceLifecycleIdentityExpected {
			if err := removeWorkspaceDeleteConfigEntry(repoRoot, plan); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			if err := workspaceLifecycleWritePhase(repoRoot, &record, "workspace_deleted"); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			return finishWorkspaceDeletePlan(repoRoot, plan, record)
		}
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError(workspaceDeleteIdentityDecisionReason(folderIdentity, configIdentity))
	case "workspace_deleted":
		if folderIdentity.State == workspaceLifecycleIdentityMissing && configIdentity.State == workspaceLifecycleIdentityExpected {
			if err := removeWorkspaceDeleteConfigEntry(repoRoot, plan); err != nil {
				return publicWorkspaceDeleteRunResult{}, err
			}
			configIdentity.State = workspaceLifecycleIdentityMissing
		}
		if folderIdentity.State == workspaceLifecycleIdentityMissing && configIdentity.State == workspaceLifecycleIdentityMissing {
			return finishWorkspaceDeletePlan(repoRoot, plan, record)
		}
		return publicWorkspaceDeleteRunResult{}, workspaceDeleteCannotResumeError(workspaceDeleteIdentityDecisionReason(folderIdentity, configIdentity))
	default:
		return publicWorkspaceDeleteRunResult{}, fmt.Errorf("workspace delete is pending in unsupported phase %q", record.Phase)
	}
}

func finishWorkspaceDeletePlan(repoRoot string, plan *workspaceDeletePlan, record lifecycle.OperationRecord) (publicWorkspaceDeleteRunResult, error) {
	result := publicWorkspaceDeleteRunResult{
		Mode:                     "run",
		PlanID:                   plan.PlanID,
		Status:                   "deleted",
		Workspace:                plan.Workspace,
		Folder:                   plan.Folder,
		NewestSavePoint:          cloneStringPtr(plan.NewestSavePoint),
		ContentSource:            cloneStringPtr(plan.ContentSource),
		UnsavedChanges:           plan.UnsavedChanges,
		FilesState:               plan.FilesState,
		FolderRemoved:            true,
		FilesChanged:             true,
		WorkspaceMetadataRemoved: true,
		SavePointStorageRemoved:  false,
		CleanupCommand:           "jvs cleanup preview",
		CleanupPreviewRun:        workspaceDeleteCleanupPreviewRun,
	}
	if err := lifecycle.ConsumeOperation(repoRoot, record.OperationID); err != nil {
		return publicWorkspaceDeleteRunResult{}, err
	}
	deleteWorkspaceDeletePlan(repoRoot, plan.PlanID)
	return result, nil
}

func workspaceDeleteOperationRecord(plan *workspaceDeletePlan, phase string) lifecycle.OperationRecord {
	return workspaceLifecycleOperationRecord(plan.RepoID, plan.PlanID, "workspace delete", phase, plan.RunCommand, map[string]any{
		"workspace":                  plan.Workspace,
		"folder":                     plan.Folder,
		"plan_id":                    plan.PlanID,
		"expected_folder_evidence":   plan.ExpectedFolderEvidence,
		"expected_newest_save_point": plan.ExpectedNewestSavePoint,
		"expected_content_source":    plan.ExpectedContentSource,
		"unsaved_changes":            plan.UnsavedChanges,
		"files_state":                plan.FilesState,
	})
}

func removeWorkspaceDeleteConfigEntry(repoRoot string, plan *workspaceDeletePlan) error {
	identity, err := workspaceDeleteConfigIdentity(repoRoot, plan)
	if err != nil {
		return err
	}
	if identity.State == workspaceLifecycleIdentityMissing {
		return nil
	}
	if identity.State != workspaceLifecycleIdentityExpected {
		return workspaceDeleteCannotResumeError(identity.Reason)
	}
	dir, err := repo.WorktreeConfigDirPath(repoRoot, plan.Workspace)
	if err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat workspace registry entry: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace registry entry is symlink: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace registry entry is not a directory: %s", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove workspace registry entry: %w", err)
	}
	return nil
}

func workspaceDeleteIdentityDecisionReason(folderIdentity workspaceLifecycleIdentity, configIdentity workspaceLifecycleConfigIdentity) string {
	return fmt.Sprintf("folder is %s and registry entry is %s", folderIdentity.State, configIdentity.State)
}

func workspaceDeleteCannotResumeError(reason string) error {
	if reason == "" {
		reason = "identity evidence is incomplete"
	}
	return fmt.Errorf("workspace delete cannot resume: %s", reason)
}

func validateWorkspaceDeletePlanTarget(repoRoot string, plan *workspaceDeletePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace delete plan is required")
	}
	if plan.Workspace == "main" {
		return fmt.Errorf("cannot delete main workspace")
	}
	status, err := buildWorkspaceStatus(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceDeleteChangedSincePreviewError()
	}
	if status.Workspace != plan.Workspace || status.Folder != plan.Folder {
		return workspaceDeleteChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.NewestSavePoint, plan.ExpectedNewestSavePoint) {
		return workspaceDeleteChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.ContentSource, plan.ExpectedContentSource) {
		return workspaceDeleteChangedSincePreviewError()
	}
	if status.UnsavedChanges != plan.UnsavedChanges || status.FilesState != plan.FilesState {
		return workspaceDeleteChangedSincePreviewError()
	}
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceDeleteChangedSincePreviewError()
	}
	if evidence != plan.ExpectedFolderEvidence {
		return workspaceDeleteChangedSincePreviewError()
	}
	return nil
}

func publicWorkspaceDeletePreviewFromPlan(plan *workspaceDeletePlan) publicWorkspaceDeletePreviewResult {
	return publicWorkspaceDeletePreviewResult{
		Mode:                     "preview",
		PlanID:                   plan.PlanID,
		Workspace:                plan.Workspace,
		Folder:                   plan.Folder,
		NewestSavePoint:          cloneStringPtr(plan.NewestSavePoint),
		ContentSource:            cloneStringPtr(plan.ContentSource),
		ExpectedNewestSavePoint:  cloneStringPtr(plan.ExpectedNewestSavePoint),
		ExpectedContentSource:    cloneStringPtr(plan.ExpectedContentSource),
		ExpectedFolderEvidence:   plan.ExpectedFolderEvidence,
		UnsavedChanges:           plan.UnsavedChanges,
		FilesState:               plan.FilesState,
		Options:                  plan.Options,
		FolderRemoved:            false,
		FilesChanged:             false,
		WorkspaceMetadataRemoved: false,
		SavePointStorageRemoved:  false,
		RunCommand:               plan.RunCommand,
		SafeRunCommand:           workspaceDeletePlanSafeRunCommand("", plan),
		CWDInsideAffectedTree:    plan.CWDInsideAffectedTree,
		CleanupPreviewRun:        plan.CleanupPreviewRun,
	}
}

func writeWorkspaceDeletePlan(repoRoot string, plan *workspaceDeletePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace delete plan is required")
	}
	dir, err := workspaceDeletePlansDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create workspace delete plan directory: %w", err)
	}
	if err := validateWorkspaceDeletePlanDir(dir); err != nil {
		return err
	}
	path, err := workspaceDeletePlanPath(repoRoot, plan.PlanID, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace delete plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func deleteWorkspaceDeletePlan(repoRoot, planID string) {
	path, err := workspaceDeletePlanPath(repoRoot, planID, true)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func loadWorkspaceDeletePlan(repoRoot, planID string) (*workspaceDeletePlan, error) {
	path, err := workspaceDeletePlanPath(repoRoot, planID, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace delete plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace delete plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace delete plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace delete plan %q is not readable", planID)
	}
	var plan workspaceDeletePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("workspace delete plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != workspaceDeletePlanSchemaVersion {
		return nil, fmt.Errorf("workspace delete plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("workspace delete plan %q plan_id does not match request", planID)
	}
	repoID, err := workspaceCurrentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("workspace delete plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func workspaceDeletePlanPath(repoRoot, planID string, missingOK bool) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	dir, err := workspaceDeletePlansDir(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, planID+".json")
	if err := validateWorkspaceDeletePlanLeaf(path, missingOK); err != nil {
		return "", err
	}
	return path, nil
}

func workspaceDeletePlansDir(repoRoot string) (string, error) {
	gcDir, err := repo.GCDirPath(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(gcDir), workspaceDeletePlansDirName), nil
}

func validateWorkspaceDeletePlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat workspace delete plan directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace delete plan directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace delete plan path is not directory: %s", path)
	}
	return nil
}

func validateWorkspaceDeletePlanLeaf(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace delete plan is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace delete plan is not a regular file: %s", path)
	}
	return nil
}

func workspaceCurrentRepoID(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, repo.JVSDirName, repo.RepoIDFile))
	if err != nil {
		return "", fmt.Errorf("read repository identity: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func cloneStringPtr(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	clone := *value
	return &clone
}

func workspaceDeleteChangedSincePreviewError() error {
	return fmt.Errorf("workspace changed since preview; run preview again. No workspace was deleted.")
}

func workspaceDeletePlanSafeRunCommand(repoRoot string, plan *workspaceDeletePlan) string {
	if plan == nil {
		return ""
	}
	if plan.SafeRunCommand != "" {
		return plan.SafeRunCommand
	}
	return workspaceSafeRunCommand(repoRoot, plan.Folder, plan.RunCommand)
}

func workspaceMovePlanSafeRunCommand(repoRoot string, plan *workspaceMovePlan) string {
	if plan == nil {
		return ""
	}
	if plan.SafeRunCommand != "" {
		return plan.SafeRunCommand
	}
	return workspaceSafeRunCommand(repoRoot, plan.SourceFolder, plan.RunCommand)
}

func workspaceSafeRunCommand(repoRoot, affectedFolder, runCommand string) string {
	parent := filepath.Dir(affectedFolder)
	return "cd " + shellQuoteArg(parent) + " && " + workspaceExplicitRunCommand(repoRoot, runCommand)
}

func workspaceExplicitRunCommand(repoRoot, runCommand string) string {
	command := strings.TrimSpace(runCommand)
	if strings.HasPrefix(command, "jvs ") {
		command = strings.TrimSpace(strings.TrimPrefix(command, "jvs "))
	}
	if strings.TrimSpace(repoRoot) == "" {
		return "jvs " + command
	}
	return "jvs --repo " + shellQuoteArg(repoRoot) + " " + command
}

func workspaceCWDInsideAffectedTree(affectedFolder string) (bool, error) {
	err := lifecycle.CheckCWDOutsideAffectedTree(lifecycle.CWDSafetyRequest{
		AffectedRoot: affectedFolder,
	})
	if err == nil {
		return false, nil
	}
	var unsafe *lifecycle.UnsafeCWDError
	if errors.As(err, &unsafe) && unsafe.Cause == nil {
		return true, nil
	}
	return false, err
}

func workspaceLifecycleOperationRecord(repoID, operationID, operationType, phase, nextCommand string, metadata map[string]any) lifecycle.OperationRecord {
	now := time.Now().UTC()
	return lifecycle.OperationRecord{
		SchemaVersion:          lifecycle.SchemaVersion,
		OperationID:            operationID,
		OperationType:          operationType,
		RepoID:                 repoID,
		Phase:                  phase,
		RecommendedNextCommand: nextCommand,
		CreatedAt:              now,
		UpdatedAt:              now,
		Metadata:               metadata,
	}
}

const (
	workspaceLifecycleIdentityExpected  = "expected"
	workspaceLifecycleIdentityMissing   = "missing"
	workspaceLifecycleIdentityDifferent = "different"
)

type workspaceLifecycleIdentity struct {
	State  string
	Reason string
}

type workspaceLifecycleConfigIdentity struct {
	State  string
	Path   string
	Reason string
	Config *model.WorktreeConfig
}

func pendingLifecycleRecordForPlan(repoRoot, planID string) (lifecycle.OperationRecord, bool, error) {
	record, err := lifecycle.ReadOperation(repoRoot, planID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lifecycle.OperationRecord{}, false, nil
		}
		return lifecycle.OperationRecord{}, false, err
	}
	if record.Phase == lifecycle.PhaseConsumed {
		return lifecycle.OperationRecord{}, false, nil
	}
	return record, true, nil
}

func workspaceLifecycleMetadataMatches(record lifecycle.OperationRecord, expected map[string]string) bool {
	for key, want := range expected {
		got, ok := record.Metadata[key].(string)
		if !ok || got != want {
			return false
		}
	}
	return true
}

func workspaceLifecycleWritePhase(repoRoot string, record *lifecycle.OperationRecord, phase string) error {
	record.Phase = phase
	record.UpdatedAt = time.Now().UTC()
	return lifecycle.WriteOperation(repoRoot, *record)
}

func workspaceLifecycleFolderIdentity(repoRoot, repoID, workspaceName, folder, expectedEvidence string) (workspaceLifecycleIdentity, error) {
	if expectedEvidence == "" {
		return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "expected folder evidence is missing"}, nil
	}
	cleanFolder, err := filepath.Abs(folder)
	if err != nil {
		return workspaceLifecycleIdentity{}, fmt.Errorf("resolve workspace folder: %w", err)
	}
	cleanFolder = filepath.Clean(cleanFolder)
	info, err := os.Lstat(cleanFolder)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityMissing}, nil
		}
		return workspaceLifecycleIdentity{}, fmt.Errorf("stat workspace folder: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace folder is symlink"}, nil
	}
	if !info.IsDir() {
		return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace folder is not a directory"}, nil
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(cleanFolder); err != nil {
		return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: err.Error()}, nil
	}

	excludeLocator := false
	locatorPath := filepath.Join(cleanFolder, repo.JVSDirName)
	locatorInfo, err := os.Lstat(locatorPath)
	if err == nil {
		if locatorInfo.IsDir() || locatorInfo.Mode()&os.ModeSymlink != 0 || !locatorInfo.Mode().IsRegular() {
			return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace locator identity changed"}, nil
		}
		diagnostic, inspectErr := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
			WorkspaceRoot:         cleanFolder,
			ExpectedRepoRoot:      repoRoot,
			ExpectedRepoID:        repoID,
			ExpectedWorkspaceName: workspaceName,
		})
		if inspectErr != nil {
			return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: inspectErr.Error()}, nil
		}
		if !diagnostic.Matches {
			return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: diagnostic.Reason}, nil
		}
		excludeLocator = true
	} else if !os.IsNotExist(err) {
		return workspaceLifecycleIdentity{}, fmt.Errorf("stat workspace locator: %w", err)
	}

	hash, err := integrity.ComputePayloadRootHashWithExclusions(cleanFolder, workspaceLifecycleRootExcluder(excludeLocator, repo.JVSDirName))
	if err != nil {
		return workspaceLifecycleIdentity{}, fmt.Errorf("scan folder evidence: %w", err)
	}
	if string(hash) != expectedEvidence {
		return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "folder evidence changed"}, nil
	}
	return workspaceLifecycleIdentity{State: workspaceLifecycleIdentityExpected}, nil
}

func workspaceLifecycleRootExcluder(enabled bool, rootName string) func(string) bool {
	if !enabled {
		return nil
	}
	return func(rel string) bool {
		clean := filepath.ToSlash(filepath.Clean(rel))
		name := filepath.ToSlash(filepath.Clean(rootName))
		return clean == name || strings.HasPrefix(clean, name+"/")
	}
}

func workspaceDeleteConfigIdentity(repoRoot string, plan *workspaceDeletePlan) (workspaceLifecycleConfigIdentity, error) {
	identity, err := workspaceLifecycleConfigIdentityForPlan(repoRoot, plan.Workspace, plan.ExpectedNewestSavePoint, plan.ExpectedContentSource)
	if err != nil || identity.State != workspaceLifecycleIdentityExpected {
		return identity, err
	}
	if !workspaceLifecycleSamePath(identity.Path, plan.Folder) {
		identity.State = workspaceLifecycleIdentityDifferent
		identity.Reason = "workspace registry path changed"
	}
	return identity, nil
}

func workspaceMoveConfigIdentity(repoRoot string, plan *workspaceMovePlan) (workspaceLifecycleConfigIdentity, error) {
	identity, err := workspaceLifecycleConfigIdentityForPlan(repoRoot, plan.Workspace, plan.ExpectedNewestSavePoint, plan.ExpectedContentSource)
	if err != nil || identity.State != workspaceLifecycleIdentityExpected {
		return identity, err
	}
	if !workspaceLifecycleSamePath(identity.Path, plan.SourceFolder) && !workspaceLifecycleSamePath(identity.Path, plan.TargetFolder) {
		identity.State = workspaceLifecycleIdentityDifferent
		identity.Reason = "workspace registry path changed"
	}
	return identity, nil
}

func workspaceLifecycleConfigIdentityForPlan(repoRoot, workspaceName string, expectedNewestSavePoint, expectedContentSource *string) (workspaceLifecycleConfigIdentity, error) {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, workspaceName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceLifecycleConfigIdentity{State: workspaceLifecycleIdentityMissing, Reason: "workspace registry entry missing"}, nil
		}
		return workspaceLifecycleConfigIdentity{}, fmt.Errorf("load workspace registry entry: %w", err)
	}
	if cfg.Name != workspaceName {
		return workspaceLifecycleConfigIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace registry name changed", Config: cfg}, nil
	}
	if !workspaceLifecycleSnapshotMatches(cfg.LatestSnapshotID, expectedNewestSavePoint) {
		return workspaceLifecycleConfigIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace registry newest save point changed", Config: cfg}, nil
	}
	if !workspaceLifecycleSnapshotMatches(cfg.HeadSnapshotID, expectedContentSource) {
		return workspaceLifecycleConfigIdentity{State: workspaceLifecycleIdentityDifferent, Reason: "workspace registry content source changed", Config: cfg}, nil
	}
	configPath, err := workspaceLifecycleConfiguredPath(repoRoot, cfg)
	if err != nil {
		return workspaceLifecycleConfigIdentity{}, err
	}
	return workspaceLifecycleConfigIdentity{
		State:  workspaceLifecycleIdentityExpected,
		Path:   configPath,
		Config: cfg,
	}, nil
}

func workspaceLifecycleSnapshotMatches(actual model.SnapshotID, expected *string) bool {
	if expected == nil || *expected == "" {
		return actual == ""
	}
	return string(actual) == *expected
}

func workspaceLifecycleConfiguredPath(repoRoot string, cfg *model.WorktreeConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("workspace registry entry is required")
	}
	if cfg.RealPath != "" {
		path, err := filepath.Abs(cfg.RealPath)
		if err != nil {
			return "", fmt.Errorf("resolve workspace registry real path: %w", err)
		}
		return filepath.Clean(path), nil
	}
	return "", fmt.Errorf("workspace %q registry real path is required", cfg.Name)
}

func workspaceLifecycleSamePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftClean := filepath.Clean(leftAbs)
	rightClean := filepath.Clean(rightAbs)
	if leftClean == rightClean {
		return true
	}
	leftPhysical, leftPhysicalErr := filepath.EvalSymlinks(leftClean)
	rightPhysical, rightPhysicalErr := filepath.EvalSymlinks(rightClean)
	if leftPhysicalErr == nil && rightPhysicalErr == nil && filepath.Clean(leftPhysical) == filepath.Clean(rightPhysical) {
		return true
	}
	return false
}
