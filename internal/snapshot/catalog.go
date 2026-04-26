package snapshot

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// CatalogEntry is one published checkpoint payload plus its descriptor load
// result. Callers that resolve refs can distinguish a missing ref from a
// published checkpoint whose descriptor is missing or corrupt.
type CatalogEntry struct {
	SnapshotID    model.SnapshotID
	Descriptor    *model.Descriptor
	DescriptorErr error
}

// ListAll returns all snapshot descriptors sorted by creation time (newest first).
func ListAll(repoRoot string) ([]*model.Descriptor, error) {
	entries, err := ListCatalogEntries(repoRoot)
	if err != nil {
		return nil, err
	}

	var descriptors []*model.Descriptor
	for _, entry := range entries {
		if entry.DescriptorErr != nil {
			continue
		}
		descriptors = append(descriptors, entry.Descriptor)
	}

	// Sort by creation time (newest first)
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].CreatedAt.After(descriptors[j].CreatedAt)
	})

	return descriptors, nil
}

// ListCatalogEntries returns ready checkpoint payloads with descriptor load
// state, preserving corrupt descriptor candidates for ref resolution.
func ListCatalogEntries(repoRoot string) ([]CatalogEntry, error) {
	snapshotsDir, err := repo.SnapshotsDirPath(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve snapshots directory: %w", err)
	}
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshots directory: %w", err)
	}

	var catalogEntries []CatalogEntry
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		snapshotID := model.SnapshotID(entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			if err := snapshotID.Validate(); err == nil {
				return nil, fmt.Errorf("snapshot leaf is symlink: %s", entry.Name())
			}
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if err := snapshotID.Validate(); err != nil {
			continue
		}
		state, issue := InspectPublishState(repoRoot, snapshotID, PublishStateOptions{RequireReady: true})
		if issue != nil {
			switch issue.Code {
			case PublishStateCodeReadyMissing:
				continue
			case PublishStateCodeReadyInvalid:
				return nil, PublishStateIssueError(issue)
			case PublishStateCodeDescriptorCorrupt, PublishStateCodeDescriptorMissing, PublishStateCodeReadyDescriptorMissing:
				catalogEntries = append(catalogEntries, CatalogEntry{
					SnapshotID:    snapshotID,
					DescriptorErr: PublishStateIssueError(issue),
				})
				continue
			default:
				return nil, PublishStateIssueError(issue)
			}
		}
		catalogEntries = append(catalogEntries, CatalogEntry{
			SnapshotID:    snapshotID,
			Descriptor:    state.Descriptor,
			DescriptorErr: nil,
		})
	}

	return catalogEntries, nil
}

// PublishedSnapshotExists reports whether snapshotID has a ready published
// payload, independent of descriptor readability.
func PublishedSnapshotExists(repoRoot string, snapshotID model.SnapshotID) (bool, error) {
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return PublishReadyMarkerExists(snapshotDir)
}

// FilterOptions for searching snapshots.
type FilterOptions struct {
	WorktreeName string
	NoteContains string
	HasTag       string
	Since        time.Time
	Until        time.Time
}

// Find returns snapshots matching filter criteria.
func Find(repoRoot string, opts FilterOptions) ([]*model.Descriptor, error) {
	all, err := ListAll(repoRoot)
	if err != nil {
		return nil, err
	}

	var result []*model.Descriptor
	for _, desc := range all {
		if !matchesFilter(desc, opts) {
			continue
		}
		result = append(result, desc)
	}

	return result, nil
}

func matchesFilter(desc *model.Descriptor, opts FilterOptions) bool {
	if opts.WorktreeName != "" && desc.WorktreeName != opts.WorktreeName {
		return false
	}
	if opts.NoteContains != "" && !strings.Contains(desc.Note, opts.NoteContains) {
		return false
	}
	if opts.HasTag != "" && !hasTag(desc, opts.HasTag) {
		return false
	}
	if !opts.Since.IsZero() && desc.CreatedAt.Before(opts.Since) {
		return false
	}
	if !opts.Until.IsZero() && desc.CreatedAt.After(opts.Until) {
		return false
	}
	return true
}

func hasTag(desc *model.Descriptor, tag string) bool {
	for _, t := range desc.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// FindOne finds a single snapshot by fuzzy match (note/tag prefix).
// Returns error if multiple matches or no matches.
func FindOne(repoRoot string, query string) (*model.Descriptor, error) {
	entries, err := ListCatalogEntries(repoRoot)
	if err != nil {
		return nil, err
	}

	if id := model.SnapshotID(query); id.IsValid() {
		for _, entry := range entries {
			if entry.SnapshotID != id {
				continue
			}
			if entry.DescriptorErr != nil {
				return nil, entry.DescriptorErr
			}
			return entry.Descriptor, nil
		}
		return nil, fmt.Errorf("no snapshot found matching %q", query)
	}

	var matches []CatalogEntry
	var firstDescriptorErr *CatalogEntry
	for _, entry := range entries {
		if entry.DescriptorErr != nil {
			if firstDescriptorErr == nil {
				current := entry
				firstDescriptorErr = &current
			}
			if strings.HasPrefix(string(entry.SnapshotID), query) {
				matches = append(matches, entry)
			}
			continue
		}
		if matchesQuery(entry.Descriptor, query) {
			matches = append(matches, entry)
		}
	}

	if len(matches) > 1 {
		var ids []string
		for _, m := range matches {
			ids = append(ids, string(m.SnapshotID))
		}
		return nil, fmt.Errorf("ambiguous query %q matches multiple snapshots: %s", query, strings.Join(ids, ", "))
	}
	if firstDescriptorErr != nil {
		return nil, firstDescriptorErr.DescriptorErr
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no snapshot found matching %q", query)
	}

	if matches[0].DescriptorErr != nil {
		return nil, matches[0].DescriptorErr
	}
	return matches[0].Descriptor, nil
}

func matchesQuery(desc *model.Descriptor, query string) bool {
	// Check if query matches note prefix
	if strings.HasPrefix(desc.Note, query) {
		return true
	}
	// Check if query matches any tag
	for _, tag := range desc.Tags {
		if tag == query || strings.HasPrefix(tag, query) {
			return true
		}
	}
	// Check if query matches snapshot ID prefix
	if strings.HasPrefix(string(desc.SnapshotID), query) {
		return true
	}
	return false
}

// FindByTag returns the latest snapshot with the given tag.
func FindByTag(repoRoot string, tag string) (*model.Descriptor, error) {
	opts := FilterOptions{HasTag: tag}
	matches, err := Find(repoRoot, opts)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no snapshot found with tag %q", tag)
	}
	// ListAll returns newest first, so first match is latest
	return matches[0], nil
}
