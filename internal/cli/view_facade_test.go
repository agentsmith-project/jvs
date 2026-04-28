package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewHelpUsesSavePointReadOnlyVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "view", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "save point")
	assert.Contains(t, stdout, "read-only")
	assert.Contains(t, stdout, "folder")
	assert.Contains(t, stdout, "workspace")
	assertViewOutputOmitsLegacyVocabulary(t, stdout)
}

func TestViewSavePointPathIsReadOnlyAndDoesNotChangeWorkspace(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "src", "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	firstPrefix := uniqueSavePointPrefix(firstID, secondID)

	statusBefore, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyBefore, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	catalogBefore := savePointCatalogCount(t, repoRoot)
	descriptorsBefore := descriptorFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstPrefix, "src/app.txt")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Folder: "+repoRoot)
	assert.Contains(t, stdout, "Workspace: main")
	assert.Contains(t, stdout, "Opened read-only view.")
	assert.Contains(t, stdout, "Save point: "+firstID)
	assert.Contains(t, stdout, "Path inside save point: src/app.txt")
	assert.Contains(t, stdout, "No workspace or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, stdout)

	viewPath := viewPathFromHumanOutput(t, stdout)
	restoreViewWriteBitsForCleanup(t, viewPath)
	content, err := os.ReadFile(viewPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
	assertReadOnlyPath(t, viewPath)
	require.Error(t, os.WriteFile(viewPath, []byte("mutate view"), 0644))
	payloadRoot := viewPayloadRoot(t, viewPath, "src/app.txt")
	require.NoFileExists(t, filepath.Join(payloadRoot, ".READY"))
	require.NoDirExists(t, filepath.Join(payloadRoot, ".jvs"))

	workspaceContent, err := os.ReadFile(filepath.Join(repoRoot, "src", "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(workspaceContent))

	statusAfter, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyAfter, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	assert.JSONEq(t, string(decodeContractEnvelope(t, statusBefore).Data), string(decodeContractEnvelope(t, statusAfter).Data))
	assert.JSONEq(t, string(decodeContractEnvelope(t, historyBefore).Data), string(decodeContractEnvelope(t, historyAfter).Data))
	assert.Equal(t, catalogBefore, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, descriptorsBefore, descriptorFileCount(t, repoRoot))
}

func TestViewJSONUsesSavePointSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "view", firstID, "file.txt")
	require.NoError(t, err)

	env, data := decodeFacadeDataMap(t, stdout)
	require.True(t, env.OK, stdout)
	require.Equal(t, "view", env.Command)
	require.Equal(t, repoRoot, data["folder"])
	require.Equal(t, "main", data["workspace"])
	require.Equal(t, firstID, data["save_point"])
	require.Equal(t, "file.txt", data["path_inside_save_point"])
	require.NotEmpty(t, data["view_id"])
	viewPath, ok := data["view_path"].(string)
	require.True(t, ok, "view_path should be a string: %#v", data["view_path"])
	restoreViewWriteBitsForCleanup(t, viewPath)
	require.FileExists(t, viewPath)
	require.Equal(t, true, data["read_only"])
	require.Equal(t, true, data["no_workspace_or_history_changed"])
	assertViewJSONOmitsLegacyFields(t, data)

	content, err := os.ReadFile(viewPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

func TestViewOpenCreatesPersistentPinAndCloseRemovesViewWithoutChangingWorkspace(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.NoError(t, err)
	viewID := viewIDFromHumanOutput(t, stdout)
	viewPath := viewPathFromHumanOutput(t, stdout)
	require.DirExists(t, viewPath)
	requireViewPin(t, repoRoot, viewID, model.SnapshotID(firstID))
	assert.Equal(t, before.pins+1, documentedPinCount(t, repoRoot))

	closeOut, err := executeCommand(createTestRootCmd(), "view", "close", viewID)
	require.NoError(t, err)
	assert.Contains(t, closeOut, "Closed read-only view.")
	assert.Contains(t, closeOut, "View: "+viewID)
	assert.Contains(t, closeOut, "Save point: "+firstID)
	assert.Contains(t, closeOut, "View path removed: yes")
	assert.Contains(t, closeOut, "No workspace or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, closeOut)

	assert.NoDirExists(t, viewRootForPath(t, viewPath))
	assert.NoFileExists(t, viewPinPath(repoRoot, viewID))

	againOut, err := executeCommand(createTestRootCmd(), "view", "close", viewID)
	require.NoError(t, err)
	assert.Contains(t, againOut, "Read-only view already closed.")
	assert.Contains(t, againOut, "View path removed: yes")
	before.assertUnchanged(t, repoRoot)
}

func TestViewCloseFromViewPathIsRepoScopedAndJSONUsesCloseSchema(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.NoError(t, err)
	viewID := viewIDFromHumanOutput(t, stdout)
	viewPath := viewPathFromHumanOutput(t, stdout)
	require.NoError(t, os.Chdir(viewPath))

	jsonOut, err := executeCommand(createTestRootCmd(), "--json", "view", "close", viewID)
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoRoot))

	env, data := decodeFacadeDataMap(t, jsonOut)
	require.True(t, env.OK, jsonOut)
	assert.Equal(t, "view close", env.Command)
	assert.Equal(t, "close", data["mode"])
	assert.Equal(t, "closed", data["status"])
	assert.Equal(t, viewID, data["view_id"])
	assert.Equal(t, firstID, data["save_point"])
	assert.Equal(t, true, data["view_path_removed"])
	assert.Equal(t, true, data["no_workspace_or_history_changed"])
	assertViewJSONOmitsLegacyFields(t, data)
	assert.NoDirExists(t, viewRootForPath(t, viewPath))
	assert.NoFileExists(t, viewPinPath(repoRoot, viewID))
}

func TestViewCloseRejectsInvalidIDWithPublicVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)

	stdout, err := executeCommand(createTestRootCmd(), "view", "close", "../bad")
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "read-only view ID")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	assert.NotContains(t, strings.ToLower(err.Error()), "pin")
	assert.NotContains(t, strings.ToLower(err.Error()), "gc")
	assert.NotContains(t, strings.ToLower(err.Error()), "internal")

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "view", "close", "../bad")
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "read-only view ID")
	assert.Contains(t, env.Error.Message, "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, env.Error.Message)
	assert.NotContains(t, strings.ToLower(env.Error.Message), "pin")
	assert.NotContains(t, strings.ToLower(env.Error.Message), "gc")
	assert.NotContains(t, strings.ToLower(env.Error.Message), "internal")
}

