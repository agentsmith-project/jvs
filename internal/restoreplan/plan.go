// Package restoreplan builds and persists preview plans for destructive restore
// operations.
package restoreplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	SchemaVersion = 1
	sampleLimit   = 5
	metadataFloor = 1 << 20

	ScopeWhole = "whole"
	ScopePath  = "path"
)

var planCapacityGate = capacitygate.Default()
var planTransferPlanner transfer.EnginePlanner = engine.TransferPlanner{}

type Options struct {
	DiscardUnsaved bool `json:"discard_unsaved,omitempty"`
	SaveFirst      bool `json:"save_first,omitempty"`
}

type ExpectedSeparatedContext struct {
	RepoID      string
	ControlRoot string
	PayloadRoot string
	Workspace   string
}

type ChangeSummary struct {
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

type ManagedFilesImpact struct {
	Overwrite ChangeSummary `json:"overwrite"`
	Delete    ChangeSummary `json:"delete"`
	Create    ChangeSummary `json:"create"`
}

type Plan struct {
	SchemaVersion           int                `json:"schema_version"`
	RepoID                  string             `json:"repo_id"`
	PlanID                  string             `json:"plan_id"`
	CreatedAt               time.Time          `json:"created_at"`
	Scope                   string             `json:"scope,omitempty"`
	Folder                  string             `json:"folder"`
	Workspace               string             `json:"workspace"`
	SourceSavePoint         model.SnapshotID   `json:"source_save_point"`
	Path                    string             `json:"path,omitempty"`
	NewestSavePoint         *model.SnapshotID  `json:"newest_save_point"`
	HistoryHead             *model.SnapshotID  `json:"history_head"`
	ExpectedNewestSavePoint *model.SnapshotID  `json:"expected_newest_save_point"`
	ExpectedFolderEvidence  string             `json:"expected_folder_evidence,omitempty"`
	ExpectedPathEvidence    string             `json:"expected_path_evidence,omitempty"`
	Options                 Options            `json:"options,omitempty"`
	ManagedFiles            ManagedFilesImpact `json:"managed_files"`
	Transfers               []transfer.Record  `json:"transfers,omitempty"`
	RunCommand              string             `json:"run_command"`
	DecisionOnly            bool               `json:"decision_only,omitempty"`
}

func (p *Plan) EffectiveScope() string {
	if p == nil || p.Scope == "" {
		return ScopeWhole
	}
	return p.Scope
}

func (p *Plan) IsRunnable() bool {
	return p != nil && !p.DecisionOnly && strings.TrimSpace(p.PlanID) != "" && strings.TrimSpace(p.RunCommand) != ""
}

func SetCapacityGateForTest(gate capacitygate.Gate) func() {
	previous := planCapacityGate
	planCapacityGate = gate
	return func() {
		planCapacityGate = previous
	}
}

func SetTransferPlannerForTest(planner transfer.EnginePlanner) func() {
	previous := planTransferPlanner
	planTransferPlanner = planner
	return func() {
		planTransferPlanner = previous
	}
}

func Create(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, options Options) (*Plan, error) {
	return CreateWithExpectedSeparatedContext(repoRoot, workspaceName, sourceID, engineType, options, ExpectedSeparatedContext{})
}

func CreateWithExpectedSeparatedContext(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, options Options, expected ExpectedSeparatedContext) (*Plan, error) {
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan, err := buildWholePreviewPlan(repoRoot, workspaceName, sourceID, engineType, options)
	if err != nil {
		return nil, err
	}
	makePlanRunnable(plan, expected)
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	if err := Write(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func CreateDecisionPreview(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType) (*Plan, error) {
	return CreateDecisionPreviewWithExpectedSeparatedContext(repoRoot, workspaceName, sourceID, engineType, ExpectedSeparatedContext{})
}

func CreateDecisionPreviewWithExpectedSeparatedContext(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, expected ExpectedSeparatedContext) (*Plan, error) {
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan, err := buildWholePreviewPlan(repoRoot, workspaceName, sourceID, engineType, Options{})
	if err != nil {
		return nil, err
	}
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan.DecisionOnly = true
	return plan, nil
}

func buildWholePreviewPlan(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, options Options) (*Plan, error) {
	if options.DiscardUnsaved && options.SaveFirst {
		return nil, fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
	}
	if sourceID == "" {
		return nil, fmt.Errorf("source save point is required")
	}
	if err := sourceID.Validate(); err != nil {
		return nil, fmt.Errorf("source save point: %w", err)
	}

	repoID, err := currentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("workspace folder: %w", err)
	}
	evidence, err := WorkspaceEvidence(repoRoot, workspaceName)
	if err != nil {
		return nil, err
	}
	sourceState, err := InspectSourceReadOnly(repoRoot, sourceID)
	if err != nil {
		return nil, err
	}
	if _, err := checkPreviewCapacity(repoRoot, folder, workspaceName, sourceID, "", sourceState.SnapshotDir, sourceState.Descriptor); err != nil {
		return nil, err
	}
	sourceRoot, cleanup, transferRecord, err := validateSourcePayloadWithTransfer(repoRoot, workspaceName, sourceID, engineType, restorePreviewValidationTransferOptions(folder))
	if err != nil {
		return nil, err
	}
	defer cleanup()
	impact, err := computeManagedImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot)
	if err != nil {
		return nil, err
	}

	expectedNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	plan := &Plan{
		SchemaVersion:           SchemaVersion,
		RepoID:                  repoID,
		CreatedAt:               time.Now().UTC(),
		Scope:                   ScopeWhole,
		Folder:                  folder,
		Workspace:               workspaceName,
		SourceSavePoint:         sourceID,
		NewestSavePoint:         cloneSnapshotIDPtr(expectedNewest),
		HistoryHead:             cloneSnapshotIDPtr(expectedNewest),
		ExpectedNewestSavePoint: cloneSnapshotIDPtr(expectedNewest),
		ExpectedFolderEvidence:  evidence,
		Options:                 options,
		ManagedFiles:            impact,
		Transfers:               transferRecordsFrom(transferRecord),
	}
	return plan, nil
}

func CreatePath(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType, options Options) (*Plan, error) {
	return CreatePathWithExpectedSeparatedContext(repoRoot, workspaceName, sourceID, path, engineType, options, ExpectedSeparatedContext{})
}

func CreatePathWithExpectedSeparatedContext(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType, options Options, expected ExpectedSeparatedContext) (*Plan, error) {
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan, err := buildPathPreviewPlan(repoRoot, workspaceName, sourceID, path, engineType, options)
	if err != nil {
		return nil, err
	}
	makePlanRunnable(plan, expected)
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	if err := Write(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func CreatePathDecisionPreview(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType) (*Plan, error) {
	return CreatePathDecisionPreviewWithExpectedSeparatedContext(repoRoot, workspaceName, sourceID, path, engineType, ExpectedSeparatedContext{})
}

func CreatePathDecisionPreviewWithExpectedSeparatedContext(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType, expected ExpectedSeparatedContext) (*Plan, error) {
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan, err := buildPathPreviewPlan(repoRoot, workspaceName, sourceID, path, engineType, Options{})
	if err != nil {
		return nil, err
	}
	if err := validateExpectedSeparatedContext(repoRoot, workspaceName, expected); err != nil {
		return nil, err
	}
	plan.DecisionOnly = true
	return plan, nil
}

func buildPathPreviewPlan(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType, options Options) (*Plan, error) {
	if options.DiscardUnsaved && options.SaveFirst {
		return nil, fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
	}
	if sourceID == "" {
		return nil, fmt.Errorf("source save point is required")
	}
	if err := sourceID.Validate(); err != nil {
		return nil, fmt.Errorf("source save point: %w", err)
	}
	path, err := validatePlanRelativePath(path)
	if err != nil {
		return nil, err
	}

	repoID, err := currentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("workspace folder: %w", err)
	}
	pathEvidence, err := PathEvidence(repoRoot, workspaceName, path)
	if err != nil {
		return nil, err
	}
	sourceState, err := InspectSourceReadOnly(repoRoot, sourceID)
	if err != nil {
		return nil, err
	}
	if _, err := checkPreviewCapacity(repoRoot, folder, workspaceName, sourceID, path, sourceState.SnapshotDir, sourceState.Descriptor); err != nil {
		return nil, err
	}
	publishedDestination := filepath.Join(folder, filepath.FromSlash(path))
	sourceRoot, cleanup, transferRecord, err := validateSourcePayloadWithTransfer(repoRoot, workspaceName, sourceID, engineType, restorePreviewValidationTransferOptions(publishedDestination))
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if err := validateSourcePathExists(sourceRoot, path); err != nil {
		return nil, sourceNotRestorableError(err)
	}
	impact, err := computeManagedPathImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot, path)
	if err != nil {
		return nil, err
	}

	expectedNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	plan := &Plan{
		SchemaVersion:           SchemaVersion,
		RepoID:                  repoID,
		CreatedAt:               time.Now().UTC(),
		Scope:                   ScopePath,
		Folder:                  folder,
		Workspace:               workspaceName,
		SourceSavePoint:         sourceID,
		Path:                    path,
		NewestSavePoint:         cloneSnapshotIDPtr(expectedNewest),
		HistoryHead:             cloneSnapshotIDPtr(expectedNewest),
		ExpectedNewestSavePoint: cloneSnapshotIDPtr(expectedNewest),
		ExpectedPathEvidence:    pathEvidence,
		Options:                 options,
		ManagedFiles:            impact,
		Transfers:               transferRecordsFrom(transferRecord),
	}
	return plan, nil
}

func makePlanRunnable(plan *Plan, expected ExpectedSeparatedContext) {
	planID := uuidutil.NewV4()
	plan.PlanID = planID
	plan.RunCommand = restoreRunCommand(planID, expected)
}

func restoreRunCommand(planID string, expected ExpectedSeparatedContext) string {
	prefix := "jvs"
	if strings.TrimSpace(expected.ControlRoot) != "" {
		workspace := strings.TrimSpace(expected.Workspace)
		if workspace == "" {
			workspace = "main"
		}
		prefix += " --control-root " + shellQuoteArg(expected.ControlRoot) + " --workspace " + shellQuoteArg(workspace)
	}
	return prefix + " restore --run " + planID
}

func Write(repoRoot string, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, repo.JVSDirName, "restore-plans"), 0755); err != nil {
		return fmt.Errorf("create restore plan directory: %w", err)
	}
	path, err := repo.RestorePlanPathForWrite(repoRoot, plan.PlanID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal restore plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func Load(repoRoot, planID string) (*Plan, error) {
	path, err := repo.RestorePlanPathForRead(repoRoot, planID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("restore plan %q not found", planID)
		}
		return nil, fmt.Errorf("restore plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("restore plan %q not found", planID)
		}
		return nil, fmt.Errorf("restore plan %q is not readable", planID)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("restore plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("restore plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("restore plan %q plan_id does not match request", planID)
	}
	repoID, err := currentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("restore plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func Discard(repoRoot, workspaceName, planID string) (*Plan, error) {
	plan, err := Load(repoRoot, planID)
	if err != nil {
		return nil, err
	}
	if plan.Workspace != workspaceName {
		return nil, fmt.Errorf("restore plan %q belongs to workspace %q, not %q", planID, plan.Workspace, workspaceName)
	}
	if !plan.IsRunnable() {
		return nil, fmt.Errorf("restore plan %q is not a runnable restore preview", planID)
	}
	path, err := repo.RestorePlanPathForRead(repoRoot, planID)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("restore plan %q not found", planID)
		}
		return nil, fmt.Errorf("discard restore plan %q: %w", planID, err)
	}
	return plan, nil
}

