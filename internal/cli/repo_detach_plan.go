package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	repoDetachPlanSchemaVersion = 1
	repoDetachPlansDirName      = "repo-detach-plans"
	repoDetachArchiveRootName   = ".jvs-detached"
	repoDetachJournalType       = "repo detach"
	repoDetachOperation         = "repo_detach"
	repoDetachTimestampFormat   = "20060102T150405Z"
)

type repoDetachPlanRunHooks struct {
	afterArchive  func() error
	afterDetached func() error
}

var repoDetachRunHooks repoDetachPlanRunHooks

type repoDetachPlan struct {
	SchemaVersion      int                           `json:"schema_version"`
	RepoID             string                        `json:"repo_id"`
	PlanID             string                        `json:"plan_id"`
	OperationID        string                        `json:"operation_id"`
	CreatedAt          time.Time                     `json:"created_at"`
	Operation          string                        `json:"operation"`
	RepoRoot           string                        `json:"repo_root"`
	ArchivePath        string                        `json:"archive_path"`
	ExternalWorkspaces []repoDetachExternalWorkspace `json:"external_workspaces"`
	RunCommand         string                        `json:"run_command"`
}

type repoDetachExternalWorkspace struct {
	Workspace string `json:"workspace"`
	Folder    string `json:"folder"`
}

type publicRepoDetachPreviewResult struct {
	Mode                    string `json:"mode"`
	Operation               string `json:"operation"`
	PlanID                  string `json:"plan_id"`
	OperationID             string `json:"operation_id"`
	RepoRoot                string `json:"repo_root"`
	RepoID                  string `json:"repo_id"`
	ArchivePath             string `json:"archive_path"`
	WorkingFilesPreserved   bool   `json:"working_files_preserved"`
	ActiveRepoDetached      bool   `json:"active_repo_detached"`
	SavePointStorageRemoved bool   `json:"save_point_storage_removed"`
	ExternalWorkspaces      int    `json:"external_workspaces"`
	RunCommand              string `json:"run_command"`
}

type publicRepoDetachRunResult struct {
	Mode                      string `json:"mode"`
	Operation                 string `json:"operation"`
	PlanID                    string `json:"plan_id"`
	OperationID               string `json:"operation_id"`
	Status                    string `json:"status"`
	RepoRoot                  string `json:"repo_root"`
	RepoID                    string `json:"repo_id"`
	ArchivePath               string `json:"archive_path"`
	WorkingFilesPreserved     bool   `json:"working_files_preserved"`
	ActiveRepoDetached        bool   `json:"active_repo_detached"`
	SavePointStorageRemoved   bool   `json:"save_point_storage_removed"`
	ExternalWorkspacesUpdated int    `json:"external_workspaces_updated"`
	RecommendedNextCommand    string `json:"recommended_next_command"`
}

type repoDetachMarker struct {
	SchemaVersion          int                                  `json:"schema_version"`
	Status                 string                               `json:"status"`
	Operation              string                               `json:"operation"`
	PlanID                 string                               `json:"plan_id"`
	OperationID            string                               `json:"operation_id"`
	RepoID                 string                               `json:"repo_id"`
	OldRepoRoot            string                               `json:"old_repo_root"`
	ArchivePath            string                               `json:"archive_path"`
	ExpectedActiveJVS      repoDetachActiveJVSIdentity          `json:"expected_active_jvs_identity"`
	RegisteredWorkspaces   repoDetachRegisteredWorkspaceSummary `json:"registered_workspace_summary"`
	RecommendedNextCommand string                               `json:"recommended_next_command"`
	CreatedAt              time.Time                            `json:"created_at"`
	UpdatedAt              time.Time                            `json:"updated_at"`
}

type repoDetachActiveJVSIdentity struct {
	Path              string `json:"path"`
	RepoID            string `json:"repo_id"`
	FormatVersion     int    `json:"format_version"`
	MainWorkspace     string `json:"main_workspace"`
	MainWorkspaceRoot string `json:"main_workspace_root"`
}

type repoDetachRegisteredWorkspaceSummary struct {
	Main          repoDetachExternalWorkspace   `json:"main"`
	External      []repoDetachExternalWorkspace `json:"external"`
	ExternalCount int                           `json:"external_count"`
}

type repoDetachRunTarget struct {
	ProjectRoot string
	ControlRoot string
	ArchivePath string
	Archived    bool
	Complete    bool
	Marker      repoDetachMarker
}