func TestViewCloseFailsClosedForNonViewDocumentedPin(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	savePointID := model.NewSnapshotID()
	viewID := "view-" + string(model.NewSnapshotID())
	writeDocumentedPinForViewTest(t, repoRoot, model.Pin{
		PinID:      viewID,
		SnapshotID: savePointID,
		CreatedAt:  time.Now().UTC(),
		PinnedAt:   time.Now().UTC(),
		Reason:     "manual operator protection",
	})

	stdout, err := executeCommand(createTestRootCmd(), "view", "close", viewID)
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "read-only view")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	assert.FileExists(t, viewPinPath(repoRoot, viewID))
}

func TestSaveAndRestoreFromReadOnlyViewPathFailWithViewSpecificError(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	require.NoError(t, err)
	viewID := viewIDFromHumanOutput(t, stdout)
	viewPath := viewPathFromHumanOutput(t, stdout)
	require.NoError(t, os.Chdir(viewPath))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "save", args: []string{"save", "-m", "from view"}},
		{name: "restore", args: []string{"restore", firstID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), tc.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "This path is a read-only view of save point "+firstID)
			assert.Contains(t, err.Error(), "not a workspace")
			assert.Contains(t, err.Error(), "No files or history were changed.")
			assertViewOutputOmitsLegacyVocabulary(t, err.Error())
		})
	}

	closeOut, err := executeCommand(createTestRootCmd(), "view", "close", viewID)
	require.NoError(t, err)
	assert.Contains(t, closeOut, "Closed read-only view.")
	require.NoError(t, os.Chdir(repoRoot))
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsNonIDRefsWithSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "baseline")
	_ = createCheckpointForPublicCLI(t, "tagged save point", "--tag", "release-tag")
	statusBefore, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyBefore, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	catalogBefore := savePointCatalogCount(t, repoRoot)
	descriptorsBefore := descriptorFileCount(t, repoRoot)

	for _, ref := range []string{"latest", "current", "dirty", "baseline", "release-tag", "file.txt"} {
		t.Run(ref, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "view", ref)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "Choose a save point ID")
			assert.Contains(t, err.Error(), "No files or history changed.")
			assertViewOutputOmitsLegacyVocabulary(t, err.Error())
		})
	}

	jsonOut, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "view", "latest")
	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, jsonOut)
	require.False(t, env.OK, jsonOut)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, "Choose a save point ID")
	assert.Contains(t, env.Error.Message, "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, env.Error.Message)

	statusAfter, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyAfter, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	assert.JSONEq(t, string(decodeContractEnvelope(t, statusBefore).Data), string(decodeContractEnvelope(t, statusAfter).Data))
	assert.JSONEq(t, string(decodeContractEnvelope(t, historyBefore).Data), string(decodeContractEnvelope(t, historyAfter).Data))
	assert.Equal(t, catalogBefore, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, descriptorsBefore, descriptorFileCount(t, repoRoot))
}

