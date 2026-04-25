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

	"github.com/jvs-project/jvs/internal/audit"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/uuidutil"
)

// Collector handles garbage collection of unused snapshots.
type Collector struct {
	repoRoot         string
	auditLogger      *audit.FileAppender
	progressCallback func(string, int, int, string)
}

var collectorFsyncDir = fsutil.FsyncDir

type planInputs struct {
	protectedSet         []model.SnapshotID
	protectedByLineage   int
	protectedByPin       int
	protectedByRetention int
	toDelete             []model.SnapshotID
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
	inputs, err := c.computePlanInputs(policy)
	if err != nil {
		return nil, err
	}

	deletableBytes := int64(len(inputs.toDelete)) * 1024 * 1024

	plan := &model.GCPlan{
		PlanID:                 uuidutil.NewV4(),
		CreatedAt:              time.Now().UTC(),
		ProtectedSet:           inputs.protectedSet,
		ProtectedByPin:         inputs.protectedByPin,
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

	pending, err := c.revalidatePlan(plan)
	if err != nil {
		return err
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

	// Cleanup plan
	c.deletePlan(planID)

	// Audit
	c.auditLogger.Append(model.EventTypeGCRun, "", "", map[string]any{
		"plan_id":       planID,
		"deleted_count": deleted,
	})

	return nil
}

func (c *Collector) computePlanInputs(policy model.RetentionPolicy) (*planInputs, error) {
	return c.computePlanInputsAllowingReadyMissingDescriptors(policy, nil)
}

func (c *Collector) computePlanInputsAllowingReadyMissingDescriptors(policy model.RetentionPolicy, allowReadyMissingDescriptor map[model.SnapshotID]bool) (*planInputs, error) {
	protectedSet, protectedByLineage, protectedByPin, err := c.computeProtectedSet()
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
		protectedByLineage:   protectedByLineage,
		protectedByPin:       protectedByPin,
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
		return nil, errclass.ErrGCPlanMismatch.WithMessagef("candidate set changed: planned=%v current=%v", currentComparable, currentCandidates)
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

func (c *Collector) computeProtectedSet() ([]model.SnapshotID, int, int, error) {
	protected := make(map[model.SnapshotID]bool)
	lineageCount := 0
	pinCount := 0

	// 1. All live worktree roots. Latest remains a live restore target when a
	// worktree is detached, and Base is conservatively treated as live.
	wtMgr := worktree.NewManager(c.repoRoot)
	wtList, err := wtMgr.List()
	if err != nil {
		return nil, 0, 0, err
	}
	roots := make(map[model.SnapshotID]bool)
	addRoot := func(id model.SnapshotID) {
		if id != "" {
			roots[id] = true
		}
	}
	for _, cfg := range wtList {
		addRoot(cfg.HeadSnapshotID)
		addRoot(cfg.LatestSnapshotID)
		addRoot(cfg.BaseSnapshotID)
	}

	rootIDs := make([]model.SnapshotID, 0, len(roots))
	for id := range roots {
		if err := id.Validate(); err != nil {
			continue
		}
		protected[id] = true
		rootIDs = append(rootIDs, id)
	}
	sortSnapshotIDs(rootIDs)

	// 2. Lineage traversal (keep parent chains)
	for _, id := range rootIDs {
		lineageCount += c.walkLineage(id, protected)
	}

	// 3. All intents (in-progress operations)
	intentsDir, err := repo.IntentsDirPath(c.repoRoot)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, 0, 0, fmt.Errorf("read intents directory: %w", err)
		}
	} else {
		entries, err := os.ReadDir(intentsDir)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read intents directory: %w", err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasSuffix(name, ".json") {
				protected[model.SnapshotID(strings.TrimSuffix(name, ".json"))] = true
			}
		}
	}

	// 4. All pins. The documented v0.x path is .jvs/gc/pins. The legacy
	// .jvs/pins directory remains supported when present, but is validated and
	// read with the same fail-closed rules.
	pinCount, err = c.addPinsToProtectedSet(protected)
	if err != nil {
		return nil, 0, 0, err
	}

	var result []model.SnapshotID
	for id := range protected {
		result = append(result, id)
	}
	sortSnapshotIDs(result)
	return result, lineageCount, pinCount, nil
}

type pinDirReader struct {
	label       string
	optional    bool
	dirPath     func(string) (string, error)
	pinFilePath func(string, string) (string, error)
}

func (c *Collector) addPinsToProtectedSet(protected map[model.SnapshotID]bool) (int, error) {
	readers := []pinDirReader{
		{
			label:       ".jvs/gc/pins",
			dirPath:     repo.GCPinsDirPath,
			pinFilePath: repo.GCPinPathForRead,
		},
		{
			label:       ".jvs/pins",
			optional:    true,
			dirPath:     repo.LegacyPinsDirPath,
			pinFilePath: repo.LegacyPinPathForRead,
		},
	}

	pinCount := 0
	now := time.Now()
	for _, reader := range readers {
		pinsDir, err := reader.dirPath(c.repoRoot)
		if err != nil {
			if reader.optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, fmt.Errorf("validate pin directory %s: %w", reader.label, err)
		}

		pinEntries, err := os.ReadDir(pinsDir)
		if err != nil {
			if reader.optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, fmt.Errorf("read pin directory %s: %w", reader.label, err)
		}

		for _, entry := range pinEntries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}

			pinPath, err := reader.pinFilePath(c.repoRoot, name)
			if err != nil {
				return 0, fmt.Errorf("validate pin file %s/%s: %w", reader.label, name, err)
			}
			data, err := os.ReadFile(pinPath)
			if err != nil {
				return 0, fmt.Errorf("read pin file %s/%s: %w", reader.label, name, err)
			}
			var pin model.Pin
			if err := json.Unmarshal(data, &pin); err != nil {
				return 0, fmt.Errorf("parse pin file %s/%s: %w", reader.label, name, err)
			}
			if err := pin.SnapshotID.Validate(); err != nil {
				return 0, fmt.Errorf("validate pin file %s/%s snapshot_id: %w", reader.label, name, err)
			}
			if pin.ExpiresAt != nil && pin.ExpiresAt.Before(now) {
				continue
			}
			if !protected[pin.SnapshotID] {
				protected[pin.SnapshotID] = true
				pinCount++
			}
		}
	}
	return pinCount, nil
}

func (c *Collector) walkLineage(snapshotID model.SnapshotID, protected map[model.SnapshotID]bool) int {
	return c.walkLineageSeen(snapshotID, protected, make(map[model.SnapshotID]bool))
}

func (c *Collector) walkLineageSeen(snapshotID model.SnapshotID, protected map[model.SnapshotID]bool, seen map[model.SnapshotID]bool) int {
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
	if desc.ParentID != nil && !protected[*desc.ParentID] {
		protected[*desc.ParentID] = true
		count = 1 + c.walkLineageSeen(*desc.ParentID, protected, seen)
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

// LoadPlan loads a GC plan by ID.
func (c *Collector) LoadPlan(planID string) (*model.GCPlan, error) {
	path, err := repo.GCPlanPathForRead(c.repoRoot, planID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan model.GCPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
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
