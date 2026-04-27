package model

import (
	"fmt"
	pathpkg "path"
	"sort"
	"strings"
)

// PathSourceStatus describes whether a restored path still exactly matches its source.
type PathSourceStatus string

const (
	PathSourceExact                PathSourceStatus = "exact"
	PathSourceModifiedAfterRestore PathSourceStatus = "modified_after_restore"
	PathSourceUnresolved           PathSourceStatus = "unresolved"
)

// PathSourceEvidence stores optional evidence captured for a path source.
type PathSourceEvidence struct {
	Hash HashValue `json:"hash,omitempty"`
}

// PathSource records where a workspace path was most recently restored from.
type PathSource struct {
	SourceSnapshotID SnapshotID          `json:"source_snapshot_id"`
	SourcePath       string              `json:"source_path"`
	Status           PathSourceStatus    `json:"status"`
	Evidence         *PathSourceEvidence `json:"evidence,omitempty"`
}

// RestoredPathSource is the save-point provenance form of a path source.
type RestoredPathSource struct {
	TargetPath       string              `json:"target_path"`
	SourceSnapshotID SnapshotID          `json:"source_snapshot_id"`
	SourcePath       string              `json:"source_path"`
	Status           PathSourceStatus    `json:"status"`
	Evidence         *PathSourceEvidence `json:"evidence,omitempty"`
}

// PathSources maps normalized workspace-relative target paths to their sources.
type PathSources map[string]PathSource

// NewPathSources returns an empty path source set.
func NewPathSources() PathSources {
	return make(PathSources)
}

