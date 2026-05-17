package afscp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectSaveFastPathUsesJuiceFSCloneAndPublishesMetadataOnly(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	cloneLog := installFakeJuiceFSClone(t)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	require.NoError(t, err)
	save := result.(SaveResult)
	require.NotEmpty(t, save.SavePointID)
	assert.Equal(t, save.SavePointID, save.HistoryHead)
	assert.Equal(t, "baseline", save.Message)
	require.Len(t, save.CloneEvidence, 1)
	assertDirectCloneEvidence(t, save.CloneEvidence[0], "save", "save_point_payload")

	cloneArgs := readFakeJuiceFSCloneArgs(t, cloneLog)
	require.Len(t, cloneArgs, 3)
	assert.Equal(t, "clone", cloneArgs[0])
	assert.Equal(t, home, cloneArgs[1])
	assert.True(t, strings.HasPrefix(cloneArgs[2], filepath.Join(controlRoot, "afscp-direct-v1", "tmp")+string(os.PathSeparator)), cloneArgs[2])
	assert.Equal(t, "payload", filepath.Base(cloneArgs[2]))

	snapshotDir := directTestSnapshotDir(controlRoot, save.SavePointID)
	payloadDir := filepath.Join(snapshotDir, "payload")
	assert.DirExists(t, payloadDir)
	assert.FileExists(t, filepath.Join(payloadDir, "profile.txt"))
	assert.FileExists(t, filepath.Join(snapshotDir, "descriptor.json"))
	assert.FileExists(t, filepath.Join(snapshotDir, "ready"))
	assert.FileExists(t, filepath.Join(controlRoot, "afscp-direct-v1", "history.json"))
	assert.FileExists(t, filepath.Join(controlRoot, "afscp-direct-v1", "journal.json"))
	assertDirectJSONFieldNonEmpty(t, filepath.Join(snapshotDir, "descriptor.json"), "metadata_checksum")
	assertDirectJSONFieldNonEmpty(t, filepath.Join(snapshotDir, "ready"), "descriptor_checksum")
	assertDirectJSONFieldNonEmpty(t, filepath.Join(snapshotDir, "ready"), "metadata_checksum")
	assertDirectJSONFieldNonEmpty(t, filepath.Join(controlRoot, "afscp-direct-v1", "journal.json"), "metadata_checksum")
	assert.NoDirExists(t, filepath.Join(home, ".jvs"))
	assert.NoDirExists(t, filepath.Dir(cloneArgs[2]))

	rawDescriptor, err := os.ReadFile(filepath.Join(snapshotDir, "descriptor.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(rawDescriptor), home)
	assert.NotContains(t, string(rawDescriptor), controlRoot)
	assert.NotContains(t, string(rawDescriptor), "payload_root_hash")
	assert.NotContains(t, string(rawDescriptor), "content_root_hash")

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.NotNil(t, list.HistoryHead)
	assert.Equal(t, save.SavePointID, *list.HistoryHead)
	require.Len(t, list.SavePoints, 1)
	assert.Equal(t, save.SavePointID, list.SavePoints[0].SavePointID)
	assert.True(t, list.SavePoints[0].HistoryHead)
	assert.Equal(t, "ready", list.MetadataState)
}

func TestDirectSavePersistsStructuredTemplateSourcePurpose(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("template"), 0644))
	installFakeJuiceFSClone(t)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "Template source: starter",
		Purpose:  "template_source",
	})
	require.NoError(t, err)
	save := result.(SaveResult)
	assert.Equal(t, "template_source", save.Purpose)

	snapshotDir := directTestSnapshotDir(controlRoot, save.SavePointID)
	var descriptor map[string]any
	require.NoError(t, json.Unmarshal(directTestReadFile(t, filepath.Join(snapshotDir, "descriptor.json")), &descriptor))
	assert.Equal(t, "template_source", descriptor["purpose"])
	var history map[string]any
	require.NoError(t, json.Unmarshal(directTestReadFile(t, filepath.Join(controlRoot, "afscp-direct-v1", "history.json")), &history))
	rawSavePoints, ok := history["save_points"].([]any)
	require.True(t, ok)
	require.Len(t, rawSavePoints, 1)
	entry, ok := rawSavePoints[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "template_source", entry["purpose"])

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.Len(t, list.SavePoints, 1)
	assert.Equal(t, "template_source", list.SavePoints[0].Purpose)
}

