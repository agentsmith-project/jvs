package afscp

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
)

func cloneDirect(ctx context.Context, source, target ResolvedSelector, savePointID string) (CloneResult, error) {
	layout, initialized, err := openDirectLayout(source)
	if err != nil {
		return CloneResult{}, err
	}
	if !initialized {
		return CloneResult{}, NewError(ErrorCodeSavePointNotFound, "save point not found", false)
	}
	history, snapshotPayload, selected, err := selectDirectCloneSnapshot(layout, savePointID)
	if err != nil {
		return CloneResult{}, err
	}
	_ = history

	cloneTargetHomeEvidence, err := runStrictJuiceFSCloneWithEvidence(ctx, "clone", "clone_target_home", snapshotPayload, target.Home)
	if err != nil {
		return CloneResult{}, err
	}
	cleanupTarget := true
	defer func() {
		if cleanupTarget {
			_ = os.RemoveAll(target.Home)
			_ = os.RemoveAll(target.ControlRoot)
		}
	}()
	if err := requireRealDirectory(target.Home); err != nil {
		return CloneResult{}, NewError(ErrorCodeCloneFailed, "juicefs clone did not create target home", false)
	}

	targetRepo, err := repo.InitAFSCPDirectControl(target.ControlRoot, target.Home, directWorkspaceName)
	if err != nil {
		return CloneResult{}, NewError(ErrorCodeInvalidArgument, "initialize direct target control", false)
	}
	cloneTargetSnapshotEvidence, err := publishDirectCloneMetadata(ctx, snapshotPayload, target, selected)
	if err != nil {
		return CloneResult{}, err
	}
	cleanupTarget = false

	return CloneResult{
		SourceRepoID:          readDirectRepoID(source.ControlRoot),
		TargetRepoID:          targetRepo.RepoID,
		SavePointID:           selected.SavePointID,
		SavePointsCopiedCount: 1,
		CloneEvidence: []CloneEvidence{
			cloneTargetHomeEvidence,
			cloneTargetSnapshotEvidence,
		},
	}, nil
}

func selectDirectCloneSnapshot(layout directLayout, savePointID string) (directHistory, string, directDescriptor, error) {
	history, err := readDirectHistory(layout)
	if err != nil {
		return directHistory{}, "", directDescriptor{}, err
	}
	if savePointID == "" {
		if history.Head == nil || *history.Head == "" {
			return directHistory{}, "", directDescriptor{}, NewError(ErrorCodeSavePointNotFound, "save point not found", false)
		}
		savePointID = *history.Head
	}
	if !directHistoryContains(history, savePointID) {
		return directHistory{}, "", directDescriptor{}, NewError(ErrorCodeSavePointNotFound, "save point not found", false)
	}
	desc, err := readDirectDescriptor(layout, savePointID)
	if err != nil {
		return directHistory{}, "", directDescriptor{}, err
	}
	if err := requireDirectReady(layout, savePointID, desc.Checksum); err != nil {
		return directHistory{}, "", directDescriptor{}, err
	}
	if err := requireDirectJournalAllowsRestore(layout); err != nil {
		return directHistory{}, "", directDescriptor{}, err
	}
	snapshotPayload := filepath.Join(layout.snapshots, savePointID, directPayloadDirName)
	if err := requireRealDirectory(snapshotPayload); err != nil {
		return directHistory{}, "", directDescriptor{}, NewError(ErrorCodeMetadataInvalid, "direct snapshot payload metadata is invalid", false)
	}
	return history, snapshotPayload, desc, nil
}

func publishDirectCloneMetadata(ctx context.Context, snapshotPayload string, target ResolvedSelector, source directDescriptor) (CloneEvidence, error) {
	layout := newDirectLayout(target)
	if err := ensureDirectLayout(layout); err != nil {
		return CloneEvidence{}, err
	}
	snapshotDir := filepath.Join(layout.snapshots, source.SavePointID)
	if err := os.Mkdir(snapshotDir, 0755); err != nil {
		return CloneEvidence{}, NewError(ErrorCodeInternal, "create direct clone snapshot metadata", false)
	}
	cloneEvidence, err := runStrictJuiceFSCloneWithEvidence(ctx, "clone", "clone_target_snapshot", snapshotPayload, filepath.Join(snapshotDir, directPayloadDirName))
	if err != nil {
		return CloneEvidence{}, err
	}
	if err := requireRealDirectory(filepath.Join(snapshotDir, directPayloadDirName)); err != nil {
		return CloneEvidence{}, NewError(ErrorCodeCloneFailed, "juicefs clone did not create direct clone snapshot", false)
	}
	desc := directDescriptor{
		Version:      1,
		Contract:     ContractVersion,
		Workspace:    directWorkspaceName,
		SavePointID:  source.SavePointID,
		CreatedAt:    source.CreatedAt,
		Message:      source.Message,
		Purpose:      source.Purpose,
		PreviousHead: nil,
		PayloadState: directMetadataReady,
	}
	if err := writeDirectDescriptor(layout, desc); err != nil {
		return CloneEvidence{}, err
	}
	desc, err = readDirectDescriptor(layout, source.SavePointID)
	if err != nil {
		return CloneEvidence{}, err
	}
	if err := writeDirectReady(layout, directReady{
		Version:            1,
		Contract:           ContractVersion,
		Workspace:          directWorkspaceName,
		SavePointID:        source.SavePointID,
		State:              directMetadataReady,
		DescriptorChecksum: desc.Checksum,
		CreatedAt:          source.CreatedAt,
	}); err != nil {
		return CloneEvidence{}, err
	}
	head := source.SavePointID
	if err := writeDirectJSON(layout.history, directHistory{
		Version:   1,
		Contract:  ContractVersion,
		Workspace: directWorkspaceName,
		Head:      &head,
		SavePoints: []directHistoryEntry{{
			SavePointID: source.SavePointID,
			CreatedAt:   source.CreatedAt,
			Message:     source.Message,
			Purpose:     source.Purpose,
		}},
	}, 0644); err != nil {
		return CloneEvidence{}, err
	}
	if err := writeDirectSaveJournal(layout, directJournalPhaseIdle, &head, source.SavePointID, time.Now().UTC().Format(time.RFC3339Nano), "", ""); err != nil {
		return CloneEvidence{}, err
	}
	return cloneEvidence, nil
}
