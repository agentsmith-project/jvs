package afscp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	directRestoreTmpPrefix = ".jvs-afscp-restore-tmp-"
)

var directRestoreAfterBackupHomeHook func() error
var directRestoreAfterPublishHomeHook func() error

type directHomeIdentity struct {
	info os.FileInfo
}

type directRestoreState struct {
	homeIdentity directHomeIdentity
	tmpName      string
	tmpTop       string
	tmpPayload   string
	cleanupTop   string
	backupTop    string
	cleanupTmp   bool
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

	if err := backupDirectRestoreCurrentHome(layout, selector, history, savePointID, state); err != nil {
		return RestoreResult{}, err
	}
	if err := publishDirectRestorePayload(layout, selector, history, savePointID, state); err != nil {
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
	homeIdentity, err := captureDirectHomeIdentity(selector.Home)
	if err != nil {
		return nil, err
	}
	tmpName, err := newDirectRestoreTmpName(selector.Home, snapshotPayload)
	if err != nil {
		return nil, err
	}
	state := &directRestoreState{
		homeIdentity: homeIdentity,
		tmpName:      tmpName,
		tmpTop:       filepath.Join(selector.Home, tmpName),
		cleanupTop:   filepath.Join(layout.cleanup, "restore-"+uuidutil.NewV4()),
		cleanupTmp:   true,
	}
	state.tmpPayload = filepath.Join(state.tmpTop, directPayloadDirName)
	state.backupTop = filepath.Join(state.cleanupTop, directPayloadDirName)
	if err := os.Mkdir(state.tmpTop, 0700); err != nil {
		return nil, NewError(ErrorCodeInternal, "create direct restore staging", false)
	}

	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreStaging, savePointID, tmpName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		state.cleanup()
		return nil, err
	}
	if err := runStrictJuiceFSClone(ctx, snapshotPayload, state.tmpPayload); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return nil, err
	}
	if err := requireRealDirectory(state.tmpPayload); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return nil, NewError(ErrorCodeCloneFailed, "juicefs clone did not create restore payload", false)
	}
	if err := homeIdentity.check(selector.Home); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return nil, err
	}
	return state, nil
}

func backupDirectRestoreCurrentHome(layout directLayout, selector ResolvedSelector, history directHistory, savePointID string, state *directRestoreState) error {
	if err := os.MkdirAll(state.backupTop, 0700); err != nil {
		abortDirectRestoreAttempt(layout, history.Head, state)
		return NewError(ErrorCodeInternal, "create direct restore backup", false)
	}
	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreBackingUp, savePointID, state.tmpName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		cleanupDirectRestoreBackup(state)
		return err
	}
	if err := backupDirectHomeTopLevel(selector.Home, state.tmpName, state.backupTop); err != nil {
		return rollbackDirectRestoreBackupFailure(layout, selector, history, state, "direct restore could not backup HOME")
	}
	if directRestoreAfterBackupHomeHook == nil {
		return nil
	}
	if err := directRestoreAfterBackupHomeHook(); err != nil {
		return rollbackDirectRestoreBackupFailure(layout, selector, history, state, "direct restore interrupted before replace")
	}
	return nil
}

func publishDirectRestorePayload(layout directLayout, selector ResolvedSelector, history directHistory, savePointID string, state *directRestoreState) error {
	if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreReplacing, savePointID, state.tmpName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		if rollbackErr := rollbackDirectRestoreBackup(selector.Home, state.tmpName, state.backupTop); rollbackErr != nil {
			state.cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
		}
		cleanupDirectRestoreBackup(state)
		abortDirectRestoreAttempt(layout, history.Head, state)
		return err
	}
	if err := moveDirectRestorePayloadEntries(state.tmpPayload, selector.Home); err != nil {
		state.cleanupTmp = false
		return directRestoreRecoveryRequired("direct restore could not publish payload entries")
	}
	if directRestoreAfterPublishHomeHook != nil {
		if err := directRestoreAfterPublishHomeHook(); err != nil {
			state.cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore interrupted after publishing HOME")
		}
	}
	if err := state.homeIdentity.check(selector.Home); err != nil {
		state.cleanupTmp = false
		return directRestoreRecoveryRequired("direct restore HOME identity changed")
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
	if err := os.Remove(state.tmpPayload); err != nil {
		state.cleanupTmp = false
		return RestoreResult{}, directRestoreRecoveryRequired("direct restore could not clean staging payload")
	}
	if err := os.Remove(state.tmpTop); err != nil {
		state.cleanupTmp = false
		return RestoreResult{}, directRestoreRecoveryRequired("direct restore could not clean staging")
	}
	state.cleanupTmp = false
	if err := writeDirectRestoreIdleJournal(layout, history.Head); err != nil {
		return RestoreResult{}, directRestoreRecoveryRequired("direct restore could not mark journal idle")
	}
	return RestoreResult{
		RestoredSavePointID: savePointID,
		PreviousHead:        previousHead,
		NewHead:             savePointID,
	}, nil
}