func TestDirectSaveWholeHomeIncludesDotRuntimeDirsWorkspaceSymlinkAndEmptyDir(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	externalWorkspace := filepath.Join(base, "workspace-target")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cache", "agent"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "empty-dir"), 0755))
	require.NoError(t, os.Mkdir(externalWorkspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cache", "agent", "runtime.json"), []byte(`{"ok":true}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host *\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(home, "notes.txt"), []byte("whole home"), 0644))
	require.NoError(t, os.Symlink(externalWorkspace, filepath.Join(home, "workspace")))
	installFakeJuiceFSClone(t)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "whole home",
	})
	require.NoError(t, err)
	save := result.(SaveResult)

	payloadDir := filepath.Join(directTestSnapshotDir(controlRoot, save.SavePointID), "payload")
	assertTreesEqual(t, home, payloadDir)
}

func TestDirectListReadsHistoryHeadWithoutWalkingHome(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installFakeJuiceFSClone(t)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	require.NoError(t, err)
	save := result.(SaveResult)

	require.NoError(t, os.Mkdir(filepath.Join(home, "unreadable-after-save"), 0000))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(home, "unreadable-after-save"), 0755)
	})
	require.NoError(t, os.WriteFile(filepath.Join(home, "current-home-change.txt"), []byte("not history"), 0644))
	require.NoError(t, os.RemoveAll(filepath.Join(directTestSnapshotDir(controlRoot, save.SavePointID), "payload")))

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.NotNil(t, list.HistoryHead)
	assert.Equal(t, save.SavePointID, *list.HistoryHead)
	require.Len(t, list.SavePoints, 1)
	assert.Equal(t, "baseline", list.SavePoints[0].Message)
	assert.Equal(t, "ready", list.MetadataState)
}

func TestDirectStatusAndDoctorStableShapeAfterControlInit(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	r, err := repo.InitSeparatedControl(controlRoot, home, "main")
	require.NoError(t, err)

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Nil(t, list.HistoryHead)
	assert.Empty(t, list.SavePoints)
	assert.Equal(t, "ready", list.MetadataState)

	require.NoError(t, os.RemoveAll(home))

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, r.RepoID, status.RepoID)
	assert.Nil(t, status.HistoryHead)
	assert.Equal(t, "none", status.ActiveOperation)
	assert.Equal(t, "ready", status.MetadataState)
	assert.Equal(t, "none", status.Recovery)

	doctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, r.RepoID, doctor.RepoID)
	assert.True(t, doctor.Healthy)
	assert.Empty(t, doctor.Findings)
	assert.Equal(t, "ready", doctor.MetadataState)
	assert.Equal(t, "clean", doctor.Journal)
	assert.Equal(t, "none", doctor.Recovery)
}

func TestDirectStatusAndDoctorReportUninitializedBeforeControlInit(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))

	list, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Nil(t, list.HistoryHead)
	assert.Empty(t, list.SavePoints)
	assert.Equal(t, "uninitialized", list.MetadataState)

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, "", status.RepoID)
	assert.Nil(t, status.HistoryHead)
	assert.Equal(t, "none", status.ActiveOperation)
	assert.Equal(t, "uninitialized", status.MetadataState)
	assert.Equal(t, "none", status.Recovery)

	doctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, "", doctor.RepoID)
	assert.True(t, doctor.Healthy)
	assert.Empty(t, doctor.Findings)
	assert.Equal(t, "uninitialized", doctor.MetadataState)
	assert.Equal(t, "clean", doctor.Journal)
	assert.Equal(t, "none", doctor.Recovery)
}

func TestDirectInitialReadyStateRequiresWorkspaceHomeBinding(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	otherHome := filepath.Join(base, "other-home")
	_, err := repo.InitSeparatedControl(controlRoot, home, "main")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(otherHome, 0755))

	_, err = NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: otherHome},
	})
	requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)

	_, err = NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: otherHome},
	})
	requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)

	_, err = NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: otherHome},
	})
	requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)
}

