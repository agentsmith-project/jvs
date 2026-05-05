package repoclone_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/repoclone"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedCloneMainCreatesSplitTargetWithNewRepoIDAndPayloadOnly(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, sourceControl, "main", "main baseline")
	seedSeparatedCloneRuntimeSentinels(t, sourceControl)

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	require.NoError(t, err)

	assert.Equal(t, "repo_clone", result.Operation)
	assert.Equal(t, sourceControl, result.SourceRepoRoot)
	assert.Empty(t, result.TargetRepoRoot)
	assert.Equal(t, targetPayload, result.TargetFolder)
	assert.Equal(t, targetControl, result.TargetControlRoot)
	assert.Equal(t, targetPayload, result.TargetPayloadRoot)
	assert.Equal(t, "passed", result.DoctorStrict)
	assert.Equal(t, repoclone.SavePointsModeMain, result.SavePointsMode)
	assert.Equal(t, []model.SnapshotID{mainID}, result.SavePointsCopied)
	assert.Equal(t, []string{"main"}, result.WorkspacesCreated)
	assert.False(t, result.RuntimeStateCopied)
	require.Len(t, result.Transfers, 2)
	assertRepoCloneTransfer(t, result.Transfers[0], "repo-clone-save-points", "save_point_storage_copy", "save_point_storage", "target_save_point_storage", transferFinalExecution)
	assert.Equal(t, filepath.Join(sourceControl, ".jvs"), result.Transfers[0].SourcePath)
	assert.Equal(t, filepath.Join(targetControl, ".jvs"), result.Transfers[0].PublishedDestination)
	assert.NotContains(t, filepath.ToSlash(result.Transfers[0].MaterializationDestination), filepath.ToSlash(targetPayload))
	assertRepoCloneTransfer(t, result.Transfers[1], "repo-clone-main-workspace", "main_workspace_materialization", "source_main_current_state", "target_main_workspace", transferFinalExecution)
	assert.Equal(t, sourcePayload, result.Transfers[1].SourcePath)
	assert.Equal(t, targetPayload, result.Transfers[1].PublishedDestination)
	assert.Equal(t, filepath.Dir(targetPayload), result.Transfers[1].CapabilityProbePath)
	assert.NotContains(t, filepath.ToSlash(result.Transfers[1].MaterializationDestination), filepath.ToSlash(targetControl))

	sourceRepo, err := repo.OpenControlRoot(sourceControl)
	require.NoError(t, err)
	targetRepo, err := repo.OpenControlRoot(targetControl)
	require.NoError(t, err)
	assert.NotEqual(t, sourceRepo.RepoID, targetRepo.RepoID)
	assert.Equal(t, sourceRepo.RepoID, result.SourceRepoID)
	assert.Equal(t, targetRepo.RepoID, result.TargetRepoID)

	targetCtx, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{ControlRoot: targetControl, Workspace: "main"})
	require.NoError(t, err)
	assert.Equal(t, targetPayload, targetCtx.PayloadRoot)
	assert.NoDirExists(t, filepath.Join(targetPayload, ".jvs"))
	assert.FileExists(t, filepath.Join(targetControl, ".jvs", "repo_id"))
	assertFileContent(t, filepath.Join(targetPayload, "app.txt"), "main v1")
	assert.NoFileExists(t, filepath.Join(targetControl, ".jvs", "locks", "platform.lock"))
	assert.NoFileExists(t, filepath.Join(targetControl, ".jvs", "runtime", "platform.tmp"))
	assert.NoFileExists(t, filepath.Join(targetControl, ".jvs", "restore-plans", "platform-plan.json"))
	assert.NoFileExists(t, filepath.Join(targetControl, ".jvs", "views", "source-view-state"))
	assertSeparatedCloneRuntimeSentinelsIntact(t, sourceControl)

	cfg, err := repo.LoadWorktreeConfig(targetControl, "main")
	require.NoError(t, err)
	assert.Equal(t, "main", cfg.Name)
	assert.Equal(t, targetPayload, cfg.RealPath)
	assert.Equal(t, mainID, cfg.HeadSnapshotID)
	assert.Equal(t, mainID, cfg.LatestSnapshotID)

	copied, err := snapshot.ListAll(targetControl)
	require.NoError(t, err)
	require.Len(t, copied, 1)
	assert.Equal(t, mainID, copied[0].SnapshotID)

	strict, err := doctor.CheckSeparatedStrict(repo.SeparatedContextRequest{ControlRoot: targetControl, Workspace: "main"})
	require.NoError(t, err)
	assert.True(t, strict.Healthy)
	assert.Equal(t, "passed", strict.DoctorStrict)
}

