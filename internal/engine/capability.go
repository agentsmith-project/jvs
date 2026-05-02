package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/model"
)

const (
	CapabilityConfirmed = "confirmed"
	CapabilityUnknown   = "unknown"
	EngineAuto          = model.EngineType("auto")
)

// Capability describes whether one transfer mechanism can be used at a target.
type Capability struct {
	Available  bool     `json:"available"`
	Supported  bool     `json:"supported"`
	Confidence string   `json:"confidence"`
	Warnings   []string `json:"warnings"`
}

// CapabilityReport is a non-mutating report by default. Reflink support is
// only confirmed when writeProbe is true.
type CapabilityReport struct {
	TargetPath           string                     `json:"target_path"`
	ProbePath            string                     `json:"probe_path"`
	WriteProbe           bool                       `json:"write_probe"`
	Write                Capability                 `json:"write"`
	JuiceFS              Capability                 `json:"juicefs"`
	Reflink              Capability                 `json:"reflink"`
	Copy                 Capability                 `json:"copy"`
	RecommendedEngine    model.EngineType           `json:"recommended_engine"`
	MetadataPreservation model.MetadataPreservation `json:"metadata_preservation"`
	PerformanceClass     string                     `json:"performance_class"`
	Warnings             []string                   `json:"warnings"`
}

// CapabilityProber is the filesystem probing surface used by capability
// reporting and transfer planning. Tests can replace it without touching the
// actual filesystem.
type CapabilityProber interface {
	ProbeCapabilities(targetPath string, writeProbe bool) (*CapabilityReport, error)
	ProbeTransferPair(sourcePath, destinationPath string) (*TransferPairReport, error)
}

// TransferPairReport describes capabilities that depend on both source and
// destination paths.
type TransferPairReport struct {
	SourcePath      string     `json:"source_path"`
	DestinationPath string     `json:"destination_path"`
	Reflink         Capability `json:"reflink"`
	Warnings        []string   `json:"warnings,omitempty"`
}

// TransferPlanRequest contains the inputs needed to choose a transfer engine.
type TransferPlanRequest struct {
	SourcePath      string
	DestinationPath string
	CapabilityPath  string
	RequestedEngine model.EngineType
}

// TransferSelectionRequest is passed to a selector after destination
// capabilities are known.
type TransferSelectionRequest struct {
	RequestedEngine model.EngineType
	Capabilities    *CapabilityReport
}

// TransferSelection is a destination-level engine choice before pair probes.
type TransferSelection struct {
	TransferEngine  model.EngineType
	DegradedReasons []string
	Warnings        []string
}

// TransferSelector chooses the destination-level engine from a capability
// report and a requested engine.
type TransferSelector interface {
	SelectTransfer(TransferSelectionRequest) TransferSelection
}

// DefaultTransferSelector is the production selector.
type DefaultTransferSelector struct{}

// TransferPlanner plans source+destination transfers from shared capabilities.
type TransferPlanner struct {
	Prober   CapabilityProber
	Selector TransferSelector
}

// TransferPlan is the contract surfaced by CLI lifecycle operations.
type TransferPlan struct {
	RequestedEngine   model.EngineType  `json:"requested_engine"`
	TransferEngine    model.EngineType  `json:"transfer_engine"`
	EffectiveEngine   model.EngineType  `json:"effective_engine"`
	OptimizedTransfer bool              `json:"optimized_transfer"`
	Capabilities      *CapabilityReport `json:"capabilities,omitempty"`
	DegradedReasons   []string          `json:"degraded_reasons"`
	Warnings          []string          `json:"warnings,omitempty"`
}

type filesystemCapabilityProber struct{}

// ProbeCapabilities reports JuiceFS, reflink, and copy support for targetPath.
// With writeProbe=false it does not create test files.
func ProbeCapabilities(targetPath string, writeProbe bool) (*CapabilityReport, error) {
	return filesystemCapabilityProber{}.ProbeCapabilities(targetPath, writeProbe)
}

