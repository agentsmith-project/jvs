package repoclone_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/repoclone"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneMainCreatesNewRepoWithOnlyMainWorkspaceAndMaterializedRoot(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")
	featureID, featurePath := createCleanFeatureWorkspaceSavePoint(t, source, mainID)
	require.NotEqual(t, mainID, featureID)

	target := filepath.Join(t.TempDir(), "target")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeMain,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	assert.Equal(t, "repo_clone", result.Operation)
	assert.Equal(t, target, result.TargetRepoRoot)
	assert.Equal(t, repoclone.SavePointsModeMain, result.SavePointsMode)
	assert.Equal(t, 1, result.SavePointsCopiedCount)
	assert.Equal(t, []model.SnapshotID{mainID}, result.SavePointsCopied)
	assert.Equal(t, []string{"main"}, result.WorkspacesCreated)
	assert.Equal(t, []string{"feature"}, result.SourceWorkspacesNotCreated)
	assert.False(t, result.RuntimeStateCopied)
	require.Len(t, result.Transfers, 2)
	assertRepoCloneTransfer(t, result.Transfers[0], "repo-clone-save-points", "save_point_storage_copy", "save_point_storage", "target_save_point_storage", transferFinalExecution)
	assertRepoCloneTransfer(t, result.Transfers[1], "repo-clone-main-workspace", "main_workspace_materialization", "source_main_current_state", "target_main_workspace", transferFinalExecution)

	sourceRepo, err := repo.Discover(source)
	require.NoError(t, err)
	targetRepo, err := repo.Discover(target)
	require.NoError(t, err)
	assert.NotEqual(t, sourceRepo.RepoID, targetRepo.RepoID)
	assert.Equal(t, sourceRepo.RepoID, result.SourceRepoID)
	assert.Equal(t, targetRepo.RepoID, result.TargetRepoID)

	assert.FileExists(t, filepath.Join(target, "app.txt"))
	assert.NoDirExists(t, filepath.Join(target, "main"))
	assert.DirExists(t, filepath.Join(target, ".jvs"))
	assert.NoDirExists(t, filepath.Join(target, ".jvs", "worktrees", "feature"))
	assert.NoFileExists(t, filepath.Join(target, "feature.txt"))
	assert.NoFileExists(t, filepath.Join(target, ".jvs", "intents", "source-runtime.json"))
	assert.FileExists(t, filepath.Join(featurePath, ".jvs"), "source external workspace locator should remain in source")

	cfg, err := repo.LoadWorktreeConfig(target, "main")
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Name)
	assert.Equal(t, target, cfg.RealPath)
	assert.Equal(t, mainID, cfg.HeadSnapshotID)
	assert.Equal(t, mainID, cfg.LatestSnapshotID)
	assert.Empty(t, cfg.StartedFromSnapshotID)

	copied, err := snapshot.ListAll(target)
	require.NoError(t, err)
	require.Len(t, copied, 1)
	assert.Equal(t, mainID, copied[0].SnapshotID)
	assert.NoFileExists(t, clonehistory.ManifestPath(target), "main-mode clone should not write imported all-history manifest")
}

func TestCloneAllCopiesReadySavePointsButOnlyCreatesMainAndManifest(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")
	featureID, _ := createCleanFeatureWorkspaceSavePoint(t, source, mainID)

	target := filepath.Join(t.TempDir(), "target")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	assert.ElementsMatch(t, []model.SnapshotID{mainID, featureID}, result.SavePointsCopied)
	assert.Equal(t, 2, result.SavePointsCopiedCount)
	assert.Equal(t, []string{"feature"}, result.SourceWorkspacesNotCreated)
	assert.DirExists(t, filepath.Join(target, ".jvs", "snapshots", string(featureID)))
	assert.FileExists(t, filepath.Join(target, ".jvs", "descriptors", string(featureID)+".json"))
	assert.NoDirExists(t, filepath.Join(target, ".jvs", "worktrees", "feature"))

	manifest, ok, err := clonehistory.LoadValidatedManifest(target)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, result.SourceRepoID, manifest.SourceRepoID)
	assert.Equal(t, result.TargetRepoID, manifest.TargetRepoID)
	assert.Equal(t, clonehistory.SavePointsModeAll, manifest.SavePointsMode)
	assert.False(t, manifest.RuntimeStateCopied)
	assert.ElementsMatch(t, []model.SnapshotID{mainID, featureID}, manifest.ImportedSavePoints)
}

