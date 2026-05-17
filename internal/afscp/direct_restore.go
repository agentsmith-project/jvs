package afscp

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	directRestoreTmpPrefix    = ".jvs-afscp-restore-tmp-"
	directRestoreBackupPrefix = ".jvs-afscp-restore-backup-"
)

var directRestoreAfterBackupHomeHook func() error
var directRestoreAfterPublishHomeHook func() error

type directRestoreState struct {
	tmpName       string
	tmpHome       string
	backupName    string
	backupHome    string
	cleanupMarker string
	cleanupTmp    bool
	cloneEvidence CloneEvidence
}

func restoreDirect(ctx context.Context, selector ResolvedSelector, savePointID string) (RestoreResult, error) {
	layout, initialized, err := openDirectLayout(selector)
	if err != nil {
		return RestoreResult{}, err
	}
	if !initialized {
		return RestoreResult{}, NewError(ErrorCodeSavePointNotFound, "save point not found", false)
	}
	var result RestoreResult
	err = withDirectMutationLock(layout, func() error {
		var err error
		result, err = restoreDirectLocked(ctx, selector, layout, savePointID)
		return err
	})
	if err != nil {
		return RestoreResult{}, err
	}
	return result, nil
}

func restoreDirectLocked(ctx context.Context, selector ResolvedSelector, layout directLayout, savePointID string) (RestoreResult, error) {
	history, snapshotPayload, err := validateDirectRestoreTarget(layout, savePointID)
	if err != nil {
		return RestoreResult{}, err
	}
	state, err := stageDirectRestorePayload(ctx, selector, layout, history, savePointID, snapshotPayload)
	if err != nil {
		return RestoreResult{}, err
	}
	defer state.cleanup()

	if err := publishDirectRestoreHome(layout, selector, history, savePointID, state); err != nil {
		return RestoreResult{}, err
	}
	return finishDirectRestore(layout, history, savePointID, state)
}

func validateDirectRestoreTarget(layout directLayout, savePointID string) (directHistory, string, error) {
	history, err := readDirectHistory(layout)
	if err != nil {
		return directHistory{}, "", err
	}
	if !directHistoryContains(history, savePointID) {
		return directHistory{}, "", NewError(ErrorCodeSavePointNotFound, "save point not found", false)
	}
	desc, err := readDirectDescriptor(layout, savePointID)
	if err != nil {
		return directHistory{}, "", err
	}
	if err := requireDirectReady(layout, savePointID, desc.Checksum); err != nil {
		return directHistory{}, "", err
	}
	snapshotPayload := filepath.Join(layout.snapshots, savePointID, directPayloadDirName)
	if err := requireRealDirectory(snapshotPayload); err != nil {
		return directHistory{}, "", NewError(ErrorCodeMetadataInvalid, "direct snapshot payload metadata is invalid", false)
	}
	if err := requireDirectJournalAllowsRestore(layout); err != nil {
		return directHistory{}, "", err
	}
	return history, snapshotPayload, nil
}

func stageDirectRestorePayload(ctx context.Context, selector ResolvedSelector, layout directLayout, history directHistory, savePointID, snapshotPayload string) (*directRestoreState, error) {
	tmpName, err := newDirectRestoreSiblingName(selector.Home, directRestoreTmpPrefix)
	if err != nil {
		return nil, err
	}
	backupName, err := newDirectRestoreSiblingName(selector.Home, directRestoreBackupPrefix)
	if err != nil {
		return nil, err
	}
	homeParent := filepath.Dir(selector.Home)
	state := &directRestoreState{
		tmpName:       tmpName,
		tmpHome:       filepath.Join(homeParent, tmpName),
		backupName:    backupName,
		backupHome:    filepath.Join(homeParent, backupName),
		cleanupMarker: filepath.Join(layout.cleanup, "restore-"+uuidutil.NewV4()),
		cleanupTmp:    true,
	}

	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreStaging, savePointID, tmpName, backupName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		state.cleanup()
		return nil, err
	}
	cloneEvidence, err := runStrictJuiceFSCloneWithEvidence(ctx, "restore", "restore_staging", snapshotPayload, state.tmpHome)
	if err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return nil, err
	}
	state.cloneEvidence = cloneEvidence
	if err := requireRealDirectory(state.tmpHome); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return nil, NewError(ErrorCodeCloneFailed, "juicefs clone did not create restore payload", false)
	}
	return state, nil
}

func publishDirectRestoreHome(layout directLayout, selector ResolvedSelector, history directHistory, savePointID string, state *directRestoreState) error {
	if err := os.Mkdir(state.cleanupMarker, 0700); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return NewError(ErrorCodeInternal, "create direct restore cleanup marker", false)
	}
	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreBackingUp, savePointID, state.tmpName, state.backupName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		cleanupDirectRestoreMarker(state)
		abortDirectRestoreAttempt(layout, history.Head, state)
		return err
	}
	if err := os.Rename(selector.Home, state.backupHome); err != nil {
		cleanupDirectRestoreMarker(state)
		abortDirectRestoreAttempt(layout, history.Head, state)
		return NewError(ErrorCodeInternal, "direct restore could not move HOME to backup", false)
	}
	if err := writeDirectRestoreCleanupMetadata(state, savePointID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return rollbackDirectRestoreSiblingBackupFailure(layout, selector, history, state, "direct restore could not write cleanup metadata")
	}
	if directRestoreAfterBackupHomeHook != nil {
		if err := directRestoreAfterBackupHomeHook(); err != nil {
			return rollbackDirectRestoreSiblingBackupFailure(layout, selector, history, state, "direct restore interrupted before replace")
		}
	}
	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreReplacing, savePointID, state.tmpName, state.backupName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		return rollbackDirectRestoreSiblingBackupFailure(layout, selector, history, state, "direct restore could not enter replace boundary")
	}
	if err := os.Rename(state.tmpHome, selector.Home); err != nil {
		if rollbackErr := os.Rename(state.backupHome, selector.Home); rollbackErr != nil {
			state.cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not publish cloned HOME")
		}
		cleanupDirectRestoreMarker(state)
		abortDirectRestoreAttempt(layout, history.Head, state)
		return NewError(ErrorCodeInternal, "direct restore could not publish cloned HOME", false)
	}
	state.cleanupTmp = false
	if directRestoreAfterPublishHomeHook != nil {
		if err := directRestoreAfterPublishHomeHook(); err != nil {
			return directRestoreRecoveryRequired("direct restore interrupted after publishing HOME")
		}
	}
	return nil
}

