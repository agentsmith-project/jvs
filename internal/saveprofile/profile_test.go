package saveprofile

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecorderProfileKeepsOnlyPublicPhasesAndAggregateCounts(t *testing.T) {
	recorder := New(model.EngineCopy)
	recorder.AddDuration("workspace_dirty_check", 2*time.Millisecond)
	recorder.AddDuration("/control-root-secret/.jvs/snapshots/private.tmp", 3*time.Millisecond)
	recorder.AddCounts("workspace_evidence_pre_hash", map[string]int64{
		"files":                              1,
		"bytes":                              7,
		"/payload-root-secret/customer.md":   1,
		"symlink:internal-target-secret.txt": 1,
		"TOPSECRET_PROFILE_PAYLOAD_CONTENT":  1,
	})
	recorder.AddCounts("/payload-root-secret/private-phase", map[string]int64{
		"files": 99,
	})

	profile := recorder.Profile(nil)

	require.Contains(t, profile.PhaseDurationsMS, "workspace_dirty_check")
	assert.NotContains(t, profile.PhaseDurationsMS, "/control-root-secret/.jvs/snapshots/private.tmp")
	counts := profile.PhaseCounts["workspace_evidence_pre_hash"]
	require.NotNil(t, counts)
	assert.Equal(t, int64(1), counts["files"])
	assert.Equal(t, int64(7), counts["bytes"])
	assert.NotContains(t, counts, "/payload-root-secret/customer.md")
	assert.NotContains(t, counts, "symlink:internal-target-secret.txt")
	assert.NotContains(t, counts, "TOPSECRET_PROFILE_PAYLOAD_CONTENT")
	assert.NotContains(t, profile.PhaseCounts, "/payload-root-secret/private-phase")

	payload, err := json.Marshal(profile)
	require.NoError(t, err)
	serialized := string(payload)
	assert.NotContains(t, serialized, "/control-root-secret")
	assert.NotContains(t, serialized, "/payload-root-secret")
	assert.NotContains(t, serialized, "customer.md")
	assert.NotContains(t, serialized, "internal-target-secret.txt")
	assert.NotContains(t, serialized, "TOPSECRET_PROFILE_PAYLOAD_CONTENT")
}
