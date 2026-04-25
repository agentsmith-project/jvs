package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestInitCommand_MultiLevelAbsoluteJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "parent", "child", "repo")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "init", target)
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, target, got["repo_root"])
	require.Equal(t, filepath.Join(target, "main"), got["main_workspace"])
	require.Contains(t, got, "capabilities")
	require.DirExists(t, filepath.Join(target, ".jvs"))
	require.DirExists(t, filepath.Join(target, "main"))
}

func TestInitCommand_RejectsNonEmptyTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(target, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "existing.txt"), []byte("data"), 0644))

	_, err := executeCommand(createTestRootCmd(), "init", target)
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(target, ".jvs"))
}

func TestInitCommand_RejectsNestedRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "outer")
	_, err := executeCommand(createTestRootCmd(), "init", root)
	require.NoError(t, err)

	_, err = executeCommand(createTestRootCmd(), "init", filepath.Join(root, "main", "nested"))
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(root, "main", "nested", ".jvs"))
}

func TestImportCommand_CopiesSourceCreatesInitialCheckpoint(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "imported", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "dir", "file.txt"), []byte("hello"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "import", source, dest)
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, "import", got["scope"])
	require.Equal(t, "import", got["requested_scope"])
	require.Equal(t, source, got["provenance"])
	require.NotEmpty(t, got["initial_checkpoint"])
	require.NotEmpty(t, got["engine"])
	require.NotEmpty(t, got["requested_engine"])
	require.NotEmpty(t, got["transfer_engine"])
	require.NotEmpty(t, got["effective_engine"])
	require.IsType(t, true, got["optimized_transfer"])
	require.Contains(t, got, "warnings")
	require.FileExists(t, filepath.Join(source, "dir", "file.txt"))
	require.FileExists(t, filepath.Join(dest, "main", "dir", "file.txt"))

	snaps, err := snapshot.ListAll(dest)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	require.Contains(t, snaps[0].Note, "import")

	cfg, err := repo.LoadWorktreeConfig(dest, "main")
	require.NoError(t, err)
	require.Equal(t, snaps[0].SnapshotID, cfg.HeadSnapshotID)
	require.Equal(t, snaps[0].SnapshotID, cfg.LatestSnapshotID)
}

func TestImportCommand_VerifiesInitialCheckpointAndCleansUpOnFailure(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "imported", "repo")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("hello"), 0644))

	oldVerify := verifyLifecycleCheckpoint
	verifyCalls := 0
	verifyLifecycleCheckpoint = func(repoRoot string, snapshotID model.SnapshotID) error {
		verifyCalls++
		require.Equal(t, dest, repoRoot)
		require.NotEmpty(t, snapshotID)
		return errclass.ErrPayloadHashMismatch.WithMessage("forced lifecycle verification failure")
	}
	t.Cleanup(func() { verifyLifecycleCheckpoint = oldVerify })

	_, err := executeCommand(createTestRootCmd(), "import", source, dest)
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrPayloadHashMismatch)
	require.Equal(t, 1, verifyCalls)
	require.NoDirExists(t, dest)
}

func TestImportCommand_RejectsSourceContainingJVS(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(source, ".jvs"), 0755))

	_, err := executeCommand(createTestRootCmd(), "import", source, dest)
	require.Error(t, err)
	require.NoDirExists(t, dest)
}

func TestImportCommand_RejectsPhysicalOverlapViaSymlinkParent(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	realSubdir := filepath.Join(source, "data")
	require.NoError(t, os.MkdirAll(realSubdir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realSubdir, "file.txt"), []byte("hello"), 0644))

	linkParent := filepath.Join(base, "link-to-data")
	requireLifecycleSymlink(t, realSubdir, linkParent)
	dest := filepath.Join(linkParent, "repo")

	_, err := executeCommand(createTestRootCmd(), "import", source, dest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "physical path overlap")
	require.NoDirExists(t, filepath.Join(realSubdir, "repo", ".jvs"))
}

