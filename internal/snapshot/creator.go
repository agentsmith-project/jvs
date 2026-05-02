// Package snapshot handles snapshot creation, listing, and querying.
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/audit"
	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/workspacepath"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

// Creator handles snapshot creation using the 12-step protocol.
type Creator struct {
	repoRoot         string
	engineType       model.EngineType
	engine           engine.Engine
	auditLogger      *audit.FileAppender
	compression      *compression.Compressor
	transferPlanner  transfer.EnginePlanner
	cloneToNew       func(engine.Engine, string, string) (*engine.CloneResult, error)
	lastTransfer     *transfer.Record
	descriptorWriter func(string, *model.Descriptor) error
	snapshotRenamer  func(string, string) error
	latestUpdater    func(*worktree.Manager, string, model.SnapshotID) error
}

type snapshotPublishPaths struct {
	tmpDir         string
	dir            string
	descriptorPath string
}

type descriptorLineageFunc func(model.SnapshotID, *model.WorktreeConfig) model.WorkspaceSaveLineage
type SnapshotPayloadStagedHook func(model.SnapshotID, string) error

var ErrDescriptorNotFound = errors.New("descriptor not found")
var afterSnapshotPayloadStagedHook SnapshotPayloadStagedHook

// SetAfterSnapshotPayloadStagedHookForTest installs a hook used by tests to
// deterministically simulate a workspace write after payload staging.
func SetAfterSnapshotPayloadStagedHookForTest(hook SnapshotPayloadStagedHook) func() {
	previous := afterSnapshotPayloadStagedHook
	afterSnapshotPayloadStagedHook = hook
	return func() {
		afterSnapshotPayloadStagedHook = previous
	}
}

func IsDescriptorNotFound(err error) bool {
	return errors.Is(err, ErrDescriptorNotFound) || errors.Is(err, os.ErrNotExist)
}

// NewCreator creates a new snapshot creator.
func NewCreator(repoRoot string, engineType model.EngineType) *Creator {
	return NewCreatorWithCompression(repoRoot, engineType, nil)
}

// NewCreatorWithCompression creates a new snapshot creator with compression.
func NewCreatorWithCompression(repoRoot string, engineType model.EngineType, comp *compression.Compressor) *Creator {
	eng := engine.NewEngine(engineType)

	auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
	return &Creator{
		repoRoot:         repoRoot,
		engineType:       engineType,
		engine:           eng,
		auditLogger:      audit.NewFileAppender(auditPath),
		compression:      comp,
		transferPlanner:  engine.TransferPlanner{},
		cloneToNew:       engine.CloneToNew,
		descriptorWriter: writeDescriptorFile,
		snapshotRenamer:  fsutil.RenameNoReplaceAndSync,
		latestUpdater: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			return wtMgr.SetLatest(worktreeName, snapshotID)
		},
	}
}

// SetCompression sets the compression level for this creator.
func (c *Creator) SetCompression(level compression.CompressionLevel) {
	c.compression = compression.NewCompressor(level)
}

// LastTransferRecord returns the final primary transfer from the most recent
// successful public save-point creation handled by this creator.
func (c *Creator) LastTransferRecord() (transfer.Record, bool) {
	if c == nil || c.lastTransfer == nil {
		return transfer.Record{}, false
	}
	return *c.lastTransfer, true
}

// Create performs a full snapshot of the worktree using the 12-step protocol.
func (c *Creator) Create(worktreeName, note string, tags []string) (*model.Descriptor, error) {
	return c.CreatePartial(worktreeName, note, tags, nil)
}

// CreateWithParent performs a full snapshot while using parentID as the
// descriptor lineage parent, independent of the worktree's current head.
func (c *Creator) CreateWithParent(worktreeName, note string, tags []string, parentID model.SnapshotID) (*model.Descriptor, error) {
	var desc *model.Descriptor
	err := repo.WithMutationLock(c.repoRoot, "snapshot", func() error {
		var err error
		desc, err = c.createWithParent(worktreeName, note, tags, parentID)
		return err
	})
	return desc, err
}

// CreateSavePoint performs a full snapshot for the public save path. The
// descriptor parent is selected from the worktree's latest snapshot while the
// mutation lock is held, so concurrent saves cannot publish a stale lineage.
func (c *Creator) CreateSavePoint(worktreeName, note string, tags []string) (*model.Descriptor, error) {
	var desc *model.Descriptor
	err := repo.WithMutationLock(c.repoRoot, "snapshot", func() error {
		var err error
		desc, err = c.createSavePoint(worktreeName, note, tags)
		return err
	})
	return desc, err
}

// CreateSavePointLocked creates a public save point while the caller already
// holds the repository mutation lock.
func (c *Creator) CreateSavePointLocked(worktreeName, note string, tags []string) (*model.Descriptor, error) {
	return c.createSavePoint(worktreeName, note, tags)
}

// CreatePartial performs a snapshot of specific paths within the worktree.
// If paths is nil or empty, performs a full snapshot.
func (c *Creator) CreatePartial(worktreeName, note string, tags []string, paths []string) (*model.Descriptor, error) {
	var desc *model.Descriptor
	err := repo.WithMutationLock(c.repoRoot, "snapshot", func() error {
		var err error
		desc, err = c.createPartial(worktreeName, note, tags, paths)
		return err
	})
	return desc, err
}

