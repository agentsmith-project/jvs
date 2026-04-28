package cli

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWholeRestoreFailureCreatesRecoveryPlanStatusAndProtectsSource(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "Restore did not finish safely.")
	assert.Contains(t, err.Error(), "Recovery plan:")
	assert.Contains(t, err.Error(), "jvs recovery status")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	statusOut, err := executeCommand(createTestRootCmd(), "recovery", "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, recoveryPlanID)
	assert.Contains(t, statusOut, "active")
	assertRecoveryOutputOmitsInternalVocabulary(t, statusOut)

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "recovery", "status", recoveryPlanID)
	require.NoError(t, err)
	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "recovery status", env.Command)
	assert.Equal(t, recoveryPlanID, data["plan_id"])
	assert.Equal(t, "active", data["status"])
	assert.Equal(t, "restore", data["operation"])
	assert.Equal(t, sourceID, data["source_save_point"])
	assert.Contains(t, data["recommended_next_command"], "jvs recovery")
	assertRecoveryOutputOmitsInternalVocabulary(t, jsonOut)

	gcPlan, err := gc.NewCollector(repoRoot).PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.Contains(t, gcPlan.ProtectedSet, model.SnapshotID(sourceID))

	anotherPreview, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	anotherRestorePlanID := restorePlanIDFromHumanOutput(t, anotherPreview)
	stdout, err = executeCommand(createTestRootCmd(), "restore", "--run", anotherRestorePlanID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "active recovery plan")
	assert.Contains(t, err.Error(), "jvs recovery status")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
}

func TestRestoreRunCommitUncertainVisibleRecoveryPlanStopsBeforeMutationAndProtectsSource(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		require.NoError(t, os.WriteFile(path, data, perm))
		return &fsutil.CommitUncertainError{Op: "atomic write", Path: path, Err: errors.New("injected post-commit fsync failure")}
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "jvs recovery status")
	assert.Contains(t, err.Error(), "jvs recovery resume")
	assert.Contains(t, err.Error(), "jvs recovery rollback")
	assert.Contains(t, err.Error(), "No files were changed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())

	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, recovery.StatusActive, plans[0].Status)
	assert.Equal(t, model.SnapshotID(sourceID), plans[0].SourceSavePoint)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)

	gcPlan, err := gc.NewCollector(repoRoot).PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.Contains(t, gcPlan.ProtectedSet, model.SnapshotID(sourceID))
}

func TestRecoveryResumeResolvesStalePlanAfterSuccessfulRestorePlanWriteFailure(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected post-mutation recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	recoveryPlanID := plans[0].PlanID
	assert.Equal(t, recovery.StatusActive, plans[0].Status)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestWholeRecoveryRollbackRestoresFilesMetadataAndResolvesPlan(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)
	originalBackupCount := restoreBackupCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	t.Cleanup(restoreHooks)
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assert.Contains(t, rollbackOut, "History was restored to the pre-restore state.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Empty(t, cfg.PathSources)
	assert.Equal(t, originalBackupCount, restoreBackupCount(t, repoRoot))
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestRecoveryRollbackResolvesAfterBackupRestoreAppliedButPlanWriteFailed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			return errors.New("injected metadata confirmation failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())

	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 1 {
			return errors.New("injected rollback recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})
	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
}

func TestWholeRecoveryResumeCompletesRestoreAndResolvesPlan(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	beforePins := documentedPinCount(t, repoRoot)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(*worktree.Manager, string, model.SnapshotID) error {
			return errors.New("injected update metadata failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())
	restoreHooks()

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Empty(t, cfg.PathSources)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
}

func TestPathRecoveryResumeResolvesAfterSuccessfulRestorePlanWriteFailure(t *testing.T) {
	repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID, "--path", "app.txt")
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	writes := 0
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected path post-mutation recovery plan write failure")
		}
		return fsutil.AtomicWrite(path, data, perm)
	})

	stdout, err := executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreWrite()
	require.Error(t, err)
	require.Empty(t, stdout)
	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	recoveryPlanID := plans[0].PlanID
	assert.Equal(t, recovery.StatusActive, plans[0].Status)

	app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(app))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
	entry, ok, err := cfg.PathSources.SourceForPath("app.txt")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.SnapshotID(sourceID), entry.SourceSnapshotID)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored path: app.txt")
	assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestPathRecoveryRollbackIsScopedAndResumeRecordsPathSource(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
		recoveryPlanID := createPathRecoveryFailure(t, sourceID)

		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside changed after failure"), 0644))
		rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
		require.NoError(t, err)
		assert.Contains(t, rollbackOut, "Recovery rollback completed.")
		assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

		app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
		require.NoError(t, err)
		assert.Equal(t, "v2", string(app))
		outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
		require.NoError(t, err)
		assert.Equal(t, "outside changed after failure", string(outside))
		cfg, err := worktree.NewManager(repoRoot).Get("main")
		require.NoError(t, err)
		assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
		assert.Equal(t, model.SnapshotID(latestID), cfg.LatestSnapshotID)
		assert.Empty(t, cfg.PathSources)
		assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	})

	t.Run("resume", func(t *testing.T) {
		repoRoot, sourceID, latestID := setupPathRecoveryRepo(t)
		recoveryPlanID := createPathRecoveryFailure(t, sourceID)

		resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", recoveryPlanID)
		require.NoError(t, err)
		assert.Contains(t, resumeOut, "Recovery resume completed.")
		assert.Contains(t, resumeOut, "Restored path: app.txt")
		assert.Contains(t, resumeOut, "From save point: "+sourceID)
		assertRecoveryOutputOmitsInternalVocabulary(t, resumeOut)

		app, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
		require.NoError(t, err)
		assert.Equal(t, "v1", string(app))
		outside, err := os.ReadFile(filepath.Join(repoRoot, "outside.txt"))
		require.NoError(t, err)
		assert.Equal(t, "outside v2", string(outside))
		cfg, err := worktree.NewManager(repoRoot).Get("main")
		require.NoError(t, err)
		assert.Equal(t, model.SnapshotID(latestID), cfg.HeadSnapshotID)
		assert.Equal(t, model.SnapshotID(latestID), cfg.LatestSnapshotID)
		entry, ok, err := cfg.PathSources.SourceForPath("app.txt")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, model.SnapshotID(sourceID), entry.SourceSnapshotID)
		assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	})
}

