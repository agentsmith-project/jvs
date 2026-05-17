package afscp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	directLayoutName              = "afscp-direct-v1"
	directWorkspaceName           = "main"
	directMetadataUninit          = "uninitialized"
	directMetadataReady           = "ready"
	directMetadataInvalid         = "invalid"
	directBindingFileName         = "binding.json"
	directHistoryFileName         = "history.json"
	directJournalFileName         = "journal.json"
	directMutationLockDirName     = "mutation.lock"
	directPendingCleanupDir       = "pending-cleanups"
	directCleanupMetadataFileName = "cleanup.json"
	directSnapshotsDirName        = "snapshots"
	directTmpDirName              = "tmp"
	directDescriptorFileName      = "descriptor.json"
	directReadyFileName           = "ready"
	directPayloadDirName          = "payload"
	directChecksumPrefix          = "sha256:"
)

const (
	directCleanupKindRestoreBackup  = "restore_backup"
	directCleanupStatePending       = "pending"
	directPendingCleanupFreshWindow = 24 * time.Hour
)

const (
	directProjectionNone  = "none"
	directProjectionClean = "clean"
)

const (
	directJournalPhaseIdle             = "idle"
	directJournalPhaseSaveIntent       = "save_intent"
	directJournalPhaseSaveRunning      = "save_running"
	directJournalPhaseSaveFailed       = "save_failed"
	directJournalPhaseRestoreStaging   = "restore_staging"
	directJournalPhaseRestoreBackingUp = "restore_backing_up"
	directJournalPhaseRestoreReplacing = "restore_replacing"
)

type directLayout struct {
	root      string
	snapshots string
	tmp       string
	binding   string
	history   string
	journal   string
	lock      string
	cleanup   string
	selector  ResolvedSelector
}

type directBinding struct {
	Version   int    `json:"version"`
	Contract  string `json:"contract"`
	Workspace string `json:"workspace"`
	Home      string `json:"home"`
}

type directDescriptor struct {
	Version      int     `json:"version"`
	Contract     string  `json:"contract"`
	Workspace    string  `json:"workspace"`
	SavePointID  string  `json:"save_point_id"`
	CreatedAt    string  `json:"created_at"`
	Message      string  `json:"message,omitempty"`
	PreviousHead *string `json:"previous_head"`
	PayloadState string  `json:"payload_state"`
	Checksum     string  `json:"metadata_checksum,omitempty"`
}

type directReady struct {
	Version            int    `json:"version"`
	Contract           string `json:"contract"`
	Workspace          string `json:"workspace"`
	SavePointID        string `json:"save_point_id"`
	State              string `json:"state"`
	DescriptorChecksum string `json:"descriptor_checksum"`
	CreatedAt          string `json:"created_at"`
	Checksum           string `json:"metadata_checksum,omitempty"`
}

type directHistory struct {
	Version    int                  `json:"version"`
	Contract   string               `json:"contract"`
	Workspace  string               `json:"workspace"`
	Head       *string              `json:"head"`
	SavePoints []directHistoryEntry `json:"save_points"`
}

type directHistoryEntry struct {
	SavePointID string `json:"save_point_id"`
	CreatedAt   string `json:"created_at"`
	Message     string `json:"message,omitempty"`
}

type directJournal struct {
	Version           int    `json:"version"`
	Contract          string `json:"contract"`
	Workspace         string `json:"workspace"`
	Phase             string `json:"phase"`
	LastSavePointID   string `json:"last_save_point_id,omitempty"`
	TargetSavePointID string `json:"target_save_point_id,omitempty"`
	RestoreTmpName    string `json:"restore_tmp_name,omitempty"`
	BackupHomeName    string `json:"backup_home_name,omitempty"`
	FailureCode       string `json:"failure_code,omitempty"`
	Reason            string `json:"reason,omitempty"`
	UpdatedAt         string `json:"updated_at"`
	Checksum          string `json:"metadata_checksum,omitempty"`
}

type directCleanupMetadata struct {
	Version        int    `json:"version"`
	Contract       string `json:"contract"`
	Workspace      string `json:"workspace"`
	Kind           string `json:"kind"`
	State          string `json:"state"`
	SavePointID    string `json:"save_point_id"`
	BackupHomeName string `json:"backup_home_name"`
	CreatedAt      string `json:"created_at"`
	Checksum       string `json:"metadata_checksum,omitempty"`
}

type directStatusProjection struct {
	activeOperation string
	recovery        string
	recoveryReason  string
}

