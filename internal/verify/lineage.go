package verify

import (
	"errors"
	"fmt"
	"os"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/pkg/model"
)

const (
	ErrorCodeLineageParentMissing = "E_LINEAGE_PARENT_MISSING"
	ErrorCodeLineageParentInvalid = "E_LINEAGE_PARENT_INVALID"
	ErrorCodeLineageCycle         = "E_LINEAGE_CYCLE"
)

// LineageIssue describes a machine-readable lineage problem.
type LineageIssue struct {
	Code       string
	SnapshotID model.SnapshotID
	ParentID   model.SnapshotID
	Message    string
}

func (i *LineageIssue) Error() string {
	if i == nil {
		return ""
	}
	return i.Message
}

// CheckLineage validates parent links reachable from snapshotID.
func CheckLineage(repoRoot string, snapshotID model.SnapshotID) *LineageIssue {
	current := snapshotID
	seen := make(map[model.SnapshotID]bool)
	for current != "" {
		if seen[current] {
			return &LineageIssue{
				Code:       ErrorCodeLineageCycle,
				SnapshotID: current,
				Message:    fmt.Sprintf("lineage cycle detected at snapshot %s", current),
			}
		}
		seen[current] = true

		desc, err := snapshot.LoadDescriptor(repoRoot, current)
		if err != nil {
			return &LineageIssue{
				Code:       ErrorCodeLineageParentMissing,
				SnapshotID: current,
				Message:    fmt.Sprintf("lineage descriptor missing or unreadable for snapshot %s: %v", current, err),
			}
		}
		if desc.ParentID == nil {
			return nil
		}

		parentID := *desc.ParentID
		if err := parentID.Validate(); err != nil {
			return &LineageIssue{
				Code:       ErrorCodeLineageParentInvalid,
				SnapshotID: current,
				ParentID:   parentID,
				Message:    fmt.Sprintf("snapshot %s has invalid parent %s: %v", current, parentID, err),
			}
		}
		if _, err := repo.SnapshotDescriptorPathForRead(repoRoot, parentID); err != nil {
			code := ErrorCodeLineageParentInvalid
			if errors.Is(err, os.ErrNotExist) {
				code = ErrorCodeLineageParentMissing
			}
			return &LineageIssue{
				Code:       code,
				SnapshotID: current,
				ParentID:   parentID,
				Message:    fmt.Sprintf("snapshot %s parent %s is missing or unreadable: %v", current, parentID, err),
			}
		}
		current = parentID
	}
	return nil
}

// IsAncestor reports whether ancestorID is reachable from descendantID by
// following parent links. It returns a LineageIssue when traversal encounters
// missing parents, invalid parent IDs, or cycles.
func IsAncestor(repoRoot string, ancestorID, descendantID model.SnapshotID) (bool, error) {
	if ancestorID == "" || descendantID == "" {
		return false, nil
	}
	if err := ancestorID.Validate(); err != nil {
		return false, &LineageIssue{
			Code:       ErrorCodeLineageParentInvalid,
			SnapshotID: ancestorID,
			Message:    fmt.Sprintf("invalid ancestor snapshot ID %s: %v", ancestorID, err),
		}
	}
	if err := descendantID.Validate(); err != nil {
		return false, &LineageIssue{
			Code:       ErrorCodeLineageParentInvalid,
			SnapshotID: descendantID,
			Message:    fmt.Sprintf("invalid descendant snapshot ID %s: %v", descendantID, err),
		}
	}

	current := descendantID
	seen := make(map[model.SnapshotID]bool)
	for current != "" {
		if current == ancestorID {
			return true, nil
		}
		if seen[current] {
			return false, &LineageIssue{
				Code:       ErrorCodeLineageCycle,
				SnapshotID: current,
				Message:    fmt.Sprintf("lineage cycle detected at snapshot %s", current),
			}
		}
		seen[current] = true

		desc, err := snapshot.LoadDescriptor(repoRoot, current)
		if err != nil {
			return false, &LineageIssue{
				Code:       ErrorCodeLineageParentMissing,
				SnapshotID: current,
				Message:    fmt.Sprintf("lineage descriptor missing or unreadable for snapshot %s: %v", current, err),
			}
		}
		if desc.ParentID == nil {
			return false, nil
		}
		parentID := *desc.ParentID
		if err := parentID.Validate(); err != nil {
			return false, &LineageIssue{
				Code:       ErrorCodeLineageParentInvalid,
				SnapshotID: current,
				ParentID:   parentID,
				Message:    fmt.Sprintf("snapshot %s has invalid parent %s: %v", current, parentID, err),
			}
		}
		current = parentID
	}
	return false, nil
}
