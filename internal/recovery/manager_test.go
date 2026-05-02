package recovery_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/sourcepin"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerWriteLoadListAndResolveReleasesProtection(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	sourceID := model.NewSnapshotID()
	pin, err := sourcepin.NewManager(repoRoot).CreateWithID(sourceID, "recovery-RP-test", "active recovery plan RP-test")
	require.NoError(t, err)

	plan := recovery.Plan{
		SchemaVersion:           recovery.SchemaVersion,
		RepoID:                  repoID,
		PlanID:                  "RP-test",
		Status:                  recovery.StatusActive,
		Operation:               recovery.OperationRestore,
		RestorePlanID:           "restore-preview",
		Workspace:               "main",
		Folder:                  filepath.Join(repoRoot, "main"),
		SourceSavePoint:         sourceID,
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
		PreWorktreeState:        recovery.WorktreeState{Name: "main", HeadSnapshotID: sourceID},
		Backup:                  recovery.Backup{Path: filepath.Join(repoRoot, "main.restore-backup-test"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		CompletedSteps:          []string{"recovery plan created"},
		PendingSteps:            []string{"resume restore or rollback"},
		RecommendedNextCommand:  "jvs recovery status RP-test",
		CleanupProtectionPinIDs: []string{pin.Pin.PinID},
		CleanupProtectionPins:   []model.Pin{pin.Pin},
	}

	mgr := recovery.NewManager(repoRoot)
	require.NoError(t, mgr.Write(&plan))

	loaded, err := mgr.Load("RP-test")
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusActive, loaded.Status)
	assert.Equal(t, sourceID, loaded.SourceSavePoint)
	assert.Equal(t, []string{pin.Pin.PinID}, loaded.CleanupProtectionPinIDs)

	plans, err := mgr.List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "RP-test", plans[0].PlanID)

	require.NoError(t, mgr.MarkResolved("RP-test"))
	resolved, err := mgr.Load("RP-test")
	require.NoError(t, err)
	assert.Equal(t, recovery.StatusResolved, resolved.Status)
	assert.NotNil(t, resolved.ResolvedAt)
	assert.NoFileExists(t, filepath.Join(repoRoot, ".jvs", "gc", "pins", pin.Pin.PinID+".json"))
}

