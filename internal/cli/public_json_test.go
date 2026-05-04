package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clidoctor "github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicErrorCodeVocabularyDistinguishesWorkspaceAndSavePointPayloadCodes(t *testing.T) {
	tests := map[string]string{
		"E_WORKTREE_PAYLOAD_INVALID": "E_WORKSPACE_PATH_BINDING_INVALID",
		"E_WORKTREE_PAYLOAD_MISSING": "E_WORKSPACE_MISSING",
		"E_PAYLOAD_MISSING":          "E_SAVE_POINT_MISSING",
		"E_PAYLOAD_INVALID":          "E_SAVE_POINT_INVALID",
		"E_PAYLOAD_HASH_MISMATCH":    "E_SAVE_POINT_HASH_MISMATCH",
		"E_PARTIAL_SNAPSHOT":         "E_PARTIAL_SAVE_POINT",
		"E_GC_PLAN_MISMATCH":         "E_CLEANUP_PLAN_MISMATCH",
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got := publicErrorCodeVocabulary(raw)
			assert.Equal(t, want, got)
			assert.NotContains(t, got, "PAYLOAD")
		})
	}
}

func TestPublicDoctorJSONOmitsPayloadVocabulary(t *testing.T) {
	record := publicDoctor(&clidoctor.Result{
		Findings: []clidoctor.Finding{
			{
				Category:    "worktree",
				Description: "worktree 'main' payload path invalid",
				Severity:    "error",
				ErrorCode:   "E_WORKTREE_PAYLOAD_INVALID",
			},
			{
				Category:    "integrity",
				Description: "snapshot 1708300800000-deadbeef: payload missing",
				Severity:    "critical",
				ErrorCode:   "E_PAYLOAD_MISSING",
			},
			{
				Category:    "integrity",
				Description: "snapshot 1708300800000-feedface: payload hash mismatch",
				Severity:    "critical",
				ErrorCode:   "E_PAYLOAD_HASH_MISMATCH",
			},
		},
	})

	data, err := json.Marshal(record)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(data)), "payload")

	require.Len(t, record.Findings, 3)
	assert.Equal(t, "E_WORKSPACE_PATH_BINDING_INVALID", record.Findings[0].ErrorCode)
	assert.Equal(t, "E_SAVE_POINT_MISSING", record.Findings[1].ErrorCode)
	assert.Equal(t, "E_SAVE_POINT_HASH_MISMATCH", record.Findings[2].ErrorCode)
}

func TestPublicJSONBoundarySanitizesTransferRecords(t *testing.T) {
	raw := struct {
		Transfers []transfer.Record `json:"transfers"`
	}{
		Transfers: []transfer.Record{
			{
				TransferID:                 "save-primary",
				Operation:                  "save",
				Phase:                      "materialization",
				Primary:                    true,
				ResultKind:                 transfer.ResultKindFinal,
				PermissionScope:            transfer.PermissionScopeExecution,
				SourceRole:                 "workspace_content",
				SourcePath:                 "/workspace/project",
				DestinationRole:            "save_point_staging",
				MaterializationDestination: "/repo/.jvs/tmp/snapshots/1708300800000-deadbeef.tmp",
				CapabilityProbePath:        "/repo/.jvs/tmp/snapshots",
				PublishedDestination:       "/repo/.jvs/snapshots/1708300800000-deadbeef",
				CheckedForThisOperation:    true,
				RequestedEngine:            engine.EngineAuto,
				EffectiveEngine:            model.EngineCopy,
				PerformanceClass:           transfer.PerformanceClassNormalCopy,
				DegradedReasons:            []string{},
				Warnings:                   []string{},
			},
		},
	}

	publicData, err := publicJSONData(raw)
	require.NoError(t, err)
	payload, err := json.Marshal(publicData)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "payload")
	assert.NotContains(t, string(payload), ".jvs/snapshots")
	assert.NotContains(t, string(payload), "save_point_staging")

	data, ok := publicData.(map[string]any)
	require.True(t, ok, "public JSON data should be an object: %#v", publicData)
	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.Len(t, transfers, 1)
	record, ok := transfers[0].(map[string]any)
	require.True(t, ok, "transfer should be an object: %#v", transfers[0])
	assert.Equal(t, "workspace_content", record["source_role"])
	assert.Equal(t, "/workspace/project", record["source_path"])
	assert.Equal(t, "save_point_content", record["destination_role"])
	assert.Equal(t, "temporary_folder", record["materialization_destination"])
	assert.Equal(t, "control_data", record["capability_probe_path"])
	assert.Equal(t, "save_point:1708300800000-deadbeef", record["published_destination"])
}

