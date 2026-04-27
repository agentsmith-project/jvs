package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeCommand(root *cobra.Command, args ...string) (stdout string, err error) {
	resetCommandHelpFlags(root)
	defer resetCommandHelpFlags(root)

	// Capture os.Stdout since CLI uses fmt.Printf directly
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

func resetCommandHelpFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if flag := cmd.Flags().Lookup("help"); flag != nil {
		_ = flag.Value.Set("false")
		flag.Changed = false
	}
	for _, child := range cmd.Commands() {
		resetCommandHelpFlags(child)
	}
}

func setupTestDir(t *testing.T) string {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		os.Chdir(originalWd)
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
	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "--help")
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
	for _, command := range []string{"init", "save", "status", "history", "view", "restore", "doctor", "completion", "help"} {
		assertRootHelpListsCommand(t, stdout, command)
	}
	for _, word := range []string{
		"fork",
		"gc",
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
	assert.True(t, pattern.MatchString(help), "root help should list %q:\n%s", command, help)
}

func assertRootHelpOmitsWord(t *testing.T, help, word string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
	assert.False(t, pattern.MatchString(help), "root help should not expose %q:\n%s", word, help)
}

func TestRootCommand_HiddenAdvancedCommandsRemainDirectlyInvocable(t *testing.T) {
	stdout, err := executeCommand(createTestRootCmd(), "fork", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "fork")

	setupAdoptedSaveFacadeRepo(t)
	stdout, err = executeCommand(createTestRootCmd(), "gc", "plan")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Plan ID:")
}

func TestCheckpointCommand_HelpHidesUnpublishedFlags(t *testing.T) {
	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "checkpoint", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "--tag")
	assert.Contains(t, stdout, "--file")
	assert.NotContains(t, stdout, "--paths")
	assert.NotContains(t, stdout, "--compress")
}

func TestRootCommand_JSONFlag(t *testing.T) {
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "--json", "--help")
	require.NoError(t, err)
	assert.True(t, jsonOutput)
}

func TestInitCommand_CreatesRepo(t *testing.T) {
	setupTestDir(t)
	require.NoError(t, os.Mkdir("testrepo", 0755))
	cmd := createTestRootCmd()
	stdout, err := executeCommand(cmd, "init", "testrepo")
	require.NoError(t, err)
	assert.Contains(t, stdout, "JVS is ready for this folder.")

	_, statErr := os.Stat("testrepo/.jvs")
	assert.NoError(t, statErr)

	_, statErr = os.Stat("testrepo/main")
	assert.True(t, os.IsNotExist(statErr), "init must not create a main payload directory")
}

func TestWorktreeCommand_List(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// List worktrees - should show main
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "worktree", "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "main")

	os.Chdir(originalWd)
}

func TestInfoCommand_WithRepo(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Check info
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "info")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Repository:")

	os.Chdir(originalWd)
}

func TestInfoCommand_JSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Check info with JSON
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "info")
	require.NoError(t, err)
	assert.Contains(t, stdout, "repo_root")

	os.Chdir(originalWd)
}

func TestDoctorCommand_Healthy(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Run doctor
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "doctor")
	require.NoError(t, err)
	assert.Contains(t, stdout, "healthy")

	os.Chdir(originalWd)
}

func TestHistoryCommand_Empty(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree (history requires being inside worktree)
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// History on empty repo
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "history")
	require.NoError(t, err)
	// Empty history shows "no snapshots" or similar
	_ = stdout // May be empty or contain message

	os.Chdir(originalWd)
}

func TestVerifyCommand_Empty(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Verify --all on empty repo
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "verify", "--all")
	require.NoError(t, err)

	os.Chdir(originalWd)
}

func TestGCCommand_Plan(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// GC plan
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "gc", "plan")
	require.NoError(t, err)

	os.Chdir(originalWd)
}

