// Package doctor provides repository health checking and repair operations.
package doctor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
)

// Finding represents a detected issue.
type Finding struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	ErrorCode   string `json:"error_code,omitempty"`
	Path        string `json:"path,omitempty"`
}

// Result contains doctor check results.
type Result struct {
	Healthy  bool      `json:"healthy"`
	Findings []Finding `json:"findings"`
}

var intentSnapshotIDFieldPattern = regexp.MustCompile(`"snapshot_id"\s*:\s*"([^"]+)"`)

func severityAffectsHealth(severity string) bool {
	switch strings.ToLower(severity) {
	case "error", "critical":
		return true
	default:
		return false
	}
}

func (r *Result) updateHealthFromFindings() {
	r.Healthy = true
	for _, finding := range r.Findings {
		if severityAffectsHealth(finding.Severity) {
			r.Healthy = false
			return
		}
	}
}

// RepairAction describes a repair operation.
type RepairAction struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	AutoSafe    bool   `json:"auto_safe"`
}

// RepairResult contains the result of a repair operation.
type RepairResult struct {
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Cleaned int    `json:"cleaned,omitempty"`
}

// Doctor performs repository health checks.
type Doctor struct {
	repoRoot string
}

// NewDoctor creates a new doctor.
func NewDoctor(repoRoot string) *Doctor {
	return &Doctor{repoRoot: repoRoot}
}

// ListRepairActions returns all available repair actions.
func (d *Doctor) ListRepairActions() []RepairAction {
	return []RepairAction{
		{ID: "clean_locks", Description: "Remove safely stale repository mutation locks", AutoSafe: true},
		{ID: "clean_tmp", Description: "Remove orphan .tmp files and directories", AutoSafe: true},
		{ID: "clean_intents", Description: "Remove completed/abandoned intent files", AutoSafe: true},
		{ID: "rebuild_index", Description: "Rebuild index from snapshot state", AutoSafe: false},
		{ID: "audit_repair", Description: "Recompute audit hash chain", AutoSafe: false},
		{ID: "advance_head", Description: "Advance stale head to latest READY", AutoSafe: false},
	}
}

// Repair executes the specified repair actions.
func (d *Doctor) Repair(actions []string) ([]RepairResult, error) {
	var results []RepairResult
	var lockedActions []string
	for _, action := range actions {
		if action == "clean_locks" {
			results = append(results, d.repairCleanLocks())
			continue
		}
		lockedActions = append(lockedActions, action)
	}
	if len(lockedActions) == 0 {
		return results, nil
	}

	err := repo.WithMutationLock(d.repoRoot, "doctor repair", func() error {
		var err error
		var lockedResults []RepairResult
		lockedResults, err = d.repair(lockedActions)
		results = append(results, lockedResults...)
		return err
	})
	return results, err
}

func (d *Doctor) repair(actions []string) ([]RepairResult, error) {
	results := []RepairResult{}
	for _, action := range actions {
		switch action {
		case "clean_locks":
			results = append(results, d.repairCleanLocks())
		case "clean_tmp":
			results = append(results, d.repairCleanTmp())
		case "clean_intents":
			results = append(results, d.repairCleanIntents())
		case "advance_head":
			results = append(results, d.repairAdvanceHead())
		default:
			results = append(results, RepairResult{
				Action:  action,
				Success: false,
				Message: "unknown repair action",
			})
		}
	}
	return results, nil
}

func (d *Doctor) repairCleanLocks() RepairResult {
	inspection, removed, err := repo.RemoveStaleMutationLock(d.repoRoot)
	if err != nil {
		return RepairResult{Action: "clean_locks", Success: false, Message: err.Error()}
	}
	if removed {
		return RepairResult{
			Action:  "clean_locks",
			Success: true,
			Message: "cleaned 1 stale repository lock",
			Cleaned: 1,
		}
	}
	if inspection.Status == repo.MutationLockInvalid {
		return RepairResult{
			Action:  "clean_locks",
			Success: false,
			Message: fmt.Sprintf("retained repository lock: %s", inspection.Reason),
		}
	}
	return RepairResult{
		Action:  "clean_locks",
		Success: true,
		Message: "cleaned 0 stale repository locks",
	}
}

