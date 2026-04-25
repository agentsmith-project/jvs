// Package repo handles JVS repository initialization and discovery.
package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
	"github.com/jvs-project/jvs/pkg/uuidutil"
)

const (
	// FormatVersion is the current repository format version.
	FormatVersion = 1
	// JVSDirName is the name of the JVS metadata directory.
	JVSDirName = ".jvs"
	// FormatVersionFile is the name of the file storing the format version.
	FormatVersionFile = "format_version"
	// RepoIDFile is the name of the file storing the repository ID.
	RepoIDFile = "repo_id"
)

// Repo represents an initialized JVS repository.
type Repo struct {
	Root          string
	FormatVersion int
	RepoID        string
}

// ValidateInitTarget returns the absolute target path after enforcing the
// repository creation rules: the target must be missing or empty, must not
// already contain .jvs metadata, and must not be nested inside a JVS repo.
func ValidateInitTarget(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("repository path must not be empty")
	}

	target, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	target = filepath.Clean(target)

	if err := ensureTargetEmptyOrMissing(target); err != nil {
		return "", err
	}
	if err := rejectNestedInitTarget(target); err != nil {
		return "", err
	}
	return target, nil
}

// InitTarget creates a new repository at an absolute or relative target path.
func InitTarget(path string) (*Repo, error) {
	target, err := ValidateInitTarget(path)
	if err != nil {
		return nil, err
	}
	return initAt(target)
}

// Init creates a new JVS repository at the specified path.
func Init(path string, name string) (*Repo, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}
	target, err := ValidateInitTarget(path)
	if err != nil {
		return nil, err
	}
	return initAt(target)
}

func initAt(path string) (*Repo, error) {
	// Create directory structure
	jvsDir := filepath.Join(path, JVSDirName)
	dirs := []string{
		jvsDir,
		filepath.Join(jvsDir, "worktrees", "main"),
		filepath.Join(jvsDir, "snapshots"),
		filepath.Join(jvsDir, "descriptors"),
		filepath.Join(jvsDir, "intents"),
		filepath.Join(jvsDir, "audit"),
		filepath.Join(jvsDir, "locks"),
		filepath.Join(jvsDir, "gc"),
		filepath.Join(jvsDir, "gc", "pins"),
		filepath.Join(jvsDir, "gc", "tombstones"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Write format_version
	if err := os.WriteFile(filepath.Join(jvsDir, FormatVersionFile), []byte("1\n"), 0600); err != nil {
		return nil, fmt.Errorf("write format_version: %w", err)
	}

	// Write repo_id
	repoID := uuidutil.NewV4()
	if err := os.WriteFile(filepath.Join(jvsDir, RepoIDFile), []byte(repoID+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("write repo_id: %w", err)
	}

	// Create main/ payload directory
	mainDir := filepath.Join(path, "main")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		return nil, fmt.Errorf("create main directory: %w", err)
	}

	// Create worktrees/ payload directory
	worktreesPayload := filepath.Join(path, "worktrees")
	if err := os.MkdirAll(worktreesPayload, 0755); err != nil {
		return nil, fmt.Errorf("create worktrees directory: %w", err)
	}

	// Write main worktree config
	cfg := &model.WorktreeConfig{
		Name:      "main",
		CreatedAt: time.Now().UTC(),
	}
	if err := WriteWorktreeConfig(path, "main", cfg); err != nil {
		return nil, fmt.Errorf("write main config: %w", err)
	}

	// Fsync parent to ensure durability
	if err := fsutil.FsyncDir(path); err != nil {
		return nil, fmt.Errorf("fsync repo root: %w", err)
	}

	return &Repo{
		Root:          path,
		FormatVersion: FormatVersion,
		RepoID:        repoID,
	}, nil
}

func ensureTargetEmptyOrMissing(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			if err := rejectExistingMetadataAt(target); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("stat repository target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository target must not be a symlink: %s", target)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository target exists and is not a directory: %s", target)
	}
	if err := rejectExistingMetadataAt(target); err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read repository target: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("repository target must be empty or not exist: %s", target)
	}
	return nil
}

func rejectExistingMetadataAt(target string) error {
	jvsDir := filepath.Join(target, JVSDirName)
	info, err := os.Stat(jvsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat existing metadata: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("repository target already contains %s: %s", JVSDirName, target)
	}
	return fmt.Errorf("repository target contains reserved %s path: %s", JVSDirName, target)
}

func rejectNestedInitTarget(target string) error {
	parent := filepath.Dir(target)
	for {
		if info, err := os.Stat(filepath.Join(parent, JVSDirName)); err == nil && info.IsDir() {
			return fmt.Errorf("cannot initialize nested repository inside existing JVS repository: %s", target)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return nil
		}
		parent = next
	}
}

