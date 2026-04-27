package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/workspacepath"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
)

func workspaceDirty(repoRoot, workspaceName string) (bool, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return false, fmt.Errorf("load workspace: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return false, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return false, err
	}

	if cfg.HeadSnapshotID == "" {
		return workspaceHasManagedContent(boundary)
	}

	if len(cfg.PathSources) > 0 {
		expectedRoot, cleanup, err := workspacepath.MaterializeExpectedWorkspace(repoRoot, cfg, boundary)
		if err != nil {
			return false, err
		}
		defer cleanup()
		matches, err := workspacepath.ManagedPathEqual(boundary.Root, expectedRoot, "", boundary.ExcludesRelativePath)
		if err != nil {
			return false, fmt.Errorf("compare workspace to known sources: %w", err)
		}
		return !matches, nil
	}

	desc, err := snapshot.LoadDescriptor(repoRoot, cfg.HeadSnapshotID)
	if err != nil {
		return false, fmt.Errorf("load current checkpoint: %w", err)
	}
	if len(desc.PartialPaths) > 0 {
		return true, nil
	}

	hash, err := integrity.ComputePayloadRootHashWithExclusions(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return false, fmt.Errorf("hash workspace: %w", err)
	}
	return hash != desc.PayloadRootHash, nil
}

func workspacePathDirty(repoRoot, workspaceName, relPath string) (bool, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return false, fmt.Errorf("load workspace: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return false, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return false, err
	}

	expectedRoot, cleanup, err := workspacepath.MaterializeExpectedWorkspace(repoRoot, cfg, boundary)
	if err != nil {
		return false, err
	}
	defer cleanup()

	matches, err := workspacepath.ManagedPathEqual(boundary.Root, expectedRoot, relPath, boundary.ExcludesRelativePath)
	if err != nil {
		return false, fmt.Errorf("compare path to known source: %w", err)
	}
	return !matches, nil
}

func workspaceHasManagedContent(boundary repo.WorktreePayloadBoundary) (bool, error) {
	hasContent := false
	err := filepath.Walk(boundary.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == boundary.Root {
			return nil
		}
		rel, err := filepath.Rel(boundary.Root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		if boundary.ExcludesRelativePath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hasContent = true
		return filepath.SkipDir
	})
	if err != nil {
		return false, fmt.Errorf("scan workspace: %w", err)
	}
	return hasContent, nil
}

func rejectDirtyWorkspace(repoRoot, workspaceName, operation string, discardDirty, includeWorking bool) error {
	if discardDirty && includeWorking {
		return fmt.Errorf("--discard-dirty and --include-working cannot be used together")
	}
	dirty, err := workspaceDirty(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if !dirty || discardDirty || includeWorking {
		return nil
	}
	return fmt.Errorf("workspace has dirty changes; run jvs checkpoint, or retry %s with --include-working or --discard-dirty", operation)
}

func statusRecoveryHints(current, latest model.SnapshotID, dirty bool) []string {
	var hints []string
	if dirty {
		hints = append(hints, "run jvs checkpoint to save working changes")
		hints = append(hints, "use --include-working to checkpoint changes before the operation")
		hints = append(hints, "use --discard-dirty to discard working changes for the operation")
	}
	if current != "" && latest != "" && current != latest {
		hints = append(hints, "run jvs restore latest to return to the latest checkpoint")
	}
	if current == "" && latest == "" {
		hints = append(hints, "run jvs checkpoint to create the first checkpoint")
	}
	return hints
}
