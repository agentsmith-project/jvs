package transfer

import (
	"regexp"
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

// PublicRecord is the public data.transfers[] entry exported by CLI JSON.
// Internal records keep storage paths and planner roles; public records expose
// user-facing roles and location references instead of promising .jvs layout.
type PublicRecord struct {
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

// PublicData is the public JSON payload shape for transfer-capable command
// data after internal transfer records have been exported.
type PublicData struct {
	Transfers []PublicRecord `json:"transfers"`
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
		PreviewOnly:     i.PermissionScope == PermissionScopePreviewOnly,
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

// PublicRecordFromRecord exports a transfer record for CLI JSON. It preserves
// operation identity and copy evidence, while converting storage roles and
// paths into stable public vocabulary.
func PublicRecordFromRecord(record Record) PublicRecord {
	return PublicRecord{
		TransferID:                 record.TransferID,
		Operation:                  publicTransferToken(record.Operation),
		Phase:                      publicTransferToken(record.Phase),
		Primary:                    record.Primary,
		ResultKind:                 record.ResultKind,
		PermissionScope:            record.PermissionScope,
		SourceRole:                 PublicRole(record.SourceRole),
		SourcePath:                 publicTransferLocation(record, record.SourcePath, publicLocationSource),
		DestinationRole:            PublicRole(record.DestinationRole),
		MaterializationDestination: publicTransferLocation(record, record.MaterializationDestination, publicLocationMaterialization),
		CapabilityProbePath:        publicTransferLocation(record, record.CapabilityProbePath, publicLocationCapabilityProbe),
		PublishedDestination:       publicTransferLocation(record, record.PublishedDestination, publicLocationPublished),
		CheckedForThisOperation:    record.CheckedForThisOperation,
		RequestedEngine:            record.RequestedEngine,
		EffectiveEngine:            record.EffectiveEngine,
		OptimizedTransfer:          record.OptimizedTransfer,
		PerformanceClass:           record.PerformanceClass,
		DegradedReasons:            publicStringList(record.DegradedReasons),
		Warnings:                   publicStringList(record.Warnings),
	}
}

// PublicRecordsFromRecords exports a slice of internal records for CLI JSON.
func PublicRecordsFromRecords(records []Record) []PublicRecord {
	if records == nil {
		return nil
	}
	out := make([]PublicRecord, 0, len(records))
	for _, record := range records {
		out = append(out, PublicRecordFromRecord(record))
	}
	return out
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

// PublicRole returns the transfer role token used by public CLI JSON.
func PublicRole(role string) string {
	switch role {
	case "save_point_payload":
		return "save_point_content"
	case "save_point_staging":
		return "save_point_content"
	case "view_directory":
		return "content_view"
	case "restore_staging", "restore_preview_validation", "restore_source_validation":
		return "temporary_folder"
	case "recovery_restore_staging":
		return "temporary_folder"
	default:
		return publicTransferToken(role)
	}
}

type publicLocationField string

const (
	publicLocationSource          publicLocationField = "source"
	publicLocationMaterialization publicLocationField = "materialization"
	publicLocationCapabilityProbe publicLocationField = "capability_probe"
	publicLocationPublished       publicLocationField = "published"
)

func publicTransferLocation(record Record, raw string, field publicLocationField) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if ref, ok := contentViewLocationRef(raw); ok {
		return ref
	}
	if field == publicLocationMaterialization && isTemporaryTransferLocation(record, raw) {
		return "temporary_folder"
	}
	if ref, ok := savePointLocationRef(raw); ok {
		return ref
	}
	if isControlDataLocation(raw) {
		switch field {
		case publicLocationCapabilityProbe, publicLocationSource, publicLocationPublished:
			return "control_data"
		case publicLocationMaterialization:
			return "temporary_folder"
		}
	}
	return raw
}

func publicStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, publicTransferFreeText(value))
	}
	return out
}

func publicTransferToken(value string) string {
	return strings.NewReplacer(
		"save_point_payload", "save_point_content",
		"payload", "content",
		"snapshot", "save_point",
		"worktree", "workspace",
	).Replace(value)
}