func (c *Creator) createSavePoint(worktreeName, note string, tags []string) (*model.Descriptor, error) {
	cfg, err := worktree.NewManager(c.repoRoot).Get(worktreeName)
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}
	return c.createPartialWithDescriptorParentAndLineage(worktreeName, note, tags, nil, cfg.LatestSnapshotID, true, true, savePointLineage)
}

func (c *Creator) createWithParent(worktreeName, note string, tags []string, parentID model.SnapshotID) (*model.Descriptor, error) {
	if parentID != "" {
		if err := parentID.Validate(); err != nil {
			return nil, fmt.Errorf("validate parent save point: %w", err)
		}
		if _, err := LoadDescriptor(c.repoRoot, parentID); err != nil {
			return nil, fmt.Errorf("load parent save point: %w", err)
		}
	}
	return c.createPartialWithDescriptorParent(worktreeName, note, tags, nil, parentID, true)
}

func (c *Creator) createPartial(worktreeName, note string, tags []string, paths []string) (*model.Descriptor, error) {
	return c.createPartialWithDescriptorParent(worktreeName, note, tags, paths, "", false)
}

func (c *Creator) createPartialWithDescriptorParent(worktreeName, note string, tags []string, paths []string, parentOverride model.SnapshotID, overrideParent bool) (*model.Descriptor, error) {
	return c.createPartialWithDescriptorParentAndLineage(worktreeName, note, tags, paths, parentOverride, overrideParent, false, nil)
}

func (c *Creator) createPartialWithDescriptorParentAndLineage(worktreeName, note string, tags []string, paths []string, parentOverride model.SnapshotID, overrideParent bool, validateSaveEvidence bool, lineageFn descriptorLineageFunc) (*model.Descriptor, error) {
	c.lastTransfer = nil

	// Step 1: Validate worktree exists
	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}
	if overrideParent {
		if parentOverride != cfg.LatestSnapshotID {
			return nil, fmt.Errorf("snapshot parent %s is stale; latest is %s", parentOverride, cfg.LatestSnapshotID)
		}
	}

	// Normalize and validate paths if provided
	partialPaths, err := c.normalizePartialPaths(paths, worktreeName)
	if err != nil {
		return nil, err
	}

	// Step 2: Generate snapshot ID
	snapshotID := model.NewSnapshotID()

	publishPaths, err := c.resolveSnapshotPublishPaths(snapshotID)
	if err != nil {
		return nil, err
	}

	boundary, err := repo.WorktreeManagedPayloadBoundary(c.repoRoot, worktreeName)
	if err != nil {
		return nil, fmt.Errorf("worktree payload path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return nil, err
	}
	reconciledPathSources, err := workspacepath.ReconcilePathSources(c.repoRoot, boundary, cfg.PathSources)
	if err != nil {
		return nil, fmt.Errorf("reconcile restored paths: %w", err)
	}
	cfg.PathSources = reconciledPathSources
	descriptorCfg := *cfg
	if overrideParent {
		descriptorCfg.HeadSnapshotID = parentOverride
	}
	if err := c.auditLogger.EnsureAppendable(); err != nil {
		return nil, fmt.Errorf("audit log not appendable: %w", err)
	}
	var saveEvidence model.HashValue
	if validateSaveEvidence && len(partialPaths) == 0 {
		saveEvidence, err = computeSaveWorkspaceEvidence(boundary)
		if err != nil {
			return nil, fmt.Errorf("read workspace before saving: %w", err)
		}
	}

	// Step 3: Create intent record (for crash recovery)
	intentPath, err := c.writeCreateIntent(snapshotID, worktreeName)
	if err != nil {
		return nil, err
	}

	var lineage *model.WorkspaceSaveLineage
	if lineageFn != nil {
		nextLineage := lineageFn(snapshotID, cfg)
		lineage = &nextLineage
	}

	var transferIntent *transfer.Intent
	if validateSaveEvidence && len(partialPaths) == 0 {
		intent := c.savePrimaryTransferIntent(boundary, publishPaths)
		transferIntent = &intent
	}

	desc, stagedTransfer, err := c.stageSnapshot(&descriptorCfg, boundary, publishPaths, snapshotID, worktreeName, note, tags, partialPaths, lineage, saveEvidence, transferIntent)
	if err != nil {
		return nil, cleanupIntentAfterDefiniteFailure(intentPath, err)
	}

	// Step 12: Write descriptor atomically before publishing the READY payload.
	if err := c.publishStagedSnapshot(publishPaths, desc); err != nil {
		return nil, cleanupIntentAfterDefiniteFailure(intentPath, err)
	}

	// Step 14: Recheck audit appendability before the save point enters history.
	if err := c.ensureAuditAppendableBeforeHistoryUpdate(publishPaths, desc); err != nil {
		return nil, cleanupIntentAfterDefiniteFailure(intentPath, err)
	}

	// Step 15: Update worktree head and latest
	if err := c.updateLatestAfterPublish(wtMgr, worktreeName, desc, publishPaths); err != nil {
		return nil, cleanupIntentAfterDefiniteFailure(intentPath, err)
	}

	// Step 16: Remove intent only after the snapshot is fully published.
	removeSnapshotIntent(intentPath)

	// Step 17: Write audit log. Once history has changed, a late audit write
	// failure is reported as a warning so callers never see a failed save that
	// already entered history.
	if err := c.appendCreateAudit(worktreeName, snapshotID, note, desc, stagedTransfer, partialPaths); err != nil {
		fmt.Fprintf(os.Stderr, "warning: saved save point %s but could not write audit log: %v\n", snapshotID, err)
	}

	if stagedTransfer != nil {
		c.lastTransfer = stagedTransfer
	}

	return desc, nil
}

