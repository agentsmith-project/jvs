package worktree_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerForkMaterializesSnapshotIntoMissingStagingDestination(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createManagerSnapshot(t, repoPath)

	var destinations []string
	strictClone := func(src, dst string) error {
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("fork materialize destination was precreated: %s", dst)
		} else if !os.IsNotExist(err) {
			return err
		}
		if _, err := os.Stat(filepath.Dir(dst)); err != nil {
			return fmt.Errorf("fork materialize destination parent missing: %w", err)
		}
		destinations = append(destinations, dst)
		if !strings.Contains(filepath.Base(dst), ".strict-fork.staging-") {
			return fmt.Errorf("fork materialize destination should be owned staging path: %s", dst)
		}
		return copySnapshotTree(src, dst)
	}

	cfg, err := worktree.NewManager(repoPath).Fork(snapshotID, "strict-fork", strictClone)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, destinations, 1)
	assert.FileExists(t, filepath.Join(managedPayloadPath(repoPath, "strict-fork"), "snapshot.txt"))
}
