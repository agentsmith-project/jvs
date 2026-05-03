package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/terminal"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeCommand(root *cobra.Command, args ...string) (stdout string, err error) {
	resetCommandHelpFlags(root)
	defer resetCommandHelpFlags(root)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	root.SetArgs(args)
	err = root.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String(), err
}

func executeCommandWithErrorReport(root *cobra.Command, args ...string) (stdout string, stderr string, err error) {
	resetCommandHelpFlags(root)
	defer resetCommandHelpFlags(root)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW

	root.SetArgs(args)
	primeOutputFlagsFromArgs(args)
	cmd, err := root.ExecuteC()
	if err != nil {
		reportCommandErrorForCommand(cmd, err)
	}

	stdoutW.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf bytes.Buffer
	io.Copy(&stdoutBuf, stdoutR)
	var stderrBuf bytes.Buffer
	io.Copy(&stderrBuf, stderrR)
	return stdoutBuf.String(), stderrBuf.String(), err
}

func resetCommandHelpFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" {
			_ = flag.Value.Set("false")
		}
		flag.Changed = false
	})
	for _, child := range cmd.Commands() {
		resetCommandHelpFlags(child)
	}
}

func setupTestDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})
	return dir
}

func initLegacyRepoForCLITest(t testing.TB, path string) string {
	t.Helper()

	r, err := repo.InitTarget(path)
	require.NoError(t, err)
	return r.Root
}

func TestRootCommand_Help(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "--help")
	require.NoError(t, err)

	for _, line := range []string{
		"Start with:",
		"jvs init",
		`jvs save -m "baseline"`,
		"jvs history",
		"jvs view <save> [path]",
		"jvs restore <save>",
	} {
		assert.Contains(t, stdout, line)
	}
	for _, command := range []string{"init", "save", "status", "history", "view", "restore", "repo", "workspace", "recovery", "doctor", "cleanup", "completion", "help"} {
		assertRootHelpListsCommand(t, stdout, command)
	}
	for _, word := range []string{
		"fork",
		"gc",
		"pin",
		"internal",
		"clone",
		"import",
		"checkpoint",
		"snapshot",
		"worktree",
		"branch",
		"checkout",
		"commit",
		"promote",
		"detached",
		"current",
		"latest",
		"dirty",
		"config",
		"conformance",
		"diff",
		"verify",
		"capability",
		"info",
	} {
		assertRootHelpOmitsWord(t, stdout, word)
	}
}

func assertRootHelpListsCommand(t *testing.T, help, command string) {
	t.Helper()

	pattern := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(command) + `\s+`)
	assert.True(t, pattern.MatchString(help), "help should list %q:\n%s", command, help)
}

func assertRootHelpOmitsWord(t *testing.T, help, word string) {
	t.Helper()

	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
	assert.False(t, pattern.MatchString(help), "root help should not expose %q:\n%s", word, help)
}

func TestRootCommand_RemovedOldPublicCommandsAreUnknown(t *testing.T) {
	for _, oldCommand := range []string{
		"checkpoint",
		"snapshot",
		"fork",
		"worktree",
		"gc",
		"verify",
		"capability",
		"info",
		"diff",
		"import",
		"clone",
		"config",
		"conformance",
	} {
		t.Run(oldCommand, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), oldCommand, "--help")
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), "unknown command")
		})
	}
}

func TestWorkspaceCommand_HelpListsPublicManagementSubcommands(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "--help")
	require.NoError(t, err)

	for _, command := range []string{"new", "list", "path", "rename", "move", "delete"} {
		assertRootHelpListsCommand(t, stdout, command)
	}
	assert.NotContains(t, stdout, "  remove")
	assert.NotContains(t, stdout, "worktree")
	assert.NotContains(t, stdout, "checkpoint")
}

