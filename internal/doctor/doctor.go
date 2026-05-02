// Package doctor provides repository health checking and repair operations.
package doctor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/agentsmith-project/jvs/internal/audit"
	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/verify"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
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

type repairActionDef struct {
	RepairAction
	RuntimeSafe bool
	Implemented bool
	run         func(*Doctor) RepairResult
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

const (
	RepairCleanLocks               = "clean_locks"
	RepairRebindWorkspacePaths     = "rebind_workspace_paths"
	RepairCleanRuntimeTmp          = "clean_runtime_tmp"
	RepairCleanRuntimeOperations   = "clean_runtime_operations"
	RepairCleanRuntimeCleanupPlans = "clean_runtime_cleanup_plans"
)

const (
	ErrorCodeFormatVersionMissing        = "E_FORMAT_VERSION_MISSING"
	ErrorCodeFormatVersionInvalid        = "E_FORMAT_VERSION_INVALID"
	ErrorCodeFormatVersionUnsupported    = "E_FORMAT_VERSION_UNSUPPORTED"
	ErrorCodeWorktreeListFailed          = "E_WORKTREE_LIST_FAILED"
	ErrorCodeWorktreePayloadInvalid      = "E_WORKTREE_PAYLOAD_INVALID"
	ErrorCodeWorktreePathBindingInvalid  = "E_WORKTREE_PATH_BINDING_INVALID"
	ErrorCodeWorktreePayloadMissing      = "E_WORKTREE_PAYLOAD_MISSING"
	ErrorCodeWorktreeHeadMissing         = "E_WORKTREE_HEAD_MISSING"
	ErrorCodeWorktreeHeadInvalid         = "E_WORKTREE_HEAD_INVALID"
	ErrorCodeWorktreeInvalidSnapshotID   = "E_WORKTREE_INVALID_SNAPSHOT_ID"
	ErrorCodeWorktreeHeadLatestMismatch  = "E_WORKTREE_HEAD_LATEST_MISMATCH"
	ErrorCodeDescriptorControlInvalid    = "E_DESCRIPTOR_CONTROL_INVALID"
	ErrorCodeDescriptorFilenameInvalid   = "E_DESCRIPTOR_FILENAME_INVALID"
	ErrorCodeLineageDescriptorListFailed = "E_LINEAGE_DESCRIPTOR_LIST_FAILED"
	ErrorCodeIntentControlInvalid        = "E_INTENT_CONTROL_INVALID"
	ErrorCodeIntentOrphan                = "E_INTENT_ORPHAN"
	ErrorCodeReadyControlInvalid         = "E_READY_CONTROL_INVALID"
	ErrorCodeReadyInvalid                = snapshot.PublishStateCodeReadyInvalid
	ErrorCodeReadyInvalidSnapshotID      = "E_READY_INVALID_SNAPSHOT_ID"
	ErrorCodeReadyDescriptorInvalid      = "E_READY_DESCRIPTOR_INVALID"
	ErrorCodeReadyDescriptorMissing      = snapshot.PublishStateCodeReadyDescriptorMissing
	ErrorCodeReadyMissing                = snapshot.PublishStateCodeReadyMissing
	ErrorCodeStrictVerifyFailed          = "E_STRICT_VERIFY_FAILED"
	ErrorCodeTmpControlInvalid           = "E_TMP_CONTROL_INVALID"
	ErrorCodeTmpOrphan                   = "E_TMP_ORPHAN"
	ErrorCodeAuditScanFailed             = "E_AUDIT_SCAN_FAILED"
	ErrorCodeCloneHistoryInvalid         = "E_CLONE_HISTORY_INVALID"
)

var repairRegistry = []repairActionDef{
	{
		RepairAction: RepairAction{
			ID:          RepairCleanLocks,
			Description: "Remove stale repository mutation locks",
			AutoSafe:    true,
		},
		RuntimeSafe: true,
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			return d.repairCleanLocks()
		},
	},
	{
		RepairAction: RepairAction{
			ID:          RepairRebindWorkspacePaths,
			Description: "Rebind workspace folder paths after filesystem migration",
			AutoSafe:    true,
		},
		RuntimeSafe: true,
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			return d.repairRebindWorkspacePaths()
		},
	},
	{
		RepairAction: RepairAction{
			ID:          RepairCleanRuntimeTmp,
			Description: "Remove stale runtime temporary files",
			AutoSafe:    true,
		},
		RuntimeSafe: true,
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			result := d.repairCleanTmp()
			result.Action = RepairCleanRuntimeTmp
			return result
		},
	},
	{
		RepairAction: RepairAction{
			ID:          RepairCleanRuntimeOperations,
			Description: "Remove stale runtime operation records",
			AutoSafe:    true,
		},
		RuntimeSafe: true,
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			result := d.repairCleanIntents()
			result.Action = RepairCleanRuntimeOperations
			return result
		},
	},
	{
		RepairAction: RepairAction{
			ID:          RepairCleanRuntimeCleanupPlans,
			Description: "Remove stale cleanup preview/run plan state",
			AutoSafe:    true,
		},
		RuntimeSafe: true,
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			return d.repairCleanRuntimeCleanupPlans()
		},
	},
	{
		RepairAction: RepairAction{
			ID:          "advance_head",
			Description: "Advance stale internal current metadata to latest READY",
			AutoSafe:    false,
		},
		Implemented: true,
		run: func(d *Doctor) RepairResult {
			return d.repairAdvanceHead()
		},
	},
}