func saveDirect(ctx context.Context, selector ResolvedSelector, message string) (SaveResult, error) {
	layout := newDirectLayout(selector)
	var result SaveResult
	err := withDirectMutationLock(layout, func() error {
		if err := ensureDirectLayout(layout); err != nil {
			return err
		}

		history, err := readDirectHistory(layout)
		if err != nil {
			return err
		}
		createdAt := time.Now().UTC().Format(time.RFC3339Nano)
		savePointID := uuidutil.NewV4()
		if err := writeDirectSaveJournal(layout, directJournalPhaseSaveIntent, history.Head, savePointID, createdAt, "", ""); err != nil {
			return err
		}
		if err := writeDirectSaveJournal(layout, directJournalPhaseSaveRunning, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
			return err
		}

		tmpDir, err := os.MkdirTemp(layout.tmp, "save-"+savePointID+"-")
		if err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "create direct clone staging")
			return NewError(ErrorCodeInternal, "create direct clone staging", false)
		}
		tmpPayload := filepath.Join(tmpDir, directPayloadDirName)
		cleanupTmp := true
		defer func() {
			if cleanupTmp {
				_ = os.RemoveAll(tmpDir)
			}
		}()

		cloneEvidence, err := runStrictJuiceFSCloneWithEvidence(ctx, "save", "save_point_payload", selector.Home, tmpPayload)
		if err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), directJournalFailureCode(err), directJournalFailureReason(err))
			return err
		}
		if err := requireDirectory(tmpPayload); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeCloneFailed), "juicefs clone did not create payload")
			return NewError(ErrorCodeCloneFailed, "juicefs clone did not create payload", false)
		}

		snapshotDir := filepath.Join(layout.snapshots, savePointID)
		if err := os.Mkdir(snapshotDir, 0755); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "create direct snapshot metadata")
			return NewError(ErrorCodeInternal, "create direct snapshot metadata", false)
		}
		if err := fsutil.RenameNoReplaceAndSync(tmpPayload, filepath.Join(snapshotDir, directPayloadDirName)); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "publish direct snapshot payload")
			return NewError(ErrorCodeInternal, "publish direct snapshot payload", false)
		}

		desc := directDescriptor{
			Version:      1,
			Contract:     ContractVersion,
			Workspace:    directWorkspaceName,
			SavePointID:  savePointID,
			CreatedAt:    createdAt,
			Message:      message,
			PreviousHead: history.Head,
			PayloadState: directMetadataReady,
		}
		if err := writeDirectDescriptor(layout, desc); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "write direct descriptor")
			return err
		}
		desc, err = readDirectDescriptor(layout, savePointID)
		if err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeMetadataInvalid), "verify direct descriptor")
			return err
		}
		if err := writeDirectReady(layout, directReady{
			Version:            1,
			Contract:           ContractVersion,
			Workspace:          directWorkspaceName,
			SavePointID:        savePointID,
			State:              directMetadataReady,
			DescriptorChecksum: desc.Checksum,
			CreatedAt:          createdAt,
		}); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "write direct ready marker")
			return err
		}

		history.Head = &savePointID
		history.SavePoints = append(history.SavePoints, directHistoryEntry{
			SavePointID: savePointID,
			CreatedAt:   createdAt,
			Message:     message,
		})
		if err := writeDirectJSON(layout.history, history, 0644); err != nil {
			_ = writeDirectSaveJournal(layout, directJournalPhaseSaveFailed, nil, savePointID, time.Now().UTC().Format(time.RFC3339Nano), string(ErrorCodeInternal), "write direct history")
			return err
		}
		if err := writeDirectSaveJournal(layout, directJournalPhaseIdle, history.Head, savePointID, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
			return err
		}

		cleanupTmp = false
		_ = os.RemoveAll(tmpDir)
		result = SaveResult{
			SavePointID:   savePointID,
			CreatedAt:     createdAt,
			Message:       message,
			HistoryHead:   savePointID,
			CloneEvidence: []CloneEvidence{cloneEvidence},
		}
		return nil
	})
	if err != nil {
		return SaveResult{}, err
	}
	return result, nil
}

func listDirect(selector ResolvedSelector) (ListResult, error) {
	layout, initialized, err := openDirectLayout(selector)
	if err != nil {
		return ListResult{}, err
	}
	if !initialized {
		return ListResult{
			HistoryHead:   nil,
			SavePoints:    []SavePoint{},
			MetadataState: directMetadataUninit,
		}, nil
	}

	history, err := readDirectHistory(layout)
	if err != nil {
		return ListResult{}, err
	}
	savePoints := make([]SavePoint, 0, len(history.SavePoints))
	for _, entry := range history.SavePoints {
		desc, err := readDirectDescriptor(layout, entry.SavePointID)
		if err != nil {
			return ListResult{}, err
		}
		if err := requireDirectReady(layout, entry.SavePointID, desc.Checksum); err != nil {
			return ListResult{}, err
		}
		savePoints = append(savePoints, SavePoint{
			SavePointID: desc.SavePointID,
			CreatedAt:   desc.CreatedAt,
			Message:     desc.Message,
			HistoryHead: history.Head != nil && *history.Head == desc.SavePointID,
		})
	}

	return ListResult{
		HistoryHead:   history.Head,
		SavePoints:    savePoints,
		MetadataState: directMetadataReady,
	}, nil
}

