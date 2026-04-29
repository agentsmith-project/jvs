// Package gc provides garbage collection for snapshots.
package gc

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
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

// Collector handles garbage collection of unused snapshots.
type Collector struct {
	repoRoot         string
	auditLogger      *audit.FileAppender
	progressCallback func(string, int, int, string)
}

var collectorFsyncDir = fsutil.FsyncDir

const (
	cleanupActiveOperationsUnavailableMessage = "cleanup cannot determine active operations safely; run jvs doctor --strict before cleanup"
	cleanupSavePointStorageUnavailableMessage = "cleanup cannot verify save point storage safely; run jvs doctor --strict before cleanup"
)

type planInputs struct {
	protectedSet         []model.SnapshotID
	protectionGroups     []model.GCProtectionGroup
	protectedByLineage   int
	protectedByRetention int
	toDelete             []model.SnapshotID
}

type protectionBuilder struct {
	protected map[model.SnapshotID]bool
	groups    map[string]map[model.SnapshotID]bool
}

type cleanupPublicError struct {
	public *errclass.JVSError
	cause  error
}

func newCleanupPublicError(message string, cause error) error {
	return &cleanupPublicError{
		public: errclass.ErrGCPlanMismatch.WithMessage(message),
		cause:  cause,
	}
}

func (e *cleanupPublicError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.public.Error()
}

func (e *cleanupPublicError) Is(target error) bool {
	if errors.Is(e.public, target) {
		return true
	}
	var targetJVS *errclass.JVSError
	if !errors.As(target, &targetJVS) {
		return false
	}
	var causeJVS *errclass.JVSError
	return errors.As(e.cause, &causeJVS) && causeJVS.Code == targetJVS.Code
}

func (e *cleanupPublicError) As(target any) bool {
	if targetJVS, ok := target.(**errclass.JVSError); ok {
		*targetJVS = e.public
		return true
	}
	return false
}

func newProtectionBuilder() *protectionBuilder {
	return &protectionBuilder{
		protected: make(map[model.SnapshotID]bool),
		groups:    make(map[string]map[model.SnapshotID]bool),
	}
}

func (b *protectionBuilder) add(reason string, id model.SnapshotID) bool {
	if id == "" {
		return false
	}
	wasProtected := b.protected[id]
	b.protected[id] = true
	if reason != "" {
		if b.groups[reason] == nil {
			b.groups[reason] = make(map[model.SnapshotID]bool)
		}
		b.groups[reason][id] = true
	}
	return !wasProtected
}

func (b *protectionBuilder) protectedSet() []model.SnapshotID {
	ids := make([]model.SnapshotID, 0, len(b.protected))
	for id := range b.protected {
		ids = append(ids, id)
	}
	sortSnapshotIDs(ids)
	return ids
}

func (b *protectionBuilder) protectionGroups() []model.GCProtectionGroup {
	reasons := make([]string, 0, len(b.groups))
	known := []string{
		model.GCProtectionReasonHistory,
		model.GCProtectionReasonOpenView,
		model.GCProtectionReasonActiveRecovery,
		model.GCProtectionReasonActiveOperation,
	}
	seen := make(map[string]bool, len(known))
	for _, reason := range known {
		seen[reason] = true
		if len(b.groups[reason]) > 0 {
			reasons = append(reasons, reason)
		}
	}
	var extra []string
	for reason, ids := range b.groups {
		if seen[reason] || len(ids) == 0 {
			continue
		}
		extra = append(extra, reason)
	}
	sort.Strings(extra)
	reasons = append(reasons, extra...)

	groups := make([]model.GCProtectionGroup, 0, len(reasons))
	for _, reason := range reasons {
		ids := make([]model.SnapshotID, 0, len(b.groups[reason]))
		for id := range b.groups[reason] {
			ids = append(ids, id)
		}
		sortSnapshotIDs(ids)
		groups = append(groups, model.GCProtectionGroup{
			Reason:     reason,
			Count:      len(ids),
			SavePoints: ids,
		})
	}
	return groups
}

// NewCollector creates a new GC collector.
func NewCollector(repoRoot string) *Collector {
	auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
	return &Collector{
		repoRoot:    repoRoot,
		auditLogger: audit.NewFileAppender(auditPath),
	}
}