func TestDirectStatusAndDoctorAreMetadataOnly(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	r, err := repo.InitSeparatedControl(controlRoot, home, "main")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cache", "agent"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cache", "agent", "runtime.json"), []byte(`{"ok":true}`), 0644))
	installFakeJuiceFSClone(t)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	require.NoError(t, err)
	save := result.(SaveResult)

	require.NoError(t, os.Chmod(filepath.Join(home, ".cache"), 0000))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(home, ".cache"), 0755)
	})
	require.NoError(t, os.RemoveAll(filepath.Join(directTestSnapshotDir(controlRoot, save.SavePointID), "payload")))

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, r.RepoID, status.RepoID)
	require.NotNil(t, status.HistoryHead)
	assert.Equal(t, save.SavePointID, *status.HistoryHead)
	assert.Equal(t, "none", status.ActiveOperation)
	assert.Equal(t, "ready", status.MetadataState)
	assert.Equal(t, "none", status.Recovery)

	doctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, r.RepoID, doctor.RepoID)
	assert.True(t, doctor.Healthy)
	assert.Empty(t, doctor.Findings)
	assert.Equal(t, "ready", doctor.MetadataState)
	assert.Equal(t, "clean", doctor.Journal)
	assert.Equal(t, "none", doctor.Recovery)
}

func TestDirectStatusAndDoctorFailClosedForInvalidPendingCleanup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mutate      func(t *testing.T, layout directLayout, marker string, cleanup directCleanupMetadata)
		wantFinding string
	}{
		{
			name: "unreferenced marker",
			mutate: func(t *testing.T, _ directLayout, marker string, _ directCleanupMetadata) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(marker, directCleanupMetadataFileName)))
			},
			wantFinding: "cleanup unreferenced",
		},
		{
			name: "invalid cleanup metadata",
			mutate: func(t *testing.T, _ directLayout, marker string, cleanup directCleanupMetadata) {
				t.Helper()
				cleanup.State = "unknown"
				require.NoError(t, writeDirectCleanupMetadata(marker, cleanup))
			},
			wantFinding: "cleanup non-convergent",
		},
		{
			name: "missing referenced backup",
			mutate: func(t *testing.T, layout directLayout, _ string, cleanup directCleanupMetadata) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(filepath.Dir(layout.selector.Home), cleanup.BackupHomeName)))
			},
			wantFinding: "cleanup non-convergent",
		},
		{
			name: "stale marker",
			mutate: func(t *testing.T, _ directLayout, marker string, cleanup directCleanupMetadata) {
				t.Helper()
				cleanup.CreatedAt = time.Now().UTC().Add(-directPendingCleanupFreshWindow - time.Hour).Format(time.RFC3339Nano)
				require.NoError(t, writeDirectCleanupMetadata(marker, cleanup))
			},
			wantFinding: "cleanup stale",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			home := filepath.Join(base, "home")
			require.NoError(t, os.Mkdir(controlRoot, 0755))
			require.NoError(t, os.Mkdir(home, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
			installFakeJuiceFSClone(t)
			save := directTestSave(t, controlRoot, home, "baseline")

			require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("dirty"), 0644))
			_, err := NewService().Restore(context.Background(), Request{
				Selector:    Selector{ControlRoot: controlRoot, Home: home},
				SavePointID: save.SavePointID,
			})
			require.NoError(t, err)

			layout := newDirectLayout(ResolvedSelector{ControlRoot: controlRoot, Home: home})
			cleanupMarkers := directTestPendingCleanupMarkers(t, controlRoot)
			require.Len(t, cleanupMarkers, 1)
			cleanupMetadata, err := readDirectCleanupMetadata(layout, cleanupMarkers[0])
			require.NoError(t, err)
			tc.mutate(t, layout, cleanupMarkers[0], cleanupMetadata)

			status, err := NewService().Status(context.Background(), Request{
				Selector: Selector{ControlRoot: controlRoot, Home: home},
			})
			require.NoError(t, err)
			assert.Equal(t, "invalid", status.MetadataState)
			assert.Equal(t, "repair_metadata", status.Recovery)

			doctor, err := NewService().Doctor(context.Background(), Request{
				Selector: Selector{ControlRoot: controlRoot, Home: home},
			})
			require.NoError(t, err)
			assert.False(t, doctor.Healthy)
			assert.Equal(t, "invalid", doctor.MetadataState)
			assert.Equal(t, "repair_metadata", doctor.Recovery)
			assertDirectDoctorFindingContains(t, doctor, tc.wantFinding)
		})
	}
}