func TestRecoveryRollbackCompletesAfterBackupPayloadRestoredButMetadataWriteFailed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID)
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)

	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		UpdateHead: func(wtMgr *worktree.Manager, worktreeName string, snapshotID model.SnapshotID) error {
			if err := wtMgr.UpdateHead(worktreeName, snapshotID); err != nil {
				return err
			}
			return errors.New("injected metadata confirmation failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	recoveryPlanID := recoveryPlanIDFromText(t, err.Error())

	restoreMetadata := recovery.SetWriteWorktreeConfigHookForTest(func(repoRoot, name string, cfg *model.WorktreeConfig) error {
		return errors.New("injected recovery metadata write failure")
	})
	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	restoreMetadata()
	require.Error(t, err)
	require.Empty(t, stdout)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", recoveryPlanID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)
	plan, err := recovery.NewManager(repoRoot).Load(recoveryPlanID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
	assert.Equal(t, 0, restoreBackupCount(t, repoRoot))
	cfg, err = worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
}

func TestRecoveryRollbackWithEvidenceAndMissingBackupResolvesWithoutMutation(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	rollbackOut, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.NoError(t, err)
	assert.Contains(t, rollbackOut, "Recovery rollback completed.")
	assert.Contains(t, rollbackOut, "No recovery backup was present.")
	assert.NotContains(t, rollbackOut, "Recovery backup removed.")
	assertRecoveryOutputOmitsInternalVocabulary(t, rollbackOut)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(originalID), cfg.HeadSnapshotID)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	plan, err := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, plan.Status)
}

func TestRecoveryResumeWithEvidenceAndMissingBackupRetriesRestore(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(content))
	cfg, err := worktree.NewManager(repoRoot).Get("main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
}

func TestRecoveryMissingBackupWithChangedWorkspaceMetadataFailsClosed(t *testing.T) {
	repoRoot, sourceID, originalID := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	cfg, err := repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = model.SnapshotID(sourceID)
	require.NoError(t, repo.WriteWorktreeConfig(repoRoot, "main", cfg))
	beforePins := documentedPinCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "current workspace state")
	assert.Contains(t, err.Error(), "no files were changed")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	stdout, err = executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "current workspace state")
	assert.Contains(t, err.Error(), "no files were changed")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr = recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := filepath.Join(repoRoot, "main")
	content, err := os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "source.txt"))
	cfg, err = repo.LoadWorktreeConfig(repoRoot, "main")
	require.NoError(t, err)
	assert.Equal(t, model.SnapshotID(sourceID), cfg.HeadSnapshotID)
	assert.Equal(t, model.SnapshotID(originalID), cfg.LatestSnapshotID)
}

func TestRecoveryMissingRequiredBackupFailsClosedAndPlanStaysActive(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createRequiredMissingBackupPlanAfterPayloadMutation(t, repoRoot, sourceID)
	beforePins := documentedPinCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "rollback", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot be completed safely")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))

	mainPath := filepath.Join(repoRoot, "main")
	content, readErr := os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))

	stdout, err = executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "cannot return to the saved recovery point safely")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	plan, loadErr = recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.Equal(t, beforePins, documentedPinCount(t, repoRoot))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "source.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "source", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "original.txt"))
}

func TestRecoveryResumeNonIncompleteFailureKeepsPlanActiveAndResumable(t *testing.T) {
	repoRoot, sourceID, _ := setupWholeRecoveryRepo(t)
	planID := createCrashRecoveryPlanWithMissingBackup(t, repoRoot, sourceID)
	auditPath := filepath.Join(repoRoot, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.Remove(auditPath))
	require.NoError(t, os.Mkdir(auditPath, 0755))

	stdout, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.Error(t, err)
	require.Empty(t, stdout)
	plan, loadErr := recovery.NewManager(repoRoot).Load(planID)
	require.NoError(t, loadErr)
	assert.Equal(t, recovery.StatusActive, plan.Status)
	assert.NotEmpty(t, plan.RecoveryEvidence)
	assert.NotEmpty(t, plan.LastError)

	require.NoError(t, os.RemoveAll(auditPath))
	resumeOut, err := executeCommand(createTestRootCmd(), "recovery", "resume", planID)
	require.NoError(t, err)
	assert.Contains(t, resumeOut, "Recovery resume completed.")
	assert.Contains(t, resumeOut, "Restored save point: "+sourceID)
}

