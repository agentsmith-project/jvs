package repoclone

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedCloneRollbackDoesNotRemoveLateExternalWrite(t *testing.T) {
	targetRoot, published := publishRollbackTestPayload(t)
	installRollbackBeforeRemoveHook(t, func(root string) error {
		require.Equal(t, targetRoot, root)
		return os.WriteFile(filepath.Join(root, "external.txt"), []byte("external write"), 0644)
	})

	err := rollbackSeparatedPublishedRoot(published)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "target folder changed after publish; refusing to remove")
	assert.Contains(t, err.Error(), "inspect and remove it manually")
	assertFileContentInternal(t, filepath.Join(targetRoot, "external.txt"), "external write")
	assertFileContentInternal(t, filepath.Join(targetRoot, "app.txt"), "main v1")
	assertNoRollbackQuarantineInternal(t, targetRoot)
}

func TestSeparatedCloneRollbackDoesNotRemoveLateRootReplacement(t *testing.T) {
	targetRoot, published := publishRollbackTestPayload(t)
	installRollbackBeforeRemoveHook(t, func(root string) error {
		require.Equal(t, targetRoot, root)
		if err := os.RemoveAll(root); err != nil {
			return err
		}
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, "external.txt"), []byte("external replacement"), 0644)
	})

	err := rollbackSeparatedPublishedRoot(published)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "target folder changed after publish; refusing to remove")
	assert.Contains(t, err.Error(), "inspect and remove it manually")
	assertFileContentInternal(t, filepath.Join(targetRoot, "external.txt"), "external replacement")
	assert.NoFileExists(t, filepath.Join(targetRoot, "app.txt"))
	assertNoRollbackQuarantineInternal(t, targetRoot)
}

func TestSeparatedClonePublishDoesNotRemoveLatePreexistingTargetReplacement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replace func(t *testing.T, path string)
		assert  func(t *testing.T, path string)
	}{
		{
			name: "symlink",
			replace: func(t *testing.T, path string) {
				replacementTarget := filepath.Join(filepath.Dir(path), "external-real")
				require.NoError(t, os.MkdirAll(replacementTarget, 0755))
				require.NoError(t, os.Remove(path))
				if err := os.Symlink(replacementTarget, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
			assert: func(t *testing.T, path string) {
				info, err := os.Lstat(path)
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			},
		},
		{
			name: "file",
			replace: func(t *testing.T, path string) {
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.WriteFile(path, []byte("external file\n"), 0644))
			},
			assert: func(t *testing.T, path string) {
				assertFileContentInternal(t, path, "external file\n")
			},
		},
		{
			name: "non-empty directory",
			replace: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(filepath.Join(path, "external.txt"), []byte("external directory\n"), 0644))
			},
			assert: func(t *testing.T, path string) {
				assertFileContentInternal(t, filepath.Join(path, "external.txt"), "external directory\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceControl, sourcePayload := setupSeparatedCloneSourceRepoInternal(t)
			require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
			_ = createCloneSavePointInternal(t, sourceControl, "main", "main baseline")

			targetBase := t.TempDir()
			targetControl := filepath.Join(targetBase, "target-control")
			targetPayload := filepath.Join(targetBase, "target-payload")
			require.NoError(t, os.MkdirAll(targetControl, 0755))
			require.NoError(t, os.MkdirAll(targetPayload, 0755))
			hookCalled := false
			installBeforeRemoveEmptyTargetRootHook(t, func(root, role string) error {
				if root != targetPayload || role != "payload" {
					return nil
				}
				hookCalled = true
				tc.replace(t, root)
				return nil
			})

			_, err := Clone(Options{
				SourceRepoRoot:    sourceControl,
				TargetControlRoot: targetControl,
				TargetPayloadRoot: targetPayload,
				SavePointsMode:    SavePointsModeMain,
				RequestedEngine:   model.EngineCopy,
			})

			require.True(t, hookCalled, "test must replace the target after empty-root validation and before remove")
			assertJVSErrorCodeInternal(t, err, errclass.ErrAtomicPublishBlocked.Code)
			assert.Contains(t, err.Error(), "changed before publish; refusing to remove")
			tc.assert(t, targetPayload)
			assert.NoDirExists(t, filepath.Join(targetControl, repo.JVSDirName))
		})
	}
}

