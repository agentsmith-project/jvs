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
	require.Equal(t, "copy", got["effective_engine"])
	require.Equal(t, "copy", got["transfer_engine"])
	require.Equal(t, "copy", got["transfer_mode"])
	require.Equal(t, "linear-data-copy", got["performance_class"])
	require.IsType(t, []any{}, got["degraded_reasons"])
}

func TestLifecycleJSONEffectiveEngineMatchesInitialCheckpointDescriptor(t *testing.T) {
	base := t.TempDir()
	report, err := engine.ProbeCapabilities(base, true)
	require.NoError(t, err)
	if report.JuiceFS.Supported {
		t.Skip("test requires a non-JuiceFS temp directory")
	}

	t.Setenv("JVS_SNAPSHOT_ENGINE", string(model.EngineJuiceFSClone))
	t.Setenv("JVS_ENGINE", "")

	t.Run("import", func(t *testing.T) {
		source := filepath.Join(base, "import-source")
		dest := filepath.Join(base, "import-dest")
		require.NoError(t, os.MkdirAll(source, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("v1"), 0644))

		stdout, err := executeCommand(createTestRootCmd(), "--json", "import", source, dest)
		require.NoError(t, err)

		requireLifecycleSetupJSONMatchesInitialDescriptor(t, stdout, dest)
	})

	t.Run("clone-current", func(t *testing.T) {
		source := filepath.Join(base, "clone-source")
		dest := filepath.Join(base, "clone-dest")
		_, err := executeCommand(createTestRootCmd(), "init", source)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644))

		stdout, err := executeCommand(createTestRootCmd(), "--json", "clone", source, dest, "--scope", "current")
		require.NoError(t, err)

		requireLifecycleSetupJSONMatchesInitialDescriptor(t, stdout, dest)
	})
}

func TestLifecycleJSONMaterializationFieldsPreferCheckpointDescriptor(t *testing.T) {
	descriptorMetadata := model.MetadataPreservation{
		Symlinks:   "descriptor-symlinks",
		Hardlinks:  "descriptor-hardlinks",
		Mode:       "descriptor-mode",
		Timestamps: "descriptor-timestamps",
		Ownership:  "descriptor-ownership",
		Xattrs:     "descriptor-xattrs",
		ACLs:       "descriptor-acls",
	}
	plan := &engine.TransferPlan{
		RequestedEngine:   model.EngineJuiceFSClone,
		TransferEngine:    model.EngineCopy,
		EffectiveEngine:   model.EngineCopy,
		OptimizedTransfer: false,
		Capabilities:      &engine.CapabilityReport{},
		DegradedReasons:   []string{"transfer degraded to copy"},
		Warnings:          []string{"transfer warning"},
	}
	desc := &model.Descriptor{
		Engine:               model.EngineJuiceFSClone,
		ActualEngine:         model.EngineCopy,
		EffectiveEngine:      model.EngineReflinkCopy,
		MetadataPreservation: &descriptorMetadata,
		PerformanceClass:     "descriptor-performance",
	}

	output := map[string]any{}
	applyTransferJSONFields(output, plan, desc)

	require.Equal(t, model.EngineCopy, output["transfer_engine"])
	require.Equal(t, string(model.EngineCopy), output["transfer_mode"])
	require.Equal(t, model.EngineReflinkCopy, output["effective_engine"])
	require.Equal(t, descriptorMetadata, output["metadata_preservation"])
	require.Equal(t, "descriptor-performance", output["performance_class"])
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

func TestCloneFullRejectsWorkspacePathSource(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	dest := filepath.Join(base, "dest")
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)

	_, err = executeCommand(createTestRootCmd(), "clone", filepath.Join(source, "main"), dest, "--scope", "full")
	require.Error(t, err)
	require.Contains(t, err.Error(), "source must be the repository root")
	require.Contains(t, err.Error(), "source-workspace")
	require.NoDirExists(t, dest)
}

func TestSetupDestinationTargetsAllowMissingMultiLevelAndEmptyDirs(t *testing.T) {
	originalWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	for _, op := range []string{"init", "import", "clone-current", "clone-full"} {
		for _, tc := range []struct {
			name      string
			destArg   func(cwd, base string) string
			wantRoot  func(cwd, base string) string
			precreate bool
		}{
			{
				name:     "relative_multi_level",
				destArg:  func(cwd, base string) string { return filepath.Join("relative", "multi", "repo") },
				wantRoot: func(cwd, base string) string { return filepath.Join(cwd, "relative", "multi", "repo") },
			},
			{
				name:     "absolute_multi_level",
				destArg:  func(cwd, base string) string { return filepath.Join(base, "absolute", "multi", "repo") },
				wantRoot: func(cwd, base string) string { return filepath.Join(base, "absolute", "multi", "repo") },
			},
			{
				name:      "existing_empty_dir",
				destArg:   func(cwd, base string) string { return filepath.Join(base, "empty", "repo") },
				wantRoot:  func(cwd, base string) string { return filepath.Join(base, "empty", "repo") },
				precreate: true,
			},
		} {
			t.Run(op+"_"+tc.name, func(t *testing.T) {
				base := t.TempDir()
				cwd := filepath.Join(base, "cwd")
				require.NoError(t, os.MkdirAll(cwd, 0755))
				require.NoError(t, os.Chdir(cwd))

				destArg := tc.destArg(cwd, base)
				wantRoot := tc.wantRoot(cwd, base)
				if tc.precreate {
					require.NoError(t, os.MkdirAll(wantRoot, 0755))
				}

				stdout, err := runLifecycleSetupOperation(t, op, base, destArg)
				require.NoError(t, err, stdout)
				got := decodeJSONDataForTest(t, stdout)
				require.Equal(t, wantRoot, got["repo_root"])
				require.DirExists(t, filepath.Join(wantRoot, ".jvs"))
				require.DirExists(t, filepath.Join(wantRoot, "main"))
			})
		}
	}
}