// NormalizeWorkspaceRelativePathKey normalizes a workspace-relative metadata key.
//
// This is lexical normalization only. It does not resolve a workspace root,
// symlinks, filesystem aliases, or control paths such as .jvs. Code that reads,
// writes, restores, or materializes files must perform a separate root-aware
// canonical containment check before touching the filesystem.
func NormalizeWorkspaceRelativePathKey(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("workspace path must not be empty")
	}
	if strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("workspace path %q contains NUL", raw)
	}

	slashPath := strings.ReplaceAll(raw, `\`, "/")
	if isWindowsVolumePath(slashPath) {
		return "", fmt.Errorf("workspace path %q must be relative and volume-free", raw)
	}
	if pathpkg.IsAbs(slashPath) {
		return "", fmt.Errorf("workspace path %q must be relative", raw)
	}

	clean := pathpkg.Clean(slashPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("workspace path %q escapes workspace root", raw)
	}
	return clean, nil
}

// NormalizeWorkspacePath normalizes a workspace-relative metadata key.
//
// Deprecated: use NormalizeWorkspaceRelativePathKey. This function is not a
// filesystem safety validator and does not perform root-aware containment checks.
func NormalizeWorkspacePath(raw string) (string, error) {
	return NormalizeWorkspaceRelativePathKey(raw)
}

// Restore records that targetPath was restored from the same path in source.
func (ps *PathSources) Restore(targetPath string, source SnapshotID) error {
	return ps.RestoreFromPath(targetPath, source, targetPath)
}

// RestoreFromPath records that targetPath was restored from sourcePath in source.
func (ps *PathSources) RestoreFromPath(targetPath string, source SnapshotID, sourcePath string) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("source snapshot ID: %w", err)
	}

	target, err := NormalizeWorkspaceRelativePathKey(targetPath)
	if err != nil {
		return err
	}
	sourcePath = defaultString(sourcePath, targetPath)
	normalizedSource, err := NormalizeWorkspaceRelativePathKey(sourcePath)
	if err != nil {
		return err
	}

	if *ps == nil {
		*ps = NewPathSources()
	}
	for existing := range *ps {
		if isDescendantPath(target, existing) {
			delete(*ps, existing)
		}
	}
	(*ps)[target] = PathSource{
		SourceSnapshotID: source,
		SourcePath:       normalizedSource,
		Status:           PathSourceExact,
	}
	return nil
}

// SourceForPath returns the most-specific source entry that applies to path.
func (ps PathSources) SourceForPath(rawPath string) (RestoredPathSource, bool, error) {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return RestoredPathSource{}, false, err
	}

	bestTarget := ""
	var best PathSource
	for existing, source := range ps {
		if existing == target || isDescendantPath(existing, target) {
			if len(existing) > len(bestTarget) {
				bestTarget = existing
				best = source
			}
		}
	}
	if bestTarget == "" {
		return RestoredPathSource{}, false, nil
	}
	return restoredPathSource(bestTarget, best), true, nil
}

// MarkModified downgrades matching path source provenance after an edit.
func (ps *PathSources) MarkModified(rawPath string) error {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return err
	}
	if ps == nil || *ps == nil {
		return nil
	}

	var matching []string
	for existing := range *ps {
		if existing == target || isDescendantPath(existing, target) {
			matching = appendMostSpecificPath(matching, existing)
		}
	}
	if len(matching) == 0 {
		for existing := range *ps {
			if isDescendantPath(target, existing) {
				matching = append(matching, existing)
			}
		}
	}

	for _, existing := range matching {
		source := (*ps)[existing]
		source.Status = PathSourceModifiedAfterRestore
		(*ps)[existing] = source
	}
	return nil
}

// MarkDeleted clears deleted restored paths and downgrades containing restored directories.
func (ps *PathSources) MarkDeleted(rawPath string) error {
	target, err := NormalizeWorkspaceRelativePathKey(rawPath)
	if err != nil {
		return err
	}
	if ps == nil || *ps == nil {
		return nil
	}

	for existing, source := range *ps {
		switch {
		case existing == target || isDescendantPath(target, existing):
			delete(*ps, existing)
		case isDescendantPath(existing, target):
			source.Status = PathSourceModifiedAfterRestore
			(*ps)[existing] = source
		}
	}
	return nil
}

// RestoredPaths returns a stable save-point provenance copy sorted by target path.
func (ps PathSources) RestoredPaths() []RestoredPathSource {
	if len(ps) == 0 {
		return nil
	}

	targets := make([]string, 0, len(ps))
	for target := range ps {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	restored := make([]RestoredPathSource, 0, len(targets))
	for _, target := range targets {
		restored = append(restored, restoredPathSource(target, ps[target]))
	}
	return restored
}

// Clone returns an independent copy of path sources.
func (ps PathSources) Clone() PathSources {
	if len(ps) == 0 {
		return NewPathSources()
	}
	clone := make(PathSources, len(ps))
	for target, source := range ps {
		source.Evidence = clonePathSourceEvidence(source.Evidence)
		clone[target] = source
	}
	return clone
}

func restoredPathSource(target string, source PathSource) RestoredPathSource {
	return RestoredPathSource{
		TargetPath:       target,
		SourceSnapshotID: source.SourceSnapshotID,
		SourcePath:       source.SourcePath,
		Status:           source.Status,
		Evidence:         clonePathSourceEvidence(source.Evidence),
	}
}

func cloneRestoredPathSources(sources []RestoredPathSource) []RestoredPathSource {
	if len(sources) == 0 {
		return nil
	}
	clone := make([]RestoredPathSource, len(sources))
	for i, source := range sources {
		clone[i] = source
		clone[i].Evidence = clonePathSourceEvidence(source.Evidence)
	}
	return clone
}

func clonePathSourceEvidence(evidence *PathSourceEvidence) *PathSourceEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	return &clone
}

func isDescendantPath(parent, child string) bool {
	return child != parent && strings.HasPrefix(child, parent+"/")
}

func appendMostSpecificPath(paths []string, candidate string) []string {
	for i := 0; i < len(paths); i++ {
		existing := paths[i]
		if existing == candidate {
			return paths
		}
		if isDescendantPath(existing, candidate) {
			paths = append(paths[:i], paths[i+1:]...)
			i--
			continue
		}
		if isDescendantPath(candidate, existing) {
			return paths
		}
	}
	return append(paths, candidate)
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func isWindowsVolumePath(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	first := path[0]
	return ('A' <= first && first <= 'Z') || ('a' <= first && first <= 'z')
}
