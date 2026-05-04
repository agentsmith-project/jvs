package jvs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
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
		CleanupProtectionReasonHistory:              "history",
		CleanupProtectionReasonOpenView:             "open_view",
		CleanupProtectionReasonActiveRecovery:       "active_recovery",
		CleanupProtectionReasonActiveOperation:      "active_operation",
		CleanupProtectionReasonImportedCloneHistory: "imported_clone_history",
	}

	require.Len(t, reasons, 5)
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

func TestSavePointPublicFacadeUsesContentRootHash(t *testing.T) {
	desc := &model.Descriptor{
		SnapshotID:         model.SnapshotID("1708300800000-feedface"),
		WorktreeName:       "main",
		CreatedAt:          time.Unix(0, 0).UTC(),
		Engine:             model.EngineCopy,
		PayloadRootHash:    model.HashValue("content-hash"),
		DescriptorChecksum: model.HashValue("descriptor-checksum"),
		IntegrityState:     model.IntegrityVerified,
	}

	savePoint := publicSavePoint(desc)
	require.NotNil(t, savePoint)
	assert.Equal(t, model.HashValue("content-hash"), savePoint.ContentRootHash)

	data, err := json.Marshal(savePoint)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"content_root_hash":"content-hash"`)
	assert.NotContains(t, string(data), "payload_root_hash")
}

func TestPublicCleanupPlanMapsProtectionReasonsToFacadeConstants(t *testing.T) {
	plan, err := publicCleanupPlan(&model.GCPlan{
		ProtectionGroups: []model.GCProtectionGroup{
			{Reason: model.GCProtectionReasonHistory, Count: 2},
			{Reason: model.GCProtectionReasonOpenView, Count: 1},
			{Reason: model.GCProtectionReasonActiveRecovery, Count: 1},
			{Reason: model.GCProtectionReasonActiveOperation, Count: 1},
			{Reason: model.GCProtectionReasonImportedCloneHistory, Count: 1},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.ProtectionGroups, 5)

	assert.Equal(t, CleanupProtectionReasonHistory, plan.ProtectionGroups[0].Reason)
	assert.Equal(t, CleanupProtectionReasonOpenView, plan.ProtectionGroups[1].Reason)
	assert.Equal(t, CleanupProtectionReasonActiveRecovery, plan.ProtectionGroups[2].Reason)
	assert.Equal(t, CleanupProtectionReasonActiveOperation, plan.ProtectionGroups[3].Reason)
	assert.Equal(t, CleanupProtectionReasonImportedCloneHistory, plan.ProtectionGroups[4].Reason)
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

func TestCleanupFacadeErrorAsIsExposeOnlyPublicCleanupClass(t *testing.T) {
	dir := t.TempDir()
	client, err := Init(dir, InitOptions{Name: "client-test", EngineType: model.EngineCopy})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("baseline"), 0644))
	savePoint, err := client.Save(context.Background(), SaveOptions{Message: "baseline"})
	require.NoError(t, err)

	readyPath := filepath.Join(client.RepoRoot(), ".jvs", "snapshots", string(savePoint.SavePointID), ".READY")
	require.NoError(t, os.WriteFile(readyPath, []byte("{not json"), 0644))

	_, err = client.PreviewCleanup(context.Background(), CleanupOptions{})
	require.Error(t, err)

	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "cleanup facade error should expose a public JVSError")
	assert.Equal(t, errclass.ErrCleanupPlanMismatch.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "save point storage")
	assert.True(t, errors.Is(err, errclass.ErrCleanupPlanMismatch), "cleanup facade error should match the public cleanup class")
	assert.False(t, errors.Is(err, &errclass.JVSError{Code: "E_READY_INVALID"}), "cleanup facade error must not match internal readiness classes")

	assert.Contains(t, err.Error(), "save point storage")
	assert.NotContains(t, err.Error(), "publish state")
	assert.NotContains(t, err.Error(), "READY")
	assert.NotContains(t, err.Error(), ".jvs")
}

func TestPreviewCleanupRepoBusyUsesPublicCleanupVocabulary(t *testing.T) {
	dir := t.TempDir()
	client, err := Init(dir, InitOptions{Name: "client-test", EngineType: model.EngineCopy})
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(client.RepoRoot(), "held-by-test")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, held.Release()) })

	_, err = client.PreviewCleanup(context.Background(), CleanupOptions{})
	require.Error(t, err)
	assertPublicCleanupBusyError(t, err, "cleanup preview")
}

func TestRunCleanupRepoBusyUsesPublicCleanupVocabulary(t *testing.T) {
	dir := t.TempDir()
	client, err := Init(dir, InitOptions{Name: "client-test", EngineType: model.EngineCopy})
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(client.RepoRoot(), "held-by-test")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, held.Release()) })

	err = client.RunCleanup(context.Background(), "missing-plan")
	require.Error(t, err)
	assertPublicCleanupBusyError(t, err, "cleanup run")
}

func clientProtectionGroupByReason(groups []CleanupProtectionGroup, reason CleanupProtectionReason) *CleanupProtectionGroup {
	for i := range groups {
		if groups[i].Reason == reason {
			return &groups[i]
		}
	}
	return nil
}

func assertPublicCleanupBusyError(t *testing.T, err error, operation string) {
	t.Helper()

	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "cleanup facade error should expose a public JVSError")
	assert.Equal(t, errclass.ErrRepoBusy.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, operation)
	assert.NotContains(t, strings.ToLower(jvsErr.Code), "gc")
	assert.NotContains(t, strings.ToLower(jvsErr.Message), "gc")
	assert.NotContains(t, strings.ToLower(err.Error()), "gc")
	assert.True(t, errors.Is(err, errclass.ErrRepoBusy), "cleanup facade error should match repo busy")
}