func TestCloneMainClosureFollowsDescriptorProvenanceWithoutWorktreeNameFilter(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")
	featureID, featurePath := createCleanFeatureWorkspaceSavePoint(t, source, mainID)
	assertFeatureDescriptor(t, source, featureID)

	featureContent, err := os.ReadFile(filepath.Join(featurePath, "feature.txt"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "feature.txt"), featureContent, 0644))
	require.NoError(t, os.Remove(filepath.Join(source, "app.txt")))
	mainCfg, err := repo.LoadWorktreeConfig(source, "main")
	require.NoError(t, err)
	mainCfg.HeadSnapshotID = featureID
	mainCfg.LatestSnapshotID = featureID
	require.NoError(t, repo.WriteWorktreeConfig(source, "main", mainCfg))

	target := filepath.Join(t.TempDir(), "target")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeMain,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	assert.ElementsMatch(t, []model.SnapshotID{mainID, featureID}, result.SavePointsCopied)
	assert.DirExists(t, filepath.Join(target, ".jvs", "snapshots", string(mainID)))
	assert.DirExists(t, filepath.Join(target, ".jvs", "snapshots", string(featureID)))
	assertFileContent(t, filepath.Join(target, "feature.txt"), "feature v1")
	assert.NoFileExists(t, filepath.Join(target, "app.txt"))
}

func TestCloneMainDanglingProvenanceFailsBeforePublishingTarget(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")
	missing := model.SnapshotID("1708300800000-deadbeef")
	rewriteDescriptorStartedFrom(t, source, mainID, missing)

	target := filepath.Join(t.TempDir(), "target")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeMain,
		RequestedEngine: model.EngineCopy,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
	assert.NoDirExists(t, target)
}

func TestCloneRejectsTargetInsideSourceMainWorkspaceBeforeStaging(t *testing.T) {
	t.Run("relative target from source root", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		withWorkingDir(t, source)

		_, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot: ".",
			TargetPath:     "target",
		})

		assertTargetInsideSourceWorkspaceError(t, err)
		assert.NoDirExists(t, filepath.Join(source, "target"))
		assertNoCloneStaging(t, source)
	})

	t.Run("absolute target under source root", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		target := filepath.Join(source, "target")

		_, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot: source,
			TargetPath:     target,
		})

		assertTargetInsideSourceWorkspaceError(t, err)
		assert.NoDirExists(t, target)
		assertNoCloneStaging(t, source)
	})

	t.Run("symlinked parent resolves under source root", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		base := t.TempDir()
		link := filepath.Join(base, "source-link")
		if err := os.Symlink(source, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		target := filepath.Join(link, "target")

		_, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot: source,
			TargetPath:     target,
		})

		assertTargetInsideSourceWorkspaceError(t, err)
		assert.NoDirExists(t, filepath.Join(source, "target"))
		assertNoCloneStaging(t, source)
	})
}

func TestCloneRejectsTargetInsideSourceProjectRootBeforeStaging(t *testing.T) {
	repoRoot, mainWorkspace := setupSplitCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(mainWorkspace, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, repoRoot, "main", "main baseline")
	target := filepath.Join(repoRoot, "clone-target")

	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  repoRoot,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		RequestedEngine: model.EngineCopy,
	})

	assertTargetInsideSourceProjectError(t, err)
	assert.NoDirExists(t, target)
	assertNoCloneStaging(t, repoRoot)
}

func TestCloneAcceptsSiblingTargetOutsideSourceWorkspace(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, source, "main", "main baseline")

	withWorkingDir(t, source)
	target := filepath.Join("..", "target")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  ".",
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeMain,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "target"), result.TargetRepoRoot)
	assertFileContent(t, filepath.Join(base, "target", "app.txt"), "main v1")
}

