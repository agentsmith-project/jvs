package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvs-project/jvs/pkg/model"
)

const (
	CapabilityConfirmed = "confirmed"
	CapabilityUnknown   = "unknown"
)

// Capability describes whether one transfer mechanism can be used at a target.
type Capability struct {
	Available  bool     `json:"available"`
	Supported  bool     `json:"supported"`
	Confidence string   `json:"confidence"`
	Warnings   []string `json:"warnings,omitempty"`
}

// CapabilityReport is a non-mutating report by default. Reflink support is
// only confirmed when writeProbe is true.
type CapabilityReport struct {
	TargetPath        string           `json:"target_path"`
	ProbePath         string           `json:"probe_path"`
	WriteProbe        bool             `json:"write_probe"`
	JuiceFS           Capability       `json:"juicefs"`
	Reflink           Capability       `json:"reflink"`
	Copy              Capability       `json:"copy"`
	RecommendedEngine model.EngineType `json:"recommended_engine"`
	Warnings          []string         `json:"warnings,omitempty"`
}

// ProbeCapabilities reports JuiceFS, reflink, and copy support for targetPath.
// With writeProbe=false it does not create test files.
func ProbeCapabilities(targetPath string, writeProbe bool) (*CapabilityReport, error) {
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
		Copy: Capability{
			Available:  true,
			Supported:  true,
			Confidence: CapabilityConfirmed,
		},
		Warnings: warnings,
	}

	report.JuiceFS = probeJuiceFS(probePath)
	report.Reflink = probeReflink(probePath, writeProbe)
	report.RecommendedEngine = recommendedEngine(report)
	report.Warnings = append(report.Warnings, report.JuiceFS.Warnings...)
	report.Warnings = append(report.Warnings, report.Reflink.Warnings...)

	return report, nil
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

func probeReflink(path string, writeProbe bool) Capability {
	capability := Capability{
		Available:  true,
		Supported:  false,
		Confidence: CapabilityUnknown,
	}
	if !writeProbe {
		capability.Warnings = append(capability.Warnings, "reflink support requires --write-probe to confirm")
		return capability
	}

	tempDir, err := os.MkdirTemp(path, ".jvs-capability-")
	if err != nil {
		capability.Confidence = CapabilityConfirmed
		capability.Warnings = append(capability.Warnings, fmt.Sprintf("cannot create reflink probe directory: %v", err))
		return capability
	}
	defer os.RemoveAll(tempDir)

	src := filepath.Join(tempDir, "src")
	dst := filepath.Join(tempDir, "dst")
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
	default:
		return model.EngineCopy
	}
}