func ValidateTarget(repoRoot, workspaceName string, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if plan.EffectiveScope() != ScopeWhole {
		return fmt.Errorf("restore plan scope is not whole")
	}
	if plan.Workspace != workspaceName {
		return changedSincePreviewError()
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return fmt.Errorf("workspace folder: %w", err)
	}
	if folder != plan.Folder {
		return changedSincePreviewError()
	}
	currentNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	if !sameSnapshotIDPtr(currentNewest, plan.ExpectedNewestSavePoint) {
		return changedSincePreviewError()
	}
	evidence, err := WorkspaceEvidence(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if evidence != plan.ExpectedFolderEvidence {
		return changedSincePreviewError()
	}
	return nil
}

func ValidatePathTarget(repoRoot, workspaceName string, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if plan.EffectiveScope() != ScopePath {
		return fmt.Errorf("restore plan scope is not path")
	}
	if plan.Workspace != workspaceName {
		return changedSincePreviewError()
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return fmt.Errorf("workspace folder: %w", err)
	}
	if folder != plan.Folder {
		return changedSincePreviewError()
	}
	currentNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	if !sameSnapshotIDPtr(currentNewest, plan.ExpectedNewestSavePoint) {
		return changedSincePreviewError()
	}
	if strings.TrimSpace(plan.Path) == "" || strings.TrimSpace(plan.ExpectedPathEvidence) == "" {
		return changedSincePreviewError()
	}
	evidence, err := PathEvidence(repoRoot, workspaceName, plan.Path)
	if err != nil {
		return changedSincePreviewError()
	}
	if evidence != plan.ExpectedPathEvidence {
		return changedSincePreviewError()
	}
	return nil
}

func ValidateSource(repoRoot, workspaceName string, plan *Plan, engineType model.EngineType) (*transfer.Record, error) {
	if plan == nil {
		return nil, fmt.Errorf("restore plan is required")
	}
	if plan.SourceSavePoint == "" {
		return nil, sourceNotRestorableError(fmt.Errorf("source save point is required"))
	}
	publishedDestination, err := restoreValidationPublishedDestination(repoRoot, workspaceName, plan)
	if err != nil {
		return nil, err
	}
	_, cleanup, transferRecord, err := validateSourcePayloadWithTransfer(repoRoot, workspaceName, plan.SourceSavePoint, engineType, restoreRunValidationTransferOptions(publishedDestination))
	if err != nil {
		return nil, sourceNotRestorableError(err)
	}
	defer cleanup()
	return transferRecord, nil
}

func ValidateSourcePath(repoRoot, workspaceName string, plan *Plan, engineType model.EngineType) (*transfer.Record, error) {
	if plan == nil {
		return nil, fmt.Errorf("restore plan is required")
	}
	if plan.SourceSavePoint == "" {
		return nil, sourceNotRestorableError(fmt.Errorf("source save point is required"))
	}
	if strings.TrimSpace(plan.Path) == "" {
		return nil, sourceNotRestorableError(fmt.Errorf("path is required"))
	}
	publishedDestination, err := restoreValidationPublishedDestination(repoRoot, workspaceName, plan)
	if err != nil {
		return nil, err
	}
	sourceRoot, cleanup, transferRecord, err := validateSourcePayloadWithTransfer(repoRoot, workspaceName, plan.SourceSavePoint, engineType, restoreRunValidationTransferOptions(publishedDestination))
	if err != nil {
		return nil, sourceNotRestorableError(err)
	}
	defer cleanup()
	if err := validateSourcePathExists(sourceRoot, plan.Path); err != nil {
		return nil, sourceNotRestorableError(err)
	}
	return transferRecord, nil
}

type sourceTransferOptions struct {
	RecordTransfer       bool
	TransferID           string
	Operation            string
	Phase                string
	Primary              bool
	ResultKind           transfer.ResultKind
	PermissionScope      transfer.PermissionScope
	SourceRole           string
	DestinationRole      string
	PublishedDestination string
	TempParentPattern    string
}

func restorePreviewValidationTransferOptions(publishedDestination string) sourceTransferOptions {
	return sourceTransferOptions{
		RecordTransfer:       true,
		TransferID:           "restore-preview-validation-primary",
		Operation:            "restore",
		Phase:                "preview_validation",
		Primary:              true,
		ResultKind:           transfer.ResultKindExpected,
		PermissionScope:      transfer.PermissionScopePreviewOnly,
		SourceRole:           "save_point_payload",
		DestinationRole:      "restore_preview_validation",
		PublishedDestination: publishedDestination,
		TempParentPattern:    "restore-preview-*",
	}
}

func restoreRunValidationTransferOptions(publishedDestination string) sourceTransferOptions {
	return sourceTransferOptions{
		RecordTransfer:       true,
		TransferID:           "restore-run-source-validation",
		Operation:            "restore",
		Phase:                "source_validation",
		Primary:              false,
		ResultKind:           transfer.ResultKindFinal,
		PermissionScope:      transfer.PermissionScopeExecution,
		SourceRole:           "save_point_payload",
		DestinationRole:      "restore_source_validation",
		PublishedDestination: publishedDestination,
		TempParentPattern:    "restore-run-validation-*",
	}
}

func restoreValidationPublishedDestination(repoRoot, workspaceName string, plan *Plan) (string, error) {
	folder := strings.TrimSpace(plan.Folder)
	if folder == "" {
		var err error
		folder, err = worktree.NewManager(repoRoot).Path(workspaceName)
		if err != nil {
			return "", fmt.Errorf("workspace folder: %w", err)
		}
	}
	if plan.EffectiveScope() == ScopePath {
		cleanPath, err := validatePlanRelativePath(plan.Path)
		if err != nil {
			return "", err
		}
		return filepath.Join(folder, filepath.FromSlash(cleanPath)), nil
	}
	return folder, nil
}

func InspectSourceReadOnly(repoRoot string, sourceID model.SnapshotID) (*snapshot.PublishState, error) {
	state, issue := snapshot.InspectPublishState(repoRoot, sourceID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        false,
	})
	if issue != nil {
		return state, sourceNotRestorableError(snapshot.PublishStateIssueError(issue))
	}
	return state, nil
}