func TestCloneRejectsTargetInsideSourceExternalWorkspaceBeforeStaging(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")
	_, featurePath := createCleanFeatureWorkspaceSavePoint(t, source, mainID)
	target := filepath.Join(featurePath, "target")

	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot: source,
		TargetPath:     target,
		SavePointsMode: repoclone.SavePointsModeAll,
	})

	assertTargetInsideSourceWorkspaceError(t, err)
	assert.NoDirExists(t, target)
	assertNoCloneStaging(t, featurePath)
	assertNoCloneStaging(t, source)
}

func TestCloneRejectsDirtyMainAndDirtyNonMainBeforeTargetWrites(t *testing.T) {
	t.Run("main", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
		_ = createCloneSavePoint(t, source, "main", "main baseline")
		require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("unsaved"), 0644))

		target := filepath.Join(t.TempDir(), "target")
		_, err := repoclone.Clone(repoclone.Options{SourceRepoRoot: source, TargetPath: target, SavePointsMode: repoclone.SavePointsModeAll})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `source workspace "main" has unsaved changes`)
		assert.NoDirExists(t, target)
	})

	t.Run("non-main", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
		mainID := createCloneSavePoint(t, source, "main", "main baseline")
		_, featurePath := createCleanFeatureWorkspaceSavePoint(t, source, mainID)
		require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("unsaved"), 0644))

		target := filepath.Join(t.TempDir(), "target")
		_, err := repoclone.Clone(repoclone.Options{SourceRepoRoot: source, TargetPath: target, SavePointsMode: repoclone.SavePointsModeAll})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `source workspace "feature" has unsaved changes`)
		assert.NoDirExists(t, target)
	})
}

func TestCloneDryRunPlansExpectedTransfersWithoutCreatingTargetOrManifest(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, source, "main", "main baseline")

	target := filepath.Join(t.TempDir(), "target")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		DryRun:          true,
		RequestedEngine: model.EngineCopy,
	})
	require.NoError(t, err)

	assert.True(t, result.DryRun)
	assert.Equal(t, []model.SnapshotID{mainID}, result.SavePointsCopied)
	assert.NoDirExists(t, target)
	assert.Empty(t, result.TargetRepoID)
	require.Len(t, result.Transfers, 2)
	for _, record := range result.Transfers {
		assert.Equal(t, "expected", string(record.ResultKind))
		assert.Equal(t, "preview_only", string(record.PermissionScope))
		assert.Equal(t, "repo_clone", record.Operation)
	}
}

func TestCloneDryRunUsesReadOnlyPlanningWithoutProbeWrites(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, source, "main", "main baseline")

	targetParent := filepath.Join(t.TempDir(), "parent")
	require.NoError(t, os.MkdirAll(targetParent, 0755))
	target := filepath.Join(targetParent, "target")
	require.NoError(t, os.Chmod(targetParent, 0555))
	t.Cleanup(func() { _ = os.Chmod(targetParent, 0755) })

	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot: source,
		TargetPath:     target,
		SavePointsMode: repoclone.SavePointsModeAll,
		DryRun:         true,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.NoDirExists(t, target)
	assertNoProbeResidue(t, targetParent, source)
}

func TestCloneDryRunMarksTransferPlanningPreviewOnly(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, source, "main", "main baseline")
	planner := &recordingCloneTransferPlanner{}

	target := filepath.Join(t.TempDir(), "target")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		DryRun:          true,
		TransferPlanner: planner,
	})
	require.NoError(t, err)
	require.Len(t, planner.requests, 2)
	for _, req := range planner.requests {
		assert.True(t, req.PreviewOnly)
	}
}

func TestCloneCapacityGateRunsBeforeTargetWrites(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, source, "main", "main baseline")
	meter := &cloneCapacityMeter{available: 0}
	restore := repoclone.SetCapacityGateForTest(capacitygate.Gate{Meter: meter, SafetyMarginBytes: 0})
	t.Cleanup(restore)

	targetParent := t.TempDir()
	target := filepath.Join(targetParent, "target")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  source,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeAll,
		RequestedEngine: model.EngineCopy,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.True(t, meter.called)
	assert.NoDirExists(t, target)
	assertNoCloneStaging(t, targetParent)
}

