package snapshot

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/pkg/model"
)

// ListAll returns all snapshot descriptors sorted by creation time (newest first).
func ListAll(repoRoot string) ([]*model.Descriptor, error) {
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

	var descriptors []*model.Descriptor
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
		if _, err := repo.SnapshotPathForRead(repoRoot, snapshotID); err != nil {
			return nil, err
		}
		desc, err := LoadDescriptor(repoRoot, snapshotID)
		if err != nil {
			// Skip corrupted/missing descriptors
			continue
		}
		descriptors = append(descriptors, desc)
	}

	// Sort by creation time (newest first)
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].CreatedAt.After(descriptors[j].CreatedAt)
	})

	return descriptors, nil
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
	all, err := ListAll(repoRoot)
	if err != nil {
		return nil, err
	}

	var matches []*model.Descriptor
	for _, desc := range all {
		if matchesQuery(desc, query) {
			matches = append(matches, desc)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no snapshot found matching %q", query)
	}
	if len(matches) > 1 {
		var ids []string
		for _, m := range matches {
			ids = append(ids, string(m.SnapshotID))
		}
		return nil, fmt.Errorf("ambiguous query %q matches multiple snapshots: %s", query, strings.Join(ids, ", "))
	}

	return matches[0], nil
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