func statusDirect(selector ResolvedSelector) (StatusResult, error) {
	repoID := readDirectRepoID(selector.ControlRoot)
	layout, initialized, err := openDirectLayout(selector)
	if err != nil {
		return StatusResult{}, err
	}
	if !initialized {
		return StatusResult{
			RepoID:          repoID,
			HistoryHead:     nil,
			ActiveOperation: directProjectionNone,
			MetadataState:   directMetadataUninit,
			Recovery:        directProjectionNone,
		}, nil
	}

	history, err := readDirectHistory(layout)
	if err != nil {
		return StatusResult{
			RepoID:          repoID,
			HistoryHead:     nil,
			ActiveOperation: directProjectionNone,
			MetadataState:   directMetadataInvalid,
			Recovery:        "repair_metadata",
		}, nil
	}
	metadataErr := validateDirectSavePointMetadata(layout, history)
	journal, journalErr := readDirectJournal(layout)
	if metadataErr != nil || journalErr != nil {
		return StatusResult{
			RepoID:          repoID,
			HistoryHead:     history.Head,
			ActiveOperation: directProjectionNone,
			MetadataState:   directMetadataInvalid,
			Recovery:        directMetadataRecovery(metadataErr, journalErr).recovery,
		}, nil
	}
	projection := directStatusFromJournal(layout, journal)
	if projection.recovery == directProjectionNone && projection.activeOperation == directProjectionNone {
		cleanupProjection, _, _ := directPendingCleanupProjection(layout)
		if cleanupProjection.recovery != directProjectionNone {
			projection = cleanupProjection
		}
	}
	metadataState := directMetadataReady
	if projection.recovery != directProjectionNone && projection.recovery != "cleanup_pending" {
		metadataState = directMetadataInvalid
	}
	return StatusResult{
		RepoID:          repoID,
		HistoryHead:     history.Head,
		ActiveOperation: projection.activeOperation,
		MetadataState:   metadataState,
		Recovery:        projection.recovery,
	}, nil
}

func doctorDirect(selector ResolvedSelector) (DoctorResult, error) {
	repoID := readDirectRepoID(selector.ControlRoot)
	layout, initialized, err := openDirectLayout(selector)
	if err != nil {
		return DoctorResult{}, err
	}
	if !initialized {
		return DoctorResult{
			Healthy:       true,
			RepoID:        repoID,
			Findings:      []FindingProjection{},
			MetadataState: directMetadataUninit,
			Journal:       directProjectionClean,
			Recovery:      directProjectionNone,
		}, nil
	}

	findings := []FindingProjection{}
	metadataInvalid := false
	history, err := readDirectHistory(layout)
	if err != nil {
		metadataInvalid = true
		findings = append(findings, directMetadataFinding("history metadata is invalid"))
	}
	if err == nil {
		for _, entry := range history.SavePoints {
			desc, descErr := readDirectDescriptor(layout, entry.SavePointID)
			if descErr != nil {
				metadataInvalid = true
				findings = append(findings, directMetadataFinding("descriptor metadata is invalid"))
				continue
			}
			if readyErr := requireDirectReady(layout, entry.SavePointID, desc.Checksum); readyErr != nil {
				metadataInvalid = true
				findings = append(findings, directMetadataFinding("ready metadata is invalid"))
			}
		}
	}
	journal, journalErr := readDirectJournal(layout)
	if journalErr != nil {
		metadataInvalid = true
		findings = append(findings, directMetadataFinding("journal metadata is invalid"))
	}
	journalProjection := directJournalProjection(journal, journalErr)
	projection := directStatusFromJournal(layout, journal)
	if projection.recovery != directProjectionNone {
		findings = append(findings, directJournalRecoveryFinding(projection.recoveryReason))
	}
	if projection.recovery == directProjectionNone && metadataInvalid {
		projection = directMetadataRecovery(err, journalErr)
	}
	if projection.recovery == directProjectionNone && projection.activeOperation == directProjectionNone {
		cleanupProjection, cleanupFinding, hasCleanupFinding := directPendingCleanupProjection(layout)
		if cleanupProjection.recovery != directProjectionNone {
			projection = cleanupProjection
			if hasCleanupFinding {
				findings = append(findings, cleanupFinding)
			}
		}
	}
	state := directMetadataReady
	if directFindingsHaveErrors(findings) {
		state = directMetadataInvalid
	}
	return DoctorResult{
		Healthy:       len(findings) == 0,
		RepoID:        repoID,
		Findings:      findings,
		MetadataState: state,
		Journal:       journalProjection,
		Recovery:      projection.recovery,
	}, nil
}