func checkPreviewCapacity(repoRoot, folder, workspaceName string, sourceID model.SnapshotID, path, snapshotDir string, desc *model.Descriptor) (*capacitygate.Decision, error) {
	sourceEstimate, err := snapshotpayload.EstimateMaterializationCapacity(snapshotDir, snapshotpayload.OptionsFromDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return planCapacityGate.Check(capacitygate.Request{
		Operation:       "restore preview",
		Folder:          folder,
		Workspace:       workspaceName,
		SourceSavePoint: string(sourceID),
		Path:            path,
		Components: []capacitygate.Component{
			{Name: "source hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-probe"), Bytes: sourceEstimate.PeakBytes},
			{Name: "source validation", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-preview-probe", "source"), Bytes: sourceEstimate.PeakBytes},
			{Name: "impact preview", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-preview-probe", "impact"), Bytes: sourceEstimate.PeakBytes},
			{Name: "restore plan", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-plans"), Bytes: metadataFloor},
		},
		FailureMessages: []string{"No restore plan was created.", "No files were changed."},
	})
}

func validateSourcePayloadWithTransfer(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, transferOptions sourceTransferOptions) (string, func(), *transfer.Record, error) {
	if err := snapshot.VerifySnapshot(repoRoot, sourceID, true); err != nil {
		return "", func() {}, nil, err
	}
	desc, err := snapshot.LoadDescriptor(repoRoot, sourceID)
	if err != nil {
		return "", func() {}, nil, err
	}
	if desc.SnapshotID != sourceID {
		return "", func() {}, nil, fmt.Errorf("descriptor save point ID %s does not match requested %s", desc.SnapshotID, sourceID)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", func() {}, nil, err
	}
	sourceRoot, cleanup, transferRecord, err := materializeSourceWithTransfer(repoRoot, sourceID, desc, engineType, transferOptions)
	if err != nil {
		return "", cleanup, nil, err
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(sourceRoot); err != nil {
		cleanup()
		return "", func() {}, nil, err
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, sourceRoot); err != nil {
		cleanup()
		return "", func() {}, nil, err
	}
	return sourceRoot, cleanup, transferRecord, nil
}

func WorkspaceEvidence(repoRoot, workspaceName string) (string, error) {
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return "", err
	}
	hash, err := integrity.ComputePayloadRootHashWithExclusions(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return "", fmt.Errorf("scan folder evidence: %w", err)
	}
	return string(hash), nil
}

func PathEvidence(repoRoot, workspaceName, normalizedPath string) (string, error) {
	cleanPath, err := validatePlanRelativePath(normalizedPath)
	if err != nil {
		return "", err
	}
	normalizedPath = cleanPath
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if boundary.ExcludesRelativePath(normalizedPath) {
		return "", fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return "", err
	}
	if err := pathutil.ValidateNoSymlinkParents(boundary.Root, normalizedPath); err != nil {
		return "", fmt.Errorf("path parent containment for %s: %w", normalizedPath, err)
	}

	return PathEvidenceFromRoot(boundary.Root, normalizedPath, boundary.ExcludesRelativePath)
}

// PathEvidenceFromRoot computes path-level evidence for a managed payload root.
func PathEvidenceFromRoot(root, normalizedPath string, excluded func(rel string) bool) (string, error) {
	cleanPath, err := validatePlanRelativePath(normalizedPath)
	if err != nil {
		return "", err
	}
	normalizedPath = cleanPath
	if excluded != nil && excluded(normalizedPath) {
		return "", fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(root); err != nil {
		return "", err
	}
	if err := pathutil.ValidateNoSymlinkParents(root, normalizedPath); err != nil {
		return "", fmt.Errorf("path parent containment for %s: %w", normalizedPath, err)
	}

	target := filepath.Join(root, filepath.FromSlash(normalizedPath))
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return hashEvidenceLines([]string{"missing\t" + normalizedPath}), nil
	}
	if err != nil {
		return "", fmt.Errorf("stat path evidence %s: %w", normalizedPath, err)
	}

	lines := []string{}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		err = filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("relative path: %w", err)
			}
			rel = filepath.ToSlash(rel)
			if excluded != nil && excluded(rel) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("stat %s: %w", rel, err)
			}
			line, err := evidenceLineForPath(path, rel, info)
			if err != nil {
				return err
			}
			lines = append(lines, line)
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("scan path evidence: %w", err)
		}
	} else {
		line, err := evidenceLineForPath(target, normalizedPath, info)
		if err != nil {
			return "", fmt.Errorf("scan path evidence: %w", err)
		}
		lines = append(lines, line)
	}
	return hashEvidenceLines(lines), nil
}