// SetProgressCallback sets a callback for progress updates.
func (c *Collector) SetProgressCallback(cb func(string, int, int, string)) {
	c.progressCallback = cb
}

// Plan creates a GC plan.
func (c *Collector) Plan() (*model.GCPlan, error) {
	return c.PlanWithPolicy(model.DefaultRetentionPolicy())
}

// PlanWithPolicy creates a GC plan using the given retention policy.
func (c *Collector) PlanWithPolicy(policy model.RetentionPolicy) (*model.GCPlan, error) {
	var plan *model.GCPlan
	err := repo.WithMutationLock(c.repoRoot, "gc plan", func() error {
		var err error
		plan, err = c.planWithPolicy(policy)
		return err
	})
	return plan, err
}

func (c *Collector) planWithPolicy(policy model.RetentionPolicy) (*model.GCPlan, error) {
	if err := c.ensurePublishStateConsistent(); err != nil {
		return nil, err
	}

	inputs, err := c.computePlanInputs(policy)
	if err != nil {
		return nil, err
	}

	repoID, err := c.currentRepoID()
	if err != nil {
		return nil, err
	}
	deletableBytes, err := c.estimateDeletableBytes(inputs.toDelete)
	if err != nil {
		return nil, fmt.Errorf("estimate deletable bytes: %w", err)
	}
	if err := c.ensureAuditAppendable(); err != nil {
		return nil, fmt.Errorf("audit log not appendable: %w", err)
	}

	plan := &model.GCPlan{
		SchemaVersion:          model.GCPlanSchemaVersion,
		RepoID:                 repoID,
		PlanID:                 uuidutil.NewV4(),
		CreatedAt:              time.Now().UTC(),
		ProtectedSet:           inputs.protectedSet,
		ProtectionGroups:       inputs.protectionGroups,
		ProtectedByLineage:     inputs.protectedByLineage,
		ProtectedByRetention:   inputs.protectedByRetention,
		CandidateCount:         len(inputs.toDelete),
		ToDelete:               inputs.toDelete,
		DeletableBytesEstimate: deletableBytes,
		RetentionPolicy:        policy,
	}

	if err := c.writePlan(plan); err != nil {
		return nil, fmt.Errorf("write plan: %w", err)
	}
	if err := c.auditLogger.Append(model.EventTypeGCPlan, "", "", map[string]any{
		"plan_id":         plan.PlanID,
		"candidate_count": plan.CandidateCount,
	}); err != nil {
		c.deletePlan(plan.PlanID)
		return nil, fmt.Errorf("write audit log: %w", err)
	}

	return plan, nil
}

// Run executes a GC plan.
func (c *Collector) Run(planID string) error {
	return repo.WithMutationLock(c.repoRoot, "gc run", func() error {
		return c.run(planID)
	})
}