func TestSeparatedCloneAcceptsTargetPathWithTargetControlRoot(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	mainID := createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetFolder := filepath.Join(targetBase, "target-folder")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetPath:        targetFolder,
		TargetControlRoot: targetControl,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	require.NoError(t, err)

	assert.Empty(t, result.TargetRepoRoot)
	assert.Equal(t, targetFolder, result.TargetFolder)
	assert.Equal(t, targetControl, result.TargetControlRoot)
	assert.Equal(t, targetFolder, result.TargetPayloadRoot)
	assert.Equal(t, []model.SnapshotID{mainID}, result.SavePointsCopied)
	assertFileContent(t, filepath.Join(targetFolder, "app.txt"), "main v1")
	assert.NoDirExists(t, filepath.Join(targetFolder, ".jvs"))
	assert.FileExists(t, filepath.Join(targetControl, ".jvs", "repo_id"))
}

func TestSeparatedCloneAcceptsEmptyTargetRoots(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "empty-control")
	targetPayload := filepath.Join(targetBase, "empty-payload")
	require.NoError(t, os.MkdirAll(targetControl, 0755))
	require.NoError(t, os.MkdirAll(targetPayload, 0755))

	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	require.NoError(t, err)

	assert.Empty(t, result.TargetRepoRoot)
	assert.Equal(t, targetPayload, result.TargetFolder)
	assert.Equal(t, targetControl, result.TargetControlRoot)
	assert.FileExists(t, filepath.Join(targetControl, ".jvs", "repo_id"))
	assert.NoDirExists(t, filepath.Join(targetPayload, ".jvs"))
	assertFileContent(t, filepath.Join(targetPayload, "app.txt"), "main v1")
}

func TestSeparatedCloneRejectsAllModeForSplitTargetFromEmbeddedSource(t *testing.T) {
	source := setupCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, source, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    source,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeAll,
		RequestedEngine:   model.EngineCopy,
	})

	assertJVSErrorCode(t, err, errclass.ErrImportedHistoryProtectionMissing.Code)
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
	assert.NoDirExists(t, filepath.Join(targetPayload, ".jvs"))
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
}

