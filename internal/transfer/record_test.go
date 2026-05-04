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

func TestPublicRecordFromRecordSanitizesInternalStorageVocabulary(t *testing.T) {
	record := Record{
		TransferID:                 "view-primary",
		Operation:                  "view",
		Phase:                      "view_materialization",
		Primary:                    true,
		ResultKind:                 ResultKindFinal,
		PermissionScope:            PermissionScopeExecution,
		SourceRole:                 "save_point_payload",
		SourcePath:                 "/repo/.jvs/snapshots/1708300800000-deadbeef",
		DestinationRole:            "view_directory",
		MaterializationDestination: "/repo/.jvs/views/view-1708300800001-feedface/payload",
		CapabilityProbePath:        "/repo/.jvs/views/view-1708300800001-feedface",
		PublishedDestination:       "/repo/.jvs/views/view-1708300800001-feedface/payload/src/app.txt",
		CheckedForThisOperation:    true,
		RequestedEngine:            engine.EngineAuto,
		EffectiveEngine:            model.EngineCopy,
		PerformanceClass:           PerformanceClassNormalCopy,
		DegradedReasons:            []string{},
		Warnings:                   []string{},
	}

	public := PublicRecordFromRecord(record)

	require.Equal(t, "save_point_content", public.SourceRole)
	require.Equal(t, "save_point:1708300800000-deadbeef", public.SourcePath)
	require.Equal(t, "content_view", public.DestinationRole)
	require.Equal(t, "content_view:view-1708300800001-feedface", public.MaterializationDestination)
	require.Equal(t, "content_view:view-1708300800001-feedface", public.CapabilityProbePath)
	require.Equal(t, "content_view:view-1708300800001-feedface/src/app.txt", public.PublishedDestination)

	payload, err := json.Marshal(Data{Transfers: []Record{record}})
	require.NoError(t, err)
	require.Contains(t, string(payload), "save_point_payload", "internal record JSON is still storage/reporting evidence")

	publicPayload, err := json.Marshal(PublicData{Transfers: []PublicRecord{public}})
	require.NoError(t, err)
	require.NotContains(t, string(publicPayload), "payload")
	require.NotContains(t, string(publicPayload), ".jvs/snapshots")
	require.NotContains(t, string(publicPayload), "save_point_payload")
}

func TestPublicRecordFromRecordSanitizesFreeTextDetails(t *testing.T) {
	record := Record{
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
		CheckedForThisOperation:    true,
		RequestedEngine:            engine.EngineAuto,
		EffectiveEngine:            model.EngineCopy,
		PerformanceClass:           PerformanceClassNormalCopy,
		DegradedReasons: []string{
			"juicefs failed on /repo/.jvs/snapshots/one/payload: payload missing",
		},
		Warnings: []string{
			"fallback copied from /repo/.jvs/tmp/staging/payload after snapshot probe failed",
		},
	}

	internalPayload, err := json.Marshal(Data{Transfers: []Record{record}})
	require.NoError(t, err)
	require.Contains(t, string(internalPayload), "/repo/.jvs/snapshots/one/payload")
	require.Contains(t, string(internalPayload), "payload missing")

	public := PublicRecordFromRecord(record)
	publicPayload, err := json.Marshal(PublicData{Transfers: []PublicRecord{public}})
	require.NoError(t, err)
	publicJSON := string(publicPayload)
	require.NotContains(t, publicJSON, ".jvs")
	require.NotContains(t, publicJSON, "payload")
	require.NotContains(t, publicJSON, "snapshot")
	require.NotContains(t, publicJSON, "/repo/")
	require.Contains(t, public.DegradedReasons[0], "save point content")
	require.Contains(t, public.DegradedReasons[0], "save point content missing")
	require.Contains(t, public.Warnings[0], "internal storage path")
	require.Contains(t, public.Warnings[0], "save point probe failed")
}

func TestPublicRecordFromRecordRedactsRawEngineDiagnostics(t *testing.T) {
	record := Record{
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
		CheckedForThisOperation:    true,
		RequestedEngine:            engine.EngineAuto,
		EffectiveEngine:            model.EngineCopy,
		PerformanceClass:           PerformanceClassNormalCopy,
		DegradedReasons: []string{
			"fast copy unavailable: juicefs-clone-context: stderr: failed on /repo/.jvs/snapshots/one/payload",
		},
		Warnings: []string{
			"juicefs-clone-context: stdout: copied /repo/.jvs/snapshots/one/payload before fallback",
		},
	}

	internalPayload, err := json.Marshal(Data{Transfers: []Record{record}})
	require.NoError(t, err)
	require.Contains(t, string(internalPayload), "stderr:")
	require.Contains(t, string(internalPayload), "stdout:")
	require.Contains(t, string(internalPayload), "/repo/.jvs/snapshots/one/payload")

	public := PublicRecordFromRecord(record)
	publicPayload, err := json.Marshal(PublicData{Transfers: []PublicRecord{public}})
	require.NoError(t, err)
	publicJSON := string(publicPayload)
	require.NotContains(t, publicJSON, "stderr:")
	require.NotContains(t, publicJSON, "stdout:")
	require.NotContains(t, publicJSON, ".jvs")
	require.NotContains(t, publicJSON, "payload")
	require.NotContains(t, publicJSON, "snapshot")
	require.NotContains(t, publicJSON, "/repo/")
	require.Contains(t, public.DegradedReasons[0], "fast copy unavailable")
	require.Contains(t, public.DegradedReasons[0], "engine diagnostic redacted")
	require.Contains(t, public.Warnings[0], "engine diagnostic redacted")
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

func TestRecordFromPlanAndRuntimeKeepsRuntimeDiagnosticEvidenceInternal(t *testing.T) {
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
	}
	runtime := engine.NewCloneResult(model.EngineJuiceFSClone)
	runtime.AddDegradation("juicefs-clone-context: stderr: failed on /repo/.jvs/snapshots/one/payload", model.EngineCopy)

	record := RecordFromPlanAndRuntime(intent, plan, runtime)

	require.Len(t, record.DegradedReasons, 1)
	require.Contains(t, record.DegradedReasons[0], "stderr:")
	require.Contains(t, record.DegradedReasons[0], "/repo/.jvs/snapshots/one/payload")

	internalPayload, err := json.Marshal(Data{Transfers: []Record{record}})
	require.NoError(t, err)
	require.Contains(t, string(internalPayload), "stderr:")
	require.Contains(t, string(internalPayload), "/repo/.jvs/snapshots/one/payload")

	publicPayload, err := json.Marshal(PublicData{Transfers: []PublicRecord{PublicRecordFromRecord(record)}})
	require.NoError(t, err)
	require.NotContains(t, string(publicPayload), "stderr:")
	require.NotContains(t, string(publicPayload), ".jvs")
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