func TestViewHumanSubprocessErrorKeepsSavePointVocabulary(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	_ = savePointIDFromCLI(t, "baseline")
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, "view", "latest")

	require.NotZero(t, exitCode)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "Choose a save point ID")
	assert.Contains(t, stderr, "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, stderr)
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsPathTraversalAndControlDataWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")

	statusBefore, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyBefore, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	catalogBefore := savePointCatalogCount(t, repoRoot)
	descriptorsBefore := descriptorFileCount(t, repoRoot)

	for _, path := range []string{
		filepath.Join(repoRoot, "file.txt"),
		"../file.txt",
		"",
		`C:\Users\alice\file.txt`,
		"bad\x00path",
		".jvs/format_version",
	} {
		t.Run(path, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), "view", firstID, path)
			require.Error(t, err)
			require.Empty(t, stdout)
			assert.Contains(t, err.Error(), "workspace-relative path")
			assert.Contains(t, err.Error(), "No files or history changed.")
			assertViewOutputOmitsLegacyVocabulary(t, err.Error())

			content, readErr := os.ReadFile(filepath.Join(repoRoot, "file.txt"))
			require.NoError(t, readErr)
			assert.Equal(t, "v1", string(content))
		})
	}

	statusAfter, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyAfter, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	assert.JSONEq(t, string(decodeContractEnvelope(t, statusBefore).Data), string(decodeContractEnvelope(t, statusAfter).Data))
	assert.JSONEq(t, string(decodeContractEnvelope(t, historyBefore).Data), string(decodeContractEnvelope(t, historyAfter).Data))
	assert.Equal(t, catalogBefore, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, descriptorsBefore, descriptorFileCount(t, repoRoot))
}