var (
	publicInternalPathPattern = regexp.MustCompile(`(?i)(?:[A-Za-z]:)?(?:[\\/][^\s,;:"')\]}]+)*[\\/]\.jvs(?:[A-Za-z0-9._-]*)?(?:[\\/][^\s,;:"')\]}]+)*`)
	publicJVSTokenPattern     = regexp.MustCompile(`(?i)\.jvs(?:[A-Za-z0-9._-]*)?`)
	publicRawDiagnosticMarker = regexp.MustCompile(`(?i)\b(?:stderr|stdout)\s*:`)
	publicFreeTextTerms       = []publicFreeTextReplacement{
		{regexp.MustCompile(`(?i)\bsave_point_payload\b`), "save_point_content"},
		{regexp.MustCompile(`(?i)\bsave point payload\b`), "save point content"},
		{regexp.MustCompile(`(?i)\bpayloads\b`), "save point contents"},
		{regexp.MustCompile(`(?i)\bpayload\b`), "save point content"},
		{regexp.MustCompile(`(?i)\bsnapshots\b`), "save points"},
		{regexp.MustCompile(`(?i)\bsnapshot\b`), "save point"},
		{regexp.MustCompile(`(?i)\bworktrees\b`), "workspaces"},
		{regexp.MustCompile(`(?i)\bworktree\b`), "workspace"},
	}
)

type publicFreeTextReplacement struct {
	pattern     *regexp.Regexp
	replacement string
}

func publicTransferFreeText(value string) string {
	value = publicTransferDiagnosticSummary(value)
	value = publicInternalPathPattern.ReplaceAllStringFunc(value, publicInternalPathSummary)
	value = publicJVSTokenPattern.ReplaceAllString(value, "internal storage")
	for _, replacement := range publicFreeTextTerms {
		value = replacement.pattern.ReplaceAllString(value, replacement.replacement)
	}
	return strings.TrimSpace(value)
}

func publicTransferDiagnosticSummary(value string) string {
	value = strings.TrimSpace(value)
	match := publicRawDiagnosticMarker.FindStringIndex(value)
	if match == nil {
		return value
	}
	prefix := strings.TrimSpace(value[:match[0]])
	prefix = stripPublicDiagnosticContext(prefix)
	prefix = strings.TrimRight(prefix, " \t\r\n:-([")
	if prefix == "" {
		return "engine diagnostic redacted"
	}
	return prefix + ": engine diagnostic redacted"
}

func stripPublicDiagnosticContext(value string) string {
	for {
		trimmed := strings.TrimSpace(value)
		lower := strings.ToLower(trimmed)
		const contextSuffix = "juicefs-clone-context:"
		if !strings.HasSuffix(lower, contextSuffix) {
			return trimmed
		}
		value = trimmed[:len(trimmed)-len(contextSuffix)]
	}
}

func publicInternalPathSummary(raw string) string {
	if _, ok := savePointLocationRef(raw); ok {
		return "save point content"
	}
	if _, ok := contentViewLocationRef(raw); ok {
		return "content view"
	}
	return "internal storage path"
}

func isTemporaryTransferLocation(record Record, raw string) bool {
	if raw != "" && record.PublishedDestination != "" && raw != record.PublishedDestination {
		return true
	}
	slash := slashPath(raw)
	return strings.Contains(slash, "/.restore-tmp-") ||
		strings.Contains(slash, "/.restore-path-tmp-") ||
		strings.Contains(slash, "/.jvs/tmp/") ||
		strings.Contains(slash, "/.jvs/restore-preview-") ||
		strings.Contains(slash, "/.jvs/restore-run-validation-")
}

func savePointLocationRef(raw string) (string, bool) {
	segments := pathSegments(raw)
	for i := 0; i+2 < len(segments); i++ {
		if segments[i] != ".jvs" || segments[i+1] != "snapshots" {
			continue
		}
		id := strings.TrimSpace(segments[i+2])
		if id == "" {
			return "", false
		}
		tail := segments[i+3:]
		if len(tail) > 0 && tail[0] == "payload" {
			tail = tail[1:]
		}
		return appendPublicRefPath("save_point:"+id, tail), true
	}
	return "", false
}

func contentViewLocationRef(raw string) (string, bool) {
	segments := pathSegments(raw)
	for i := 0; i+2 < len(segments); i++ {
		if segments[i] != ".jvs" || segments[i+1] != "views" {
			continue
		}
		viewID := strings.TrimSpace(segments[i+2])
		if viewID == "" {
			return "content_view", true
		}
		tail := segments[i+3:]
		if len(tail) > 0 && (tail[0] == "payload" || tail[0] == "content") {
			tail = tail[1:]
		}
		return appendPublicRefPath("content_view:"+viewID, tail), true
	}
	return "", false
}

func isControlDataLocation(raw string) bool {
	segments := pathSegments(raw)
	for _, segment := range segments {
		if segment == ".jvs" {
			return true
		}
	}
	return false
}

func pathSegments(raw string) []string {
	parts := strings.Split(slashPath(raw), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

func slashPath(raw string) string {
	return strings.ReplaceAll(raw, "\\", "/")
}

func appendPublicRefPath(ref string, tail []string) string {
	if len(tail) == 0 {
		return ref
	}
	return ref + "/" + strings.Join(tail, "/")
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
