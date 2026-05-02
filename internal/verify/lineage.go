package verify

import (
	"errors"
	"fmt"
	"os"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
)

const (
	ErrorCodeLineageParentMissing    = "E_LINEAGE_PARENT_MISSING"
	ErrorCodeLineageParentInvalid    = "E_LINEAGE_PARENT_INVALID"
	ErrorCodeLineageCycle            = "E_LINEAGE_CYCLE"
	ErrorCodeLineageReferenceMissing = "E_LINEAGE_REFERENCE_MISSING"
	ErrorCodeLineageReferenceInvalid = "E_LINEAGE_REFERENCE_INVALID"
)

// LineageIssue describes a machine-readable lineage problem.
type LineageIssue struct {
	Code           string
	SnapshotID     model.SnapshotID
	ParentID       model.SnapshotID
	ReferenceID    model.SnapshotID
	ReferenceField string
	Message        string
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
			if issue := checkDescriptorProvenanceReferences(repoRoot, current, desc); issue != nil {
				return issue
			}
			return nil
		}
		if issue := checkDescriptorProvenanceReferences(repoRoot, current, desc); issue != nil {
			return issue
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

type descriptorReference struct {
	field string
	id    model.SnapshotID
}

func checkDescriptorProvenanceReferences(repoRoot string, snapshotID model.SnapshotID, desc *model.Descriptor) *LineageIssue {
	for _, ref := range descriptorProvenanceReferences(desc) {
		if err := ref.id.Validate(); err != nil {
			return &LineageIssue{
				Code:           ErrorCodeLineageReferenceInvalid,
				SnapshotID:     snapshotID,
				ReferenceID:    ref.id,
				ReferenceField: ref.field,
				Message:        fmt.Sprintf("snapshot %s has invalid %s %s: %v", snapshotID, ref.field, ref.id, err),
			}
		}
		if _, err := repo.SnapshotDescriptorPathForRead(repoRoot, ref.id); err != nil {
			code := ErrorCodeLineageReferenceInvalid
			if errors.Is(err, os.ErrNotExist) {
				code = ErrorCodeLineageReferenceMissing
			}
			return &LineageIssue{
				Code:           code,
				SnapshotID:     snapshotID,
				ReferenceID:    ref.id,
				ReferenceField: ref.field,
				Message:        fmt.Sprintf("snapshot %s %s %s is missing or unreadable: %v", snapshotID, ref.field, ref.id, err),
			}
		}
	}
	return nil
}

func descriptorProvenanceReferences(desc *model.Descriptor) []descriptorReference {
	if desc == nil {
		return nil
	}
	var refs []descriptorReference
	if desc.StartedFrom != nil {
		refs = append(refs, descriptorReference{field: "started_from", id: *desc.StartedFrom})
	}
	if desc.RestoredFrom != nil {
		refs = append(refs, descriptorReference{field: "restored_from", id: *desc.RestoredFrom})
	}
	for i, restored := range desc.RestoredPaths {
		refs = append(refs, descriptorReference{
			field: fmt.Sprintf("restored_paths[%d].source_snapshot_id", i),
			id:    restored.SourceSnapshotID,
		})
	}
	return refs
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
