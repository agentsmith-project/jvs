package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

type restoreRunCapacityDeviceMeter struct {
	repoRoot          string
	tempRoot          string
	siblingParent     string
	availableByDevice map[string]int64
}

func (m restoreRunCapacityDeviceMeter) AvailableBytes(path string) (int64, error) {
	device, err := m.DeviceID(path)
	if err != nil {
		return 0, err
	}
	return m.availableByDevice[device], nil
}

func (m restoreRunCapacityDeviceMeter) DeviceID(path string) (string, error) {
	slashPath := filepath.ToSlash(path)
	if m.tempRoot != "" && restoreRunCapacityPathHasPrefix(slashPath, filepath.ToSlash(m.tempRoot)) {
		return "temp-fs", nil
	}
	if m.siblingParent != "" && slashPath == filepath.ToSlash(m.siblingParent) {
		return "sibling-fs", nil
	}
	if m.repoRoot != "" && restoreRunCapacityPathHasPrefix(slashPath, filepath.ToSlash(m.repoRoot)) {
		return "repo-fs", nil
	}
	return slashPath, nil
}

func TestCheckRestoreRunCapacityDoesNotBudgetRenameBackupAsCopy(t *testing.T) {
	fixture := newRestoreRunCapacityFixture(t)
	err := fixture.checkWithAvailableBytes(t, map[string]int64{
		"repo-fs":    fixture.sourcePeak + metadataFloor,
		"temp-fs":    2 * fixture.sourcePeak,
		"sibling-fs": fixture.sourcePeak,
	}, false)
	require.NoError(t, err)
}

func TestCheckRestoreRunCapacityStillBudgetsRealRestoreWrites(t *testing.T) {
	fixture := newRestoreRunCapacityFixture(t)

	t.Run("source validation metadata", func(t *testing.T) {
		err := fixture.checkWithAvailableBytes(t, map[string]int64{
			"repo-fs":    fixture.sourcePeak + metadataFloor - 1,
			"temp-fs":    2 * fixture.sourcePeak,
			"sibling-fs": fixture.sourcePeak,
		}, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Not enough free space")
	})

	t.Run("restore payload staging", func(t *testing.T) {
		err := fixture.checkWithAvailableBytes(t, map[string]int64{
			"repo-fs":    fixture.sourcePeak + metadataFloor,
			"temp-fs":    2 * fixture.sourcePeak,
			"sibling-fs": fixture.sourcePeak - 1,
		}, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Not enough free space")
	})

	t.Run("save-first safety save", func(t *testing.T) {
		err := fixture.checkWithAvailableBytes(t, map[string]int64{
			"repo-fs":    fixture.sourcePeak + metadataFloor,
			"temp-fs":    2 * fixture.sourcePeak,
			"sibling-fs": fixture.sourcePeak,
		}, true)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Not enough free space")
	})
}

type restoreRunCapacityFixture struct {
	repoRoot    string
	source      *model.Descriptor
	snapshotDir string
	sourcePeak  int64
}

func newRestoreRunCapacityFixture(t *testing.T) restoreRunCapacityFixture {
	t.Helper()
	repoRoot := t.TempDir()
	_, err := repo.InitAdoptedWorkspace(repoRoot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("source"), 0644))
	source, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "source", nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "workspace-only.bin"), []byte(strings.Repeat("x", 64<<10)), 0644))
	_, err = snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("main", "workspace", nil)
	require.NoError(t, err)

	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, source.SnapshotID)
	require.NoError(t, err)
	sourceEstimate, err := snapshotpayload.EstimateMaterializationCapacity(snapshotDir, snapshotpayload.OptionsFromDescriptor(source))
	require.NoError(t, err)
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, "main")
	require.NoError(t, err)
	workspaceBytes, err := capacitygate.TreeSize(boundary.Root, boundary.ExcludesRelativePath)
	require.NoError(t, err)
	require.Greater(t, workspaceBytes, sourceEstimate.PeakBytes)
	return restoreRunCapacityFixture{
		repoRoot:    repoRoot,
		source:      source,
		snapshotDir: snapshotDir,
		sourcePeak:  sourceEstimate.PeakBytes,
	}
}

func (f restoreRunCapacityFixture) checkWithAvailableBytes(t *testing.T, availableByDevice map[string]int64, saveFirst bool) error {
	t.Helper()
	tempRoot := filepath.Join(t.TempDir(), "temp")
	require.NoError(t, os.MkdirAll(tempRoot, 0755))
	t.Setenv("TMPDIR", tempRoot)
	restoreGate := restoreRunCapacityGate
	restoreRunCapacityGate = capacitygate.Gate{
		Meter: restoreRunCapacityDeviceMeter{
			repoRoot:          f.repoRoot,
			tempRoot:          tempRoot,
			siblingParent:     filepath.Dir(f.repoRoot),
			availableByDevice: availableByDevice,
		},
		SafetyMarginBytes: 0,
	}
	t.Cleanup(func() {
		restoreRunCapacityGate = restoreGate
	})

	return checkRestoreRunCapacity(f.repoRoot, "main", &restoreplan.Plan{
		Folder:          f.repoRoot,
		Workspace:       "main",
		SourceSavePoint: f.source.SnapshotID,
		Options:         restoreplan.Options{SaveFirst: saveFirst},
	}, f.snapshotDir, f.source)
}

func restoreRunCapacityPathHasPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/")
}
