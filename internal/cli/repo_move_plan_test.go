package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repoMovePreviewData struct {
	Mode                    string `json:"mode"`
	Operation               string `json:"operation"`
	PlanID                  string `json:"plan_id"`
	SourceRepoRoot          string `json:"source_repo_root"`
	TargetRepoRoot          string `json:"target_repo_root"`
	RepoID                  string `json:"repo_id"`
	MoveMethod              string `json:"move_method"`
	FolderMoved             bool   `json:"folder_moved"`
	RepoIDChanged           bool   `json:"repo_id_changed"`
	SavePointHistoryChanged bool   `json:"save_point_history_changed"`
	MainWorkspaceUpdated    bool   `json:"main_workspace_updated"`
	ExternalWorkspaces      int    `json:"external_workspaces"`
	RunCommand              string `json:"run_command"`
	SafeRunCommand          string `json:"safe_run_command"`
}

type repoMoveRunData struct {
	Mode                      string `json:"mode"`
	Operation                 string `json:"operation"`
	PlanID                    string `json:"plan_id"`
	Status                    string `json:"status"`
	SourceRepoRoot            string `json:"source_repo_root"`
	TargetRepoRoot            string `json:"target_repo_root"`
	RepoRoot                  string `json:"repo_root"`
	RepoID                    string `json:"repo_id"`
	FolderMoved               bool   `json:"folder_moved"`
	RepoIDChanged             bool   `json:"repo_id_changed"`
	SavePointHistoryChanged   bool   `json:"save_point_history_changed"`
	MainWorkspaceUpdated      bool   `json:"main_workspace_updated"`
	ExternalWorkspacesUpdated int    `json:"external_workspaces_updated"`
}

func TestRepoMovePreviewFromInsideRepoWritesPlanOnlyAndIncludesExplicitRepoSafeCommands(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomovepreview")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
	subdir := filepath.Join(fixture.repoRoot, "src")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.Chdir(subdir))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "move", target)
	require.NoError(t, err, stdout)
	env := decodeRootJSONData(t, stdout, &repoMovePreviewData{})
	assert.Equal(t, "repo move", env.Command)
	preview := decodeRepoMovePreview(t, stdout)
	require.NotEmpty(t, preview.PlanID)
	assert.Equal(t, "preview", preview.Mode)
	assert.Equal(t, "repo_move", preview.Operation)
	assert.Equal(t, fixture.repoRoot, preview.SourceRepoRoot)
	assert.Equal(t, target, preview.TargetRepoRoot)
	assert.Equal(t, fixture.repoID, preview.RepoID)
	assert.Equal(t, "atomic rename required", preview.MoveMethod)
	assert.False(t, preview.FolderMoved)
	assert.False(t, preview.RepoIDChanged)
	assert.False(t, preview.SavePointHistoryChanged)
	assert.False(t, preview.MainWorkspaceUpdated)
	assert.Equal(t, 1, preview.ExternalWorkspaces)
	assert.Equal(t, "jvs --repo "+fixture.repoRoot+" repo move --run "+preview.PlanID, preview.RunCommand)
	assert.Equal(t, "cd "+filepath.Dir(fixture.repoRoot)+" && jvs --repo "+fixture.repoRoot+" repo move --run "+preview.PlanID, preview.SafeRunCommand)
	assert.DirExists(t, fixture.repoRoot)
	assert.NoDirExists(t, target)
	requireRepoMovePlanFile(t, fixture.repoRoot, preview.PlanID)
	pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
}

