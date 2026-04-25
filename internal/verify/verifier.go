// Package verify provides snapshot integrity verification.
package verify

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/pkg/model"
)

const (
	ErrorCodeSnapshotIDInvalid          = "E_SNAPSHOT_ID_INVALID"
	ErrorCodeDescriptorMissing          = "E_DESCRIPTOR_MISSING"
	ErrorCodeDescriptorCorrupt          = "E_DESCRIPTOR_CORRUPT"
	ErrorCodeDescriptorChecksumMismatch = "E_DESCRIPTOR_CHECKSUM_MISMATCH"
	ErrorCodePayloadMissing             = "E_PAYLOAD_MISSING"
	ErrorCodePayloadInvalid             = "E_PAYLOAD_INVALID"
	ErrorCodePayloadHashMismatch        = "E_PAYLOAD_HASH_MISMATCH"
	ErrorCodeReadyMissing               = "E_READY_MISSING"
	ErrorCodeReadyInvalid               = "E_READY_INVALID"
)

// Result contains verification results for a single snapshot.
type Result struct {
	SnapshotID       model.SnapshotID `json:"snapshot_id"`
	ChecksumValid    bool             `json:"checksum_valid"`
	PayloadHashValid bool             `json:"payload_hash_valid"`
	TamperDetected   bool             `json:"tamper_detected"`
	Severity         string           `json:"severity,omitempty"`
	Error            string           `json:"error,omitempty"`
	ErrorCode        string           `json:"error_code,omitempty"`
}

// Verifier performs integrity verification on snapshots.
type Verifier struct {
	repoRoot string
}

// NewVerifier creates a new verifier.
func NewVerifier(repoRoot string) *Verifier {
	return &Verifier{repoRoot: repoRoot}
}

// VerifySnapshot verifies a single snapshot's integrity.
func (v *Verifier) VerifySnapshot(snapshotID model.SnapshotID, verifyPayloadHash bool) (*Result, error) {
	result := &Result{
		SnapshotID: snapshotID,
	}
	if err := snapshotID.Validate(); err != nil {
		return markTampered(result, "critical", ErrorCodeSnapshotIDInvalid, fmt.Sprintf("invalid snapshot ID: %v", err)), nil
	}

	desc, err := snapshot.LoadDescriptor(v.repoRoot, snapshotID)
	if err != nil {
		return markTampered(result, "critical", descriptorLoadErrorCode(err), err.Error()), nil
	}

	// Verify descriptor checksum
	computedChecksum, err := integrity.ComputeDescriptorChecksum(desc)
	if err != nil {
		return markTampered(result, "error", ErrorCodeDescriptorCorrupt, fmt.Sprintf("compute checksum: %v", err)), nil
	}

	result.ChecksumValid = computedChecksum == desc.DescriptorChecksum
	if !result.ChecksumValid {
		return markTampered(result, "critical", ErrorCodeDescriptorChecksumMismatch, "descriptor checksum mismatch"), nil
	}

	if issue := CheckLineage(v.repoRoot, snapshotID); issue != nil {
		return markTampered(result, "critical", issue.Code, issue.Message), nil
	}

	// Optionally verify payload hash (expensive)
	if verifyPayloadHash {
		snapshotDir, err := repo.SnapshotPathForRead(v.repoRoot, snapshotID)
		if err != nil {
			return markTampered(result, "critical", payloadPathErrorCode(err), err.Error()), nil
		}
		readyExists, err := readyMarkerExists(snapshotDir)
		if err != nil {
			return markTampered(result, "critical", ErrorCodeReadyInvalid, fmt.Sprintf("READY marker invalid: %v", err)), nil
		}
		if !readyExists {
			return markTampered(result, "critical", ErrorCodeReadyMissing, "READY marker missing"), nil
		}
		computedHash, err := snapshotpayload.ComputeHash(snapshotDir, snapshotpayload.OptionsFromDescriptor(desc))
		if err != nil {
			return markTampered(result, "critical", ErrorCodePayloadInvalid, fmt.Sprintf("compute payload hash: %v", err)), nil
		}

		result.PayloadHashValid = computedHash == desc.PayloadRootHash
		if !result.PayloadHashValid {
			return markTampered(result, "critical", ErrorCodePayloadHashMismatch, "payload hash mismatch"), nil
		}
	}

	return result, nil
}