func TestViewRejectsFullViewContainingSymlinkWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID := writeViewSavePointSnapshot(t, repoRoot, func(snapshotDir string) {
		require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "target.txt"), []byte("v1"), 0644))
		requireViewSymlink(t, "target.txt", filepath.Join(snapshotDir, "link.txt"))
	})
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	if err == nil && strings.TrimSpace(stdout) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, stdout))
	}

	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "symlink")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsSymlinkParentPathWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0644))
	firstID := writeViewSavePointSnapshot(t, repoRoot, func(snapshotDir string) {
		requireViewSymlink(t, outside, filepath.Join(snapshotDir, "linkdir"))
	})
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID, "linkdir/secret.txt")
	if err == nil && strings.TrimSpace(stdout) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, stdout))
	}

	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "symlink")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	secret, readErr := os.ReadFile(filepath.Join(outside, "secret.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "outside", string(secret))
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsSymlinkPayloadAndCleansReadOnlyMaterializedDirs(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	firstID := writeViewSavePointSnapshot(t, repoRoot, func(snapshotDir string) {
		lockedDir := filepath.Join(snapshotDir, "locked")
		require.NoError(t, os.MkdirAll(lockedDir, 0755))
		requireViewSymlink(t, "../target.txt", filepath.Join(lockedDir, "link.txt"))
		require.NoError(t, os.Chmod(lockedDir, 0555))
		require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, "target.txt"), []byte("v1"), 0644))
	})
	restoreSnapshotWriteBitsForCleanup(t, repoRoot, model.SnapshotID(firstID))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	if err == nil && strings.TrimSpace(stdout) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, stdout))
	}

	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "symlink")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsTaintedControlPayloadWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "tainted later")
	injectControlDataIntoSavePoint(t, repoRoot, model.SnapshotID(firstID))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	if err == nil && strings.TrimSpace(stdout) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, stdout))
	}

	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "control data")
	assert.Contains(t, err.Error(), "not managed")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsTaintedControlPayloadAndCleansReadOnlyMaterializedDirs(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "tainted read only")
	injectControlDataIntoSavePoint(t, repoRoot, model.SnapshotID(firstID))
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, model.SnapshotID(firstID))
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(snapshotDir, repo.JVSDirName), 0555))
	rewriteViewSavePointIntegrity(t, repoRoot, model.SnapshotID(firstID))
	restoreSnapshotWriteBitsForCleanup(t, repoRoot, model.SnapshotID(firstID))
	before := captureViewMutationSnapshot(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID)
	if err == nil && strings.TrimSpace(stdout) != "" {
		restoreViewWriteBitsForCleanup(t, viewPathFromHumanOutput(t, stdout))
	}

	require.Error(t, err)
	require.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, err.Error(), "control data")
	assert.Contains(t, err.Error(), "not managed")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())
	before.assertUnchanged(t, repoRoot)
}

func TestViewRejectsMissingPathWithoutMutation(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	statusBefore, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyBefore, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	catalogBefore := savePointCatalogCount(t, repoRoot)
	descriptorsBefore := descriptorFileCount(t, repoRoot)

	stdout, err := executeCommand(createTestRootCmd(), "view", firstID, "missing.txt")
	require.Error(t, err)
	require.Empty(t, stdout)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), "No files or history changed.")
	assertViewOutputOmitsLegacyVocabulary(t, err.Error())

	content, readErr := os.ReadFile(filepath.Join(repoRoot, "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "v1", string(content))
	statusAfter, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	historyAfter, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	assert.JSONEq(t, string(decodeContractEnvelope(t, statusBefore).Data), string(decodeContractEnvelope(t, statusAfter).Data))
	assert.JSONEq(t, string(decodeContractEnvelope(t, historyBefore).Data), string(decodeContractEnvelope(t, historyAfter).Data))
	assert.Equal(t, catalogBefore, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, descriptorsBefore, descriptorFileCount(t, repoRoot))
}

func viewPathFromHumanOutput(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "View path: ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "View path: "))
			require.NotEmpty(t, path)
			return path
		}
	}
	t.Fatalf("view path not found in output:\n%s", stdout)
	return ""
}

func viewIDFromHumanOutput(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "View: ") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "View: "))
			require.NotEmpty(t, id)
			return id
		}
	}
	t.Fatalf("view ID not found in output:\n%s", stdout)
	return ""
}