func TestRepoMoveAndRenamePreviewShellQuotesRunCommands(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		args    func(repoRoot string) []string
	}{
		{
			name:    "move",
			command: "move",
			args: func(repoRoot string) []string {
				return []string{filepath.Join(filepath.Dir(repoRoot), "moved repo")}
			},
		},
		{
			name:    "rename",
			command: "rename",
			args: func(string) []string {
				return []string{"renamed-repo"}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := setupRepoMoveUnsafePathRepo(t)
			jsonArgs := append([]string{"--json", "repo", tc.command}, tc.args(repoRoot)...)
			stdout, err := executeCommand(createTestRootCmd(), jsonArgs...)
			require.NoError(t, err, stdout)
			preview := decodeRepoMovePreview(t, stdout)
			base := filepath.Dir(filepath.Dir(repoRoot))
			sep := string(os.PathSeparator)
			expectedRepoArg := "'" + base + sep + "parent dir; echo '\\''parent'\\''" + sep + "repo dir; echo '\\''repo'\\'''"
			expectedParentArg := "'" + base + sep + "parent dir; echo '\\''parent'\\'''"
			expectedRunPrefix := "jvs --repo " + expectedRepoArg + " repo " + tc.command + " --run "
			expectedRun := expectedRunPrefix + preview.PlanID
			expectedSafeRunPrefix := "cd " + expectedParentArg + " && " + expectedRunPrefix
			expectedSafeRun := expectedSafeRunPrefix + preview.PlanID
			assert.Equal(t, expectedRun, preview.RunCommand)
			assert.Equal(t, expectedSafeRun, preview.SafeRunCommand)

			humanArgs := append([]string{"repo", tc.command}, tc.args(repoRoot)...)
			human, err := executeCommand(createTestRootCmd(), humanArgs...)
			require.NoError(t, err, human)
			assert.Contains(t, human, "Run: `"+expectedRunPrefix)
			assert.Contains(t, human, "Safe run: `"+expectedSafeRunPrefix)
		})
	}
}

func TestRepoMoveRunFromInsideOldRepoFailsBeforeJournalOrMutation(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomoveunsafecwd")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
	preview := previewRepoMove(t, fixture.repoRoot, target)
	planPath := requireRepoMovePlanFile(t, fixture.repoRoot, preview.PlanID)
	subdir := filepath.Join(fixture.repoRoot, "nested")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.Chdir(subdir))

	stdout, err := executeCommand(createTestRootCmd(), "repo", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.ErrorIs(t, err, errclass.ErrLifecycleUnsafeCWD)
	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr))
	assert.Equal(t, "E_LIFECYCLE_UNSAFE_CWD", jvsErr.Code)
	assert.Contains(t, err.Error(), "jvs --repo "+fixture.repoRoot+" repo move --run "+preview.PlanID)
	assert.DirExists(t, fixture.repoRoot)
	assert.NoDirExists(t, target)
	assert.FileExists(t, planPath)
	pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
}

func TestRepoMoveRunFromOutsideWithExplicitRepoMovesRootAndKeepsHistoryAndExternalWorkspaceHealthy(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomoverun")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
	preview := previewRepoMove(t, fixture.repoRoot, target)
	require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

	runOut, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "--json", "repo", "move", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeRepoMoveRun(t, runOut)
	assert.Equal(t, "run", run.Mode)
	assert.Equal(t, "repo_move", run.Operation)
	assert.Equal(t, "moved", run.Status)
	assert.Equal(t, fixture.repoRoot, run.SourceRepoRoot)
	assert.Equal(t, target, run.TargetRepoRoot)
	assert.Equal(t, target, run.RepoRoot)
	assert.Equal(t, fixture.repoID, run.RepoID)
	assert.True(t, run.FolderMoved)
	assert.False(t, run.RepoIDChanged)
	assert.False(t, run.SavePointHistoryChanged)
	assert.True(t, run.MainWorkspaceUpdated)
	assert.Equal(t, 1, run.ExternalWorkspacesUpdated)
	assert.NoDirExists(t, fixture.repoRoot)
	assert.DirExists(t, target)
	assert.Equal(t, fixture.repoID, readRepoMoveTestRepoID(t, target))
	assert.NoFileExists(t, filepath.Join(target, ".jvs", repoMovePlansDirName, preview.PlanID+".json"))
	pending, pendingErr := lifecycle.ListPendingOperations(target)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)

	mainCfg, err := repo.LoadWorktreeConfig(target, "main")
	require.NoError(t, err)
	assert.Equal(t, target, mainCfg.RealPath)
	featureCfg, err := repo.LoadWorktreeConfig(target, "feature")
	require.NoError(t, err)
	assert.Equal(t, fixture.externalWorkspace, featureCfg.RealPath)
	assertRepoMoveExternalLocator(t, fixture.externalWorkspace, target, fixture.repoID, "feature")

	require.NoError(t, os.Chdir(target))
	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, target, status.Folder)
	assert.Equal(t, "main", status.Workspace)
	require.NotNil(t, status.NewestSavePoint)
	assert.Equal(t, fixture.savePointID, *status.NewestSavePoint)
	historyOut, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err, historyOut)
	assert.Contains(t, historyOut, fixture.savePointID)
	require.NoError(t, os.WriteFile(filepath.Join(target, "app.txt"), []byte("v2"), 0644))
	saveOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "after repo move")
	require.NoError(t, err, saveOut)
	assert.Contains(t, saveOut, "after repo move")
	doctorOut, err := executeCommand(createTestRootCmd(), "--json", "doctor", "--strict")
	require.NoError(t, err, doctorOut)
	doctorData := decodeContractDataMap(t, doctorOut)
	assert.Equal(t, true, doctorData["healthy"])

	require.NoError(t, os.Chdir(fixture.externalWorkspace))
	externalStatusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, externalStatusOut)
	externalEnv := decodeRootJSONData(t, externalStatusOut, &status)
	require.NotNil(t, externalEnv.RepoRoot)
	assert.Equal(t, target, *externalEnv.RepoRoot)
	assert.Equal(t, fixture.externalWorkspace, status.Folder)
	assert.Equal(t, "feature", status.Workspace)
	externalDoctorOut, err := executeCommand(createTestRootCmd(), "--json", "doctor", "--strict")
	require.NoError(t, err, externalDoctorOut)
	externalDoctorData := decodeContractDataMap(t, externalDoctorOut)
	assert.Equal(t, true, externalDoctorData["healthy"])
}