func TestDirectStatusAndDoctorTreatRestoreCleanupEvidenceAsNonBlocking(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")

	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("dirty"), 0644))
	_, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	require.NoError(t, err)
	require.Len(t, directTestPendingCleanupMarkers(t, controlRoot), 1)
	require.Len(t, directTestRestoreSiblingBackupNames(t, home), 1)

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
	assert.Equal(t, "cleanup_pending", doctor.Recovery)
	assertDirectDoctorFindingContains(t, doctor, "cleanup pending")
}

func TestDirectStatusAndDoctorIgnoreUnrelatedSiblingRestoreBackups(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")
	require.NoError(t, os.Mkdir(filepath.Join(filepath.Dir(home), directRestoreBackupPrefix+"unrelated"), 0755))

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	require.NotNil(t, status.HistoryHead)
	assert.Equal(t, save.SavePointID, *status.HistoryHead)
	assert.Equal(t, "none", status.ActiveOperation)
	assert.Equal(t, "ready", status.MetadataState)
	assert.Equal(t, "none", status.Recovery)

	doctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.True(t, doctor.Healthy)
	assert.Empty(t, doctor.Findings)
	assert.Equal(t, "ready", doctor.MetadataState)
	assert.Equal(t, "none", doctor.Recovery)
}

func TestDirectSaveFailsFastWhenJuiceFSUnavailableWithoutCopyFallback(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("must not be copied"), 0644))
	t.Setenv("PATH", t.TempDir())

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeCloneUnavailable, ExitStorage)
	assert.NoDirExists(t, filepath.Join(controlRoot, "afscp-direct-v1", "snapshots", "copied-fallback"))
	assert.NoDirExists(t, filepath.Join(home, ".jvs"))
}

func TestDirectMutationsReturnLockedWhenMutationLockExists(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installFakeJuiceFSClone(t)
	save := directTestSave(t, controlRoot, home, "baseline")

	require.NoError(t, os.Mkdir(directTestMutationLockDir(controlRoot), 0700))

	saveResult, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "conflicting save",
	})
	assert.Nil(t, saveResult)
	requireDirectError(t, err, ErrorCodeLocked, ExitLocked)

	restoreResult, err := NewService().Restore(context.Background(), Request{
		Selector:    Selector{ControlRoot: controlRoot, Home: home},
		SavePointID: save.SavePointID,
	})
	assert.Nil(t, restoreResult)
	requireDirectError(t, err, ErrorCodeLocked, ExitLocked)
}

func TestDirectSaveWritesRunningAndFailedJournalAroundClone(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	journalPath := filepath.Join(controlRoot, "afscp-direct-v1", "journal.json")
	cloneLog := installInspectingFailingJuiceFSClone(t, journalPath)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeCloneFailed, ExitStorage)

	journalDuringClone := string(directTestReadFile(t, cloneLog))
	assert.Contains(t, journalDuringClone, `"phase": "save_running"`)
	assert.Contains(t, journalDuringClone, `"metadata_checksum"`)

	layout, initialized, err := openDirectLayout(ResolvedSelector{ControlRoot: controlRoot, Home: home})
	require.NoError(t, err)
	require.True(t, initialized)
	journal, err := readDirectJournal(layout)
	require.NoError(t, err)
	require.NotNil(t, journal)
	assert.Equal(t, "save_failed", journal.Phase)
	assert.NotEmpty(t, journal.TargetSavePointID)

	status, err := NewService().Status(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
	})
	require.NoError(t, err)
	assert.Equal(t, "failed", status.ActiveOperation)
}