func (d *Doctor) repairCleanTmp() RepairResult {
	cleaned := 0

	// Clean root-level JVS temp files without recursing into worktree payloads.
	if entries, err := os.ReadDir(d.repoRoot); err == nil {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".jvs-tmp-") {
				continue
			}
			path := filepath.Join(d.repoRoot, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if err := os.RemoveAll(path); err == nil {
					cleaned++
				}
				continue
			}
			if err := os.Remove(path); err == nil {
				cleaned++
			}
		}
	}

	// Clean orphan snapshot .tmp directories
	snapshotsDir, err := repo.SnapshotsDirPath(d.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepairResult{
				Action:  "clean_tmp",
				Success: true,
				Message: fmt.Sprintf("cleaned %d tmp files/directories", cleaned),
				Cleaned: cleaned,
			}
		}
		return RepairResult{Action: "clean_tmp", Success: false, Message: err.Error(), Cleaned: cleaned}
	}
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return RepairResult{Action: "clean_tmp", Success: false, Message: err.Error(), Cleaned: cleaned}
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		tmpPath, info, err := safeControlChildInfo(snapshotsDir, entry.Name())
		if err != nil {
			return RepairResult{Action: "clean_tmp", Success: false, Message: err.Error(), Cleaned: cleaned}
		}
		if info.IsDir() {
			if err := os.RemoveAll(tmpPath); err == nil {
				cleaned++
			} else {
				return RepairResult{Action: "clean_tmp", Success: false, Message: err.Error(), Cleaned: cleaned}
			}
		}
	}

	return RepairResult{
		Action:  "clean_tmp",
		Success: true,
		Message: fmt.Sprintf("cleaned %d tmp files/directories", cleaned),
		Cleaned: cleaned,
	}
}

func (d *Doctor) repairCleanIntents() RepairResult {
	cleaned := 0
	retained := 0

	intentsDir, err := repo.IntentsDirPath(d.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepairResult{Action: "clean_intents", Success: true, Message: "no intents directory"}
		}
		return RepairResult{Action: "clean_intents", Success: false, Message: err.Error()}
	}
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return RepairResult{Action: "clean_intents", Success: true, Message: "no intents directory"}
		}
		return RepairResult{Action: "clean_intents", Success: false, Message: err.Error()}
	}

	metadataRefs := make(map[model.SnapshotID]bool)
	if len(entries) > 0 {
		metadataRefs, err = d.collectMetadataSnapshotRefs()
		if err != nil {
			return RepairResult{Action: "clean_intents", Success: false, Message: fmt.Sprintf("inspect worktree metadata: %v", err)}
		}
	}

	for _, entry := range entries {
		intentPath, info, err := safeControlChildInfo(intentsDir, entry.Name())
		if err != nil {
			return RepairResult{Action: "clean_intents", Success: false, Message: err.Error(), Cleaned: cleaned}
		}
		if info.IsDir() {
			continue
		}
		retain, err := d.intentHasRecoveryEvidence(intentPath, entry.Name(), metadataRefs)
		if err != nil {
			return RepairResult{Action: "clean_intents", Success: false, Message: err.Error(), Cleaned: cleaned}
		}
		if retain {
			retained++
			continue
		}
		if err := os.Remove(intentPath); err == nil {
			cleaned++
		} else if !os.IsNotExist(err) {
			return RepairResult{Action: "clean_intents", Success: false, Message: err.Error(), Cleaned: cleaned}
		}
	}

	message := fmt.Sprintf("cleaned %d orphan intent files", cleaned)
	if retained > 0 {
		message = fmt.Sprintf("%s; warning: retained %d recoverable intent files", message, retained)
	}

	return RepairResult{Action: "clean_intents", Success: true, Message: message, Cleaned: cleaned}
}

