// Package diff implements snapshot differencing for JVS.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/pkg/model"
)

// ChangeType represents the type of filesystem change.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// Change represents a single file/directory change between snapshots.
type Change struct {
	Path      string      `json:"path"`
	Type      ChangeType  `json:"type"`
	Mode      os.FileMode `json:"mode,omitempty"`
	Size      int64       `json:"size,omitempty"`
	OldSize   int64       `json:"old_size,omitempty"`
	OldHash   string      `json:"old_hash,omitempty"`
	NewHash   string      `json:"new_hash,omitempty"`
	IsSymlink bool        `json:"is_symlink,omitempty"`
}

// DiffResult represents the result of comparing two snapshots.
type DiffResult struct {
	FromSnapshotID model.SnapshotID `json:"from_snapshot_id"`
	ToSnapshotID   model.SnapshotID `json:"to_snapshot_id"`
	FromTime       time.Time        `json:"from_time"`
	ToTime         time.Time        `json:"to_time"`
	Added          []*Change        `json:"added"`
	Removed        []*Change        `json:"removed"`
	Modified       []*Change        `json:"modified"`
	TotalAdded     int              `json:"total_added"`
	TotalRemoved   int              `json:"total_removed"`
	TotalModified  int              `json:"total_modified"`
}

// Differ computes differences between snapshots.
type Differ struct {
	repoRoot string
}

// NewDiffer creates a new Differ.
func NewDiffer(repoRoot string) *Differ {
	return &Differ{repoRoot: repoRoot}
}

// Diff compares two snapshots and returns the differences.
// If fromID is empty, compares against an empty snapshot (shows all as added).
func (d *Differ) Diff(fromID, toID model.SnapshotID) (*DiffResult, error) {
	fromPath := ""
	var fromDesc *model.Descriptor
	if fromID != "" {
		fromState, err := d.inspectPublishedSnapshot(fromID)
		if err != nil {
			return nil, fmt.Errorf("inspect from publish state: %w", err)
		}
		fromPath = fromState.SnapshotDir
		fromDesc = fromState.Descriptor
	}

	toState, err := d.inspectPublishedSnapshot(toID)
	if err != nil {
		return nil, fmt.Errorf("inspect to publish state: %w", err)
	}
	toPath := toState.SnapshotDir
	toDesc := toState.Descriptor

	tempRoot, err := os.MkdirTemp("", "jvs-diff-*")
	if err != nil {
		return nil, fmt.Errorf("create diff temp dir: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	if fromPath != "" {
		fromPath, err = d.materializeSnapshot(fromPath, filepath.Join(tempRoot, "from"), fromDesc)
		if err != nil {
			return nil, fmt.Errorf("materialize from snapshot: %w", err)
		}
	}
	toPath, err = d.materializeSnapshot(toPath, filepath.Join(tempRoot, "to"), toDesc)
	if err != nil {
		return nil, fmt.Errorf("materialize to snapshot: %w", err)
	}

	// Build file trees for comparison
	fromTree := make(map[string]*fileInfo)
	toTree := make(map[string]*fileInfo)

	if fromPath != "" {
		if err := d.buildTree(fromPath, "", fromTree); err != nil {
			return nil, fmt.Errorf("build from tree: %w", err)
		}
	}
	if err := d.buildTree(toPath, "", toTree); err != nil {
		return nil, fmt.Errorf("build to tree: %w", err)
	}

	// Compute differences
	result := &DiffResult{
		FromSnapshotID: fromID,
		ToSnapshotID:   toID,
		ToTime:         toDesc.CreatedAt,
	}
	if fromDesc != nil {
		result.FromTime = fromDesc.CreatedAt
	}

	// Find added and modified files
	for path, toInfo := range toTree {
		fromInfo, exists := fromTree[path]
		if !exists {
			// File was added
			result.Added = append(result.Added, &Change{
				Path:      path,
				Type:      ChangeAdded,
				Mode:      toInfo.Mode,
				Size:      toInfo.Size,
				NewHash:   toInfo.Hash,
				IsSymlink: toInfo.IsSymlink,
			})
		} else if !fromInfo.equals(toInfo) {
			// File was modified
			result.Modified = append(result.Modified, &Change{
				Path:      path,
				Type:      ChangeModified,
				Mode:      toInfo.Mode,
				Size:      toInfo.Size,
				OldSize:   fromInfo.Size,
				OldHash:   fromInfo.Hash,
				NewHash:   toInfo.Hash,
				IsSymlink: toInfo.IsSymlink,
			})
		}
	}

	// Find removed files
	for path, fromInfo := range fromTree {
		if _, exists := toTree[path]; !exists {
			result.Removed = append(result.Removed, &Change{
				Path:      path,
				Type:      ChangeRemoved,
				Mode:      fromInfo.Mode,
				Size:      fromInfo.Size,
				OldHash:   fromInfo.Hash,
				IsSymlink: fromInfo.IsSymlink,
			})
		}
	}

	// Sort changes by path
	sortChanges(result.Added)
	sortChanges(result.Removed)
	sortChanges(result.Modified)

	result.TotalAdded = len(result.Added)
	result.TotalRemoved = len(result.Removed)
	result.TotalModified = len(result.Modified)

	return result, nil
}

func (d *Differ) inspectPublishedSnapshot(snapshotID model.SnapshotID) (*snapshot.PublishState, error) {
	state, issue := snapshot.InspectPublishState(d.repoRoot, snapshotID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return nil, snapshot.PublishStateIssueError(issue)
	}
	return state, nil
}

// fileInfo represents metadata about a file in a snapshot.
type fileInfo struct {
	Path      string
	Mode      os.FileMode
	Size      int64
	Hash      string
	IsSymlink bool
}

func (f *fileInfo) equals(other *fileInfo) bool {
	if f.IsSymlink != other.IsSymlink {
		return false
	}
	if f.Mode.Perm() != other.Mode.Perm() {
		return false
	}
	if f.IsSymlink {
		// For symlinks, we compare the hash which contains the target
		return f.Hash == other.Hash
	}
	// For regular files, compare hash (size is implied by hash)
	return f.Hash == other.Hash
}

func (d *Differ) materializeSnapshot(src, dst string, desc *model.Descriptor) (string, error) {
	if err := snapshotpayload.Materialize(src, dst, snapshotpayload.OptionsFromDescriptor(desc), cloneSnapshotTree); err != nil {
		return "", err
	}
	return dst, nil
}

func cloneSnapshotTree(src, dst string) error {
	_, err := engine.NewCopyEngine().Clone(src, dst)
	return err
}

// buildTree recursively builds a map of path -> fileInfo for a directory.
func (d *Differ) buildTree(root, relPath string, tree map[string]*fileInfo) error {
	fullPath := filepath.Join(root, relPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		entryPath := filepath.Join(relPath, name)
		fullEntryPath := filepath.Join(root, entryPath)

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Check if it's a symlink
		isSymlink := entry.Type()&os.ModeSymlink != 0

		var hash string
		var size int64

		switch {
		case isSymlink:
			target, err := os.Readlink(fullEntryPath)
			if err != nil {
				return err
			}
			hash = hashString(target)
			size = info.Size()
		case info.IsDir():
			if err := d.buildTree(root, entryPath, tree); err != nil {
				return err
			}
			continue
		case info.Mode().IsRegular():
			hash, err = d.hashFile(fullEntryPath)
			if err != nil {
				return err
			}
			size = info.Size()
		default:
			continue
		}

		tree[entryPath] = &fileInfo{
			Path:      entryPath,
			Mode:      info.Mode(),
			Size:      size,
			Hash:      hash,
			IsSymlink: isSymlink,
		}
	}

	return nil
}

// hashFile computes SHA-256 hash of a file.
func (d *Differ) hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashString hashes a string (for symlink targets).
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// sortChanges sorts changes by path.
func sortChanges(changes []*Change) {
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})
}