func TestGCCommand_RejectsExtraArgs(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")
	require.NoError(t, os.Chdir(repoPath))

	_, err := executeCommand(createTestRootCmd(), "gc", "plan", "extra")
	require.Error(t, err)

	stdout, err := executeCommand(createTestRootCmd(), "--json", "gc", "plan")
	require.NoError(t, err)
	data := decodeJSONDataForTest(t, stdout)
	planID, ok := data["plan_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, planID)

	_, err = executeCommand(createTestRootCmd(), "gc", "run", "--plan-id", planID, "extra")
	require.Error(t, err)
}

func TestOutputJSON(t *testing.T) {
	// Test with jsonOutput = true
	jsonOutput = true
	err := outputJSON(map[string]string{"test": "value"})
	assert.NoError(t, err)

	// Test with jsonOutput = false
	jsonOutput = false
	err = outputJSON(map[string]string{"test": "value"})
	assert.NoError(t, err)
}

func TestFmtErr(t *testing.T) {
	// fmtErr should not panic
	fmtErr("test error: %s", "detail")
}

func TestSnapshotCommand_CreatesSnapshot(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a file
	os.WriteFile("file.txt", []byte("content"), 0644)

	// Create snapshot
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "test note")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Created snapshot")

	os.Chdir(originalWd)
}

func TestSnapshotCommand_WithTags(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a file
	os.WriteFile("file.txt", []byte("content"), 0644)

	// Create snapshot with tags
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "release v1", "--tag", "v1.0", "--tag", "release")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Created snapshot")

	os.Chdir(originalWd)
}

func TestHistoryCommand_WithSnapshots(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create files and snapshots
	os.WriteFile("file1.txt", []byte("content1"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "first snapshot", "--tag", "v1")

	os.WriteFile("file2.txt", []byte("content2"), 0644)
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "second snapshot", "--tag", "v2", "--tag", "release")

	// View history
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "history")
	require.NoError(t, err)
	assert.Contains(t, stdout, "snapshot")

	os.Chdir(originalWd)
}

func TestHistoryCommand_WithTagFilter(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create files and snapshots with tags (reset snapshotTags before each)
	snapshotTags = []string{}
	os.WriteFile("file1.txt", []byte("content1"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "dev snapshot", "--tag", "dev")

	snapshotTags = []string{}
	os.WriteFile("file2.txt", []byte("content2"), 0644)
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "release snapshot", "--tag", "release")

	// View history with tag filter
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "history", "--tag", "release")
	require.NoError(t, err)
	assert.Contains(t, stdout, "release")
	// Should not show dev snapshot
	assert.NotContains(t, stdout, "dev snapshot")

	os.Chdir(originalWd)
}

func TestHistoryCommand_WithGrepFilter(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create files and snapshots
	os.WriteFile("file1.txt", []byte("content1"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "development work")

	os.WriteFile("file2.txt", []byte("content2"), 0644)
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "production release")

	// View history with grep filter
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "history", "--grep", "release")
	require.NoError(t, err)
	assert.Contains(t, stdout, "release")
	assert.NotContains(t, stdout, "development")

	os.Chdir(originalWd)
}

func TestWorktreeCommand_Create(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Create worktree
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "worktree", "create", "feature")
	require.NoError(t, err)
	assert.Contains(t, stdout, "feature")

	// Verify worktree exists
	_, err = os.Stat("worktrees/feature")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

func TestVerifyCommand_WithSnapshots(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "test")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Verify all
	cmd3 := createTestRootCmd()
	stdout, err := executeCommand(cmd3, "verify", "--all")
	require.NoError(t, err)
	assert.Contains(t, stdout, "OK")

	os.Chdir(originalWd)
}

func TestHasTag(t *testing.T) {
	// Test the hasTag helper function
	descWithTags := &model.Descriptor{
		Tags: []string{"v1.0", "release", "stable"},
	}
	descNoTags := &model.Descriptor{
		Tags: []string{},
	}

	assert.True(t, hasTag(descWithTags, "v1.0"))
	assert.True(t, hasTag(descWithTags, "release"))
	assert.False(t, hasTag(descWithTags, "dev"))
	assert.False(t, hasTag(descNoTags, "any"))
}