func ComputeManagedImpact(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType) (ManagedFilesImpact, error) {
	desc, err := snapshot.LoadDescriptor(repoRoot, sourceID)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: %w", err)
	}
	if desc.SnapshotID != sourceID {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: descriptor save point ID %s does not match requested %s", desc.SnapshotID, sourceID)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return ManagedFilesImpact{}, err
	}
	sourceRoot, cleanup, err := materializeSource(repoRoot, sourceID, desc, engineType)
	if err != nil {
		return ManagedFilesImpact{}, err
	}
	defer cleanup()
	return computeManagedImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot)
}

func computeManagedImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot string) (ManagedFilesImpact, error) {
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return ManagedFilesImpact{}, err
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(sourceRoot); err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("source save point payload: %w", err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, sourceRoot); err != nil {
		return ManagedFilesImpact{}, err
	}

	currentFiles, err := scanManagedFiles(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan folder: %w", err)
	}
	sourceFiles, err := scanManagedFiles(sourceRoot, boundary.ExcludesRelativePath)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan source save point: %w", err)
	}
	return compareManagedFiles(currentFiles, sourceFiles), nil
}

func ComputeManagedPathImpact(repoRoot, workspaceName string, sourceID model.SnapshotID, path string, engineType model.EngineType) (ManagedFilesImpact, error) {
	cleanPath, err := validatePlanRelativePath(path)
	if err != nil {
		return ManagedFilesImpact{}, err
	}
	path = cleanPath
	desc, err := snapshot.LoadDescriptor(repoRoot, sourceID)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: %w", err)
	}
	if desc.SnapshotID != sourceID {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: descriptor save point ID %s does not match requested %s", desc.SnapshotID, sourceID)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("workspace path: %w", err)
	}
	if boundary.ExcludesRelativePath(path) {
		return ManagedFilesImpact{}, fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return ManagedFilesImpact{}, err
	}
	sourceRoot, cleanup, err := materializeSource(repoRoot, sourceID, desc, engineType)
	if err != nil {
		return ManagedFilesImpact{}, err
	}
	defer cleanup()
	return computeManagedPathImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot, path)
}