func TestSeparatedCloneRejectsTargetOverlapOccupiedAndAllProtection(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	for _, tc := range []struct {
		name        string
		setup       func(base string) (controlRoot, payloadRoot string)
		mode        repoclone.SavePointsMode
		code        string
		wantMessage string
	}{
		{
			name: "target control occupied",
			setup: func(base string) (string, string) {
				controlRoot := filepath.Join(base, "target-control")
				payloadRoot := filepath.Join(base, "target-payload")
				require.NoError(t, os.MkdirAll(controlRoot, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, "occupied.txt"), []byte("keep"), 0644))
				return controlRoot, payloadRoot
			},
			mode: repoclone.SavePointsModeMain,
			code: errclass.ErrTargetRootOccupied.Code,
		},
		{
			name: "target payload occupied",
			setup: func(base string) (string, string) {
				controlRoot := filepath.Join(base, "target-control")
				payloadRoot := filepath.Join(base, "target-payload")
				require.NoError(t, os.MkdirAll(payloadRoot, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "occupied.txt"), []byte("keep"), 0644))
				return controlRoot, payloadRoot
			},
			mode:        repoclone.SavePointsModeMain,
			code:        errclass.ErrTargetRootOccupied.Code,
			wantMessage: "target folder",
		},
		{
			name: "same target root",
			setup: func(base string) (string, string) {
				root := filepath.Join(base, "target")
				return root, root
			},
			mode:        repoclone.SavePointsModeMain,
			code:        errclass.ErrControlWorkspaceOverlap.Code,
			wantMessage: "workspace folder",
		},
		{
			name: "payload inside control",
			setup: func(base string) (string, string) {
				controlRoot := filepath.Join(base, "target-control")
				return controlRoot, filepath.Join(controlRoot, "payload")
			},
			mode:        repoclone.SavePointsModeMain,
			code:        errclass.ErrWorkspaceInsideControl.Code,
			wantMessage: "target folder",
		},
		{
			name: "control inside payload",
			setup: func(base string) (string, string) {
				payloadRoot := filepath.Join(base, "target-payload")
				return filepath.Join(payloadRoot, "control"), payloadRoot
			},
			mode:        repoclone.SavePointsModeMain,
			code:        errclass.ErrControlInsideWorkspace.Code,
			wantMessage: "target folder",
		},
		{
			name: "all protection missing",
			setup: func(base string) (string, string) {
				return filepath.Join(base, "target-control"), filepath.Join(base, "target-payload")
			},
			mode: repoclone.SavePointsModeAll,
			code: errclass.ErrImportedHistoryProtectionMissing.Code,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot, payloadRoot := tc.setup(base)
			_, err := repoclone.Clone(repoclone.Options{
				SourceRepoRoot:    sourceControl,
				TargetControlRoot: controlRoot,
				TargetPayloadRoot: payloadRoot,
				SavePointsMode:    tc.mode,
				RequestedEngine:   model.EngineCopy,
			})

			assertJVSErrorCode(t, err, tc.code)
			if tc.wantMessage != "" {
				assert.NotContains(t, err.Error(), "payload root")
				assert.Contains(t, err.Error(), tc.wantMessage)
			}
			assert.NoDirExists(t, filepath.Join(controlRoot, ".jvs"))
			assert.NoDirExists(t, filepath.Join(payloadRoot, ".jvs"))
		})
	}
}

func TestSeparatedCloneRejectsPositionalEmbeddedTarget(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	target := filepath.Join(t.TempDir(), "embedded-target")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:  sourceControl,
		TargetPath:      target,
		SavePointsMode:  repoclone.SavePointsModeMain,
		RequestedEngine: model.EngineCopy,
	})

	assertJVSErrorCode(t, err, errclass.ErrExplicitTargetRequired.Code)
	assert.NoDirExists(t, filepath.Join(target, ".jvs"))
}

