package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestCreatorCreateSavePointRecordsPrimaryTransferFromWorkspaceToSnapshotStaging(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(mainPayloadPath(t, repoPath), "app.txt"), []byte("v1"), 0644))

	planner := &recordingTransferPlanner{
		plan: &engine.TransferPlan{
			RequestedEngine:   model.EngineReflinkCopy,
			TransferEngine:    model.EngineCopy,
			EffectiveEngine:   model.EngineCopy,
			OptimizedTransfer: false,
			DegradedReasons:   []string{"these two locations cannot use fast copy together"},
		},
	}
	creator := NewCreator(repoPath, model.EngineReflinkCopy)
	creator.transferPlanner = planner

	desc, err := creator.CreateSavePoint("main", "transfer planned", nil)
	require.NoError(t, err)
	require.NotNil(t, desc)

	req := planner.request
	require.Equal(t, mainPayloadPath(t, repoPath), req.SourcePath)
	require.True(t, strings.HasSuffix(req.DestinationPath, ".tmp"), "save should materialize into unpublished snapshot tmp: %s", req.DestinationPath)
	require.Equal(t, filepath.Dir(req.DestinationPath), req.CapabilityPath)
	require.DirExists(t, req.CapabilityPath)
	require.NotEqual(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID)), req.DestinationPath)

	record, ok := creator.LastTransferRecord()
	require.True(t, ok)
	require.Equal(t, "save-primary", record.TransferID)
	require.Equal(t, "save", record.Operation)
	require.True(t, record.Primary)
	require.Equal(t, "workspace_content", record.SourceRole)
	require.Equal(t, req.SourcePath, record.SourcePath)
	require.Equal(t, "save_point_staging", record.DestinationRole)
	require.Equal(t, req.DestinationPath, record.MaterializationDestination)
	require.Equal(t, req.CapabilityPath, record.CapabilityProbePath)
	require.Equal(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID)), record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, model.EngineReflinkCopy, record.RequestedEngine)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, "normal_copy", string(record.PerformanceClass))
	require.Equal(t, []string{"these two locations cannot use fast copy together"}, record.DegradedReasons)

	require.Equal(t, model.EngineReflinkCopy, desc.Engine)
	require.Equal(t, model.EngineCopy, desc.ActualEngine)
	require.Equal(t, model.EngineCopy, desc.EffectiveEngine)
	require.Equal(t, []string{"these two locations cannot use fast copy together"}, desc.DegradedReasons)
	require.Equal(t, "linear-data-copy", desc.PerformanceClass)
}

func TestCreatorCreateSavePointWithAutoPersistsConcreteEngineAndAuditsTransferEvidence(t *testing.T) {
	repoPath := setupCreatorFailureRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(mainPayloadPath(t, repoPath), "app.txt"), []byte("v1"), 0644))

	creator := NewCreator(repoPath, engine.EngineAuto)
	desc, err := creator.CreateSavePoint("main", "auto concrete", nil)
	require.NoError(t, err)

	require.NotEqual(t, engine.EngineAuto, desc.Engine)
	require.NotEqual(t, engine.EngineAuto, desc.ActualEngine)
	require.NotEqual(t, engine.EngineAuto, desc.EffectiveEngine)
	require.Equal(t, desc.EffectiveEngine, desc.Engine)
	require.Equal(t, engine.PerformanceClassForEngine(desc.EffectiveEngine), desc.PerformanceClass)

	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID), ".READY")
	readyData, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	var ready model.ReadyMarker
	require.NoError(t, json.Unmarshal(readyData, &ready))
	require.Equal(t, desc.Engine, ready.Engine)
	require.NotEqual(t, engine.EngineAuto, ready.Engine)

	record, ok := creator.LastTransferRecord()
	require.True(t, ok)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Equal(t, desc.EffectiveEngine, record.EffectiveEngine)
	require.NotEqual(t, engine.EngineAuto, record.EffectiveEngine)

	auditRecord := readLastSnapshotAuditRecordForTransferTest(t, repoPath)
	require.Equal(t, string(desc.Engine), auditRecord.Details["engine"])
	require.NotEqual(t, "auto", auditRecord.Details["engine"])

	transfers := auditTransfersForTransferTest(t, auditRecord)
	require.Len(t, transfers, 1)
	primary := transfers[0]
	require.Equal(t, "save-primary", primary["transfer_id"])
	require.Equal(t, "save", primary["operation"])
	require.Equal(t, "workspace_content", primary["source_role"])
	require.NotEmpty(t, primary["source_path"])
	require.Equal(t, "save_point_staging", primary["destination_role"])
	require.NotEmpty(t, primary["materialization_destination"])
	require.NotEmpty(t, primary["published_destination"])
	require.Equal(t, "auto", primary["requested_engine"])
	require.Equal(t, string(desc.EffectiveEngine), primary["effective_engine"])
	require.Contains(t, []any{"fast_copy", "normal_copy"}, primary["performance_class"])
	require.IsType(t, []any{}, primary["degraded_reasons"])
	require.IsType(t, []any{}, primary["warnings"])
}

func TestCreatorCreateSavePointEmptyAdoptedWorkspaceWithAutoUsesConcretePlanResult(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "empty-project")
	require.NoError(t, os.MkdirAll(folder, 0755))
	r, err := repo.InitAdoptedWorkspace(folder)
	require.NoError(t, err)

	creator := NewCreator(r.Root, engine.EngineAuto)
	desc, err := creator.CreateSavePoint("main", "empty adopted", nil)
	require.NoError(t, err)

	record, ok := creator.LastTransferRecord()
	require.True(t, ok)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.NotEqual(t, engine.EngineAuto, record.EffectiveEngine)
	require.NotEmpty(t, record.EffectiveEngine)
	require.Equal(t, record.EffectiveEngine, desc.EffectiveEngine)
	require.Equal(t, record.EffectiveEngine, desc.Engine)
	require.NotEqual(t, engine.EngineAuto, desc.ActualEngine)
	require.Equal(t, engine.PerformanceClassForEngine(record.EffectiveEngine), desc.PerformanceClass)
}

type recordingTransferPlanner struct {
	request engine.TransferPlanRequest
	plan    *engine.TransferPlan
}

func (p *recordingTransferPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	p.request = req
	plan := *p.plan
	plan.RequestedEngine = req.RequestedEngine
	return &plan, nil
}

func readLastSnapshotAuditRecordForTransferTest(t *testing.T, repoPath string) model.AuditRecord {
	t.Helper()

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	auditData, err := os.ReadFile(auditPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(auditData)), "\n")
	require.NotEmpty(t, lines)
	var record model.AuditRecord
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &record))
	require.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
	require.NotNil(t, record.Details)
	return record
}

func auditTransfersForTransferTest(t *testing.T, record model.AuditRecord) []map[string]any {
	t.Helper()

	encoded, err := json.Marshal(record.Details["transfers"])
	require.NoError(t, err)
	var transfers []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &transfers))
	return transfers
}
