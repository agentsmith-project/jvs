package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSaveWithCompressibleContent covers public save behavior with larger content.
func TestSaveWithCompressibleContent(t *testing.T) {
	setupCoverageRepo(t, "compressrepo")

	data := make([]byte, 50000)
	for i := range data {
		data[i] = byte(i % 10)
	}
	assert.NoError(t, os.WriteFile("compressible.dat", data, 0644))

	stdout, err := executeCommand(createTestRootCmd(), "save", "-m", "compressible content")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "save point")
}

// TestSaveJSONWithContentUsesSavePointSchema ensures the public JSON surface stays current.
func TestSaveJSONWithContentUsesSavePointSchema(t *testing.T) {
	setupCoverageRepo(t, "compressrepojson")
	assert.NoError(t, os.WriteFile("content.txt", []byte("content"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "content")
	assert.NoError(t, err)
	assert.Contains(t, stdout, "save_point_id")
	assert.NotContains(t, stdout, "snapshot_id")
}
