package cli

import (
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
)

// suggestSnapshots provides helpful suggestions when a checkpoint is not found.
// Returns a formatted suggestion string.
func suggestSnapshots(query string, repoRoot string) string {
	// Try to find close matches
	matches, err := snapshot.FindMultiple(repoRoot, query, 3)
	if err == nil && len(matches) > 0 {
		// Build "Did you mean?" message with best matches
		var suggestions []string
		for i, m := range matches {
			if i >= 3 {
				break
			}
			suggestion := color.SnapshotID(m.Desc.SnapshotID.ShortID())
			if m.Desc.Note != "" {
				suggestion += fmt.Sprintf(" (%s)", color.Dim(m.Desc.Note))
			}
			suggestions = append(suggestions, suggestion)
		}

		hint := "Did you mean"
		if len(suggestions) > 1 {
			hint += " one of"
		}
		return fmt.Sprintf("%s: %s?", hint, strings.Join(suggestions, ", "))
	}

	// No close matches - suggest listing checkpoints.
	return fmt.Sprintf("Run %s to see available checkpoints.", color.Code("jvs checkpoint list"))
}

// suggestWorktrees provides helpful suggestions when a workspace is not found.
// Returns a formatted suggestion string.
func suggestWorktrees(name string, repoRoot string) string {
	mgr := worktree.NewManager(repoRoot)
	list, err := mgr.List()
	if err != nil {
		return fmt.Sprintf("Run %s to see available workspaces.", color.Code("jvs workspace list"))
	}

	if len(list) == 0 {
		return "No workspaces exist yet."
	}

	// Try to find close matches by name
	var matches []string
	for _, cfg := range list {
		if strings.HasPrefix(strings.ToLower(cfg.Name), strings.ToLower(name)) {
			matches = append(matches, color.Success(cfg.Name))
		}
	}

	// If no prefix matches, try substring
	if len(matches) == 0 {
		for _, cfg := range list {
			if strings.Contains(strings.ToLower(cfg.Name), strings.ToLower(name)) {
				matches = append(matches, color.Success(cfg.Name))
			}
		}
	}

	if len(matches) > 0 {
		hint := "Did you mean"
		if len(matches) > 1 {
			hint += " one of"
		}
		return fmt.Sprintf("%s: %s?", hint, strings.Join(matches, ", "))
	}

	// List all available workspaces.
	var names []string
	for _, cfg := range list {
		names = append(names, color.Success(cfg.Name))
	}
	return fmt.Sprintf("Available workspaces: %s", strings.Join(names, ", "))
}

// suggestInit provides a suggestion to initialize a repository.
func suggestInit() string {
	return fmt.Sprintf("Run %s to create a new repository.", color.Code("jvs init <name>"))
}

// formatSnapshotNotFoundError formats a checkpoint not found error with suggestions.
func formatSnapshotNotFoundError(query string, repoRoot string) string {
	var sb strings.Builder

	sb.WriteString(color.Error(fmt.Sprintf("checkpoint '%s' not found", query)))
	sb.WriteString("\n")

	// Add suggestions
	suggestion := suggestSnapshots(query, repoRoot)
	sb.WriteString(color.Dim("  " + suggestion))

	return sb.String()
}

// formatWorktreeNotFoundError formats a workspace not found error with suggestions.
func formatWorktreeNotFoundError(name string, repoRoot string) string {
	var sb strings.Builder

	sb.WriteString(color.Error(fmt.Sprintf("workspace '%s' not found", name)))
	sb.WriteString("\n")

	// Add suggestions
	suggestion := suggestWorktrees(name, repoRoot)
	sb.WriteString(color.Dim("  " + suggestion))

	return sb.String()
}

// formatNotInRepositoryError formats an error when not in a JVS repository.
func formatNotInRepositoryError() string {
	var sb strings.Builder

	sb.WriteString(color.Error("not a JVS repository (or any parent)"))
	sb.WriteString("\n")
	sb.WriteString(color.Dim("  " + suggestInit()))

	return sb.String()
}