func rollbackDirectRestoreBackupFailure(layout directLayout, selector ResolvedSelector, history directHistory, state *directRestoreState, message string) error {
	if rollbackErr := rollbackDirectRestoreBackup(selector.Home, state.tmpName, state.backupTop); rollbackErr != nil {
		state.cleanupTmp = false
		return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
	}
	cleanupDirectRestoreBackup(state)
	abortDirectRestoreAttempt(layout, history.Head, state)
	return NewError(ErrorCodeInternal, message, false)
}

func abortDirectRestoreAttempt(layout directLayout, historyHead *string, state *directRestoreState) {
	state.cleanup()
	_ = writeDirectRestoreIdleJournal(layout, historyHead)
}

func cleanupDirectRestoreBackup(state *directRestoreState) {
	_ = os.Remove(state.backupTop)
	_ = os.Remove(state.cleanupTop)
}

func (state *directRestoreState) cleanup() {
	if state != nil && state.cleanupTmp {
		_ = os.RemoveAll(state.tmpTop)
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

func captureDirectHomeIdentity(home string) (directHomeIdentity, error) {
	info, err := os.Lstat(home)
	if err != nil {
		return directHomeIdentity{}, NewError(ErrorCodeInvalidArgument, "home must be an existing directory", false)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return directHomeIdentity{}, NewError(ErrorCodeInvalidArgument, "home must be a real directory", false)
	}
	return directHomeIdentity{info: info}, nil
}

func (identity directHomeIdentity) check(home string) error {
	info, err := os.Lstat(home)
	if err != nil {
		return NewError(ErrorCodeMetadataInvalid, "home root changed during restore", false)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !os.SameFile(identity.info, info) {
		return NewError(ErrorCodeMetadataInvalid, "home root changed during restore", false)
	}
	return nil
}

func newDirectRestoreTmpName(home, snapshotPayload string) (string, error) {
	for i := 0; i < 8; i++ {
		name := directRestoreTmpPrefix + uuidutil.NewV4()
		if directPathExists(filepath.Join(home, name)) || directPathExists(filepath.Join(snapshotPayload, name)) {
			continue
		}
		return name, nil
	}
	return "", NewError(ErrorCodeInternal, "create direct restore staging", false)
}

func backupDirectHomeTopLevel(home, keepName, backupTop string) error {
	entries, err := os.ReadDir(home)
	if err != nil {
		return fmt.Errorf("read HOME: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == keepName {
			continue
		}
		if err := os.Rename(filepath.Join(home, entry.Name()), filepath.Join(backupTop, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func rollbackDirectRestoreBackup(home, keepName, backupTop string) error {
	entries, err := os.ReadDir(backupTop)
	if err != nil {
		return fmt.Errorf("read restore backup: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == keepName {
			return fmt.Errorf("restore backup contains staging name")
		}
		if err := os.Rename(filepath.Join(backupTop, entry.Name()), filepath.Join(home, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func moveDirectRestorePayloadEntries(payload, home string) error {
	entries, err := os.ReadDir(payload)
	if err != nil {
		return fmt.Errorf("read restore payload: %w", err)
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(payload, entry.Name()), filepath.Join(home, entry.Name())); err != nil {
			return err
		}
	}
	return nil
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

func writeDirectRestoreJournal(layout directLayout, phase, savePointID, tmpName, updatedAt, failureCode, reason string) error {
	return writeDirectJournal(layout, directJournal{
		Version:           1,
		Contract:          ContractVersion,
		Workspace:         directWorkspaceName,
		Phase:             phase,
		TargetSavePointID: savePointID,
		RestoreTmpName:    tmpName,
		FailureCode:       failureCode,
		Reason:            reason,
		UpdatedAt:         updatedAt,
	})
}

func directRestoreRecoveryRequired(message string) error {
	return NewError(ErrorCodeJournalRecoveryRequired, message, false)
}
