package transfer

import (
	"encoding/json"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestRecordFromPlanAndRuntimeFastPathSuccess(t *testing.T) {
	intent := Intent{
		TransferID:                 "workspace-new-primary",
		Operation:                  "workspace_new",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "save_point_payload",
		SourcePath:                 "/repo/.jvs/snapshots/one/payload",
		DestinationRole:            "workspace_folder",
		MaterializationDestination: "/work/.jvs-tmp/materialize",
		CapabilityProbePath:        "/work",
		PublishedDestination:       "/work/experiment",
		RequestedEngine:            engine.EngineAuto,
	}
	plan := &engine.TransferPlan{
		RequestedEngine:   engine.EngineAuto,
		TransferEngine:    model.EngineReflinkCopy,
		EffectiveEngine:   model.EngineReflinkCopy,
		OptimizedTransfer: true,
	}
	runtime := engine.NewCloneResult(model.EngineReflinkCopy)

	record := RecordFromPlanAndRuntime(intent, plan, runtime)

	require.Equal(t, "workspace-new-primary", record.TransferID)
	require.Equal(t, "workspace_new", record.Operation)
	require.Equal(t, "materialization", record.Phase)
	require.True(t, record.Primary)
	require.Equal(t, ResultKindFinal, record.ResultKind)
	require.Equal(t, PermissionScopeExecution, record.PermissionScope)
	require.Equal(t, "save_point_payload", record.SourceRole)
	require.Equal(t, "/repo/.jvs/snapshots/one/payload", record.SourcePath)
	require.Equal(t, "workspace_folder", record.DestinationRole)
	require.Equal(t, "/work/.jvs-tmp/materialize", record.MaterializationDestination)
	require.Equal(t, "/work", record.CapabilityProbePath)
	require.Equal(t, "/work/experiment", record.PublishedDestination)
	require.True(t, record.CheckedForThisOperation)
	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Equal(t, model.EngineReflinkCopy, record.EffectiveEngine)
	require.True(t, record.OptimizedTransfer)
	require.Equal(t, PerformanceClassFastCopy, record.PerformanceClass)
	require.Empty(t, record.DegradedReasons)
	require.Empty(t, record.Warnings)

	payload, err := json.Marshal(Data{Transfers: []Record{record}})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"transfers": [{
			"transfer_id": "workspace-new-primary",
			"operation": "workspace_new",
			"phase": "materialization",
			"primary": true,
			"result_kind": "final",
			"permission_scope": "execution",
			"source_role": "save_point_payload",
			"source_path": "/repo/.jvs/snapshots/one/payload",
			"destination_role": "workspace_folder",
			"materialization_destination": "/work/.jvs-tmp/materialize",
			"capability_probe_path": "/work",
			"published_destination": "/work/experiment",
			"checked_for_this_operation": true,
			"requested_engine": "auto",
			"effective_engine": "reflink-copy",
			"optimized_transfer": true,
			"performance_class": "fast_copy",
			"degraded_reasons": [],
			"warnings": []
		}]
	}`, string(payload))
}

func TestPlanIntentPairUnsupportedProducesNormalCopyRecord(t *testing.T) {
	intent := Intent{
		TransferID:                 "restore-run-primary",
		Operation:                  "restore",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "save_point_payload",
		SourcePath:                 "/snap/payload",
		DestinationRole:            "restore_staging",
		MaterializationDestination: "/workspace/.jvs-restore/staging",
		CapabilityProbePath:        "/workspace",
		PublishedDestination:       "/workspace/project",
		RequestedEngine:            model.EngineReflinkCopy,
	}
	prober := &fakeCapabilityProber{
		report: &engine.CapabilityReport{
			Write: engine.Capability{
				Available:  true,
				Supported:  true,
				Confidence: engine.CapabilityConfirmed,
			},
			Copy: engine.Capability{
				Available:  true,
				Supported:  true,
				Confidence: engine.CapabilityConfirmed,
			},
			Reflink: engine.Capability{
				Available:  true,
				Supported:  true,
				Confidence: engine.CapabilityConfirmed,
			},
			RecommendedEngine: model.EngineReflinkCopy,
		},
		pair: &engine.TransferPairReport{
			Warnings: []string{"reflink pair probe failed: invalid cross-device link"},
			Reflink: engine.Capability{
				Available:  true,
				Supported:  false,
				Confidence: engine.CapabilityConfirmed,
				Warnings:   []string{"reflink pair probe failed: invalid cross-device link"},
			},
		},
	}

	plan, err := PlanIntent(engine.TransferPlanner{Prober: prober}, intent)
	require.NoError(t, err)
	record := RecordFromPlanAndRuntime(intent, plan, nil)

	require.Equal(t, "/workspace", prober.capabilityPath)
	require.Equal(t, "/snap/payload", prober.pairSourcePath)
	require.Equal(t, "/workspace", prober.pairDestinationPath)
	require.Equal(t, "/workspace/.jvs-restore/staging", record.MaterializationDestination)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, PerformanceClassNormalCopy, record.PerformanceClass)
	require.Len(t, record.DegradedReasons, 1)
	require.Contains(t, record.DegradedReasons[0], "source/destination pair")
	require.Contains(t, record.DegradedReasons[0], "invalid cross-device link")
	require.Contains(t, record.Warnings, "reflink pair probe failed: invalid cross-device link")
}

func TestRecordFromPlanAndRuntimeMergesRuntimeFallbackReasons(t *testing.T) {
	intent := Intent{
		TransferID:                 "save-primary",
		Operation:                  "save",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "workspace_content",
		SourcePath:                 "/workspace/project",
		DestinationRole:            "save_point_staging",
		MaterializationDestination: "/repo/.jvs/tmp/save/payload",
		CapabilityProbePath:        "/repo/.jvs/tmp",
		PublishedDestination:       "/repo/.jvs/snapshots/one/payload",
		RequestedEngine:            engine.EngineAuto,
	}
	plan := &engine.TransferPlan{
		RequestedEngine:   engine.EngineAuto,
		TransferEngine:    model.EngineJuiceFSClone,
		EffectiveEngine:   model.EngineJuiceFSClone,
		OptimizedTransfer: true,
		DegradedReasons:   []string{"runtime optimized clone failed"},
		Warnings:          []string{"destination capability warning"},
	}
	runtime := engine.NewCloneResult(model.EngineJuiceFSClone)
	runtime.PerformanceClass = "constant-time-metadata-clone"
	runtime.AddDegradation("runtime optimized clone failed", model.EngineCopy)
	runtime.AddDegradation("fallback copy completed", model.EngineCopy)

	record := RecordFromPlanAndRuntime(intent, plan, runtime)

	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, PerformanceClassNormalCopy, record.PerformanceClass)
	require.NotEqual(t, runtime.PerformanceClass, string(record.PerformanceClass))
	require.Equal(t, []string{"runtime optimized clone failed", "fallback copy completed"}, record.DegradedReasons)
	require.Equal(t, []string{"destination capability warning"}, record.Warnings)
}

func TestRecordFromPlanAndRuntimeKeepsConcretePlanWhenRuntimeReportsRequestedAuto(t *testing.T) {
	intent := Intent{
		TransferID:                 "save-primary",
		Operation:                  "save",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "workspace_content",
		SourcePath:                 "/workspace/project",
		DestinationRole:            "save_point_staging",
		MaterializationDestination: "/repo/.jvs/tmp/save/payload",
		CapabilityProbePath:        "/repo/.jvs/tmp",
		PublishedDestination:       "/repo/.jvs/snapshots/one/payload",
		RequestedEngine:            engine.EngineAuto,
	}
	plan := &engine.TransferPlan{
		RequestedEngine:   engine.EngineAuto,
		TransferEngine:    model.EngineCopy,
		EffectiveEngine:   model.EngineCopy,
		OptimizedTransfer: false,
	}
	runtime := engine.NewCloneResult(engine.EngineAuto)

	record := RecordFromPlanAndRuntime(intent, plan, runtime)

	require.Equal(t, engine.EngineAuto, record.RequestedEngine)
	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Equal(t, PerformanceClassNormalCopy, record.PerformanceClass)
}

func TestRecordFromPlanAndRuntimeDoesNotTreatCopyMetadataDegradationAsTransferFallback(t *testing.T) {
	intent := Intent{
		TransferID:                 "save-primary",
		Operation:                  "save",
		Phase:                      "materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "workspace_content",
		SourcePath:                 "/workspace/project",
		DestinationRole:            "save_point_staging",
		MaterializationDestination: "/repo/.jvs/tmp/save/payload",
		CapabilityProbePath:        "/repo/.jvs/tmp",
		PublishedDestination:       "/repo/.jvs/snapshots/one/payload",
		RequestedEngine:            engine.EngineAuto,
	}
	plan := &engine.TransferPlan{
		RequestedEngine:   engine.EngineAuto,
		TransferEngine:    model.EngineCopy,
		EffectiveEngine:   model.EngineCopy,
		OptimizedTransfer: false,
	}
	runtime := engine.NewCloneResult(model.EngineCopy)
	runtime.AddDegradation("hardlink", model.EngineCopy)

	record := RecordFromPlanAndRuntime(intent, plan, runtime)

	require.Equal(t, model.EngineCopy, record.EffectiveEngine)
	require.False(t, record.OptimizedTransfer)
	require.Empty(t, record.DegradedReasons)
}

type fakeCapabilityProber struct {
	report              *engine.CapabilityReport
	pair                *engine.TransferPairReport
	capabilityPath      string
	pairSourcePath      string
	pairDestinationPath string
}

func (p *fakeCapabilityProber) ProbeCapabilities(path string, writeProbe bool) (*engine.CapabilityReport, error) {
	p.capabilityPath = path
	return p.report, nil
}

func (p *fakeCapabilityProber) ProbeTransferPair(sourcePath, destinationPath string) (*engine.TransferPairReport, error) {
	p.pairSourcePath = sourcePath
	p.pairDestinationPath = destinationPath
	return p.pair, nil
}