// VerifyAll verifies all snapshots in the repository.
func (v *Verifier) VerifyAll(verifyPayloadHash bool) ([]*Result, error) {
	ids, err := v.inventorySnapshotIDs()
	if err != nil {
		return nil, err
	}

	results := make([]*Result, 0, len(ids))
	for _, snapshotID := range ids {
		result, err := v.VerifySnapshot(snapshotID, verifyPayloadHash)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func markTampered(result *Result, severity, code, message string) *Result {
	result.TamperDetected = true
	result.Severity = severity
	result.ErrorCode = code
	result.Error = message
	return result
}

func descriptorLoadErrorCode(err error) string {
	if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "not found") {
		return ErrorCodeDescriptorMissing
	}
	return ErrorCodeDescriptorCorrupt
}

func payloadPathErrorCode(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return ErrorCodePayloadMissing
	}
	return ErrorCodePayloadInvalid
}

func readyMarkerExists(snapshotDir string) (bool, error) {
	found := false
	for _, name := range []string{".READY", ".READY.gz"} {
		readyPath := snapshotDir + string(os.PathSeparator) + name
		info, err := os.Lstat(readyPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("control leaf is symlink: %s", readyPath)
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("control leaf is not regular file: %s", readyPath)
		}
		found = true
	}
	return found, nil
}

func (v *Verifier) inventorySnapshotIDs() ([]model.SnapshotID, error) {
	seen := make(map[model.SnapshotID]bool)
	if err := v.collectDescriptorIDs(seen); err != nil {
		return nil, err
	}
	if err := v.collectPayloadIDs(seen); err != nil {
		return nil, err
	}
	if err := v.collectWorkspaceRefIDs(seen); err != nil {
		return nil, err
	}

	ids := make([]model.SnapshotID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids, nil
}

func (v *Verifier) collectDescriptorIDs(seen map[model.SnapshotID]bool) error {
	descriptorsDir, err := repo.DescriptorsDirPath(v.repoRoot)
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

func (v *Verifier) collectPayloadIDs(seen map[model.SnapshotID]bool) error {
	snapshotsDir, err := repo.SnapshotsDirPath(v.repoRoot)
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
		if id, ok := readyMarkerSnapshotID(entry.Name()); ok {
			seen[id] = true
			continue
		}
		id := model.SnapshotID(entry.Name())
		if id.IsValid() {
			seen[id] = true
		}
	}
	return nil
}

func readyMarkerSnapshotID(name string) (model.SnapshotID, bool) {
	for _, suffix := range []string{".READY", ".READY.gz"} {
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		id := model.SnapshotID(strings.TrimSuffix(name, suffix))
		return id, id.IsValid()
	}
	return "", false
}

func (v *Verifier) collectWorkspaceRefIDs(seen map[model.SnapshotID]bool) error {
	worktreesDir, err := repo.WorktreesDirPath(v.repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve worktrees directory: %w", err)
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read worktrees directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		cfg, err := repo.LoadWorktreeConfig(v.repoRoot, entry.Name())
		if err != nil {
			return fmt.Errorf("load worktree metadata %s: %w", entry.Name(), err)
		}
		addInventoryRef(seen, cfg.HeadSnapshotID)
		addInventoryRef(seen, cfg.LatestSnapshotID)
	}
	return nil
}

func addInventoryRef(seen map[model.SnapshotID]bool, id model.SnapshotID) {
	if id == "" {
		return
	}
	seen[id] = true
}