func TestManagerLoadRejectsRepoMismatchUnsafeIDAndUnsafeLeaf(t *testing.T) {
	repoRoot, _ := setupRecoveryManagerRepo(t)
	mgr := recovery.NewManager(repoRoot)

	require.NoError(t, writeRawRecoveryPlan(t, repoRoot, "RP-other", map[string]any{
		"schema_version":    recovery.SchemaVersion,
		"repo_id":           "different-repo",
		"plan_id":           "RP-other",
		"status":            recovery.StatusActive,
		"operation":         recovery.OperationRestore,
		"workspace":         "main",
		"folder":            filepath.Join(repoRoot, "main"),
		"source_save_point": model.NewSnapshotID(),
		"backup": map[string]any{
			"path":  filepath.Join(repoRoot, "main.restore-backup-test"),
			"scope": recovery.BackupScopeWhole,
		},
	}))
	_, err := mgr.Load("../bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recovery plan")

	_, err = mgr.Load("RP-other")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different repository")

	plansDir := filepath.Join(repoRoot, ".jvs", "recovery-plans")
	require.NoError(t, os.MkdirAll(plansDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(plansDir, "RP-dir.json"), 0755))
	_, err = mgr.Load("RP-dir")
	require.Error(t, err)

	linkPath := filepath.Join(plansDir, "RP-link.json")
	if err := os.Symlink(filepath.Join(repoRoot, ".jvs", "repo_id"), linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	_, err = mgr.Load("RP-link")
	require.Error(t, err)
}

func TestCreateActiveForRestoreRecordsRecoveryEvidence(t *testing.T) {
	repoRoot, _ := setupRecoveryManagerRepo(t)
	sourceID := model.NewSnapshotID()
	evidence, err := restoreplan.WorkspaceEvidence(repoRoot, "main")
	require.NoError(t, err)
	preview := &restoreplan.Plan{
		PlanID:                 "restore-preview",
		Workspace:              "main",
		Folder:                 filepath.Join(repoRoot, "main"),
		SourceSavePoint:        sourceID,
		ExpectedFolderEvidence: evidence,
	}

	plan, err := recovery.NewManager(repoRoot).CreateActiveForRestore(preview, filepath.Join(repoRoot, "main.restore-backup-test"))
	require.NoError(t, err)
	assert.NotEmpty(t, plan.RecoveryEvidence)
	assert.Equal(t, evidence, plan.RecoveryEvidence)
	assert.Equal(t, recovery.BackupStatePending, plan.Backup.State)
	require.NoError(t, recovery.NewManager(repoRoot).MarkResolved(plan.PlanID))
}

func TestCreateActiveForRestoreDefiniteWriteFailureReleasesHiddenSourceProtection(t *testing.T) {
	repoRoot, _ := setupRecoveryManagerRepo(t)
	sourceID := model.NewSnapshotID()
	preview := &restoreplan.Plan{
		PlanID:          "restore-preview",
		Workspace:       "main",
		Folder:          filepath.Join(repoRoot, "main"),
		SourceSavePoint: sourceID,
	}
	restoreWrite := recovery.SetWriteHookForTest(func(string, []byte, os.FileMode) error {
		return errors.New("injected write failure")
	})
	defer restoreWrite()

	plan, err := recovery.NewManager(repoRoot).CreateActiveForRestore(preview, filepath.Join(repoRoot, "main.restore-backup-test"))
	require.Error(t, err)
	require.Nil(t, plan)

	protected, err := sourcepin.NewManager(repoRoot).ProtectedSnapshotIDs()
	require.NoError(t, err)
	assert.NotContains(t, protected, sourceID)
}

func TestCreateActiveForRestoreCommitUncertainVisiblePlanReturnsErrorWithoutHiddenProtection(t *testing.T) {
	repoRoot, _ := setupRecoveryManagerRepo(t)
	sourceID := model.NewSnapshotID()
	preview := &restoreplan.Plan{
		PlanID:          "restore-preview",
		Workspace:       "main",
		Folder:          filepath.Join(repoRoot, "main"),
		SourceSavePoint: sourceID,
	}
	restoreWrite := recovery.SetWriteHookForTest(func(path string, data []byte, perm os.FileMode) error {
		require.NoError(t, os.WriteFile(path, data, perm))
		return &fsutil.CommitUncertainError{Op: "atomic write", Path: path, Err: errors.New("injected post-commit failure")}
	})
	defer restoreWrite()

	plan, err := recovery.NewManager(repoRoot).CreateActiveForRestore(preview, filepath.Join(repoRoot, "main.restore-backup-test"))
	require.Error(t, err)
	require.Nil(t, plan)
	assert.Contains(t, err.Error(), "jvs recovery status")

	protected, err := sourcepin.NewManager(repoRoot).ProtectedSnapshotIDs()
	require.NoError(t, err)
	assert.NotContains(t, protected, sourceID)
	plans, err := recovery.NewManager(repoRoot).List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, sourceID, plans[0].SourceSavePoint)
}

func TestMarkResolvedRetriesProtectionReleaseForResolvedPlan(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	sourceID := model.NewSnapshotID()
	pin, err := sourcepin.NewManager(repoRoot).CreateWithID(sourceID, "recovery-RP-resolved", "active recovery plan RP-resolved")
	require.NoError(t, err)
	now := time.Now().UTC()
	plan := recovery.Plan{
		SchemaVersion:           recovery.SchemaVersion,
		RepoID:                  repoID,
		PlanID:                  "RP-resolved",
		Status:                  recovery.StatusResolved,
		Operation:               recovery.OperationRestore,
		RestorePlanID:           "restore-preview",
		Workspace:               "main",
		Folder:                  filepath.Join(repoRoot, "main"),
		SourceSavePoint:         sourceID,
		CreatedAt:               now,
		UpdatedAt:               now,
		ResolvedAt:              &now,
		PreWorktreeState:        recovery.WorktreeState{Name: "main"},
		Backup:                  recovery.Backup{Path: filepath.Join(repoRoot, "main.restore-backup-test"), Scope: recovery.BackupScopeWhole, State: recovery.BackupStatePending},
		CleanupProtectionPinIDs: []string{pin.Pin.PinID},
		CleanupProtectionPins:   []model.Pin{pin.Pin},
	}
	require.NoError(t, recovery.NewManager(repoRoot).Write(&plan))

	require.NoError(t, recovery.NewManager(repoRoot).MarkResolved("RP-resolved"))
	assert.NoFileExists(t, filepath.Join(repoRoot, ".jvs", "gc", "pins", pin.Pin.PinID+".json"))
}

