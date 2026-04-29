package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	workspaceRemovePlanSchemaVersion = 1
	workspaceRemovePlansDirName      = "workspace-remove-plans"
	workspaceRemoveCleanupPreviewRun = "jvs cleanup preview, then jvs cleanup run --plan-id <cleanup-plan-id>"
)

type workspaceRemovePlan struct {
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
	Options                 workspaceRemovePlanOptions `json:"options,omitempty"`
	RunCommand              string                     `json:"run_command"`
	CleanupPreviewRun       string                     `json:"cleanup_preview_run"`
}

type workspaceRemovePlanOptions struct {
	DiscardUnsaved     bool `json:"discard_unsaved,omitempty"`
	RemovesUnsavedWork bool `json:"removes_unsaved_work,omitempty"`
}

type publicWorkspaceRemovePreviewResult struct {
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
	Options                  workspaceRemovePlanOptions `json:"options,omitempty"`
	FolderRemoved            bool                       `json:"folder_removed"`
	FilesChanged             bool                       `json:"files_changed"`
	WorkspaceMetadataRemoved bool                       `json:"workspace_metadata_removed"`
	SavePointStorageRemoved  bool                       `json:"save_point_storage_removed"`
	RunCommand               string                     `json:"run_command"`
	CleanupPreviewRun        string                     `json:"cleanup_preview_run"`
}

type publicWorkspaceRemoveRunResult struct {
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

func createWorkspaceRemovePlan(repoRoot, name string, force bool) (*workspaceRemovePlan, error) {
	if _, err := validateWorkspaceRemoval(repoRoot, name, force); err != nil {
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
	repoID, err := workspaceRemoveCurrentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}

	options := workspaceRemovePlanOptions{}
	if force && status.UnsavedChanges {
		options.DiscardUnsaved = true
		options.RemovesUnsavedWork = true
	}

	planID := uuidutil.NewV4()
	plan := &workspaceRemovePlan{
		SchemaVersion:           workspaceRemovePlanSchemaVersion,
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
		Options:                 options,
		RunCommand:              "jvs workspace remove --run " + planID,
		CleanupPreviewRun:       workspaceRemoveCleanupPreviewRun,
	}
	if err := writeWorkspaceRemovePlan(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func executeWorkspaceRemovePlan(repoRoot, planID string) (publicWorkspaceRemoveRunResult, error) {
	var result publicWorkspaceRemoveRunResult
	err := repo.WithMutationLock(repoRoot, "workspace remove run", func() error {
		plan, err := loadWorkspaceRemovePlan(repoRoot, planID)
		if err != nil {
			return err
		}
		if err := validateWorkspaceRemovePlanTarget(repoRoot, plan); err != nil {
			return err
		}
		if err := worktree.NewManager(repoRoot).RemoveLocked(plan.Workspace); err != nil {
			return err
		}
		result = publicWorkspaceRemoveRunResult{
			Mode:                     "run",
			PlanID:                   plan.PlanID,
			Status:                   "removed",
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
			CleanupPreviewRun:        workspaceRemoveCleanupPreviewRun,
		}
		deleteWorkspaceRemovePlan(repoRoot, plan.PlanID)
		return nil
	})
	return result, err
}

func validateWorkspaceRemovePlanTarget(repoRoot string, plan *workspaceRemovePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace remove plan is required")
	}
	if plan.Workspace == "main" {
		return fmt.Errorf("cannot remove main workspace")
	}
	status, err := buildWorkspaceStatus(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceRemoveChangedSincePreviewError()
	}
	if status.Workspace != plan.Workspace || status.Folder != plan.Folder {
		return workspaceRemoveChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.NewestSavePoint, plan.ExpectedNewestSavePoint) {
		return workspaceRemoveChangedSincePreviewError()
	}
	if !sameStatusSavePoint(status.ContentSource, plan.ExpectedContentSource) {
		return workspaceRemoveChangedSincePreviewError()
	}
	if status.UnsavedChanges != plan.UnsavedChanges || status.FilesState != plan.FilesState {
		return workspaceRemoveChangedSincePreviewError()
	}
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, plan.Workspace)
	if err != nil {
		return workspaceRemoveChangedSincePreviewError()
	}
	if evidence != plan.ExpectedFolderEvidence {
		return workspaceRemoveChangedSincePreviewError()
	}
	return nil
}

func publicWorkspaceRemovePreviewFromPlan(plan *workspaceRemovePlan) publicWorkspaceRemovePreviewResult {
	return publicWorkspaceRemovePreviewResult{
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
		CleanupPreviewRun:        plan.CleanupPreviewRun,
	}
}

func writeWorkspaceRemovePlan(repoRoot string, plan *workspaceRemovePlan) error {
	if plan == nil {
		return fmt.Errorf("workspace remove plan is required")
	}
	dir, err := workspaceRemovePlansDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create workspace remove plan directory: %w", err)
	}
	if err := validateWorkspaceRemovePlanDir(dir); err != nil {
		return err
	}
	path, err := workspaceRemovePlanPath(repoRoot, plan.PlanID, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace remove plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func deleteWorkspaceRemovePlan(repoRoot, planID string) {
	path, err := workspaceRemovePlanPath(repoRoot, planID, true)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func loadWorkspaceRemovePlan(repoRoot, planID string) (*workspaceRemovePlan, error) {
	path, err := workspaceRemovePlanPath(repoRoot, planID, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace remove plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace remove plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace remove plan %q not found", planID)
		}
		return nil, fmt.Errorf("workspace remove plan %q is not readable", planID)
	}
	var plan workspaceRemovePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("workspace remove plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != workspaceRemovePlanSchemaVersion {
		return nil, fmt.Errorf("workspace remove plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("workspace remove plan %q plan_id does not match request", planID)
	}
	repoID, err := workspaceRemoveCurrentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("workspace remove plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func workspaceRemovePlanPath(repoRoot, planID string, missingOK bool) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	dir, err := workspaceRemovePlansDir(repoRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, planID+".json")
	if err := validateWorkspaceRemovePlanLeaf(path, missingOK); err != nil {
		return "", err
	}
	return path, nil
}

func workspaceRemovePlansDir(repoRoot string) (string, error) {
	gcDir, err := repo.GCDirPath(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(gcDir), workspaceRemovePlansDirName), nil
}

func validateWorkspaceRemovePlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat workspace remove plan directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace remove plan directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace remove plan path is not directory: %s", path)
	}
	return nil
}

func validateWorkspaceRemovePlanLeaf(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace remove plan is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace remove plan is not a regular file: %s", path)
	}
	return nil
}

func workspaceRemoveCurrentRepoID(repoRoot string) (string, error) {
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

func workspaceRemoveChangedSincePreviewError() error {
	return fmt.Errorf("workspace changed since preview; run preview again. No workspace was removed.")
}