func (d *Doctor) collectMetadataSnapshotRefs() (map[model.SnapshotID]bool, error) {
	refs := make(map[model.SnapshotID]bool)
	worktreesDir, err := repo.WorktreesDirPath(d.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return refs, nil
		}
		return nil, err
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return refs, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		configDir, info, err := safeControlChildInfo(worktreesDir, entry.Name())
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		configPath, configInfo, err := safeControlChildInfo(configDir, "config.json")
		if err != nil {
			return nil, err
		}
		if configInfo.IsDir() {
			return nil, fmt.Errorf("worktree config is directory: %s", configPath)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		var cfg model.WorktreeConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse worktree config %s: %w", configPath, err)
		}
		addMetadataSnapshotRef(refs, cfg.BaseSnapshotID)
		addMetadataSnapshotRef(refs, cfg.HeadSnapshotID)
		addMetadataSnapshotRef(refs, cfg.LatestSnapshotID)
	}
	return refs, nil
}

func addMetadataSnapshotRef(refs map[model.SnapshotID]bool, id model.SnapshotID) {
	if id != "" && id.IsValid() {
		refs[id] = true
	}
}

func (d *Doctor) intentHasRecoveryEvidence(intentPath, name string, metadataRefs map[model.SnapshotID]bool) (bool, error) {
	data, err := os.ReadFile(intentPath)
	if err != nil {
		return false, fmt.Errorf("read intent %s: %w", intentPath, err)
	}

	ids := make(map[model.SnapshotID]bool)
	if id, ok := snapshotIDFromIntentFilename(name); ok {
		ids[id] = true
	}

	var intent model.IntentRecord
	if err := json.Unmarshal(data, &intent); err == nil {
		if intent.SnapshotID.IsValid() {
			ids[intent.SnapshotID] = true
		}
	} else {
		for _, id := range snapshotIDsFromMalformedIntent(data) {
			ids[id] = true
		}
	}

	for id := range ids {
		hasEvidence, err := d.snapshotHasRecoveryEvidence(id, metadataRefs)
		if err != nil {
			return false, err
		}
		if hasEvidence {
			return true, nil
		}
	}
	return false, nil
}

func snapshotIDFromIntentFilename(name string) (model.SnapshotID, bool) {
	if !strings.HasSuffix(name, ".json") {
		return "", false
	}
	id := model.SnapshotID(strings.TrimSuffix(name, ".json"))
	if !id.IsValid() {
		return "", false
	}
	return id, true
}