// NewDoctor creates a new doctor.
func NewDoctor(repoRoot string) *Doctor {
	return &Doctor{repoRoot: repoRoot}
}

// ListRepairActions returns executable public runtime-safe repair actions.
func (d *Doctor) ListRepairActions() []RepairAction {
	actions := make([]RepairAction, 0, len(repairRegistry))
	for _, def := range repairRegistry {
		if !def.RuntimeSafe || !def.Implemented || def.run == nil {
			continue
		}
		actions = append(actions, def.RepairAction)
	}
	return actions
}

// RuntimeRepairActionIDs returns executable public runtime-safe repair IDs.
func RuntimeRepairActionIDs() []string {
	ids := make([]string, 0, len(repairRegistry))
	for _, def := range repairRegistry {
		if !def.RuntimeSafe || !def.Implemented || def.run == nil {
			continue
		}
		ids = append(ids, def.ID)
	}
	return ids
}

// Repair executes the specified repair actions.
func (d *Doctor) Repair(actions []string) ([]RepairResult, error) {
	var results []RepairResult
	var lockedActions []string
	for _, action := range actions {
		def, ok := repairActionByID(action)
		if !ok || !def.Implemented || def.run == nil {
			results = append(results, unknownRepairResult(action))
			continue
		}
		if action == RepairCleanLocks {
			results = append(results, def.run(d))
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
		def, ok := repairActionByID(action)
		if !ok || !def.Implemented || def.run == nil {
			results = append(results, unknownRepairResult(action))
			continue
		}
		results = append(results, def.run(d))
	}
	return results, nil
}

func repairActionByID(id string) (repairActionDef, bool) {
	for _, def := range repairRegistry {
		if def.ID == id {
			return def, true
		}
	}
	return repairActionDef{}, false
}

func unknownRepairResult(action string) RepairResult {
	return RepairResult{
		Action:  action,
		Success: false,
		Message: "unknown repair action",
	}
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

func (d *Doctor) repairRebindWorkspacePaths() RepairResult {
	mgr := worktree.NewManager(d.repoRoot)
	list, err := mgr.List()
	if err != nil {
		return RepairResult{Action: RepairRebindWorkspacePaths, Success: false, Message: err.Error()}
	}

	rebound := 0
	skipped := 0
	failed := 0
	var reasons []string
	for _, cfg := range list {
		if cfg == nil || strings.TrimSpace(cfg.RealPath) == "" {
			continue
		}
		candidate, needsRebind, ok, reason := d.workspaceRebindPlan(cfg)
		if !needsRebind {
			continue
		}
		if !ok {
			skipped++
			reasons = append(reasons, fmt.Sprintf("%s: %s", cfg.Name, reason))
			continue
		}
		if err := mgr.RebindRealPath(cfg.Name, candidate); err != nil {
			failed++
			reasons = append(reasons, fmt.Sprintf("%s: %v", cfg.Name, err))
			continue
		}
		if cfg.Name != "main" {
			if err := repo.WriteWorkspaceLocator(candidate, d.repoRoot, cfg.Name); err != nil {
				failed++
				reasons = append(reasons, fmt.Sprintf("%s locator: %v", cfg.Name, err))
				continue
			}
		}
		rebound++
	}

	success := skipped == 0 && failed == 0
	message := fmt.Sprintf("rebound %d workspace path bindings", rebound)
	if skipped > 0 {
		message = fmt.Sprintf("%s; skipped %d without safe destination evidence", message, skipped)
	}
	if failed > 0 {
		message = fmt.Sprintf("%s; failed to rebind %d", message, failed)
	}
	if len(reasons) > 0 {
		message = fmt.Sprintf("%s (%s)", message, summarizeRepairReasons(reasons))
	}
	return RepairResult{
		Action:  RepairRebindWorkspacePaths,
		Success: success,
		Message: message,
		Cleaned: rebound,
	}
}

func (d *Doctor) workspaceRebindPlan(cfg *model.WorktreeConfig) (candidate string, needsRebind bool, ok bool, reason string) {
	if cfg.Name == "main" {
		return d.mainWorkspaceRebindPlan(cfg)
	}
	return d.externalWorkspaceRebindPlan(cfg)
}

func (d *Doctor) mainWorkspaceRebindPlan(cfg *model.WorktreeConfig) (string, bool, bool, string) {
	if strings.TrimSpace(cfg.RealPath) == "" || !filepath.IsAbs(cfg.RealPath) {
		return "", false, false, ""
	}
	if pathsReferToSameLocation(cfg.RealPath, d.repoRoot) {
		return "", false, true, ""
	}
	return d.repoRoot, true, true, ""
}

func (d *Doctor) externalWorkspaceRebindPlan(cfg *model.WorktreeConfig) (string, bool, bool, string) {
	if strings.TrimSpace(cfg.RealPath) == "" || !filepath.IsAbs(cfg.RealPath) {
		return "", false, false, ""
	}
	candidate := filepath.Join(filepath.Dir(d.repoRoot), cfg.Name)
	if d.externalWorkspaceBindingIsDestinationLocal(cfg, candidate) {
		current, reason := d.externalWorkspaceLocatorIsCurrent(candidate)
		if reason != "" {
			return "", true, false, reason
		}
		if !current {
			candidate, ok, reason := d.externalWorkspaceRebindCandidate(cfg, candidate)
			return candidate, true, ok, reason
		}
		return "", false, true, ""
	}
	candidate, ok, reason := d.externalWorkspaceRebindCandidate(cfg, candidate)
	return candidate, true, ok, reason
}

func (d *Doctor) externalWorkspaceBindingIsDestinationLocal(cfg *model.WorktreeConfig, candidate string) bool {
	if cfg == nil || strings.TrimSpace(cfg.RealPath) == "" || !filepath.IsAbs(cfg.RealPath) {
		return false
	}
	configured, ok := cleanAbsPathForCompare(cfg.RealPath)
	if !ok {
		return false
	}
	local, ok := cleanAbsPathForCompare(candidate)
	if !ok {
		return false
	}

	info, err := os.Lstat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return configured == local
		}
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if configured == local {
		return true
	}
	if !info.IsDir() {
		return false
	}
	return pathsReferToSameLocation(configured, local)
}

func (d *Doctor) externalWorkspaceRebindCandidate(cfg *model.WorktreeConfig, candidate string) (string, bool, string) {
	if filepath.Base(filepath.Clean(cfg.RealPath)) != cfg.Name {
		return "", false, "configured folder name does not match workspace name"
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, "destination sibling folder is missing"
		}
		return "", false, fmt.Sprintf("inspect destination sibling folder: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, "destination sibling folder is a symlink"
	}
	if !info.IsDir() {
		return "", false, "destination sibling path is not a directory"
	}
	if ok, reason := d.externalWorkspaceCandidateMatchesRecordedSource(cfg, candidate); !ok {
		return "", false, reason
	}
	return candidate, true, ""
}

func (d *Doctor) externalWorkspaceLocatorIsCurrent(candidate string) (bool, string) {
	matches, err := repo.WorkspaceLocatorMatchesRepo(candidate, d.repoRoot)
	if err != nil {
		return false, fmt.Sprintf("inspect destination workspace locator: %v", err)
	}
	return matches, ""
}

func (d *Doctor) externalWorkspaceCandidateMatchesRecordedSource(cfg *model.WorktreeConfig, candidate string) (bool, string) {
	if cfg.HeadSnapshotID == "" {
		return false, "workspace has no recorded content source"
	}
	if len(cfg.PathSources) > 0 {
		return false, "workspace has restored path sources"
	}
	desc, err := snapshot.LoadDescriptor(d.repoRoot, cfg.HeadSnapshotID)
	if err != nil {
		return false, fmt.Sprintf("load content source save point: %v", err)
	}
	if len(desc.PartialPaths) > 0 {
		return false, "content source is a partial save point"
	}
	excluded, err := destinationWorkspaceControlExclusions(candidate)
	if err != nil {
		return false, fmt.Sprintf("inspect destination workspace locator: %v", err)
	}
	hash, err := integrity.ComputePayloadRootHashWithExclusions(candidate, excluded)
	if err != nil {
		return false, fmt.Sprintf("hash destination sibling folder: %v", err)
	}
	if hash != desc.PayloadRootHash {
		return false, "destination sibling content does not match recorded content source"
	}
	return true, ""
}

func destinationWorkspaceControlExclusions(candidate string) (func(string) bool, error) {
	hasLocator, err := repo.WorkspaceLocatorPresent(candidate)
	if err != nil {
		return nil, err
	}
	if !hasLocator {
		return nil, nil
	}
	boundary := repo.WorktreePayloadBoundary{Root: candidate, ExcludedRootNames: []string{repo.JVSDirName}}
	return boundary.ExcludesRelativePath, nil
}

func pathsReferToSameLocation(left, right string) bool {
	leftAbs, leftOK := cleanAbsPathForCompare(left)
	rightAbs, rightOK := cleanAbsPathForCompare(right)
	if !leftOK || !rightOK {
		return false
	}
	if leftAbs == rightAbs {
		return true
	}

	leftPhysical, leftErr := filepath.EvalSymlinks(leftAbs)
	rightPhysical, rightErr := filepath.EvalSymlinks(rightAbs)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return filepath.Clean(leftPhysical) == filepath.Clean(rightPhysical)
}

func cleanAbsPathForCompare(path string) (string, bool) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(abs), true
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

func (d *Doctor) repairCleanRuntimeCleanupPlans() RepairResult {
	cleaned, err := gc.RemoveRuntimePlans(d.repoRoot)
	if err != nil {
		return RepairResult{
			Action:  RepairCleanRuntimeCleanupPlans,
			Success: false,
			Message: "cleanup plan state could not be safely cleaned",
			Cleaned: cleaned,
		}
	}
	return RepairResult{
		Action:  RepairCleanRuntimeCleanupPlans,
		Success: true,
		Message: fmt.Sprintf("cleaned %d stale cleanup plan%s", cleaned, pluralSuffix(cleaned)),
		Cleaned: cleaned,
	}
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

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
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
		d.checkImportedCloneHistory(result)
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
			ErrorCode:   ErrorCodeFormatVersionMissing,
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
			ErrorCode:   ErrorCodeFormatVersionInvalid,
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
			ErrorCode:   ErrorCodeFormatVersionUnsupported,
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
			ErrorCode:   ErrorCodeWorktreeListFailed,
		})
		return
	}

	for _, cfg := range list {
		d.checkWorkspaceLocalBinding(result, cfg)

		// Check payload directory exists
		payloadPath, err := wtMgr.Path(cfg.Name)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "worktree",
				Description: fmt.Sprintf("worktree '%s' payload path invalid: %v", cfg.Name, err),
				Severity:    "error",
				ErrorCode:   ErrorCodeWorktreePayloadInvalid,
			})
		} else if _, err := os.Stat(payloadPath); os.IsNotExist(err) {
			result.Findings = append(result.Findings, Finding{
				Category:    "worktree",
				Description: fmt.Sprintf("worktree '%s' payload directory missing", cfg.Name),
				Severity:    "error",
				ErrorCode:   ErrorCodeWorktreePayloadMissing,
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
						ErrorCode:   ErrorCodeWorktreeHeadMissing,
					})
					continue
				}
				result.Findings = append(result.Findings, Finding{
					Category:    "worktree",
					Description: fmt.Sprintf("worktree '%s' head snapshot %s path invalid: %v", cfg.Name, cfg.HeadSnapshotID, err),
					Severity:    "error",
					ErrorCode:   ErrorCodeWorktreeHeadInvalid,
				})
			}
		}
	}
}

