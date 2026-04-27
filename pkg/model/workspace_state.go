package model

// WorkspaceState captures the workspace-local state needed by save point semantics.
type WorkspaceState struct {
	HistoryHead              *SnapshotID  `json:"history_head,omitempty"`
	ContentSource            *SnapshotID  `json:"content_source,omitempty"`
	StartedFrom              *SnapshotID  `json:"started_from,omitempty"`
	PathSources              PathSources  `json:"path_sources,omitempty"`
	DirtyPaths               PathDirtySet `json:"dirty_paths,omitempty"`
	HasUnknownUnsavedChanges bool         `json:"has_unknown_unsaved_changes,omitempty"`
	HasUnsavedChanges        bool         `json:"has_unsaved_changes"`
}

// WorkspaceSaveLineage is the creation-time lineage/provenance for the next save.
type WorkspaceSaveLineage struct {
	SnapshotID    SnapshotID           `json:"snapshot_id,omitempty"`
	ParentID      *SnapshotID          `json:"parent_id,omitempty"`
	StartedFrom   *SnapshotID          `json:"started_from,omitempty"`
	RestoredFrom  *SnapshotID          `json:"restored_from,omitempty"`
	RestoredPaths []RestoredPathSource `json:"restored_paths,omitempty"`
}

// WorkspaceStateAtSavePoint returns a clean workspace whose files match head.
func WorkspaceStateAtSavePoint(head SnapshotID) WorkspaceState {
	return WorkspaceState{
		HistoryHead:       snapshotIDPtr(head),
		ContentSource:     snapshotIDPtr(head),
		PathSources:       NewPathSources(),
		DirtyPaths:        NewPathDirtySet(),
		HasUnsavedChanges: false,
	}
}

// WorkspaceStateStartedFrom returns the initial state for workspace new --from.
func WorkspaceStateStartedFrom(source SnapshotID) WorkspaceState {
	return WorkspaceState{
		ContentSource:     snapshotIDPtr(source),
		StartedFrom:       snapshotIDPtr(source),
		PathSources:       NewPathSources(),
		DirtyPaths:        NewPathDirtySet(),
		HasUnsavedChanges: false,
	}
}

// RestoreWhole records a whole-workspace materialization without moving history.
func (s *WorkspaceState) RestoreWhole(source SnapshotID) {
	s.ContentSource = snapshotIDPtr(source)
	s.PathSources = NewPathSources()
	s.DirtyPaths = NewPathDirtySet()
	s.HasUnknownUnsavedChanges = false
	s.HasUnsavedChanges = false
}

// RestorePath records path-level restore provenance without moving history.
func (s *WorkspaceState) RestorePath(path string, source SnapshotID) error {
	if err := s.PathSources.Restore(path, source); err != nil {
		return err
	}
	s.carryImplicitUnknownDirty()
	if err := s.DirtyPaths.ClearPath(path); err != nil {
		return err
	}
	s.refreshUnsavedChanges()
	return nil
}

// MarkUnsaved records that managed files have changed since the known source state.
func (s *WorkspaceState) MarkUnsaved() {
	s.HasUnknownUnsavedChanges = true
	s.refreshUnsavedChanges()
}

// MarkPathModified records a path edit in the path-source provenance and marks the workspace dirty.
func (s *WorkspaceState) MarkPathModified(path string) error {
	if err := s.PathSources.MarkModified(path); err != nil {
		return err
	}
	if err := s.DirtyPaths.Mark(path); err != nil {
		return err
	}
	s.refreshUnsavedChanges()
	return nil
}

// MarkPathDeleted records a path deletion in the path-source provenance and marks the workspace dirty.
func (s *WorkspaceState) MarkPathDeleted(path string) error {
	if err := s.PathSources.MarkDeleted(path); err != nil {
		return err
	}
	if err := s.DirtyPaths.Mark(path); err != nil {
		return err
	}
	s.refreshUnsavedChanges()
	return nil
}

// Save computes creation-time lineage for newSave and moves the workspace to it.
func (s *WorkspaceState) Save(newSave SnapshotID) WorkspaceSaveLineage {
	lineage := s.NextSaveLineage(newSave)

	s.HistoryHead = snapshotIDPtr(newSave)
	s.ContentSource = snapshotIDPtr(newSave)
	s.PathSources = NewPathSources()
	s.DirtyPaths = NewPathDirtySet()
	s.HasUnknownUnsavedChanges = false
	s.HasUnsavedChanges = false

	return lineage
}

// NextSaveLineage computes creation-time lineage without mutating workspace state.
func (s WorkspaceState) NextSaveLineage(newSave SnapshotID) WorkspaceSaveLineage {
	lineage := WorkspaceSaveLineage{
		SnapshotID:    newSave,
		ParentID:      cloneSnapshotIDPtr(s.HistoryHead),
		RestoredPaths: s.PathSources.RestoredPaths(),
	}

	firstSaveStartedFrom := s.HistoryHead == nil && s.StartedFrom != nil
	if firstSaveStartedFrom {
		lineage.StartedFrom = cloneSnapshotIDPtr(s.StartedFrom)
	}

	if s.ContentSource != nil && !sameSnapshotIDPtr(s.ContentSource, s.HistoryHead) {
		if !(firstSaveStartedFrom && sameSnapshotIDPtr(s.ContentSource, s.StartedFrom)) {
			lineage.RestoredFrom = cloneSnapshotIDPtr(s.ContentSource)
		}
	}

	return lineage
}

// ApplyToDescriptor copies lineage/provenance into a snapshot descriptor.
func (l WorkspaceSaveLineage) ApplyToDescriptor(desc *Descriptor) {
	if desc == nil {
		return
	}
	desc.ParentID = cloneSnapshotIDPtr(l.ParentID)
	desc.StartedFrom = cloneSnapshotIDPtr(l.StartedFrom)
	desc.RestoredFrom = cloneSnapshotIDPtr(l.RestoredFrom)
	desc.RestoredPaths = cloneRestoredPathSources(l.RestoredPaths)
}

func snapshotIDPtr(id SnapshotID) *SnapshotID {
	idCopy := id
	return &idCopy
}

func cloneSnapshotIDPtr(id *SnapshotID) *SnapshotID {
	if id == nil {
		return nil
	}
	return snapshotIDPtr(*id)
}

func sameSnapshotIDPtr(left, right *SnapshotID) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func (s *WorkspaceState) carryImplicitUnknownDirty() {
	if s.HasUnsavedChanges && !s.HasUnknownUnsavedChanges && !s.DirtyPaths.HasDirty() {
		s.HasUnknownUnsavedChanges = true
	}
}

func (s *WorkspaceState) refreshUnsavedChanges() {
	s.HasUnsavedChanges = s.HasUnknownUnsavedChanges || s.DirtyPaths.HasDirty()
}