func snapshotIDsFromMalformedIntent(data []byte) []model.SnapshotID {
	var ids []model.SnapshotID
	seen := make(map[model.SnapshotID]bool)
	for _, match := range intentSnapshotIDFieldPattern.FindAllSubmatch(data, -1) {
		if len(match) != 2 {
			continue
		}
		id := model.SnapshotID(string(match[1]))
		if !id.IsValid() || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func (d *Doctor) snapshotHasRecoveryEvidence(snapshotID model.SnapshotID, metadataRefs map[model.SnapshotID]bool) (bool, error) {
	if metadataRefs[snapshotID] {
		return true, nil
	}

	if _, err := repo.SnapshotDescriptorPathForRead(d.repoRoot, snapshotID); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if _, err := repo.SnapshotPathForRead(d.repoRoot, snapshotID); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func safeControlChildInfo(parent, name string) (string, os.FileInfo, error) {
	if name == "." || name == ".." || filepath.Base(name) != name {
		return "", nil, fmt.Errorf("invalid control child name: %s", name)
	}
	clean, err := pathutil.CleanRel(name)
	if err != nil {
		return "", nil, err
	}
	if clean != name {
		return "", nil, fmt.Errorf("invalid control child name: %s", name)
	}
	if err := pathutil.ValidateNoSymlinkParents(parent, clean); err != nil {
		return "", nil, err
	}
	path := filepath.Join(parent, clean)
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("control child is symlink: %s", path)
	}
	return path, info, nil
}

func (d *Doctor) repairAdvanceHead() RepairResult {
	// Find worktrees with stale head_snapshot_id and advance to latest READY
	wtMgr := worktree.NewManager(d.repoRoot)
	list, err := wtMgr.List()
	if err != nil {
		return RepairResult{Action: "advance_head", Success: false, Message: err.Error()}
	}

	advanced := 0
	skippedInvalid := 0
	skippedUnsafe := 0
	updateFailed := 0
	var unsafeReasons []string
	var updateReasons []string
	for _, cfg := range list {
		if !worktreeMetadataRefsValid(cfg) {
			skippedInvalid++
			continue
		}
		if cfg.LatestSnapshotID == "" {
			continue
		}

		if cfg.HeadSnapshotID != cfg.LatestSnapshotID {
			if err := d.snapshotReadyForMetadataAdvance(cfg.LatestSnapshotID); err != nil {
				skippedUnsafe++
				unsafeReasons = append(unsafeReasons, fmt.Sprintf("%s latest %s: %v", cfg.Name, cfg.LatestSnapshotID, err))
				continue
			}
			if cfg.HeadSnapshotID != "" {
				isHistoricalHead, err := verify.IsAncestor(d.repoRoot, cfg.HeadSnapshotID, cfg.LatestSnapshotID)
				if err != nil {
					skippedUnsafe++
					unsafeReasons = append(unsafeReasons, fmt.Sprintf("%s lineage %s..%s: %v", cfg.Name, cfg.HeadSnapshotID, cfg.LatestSnapshotID, err))
					continue
				}
				if isHistoricalHead {
					continue
				}
				skippedUnsafe++
				unsafeReasons = append(unsafeReasons, fmt.Sprintf("%s head %s is not in latest %s lineage", cfg.Name, cfg.HeadSnapshotID, cfg.LatestSnapshotID))
				continue
			}
			if err := wtMgr.SetLatest(cfg.Name, cfg.LatestSnapshotID); err != nil {
				updateFailed++
				updateReasons = append(updateReasons, fmt.Sprintf("%s: %v", cfg.Name, err))
				continue
			}
			advanced++
		}
	}

	message := fmt.Sprintf("advanced %d stale heads to latest", advanced)
	success := true
	if skippedInvalid > 0 {
		success = false
		message = fmt.Sprintf("%s; skipped %d worktrees with invalid snapshot metadata", message, skippedInvalid)
	}
	if skippedUnsafe > 0 {
		success = false
		message = fmt.Sprintf("%s; skipped %d worktrees with unsafe/recoverable snapshot metadata", message, skippedUnsafe)
		if len(unsafeReasons) > 0 {
			message = fmt.Sprintf("%s (%s)", message, summarizeRepairReasons(unsafeReasons))
		}
	}
	if updateFailed > 0 {
		success = false
		message = fmt.Sprintf("%s; failed to update %d worktrees", message, updateFailed)
		if len(updateReasons) > 0 {
			message = fmt.Sprintf("%s (%s)", message, summarizeRepairReasons(updateReasons))
		}
	}

	return RepairResult{
		Action:  "advance_head",
		Success: success,
		Message: message,
		Cleaned: advanced,
	}
}

func (d *Doctor) snapshotReadyForMetadataAdvance(snapshotID model.SnapshotID) error {
	if err := snapshotID.Validate(); err != nil {
		return err
	}

	snapshotDir, err := repo.SnapshotPathForRead(d.repoRoot, snapshotID)
	if err != nil {
		return fmt.Errorf("snapshot path invalid: %w", err)
	}

	readyExists, err := readyMarkerExists(snapshotDir)
	if err != nil {
		return fmt.Errorf("READY marker invalid: %w", err)
	}
	if !readyExists {
		return errors.New("READY marker missing")
	}

	descriptorPath, err := repo.SnapshotDescriptorPathForRead(d.repoRoot, snapshotID)
	if err != nil {
		return fmt.Errorf("descriptor path invalid: %w", err)
	}
	if err := validateDescriptorForMetadataAdvance(descriptorPath, snapshotID); err != nil {
		return err
	}
	return nil
}

func validateDescriptorForMetadataAdvance(descriptorPath string, snapshotID model.SnapshotID) error {
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		return fmt.Errorf("read descriptor: %w", err)
	}
	var desc model.Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return fmt.Errorf("parse descriptor: %w", err)
	}
	if desc.SnapshotID != snapshotID {
		return fmt.Errorf("descriptor snapshot ID %q does not match requested %q", desc.SnapshotID, snapshotID)
	}
	computedChecksum, err := integrity.ComputeDescriptorChecksum(&desc)
	if err != nil {
		return fmt.Errorf("compute descriptor checksum: %w", err)
	}
	if computedChecksum != desc.DescriptorChecksum {
		return errors.New("descriptor checksum mismatch")
	}
	return nil
}

func summarizeRepairReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}
	return fmt.Sprintf("%s; +%d more", reasons[0], len(reasons)-1)
}

// Check runs all diagnostic checks.
func (d *Doctor) Check(strict bool) (*Result, error) {
	result := &Result{Healthy: true}

	// 1. Check format version
	d.checkFormatVersion(result)

	// 2. Check repository mutation lock state
	d.checkMutationLock(result)

	// 3. Check worktrees
	d.checkWorktrees(result)

	// 4. Check snapshot completion markers
	d.checkReadyMarkers(result, strict)

	// 5. Check descriptor parent links and worktree head/latest reachability
	d.checkLineage(result)

	// 6. Check for orphan intents
	d.checkOrphanIntents(result)

	// 7. Check snapshot integrity (if strict)
	if strict {
		d.checkSnapshotIntegrity(result)
		// 8. Check audit chain (if strict)
		d.checkAuditChain(result)
	}

	// 9. Check for orphan tmp files
	d.checkOrphanTmp(result)

	result.updateHealthFromFindings()

	return result, nil
}

func (d *Doctor) checkMutationLock(result *Result) {
	inspection, err := repo.InspectMutationLock(d.repoRoot)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "lock",
			Description: fmt.Sprintf("cannot inspect repository mutation lock: %v", err),
			Severity:    "error",
			ErrorCode:   "E_REPO_LOCK_INSPECT_FAILED",
		})
		return
	}
	switch inspection.Status {
	case repo.MutationLockStale:
		result.Findings = append(result.Findings, Finding{
			Category:    "lock",
			Description: fmt.Sprintf("stale repository mutation lock: %s", inspection.Reason),
			Severity:    "error",
			ErrorCode:   "E_REPO_LOCK_STALE",
			Path:        inspection.Path,
		})
	case repo.MutationLockInvalid:
		result.Findings = append(result.Findings, Finding{
			Category:    "lock",
			Description: fmt.Sprintf("repository mutation lock owner invalid: %s", inspection.Reason),
			Severity:    "error",
			ErrorCode:   "E_REPO_LOCK_OWNER_INVALID",
			Path:        inspection.OwnerPath,
		})
	}
}

func (d *Doctor) checkFormatVersion(result *Result) {
	versionPath := filepath.Join(d.repoRoot, ".jvs", "format_version")
	data, err := os.ReadFile(versionPath)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "format",
			Description: "format_version file missing or unreadable",
			Severity:    "critical",
			Path:        versionPath,
		})
		result.Healthy = false
		return
	}

	version, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || version <= 0 {
		result.Findings = append(result.Findings, Finding{
			Category:    "format",
			Description: fmt.Sprintf("format_version file contains invalid content: %q", strings.TrimSpace(string(data))),
			Severity:    "critical",
			Path:        versionPath,
		})
		result.Healthy = false
		return
	}

	if version > repo.FormatVersion {
		result.Findings = append(result.Findings, Finding{
			Category:    "format",
			Description: fmt.Sprintf("format version %d > supported %d", version, repo.FormatVersion),
			Severity:    "critical",
		})
		result.Healthy = false
	}
}

