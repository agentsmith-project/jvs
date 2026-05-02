package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestRestoreRunPlansTransferForActualTempMaterialization(t *testing.T) {
	repoPath := t.TempDir()
	_, err := repo.Init(repoPath, "test")
	require.NoError(t, err)
	mainPath := filepath.Join(repoPath, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("snapshot"), 0644))
	desc, err := snapshot.NewCreator(repoPath, model.EngineCopy).CreateSavePoint("main", "base", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("modified"), 0644))

	planner := &restoreRecordingTransferPlanner{
		plan: &engine.TransferPlan{
			TransferEngine:    model.EngineCopy,
			EffectiveEngine:   model.EngineCopy,
			OptimizedTransfer: false,
			DegradedReasons:   []string{"these two locations cannot use fast copy together"},
			Warnings:          []string{},
		},
	}
	restorer := NewRestorer(repoPath, engine.EngineAuto)
	restorer.transferPlanner = planner

	err = restorer.Restore("main", desc.SnapshotID)
	require.NoError(t, err)

	req := planner.request
	require.Equal(t, filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID)), req.SourcePath)
	require.Contains(t, req.DestinationPath, ".restore-tmp-")
	require.Equal(t, filepath.Dir(req.DestinationPath), req.CapabilityPath)
	require.Equal(t, engine.EngineAuto, req.RequestedEngine)

	record, ok := restorer.LastTransferRecord()
	require.True(t, ok)
	require.Equal(t, "restore-run-primary", record.TransferID)
	require.Equal(t, "restore", record.Operation)
	require.Equal(t, "materialization", record.Phase)
	require.True(t, record.Primary)
	require.Equal(t, transfer.ResultKindFinal, record.ResultKind)
	require.Equal(t, transfer.PermissionScopeExecution, record.PermissionScope)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, req.SourcePath, record.SourcePath)
	require.Equal(t, "restore_staging", record.DestinationRole)
	require.Equal(t, req.DestinationPath, record.MaterializationDestination)
	require.Equal(t, req.CapabilityPath, record.CapabilityProbePath)
	require.Equal(t, mainPath, record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, transfer.PerformanceClassNormalCopy, record.PerformanceClass)
	require.Equal(t, []string{"these two locations cannot use fast copy together"}, record.DegradedReasons)
	require.False(t, strings.Contains(record.MaterializationDestination, ".restore-backup-"), "backup ledger path must not be modeled as a copy transfer")
}

type restoreRecordingTransferPlanner struct {
	request engine.TransferPlanRequest
	plan    *engine.TransferPlan
}

func (p *restoreRecordingTransferPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	p.request = req
	plan := *p.plan
	plan.RequestedEngine = req.RequestedEngine
	return &plan, nil
}