func TestCloneCommand_FullCopiesHistoryAndCreatesNewIdentity(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(filepath.Join(source, "main")))
	_, err = executeCommand(createTestRootCmd(), "snapshot", "source checkpoint")
	require.NoError(t, err)
	require.NoError(t, os.Chdir(originalWd))

	sourceID := readRepoIDForTest(t, source)
	stdout, err := executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "full")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, "full", got["scope"])
	require.NotEmpty(t, got["transfer_mode"])
	require.FileExists(t, filepath.Join(dest, "main", "file.txt"))
	require.NotEqual(t, sourceID, readRepoIDForTest(t, dest))

	sourceSnaps, err := snapshot.ListAll(source)
	require.NoError(t, err)
	destSnaps, err := snapshot.ListAll(dest)
	require.NoError(t, err)
	require.Len(t, destSnaps, len(sourceSnaps))
}

func TestCloneCommand_FullRejectsCorruptedSourceRepository(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(filepath.Join(source, "main")))
	_, err = executeCommand(createTestRootCmd(), "snapshot", "source checkpoint")
	require.NoError(t, err)
	require.NoError(t, os.Chdir(originalWd))

	sourceSnaps, err := snapshot.ListAll(source)
	require.NoError(t, err)
	require.Len(t, sourceSnaps, 1)
	require.NoError(t, os.WriteFile(
		filepath.Join(source, ".jvs", "snapshots", string(sourceSnaps[0].SnapshotID), "tampered.txt"),
		[]byte("tampered"),
		0644,
	))

	_, err = executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", "full")
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrPayloadHashMismatch)
	require.NoDirExists(t, dest)
}

func TestCloneCommand_FullRejectsPhysicalOverlapViaSymlinkParent(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

	linkParent := filepath.Join(base, "link-to-main")
	requireLifecycleSymlink(t, filepath.Join(source, "main"), linkParent)
	dest := filepath.Join(linkParent, "repo")

	_, err = executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", "full")
	require.Error(t, err)
	require.Contains(t, err.Error(), "physical path overlap")
	require.NoDirExists(t, filepath.Join(source, "main", "repo", ".jvs"))
}

