package worktree_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestManagerCreateStartedFromSnapshotAtRecordsPrimaryTransferToExplicitStagingPath(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)
	targetFolder := filepath.Join(t.TempDir(), "experiment")
	planner := &workspaceRecordingTransferPlanner{
		plan: &engine.TransferPlan{
			RequestedEngine:   model.EngineReflinkCopy,
			TransferEngine:    model.EngineCopy,
			EffectiveEngine:   model.EngineCopy,
			OptimizedTransfer: false,
			DegradedReasons:   []string{"these two locations cannot use fast copy together"},
		},
	}

	mgr := worktree.NewManager(repoPath)
	cfg, err := mgr.CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Name:            "experiment",
		Folder:          targetFolder,
		SnapshotID:      snapshotID,
		RequestedEngine: model.EngineReflinkCopy,
		TransferPlanner: planner,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "experiment", cfg.Name)
	require.Equal(t, targetFolder, cfg.RealPath)

	req := planner.request
	require.Equal(t, filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID)), req.SourcePath)
	require.Equal(t, filepath.Dir(targetFolder), req.CapabilityPath)
	require.Equal(t, filepath.Dir(targetFolder), filepath.Dir(req.DestinationPath))
	require.True(t, strings.Contains(filepath.Base(req.DestinationPath), ".experiment.staging-"), "workspace should materialize into explicit target staging: %s", req.DestinationPath)
	require.NotEqual(t, targetFolder, req.DestinationPath)
	require.FileExists(t, filepath.Join(targetFolder, "snapshot.txt"))

	record, ok := mgr.LastTransferRecord()
	require.True(t, ok)
	require.Equal(t, "workspace-new-primary", record.TransferID)
	require.Equal(t, "workspace_new", record.Operation)
	require.True(t, record.Primary)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, req.SourcePath, record.SourcePath)
	require.Equal(t, "workspace_folder", record.DestinationRole)
	require.Equal(t, req.DestinationPath, record.MaterializationDestination)
	require.Equal(t, req.CapabilityPath, record.CapabilityProbePath)
	require.Equal(t, targetFolder, record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, model.EngineReflinkCopy, record.RequestedEngine)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, "normal_copy", string(record.PerformanceClass))
	require.Equal(t, []string{"these two locations cannot use fast copy together"}, record.DegradedReasons)

	path, err := mgr.Path("experiment")
	require.NoError(t, err)
	require.Equal(t, targetFolder, path)
}

type workspaceRecordingTransferPlanner struct {
	request engine.TransferPlanRequest
	plan    *engine.TransferPlan
}

func (p *workspaceRecordingTransferPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	p.request = req
	plan := *p.plan
	plan.RequestedEngine = req.RequestedEngine
	return &plan, nil
}
