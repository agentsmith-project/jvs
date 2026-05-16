package afscp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectRestoreFastPathClonesSnapshotPayloadToHomeRestoreTmp(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	cloneLog := installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	require.NoError(t, os.WriteFile(cloneLog, nil, 0644))

	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("dirty"), 0644))
	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	require.NoError(t, err)
	restore := result.(RestoreResult)
	assert.Equal(t, save.SavePointID, restore.RestoredSavePointID)
	assert.Equal(t, save.SavePointID, restore.NewHead)
	require.Len(t, restore.CloneEvidence, 1)
	assertDirectCloneEvidence(t, restore.CloneEvidence[0], "restore", "restore_staging")

	cloneArgs := readFakeJuiceFSCloneArgs(t, cloneLog)
	require.Len(t, cloneArgs, 3)
	assert.Equal(t, "clone", cloneArgs[0])
	assert.Equal(t, filepath.Join(directTestSnapshotDir(controlRoot, save.SavePointID), "payload"), cloneArgs[1])
	assert.Equal(t, filepath.Dir(home), filepath.Dir(cloneArgs[2]))
	assert.True(t, strings.HasPrefix(filepath.Base(cloneArgs[2]), directRestoreTmpPrefix), cloneArgs[2])
	assert.Empty(t, directTestRestoreTmpNames(t, home))
	assert.Equal(t, "saved", string(directTestReadFile(t, filepath.Join(home, "profile.txt"))))
}