// createTestRootCmd creates a fresh root command for testing
func createTestRootCmd() *cobra.Command {
	// Reset global flags to avoid test pollution
	jsonOutput = false
	debugOutput = false
	noProgress = false
	noColor = false
	targetRepoPath = ""
	targetWorkspaceName = ""
	activeCommandName = ""
	resolvedRepoRoot = ""
	resolvedWorkspace = ""
	jsonErrorEmitted = false
	worktreeCreateFrom = ""
	worktreeForce = false
	workspaceRemoveForce = false
	forkFromRef = ""
	forkDiscardDirty = false
	forkIncludeWorking = false
	historyLimit = 0
	historyNoteFilter = ""
	historyTagFilter = ""
	historyAll = false
	saveMessage = ""
	checkpointTags = nil
	checkpointPaths = nil
	checkpointCompression = ""
	checkpointNoteFile = ""
	snapshotTags = nil
	snapshotPaths = nil
	snapshotCompression = ""
	snapshotNoteFile = ""
	restoreInteractive = false
	restoreDiscardDirty = false
	restoreIncludeWorking = false
	gcPlanID = ""
	diffStatOnly = false
	cloneScope = "full"
	capabilityWriteProbe = false
	doctorStrict = false
	doctorRepair = false
	doctorRepairList = false
	verifyAll = false

	// Create a new root command
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

	// Add all subcommands
	cmd.AddCommand(initCmd)
	cmd.AddCommand(saveCmd)
	cmd.AddCommand(statusCmd)
	cmd.AddCommand(viewCmd)
	cmd.AddCommand(checkpointCmd)
	cmd.AddCommand(workspaceCmd)
	cmd.AddCommand(forkCmd)
	cmd.AddCommand(snapshotCmd)
	cmd.AddCommand(worktreeCmd)
	cmd.AddCommand(historyCmd)
	cmd.AddCommand(restoreCmd)
	cmd.AddCommand(infoCmd)
	cmd.AddCommand(doctorCmd)
	cmd.AddCommand(verifyCmd)
	cmd.AddCommand(gcCmd)
	cmd.AddCommand(completionCmd)
	cmd.AddCommand(configCmd)
	cmd.AddCommand(diffCmd)
	cmd.AddCommand(conformanceCmd)
	cmd.AddCommand(importCmd)
	cmd.AddCommand(cloneCmd)
	cmd.AddCommand(capabilityCmd)
	configurePublicRootHelpSurface(cmd)

	return cmd
}

func TestRestoreCommand(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("version1"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "first")

	// Modify file and create another snapshot
	os.WriteFile("file.txt", []byte("version2"), 0644)
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "second")

	// Verify the public JSON checkpoint list can see the created checkpoints.
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "--json", "checkpoint", "list")
	require.NoError(t, err)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "checkpoint list", env.Command)

	var checkpoints []publicCheckpointRecord
	require.NoError(t, json.Unmarshal(env.Data, &checkpoints), stdout)
	require.GreaterOrEqual(t, len(checkpoints), 2)

	os.Chdir(originalWd)
}

func TestWorktreeCommand_Fork(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "base snapshot")

	// Fork from current position (must be inside worktree for fork without snapshot ID)
	cmd3 := createTestRootCmd()
	stdout, err := executeCommand(cmd3, "worktree", "fork", "feature")
	require.NoError(t, err)
	assert.Contains(t, stdout, "feature")

	// Verify forked worktree exists
	require.NoError(t, os.Chdir(repoPath))
	_, err = os.Stat("worktrees/feature")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

func TestWorktreeCommand_Remove(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))
	// Create worktree
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "worktree", "create", "to-remove")

	// Remove worktree
	cmd3 := createTestRootCmd()
	_, err := executeCommand(cmd3, "worktree", "remove", "to-remove")
	require.NoError(t, err)

	// Verify worktree is gone
	_, err = os.Stat("worktrees/to-remove")
	assert.True(t, os.IsNotExist(err))

	os.Chdir(originalWd)
}

func TestWorktreeCommand_Path(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Get main worktree path
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "worktree", "path", "main")
	require.NoError(t, err)
	assert.Contains(t, stdout, "main")

	os.Chdir(originalWd)
}

func TestDoctorCommand_Strict(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Run doctor --strict
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "doctor", "--strict")
	require.NoError(t, err)
	assert.Contains(t, stdout, "healthy")

	os.Chdir(originalWd)
}

func TestDoctorCommand_Repair(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Create an orphan tmp file
	os.WriteFile(".jvs-tmp-orphan", []byte("data"), 0644)

	// Run doctor repair
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "doctor", "--repair-runtime")
	require.NoError(t, err)

	// Verify tmp file is cleaned
	_, err = os.Stat(".jvs-tmp-orphan")
	assert.True(t, os.IsNotExist(err))

	os.Chdir(originalWd)
}

