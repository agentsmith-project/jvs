package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

const workspaceRenameMainGuidance = "main workspace is the repo root; use jvs repo rename to rename the folder."

type publicWorkspaceRenameResult struct {
	Mode                       string  `json:"mode"`
	Status                     string  `json:"status"`
	Operation                  string  `json:"operation"`
	OperationID                *string `json:"operation_id,omitempty"`
	OldWorkspace               string  `json:"old_workspace"`
	Workspace                  string  `json:"workspace"`
	Folder                     string  `json:"folder"`
	FolderMoved                bool    `json:"folder_moved"`
	WorkspaceConnectionUpdated bool    `json:"workspace_connection_updated"`
	SavePointHistoryChanged    bool    `json:"save_point_history_changed"`
}

func executeWorkspaceRename(repoRoot, oldName, newName string, dryRun bool) (publicWorkspaceRenameResult, error) {
	if err := pathutil.ValidateName(oldName); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if err := pathutil.ValidateName(newName); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if oldName == "main" {
		return publicWorkspaceRenameResult{}, errors.New(workspaceRenameMainGuidance)
	}

	var result publicWorkspaceRenameResult
	err := repo.WithMutationLock(repoRoot, "workspace rename", func() error {
		repoID, err := workspaceCurrentRepoID(repoRoot)
		if err != nil {
			return err
		}
		opID := workspaceRenameOperationID(oldName, newName)
		record, pending, err := pendingWorkspaceRenameRecord(repoRoot, opID, repoID, oldName, newName)
		if err != nil {
			return err
		}
		if pending {
			result, err = resumeWorkspaceRename(repoRoot, record)
			return err
		}

		mgr := worktree.NewManager(repoRoot)
		if _, err := mgr.Get(newName); err == nil {
			return fmt.Errorf("workspace %s already exists", newName)
		}
		oldCfg, err := mgr.Get(oldName)
		if err != nil {
			return missingWorkspaceError(oldName)
		}
		if oldCfg.Name != oldName {
			return fmt.Errorf("config name mismatch for %s: %q", oldName, oldCfg.Name)
		}
		folder, err := mgr.Path(oldName)
		if err != nil {
			return err
		}
		if dryRun {
			result = publicWorkspaceRenameResult{
				Mode:                       "dry-run",
				Status:                     "planned",
				Operation:                  "workspace_rename",
				OldWorkspace:               oldName,
				Workspace:                  newName,
				Folder:                     folder,
				FolderMoved:                false,
				WorkspaceConnectionUpdated: false,
				SavePointHistoryChanged:    false,
			}
			return nil
		}

		locatorRequired := oldCfg.RealPath != ""
		locatorPresent, err := validateWorkspaceRenameLocatorBeforeRegistryChange(repoRoot, repoID, folder, oldName, newName, locatorRequired)
		if err != nil {
			return err
		}

		record = workspaceLifecycleOperationRecord(repoID, opID, "workspace rename", "started", "jvs workspace rename "+oldName+" "+newName, map[string]any{
			"old_workspace":    oldName,
			"new_workspace":    newName,
			"folder":           folder,
			"locator_present":  locatorPresent,
			"locator_required": locatorRequired,
		})
		if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
			return err
		}
		if err := mgr.RenameLocked(oldName, newName); err != nil {
			return err
		}
		record.Phase = "config_renamed"
		record.UpdatedAt = time.Now().UTC()
		if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
			return err
		}
		result, err = finishWorkspaceRename(repoRoot, record)
		return err
	})
	return result, err
}

func pendingWorkspaceRenameRecord(repoRoot, operationID, repoID, oldName, newName string) (lifecycle.OperationRecord, bool, error) {
	record, err := lifecycle.ReadOperation(repoRoot, operationID)
	if err != nil {
		return lifecycle.OperationRecord{}, false, nil
	}
	if record.OperationType != "workspace rename" || record.RepoID != repoID {
		return lifecycle.OperationRecord{}, false, nil
	}
	metaOld, _ := record.Metadata["old_workspace"].(string)
	metaNew, _ := record.Metadata["new_workspace"].(string)
	if metaOld != oldName || metaNew != newName {
		return lifecycle.OperationRecord{}, false, nil
	}
	if record.Phase == lifecycle.PhaseConsumed {
		return lifecycle.OperationRecord{}, false, nil
	}
	return record, true, nil
}

func resumeWorkspaceRename(repoRoot string, record lifecycle.OperationRecord) (publicWorkspaceRenameResult, error) {
	if _, _, _, _, err := workspaceRenameMetadata(record); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if record.Phase == "started" {
		if err := ensureWorkspaceRenameConfigRenamed(repoRoot, record); err != nil {
			return publicWorkspaceRenameResult{}, err
		}
		record.Phase = "config_renamed"
		record.UpdatedAt = time.Now().UTC()
		if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
			return publicWorkspaceRenameResult{}, err
		}
	}
	if record.Phase == "config_renamed" || record.Phase == "locator_rewritten" {
		return finishWorkspaceRename(repoRoot, record)
	}
	return publicWorkspaceRenameResult{}, fmt.Errorf("workspace rename is pending in unsupported phase %q", record.Phase)
}