func TestRepoRenameValidatesBasenameOnlyAndRunsAsRepoRename(t *testing.T) {
	for _, invalid := range []string{filepath.Join(t.TempDir(), "absolute"), "child/name", ".", ".."} {
		t.Run("invalid "+filepath.Base(invalid), func(t *testing.T) {
			fixture := setupRepoMoveFixture(t, "reporenameinvalid")
			stdout, err := executeCommand(createTestRootCmd(), "repo", "rename", invalid)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.DirExists(t, fixture.repoRoot)
		})
	}

	fixture := setupRepoMoveFixture(t, "reporenamesuccess")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "renamed-repo")
	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "rename", "renamed-repo")
	require.NoError(t, err, stdout)
	env := decodeRootJSONData(t, stdout, &repoMovePreviewData{})
	assert.Equal(t, "repo rename", env.Command)
	preview := decodeRepoMovePreview(t, stdout)
	assert.Equal(t, "repo_rename", preview.Operation)
	assert.Equal(t, fixture.repoRoot, preview.SourceRepoRoot)
	assert.Equal(t, target, preview.TargetRepoRoot)
	assert.Equal(t, "jvs --repo "+fixture.repoRoot+" repo rename --run "+preview.PlanID, preview.RunCommand)

	require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))
	runOut, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "--json", "repo", "rename", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	runEnv := decodeRootJSONData(t, runOut, &repoMoveRunData{})
	assert.Equal(t, "repo rename", runEnv.Command)
	run := decodeRepoMoveRun(t, runOut)
	assert.Equal(t, "repo_rename", run.Operation)
	assert.Equal(t, "moved", run.Status)
	assert.NoDirExists(t, fixture.repoRoot)
	assert.DirExists(t, target)
	assert.Equal(t, fixture.repoID, readRepoMoveTestRepoID(t, target))
}

func TestRepoMovePreviewRejectsTargetInsideRegisteredExternalWorkspace(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomoveexternaloverlap")
	target := filepath.Join(fixture.externalWorkspace, "nested-repo")

	stdout, err := executeCommand(createTestRootCmd(), "repo", "move", target)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "external workspace")
	assert.DirExists(t, fixture.repoRoot)
	assert.NoDirExists(t, target)
	assert.NoDirExists(t, filepath.Join(fixture.repoRoot, ".jvs", repoMovePlansDirName))
	pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
}

func TestRepoMoveExternalLocatorPreflightFailsClosedBeforeMovingRoot(t *testing.T) {
	t.Run("malformed locator blocks preview", func(t *testing.T) {
		fixture := setupRepoMoveFixture(t, "repomovemalformedlocator")
		target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
		require.NoError(t, os.WriteFile(filepath.Join(fixture.externalWorkspace, repo.JVSDirName), []byte("{not-json"), 0644))

		stdout, err := executeCommand(createTestRootCmd(), "repo", "move", target)
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.DirExists(t, fixture.repoRoot)
		assert.NoDirExists(t, target)
	})

	t.Run("wrong locator freshness blocks run before move", func(t *testing.T) {
		fixture := setupRepoMoveFixture(t, "repomovewronglocator")
		target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
		preview := previewRepoMove(t, fixture.repoRoot, target)
		writeRepoMoveTestWorkspaceLocator(t, fixture.externalWorkspace, fixture.repoRoot, fixture.repoID, "wrong-name")
		require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

		stdout, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "repo", "move", "--run", preview.PlanID)
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.Contains(t, err.Error(), "workspace_name mismatch")
		assert.DirExists(t, fixture.repoRoot)
		assert.NoDirExists(t, target)
		pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
		require.NoError(t, pendingErr)
		assert.Empty(t, pending)
	})

	t.Run("unwritable locator blocks run before move", func(t *testing.T) {
		fixture := setupRepoMoveFixture(t, "repomoveunwritablelocator")
		target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
		preview := previewRepoMove(t, fixture.repoRoot, target)
		require.NoError(t, os.Chmod(fixture.externalWorkspace, 0555))
		t.Cleanup(func() { _ = os.Chmod(fixture.externalWorkspace, 0755) })
		require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

		stdout, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "repo", "move", "--run", preview.PlanID)
		require.Error(t, err)
		assert.Empty(t, stdout)
		assert.DirExists(t, fixture.repoRoot)
		assert.NoDirExists(t, target)
		pending, pendingErr := lifecycle.ListPendingOperations(fixture.repoRoot)
		require.NoError(t, pendingErr)
		assert.Empty(t, pending)
	})
}