func setupWholeRecoveryRepo(t *testing.T) (repoRoot, sourceID, originalID string) {
	t.Helper()
	repoRoot = t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	_, err = repo.Init(repoRoot, "test")
	require.NoError(t, err)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.Chdir(mainPath))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "original.txt"), []byte("original"), 0644))
	originalID = savePointIDFromCLI(t, "original")

	mgr := worktree.NewManager(repoRoot)
	_, err = mgr.Create("source", nil)
	require.NoError(t, err)
	sourcePath, err := mgr.Path("source")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePath, "source.txt"), []byte("source"), 0644))
	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint("source", "source", nil)
	require.NoError(t, err)
	require.NoError(t, mgr.Remove("source"))
	require.NoError(t, os.Chdir(mainPath))
	return repoRoot, string(desc.SnapshotID), originalID
}

func createCrashRecoveryPlanWithMissingBackup(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: folder + ".restore-backup-missing", Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery status " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func createRequiredMissingBackupPlanAfterPayloadMutation(t *testing.T, repoRoot, sourceID string) string {
	t.Helper()
	r, err := repo.Discover(repoRoot)
	require.NoError(t, err)
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get("main")
	require.NoError(t, err)
	folder, err := mgr.Path("main")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(folder, "original.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "source.txt"), []byte("source"), 0644))
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	planID := "RP-" + string(model.NewSnapshotID())
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:          recovery.SchemaVersion,
		RepoID:                 r.RepoID,
		PlanID:                 planID,
		Status:                 recovery.StatusActive,
		Operation:              recovery.OperationRestore,
		RestorePlanID:          "restore-preview",
		Workspace:              "main",
		Folder:                 folder,
		SourceSavePoint:        model.SnapshotID(sourceID),
		CreatedAt:              now,
		UpdatedAt:              now,
		PreWorktreeState:       recovery.WorktreeState{Name: cfg.Name, RealPath: cfg.RealPath, BaseSnapshotID: cfg.BaseSnapshotID, HeadSnapshotID: cfg.HeadSnapshotID, LatestSnapshotID: cfg.LatestSnapshotID, PathSources: cfg.PathSources.Clone(), CreatedAt: cfg.CreatedAt},
		Backup:                 recovery.Backup{Path: folder + ".restore-backup-missing", Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
		RecoveryEvidence:       evidence,
		RecommendedNextCommand: "jvs recovery status " + planID,
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))
	return planID
}

func setupPathRecoveryRepo(t *testing.T) (repoRoot, sourceID, latestID string) {
	t.Helper()
	repoRoot = setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v1"), 0644))
	sourceID = savePointIDFromCLI(t, "source")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "outside.txt"), []byte("outside v2"), 0644))
	latestID = savePointIDFromCLI(t, "latest")
	return repoRoot, sourceID, latestID
}

func createPathRecoveryFailure(t *testing.T, sourceID string) string {
	t.Helper()
	previewOut, err := executeCommand(createTestRootCmd(), "restore", sourceID, "--path", "app.txt")
	require.NoError(t, err)
	restorePlanID := restorePlanIDFromHumanOutput(t, previewOut)
	restoreHooks := restore.SetHooksForTest(restore.Hooks{
		RecordPathSource: func(string, string, string, model.SnapshotID) error {
			return errors.New("injected record path source failure")
		},
	})
	_, err = executeCommand(createTestRootCmd(), "restore", "--run", restorePlanID)
	restoreHooks()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Recovery plan:")
	assertRecoveryOutputOmitsInternalVocabulary(t, err.Error())
	return recoveryPlanIDFromText(t, err.Error())
}

func recoveryPlanIDFromText(t *testing.T, value string) string {
	t.Helper()
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(line, "Recovery plan: ") {
			planID := strings.TrimSpace(strings.TrimPrefix(line, "Recovery plan: "))
			require.NotEmpty(t, planID)
			return planID
		}
	}
	t.Fatalf("recovery plan ID not found in:\n%s", value)
	return ""
}

func restoreBackupCount(t *testing.T, repoRoot string) int {
	t.Helper()
	matches, err := filepath.Glob(repoRoot + ".restore-backup-*")
	require.NoError(t, err)
	nested, err := filepath.Glob(filepath.Join(repoRoot, "*.restore-backup-*"))
	require.NoError(t, err)
	return len(matches) + len(nested)
}

func assertRecoveryOutputOmitsInternalVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{"checkpoint", "snapshot", "worktree", "pin", "gc", "internal"} {
		assert.False(t, regexp.MustCompile(`\b`+regexp.QuoteMeta(word)+`\b`).MatchString(lower), "output should not expose %q:\n%s", word, value)
	}
}
