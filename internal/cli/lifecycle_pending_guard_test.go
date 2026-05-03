package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingLifecycleGuardFailsClosedForOrdinaryRepoCommands(t *testing.T) {
	isolateContractCLIState(t)
	restoreCapacity := installCapacityGateHooks(capacitygate.Gate{Meter: &cliFakeCapacityMeter{available: 1 << 40}, SafetyMarginBytes: 0})
	t.Cleanup(restoreCapacity)
	repoRoot := setupCurrentContractRepo(t)
	t.Cleanup(func() {
		_ = filepath.Walk(filepath.Join(repoRoot, ".jvs", "views"), func(path string, _ os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0700)
			}
			return nil
		})
	})
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointForContract(t, "baseline")
	record := writePendingLifecycleForCLIGuardTest(t, repoRoot)
	savePointsBefore := countCLIGuardSavePoints(t, repoRoot)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status"}},
		{name: "history", args: []string{"history"}},
		{name: "save", args: []string{"save", "-m", "must not save"}},
		{name: "restore", args: []string{"restore", savePointID}},
		{name: "view", args: []string{"view", savePointID}},
		{name: "workspace list", args: []string{"workspace", "list"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), append([]string{"--json"}, tc.args...)...)
			require.Error(t, err, stdout)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			require.NotNil(t, env.Error)
			assert.Equal(t, errclassLifecyclePendingCodeForCLIGuardTest, env.Error.Code)
			assert.Contains(t, env.Error.Message, record.OperationID)
			assert.Equal(t, record.RecommendedNextCommand, env.Error.RecommendedNextCommand)
			assert.Contains(t, env.Error.Hint, record.RecommendedNextCommand)
		})
	}

	assert.Equal(t, savePointsBefore, countCLIGuardSavePoints(t, repoRoot), "blocked save must not create a save point")
	assert.NoDirExists(t, filepath.Join(repoRoot, ".jvs", "views"), "blocked view must not create a read-only view")
}

func writePendingLifecycleForCLIGuardTest(t *testing.T, repoRoot string) lifecycle.OperationRecord {
	t.Helper()

	repoID, err := workspaceCurrentRepoID(repoRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	record := lifecycle.OperationRecord{
		SchemaVersion:          lifecycle.SchemaVersion,
		OperationID:            "op-cli-pending",
		OperationType:          "workspace move",
		RepoID:                 repoID,
		Phase:                  "workspace_moved",
		RecommendedNextCommand: "jvs workspace move --run plan-cli-pending",
		CreatedAt:              now,
		UpdatedAt:              now,
		Metadata: map[string]any{
			"plan_id":   "plan-cli-pending",
			"workspace": "feature",
		},
	}
	require.NoError(t, lifecycle.WriteOperation(repoRoot, record))
	return record
}

func countCLIGuardSavePoints(t *testing.T, repoRoot string) int {
	t.Helper()

	entries, err := snapshot.ListCatalogEntries(repoRoot)
	require.NoError(t, err)
	return len(entries)
}

const errclassLifecyclePendingCodeForCLIGuardTest = "E_LIFECYCLE_PENDING"