func (d *Doctor) checkWorktrees(result *Result) {
	wtMgr := worktree.NewManager(d.repoRoot)
	list, err := wtMgr.List()
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "worktree",
			Description: fmt.Sprintf("cannot list worktrees: %v", err),
			Severity:    "error",
		})
		return
	}

	for _, cfg := range list {
		// Check payload directory exists
		payloadPath, err := wtMgr.Path(cfg.Name)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "worktree",
				Description: fmt.Sprintf("worktree '%s' payload path invalid: %v", cfg.Name, err),
				Severity:    "error",
			})
		} else if _, err := os.Stat(payloadPath); os.IsNotExist(err) {
			result.Findings = append(result.Findings, Finding{
				Category:    "worktree",
				Description: fmt.Sprintf("worktree '%s' payload directory missing", cfg.Name),
				Severity:    "error",
				Path:        payloadPath,
			})
		}

		d.checkWorktreeSnapshotRef(result, cfg.Name, "base_snapshot_id", cfg.BaseSnapshotID)
		headValid := d.checkWorktreeSnapshotRef(result, cfg.Name, "head_snapshot_id", cfg.HeadSnapshotID)
		d.checkWorktreeSnapshotRef(result, cfg.Name, "latest_snapshot_id", cfg.LatestSnapshotID)
		d.checkWorktreeLineagePosition(result, cfg)

		// Check head snapshot exists
		if cfg.HeadSnapshotID != "" && headValid {
			if _, err := repo.SnapshotDescriptorPathForRead(d.repoRoot, cfg.HeadSnapshotID); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					result.Findings = append(result.Findings, Finding{
						Category:    "worktree",
						Description: fmt.Sprintf("worktree '%s' head snapshot %s not found", cfg.Name, cfg.HeadSnapshotID),
						Severity:    "warning",
					})
					continue
				}
				result.Findings = append(result.Findings, Finding{
					Category:    "worktree",
					Description: fmt.Sprintf("worktree '%s' head snapshot %s path invalid: %v", cfg.Name, cfg.HeadSnapshotID, err),
					Severity:    "error",
				})
			}
		}
	}
}

func (d *Doctor) checkWorktreeSnapshotRef(result *Result, worktreeName, field string, id model.SnapshotID) bool {
	if worktreeSnapshotRefValid(id) {
		return true
	}
	err := id.Validate()
	result.Findings = append(result.Findings, Finding{
		Category:    "worktree",
		Description: fmt.Sprintf("worktree '%s' %s %s invalid: %v", worktreeName, field, id, err),
		Severity:    "error",
		ErrorCode:   "E_WORKTREE_INVALID_SNAPSHOT_ID",
	})
	return false
}

func worktreeSnapshotRefValid(id model.SnapshotID) bool {
	return id == "" || id.IsValid()
}

func worktreeMetadataRefsValid(cfg *model.WorktreeConfig) bool {
	return worktreeSnapshotRefValid(cfg.BaseSnapshotID) &&
		worktreeSnapshotRefValid(cfg.HeadSnapshotID) &&
		worktreeSnapshotRefValid(cfg.LatestSnapshotID)
}

func (d *Doctor) checkWorktreeLineagePosition(result *Result, cfg *model.WorktreeConfig) {
	if cfg == nil || cfg.HeadSnapshotID == "" || cfg.LatestSnapshotID == "" || cfg.HeadSnapshotID == cfg.LatestSnapshotID {
		return
	}
	if !worktreeMetadataRefsValid(cfg) {
		return
	}

	isAncestor, err := verify.IsAncestor(d.repoRoot, cfg.HeadSnapshotID, cfg.LatestSnapshotID)
	if err != nil {
		return
	}
	if isAncestor {
		return
	}
	result.Findings = append(result.Findings, Finding{
		Category:    "worktree",
		Description: fmt.Sprintf("worktree '%s' head snapshot %s is not in latest snapshot %s lineage", cfg.Name, cfg.HeadSnapshotID, cfg.LatestSnapshotID),
		Severity:    "error",
		ErrorCode:   "E_WORKTREE_HEAD_LATEST_MISMATCH",
	})
}

func (d *Doctor) checkLineage(result *Result) {
	ids, err := d.listDescriptorSnapshotIDs()
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "lineage",
			Description: fmt.Sprintf("cannot list descriptors: %v", err),
			Severity:    "error",
			ErrorCode:   "E_LINEAGE_DESCRIPTOR_LIST_FAILED",
		})
		return
	}

	for _, snapshotID := range ids {
		if issue := verify.CheckLineage(d.repoRoot, snapshotID); issue != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "lineage",
				Description: issue.Message,
				Severity:    "error",
				ErrorCode:   issue.Code,
			})
		}
	}
}