// Discover walks up from cwd to find the repo root (directory containing .jvs/).
func Discover(cwd string) (*Repo, error) {
	path := cwd
	for {
		jvsDir := filepath.Join(path, JVSDirName)
		if info, err := os.Stat(jvsDir); err == nil && info.IsDir() {
			// Found .jvs/, read format_version
			version, err := readFormatVersion(jvsDir)
			if err != nil {
				return nil, err
			}
			if version > FormatVersion {
				return nil, errclass.ErrFormatUnsupported.WithMessagef(
					"format version %d > supported %d", version, FormatVersion)
			}
			repoID, _ := readRepoID(jvsDir)
			return &Repo{
				Root:          path,
				FormatVersion: version,
				RepoID:        repoID,
			}, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			// Reached root without finding .jvs/
			return nil, fmt.Errorf("no JVS repository found (no .jvs/ in parent directories)")
		}
		path = parent
	}
}

// DiscoverWorktree discovers the repo and maps cwd to a worktree name.
func DiscoverWorktree(cwd string) (*Repo, string, error) {
	r, err := Discover(cwd)
	if err != nil {
		return nil, "", err
	}

	// Get relative path from repo root
	rel, err := filepath.Rel(r.Root, cwd)
	if err != nil {
		return nil, "", fmt.Errorf("compute relative path: %w", err)
	}

	// Map to worktree name
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return r, "", nil
	}

	switch parts[0] {
	case "main":
		return r, "main", nil
	case "worktrees":
		if len(parts) >= 2 {
			return r, parts[1], nil
		}
		return r, "", nil
	case JVSDirName:
		// Inside .jvs/, not a worktree
		return r, "", nil
	default:
		return r, "", nil
	}
}

// WorktreeConfigPath returns the path to a worktree's config.json.
func WorktreeConfigPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, JVSDirName, "worktrees", name, "config.json"), nil
}

// WorktreeConfigDirPath returns the metadata directory for a worktree.
func WorktreeConfigDirPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, JVSDirName, "worktrees", name), nil
}

// WorktreesDirPath returns the worktrees control directory after validating it.
func WorktreesDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "worktrees")
}

// WriteWorktreeConfig atomically writes a worktree config.
func WriteWorktreeConfig(repoRoot, name string, cfg *model.WorktreeConfig) error {
	path, err := safeWorktreeConfigPath(repoRoot, name)
	if err != nil {
		return err
	}
	if err := rejectSymlinkOrDirLeaf(path); err != nil {
		return fmt.Errorf("validate worktree config path: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worktree config: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

// LoadWorktreeConfig loads a worktree config.
func LoadWorktreeConfig(repoRoot, name string) (*model.WorktreeConfig, error) {
	path, err := safeWorktreeConfigPath(repoRoot, name)
	if err != nil {
		return nil, err
	}
	if err := rejectSymlinkOrDirLeaf(path); err != nil {
		return nil, fmt.Errorf("read worktree config: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read worktree config: %w", err)
	}
	var cfg model.WorktreeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse worktree config: %w", err)
	}
	return &cfg, nil
}

// WorktreePayloadPath returns the payload directory for a worktree.
func WorktreePayloadPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	if name == "main" {
		return filepath.Join(repoRoot, "main"), nil
	}
	return filepath.Join(repoRoot, "worktrees", name), nil
}

func safeWorktreeConfigPath(repoRoot, name string) (string, error) {
	path, err := WorktreeConfigPath(repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := validateControlDirs(repoRoot, "worktrees", name); err != nil {
		return "", fmt.Errorf("validate worktree config path: %w", err)
	}
	return path, nil
}

func rejectSymlinkOrDirLeaf(path string) error {
	return validateControlLeaf(path, controlLeafRegularFile, true)
}

// SnapshotsDirPath returns the snapshots control directory after validating it.
func SnapshotsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "snapshots")
}

// IntentsDirPath returns the intents control directory after validating it.
func IntentsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "intents")
}

// DescriptorsDirPath returns the descriptors control directory after validating it.
func DescriptorsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "descriptors")
}

// GCDirPath returns the GC control directory after validating it.
func GCDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc")
}

// GCTombstonesDirPath returns the tombstones control directory after validating it.
func GCTombstonesDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc", "tombstones")
}

// GCPinsDirPath returns the documented GC pins control directory after validating it.
func GCPinsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc", "pins")
}

// LegacyPinsDirPath returns the legacy pins control directory after validating it.
func LegacyPinsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "pins")
}

// SnapshotPath returns the on-disk snapshot storage path for a canonical ID.
func SnapshotPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"snapshots"}, string(snapshotID))
}

// SnapshotPathForRead returns an existing snapshot directory path after
// rejecting a symlink or wrong-type final leaf.
func SnapshotPathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafDirectory, false); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotPathForDelete returns a snapshot directory path after rejecting a
// symlink or wrong-type final leaf. Missing leaves are allowed for idempotent
// delete/retry paths.
func SnapshotPathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafDirectory, true); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotTmpPath returns the unpublished temporary snapshot path for a canonical ID.
func SnapshotTmpPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"snapshots"}, string(snapshotID)+".tmp")
}

