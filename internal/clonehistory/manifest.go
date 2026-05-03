// Package clonehistory owns durable metadata for save points imported by
// repository clone. It deliberately avoids clone orchestration.
package clonehistory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
)

const (
	ManifestSchemaVersion = 1

	OperationRepoClone = "repo_clone"
	SavePointsModeAll  = "all"
)

const (
	manifestDirName  = "clone-history"
	manifestFileName = "imported-save-points.json"
)

// Manifest records imported all-clone save points that must remain protected
// from reviewed cleanup.
type Manifest struct {
	SchemaVersion      int                `json:"schema_version"`
	Operation          string             `json:"operation"`
	SourceRepoID       string             `json:"source_repo_id"`
	TargetRepoID       string             `json:"target_repo_id"`
	SavePointsMode     string             `json:"save_points_mode"`
	RuntimeStateCopied bool               `json:"runtime_state_copied"`
	ProtectionReason   string             `json:"protection_reason"`
	ImportedSavePoints []model.SnapshotID `json:"imported_save_points"`
	// ImportedSavePointsCount and ImportedSavePointsChecksum are durable
	// self-evidence for the canonical imported save point set.
	ImportedSavePointsCount    int    `json:"imported_save_points_count"`
	ImportedSavePointsChecksum string `json:"imported_save_points_checksum"`
}

// ManifestPath returns the durable imported clone history manifest path.
func ManifestPath(repoRoot string) string {
	return filepath.Join(repoRoot, repo.JVSDirName, manifestDirName, manifestFileName)
}

// LoadManifest reads the imported clone history manifest if present. Malformed
// or unsafe manifest storage returns an error so callers can fail closed.
func LoadManifest(repoRoot string) (*Manifest, bool, error) {
	path := ManifestPath(repoRoot)
	ok, err := manifestFilePresent(repoRoot, path)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read imported clone history manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false, fmt.Errorf("imported clone history manifest is invalid JSON: %w", err)
	}
	return &manifest, true, nil
}

// LoadValidatedManifest reads and validates the manifest if present.
func LoadValidatedManifest(repoRoot string) (*Manifest, bool, error) {
	manifest, ok, err := LoadManifest(repoRoot)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if err := ValidateManifest(repoRoot, manifest); err != nil {
		return nil, false, err
	}
	return manifest, true, nil
}

// WriteManifest validates and atomically writes the imported clone history
// manifest.
func WriteManifest(repoRoot string, manifest Manifest) error {
	normalized := normalizeManifest(manifest)
	if err := ValidateManifest(repoRoot, &normalized); err != nil {
		return err
	}
	if err := ensureManifestDirForWrite(repoRoot); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal imported clone history manifest: %w", err)
	}
	return fsutil.AtomicWrite(ManifestPath(repoRoot), data, 0644)
}

// ValidateManifest verifies manifest identity, shape, and referenced save point
// publish state. A validation error means cleanup and doctor must fail closed.
func ValidateManifest(repoRoot string, manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("imported clone history manifest is missing")
	}
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("imported clone history manifest schema_version %d is unsupported", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.Operation) != OperationRepoClone {
		return fmt.Errorf("imported clone history manifest operation %q is unsupported", manifest.Operation)
	}
	if strings.TrimSpace(manifest.SourceRepoID) == "" {
		return fmt.Errorf("imported clone history manifest source_repo_id is required")
	}
	r, err := repo.Discover(repoRoot)
	if err != nil {
		return fmt.Errorf("read target repo identity for imported clone history manifest: %w", err)
	}
	if strings.TrimSpace(r.RepoID) == "" {
		return fmt.Errorf("target repo id is missing for imported clone history manifest")
	}
	if strings.TrimSpace(manifest.TargetRepoID) != r.RepoID {
		return errclass.ErrRepoIDMismatch.WithMessagef("imported clone history manifest target repo mismatch: manifest has %s, target repo has %s", strings.TrimSpace(manifest.TargetRepoID), r.RepoID)
	}
	if manifest.SavePointsMode != SavePointsModeAll {
		return fmt.Errorf("imported clone history manifest save_points_mode %q is unsupported", manifest.SavePointsMode)
	}
	if manifest.RuntimeStateCopied {
		return fmt.Errorf("imported clone history manifest runtime_state_copied must be false")
	}
	if manifest.ProtectionReason != model.GCProtectionReasonImportedCloneHistory {
		return fmt.Errorf("imported clone history manifest protection_reason %q is unsupported", manifest.ProtectionReason)
	}
	if err := validateImportedSavePointsEvidence(manifest); err != nil {
		return err
	}
	return validateImportedSavePoints(repoRoot, manifest.ImportedSavePoints)
}