func TestDirectMetadataChecksumsFailClosedForDescriptorReadyAndJournal(t *testing.T) {
	for _, tc := range []struct {
		name           string
		corrupt        func(t *testing.T, controlRoot, savePointID string)
		wantListError  bool
		wantRestoreErr bool
		wantFinding    string
	}{
		{
			name: "descriptor checksum mismatch",
			corrupt: func(t *testing.T, controlRoot, savePointID string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(directTestSnapshotDir(controlRoot, savePointID), "descriptor.json"), []byte(`{"version":1}`), 0644))
			},
			wantListError:  true,
			wantRestoreErr: true,
			wantFinding:    "descriptor",
		},
		{
			name: "ready checksum mismatch",
			corrupt: func(t *testing.T, controlRoot, savePointID string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(directTestSnapshotDir(controlRoot, savePointID), "ready"), []byte(`{"state":"ready"}`), 0644))
			},
			wantListError:  true,
			wantRestoreErr: true,
			wantFinding:    "ready",
		},
		{
			name: "journal checksum mismatch",
			corrupt: func(t *testing.T, controlRoot, savePointID string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(controlRoot, "afscp-direct-v1", "journal.json"), []byte(`{"phase":"idle"}`), 0644))
			},
			wantRestoreErr: true,
			wantFinding:    "journal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			home := filepath.Join(base, "home")
			require.NoError(t, os.Mkdir(controlRoot, 0755))
			require.NoError(t, os.Mkdir(home, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
			installFakeJuiceFSClone(t)
			save := directTestSave(t, controlRoot, home, "baseline")

			tc.corrupt(t, controlRoot, save.SavePointID)

			if tc.wantListError {
				_, err := NewService().List(context.Background(), Request{
					Selector: Selector{ControlRoot: controlRoot, Home: home},
				})
				requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)
			}
			if tc.wantRestoreErr {
				result, err := NewService().Restore(context.Background(), Request{
					Selector:    Selector{ControlRoot: controlRoot, Home: home},
					SavePointID: save.SavePointID,
				})
				assert.Nil(t, result)
				requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)
			}

			status, err := NewService().Status(context.Background(), Request{
				Selector: Selector{ControlRoot: controlRoot, Home: home},
			})
			require.NoError(t, err)
			assert.Equal(t, "invalid", status.MetadataState)
			assert.Equal(t, "repair_metadata", status.Recovery)

			doctor, err := NewService().Doctor(context.Background(), Request{
				Selector: Selector{ControlRoot: controlRoot, Home: home},
			})
			require.NoError(t, err)
			assert.False(t, doctor.Healthy)
			assert.Equal(t, "invalid", doctor.MetadataState)
			assert.Equal(t, "repair_metadata", doctor.Recovery)
			assertDirectDoctorFindingContains(t, doctor, tc.wantFinding)
		})
	}
}

func TestDirectMetadataBindingRejectsDifferentCanonicalHome(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	otherHome := filepath.Join(base, "other-home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.Mkdir(otherHome, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "profile.txt"), []byte("baseline"), 0644))
	installFakeJuiceFSClone(t)

	_, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: home},
		Message:  "baseline",
	})
	require.NoError(t, err)
	t.Setenv("PATH", t.TempDir())

	_, err = NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: otherHome},
	})
	requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)

	result, err := NewService().Save(context.Background(), Request{
		Selector: Selector{ControlRoot: controlRoot, Home: otherHome},
		Message:  "must fail before clone availability",
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeMetadataInvalid, ExitMetadata)
}

func installFakeJuiceFSClone(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "juicefs.log")
	juicefsPath := filepath.Join(binDir, "juicefs")
	script := `#!/bin/sh
set -eu
if [ "$#" -lt 3 ] || [ "$1" != "clone" ]; then
  printf 'unexpected juicefs args: %s\n' "$*" >&2
  exit 64
fi
printf '%s\n%s\n%s\n' "$1" "$2" "$3" >> "$JVS_TEST_JUICEFS_LOG"
/bin/mkdir -p "$3"
/bin/cp -a "$2"/. "$3"/
`
	require.NoError(t, os.WriteFile(juicefsPath, []byte(script), 0755))
	t.Setenv("PATH", binDir)
	t.Setenv("JVS_TEST_JUICEFS_LOG", logPath)
	return logPath
}