func finishDirectRestore(layout directLayout, history directHistory, savePointID string, state *directRestoreState) (RestoreResult, error) {
	previousHead := history.Head
	history.Head = &savePointID
	if err := writeDirectJSON(layout.history, history, 0644); err != nil {
		state.cleanupTmp = false
		return RestoreResult{}, directRestoreRecoveryRequired("direct restore could not update history")
	}
	if err := writeDirectRestoreIdleJournal(layout, history.Head); err != nil {
		return RestoreResult{}, directRestoreRecoveryRequired("direct restore could not mark journal idle")
	}
	return RestoreResult{
		RestoredSavePointID: savePointID,
		PreviousHead:        previousHead,
		NewHead:             savePointID,
		CloneEvidence:       []CloneEvidence{state.cloneEvidence},
	}, nil
}

func rollbackDirectRestoreSiblingBackupFailure(layout directLayout, selector ResolvedSelector, history directHistory, state *directRestoreState, message string) error {
	if rollbackErr := os.Rename(state.backupHome, selector.Home); rollbackErr != nil {
		state.cleanupTmp = false
		return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
	}
	cleanupDirectRestoreMarker(state)
	abortDirectRestoreAttempt(layout, history.Head, state)
	return NewError(ErrorCodeInternal, message, false)
}

func abortDirectRestoreAttempt(layout directLayout, historyHead *string, state *directRestoreState) {
	state.cleanup()
	cleanupDirectRestoreMarker(state)
	_ = writeDirectRestoreIdleJournal(layout, historyHead)
}

func cleanupDirectRestoreMarker(state *directRestoreState) {
	if state == nil || state.cleanupMarker == "" {
		return
	}
	_ = os.Remove(state.cleanupMarker)
}

func writeDirectRestoreCleanupMetadata(state *directRestoreState, savePointID, createdAt string) error {
	return writeDirectCleanupMetadata(state.cleanupMarker, directCleanupMetadata{
		Version:        1,
		Contract:       ContractVersion,
		Workspace:      directWorkspaceName,
		Kind:           directCleanupKindRestoreBackup,
		State:          directCleanupStatePending,
		SavePointID:    savePointID,
		BackupHomeName: state.backupName,
		CreatedAt:      createdAt,
	})
}

func (state *directRestoreState) cleanup() {
	if state != nil && state.cleanupTmp {
		_ = os.RemoveAll(state.tmpHome)
	}
}

func directHistoryContains(history directHistory, savePointID string) bool {
	for _, entry := range history.SavePoints {
		if entry.SavePointID == savePointID {
			return true
		}
	}
	return false
}

func requireDirectJournalAllowsRestore(layout directLayout) error {
	journal, err := readDirectJournal(layout)
	if err != nil {
		return err
	}
	if journal == nil ||
		journal.Phase == "" ||
		journal.Phase == directJournalPhaseIdle ||
		journal.Phase == directJournalPhaseSaveFailed {
		return nil
	}
	return directRestoreRecoveryRequired("direct restore recovery is required")
}

func newDirectRestoreSiblingName(home, prefix string) (string, error) {
	parent := filepath.Dir(home)
	for i := 0; i < 8; i++ {
		name := prefix + uuidutil.NewV4()
		if directPathExists(filepath.Join(parent, name)) {
			continue
		}
		return name, nil
	}
	return "", NewError(ErrorCodeInternal, "create direct restore staging", false)
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.ErrInvalid
	}
	return nil
}

func directPathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !os.IsNotExist(err)
}

func writeDirectRestoreIdleJournal(layout directLayout, historyHead *string) error {
	last := ""
	if historyHead != nil {
		last = *historyHead
	}
	return writeDirectJournal(layout, directJournal{
		Version:         1,
		Contract:        ContractVersion,
		Workspace:       directWorkspaceName,
		Phase:           directJournalPhaseIdle,
		LastSavePointID: last,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func writeDirectRestoreJournal(layout directLayout, phase, savePointID, tmpName, backupName, updatedAt, failureCode, reason string) error {
	return writeDirectJournal(layout, directJournal{
		Version:           1,
		Contract:          ContractVersion,
		Workspace:         directWorkspaceName,
		Phase:             phase,
		TargetSavePointID: savePointID,
		RestoreTmpName:    tmpName,
		BackupHomeName:    backupName,
		FailureCode:       failureCode,
		Reason:            reason,
		UpdatedAt:         updatedAt,
	})
}

func directRestoreRecoveryRequired(message string) error {
	return NewError(ErrorCodeJournalRecoveryRequired, message, false)
}