func (c *Collector) run(planID string) error {
	if planID == "" {
		return fmt.Errorf("plan ID is required")
	}

	plan, err := c.LoadPlan(planID)
	if err != nil {
		return fmt.Errorf("load plan: %w", err)
	}
	if err := c.ensurePublishStateConsistent(); err != nil {
		return err
	}

	pending, err := c.revalidatePlan(plan)
	if err != nil {
		return err
	}
	if err := c.ensureAuditAppendable(); err != nil {
		return fmt.Errorf("audit log not appendable: %w", err)
	}
	if err := c.markPlan(pending); err != nil {
		return err
	}

	totalToDelete := len(pending)

	// Delete snapshots
	deleted := 0
	for i, snapshotID := range pending {
		tombstone, err := c.loadTombstone(snapshotID)
		if err != nil {
			return fmt.Errorf("load tombstone %s: %w", snapshotID, err)
		}
		if tombstone != nil && tombstone.GCState == model.GCStateCommitted {
			residue, err := c.deletionResidue(snapshotID)
			if err != nil {
				return fmt.Errorf("check committed tombstone residue %s: %w", snapshotID, err)
			}
			if !residue.any() {
				continue
			}
		}

		// Report progress
		if c.progressCallback != nil {
			c.progressCallback("gc", i+1, totalToDelete, fmt.Sprintf("deleting %s", snapshotID.ShortID()))
		}

		if err := c.deleteSnapshot(snapshotID); err != nil {
			failed := &model.Tombstone{
				SnapshotID:  snapshotID,
				DeletedAt:   time.Now().UTC(),
				Reclaimable: false,
				GCState:     model.GCStateFailed,
				Reason:      err.Error(),
			}
			if writeErr := c.writeTombstone(failed); writeErr != nil {
				return fmt.Errorf("delete snapshot %s: %w; additionally failed to write failed tombstone: %v", snapshotID, err, writeErr)
			}
			return fmt.Errorf("delete snapshot %s: %w", snapshotID, err)
		}

		tombstone = &model.Tombstone{
			SnapshotID:  snapshotID,
			DeletedAt:   time.Now().UTC(),
			Reclaimable: true,
			GCState:     model.GCStateCommitted,
		}
		if err := c.writeTombstone(tombstone); err != nil {
			return fmt.Errorf("write committed tombstone %s: %w", snapshotID, err)
		}
		deleted++
	}

	// Report completion
	if c.progressCallback != nil && totalToDelete > 0 {
		c.progressCallback("gc", totalToDelete, totalToDelete, fmt.Sprintf("deleted %d snapshots", deleted))
	}

	// Audit
	if err := c.auditLogger.Append(model.EventTypeGCRun, "", "", map[string]any{
		"plan_id":       planID,
		"deleted_count": deleted,
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	// Cleanup plan after the committed run has been audited.
	c.deletePlan(planID)

	return nil
}

func (c *Collector) ensurePublishStateConsistent() error {
	ids, err := c.publishStateInventoryIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		_, issue := snapshot.InspectPublishState(c.repoRoot, id, snapshot.PublishStateOptions{
			RequireReady:             true,
			RequirePayload:           true,
			VerifyDescriptorChecksum: true,
			VerifyPayloadHash:        true,
		})
		if issue != nil {
			cause := &errclass.JVSError{
				Code:    issue.Code,
				Message: fmt.Sprintf("checkpoint %s publish state invalid: %s", id, issue.Message),
			}
			return newCleanupPublicError(cleanupSavePointStorageUnavailableMessage, cause)
		}
	}
	return nil
}

func (c *Collector) publishStateInventoryIDs() ([]model.SnapshotID, error) {
	seen := make(map[model.SnapshotID]bool)
	if err := c.collectPublishStateDescriptorIDs(seen); err != nil {
		return nil, err
	}
	if err := c.collectPublishStatePayloadIDs(seen); err != nil {
		return nil, err
	}

	ids := make([]model.SnapshotID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sortSnapshotIDs(ids)
	return ids, nil
}

func (c *Collector) collectPublishStateDescriptorIDs(seen map[model.SnapshotID]bool) error {
	descriptorsDir, err := repo.DescriptorsDirPath(c.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve descriptors directory: %w", err)
	}
	entries, err := os.ReadDir(descriptorsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read descriptors directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := model.SnapshotID(strings.TrimSuffix(entry.Name(), ".json"))
		if id.IsValid() {
			seen[id] = true
		}
	}
	return nil
}

func (c *Collector) collectPublishStatePayloadIDs(seen map[model.SnapshotID]bool) error {
	snapshotsDir, err := repo.SnapshotsDirPath(c.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve snapshots directory: %w", err)
	}
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read snapshots directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		id := model.SnapshotID(entry.Name())
		if id.IsValid() {
			seen[id] = true
		}
	}
	return nil
}

func (c *Collector) ensureAuditAppendable() error {
	issues, err := audit.VerifyFile(filepath.Join(c.repoRoot, ".jvs", "audit", "audit.jsonl"))
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		issue := issues[0]
		return fmt.Errorf("%s: %s", issue.ErrorCode, issue.Message)
	}
	return c.auditLogger.EnsureAppendable()
}

func (c *Collector) computePlanInputs(policy model.RetentionPolicy) (*planInputs, error) {
	return c.computePlanInputsAllowingReadyMissingDescriptors(policy, nil)
}