// FormatHuman returns a human-readable string representation of the diff.
func (r *DiffResult) FormatHuman() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Diff %s -> %s\n", r.FromSnapshotID.ShortID(), r.ToSnapshotID.ShortID()))
	if !r.FromTime.IsZero() {
		sb.WriteString(fmt.Sprintf("From: %s\n", r.FromTime.Format("2006-01-02 15:04:05")))
	}
	sb.WriteString(fmt.Sprintf("To:   %s\n", r.ToTime.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n")

	if r.TotalAdded > 0 {
		sb.WriteString(fmt.Sprintf("Added (%d):\n", r.TotalAdded))
		for _, c := range r.Added {
			sb.WriteString(fmt.Sprintf("  + %s\n", c.Path))
		}
		sb.WriteString("\n")
	}

	if r.TotalRemoved > 0 {
		sb.WriteString(fmt.Sprintf("Removed (%d):\n", r.TotalRemoved))
		for _, c := range r.Removed {
			sb.WriteString(fmt.Sprintf("  - %s\n", c.Path))
		}
		sb.WriteString("\n")
	}

	if r.TotalModified > 0 {
		sb.WriteString(fmt.Sprintf("Modified (%d):\n", r.TotalModified))
		for _, c := range r.Modified {
			sb.WriteString(fmt.Sprintf("  ~ %s", c.Path))
			if c.OldSize != c.Size {
				sb.WriteString(fmt.Sprintf(" (%d -> %d bytes)", c.OldSize, c.Size))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if r.TotalAdded == 0 && r.TotalRemoved == 0 && r.TotalModified == 0 {
		sb.WriteString("No changes.\n")
	}

	return sb.String()
}

// SetTimes sets the timestamp fields from descriptors.
func (r *DiffResult) SetTimes(fromTime, toTime time.Time) {
	r.FromTime = fromTime
	r.ToTime = toTime
}