func (c *Creator) savePrimaryTransferIntent(boundary repo.WorktreePayloadBoundary, publishPaths snapshotPublishPaths) transfer.Intent {
	return transfer.Intent{
		TransferID:                 "save-primary",
		Operation:                  "save",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 transfer.ResultKindFinal,
		PermissionScope:            transfer.PermissionScopeExecution,
		SourceRole:                 "workspace_content",
		SourcePath:                 boundary.Root,
		DestinationRole:            "save_point_staging",
		MaterializationDestination: publishPaths.tmpDir,
		CapabilityProbePath:        filepath.Dir(publishPaths.tmpDir),
		PublishedDestination:       publishPaths.dir,
		RequestedEngine:            c.engineType,
	}
}

func savePointLineage(snapshotID model.SnapshotID, cfg *model.WorktreeConfig) model.WorkspaceSaveLineage {
	state := workspaceStateFromWorktreeConfig(cfg)
	return state.NextSaveLineage(snapshotID)
}

func workspaceStateFromWorktreeConfig(cfg *model.WorktreeConfig) model.WorkspaceState {
	if cfg == nil {
		return model.WorkspaceState{}
	}
	if cfg.LatestSnapshotID == "" {
		if cfg.StartedFromSnapshotID != "" {
			state := model.WorkspaceStateStartedFrom(cfg.StartedFromSnapshotID)
			if cfg.HeadSnapshotID != "" && cfg.HeadSnapshotID != cfg.StartedFromSnapshotID {
				state.RestoreWhole(cfg.HeadSnapshotID)
			}
			state.PathSources = cfg.PathSources.Clone()
			return state
		}
		return model.WorkspaceState{}
	}
	state := model.WorkspaceStateAtSavePoint(cfg.LatestSnapshotID)
	if cfg.StartedFromSnapshotID != "" {
		startedFrom := cfg.StartedFromSnapshotID
		state.StartedFrom = &startedFrom
	}
	if cfg.HeadSnapshotID != "" && cfg.HeadSnapshotID != cfg.LatestSnapshotID {
		state.RestoreWhole(cfg.HeadSnapshotID)
	}
	state.PathSources = cfg.PathSources.Clone()
	return state
}

func (c *Creator) normalizePartialPaths(paths []string, worktreeName string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	return c.validateAndNormalizePaths(paths, worktreeName)
}

func (c *Creator) resolveSnapshotPublishPaths(snapshotID model.SnapshotID) (snapshotPublishPaths, error) {
	tmpDir, err := repo.SnapshotTmpPath(c.repoRoot, snapshotID)
	if err != nil {
		return snapshotPublishPaths{}, fmt.Errorf("resolve snapshot tmp path: %w", err)
	}
	dir, err := repo.SnapshotPath(c.repoRoot, snapshotID)
	if err != nil {
		return snapshotPublishPaths{}, fmt.Errorf("resolve snapshot path: %w", err)
	}
	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(c.repoRoot, snapshotID)
	if err != nil {
		return snapshotPublishPaths{}, fmt.Errorf("resolve descriptor path: %w", err)
	}
	return snapshotPublishPaths{
		tmpDir:         tmpDir,
		dir:            dir,
		descriptorPath: descriptorPath,
	}, nil
}

func (c *Creator) writeCreateIntent(snapshotID model.SnapshotID, worktreeName string) (string, error) {
	intentPath, err := repo.IntentPath(c.repoRoot, snapshotID)
	if err != nil {
		return "", fmt.Errorf("resolve intent path: %w", err)
	}
	intent := &model.IntentRecord{
		SnapshotID:   snapshotID,
		WorktreeName: worktreeName,
		StartedAt:    time.Now().UTC(),
		Engine:       c.engineType,
	}
	if err := c.writeIntent(intentPath, intent); err != nil {
		return "", fmt.Errorf("write intent: %w", err)
	}
	return intentPath, nil
}

