package cli

import (
	"fmt"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/worktree"
)

func workspaceDirty(repoRoot, workspaceName string) (bool, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return false, fmt.Errorf("load workspace: %w", err)
	}
	if _, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName); err != nil {
		return false, fmt.Errorf("workspace path: %w", err)
	}

	if cfg.HeadSnapshotID == "" {
		return true, nil
	}
	if len(cfg.PathSources) > 0 {
		return true, nil
	}
	return cfg.HeadSnapshotID != cfg.LatestSnapshotID, nil
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
	_ = boundary
	if cfg.HeadSnapshotID == "" {
		return true, nil
	}
	if len(cfg.PathSources) > 0 || relPath != "" {
		return true, nil
	}
	return cfg.HeadSnapshotID != cfg.LatestSnapshotID, nil
}