func TestSeparatedCloneRejectsActiveSourceStateBeforeTargetWrites(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, controlRoot string)
		code  string
	}{
		{
			name: "repo mutation lock",
			setup: func(t *testing.T, controlRoot string) {
				lock, err := repo.AcquireMutationLock(controlRoot, "held by test")
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, lock.Release()) })
			},
			code: errclass.ErrActiveOperationBlocking.Code,
		},
		{
			name: "lifecycle pending",
			setup: func(t *testing.T, controlRoot string) {
				sourceRepo, err := repo.OpenControlRoot(controlRoot)
				require.NoError(t, err)
				require.NoError(t, lifecycle.WriteOperation(controlRoot, lifecycle.OperationRecord{
					SchemaVersion:          lifecycle.SchemaVersion,
					OperationID:            "op-pending",
					OperationType:          "repo_move",
					RepoID:                 sourceRepo.RepoID,
					Phase:                  "prepared",
					RecommendedNextCommand: "jvs repo move --run op-pending",
					CreatedAt:              time.Now().UTC(),
					UpdatedAt:              time.Now().UTC(),
				}))
			},
			code: errclass.ErrActiveOperationBlocking.Code,
		},
		{
			name: "snapshot intent",
			setup: func(t *testing.T, controlRoot string) {
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "intents", "1708300800000-a3f7c1b2.json"), []byte("{}\n"), 0644))
			},
			code: errclass.ErrActiveOperationBlocking.Code,
		},
		{
			name: "cleanup plan",
			setup: func(t *testing.T, controlRoot string) {
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "gc", "cleanup-plan.json"), []byte("{}\n"), 0644))
			},
			code: errclass.ErrActiveOperationBlocking.Code,
		},
		{
			name: "restore plan",
			setup: func(t *testing.T, controlRoot string) {
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "restore-plan.json"), []byte("{}\n"), 0644))
			},
			code: errclass.ErrRecoveryBlocking.Code,
		},
		{
			name: "restore plans directory symlink",
			setup: func(t *testing.T, controlRoot string) {
				plansDir := filepath.Join(controlRoot, ".jvs", "restore-plans")
				require.NoError(t, os.RemoveAll(plansDir))
				outsideDir := filepath.Join(filepath.Dir(controlRoot), "outside-restore-plans")
				require.NoError(t, os.MkdirAll(outsideDir, 0755))
				if err := os.Symlink(outsideDir, plansDir); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			code: errclass.ErrRecoveryBlocking.Code,
		},
		{
			name: "recovery plan",
			setup: func(t *testing.T, controlRoot string) {
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "recovery-plans", "RP-active.json"), []byte("{}\n"), 0644))
			},
			code: errclass.ErrRecoveryBlocking.Code,
		},
		{
			name: "active recovery identity mismatch",
			setup: func(t *testing.T, controlRoot string) {
				cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
				require.NoError(t, err)
				source := createCloneSavePoint(t, controlRoot, "main", "recovery source")
				r, err := repo.OpenControlRoot(controlRoot)
				require.NoError(t, err)
				now := time.Now().UTC()
				plan := &recovery.Plan{
					SchemaVersion:          recovery.SchemaVersion,
					RepoID:                 r.RepoID,
					PlanID:                 "RP-identity-mismatch",
					Status:                 recovery.StatusActive,
					Operation:              recovery.OperationRestore,
					RestorePlanID:          "restore-preview",
					Workspace:              "feature",
					Folder:                 cfg.RealPath,
					SourceSavePoint:        source,
					CreatedAt:              now,
					UpdatedAt:              now,
					PreWorktreeState:       recovery.WorktreeState{Name: "main", RealPath: cfg.RealPath},
					Backup:                 recovery.Backup{Path: filepath.Join(filepath.Dir(cfg.RealPath), "backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
					Phase:                  recovery.PhasePending,
					RecommendedNextCommand: "jvs recovery status RP-identity-mismatch",
				}
				require.NoError(t, recovery.NewManager(controlRoot).Write(plan))
			},
			code: errclass.ErrRecoveryBlocking.Code,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
			require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
			_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")
			tc.setup(t, sourceControl)

			targetBase := t.TempDir()
			targetControl := filepath.Join(targetBase, "target-control")
			targetPayload := filepath.Join(targetBase, "target-payload")
			_, err := repoclone.Clone(repoclone.Options{
				SourceRepoRoot:    sourceControl,
				TargetControlRoot: targetControl,
				TargetPayloadRoot: targetPayload,
				SavePointsMode:    repoclone.SavePointsModeMain,
				RequestedEngine:   model.EngineCopy,
			})

			assertJVSErrorCode(t, err, tc.code)
			assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
			assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
		})
	}
}

func TestSeparatedCloneAllowsCompletedRestoreResidue(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	plan := createSeparatedCloneRestorePreview(t, sourceControl, sourcePayload, "source\n")
	writeSeparatedCloneResolvedRecovery(t, sourceControl, plan)

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	require.NoError(t, err)
	assert.Equal(t, "repo_clone", result.Operation)
	assertFileContent(t, filepath.Join(targetPayload, "app.txt"), "source\n")
	assert.NoDirExists(t, filepath.Join(targetPayload, ".jvs"))
}