func TestRestoreBackupRejectsMismatchedFolderWithoutMutatingWorkspace(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "main", "file.txt"), []byte("current"), 0644))
	backupPath := filepath.Join(repoRoot, "wrong-main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "file.txt"), []byte("backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-mismatch",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           filepath.Join(repoRoot, "wrong-main"),
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}

	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(repoRoot, "main", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current", string(content))
}

func TestRestoreBackupRejectsSiblingBackupNotGeneratedForWorkspaceBoundary(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("current"), 0644))
	backupPath := filepath.Join(repoRoot, "other.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "file.txt"), []byte("wrong backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-wrong-backup-prefix",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}

	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(mainPath, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current", string(content))
}

func TestRestoreBackupDoesNotRestoreControlDataFromBackup(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("current"), 0644))
	r, err := repo.InitAdoptedWorkspace(repoRoot)
	require.NoError(t, err)
	backupPath := repoRoot + ".restore-backup-test"
	require.NoError(t, os.MkdirAll(filepath.Join(backupPath, ".jvs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "file.txt"), []byte("backup"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, ".jvs", "evil"), []byte("evil"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           r.RepoID,
		PlanID:           "RP-control",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           repoRoot,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main", RealPath: repoRoot},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}

	require.NoError(t, recovery.NewManager(repoRoot).RestoreBackup(&plan))
	content, err := os.ReadFile(filepath.Join(repoRoot, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "backup", string(content))
	assert.NoFileExists(t, filepath.Join(repoRoot, ".jvs", "evil"))
}

func TestRestorePathBackupRequiredEntryMissingFailsClosedWithoutDeletingCurrentPath(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-missing-required-backup",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "app.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:         backupPath,
			Scope:        recovery.BackupScopePath,
			State:        recovery.BackupStateRequired,
			TouchedPaths: []string{"app.txt"},
			Entries:      []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}},
		},
	}

	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(mainPath, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "restored current", string(content))
}

