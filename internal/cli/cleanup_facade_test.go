package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestDoctorRepairRuntimeInvalidatesCopiedCleanupPlanPublicly(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	_ = savePointForContract(t, "baseline")

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, previewOut)
	previewData := decodeContractDataMap(t, previewOut)
	planID, ok := previewData["plan_id"].(string)
	require.True(t, ok, "cleanup preview should expose plan_id: %#v", previewData)
	require.NotEmpty(t, planID)
	require.FileExists(t, filepath.Join(repoRoot, ".jvs", "gc", planID+".json"))

	doctorOut, err := executeCommand(createTestRootCmd(), "--json", "doctor", "--strict", "--repair-runtime")
	require.NoError(t, err, doctorOut)
	doctorData := decodeContractDataMap(t, doctorOut)
	repairs, ok := doctorData["repairs"].([]any)
	require.True(t, ok, "doctor repair-runtime should expose repairs: %#v", doctorData)
	repair := cleanupPlanRepairByAction(t, repairs, "clean_runtime_cleanup_plans")
	require.NotNil(t, repair)
	assert.Equal(t, true, repair["success"])
	assert.Equal(t, float64(1), repair["cleaned"])
	message, _ := repair["message"].(string)
	assert.Contains(t, message, "cleanup plan")
	assert.NotContains(t, message, ".jvs")
	assert.NotContains(t, message, ".jvs/gc")
	assert.NoFileExists(t, filepath.Join(repoRoot, ".jvs", "gc", planID+".json"))

	runOut, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--json", "cleanup", "run", "--plan-id", planID)
	require.Error(t, err)
	assert.Empty(t, stderr)
	env := decodeContractEnvelope(t, runOut)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "cleanup plan")
	assert.Contains(t, env.Error.Message, "not found")
	assert.NotContains(t, env.Error.Message, ".jvs")
}

func TestCleanupPreviewJSONActiveOperationScanFailureUsesPublicLanguage(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	blockCLIIntentDirectory(t, repoRoot)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--json", "cleanup", "preview")
	require.Error(t, err)
	assert.Empty(t, stderr)

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_CLEANUP_PLAN_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "active operations")
	assert.Contains(t, env.Error.Message, "doctor --strict")
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, env.Error.Message)
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, env.Error.Code)
}

func TestCleanupPreviewJSONDamagedReadyUsesPublicLanguage(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointForContract(t, "baseline")
	corruptCLIReadyMarker(t, repoRoot, savePointID)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--json", "cleanup", "preview")
	require.Error(t, err)
	assert.Empty(t, stderr)

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_CLEANUP_PLAN_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "save point storage")
	assert.Contains(t, env.Error.Message, "doctor --strict")
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, env.Error.Message)
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, env.Error.Code)
}

func TestCleanupRunHumanDamagedReadyUsesPublicLanguage(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointForContract(t, "baseline")

	previewOut, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, previewOut)
	previewData := decodeContractDataMap(t, previewOut)
	planID, ok := previewData["plan_id"].(string)
	require.True(t, ok, "cleanup preview should expose plan_id: %#v", previewData)
	require.NotEmpty(t, planID)

	corruptCLIReadyMarker(t, repoRoot, savePointID)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "cleanup", "run", "--plan-id", planID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "save point storage")
	assert.Contains(t, stderr, "doctor --strict")
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, stderr)
	assert.NotContains(t, stderr, "E_READY_INVALID")
}

func TestPublicCleanupRejectsUnknownProtectionReason(t *testing.T) {
	_, err := publicCleanup(&model.GCPlan{
		ProtectionGroups: []model.GCProtectionGroup{
			{Reason: "lineage", Count: 1},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cleanup protection reason")
	assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t, err.Error())
	assert.NotContains(t, err.Error(), "lineage")
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

func blockCLIIntentDirectory(t *testing.T, repoRoot string) {
	t.Helper()

	intentsDir := filepath.Join(repoRoot, ".jvs", "intents")
	require.NoError(t, os.RemoveAll(intentsDir))
	require.NoError(t, os.WriteFile(intentsDir, []byte("not a directory"), 0644))
}

func corruptCLIReadyMarker(t *testing.T, repoRoot, savePointID string) {
	t.Helper()

	readyPath := filepath.Join(repoRoot, ".jvs", "snapshots", savePointID, ".READY")
	require.NoError(t, os.WriteFile(readyPath, []byte("{not json"), 0644))
}

func assertPublicCleanupErrorOmitsInternalActiveOperationVocabulary(t *testing.T, value string) {
	t.Helper()

	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"checkpoint",
		"publish state",
		"ready marker",
		"intents",
		"intent",
		".jvs",
		"control path",
		"control directory",
		"stat ",
		"gc",
	} {
		assert.NotContains(t, lower, forbidden)
	}
}

func cleanupPlanRepairByAction(t *testing.T, repairs []any, action string) map[string]any {
	t.Helper()

	for _, raw := range repairs {
		repair, ok := raw.(map[string]any)
		require.True(t, ok, "repair should be an object: %#v", raw)
		if repair["action"] == action {
			return repair
		}
	}
	return nil
}