func newDirectLayout(selector ResolvedSelector) directLayout {
	root := filepath.Join(selector.ControlRoot, directLayoutName)
	return directLayout{
		root:      root,
		snapshots: filepath.Join(root, directSnapshotsDirName),
		tmp:       filepath.Join(root, directTmpDirName),
		binding:   filepath.Join(root, directBindingFileName),
		history:   filepath.Join(root, directHistoryFileName),
		journal:   filepath.Join(root, directJournalFileName),
		lock:      filepath.Join(root, directMutationLockDirName),
		cleanup:   filepath.Join(root, directPendingCleanupDir),
		selector:  selector,
	}
}

func openDirectLayout(selector ResolvedSelector) (directLayout, bool, error) {
	layout := newDirectLayout(selector)
	info, err := os.Lstat(layout.root)
	if err != nil {
		if os.IsNotExist(err) {
			return layout, false, nil
		}
		return layout, false, NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
	}
	if !info.IsDir() {
		return layout, false, NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
	}
	if err := validateDirectBinding(layout); err != nil {
		return layout, false, err
	}
	return layout, true, nil
}

func ensureDirectLayout(layout directLayout) error {
	if err := os.MkdirAll(layout.root, 0755); err != nil {
		return NewError(ErrorCodeInternal, "create direct metadata layout", false)
	}
	if err := ensureDirectBinding(layout); err != nil {
		return err
	}
	for _, dir := range []string{layout.snapshots, layout.tmp, layout.cleanup} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return NewError(ErrorCodeInternal, "create direct metadata layout", false)
		}
	}
	return nil
}

func ensureDirectLayoutRoot(layout directLayout) error {
	if err := os.MkdirAll(layout.root, 0755); err != nil {
		return NewError(ErrorCodeInternal, "create direct metadata layout", false)
	}
	return nil
}

func ensureDirectBinding(layout directLayout) error {
	info, err := os.Lstat(layout.binding)
	if err != nil {
		if !os.IsNotExist(err) {
			return NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
		}
		return writeDirectJSON(layout.binding, directBinding{
			Version:   1,
			Contract:  ContractVersion,
			Workspace: directWorkspaceName,
			Home:      layout.selector.Home,
		}, 0644)
	}
	if info.IsDir() {
		return NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
	}
	return validateDirectBinding(layout)
}

type directMutationLock struct {
	path string
}

func withDirectMutationLock(layout directLayout, run func() error) error {
	lock, err := acquireDirectMutationLock(layout)
	if err != nil {
		return err
	}
	defer lock.release()
	return run()
}

func acquireDirectMutationLock(layout directLayout) (*directMutationLock, error) {
	if err := ensureDirectLayoutRoot(layout); err != nil {
		return nil, err
	}
	if err := os.Mkdir(layout.lock, 0700); err != nil {
		if os.IsExist(err) {
			return nil, NewError(ErrorCodeLocked, "direct mutation is already running", true)
		}
		return nil, NewError(ErrorCodeInternal, "acquire direct mutation lock", false)
	}
	return &directMutationLock{path: layout.lock}, nil
}

func (lock *directMutationLock) release() {
	if lock == nil || lock.path == "" {
		return
	}
	_ = os.Remove(lock.path)
}

func directMutationLockBusy(layout directLayout) bool {
	if err := os.Mkdir(layout.lock, 0700); err != nil {
		return os.IsExist(err)
	}
	_ = os.Remove(layout.lock)
	return false
}

func validateDirectBinding(layout directLayout) error {
	var binding directBinding
	if err := readDirectJSON(layout.binding, &binding); err != nil {
		return NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
	}
	if binding.Version != 1 ||
		binding.Contract != ContractVersion ||
		binding.Workspace != directWorkspaceName ||
		binding.Home == "" {
		return NewError(ErrorCodeMetadataInvalid, "direct metadata is invalid", false)
	}
	if filepath.Clean(binding.Home) != layout.selector.Home {
		return NewError(ErrorCodeMetadataInvalid, "direct metadata binding does not match selector", false)
	}
	return nil
}