func TestSeparatedCloneRestorePlanDiagnosticsUsePublicCommands(t *testing.T) {
	t.Run("pending preview", func(t *testing.T) {
		sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("source\n"), 0644))
		source := createCloneSavePoint(t, sourceControl, "main", "source")
		require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("current\n"), 0644))
		_ = createCloneSavePoint(t, sourceControl, "main", "current")
		plan, err := restoreplan.Create(sourceControl, "main", source, model.EngineCopy, restoreplan.Options{})
		require.NoError(t, err)

		err = cloneSeparatedSourceForError(t, sourceControl)

		assertJVSErrorCode(t, err, errclass.ErrRecoveryBlocking.Code)
		assert.Contains(t, err.Error(), "restore plan "+plan.PlanID)
		assert.NotContains(t, err.Error(), "restore --run "+plan.PlanID)
		assertJVSErrorHintContains(t, err, "restore --run "+plan.PlanID)
		assert.NotContains(t, err.Error(), ".jvs/restore-plans")
	})

	t.Run("malformed preview", func(t *testing.T) {
		sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("source\n"), 0644))
		_ = createCloneSavePoint(t, sourceControl, "main", "source")
		require.NoError(t, os.WriteFile(filepath.Join(sourceControl, ".jvs", "restore-plans", "corrupt-pending.json"), []byte("{not-json\n"), 0644))

		err := cloneSeparatedSourceForError(t, sourceControl)

		assertJVSErrorCode(t, err, errclass.ErrRecoveryBlocking.Code)
		assert.Contains(t, err.Error(), "restore plan corrupt-pending")
		assert.Contains(t, err.Error(), "not valid JSON")
		assert.NotContains(t, err.Error(), "doctor --strict --json")
		assertJVSErrorHintContains(t, err, "doctor --strict --json")
		assert.NotContains(t, err.Error(), ".jvs/restore-plans")
	})

	t.Run("malformed preview wins over pending preview", func(t *testing.T) {
		sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
		plan := createSeparatedCloneRestorePreview(t, sourceControl, sourcePayload, "source\n")
		require.NoError(t, os.WriteFile(filepath.Join(sourceControl, ".jvs", "restore-plans", "zzzz-corrupt.json"), []byte("{not-json\n"), 0644))

		err := cloneSeparatedSourceForError(t, sourceControl)

		assertJVSErrorCode(t, err, errclass.ErrRecoveryBlocking.Code)
		assert.Contains(t, err.Error(), "restore plan zzzz-corrupt")
		assert.Contains(t, err.Error(), "not valid JSON")
		assert.NotContains(t, err.Error(), "restore plan "+plan.PlanID)
		assertJVSErrorHintContains(t, err, "doctor --strict --json")
		assert.NotContains(t, err.Error(), ".jvs/restore-plans")
	})
}

func TestSeparatedCloneRejectsSourcePayloadSymlinkEscapeBeforeTargetWrites(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	if err := os.Symlink(filepath.Join(sourceControl, ".jvs", "repo_id"), filepath.Join(sourcePayload, "control-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})

	assertJVSErrorCode(t, err, errclass.ErrPathBoundaryEscape.Code)
	assert.NoDirExists(t, targetControl)
	assert.NoDirExists(t, targetPayload)
	assertFileContent(t, filepath.Join(sourcePayload, "app.txt"), "main v1")
}

func TestSeparatedCloneAtomicControlPublishFailureRollsBackPayloadAndRetrySucceeds(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			AfterSeparatedPayloadPublish: func(publishedPayloadRoot, targetPayloadRoot string) error {
				require.Equal(t, targetPayload, targetPayloadRoot)
				require.DirExists(t, publishedPayloadRoot)
				require.NoError(t, os.MkdirAll(targetControl, 0755))
				return os.WriteFile(filepath.Join(targetControl, "block-control-publish.txt"), []byte("block"), 0644)
			},
		},
	})
	assertJVSErrorCode(t, err, errclass.ErrAtomicPublishBlocked.Code)
	assertFileContent(t, filepath.Join(sourcePayload, "app.txt"), "main v1")
	assert.NoDirExists(t, targetPayload)
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))

	require.NoError(t, os.RemoveAll(targetControl))
	result, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	require.NoError(t, err)
	assert.Equal(t, targetControl, result.TargetControlRoot)
	assertFileContent(t, filepath.Join(targetPayload, "app.txt"), "main v1")
}

func TestSeparatedCloneControlPublishFailureRestoresPreexistingEmptyPayloadRoot(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	require.NoError(t, os.MkdirAll(targetControl, 0755))
	require.NoError(t, os.MkdirAll(targetPayload, 0755))

	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			AfterSeparatedPayloadPublish: func(publishedPayloadRoot, targetPayloadRoot string) error {
				require.Equal(t, targetPayload, targetPayloadRoot)
				require.DirExists(t, publishedPayloadRoot)
				return os.WriteFile(filepath.Join(targetControl, "block-control-publish.txt"), []byte("block"), 0644)
			},
		},
	})

	assertJVSErrorCode(t, err, errclass.ErrAtomicPublishBlocked.Code)
	assert.DirExists(t, targetPayload)
	assert.Empty(t, readCloneDirNames(t, targetPayload))
	assert.NoDirExists(t, filepath.Join(targetPayload, ".jvs"))
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
	assertFileContent(t, filepath.Join(targetControl, "block-control-publish.txt"), "block")
}