func TestCloneTargetMustNotExistAndPublishUsesNoReplace(t *testing.T) {
	t.Run("existing empty directory", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		target := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.Mkdir(target, 0755))

		_, err := repoclone.Clone(repoclone.Options{SourceRepoRoot: source, TargetPath: target})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target folder already exists")
		assert.NoDirExists(t, filepath.Join(target, ".jvs"))
	})

	t.Run("existing symlink", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		base := t.TempDir()
		realTarget := filepath.Join(base, "real")
		target := filepath.Join(base, "target-link")
		require.NoError(t, os.Mkdir(realTarget, 0755))
		if err := os.Symlink(realTarget, target); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}

		_, err := repoclone.Clone(repoclone.Options{SourceRepoRoot: source, TargetPath: target})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target folder already exists")
		assert.NoDirExists(t, filepath.Join(realTarget, ".jvs"))
	})

	t.Run("concurrent final target", func(t *testing.T) {
		source := setupCloneSourceRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
		_ = createCloneSavePoint(t, source, "main", "main baseline")
		target := filepath.Join(t.TempDir(), "target")

		_, err := repoclone.Clone(repoclone.Options{
			SourceRepoRoot:  source,
			TargetPath:      target,
			SavePointsMode:  repoclone.SavePointsModeAll,
			RequestedEngine: model.EngineCopy,
			Hooks: repoclone.Hooks{
				BeforePublish: func(stagingPath, targetPath string) error {
					require.DirExists(t, stagingPath)
					return os.Mkdir(targetPath, 0755)
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "publish")
		assert.DirExists(t, target)
		assert.NoDirExists(t, filepath.Join(target, ".jvs"))
	})
}

func TestIsRemoteLikeInputRejectsURLsAndSCPStyleWithoutRejectingLocalDrivePaths(t *testing.T) {
	for _, input := range []string{
		"https://example.com/repo",
		"ssh://host/path",
		"git@host:org/repo",
		"user@host:path",
		"github.com:org/repo",
		"host:org/repo",
	} {
		require.True(t, repoclone.IsRemoteLikeInput(input), input)
	}
	for _, input := range []string{
		"/tmp/user@host:path",
		"local:path",
		`C:\work\project`,
		"D:/work/project",
		"C:work/project",
	} {
		require.False(t, repoclone.IsRemoteLikeInput(input), input)
	}
}

type transferExpectation int

const (
	transferFinalExecution transferExpectation = iota
)

func assertRepoCloneTransfer(t *testing.T, record transfer.Record, id, phase, sourceRole, destinationRole string, expectation transferExpectation) {
	t.Helper()

	assert.Equal(t, id, record.TransferID)
	assert.Equal(t, "repo_clone", record.Operation)
	assert.Equal(t, phase, record.Phase)
	assert.True(t, record.Primary)
	assert.Equal(t, sourceRole, record.SourceRole)
	assert.Equal(t, destinationRole, record.DestinationRole)
	assert.True(t, record.CheckedForThisOperation)
	if expectation == transferFinalExecution {
		assert.Equal(t, transfer.ResultKindFinal, record.ResultKind)
		assert.Equal(t, transfer.PermissionScopeExecution, record.PermissionScope)
	}
	assert.NotEmpty(t, record.SourcePath)
	assert.NotEmpty(t, record.MaterializationDestination)
	assert.NotEmpty(t, record.PublishedDestination)
}

func setupCloneSourceRepo(t *testing.T) string {
	t.Helper()

	source := t.TempDir()
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(source, ".jvs", "intents"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, ".jvs", "intents", "source-runtime.json"), []byte("{}"), 0644))
	return source
}

func setupSplitCloneSourceRepo(t *testing.T) (string, string) {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), "source")
	_, err := repo.Init(repoRoot, "source")
	require.NoError(t, err)
	return repoRoot, filepath.Join(repoRoot, "main")
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(original)) })
}