func TestRepoMoveRunResumesAfterRootMoveBeforeExternalLocatorUpdate(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomoveresume")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
	preview := previewRepoMove(t, fixture.repoRoot, target)
	oldHooks := repoMoveRunHooks
	repoMoveRunHooks.afterRootMoved = func() error {
		return errors.New("injected crash after repo root move")
	}
	t.Cleanup(func() { repoMoveRunHooks = oldHooks })
	require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

	stdout, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "repo", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "injected crash")
	assert.NoDirExists(t, fixture.repoRoot)
	assert.DirExists(t, target)
	pending, pendingErr := lifecycle.ListPendingOperations(target)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.PlanID, pending[0].OperationID)
	assertRepoMoveExternalLocator(t, fixture.externalWorkspace, fixture.repoRoot, fixture.repoID, "feature")

	repoMoveRunHooks = oldHooks
	require.NoError(t, os.Chdir(target))
	runOut, err := executeCommand(createTestRootCmd(), "--json", "repo", "move", "--run", preview.PlanID)
	require.NoError(t, err, runOut)
	run := decodeRepoMoveRun(t, runOut)
	assert.Equal(t, "moved", run.Status)
	assert.Equal(t, target, run.RepoRoot)
	assert.NoDirExists(t, fixture.repoRoot)
	assert.DirExists(t, target)
	assertRepoMoveExternalLocator(t, fixture.externalWorkspace, target, fixture.repoID, "feature")
	mainCfg, err := repo.LoadWorktreeConfig(target, "main")
	require.NoError(t, err)
	assert.Equal(t, target, mainCfg.RealPath)
	pending, pendingErr = lifecycle.ListPendingOperations(target)
	require.NoError(t, pendingErr)
	assert.Empty(t, pending)
	assert.NoFileExists(t, filepath.Join(target, ".jvs", repoMovePlansDirName, preview.PlanID+".json"))
}