func (d *Doctor) checkWorkspaceLocalBinding(result *Result, cfg *model.WorktreeConfig) {
	if cfg == nil || strings.TrimSpace(cfg.RealPath) == "" {
		return
	}
	if cfg.Name == "main" {
		d.checkMainWorkspaceLocalBinding(result, cfg)
		return
	}
	d.checkExternalWorkspaceLocalBinding(result, cfg)
}

func (d *Doctor) checkMainWorkspaceLocalBinding(result *Result, cfg *model.WorktreeConfig) {
	_, needsRebind, _, _ := d.mainWorkspaceRebindPlan(cfg)
	if !needsRebind {
		return
	}
	result.Findings = append(result.Findings, Finding{
		Category:    "worktree",
		Description: fmt.Sprintf("worktree '%s' payload path is bound to a different repository folder; run doctor --repair-runtime", cfg.Name),
		Severity:    "error",
		ErrorCode:   ErrorCodeWorktreePayloadInvalid,
		Path:        cfg.RealPath,
	})
}

func (d *Doctor) checkExternalWorkspaceLocalBinding(result *Result, cfg *model.WorktreeConfig) {
	if !filepath.IsAbs(cfg.RealPath) {
		return
	}
	candidate := filepath.Join(filepath.Dir(d.repoRoot), cfg.Name)
	if !d.externalWorkspaceBindingIsDestinationLocal(cfg, candidate) {
		result.Findings = append(result.Findings, Finding{
			Category:    "worktree",
			Description: fmt.Sprintf("worktree '%s' path binding is not destination-local; run doctor --repair-runtime", cfg.Name),
			Severity:    "error",
			ErrorCode:   ErrorCodeWorktreePathBindingInvalid,
			Path:        cfg.RealPath,
		})
		return
	}
	locatorCurrent, reason := d.externalWorkspaceLocatorIsCurrent(candidate)
	if reason != "" {
		result.Findings = append(result.Findings, Finding{
			Category:    "worktree",
			Description: fmt.Sprintf("worktree '%s' workspace locator invalid: %s; run doctor --repair-runtime", cfg.Name, reason),
			Severity:    "error",
			ErrorCode:   ErrorCodeWorktreePathBindingInvalid,
			Path:        filepath.Join(candidate, repo.JVSDirName),
		})
		return
	}
	if !locatorCurrent {
		result.Findings = append(result.Findings, Finding{
			Category:    "worktree",
			Description: fmt.Sprintf("worktree '%s' workspace locator is not bound to this repository; run doctor --repair-runtime", cfg.Name),
			Severity:    "error",
			ErrorCode:   ErrorCodeWorktreePathBindingInvalid,
			Path:        filepath.Join(candidate, repo.JVSDirName),
		})
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
		ErrorCode:   ErrorCodeWorktreeInvalidSnapshotID,
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
		ErrorCode:   ErrorCodeWorktreeHeadLatestMismatch,
	})
}