func (c *Creator) stageSnapshot(
	cfg *model.WorktreeConfig,
	boundary repo.WorktreePayloadBoundary,
	publishPaths snapshotPublishPaths,
	snapshotID model.SnapshotID,
	worktreeName string,
	note string,
	tags []string,
	partialPaths []string,
	lineage *model.WorkspaceSaveLineage,
	saveEvidence model.HashValue,
	transferIntent *transfer.Intent,
) (*model.Descriptor, *transfer.Record, error) {
	cleanupTmp := func() {
		os.RemoveAll(publishPaths.tmpDir)
	}

	cloneResult, transferRecord, err := c.cloneSnapshotPayload(boundary, publishPaths.tmpDir, partialPaths, transferIntent)
	if err != nil {
		cleanupTmp()
		return nil, nil, err
	}
	if afterSnapshotPayloadStagedHook != nil {
		if err := afterSnapshotPayloadStagedHook(snapshotID, publishPaths.tmpDir); err != nil {
			cleanupTmp()
			return nil, nil, fmt.Errorf("after snapshot payload staged hook: %w", err)
		}
	}

	// Step 6: Compute payload root hash before any storage-only compression.
	payloadHash, err := integrity.ComputePayloadRootHash(publishPaths.tmpDir)
	if err != nil {
		cleanupTmp()
		return nil, nil, fmt.Errorf("compute payload hash: %w", err)
	}
	if saveEvidence != "" && payloadHash != saveEvidence {
		cleanupTmp()
		return nil, nil, saveEvidenceChangedError()
	}

	// Step 7: Create descriptor
	desc := c.newSnapshotDescriptor(cfg, snapshotID, worktreeName, note, tags, payloadHash, partialPaths, cloneResult, transferRecord)
	if lineage != nil {
		lineage.ApplyToDescriptor(desc)
	}

	// Step 8: Compute descriptor checksum
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	if err != nil {
		cleanupTmp()
		return nil, nil, fmt.Errorf("compute checksum: %w", err)
	}
	desc.DescriptorChecksum = checksum

	// Step 9: Write .READY marker in tmp
	if err := c.writeSnapshotReadyMarker(publishPaths.tmpDir, snapshotID, payloadHash, checksum, desc.Engine); err != nil {
		cleanupTmp()
		return nil, nil, err
	}

	// Step 10: Compress snapshot storage inside the unpublished tmp tree.
	if err := c.compressSnapshotStorage(publishPaths.tmpDir); err != nil {
		cleanupTmp()
		return nil, nil, err
	}

	// Step 11: Fsync the final staged tree for durability.
	if err := fsutil.FsyncTree(publishPaths.tmpDir); err != nil {
		cleanupTmp()
		return nil, nil, fmt.Errorf("fsync snapshot tree: %w", err)
	}
	if saveEvidence != "" {
		currentEvidence, err := computeSaveWorkspaceEvidence(boundary)
		if err != nil {
			cleanupTmp()
			return nil, nil, saveEvidenceChangedError()
		}
		if currentEvidence != saveEvidence {
			cleanupTmp()
			return nil, nil, saveEvidenceChangedError()
		}
	}

	return desc, transferRecord, nil
}

func computeSaveWorkspaceEvidence(boundary repo.WorktreePayloadBoundary) (model.HashValue, error) {
	return integrity.ComputePayloadRootHashWithExclusions(boundary.Root, boundary.ExcludesRelativePath)
}

func saveEvidenceChangedError() error {
	return errors.New("workspace files changed while saving. No save point was created. Run jvs save again.")
}

func (c *Creator) cloneSnapshotPayload(boundary repo.WorktreePayloadBoundary, snapshotTmpDir string, partialPaths []string, intent *transfer.Intent) (*engine.CloneResult, *transfer.Record, error) {
	if intent == nil {
		result, err := c.cloneSnapshotPayloadRuntime(boundary, snapshotTmpDir, partialPaths)
		return result, nil, err
	}

	plan, err := transfer.PlanIntent(c.transferPlanner, *intent)
	if err != nil {
		return nil, nil, fmt.Errorf("plan transfer: %w", err)
	}

	originalEngine := c.engine
	c.engine = engine.NewEngine(plan.TransferEngine)
	result, err := c.cloneSnapshotPayloadRuntime(boundary, snapshotTmpDir, partialPaths)
	c.engine = originalEngine
	if err != nil {
		return nil, nil, err
	}

	record := transfer.RecordFromPlanAndRuntime(*intent, plan, result)
	return result, &record, nil
}

func (c *Creator) cloneSnapshotPayloadRuntime(boundary repo.WorktreePayloadBoundary, snapshotTmpDir string, partialPaths []string) (*engine.CloneResult, error) {
	// For partial snapshots, only copy specified paths
	if len(partialPaths) > 0 {
		if err := os.MkdirAll(snapshotTmpDir, 0755); err != nil {
			return nil, fmt.Errorf("create snapshot tmp dir: %w", err)
		}
		result, err := c.clonePaths(boundary.Root, snapshotTmpDir, partialPaths)
		if err != nil {
			return nil, fmt.Errorf("clone partial paths: %w", err)
		}
		return result, nil
	}
	if len(boundary.ExcludedRootNames) > 0 {
		return c.clonePayloadRootEntries(boundary, snapshotTmpDir)
	}
	result, err := c.cloneToNew(c.engine, boundary.Root, snapshotTmpDir)
	if err != nil {
		return nil, fmt.Errorf("clone payload: %w", err)
	}
	return result, nil
}

func (c *Creator) clonePayloadRootEntries(boundary repo.WorktreePayloadBoundary, snapshotTmpDir string) (*engine.CloneResult, error) {
	if err := engine.PrepareCloneToNewDestination(snapshotTmpDir); err != nil {
		return nil, err
	}
	if err := os.Mkdir(snapshotTmpDir, 0755); err != nil {
		return nil, fmt.Errorf("create snapshot tmp dir: %w", err)
	}

	entries, err := os.ReadDir(boundary.Root)
	if err != nil {
		return nil, fmt.Errorf("read payload root: %w", err)
	}

	combined := engine.NewCloneResult(c.runtimeEngineType())
	for _, entry := range entries {
		name := entry.Name()
		if boundary.ExcludesRelativePath(name) {
			continue
		}
		result, err := c.cloneToNew(c.engine, filepath.Join(boundary.Root, name), filepath.Join(snapshotTmpDir, name))
		if err != nil {
			return nil, fmt.Errorf("clone payload root entry %s: %w", name, err)
		}
		mergeCloneResult(combined, result)
	}
	if err := fsutil.FsyncDir(snapshotTmpDir); err != nil {
		return nil, fmt.Errorf("fsync snapshot tmp dir: %w", err)
	}
	return combined, nil
}