func readDirectHistory(layout directLayout) (directHistory, error) {
	history := emptyDirectHistory()
	err := readDirectJSON(layout.history, &history)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyDirectHistory(), nil
		}
		return directHistory{}, NewError(ErrorCodeMetadataInvalid, "direct history metadata is invalid", false)
	}
	if history.Version != 1 || history.Contract != ContractVersion || history.Workspace != directWorkspaceName {
		return directHistory{}, NewError(ErrorCodeMetadataInvalid, "direct history metadata is invalid", false)
	}
	seen := map[string]struct{}{}
	for _, entry := range history.SavePoints {
		if !validSavePointID(entry.SavePointID) {
			return directHistory{}, NewError(ErrorCodeMetadataInvalid, "direct history metadata is invalid", false)
		}
		if _, ok := seen[entry.SavePointID]; ok {
			return directHistory{}, NewError(ErrorCodeMetadataInvalid, "direct history metadata is invalid", false)
		}
		seen[entry.SavePointID] = struct{}{}
	}
	if history.Head != nil {
		if _, ok := seen[*history.Head]; !ok {
			return directHistory{}, NewError(ErrorCodeMetadataInvalid, "direct history metadata is invalid", false)
		}
	}
	if history.SavePoints == nil {
		history.SavePoints = []directHistoryEntry{}
	}
	return history, nil
}

func emptyDirectHistory() directHistory {
	return directHistory{
		Version:    1,
		Contract:   ContractVersion,
		Workspace:  directWorkspaceName,
		Head:       nil,
		SavePoints: []directHistoryEntry{},
	}
}

func writeDirectSaveJournal(layout directLayout, phase string, historyHead *string, savePointID, updatedAt, failureCode, reason string) error {
	last := ""
	if historyHead != nil {
		last = *historyHead
	}
	return writeDirectJournal(layout, directJournal{
		Version:           1,
		Contract:          ContractVersion,
		Workspace:         directWorkspaceName,
		Phase:             phase,
		LastSavePointID:   last,
		TargetSavePointID: savePointID,
		FailureCode:       failureCode,
		Reason:            reason,
		UpdatedAt:         updatedAt,
	})
}

func directJournalFailureCode(err error) string {
	var directErr *Error
	if errors.As(err, &directErr) {
		return string(directErr.Code)
	}
	return string(ErrorCodeInternal)
}

func directJournalFailureReason(err error) string {
	var directErr *Error
	if errors.As(err, &directErr) && directErr.Message != "" {
		return directErr.Message
	}
	return "direct operation failed"
}

func readDirectDescriptor(layout directLayout, savePointID string) (directDescriptor, error) {
	var desc directDescriptor
	if !validSavePointID(savePointID) {
		return directDescriptor{}, NewError(ErrorCodeMetadataInvalid, "direct descriptor metadata is invalid", false)
	}
	if err := readDirectJSON(filepath.Join(layout.snapshots, savePointID, directDescriptorFileName), &desc); err != nil {
		return directDescriptor{}, NewError(ErrorCodeMetadataInvalid, "direct descriptor metadata is invalid", false)
	}
	if !directDescriptorChecksumValid(desc) {
		return directDescriptor{}, NewError(ErrorCodeMetadataInvalid, "direct descriptor metadata is invalid", false)
	}
	if desc.Version != 1 ||
		desc.Contract != ContractVersion ||
		desc.Workspace != directWorkspaceName ||
		desc.SavePointID != savePointID ||
		desc.PayloadState != directMetadataReady {
		return directDescriptor{}, NewError(ErrorCodeMetadataInvalid, "direct descriptor metadata is invalid", false)
	}
	return desc, nil
}

func writeDirectDescriptor(layout directLayout, desc directDescriptor) error {
	checksum, err := directDescriptorChecksum(desc)
	if err != nil {
		return err
	}
	desc.Checksum = checksum
	return writeDirectJSON(filepath.Join(layout.snapshots, desc.SavePointID, directDescriptorFileName), desc, 0644)
}

func requireDirectReady(layout directLayout, savePointID, descriptorChecksum string) error {
	_, err := readDirectReady(layout, savePointID, descriptorChecksum)
	return err
}

