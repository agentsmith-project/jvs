package uuidutil_test

import (
	"regexp"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/uuidutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewV4_Format(t *testing.T) {
	id := uuidutil.NewV4()
	require.Regexp(t, uuidPattern, id)
}

func TestNewV4_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := uuidutil.NewV4()
		assert.False(t, seen[id], "duplicate UUID: %s", id)
		seen[id] = true
	}
}