func TestRestorePathPlanRejectsWholeBackupScopeWithoutMutatingWorkspace(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("current app"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "other.txt"), []byte("current other"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("backup app"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-whole-scope",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "app.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}

	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(mainPath, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current app", string(content))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "other.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current other", string(content))
}

func TestRestoreWholePlanRejectsPathBackupScopeWithoutMutatingWorkspace(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("current app"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("backup app"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-whole-path-scope",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:    backupPath,
			Scope:   recovery.BackupScopePath,
			State:   recovery.BackupStateRequired,
			Entries: []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}},
		},
	}

	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(mainPath, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current app", string(content))
}

func TestRestorePathBackupRejectsTamperedEntriesBeforeMutation(t *testing.T) {
	t.Run("extra path", func(t *testing.T) {
		repoRoot, repoID := setupRecoveryManagerRepo(t)
		mainPath := filepath.Join(repoRoot, "main")
		require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("current app"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(mainPath, "other.txt"), []byte("current other"), 0644))
		backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
		require.NoError(t, os.MkdirAll(backupPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("backup app"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(backupPath, "other.txt"), []byte("backup other"), 0644))
		plan := recovery.Plan{
			SchemaVersion:    recovery.SchemaVersion,
			RepoID:           repoID,
			PlanID:           "RP-path-extra-entry",
			Status:           recovery.StatusActive,
			Operation:        recovery.OperationRestorePath,
			RestorePlanID:    "restore-preview",
			Workspace:        "main",
			Folder:           mainPath,
			SourceSavePoint:  model.NewSnapshotID(),
			Path:             "app.txt",
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
			PreWorktreeState: recovery.WorktreeState{Name: "main"},
			Backup: recovery.Backup{
				Path:    backupPath,
				Scope:   recovery.BackupScopePath,
				State:   recovery.BackupStateRequired,
				Entries: []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}, {Path: "other.txt", HadOriginal: true}},
			},
		}

		err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
		require.Error(t, err)
		content, readErr := os.ReadFile(filepath.Join(mainPath, "app.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, "current app", string(content))
		content, readErr = os.ReadFile(filepath.Join(mainPath, "other.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, "current other", string(content))
	})

	t.Run("control data", func(t *testing.T) {
		repoRoot := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("current app"), 0644))
		r, err := repo.InitAdoptedWorkspace(repoRoot)
		require.NoError(t, err)
		backupPath := repoRoot + ".restore-backup-test"
		require.NoError(t, os.MkdirAll(filepath.Join(backupPath, ".jvs"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(backupPath, ".jvs", "format_version"), []byte("backup control"), 0644))
		plan := recovery.Plan{
			SchemaVersion:    recovery.SchemaVersion,
			RepoID:           r.RepoID,
			PlanID:           "RP-path-control-entry",
			Status:           recovery.StatusActive,
			Operation:        recovery.OperationRestorePath,
			RestorePlanID:    "restore-preview",
			Workspace:        "main",
			Folder:           repoRoot,
			SourceSavePoint:  model.NewSnapshotID(),
			Path:             "app.txt",
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
			PreWorktreeState: recovery.WorktreeState{Name: "main", RealPath: repoRoot},
			Backup: recovery.Backup{
				Path:    backupPath,
				Scope:   recovery.BackupScopePath,
				State:   recovery.BackupStateRequired,
				Entries: []recovery.BackupEntry{{Path: ".jvs/format_version", HadOriginal: true}},
			},
		}

		err = recovery.NewManager(repoRoot).RestoreBackup(&plan)
		require.Error(t, err)
		content, readErr := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, "current app", string(content))
		control, readErr := os.ReadFile(filepath.Join(repoRoot, ".jvs", "format_version"))
		require.NoError(t, readErr)
		assert.Equal(t, "1\n", string(control))
	})
}

func TestRestorePathBackupCopyFailureKeepsBackupReusable(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("original backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-copy-failure",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "app.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:         backupPath,
			Scope:        recovery.BackupScopePath,
			State:        recovery.BackupStateRequired,
			TouchedPaths: []string{"app.txt"},
			Entries:      []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}},
		},
	}
	restoreClone := recovery.SetRestoreBackupCloneHookForTest(func(src, dst string) error {
		return errors.New("injected backup copy failure")
	})
	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	restoreClone()
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(backupPath, "app.txt"))
	content, readErr := os.ReadFile(filepath.Join(mainPath, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "restored current", string(content))

	require.NoError(t, recovery.NewManager(repoRoot).RestoreBackup(&plan))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "app.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "original backup", string(content))
	assert.FileExists(t, filepath.Join(backupPath, "app.txt"))
}