// ProbeCapabilities reports JuiceFS, reflink, copy, and generic write support.
func (filesystemCapabilityProber) ProbeCapabilities(targetPath string, writeProbe bool) (*CapabilityReport, error) {
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("resolve target path: %w", err)
	}
	target = filepath.Clean(target)

	probePath, warnings, err := existingProbeDir(target)
	if err != nil {
		return nil, err
	}

	report := &CapabilityReport{
		TargetPath: target,
		ProbePath:  probePath,
		WriteProbe: writeProbe,
		Warnings:   warnings,
	}

	report.Write, report.Copy = probeWriteAndCopy(probePath, writeProbe)
	probeDir, cleanup := createReusableProbeDir(probePath, writeProbe, report.Write)
	if cleanup != nil {
		defer cleanup()
	}
	report.JuiceFS = probeJuiceFS(probePath)
	report.Reflink = probeReflink(probeDir, writeProbe, report.Write)
	report.RecommendedEngine = recommendedEngine(report)
	report.MetadataPreservation = MetadataPreservationForEngine(report.RecommendedEngine)
	report.PerformanceClass = PerformanceClassForEngine(report.RecommendedEngine)
	report.Warnings = append(report.Warnings, report.Write.Warnings...)
	report.Warnings = append(report.Warnings, report.JuiceFS.Warnings...)
	report.Warnings = append(report.Warnings, report.Reflink.Warnings...)
	report.Warnings = append(report.Warnings, report.Copy.Warnings...)
	report.Warnings = uniqueStrings(report.Warnings)
	normalizeCapabilityReport(report)

	return report, nil
}

