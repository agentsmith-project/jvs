package uuidutil_test

import (
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/uuidutil"
	"github.com/stretchr/testify/assert"
)

func TestNewV4_Length(t *testing.T) {
	id := uuidutil.NewV4()
	// UUID v4 standard format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx (36 chars)
	assert.Equal(t, 36, len(id), "UUID should be 36 characters long")
}

func TestNewV4_Segments(t *testing.T) {
	id := uuidutil.NewV4()
	parts := strings.Split(id, "-")
	assert.Equal(t, 5, len(parts), "UUID should have 5 segments separated by dashes")

	// Check segment lengths
	assert.Equal(t, 8, len(parts[0]), "First segment should be 8 chars")
	assert.Equal(t, 4, len(parts[1]), "Second segment should be 4 chars")
	assert.Equal(t, 4, len(parts[2]), "Third segment should be 4 chars")
	assert.Equal(t, 4, len(parts[3]), "Fourth segment should be 4 chars")
	assert.Equal(t, 12, len(parts[4]), "Fifth segment should be 12 chars")
}

func TestNewV4_Version(t *testing.T) {
	// UUID v4 should have '4' in the version position (14th character, index 14 after dashes)
	id := uuidutil.NewV4()
	// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// The version character is at position 14 (after removing dashes it's index 12)
	versionChar := id[14:15]
	assert.Equal(t, "4", versionChar, "UUID v4 should have '4' as version character")
}

func TestNewV4_Variant(t *testing.T) {
	// Variant bits should be 10xx, meaning char 19 should be 8, 9, a, or b
	id := uuidutil.NewV4()
	// Format: xxxxxxxx-xxxx-xxxx-yxxx-xxxxxxxxxxxx
	// The variant character is at position 19
	variantChar := id[19:20]
	assert.Contains(t, "89ab", variantChar, "UUID v4 variant should be one of 8, 9, a, b")
}

func TestNewV4_HexCharacters(t *testing.T) {
	id := uuidutil.NewV4()
	// Remove dashes and check all remaining chars are hex
	cleanID := strings.ReplaceAll(id, "-", "")
	for _, c := range cleanID {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"UUID should only contain hexadecimal characters, found: %c", c)
	}
}

func TestNewV4_DashPositions(t *testing.T) {
	id := uuidutil.NewV4()
	// Standard UUID format has dashes at positions 8, 13, 18, 23
	assert.Equal(t, '-', rune(id[8]), "Dash should be at position 8")
	assert.Equal(t, '-', rune(id[13]), "Dash should be at position 13")
	assert.Equal(t, '-', rune(id[18]), "Dash should be at position 18")
	assert.Equal(t, '-', rune(id[23]), "Dash should be at position 23")
}

func TestNewV4_Lowercase(t *testing.T) {
	id := uuidutil.NewV4()
	// Our implementation uses %x which produces lowercase
	originalID := id
	lowerID := strings.ToLower(id)
	assert.Equal(t, originalID, lowerID, "UUID should be in lowercase")
}

func TestNewV4_MultipleCalls(t *testing.T) {
	// Generate multiple UUIDs and verify they all have valid format
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		ids[i] = uuidutil.NewV4()
	}

	// All should be unique
	uniqueIDs := make(map[string]bool)
	for _, id := range ids {
		assert.False(t, uniqueIDs[id], "Found duplicate UUID: %s", id)
		uniqueIDs[id] = true
	}

	// All should have correct format
	for _, id := range ids {
		assert.Equal(t, 36, len(id), "UUID should be 36 characters")
		assert.Equal(t, "4", id[14:15], "Should be version 4")
	}
}

func TestNewV4_NonEmpty(t *testing.T) {
	id := uuidutil.NewV4()
	assert.NotEmpty(t, id, "UUID should not be empty")
	assert.NotEqual(t, "00000000-0000-4000-8000-000000000000", id,
		"UUID should not be all zeros (extremely unlikely)")
}

func TestNewV4_Prefix(t *testing.T) {
	// Test slicing behavior used in the codebase
	id := uuidutil.NewV4()
	prefix := id[:8]
	assert.Equal(t, 8, len(prefix), "First 8 characters should be extractable")
	assert.Regexp(t, "^[0-9a-f]{8}$", prefix, "Prefix should be 8 hex chars")
}

func TestNewV4_StringImmutable(t *testing.T) {
	// Verify that each call returns a new string
	id1 := uuidutil.NewV4()
	id2 := uuidutil.NewV4()
	assert.NotEqual(t, id1, id2, "Each call should generate a new UUID")
}

func TestNewV4_NoWhitespace(t *testing.T) {
	id := uuidutil.NewV4()
	assert.False(t, strings.ContainsAny(id, " \t\n\r"),
		"UUID should not contain whitespace")
}

func TestNewV4_ConsistentFormat(t *testing.T) {
	// Generate many UUIDs and verify consistent formatting
	ids := make([]string, 50)
	for i := 0; i < 50; i++ {
		ids[i] = uuidutil.NewV4()
	}

	// All should have same length
	for _, id := range ids {
		assert.Equal(t, 36, len(id))
	}

	// All should have dashes at same positions
	for _, id := range ids {
		assert.Equal(t, '-', rune(id[8]))
		assert.Equal(t, '-', rune(id[13]))
		assert.Equal(t, '-', rune(id[18]))
		assert.Equal(t, '-', rune(id[23]))
	}
}