func TestRestoreWholeBackupCopyFailureKeepsBackupReusable(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "current.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "original.txt"), []byte("original backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-whole-copy-failure",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}
	restoreClone := recovery.SetRestoreBackupCloneHookForTest(func(src, dst string) error {
		return errors.New("injected backup copy failure")
	})
	err := recovery.NewManager(repoRoot).RestoreBackup(&plan)
	restoreClone()
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(backupPath, "original.txt"))
	content, readErr := os.ReadFile(filepath.Join(mainPath, "current.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "restored current", string(content))

	require.NoError(t, recovery.NewManager(repoRoot).RestoreBackup(&plan))
	content, readErr = os.ReadFile(filepath.Join(mainPath, "original.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "original backup", string(content))
	assert.NoFileExists(t, filepath.Join(mainPath, "current.txt"))
	assert.FileExists(t, filepath.Join(backupPath, "original.txt"))
}

func TestRestoreWholeBackupCapacityFailurePreventsCopyAndPreservesPayloads(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "current.txt"), []byte("current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(filepath.Join(backupPath, ".jvs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "original.txt"), []byte("12345"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, ".jvs", "control"), []byte("0123456789"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-whole-capacity",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}
	restoreCapacity := recovery.SetRestoreBackupCapacityGateForTest(capacitygate.Gate{Meter: &recoveryStaticMeter{available: 5}, SafetyMarginBytes: 0})
	defer restoreCapacity()
	cloneCalled := false
	restoreClone := recovery.SetRestoreBackupCloneHookForTest(func(src, dst string) error {
		cloneCalled = true
		return errors.New("clone should not run")
	})
	defer restoreClone()

	mgr := recovery.NewManager(repoRoot)
	err := mgr.RestoreBackup(&plan)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Not enough free space")
	assert.Contains(t, err.Error(), "Required bytes: 15")
	assert.False(t, cloneCalled)
	assert.Empty(t, mgr.LastTransferRecords())
	content, readErr := os.ReadFile(filepath.Join(mainPath, "current.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "current", string(content))
	backupContent, readErr := os.ReadFile(filepath.Join(backupPath, "original.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "12345", string(backupContent))
}

func TestRestoreWholeBackupRecordsFinalTransferForRecoveryCopyPoint(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "current.txt"), []byte("current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "original.txt"), []byte("original backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-whole-transfer",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestore,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup:           recovery.Backup{Path: backupPath, Scope: recovery.BackupScopeWhole, State: recovery.BackupStateRequired},
	}
	planner := &recoveryRecordingPlanner{}
	restorePlanner := recovery.SetRestoreBackupTransferPlannerForTest(planner)
	defer restorePlanner()

	mgr := recovery.NewManager(repoRoot)
	require.NoError(t, mgr.RestoreBackup(&plan))

	records := mgr.LastTransferRecords()
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, "recovery-backup-restore-primary", record.TransferID)
	assert.Equal(t, "recovery_backup_restore", record.Operation)
	assert.Equal(t, "backup_restore", record.Phase)
	assert.True(t, record.Primary)
	assert.Equal(t, transfer.ResultKindFinal, record.ResultKind)
	assert.Equal(t, transfer.PermissionScopeExecution, record.PermissionScope)
	assert.Equal(t, "recovery_backup_payload", record.SourceRole)
	assert.Equal(t, backupPath, record.SourcePath)
	assert.Equal(t, "recovery_restore_staging", record.DestinationRole)
	assert.True(t, strings.HasPrefix(record.MaterializationDestination, mainPath+".recovery-restore-tmp-"), record.MaterializationDestination)
	assert.Equal(t, filepath.Dir(record.MaterializationDestination), record.CapabilityProbePath)
	assert.Equal(t, mainPath, record.PublishedDestination)
	assert.True(t, record.CheckedForThisOperation)
	assert.Equal(t, model.EngineCopy, record.EffectiveEngine)
	assert.False(t, record.OptimizedTransfer)
	assert.Equal(t, transfer.PerformanceClassNormalCopy, record.PerformanceClass)
	require.Len(t, planner.requests, 1)
	assert.Equal(t, backupPath, planner.requests[0].SourcePath)
	assert.Equal(t, record.MaterializationDestination, planner.requests[0].DestinationPath)
	assert.Equal(t, record.CapabilityProbePath, planner.requests[0].CapabilityPath)
	assert.Equal(t, engine.EngineAuto, planner.requests[0].RequestedEngine)
}

func TestRestorePathBackupAbsentOriginalRemovesRestoredPath(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "created.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-absent-original",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "created.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:         backupPath,
			Scope:        recovery.BackupScopePath,
			State:        recovery.BackupStateRequired,
			TouchedPaths: []string{"created.txt"},
			Entries:      []recovery.BackupEntry{{Path: "created.txt", HadOriginal: false}},
		},
	}

	require.NoError(t, recovery.NewManager(repoRoot).RestoreBackup(&plan))
	assert.NoFileExists(t, filepath.Join(mainPath, "created.txt"))
}

func TestRestorePathBackupAbsentOriginalDoesNotGateCopyOrRecordTransfer(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "created.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-no-copy-transfer",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "created.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:         backupPath,
			Scope:        recovery.BackupScopePath,
			State:        recovery.BackupStateRequired,
			TouchedPaths: []string{"created.txt"},
			Entries:      []recovery.BackupEntry{{Path: "created.txt", HadOriginal: false}},
		},
	}
	restoreCapacity := recovery.SetRestoreBackupCapacityGateForTest(capacitygate.Gate{Meter: &recoveryStaticMeter{available: 0}, SafetyMarginBytes: 0})
	defer restoreCapacity()
	cloneCalled := false
	restoreClone := recovery.SetRestoreBackupCloneHookForTest(func(src, dst string) error {
		cloneCalled = true
		return errors.New("clone should not run")
	})
	defer restoreClone()

	mgr := recovery.NewManager(repoRoot)
	require.NoError(t, mgr.RestoreBackup(&plan))

	assert.False(t, cloneCalled)
	assert.Empty(t, mgr.LastTransferRecords())
	assert.NoFileExists(t, filepath.Join(mainPath, "created.txt"))
}

func TestRestorePathBackupOriginalRecordsTransferOnlyForCopiedEntry(t *testing.T) {
	repoRoot, repoID := setupRecoveryManagerRepo(t)
	mainPath := filepath.Join(repoRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("restored current"), 0644))
	backupPath := filepath.Join(repoRoot, "main.restore-backup-test")
	require.NoError(t, os.MkdirAll(backupPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(backupPath, "app.txt"), []byte("original backup"), 0644))
	plan := recovery.Plan{
		SchemaVersion:    recovery.SchemaVersion,
		RepoID:           repoID,
		PlanID:           "RP-path-transfer",
		Status:           recovery.StatusActive,
		Operation:        recovery.OperationRestorePath,
		RestorePlanID:    "restore-preview",
		Workspace:        "main",
		Folder:           mainPath,
		SourceSavePoint:  model.NewSnapshotID(),
		Path:             "app.txt",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PreWorktreeState: recovery.WorktreeState{Name: "main"},
		Backup: recovery.Backup{
			Path:         backupPath,
			Scope:        recovery.BackupScopePath,
			State:        recovery.BackupStateRequired,
			TouchedPaths: []string{"app.txt"},
			Entries:      []recovery.BackupEntry{{Path: "app.txt", HadOriginal: true}},
		},
	}
	planner := &recoveryRecordingPlanner{}
	restorePlanner := recovery.SetRestoreBackupTransferPlannerForTest(planner)
	defer restorePlanner()

	mgr := recovery.NewManager(repoRoot)
	require.NoError(t, mgr.RestoreBackup(&plan))

	records := mgr.LastTransferRecords()
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, "recovery-path-backup-restore-primary", record.TransferID)
	assert.Equal(t, filepath.Join(backupPath, "app.txt"), record.SourcePath)
	assert.True(t, strings.HasPrefix(record.MaterializationDestination, mainPath+".recovery-path-tmp-"), record.MaterializationDestination)
	assert.True(t, strings.HasSuffix(record.MaterializationDestination, filepath.Join("", "app.txt")), record.MaterializationDestination)
	assert.Equal(t, filepath.Dir(filepath.Dir(record.MaterializationDestination)), record.CapabilityProbePath)
	assert.Equal(t, filepath.Join(mainPath, "app.txt"), record.PublishedDestination)
	assert.Equal(t, "original backup", string(requireReadFile(t, filepath.Join(mainPath, "app.txt"))))
	require.Len(t, planner.requests, 1)
	assert.Equal(t, filepath.Join(backupPath, "app.txt"), planner.requests[0].SourcePath)
	assert.Equal(t, record.MaterializationDestination, planner.requests[0].DestinationPath)
}