// SnapshotDescriptorPath returns the descriptor path for a canonical snapshot ID.
func SnapshotDescriptorPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"descriptors"}, string(snapshotID)+".json")
}

// SnapshotDescriptorPathForRead returns an existing descriptor path after
// rejecting a symlink or wrong-type final leaf.
func SnapshotDescriptorPathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotDescriptorPathForWrite returns a descriptor path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func SnapshotDescriptorPathForWrite(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotDescriptorPathForDelete returns a descriptor path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func SnapshotDescriptorPathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// IntentPath returns the intent record path for a canonical snapshot ID.
func IntentPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"intents"}, string(snapshotID)+".json")
}

// GCTombstonePath returns the tombstone path for a canonical snapshot ID.
func GCTombstonePath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"gc", "tombstones"}, string(snapshotID)+".json")
}

// GCTombstonePathForRead returns an existing tombstone path after rejecting a
// symlink or wrong-type final leaf.
func GCTombstonePathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCTombstonePathForWrite returns a tombstone path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCTombstonePathForWrite(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCTombstonePathForDelete returns a tombstone path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCTombstonePathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCPinPathForRead returns an existing documented GC pin path after rejecting a
// symlink or wrong-type final leaf.
func GCPinPathForRead(repoRoot, pinFileName string) (string, error) {
	return pinPathForRead(repoRoot, []string{"gc", "pins"}, pinFileName)
}

// LegacyPinPathForRead returns an existing legacy pin path after rejecting a
// symlink or wrong-type final leaf.
func LegacyPinPathForRead(repoRoot, pinFileName string) (string, error) {
	return pinPathForRead(repoRoot, []string{"pins"}, pinFileName)
}

func pinPathForRead(repoRoot string, parentComponents []string, pinFileName string) (string, error) {
	if err := pathutil.ValidateName(pinFileName); err != nil {
		return "", err
	}
	path, err := controlFilePath(repoRoot, parentComponents, pinFileName)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPath returns the path for a GC plan ID after rejecting path-like names.
func GCPlanPath(repoRoot, planID string) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"gc"}, planID+".json")
}

// GCPlanPathForRead returns an existing GC plan path after rejecting a symlink
// or wrong-type final leaf.
func GCPlanPathForRead(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPathForWrite returns a GC plan path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCPlanPathForWrite(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPathForDelete returns a GC plan path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCPlanPathForDelete(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

func controlDirPath(repoRoot string, components ...string) (string, error) {
	if err := validateControlDirs(repoRoot, components...); err != nil {
		return "", err
	}
	return controlPath(repoRoot, components...), nil
}

func controlFilePath(repoRoot string, parentComponents []string, leaf string) (string, error) {
	if err := validateControlDirs(repoRoot, parentComponents...); err != nil {
		return "", err
	}
	parts := append([]string{repoRoot, JVSDirName}, parentComponents...)
	parts = append(parts, leaf)
	return filepath.Join(parts...), nil
}

func controlPath(repoRoot string, components ...string) string {
	parts := append([]string{repoRoot, JVSDirName}, components...)
	return filepath.Join(parts...)
}

type controlLeafKind int

const (
	controlLeafRegularFile controlLeafKind = iota
	controlLeafDirectory
)

func validateControlLeaf(path string, kind controlLeafKind, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return fmt.Errorf("stat control leaf %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("control leaf is symlink: %s", path)
	}

	switch kind {
	case controlLeafRegularFile:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("control leaf is not regular file: %s", path)
		}
	case controlLeafDirectory:
		if !info.IsDir() {
			return fmt.Errorf("control leaf is not directory: %s", path)
		}
	default:
		return fmt.Errorf("unknown control leaf kind for %s", path)
	}
	return nil
}

func validateControlDirs(repoRoot string, components ...string) error {
	current := filepath.Clean(repoRoot)
	if err := validateExistingRealDir(current); err != nil {
		return err
	}

	current = filepath.Join(current, JVSDirName)
	if err := validateExistingRealDir(current); err != nil {
		return err
	}

	for _, component := range components {
		current = filepath.Join(current, component)
		if err := validateExistingRealDir(current); err != nil {
			return err
		}
	}
	return nil
}

func validateExistingRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat control directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("control directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("control path is not directory: %s", path)
	}
	return nil
}

func readFormatVersion(jvsDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(jvsDir, FormatVersionFile))
	if err != nil {
		return 0, fmt.Errorf("read format_version: %w", err)
	}
	var version int
	if _, err := fmt.Sscanf(string(data), "%d", &version); err != nil {
		return 0, fmt.Errorf("parse format_version: %w", err)
	}
	return version, nil
}

func readRepoID(jvsDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(jvsDir, RepoIDFile))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