func (c *Creator) newSnapshotDescriptor(
	cfg *model.WorktreeConfig,
	snapshotID model.SnapshotID,
	worktreeName string,
	note string,
	tags []string,
	payloadHash model.HashValue,
	partialPaths []string,
	cloneResult *engine.CloneResult,
	transferRecord *transfer.Record,
) *model.Descriptor {
	var parentID *model.SnapshotID
	if cfg.HeadSnapshotID != "" {
		pid := cfg.HeadSnapshotID
		parentID = &pid
	}

	descriptorEngine := cloneDescriptorEngine(cloneResult, transferRecord, c.engineType)
	desc := &model.Descriptor{
		SnapshotID:           snapshotID,
		ParentID:             parentID,
		WorktreeName:         worktreeName,
		CreatedAt:            time.Now().UTC(),
		Note:                 note,
		Tags:                 tags,
		Engine:               descriptorEngine,
		ActualEngine:         cloneActualEngine(cloneResult, descriptorEngine),
		EffectiveEngine:      cloneEffectiveEngine(cloneResult, transferRecord, descriptorEngine),
		DegradedReasons:      cloneDegradedReasons(cloneResult, transferRecord),
		MetadataPreservation: cloneMetadataPreservation(cloneResult, transferRecord, descriptorEngine),
		PerformanceClass:     clonePerformanceClass(cloneResult, transferRecord, descriptorEngine),
		PayloadRootHash:      payloadHash,
		IntegrityState:       model.IntegrityVerified,
		PartialPaths:         partialPaths,
	}
	if c.compression != nil && c.compression.IsEnabled() {
		desc.Compression = &model.CompressionInfo{
			Type:  string(c.compression.Type),
			Level: int(c.compression.Level),
		}
	}
	return desc
}

func cloneDescriptorEngine(result *engine.CloneResult, record *transfer.Record, fallback model.EngineType) model.EngineType {
	if isConcreteSnapshotEngine(fallback) {
		return fallback
	}
	if record != nil && isConcreteSnapshotEngine(record.EffectiveEngine) {
		return record.EffectiveEngine
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) {
		return result.EffectiveEngine
	}
	if result != nil && isConcreteSnapshotEngine(result.ActualEngine) {
		return result.ActualEngine
	}
	return model.EngineCopy
}

func cloneActualEngine(result *engine.CloneResult, fallback model.EngineType) model.EngineType {
	if result != nil && isConcreteSnapshotEngine(result.ActualEngine) {
		return result.ActualEngine
	}
	return concreteSnapshotEngineOrCopy(fallback)
}

func cloneEffectiveEngine(result *engine.CloneResult, record *transfer.Record, fallback model.EngineType) model.EngineType {
	if record != nil && isConcreteSnapshotEngine(record.EffectiveEngine) {
		return record.EffectiveEngine
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) {
		return result.EffectiveEngine
	}
	return concreteSnapshotEngineOrCopy(fallback)
}

func cloneDegradedReasons(result *engine.CloneResult, record *transfer.Record) []string {
	var reasons []string
	if record != nil {
		reasons = appendUniqueSnapshotStrings(reasons, record.DegradedReasons...)
	}
	if result != nil {
		reasons = appendUniqueSnapshotStrings(reasons, result.Degradations...)
	}
	if len(reasons) == 0 {
		return nil
	}
	return reasons
}

func cloneMetadataPreservation(result *engine.CloneResult, record *transfer.Record, fallback model.EngineType) *model.MetadataPreservation {
	if record != nil && isConcreteSnapshotEngine(record.EffectiveEngine) {
		metadata := engine.MetadataPreservationForEngine(record.EffectiveEngine)
		return &metadata
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) && result.MetadataPreservation != (model.MetadataPreservation{}) {
		metadata := result.MetadataPreservation
		return &metadata
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) {
		metadata := engine.MetadataPreservationForEngine(result.EffectiveEngine)
		return &metadata
	}
	metadata := engine.MetadataPreservationForEngine(concreteSnapshotEngineOrCopy(fallback))
	return &metadata
}

func clonePerformanceClass(result *engine.CloneResult, record *transfer.Record, fallback model.EngineType) string {
	if record != nil && isConcreteSnapshotEngine(record.EffectiveEngine) {
		return engine.PerformanceClassForEngine(record.EffectiveEngine)
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) && result.PerformanceClass != "" {
		return result.PerformanceClass
	}
	if result != nil && isConcreteSnapshotEngine(result.EffectiveEngine) {
		return engine.PerformanceClassForEngine(result.EffectiveEngine)
	}
	return engine.PerformanceClassForEngine(concreteSnapshotEngineOrCopy(fallback))
}

