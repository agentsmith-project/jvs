package cli

import (
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type setupMaterialization struct {
	EffectiveEngine      model.EngineType
	MetadataPreservation model.MetadataPreservation
	PerformanceClass     string
}

func applySetupJSONFields(output map[string]any, capabilities *engine.CapabilityReport, effectiveEngine model.EngineType, warnings []string) {
	materialization := setupMaterialization{
		EffectiveEngine:      effectiveEngine,
		MetadataPreservation: engine.MetadataPreservationForEngine(effectiveEngine),
		PerformanceClass:     engine.PerformanceClassForEngine(effectiveEngine),
	}
	output["capabilities"] = capabilities
	output["effective_engine"] = materialization.EffectiveEngine
	output["metadata_preservation"] = materialization.MetadataPreservation
	output["performance_class"] = materialization.PerformanceClass
	output["warnings"] = stableStringSlice(warnings)
}

func stableStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return values
}