func (c *Collector) computePlanInputsAllowingReadyMissingDescriptors(policy model.RetentionPolicy, allowReadyMissingDescriptor map[model.SnapshotID]bool) (*planInputs, error) {
	protectedSet, protectionGroups, protectedByLineage, err := c.computeProtectedSet()
	if err != nil {
		return nil, fmt.Errorf("compute protected set: %w", err)
	}

	// Find all snapshots with descriptors for retention analysis
	allSnapshots, err := c.listAllSnapshots(allowReadyMissingDescriptor)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	protectedMap := make(map[model.SnapshotID]bool)
	for _, id := range protectedSet {
		protectedMap[id] = true
	}

	// Apply retention policy: protect by age
	protectedByRetention := 0
	now := time.Now()
	if policy.KeepMinAge > 0 {
		for _, id := range allSnapshots {
			if protectedMap[id] {
				continue
			}
			desc, err := snapshot.LoadDescriptor(c.repoRoot, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: gc: skipping descriptor %s: %v\n", id, err)
				continue
			}
			if now.Sub(desc.CreatedAt) < policy.KeepMinAge {
				protectedMap[id] = true
				protectedByRetention++
			}
		}
	}

	// Apply retention policy: protect by count (keep most recent N)
	if policy.KeepMinSnapshots > 0 {
		allDescs, err := snapshot.ListAll(c.repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: gc: failed to list all descriptors for retention-by-count: %v\n", err)
		}
		if err == nil {
			kept := 0
			for _, desc := range allDescs {
				if kept >= policy.KeepMinSnapshots {
					break
				}
				if !protectedMap[desc.SnapshotID] {
					protectedMap[desc.SnapshotID] = true
					protectedByRetention++
				}
				kept++
			}
		}
	}

	// Rebuild protected set from map
	protectedSet = protectedSet[:0]
	for id := range protectedMap {
		protectedSet = append(protectedSet, id)
	}
	sortSnapshotIDs(protectedSet)

	var toDelete []model.SnapshotID
	for _, id := range allSnapshots {
		if !protectedMap[id] {
			toDelete = append(toDelete, id)
		}
	}
	sortSnapshotIDs(toDelete)

	return &planInputs{
		protectedSet:         protectedSet,
		protectionGroups:     protectionGroups,
		protectedByLineage:   protectedByLineage,
		protectedByRetention: protectedByRetention,
		toDelete:             toDelete,
	}, nil
}

func (c *Collector) revalidatePlan(plan *model.GCPlan) ([]model.SnapshotID, error) {
	retryEvidence, err := c.collectRetryTombstoneEvidence(plan)
	if err != nil {
		return nil, err
	}

	current, err := c.computePlanInputsAllowingReadyMissingDescriptors(plan.RetentionPolicy, retryEvidence)
	if err != nil {
		return nil, fmt.Errorf("revalidate plan: %w", err)
	}

	pending := make([]model.SnapshotID, 0, len(plan.ToDelete))
	currentComparable := make([]model.SnapshotID, 0, len(plan.ToDelete))
	for _, id := range plan.ToDelete {
		if err := id.Validate(); err != nil {
			return nil, errclass.ErrGCPlanMismatch.WithMessagef("invalid planned snapshot ID %q: %v", string(id), err)
		}
		residue, err := c.deletionResidue(id)
		if err != nil {
			return nil, errclass.ErrGCPlanMismatch.WithMessagef("invalid planned snapshot ID %q: %v", string(id), err)
		}
		if residue.snapshotExists {
			currentComparable = append(currentComparable, id)
		}
		tombstone, err := c.loadTombstone(id)
		if err != nil {
			return nil, fmt.Errorf("load tombstone %s: %w", id, err)
		}
		if tombstone != nil && tombstone.GCState == model.GCStateCommitted {
			if residue.any() {
				pending = append(pending, id)
			}
			continue
		}
		if residue.any() {
			pending = append(pending, id)
			continue
		}
		if hasRetryableTombstoneEvidence(id, tombstone) {
			pending = append(pending, id)
			continue
		}
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("planned snapshot %s is missing without committed tombstone", id)
	}
	sortSnapshotIDs(pending)
	sortSnapshotIDs(currentComparable)

	currentCandidates := append([]model.SnapshotID(nil), current.toDelete...)
	sortSnapshotIDs(currentCandidates)
	if !sameSnapshotIDs(currentComparable, currentCandidates) {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("candidate set changed; run cleanup preview again: planned=%v current=%v", currentComparable, currentCandidates)
	}
	currentProtected := append([]model.SnapshotID(nil), current.protectedSet...)
	sortSnapshotIDs(currentProtected)
	plannedProtected := append([]model.SnapshotID(nil), plan.ProtectedSet...)
	sortSnapshotIDs(plannedProtected)
	if !sameSnapshotIDs(plannedProtected, currentProtected) {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("protected set changed; run cleanup preview again: planned=%v current=%v", plannedProtected, currentProtected)
	}
	return pending, nil
}