func TestSetupDestinationTargetsRejectInvalidDestinationsWithoutCleaningUserDirs(t *testing.T) {
	originalWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	for _, op := range []string{"init", "import", "clone-current", "clone-full"} {
		t.Run(op+"_file_dest", func(t *testing.T) {
			base := t.TempDir()
			require.NoError(t, os.Chdir(base))
			dest := filepath.Join(base, "file-dest")
			require.NoError(t, os.WriteFile(dest, []byte("user data"), 0644))

			_, err := runLifecycleSetupOperation(t, op, base, dest)
			require.Error(t, err)
			require.FileExists(t, dest)
			require.NoDirExists(t, filepath.Join(dest, ".jvs"))
		})

		t.Run(op+"_non_empty_dest", func(t *testing.T) {
			base := t.TempDir()
			require.NoError(t, os.Chdir(base))
			dest := filepath.Join(base, "non-empty")
			require.NoError(t, os.MkdirAll(dest, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("keep"), 0644))

			_, err := runLifecycleSetupOperation(t, op, base, dest)
			require.Error(t, err)
			require.FileExists(t, filepath.Join(dest, "keep.txt"))
			require.NoDirExists(t, filepath.Join(dest, ".jvs"))
		})

		t.Run(op+"_parent_component_file", func(t *testing.T) {
			base := t.TempDir()
			require.NoError(t, os.Chdir(base))
			parentFile := filepath.Join(base, "not-a-directory")
			require.NoError(t, os.WriteFile(parentFile, []byte("user data"), 0644))
			dest := filepath.Join(parentFile, "child", "repo")

			_, err := runLifecycleSetupOperation(t, op, base, dest)
			require.Error(t, err)
			require.FileExists(t, parentFile)
		})
	}
}

func TestSetupCommandsRejectSourceDestinationOverlapWithoutCleaningPrecreatedDirs(t *testing.T) {
	t.Run("import_same_source_and_dest", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "source")
		require.NoError(t, os.MkdirAll(source, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(source, "keep.txt"), []byte("keep"), 0644))

		_, err := executeCommand(createTestRootCmd(), "import", source, source)
		require.Error(t, err)
		require.FileExists(t, filepath.Join(source, "keep.txt"))
		require.NoDirExists(t, filepath.Join(source, ".jvs"))
	})

	t.Run("import_dest_inside_source", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "source")
		dest := filepath.Join(source, "precreated-empty")
		require.NoError(t, os.MkdirAll(dest, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(source, "keep.txt"), []byte("keep"), 0644))

		_, err := executeCommand(createTestRootCmd(), "import", source, dest)
		require.Error(t, err)
		require.DirExists(t, dest)
		require.FileExists(t, filepath.Join(source, "keep.txt"))
		require.NoDirExists(t, filepath.Join(dest, ".jvs"))
	})

	for _, scope := range []string{"current", "full"} {
		t.Run("clone_"+scope+"_same_source_and_dest", func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "source")
			_, err := executeCommand(createTestRootCmd(), "init", source)
			require.NoError(t, err)

			_, err = executeCommand(createTestRootCmd(), "clone", source, source, "--scope", scope)
			require.Error(t, err)
			require.DirExists(t, filepath.Join(source, ".jvs"))
		})

		t.Run("clone_"+scope+"_dest_inside_source", func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "source")
			_, err := executeCommand(createTestRootCmd(), "init", source)
			require.NoError(t, err)
			dest := filepath.Join(source, "main", "precreated-empty")
			require.NoError(t, os.MkdirAll(dest, 0755))

			_, err = executeCommand(createTestRootCmd(), "clone", source, dest, "--scope", scope)
			require.Error(t, err)
			require.DirExists(t, dest)
			require.NoDirExists(t, filepath.Join(dest, ".jvs"))
		})
	}
}