func TestDirectRestoreReplacesHomeRootWithDirectoryLevelPublish(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "saved.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	beforeRoot := directTestStat(t, home)

	require.NoError(t, os.Remove(filepath.Join(home, "saved.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(home, "post-save.txt"), []byte("remove me"), 0644))

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	require.NoError(t, err)
	restore := result.(RestoreResult)
	assert.Equal(t, save.SavePointID, restore.NewHead)
	afterRoot := directTestStat(t, home)
	assert.False(t, os.SameFile(beforeRoot, afterRoot), "restore must publish the cloned directory as the HOME root")
	assert.FileExists(t, filepath.Join(home, "saved.txt"))
	assert.NoFileExists(t, filepath.Join(home, "post-save.txt"))

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.NotNil(t, list.HistoryHead)
	assert.Equal(t, save.SavePointID, *list.HistoryHead)
	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.NotNil(t, status.HistoryHead)
	assert.Equal(t, save.SavePointID, *status.HistoryHead)
}

func TestDirectRestoreSuccessLeavesOldHomeBackupForOutOfBandCleanup(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "old-dir", "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "old-dir", "nested", "old.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")

	require.NoError(t, os.RemoveAll(filepath.Join(home, "old-dir")))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "dirty-dir", "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "dirty-dir", "nested", "dirty.txt"), []byte("keep as backup"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "post-save.txt"), []byte("backup me"), 0644))

	_, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	require.NoError(t, err)
	assert.Empty(t, directTestRestoreTmpNames(t, home))
	assert.NoFileExists(t, filepath.Join(home, "post-save.txt"))
	assert.NoDirExists(t, filepath.Join(home, "dirty-dir"))

	cleanupPayloads := directTestPendingCleanupPayloads(t, controlRoot)
	assert.Empty(t, cleanupPayloads, "restore success must not move old HOME entries into control-root payload cleanup")
	require.Len(t, directTestPendingCleanupMarkers(t, controlRoot), 1)
	backupNames := directTestRestoreSiblingBackupNames(t, home)
	require.Len(t, backupNames, 1)
	backupHome := filepath.Join(filepath.Dir(home), backupNames[0])
	assert.FileExists(t, filepath.Join(backupHome, "dirty-dir", "nested", "dirty.txt"))
	assert.FileExists(t, filepath.Join(backupHome, "post-save.txt"))

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, "ready", status.MetadataState)
	assert.Equal(t, "cleanup_pending", status.Recovery)

	doctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.False(t, doctor.Healthy)
	assert.Equal(t, "ready", doctor.MetadataState)
	assertDirectDoctorFindingContains(t, doctor, "cleanup pending")
}

func TestDirectSaveAfterRestoreDoesNotCapturePendingCleanupBackup(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "saved.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	restoreSource := directTestSave(t, controlRoot, home, "baseline")

	require.NoError(t, os.Remove(filepath.Join(home, "saved.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(home, "dirty-only.txt"), []byte("must stay in cleanup only"), 0644))
	_, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: restoreSource.SavePointID,
	})
	require.NoError(t, err)
	assert.Empty(t, directTestPendingCleanupPayloads(t, controlRoot))
	require.Len(t, directTestPendingCleanupMarkers(t, controlRoot), 1)

	nextSave := directTestSave(t, controlRoot, home, "after restore")
	nextPayload := filepath.Join(directTestSnapshotDir(controlRoot, nextSave.SavePointID), "payload")
	assert.FileExists(t, filepath.Join(nextPayload, "saved.txt"))
	assert.NoFileExists(t, filepath.Join(nextPayload, "dirty-only.txt"))
	assert.Empty(t, directTestRestoreTmpNames(t, nextPayload))
}

func TestDirectRestoreRestoresDotDirsSymlinksEmptyDirsAndRemovesPostSaveContent(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	externalWorkspace := filepath.Join(base, "workspace-target")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cache", "agent"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "empty-dir"), 0755))
	require.NoError(t, os.Mkdir(externalWorkspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cache", "agent", "runtime.json"), []byte(`{"saved":true}`), 0644))
	require.NoError(t, os.Symlink(externalWorkspace, filepath.Join(home, "workspace")))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "dot dirs")
	snapshotPayload := filepath.Join(directTestSnapshotDir(controlRoot, save.SavePointID), "payload")

	require.NoError(t, os.RemoveAll(filepath.Join(home, ".cache")))
	require.NoError(t, os.Remove(filepath.Join(home, "workspace")))
	require.NoError(t, os.WriteFile(filepath.Join(home, "post-save.txt"), []byte("remove me"), 0644))

	_, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	require.NoError(t, err)
	assertTreesEqual(t, snapshotPayload, home)
	assert.NoFileExists(t, filepath.Join(home, "post-save.txt"))
}

func TestDirectRestoreUnknownSavePointFailsBeforeHomeMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	directTestSave(t, controlRoot, home, "baseline")
	before := collectDirectTestTree(t, home)

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: "missing-save-point",
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeSavePointNotFound, ExitInvalidArgument)
	assert.Equal(t, before, collectDirectTestTree(t, home))
	assert.Empty(t, directTestRestoreTmpNames(t, home))
}

func TestDirectRestoreCloneFailureCleansTmpAndLeavesJournalIdle(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	installFailingJuiceFSClone(t)

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeCloneFailed, ExitStorage)
	assert.Empty(t, directTestRestoreTmpNames(t, home))

	layout, initialized, err := openDirectLayout(ResolvedSelector{ControlRoot: controlRoot, Home: home})
	require.NoError(t, err)
	require.True(t, initialized)
	journal, err := readDirectJournal(layout)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "idle", journal.Phase)
	assert.Equal(t, "saved", string(directTestReadFile(t, filepath.Join(home, "profile.txt"))))
}

func TestDirectRestoreFailsFastWhenJuiceFSUnavailableWithoutCopyFallback(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	before := collectDirectTestTree(t, home)
	t.Setenv("PATH", t.TempDir())

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeCloneUnavailable, ExitStorage)
	assert.Equal(t, before, collectDirectTestTree(t, home))
	assert.Empty(t, directTestRestoreTmpNames(t, home))
}

func TestDirectRestoreFailureAfterReplaceBoundaryReturnsRecoveryRequired(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	restoreErr := errors.New("injected replace boundary failure")
	directRestoreAfterPublishHomeHook = func() error { return restoreErr }
	t.Cleanup(func() { directRestoreAfterPublishHomeHook = nil })

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeJournalRecoveryRequired, ExitMetadata)

	layout, initialized, err := openDirectLayout(ResolvedSelector{ControlRoot: controlRoot, Home: home})
	require.NoError(t, err)
	require.True(t, initialized)
	journal, err := readDirectJournal(layout)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "restore_replacing", journal.Phase)
	assert.Equal(t, save.SavePointID, journal.TargetSavePointID)

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, "invalid", status.MetadataState)
	assert.Equal(t, "recover_journal", status.Recovery)
}