func TestWorkspaceCommand_RenameIsNameOnlyAndUpdatesExternalLocator(t *testing.T) {
	dir := setupTestDir(t)
	repoPath := filepath.Join(dir, "testrepo")
	require.NoError(t, os.Mkdir(repoPath, 0755))
	require.NoError(t, os.Chdir(repoPath))
	initOut, err := executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err, initOut)
	require.NoError(t, os.WriteFile("file.txt", []byte("baseline"), 0644))

	saveID := createRootTestSavePoint(t, "baseline")
	newOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "new", "../folder-stays", "--from", saveID, "--name", "old-feature")
	require.NoError(t, err, newOut)
	originalFolder := filepath.Join(filepath.Dir(repoPath), "folder-stays")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "rename", "old-feature", "new-feature")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "workspace rename", env.Command)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assert.Equal(t, "new-feature", data["workspace"])
	assert.Equal(t, "old-feature", data["old_workspace"])
	assert.Equal(t, originalFolder, data["folder"])
	assert.Equal(t, false, data["folder_moved"])
	assert.DirExists(t, originalFolder)
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repoPath), "new-feature"))

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "path", "old-feature")
	require.Error(t, err, stdout)
	assert.Empty(t, stdout)

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "path", "new-feature")
	require.NoError(t, err, stdout)
	var pathData map[string]string
	decodeRootJSONData(t, stdout, &pathData)
	assert.Equal(t, originalFolder, pathData["path"])

	locator, ok, err := repo.ReadWorkspaceLocator(originalFolder)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "new-feature", locator.WorkspaceName)

	require.NoError(t, os.Chdir(originalFolder))
	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, "new-feature", status.Workspace)
	assert.Equal(t, originalFolder, status.Folder)
}

func TestWorkspaceCommand_RenameMainFailsWithRepoRenameGuidance(t *testing.T) {
	setupCoverageRepo(t, "wsmainrename")

	stdout, err := executeCommand(createTestRootCmd(), "workspace", "rename", "main", "trunk")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "main workspace is the repo root; use jvs repo rename to rename the folder.", err.Error())
}

func TestWorkspaceCommand_RenameRerunResumesPendingLocatorRewrite(t *testing.T) {
	repoPath, _ := setupCoverageRepo(t, "wspendingrename")
	require.NoError(t, os.WriteFile("file.txt", []byte("baseline"), 0644))
	saveID := createRootTestSavePoint(t, "baseline")
	newOut, err := executeCommand(createTestRootCmd(), "workspace", "new", "../folder-stays", "--from", saveID, "--name", "old-feature")
	require.NoError(t, err, newOut)
	originalFolder := testExternalWorkspacePath(repoPath, "folder-stays")

	require.NoError(t, worktree.NewManager(repoPath).Rename("old-feature", "new-feature"))
	repoID, err := workspaceCurrentRepoID(repoPath)
	require.NoError(t, err)
	record := workspaceLifecycleOperationRecord(repoID, workspaceRenameOperationID("old-feature", "new-feature"), "workspace rename", "config_renamed", "jvs workspace rename old-feature new-feature", map[string]any{
		"old_workspace":   "old-feature",
		"new_workspace":   "new-feature",
		"folder":          originalFolder,
		"locator_present": true,
	})
	require.NoError(t, lifecycle.WriteOperation(repoPath, record))

	require.NoError(t, os.Chdir(originalFolder))
	statusOut, err := executeCommand(createTestRootCmd(), "status")
	require.Error(t, err)
	assert.Empty(t, statusOut)

	require.NoError(t, os.Chdir(repoPath))
	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "rename", "old-feature", "new-feature")
	require.NoError(t, err, stdout)
	var data publicWorkspaceRenameResult
	decodeRootJSONData(t, stdout, &data)
	assert.Equal(t, "renamed", data.Status)
	assert.Equal(t, originalFolder, data.Folder)

	pending, err := lifecycle.ListPendingOperations(repoPath)
	require.NoError(t, err)
	assert.Empty(t, pending)
	locator, ok, err := repo.ReadWorkspaceLocator(originalFolder)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "new-feature", locator.WorkspaceName)
	require.NoError(t, os.Chdir(originalFolder))
	statusOut, err = executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	var status statusCommandOutput
	decodeRootJSONData(t, statusOut, &status)
	assert.Equal(t, "new-feature", status.Workspace)
}