func TestSeparatedClonePublishDoesNotRemoveTargetReplacedAfterEmptyRevalidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replace func(t *testing.T, path string)
		assert  func(t *testing.T, path string)
	}{
		{
			name: "symlink",
			replace: func(t *testing.T, path string) {
				replacementTarget := filepath.Join(filepath.Dir(path), "external-real-after-revalidate")
				require.NoError(t, os.MkdirAll(replacementTarget, 0755))
				require.NoError(t, os.Remove(path))
				if err := os.Symlink(replacementTarget, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
			assert: func(t *testing.T, path string) {
				info, err := os.Lstat(path)
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			},
		},
		{
			name: "file",
			replace: func(t *testing.T, path string) {
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.WriteFile(path, []byte("external file after revalidate\n"), 0644))
			},
			assert: func(t *testing.T, path string) {
				assertFileContentInternal(t, path, "external file after revalidate\n")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceControl, sourcePayload := setupSeparatedCloneSourceRepoInternal(t)
			require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
			_ = createCloneSavePointInternal(t, sourceControl, "main", "main baseline")

			targetBase := t.TempDir()
			targetControl := filepath.Join(targetBase, "target-control")
			targetPayload := filepath.Join(targetBase, "target-payload")
			require.NoError(t, os.MkdirAll(targetControl, 0755))
			require.NoError(t, os.MkdirAll(targetPayload, 0755))
			hookCalled := false
			installAfterRevalidateEmptyTargetRootHook(t, func(root, role string) error {
				if root != targetPayload || role != "payload" {
					return nil
				}
				hookCalled = true
				tc.replace(t, root)
				return nil
			})

			_, err := Clone(Options{
				SourceRepoRoot:    sourceControl,
				TargetControlRoot: targetControl,
				TargetPayloadRoot: targetPayload,
				SavePointsMode:    SavePointsModeMain,
				RequestedEngine:   model.EngineCopy,
			})

			require.True(t, hookCalled, "test must replace the target after empty-root revalidation and before remove")
			assertJVSErrorCodeInternal(t, err, errclass.ErrAtomicPublishBlocked.Code)
			assert.Contains(t, err.Error(), "changed before publish; refusing to remove")
			tc.assert(t, targetPayload)
			assert.NoDirExists(t, filepath.Join(targetControl, repo.JVSDirName))
		})
	}
}

func TestSeparatedCloneRollbackDoesNotRemoveQuarantineWriteBeforeFinalCleanup(t *testing.T) {
	targetRoot, published := publishRollbackTestPayload(t)
	var quarantineRoot string
	installRollbackBeforeFinalCleanupHook(t, func(root string) error {
		quarantineRoot = root
		require.NotEqual(t, targetRoot, quarantineRoot)
		require.DirExists(t, quarantineRoot)
		return os.WriteFile(filepath.Join(quarantineRoot, "external.txt"), []byte("external write"), 0644)
	})

	err := rollbackSeparatedPublishedRoot(published)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "target folder changed after publish; refusing to remove")
	assert.Contains(t, err.Error(), "target folder was quarantined at")
	require.NotEmpty(t, quarantineRoot)
	assert.NoDirExists(t, targetRoot)
	assertFileContentInternal(t, filepath.Join(quarantineRoot, "external.txt"), "external write")
	assertFileContentInternal(t, filepath.Join(quarantineRoot, "app.txt"), "main v1")
}

func TestSeparatedCloneControlPublishCommitUncertainQuarantinesControlRoot(t *testing.T) {
	sourceControl, sourcePayload := setupSeparatedCloneSourceRepoInternal(t)
	require.NoError(t, os.WriteFile(filepath.Join(sourcePayload, "app.txt"), []byte("main v1"), 0644))
	_ = createCloneSavePointInternal(t, sourceControl, "main", "main baseline")

	targetBase := t.TempDir()
	targetControl := filepath.Join(targetBase, "target-control")
	targetPayload := filepath.Join(targetBase, "target-payload")
	controlPublishInjected := false
	installSeparatedCloneRenameHook(t, func(oldpath, newpath string) error {
		if newpath == targetControl {
			controlPublishInjected = true
			require.NoError(t, os.Rename(oldpath, newpath))
			return &fsutil.CommitUncertainError{
				Op:   "rename no-replace",
				Path: newpath,
				Err:  errors.New("injected directory fsync failure"),
			}
		}
		return fsutil.RenameNoReplaceAndSync(oldpath, newpath)
	})

	_, err := Clone(Options{
		SourceRepoRoot:    sourceControl,
		TargetControlRoot: targetControl,
		TargetPayloadRoot: targetPayload,
		SavePointsMode:    SavePointsModeMain,
		RequestedEngine:   model.EngineCopy,
	})

	require.True(t, controlPublishInjected, "test must inject after the target control root rename")
	assertJVSErrorCodeInternal(t, err, errclass.ErrAtomicPublishBlocked.Code)
	assert.Contains(t, err.Error(), "target control root was quarantined at")
	assert.NoDirExists(t, filepath.Join(targetControl, repo.JVSDirName))
	controlQuarantine := assertOneRollbackQuarantineInternal(t, targetControl)
	assert.DirExists(t, filepath.Join(controlQuarantine, repo.JVSDirName))
	assert.NoDirExists(t, targetPayload)
	payloadQuarantine := assertOneRollbackQuarantineInternal(t, targetPayload)
	assertFileContentInternal(t, filepath.Join(payloadQuarantine, "app.txt"), "main v1")
}