func finishWorkspaceRename(repoRoot string, record lifecycle.OperationRecord) (publicWorkspaceRenameResult, error) {
	oldName, newName, folder, locatorPresent, err := workspaceRenameMetadata(record)
	if err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if err := ensureWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if workspaceRenameLocatorRequired(record) && !locatorPresent {
		return publicWorkspaceRenameResult{}, fmt.Errorf("workspace connection mismatch: workspace locator missing")
	}
	if locatorPresent {
		if err := rewriteWorkspaceRenameLocator(repoRoot, record.RepoID, folder, oldName, newName); err != nil {
			return publicWorkspaceRenameResult{}, err
		}
	}
	record.Phase = "locator_rewritten"
	record.UpdatedAt = time.Now().UTC()
	if err := lifecycle.WriteOperation(repoRoot, record); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	if err := lifecycle.ConsumeOperation(repoRoot, record.OperationID); err != nil {
		return publicWorkspaceRenameResult{}, err
	}
	opID := record.OperationID
	return publicWorkspaceRenameResult{
		Mode:                       "rename",
		Status:                     "renamed",
		Operation:                  "workspace_rename",
		OperationID:                &opID,
		OldWorkspace:               oldName,
		Workspace:                  newName,
		Folder:                     folder,
		FolderMoved:                false,
		WorkspaceConnectionUpdated: locatorPresent,
		SavePointHistoryChanged:    false,
	}, nil
}

func validateWorkspaceRenameLocatorBeforeRegistryChange(repoRoot, repoID, folder, oldName, newName string, locatorRequired bool) (bool, error) {
	locatorPresent, err := repo.WorkspaceLocatorPresent(folder)
	if err != nil {
		return false, err
	}
	if !locatorPresent {
		if locatorRequired {
			return false, fmt.Errorf("workspace connection mismatch: workspace locator missing")
		}
		return false, nil
	}
	if err := validateWorkspaceRenameLocatorCanRewrite(repoRoot, repoID, folder, oldName, newName); err != nil {
		return false, err
	}
	return true, nil
}

func validateWorkspaceRenameLocatorCanRewrite(repoRoot, repoID, folder, oldName, newName string) error {
	oldDiagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         folder,
		ExpectedRepoRoot:      repoRoot,
		ExpectedRepoID:        repoID,
		ExpectedWorkspaceName: oldName,
	})
	if err != nil {
		return err
	}
	if !oldDiagnostic.Matches {
		newDiagnostic, inspectNewErr := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
			WorkspaceRoot:         folder,
			ExpectedRepoRoot:      repoRoot,
			ExpectedRepoID:        repoID,
			ExpectedWorkspaceName: newName,
		})
		if inspectNewErr != nil {
			return inspectNewErr
		}
		if !newDiagnostic.Matches {
			return fmt.Errorf("workspace connection mismatch: %s", oldDiagnostic.Reason)
		}
	}
	if err := verifyWorkspaceRenameLocatorWritable(folder); err != nil {
		return fmt.Errorf("workspace locator is not writable: %w", err)
	}
	return nil
}

func verifyWorkspaceRenameLocatorWritable(folder string) error {
	workspaceRoot, err := filepath.Abs(folder)
	if err != nil {
		return fmt.Errorf("resolve workspace folder: %w", err)
	}
	workspaceRoot = filepath.Clean(workspaceRoot)
	locatorPath := filepath.Join(workspaceRoot, repo.JVSDirName)
	info, err := os.Lstat(locatorPath)
	if err != nil {
		return fmt.Errorf("stat workspace locator: %w", err)
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("workspace locator is not a regular file: %s", locatorPath)
	}
	tmp, err := os.CreateTemp(workspaceRoot, ".jvs-locator-preflight-*")
	if err != nil {
		return fmt.Errorf("create locator preflight file: %w", err)
	}
	tmpPath := tmp.Name()
	closeErr := tmp.Close()
	removeErr := os.Remove(tmpPath)
	if closeErr != nil {
		return fmt.Errorf("close locator preflight file: %w", closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("remove locator preflight file: %w", removeErr)
	}
	if err := fsutil.FsyncDir(workspaceRoot); err != nil {
		return fmt.Errorf("fsync workspace folder: %w", err)
	}
	return nil
}

func ensureWorkspaceRenameConfigRenamed(repoRoot string, record lifecycle.OperationRecord) error {
	oldName, newName, folder, locatorPresent, err := workspaceRenameMetadata(record)
	if err != nil {
		return err
	}
	oldCfg, oldErr := repo.LoadWorktreeConfig(repoRoot, oldName)
	oldMissing := errors.Is(oldErr, os.ErrNotExist)
	if oldErr != nil && !oldMissing {
		return fmt.Errorf("load old workspace registry during rename resume: %w", oldErr)
	}
	newCfg, newErr := repo.LoadWorktreeConfig(repoRoot, newName)
	newMissing := errors.Is(newErr, os.ErrNotExist)
	if newErr != nil && !newMissing {
		return fmt.Errorf("load new workspace registry during rename resume: %w", newErr)
	}
	if !oldMissing && !newMissing {
		return fmt.Errorf("workspace rename cannot resume: both old and new registry entries exist")
	}
	if !oldMissing {
		if oldCfg.Name != oldName {
			return fmt.Errorf("workspace rename cannot resume: old registry name mismatch %q", oldCfg.Name)
		}
		if locatorPresent {
			if err := validateWorkspaceRenameLocatorCanRewrite(repoRoot, record.RepoID, folder, oldName, newName); err != nil {
				return err
			}
		} else if workspaceRenameLocatorRequired(record) {
			return fmt.Errorf("workspace connection mismatch: workspace locator missing")
		}
		if err := worktree.NewManager(repoRoot).RenameLocked(oldName, newName); err != nil {
			return err
		}
		return ensureWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder)
	}
	if !newMissing {
		return repairWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder, newCfg)
	}
	return fmt.Errorf("workspace rename cannot resume: old and new registry entries are missing")
}

func ensureWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder string) error {
	cfg, err := repo.LoadWorktreeConfig(repoRoot, newName)
	if err != nil {
		return fmt.Errorf("load renamed workspace registry: %w", err)
	}
	return repairWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder, cfg)
}

func repairWorkspaceRenameNewConfig(repoRoot, oldName, newName, folder string, cfg *model.WorktreeConfig) error {
	if cfg == nil {
		return fmt.Errorf("workspace rename cannot resume: renamed registry entry is empty")
	}
	repaired := *cfg
	changed := false
	switch repaired.Name {
	case newName:
	case oldName:
		repaired.Name = newName
		changed = true
	default:
		return fmt.Errorf("workspace rename cannot resume: renamed registry name mismatch %q", repaired.Name)
	}
	configuredPath, err := workspaceLifecycleConfiguredPath(repoRoot, &repaired)
	if err != nil {
		return err
	}
	if !workspaceLifecycleSamePath(configuredPath, folder) {
		if repaired.RealPath != "" {
			return fmt.Errorf("workspace rename cannot resume: renamed registry path changed")
		}
		repaired.RealPath = folder
		changed = true
	}
	if !changed {
		return nil
	}
	if err := repo.WriteWorktreeConfig(repoRoot, newName, &repaired); err != nil {
		return fmt.Errorf("repair renamed workspace registry: %w", err)
	}
	verified, err := repo.LoadWorktreeConfig(repoRoot, newName)
	if err != nil {
		return fmt.Errorf("verify repaired workspace registry: %w", err)
	}
	if verified.Name != newName {
		return fmt.Errorf("workspace rename cannot resume: repaired registry name mismatch %q", verified.Name)
	}
	verifiedPath, err := workspaceLifecycleConfiguredPath(repoRoot, verified)
	if err != nil {
		return err
	}
	if !workspaceLifecycleSamePath(verifiedPath, folder) {
		return fmt.Errorf("workspace rename cannot resume: repaired registry path changed")
	}
	return nil
}

func rewriteWorkspaceRenameLocator(repoRoot, repoID, folder, oldName, newName string) error {
	err := repo.RewriteWorkspaceLocator(repo.RewriteWorkspaceLocatorRequest{
		WorkspaceRoot:         folder,
		ExpectedRepoID:        repoID,
		ExpectedRepoRoot:      repoRoot,
		ExpectedWorkspaceName: oldName,
		NewRepoRoot:           repoRoot,
		NewWorkspaceName:      newName,
	})
	if err == nil {
		return nil
	}
	diagnostic, inspectErr := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         folder,
		ExpectedRepoRoot:      repoRoot,
		ExpectedRepoID:        repoID,
		ExpectedWorkspaceName: newName,
	})
	if inspectErr == nil && diagnostic.Matches {
		return nil
	}
	return err
}

func workspaceRenameMetadata(record lifecycle.OperationRecord) (oldName, newName, folder string, locatorPresent bool, err error) {
	oldName, _ = record.Metadata["old_workspace"].(string)
	newName, _ = record.Metadata["new_workspace"].(string)
	folder, _ = record.Metadata["folder"].(string)
	locatorPresent, _ = record.Metadata["locator_present"].(bool)
	if oldName == "" || newName == "" || folder == "" {
		return "", "", "", false, fmt.Errorf("workspace rename journal is missing identity metadata")
	}
	return oldName, newName, folder, locatorPresent, nil
}

func workspaceRenameLocatorRequired(record lifecycle.OperationRecord) bool {
	required, _ := record.Metadata["locator_required"].(bool)
	return required
}

func workspaceRenameOperationID(oldName, newName string) string {
	return "workspace-rename-" + oldName + "-to-" + newName
}
