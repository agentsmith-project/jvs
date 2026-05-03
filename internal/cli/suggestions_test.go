package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSuggestInit tests the suggestInit function.
func TestSuggestInit(t *testing.T) {
	result := suggestInit()
	assert.Contains(t, result, "JVS workspace")
	assert.Contains(t, result, "--control-root <control-root>")
	assert.Contains(t, result, "--workspace main")
	assert.NotContains(t, result, "jvs init")
}

// TestFormatNotInRepositoryError tests the formatNotInRepositoryError function.
func TestFormatNotInRepositoryError(t *testing.T) {
	result := formatNotInRepositoryError()
	assert.Contains(t, result, "not a JVS repository")
	assert.Contains(t, result, "--control-root <control-root>")
	assert.Contains(t, result, "--workspace main")
	assert.NotContains(t, result, "jvs init")
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