func (d *Doctor) checkLineage(result *Result) {
	ids, err := d.listDescriptorSnapshotIDs(result)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "lineage",
			Description: fmt.Sprintf("cannot list descriptors: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeLineageDescriptorListFailed,
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

func (d *Doctor) listDescriptorSnapshotIDs(result *Result) ([]model.SnapshotID, error) {
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
		entryPath, info, err := safeControlChildInfo(descriptorsDir, entry.Name())
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "descriptor",
				Description: fmt.Sprintf("descriptor entry %s invalid: %v", entry.Name(), err),
				Severity:    "error",
				ErrorCode:   ErrorCodeDescriptorControlInvalid,
				Path:        filepath.Join(descriptorsDir, entry.Name()),
			})
			continue
		}
		if !info.Mode().IsRegular() {
			result.Findings = append(result.Findings, Finding{
				Category:    "descriptor",
				Description: fmt.Sprintf("descriptor entry %s is not a regular file", entry.Name()),
				Severity:    "error",
				ErrorCode:   ErrorCodeDescriptorControlInvalid,
				Path:        entryPath,
			})
			continue
		}
		id := model.SnapshotID(strings.TrimSuffix(entry.Name(), ".json"))
		if !id.IsValid() {
			result.Findings = append(result.Findings, Finding{
				Category:    "descriptor",
				Description: fmt.Sprintf("descriptor filename %s does not contain a valid snapshot ID", entry.Name()),
				Severity:    "error",
				ErrorCode:   ErrorCodeDescriptorFilenameInvalid,
				Path:        entryPath,
			})
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (d *Doctor) checkOrphanIntents(result *Result) {
	intentsDir, err := repo.IntentsDirPath(d.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		result.Findings = append(result.Findings, Finding{
			Category:    "intent",
			Description: fmt.Sprintf("cannot read intents directory: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeIntentControlInvalid,
			Path:        filepath.Join(d.repoRoot, ".jvs", "intents"),
		})
		return
	}
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		result.Findings = append(result.Findings, Finding{
			Category:    "intent",
			Description: fmt.Sprintf("cannot read intents directory: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeIntentControlInvalid,
			Path:        intentsDir,
		})
		return
	}

	for _, entry := range entries {
		intentPath, info, err := safeControlChildInfo(intentsDir, entry.Name())
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "intent",
				Description: fmt.Sprintf("intent entry %s invalid: %v", entry.Name(), err),
				Severity:    "error",
				ErrorCode:   ErrorCodeIntentControlInvalid,
				Path:        filepath.Join(intentsDir, entry.Name()),
			})
			continue
		}
		if !info.Mode().IsRegular() {
			result.Findings = append(result.Findings, Finding{
				Category:    "intent",
				Description: fmt.Sprintf("intent entry %s is not a regular file", entry.Name()),
				Severity:    "error",
				ErrorCode:   ErrorCodeIntentControlInvalid,
				Path:        intentPath,
			})
			continue
		}
		result.Findings = append(result.Findings, Finding{
			Category:    "intent",
			Description: fmt.Sprintf("orphan intent file: %s", entry.Name()),
			Severity:    "warning",
			ErrorCode:   ErrorCodeIntentOrphan,
			Path:        intentPath,
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
			ErrorCode:   ErrorCodeReadyControlInvalid,
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
			ErrorCode:   ErrorCodeReadyControlInvalid,
			Path:        snapshotsDir,
		})
		return
	}

	seen := make(map[model.SnapshotID]bool)
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
				ErrorCode:   ErrorCodeReadyControlInvalid,
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
				ErrorCode:   ErrorCodeReadyInvalidSnapshotID,
				Path:        entryPath,
			})
			continue
		}
		seen[snapshotID] = true

		snapshotDir, err := repo.SnapshotPathForRead(d.repoRoot, snapshotID)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Category:    "snapshot",
				Description: fmt.Sprintf("snapshot %s path invalid: %v", snapshotID, err),
				Severity:    "error",
				ErrorCode:   ErrorCodeReadyControlInvalid,
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
				ErrorCode:   ErrorCodeReadyInvalid,
				Path:        snapshotDir,
			})
			continue
		}

		if readyExists {
			_, err := repo.SnapshotDescriptorPathForRead(d.repoRoot, snapshotID)
			if err == nil {
				if _, issue := snapshot.InspectPublishState(d.repoRoot, snapshotID, snapshot.PublishStateOptions{RequireReady: true}); issue != nil && issue.Code == ErrorCodeReadyInvalid {
					result.Findings = append(result.Findings, Finding{
						Category:    "snapshot",
						Description: fmt.Sprintf("snapshot %s READY marker invalid: %s", snapshotID, issue.Message),
						Severity:    "error",
						ErrorCode:   issue.Code,
						Path:        issue.Path,
					})
				}
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
					ErrorCode:   ErrorCodeReadyDescriptorInvalid,
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
				ErrorCode:   ErrorCodeReadyDescriptorMissing,
				Path:        descriptorPath,
			})
			continue
		}

		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("snapshot %s READY marker missing", snapshotID),
			Severity:    "error",
			ErrorCode:   ErrorCodeReadyMissing,
			Path:        filepath.Join(snapshotDir, ".READY"),
		})
	}

	descriptorIDs, err := d.listDescriptorSnapshotIDs(result)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("cannot inspect descriptor publish state: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeReadyDescriptorInvalid,
		})
		return
	}
	for _, snapshotID := range descriptorIDs {
		if seen[snapshotID] {
			continue
		}
		_, issue := snapshot.InspectPublishState(d.repoRoot, snapshotID, snapshot.PublishStateOptions{
			RequireReady:   true,
			RequirePayload: true,
		})
		if issue == nil {
			continue
		}
		result.Findings = append(result.Findings, Finding{
			Category:    "snapshot",
			Description: fmt.Sprintf("snapshot %s publish state invalid: %s", snapshotID, issue.Message),
			Severity:    "error",
			ErrorCode:   issue.Code,
			Path:        issue.Path,
		})
	}
}

