package cli

import (
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
)

const metadataFloor = 1 << 20

func checkSaveCapacity(repoRoot, workspaceName string) error {
	return nil
}

func checkRestorePreviewPreDirtyCapacity(repoRoot, workspaceName string, sourceID model.SnapshotID, path string) error {
	return nil
}

func checkRestoreRunCapacity(repoRoot, workspaceName string, plan *restoreplan.Plan, snapshotDir string, desc *model.Descriptor) error {
	return nil
}

func checkWorkspaceNewCapacity(repoRoot string, req worktree.StartedFromSnapshotRequest) error {
	return nil
}