func createRepoDetachPlan(repoRoot string) (*repoDetachPlan, error) {
	sourceRoot, err := canonicalPhysicalRepoRoot(repoRoot)
	if err != nil {
		return nil, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	repoID, err := workspaceCurrentRepoID(sourceRoot)
	if err != nil {
		return nil, err
	}
	if err := validateRepoDetachActiveRepo(sourceRoot, repoID); err != nil {
		return nil, err
	}
	external, err := repoDetachExternalWorkspacePlans(sourceRoot, repoID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	planID := uuidutil.NewV4()
	operationID := uuidutil.NewV4()
	plan := &repoDetachPlan{
		SchemaVersion:      repoDetachPlanSchemaVersion,
		RepoID:             repoID,
		PlanID:             planID,
		OperationID:        operationID,
		CreatedAt:          now,
		Operation:          repoDetachOperation,
		RepoRoot:           sourceRoot,
		ArchivePath:        repoDetachArchivePath(sourceRoot, repoID, operationID, now),
		ExternalWorkspaces: external,
		RunCommand:         "jvs repo detach --run " + planID,
	}
	if err := writeRepoDetachPlan(sourceRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func executeRepoDetachRunFromCWD(planID string) (publicRepoDetachRunResult, error) {
	target, err := resolveRepoDetachRunTarget(planID)
	if err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if target.Complete {
		return finishCompletedRepoDetachTarget(target)
	}
	if target.Archived {
		return executeArchivedRepoDetachPlan(target, planID)
	}
	return executeActiveRepoDetachPlan(target.ControlRoot, planID)
}

func finishCompletedRepoDetachTarget(target repoDetachRunTarget) (publicRepoDetachRunResult, error) {
	if target.Marker.OperationID != "" {
		if err := lifecycle.ConsumeOperation(target.ControlRoot, target.Marker.OperationID); err != nil {
			return publicRepoDetachRunResult{}, err
		}
	}
	return publicRepoDetachRunResultFromMarker(target.Marker), nil
}

func executeActiveRepoDetachPlan(repoRoot, planID string) (publicRepoDetachRunResult, error) {
	currentRoot, err := canonicalPhysicalRepoRoot(repoRoot)
	if err != nil {
		return publicRepoDetachRunResult{}, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
	}
	plan, err := loadRepoDetachPlan(currentRoot, planID)
	if err != nil {
		return publicRepoDetachRunResult{}, err
	}
	record, pending, err := pendingRepoDetachPlanRecord(currentRoot, plan)
	if err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if pending {
		return resumeActiveRepoDetachPlan(plan, record)
	}
	if err := prepareFreshRepoDetachRun(plan); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	record = repoDetachOperationRecord(plan, "prepared")
	if err := repoMoveWithPremoveLock(plan.RepoRoot, "repo detach run", func() error {
		return lifecycle.WriteOperation(plan.RepoRoot, record)
	}); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	return archiveRepoDetachFromPrepared(plan, record)
}

func executeArchivedRepoDetachPlan(target repoDetachRunTarget, planID string) (publicRepoDetachRunResult, error) {
	plan, err := loadRepoDetachPlan(target.ControlRoot, planID)
	if err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if target.Marker.PlanID != "" && !repoDetachMarkerMatchesPlan(target.Marker, plan) {
		return publicRepoDetachRunResult{}, fmt.Errorf("repo detach marker does not match plan %q", planID)
	}
	record, pending, err := pendingRepoDetachPlanRecord(target.ControlRoot, plan)
	if err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if !pending {
		return publicRepoDetachRunResult{}, fmt.Errorf("repo detach plan %q is archived but has no pending lifecycle journal", planID)
	}
	return resumeArchivedRepoDetachPlan(plan, record)
}

func resumeActiveRepoDetachPlan(plan *repoDetachPlan, record lifecycle.OperationRecord) (publicRepoDetachRunResult, error) {
	if record.Phase != "prepared" {
		return publicRepoDetachRunResult{}, fmt.Errorf("repo detach is pending in unsupported active phase %q", record.Phase)
	}
	if err := prepareFreshRepoDetachRun(plan); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	return archiveRepoDetachFromPrepared(plan, record)
}

func resumeArchivedRepoDetachPlan(plan *repoDetachPlan, record lifecycle.OperationRecord) (publicRepoDetachRunResult, error) {
	if err := validateRepoDetachArchivedIdentity(plan); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	switch record.Phase {
	case "prepared", "archived", "external_locators_detached":
		return finishRepoDetachArchive(plan, record)
	default:
		return publicRepoDetachRunResult{}, fmt.Errorf("repo detach is pending in unsupported archived phase %q", record.Phase)
	}
}

func prepareFreshRepoDetachRun(plan *repoDetachPlan) error {
	if plan == nil {
		return fmt.Errorf("repo detach plan is required")
	}
	if err := validateRepoDetachActiveRepo(plan.RepoRoot, plan.RepoID); err != nil {
		return err
	}
	if err := validateRepoDetachArchiveAvailable(plan); err != nil {
		return err
	}
	return validateRepoDetachExternalWorkspaces(plan)
}

func archiveRepoDetachFromPrepared(plan *repoDetachPlan, record lifecycle.OperationRecord) (publicRepoDetachRunResult, error) {
	if record.OperationID != plan.OperationID {
		return publicRepoDetachRunResult{}, fmt.Errorf("repo detach lifecycle journal operation_id does not match plan")
	}
	if err := createRepoDetachArchiveMarker(plan, record); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if err := lifecycle.MoveSameFilesystemNoOverwrite(filepath.Join(plan.RepoRoot, repo.JVSDirName), filepath.Join(plan.ArchivePath, repo.JVSDirName)); err != nil {
		return publicRepoDetachRunResult{}, fmt.Errorf("archive active JVS metadata: %w", err)
	}
	if repoDetachRunHooks.afterArchive != nil {
		if err := repoDetachRunHooks.afterArchive(); err != nil {
			return publicRepoDetachRunResult{}, err
		}
	}
	return finishRepoDetachArchive(plan, record)
}

func finishRepoDetachArchive(plan *repoDetachPlan, record lifecycle.OperationRecord) (publicRepoDetachRunResult, error) {
	if err := validateRepoDetachArchivedIdentity(plan); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	updated := 0
	if err := repo.WithMutationLock(plan.ArchivePath, "repo detach finish", func() error {
		if record.Phase == "prepared" {
			if err := workspaceLifecycleWritePhase(plan.ArchivePath, &record, "archived"); err != nil {
				return err
			}
		}
		if record.Phase == "archived" {
			for _, external := range plan.ExternalWorkspaces {
				if err := repo.WriteDetachedWorkspaceLocator(repo.DetachWorkspaceLocatorRequest{
					WorkspaceRoot:          external.Folder,
					ExpectedRepoRoot:       plan.RepoRoot,
					ExpectedRepoID:         plan.RepoID,
					ExpectedWorkspaceName:  external.Workspace,
					OperationID:            record.OperationID,
					DetachedAt:             time.Now().UTC(),
					RecommendedNextCommand: plan.RunCommand,
				}); err != nil {
					return err
				}
				updated++
			}
			if err := workspaceLifecycleWritePhase(plan.ArchivePath, &record, "external_locators_detached"); err != nil {
				return err
			}
		}
		if record.Phase == "external_locators_detached" {
			if err := writeRepoDetachMarker(filepath.Join(plan.ArchivePath, "DETACHED"), repoDetachMarkerFromPlan(plan, record, "detached")); err != nil {
				return err
			}
			if repoDetachRunHooks.afterDetached != nil {
				if err := repoDetachRunHooks.afterDetached(); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if err := lifecycle.ConsumeOperation(plan.ArchivePath, record.OperationID); err != nil {
		return publicRepoDetachRunResult{}, err
	}
	if updated == 0 {
		updated = len(plan.ExternalWorkspaces)
	}
	return publicRepoDetachRunResultFromPlan(plan, record.OperationID, updated), nil
}

func createRepoDetachArchiveMarker(plan *repoDetachPlan, record lifecycle.OperationRecord) error {
	parent := filepath.Join(plan.RepoRoot, repoDetachArchiveRootName)
	if err := ensureRepoDetachArchiveRoot(parent, plan.RepoRoot); err != nil {
		return err
	}
	if err := os.Mkdir(plan.ArchivePath, 0755); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("repo detach archive path already exists: %s", plan.ArchivePath)
		}
		return fmt.Errorf("create repo detach archive directory: %w", err)
	}
	if err := fsutil.FsyncDir(parent); err != nil {
		return fmt.Errorf("fsync repo detach archive root: %w", err)
	}
	return writeRepoDetachMarker(filepath.Join(plan.ArchivePath, "DETACHING"), repoDetachMarkerFromPlan(plan, record, "detaching"))
}

func ensureRepoDetachArchiveRoot(path, repoRoot string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repo detach archive root is symlink: %s", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("repo detach archive root is not a directory: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat repo detach archive root: %w", err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		return fmt.Errorf("create repo detach archive root: %w", err)
	}
	return fsutil.FsyncDir(repoRoot)
}

func validateRepoDetachActiveRepo(repoRoot, repoID string) error {
	identity, err := repoMoveRootIdentity(repoRoot, repoID)
	if err != nil {
		return err
	}
	if identity.State != workspaceLifecycleIdentityExpected {
		return fmt.Errorf("active repo identity mismatch: %s", identity.Reason)
	}
	return validateRepoMoveMainAtSource(repoRoot)
}

func validateRepoDetachArchiveAvailable(plan *repoDetachPlan) error {
	parent := filepath.Join(plan.RepoRoot, repoDetachArchiveRootName)
	info, err := os.Lstat(parent)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repo detach archive root is symlink: %s", parent)
		}
		if !info.IsDir() {
			return fmt.Errorf("repo detach archive root is not a directory: %s", parent)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat repo detach archive root: %w", err)
	}
	if info, err := os.Lstat(plan.ArchivePath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repo detach archive path is symlink: %s", plan.ArchivePath)
		}
		return fmt.Errorf("repo detach archive path already exists: %s", plan.ArchivePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat repo detach archive path: %w", err)
	}
	return nil
}

func validateRepoDetachArchivedIdentity(plan *repoDetachPlan) error {
	if info, err := os.Lstat(filepath.Join(plan.RepoRoot, repo.JVSDirName)); err == nil {
		if info.IsDir() {
			return fmt.Errorf("repo detach cannot resume: active .jvs exists at old repo root")
		}
		return fmt.Errorf("repo detach cannot resume: reserved .jvs path exists at old repo root")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat old active .jvs path: %w", err)
	}
	archiveJVS := filepath.Join(plan.ArchivePath, repo.JVSDirName)
	info, err := os.Lstat(archiveJVS)
	if err != nil {
		return fmt.Errorf("repo detach cannot resume: archived .jvs missing: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("repo detach cannot resume: archived .jvs identity changed")
	}
	repoID, err := workspaceCurrentRepoID(plan.ArchivePath)
	if err != nil {
		return fmt.Errorf("repo detach cannot resume: read archived repo identity: %w", err)
	}
	if repoID != plan.RepoID {
		return fmt.Errorf("repo detach cannot resume: archived repo_id changed")
	}
	return nil
}

func repoDetachExternalWorkspacePlans(repoRoot, repoID string) ([]repoDetachExternalWorkspace, error) {
	moveExternal, err := repoMoveExternalWorkspacePlans(repoRoot, repoID)
	if err != nil {
		return nil, err
	}
	external := make([]repoDetachExternalWorkspace, 0, len(moveExternal))
	for _, item := range moveExternal {
		external = append(external, repoDetachExternalWorkspace{
			Workspace: item.Workspace,
			Folder:    item.Folder,
		})
	}
	return external, nil
}

func validateRepoDetachExternalWorkspaces(plan *repoDetachPlan) error {
	for _, external := range plan.ExternalWorkspaces {
		if err := validateRepoDetachExternalWorkspace(plan, external); err != nil {
			return err
		}
	}
	return nil
}

func validateRepoDetachExternalWorkspace(plan *repoDetachPlan, external repoDetachExternalWorkspace) error {
	return validateRepoMoveExternalWorkspace(&repoMovePlan{RepoID: plan.RepoID}, repoMoveExternalWorkspace{
		Workspace: external.Workspace,
		Folder:    external.Folder,
	}, plan.RepoRoot)
}

func pendingRepoDetachPlanRecord(controlRoot string, plan *repoDetachPlan) (lifecycle.OperationRecord, bool, error) {
	record, err := lifecycle.ReadOperation(controlRoot, plan.OperationID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lifecycle.OperationRecord{}, false, nil
		}
		return lifecycle.OperationRecord{}, false, err
	}
	if record.Phase == lifecycle.PhaseConsumed {
		return lifecycle.OperationRecord{}, false, nil
	}
	if record.OperationType != repoDetachJournalType || record.RepoID != plan.RepoID {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match repo detach plan", record.OperationID)
	}
	if !workspaceLifecycleMetadataMatches(record, map[string]string{
		"plan_id":      plan.PlanID,
		"repo_root":    plan.RepoRoot,
		"archive_path": plan.ArchivePath,
	}) {
		return lifecycle.OperationRecord{}, false, fmt.Errorf("pending lifecycle journal %q does not match repo detach plan", record.OperationID)
	}
	return record, true, nil
}

func repoDetachOperationRecord(plan *repoDetachPlan, phase string) lifecycle.OperationRecord {
	return workspaceLifecycleOperationRecord(plan.RepoID, plan.OperationID, repoDetachJournalType, phase, plan.RunCommand, map[string]any{
		"plan_id":      plan.PlanID,
		"operation":    plan.Operation,
		"repo_root":    plan.RepoRoot,
		"archive_path": plan.ArchivePath,
	})
}

func publicRepoDetachPreviewFromPlan(plan *repoDetachPlan) publicRepoDetachPreviewResult {
	return publicRepoDetachPreviewResult{
		Mode:                    "preview",
		Operation:               plan.Operation,
		PlanID:                  plan.PlanID,
		OperationID:             plan.OperationID,
		RepoRoot:                plan.RepoRoot,
		RepoID:                  plan.RepoID,
		ArchivePath:             plan.ArchivePath,
		WorkingFilesPreserved:   true,
		ActiveRepoDetached:      false,
		SavePointStorageRemoved: false,
		ExternalWorkspaces:      len(plan.ExternalWorkspaces),
		RunCommand:              plan.RunCommand,
	}
}

func publicRepoDetachRunResultFromPlan(plan *repoDetachPlan, operationID string, externalUpdated int) publicRepoDetachRunResult {
	return publicRepoDetachRunResult{
		Mode:                      "run",
		Operation:                 plan.Operation,
		PlanID:                    plan.PlanID,
		OperationID:               operationID,
		Status:                    "detached",
		RepoRoot:                  plan.RepoRoot,
		RepoID:                    plan.RepoID,
		ArchivePath:               plan.ArchivePath,
		WorkingFilesPreserved:     true,
		ActiveRepoDetached:        true,
		SavePointStorageRemoved:   false,
		ExternalWorkspacesUpdated: externalUpdated,
		RecommendedNextCommand:    plan.RunCommand,
	}
}

func publicRepoDetachRunResultFromMarker(marker repoDetachMarker) publicRepoDetachRunResult {
	return publicRepoDetachRunResult{
		Mode:                      "run",
		Operation:                 marker.Operation,
		PlanID:                    marker.PlanID,
		OperationID:               marker.OperationID,
		Status:                    "detached",
		RepoRoot:                  marker.OldRepoRoot,
		RepoID:                    marker.RepoID,
		ArchivePath:               marker.ArchivePath,
		WorkingFilesPreserved:     true,
		ActiveRepoDetached:        true,
		SavePointStorageRemoved:   false,
		ExternalWorkspacesUpdated: marker.RegisteredWorkspaces.ExternalCount,
		RecommendedNextCommand:    marker.RecommendedNextCommand,
	}
}

func writeRepoDetachPlan(controlRoot string, plan *repoDetachPlan) error {
	if plan == nil {
		return fmt.Errorf("repo detach plan is required")
	}
	dir, err := repoDetachPlansDir(controlRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create repo detach plan directory: %w", err)
	}
	if err := validateRepoDetachPlanDir(dir); err != nil {
		return err
	}
	path, err := repoDetachPlanPath(controlRoot, plan.PlanID, true)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repo detach plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func loadRepoDetachPlan(controlRoot, planID string) (*repoDetachPlan, error) {
	path, err := repoDetachPlanPath(controlRoot, planID, false)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("repo detach plan %q not found", planID)
		}
		return nil, fmt.Errorf("repo detach plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("repo detach plan %q not found", planID)
		}
		return nil, fmt.Errorf("repo detach plan %q is not readable", planID)
	}
	var plan repoDetachPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("repo detach plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != repoDetachPlanSchemaVersion {
		return nil, fmt.Errorf("repo detach plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("repo detach plan %q plan_id does not match request", planID)
	}
	if err := pathutil.ValidateName(plan.OperationID); err != nil {
		return nil, fmt.Errorf("repo detach plan %q has invalid operation_id: %w", planID, err)
	}
	repoID, err := workspaceCurrentRepoID(controlRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("repo detach plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func repoDetachPlanPath(controlRoot, planID string, missingOK bool) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	dir, err := repoDetachPlansDir(controlRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, planID+".json")
	if err := validateRepoDetachPlanLeaf(path, missingOK); err != nil {
		return "", err
	}
	return path, nil
}

func repoDetachPlansDir(controlRoot string) (string, error) {
	return filepath.Join(controlRoot, repo.JVSDirName, repoDetachPlansDirName), nil
}

func validateRepoDetachPlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat repo detach plan directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repo detach plan directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo detach plan path is not directory: %s", path)
	}
	return nil
}

func validateRepoDetachPlanLeaf(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repo detach plan is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("repo detach plan is not a regular file: %s", path)
	}
	return nil
}

func resolveRepoDetachRunTarget(planID string) (repoDetachRunTarget, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return repoDetachRunTarget{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return repoDetachRunTarget{}, errclass.ErrUsage.WithMessagef("cannot get current directory: %v", err)
	}
	path, err := filepath.Abs(cwd)
	if err != nil {
		return repoDetachRunTarget{}, err
	}
	path = filepath.Clean(path)
	for {
		activeJVS := filepath.Join(path, repo.JVSDirName)
		if info, err := os.Lstat(activeJVS); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return repoDetachRunTarget{}, fmt.Errorf("active .jvs path is symlink: %s", activeJVS)
			}
			if info.IsDir() {
				root, err := canonicalPhysicalRepoRoot(path)
				if err != nil {
					return repoDetachRunTarget{}, errclass.ErrUsage.WithMessagef("resolve repository root: %v", err)
				}
				return repoDetachRunTarget{ProjectRoot: root, ControlRoot: root}, nil
			}
		} else if !os.IsNotExist(err) {
			return repoDetachRunTarget{}, fmt.Errorf("stat active .jvs path: %w", err)
		}

		target, ok, err := findRepoDetachArchiveForPlan(path, planID)
		if err != nil {
			return repoDetachRunTarget{}, err
		}
		if ok {
			return target, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return repoDetachRunTarget{}, errclass.ErrNotRepo.
				WithMessage("not an active JVS repository and no matching repo detach archive was found").
				WithHint("Run from the project folder that created the detach preview.")
		}
		path = parent
	}
}

func findRepoDetachArchiveForPlan(projectRoot, planID string) (repoDetachRunTarget, bool, error) {
	detachedRoot := filepath.Join(projectRoot, repoDetachArchiveRootName)
	info, err := os.Lstat(detachedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return repoDetachRunTarget{}, false, nil
		}
		return repoDetachRunTarget{}, false, fmt.Errorf("stat repo detach archive root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return repoDetachRunTarget{}, false, fmt.Errorf("repo detach archive root is symlink: %s", detachedRoot)
	}
	if !info.IsDir() {
		return repoDetachRunTarget{}, false, fmt.Errorf("repo detach archive root is not a directory: %s", detachedRoot)
	}
	entries, err := os.ReadDir(detachedRoot)
	if err != nil {
		return repoDetachRunTarget{}, false, fmt.Errorf("read repo detach archive root: %w", err)
	}
	var matches []repoDetachRunTarget
	for _, entry := range entries {
		archivePath := filepath.Join(detachedRoot, entry.Name())
		entryInfo, err := os.Lstat(archivePath)
		if err != nil {
			return repoDetachRunTarget{}, false, fmt.Errorf("stat repo detach archive entry: %w", err)
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return repoDetachRunTarget{}, false, fmt.Errorf("repo detach archive entry is symlink: %s", archivePath)
		}
		if !entryInfo.IsDir() {
			continue
		}
		if marker, ok, err := readRepoDetachMarkerOptional(filepath.Join(archivePath, "DETACHED")); err != nil {
			return repoDetachRunTarget{}, false, err
		} else if ok {
			if marker.PlanID == planID {
				matches = append(matches, repoDetachRunTarget{
					ProjectRoot: projectRoot,
					ControlRoot: archivePath,
					ArchivePath: archivePath,
					Archived:    true,
					Complete:    true,
					Marker:      marker,
				})
			}
			continue
		}
		marker, ok, err := readRepoDetachMarkerOptional(filepath.Join(archivePath, "DETACHING"))
		if err != nil {
			return repoDetachRunTarget{}, false, err
		}
		if ok && marker.PlanID == planID {
			matches = append(matches, repoDetachRunTarget{
				ProjectRoot: projectRoot,
				ControlRoot: archivePath,
				ArchivePath: archivePath,
				Archived:    true,
				Marker:      marker,
			})
		}
	}
	if len(matches) > 1 {
		return repoDetachRunTarget{}, false, fmt.Errorf("multiple repo detach archives match plan %q", planID)
	}
	if len(matches) == 0 {
		return repoDetachRunTarget{}, false, nil
	}
	return matches[0], true, nil
}

func readRepoDetachMarkerOptional(path string) (repoDetachMarker, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoDetachMarker{}, false, nil
		}
		return repoDetachMarker{}, false, fmt.Errorf("stat repo detach marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return repoDetachMarker{}, false, fmt.Errorf("repo detach marker is symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return repoDetachMarker{}, false, fmt.Errorf("repo detach marker is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return repoDetachMarker{}, false, fmt.Errorf("read repo detach marker: %w", err)
	}
	var marker repoDetachMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return repoDetachMarker{}, false, fmt.Errorf("parse repo detach marker %s: %w", path, err)
	}
	if marker.SchemaVersion != repoDetachPlanSchemaVersion {
		return repoDetachMarker{}, false, fmt.Errorf("repo detach marker %s has unsupported schema version", path)
	}
	if marker.Status != "detaching" && marker.Status != "detached" {
		return repoDetachMarker{}, false, fmt.Errorf("repo detach marker %s has unsupported status %q", path, marker.Status)
	}
	return marker, true, nil
}