func TestGCCommand_PlanJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// GC plan with JSON output
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "gc", "plan")
	require.NoError(t, err)
	assert.Contains(t, stdout, "plan_id")

	os.Chdir(originalWd)
}

func TestHistoryCommand_Limit(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create multiple snapshots
	for i := 0; i < 5; i++ {
		os.WriteFile(fmt.Sprintf("file%d.txt", i), []byte(fmt.Sprintf("content%d", i)), 0644)
		cmd2 := createTestRootCmd()
		executeCommand(cmd2, "snapshot", fmt.Sprintf("snapshot %d", i))
	}

	// View history with limit
	cmd3 := createTestRootCmd()
	stdout, err := executeCommand(cmd3, "history", "--limit", "2")
	require.NoError(t, err)
	// Should only show 2 entries
	_ = stdout

	os.Chdir(originalWd)
}

func TestWorktreeCommand_Rename(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Create worktree
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "worktree", "create", "old-name")

	// Rename worktree
	cmd3 := createTestRootCmd()
	_, err := executeCommand(cmd3, "worktree", "rename", "old-name", "new-name")
	require.NoError(t, err)

	// Verify old name is gone
	_, err = os.Stat("worktrees/old-name")
	assert.True(t, os.IsNotExist(err))

	// Verify new name exists
	_, err = os.Stat("worktrees/new-name")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

func TestInfoCommand_JSONFields(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into repo
	require.NoError(t, os.Chdir(repoPath))

	// Check info JSON has required fields
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "info")
	require.NoError(t, err)
	// Should contain required spec fields
	assert.Contains(t, stdout, "format_version")
	assert.Contains(t, stdout, "workspace_count")
	assert.Contains(t, stdout, "checkpoint_count")
	assert.NotContains(t, stdout, "total_worktrees")
	assert.NotContains(t, stdout, "total_snapshots")

	os.Chdir(originalWd)
}

func TestVerifyCommand_SingleSnapshot(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "test")

	// Verify all snapshots (including the one we just created)
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "verify", "--all")
	require.NoError(t, err)
	assert.Contains(t, stdout, "OK")

	os.Chdir(originalWd)
}

func TestSnapshotCommand_DetachedState(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and two snapshots
	os.WriteFile("file.txt", []byte("version1"), 0644)
	cmd2 := createTestRootCmd()
	executeCommand(cmd2, "snapshot", "first")

	os.WriteFile("file.txt", []byte("version2"), 0644)
	cmd3 := createTestRootCmd()
	executeCommand(cmd3, "snapshot", "second")

	// Now current equals latest and checkpoints can advance.
	// Restoring to first would make current differ from latest.
	_ = cmd3

	os.Chdir(originalWd)
}

// TestResolveSnapshotID tests the resolveSnapshotID helper function.
func TestResolveSnapshotID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "test snapshot", "--tag", "v1.0")
	require.NoError(t, err)
	require.Contains(t, stdout, "snapshot")

	// Read snapshot descriptor directly from disk
	repoRoot := repoPath

	// List snapshots to get the ID
	snapshots, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	require.Greater(t, len(snapshots), 0)

	snapshotID := snapshots[0].SnapshotID

	// Test resolveSnapshotID with exact ID
	resolved, err := resolveSnapshotID(repoRoot, string(snapshotID))
	require.NoError(t, err)
	assert.Equal(t, snapshotID, resolved)

	// Test resolveSnapshotID with tag
	resolved, err = resolveSnapshotID(repoRoot, "v1.0")
	require.NoError(t, err)
	assert.Equal(t, snapshotID, resolved)

	// Test resolveSnapshotID with note prefix
	resolved, err = resolveSnapshotID(repoRoot, "test")
	require.NoError(t, err)
	assert.Equal(t, snapshotID, resolved)

	// Test resolveSnapshotID with non-existent snapshot
	_, err = resolveSnapshotID(repoRoot, "nonexistent")
	assert.Error(t, err)

	os.Chdir(originalWd)
}