func TestSetupCommandsIgnoreRepoWorkspaceFlagsAndCWD(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	repoA := filepath.Join(dir, "repoA")
	_, err := executeCommand(createTestRootCmd(), "init", repoA)
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(repoA, "main")))

	unusedRepoFlag := filepath.Join(dir, "missing-repo")
	unusedWorkspaceFlag := "missing-workspace"

	initTarget := filepath.Join(dir, "setup", "init-target")
	stdout, err := executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "init", initTarget)
	require.NoError(t, err, stdout)
	require.Equal(t, initTarget, decodeJSONDataForTest(t, stdout)["repo_root"])

	source := filepath.Join(dir, "import-source")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("import"), 0644))
	importTarget := filepath.Join(dir, "setup", "import-target")
	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "import", source, importTarget)
	require.NoError(t, err, stdout)
	require.Equal(t, importTarget, decodeJSONDataForTest(t, stdout)["repo_root"])

	cloneTarget := filepath.Join(dir, "setup", "clone-target")
	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "clone", initTarget, cloneTarget, "--scope", "current")
	require.NoError(t, err, stdout)
	require.Equal(t, cloneTarget, decodeJSONDataForTest(t, stdout)["repo_root"])

	capabilityTarget := filepath.Join(dir, "missing", "a", "b")
	stdout, err = executeCommand(createTestRootCmd(), "--json", "--repo", unusedRepoFlag, "--workspace", unusedWorkspaceFlag, "capability", capabilityTarget, "--write-probe")
	require.NoError(t, err, stdout)
	capabilityData := decodeJSONDataForTest(t, stdout)
	require.Equal(t, capabilityTarget, capabilityData["target_path"])
	require.Equal(t, dir, capabilityData["probe_path"])
	require.NoDirExists(t, filepath.Join(dir, "missing"))
}

func TestSetupCommandsDoNotUseRepoWorkspaceFlagsAsPositionalArgs(t *testing.T) {
	base := t.TempDir()
	for _, args := range [][]string{
		{"--repo", filepath.Join(base, "target"), "init"},
		{"--repo", filepath.Join(base, "source"), "--workspace", filepath.Join(base, "target"), "import"},
		{"--repo", filepath.Join(base, "source"), "--workspace", filepath.Join(base, "target"), "clone"},
		{"--repo", filepath.Join(base, "target"), "capability"},
	} {
		_, err := executeCommand(createTestRootCmd(), args...)
		require.Error(t, err)
	}
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

func TestCapabilityCommand_MissingNestedTargetUsesExistingParentAndDoesNotCreateRepo(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "missing", "a", "b")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "capability", target, "--write-probe")
	require.NoError(t, err)

	got := decodeJSONDataForTest(t, stdout)
	require.Equal(t, target, got["target_path"])
	require.Equal(t, base, got["probe_path"])
	require.NotEmpty(t, got["warnings"])
	require.NoDirExists(t, filepath.Join(base, "missing"))
	require.NoDirExists(t, filepath.Join(base, ".jvs"))
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

func requireLifecycleSetupJSONMatchesInitialDescriptor(t *testing.T, stdout, repoRoot string) {
	t.Helper()

	got := decodeJSONDataForTest(t, stdout)
	descs, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	require.Len(t, descs, 1)

	desc := descs[0]
	require.Equal(t, string(desc.SnapshotID), got["initial_checkpoint"])
	require.Equal(t, string(model.EngineJuiceFSClone), got["requested_engine"])
	require.Equal(t, string(model.EngineCopy), got["transfer_engine"])
	require.Equal(t, string(model.EngineCopy), got["transfer_mode"])
	require.IsType(t, []any{}, got["degraded_reasons"])

	require.Equal(t, model.EngineJuiceFSClone, desc.Engine)
	require.Equal(t, model.EngineCopy, desc.ActualEngine)
	require.Equal(t, model.EngineCopy, desc.EffectiveEngine)
	require.NotEmpty(t, desc.DegradedReasons)
	require.Equal(t, string(desc.Engine), got["engine"])
	require.Equal(t, string(desc.EffectiveEngine), got["effective_engine"])
	require.Equal(t, desc.PerformanceClass, got["performance_class"])
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

func runLifecycleSetupOperation(t *testing.T, op, base, destArg string) (string, error) {
	t.Helper()

	switch op {
	case "init":
		return executeCommand(createTestRootCmd(), "--json", "init", destArg)
	case "import":
		source := filepath.Join(base, "source-import")
		require.NoError(t, os.MkdirAll(source, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("import"), 0644))
		return executeCommand(createTestRootCmd(), "--json", "import", source, destArg)
	case "clone-current":
		source := filepath.Join(base, "source-clone-current")
		createLifecycleSetupSourceRepo(t, source)
		return executeCommand(createTestRootCmd(), "--json", "clone", source, destArg, "--scope", "current")
	case "clone-full":
		source := filepath.Join(base, "source-clone-full")
		createLifecycleSetupSourceRepo(t, source)
		return executeCommand(createTestRootCmd(), "--json", "clone", source, destArg, "--scope", "full")
	default:
		t.Fatalf("unknown lifecycle setup operation %q", op)
		return "", nil
	}
}

func createLifecycleSetupSourceRepo(t *testing.T, source string) {
	t.Helper()
	_, err := executeCommand(createTestRootCmd(), "init", source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("clone"), 0644))
}
