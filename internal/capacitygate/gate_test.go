package capacitygate_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMeter struct {
	available int64
	probes    []string
}

func (m *fakeMeter) AvailableBytes(path string) (int64, error) {
	m.probes = append(m.probes, path)
	return m.available, nil
}

type fakeDeviceMeter struct {
	availableByDevice map[string]int64
	deviceByPath      map[string]string
	probes            []string
}

func (m *fakeDeviceMeter) AvailableBytes(path string) (int64, error) {
	m.probes = append(m.probes, path)
	device := m.deviceByPath[filepath.ToSlash(path)]
	return m.availableByDevice[device], nil
}

func (m *fakeDeviceMeter) DeviceID(path string) (string, error) {
	device := m.deviceByPath[filepath.ToSlash(path)]
	if device == "" {
		device = filepath.ToSlash(path)
	}
	return device, nil
}

func TestCapacityGateInsufficientCapacityReturnsStableErrorAndDecision(t *testing.T) {
	meter := &fakeMeter{available: 100}
	gate := capacitygate.Gate{Meter: meter, SafetyMarginBytes: 15}

	decision, err := gate.Check(capacitygate.Request{
		Operation:       "restore preview",
		Folder:          "/repo",
		Workspace:       "main",
		SourceSavePoint: "1777333000000-deadbeef",
		Components: []capacitygate.Component{
			{Name: "source materialization", Path: "/repo/.jvs/missing/source", Bytes: 70},
			{Name: "plan metadata", Path: "/repo/.jvs/restore-plans", Bytes: 20},
		},
		FailureMessages: []string{"No restore plan was created.", "No files were changed."},
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.EqualValues(t, 90, decision.RequiredBytes)
	assert.EqualValues(t, 15, decision.SafetyMarginBytes)
	assert.EqualValues(t, 100, decision.AvailableBytes)
	assert.EqualValues(t, 5, decision.ShortfallBytes)
	assert.Equal(t, "/repo/.jvs", filepath.ToSlash(decision.ProbePath))
	assert.Equal(t, []string{"/repo/.jvs"}, filepathSlashAll(meter.probes))

	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr))
	assert.Equal(t, "E_NOT_ENOUGH_SPACE", jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "Not enough free space")
	assert.Contains(t, jvsErr.Message, "Folder: /repo")
	assert.Contains(t, jvsErr.Message, "Workspace: main")
	assert.Contains(t, jvsErr.Message, "No restore plan was created.")
	assert.Contains(t, jvsErr.Message, "No files were changed.")
	assert.NotContains(t, jvsErr.Message, "snapshot")
	assert.NotContains(t, jvsErr.Message, "worktree")
}

func TestCapacityGateAllowsWhenAvailableCoversRequiredAndMargin(t *testing.T) {
	meter := &fakeMeter{available: 106}
	gate := capacitygate.Gate{Meter: meter, SafetyMarginBytes: 15}

	decision, err := gate.Check(capacitygate.Request{
		Operation: "view",
		Folder:    "/repo",
		Workspace: "main",
		Components: []capacitygate.Component{
			{Name: "payload", Path: "/repo/.jvs/views", Bytes: 90},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.EqualValues(t, 90, decision.RequiredBytes)
	assert.EqualValues(t, 15, decision.SafetyMarginBytes)
	assert.EqualValues(t, 106, decision.AvailableBytes)
	assert.Zero(t, decision.ShortfallBytes)
}

func TestCapacityGateAggregatesComponentsOnSameFilesystem(t *testing.T) {
	meter := &fakeDeviceMeter{
		availableByDevice: map[string]int64{"repo-fs": 100},
		deviceByPath: map[string]string{
			"/repo/.jvs": "repo-fs",
			"/repo/main": "repo-fs",
		},
	}
	gate := capacitygate.Gate{Meter: meter}

	decision, err := gate.Check(capacitygate.Request{
		Folder: "/repo",
		Components: []capacitygate.Component{
			{Name: "first", Path: "/repo/.jvs/a", Bytes: 60},
			{Name: "second", Path: "/repo/main/b", Bytes: 60},
		},
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.EqualValues(t, 120, decision.RequiredBytes)
	assert.EqualValues(t, 100, decision.AvailableBytes)
	assert.EqualValues(t, 20, decision.ShortfallBytes)
	assert.Len(t, meter.probes, 1)
}

func TestCapacityGateChecksEachFilesystem(t *testing.T) {
	meter := &fakeDeviceMeter{
		availableByDevice: map[string]int64{"repo-fs": 1000, "temp-fs": 10},
		deviceByPath: map[string]string{
			"/repo/.jvs": "repo-fs",
			"/tmp":       "temp-fs",
		},
	}
	gate := capacitygate.Gate{Meter: meter}

	decision, err := gate.Check(capacitygate.Request{
		Folder: "/repo",
		Components: []capacitygate.Component{
			{Name: "repo write", Path: "/repo/.jvs/views/payload", Bytes: 100},
			{Name: "hash temp", Path: "/tmp/jvs-payload-hash-probe", Bytes: 20},
		},
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.EqualValues(t, 120, decision.RequiredBytes)
	assert.EqualValues(t, 1010, decision.AvailableBytes)
	assert.EqualValues(t, 10, decision.ShortfallBytes)
	assert.ElementsMatch(t, []string{"/repo/.jvs", "/tmp"}, filepathSlashAll(meter.probes))
}

func TestCapacityGateProbesZeroByteWriters(t *testing.T) {
	meter := &fakeDeviceMeter{
		availableByDevice: map[string]int64{"repo-fs": 100, "temp-fs": 0},
		deviceByPath: map[string]string{
			"/repo/.jvs": "repo-fs",
			"/tmp":       "temp-fs",
		},
	}
	gate := capacitygate.Gate{Meter: meter, SafetyMarginBytes: 1}

	decision, err := gate.Check(capacitygate.Request{
		Folder: "/repo",
		Components: []capacitygate.Component{
			{Name: "empty payload hash", Path: "/tmp/jvs-payload-hash-probe", Bytes: 0},
			{Name: "metadata", Path: "/repo/.jvs", Bytes: 10},
		},
	})

	require.Error(t, err)
	require.NotNil(t, decision)
	assert.EqualValues(t, 10, decision.RequiredBytes)
	assert.EqualValues(t, 2, decision.SafetyMarginBytes)
	assert.EqualValues(t, 100, decision.AvailableBytes)
	assert.EqualValues(t, 1, decision.ShortfallBytes)
	assert.ElementsMatch(t, []string{"/tmp", "/repo/.jvs"}, filepathSlashAll(meter.probes))
}

func TestCapacityGateTreatsExactControlDirAsControlFilesystem(t *testing.T) {
	meter := &fakeMeter{available: 100}
	gate := capacitygate.Gate{Meter: meter}

	_, err := gate.Check(capacitygate.Request{
		Folder: "/repo",
		Components: []capacitygate.Component{
			{Name: "metadata", Path: "/repo/.jvs", Bytes: 1},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/.jvs"}, filepathSlashAll(meter.probes))
}

func filepathSlashAll(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, filepath.ToSlash(path))
	}
	return out
}