func computeManagedPathImpactFromSourceRoot(repoRoot, workspaceName, sourceRoot, path string) (ManagedFilesImpact, error) {
	cleanPath, err := validatePlanRelativePath(path)
	if err != nil {
		return ManagedFilesImpact{}, err
	}
	path = cleanPath
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("workspace path: %w", err)
	}
	if boundary.ExcludesRelativePath(path) {
		return ManagedFilesImpact{}, fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return ManagedFilesImpact{}, err
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(sourceRoot); err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("source save point payload: %w", err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, sourceRoot); err != nil {
		return ManagedFilesImpact{}, err
	}
	if err := validateSourcePathExists(sourceRoot, path); err != nil {
		return ManagedFilesImpact{}, err
	}

	currentFiles, err := scanManagedFilesUnder(boundary.Root, boundary.ExcludesRelativePath, path)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan folder: %w", err)
	}
	sourceFiles, err := scanManagedFilesUnder(sourceRoot, boundary.ExcludesRelativePath, path)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan source save point: %w", err)
	}
	return compareManagedFiles(currentFiles, sourceFiles), nil
}

func materializeSource(repoRoot string, sourceID model.SnapshotID, desc *model.Descriptor, engineType model.EngineType) (string, func(), error) {
	sourceRoot, cleanup, _, err := materializeSourceWithTransfer(repoRoot, sourceID, desc, engineType, sourceTransferOptions{})
	return sourceRoot, cleanup, err
}