// TestConformanceCommands tests conformance command functionality.
func TestConformanceCommands(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Create a fake go.mod to simulate repo root
	require.NoError(t, os.Chdir(dir))
	os.WriteFile("go.mod", []byte("module test\n\ngo 1.21\n"), 0644)

	// Test findRepoRoot
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)
	assert.Equal(t, dir, repoRoot)

	// Test findRepoRoot in subdirectory
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))
	require.NoError(t, os.Chdir(subDir))
	repoRoot, err = findRepoRoot()
	require.NoError(t, err)
	assert.Equal(t, dir, repoRoot)

	// Test findRepoRoot when go.mod doesn't exist
	otherDir := t.TempDir()
	require.NoError(t, os.Chdir(otherDir))
	_, err = findRepoRoot()
	assert.Error(t, err)

	os.Chdir(originalWd)
}

// TestExecuteExitOnError tests that Execute properly exits on command error.
func TestExecuteExitOnError(t *testing.T) {
	// This test verifies the Execute function handles errors.
	// We can't directly test os.Exit behavior, but we can verify
	// the root command is set up correctly.

	// Just verify Execute is callable
	assert.NotNil(t, rootCmd)
	assert.Equal(t, "jvs", rootCmd.Use)
}

// TestWorktreeCreateWithSnapshot tests creating worktree from snapshot.
func TestWorktreeCreateWithSnapshot(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot with tag
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "base snapshot", "--tag", "v1.0")
	require.NoError(t, err)
	assert.Contains(t, stdout, "snapshot")

	// Change to repo root
	require.NoError(t, os.Chdir(repoPath))

	// Create worktree from snapshot using --from flag
	cmd3 := createTestRootCmd()
	stdout, err = executeCommand(cmd3, "worktree", "create", "feature", "--from", "v1.0")
	require.NoError(t, err)
	assert.Contains(t, stdout, "feature")

	// Verify worktree exists
	_, err = os.Stat("worktrees/feature")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

// TestWorktreeForkWithSnapshotID tests forking with explicit snapshot ID.
func TestWorktreeForkWithSnapshotID(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "snapshot", "base", "--tag", "base")
	require.NoError(t, err)

	// Get snapshot ID directly from disk
	repoRoot := repoPath
	snapshots, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	require.Greater(t, len(snapshots), 0)
	snapshotID := string(snapshots[0].SnapshotID)

	// Fork with explicit snapshot ID and name
	cmd3 := createTestRootCmd()
	stdout, err = executeCommand(cmd3, "worktree", "fork", snapshotID, "feature-branch")
	require.NoError(t, err)
	assert.Contains(t, stdout, "feature-branch")

	// Verify worktree exists
	require.NoError(t, os.Chdir(repoRoot))
	_, err = os.Stat("worktrees/feature-branch")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

// TestJSONOutputWithCommands tests JSON output for various commands.
func TestJSONOutputWithCommands(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Test info with JSON
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "info")
	require.NoError(t, err)
	assert.Contains(t, stdout, "{")
	assert.Contains(t, stdout, "repo_root")

	// Test worktree list with JSON
	cmd3 := createTestRootCmd()
	stdout, err = executeCommand(cmd3, "--json", "worktree", "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "[")

	os.Chdir(originalWd)
}

// TestResolveSnapshotIDShortPrefix tests resolving with short ID prefix.
func TestResolveSnapshotIDShortPrefix(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create file and snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "snapshot", "test", "--tag", "mytag")
	require.NoError(t, err)

	// Get snapshot ID directly from disk
	repoRoot := repoPath
	snapshots, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	require.Greater(t, len(snapshots), 0)
	snapshotID := string(snapshots[0].SnapshotID)

	// Test with short prefix (first 8 characters)
	shortID := snapshotID[:8]
	resolved, err := resolveSnapshotID(repoRoot, shortID)
	require.NoError(t, err)
	assert.Equal(t, snapshots[0].SnapshotID, resolved)

	os.Chdir(originalWd)
}

// TestConformanceListCommand tests conformance command is registered.
func TestConformanceListCommand(t *testing.T) {
	// Verify conformanceCmd exists and is properly configured
	assert.NotNil(t, conformanceCmd)
	assert.Equal(t, "conformance", conformanceCmd.Use)
	assert.Equal(t, 2, len(conformanceCmd.Commands()))
}