func TestRepoMoveOrRenameExternalDiscoveryAfterRootMoveBeforeLocatorRewriteFailsClosedWithExplicitRunCommand(t *testing.T) {
	for _, tc := range []struct {
		name       string
		command    string
		targetName string
		preview    func(t *testing.T, repoRoot, target string) repoMovePreviewData
	}{
		{
			name:       "move",
			command:    "move",
			targetName: "moved-repo",
			preview:    previewRepoMove,
		},
		{
			name:       "rename",
			command:    "rename",
			targetName: "renamed-repo",
			preview: func(t *testing.T, repoRoot, target string) repoMovePreviewData {
				return previewRepoRename(t, repoRoot, filepath.Base(target))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := setupRepoMoveFixture(t, "repomoveexternalpending"+tc.name)
			target := filepath.Join(filepath.Dir(fixture.repoRoot), tc.targetName)
			preview := tc.preview(t, fixture.repoRoot, target)
			oldHooks := repoMoveRunHooks
			repoMoveRunHooks.afterRootMoved = func() error {
				return errors.New("injected crash after repo root move")
			}
			t.Cleanup(func() { repoMoveRunHooks = oldHooks })
			require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

			stdout, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "repo", tc.command, "--run", preview.PlanID)
			require.Error(t, err)
			assert.Empty(t, stdout)
			repoMoveRunHooks = oldHooks
			assert.NoDirExists(t, fixture.repoRoot)
			assert.DirExists(t, target)
			assertRepoMoveExternalLocator(t, fixture.externalWorkspace, fixture.repoRoot, fixture.repoID, "feature")

			expectedRunCommand := preview.RunCommand
			assert.Equal(t, "jvs --repo "+fixture.repoRoot+" repo "+tc.command+" --run "+preview.PlanID, expectedRunCommand)
			pending, pendingErr := lifecycle.ListPendingOperations(target)
			require.NoError(t, pendingErr)
			require.Len(t, pending, 1)
			assert.Equal(t, expectedRunCommand, pending[0].RecommendedNextCommand)
			pendingMarkerCommand := requireRepoMoveExternalLocatorPendingRecommendedCommand(t, fixture.externalWorkspace, fixture.repoRoot, fixture.repoID, "feature")
			assert.Equal(t, expectedRunCommand, pendingMarkerCommand)

			require.NoError(t, os.Chdir(fixture.externalWorkspace))
			statusOut, statusStderr, statusErr := executeCommandWithErrorReport(createTestRootCmd(), "--json", "status")
			require.Error(t, statusErr)
			assertRepoMovePendingExternalDiscoveryError(t, statusOut, statusStderr, expectedRunCommand)

			doctorOut, doctorStderr, doctorExitCode := runContractSubprocess(t, fixture.externalWorkspace, "--json", "doctor", "--strict")
			require.Equal(t, 1, doctorExitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", doctorOut, doctorStderr)
			assertRepoMovePendingExternalDiscoveryError(t, doctorOut, doctorStderr, expectedRunCommand)

			runOut, runStderr, runExitCode := runContractSubprocess(t, fixture.externalWorkspace, "--json", "--repo", fixture.repoRoot, "repo", tc.command, "--run", preview.PlanID)
			require.Equal(t, 0, runExitCode, "recommended command failed: stdout=%s stderr=%s", runOut, runStderr)
			assert.Empty(t, strings.TrimSpace(runStderr))
			run := decodeRepoMoveRun(t, runOut)
			assert.Equal(t, "moved", run.Status)
			assert.Equal(t, target, run.RepoRoot)
			assertRepoMoveExternalLocator(t, fixture.externalWorkspace, target, fixture.repoID, "feature")
			pending, pendingErr = lifecycle.ListPendingOperations(target)
			require.NoError(t, pendingErr)
			assert.Empty(t, pending)
		})
	}
}

func TestRepoMoveResumeFailsClosedWhenSourceDifferentAndDestinationExpected(t *testing.T) {
	fixture := setupRepoMoveFixture(t, "repomoveresumefail")
	target := filepath.Join(filepath.Dir(fixture.repoRoot), "moved-repo")
	preview := previewRepoMove(t, fixture.repoRoot, target)
	oldHooks := repoMoveRunHooks
	repoMoveRunHooks.afterRootMoved = func() error {
		return errors.New("injected crash after repo root move")
	}
	t.Cleanup(func() { repoMoveRunHooks = oldHooks })
	require.NoError(t, os.Chdir(filepath.Dir(fixture.repoRoot)))

	stdout, err := executeCommand(createTestRootCmd(), "--repo", fixture.repoRoot, "repo", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	repoMoveRunHooks = oldHooks
	require.NoError(t, os.MkdirAll(fixture.repoRoot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.repoRoot, "different.txt"), []byte("different"), 0644))
	require.NoError(t, os.Chdir(target))

	stdout, err = executeCommand(createTestRootCmd(), "repo", "move", "--run", preview.PlanID)
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "repo move cannot resume")
	assert.Contains(t, err.Error(), "source repo root identity changed")
	assert.DirExists(t, fixture.repoRoot)
	assert.DirExists(t, target)
	pending, pendingErr := lifecycle.ListPendingOperations(target)
	require.NoError(t, pendingErr)
	require.Len(t, pending, 1)
	assert.Equal(t, preview.PlanID, pending[0].OperationID)
}

func assertRepoMovePendingExternalDiscoveryError(t *testing.T, stdout, stderr, expectedRunCommand string) {
	t.Helper()
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_LIFECYCLE_PENDING", env.Error.Code)
	assert.Contains(t, env.Error.Message, expectedRunCommand)
	assert.Equal(t, expectedRunCommand, env.Error.RecommendedNextCommand)
	assert.NotContains(t, strings.ToLower(env.Error.Message), "stale")
}

type repoMoveFixture struct {
	repoRoot          string
	repoID            string
	savePointID       string
	externalWorkspace string
}

