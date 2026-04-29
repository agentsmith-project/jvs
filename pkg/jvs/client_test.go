package jvs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewCleanupExposesProtectionGroups(t *testing.T) {
	dir := t.TempDir()
	client, err := Init(dir, InitOptions{Name: "client-test", EngineType: model.EngineCopy})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("baseline"), 0644))

	savePoint, err := client.Save(context.Background(), SaveOptions{Message: "baseline"})
	require.NoError(t, err)

	plan, err := client.PreviewCleanup(context.Background(), CleanupOptions{})
	require.NoError(t, err)

	group := clientProtectionGroupByReason(plan.ProtectionGroups, CleanupProtectionReasonHistory)
	require.NotNil(t, group)
	assert.Equal(t, 1, group.Count)
	assert.Equal(t, []SavePointID{savePoint.SavePointID}, group.SavePoints)
}

func TestCleanupProtectionReasonConstantsExposeStableTokens(t *testing.T) {
	reasons := map[CleanupProtectionReason]string{
		CleanupProtectionReasonHistory:         "history",
		CleanupProtectionReasonOpenView:        "open_view",
		CleanupProtectionReasonActiveRecovery:  "active_recovery",
		CleanupProtectionReasonActiveOperation: "active_operation",
	}

	require.Len(t, reasons, 4)
	for reason, token := range reasons {
		assert.Equal(t, token, string(reason))
	}
}

func TestCleanupProtectionGroupJSONUsesStableReasonToken(t *testing.T) {
	group := CleanupProtectionGroup{
		Reason:     CleanupProtectionReasonOpenView,
		Count:      1,
		SavePoints: []SavePointID{"sp_1"},
	}

	data, err := json.Marshal(group)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reason":"open_view","count":1,"save_points":["sp_1"]}`, string(data))
}

func TestPublicCleanupPlanMapsProtectionReasonsToFacadeConstants(t *testing.T) {
	plan, err := publicCleanupPlan(&model.GCPlan{
		ProtectionGroups: []model.GCProtectionGroup{
			{Reason: model.GCProtectionReasonHistory, Count: 2},
			{Reason: model.GCProtectionReasonOpenView, Count: 1},
			{Reason: model.GCProtectionReasonActiveRecovery, Count: 1},
			{Reason: model.GCProtectionReasonActiveOperation, Count: 1},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.ProtectionGroups, 4)

	assert.Equal(t, CleanupProtectionReasonHistory, plan.ProtectionGroups[0].Reason)
	assert.Equal(t, CleanupProtectionReasonOpenView, plan.ProtectionGroups[1].Reason)
	assert.Equal(t, CleanupProtectionReasonActiveRecovery, plan.ProtectionGroups[2].Reason)
	assert.Equal(t, CleanupProtectionReasonActiveOperation, plan.ProtectionGroups[3].Reason)
	assert.Equal(t, 2, plan.ProtectedByHistory)
}

func TestPublicCleanupPlanRejectsUnknownProtectionReason(t *testing.T) {
	plan, err := publicCleanupPlan(&model.GCPlan{
		ProtectionGroups: []model.GCProtectionGroup{
			{Reason: "lineage", Count: 1},
		},
	})

	require.Nil(t, plan)
	require.ErrorContains(t, err, "cleanup protection reason")
	assert.NotContains(t, err.Error(), "lineage")
}

func clientProtectionGroupByReason(groups []CleanupProtectionGroup, reason CleanupProtectionReason) *CleanupProtectionGroup {
	for i := range groups {
		if groups[i].Reason == reason {
			return &groups[i]
		}
	}
	return nil
}
