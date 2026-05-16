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
		history, err := readDirectHistory(layout)
		if err != nil {
			return err
		}
		if !directHistoryContains(history, savePointID) {
			return NewError(ErrorCodeSavePointNotFound, "save point not found", false)
		}
		desc, err := readDirectDescriptor(layout, savePointID)
		if err != nil {
			return err
		}
		if err := requireDirectReady(layout, savePointID, desc.Checksum); err != nil {
			return err
		}
		snapshotPayload := filepath.Join(layout.snapshots, savePointID, directPayloadDirName)
		if err := requireRealDirectory(snapshotPayload); err != nil {
			return NewError(ErrorCodeMetadataInvalid, "direct snapshot payload metadata is invalid", false)
		}
		if err := requireDirectJournalAllowsRestore(layout); err != nil {
			return err
		}

		homeIdentity, err := captureDirectHomeIdentity(selector.Home)
		if err != nil {
			return err
		}
		tmpName, err := newDirectRestoreTmpName(selector.Home, snapshotPayload)
		if err != nil {
			return err
		}
		tmpTop := filepath.Join(selector.Home, tmpName)
		tmpPayload := filepath.Join(tmpTop, directPayloadDirName)
		cleanupName := "restore-" + uuidutil.NewV4()
		cleanupTop := filepath.Join(layout.cleanup, cleanupName)
		backupTop := filepath.Join(cleanupTop, directPayloadDirName)
		if err := os.Mkdir(tmpTop, 0700); err != nil {
			return NewError(ErrorCodeInternal, "create direct restore staging", false)
		}
		cleanupTmp := true
		defer func() {
			if cleanupTmp {
				_ = os.RemoveAll(tmpTop)
			}
		}()

		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreStaging, savePointID, tmpName, now, "", ""); err != nil {
			return err
		}

		if err := runStrictJuiceFSClone(ctx, snapshotPayload, tmpPayload); err != nil {
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return err
		}
		if err := requireRealDirectory(tmpPayload); err != nil {
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return NewError(ErrorCodeCloneFailed, "juicefs clone did not create restore payload", false)
		}
		if err := homeIdentity.check(selector.Home); err != nil {
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return err
		}

		if err := os.MkdirAll(backupTop, 0700); err != nil {
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return NewError(ErrorCodeInternal, "create direct restore backup", false)
		}
		if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreBackingUp, savePointID, tmpName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
			_ = os.Remove(backupTop)
			_ = os.Remove(cleanupTop)
			_ = os.RemoveAll(tmpTop)
			return err
		}
		if err := backupDirectHomeTopLevel(selector.Home, tmpName, backupTop); err != nil {
			if rollbackErr := rollbackDirectRestoreBackup(selector.Home, tmpName, backupTop); rollbackErr != nil {
				cleanupTmp = false
				return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
			}
			_ = os.Remove(backupTop)
			_ = os.Remove(cleanupTop)
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return NewError(ErrorCodeInternal, "direct restore could not backup HOME", false)
		}
		if directRestoreAfterBackupHomeHook != nil {
			if err := directRestoreAfterBackupHomeHook(); err != nil {
				if rollbackErr := rollbackDirectRestoreBackup(selector.Home, tmpName, backupTop); rollbackErr != nil {
					cleanupTmp = false
					return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
				}
				_ = os.Remove(backupTop)
				_ = os.Remove(cleanupTop)
				_ = os.RemoveAll(tmpTop)
				_ = writeDirectRestoreIdleJournal(layout, history.Head)
				return NewError(ErrorCodeInternal, "direct restore interrupted before replace", false)
			}
		}

		if err := writeDirectRestoreJournal(layout, directJournalPhaseRestoreReplacing, savePointID, tmpName, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
			if rollbackErr := rollbackDirectRestoreBackup(selector.Home, tmpName, backupTop); rollbackErr != nil {
				cleanupTmp = false
				return directRestoreRecoveryRequired("direct restore could not rollback HOME backup")
			}
			_ = os.Remove(backupTop)
			_ = os.Remove(cleanupTop)
			_ = os.RemoveAll(tmpTop)
			_ = writeDirectRestoreIdleJournal(layout, history.Head)
			return err
		}
		if err := moveDirectRestorePayloadEntries(tmpPayload, selector.Home); err != nil {
			cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not publish payload entries")
		}
		if directRestoreAfterPublishHomeHook != nil {
			if err := directRestoreAfterPublishHomeHook(); err != nil {
				cleanupTmp = false
				return directRestoreRecoveryRequired("direct restore interrupted after publishing HOME")
			}
		}
		if err := homeIdentity.check(selector.Home); err != nil {
			cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore HOME identity changed")
		}

		previousHead := history.Head
		history.Head = &savePointID
		if err := writeDirectJSON(layout.history, history, 0644); err != nil {
			cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not update history")
		}
		if err := os.Remove(tmpPayload); err != nil {
			cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not clean staging payload")
		}
		if err := os.Remove(tmpTop); err != nil {
			cleanupTmp = false
			return directRestoreRecoveryRequired("direct restore could not clean staging")
		}
		cleanupTmp = false
		if err := writeDirectRestoreIdleJournal(layout, history.Head); err != nil {
			return directRestoreRecoveryRequired("direct restore could not mark journal idle")
		}
		result = RestoreResult{
			RestoredSavePointID: savePointID,
			PreviousHead:        previousHead,
			NewHead:             savePointID,
		}
		return nil
	})
	if err != nil {
		return RestoreResult{}, err
	}
	return result, nil
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