func TestCloneCommand_FullExcludesRuntimeState(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	sourceLocks := filepath.Join(source, ".jvs", "locks")
	require.NoError(t, os.WriteFile(filepath.Join(sourceLocks, "stale.lock"), []byte("runtime"), 0600))
	sourceIntents := filepath.Join(source, ".jvs", "intents")
	require.NoError(t, os.WriteFile(filepath.Join(sourceIntents, "orphan.json"), []byte("{}"), 0600))
	sourceGC := filepath.Join(source, ".jvs", "gc")
	require.NoError(t, os.WriteFile(filepath.Join(sourceGC, "active-plan.json"), []byte(`{"plan_id":"active-plan"}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceGC, "tombstones"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceGC, "tombstones", "retained.json"), []byte(`{"gc_state":"committed"}`), 0600))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "full")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, "full", got["scope"])
	require.Equal(t, "full", got["requested_scope"])
	require.Equal(t, "full-repository", got["copy_mode"])
	require.Equal(t, "copy", got["requested_engine"])
	require.Equal(t, "copy", got["transfer_engine"])
	require.Equal(t, "copy", got["effective_engine"])
	require.Equal(t, false, got["optimized_transfer"])
	require.Equal(t, true, got["runtime_state_excluded"])
	require.Contains(t, got, "warnings")
	require.DirExists(t, filepath.Join(dest, ".jvs", "locks"))
	require.NoFileExists(t, filepath.Join(dest, ".jvs", "locks", "stale.lock"))
	require.NoDirExists(t, filepath.Join(dest, ".jvs", "locks", "repo.lock"))
	require.DirExists(t, filepath.Join(dest, ".jvs", "intents"))
	require.NoFileExists(t, filepath.Join(dest, ".jvs", "intents", "orphan.json"))
	require.NoFileExists(t, filepath.Join(dest, ".jvs", "gc", "active-plan.json"))
	require.FileExists(t, filepath.Join(dest, ".jvs", "gc", "tombstones", "retained.json"))

	lock, err := repo.AcquireMutationLock(dest, "post-clone check")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
}

func TestCloneCommand_FullRejectsBusySource(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(source, "test holder")
	require.NoError(t, err)
	defer held.Release()

	_, err = executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "full")
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
	require.Contains(t, err.Error(), "E_REPO_BUSY")
	require.NoDirExists(t, dest)
	require.NoDirExists(t, filepath.Join(dest, ".jvs", "locks", "repo.lock"))
}

func TestCloneCommand_FullBusySourceJSONEnvelope(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(source, "test holder")
	require.NoError(t, err)
	defer held.Release()

	stdout, _, exitCode := runContractSubprocess(t, "", "--json", "clone", source, dest, "--scope", "full")
	require.Equal(t, 1, exitCode)

	env := decodeContractEnvelope(t, stdout)
	require.False(t, env.OK)
	require.Equal(t, "clone", env.Command)
	require.NotNil(t, env.Error)
	require.Equal(t, "E_REPO_BUSY", env.Error.Code)
	require.JSONEq(t, "null", string(env.Data))
}

func TestCloneCommand_CurrentCopiesWorkspaceAndDisconnectsHistory(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("snapshotted"), 0644))

	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(filepath.Join(source, "main")))
	_, err = executeCommand(createTestRootCmd(), "snapshot", "source checkpoint")
	require.NoError(t, err)
	require.NoError(t, os.Chdir(originalWd))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("working state"), 0644))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "current")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, "current", got["scope"])
	require.Equal(t, "current", got["requested_scope"])
	require.NotEmpty(t, got["initial_checkpoint"])
	require.NotEmpty(t, got["requested_engine"])
	require.NotEmpty(t, got["transfer_engine"])
	require.NotEmpty(t, got["effective_engine"])
	require.IsType(t, true, got["optimized_transfer"])
	require.Contains(t, got, "warnings")
	require.NotContains(t, stdout, "source_worktree")
	provenance, ok := got["provenance"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "main", provenance["source_workspace"])

	data, err := os.ReadFile(filepath.Join(dest, "main", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "working state", string(data))

	destSnaps, err := snapshot.ListAll(dest)
	require.NoError(t, err)
	require.Len(t, destSnaps, 1)
	require.Nil(t, destSnaps[0].ParentID)
	require.Contains(t, []model.EngineType{model.EngineCopy, model.EngineReflinkCopy, model.EngineJuiceFSClone}, destSnaps[0].Engine)
	require.Contains(t, destSnaps[0].Note, "clone")
}

func TestCloneCommand_CurrentVerifiesInitialCheckpointAndCleansUpOnFailure(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("working state"), 0644))

	oldVerify := verifyLifecycleCheckpoint
	verifyCalls := 0
	verifyLifecycleCheckpoint = func(repoRoot string, snapshotID model.SnapshotID) error {
		verifyCalls++
		require.Equal(t, dest, repoRoot)
		require.NotEmpty(t, snapshotID)
		return errclass.ErrPayloadHashMismatch.WithMessage("forced lifecycle verification failure")
	}
	t.Cleanup(func() { verifyLifecycleCheckpoint = oldVerify })

	_, err = executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", "current")
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrPayloadHashMismatch)
	require.Equal(t, 1, verifyCalls)
	require.NoDirExists(t, dest)
}

func TestCloneCurrentJSONSeparatesTransferFromMaterializationEngine(t *testing.T) {
	base := t.TempDir()
	report, err := engine.ProbeCapabilities(base, true)
	require.NoError(t, err)
	if report.JuiceFS.Supported {
		t.Skip("test requires a non-JuiceFS temp directory")
	}

	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err = executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

	t.Setenv("JVS_SNAPSHOT_ENGINE", "juicefs-clone")
	t.Setenv("JVS_ENGINE", "")
	stdout, err := executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "current")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, "juicefs-clone", got["engine"])
	require.Equal(t, "juicefs-clone", got["effective_engine"])
	require.Equal(t, "copy", got["transfer_engine"])
	require.NotEqual(t, got["transfer_engine"], got["effective_engine"])
	require.IsType(t, []any{}, got["degraded_reasons"])
}

func TestCloneCommand_CurrentRejectsPhysicalOverlapViaSymlinkParent(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

	linkParent := filepath.Join(base, "link-to-main")
	requireLifecycleSymlink(t, filepath.Join(source, "main"), linkParent)
	dest := filepath.Join(linkParent, "repo")

	_, err = executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", "current")
	require.Error(t, err)
	require.Contains(t, err.Error(), "physical path overlap")
	require.NoDirExists(t, filepath.Join(source, "main", "repo", ".jvs"))
}

func TestCloneCurrentRejectsWorkspacePathSource(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	_, err = executeCommand(createTestRootCmd(), "clone", filepath.Join(source, "main"), dest, "--scope", "current")
	require.Error(t, err)
	require.Contains(t, err.Error(), "source must be the repository root")
	require.Contains(t, err.Error(), "source-workspace")
	require.NoDirExists(t, dest)
}

func TestCloneCommand_CurrentRejectsBusySource(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	held, err := repo.AcquireMutationLock(source, "test holder")
	require.NoError(t, err)
	defer held.Release()

	_, err = executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "current")
	require.Error(t, err)
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
	require.NoDirExists(t, dest)
}

func TestCloneCommand_CurrentHumanUsesWorkspaceVocabulary(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", "current")
	require.NoError(t, err)
	require.Contains(t, stdout, "Source workspace:")
	require.NotContains(t, stdout, "Source worktree:")
}

func TestCapabilityCommand_JSONShape(t *testing.T) {
	target := t.TempDir()

	stdout, err := executeCommand(createTestRootCmd(), "--json", "capability", target)
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, target, got["target_path"])
	require.Equal(t, false, got["write_probe"])
	require.Contains(t, got, "juicefs")
	require.Contains(t, got, "reflink")
	require.Contains(t, got, "copy")

	copyCapability := got["copy"].(map[string]any)
	require.Equal(t, false, copyCapability["supported"])
	require.Equal(t, "unknown", copyCapability["confidence"])
	writeCapability := got["write"].(map[string]any)
	require.Equal(t, false, writeCapability["supported"])
	require.Equal(t, "unknown", writeCapability["confidence"])
}

func TestCapabilityJSONIncludesMetadataPreservationAndPerformanceClass(t *testing.T) {
	target := t.TempDir()

	stdout, err := executeCommand(createTestRootCmd(), "--json", "capability", target)
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.NotEmpty(t, got["performance_class"])
	metadata, ok := got["metadata_preservation"].(map[string]any)
	require.True(t, ok, "metadata_preservation must be an object: %s", stdout)
	for _, field := range []string{"symlinks", "hardlinks", "mode", "timestamps", "ownership", "xattrs", "acls"} {
		require.NotEmpty(t, metadata[field], "metadata_preservation.%s must be set", field)
	}
}

func TestCapabilityCommand_WriteProbeConfirmsCopyAndCleansUp(t *testing.T) {
	target := t.TempDir()

	stdout, err := executeCommand(createTestRootCmd(), "--json", "capability", target, "--write-probe")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, true, got["write_probe"])
	copyCapability := got["copy"].(map[string]any)
	require.Equal(t, true, copyCapability["supported"])
	require.Equal(t, "confirmed", copyCapability["confidence"])
	writeCapability := got["write"].(map[string]any)
	require.Equal(t, true, writeCapability["supported"])
	require.Equal(t, "confirmed", writeCapability["confidence"])

	leftovers, err := filepath.Glob(filepath.Join(target, ".jvs-capability-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
}

func TestPerformanceDocsDoNotUseUnsupportedEngineFlagOrUnconditionalO1Claims(t *testing.T) {
	performance, err := os.ReadFile(filepath.Join("..", "..", "docs", "PERFORMANCE.md"))
	require.NoError(t, err)
	results, err := os.ReadFile(filepath.Join("..", "..", "docs", "PERFORMANCE_RESULTS.md"))
	require.NoError(t, err)

	combined := string(performance) + "\n" + string(results)
	require.NotContains(t, combined, "--engine")
	require.NotContains(t, string(performance), "Unlimited")
	require.NotContains(t, string(results), "TB/s")
	require.NotContains(t, string(results), "Snapshot time is O(1) with juicefs-clone engine")
}

func requireLifecycleSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func readRepoIDForTest(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".jvs", "repo_id"))
	require.NoError(t, err)
	return string(bytesTrimSpace(data))
}

func decodeJSONDataForTest(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	if data, ok := envelope["data"].(map[string]any); ok {
		return data
	}
	return envelope
}

func bytesTrimSpace(data []byte) []byte {
	for len(data) > 0 && (data[0] == ' ' || data[0] == '\n' || data[0] == '\r' || data[0] == '\t') {
		data = data[1:]
	}
	for len(data) > 0 {
		last := data[len(data)-1]
		if last != ' ' && last != '\n' && last != '\r' && last != '\t' {
			break
		}
		data = data[:len(data)-1]
	}
	return data
}