func TestSeparatedCloneControlPublishFailureDoesNotRemoveReplacedPayload(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			AfterSeparatedPayloadPublish: func(publishedPayloadRoot, targetPayloadRoot string) error {
				require.Equal(t, targetPayload, targetPayloadRoot)
				require.DirExists(t, publishedPayloadRoot)
				require.NoError(t, os.RemoveAll(targetPayload))
				require.NoError(t, os.MkdirAll(targetPayload, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(targetPayload, "external.txt"), []byte("external replacement"), 0644))
				require.NoError(t, os.MkdirAll(targetControl, 0755))
				return os.WriteFile(filepath.Join(targetControl, "block-control-publish.txt"), []byte("block"), 0644)
			},
		},
	})

	assertJVSErrorCode(t, err, errclass.ErrAtomicPublishBlocked.Code)
	assertFileContent(t, filepath.Join(targetPayload, "external.txt"), "external replacement")
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
}

func TestSeparatedCloneControlPublishFailureRollbackDoesNotRemoveExternallyModifiedPayload(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			AfterSeparatedPayloadPublish: func(publishedPayloadRoot, targetPayloadRoot string) error {
				require.Equal(t, targetPayload, targetPayloadRoot)
				require.DirExists(t, publishedPayloadRoot)
				require.NoError(t, os.WriteFile(filepath.Join(targetPayload, "external.txt"), []byte("external write"), 0644))
				require.NoError(t, os.MkdirAll(targetControl, 0755))
				return os.WriteFile(filepath.Join(targetControl, "block-control-publish.txt"), []byte("block"), 0644)
			},
		},
	})

	assertJVSErrorCode(t, err, errclass.ErrAtomicPublishBlocked.Code)
	assert.ErrorContains(t, err, "rollback target folder")
	assert.ErrorContains(t, err, "target folder changed after publish; refusing to remove")
	assertFileContent(t, filepath.Join(targetPayload, "external.txt"), "external write")
	assertFileContent(t, filepath.Join(targetPayload, "app.txt"), "main v1")
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
}

func TestSeparatedCloneMaterializesMainFromSavedStateWhenSourceMutatesAfterPrepare(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("saved state"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")
	planner := &mutatingCloneTransferPlanner{
		mutate: func() {
			require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("unsaved race"), 0644))
		},
	}

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		TransferPlanner:   planner,
	})
	assertJVSErrorCode(t, err, errclass.ErrSourceDirty.Code)
	assertFileContent(t, filepath.Join(sourcePayload, "app.txt"), "unsaved race")
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
}

func TestSeparatedCloneSourceRepoIDMismatchBeforePublishFailsStableContract(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			BeforePublish: func(stagingRoot, targetRoot string) error {
				return os.WriteFile(filepath.Join(sourceControl, ".jvs", "repo_id"), []byte("different-source-repo-id\n"), 0600)
			},
		},
	})

	assertJVSErrorCode(t, err, errclass.ErrRepoIDMismatch.Code)
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
}

func TestSeparatedCloneSourceCleanSavePointDriftBeforePublishFailsSourceDirty(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	baseline := createCloneSavePoint(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
		Hooks: repoclone.Hooks{
			BeforePublish: func(stagingRoot, targetRoot string) error {
				require.DirExists(t, stagingRoot)
				require.Equal(t, targetControl, targetRoot)
				require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v2"), 0644))
				next := createCloneSavePoint(t, sourceControl, "main", "main concurrent save")
				require.NotEqual(t, baseline, next)
				return nil
			},
		},
	})

	assertJVSErrorCode(t, err, errclass.ErrSourceDirty.Code)
	assertFileContent(t, filepath.Join(sourcePayload, "app.txt"), "main v2")
	assert.NoFileExists(t, filepath.Join(targetPayload, "app.txt"))
	assert.NoDirExists(t, filepath.Join(targetControl, ".jvs"))
}

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

