package saveprofile

import (
	"time"

	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/model"
)

const schemaVersion = 1

var publicPhaseNames = map[string]struct{}{
	"separated_boundary_precheck":          {},
	"mutation_lock":                        {},
	"recovery_guard":                       {},
	"capacity_check":                       {},
	"separated_boundary_final_check":       {},
	"save_point_create_total":              {},
	"workspace_load":                       {},
	"managed_content_boundary_resolve":     {},
	"path_source_reconcile":                {},
	"audit_appendability_precheck":         {},
	"workspace_evidence_pre_hash":          {},
	"create_intent":                        {},
	"transfer_plan":                        {},
	"content_clone":                        {},
	"staged_content_hash":                  {},
	"descriptor_checksum":                  {},
	"ready_marker":                         {},
	"compression":                          {},
	"staged_fsync":                         {},
	"workspace_evidence_post_hash":         {},
	"publish_save_point":                   {},
	"audit_appendability_history_precheck": {},
	"workspace_history_update":             {},
	"audit_append":                         {},
	"workspace_dirty_check":                {},
}

var publicAggregateCountKeys = map[string]struct{}{
	"entries":     {},
	"files":       {},
	"directories": {},
	"symlinks":    {},
	"bytes":       {},
}

// Profile is the stable public save --json timing surface. It intentionally
// records roles, engines, durations, and counts, but never filesystem paths.
type Profile struct {
	SchemaVersion     int                         `json:"schema_version"`
	RequestedEngine   model.EngineType            `json:"requested_engine"`
	EffectiveEngine   model.EngineType            `json:"effective_engine,omitempty"`
	OptimizedTransfer bool                        `json:"optimized_transfer"`
	CloneMode         string                      `json:"clone_mode,omitempty"`
	PerformanceClass  string                      `json:"performance_class,omitempty"`
	TotalDurationMS   int64                       `json:"total_duration_ms"`
	PhaseDurationsMS  map[string]int64            `json:"phase_durations_ms"`
	PhaseCounts       map[string]map[string]int64 `json:"phase_counts,omitempty"`
}

// Recorder accumulates save profile observations across CLI and snapshot
// layers without exposing private path data.
type Recorder struct {
	requestedEngine model.EngineType
	started         time.Time
	durations       map[string]time.Duration
	counts          map[string]map[string]int64
}

func New(requestedEngine model.EngineType) *Recorder {
	return &Recorder{
		requestedEngine: requestedEngine,
		started:         time.Now(),
		durations:       map[string]time.Duration{},
		counts:          map[string]map[string]int64{},
	}
}

func (r *Recorder) Step(name string, fn func() error) error {
	if r == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	r.AddDuration(name, time.Since(start))
	return err
}

func (r *Recorder) AddPhase(name string, duration time.Duration, counts map[string]int64) {
	if r == nil {
		return
	}
	r.AddDuration(name, duration)
	r.AddCounts(name, counts)
}

func (r *Recorder) AddDuration(name string, duration time.Duration) {
	if r == nil || !isPublicPhaseName(name) {
		return
	}
	if duration < 0 {
		duration = 0
	}
	if r.durations == nil {
		r.durations = map[string]time.Duration{}
	}
	r.durations[name] += duration
}

func (r *Recorder) AddCounts(name string, counts map[string]int64) {
	if r == nil || !isPublicPhaseName(name) || len(counts) == 0 {
		return
	}
	if !hasPublicAggregateCountKey(counts) {
		return
	}
	if r.counts == nil {
		r.counts = map[string]map[string]int64{}
	}
	dst := r.counts[name]
	if dst == nil {
		dst = map[string]int64{}
		r.counts[name] = dst
	}
	for key, value := range counts {
		if !isPublicAggregateCountKey(key) {
			continue
		}
		if value < 0 {
			value = 0
		}
		dst[key] += value
	}
}

func (r *Recorder) Merge(profile Profile) {
	if r == nil {
		return
	}
	for name, ms := range profile.PhaseDurationsMS {
		r.AddDuration(name, time.Duration(ms)*time.Millisecond)
	}
	for name, counts := range profile.PhaseCounts {
		r.AddCounts(name, counts)
	}
}

func (r *Recorder) Profile(record *transfer.Record) Profile {
	if r == nil {
		profile := Profile{
			SchemaVersion:    schemaVersion,
			PhaseDurationsMS: map[string]int64{},
		}
		profile.ApplyTransfer(record)
		return profile
	}
	profile := Profile{
		SchemaVersion:    schemaVersion,
		RequestedEngine:  r.requestedEngine,
		TotalDurationMS:  durationMS(time.Since(r.started)),
		PhaseDurationsMS: map[string]int64{},
	}
	for name, duration := range r.durations {
		if !isPublicPhaseName(name) {
			continue
		}
		profile.PhaseDurationsMS[name] = durationMS(duration)
	}
	if len(r.counts) > 0 {
		for name, counts := range r.counts {
			if !isPublicPhaseName(name) {
				continue
			}
			copied := map[string]int64{}
			for key, value := range counts {
				if !isPublicAggregateCountKey(key) {
					continue
				}
				copied[key] = value
			}
			if len(copied) == 0 {
				continue
			}
			if profile.PhaseCounts == nil {
				profile.PhaseCounts = map[string]map[string]int64{}
			}
			profile.PhaseCounts[name] = copied
		}
	}
	profile.ApplyTransfer(record)
	return profile
}

func (p *Profile) ApplyTransfer(record *transfer.Record) {
	if p == nil || record == nil {
		return
	}
	if record.RequestedEngine != "" {
		p.RequestedEngine = record.RequestedEngine
	}
	p.EffectiveEngine = record.EffectiveEngine
	p.OptimizedTransfer = record.OptimizedTransfer
	p.CloneMode = string(record.EffectiveEngine)
	p.PerformanceClass = string(record.PerformanceClass)
}

func (p *Profile) ApplyDescriptorFallback(desc *model.Descriptor) {
	if p == nil || desc == nil {
		return
	}
	if p.EffectiveEngine == "" {
		p.EffectiveEngine = desc.EffectiveEngine
		if p.EffectiveEngine == "" {
			p.EffectiveEngine = desc.Engine
		}
	}
	if p.CloneMode == "" && p.EffectiveEngine != "" {
		p.CloneMode = string(p.EffectiveEngine)
	}
	if p.PerformanceClass == "" {
		p.PerformanceClass = desc.PerformanceClass
	}
}

func durationMS(duration time.Duration) int64 {
	if duration < 0 {
		return 0
	}
	return duration.Milliseconds()
}

func isPublicPhaseName(name string) bool {
	_, ok := publicPhaseNames[name]
	return ok
}

func isPublicAggregateCountKey(key string) bool {
	_, ok := publicAggregateCountKeys[key]
	return ok
}

func hasPublicAggregateCountKey(counts map[string]int64) bool {
	for key := range counts {
		if isPublicAggregateCountKey(key) {
			return true
		}
	}
	return false
}
