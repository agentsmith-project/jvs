package snapshot

import (
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// MatchScore represents how well a snapshot matches a query.
type MatchScore struct {
	Desc      *model.Descriptor
	Score     int
	MatchType string // "id", "tag", "note"
}

// FindMultiple finds snapshots matching a query with fuzzy scoring.
// Returns up to maxResults matches, sorted by relevance.
func FindMultiple(repoRoot string, query string, maxResults int) ([]*MatchScore, error) {
	if query == "HEAD" {
		return nil, fmt.Errorf("use explicit HEAD handling")
	}

	all, err := ListAll(repoRoot)
	if err != nil {
		return nil, err
	}

	var matches []*MatchScore
	queryLower := strings.ToLower(query)

	for _, desc := range all {
		score, matchType := scoreMatch(desc, query, queryLower)
		if score > 0 {
			matches = append(matches, &MatchScore{
				Desc:      desc,
				Score:     score,
				MatchType: matchType,
			})
		}
	}

	// Sort by score (descending)
	sortMatches(matches)

	// Limit results
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	return matches, nil
}

// scoreMatch calculates a relevance score for a snapshot against a query.
// Higher score = better match. Returns 0 for no match.
func scoreMatch(desc *model.Descriptor, query, queryLower string) (int, string) {
	idStr := string(desc.SnapshotID)
	noteLower := strings.ToLower(desc.Note)

	// Exact ID match (highest score)
	if idStr == query {
		return 1000, "id"
	}

	// ID prefix match (very high score)
	if strings.HasPrefix(idStr, query) {
		return 900, "id"
	}

	// Exact tag match
	for _, tag := range desc.Tags {
		if tag == query {
			return 800, "tag"
		}
		// Tag prefix match
		if strings.HasPrefix(tag, query) {
			return 700, "tag"
		}
	}

	// Exact note match
	if noteLower == queryLower {
		return 600, "note"
	}

	// Note prefix match
	if strings.HasPrefix(noteLower, queryLower) {
		return 500, "note"
	}

	// Substring match in note (lower score)
	if strings.Contains(noteLower, queryLower) {
		return 100, "note"
	}

	// Substring match in ID (lower score)
	if strings.Contains(idStr, query) {
		return 50, "id"
	}

	return 0, ""
}

// sortMatches sorts matches by score descending.
func sortMatches(matches []*MatchScore) {
	// Simple bubble sort - we have small lists
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Score > matches[i].Score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}

// FormatMatchList formats a list of matches for interactive display.
func FormatMatchList(matches []*MatchScore) string {
	var sb strings.Builder
	sb.WriteString(color.Header("Matching snapshots:\n"))
	sb.WriteString("\n")

	for i, m := range matches {
		prefix := "  "
		if i == 0 {
			prefix = color.Success("> ") // Indicate best match
		}

		note := m.Desc.Note
		if note == "" {
			note = color.Dim("(no note)")
		}

		tags := ""
		if len(m.Desc.Tags) > 0 {
			tagColors := make([]string, len(m.Desc.Tags))
			for i, tag := range m.Desc.Tags {
				tagColors[i] = color.Tag(tag)
			}
			tags = " [" + strings.Join(tagColors, ",") + "]"
		}

		sb.WriteString(fmt.Sprintf("%s%d. %s %s%s\n",
			prefix, i+1, color.SnapshotID(m.Desc.SnapshotID.ShortID()), note, tags))
		sb.WriteString(fmt.Sprintf("   %s by %s\n",
			color.Dim(m.Desc.CreatedAt.Format("2006-01-02 15:04")),
			color.Info(m.MatchType)))
	}

	return sb.String()
}