func writeRepoDetachMarker(path string, marker repoDetachMarker) error {
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repo detach marker: %w", err)
	}
	if err := fsutil.AtomicWrite(path, data, 0644); err != nil {
		return fmt.Errorf("write repo detach marker: %w", err)
	}
	return nil
}

func repoDetachMarkerFromPlan(plan *repoDetachPlan, record lifecycle.OperationRecord, status string) repoDetachMarker {
	now := time.Now().UTC()
	return repoDetachMarker{
		SchemaVersion:          repoDetachPlanSchemaVersion,
		Status:                 status,
		Operation:              plan.Operation,
		PlanID:                 plan.PlanID,
		OperationID:            record.OperationID,
		RepoID:                 plan.RepoID,
		OldRepoRoot:            plan.RepoRoot,
		ArchivePath:            plan.ArchivePath,
		ExpectedActiveJVS:      repoDetachActiveIdentity(plan),
		RegisteredWorkspaces:   repoDetachRegisteredSummary(plan),
		RecommendedNextCommand: plan.RunCommand,
		CreatedAt:              plan.CreatedAt,
		UpdatedAt:              now,
	}
}

func repoDetachActiveIdentity(plan *repoDetachPlan) repoDetachActiveJVSIdentity {
	return repoDetachActiveJVSIdentity{
		Path:              filepath.Join(plan.RepoRoot, repo.JVSDirName),
		RepoID:            plan.RepoID,
		FormatVersion:     repo.FormatVersion,
		MainWorkspace:     "main",
		MainWorkspaceRoot: plan.RepoRoot,
	}
}

func repoDetachRegisteredSummary(plan *repoDetachPlan) repoDetachRegisteredWorkspaceSummary {
	external := append([]repoDetachExternalWorkspace(nil), plan.ExternalWorkspaces...)
	return repoDetachRegisteredWorkspaceSummary{
		Main:          repoDetachExternalWorkspace{Workspace: "main", Folder: plan.RepoRoot},
		External:      external,
		ExternalCount: len(external),
	}
}

func repoDetachMarkerMatchesPlan(marker repoDetachMarker, plan *repoDetachPlan) bool {
	return marker.PlanID == plan.PlanID &&
		marker.OperationID == plan.OperationID &&
		marker.RepoID == plan.RepoID &&
		workspaceLifecycleSamePath(marker.OldRepoRoot, plan.RepoRoot) &&
		workspaceLifecycleSamePath(marker.ArchivePath, plan.ArchivePath)
}

func repoDetachArchivePath(repoRoot, repoID, operationID string, at time.Time) string {
	name := strings.Join([]string{repoID, operationID, at.UTC().Format(repoDetachTimestampFormat)}, "-")
	return filepath.Join(repoRoot, repoDetachArchiveRootName, name)
}