func TestPublicJSONBoundarySanitizesTransferFreeText(t *testing.T) {
	raw := struct {
		Transfers []transfer.Record `json:"transfers"`
	}{
		Transfers: []transfer.Record{
			{
				TransferID:                 "save-primary",
				Operation:                  "save",
				Phase:                      "materialization",
				Primary:                    true,
				ResultKind:                 transfer.ResultKindFinal,
				PermissionScope:            transfer.PermissionScopeExecution,
				SourceRole:                 "workspace_content",
				SourcePath:                 "/workspace/project",
				DestinationRole:            "save_point_staging",
				MaterializationDestination: "/repo/.jvs/tmp/save/payload",
				CapabilityProbePath:        "/repo/.jvs/tmp",
				PublishedDestination:       "/repo/.jvs/snapshots/one/payload",
				CheckedForThisOperation:    true,
				RequestedEngine:            engine.EngineAuto,
				EffectiveEngine:            model.EngineCopy,
				PerformanceClass:           transfer.PerformanceClassNormalCopy,
				DegradedReasons: []string{
					"juicefs failed on /repo/.jvs/snapshots/one/payload: payload missing",
					"fast copy unavailable: juicefs-clone-context: stderr: failed on /repo/.jvs/snapshots/one/payload",
				},
				Warnings: []string{
					"fallback copied from /repo/.jvs/tmp/staging/payload after snapshot probe failed",
					"juicefs-clone-context: stdout: copied /repo/.jvs/snapshots/one/payload before fallback",
				},
			},
		},
	}

	publicData, err := publicJSONData(raw)
	require.NoError(t, err)
	payload, err := json.Marshal(publicData)
	require.NoError(t, err)
	publicJSON := string(payload)
	assert.NotContains(t, publicJSON, ".jvs")
	assert.NotContains(t, publicJSON, "payload")
	assert.NotContains(t, publicJSON, "snapshot")
	assert.NotContains(t, publicJSON, "stderr:")
	assert.NotContains(t, publicJSON, "stdout:")
	assert.NotContains(t, publicJSON, "/repo/")

	data, ok := publicData.(map[string]any)
	require.True(t, ok, "public JSON data should be an object: %#v", publicData)
	record := requirePublicJSONTransferByID(t, data, "save-primary")
	degradedReasons, ok := record["degraded_reasons"].([]any)
	require.True(t, ok, "degraded_reasons should be an array: %#v", record["degraded_reasons"])
	require.Len(t, degradedReasons, 2)
	assert.Contains(t, degradedReasons[0], "save point content")
	assert.Contains(t, degradedReasons[0], "save point content missing")
	assert.Contains(t, degradedReasons[1], "fast copy unavailable")
	assert.Contains(t, degradedReasons[1], "engine diagnostic redacted")
	warnings, ok := record["warnings"].([]any)
	require.True(t, ok, "warnings should be an array: %#v", record["warnings"])
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "internal storage path")
	assert.Contains(t, warnings[0], "save point probe failed")
	assert.Contains(t, warnings[1], "engine diagnostic redacted")
}

func TestPublicJSONTransferVocabularyForSaveViewRestoreFacades(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))

	saveOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "baseline")
	require.NoError(t, err)
	_, saveData := decodeFacadeDataMap(t, saveOut)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save should expose save_point_id: %#v", saveData)
	require.NotEmpty(t, savePointID)
	assertPublicTransfersUseContentVocabulary(t, saveData)
	saveTransfer := requirePublicJSONTransferByID(t, saveData, "save-primary")
	assert.Equal(t, "workspace_content", saveTransfer["source_role"])
	assert.Equal(t, repoRoot, saveTransfer["source_path"])
	assert.Equal(t, "save_point_content", saveTransfer["destination_role"])
	assert.Equal(t, "save_point:"+savePointID, saveTransfer["published_destination"])

	viewOut, err := executeCommand(createTestRootCmd(), "--json", "view", savePointID, "file.txt")
	require.NoError(t, err)
	_, viewData := decodeFacadeDataMap(t, viewOut)
	viewPath, ok := viewData["view_path"].(string)
	require.True(t, ok, "view_path should remain an openable path: %#v", viewData["view_path"])
	restoreViewWriteBitsForCleanup(t, viewPath)
	assert.NotContains(t, filepath.ToSlash(viewPath), "/payload")
	assertPublicTransfersUseContentVocabulary(t, viewData)
	viewTransfer := requirePublicJSONTransferByID(t, viewData, "view-primary")
	assert.Equal(t, "save_point_content", viewTransfer["source_role"])
	assert.Equal(t, "save_point:"+savePointID, viewTransfer["source_path"])
	assert.Equal(t, "content_view", viewTransfer["destination_role"])
	assert.Equal(t, "content_view:"+viewData["view_id"].(string)+"/file.txt", viewTransfer["published_destination"])

	restoreOut, err := executeCommand(createTestRootCmd(), "--json", "restore", savePointID)
	require.NoError(t, err)
	_, restoreData := decodeFacadeDataMap(t, restoreOut)
	assertPublicTransfersUseContentVocabulary(t, restoreData)
	restoreTransfer := requirePublicJSONTransferByID(t, restoreData, "restore-preview-validation-primary")
	assert.Equal(t, "save_point_content", restoreTransfer["source_role"])
	assert.Equal(t, "save_point:"+savePointID, restoreTransfer["source_path"])
	assert.Equal(t, "temporary_folder", restoreTransfer["destination_role"])
	assert.Equal(t, "temporary_folder", restoreTransfer["materialization_destination"])
	assert.Equal(t, repoRoot, restoreTransfer["published_destination"])
}

func assertPublicTransfersUseContentVocabulary(t *testing.T, data map[string]any) {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	require.NotEmpty(t, transfers)
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		require.True(t, ok, "transfer should be an object: %#v", item)
		encoded, err := json.Marshal(record)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "payload")
		assert.NotContains(t, string(encoded), ".jvs/snapshots")
		assert.NotContains(t, string(encoded), "save_point_payload")
		assert.NotContains(t, string(encoded), "save_point_staging")
	}
}

func requirePublicJSONTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		require.True(t, ok, "transfer should be an object: %#v", item)
		if record["transfer_id"] == id {
			return record
		}
	}
	t.Fatalf("missing transfer %q in %#v", id, transfers)
	return nil
}