func publishRollbackTestPayload(t *testing.T) (string, *separatedPublishedRoot) {
	t.Helper()

	base := t.TempDir()
	stagingRoot := filepath.Join(base, "staging")
	targetRoot := filepath.Join(base, "target")
	require.NoError(t, os.MkdirAll(stagingRoot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stagingRoot, "app.txt"), []byte("main v1"), 0644))
	published, err := publishSeparatedCloneRoot(stagingRoot, targetRoot, false, "payload", true)
	require.NoError(t, err)
	return targetRoot, published
}

func installRollbackBeforeRemoveHook(t *testing.T, hook func(string) error) {
	t.Helper()

	previous := separatedCloneRollbackBeforeRemoveHook
	separatedCloneRollbackBeforeRemoveHook = hook
	t.Cleanup(func() {
		separatedCloneRollbackBeforeRemoveHook = previous
	})
}

func installRollbackBeforeFinalCleanupHook(t *testing.T, hook func(string) error) {
	t.Helper()

	previous := separatedCloneRollbackBeforeFinalCleanupHook
	separatedCloneRollbackBeforeFinalCleanupHook = hook
	t.Cleanup(func() {
		separatedCloneRollbackBeforeFinalCleanupHook = previous
	})
}

func installBeforeRemoveEmptyTargetRootHook(t *testing.T, hook func(root, role string) error) {
	t.Helper()

	previous := separatedCloneBeforeRemoveEmptyTargetRootHook
	separatedCloneBeforeRemoveEmptyTargetRootHook = hook
	t.Cleanup(func() {
		separatedCloneBeforeRemoveEmptyTargetRootHook = previous
	})
}

func installAfterRevalidateEmptyTargetRootHook(t *testing.T, hook func(root, role string) error) {
	t.Helper()

	previous := separatedCloneAfterRevalidateEmptyTargetRootHook
	separatedCloneAfterRevalidateEmptyTargetRootHook = hook
	t.Cleanup(func() {
		separatedCloneAfterRevalidateEmptyTargetRootHook = previous
	})
}

func installSeparatedCloneRenameHook(t *testing.T, hook func(oldpath, newpath string) error) {
	t.Helper()

	previous := separatedCloneRenameNoReplaceAndSync
	separatedCloneRenameNoReplaceAndSync = hook
	t.Cleanup(func() {
		separatedCloneRenameNoReplaceAndSync = previous
	})
}

func setupSeparatedCloneSourceRepoInternal(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	controlRoot := filepath.Join(base, "source-control")
	payloadRoot := filepath.Join(base, "source-payload")
	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	return controlRoot, payloadRoot
}

func createCloneSavePointInternal(t *testing.T, repoRoot, workspaceName, note string) model.SnapshotID {
	t.Helper()

	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint(workspaceName, note, nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func assertJVSErrorCodeInternal(t *testing.T, err error, code string) {
	t.Helper()

	require.Error(t, err)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr), "expected JVS error, got %T: %v", err, err)
	assert.Equal(t, code, jvsErr.Code)
}

func assertFileContentInternal(t *testing.T, path, expected string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}

func assertNoRollbackQuarantineInternal(t *testing.T, targetRoot string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(targetRoot), "."+filepath.Base(targetRoot)+".rollback-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func assertOneRollbackQuarantineInternal(t *testing.T, targetRoot string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(targetRoot), "."+filepath.Base(targetRoot)+".rollback-*"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	return matches[0]
}
