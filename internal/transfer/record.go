package transfer

import (
	"strings"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// ResultKind describes whether a transfer record is an expected preview result
// or the final result of a write operation.
type ResultKind string

const (
	ResultKindExpected ResultKind = "expected"
	ResultKindFinal    ResultKind = "final"
)

// PermissionScope prevents preview planning evidence from being confused with
// permission to execute a later write.
type PermissionScope string

const (
	PermissionScopePreviewOnly PermissionScope = "preview_only"
	PermissionScopeExecution   PermissionScope = "execution"
)

// PerformanceClass is the user-facing transfer class. It is intentionally
// separate from descriptor-level engine performance descriptors.
type PerformanceClass string

const (
	PerformanceClassFastCopy   PerformanceClass = "fast_copy"
	PerformanceClassNormalCopy PerformanceClass = "normal_copy"
)

// Intent captures the stable operation-local transfer identity and path
// boundaries before engine planning runs.
type Intent struct {
	TransferID                 string
	Operation                  string
	Phase                      string
	Primary                    bool
	ResultKind                 ResultKind
	PermissionScope            PermissionScope
	SourceRole                 string
	SourcePath                 string
	DestinationRole            string
	MaterializationDestination string
	CapabilityProbePath        string
	PublishedDestination       string
	RequestedEngine            model.EngineType
}

// Result is the planner/runtime transfer outcome before it is combined with
// intent identity fields for public JSON, audit, or metadata surfaces.
type Result struct {
	CheckedForThisOperation bool
	RequestedEngine         model.EngineType
	EffectiveEngine         model.EngineType
	OptimizedTransfer       bool
	PerformanceClass        PerformanceClass
	DegradedReasons         []string
	Warnings                []string
}

// Record is the canonical data.transfers[] entry.
type Record struct {
	TransferID                 string           `json:"transfer_id"`
	Operation                  string           `json:"operation"`
	Phase                      string           `json:"phase"`
	Primary                    bool             `json:"primary"`
	ResultKind                 ResultKind       `json:"result_kind"`
	PermissionScope            PermissionScope  `json:"permission_scope"`
	SourceRole                 string           `json:"source_role"`
	SourcePath                 string           `json:"source_path"`
	DestinationRole            string           `json:"destination_role"`
	MaterializationDestination string           `json:"materialization_destination"`
	CapabilityProbePath        string           `json:"capability_probe_path"`
	PublishedDestination       string           `json:"published_destination"`
	CheckedForThisOperation    bool             `json:"checked_for_this_operation"`
	RequestedEngine            model.EngineType `json:"requested_engine"`
	EffectiveEngine            model.EngineType `json:"effective_engine"`
	OptimizedTransfer          bool             `json:"optimized_transfer"`
	PerformanceClass           PerformanceClass `json:"performance_class"`
	DegradedReasons            []string         `json:"degraded_reasons"`
	Warnings                   []string         `json:"warnings"`
}

// Data is the canonical JSON payload shape for transfer-capable command data.
type Data struct {
	Transfers []Record `json:"transfers"`
}

// EnginePlanner is the engine planner surface used by the transfer adapter.
type EnginePlanner interface {
	PlanTransfer(engine.TransferPlanRequest) (*engine.TransferPlan, error)
}

// PlanIntent maps transfer boundaries into the engine planner request. The
// materialization destination is the write boundary; the published destination
// is intentionally not used for pair probing.
func PlanIntent(planner EnginePlanner, intent Intent) (*engine.TransferPlan, error) {
	if planner == nil {
		planner = engine.TransferPlanner{}
	}
	return planner.PlanTransfer(intent.PlanRequest())
}

// PlanRequest returns the engine planner request for this intent.
func (i Intent) PlanRequest() engine.TransferPlanRequest {
	return engine.TransferPlanRequest{
		SourcePath:      i.SourcePath,
		DestinationPath: i.MaterializationDestination,
		CapabilityPath:  i.CapabilityProbePath,
		RequestedEngine: requestedEngine(i.RequestedEngine),
	}
}

// RecordFromPlanAndRuntime merges the engine plan and runtime clone result
// into one canonical transfer record.
func RecordFromPlanAndRuntime(intent Intent, plan *engine.TransferPlan, runtime *engine.CloneResult) Record {
	result := ResultFromPlanAndRuntime(intent, plan, runtime)
	return Record{
		TransferID:                 intent.TransferID,
		Operation:                  intent.Operation,
		Phase:                      intent.Phase,
		Primary:                    intent.Primary,
		ResultKind:                 intent.ResultKind,
		PermissionScope:            intent.PermissionScope,
		SourceRole:                 intent.SourceRole,
		SourcePath:                 intent.SourcePath,
		DestinationRole:            intent.DestinationRole,
		MaterializationDestination: intent.MaterializationDestination,
		CapabilityProbePath:        intent.CapabilityProbePath,
		PublishedDestination:       intent.PublishedDestination,
		CheckedForThisOperation:    result.CheckedForThisOperation,
		RequestedEngine:            result.RequestedEngine,
		EffectiveEngine:            result.EffectiveEngine,
		OptimizedTransfer:          result.OptimizedTransfer,
		PerformanceClass:           result.PerformanceClass,
		DegradedReasons:            result.DegradedReasons,
		Warnings:                   result.Warnings,
	}
}

// ResultFromPlanAndRuntime combines planner degradations/warnings with runtime
// fallback information. Runtime effective engine wins because it reflects what
// actually completed the write.
func ResultFromPlanAndRuntime(intent Intent, plan *engine.TransferPlan, runtime *engine.CloneResult) Result {
	result := Result{
		RequestedEngine: requestedEngine(intent.RequestedEngine),
		DegradedReasons: []string{},
		Warnings:        []string{},
	}

	if plan != nil {
		result.CheckedForThisOperation = true
		result.RequestedEngine = requestedEngine(plan.RequestedEngine)
		result.EffectiveEngine = plan.EffectiveEngine
		result.OptimizedTransfer = optimizedEngine(plan.EffectiveEngine)
		result.DegradedReasons = appendUnique(result.DegradedReasons, plan.DegradedReasons...)
		result.Warnings = appendUnique(result.Warnings, plan.Warnings...)
	}

	if runtime != nil {
		if isConcreteEngine(runtime.EffectiveEngine) {
			result.EffectiveEngine = runtime.EffectiveEngine
		} else if isConcreteEngine(runtime.ActualEngine) {
			result.EffectiveEngine = runtime.ActualEngine
		}
		if runtime.Degraded {
			result.DegradedReasons = appendUnique(result.DegradedReasons, runtimeTransferDegradations(runtime)...)
		}
	}

	result.OptimizedTransfer = optimizedEngine(result.EffectiveEngine)
	result.PerformanceClass = PerformanceClassForOptimized(result.OptimizedTransfer)
	return result
}

// PerformanceClassForOptimized maps the final transfer outcome to the stable
// user-facing class used by data.transfers[].
func PerformanceClassForOptimized(optimized bool) PerformanceClass {
	if optimized {
		return PerformanceClassFastCopy
	}
	return PerformanceClassNormalCopy
}

func requestedEngine(engineType model.EngineType) model.EngineType {
	if engineType == "" {
		return engine.EngineAuto
	}
	return engineType
}

func isConcreteEngine(engineType model.EngineType) bool {
	return engineType != "" && engineType != engine.EngineAuto
}

func optimizedEngine(engineType model.EngineType) bool {
	return engineType == model.EngineJuiceFSClone || engineType == model.EngineReflinkCopy
}

func runtimeTransferDegradations(runtime *engine.CloneResult) []string {
	if runtime == nil || len(runtime.Degradations) == 0 {
		return nil
	}
	values := make([]string, 0, len(runtime.Degradations))
	for _, reason := range runtime.Degradations {
		if isMetadataOnlyDegradation(reason) {
			continue
		}
		values = append(values, reason)
	}
	return values
}

func isMetadataOnlyDegradation(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "hardlink":
		return true
	default:
		return false
	}
}

func appendUnique(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