func setupRepoMoveFixture(t *testing.T, name string) repoMoveFixture {
	t.Helper()
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte(name), 0644))
	savePointID := createRootTestSavePoint(t, "baseline")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", "../feature", "--from", savePointID)
	require.NoError(t, err, stdout)
	return repoMoveFixture{
		repoRoot:          repoRoot,
		repoID:            readRepoMoveTestRepoID(t, repoRoot),
		savePointID:       savePointID,
		externalWorkspace: filepath.Join(filepath.Dir(repoRoot), "feature"),
	}
}

func setupRepoMoveUnsafePathRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	repoRoot := filepath.Join(base, "parent dir; echo 'parent'", "repo dir; echo 'repo'")
	require.NoError(t, os.MkdirAll(filepath.Dir(repoRoot), 0755))
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	r, err := repo.InitTarget(repoRoot)
	require.NoError(t, err)
	require.NoError(t, os.Chdir(r.Root))
	return r.Root
}

func previewRepoMove(t *testing.T, repoRoot, target string) repoMovePreviewData {
	t.Helper()
	require.NoError(t, os.Chdir(repoRoot))
	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "move", target)
	require.NoError(t, err, stdout)
	return decodeRepoMovePreview(t, stdout)
}

func previewRepoRename(t *testing.T, repoRoot, newName string) repoMovePreviewData {
	t.Helper()
	require.NoError(t, os.Chdir(repoRoot))
	stdout, err := executeCommand(createTestRootCmd(), "--json", "repo", "rename", newName)
	require.NoError(t, err, stdout)
	return decodeRepoMovePreview(t, stdout)
}

func decodeRepoMovePreview(t *testing.T, stdout string) repoMovePreviewData {
	t.Helper()
	var preview repoMovePreviewData
	decodeRootJSONData(t, stdout, &preview)
	return preview
}

func decodeRepoMoveRun(t *testing.T, stdout string) repoMoveRunData {
	t.Helper()
	var run repoMoveRunData
	decodeRootJSONData(t, stdout, &run)
	return run
}

func requireRepoMovePlanFile(t *testing.T, repoRoot, planID string) string {
	t.Helper()
	path, err := repoMovePlanPath(repoRoot, planID, false)
	require.NoError(t, err)
	require.FileExists(t, path)
	return path
}

func readRepoMoveTestRepoID(t *testing.T, repoRoot string) string {
	t.Helper()
	id, err := workspaceCurrentRepoID(repoRoot)
	require.NoError(t, err)
	return id
}

func writeRepoMoveTestWorkspaceLocator(t *testing.T, workspaceRoot, repoRoot, repoID, workspaceName string) {
	t.Helper()
	data := []byte(`{"type":"jvs-workspace","format_version":1,"repo_root":` + quoteJSONString(repoRoot) + `,"repo_id":` + quoteJSONString(repoID) + `,"workspace_name":` + quoteJSONString(workspaceName) + `}`)
	require.NoError(t, os.WriteFile(filepath.Join(workspaceRoot, repo.JVSDirName), data, 0644))
}

func quoteJSONString(value string) string {
	data := []byte(value)
	escaped := `"`
	for _, b := range data {
		switch b {
		case '\\', '"':
			escaped += `\` + string(b)
		default:
			escaped += string(b)
		}
	}
	return escaped + `"`
}

func assertRepoMoveExternalLocator(t *testing.T, workspaceRoot, repoRoot, repoID, workspaceName string) {
	t.Helper()
	diagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         workspaceRoot,
		ExpectedRepoRoot:      repoRoot,
		ExpectedRepoID:        repoID,
		ExpectedWorkspaceName: workspaceName,
	})
	require.NoError(t, err)
	assert.True(t, diagnostic.Matches, diagnostic.Reason)
}

func requireRepoMoveExternalLocatorPendingRecommendedCommand(t *testing.T, workspaceRoot, repoRoot, repoID, workspaceName string) string {
	t.Helper()
	diagnostic, err := repo.InspectWorkspaceLocator(repo.WorkspaceLocatorCheck{
		WorkspaceRoot:         workspaceRoot,
		ExpectedRepoRoot:      repoRoot,
		ExpectedRepoID:        repoID,
		ExpectedWorkspaceName: workspaceName,
	})
	require.NoError(t, err)
	require.True(t, diagnostic.Matches, diagnostic.Reason)
	require.NotNil(t, diagnostic.Locator.PendingLifecycle)
	return diagnostic.Locator.PendingLifecycle.RecommendedNextCommand
}