func uniqueSavePointPrefix(target string, others ...string) string {
	for i := 1; i <= len(target); i++ {
		prefix := target[:i]
		unique := true
		for _, other := range others {
			if strings.HasPrefix(other, prefix) {
				unique = false
				break
			}
		}
		if unique {
			return prefix
		}
	}
	return target
}

func requireViewSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

type viewMutationSnapshot struct {
	status      string
	history     string
	catalog     int
	descriptors int
	viewDirs    int
	pins        int
}

func captureViewMutationSnapshot(t *testing.T, repoRoot string) viewMutationSnapshot {
	t.Helper()
	status, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	history, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	return viewMutationSnapshot{
		status:      string(decodeContractEnvelope(t, status).Data),
		history:     string(decodeContractEnvelope(t, history).Data),
		catalog:     savePointCatalogCount(t, repoRoot),
		descriptors: descriptorFileCount(t, repoRoot),
		viewDirs:    viewDirCount(t, repoRoot),
		pins:        documentedPinCount(t, repoRoot),
	}
}

func (s viewMutationSnapshot) assertUnchanged(t *testing.T, repoRoot string) {
	t.Helper()
	status, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err)
	history, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err)
	assert.JSONEq(t, s.status, string(decodeContractEnvelope(t, status).Data))
	assert.JSONEq(t, s.history, string(decodeContractEnvelope(t, history).Data))
	assert.Equal(t, s.catalog, savePointCatalogCount(t, repoRoot))
	assert.Equal(t, s.descriptors, descriptorFileCount(t, repoRoot))
	assert.Equal(t, s.viewDirs, viewDirCount(t, repoRoot))
	assert.Equal(t, s.pins, documentedPinCount(t, repoRoot))
}

func assertReadOnlyPath(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0200, "view path should not be owner-writable: %s", path)
}

func restoreViewWriteBitsForCleanup(t *testing.T, viewPath string) {
	t.Helper()
	viewRoot := viewRootForPath(t, viewPath)
	restoreTreeWriteBitsForCleanup(t, viewRoot)
}

func restoreSnapshotWriteBitsForCleanup(t *testing.T, repoRoot string, savePointID model.SnapshotID) {
	t.Helper()
	snapshotDir, err := repo.SnapshotPath(repoRoot, savePointID)
	require.NoError(t, err)
	restoreTreeWriteBitsForCleanup(t, snapshotDir)
}

func restoreTreeWriteBitsForCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				_ = os.Chmod(path, info.Mode().Perm()|0700)
			} else {
				_ = os.Chmod(path, info.Mode().Perm()|0600)
			}
			return nil
		})
	})
}

func viewRootForPath(t *testing.T, viewPath string) string {
	t.Helper()
	for current := filepath.Clean(viewPath); ; current = filepath.Dir(current) {
		if strings.HasPrefix(filepath.Base(current), "view-") {
			return current
		}
		parent := filepath.Dir(current)
		require.NotEqual(t, current, parent, "view root not found for %s", viewPath)
	}
}

func viewPayloadRoot(t *testing.T, viewPath, pathInside string) string {
	t.Helper()
	root := viewPath
	for range strings.Split(pathInside, "/") {
		root = filepath.Dir(root)
	}
	require.Equal(t, "payload", filepath.Base(root))
	return root
}

func viewPinPath(repoRoot, viewID string) string {
	return filepath.Join(repoRoot, ".jvs", "gc", "pins", viewID+".json")
}

func requireViewPin(t *testing.T, repoRoot, viewID string, savePointID model.SnapshotID) {
	t.Helper()
	data, err := os.ReadFile(viewPinPath(repoRoot, viewID))
	require.NoError(t, err)
	var pin model.Pin
	require.NoError(t, json.Unmarshal(data, &pin))
	require.Equal(t, viewID, pin.PinID)
	require.Equal(t, savePointID, pin.SnapshotID)
	require.Contains(t, pin.Reason, "read-only view")
}