func materializeSourceWithTransfer(repoRoot string, sourceID model.SnapshotID, desc *model.Descriptor, engineType model.EngineType, transferOptions sourceTransferOptions) (string, func(), *transfer.Record, error) {
	tempParentPattern := strings.TrimSpace(transferOptions.TempParentPattern)
	if tempParentPattern == "" {
		tempParentPattern = "restore-preview-*"
	}
	tmpParent, err := os.MkdirTemp(filepath.Join(repoRoot, repo.JVSDirName), tempParentPattern)
	if err != nil {
		return "", func() {}, nil, fmt.Errorf("create restore preview staging: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpParent) }
	sourceRoot := filepath.Join(tmpParent, "source")
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, sourceID)
	if err != nil {
		cleanup()
		return "", func() {}, nil, fmt.Errorf("source save point path: %w", err)
	}

	intent := transfer.Intent{
		TransferID:                 transferOptions.TransferID,
		Operation:                  transferOptions.Operation,
		Phase:                      transferOptions.Phase,
		Primary:                    transferOptions.Primary,
		ResultKind:                 transferOptions.ResultKind,
		PermissionScope:            transferOptions.PermissionScope,
		SourceRole:                 transferOptions.SourceRole,
		SourcePath:                 snapshotDir,
		DestinationRole:            transferOptions.DestinationRole,
		MaterializationDestination: sourceRoot,
		CapabilityProbePath:        tmpParent,
		PublishedDestination:       transferOptions.PublishedDestination,
		RequestedEngine:            engineType,
	}
	plan, err := transfer.PlanIntent(planTransferPlanner, intent)
	if err != nil {
		cleanup()
		return "", func() {}, nil, fmt.Errorf("plan transfer: %w", err)
	}

	eng := engine.NewEngine(plan.TransferEngine)
	var runtimeResult *engine.CloneResult
	if err := snapshotpayload.MaterializeToNew(snapshotDir, sourceRoot, snapshotpayload.OptionsFromDescriptor(desc), func(src, dst string) error {
		var cloneErr error
		runtimeResult, cloneErr = engine.CloneToNew(eng, src, dst)
		return cloneErr
	}); err != nil {
		cleanup()
		return "", func() {}, nil, fmt.Errorf("materialize source save point: %w", err)
	}

	var transferRecord *transfer.Record
	if transferOptions.RecordTransfer {
		record := transfer.RecordFromPlanAndRuntime(intent, plan, runtimeResult)
		transferRecord = &record
	}
	return sourceRoot, cleanup, transferRecord, nil
}

func transferRecordsFrom(record *transfer.Record) []transfer.Record {
	if record == nil {
		return nil
	}
	return []transfer.Record{*record}
}

type fileSignature struct {
	Kind string
	Mode os.FileMode
	Size int64
	Hash string
}

func evidenceLineForPath(path, rel string, info os.FileInfo) (string, error) {
	mode := info.Mode()
	switch {
	case mode.IsDir():
		return fmt.Sprintf("dir\t%s\t%o", rel, mode.Perm()), nil
	case mode&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("read symlink %s: %w", rel, err)
		}
		return fmt.Sprintf("symlink\t%s\t%o\t%s", rel, mode.Perm(), target), nil
	case mode.IsRegular():
		sig, err := signatureForPath(path, info)
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", rel, err)
		}
		return fmt.Sprintf("file\t%s\t%o\t%d\t%s", rel, sig.Mode, sig.Size, sig.Hash), nil
	default:
		return "", fmt.Errorf("unsupported path type %s", rel)
	}
}