func TestWorkspaceCommand_ListPathAndDelete(t *testing.T) {
	dir := setupTestDir(t)
	repoPath := filepath.Join(dir, "testrepo")
	require.NoError(t, os.Mkdir(repoPath, 0755))
	require.NoError(t, os.Chdir(repoPath))
	initOut, err := executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err, initOut)
	require.NoError(t, os.WriteFile("file.txt", []byte("baseline"), 0644))
	saveID := createRootTestSavePoint(t, "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "new", "../feature", "--from", saveID)
	require.NoError(t, err, stdout)

	stdout, err = executeCommand(createTestRootCmd(), "workspace", "list")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, "main")
	assert.Contains(t, stdout, "feature")

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "path", "feature")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var pathData map[string]string
	require.NoError(t, json.Unmarshal(env.Data, &pathData), stdout)
	assert.Equal(t, "feature", pathData["workspace"])
	assert.DirExists(t, pathData["path"])

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "feature")
	require.NoError(t, err, stdout)
	env = decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "workspace delete", env.Command)
	var preview map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &preview), stdout)
	planID, ok := preview["plan_id"].(string)
	require.True(t, ok, "workspace delete preview should expose plan_id: %#v", preview)
	assert.DirExists(t, pathData["path"])

	stdout, err = executeCommand(createTestRootCmd(), "--json", "workspace", "delete", "--run", planID)
	require.NoError(t, err, stdout)
	env = decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "workspace delete", env.Command)
	assert.NoDirExists(t, pathData["path"])
}

func TestRootCommand_JSONFlag(t *testing.T) {
	_, err := executeCommand(createTestRootCmd(), "--json", "--help")
	require.NoError(t, err)
	assert.True(t, jsonOutput)
}

func TestInitCommand_CreatesRepo(t *testing.T) {
	setupTestDir(t)
	require.NoError(t, os.Mkdir("testrepo", 0755))
	stdout, err := executeCommand(createTestRootCmd(), "init", "testrepo")
	require.NoError(t, err)
	assert.Contains(t, stdout, "JVS is ready for this folder.")
	assert.DirExists(t, "testrepo/.jvs")
	assert.NoDirExists(t, "testrepo/main")
}

func TestHistoryCommand_Empty(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "history")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No save points")
	assert.NotContains(t, stdout, "snapshot")
	assert.NotContains(t, stdout, "checkpoint")
}

func TestHistoryCommand_WithSavePoints(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))
	require.NoError(t, os.WriteFile("file.txt", []byte("one"), 0644))
	firstID := createRootTestSavePoint(t, "first save point")
	require.NoError(t, os.WriteFile("file.txt", []byte("two"), 0644))
	secondID := createRootTestSavePoint(t, "second save point")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "history")
	require.NoError(t, err, stdout)
	var history publicHistoryResult
	decodeRootJSONData(t, stdout, &history)
	require.Len(t, history.SavePoints, 2)
	assert.Equal(t, secondID, history.SavePoints[0].SavePointID)
	assert.Equal(t, firstID, history.SavePoints[1].SavePointID)
}

func TestRestoreCommand_RestoresSavePoint(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))
	require.NoError(t, os.WriteFile("file.txt", []byte("version1"), 0644))
	firstID := createRootTestSavePoint(t, "first save point")
	require.NoError(t, os.WriteFile("file.txt", []byte("version2"), 0644))
	createRootTestSavePoint(t, "second save point")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "restore", firstID)
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "restore", env.Command)
	var preview publicRestoreResult
	require.NoError(t, json.Unmarshal(env.Data, &preview), stdout)
	require.NotEmpty(t, preview.PlanID)

	stdout, err = executeCommand(createTestRootCmd(), "--json", "restore", "--run", preview.PlanID)
	require.NoError(t, err, stdout)
	content, err := os.ReadFile("file.txt")
	require.NoError(t, err)
	assert.Equal(t, "version1", string(content))
}

func TestRestoreHelp(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "restore", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "restore")
	assert.Contains(t, stdout, "save")
	assert.NotContains(t, stdout, "checkpoint")
	assert.NotContains(t, stdout, "snapshot")
}

func TestDoctorHelpUsesSavePointIntegrityVocabulary(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "doctor", "--help")
	require.NoError(t, err)

	assert.Contains(t, stdout, "save point integrity")
	assert.NotContains(t, stdout, "checkpoint")
	assert.NotContains(t, stdout, "snapshot")
	assert.NotContains(t, stdout, "worktree")
}