func readDirectReady(layout directLayout, savePointID, descriptorChecksum string) (directReady, error) {
	if !validSavePointID(savePointID) {
		return directReady{}, NewError(ErrorCodeMetadataInvalid, "direct ready metadata is invalid", false)
	}
	var ready directReady
	if err := readDirectJSON(filepath.Join(layout.snapshots, savePointID, directReadyFileName), &ready); err != nil {
		return directReady{}, NewError(ErrorCodeMetadataInvalid, "direct ready metadata is invalid", false)
	}
	if !directReadyChecksumValid(ready) {
		return directReady{}, NewError(ErrorCodeMetadataInvalid, "direct ready metadata is invalid", false)
	}
	if ready.Version != 1 ||
		ready.Contract != ContractVersion ||
		ready.Workspace != directWorkspaceName ||
		ready.SavePointID != savePointID ||
		ready.State != directMetadataReady ||
		ready.DescriptorChecksum == "" ||
		ready.DescriptorChecksum != descriptorChecksum {
		return directReady{}, NewError(ErrorCodeMetadataInvalid, "direct ready metadata is invalid", false)
	}
	return ready, nil
}

func writeDirectReady(layout directLayout, ready directReady) error {
	checksum, err := directReadyChecksum(ready)
	if err != nil {
		return err
	}
	ready.Checksum = checksum
	return writeDirectJSON(filepath.Join(layout.snapshots, ready.SavePointID, directReadyFileName), ready, 0644)
}

func readDirectJournal(layout directLayout) (*directJournal, error) {
	var journal directJournal
	if err := readDirectJSON(layout.journal, &journal); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, NewError(ErrorCodeMetadataInvalid, "direct journal metadata is invalid", false)
	}
	if !directJournalChecksumValid(journal) ||
		journal.Version != 1 ||
		journal.Contract != ContractVersion ||
		journal.Workspace != directWorkspaceName ||
		!validDirectJournalPhase(journal.Phase) {
		return nil, NewError(ErrorCodeMetadataInvalid, "direct journal metadata is invalid", false)
	}
	return &journal, nil
}

func writeDirectJournal(layout directLayout, journal directJournal) error {
	checksum, err := directJournalChecksum(journal)
	if err != nil {
		return err
	}
	journal.Checksum = checksum
	return writeDirectJSON(layout.journal, journal, 0644)
}

func readDirectCleanupMetadata(layout directLayout, markerPath string) (directCleanupMetadata, error) {
	var cleanup directCleanupMetadata
	if err := readDirectJSON(filepath.Join(markerPath, directCleanupMetadataFileName), &cleanup); err != nil {
		return directCleanupMetadata{}, err
	}
	if !directCleanupChecksumValid(cleanup) ||
		cleanup.Version != 1 ||
		cleanup.Contract != ContractVersion ||
		cleanup.Workspace != directWorkspaceName ||
		cleanup.Kind != directCleanupKindRestoreBackup ||
		cleanup.State != directCleanupStatePending ||
		!validSavePointID(cleanup.SavePointID) ||
		!validDirectRestoreBackupName(cleanup.BackupHomeName) {
		return directCleanupMetadata{}, NewError(ErrorCodeMetadataInvalid, "direct restore cleanup metadata is invalid", false)
	}
	return cleanup, nil
}

func writeDirectCleanupMetadata(markerPath string, cleanup directCleanupMetadata) error {
	checksum, err := directCleanupChecksum(cleanup)
	if err != nil {
		return err
	}
	cleanup.Checksum = checksum
	return writeDirectJSON(filepath.Join(markerPath, directCleanupMetadataFileName), cleanup, 0600)
}

func directDescriptorChecksum(desc directDescriptor) (string, error) {
	desc.Checksum = ""
	return directChecksumForMetadata(desc)
}

func directDescriptorChecksumValid(desc directDescriptor) bool {
	if desc.Checksum == "" {
		return false
	}
	checksum, err := directDescriptorChecksum(desc)
	return err == nil && checksum == desc.Checksum
}

func directReadyChecksum(ready directReady) (string, error) {
	ready.Checksum = ""
	return directChecksumForMetadata(ready)
}

func directReadyChecksumValid(ready directReady) bool {
	if ready.Checksum == "" {
		return false
	}
	checksum, err := directReadyChecksum(ready)
	return err == nil && checksum == ready.Checksum
}

func directJournalChecksum(journal directJournal) (string, error) {
	journal.Checksum = ""
	return directChecksumForMetadata(journal)
}

func directJournalChecksumValid(journal directJournal) bool {
	if journal.Checksum == "" {
		return false
	}
	checksum, err := directJournalChecksum(journal)
	return err == nil && checksum == journal.Checksum
}

func directCleanupChecksum(cleanup directCleanupMetadata) (string, error) {
	cleanup.Checksum = ""
	return directChecksumForMetadata(cleanup)
}

func directCleanupChecksumValid(cleanup directCleanupMetadata) bool {
	if cleanup.Checksum == "" {
		return false
	}
	checksum, err := directCleanupChecksum(cleanup)
	return err == nil && checksum == cleanup.Checksum
}