// ProbeTransferPair probes optimizations that require both source and
// destination. It avoids mutating the source path.
func (filesystemCapabilityProber) ProbeTransferPair(sourcePath, destinationPath string) (*TransferPairReport, error) {
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	source = filepath.Clean(source)
	destination, err := filepath.Abs(destinationPath)
	if err != nil {
		return nil, fmt.Errorf("resolve destination path: %w", err)
	}
	destination = filepath.Clean(destination)

	if info, err := os.Stat(destination); err != nil {
		return nil, fmt.Errorf("stat destination path: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("destination path is not a directory: %s", destination)
	}

	report := &TransferPairReport{
		SourcePath:      source,
		DestinationPath: destination,
		Reflink: Capability{
			Available:  true,
			Supported:  false,
			Confidence: CapabilityUnknown,
		},
	}

	srcFile, info, found, err := firstRegularFile(source)
	if err != nil {
		return nil, err
	}
	if !found {
		warning := "no regular source file available to confirm source/destination reflink"
		report.Reflink.Supported = true
		report.Reflink.Confidence = CapabilityUnknown
		report.Reflink.Warnings = append(report.Reflink.Warnings, warning)
		report.Warnings = append(report.Warnings, warning)
		return report, nil
	}

	tempDir, err := os.MkdirTemp(destination, ".jvs-transfer-pair-")
	if err != nil {
		warning := fmt.Sprintf("cannot create transfer pair probe directory: %v", err)
		report.Reflink.Confidence = CapabilityConfirmed
		report.Reflink.Warnings = append(report.Reflink.Warnings, warning)
		report.Warnings = append(report.Warnings, warning)
		return report, nil
	}
	defer os.RemoveAll(tempDir)

	dstFile := filepath.Join(tempDir, "reflink")
	if err := reflinkFile(srcFile, dstFile, info); err != nil {
		warning := fmt.Sprintf("reflink pair probe failed: %v", err)
		report.Reflink.Confidence = CapabilityConfirmed
		report.Reflink.Warnings = append(report.Reflink.Warnings, warning)
		report.Warnings = append(report.Warnings, warning)
		return report, nil
	}

	report.Reflink.Supported = true
	report.Reflink.Confidence = CapabilityConfirmed
	return report, nil
}

// SelectTransfer chooses an engine using destination capabilities.
func (DefaultTransferSelector) SelectTransfer(req TransferSelectionRequest) TransferSelection {
	report := req.Capabilities
	requested := req.RequestedEngine
	if requested == "" {
		requested = EngineAuto
	}
	if requested == EngineAuto {
		requested = report.RecommendedEngine
	}
	if requested == "" {
		requested = model.EngineCopy
	}

	switch requested {
	case model.EngineJuiceFSClone:
		if report.JuiceFS.Supported {
			return TransferSelection{TransferEngine: model.EngineJuiceFSClone}
		}
		return TransferSelection{
			TransferEngine:  model.EngineCopy,
			DegradedReasons: []string{"juicefs-clone unavailable at destination"},
			Warnings:        report.JuiceFS.Warnings,
		}
	case model.EngineReflinkCopy:
		if report.Reflink.Supported {
			return TransferSelection{TransferEngine: model.EngineReflinkCopy}
		}
		return TransferSelection{
			TransferEngine:  model.EngineCopy,
			DegradedReasons: []string{"reflink-copy unavailable at destination"},
			Warnings:        report.Reflink.Warnings,
		}
	case model.EngineCopy:
		return TransferSelection{TransferEngine: model.EngineCopy, Warnings: report.Copy.Warnings}
	default:
		return TransferSelection{
			TransferEngine:  model.EngineCopy,
			DegradedReasons: []string{fmt.Sprintf("unknown requested engine %q; using copy", req.RequestedEngine)},
		}
	}
}

// PlanTransfer uses destination write-probed capabilities plus pair probes to
// select an engine for source+destination transfer operations.
func PlanTransfer(req TransferPlanRequest) (*TransferPlan, error) {
	return TransferPlanner{}.PlanTransfer(req)
}

// PlanTransfer uses destination write-probed capabilities plus pair probes to
// select an engine for source+destination transfer operations.
func (p TransferPlanner) PlanTransfer(req TransferPlanRequest) (*TransferPlan, error) {
	prober := p.Prober
	if prober == nil {
		prober = filesystemCapabilityProber{}
	}
	selector := p.Selector
	if selector == nil {
		selector = DefaultTransferSelector{}
	}

	capabilityPath := req.CapabilityPath
	if capabilityPath == "" {
		capabilityPath = req.DestinationPath
	}
	report, err := prober.ProbeCapabilities(capabilityPath, true)
	if err != nil {
		return nil, err
	}

	requested := req.RequestedEngine
	if requested == "" {
		requested = EngineAuto
	}
	selection := selector.SelectTransfer(TransferSelectionRequest{
		RequestedEngine: requested,
		Capabilities:    report,
	})
	if selection.TransferEngine == "" {
		selection.TransferEngine = model.EngineCopy
	}

	plan := &TransferPlan{
		RequestedEngine:   requested,
		TransferEngine:    selection.TransferEngine,
		EffectiveEngine:   selection.TransferEngine,
		OptimizedTransfer: optimizedTransfer(selection.TransferEngine),
		Capabilities:      report,
		DegradedReasons:   uniqueStrings(selection.DegradedReasons),
		Warnings:          uniqueStrings(append(append([]string{}, report.Warnings...), selection.Warnings...)),
	}

	if plan.TransferEngine == model.EngineReflinkCopy {
		pairDestinationPath := transferPairProbeDestination(req, report)
		pair, err := prober.ProbeTransferPair(req.SourcePath, pairDestinationPath)
		if err != nil {
			plan.degradeToCopy(fmt.Sprintf("reflink-copy source/destination probe failed: %v", err))
		} else {
			plan.Warnings = uniqueStrings(append(plan.Warnings, pair.Warnings...))
			if !pair.Reflink.Supported {
				reason := "reflink-copy unavailable for source/destination pair"
				if detail := firstWarning(pair.Reflink.Warnings, pair.Warnings); detail != "" {
					reason += ": " + detail
				}
				plan.degradeToCopy(reason)
			}
		}
	}

	if plan.TransferEngine == model.EngineCopy && report.Copy.Confidence == CapabilityConfirmed && !report.Copy.Supported {
		return nil, fmt.Errorf("destination does not support copy writes: %s", strings.Join(report.Copy.Warnings, "; "))
	}

	plan.DegradedReasons = uniqueStrings(plan.DegradedReasons)
	plan.Warnings = uniqueStrings(plan.Warnings)
	return plan, nil
}

func transferPairProbeDestination(req TransferPlanRequest, report *CapabilityReport) string {
	destination := req.DestinationPath
	if destination == "" {
		return firstNonEmpty(reportProbePath(report), req.CapabilityPath)
	}
	info, err := os.Stat(destination)
	if err == nil && info.IsDir() {
		return destination
	}
	if err != nil && !os.IsNotExist(err) {
		return destination
	}
	return firstNonEmpty(reportProbePath(report), req.CapabilityPath, destination)
}

func reportProbePath(report *CapabilityReport) string {
	if report == nil {
		return ""
	}
	return report.ProbePath
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (p *TransferPlan) degradeToCopy(reason string) {
	p.TransferEngine = model.EngineCopy
	p.EffectiveEngine = model.EngineCopy
	p.OptimizedTransfer = false
	if reason != "" {
		p.DegradedReasons = append(p.DegradedReasons, reason)
	}
}

func existingProbeDir(target string) (string, []string, error) {
	info, err := os.Stat(target)
	if err == nil {
		if !info.IsDir() {
			return "", nil, fmt.Errorf("target path is not a directory: %s", target)
		}
		return target, nil, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("stat target path: %w", err)
	}

	probePath := filepath.Dir(target)
	for {
		info, err := os.Stat(probePath)
		if err == nil {
			if !info.IsDir() {
				return "", nil, fmt.Errorf("probe parent is not a directory: %s", probePath)
			}
			return probePath, []string{fmt.Sprintf("target path does not exist; probed existing parent %s", probePath)}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("stat probe parent: %w", err)
		}
		next := filepath.Dir(probePath)
		if next == probePath {
			return "", nil, fmt.Errorf("no existing probe parent for target path: %s", target)
		}
		probePath = next
	}
}

func probeJuiceFS(path string) Capability {
	eng := NewJuiceFSEngine()
	available := eng.isJuiceFSAvailable()
	onJuiceFS := eng.isOnJuiceFS(path)
	capability := Capability{
		Available:  available,
		Supported:  available && onJuiceFS,
		Confidence: CapabilityConfirmed,
	}
	if !available {
		capability.Warnings = append(capability.Warnings, "juicefs command not found")
	}
	if available && !onJuiceFS {
		capability.Warnings = append(capability.Warnings, "target path is not on a JuiceFS mount")
	}
	return capability
}

func probeWriteAndCopy(path string, writeProbe bool) (Capability, Capability) {
	writeCapability := Capability{
		Available:  true,
		Supported:  false,
		Confidence: CapabilityUnknown,
	}
	copyCapability := Capability{
		Available:  true,
		Supported:  false,
		Confidence: CapabilityUnknown,
	}
	if !writeProbe {
		writeCapability.Warnings = append(writeCapability.Warnings, "write support requires --write-probe to confirm")
		copyCapability.Warnings = append(copyCapability.Warnings, "copy support requires --write-probe to confirm destination writability")
		return writeCapability, copyCapability
	}

	writeCapability.Confidence = CapabilityConfirmed
	copyCapability.Confidence = CapabilityConfirmed
	tempDir, err := os.MkdirTemp(path, ".jvs-capability-")
	if err != nil {
		warning := fmt.Sprintf("cannot create write probe directory: %v", err)
		writeCapability.Warnings = append(writeCapability.Warnings, warning)
		copyCapability.Warnings = append(copyCapability.Warnings, warning)
		return writeCapability, copyCapability
	}
	defer os.RemoveAll(tempDir)

	probeFile := filepath.Join(tempDir, "write")
	if err := os.WriteFile(probeFile, []byte("jvs write probe"), 0600); err != nil {
		warning := fmt.Sprintf("cannot write probe file: %v", err)
		writeCapability.Warnings = append(writeCapability.Warnings, warning)
		copyCapability.Warnings = append(copyCapability.Warnings, warning)
		return writeCapability, copyCapability
	}
	if err := os.Remove(probeFile); err != nil {
		warning := fmt.Sprintf("cannot remove probe file: %v", err)
		writeCapability.Warnings = append(writeCapability.Warnings, warning)
		copyCapability.Warnings = append(copyCapability.Warnings, warning)
		return writeCapability, copyCapability
	}
	if err := os.Remove(tempDir); err != nil {
		warning := fmt.Sprintf("cannot remove write probe directory: %v", err)
		writeCapability.Warnings = append(writeCapability.Warnings, warning)
		copyCapability.Warnings = append(copyCapability.Warnings, warning)
		return writeCapability, copyCapability
	}

	writeCapability.Supported = true
	copyCapability.Supported = true
	return writeCapability, copyCapability
}

func createReusableProbeDir(path string, writeProbe bool, writeCapability Capability) (string, func()) {
	if !writeProbe || !writeCapability.Supported {
		return "", nil
	}
	tempDir, err := os.MkdirTemp(path, ".jvs-capability-")
	if err != nil {
		return "", nil
	}
	return tempDir, func() { _ = os.RemoveAll(tempDir) }
}

func probeReflink(probeDir string, writeProbe bool, writeCapability Capability) Capability {
	capability := Capability{
		Available:  true,
		Supported:  false,
		Confidence: CapabilityUnknown,
	}
	if !writeProbe {
		capability.Warnings = append(capability.Warnings, "reflink support requires --write-probe to confirm")
		return capability
	}
	if !writeCapability.Supported {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, "reflink support requires writable destination")
		return capability
	}
	if probeDir == "" {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, "cannot create reflink probe directory")
		return capability
	}

	src := filepath.Join(probeDir, "src")
	dst := filepath.Join(probeDir, "dst")
	if err := os.WriteFile(src, []byte("jvs reflink probe"), 0600); err != nil {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, fmt.Sprintf("cannot write reflink probe file: %v", err))
		return capability
	}
	info, err := os.Stat(src)
	if err != nil {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, fmt.Sprintf("cannot stat reflink probe file: %v", err))
		return capability
	}
	if err := reflinkFile(src, dst, info); err != nil {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, fmt.Sprintf("reflink probe failed: %v", err))
		return capability
	}

	capability.Supported = true
	capability.Confidence = CapabilityConfirmed
	return capability
}