func hashEvidenceLines(lines []string) string {
	sort.Strings(lines)
	h := sha256.New()
	_, _ = io.WriteString(h, "path-evidence-v1\n")
	for _, line := range lines {
		_, _ = io.WriteString(h, line)
		_, _ = io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func scanManagedFiles(root string, excluded func(rel string) bool) (map[string]fileSignature, error) {
	files := map[string]fileSignature{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if excluded != nil && excluded(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.IsDir() {
			return nil
		}
		sig, err := signatureForPath(path, info)
		if err != nil {
			return fmt.Errorf("hash %s: %w", rel, err)
		}
		files[rel] = sig
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func scanManagedFilesUnder(root string, excluded func(rel string) bool, scope string) (map[string]fileSignature, error) {
	files, err := scanManagedFiles(root, excluded)
	if err != nil {
		return nil, err
	}
	scoped := map[string]fileSignature{}
	for rel, sig := range files {
		if rel == scope || strings.HasPrefix(rel, scope+"/") {
			scoped[rel] = sig
		}
	}
	return scoped, nil
}

func validateSourcePathExists(sourceRoot, path string) error {
	cleanPath, err := validatePlanRelativePath(path)
	if err != nil {
		return err
	}
	path = cleanPath
	if err := pathutil.ValidateNoSymlinkParents(sourceRoot, path); err != nil {
		return fmt.Errorf("source path parent containment for %s: %w", path, err)
	}
	target := filepath.Join(sourceRoot, filepath.FromSlash(path))
	if _, err := os.Lstat(target); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist in save point: %s", path)
		}
		return fmt.Errorf("stat source path %s: %w", path, err)
	}
	return nil
}

func validatePlanRelativePath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("path is required")
	}
	clean, err := model.NormalizeWorkspaceRelativePathKey(raw)
	if err != nil {
		return "", fmt.Errorf("path must be a workspace-relative path: %w", err)
	}
	if clean != raw {
		return "", fmt.Errorf("path must be normalized")
	}
	if clean == repo.JVSDirName || strings.HasPrefix(clean, repo.JVSDirName+"/") {
		return "", fmt.Errorf("path must be a workspace-relative path; JVS control data is not managed")
	}
	return clean, nil
}

func signatureForPath(path string, info os.FileInfo) (fileSignature, error) {
	sig := fileSignature{
		Mode: info.Mode().Perm(),
		Size: info.Size(),
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return fileSignature{}, fmt.Errorf("read symlink: %w", err)
		}
		sum := sha256.Sum256([]byte(target))
		sig.Kind = "symlink"
		sig.Hash = hex.EncodeToString(sum[:])
		return sig, nil
	default:
		f, err := os.Open(path)
		if err != nil {
			return fileSignature{}, fmt.Errorf("open file: %w", err)
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fileSignature{}, fmt.Errorf("read file: %w", err)
		}
		sig.Kind = "file"
		sig.Hash = hex.EncodeToString(h.Sum(nil))
		return sig, nil
	}
}