func writeDocumentedPinForViewTest(t *testing.T, repoRoot string, pin model.Pin) {
	t.Helper()
	data, err := json.MarshalIndent(pin, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".jvs", "gc", "pins"), 0755))
	require.NoError(t, os.WriteFile(viewPinPath(repoRoot, pin.PinID), data, 0644))
}

func documentedPinCount(t testing.TB, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "gc", "pins"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func savePointCatalogCount(t *testing.T, repoRoot string) int {
	t.Helper()
	savePoints, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	return len(savePoints)
}

func descriptorFileCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "descriptors"))
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func viewDirCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "views"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func injectControlDataIntoSavePoint(t *testing.T, repoRoot string, savePointID model.SnapshotID) {
	t.Helper()

	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, savePointID)
	require.NoError(t, err)
	controlPath := filepath.Join(snapshotDir, repo.JVSDirName, "format_version")
	require.NoError(t, os.MkdirAll(filepath.Dir(controlPath), 0755))
	require.NoError(t, os.WriteFile(controlPath, []byte("tainted"), 0644))

	rewriteViewSavePointIntegrity(t, repoRoot, savePointID)
}

func rewriteViewSavePointIntegrity(t *testing.T, repoRoot string, savePointID model.SnapshotID) {
	t.Helper()

	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, savePointID)
	require.NoError(t, err)

	desc, err := snapshot.LoadDescriptor(repoRoot, savePointID)
	require.NoError(t, err)
	payloadHash, err := integrity.ComputePayloadRootHash(snapshotDir)
	require.NoError(t, err)
	desc.PayloadRootHash = payloadHash
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(repoRoot, savePointID)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))

	ready := model.ReadyMarker{
		SnapshotID:         savePointID,
		CompletedAt:        time.Now().UTC(),
		PayloadHash:        payloadHash,
		Engine:             desc.Engine,
		DescriptorChecksum: checksum,
	}
	readyData, err := json.MarshalIndent(ready, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), readyData, 0644))
}

func writeViewSavePointSnapshot(t *testing.T, repoRoot string, mutate func(snapshotDir string)) string {
	t.Helper()

	savePointID := model.NewSnapshotID()
	snapshotDir, err := repo.SnapshotPath(repoRoot, savePointID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	mutate(snapshotDir)

	payloadHash, err := integrity.ComputePayloadRootHash(snapshotDir)
	require.NoError(t, err)
	desc := &model.Descriptor{
		SnapshotID:      savePointID,
		WorktreeName:    "main",
		CreatedAt:       time.Now().UTC(),
		Note:            "view fixture",
		Engine:          model.EngineCopy,
		PayloadRootHash: payloadHash,
		IntegrityState:  model.IntegrityVerified,
	}
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath, err := repo.SnapshotDescriptorPathForWrite(repoRoot, savePointID)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))

	ready := model.ReadyMarker{
		SnapshotID:         savePointID,
		CompletedAt:        time.Now().UTC(),
		PayloadHash:        payloadHash,
		Engine:             model.EngineCopy,
		DescriptorChecksum: checksum,
	}
	readyData, err := json.MarshalIndent(ready, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), readyData, 0644))

	return string(savePointID)
}

func assertViewOutputOmitsLegacyVocabulary(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, word := range []string{
		"checkpoint",
		"snapshot",
		"worktree",
		"current",
		"latest",
		"head",
		"fork",
		"commit",
		"dirty",
	} {
		assert.NotContains(t, lower, word)
	}
}

func assertViewJSONOmitsLegacyFields(t *testing.T, data map[string]any) {
	t.Helper()
	for _, key := range []string{
		"checkpoint_id",
		"snapshot_id",
		"worktree",
		"current",
		"latest",
		"head_snapshot",
		"latest_snapshot",
	} {
		assert.NotContains(t, data, key)
	}
}