func (c *Collector) collectRetryTombstoneEvidence(plan *model.GCPlan) (map[model.SnapshotID]bool, error) {
	evidence := make(map[model.SnapshotID]bool)
	for _, id := range plan.ToDelete {
		if err := id.Validate(); err != nil {
			return nil, errclass.ErrGCPlanMismatch.WithMessagef("invalid planned snapshot ID %q: %v", string(id), err)
		}
		tombstone, err := c.loadTombstone(id)
		if err != nil {
			return nil, fmt.Errorf("load tombstone %s: %w", id, err)
		}
		if hasRetryableTombstoneEvidence(id, tombstone) {
			evidence[id] = true
		}
	}
	return evidence, nil
}

func hasRetryableTombstoneEvidence(snapshotID model.SnapshotID, tombstone *model.Tombstone) bool {
	if tombstone == nil || tombstone.SnapshotID != snapshotID {
		return false
	}
	return tombstone.GCState == model.GCStateMarked || tombstone.GCState == model.GCStateFailed
}

func (c *Collector) markPlan(snapshotIDs []model.SnapshotID) error {
	for _, snapshotID := range snapshotIDs {
		existing, err := c.loadTombstone(snapshotID)
		if err != nil {
			return fmt.Errorf("load tombstone %s: %w", snapshotID, err)
		}
		if existing != nil && existing.GCState == model.GCStateCommitted {
			residue, err := c.deletionResidue(snapshotID)
			if err != nil {
				return fmt.Errorf("check committed tombstone residue %s: %w", snapshotID, err)
			}
			if !residue.any() {
				continue
			}
		}

		tombstone := &model.Tombstone{
			SnapshotID:  snapshotID,
			DeletedAt:   time.Now().UTC(),
			Reclaimable: false,
			GCState:     model.GCStateMarked,
		}
		if err := c.writeTombstone(tombstone); err != nil {
			return fmt.Errorf("write marked tombstone %s: %w", snapshotID, err)
		}
	}
	return nil
}

func (c *Collector) computeProtectedSet() ([]model.SnapshotID, []model.GCProtectionGroup, int, error) {
	protection := newProtectionBuilder()
	lineageCount := 0

	// 1. All live worktree roots. Latest remains a live restore target when a
	// worktree is detached, and Base is conservatively treated as live.
	wtMgr := worktree.NewManager(c.repoRoot)
	wtList, err := wtMgr.List()
	if err != nil {
		return nil, nil, 0, err
	}
	roots := make(map[model.SnapshotID]bool)
	directProtected := make(map[model.SnapshotID]bool)
	addRoot := func(id model.SnapshotID) {
		if id != "" {
			roots[id] = true
		}
	}
	addDirectProtected := func(id model.SnapshotID) {
		if id != "" {
			directProtected[id] = true
		}
	}
	for _, cfg := range wtList {
		addRoot(cfg.HeadSnapshotID)
		addRoot(cfg.LatestSnapshotID)
		addRoot(cfg.BaseSnapshotID)
		addDirectProtected(cfg.StartedFromSnapshotID)
	}

	rootIDs := make([]model.SnapshotID, 0, len(roots))
	for id := range roots {
		if err := id.Validate(); err != nil {
			continue
		}
		protection.add(model.GCProtectionReasonHistory, id)
		rootIDs = append(rootIDs, id)
	}
	for id := range directProtected {
		if err := id.Validate(); err != nil {
			continue
		}
		protection.add(model.GCProtectionReasonHistory, id)
	}
	sortSnapshotIDs(rootIDs)

	// 2. Lineage traversal (keep parent chains)
	for _, id := range rootIDs {
		lineageCount += c.walkLineage(id, protection)
	}

	// 3. Active operation intents protect only published save points. Intent-only
	// crash residue stays out of public save-point groups; publish-state
	// inventory above still fails closed for descriptor/payload residue.
	if err := c.addIntentProtections(protection); err != nil {
		return nil, nil, 0, err
	}

	// 4. Documented active source pins. These are fail-closed: any unreadable
	// or malformed pin prevents cleanup from planning deletion.
	pins, err := sourcepin.NewManager(c.repoRoot).List()
	if err != nil {
		return nil, nil, 0, err
	}
	for _, pin := range pins {
		protection.add(protectionReasonForSourcePin(pin), pin.SnapshotID)
	}

	// 5. Active restore recovery plans are visible recovery state and protect
	// their source save point even if no supplemental pin exists.
	recoveryPlans, err := recovery.NewManager(c.repoRoot).List()
	if err != nil {
		return nil, nil, 0, err
	}
	for _, plan := range recoveryPlans {
		if plan.Status != recovery.StatusActive {
			continue
		}
		protection.add(model.GCProtectionReasonActiveRecovery, plan.SourceSavePoint)
	}

	return protection.protectedSet(), protection.protectionGroups(), lineageCount, nil
}