func directChecksumForMetadata(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", NewError(ErrorCodeInternal, "checksum direct metadata", false)
	}
	sum := sha256.Sum256(data)
	return directChecksumPrefix + hex.EncodeToString(sum[:]), nil
}

func validDirectJournalPhase(phase string) bool {
	switch phase {
	case directJournalPhaseIdle,
		directJournalPhaseSaveIntent,
		directJournalPhaseSaveRunning,
		directJournalPhaseSaveFailed,
		directJournalPhaseRestoreStaging,
		directJournalPhaseRestoreBackingUp,
		directJournalPhaseRestoreReplacing:
		return true
	default:
		return false
	}
}

func runStrictJuiceFSCloneWithEvidence(ctx context.Context, operation, phase, src, dst string) (CloneEvidence, error) {
	if err := validateContext(ctx); err != nil {
		return CloneEvidence{}, err
	}
	startedAt := time.Now().UTC()
	cmd := exec.CommandContext(ctx, "juicefs", "clone", src, dst)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return CloneEvidence{}, NewError(ErrorCodeCloneUnavailable, "juicefs clone is unavailable", false)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CloneEvidence{}, NewError(ErrorCodeInternal, "operation canceled", false)
		}
		return CloneEvidence{}, NewError(ErrorCodeCloneFailed, "juicefs clone failed", false)
	}
	finishedAt := time.Now().UTC()
	return CloneEvidence{
		Operation:  operation,
		Phase:      phase,
		Engine:     "juicefs_clone",
		Status:     string(StatusSucceeded),
		StartedAt:  startedAt.Format(time.RFC3339Nano),
		FinishedAt: finishedAt.Format(time.RFC3339Nano),
		DurationMs: finishedAt.Sub(startedAt).Milliseconds(),
	}, nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	return nil
}

func writeDirectJSON(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return NewError(ErrorCodeInternal, "write direct metadata", false)
	}
	data = append(data, '\n')
	if err := fsutil.AtomicWrite(path, data, perm); err != nil {
		return NewError(ErrorCodeInternal, "write direct metadata", false)
	}
	return nil
}

func readDirectJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return err
	}
	return nil
}

func validateDirectSavePointMetadata(layout directLayout, history directHistory) error {
	for _, entry := range history.SavePoints {
		desc, err := readDirectDescriptor(layout, entry.SavePointID)
		if err != nil {
			return err
		}
		if err := requireDirectReady(layout, entry.SavePointID, desc.Checksum); err != nil {
			return err
		}
	}
	return nil
}

