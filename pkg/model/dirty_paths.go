package model

// PathDirtyState marks whether a workspace-relative key is dirty or a clean
// exception under a broader dirty marker.
type PathDirtyState string

const (
	PathDirty PathDirtyState = "dirty"
	PathClean PathDirtyState = "clean"
)

// PathDirtySet tracks known workspace-relative dirty keys and clean exceptions.
type PathDirtySet map[string]PathDirtyState

// NewPathDirtySet returns an empty path dirty set.
func NewPathDirtySet() PathDirtySet {
	return make(PathDirtySet)
}

// Mark records path as dirty, coalescing redundant descendant markers.
func (ds *PathDirtySet) Mark(rawPath string) error {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return err
	}
	if *ds == nil {
		*ds = NewPathDirtySet()
	}

	for existing := range *ds {
		if isDescendantPath(target, existing) {
			delete(*ds, existing)
		}
	}
	if state, ok := ds.mostSpecificState(target); ok && state == PathDirty {
		return nil
	}
	(*ds)[target] = PathDirty
	return nil
}

// ClearPath clears dirty markers known to be inside path.
func (ds *PathDirtySet) ClearPath(rawPath string) error {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return err
	}
	if ds == nil || *ds == nil {
		return nil
	}

	hasDirtyAncestor := false
	for existing, state := range *ds {
		if state == PathDirty && isDescendantPath(existing, target) {
			hasDirtyAncestor = true
			break
		}
	}

	for existing := range *ds {
		if existing == target || isDescendantPath(target, existing) {
			delete(*ds, existing)
		}
	}
	if hasDirtyAncestor {
		(*ds)[target] = PathClean
	}
	return nil
}

// Contains reports whether path is covered by a known dirty marker.
func (ds PathDirtySet) Contains(rawPath string) bool {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return false
	}
	state, ok := ds.mostSpecificState(target)
	return ok && state == PathDirty
}

// HasDirty reports whether the set still contains any dirty marker.
func (ds PathDirtySet) HasDirty() bool {
	for _, state := range ds {
		if state == PathDirty {
			return true
		}
	}
	return false
}

// Clone returns an independent dirty set copy.
func (ds PathDirtySet) Clone() PathDirtySet {
	if len(ds) == 0 {
		return NewPathDirtySet()
	}
	clone := make(PathDirtySet, len(ds))
	for path, state := range ds {
		clone[path] = state
	}
	return clone
}

func (ds PathDirtySet) mostSpecificState(target string) (PathDirtyState, bool) {
	bestPath := ""
	var bestState PathDirtyState
	for existing, state := range ds {
		if existing == target || isDescendantPath(existing, target) {
			if len(existing) > len(bestPath) {
				bestPath = existing
				bestState = state
			}
		}
	}
	return bestState, bestPath != ""
}
