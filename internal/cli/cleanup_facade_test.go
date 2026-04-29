package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupPreviewHumanExplainsProtectedSavePointsByReason(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointForContract(t, "baseline")
	_, err := sourcepin.NewManager(repoRoot).CreateWithID(model.SnapshotID(savePointID), "view-"+savePointID, "active read-only view")
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "cleanup", "preview")
	require.NoError(t, err, stdout)

	assert.Contains(t, stdout, "Protected save points:")
	assert.Contains(t, stdout, "workspace history:")
	assert.Contains(t, stdout, "open views:")
	assert.NotContains(t, stdout, "open_view:")
	assert.NotContains(t, stdout, "active_recovery:")
	assert.Contains(t, stdout, savePointID)
	assert.Contains(t, stdout, "Run: jvs cleanup run --plan-id")
}

func TestCleanupPreviewJSONIncludesProtectionGroups(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointForContract(t, "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	groups, ok := data["protection_groups"].([]any)
	require.True(t, ok, "protection_groups should be a stable JSON array: %#v", data)
	require.NotEmpty(t, groups)
	history := cleanupProtectionGroupByReason(t, groups, "history")
	require.NotNil(t, history)
	assert.Equal(t, float64(1), history["count"])
	assert.Equal(t, []any{savePointID}, history["save_points"])
}

func cleanupProtectionGroupByReason(t *testing.T, groups []any, reason string) map[string]any {
	t.Helper()

	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		require.True(t, ok, "protection group should be an object: %#v", raw)
		if group["reason"] == reason {
			return group
		}
	}
	return nil
}