func TestDoctorCommand_Healthy(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "doctor")
	require.NoError(t, err)
	assert.Contains(t, stdout, "healthy")
	assert.NotContains(t, stdout, "worktree")
	assert.NotContains(t, stdout, "snapshot")
}

func TestDoctorCommand_Strict(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "doctor", "--strict")
	require.NoError(t, err)
	assert.Contains(t, stdout, "healthy")
}

func TestDoctorCommand_Repair(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "doctor", "--repair-runtime")
	require.NoError(t, err)
	assert.Contains(t, stdout, "healthy")
}

func TestDoctorJSONOutput(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "doctor")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "doctor", env.Command)
	assert.NotContains(t, string(env.Data), "worktree")
	assert.NotContains(t, string(env.Data), "snapshot")
}

func TestCleanupCommand_Preview(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "cleanup", "preview")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan ID:")
	assert.Contains(t, stdout, "save points")
	assert.Contains(t, stdout, "jvs cleanup run --plan-id")
	assert.NotContains(t, stdout, "GC")
	assert.NotContains(t, stdout, "gc")
	assert.NotContains(t, stdout, "checkpoint")
}

func TestCleanupCommand_RejectsExtraArgs(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	_, err := executeCommand(createTestRootCmd(), "cleanup", "preview", "extra")
	require.Error(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)
	var plan publicCleanupPlan
	decodeRootJSONData(t, stdout, &plan)
	require.NotEmpty(t, plan.PlanID)

	_, err = executeCommand(createTestRootCmd(), "cleanup", "run", "--plan-id", plan.PlanID, "extra")
	require.Error(t, err)
}

func TestCleanupCommand_PreviewJSON(t *testing.T) {
	setupTestDir(t)
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	stdout, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "cleanup preview", env.Command)
	assert.Contains(t, string(env.Data), "protected_save_points")
	assert.NotContains(t, string(env.Data), "protected_checkpoints")
	assert.NotContains(t, string(env.Data), "gc")
}

func TestCompletionCommand(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "completion", "bash")
	require.NoError(t, err)
	assert.Contains(t, stdout, "bash completion")
}

func TestCompletionCommandHelp(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "completion", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "completion")
}

func TestCompletionCommandValidArgs(t *testing.T) {
	cmd := createTestRootCmd()
	completion := findChildCommand(t, cmd, "completion")
	args := completion.ValidArgs
	assert.Contains(t, args, "bash")
	assert.Contains(t, args, "zsh")
	assert.Contains(t, args, "fish")
	assert.Contains(t, args, "powershell")
}

func TestHumanOutputPolicyDisablesANSIInCI(t *testing.T) {
	unsetEnvForCLITest(t, "NO_COLOR")
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("TERM", "xterm-256color")
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	saveID := savePointIDFromCLI(t, "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "status")
	require.NoError(t, err)

	assert.Contains(t, stdout, "Newest save point: "+saveID)
	assertNoANSI(t, stdout)
}

func TestHumanErrorPolicyDisablesANSIWhenStderrIsNonTerminal(t *testing.T) {
	unsetEnvForCLITest(t, "NO_COLOR")
	unsetEnvForCLITest(t, "CI")
	unsetEnvForCLITest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	setupTestDir(t)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "status")
	require.Error(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "not a JVS repository")
	assertNoANSI(t, stderr)
}

func TestJSONErrorNotInRepoHintIsPlainWhenStdoutAndStderrAreInteractive(t *testing.T) {
	unsetEnvForCLITest(t, "NO_COLOR")
	unsetEnvForCLITest(t, "CI")
	unsetEnvForCLITest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)
	t.Cleanup(func() { color.Init(true) })
	color.Init(false)
	setupTestDir(t)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--json", "status")
	require.Error(t, err)

	assert.Empty(t, stderr)
	assertNoANSI(t, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "Run jvs init <name> to create a new repository.", env.Error.Hint)
	assertNoANSI(t, env.Error.Hint)
}

