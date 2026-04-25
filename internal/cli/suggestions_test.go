package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSuggestInit tests the suggestInit function.
func TestSuggestInit(t *testing.T) {
	result := suggestInit()
	assert.Contains(t, result, "jvs init")
	assert.Contains(t, result, "create a new repository")
}

// TestFormatNotInRepositoryError tests the formatNotInRepositoryError function.
func TestFormatNotInRepositoryError(t *testing.T) {
	result := formatNotInRepositoryError()
	assert.Contains(t, result, "not a JVS repository")
	assert.Contains(t, result, "jvs init")
}

// TestSuggestWorktrees tests the suggestWorktrees function with various scenarios.
func TestSuggestWorktrees(t *testing.T) {
	t.Run("No worktrees exist", func(t *testing.T) {
		// This test requires a real repo setup, so we'll test the function structure
		// by checking that it returns a non-empty string for invalid input
		result := suggestWorktrees("test", "/invalid/repo/root")
		assert.NotEmpty(t, result)
	})

	t.Run("Suggestion contains helpful info", func(t *testing.T) {
		result := suggestWorktrees("feat", "/invalid/repo/root")
		// Should suggest running list command when repo is invalid
		assert.Contains(t, result, "workspace")
	})
}

// TestSuggestSnapshots tests the suggestSnapshots function with various scenarios.
func TestSuggestSnapshots(t *testing.T) {
	t.Run("No snapshots available (invalid repo)", func(t *testing.T) {
		result := suggestSnapshots("abc123", "/invalid/repo/root")
		assert.NotEmpty(t, result)
		assert.Contains(t, result, "checkpoint list")
	})
}

// TestFormatSnapshotNotFoundError tests the formatSnapshotNotFoundError function.
func TestFormatSnapshotNotFoundError(t *testing.T) {
	t.Run("Error formatting contains key elements", func(t *testing.T) {
		result := formatSnapshotNotFoundError("abc123", "/invalid/repo/root")
		assert.Contains(t, result, "abc123")
		assert.Contains(t, result, "not found")
		// Should contain suggestion even for invalid repo
		assert.True(t, strings.Contains(result, "checkpoint list") || strings.Contains(result, "Did you mean"))
	})
}

// TestFormatWorktreeNotFoundError tests the formatWorktreeNotFoundError function.
func TestFormatWorktreeNotFoundError(t *testing.T) {
	t.Run("Error formatting contains key elements", func(t *testing.T) {
		result := formatWorktreeNotFoundError("feature-x", "/invalid/repo/root")
		assert.Contains(t, result, "feature-x")
		assert.Contains(t, result, "not found")
		assert.NotEmpty(t, result)
	})
}

// TestResolveSnapshotID_NotFound tests the resolveSnapshotID function with not found case.
func TestResolveSnapshotID_NotFound(t *testing.T) {
	t.Run("Returns error for invalid repo", func(t *testing.T) {
		_, err := resolveSnapshotID("/invalid/repo/root", "abc123")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestFormatSnapshotNotFoundError_ColorCodes tests that color codes are properly applied.
func TestFormatSnapshotNotFoundError_ColorCodes(t *testing.T) {
	result := formatSnapshotNotFoundError("test-id", "/invalid/repo/root")

	// When colors might be enabled, check that function returns structured output
	lines := strings.Split(result, "\n")
	assert.GreaterOrEqual(t, len(lines), 1)
	assert.Contains(t, lines[0], "test-id")
}

// TestSuggestWorktrees_InvalidRepo tests suggestWorktrees with invalid repository.
func TestSuggestWorktrees_InvalidRepo(t *testing.T) {
	result := suggestWorktrees("test", "/invalid/nonexistent/repo")
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "workspace list")
}

// TestSuggestSnapshots_InvalidRepo tests suggestSnapshots with invalid repository.
func TestSuggestSnapshots_InvalidRepo(t *testing.T) {
	result := suggestSnapshots("abc123", "/invalid/nonexistent/repo")
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "checkpoint list")
}

// Benchmark tests
func BenchmarkSuggestInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = suggestInit()
	}
}

func BenchmarkFormatNotInRepositoryError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = formatNotInRepositoryError()
	}
}