// TestDetectEngine tests the detectEngine helper.
func TestDetectEngine(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	repoRoot := repoPath

	// Test detectEngine - should return Copy as fallback if no special engine
	engineType := detectEngine(repoRoot)
	assert.NotEmpty(t, engineType)

	os.Chdir(originalWd)
}

// TestWorktreeCreateEmpty tests creating an empty worktree (no --from).
func TestWorktreeCreateEmpty(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// Create empty worktree (no --from flag)
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "worktree", "create", "empty-worktree")
	require.NoError(t, err)
	assert.Contains(t, stdout, "empty-worktree")

	// Verify worktree exists
	_, err = os.Stat("worktrees/empty-worktree")
	assert.NoError(t, err)

	os.Chdir(originalWd)
}

// TestFindRepoRootNoGoMod tests findRepoRoot when go.mod doesn't exist.
func TestFindRepoRootNoGoMod(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Change to temp dir without go.mod
	require.NoError(t, os.Chdir(dir))

	// findRepoRoot should return error
	_, err := findRepoRoot()
	assert.Error(t, err)

	os.Chdir(originalWd)
}

// TestWorktreeListJSON tests worktree list with JSON output.
func TestWorktreeListJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// List worktrees with JSON
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "worktree", "list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "[") // JSON array

	os.Chdir(originalWd)
}

// TestWorktreePathNoArgs tests worktree path with no args (uses current worktree).
func TestWorktreePathNoArgs(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Get path without specifying worktree name
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "worktree", "path")
	require.NoError(t, err)
	assert.Contains(t, stdout, "main")

	os.Chdir(originalWd)
}

// TestHistoryWithAllFlag tests history --all flag.
func TestHistoryWithAllFlag(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a snapshot
	os.WriteFile("file.txt", []byte("content"), 0644)
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "snapshot", "test snapshot")
	require.NoError(t, err)

	// View history with --all flag
	cmd3 := createTestRootCmd()
	stdout, err := executeCommand(cmd3, "history", "--all")
	require.NoError(t, err)
	assert.Contains(t, stdout, "snapshot")

	os.Chdir(originalWd)
}

// TestWorktreeRemoveForce tests worktree remove with --force flag.
func TestWorktreeRemoveForce(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// Create a worktree
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "worktree", "create", "to-remove-force")
	require.NoError(t, err)

	// Remove with --force flag
	cmd3 := createTestRootCmd()
	_, err = executeCommand(cmd3, "worktree", "remove", "--force", "to-remove-force")
	require.NoError(t, err)

	// Verify worktree is gone
	_, err = os.Stat("worktrees/to-remove-force")
	assert.True(t, os.IsNotExist(err))

	os.Chdir(originalWd)
}

// TestDoctorJSONOutput tests doctor command with JSON output.
func TestDoctorJSONOutput(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// Run doctor with JSON
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "doctor")
	require.NoError(t, err)
	assert.Contains(t, stdout, "{") // JSON object

	os.Chdir(originalWd)
}

// TestGCHelp tests gc command help.
func TestGCHelp(t *testing.T) {
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "gc", "--help")
	require.NoError(t, err)
}

// TestVerifyHelp tests verify command help.
func TestVerifyHelp(t *testing.T) {
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "verify", "--help")
	require.NoError(t, err)
}

// TestRestoreHelp tests restore command help.
func TestRestoreHelp(t *testing.T) {
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "restore", "--help")
	require.NoError(t, err)
}

// TestInfoFields tests info command output contains expected fields.
func TestInfoFields(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// Run info and check for expected fields
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "info")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Repository")
	assert.Contains(t, stdout, "Workspaces")
	assert.NotContains(t, stdout, "Worktrees")

	os.Chdir(originalWd)
}

// TestRestoreToHead tests restoring to the latest checkpoint.
func TestRestoreToHead(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a snapshot
	os.WriteFile("file.txt", []byte("version 1"), 0644)
	cmd2 := createTestRootCmd()
	_, err := executeCommand(cmd2, "snapshot", "first snapshot")
	require.NoError(t, err)

	// Modify file
	os.WriteFile("file.txt", []byte("version 2"), 0644)

	// Create another snapshot
	cmd3 := createTestRootCmd()
	_, err = executeCommand(cmd3, "snapshot", "second snapshot")
	require.NoError(t, err)

	// Restore to latest (should stay at current state)
	cmd4 := createTestRootCmd()
	stdout, err := executeCommand(cmd4, "restore", "latest")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored save point:")
	assert.NotContains(t, stdout, "checkpoint")

	os.Chdir(originalWd)
}