func readDirectRepoID(controlRoot string) string {
	data, err := os.ReadFile(filepath.Join(controlRoot, repo.JVSDirName, repo.RepoIDFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func directJournalProjection(journal *directJournal, journalErr error) string {
	if journalErr != nil {
		return directMetadataInvalid
	}
	if journal == nil || journal.Phase == "" || journal.Phase == directJournalPhaseIdle {
		return directProjectionClean
	}
	return journal.Phase
}

func directStatusFromJournal(layout directLayout, journal *directJournal) directStatusProjection {
	if journal == nil || journal.Phase == "" || journal.Phase == directJournalPhaseIdle {
		return directStatusProjection{
			activeOperation: directProjectionNone,
			recovery:        directProjectionNone,
		}
	}
	if journal.Phase == directJournalPhaseSaveFailed {
		return directStatusProjection{
			activeOperation: string(StatusFailed),
			recovery:        directProjectionNone,
		}
	}
	if directMutationLockBusy(layout) {
		return directStatusProjection{
			activeOperation: string(StatusRunning),
			recovery:        directProjectionNone,
		}
	}
	return directStatusProjection{
		activeOperation: string(StatusRecoveryRequired),
		recovery:        "recover_journal",
		recoveryReason:  directJournalRecoveryReason(journal),
	}
}

func directMetadataRecovery(metadataErr, journalErr error) directStatusProjection {
	reason := "direct metadata is invalid"
	if metadataErr != nil {
		reason = directJournalFailureReason(metadataErr)
	}
	if journalErr != nil {
		reason = directJournalFailureReason(journalErr)
	}
	return directStatusProjection{
		activeOperation: directProjectionNone,
		recovery:        "repair_metadata",
		recoveryReason:  reason,
	}
}

func directPendingCleanupProjection(layout directLayout) (directStatusProjection, FindingProjection, bool) {
	hasPendingCleanup, cleanupIssue := inspectDirectPendingCleanup(layout)
	if cleanupIssue != "" {
		return directCleanupMetadataRecovery(cleanupIssue), directCleanupIssueFinding(cleanupIssue), true
	}
	if hasPendingCleanup {
		return directCleanupPendingRecovery(), directCleanupPendingFinding(), true
	}
	return directStatusProjection{
		activeOperation: directProjectionNone,
		recovery:        directProjectionNone,
	}, FindingProjection{}, false
}

func inspectDirectPendingCleanup(layout directLayout) (bool, string) {
	entries, err := os.ReadDir(layout.cleanup)
	if err != nil && !os.IsNotExist(err) {
		return false, "non-convergent"
	}
	referencedBackups := map[string]struct{}{}
	hasPendingCleanup := false
	now := time.Now().UTC()
	for _, entry := range entries {
		if !entry.IsDir() {
			return false, "non-convergent"
		}
		cleanup, issue := inspectDirectPendingCleanupMarker(layout, filepath.Join(layout.cleanup, entry.Name()), now)
		if issue != "" {
			return false, issue
		}
		referencedBackups[cleanup.BackupHomeName] = struct{}{}
		hasPendingCleanup = true
	}
	if issue := inspectDirectUnreferencedBackupSiblings(layout, referencedBackups); issue != "" {
		return false, issue
	}
	return hasPendingCleanup, ""
}

func inspectDirectPendingCleanupMarker(layout directLayout, markerPath string, now time.Time) (directCleanupMetadata, string) {
	cleanup, err := readDirectCleanupMetadata(layout, markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return directCleanupMetadata{}, "unreferenced"
		}
		return directCleanupMetadata{}, "non-convergent"
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cleanup.CreatedAt)
	if err != nil {
		return directCleanupMetadata{}, "non-convergent"
	}
	if now.Sub(createdAt) > directPendingCleanupFreshWindow {
		return directCleanupMetadata{}, "stale"
	}
	if err := requireRealDirectory(filepath.Join(filepath.Dir(layout.selector.Home), cleanup.BackupHomeName)); err != nil {
		return directCleanupMetadata{}, "non-convergent"
	}
	return cleanup, ""
}

func inspectDirectUnreferencedBackupSiblings(layout directLayout, referencedBackups map[string]struct{}) string {
	entries, err := os.ReadDir(filepath.Dir(layout.selector.Home))
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, directRestoreBackupPrefix) {
			continue
		}
		if !entry.IsDir() {
			return "non-convergent"
		}
		if _, ok := referencedBackups[name]; !ok {
			return "unreferenced"
		}
	}
	return ""
}

func validDirectRestoreBackupName(name string) bool {
	return len(name) > len(directRestoreBackupPrefix) &&
		strings.HasPrefix(name, directRestoreBackupPrefix) &&
		filepath.Base(name) == name
}

func directCleanupPendingRecovery() directStatusProjection {
	return directStatusProjection{
		activeOperation: directProjectionNone,
		recovery:        "cleanup_pending",
		recoveryReason:  "direct restore cleanup pending",
	}
}

func directCleanupMetadataRecovery(issue string) directStatusProjection {
	return directStatusProjection{
		activeOperation: directProjectionNone,
		recovery:        "repair_metadata",
		recoveryReason:  "direct restore cleanup " + issue,
	}
}

func directJournalRecoveryReason(journal *directJournal) string {
	if journal == nil {
		return "direct journal recovery is required"
	}
	switch journal.Phase {
	case directJournalPhaseRestoreStaging, directJournalPhaseRestoreBackingUp, directJournalPhaseRestoreReplacing:
		return "direct restore recovery is required"
	case directJournalPhaseSaveIntent, directJournalPhaseSaveRunning:
		return "direct save recovery is required"
	default:
		return "direct journal recovery is required"
	}
}

func directMetadataFinding(message string) FindingProjection {
	return FindingProjection{
		Code:      ErrorCodeMetadataInvalid,
		Severity:  "error",
		Message:   message,
		Retryable: false,
	}
}

func directJournalRecoveryFinding(message string) FindingProjection {
	return FindingProjection{
		Code:      ErrorCodeJournalRecoveryRequired,
		Severity:  "error",
		Message:   message,
		Retryable: false,
	}
}

func directCleanupPendingFinding() FindingProjection {
	return FindingProjection{
		Severity:  "warning",
		Message:   "direct restore cleanup pending",
		Retryable: false,
	}
}

func directCleanupIssueFinding(issue string) FindingProjection {
	return FindingProjection{
		Code:      ErrorCodeMetadataInvalid,
		Severity:  "error",
		Message:   "direct restore cleanup " + issue,
		Retryable: false,
	}
}

func directFindingsHaveErrors(findings []FindingProjection) bool {
	for _, finding := range findings {
		if finding.Severity == "error" {
			return true
		}
	}
	return false
}