func (c *Collector) addIntentProtections(protection *protectionBuilder) error {
	intentsDir, err := repo.IntentsDirPath(c.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return newCleanupPublicError(cleanupActiveOperationsUnavailableMessage, fmt.Errorf("read intents directory: %w", err))
	}

	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		return newCleanupPublicError(cleanupActiveOperationsUnavailableMessage, fmt.Errorf("read intents directory: %w", err))
	}
	for _, entry := range entries {
		id, ok := snapshotIDFromIntentEntryName(entry.Name())
		if !ok {
			continue
		}
		published, err := c.intentSavePointPublished(id)
		if err != nil {
			return err
		}
		if published {
			protection.add(model.GCProtectionReasonActiveOperation, id)
		}
	}
	return nil
}

func snapshotIDFromIntentEntryName(name string) (model.SnapshotID, bool) {
	if !strings.HasSuffix(name, ".json") {
		return "", false
	}
	id := model.SnapshotID(strings.TrimSuffix(name, ".json"))
	if err := id.Validate(); err != nil {
		return "", false
	}
	return id, true
}

func (c *Collector) intentSavePointPublished(snapshotID model.SnapshotID) (bool, error) {
	_, issue := snapshot.InspectPublishState(c.repoRoot, snapshotID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue == nil {
		return true, nil
	}
	if issue.Code == snapshot.PublishStateCodeSnapshotIDInvalid ||
		issue.Code == snapshot.PublishStateCodeDescriptorMissing {
		return false, nil
	}
	cause := &errclass.JVSError{
		Code:    issue.Code,
		Message: fmt.Sprintf("checkpoint %s publish state invalid: %s", snapshotID, issue.Message),
	}
	return false, newCleanupPublicError(cleanupActiveOperationsUnavailableMessage, cause)
}

func protectionReasonForSourcePin(pin model.Pin) string {
	reason := strings.ToLower(strings.TrimSpace(pin.Reason))
	switch {
	case reason == "active read-only view":
		return model.GCProtectionReasonOpenView
	case strings.HasPrefix(reason, "active recovery plan"):
		return model.GCProtectionReasonActiveRecovery
	default:
		return model.GCProtectionReasonActiveOperation
	}
}

func (c *Collector) walkLineage(snapshotID model.SnapshotID, protection *protectionBuilder) int {
	return c.walkLineageSeen(snapshotID, protection, make(map[model.SnapshotID]bool))
}

func (c *Collector) walkLineageSeen(snapshotID model.SnapshotID, protection *protectionBuilder, seen map[model.SnapshotID]bool) int {
	count := 0
	if err := snapshotID.Validate(); err != nil {
		return count
	}
	if seen[snapshotID] {
		return count
	}
	seen[snapshotID] = true
	desc, err := snapshot.LoadDescriptor(c.repoRoot, snapshotID)
	if err != nil {
		return count
	}
	if desc.ParentID != nil {
		if protection.add(model.GCProtectionReasonHistory, *desc.ParentID) {
			count++
		}
		count += c.walkLineageSeen(*desc.ParentID, protection, seen)
	}
	if desc.StartedFrom != nil {
		if protection.add(model.GCProtectionReasonHistory, *desc.StartedFrom) {
			count++
		}
	}
	return count
}

func (c *Collector) listAllSnapshots(allowReadyMissingDescriptor map[model.SnapshotID]bool) ([]model.SnapshotID, error) {
	snapshotsDir, err := repo.SnapshotsDirPath(c.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []model.SnapshotID
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			id := model.SnapshotID(entry.Name())
			if err := id.Validate(); err == nil {
				return nil, fmt.Errorf("snapshot leaf is symlink: %s", entry.Name())
			}
			continue
		}
		if entry.IsDir() {
			id := model.SnapshotID(entry.Name())
			if err := id.Validate(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: gc: skipping invalid snapshot directory %q: %v\n", entry.Name(), err)
				continue
			}
			snapshotDir, err := repo.SnapshotPathForRead(c.repoRoot, id)
			if err != nil {
				return nil, err
			}
			if snapshotReadyMarkerExists(snapshotDir) {
				_, err := repo.SnapshotDescriptorPathForRead(c.repoRoot, id)
				if errors.Is(err, os.ErrNotExist) {
					if allowReadyMissingDescriptor[id] {
						ids = append(ids, id)
						continue
					}
					return nil, fmt.Errorf("READY snapshot missing descriptor: %s", id)
				}
				if err != nil {
					return nil, fmt.Errorf("validate descriptor for READY snapshot %s: %w", id, err)
				}
			}
			ids = append(ids, id)
		}
	}
	sortSnapshotIDs(ids)
	return ids, nil
}