func TestNoColorAppliesToValidationErrorBeforePersistentPreRun(t *testing.T) {
	unsetEnvForCLITest(t, "NO_COLOR")
	unsetEnvForCLITest(t, "CI")
	unsetEnvForCLITest(t, "GITHUB_ACTIONS")
	t.Setenv("TERM", "xterm-256color")
	restoreTerminal := terminal.WithIsTerminalForTest(func(*os.File) bool { return true })
	t.Cleanup(restoreTerminal)
	t.Cleanup(func() { color.Init(true) })
	color.Init(false)

	stdout, stderr, err := executeCommandWithErrorReport(createTestRootCmd(), "--no-color", "nosuch")
	require.Error(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `unknown command "nosuch"`)
	assertNoANSI(t, stderr)
}

func findChildCommand(t *testing.T, cmd *cobra.Command, name string) *cobra.Command {
	t.Helper()

	for _, child := range cmd.Commands() {
		if child.Name() == name {
			return child
		}
	}
	t.Fatalf("command %q not found", name)
	return nil
}

func createRootTestSavePoint(t *testing.T, note string) string {
	t.Helper()

	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", note)
	require.NoError(t, err, stdout)
	var saved publicSavePointCreatedRecord
	decodeRootJSONData(t, stdout, &saved)
	require.NotEmpty(t, saved.SavePointID)
	return saved.SavePointID
}

func decodeRootJSONData(t *testing.T, stdout string, target any) contractEnvelope {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NoError(t, json.Unmarshal(env.Data, target), stdout)
	return env
}

func assertNoANSI(t *testing.T, value string) {
	t.Helper()
	assert.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`), value)
}

func unsetEnvForCLITest(t *testing.T, key string) {
	t.Helper()
	original, exists := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if exists {
			require.NoError(t, os.Setenv(key, original))
			return
		}
		require.NoError(t, os.Unsetenv(key))
	})
}

func createTestRootCmd() *cobra.Command {
	jsonOutput = false
	debugOutput = false
	noProgress = false
	noColor = false
	autoProgressEnabled = defaultAutoProgressEnabled
	targetRepoPath = ""
	targetWorkspaceName = ""
	activeCommand = nil
	activeCommandName = ""
	activeCommandArgs = nil
	resolvedRepoRoot = ""
	resolvedWorkspace = ""
	jsonErrorEmitted = false
	workspaceRenameDryRun = false
	workspaceDeleteRunID = ""
	workspaceMoveRunID = ""
	workspaceNewFromRef = ""
	workspaceNewName = ""
	workspaceListStatus = false
	repoCloneSavePoints = "all"
	repoCloneDryRun = false
	repoMoveRunID = ""
	repoRenameRunID = ""
	repoDetachRunID = ""
	historyLimit = defaultHistoryLimit
	historyNoteFilter = ""
	historyPath = ""
	saveMessage = ""
	restoreInteractive = false
	restoreDiscardDirty = false
	restoreIncludeWorking = false
	restorePath = ""
	restoreRunPlanID = ""
	cleanupPlanID = ""
	doctorStrict = false
	doctorRepair = false
	doctorRepairList = false

	cmd := &cobra.Command{
		Use:              "jvs",
		Short:            "JVS - Juicy Versioned Workspaces",
		Long:             publicRootLong,
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRun: cliPersistentPreRun,
	}
	installPublicRootHelpSurface(cmd)
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.PersistentFlags().BoolVar(&debugOutput, "debug", false, "enable debug logging")
	cmd.PersistentFlags().BoolVar(&noProgress, "no-progress", false, "disable progress bars")
	cmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output (also respects NO_COLOR env var)")
	cmd.PersistentFlags().StringVar(&targetRepoPath, "repo", "", "target repository root or path inside a repository")
	cmd.PersistentFlags().StringVar(&targetWorkspaceName, "workspace", "", "target workspace name")

	cmd.AddCommand(initCmd)
	cmd.AddCommand(saveCmd)
	cmd.AddCommand(statusCmd)
	cmd.AddCommand(viewCmd)
	cmd.AddCommand(repoCmd)
	cmd.AddCommand(workspaceCmd)
	cmd.AddCommand(historyCmd)
	cmd.AddCommand(restoreCmd)
	cmd.AddCommand(recoveryCmd)
	cmd.AddCommand(doctorCmd)
	cmd.AddCommand(cleanupCmd)
	cmd.AddCommand(completionCmd)
	configurePublicRootHelpSurface(cmd)

	return cmd
}
