package cli

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	clidoctor "github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type publicSavePointRecord struct {
	SavePointID string    `json:"save_point_id"`
	Workspace   string    `json:"workspace"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type publicSavePointCreatedRecord struct {
	transfer.Data
	SavePointID          string                     `json:"save_point_id"`
	Workspace            string                     `json:"workspace"`
	Message              string                     `json:"message"`
	CreatedAt            time.Time                  `json:"created_at"`
	NewestSavePoint      string                     `json:"newest_save_point"`
	StartedFromSavePoint string                     `json:"started_from_save_point,omitempty"`
	RestoredFrom         string                     `json:"restored_from,omitempty"`
	RestoredPaths        []publicRestoredPathSource `json:"restored_paths,omitempty"`
	UnsavedChanges       bool                       `json:"unsaved_changes"`
}

type publicDoctorResult struct {
	Healthy  bool                  `json:"healthy"`
	Findings []publicDoctorFinding `json:"findings"`
	Repairs  []publicDoctorRepair  `json:"repairs,omitempty"`
}

type publicDoctorFinding struct {
	Category               string `json:"category"`
	Description            string `json:"description"`
	Severity               string `json:"severity"`
	ErrorCode              string `json:"error_code,omitempty"`
	RecommendedNextCommand string `json:"recommended_next_command,omitempty"`
}

type publicDoctorRepair struct {
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Cleaned int    `json:"cleaned,omitempty"`
}

type publicCleanupPlan struct {
	PlanID                   string                         `json:"plan_id"`
	CreatedAt                time.Time                      `json:"created_at"`
	ProtectedSavePoints      []string                       `json:"protected_save_points"`
	ProtectionGroups         []publicCleanupProtectionGroup `json:"protection_groups"`
	ProtectedByHistory       int                            `json:"protected_by_history"`
	CandidateCount           int                            `json:"candidate_count"`
	ReclaimableSavePoints    []string                       `json:"reclaimable_save_points"`
	ReclaimableBytesEstimate int64                          `json:"reclaimable_bytes_estimate"`
}

type publicCleanupProtectionGroup struct {
	Reason     string   `json:"reason"`
	Count      int      `json:"count"`
	SavePoints []string `json:"save_points"`
}

type publicRestoredPathSource struct {
	TargetPath      string                 `json:"target_path"`
	SourceSavePoint string                 `json:"source_save_point"`
	SourcePath      string                 `json:"source_path"`
	Status          model.PathSourceStatus `json:"status"`
}

func publicSavePoint(desc *model.Descriptor) publicSavePointRecord {
	return publicSavePointRecord{
		SavePointID: string(desc.SnapshotID),
		Workspace:   desc.WorktreeName,
		Message:     desc.Note,
		CreatedAt:   desc.CreatedAt,
	}
}

func publicSavePoints(descs []*model.Descriptor) []publicSavePointRecord {
	records := make([]publicSavePointRecord, 0, len(descs))
	for _, desc := range descs {
		records = append(records, publicSavePoint(desc))
	}
	return records
}

func publicSavePointCreated(desc *model.Descriptor, unsavedChanges bool, transferData transfer.Data) publicSavePointCreatedRecord {
	record := publicSavePointCreatedRecord{
		Data:            transferData,
		SavePointID:     string(desc.SnapshotID),
		Workspace:       desc.WorktreeName,
		Message:         desc.Note,
		CreatedAt:       desc.CreatedAt,
		NewestSavePoint: string(desc.SnapshotID),
		UnsavedChanges:  unsavedChanges,
	}
	if desc.RestoredFrom != nil {
		record.RestoredFrom = string(*desc.RestoredFrom)
	}
	if desc.StartedFrom != nil {
		record.StartedFromSavePoint = string(*desc.StartedFrom)
	}
	record.RestoredPaths = publicRestoredPathSources(desc.RestoredPaths)
	return record
}

func publicJSONData(data any) (any, error) {
	if data == nil {
		return nil, nil
	}
	state := &publicJSONState{seen: map[publicJSONVisit]bool{}}
	return publicJSONValue(state, reflect.ValueOf(data), "")
}

type publicJSONObjectOverlay struct {
	data          any
	fields        map[string]any
	defaultFields map[string]any
	objectError   string
}

func publicJSONDataWithObjectFields(data any, fields, defaultFields map[string]any, objectError string) any {
	return publicJSONObjectOverlay{
		data:          data,
		fields:        fields,
		defaultFields: defaultFields,
		objectError:   objectError,
	}
}

func publicJSONValue(state *publicJSONState, value reflect.Value, jsonName string) (any, error) {
	if jsonName == "transfers" {
		return publicJSONTransfers(value)
	}

	value = publicJSONUnwrapInterface(value)
	if !value.IsValid() {
		return nil, nil
	}
	if overlay, ok := publicJSONObjectOverlayFromValue(value); ok {
		return publicJSONOverlayObject(state, overlay)
	}
	if publicJSONUsesCustomMarshaler(value) {
		return value.Interface(), nil
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil, nil
		}
		leave, err := state.enter(value)
		if err != nil {
			return nil, err
		}
		defer leave()
		return publicJSONValue(state, value.Elem(), jsonName)
	case reflect.Struct:
		return publicJSONStruct(state, value)
	case reflect.Map:
		leave, err := state.enter(value)
		if err != nil {
			return nil, err
		}
		defer leave()
		return publicJSONMap(state, value)
	case reflect.Slice:
		if value.IsNil() {
			return nil, nil
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return value.Interface(), nil
		}
		leave, err := state.enter(value)
		if err != nil {
			return nil, err
		}
		defer leave()
		return publicJSONSlice(state, value)
	case reflect.Array:
		return publicJSONSlice(state, value)
	default:
		return value.Interface(), nil
	}
}

func publicJSONObjectOverlayFromValue(value reflect.Value) (publicJSONObjectOverlay, bool) {
	if !value.IsValid() || !value.CanInterface() {
		return publicJSONObjectOverlay{}, false
	}
	switch overlay := value.Interface().(type) {
	case publicJSONObjectOverlay:
		return overlay, true
	case *publicJSONObjectOverlay:
		if overlay == nil {
			return publicJSONObjectOverlay{}, false
		}
		return *overlay, true
	default:
		return publicJSONObjectOverlay{}, false
	}
}

func publicJSONOverlayObject(state *publicJSONState, overlay publicJSONObjectOverlay) (map[string]any, error) {
	baseValue := reflect.ValueOf(overlay.data)
	publicBase, err := publicJSONValue(state, baseValue, "")
	if err != nil {
		return nil, err
	}

	base, ok := publicBase.(map[string]any)
	if !ok {
		if publicBase == nil && publicJSONOverlayNilStringMap(baseValue) {
			base = map[string]any{}
		} else {
			if overlay.objectError != "" {
				return nil, errors.New(overlay.objectError)
			}
			return nil, errors.New("public JSON data with object fields must be an object")
		}
	}

	out := make(map[string]any, len(base)+len(overlay.defaultFields)+len(overlay.fields))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay.defaultFields {
		if _, exists := out[key]; exists {
			continue
		}
		publicValue, err := publicJSONValue(state, reflect.ValueOf(value), key)
		if err != nil {
			return nil, err
		}
		out[key] = publicValue
	}
	for key, value := range overlay.fields {
		publicValue, err := publicJSONValue(state, reflect.ValueOf(value), key)
		if err != nil {
			return nil, err
		}
		out[key] = publicValue
	}
	return out, nil
}

func publicJSONOverlayNilStringMap(value reflect.Value) bool {
	value = publicJSONUnwrapInterface(value)
	return value.IsValid() && value.Kind() == reflect.Map && value.IsNil() && value.Type().Key().Kind() == reflect.String
}

type publicJSONState struct {
	seen map[publicJSONVisit]bool
}

type publicJSONVisit struct {
	typ reflect.Type
	ptr uintptr
}

func (s *publicJSONState) enter(value reflect.Value) (func(), error) {
	key, ok := publicJSONVisitKey(value)
	if !ok {
		return func() {}, nil
	}
	if s.seen[key] {
		return nil, fmt.Errorf("encode public JSON data: cyclic value")
	}
	s.seen[key] = true
	return func() {
		delete(s.seen, key)
	}, nil
}

func publicJSONVisitKey(value reflect.Value) (publicJSONVisit, bool) {
	switch value.Kind() {
	case reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return publicJSONVisit{}, false
		}
		ptr := value.Pointer()
		if ptr == 0 {
			return publicJSONVisit{}, false
		}
		return publicJSONVisit{typ: value.Type(), ptr: ptr}, true
	default:
		return publicJSONVisit{}, false
	}
}

func publicJSONUnwrapInterface(value reflect.Value) reflect.Value {
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func publicJSONUsesCustomMarshaler(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	if value.CanInterface() {
		if _, ok := value.Interface().(json.Marshaler); ok {
			return true
		}
		if _, ok := value.Interface().(encoding.TextMarshaler); ok {
			return true
		}
	}
	if value.CanAddr() {
		address := value.Addr()
		if address.CanInterface() {
			if _, ok := address.Interface().(json.Marshaler); ok {
				return true
			}
			if _, ok := address.Interface().(encoding.TextMarshaler); ok {
				return true
			}
		}
	}
	return false
}

func publicJSONStruct(state *publicJSONState, value reflect.Value) (map[string]any, error) {
	out := map[string]any{}
	valueType := value.Type()
	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		fieldName, tagOptions := publicJSONParseTag(field.Tag.Get("json"))
		if fieldName == "-" {
			continue
		}
		flattenAnonymous := publicJSONFlattensAnonymousField(field, fieldName)
		if field.PkgPath != "" && !flattenAnonymous {
			continue
		}

		fieldValue := value.Field(i)
		if flattenAnonymous {
			publicValue, err := publicJSONValue(state, fieldValue, "")
			if err != nil {
				return nil, err
			}
			if publicMap, ok := publicValue.(map[string]any); ok {
				for key, value := range publicMap {
					out[key] = value
				}
			}
			continue
		}

		if fieldName == "" {
			fieldName = field.Name
		}
		if tagOptions.Contains("omitempty") && publicJSONIsEmptyValue(fieldValue) {
			continue
		}
		publicValue, err := publicJSONValue(state, fieldValue, fieldName)
		if err != nil {
			return nil, err
		}
		out[fieldName] = publicValue
	}
	return out, nil
}

func publicJSONMap(state *publicJSONState, value reflect.Value) (any, error) {
	if value.IsNil() {
		return nil, nil
	}
	if value.Type().Key().Kind() != reflect.String {
		return value.Interface(), nil
	}
	out := make(map[string]any, value.Len())
	iter := value.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		publicValue, err := publicJSONValue(state, iter.Value(), key)
		if err != nil {
			return nil, err
		}
		out[key] = publicValue
	}
	return out, nil
}

func publicJSONSlice(state *publicJSONState, value reflect.Value) ([]any, error) {
	out := make([]any, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		publicValue, err := publicJSONValue(state, value.Index(i), "")
		if err != nil {
			return nil, err
		}
		out = append(out, publicValue)
	}
	return out, nil
}

func publicJSONTransfers(value reflect.Value) ([]any, error) {
	value = publicJSONUnwrapInterface(value)
	if !value.IsValid() {
		return nil, fmt.Errorf("public JSON transfers must be an array")
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("public JSON transfers must be an array")
		}
		value = publicJSONUnwrapInterface(value.Elem())
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return nil, fmt.Errorf("public JSON transfers must be an array")
	}
	if value.Kind() == reflect.Slice && value.IsNil() {
		return nil, fmt.Errorf("public JSON transfers must be an array")
	}

	out := make([]any, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		publicMap, err := publicJSONTransferRecordMap(value.Index(i))
		if err != nil {
			return nil, err
		}
		out = append(out, publicMap)
	}
	return out, nil
}

func publicJSONTransferRecordMap(value reflect.Value) (map[string]any, error) {
	value = publicJSONUnwrapInterface(value)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("public JSON transfer must be an object")
		}
		value = publicJSONUnwrapInterface(value.Elem())
	}
	if !value.IsValid() || !value.CanInterface() {
		return nil, fmt.Errorf("public JSON transfer must be an object")
	}

	switch record := value.Interface().(type) {
	case transfer.Record:
		return publicTransferRecordMap(transfer.PublicRecordFromRecord(record))
	case transfer.PublicRecord:
		return publicTransferRecordMap(record)
	}

	raw, err := json.Marshal(value.Interface())
	if err != nil {
		return nil, fmt.Errorf("encode public transfer record: %w", err)
	}
	var record transfer.Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("decode public transfer record: %w", err)
	}
	return publicTransferRecordMap(transfer.PublicRecordFromRecord(record))
}

func publicTransferRecordMap(record transfer.PublicRecord) (map[string]any, error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode public transfer record: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode public transfer record: %w", err)
	}
	return out, nil
}

type publicJSONTagOptions string

func publicJSONParseTag(tag string) (string, publicJSONTagOptions) {
	if comma := strings.Index(tag, ","); comma >= 0 {
		return tag[:comma], publicJSONTagOptions(tag[comma+1:])
	}
	return tag, ""
}

func (o publicJSONTagOptions) Contains(option string) bool {
	if len(o) == 0 {
		return false
	}
	for len(o) > 0 {
		var next string
		if comma := strings.Index(string(o), ","); comma >= 0 {
			next = string(o[:comma])
			o = o[comma+1:]
		} else {
			next = string(o)
			o = ""
		}
		if next == option {
			return true
		}
	}
	return false
}

func publicJSONFlattensAnonymousField(field reflect.StructField, fieldName string) bool {
	if !field.Anonymous || fieldName != "" {
		return false
	}
	fieldType := field.Type
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	if fieldType.Kind() != reflect.Struct {
		return false
	}
	if fieldType.Implements(publicJSONMarshalerType) || reflect.PointerTo(fieldType).Implements(publicJSONMarshalerType) {
		return false
	}
	if fieldType.Implements(publicTextMarshalerType) || reflect.PointerTo(fieldType).Implements(publicTextMarshalerType) {
		return false
	}
	return true
}

func publicJSONIsEmptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return value.IsNil()
	default:
		return false
	}
}

var (
	publicJSONMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	publicTextMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

func publicRestoredPathSources(sources []model.RestoredPathSource) []publicRestoredPathSource {
	if len(sources) == 0 {
		return nil
	}
	records := make([]publicRestoredPathSource, 0, len(sources))
	for _, source := range sources {
		records = append(records, publicRestoredPathSource{
			TargetPath:      source.TargetPath,
			SourceSavePoint: string(source.SourceSnapshotID),
			SourcePath:      source.SourcePath,
			Status:          source.Status,
		})
	}
	return records
}

func publicDoctor(result *clidoctor.Result) publicDoctorResult {
	findings := make([]publicDoctorFinding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, publicDoctorFinding{
			Category:               publicContractVocabulary(finding.Category),
			Description:            publicContractVocabulary(finding.Description),
			Severity:               finding.Severity,
			ErrorCode:              publicErrorCodeVocabulary(finding.ErrorCode),
			RecommendedNextCommand: finding.RecommendedNextCommand,
		})
	}
	return publicDoctorResult{
		Healthy:  result.Healthy,
		Findings: findings,
	}
}

func publicDoctorWithRepairs(result *clidoctor.Result, repairs []clidoctor.RepairResult) publicDoctorResult {
	record := publicDoctor(result)
	if repairs == nil {
		return record
	}
	record.Repairs = make([]publicDoctorRepair, 0, len(repairs))
	for _, repair := range repairs {
		record.Repairs = append(record.Repairs, publicDoctorRepair{
			Action:  repair.Action,
			Success: repair.Success,
			Message: publicContractVocabulary(repair.Message),
			Cleaned: repair.Cleaned,
		})
	}
	return record
}

func publicCleanup(plan *model.GCPlan) (publicCleanupPlan, error) {
	protectionGroups, err := publicCleanupProtectionGroups(plan.ProtectionGroups)
	if err != nil {
		return publicCleanupPlan{}, err
	}
	return publicCleanupPlan{
		PlanID:                   plan.PlanID,
		CreatedAt:                plan.CreatedAt,
		ProtectedSavePoints:      publicSnapshotIDs(plan.ProtectedSet),
		ProtectionGroups:         protectionGroups,
		ProtectedByHistory:       cleanupProtectionGroupCount(protectionGroups, model.GCProtectionReasonHistory, plan.ProtectedByLineage),
		CandidateCount:           plan.CandidateCount,
		ReclaimableSavePoints:    publicSnapshotIDs(plan.ToDelete),
		ReclaimableBytesEstimate: plan.DeletableBytesEstimate,
	}, nil
}

func publicCleanupProtectionGroups(groups []model.GCProtectionGroup) ([]publicCleanupProtectionGroup, error) {
	out := make([]publicCleanupProtectionGroup, 0, len(groups))
	for _, group := range groups {
		reason, err := publicCleanupProtectionReason(group.Reason)
		if err != nil {
			return nil, err
		}
		out = append(out, publicCleanupProtectionGroup{
			Reason:     reason,
			Count:      group.Count,
			SavePoints: publicSnapshotIDs(group.SavePoints),
		})
	}
	return out, nil
}

func publicCleanupProtectionReason(reason string) (string, error) {
	switch reason {
	case model.GCProtectionReasonHistory,
		model.GCProtectionReasonOpenView,
		model.GCProtectionReasonActiveRecovery,
		model.GCProtectionReasonActiveOperation,
		model.GCProtectionReasonImportedCloneHistory:
		return reason, nil
	default:
		return "", fmt.Errorf("cleanup plan contains unsupported cleanup protection reason")
	}
}

func cleanupProtectionGroupCount(groups []publicCleanupProtectionGroup, reason string, fallback int) int {
	for _, group := range groups {
		if group.Reason == reason {
			return group.Count
		}
	}
	return fallback
}

func publicSnapshotIDs(ids []model.SnapshotID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func publicErrorCodeVocabulary(code string) string {
	switch code {
	case "E_WORKTREE_PAYLOAD_INVALID":
		return "E_WORKSPACE_PATH_BINDING_INVALID"
	case "E_WORKTREE_PAYLOAD_MISSING":
		return errclass.ErrWorkspaceMissing.Code
	case "E_CONTROL_PAYLOAD_OVERLAP":
		return errclass.ErrControlWorkspaceOverlap.Code
	case "E_PAYLOAD_INSIDE_CONTROL":
		return errclass.ErrWorkspaceInsideControl.Code
	case "E_CONTROL_INSIDE_PAYLOAD":
		return errclass.ErrControlInsideWorkspace.Code
	case "E_PAYLOAD_LOCATOR_PRESENT":
		return errclass.ErrWorkspaceControlMarkerPresent.Code
	case "E_PAYLOAD_MISSING":
		return "E_SAVE_POINT_MISSING"
	case "E_PAYLOAD_INVALID":
		return "E_SAVE_POINT_INVALID"
	case "E_PAYLOAD_HASH_MISMATCH":
		return errclass.ErrSavePointHashMismatch.Code
	}
	code = strings.ReplaceAll(code, "WORKTREE", "WORKSPACE")
	code = strings.ReplaceAll(code, "SNAPSHOT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "CHECKPOINT", "SAVE_POINT")
	code = strings.ReplaceAll(code, "GC", "CLEANUP")
	code = strings.ReplaceAll(code, "_HEAD_", "_SOURCE_")
	return code
}

func publicContractVocabulary(value string) string {
	value = strings.NewReplacer(
		"head_snapshot_id", "content_source",
		"latest_snapshot_id", "newest_save_point",
		"base_snapshot_id", "started_from_save_point",
		"orphan intent files", "stale operation records",
		"intent files", "operation records",
		"intents directory", "operations directory",
		"intents", "operations",
		"intent", "operation",
		"head snapshot", "content source save point",
		"latest snapshot", "newest save point",
		"base snapshot", "started from save point",
		"snapshot_id", "save_point_id",
		"checkpoints", "save points",
		"checkpoint", "save point",
		"snapshots", "save points",
		"snapshot", "save point",
		"Checkpoints", "Save points",
		"Checkpoint", "Save point",
		"Snapshots", "Save points",
		"Snapshot", "Save point",
		"GC", "Cleanup",
		"gc", "cleanup",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"Worktrees", "Workspaces",
		"Worktree", "Workspace",
	).Replace(value)
	return strings.NewReplacer(
		"payload_root_hash", "save_point_hash",
		"payload path is bound", "folder path is bound",
		"payload path invalid", "folder path invalid",
		"payload directory missing", "folder missing",
		"compute payload hash", "compute save point hash",
		"payload hash mismatch", "save point hash mismatch",
		"payload missing", "save point storage missing",
		"READY payload_root_hash", "READY save_point_hash",
		"Payload path is bound", "Folder path is bound",
		"Payload path invalid", "Folder path invalid",
		"Payload directory missing", "Folder missing",
		"Compute payload hash", "Compute save point hash",
		"Payload hash mismatch", "Save point hash mismatch",
		"Payload missing", "Save point storage missing",
	).Replace(value)
}