func compareManagedFiles(current, source map[string]fileSignature) ManagedFilesImpact {
	var overwrite, deletePaths, createPaths []string
	for path, currentSig := range current {
		sourceSig, ok := source[path]
		if !ok {
			deletePaths = append(deletePaths, path)
			continue
		}
		if currentSig != sourceSig {
			overwrite = append(overwrite, path)
		}
	}
	for path := range source {
		if _, ok := current[path]; !ok {
			createPaths = append(createPaths, path)
		}
	}
	return ManagedFilesImpact{
		Overwrite: summarizePaths(overwrite),
		Delete:    summarizePaths(deletePaths),
		Create:    summarizePaths(createPaths),
	}
}

func summarizePaths(paths []string) ChangeSummary {
	sort.Strings(paths)
	samples := paths
	if len(samples) > sampleLimit {
		samples = samples[:sampleLimit]
	}
	return ChangeSummary{
		Count:   len(paths),
		Samples: append([]string(nil), samples...),
	}
}

func changedSincePreviewError() error {
	return fmt.Errorf("folder changed since preview; run preview again. No files were changed.")
}

func IsChangedSincePreview(err error) bool {
	return err != nil && strings.Contains(err.Error(), "folder changed since preview")
}

func sourceNotRestorableError(cause error) error {
	return fmt.Errorf("source save point is not restorable: %v. No files were changed.", cause)
}

func currentRepoID(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, repo.JVSDirName, repo.RepoIDFile))
	if err != nil {
		return "", fmt.Errorf("read repository identity: %w", err)
	}
	return string(bytesTrimSpace(data)), nil
}

func validateExpectedSeparatedContext(repoRoot, workspaceName string, expected ExpectedSeparatedContext) error {
	if strings.TrimSpace(expected.RepoID) == "" &&
		strings.TrimSpace(expected.ControlRoot) == "" &&
		strings.TrimSpace(expected.PayloadRoot) == "" &&
		strings.TrimSpace(expected.Workspace) == "" {
		return nil
	}
	controlRoot := strings.TrimSpace(expected.ControlRoot)
	if controlRoot == "" {
		controlRoot = repoRoot
	}
	workspace := strings.TrimSpace(expected.Workspace)
	if workspace == "" {
		workspace = workspaceName
	}
	_, err := repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
		ControlRoot:         controlRoot,
		Workspace:           workspace,
		ExpectedRepoID:      expected.RepoID,
		ExpectedPayloadRoot: expected.PayloadRoot,
	})
	return err
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		if isShellSafeRune(r) {
			continue
		}
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}

func isShellSafeRune(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '_', '-', '.', '/', ':', '@', '%', '+', '=':
		return true
	default:
		return false
	}
}

func snapshotIDPtrOrNil(id model.SnapshotID) *model.SnapshotID {
	if id == "" {
		return nil
	}
	value := id
	return &value
}

func cloneSnapshotIDPtr(id *model.SnapshotID) *model.SnapshotID {
	if id == nil {
		return nil
	}
	value := *id
	return &value
}

func sameSnapshotIDPtr(left, right *model.SnapshotID) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func bytesTrimSpace(data []byte) []byte {
	start := 0
	for start < len(data) && isSpace(data[start]) {
		start++
	}
	end := len(data)
	for end > start && isSpace(data[end-1]) {
		end--
	}
	return data[start:end]
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}
