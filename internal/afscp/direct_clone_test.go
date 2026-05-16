package afscp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectCloneUsesHistoryHeadAndPublishesDirectTarget(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourceHome := filepath.Join(base, "source-home")
	targetControl := filepath.Join(base, "target-control")
	targetHome := filepath.Join(base, "target-home")
	externalWorkspace := filepath.Join(base, "workspace-target")
	require.NoError(t, os.Mkdir(sourceControl, 0755))
	require.NoError(t, os.Mkdir(sourceHome, 0755))
	require.NoError(t, os.Mkdir(externalWorkspace, 0755))
	sourceRepo, err := repo.InitAFSCPDirectControl(sourceControl, sourceHome, "main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("saved"), 0644))
	require.NoError(t, os.Symlink(externalWorkspace, filepath.Join(sourceHome, "workspace")))
	cloneLog := installFakeJuiceFSClone(t)
	save := directTestSave(t, sourceControl, sourceHome, "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("dirty"), 0644))
	require.NoError(t, os.WriteFile(cloneLog, nil, 0644))

	result, err := NewService().Clone(context.Background(), Request{
		Selector:       Selector{ControlRoot: sourceControl, Home: sourceHome},
		TargetSelector: Selector{ControlRoot: targetControl, Home: targetHome},
	})
	require.NoError(t, err)
	clone := result.(CloneResult)
	assert.Equal(t, sourceRepo.RepoID, clone.SourceRepoID)
	assert.NotEmpty(t, clone.TargetRepoID)
	assert.NotEqual(t, sourceRepo.RepoID, clone.TargetRepoID)
	assert.Equal(t, save.SavePointID, clone.SavePointID)
	assert.Equal(t, 1, clone.SavePointsCopiedCount)
	require.Len(t, clone.CloneEvidence, 2)
	assertDirectCloneEvidence(t, clone.CloneEvidence[0], "clone", "clone_target_home")
	assertDirectCloneEvidence(t, clone.CloneEvidence[1], "clone", "clone_target_snapshot")

	assert.Equal(t, "saved", string(directTestReadFile(t, filepath.Join(targetHome, "profile.txt"))))
	assert.NoFileExists(t, filepath.Join(targetHome, "dirty-only.txt"))
	assertTreesEqual(t, filepath.Join(directTestSnapshotDir(sourceControl, save.SavePointID), "payload"), targetHome)
	targetList, err := NewService().List(context.Background(), Request{
		Selector: Selector{ControlRoot: targetControl, Home: targetHome},
	})
	require.NoError(t, err)
	require.NotNil(t, targetList.HistoryHead)
	assert.Equal(t, save.SavePointID, *targetList.HistoryHead)
	targetDoctor, err := NewService().Doctor(context.Background(), Request{
		Selector: Selector{ControlRoot: targetControl, Home: targetHome},
	})
	require.NoError(t, err)
	assert.True(t, targetDoctor.Healthy)
	assert.Equal(t, clone.TargetRepoID, targetDoctor.RepoID)

	cloneArgs := readFakeJuiceFSCloneArgs(t, cloneLog)
	require.Len(t, cloneArgs, 6)
	assert.Equal(t, []string{"clone", filepath.Join(directTestSnapshotDir(sourceControl, save.SavePointID), "payload"), targetHome}, cloneArgs[:3])
	assert.Equal(t, []string{"clone", filepath.Join(directTestSnapshotDir(sourceControl, save.SavePointID), "payload"), filepath.Join(directTestSnapshotDir(targetControl, save.SavePointID), "payload")}, cloneArgs[3:])
}

func TestDirectCloneExplicitSavePointDoesNotReadDirtyHome(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourceHome := filepath.Join(base, "source-home")
	targetControl := filepath.Join(base, "target-control")
	targetHome := filepath.Join(base, "target-home")
	require.NoError(t, os.Mkdir(sourceControl, 0755))
	require.NoError(t, os.Mkdir(sourceHome, 0755))
	_, err := repo.InitAFSCPDirectControl(sourceControl, sourceHome, "main")
	require.NoError(t, err)
	installFakeJuiceFSClone(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("first"), 0644))
	first := directTestSave(t, sourceControl, sourceHome, "first")
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("second"), 0644))
	directTestSave(t, sourceControl, sourceHome, "second")
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("dirty current home must not be cloned"), 0644))

	_, err = NewService().Clone(context.Background(), Request{
		Selector:       Selector{ControlRoot: sourceControl, Home: sourceHome},
		TargetSelector: Selector{ControlRoot: targetControl, Home: targetHome},
		SavePointID:    first.SavePointID,
	})
	require.NoError(t, err)
	assert.Equal(t, "first", string(directTestReadFile(t, filepath.Join(targetHome, "profile.txt"))))
}

func TestDirectCloneFailsFastWhenJuiceFSUnavailableWithoutCopyFallback(t *testing.T) {
	base := t.TempDir()
	sourceControl := filepath.Join(base, "source-control")
	sourceHome := filepath.Join(base, "source-home")
	targetControl := filepath.Join(base, "target-control")
	targetHome := filepath.Join(base, "target-home")
	require.NoError(t, os.Mkdir(sourceControl, 0755))
	require.NoError(t, os.Mkdir(sourceHome, 0755))
	_, err := repo.InitAFSCPDirectControl(sourceControl, sourceHome, "main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sourceHome, "profile.txt"), []byte("saved"), 0644))
	installFakeJuiceFSClone(t)
	directTestSave(t, sourceControl, sourceHome, "baseline")
	t.Setenv("PATH", t.TempDir())

	result, err := NewService().Clone(context.Background(), Request{
		Selector:       Selector{ControlRoot: sourceControl, Home: sourceHome},
		TargetSelector: Selector{ControlRoot: targetControl, Home: targetHome},
	})
	assert.Nil(t, result)
	requireDirectError(t, err, ErrorCodeCloneUnavailable, ExitStorage)
	assert.NoDirExists(t, targetHome)
	assert.NoDirExists(t, targetControl)
}