func TestDirectRestoreRollsBackWhenBackupBoundaryFailsBeforeReplace(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("dirty"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, "post-save.txt"), []byte("keep after rollback"), 0644))
	before := collectDirectTestTree(t, home)

	directRestoreAfterBackupHomeHook = func() error { return errors.New("injected pre-replace failure") }
	t.Cleanup(func() { directRestoreAfterBackupHomeHook = nil })

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeInternal, ExitInternal)
	assert.Equal(t, before, collectDirectTestTree(t, home))
	assert.Empty(t, directTestRestoreTmpNames(t, home))

	layout, initialized, err := openDirectLayout(ResolvedSelector{ControlRoot: controlRoot, Home: home})
	require.NoError(t, err)
	require.True(t, initialized)
	journal, err := readDirectJournal(layout)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "idle", journal.Phase)
}

func TestDirectRestoreBlocksDifferentSavePointWhenRecoveryRequired(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "version.txt"), []byte("one"), 0644))
	installFakeJuiceFSClone(t)
	first := directTestSave(t, controlRoot, home, "one")
	require.NoError(t, os.WriteFile(filepath.Join(home, "version.txt"), []byte("two"), 0644))
	second := directTestSave(t, controlRoot, home, "two")

	directRestoreAfterPublishHomeHook = func() error { return errors.New("injected replace boundary failure") }
	_, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: first.SavePointID,
	})
	requireDirectError(t, err, ErrorCodeJournalRecoveryRequired, ExitMetadata)
	directRestoreAfterPublishHomeHook = nil
	t.Cleanup(func() { directRestoreAfterPublishHomeHook = nil })

	result, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: second.SavePointID,
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeJournalRecoveryRequired, ExitMetadata)
}

func directTestSave(t *testing.T, controlRoot, home, message string) SaveResult {
	t.Helper()

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  message,
	})
	require.NoError(t, err)
	save, ok := result.(SaveResult)
	require.True(t, ok, "save result type: %#v", result)
	return save
}

func installFailingJuiceFSClone(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	juicefsPath := filepath.Join(binDir, "juicefs")
	script := `#!/bin/sh
set -eu
if [ "$#" -ge 3 ]; then
  /bin/mkdir -p "$3"
  printf stale > "$3/stale.txt"
fi
exit 7
`
	require.NoError(t, os.WriteFile(juicefsPath, []byte(script), 0755))
	t.Setenv("PATH", binDir)
}

func directTestRestoreTmpNames(t *testing.T, home string) []string {
	t.Helper()

	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".jvs-afscp-restore-tmp-") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func directTestRestoreSiblingBackupNames(t *testing.T, home string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Dir(home))
	require.NoError(t, err)
	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), directRestoreBackupPrefix) {
			names = append(names, entry.Name())
		}
	}
	return names
}

func directTestPendingCleanupRoot(controlRoot string) string {
	return filepath.Join(controlRoot, "afscp-direct-v1", "pending-cleanups")
}

func directTestPendingCleanupPayloads(t *testing.T, controlRoot string) []string {
	t.Helper()

	cleanupRoot := directTestPendingCleanupRoot(controlRoot)
	entries, err := os.ReadDir(cleanupRoot)
	require.NoError(t, err)
	payloads := []string{}
	for _, entry := range entries {
		payload := filepath.Join(cleanupRoot, entry.Name(), "payload")
		if entry.IsDir() && directTestPathIsDir(payload) {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

func directTestPendingCleanupMarkers(t *testing.T, controlRoot string) []string {
	t.Helper()

	cleanupRoot := directTestPendingCleanupRoot(controlRoot)
	entries, err := os.ReadDir(cleanupRoot)
	require.NoError(t, err)
	markers := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			markers = append(markers, filepath.Join(cleanupRoot, entry.Name()))
		}
	}
	return markers
}

func directTestReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func directTestPathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func directTestStat(t *testing.T, path string) os.FileInfo {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	return info
}