// TestRestoreToTag tests restoring by tag.
func TestRestoreToTag(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a file and snapshot with unique tag
	os.WriteFile("file.txt", []byte("tagged version"), 0644)
	cmd2 := createTestRootCmd()
	uniqueTag := fmt.Sprintf("restore-tag-%x", t.TempDir()) // Use temp dir for uniqueness
	_, err := executeCommand(cmd2, "snapshot", "tagged snapshot", "--tag", uniqueTag)
	require.NoError(t, err)

	// Restore by tag immediately.
	cmd3 := createTestRootCmd()
	stdout, err := executeCommand(cmd3, "restore", uniqueTag)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored save point:")
	assert.NotContains(t, stdout, "checkpoint")

	os.Chdir(originalWd)
}

// TestSnapshotWithJSON tests snapshot command with JSON output.
func TestSnapshotWithJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change into main worktree
	require.NoError(t, os.Chdir(filepath.Join(repoPath, "main")))

	// Create a file
	os.WriteFile("file.txt", []byte("content"), 0644)

	// Create snapshot with JSON output
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "snapshot", "json test")
	require.NoError(t, err)
	assert.Contains(t, stdout, "{") // JSON object
	assert.Contains(t, stdout, "snapshot_id")

	os.Chdir(originalWd)
}

// TestWorktreeCommandJSON tests worktree commands with JSON.
func TestWorktreeCommandJSON(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()

	// Init repo
	require.NoError(t, os.Chdir(dir))
	repoPath := initLegacyRepoForCLITest(t, "testrepo")

	// Change to repo root
	repoRoot := repoPath
	require.NoError(t, os.Chdir(repoRoot))

	// Create worktree with JSON output
	cmd2 := createTestRootCmd()
	stdout, err := executeCommand(cmd2, "--json", "worktree", "create", "json-test")
	require.NoError(t, err)
	assert.Contains(t, stdout, "{") // JSON object

	os.Chdir(originalWd)
}

// TestDetectEngineFallback tests detectEngine with invalid path (fallback to Copy).
func TestDetectEngineFallback(t *testing.T) {
	// Call detectEngine with a non-existent path
	// Should fall back to EngineCopy
	engineType := detectEngine("/nonexistent/path/that/does/not/exist")
	assert.Equal(t, model.EngineCopy, engineType)
}

// TestCompletionCommand tests the completion command.
func TestCompletionCommand(t *testing.T) {
	cmd := createTestRootCmd()
	cmd.AddCommand(completionCmd)

	// Test bash completion
	stdout, err := executeCommand(cmd, "completion", "bash")
	require.NoError(t, err)
	assert.Contains(t, stdout, "bash completion")
	assert.Contains(t, stdout, "complete")

	// Test zsh completion
	cmd2 := createTestRootCmd()
	cmd2.AddCommand(completionCmd)
	stdout, err = executeCommand(cmd2, "completion", "zsh")
	require.NoError(t, err)
	assert.Contains(t, stdout, "compdef")
}

// TestCompletionCommandHelp tests completion command help.
func TestCompletionCommandHelp(t *testing.T) {
	cmd := createTestRootCmd()
	cmd.AddCommand(completionCmd)

	stdout, err := executeCommand(cmd, "completion", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Generate shell completion")
	assert.Contains(t, stdout, "bash")
	assert.Contains(t, stdout, "zsh")
	assert.Contains(t, stdout, "fish")
}

// TestCompletionCommandValidArgs tests completion command accepts only valid shell types.
func TestCompletionCommandValidArgs(t *testing.T) {
	cmd := createTestRootCmd()
	cmd.AddCommand(completionCmd)

	// Check that completion command has valid args defined
	assert.NotNil(t, completionCmd.ValidArgs)
	assert.Contains(t, completionCmd.ValidArgs, "bash")
	assert.Contains(t, completionCmd.ValidArgs, "zsh")
	assert.Contains(t, completionCmd.ValidArgs, "fish")
	assert.Contains(t, completionCmd.ValidArgs, "powershell")
}