func recommendedEngine(report *CapabilityReport) model.EngineType {
	switch {
	case report.JuiceFS.Supported:
		return model.EngineJuiceFSClone
	case report.Reflink.Supported:
		return model.EngineReflinkCopy
	case report.Copy.Supported || (report.Copy.Available && report.Copy.Confidence == CapabilityUnknown):
		return model.EngineCopy
	default:
		return ""
	}
}

func normalizeCapabilityReport(report *CapabilityReport) {
	if report == nil {
		return
	}
	report.Write = normalizeCapability(report.Write)
	report.JuiceFS = normalizeCapability(report.JuiceFS)
	report.Reflink = normalizeCapability(report.Reflink)
	report.Copy = normalizeCapability(report.Copy)
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
}

func normalizeCapability(capability Capability) Capability {
	if capability.Warnings == nil {
		capability.Warnings = []string{}
	}
	return capability
}

func firstRegularFile(root string) (string, os.FileInfo, bool, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", nil, false, fmt.Errorf("stat source path: %w", err)
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() {
			return root, info, true, nil
		}
		return "", nil, false, nil
	}

	var foundPath string
	var foundInfo os.FileInfo
	errStop := fmt.Errorf("found regular file")
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			foundPath = path
			foundInfo = info
			return errStop
		}
		return nil
	})
	if err != nil && err != errStop {
		return "", nil, false, fmt.Errorf("scan source for pair probe: %w", err)
	}
	return foundPath, foundInfo, foundPath != "", nil
}

