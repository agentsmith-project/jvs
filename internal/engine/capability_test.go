package engine_test

import (
	"path/filepath"
	"testing"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestProbeCapabilities_NoWriteProbeKeepsWriteDependentSupportUnknown(t *testing.T) {
	report, err := engine.ProbeCapabilities(t.TempDir(), false)
	require.NoError(t, err)

	require.False(t, report.WriteProbe)
	require.False(t, report.Write.Supported)
	require.Equal(t, engine.CapabilityUnknown, report.Write.Confidence)
	require.False(t, report.Copy.Supported)
	require.Equal(t, engine.CapabilityUnknown, report.Copy.Confidence)
	require.False(t, report.Reflink.Supported)
	require.Equal(t, engine.CapabilityUnknown, report.Reflink.Confidence)
}

func TestProbeCapabilities_WriteProbeConfirmsCopyAndCleansUp(t *testing.T) {
	target := t.TempDir()

	report, err := engine.ProbeCapabilities(target, true)
	require.NoError(t, err)

	require.True(t, report.WriteProbe)
	require.True(t, report.Write.Supported)
	require.Equal(t, engine.CapabilityConfirmed, report.Write.Confidence)
	require.True(t, report.Copy.Supported)
	require.Equal(t, engine.CapabilityConfirmed, report.Copy.Confidence)

	leftovers, err := filepath.Glob(filepath.Join(target, ".jvs-capability-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
}

func TestTransferPlanner_DegradesReflinkPairOnce(t *testing.T) {
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
			Reflink: engine.Capability{
				Available:  true,
				Supported:  false,
				Confidence: engine.CapabilityConfirmed,
				Warnings:   []string{"reflink pair probe failed: invalid cross-device link"},
			},
		},
	}
	selector := &fakeTransferSelector{
		selection: engine.TransferSelection{TransferEngine: model.EngineReflinkCopy},
	}

	plan, err := engine.TransferPlanner{
		Prober:   prober,
		Selector: selector,
	}.PlanTransfer(engine.TransferPlanRequest{
		SourcePath:      "/src",
		DestinationPath: "/dst",
		RequestedEngine: model.EngineType("auto"),
	})
	require.NoError(t, err)

	require.Equal(t, model.EngineType("auto"), plan.RequestedEngine)
	require.Equal(t, model.EngineCopy, plan.TransferEngine)
	require.Equal(t, model.EngineCopy, plan.EffectiveEngine)
	require.False(t, plan.OptimizedTransfer)
	require.Len(t, plan.DegradedReasons, 1)
	require.Contains(t, plan.DegradedReasons[0], "source/destination pair")
	require.Contains(t, plan.DegradedReasons[0], "invalid cross-device link")
	require.Equal(t, 1, prober.pairCalls)
	require.Equal(t, 1, selector.calls)
}

type fakeCapabilityProber struct {
	report    *engine.CapabilityReport
	pair      *engine.TransferPairReport
	pairCalls int
}

func (p *fakeCapabilityProber) ProbeCapabilities(path string, writeProbe bool) (*engine.CapabilityReport, error) {
	return p.report, nil
}

func (p *fakeCapabilityProber) ProbeTransferPair(sourcePath, destinationPath string) (*engine.TransferPairReport, error) {
	p.pairCalls++
	return p.pair, nil
}

type fakeTransferSelector struct {
	selection engine.TransferSelection
	calls     int
}

func (s *fakeTransferSelector) SelectTransfer(req engine.TransferSelectionRequest) engine.TransferSelection {
	s.calls++
	return s.selection
}