func (d *Doctor) listDescriptorSnapshotIDs() ([]model.SnapshotID, error) {
	descriptorsDir, err := repo.DescriptorsDirPath(d.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(descriptorsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []model.SnapshotID
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		_, info, err := safeControlChildInfo(descriptorsDir, entry.Name())
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		id := model.SnapshotID(strings.TrimSuffix(entry.Name(), ".json"))
		if !id.IsValid() {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (d *Doctor) checkOrphanIntents(result *Result) {
	intentsDir := filepath.Join(d.repoRoot, ".jvs", "intents")
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		return // directory doesn't exist, that's fine
	}

	for _, entry := range entries {
		result.Findings = append(result.Findings, Finding{
			Category:    "intent",
			Description: fmt.Sprintf("orphan intent file: %s", entry.Name()),
			Severity:    "warning",
			Path:        filepath.Join(intentsDir, entry.Name()),
		})
	}
}

func (d *Doctor) checkReadyMarkers(result *Result, strict bool) {
	snapshotsDir, err := repo.SnapshotsDirPath(d.repoRoot)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("cannot read snapshots directory: %v", err),
			Severity:    "error",
		})
		return
	}

	entries, err := os.ReadDir(snapshotsDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("cannot read snapshots directory: %v", err),
			Severity:    "error",
			Path:        snapshotsDir,
		})
		return
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}

		entryPath, info, err := safeControlChildInfo(snapshotsDir, entry.Name())
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot entry %s invalid: %v", entry.Name(), err),
				Severity:    "error",
				ErrorCode:   "E_READY_CONTROL_INVALID",
			})
			continue
		}
		if !info.IsDir() {
			continue
		}

		snapshotID := model.SnapshotID(entry.Name())
		if err := snapshotID.Validate(); err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot entry %s invalid: %v", entry.Name(), err),
				Severity:    "error",
				ErrorCode:   "E_READY_INVALID_SNAPSHOT_ID",
				Path:        entryPath,
			})
			continue
		}

		snapshotDir, err := repo.SnapshotPathForRead(d.repoRoot, snapshotID)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot %s path invalid: %v", snapshotID, err),
				Severity:    "error",
				ErrorCode:   "E_READY_CONTROL_INVALID",
				Path:        entryPath,
			})
			continue
		}

		readyExists, err := readyMarkerExists(snapshotDir)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot %s READY marker invalid: %v", snapshotID, err),
				Severity:    "error",
				ErrorCode:   "E_READY_CONTROL_INVALID",
				Path:        snapshotDir,
			})
			continue
		}

		if readyExists {
			_, err := repo.SnapshotDescriptorPathForRead(d.repoRoot, snapshotID)
			if err == nil {
				continue
			}
			descriptorPath, pathErr := repo.SnapshotDescriptorPath(d.repoRoot, snapshotID)
			if pathErr != nil {
				descriptorPath = ""
			}
			if !errors.Is(err, os.ErrNotExist) {
				result.Findings = append(result.Findings, Finding{
					Category:    "snapshot",
					Description: fmt.Sprintf("snapshot %s READY descriptor path invalid: %v", snapshotID, err),
					Severity:    "error",
					ErrorCode:   "E_READY_DESCRIPTOR_INVALID",
					Path:        descriptorPath,
				})
				continue
			}
			severity := "error"
			if strict {
				severity = "critical"
			}
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot %s READY marker present but descriptor missing", snapshotID),
				Severity:    severity,
				ErrorCode:   "E_READY_DESCRIPTOR_MISSING",
				Path:        descriptorPath,
			})
			continue
		}

		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("snapshot %s READY marker missing", snapshotID),
			Severity:    "error",
			ErrorCode:   "E_READY_MISSING",
			Path:        filepath.Join(snapshotDir, ".READY"),
		})
	}
}

func readyMarkerExists(snapshotDir string) (bool, error) {
	found := false
	for _, name := range []string{".READY", ".READY.gz"} {
		exists, err := readyMarkerLeafExists(snapshotDir, name)
		if err != nil {
			return false, err
		}
		found = found || exists
	}
	return found, nil
}

func readyMarkerLeafExists(snapshotDir, name string) (bool, error) {
	readyPath, info, err := safeControlChildInfo(snapshotDir, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("READY marker is not regular file: %s", readyPath)
	}
	return true, nil
}