func optimizedTransfer(engineType model.EngineType) bool {
	return engineType == model.EngineJuiceFSClone || engineType == model.EngineReflinkCopy
}

// MetadataPreservationForEngine returns the documented preservation contract
// for materializing payloads with engineType.
func MetadataPreservationForEngine(engineType model.EngineType) model.MetadataPreservation {
	switch engineType {
	case model.EngineJuiceFSClone:
		return model.MetadataPreservation{
			Symlinks:   "preserved",
			Hardlinks:  "identity not guaranteed in v0; files may materialize independently",
			Mode:       "preserved",
			Timestamps: "preserved",
			Ownership:  "filesystem-dependent",
			Xattrs:     "filesystem-dependent",
			ACLs:       "filesystem-dependent",
		}
	case model.EngineReflinkCopy:
		return model.MetadataPreservation{
			Symlinks:   "preserved",
			Hardlinks:  "identity not guaranteed in v0; files may materialize independently",
			Mode:       "preserved",
			Timestamps: "preserved",
			Ownership:  "not preserved",
			Xattrs:     "not preserved",
			ACLs:       "not preserved",
		}
	case model.EngineCopy:
		return model.MetadataPreservation{
			Symlinks:   "preserved",
			Hardlinks:  "identity not guaranteed in v0; files may materialize independently",
			Mode:       "preserved",
			Timestamps: "preserved",
			Ownership:  "not preserved",
			Xattrs:     "not preserved",
			ACLs:       "not preserved",
		}
	default:
		return model.MetadataPreservation{
			Symlinks:   "unknown",
			Hardlinks:  "unknown",
			Mode:       "unknown",
			Timestamps: "unknown",
			Ownership:  "unknown",
			Xattrs:     "unknown",
			ACLs:       "unknown",
		}
	}
}

// PerformanceClassForEngine returns the public performance class for an engine
// without promising benchmark-specific throughput.
func PerformanceClassForEngine(engineType model.EngineType) string {
	switch engineType {
	case model.EngineJuiceFSClone:
		return "constant-time-metadata-clone"
	case model.EngineReflinkCopy:
		return "linear-tree-walk-cow-data"
	case model.EngineCopy:
		return "linear-data-copy"
	default:
		return "unknown"
	}
}

func firstWarning(groups ...[]string) string {
	for _, group := range groups {
		if len(group) > 0 {
			return group[0]
		}
	}
	return ""
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, value := range in {
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