func readFakeJuiceFSCloneArgs(t *testing.T, logPath string) []string {
	t.Helper()

	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func directTestSnapshotDir(controlRoot, savePointID string) string {
	return filepath.Join(controlRoot, "afscp-direct-v1", "snapshots", savePointID)
}

func directTestMutationLockDir(controlRoot string) string {
	return filepath.Join(controlRoot, "afscp-direct-v1", "mutation.lock")
}

func installInspectingFailingJuiceFSClone(t *testing.T, journalPath string) string {
	t.Helper()

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "juicefs.log")
	juicefsPath := filepath.Join(binDir, "juicefs")
	script := `#!/bin/sh
set -eu
if [ "$#" -lt 3 ] || [ "$1" != "clone" ]; then
  printf 'unexpected juicefs args: %s\n' "$*" >&2
  exit 64
fi
if [ -f "$JVS_TEST_DIRECT_JOURNAL" ]; then
  /bin/cat "$JVS_TEST_DIRECT_JOURNAL" >> "$JVS_TEST_JUICEFS_LOG"
fi
exit 7
`
	require.NoError(t, os.WriteFile(juicefsPath, []byte(script), 0755))
	t.Setenv("PATH", binDir)
	t.Setenv("JVS_TEST_DIRECT_JOURNAL", journalPath)
	t.Setenv("JVS_TEST_JUICEFS_LOG", logPath)
	return logPath
}

func assertDirectJSONFieldNonEmpty(t *testing.T, path, field string) {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(directTestReadFile(t, path), &payload))
	value, ok := payload[field].(string)
	require.True(t, ok, "field %s should be a string in %s: %#v", field, path, payload)
	assert.NotEmpty(t, value)
}

func assertDirectDoctorFindingContains(t *testing.T, doctor DoctorResult, want string) {
	t.Helper()

	for _, finding := range doctor.Findings {
		if strings.Contains(finding.Message, want) {
			return
		}
	}
	t.Fatalf("doctor findings should contain %q: %#v", want, doctor.Findings)
}

func assertDirectCloneEvidence(t *testing.T, evidence CloneEvidence, operation, phase string) {
	t.Helper()

	assert.Equal(t, operation, evidence.Operation)
	assert.Equal(t, phase, evidence.Phase)
	assert.Equal(t, "juicefs_clone", evidence.Engine)
	assert.Equal(t, string(StatusSucceeded), evidence.Status)
	started, err := time.Parse(time.RFC3339Nano, evidence.StartedAt)
	require.NoError(t, err)
	finished, err := time.Parse(time.RFC3339Nano, evidence.FinishedAt)
	require.NoError(t, err)
	assert.False(t, finished.Before(started))
	assert.GreaterOrEqual(t, evidence.DurationMs, int64(0))
}

type directTestTreeEntry struct {
	Kind    string
	Content string
	Target  string
}

func assertTreesEqual(t *testing.T, wantRoot, gotRoot string) {
	t.Helper()

	assert.Equal(t, collectDirectTestTree(t, wantRoot), collectDirectTestTree(t, gotRoot))
}

func collectDirectTestTree(t *testing.T, root string) map[string]directTestTreeEntry {
	t.Helper()

	entries := map[string]directTestTreeEntry{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries[rel] = directTestTreeEntry{Kind: "symlink", Target: target}
		case info.IsDir():
			entries[rel] = directTestTreeEntry{Kind: "dir"}
		case info.Mode().IsRegular():
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entries[rel] = directTestTreeEntry{Kind: "file", Content: string(content)}
		default:
			encoded, err := json.Marshal(info.Mode().String())
			if err != nil {
				return err
			}
			entries[rel] = directTestTreeEntry{Kind: "other", Content: string(encoded)}
		}
		return nil
	})
	require.NoError(t, err)
	return entries
}