// ComputeImportedSavePointsEvidence returns the canonical count and checksum for
// imported save point IDs. It is exposed for tests and low-level recovery tools
// that need to construct raw manifest fixtures without going through
// WriteManifest.
func ComputeImportedSavePointsEvidence(ids []model.SnapshotID) (int, string) {
	canonical := canonicalImportedSavePointStrings(ids)
	data, err := json.Marshal(canonical)
	if err != nil {
		// json.Marshal on []string cannot fail in practice; keep the format
		// deterministic even if the standard library contract changes.
		data = []byte(strings.Join(canonical, "\n"))
	}
	sum := sha256.Sum256(data)
	return len(canonical), "sha256:" + hex.EncodeToString(sum[:])
}

func validateImportedSavePointsEvidence(manifest *Manifest) error {
	count, checksum := ComputeImportedSavePointsEvidence(manifest.ImportedSavePoints)
	if manifest.ImportedSavePointsCount != count {
		return fmt.Errorf("imported clone history manifest imported_save_points_count mismatch: got %d want %d", manifest.ImportedSavePointsCount, count)
	}
	if strings.TrimSpace(manifest.ImportedSavePointsChecksum) != checksum {
		return fmt.Errorf("imported clone history manifest imported_save_points_checksum mismatch")
	}
	return nil
}

func validateImportedSavePoints(repoRoot string, ids []model.SnapshotID) error {
	seen := make(map[model.SnapshotID]bool, len(ids))
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("imported clone history manifest imported save point %q invalid: %w", string(id), err)
		}
		if seen[id] {
			return fmt.Errorf("imported clone history manifest imported save point %s is duplicated", id)
		}
		seen[id] = true

		_, issue := snapshot.InspectPublishState(repoRoot, id, snapshot.PublishStateOptions{
			RequireReady:             true,
			RequirePayload:           true,
			VerifyDescriptorChecksum: true,
			VerifyPayloadHash:        true,
		})
		if issue != nil {
			return fmt.Errorf("imported clone history manifest imported save point %s is not ready: %s", id, issue.Message)
		}
	}
	return nil
}

func normalizeManifest(manifest Manifest) Manifest {
	manifest.SourceRepoID = strings.TrimSpace(manifest.SourceRepoID)
	manifest.TargetRepoID = strings.TrimSpace(manifest.TargetRepoID)
	manifest.ImportedSavePoints = append([]model.SnapshotID(nil), manifest.ImportedSavePoints...)
	sort.Slice(manifest.ImportedSavePoints, func(i, j int) bool {
		return string(manifest.ImportedSavePoints[i]) < string(manifest.ImportedSavePoints[j])
	})
	manifest.ImportedSavePointsCount, manifest.ImportedSavePointsChecksum = ComputeImportedSavePointsEvidence(manifest.ImportedSavePoints)
	return manifest
}

func canonicalImportedSavePointStrings(ids []model.SnapshotID) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, string(id))
	}
	sort.Strings(values)
	return values
}

func manifestFilePresent(repoRoot, path string) (bool, error) {
	dir := filepath.Join(repoRoot, repo.JVSDirName, manifestDirName)
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect imported clone history manifest directory: %w", err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("imported clone history manifest directory is a symlink")
	}
	if !dirInfo.IsDir() {
		return false, fmt.Errorf("imported clone history manifest directory is not a directory")
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("imported clone history manifest file is missing")
		}
		return false, fmt.Errorf("inspect imported clone history manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("imported clone history manifest is a symlink")
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("imported clone history manifest is not a regular file")
	}
	return true, nil
}

func ensureManifestDirForWrite(repoRoot string) error {
	jvsDir := filepath.Join(repoRoot, repo.JVSDirName)
	dir := filepath.Join(jvsDir, manifestDirName)
	info, err := os.Lstat(dir)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("imported clone history manifest directory is a symlink")
		}
		if !info.IsDir() {
			return fmt.Errorf("imported clone history manifest directory is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect imported clone history manifest directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create imported clone history manifest directory: %w", err)
	}
	return fsutil.FsyncDir(jvsDir)
}
