package engine

import (
	"github.com/jvs-project/jvs/pkg/model"
)

// CloneResult contains the result of a clone operation, including any
// degradation information if the clone could not use the optimal method.
type CloneResult struct {
	ActualEngine         model.EngineType           // engine that actually wrote data
	EffectiveEngine      model.EngineType           // public effective transfer/materialization engine
	MetadataPreservation model.MetadataPreservation // metadata preservation contract
	PerformanceClass     string                     // performance class for the effective engine
	Degraded             bool                       // true if any degradation occurred
	Degradations         []string                   // list of degradation types
}

// Engine defines the snapshot engine interface for copying worktree data.
type Engine interface {
	// Name returns the engine type identifier.
	Name() model.EngineType

	// Clone performs a copy of src to dst.
	// Returns CloneResult with degradation info if applicable.
	Clone(src, dst string) (*CloneResult, error)
}

// NewCloneResult initializes result metadata for engineType.
func NewCloneResult(engineType model.EngineType) *CloneResult {
	return &CloneResult{
		ActualEngine:         engineType,
		EffectiveEngine:      engineType,
		MetadataPreservation: MetadataPreservationForEngine(engineType),
		PerformanceClass:     PerformanceClassForEngine(engineType),
	}
}

// AddDegradation records a degradation and updates the effective engine when
// the operation fell back to a different public class.
func (r *CloneResult) AddDegradation(reason string, effectiveEngine model.EngineType) {
	if r == nil {
		return
	}
	r.Degraded = true
	if reason != "" {
		r.Degradations = appendUnique(r.Degradations, reason)
	}
	if effectiveEngine != "" {
		r.EffectiveEngine = effectiveEngine
		r.MetadataPreservation = MetadataPreservationForEngine(effectiveEngine)
		r.PerformanceClass = PerformanceClassForEngine(effectiveEngine)
	}
}

func appendUnique(base []string, value string) []string {
	for _, existing := range base {
		if existing == value {
			return base
		}
	}
	return append(base, value)
}