func (d *Doctor) checkSnapshotIntegrity(result *Result) {
	verifier := verify.NewVerifier(d.repoRoot)
	results, err := verifier.VerifyAll(true)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "integrity",
			Description: fmt.Sprintf("verification failed: %v", err),
			Severity:    "error",
		})
		return
	}

	for _, r := range results {
		if r.TamperDetected || r.Error != "" || severityAffectsHealth(r.Severity) {
			severity := r.Severity
			if severity == "" {
				severity = "error"
			}
			description := fmt.Sprintf("snapshot %s verification failed", r.SnapshotID)
			if r.Error != "" {
				description = fmt.Sprintf("snapshot %s: %s", r.SnapshotID, r.Error)
			}
			result.Findings = append(result.Findings, Finding{
				Category:    "integrity",
				Description: description,
				Severity:    severity,
			})
		}
	}
}

func (d *Doctor) checkOrphanTmp(result *Result) {
	// Check root-level JVS temp files without recursing into worktree payloads.
	if entries, err := os.ReadDir(d.repoRoot); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, ".jvs-tmp-") {
				continue
			}
			result.Findings = append(result.Findings, Finding{
				Category:    "tmp",
				Description: fmt.Sprintf("orphan temp file: %s", name),
				Severity:    "info",
				Path:        filepath.Join(d.repoRoot, name),
			})
		}
	}

	// Check for orphan snapshot .tmp directories
	snapshotsDir, err := repo.SnapshotsDirPath(d.repoRoot)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "tmp",
			Description: fmt.Sprintf("cannot read snapshots directory: %v", err),
			Severity:    "error",
			ErrorCode:   "E_TMP_CONTROL_INVALID",
			Path:        filepath.Join(d.repoRoot, ".jvs", "snapshots"),
		})
		return
	}
	entries, err := os.ReadDir(snapshotsDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "tmp",
			Description: fmt.Sprintf("cannot read snapshots directory: %v", err),
			Severity:    "error",
			ErrorCode:   "E_TMP_CONTROL_INVALID",
			Path:        snapshotsDir,
		})
		return
	}

	for _, entry := range entries {
		entryPath, info, err := safeControlChildInfo(snapshotsDir, entry.Name())
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "tmp",
				Description: fmt.Sprintf("snapshot tmp entry %s invalid: %v", entry.Name(), err),
				Severity:    "error",
				ErrorCode:   "E_TMP_CONTROL_INVALID",
			})
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") && info.IsDir() {
			result.Findings = append(result.Findings, Finding{
				Category:    "tmp",
				Description: fmt.Sprintf("orphan snapshot tmp directory: %s", entry.Name()),
				Severity:    "warning",
				Path:        entryPath,
			})
		}
	}
}

// checkAuditChain verifies the audit log hash chain integrity.
func (d *Doctor) checkAuditChain(result *Result) {
	auditPath := filepath.Join(d.repoRoot, ".jvs", "audit", "audit.jsonl")
	file, err := os.Open(auditPath)
	if os.IsNotExist(err) {
		return // No audit log yet is OK
	}
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "audit",
			Description: fmt.Sprintf("cannot open audit log: %v", err),
			Severity:    "warning",
			Path:        auditPath,
		})
		return
	}
	defer file.Close()

	var prevHash model.HashValue
	var lineNum int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		var record model.AuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "audit",
				Description: fmt.Sprintf("malformed record at line %d", lineNum),
				Severity:    "warning",
				Path:        auditPath,
			})
			continue
		}

		// Verify chain linkage (skip first record which has empty prevHash)
		if prevHash != "" && record.PrevHash != prevHash {
			result.Findings = append(result.Findings, Finding{
				Category:    "audit",
				Description: fmt.Sprintf("audit hash chain broken at line %d", lineNum),
				Severity:    "critical",
				ErrorCode:   "E_AUDIT_CHAIN_BROKEN",
				Path:        auditPath,
			})
			result.Healthy = false
			return
		}
		prevHash = record.RecordHash
	}

	if err := scanner.Err(); err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "audit",
			Description: fmt.Sprintf("error reading audit log: %v", err),
			Severity:    "warning",
			Path:        auditPath,
		})
	}
}