func TestCloneRejectsTargetInsideInitializedSourceMainWorkspaceBeforeStaging(t *testing.T) {
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

	assertTargetInsideSourceWorkspaceError(t, err)
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
	mainWorkspace, err := repo.WorktreePayloadPath(repoRoot, "main")
	require.NoError(t, err)
	return repoRoot, mainWorkspace
}

func setupSeparatedCloneSourceRepo(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	controlRoot := filepath.Join(base, "source-control")
	payloadRoot := filepath.Join(base, "source-payload")
	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	return controlRoot, payloadRoot
}

func createSeparatedCloneRestorePreview(t *testing.T, controlRoot, payloadRoot, content string) *restoreplan.Plan {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte(content), 0644))
	source := createCloneSavePoint(t, controlRoot, "main", "source")
	plan, err := restoreplan.Create(controlRoot, "main", source, model.EngineCopy, restoreplan.Options{})
	require.NoError(t, err)
	return plan
}

func writeSeparatedCloneResolvedRecovery(t *testing.T, controlRoot string, restorePlan *restoreplan.Plan) {
	t.Helper()

	r, err := repo.OpenControlRoot(controlRoot)
	require.NoError(t, err)
	now := time.Now().UTC()
	resolved := &recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 "RP-" + restorePlan.PlanID,
		Status:                 recovery.StatusResolved,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          restorePlan.PlanID,
		Workspace:              restorePlan.Workspace,
		Folder:                 restorePlan.Folder,
		SourceSavePoint:        restorePlan.SourceSavePoint,
		CreatedAt:              now,
		UpdatedAt:              now,
		ResolvedAt:             &now,
		PreWorktreeState:       recovery.WorktreeState{Name: restorePlan.Workspace, RealPath: restorePlan.Folder},
		Backup:                 recovery.Backup{Path: filepath.Join(filepath.Dir(restorePlan.Folder), "restore-backup"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRolledBack},
		Phase:                  recovery.PhaseRestoreApplied,
		RecommendedNextCommand: "jvs recovery status RP-" + restorePlan.PlanID,
	}
	require.NoError(t, recovery.NewManager(controlRoot).Write(resolved))
}

func cloneSeparatedSourceForError(t *testing.T, sourceControl string) error {
	t.Helper()

	targetBase := t.TempDir()
	_, err := repoclone.Clone(repoclone.Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: filepath.Join(targetBase, "target-control"),
		TargetPayloadRoot: filepath.Join(targetBase, "target-payload"),
		SavePointsMode:    repoclone.SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})
	return err
}

func seedSeparatedCloneRuntimeSentinels(t *testing.T, controlRoot string) {
	t.Helper()

	for _, name := range []string{"locks", "runtime", "views"} {
		require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), []byte("lock sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "views", "source-view-state"), []byte("{}\n"), 0644))
}

func assertSeparatedCloneRuntimeSentinelsIntact(t *testing.T, controlRoot string) {
	t.Helper()

	assertFileContent(t, filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), "lock sentinel\n")
	assertFileContent(t, filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), "runtime sentinel\n")
	assertFileContent(t, filepath.Join(controlRoot, ".jvs", "views", "source-view-state"), "{}\n")
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

func assertJVSErrorCode(t *testing.T, err error, code string) {
	t.Helper()

	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected JVS error, got %T: %v", err, err)
	assert.Equal(t, code, jvsErr.Code)
}

func assertJVSErrorHintContains(t *testing.T, err error, want string) {
	t.Helper()

	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected JVS error, got %T: %v", err, err)
	assert.Contains(t, jvsErr.Hint, want)
}

func assertNoCloneStaging(t *testing.T, dir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.clone-staging-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func readCloneDirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
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

type mutatingCloneTransferPlanner struct {
	mutate  func()
	mutated bool
}

func (p *mutatingCloneTransferPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	if !p.mutated && p.mutate != nil {
		p.mutated = true
		p.mutate()
	}
	return (&recordingCloneTransferPlanner{}).PlanTransfer(req)
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