func (c *Creator) runtimeEngineType() model.EngineType {
	if c != nil && c.engine != nil {
		if name := c.engine.Name(); isConcreteSnapshotEngine(name) {
			return name
		}
	}
	if c != nil && isConcreteSnapshotEngine(c.engineType) {
		return c.engineType
	}
	return model.EngineCopy
}

func isConcreteSnapshotEngine(engineType model.EngineType) bool {
	return engineType != "" && engineType != engine.EngineAuto
}

func concreteSnapshotEngineOrCopy(engineType model.EngineType) model.EngineType {
	if isConcreteSnapshotEngine(engineType) {
		return engineType
	}
	return model.EngineCopy
}

func appendUniqueSnapshotStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (c *Creator) writeSnapshotReadyMarker(snapshotTmpDir string, snapshotID model.SnapshotID, payloadHash, checksum model.HashValue, engineType model.EngineType) error {
	readyMarker := &model.ReadyMarker{
		SnapshotID:         snapshotID,
		CompletedAt:        time.Now().UTC(),
		PayloadHash:        payloadHash,
		Engine:             concreteSnapshotEngineOrCopy(engineType),
		DescriptorChecksum: checksum,
	}
	readyPath := filepath.Join(snapshotTmpDir, ".READY")
	if err := c.writeReadyMarker(readyPath, readyMarker); err != nil {
		return fmt.Errorf("write ready marker: %w", err)
	}
	return nil
}

func (c *Creator) compressSnapshotStorage(snapshotTmpDir string) error {
	if c.compression == nil || !c.compression.IsEnabled() {
		return nil
	}
	count, err := c.compression.CompressDir(snapshotTmpDir)
	if err != nil {
		return fmt.Errorf("compress snapshot: %w", err)
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "compressed %d files\n", count)
	}
	return nil
}

func (c *Creator) publishStagedSnapshot(publishPaths snapshotPublishPaths, desc *model.Descriptor) error {
	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(c.repoRoot, desc.SnapshotID)
	if err != nil {
		cleanupErr := c.cleanupUncommittedPublishArtifacts(publishPaths.tmpDir, "")
		return withCleanupError(fmt.Errorf("validate descriptor path: %w", err), cleanupErr)
	}
	publishPaths.descriptorPath = descriptorPath

	if err := c.descriptorWriter(publishPaths.descriptorPath, desc); err != nil {
		cleanupErr := c.cleanupUncommittedPublishArtifacts(publishPaths.tmpDir, publishPaths.descriptorPath)
		return withCleanupError(fmt.Errorf("write descriptor: %w", err), cleanupErr)
	}

	// Step 13: Atomic rename tmp -> final
	if err := c.snapshotRenamer(publishPaths.tmpDir, publishPaths.dir); err != nil {
		if fsutil.IsCommitUncertain(err) {
			return fmt.Errorf("atomic rename snapshot commit uncertain after publishing snapshot %s; retained descriptor, payload, and intent for recovery: %w", desc.SnapshotID, err)
		}
		cleanupErr := c.cleanupFailedSnapshotRename(publishPaths, desc)
		return withCleanupError(fmt.Errorf("atomic rename snapshot: %w", err), cleanupErr)
	}
	return nil
}

func (c *Creator) updateLatestAfterPublish(
	wtMgr *worktree.Manager,
	worktreeName string,
	desc *model.Descriptor,
	publishPaths snapshotPublishPaths,
) error {
	snapshotID := desc.SnapshotID
	if err := c.latestUpdater(wtMgr, worktreeName, snapshotID); err != nil {
		if fsutil.IsCommitUncertain(err) {
			return fmt.Errorf("update head commit uncertain after publishing snapshot %s; retained descriptor and payload for recovery: %w", snapshotID, err)
		}
		cleanupErr := c.cleanupOwnedPublishedSnapshot(publishPaths, desc)
		return withCleanupError(fmt.Errorf("update head: %w", err), cleanupErr)
	}
	return nil
}

func (c *Creator) ensureAuditAppendableBeforeHistoryUpdate(publishPaths snapshotPublishPaths, desc *model.Descriptor) error {
	if err := c.auditLogger.EnsureAppendable(); err != nil {
		cleanupErr := c.cleanupOwnedPublishedSnapshot(publishPaths, desc)
		return withCleanupError(fmt.Errorf("audit log not appendable before history update: %w", err), cleanupErr)
	}
	return nil
}

func removeSnapshotIntent(intentPath string) {
	if err := os.Remove(intentPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: failed to remove snapshot intent %s: %v\n", intentPath, err)
	}
}

func cleanupIntentAfterDefiniteFailure(intentPath string, err error) error {
	if err != nil && !fsutil.IsCommitUncertain(err) {
		removeSnapshotIntent(intentPath)
	}
	return err
}