func setupRecoveryManagerRepo(t *testing.T) (repoRoot string, repoID string) {
	t.Helper()
	repoRoot = t.TempDir()
	r, err := repo.Init(repoRoot, "test")
	require.NoError(t, err)
	return repoRoot, r.RepoID
}

type recoveryStaticMeter struct {
	available int64
}

func (m *recoveryStaticMeter) AvailableBytes(string) (int64, error) {
	return m.available, nil
}

type recoveryRecordingPlanner struct {
	requests []engine.TransferPlanRequest
}

func (p *recoveryRecordingPlanner) PlanTransfer(req engine.TransferPlanRequest) (*engine.TransferPlan, error) {
	p.requests = append(p.requests, req)
	requested := req.RequestedEngine
	if requested == "" {
		requested = engine.EngineAuto
	}
	return &engine.TransferPlan{
		RequestedEngine:   requested,
		TransferEngine:    model.EngineCopy,
		EffectiveEngine:   model.EngineCopy,
		OptimizedTransfer: false,
	}, nil
}

func requireReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func writeRawRecoveryPlan(t *testing.T, repoRoot, planID string, value map[string]any) error {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".jvs", "recovery-plans"), 0755))
	return os.WriteFile(filepath.Join(repoRoot, ".jvs", "recovery-plans", planID+".json"), data, 0644)
}