func assertTargetInsideSourceWorkspaceError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected public usage error, got %T: %v", err, err)
	assert.Equal(t, errclass.ErrUsage.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "target cannot be inside a source workspace")
	assert.Contains(t, jvsErr.Hint, "Choose a folder outside the source project/workspaces")
	assert.NotContains(t, err.Error(), "nested")
	assert.NotContains(t, err.Error(), "staging")
}

func assertTargetInsideSourceProjectError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected public usage error, got %T: %v", err, err)
	assert.Equal(t, errclass.ErrUsage.Code, jvsErr.Code)
	assert.Contains(t, jvsErr.Message, "target cannot be inside the source project")
	assert.Contains(t, jvsErr.Hint, "Choose a folder outside the source project/workspaces")
	assert.NotContains(t, err.Error(), "nested")
	assert.NotContains(t, err.Error(), "staging")
}

func assertNoCloneStaging(t *testing.T, dir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.clone-staging-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func assertNoProbeResidue(t *testing.T, dirs ...string) {
	t.Helper()

	for _, dir := range dirs {
		for _, pattern := range []string{".jvs-capability-*", ".jvs-transfer-pair-*"} {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			require.NoError(t, err)
			assert.Empty(t, matches, "unexpected dry-run probe residue under %s", dir)
		}
	}
}

type recordingCloneTransferPlanner struct {
	requests []engine.TransferPlanRequest
}

func (p *recordingCloneTransferPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	p.requests = append(p.requests, req)
	return &engine.TransferPlan{
		RequestedEngine:   req.RequestedEngine,
		TransferEngine:    model.EngineCopy,
		EffectiveEngine:   model.EngineCopy,
		OptimizedTransfer: false,
		DegradedReasons:   []string{},
		Warnings:          []string{},
	}, nil
}

type cloneCapacityMeter struct {
	available int64
	called    bool
}

func (m *cloneCapacityMeter) AvailableBytes(string) (int64, error) {
	m.called = true
	return m.available, nil
}

func createCloneSavePoint(t *testing.T, repoRoot, workspaceName, note string) model.SnapshotID {
	t.Helper()

	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint(workspaceName, note, nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func createCleanFeatureWorkspaceSavePoint(t *testing.T, repoRoot string, sourceID model.SnapshotID) (model.SnapshotID, string) {
	t.Helper()

	featurePath := filepath.Join(t.TempDir(), "feature")
	cfg, err := worktree.NewManager(repoRoot).CreateStartedFromSnapshotAt(worktree.StartedFromSnapshotRequest{
		Name:            "feature",
		Folder:          featurePath,
		SnapshotID:      sourceID,
		RequestedEngine: model.EngineCopy,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "feature", cfg.Name)
	require.NoError(t, os.Remove(filepath.Join(featurePath, "app.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature v1"), 0644))
	id := createCloneSavePoint(t, repoRoot, "feature", "feature baseline")
	return id, featurePath
}

func assertFeatureDescriptor(t *testing.T, repoRoot string, snapshotID model.SnapshotID) {
	t.Helper()

	desc, err := snapshot.LoadDescriptor(repoRoot, snapshotID)
	require.NoError(t, err)
	require.Equal(t, "feature", desc.WorktreeName)
}

func rewriteDescriptorStartedFrom(t *testing.T, repoRoot string, snapshotID, startedFrom model.SnapshotID) {
	t.Helper()

	desc, err := snapshot.LoadDescriptor(repoRoot, snapshotID)
	require.NoError(t, err)
	desc.StartedFrom = &startedFrom
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum
	descriptorData, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(repoRoot, snapshotID)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, descriptorData, 0644))

	readyPath := filepath.Join(repoRoot, ".jvs", "snapshots", string(snapshotID), ".READY")
	var ready model.ReadyMarker
	readyData, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(readyData, &ready))
	ready.DescriptorChecksum = checksum
	readyData, err = json.MarshalIndent(ready, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readyPath, readyData, 0644))
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}