func snapshotReadyMarkerExists(snapshotDir string) bool {
	return regularFileExists(filepath.Join(snapshotDir, ".READY")) ||
		regularFileExists(filepath.Join(snapshotDir, ".READY.gz"))
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func (c *Collector) currentRepoID() (string, error) {
	r, err := repo.Discover(c.repoRoot)
	if err != nil {
		return "", errclass.ErrGCPlanMismatch.WithMessage("cannot read current repository identity")
	}
	if r.RepoID == "" {
		return "", errclass.ErrGCPlanMismatch.WithMessage("current repository identity is missing")
	}
	return r.RepoID, nil
}

func (c *Collector) estimateDeletableBytes(snapshotIDs []model.SnapshotID) (int64, error) {
	var total int64
	for _, snapshotID := range snapshotIDs {
		snapshotDir, err := repo.SnapshotPathForRead(c.repoRoot, snapshotID)
		if err == nil {
			size, err := regularTreeSize(snapshotDir)
			if err != nil {
				return 0, fmt.Errorf("snapshot %s: %w", snapshotID, err)
			}
			total += size
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}

		descriptorPath, err := repo.SnapshotDescriptorPathForRead(c.repoRoot, snapshotID)
		if err == nil {
			info, err := os.Lstat(descriptorPath)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return 0, err
				}
			} else if info.Mode().IsRegular() {
				total += info.Size()
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}
	return total, nil
}

func regularTreeSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

type deletionResidue struct {
	snapshotExists   bool
	descriptorExists bool
}

func (r deletionResidue) any() bool {
	return r.snapshotExists || r.descriptorExists
}

func (c *Collector) deletionResidue(snapshotID model.SnapshotID) (deletionResidue, error) {
	snapshotExists, err := c.snapshotDirExists(snapshotID)
	if err != nil {
		return deletionResidue{}, err
	}
	descriptorExists, err := c.descriptorFileExists(snapshotID)
	if err != nil {
		return deletionResidue{}, err
	}
	return deletionResidue{
		snapshotExists:   snapshotExists,
		descriptorExists: descriptorExists,
	}, nil
}

func (c *Collector) deleteSnapshot(snapshotID model.SnapshotID) error {
	snapshotDir, err := repo.SnapshotPathForDelete(c.repoRoot, snapshotID)
	if err != nil {
		return err
	}
	descriptorPath, err := repo.SnapshotDescriptorPathForDelete(c.repoRoot, snapshotID)
	if err != nil {
		return err
	}
	residue, err := c.deletionResidue(snapshotID)
	if err != nil {
		return err
	}

	// Keep the descriptor inspectable until payload removal succeeds. If the
	// descriptor removal fails after payload deletion, the failed tombstone lets
	// a retry finish the metadata cleanup idempotently.
	if residue.snapshotExists {
		if err := os.RemoveAll(snapshotDir); err != nil {
			return fmt.Errorf("remove snapshot dir: %w", err)
		}
		if err := collectorFsyncDir(filepath.Dir(snapshotDir)); err != nil {
			return fmt.Errorf("fsync snapshot parent after remove: %w", err)
		}
	}
	if residue.descriptorExists {
		if err := os.Remove(descriptorPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove descriptor: %w", err)
		}
		if err := collectorFsyncDir(filepath.Dir(descriptorPath)); err != nil {
			return fmt.Errorf("fsync descriptor parent after remove: %w", err)
		}
	}

	return nil
}

func (c *Collector) writePlan(plan *model.GCPlan) error {
	path, err := repo.GCPlanPathForWrite(c.repoRoot, plan.PlanID)
	if err != nil {
		return err
	}
	gcDir := filepath.Dir(path)
	if err := os.MkdirAll(gcDir, 0755); err != nil {
		return fmt.Errorf("create gc dir: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

// RemoveRuntimePlans removes top-level cleanup preview/run plan state. Durable
// cleanup evidence under child directories is left intact.
func RemoveRuntimePlans(repoRoot string) (int, error) {
	gcDir, err := repo.GCDirPath(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	entries, err := os.ReadDir(gcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cleaned := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		planID := strings.TrimSuffix(name, ".json")
		path, err := repo.GCPlanPathForDelete(repoRoot, planID)
		if err != nil {
			return cleaned, err
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cleaned, err
		}
		cleaned++
	}
	if cleaned > 0 {
		if err := collectorFsyncDir(gcDir); err != nil {
			return cleaned, err
		}
	}
	return cleaned, nil
}

// LoadPlan loads a GC plan by ID.
func (c *Collector) LoadPlan(planID string) (*model.GCPlan, error) {
	path, err := repo.GCPlanPathForRead(c.repoRoot, planID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q not found", planID)
		}
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q not found", planID)
		}
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q is not readable", planID)
	}
	var plan model.GCPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != model.GCPlanSchemaVersion {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q plan_id does not match request", planID)
	}
	repoID, err := c.currentRepoID()
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("cleanup plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func (c *Collector) deletePlan(planID string) {
	path, err := repo.GCPlanPathForDelete(c.repoRoot, planID)
	if err != nil {
		return
	}
	os.Remove(path)
}

func (c *Collector) snapshotDirExists(snapshotID model.SnapshotID) (bool, error) {
	_, err := repo.SnapshotPathForRead(c.repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Collector) descriptorFileExists(snapshotID model.SnapshotID) (bool, error) {
	_, err := repo.SnapshotDescriptorPathForRead(c.repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Collector) loadTombstone(snapshotID model.SnapshotID) (*model.Tombstone, error) {
	path, err := repo.GCTombstonePathForRead(c.repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var tombstone model.Tombstone
	if err := json.Unmarshal(data, &tombstone); err != nil {
		return nil, err
	}
	return &tombstone, nil
}

func (c *Collector) writeTombstone(tombstone *model.Tombstone) error {
	path, err := repo.GCTombstonePathForWrite(c.repoRoot, tombstone.SnapshotID)
	if err != nil {
		return err
	}
	gcDir := filepath.Dir(path)
	if err := os.MkdirAll(gcDir, 0755); err != nil {
		return fmt.Errorf("create tombstones dir: %w", err)
	}
	data, err := json.MarshalIndent(tombstone, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tombstone: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func sortSnapshotIDs(ids []model.SnapshotID) {
	sort.Slice(ids, func(i, j int) bool {
		return string(ids[i]) < string(ids[j])
	})
}

func sameSnapshotIDs(a, b []model.SnapshotID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