func (c *Creator) appendCreateAudit(worktreeName string, snapshotID model.SnapshotID, note string, desc *model.Descriptor, transferRecord *transfer.Record, partialPaths []string) error {
	checksum := model.HashValue("")
	engineType := model.EngineCopy
	if desc != nil {
		checksum = desc.DescriptorChecksum
		engineType = concreteSnapshotEngineOrCopy(desc.Engine)
	}
	auditData := map[string]any{
		"engine":   string(engineType),
		"note":     note,
		"checksum": string(checksum),
	}
	if transferRecord != nil {
		auditData["transfers"] = []transfer.Record{*transferRecord}
	}
	if len(partialPaths) > 0 {
		auditData["partial_paths"] = partialPaths
	}
	if err := c.auditLogger.Append(model.EventTypeSnapshotCreate, worktreeName, snapshotID, auditData); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// validateAndNormalizePaths validates and normalizes the partial snapshot paths.
func (c *Creator) validateAndNormalizePaths(paths []string, worktreeName string) ([]string, error) {
	boundary, err := repo.WorktreeManagedPayloadBoundary(c.repoRoot, worktreeName)
	if err != nil {
		return nil, fmt.Errorf("worktree payload path: %w", err)
	}
	wtPath := boundary.Root

	var normalized []string
	for _, p := range paths {
		clean, err := pathutil.CleanRel(p)
		if err != nil {
			return nil, err
		}
		p = clean
		if boundary.ExcludesRelativePath(p) {
			return nil, fmt.Errorf("path is repo control data and is not managed: %s", p)
		}

		if err := pathutil.ValidateNoSymlinkParents(wtPath, p); err != nil {
			return nil, fmt.Errorf("path escapes worktree through parent: %s: %w", p, err)
		}

		// Build full path and verify it exists within worktree
		fullPath := filepath.Join(wtPath, p)
		if _, err := os.Lstat(fullPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", p)
		} else if err != nil {
			return nil, fmt.Errorf("stat path %s: %w", p, err)
		}

		normalized = append(normalized, p)
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, p := range normalized {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	return collapseAncestorCoveredPaths(unique), nil
}

func collapseAncestorCoveredPaths(paths []string) []string {
	sort.Strings(paths)

	collapsed := make([]string, 0, len(paths))
	for _, p := range paths {
		covered := false
		for _, existing := range collapsed {
			if partialPathCovers(existing, p) {
				covered = true
				break
			}
		}
		if !covered {
			collapsed = append(collapsed, p)
		}
	}
	return collapsed
}

func partialPathCovers(ancestor, path string) bool {
	rel, err := filepath.Rel(ancestor, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// clonePaths clones only the specified paths from source to destination.
func (c *Creator) clonePaths(src, dst string, paths []string) (*engine.CloneResult, error) {
	combined := engine.NewCloneResult(c.runtimeEngineType())
	for _, p := range paths {
		srcPath := filepath.Join(src, p)
		dstPath := filepath.Join(dst, p)

		// Get source info
		info, err := os.Lstat(srcPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if info.IsDir() {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return nil, fmt.Errorf("create parent dir for %s: %w", p, err)
			}
			// Clone directory tree
			result, err := c.cloneToNew(c.engine, srcPath, dstPath)
			if err != nil {
				return nil, fmt.Errorf("clone directory %s: %w", p, err)
			}
			mergeCloneResult(combined, result)
		} else {
			// Clone single file - ensure parent dir exists
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return nil, fmt.Errorf("create parent dir for %s: %w", p, err)
			}
			result, err := c.cloneToNew(c.engine, srcPath, dstPath)
			if err != nil {
				return nil, fmt.Errorf("clone file %s: %w", p, err)
			}
			mergeCloneResult(combined, result)
		}
	}
	return combined, nil
}

func mergeCloneResult(combined, result *engine.CloneResult) {
	if combined == nil || result == nil {
		return
	}
	if result.Degraded {
		if isConcreteSnapshotEngine(result.ActualEngine) {
			combined.ActualEngine = result.ActualEngine
		}
		if len(result.Degradations) == 0 {
			combined.AddDegradation("", result.EffectiveEngine)
			return
		}
		for _, reason := range result.Degradations {
			combined.AddDegradation(reason, result.EffectiveEngine)
		}
		return
	}
	if combined.Degraded {
		return
	}
	if isConcreteSnapshotEngine(result.ActualEngine) {
		combined.ActualEngine = result.ActualEngine
	}
	if isConcreteSnapshotEngine(result.EffectiveEngine) {
		combined.EffectiveEngine = result.EffectiveEngine
		combined.MetadataPreservation = result.MetadataPreservation
		combined.PerformanceClass = result.PerformanceClass
	}
}

func (c *Creator) writeIntent(path string, intent *model.IntentRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func (c *Creator) writeReadyMarker(path string, marker *model.ReadyMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func writeDescriptorFile(path string, desc *model.Descriptor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(desc, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func (c *Creator) cleanupUncommittedPublishArtifacts(snapshotTmpDir, descriptorPath string) error {
	var errs []error

	if descriptorPath != "" {
		if err := removeFileIfExists(descriptorPath); err != nil {
			errs = append(errs, err)
		}
	}
	if snapshotTmpDir != "" {
		if err := removeDirIfExists(snapshotTmpDir); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *Creator) cleanupFailedSnapshotRename(publishPaths snapshotPublishPaths, desc *model.Descriptor) error {
	var errs []error

	if publishPaths.descriptorPath != "" {
		if err := removeFileIfExists(publishPaths.descriptorPath); err != nil {
			errs = append(errs, err)
		}
	}

	tmpExists, err := pathExists(publishPaths.tmpDir)
	if err != nil {
		errs = append(errs, fmt.Errorf("stat snapshot tmp dir: %w", err))
		return errors.Join(errs...)
	}
	if tmpExists {
		if err := removeDirIfExists(publishPaths.tmpDir); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}

	if err := removeOwnedSnapshotDir(publishPaths.dir, desc); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *Creator) cleanupOwnedPublishedSnapshot(publishPaths snapshotPublishPaths, desc *model.Descriptor) error {
	var errs []error

	if err := removeOwnedSnapshotDir(publishPaths.dir, desc); err != nil {
		errs = append(errs, err)
	}
	if publishPaths.descriptorPath != "" {
		if err := removeFileIfExists(publishPaths.descriptorPath); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func removeOwnedSnapshotDir(snapshotDir string, desc *model.Descriptor) error {
	owned, err := snapshotDirOwnedByDescriptor(snapshotDir, desc)
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	if err := removeReadyMarkers(snapshotDir); err != nil {
		return err
	}
	return removeDirIfExists(snapshotDir)
}

func snapshotDirOwnedByDescriptor(snapshotDir string, desc *model.Descriptor) (bool, error) {
	if snapshotDir == "" || desc == nil {
		return false, nil
	}
	info, err := os.Lstat(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat snapshot dir %s: %w", snapshotDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}

	readyPath := filepath.Join(snapshotDir, ".READY")
	readyInfo, err := os.Lstat(readyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat ready marker %s: %w", readyPath, err)
	}
	if readyInfo.Mode()&os.ModeSymlink != 0 || !readyInfo.Mode().IsRegular() {
		return false, nil
	}

	data, err := os.ReadFile(readyPath)
	if err != nil {
		return false, fmt.Errorf("read ready marker %s: %w", readyPath, err)
	}
	var marker model.ReadyMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return false, nil
	}
	return marker.SnapshotID == desc.SnapshotID &&
		marker.PayloadHash == desc.PayloadRootHash &&
		marker.Engine == desc.Engine &&
		marker.DescriptorChecksum == desc.DescriptorChecksum, nil
}

func removeReadyMarkers(snapshotDir string) error {
	info, err := os.Lstat(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat snapshot dir before removing ready marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to remove ready marker through symlink: %s", snapshotDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("snapshot path is not directory: %s", snapshotDir)
	}

	var errs []error
	removed := false
	for _, name := range []string{".READY", ".READY.gz"} {
		path := filepath.Join(snapshotDir, name)
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("remove ready marker %s: %w", path, err))
			}
			continue
		}
		removed = true
	}
	if removed {
		if err := fsutil.FsyncDir(snapshotDir); err != nil {
			errs = append(errs, fmt.Errorf("fsync snapshot dir after unpublish: %w", err))
		}
	}
	return errors.Join(errs...)
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func removeFileIfExists(path string) error {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := fsutil.FsyncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("fsync parent after remove %s: %w", path, err)
	}
	return nil
}

func removeDirIfExists(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := fsutil.FsyncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("fsync parent after remove %s: %w", path, err)
	}
	return nil
}

func withCleanupError(err, cleanupErr error) error {
	if cleanupErr == nil {
		return err
	}
	return fmt.Errorf("%w; additionally failed cleanup: %v", err, cleanupErr)
}

// LoadDescriptor loads a descriptor from disk.
func LoadDescriptor(repoRoot string, snapshotID model.SnapshotID) (*model.Descriptor, error) {
	if err := snapshotID.Validate(); err != nil {
		return nil, err
	}
	path, err := repo.SnapshotDescriptorPathForRead(repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, descriptorNotFoundError(snapshotID)
		}
		return nil, errclass.ErrDescriptorCorrupt.WithMessagef("descriptor path invalid: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, descriptorNotFoundError(snapshotID)
		}
		return nil, errclass.ErrDescriptorCorrupt.WithMessagef("read descriptor: %v", err)
	}
	var desc model.Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, errclass.ErrDescriptorCorrupt.WithMessagef("parse descriptor: %v", err)
	}
	if desc.SnapshotID != snapshotID {
		return nil, errclass.ErrDescriptorCorrupt.WithMessagef("descriptor snapshot ID %q does not match requested %q", desc.SnapshotID, snapshotID)
	}
	return &desc, nil
}

func descriptorNotFoundError(snapshotID model.SnapshotID) error {
	return fmt.Errorf("%w: %s: %w", ErrDescriptorNotFound, snapshotID, os.ErrNotExist)
}

// VerifySnapshot verifies a snapshot's integrity.
func VerifySnapshot(repoRoot string, snapshotID model.SnapshotID, verifyPayloadHash bool) error {
	if verifyPayloadHash {
		_, issue := InspectPublishState(repoRoot, snapshotID, PublishStateOptions{
			RequireReady:             true,
			RequirePayload:           true,
			VerifyDescriptorChecksum: true,
			VerifyPayloadHash:        true,
		})
		if issue != nil {
			return &errclass.JVSError{Code: issue.Code, Message: issue.Message}
		}
		return nil
	}

	desc, err := LoadDescriptor(repoRoot, snapshotID)
	if err != nil {
		return err
	}

	// Verify checksum
	computedChecksum, err := integrity.ComputeDescriptorChecksum(desc)
	if err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}
	if computedChecksum != desc.DescriptorChecksum {
		return errclass.ErrDescriptorCorrupt.WithMessage("checksum mismatch")
	}

	return nil
}