func (d *Doctor) checkImportedCloneHistory(result *Result) {
	if _, _, err := clonehistory.LoadValidatedManifest(d.repoRoot); err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "clone_history",
			Description: fmt.Sprintf("imported clone history manifest invalid: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeCloneHistoryInvalid,
		})
	}
}

func readyMarkerExists(snapshotDir string) (bool, error) {
	return snapshot.PublishReadyMarkerExists(snapshotDir)
}

func (d *Doctor) checkSnapshotIntegrity(result *Result) {
	verifier := verify.NewVerifier(d.repoRoot)
	results, err := verifier.VerifyAll(true)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "integrity",
			Description: fmt.Sprintf("verification failed: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeStrictVerifyFailed,
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
			errorCode := r.ErrorCode
			if errorCode == "" {
				errorCode = ErrorCodeStrictVerifyFailed
			}
			result.Findings = append(result.Findings, Finding{
				Category:    "integrity",
				Description: description,
				Severity:    severity,
				ErrorCode:   errorCode,
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
				ErrorCode:   ErrorCodeTmpOrphan,
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
			ErrorCode:   ErrorCodeTmpControlInvalid,
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
			ErrorCode:   ErrorCodeTmpControlInvalid,
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
				ErrorCode:   ErrorCodeTmpControlInvalid,
			})
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") && info.IsDir() {
			result.Findings = append(result.Findings, Finding{
				Category:    "tmp",
				Description: fmt.Sprintf("orphan snapshot tmp directory: %s", entry.Name()),
				Severity:    "warning",
				ErrorCode:   ErrorCodeTmpOrphan,
				Path:        entryPath,
			})
		}
	}
}

// checkAuditChain verifies the audit log hash chain integrity.
func (d *Doctor) checkAuditChain(result *Result) {
	auditPath := filepath.Join(d.repoRoot, ".jvs", "audit", "audit.jsonl")
	issues, err := audit.VerifyFile(auditPath)
	if err != nil {
		result.Findings = append(result.Findings, Finding{
			Category:    "audit",
			Description: fmt.Sprintf("cannot verify audit log: %v", err),
			Severity:    "error",
			ErrorCode:   ErrorCodeAuditScanFailed,
			Path:        auditPath,
		})
		return
	}

	for _, issue := range issues {
		result.Findings = append(result.Findings, Finding{
			Category:    "audit",
			Description: issue.Message,
			Severity:    issue.Severity,
			ErrorCode:   issue.ErrorCode,
			Path:        auditPath,
		})
	}
}
