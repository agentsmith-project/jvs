package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkpointCommandOutput struct {
	CheckpointID string `json:"checkpoint_id"`
}

type statusCommandOutput struct {
	Current       string   `json:"current"`
	Latest        string   `json:"latest"`
	Dirty         bool     `json:"dirty"`
	AtLatest      bool     `json:"at_latest"`
	Workspace     string   `json:"workspace"`
	Repo          string   `json:"repo"`
	Engine        string   `json:"engine"`
	RecoveryHints []string `json:"recovery_hints"`
}

func setupPublicCLIRepo(t *testing.T, name string) (repoPath string, mainPath string) {
	t.Helper()

	dir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err = executeCommand(cmd, "init", name)
	require.NoError(t, err)

	repoPath = filepath.Join(dir, name)
	mainPath = filepath.Join(repoPath, "main")
	require.NoError(t, os.Chdir(mainPath))
	return repoPath, mainPath
}

func runPublicCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := createTestRootCmd()
	return executeCommand(cmd, args...)
}

func createCheckpointForPublicCLI(t *testing.T, note string, args ...string) string {
	t.Helper()

	allArgs := append([]string{"--json", "checkpoint", note}, args...)
	stdout, err := runPublicCLI(t, allArgs...)
	require.NoError(t, err, stdout)

	var out checkpointCommandOutput
	decodePublicData(t, stdout, &out)
	require.NotEmpty(t, out.CheckpointID)
	return out.CheckpointID
}

func readStatusForPublicCLI(t *testing.T) statusCommandOutput {
	t.Helper()

	stdout, err := runPublicCLI(t, "--json", "status")
	require.NoError(t, err, stdout)

	var out statusCommandOutput
	decodePublicData(t, stdout, &out)
	return out
}

func decodePublicData(t *testing.T, stdout string, target any) contractEnvelope {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NoError(t, json.Unmarshal(env.Data, target), stdout)
	return env
}

func TestPublicCLIStatusAndCheckpointCleanliness(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "statusrepo")

	require.NoError(t, os.WriteFile("file.txt", []byte("before checkpoint"), 0644))
	dirty := readStatusForPublicCLI(t)
	assert.True(t, dirty.Dirty)
	assert.False(t, dirty.AtLatest)
	assert.Equal(t, "main", dirty.Workspace)
	assert.Equal(t, repoPath, dirty.Repo)
	assert.NotEmpty(t, dirty.Engine)
	assert.NotEmpty(t, dirty.RecoveryHints)

	id := createCheckpointForPublicCLI(t, "first checkpoint")
	clean := readStatusForPublicCLI(t)
	assert.False(t, clean.Dirty)
	assert.True(t, clean.AtLatest)
	assert.Equal(t, id, clean.Current)
	assert.Equal(t, id, clean.Latest)
	assert.Equal(t, "main", clean.Workspace)
	assert.Equal(t, repoPath, clean.Repo)
	assert.NotEmpty(t, clean.Engine)
}

func TestPublicCLIStatusTreatsRootReadyAsDirtyOrReserved(t *testing.T) {
	setupPublicCLIRepo(t, "reservedstatus")

	require.NoError(t, os.WriteFile("file.txt", []byte("before checkpoint"), 0644))
	createCheckpointForPublicCLI(t, "baseline")
	require.NoError(t, os.WriteFile(".READY", []byte("user data"), 0644))

	stdout, err := runPublicCLI(t, "--json", "status")
	if err != nil {
		assert.Contains(t, err.Error(), "reserved")
		return
	}

	var status statusCommandOutput
	decodePublicData(t, stdout, &status)
	assert.True(t, status.Dirty, "root .READY must not be reported clean")
}

func TestPublicCLIDirtyRestoreRequiresExplicitChoice(t *testing.T) {
	setupPublicCLIRepo(t, "dirtyrestore")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")

	_, err := runPublicCLI(t, "--json", "restore", first)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("file.txt", []byte("local edit"), 0644))

	stdout, err := runPublicCLI(t, "restore", "latest")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "dirty")
	content, readErr := os.ReadFile("file.txt")
	require.NoError(t, readErr)
	assert.Equal(t, "local edit", string(content))

	stdout, err = runPublicCLI(t, "--json", "restore", "latest", "--discard-dirty")
	require.NoError(t, err, stdout)
	var restored map[string]any
	decodePublicData(t, stdout, &restored)
	assert.Equal(t, "restored", restored["status"])
	assert.Equal(t, second, restored["checkpoint_id"])
	assert.Equal(t, second, restored["current"])
	assert.Equal(t, second, restored["latest"])
	assert.Equal(t, false, restored["dirty"])
	assert.Equal(t, true, restored["at_latest"])
	content, readErr = os.ReadFile("file.txt")
	require.NoError(t, readErr)
	assert.Equal(t, "v2", string(content))
}

func TestPublicCLIDirtyRestoreIncludeWorkingCheckpointsFirst(t *testing.T) {
	setupPublicCLIRepo(t, "includeonrestore")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")
	require.NoError(t, os.WriteFile("file.txt", []byte("local edit"), 0644))

	stdout, err := runPublicCLI(t, "--json", "restore", first, "--include-working")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, `"checkpoint_id": "`+first+`"`)

	status := readStatusForPublicCLI(t)
	assert.False(t, status.Dirty)
	assert.False(t, status.AtLatest)
	assert.Equal(t, first, status.Current)
	assert.NotEqual(t, second, status.Latest)

	stdout, err = runPublicCLI(t, "--json", "checkpoint", "list")
	require.NoError(t, err, stdout)
	var checkpoints []checkpointCommandOutput
	decodePublicData(t, stdout, &checkpoints)
	assert.Len(t, checkpoints, 3)
}

func TestPublicCLIDirtyForkRequiresExplicitChoice(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "dirtyfork")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	stdout, err := runPublicCLI(t, "fork", "branch")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "dirty")

	stdout, err = runPublicCLI(t, "--json", "fork", "branch", "--discard-dirty")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, `"workspace": "branch"`)
	content, readErr := os.ReadFile(filepath.Join(repoPath, "worktrees", "branch", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "clean", string(content))

	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))
	stdout, err = runPublicCLI(t, "--json", "fork", "working-branch", "--include-working")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, `"workspace": "working-branch"`)
	content, readErr = os.ReadFile(filepath.Join(repoPath, "worktrees", "working-branch", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
}

func TestLegacyWorktreeForkDirtyNoCheckpointRequiresExplicitChoice(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacydirtyfreshfork")

	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	destPath := filepath.Join(repoPath, "worktrees", "branch")
	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "worktree", "fork", "branch")
	assert.Equal(t, 1, exitCode, "dirty legacy fork unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "dirty")
	assert.Contains(t, stderr, "--include-working")
	assert.Contains(t, stderr, "--discard-dirty")
	assert.NotContains(t, stderr, "no checkpoints")
	_, statErr := os.Stat(destPath)
	assert.True(t, os.IsNotExist(statErr), "destination workspace should not exist: %v", statErr)

	content, readErr := os.ReadFile("file.txt")
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
}

func TestLegacyWorktreeForkIncludeWorkingNoCheckpoint(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacyincludefreshfork")

	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "worktree", "fork", "working-branch", "--include-working")
	require.Equal(t, 0, exitCode, "legacy include-working fork failed: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Contains(t, stdout, `"name": "working-branch"`)

	content, readErr := os.ReadFile(filepath.Join(repoPath, "worktrees", "working-branch", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
	status := readStatusForPublicCLI(t)
	assert.False(t, status.Dirty)
	assert.NotEmpty(t, status.Current)
	assert.Equal(t, status.Current, status.Latest)
}

func TestLegacyWorktreeForkRejectsDirtyWorkspaceByDefault(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacydirtyfork")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	destPath := filepath.Join(repoPath, "worktrees", "legacy-branch")
	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "worktree", "fork", "legacy-branch")
	assert.Equal(t, 1, exitCode, "dirty legacy fork unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "dirty")
	_, statErr := os.Stat(destPath)
	assert.True(t, os.IsNotExist(statErr), "destination workspace should not exist: %v", statErr)

	content, readErr := os.ReadFile("file.txt")
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
}

func TestLegacyWorktreeForkDirtyRequiresExplicitChoice(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacydirtyforkchoice")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	destPath := filepath.Join(repoPath, "worktrees", "legacy-choice")
	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "worktree", "fork", "legacy-choice")
	assert.Equal(t, 1, exitCode, "dirty legacy fork unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "--include-working")
	assert.Contains(t, stderr, "--discard-dirty")
	_, statErr := os.Stat(destPath)
	assert.True(t, os.IsNotExist(statErr), "destination workspace should not exist: %v", statErr)

	help, err := runPublicCLI(t, "worktree", "fork", "--help")
	require.NoError(t, err, help)
	assert.Contains(t, help, "--include-working")
	assert.Contains(t, help, "--discard-dirty")
}

func TestLegacyWorktreeForkDiscardDirty(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacydiscardfork")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "worktree", "fork", "discard-branch", "--discard-dirty")
	require.Equal(t, 0, exitCode, "legacy discard fork failed: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Contains(t, stdout, `"name": "discard-branch"`)

	content, readErr := os.ReadFile(filepath.Join(repoPath, "worktrees", "discard-branch", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "clean", string(content))
	content, readErr = os.ReadFile("file.txt")
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
}

func TestLegacyWorktreeForkIncludeWorking(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacyincludefork")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	require.NoError(t, os.WriteFile("file.txt", []byte("working"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "--json", "worktree", "fork", "working-branch", "--include-working")
	require.Equal(t, 0, exitCode, "legacy include-working fork failed: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Contains(t, stdout, `"name": "working-branch"`)

	content, readErr := os.ReadFile(filepath.Join(repoPath, "worktrees", "working-branch", "file.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "working", string(content))
	status := readStatusForPublicCLI(t)
	assert.False(t, status.Dirty)
}

func TestPublicCLIRefResolverConflictsAndNoFuzzyNotes(t *testing.T) {
	setupPublicCLIRepo(t, "refrepo")

	require.NoError(t, os.WriteFile("file.txt", []byte("one"), 0644))
	first := createCheckpointForPublicCLI(t, "needle note", "--tag", "shared")
	require.NoError(t, os.WriteFile("file.txt", []byte("two"), 0644))
	createCheckpointForPublicCLI(t, "second note", "--tag", "shared")

	for _, reserved := range []string{"current", "latest", "dirty"} {
		stdout, err := runPublicCLI(t, "checkpoint", "reserved tag", "--tag", reserved)
		require.Error(t, err, reserved)
		assert.Empty(t, stdout)
		assert.Contains(t, err.Error(), "reserved")
	}

	stdout, err := runPublicCLI(t, "--json", "diff", "needle note", "latest")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "not found")

	stdout, err = runPublicCLI(t, "--json", "diff", "shared", "latest")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "ambiguous")

	uniquePrefix := first[:len(first)-2]
	stdout, err = runPublicCLI(t, "--json", "diff", uniquePrefix, uniquePrefix)
	require.NoError(t, err, stdout)
	decodeContractEnvelope(t, stdout)
}

func TestPublicCLIWorkspaceForkAmbiguousOneArgRef(t *testing.T) {
	setupPublicCLIRepo(t, "forkambiguity")

	require.NoError(t, os.WriteFile("file.txt", []byte("base"), 0644))
	createCheckpointForPublicCLI(t, "base", "--tag", "release")

	stdout, err := runPublicCLI(t, "fork", "release")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "ambiguous")

	stdout, err = runPublicCLI(t, "--json", "fork", "release", "release-workspace")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, `"workspace": "release-workspace"`)
}

func TestPublicCLIReservedRefsCannotBeWorkspaceNames(t *testing.T) {
	setupPublicCLIRepo(t, "reservedworkspaces")

	require.NoError(t, os.WriteFile("file.txt", []byte("base"), 0644))
	createCheckpointForPublicCLI(t, "base")
	stdout, err := runPublicCLI(t, "--json", "fork", "feature")
	require.NoError(t, err, stdout)

	for _, reserved := range []string{"current", "latest", "dirty"} {
		t.Run("rename_"+reserved, func(t *testing.T) {
			stdout, err := runPublicCLI(t, "workspace", "rename", "feature", reserved)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), "reserved")
		})

		t.Run("fork_"+reserved, func(t *testing.T) {
			stdout, err := runPublicCLI(t, "fork", reserved)
			require.Error(t, err)
			assert.Empty(t, stdout)
			assert.Contains(t, err.Error(), "reserved")
		})
	}

	stdout, err = runPublicCLI(t, "--json", "fork", "latest", "latest-feature")
	require.NoError(t, err, stdout)
	assert.Contains(t, stdout, `"workspace": "latest-feature"`)
}

func TestLegacyWorktreeRemoveRejectsDirtyAtLatestWorkspace(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacydirtyremove")

	require.NoError(t, os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")
	stdout, err := runPublicCLI(t, "fork", "feature")
	require.NoError(t, err, stdout)

	featureFile := filepath.Join(repoPath, "worktrees", "feature", "file.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("dirty"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, mainPath, "worktree", "remove", "feature")
	require.Equal(t, 1, exitCode, "dirty legacy remove unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "dirty")
	require.FileExists(t, featureFile)
}

func TestPublicHelpHidesLegacyVocabulary(t *testing.T) {
	for _, args := range [][]string{
		{"init", "--help"},
		{"doctor", "--help"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, err := runPublicCLI(t, args...)
			require.NoError(t, err)
			assert.NotContains(t, stdout, "worktree")
			assert.NotContains(t, stdout, "snapshot")
		})
	}
}

func TestPublicCLIRestoreLatestAndJSONBoolConsistency(t *testing.T) {
	setupPublicCLIRepo(t, "restorelatest")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")

	stdout, err := runPublicCLI(t, "--json", "restore", first)
	require.NoError(t, err, stdout)
	var restored map[string]any
	decodePublicData(t, stdout, &restored)
	assert.Equal(t, false, restored["at_latest"])
	assert.Equal(t, false, restored["dirty"])

	stdout, err = runPublicCLI(t, "--json", "restore", "latest")
	require.NoError(t, err, stdout)
	restored = map[string]any{}
	decodePublicData(t, stdout, &restored)
	assert.Equal(t, second, restored["checkpoint_id"])
	assert.Equal(t, true, restored["at_latest"])
	assert.Equal(t, false, restored["dirty"])

	status := readStatusForPublicCLI(t)
	assert.IsType(t, true, status.Dirty)
	assert.IsType(t, true, status.AtLatest)
}

func TestPublicCLIDiffJSONIsPureAndRequiresTwoRefs(t *testing.T) {
	setupPublicCLIRepo(t, "diffpure")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	createCheckpointForPublicCLI(t, "second")

	stdout, err := runPublicCLI(t, "--json", "diff", "current", "latest")
	require.NoError(t, err, stdout)
	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.True(t, json.Valid(env.Data), stdout)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(stdout), "{"))
	assert.NotContains(t, stdout, "Note:")
	assert.NotContains(t, stdout, "jvs:")

	stdout, err = runPublicCLI(t, "diff", "current")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "requires two")
}

func TestPublicCLIWorkspacePathJSONUsesEnvelope(t *testing.T) {
	_, mainPath := setupPublicCLIRepo(t, "pathjson")

	stdout, err := runPublicCLI(t, "--json", "workspace", "path")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)

	var out map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &out), stdout)
	assert.Equal(t, "main", out["workspace"])
	assert.Equal(t, mainPath, out["path"])
	assert.NotContains(t, strings.TrimSpace(stdout), "\n"+mainPath)
}

func TestPublicCLIGCRunJSONUsesEnvelope(t *testing.T) {
	setupPublicCLIRepo(t, "gcrunjson")

	planOut, err := runPublicCLI(t, "--json", "gc", "plan")
	require.NoError(t, err, planOut)
	var plan map[string]any
	decodePublicData(t, planOut, &plan)
	planID, _ := plan["plan_id"].(string)
	require.NotEmpty(t, planID)

	runOut, err := runPublicCLI(t, "--json", "gc", "run", "--plan-id", planID)
	require.NoError(t, err, runOut)
	env := decodeContractEnvelope(t, runOut)
	require.True(t, env.OK, runOut)
	assert.Equal(t, "gc run", env.Command)
	assert.JSONEq(t, `{"plan_id":"`+planID+`","status":"completed"}`, string(env.Data))
}

func TestPublicCLIGCPlanJSONUsesSpecDeletionField(t *testing.T) {
	setupPublicCLIRepo(t, "gcplanjson")

	stdout, err := runPublicCLI(t, "--json", "gc", "plan")
	require.NoError(t, err, stdout)

	var plan map[string]any
	decodePublicData(t, stdout, &plan)
	assert.Contains(t, plan, "created_at")
	assert.Contains(t, plan, "protected_checkpoints")
	assert.Contains(t, plan, "to_delete")
	assert.IsType(t, []any{}, plan["protected_checkpoints"])
	assert.IsType(t, []any{}, plan["to_delete"])
	assert.NotContains(t, plan, "delete_checkpoints")
	assert.NotContains(t, plan, "protected_by_pin")
	assert.NotContains(t, plan, "protected_by_retention")
	assert.NotContains(t, plan, "retention")
	assert.NotContains(t, plan, "retention_policy")
}

func TestPublicCLIGCRunJSONMissingPlanUsesStableError(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "gcmissingplan")

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "gc", "run", "--plan-id", "missing")
	require.Equal(t, 1, exitCode, "gc run unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "gc run", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_GC_PLAN_MISMATCH", env.Error.Code)
	assert.NotContains(t, env.Error.Message, "control leaf")
	assert.NotContains(t, env.Error.Message, "stat ")
	assert.NotContains(t, env.Error.Message, repoPath)
	assert.NotContains(t, env.Error.Message, ".jvs")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIFullCloneExcludesActiveGCPlans(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "gcclonesource")
	require.NoError(t, os.WriteFile("file.txt", []byte("main"), 0644))
	createCheckpointForPublicCLI(t, "main")

	stdout, err := runPublicCLI(t, "--json", "fork", "old-feature")
	require.NoError(t, err, stdout)
	featurePath := filepath.Join(repoPath, "worktrees", "old-feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	createCheckpointForPublicCLI(t, "feature")
	require.NoError(t, os.Chdir(mainPath))
	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "old-feature", "--force")
	require.NoError(t, err, stdout)

	planOut, err := runPublicCLI(t, "--json", "gc", "plan")
	require.NoError(t, err, planOut)
	var plan map[string]any
	decodePublicData(t, planOut, &plan)
	planID, _ := plan["plan_id"].(string)
	require.NotEmpty(t, planID)
	require.NotZero(t, plan["candidate_count"])

	dest := filepath.Join(filepath.Dir(repoPath), "gcclonedest")
	cloneOut, err := runPublicCLI(t, "--json", "clone", repoPath, dest, "--scope", "full")
	require.NoError(t, err, cloneOut)
	require.NoFileExists(t, filepath.Join(dest, ".jvs", "gc", planID+".json"))

	stdout, stderr, exitCode := runContractSubprocess(t, dest, "--json", "gc", "run", "--plan-id", planID)
	require.Equal(t, 1, exitCode, "cloned gc run unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_GC_PLAN_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "not found")
	assert.NotContains(t, env.Error.Message, repoPath)
	assert.NotContains(t, env.Error.Message, dest)
}

func TestPublicCLIWorkspaceRemoveRejectsDirtyByDefault(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "dirtyremove")

	require.NoError(t, os.WriteFile("file.txt", []byte("clean"), 0644))
	createCheckpointForPublicCLI(t, "base")

	stdout, err := runPublicCLI(t, "--json", "fork", "feature")
	require.NoError(t, err, stdout)

	featureFile := filepath.Join(repoPath, "worktrees", "feature", "file.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("dirty"), 0644))

	stdout, err = runPublicCLI(t, "workspace", "remove", "feature")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "dirty")
	assert.FileExists(t, featureFile)

	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "feature", "--force")
	require.NoError(t, err, stdout)
	assert.NoDirExists(t, filepath.Join(repoPath, "worktrees", "feature"))
}

func TestPublicCLIStableJSONErrorsUsePublicVocabulary(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "publicerrors")
	require.NoError(t, os.Chdir(repoPath))

	stdout, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "status")
	require.Equal(t, 1, exitCode, "status unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "status", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
	assertPublicErrorOmitsLegacyVocabulary(t, env.Error)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIErrorsPreserveUserRepoPathsWithSpacesAndLegacyWords(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "target worktree snapshot history")
	require.NoError(t, err)
	cmd = createTestRootCmd()
	_, err = executeCommand(cmd, "init", "current worktree snapshot history")
	require.NoError(t, err)

	targetRepo := filepath.Join(dir, "target worktree snapshot history")
	currentRepo := filepath.Join(dir, "current worktree snapshot history")
	stdout, stderr, exitCode := runContractSubprocess(
		t,
		filepath.Join(currentRepo, "main"),
		"--json", "--repo", targetRepo, "--workspace", "main", "status",
	)
	require.Equal(t, 1, exitCode, "mismatched repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_TARGET_MISMATCH", env.Error.Code)
	assert.Contains(t, env.Error.Message, "targeting mismatch")
	assert.Contains(t, env.Error.Message, targetRepo)
	assert.Contains(t, env.Error.Message, currentRepo)
	assertPublicErrorOmitsLegacyVocabularyExcept(t, env.Error, targetRepo, currentRepo)
	assert.JSONEq(t, `null`, string(env.Data))

	stdout, stderr, exitCode = runContractSubprocess(
		t,
		filepath.Join(currentRepo, "main"),
		"--repo", targetRepo, "--workspace", "main", "status",
	)
	require.Equal(t, 1, exitCode, "mismatched repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, targetRepo)
	assert.Contains(t, stderr, currentRepo)
	sanitized := strings.ReplaceAll(stderr, targetRepo, "")
	sanitized = strings.ReplaceAll(sanitized, currentRepo, "")
	assert.NotContains(t, strings.ToLower(sanitized), "worktree")
	assert.NotContains(t, strings.ToLower(sanitized), "snapshot")
	assert.NotContains(t, strings.ToLower(sanitized), "history")
}

func TestPublicCLIJSONErrorsPreserveUserRepoPathWhenTargetIsMissing(t *testing.T) {
	repoPath, _ := setupPublicCLIRepo(t, "missingtarget")

	missingTarget := "missing worktree snapshot history"
	stdout, stderr, exitCode := runContractSubprocess(t, filepath.Join(repoPath, "main"), "--json", "--repo", missingTarget, "info")
	require.Equal(t, 1, exitCode, "missing --repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_REPO", env.Error.Code)
	assert.Contains(t, env.Error.Message, missingTarget)
	assertPublicErrorOmitsLegacyVocabularyExcept(t, env.Error, missingTarget)
	assert.JSONEq(t, `null`, string(env.Data))

	stdout, stderr, exitCode = runContractSubprocess(t, filepath.Join(repoPath, "main"), "--repo", missingTarget, "status")
	require.Equal(t, 1, exitCode, "missing --repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, missingTarget)
	sanitized := strings.ReplaceAll(stderr, missingTarget, "")
	assert.NotContains(t, strings.ToLower(sanitized), "worktree")
	assert.NotContains(t, strings.ToLower(sanitized), "snapshot")
	assert.NotContains(t, strings.ToLower(sanitized), "history")

	stdout, stderr, exitCode = runContractSubprocess(t, filepath.Join(repoPath, "main"), "--repo", missingTarget, "info")
	require.Equal(t, 1, exitCode, "missing --repo unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, missingTarget)
	sanitized = strings.ReplaceAll(stderr, missingTarget, "")
	assert.NotContains(t, strings.ToLower(sanitized), "worktree")
	assert.NotContains(t, strings.ToLower(sanitized), "snapshot")
	assert.NotContains(t, strings.ToLower(sanitized), "history")
}

func TestPublicCLIJSONImportOverlapPreservesParenthesizedUserPaths(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source worktree snapshot history")
	dest := filepath.Join(source, "child repo")
	require.NoError(t, os.Mkdir(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("source"), 0644))

	stdout, stderr, exitCode := runContractSubprocess(t, dir, "--json", "import", source, dest)
	require.Equal(t, 1, exitCode, "overlapping import unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "import", env.Command)
	require.NotNil(t, env.Error)
	assert.Contains(t, env.Error.Message, source)
	assert.Contains(t, env.Error.Message, dest)
	assertPublicErrorOmitsLegacyVocabularyExcept(t, env.Error, source, dest)
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestPublicCLIErrorVocabularyPreservesQuotedUserText(t *testing.T) {
	got := publicCLIErrorVocabulary(`snapshot "worktree-snapshot-history" history`)
	assert.Equal(t, `checkpoint "worktree-snapshot-history" checkpoint list`, got)
	assert.Equal(t, "source_workspace total_checkpoints checkpoint_id", publicCLIErrorVocabulary("source_worktree total_snapshots snapshot_id"))
	got = publicCLIErrorVocabulary("snapshot /tmp/missing worktree snapshot history, history")
	assert.Equal(t, "checkpoint /tmp/missing worktree snapshot history, checkpoint list", got)
	got = publicCLIErrorVocabulary("--repo is not inside a JVS repository: missing worktree snapshot history")
	assert.Equal(t, "--repo is not inside a JVS repository: missing worktree snapshot history", got)
	got = publicCLIErrorVocabulary("snapshot (/tmp/missing worktree snapshot history) history")
	assert.Equal(t, "checkpoint (/tmp/missing worktree snapshot history) checkpoint list", got)
	got = publicCLIErrorVocabulary("snapshot (worktree snapshot history) history")
	assert.Equal(t, "checkpoint (workspace checkpoint checkpoint list) checkpoint list", got)
}

func TestPublicCLIJSONUsesCheckpointWorkspaceTerms(t *testing.T) {
	setupPublicCLIRepo(t, "terms")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	second := createCheckpointForPublicCLI(t, "second")

	cases := [][]string{
		{"--json", "checkpoint", "list"},
		{"--json", "workspace", "list"},
		{"--json", "fork", "terms-branch"},
		{"--json", "diff", first, second},
		{"--json", "verify", second},
		{"--json", "info"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, err := runPublicCLI(t, args...)
			require.NoError(t, err, stdout)
			env := decodeContractEnvelope(t, stdout)
			require.True(t, env.OK, stdout)
			assert.NotContains(t, string(env.Data), "snapshot_id")
			assert.NotContains(t, string(env.Data), "worktree")
			assert.NotContains(t, string(env.Data), "head_snapshot")
			assert.NotContains(t, string(env.Data), "latest_snapshot")
			assert.NotContains(t, string(env.Data), "from_snapshot")
			assert.NotContains(t, string(env.Data), "to_snapshot")
		})
	}
}

func TestPublicCLIDoctorAndGCJSONHideInternalContractFields(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "publicjsoncontract")

	emptyPlanOut, err := runPublicCLI(t, "--json", "gc", "plan")
	require.NoError(t, err, emptyPlanOut)
	assertPublicJSONDataOmitsInternalContractFields(t, emptyPlanOut)

	require.NoError(t, os.WriteFile("file.txt", []byte("main"), 0644))
	createCheckpointForPublicCLI(t, "main")
	stdout, err := runPublicCLI(t, "--json", "fork", "old-feature")
	require.NoError(t, err, stdout)
	featurePath := filepath.Join(repoPath, "worktrees", "old-feature")
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644))
	require.NoError(t, os.Chdir(featurePath))
	createCheckpointForPublicCLI(t, "feature")
	require.NoError(t, os.Chdir(mainPath))
	stdout, err = runPublicCLI(t, "--json", "workspace", "remove", "old-feature", "--force")
	require.NoError(t, err, stdout)
	makeAllDescriptorsOldForPublicCLI(t, repoPath)

	nonEmptyPlanOut, err := runPublicCLI(t, "--json", "gc", "plan")
	require.NoError(t, err, nonEmptyPlanOut)
	assertPublicJSONDataOmitsInternalContractFields(t, nonEmptyPlanOut)
	var plan map[string]any
	decodePublicData(t, nonEmptyPlanOut, &plan)
	assert.NotZero(t, plan["candidate_count"])

	require.NoError(t, os.RemoveAll(mainPath))
	doctorOut, stderr, exitCode := runContractSubprocess(t, repoPath, "--json", "doctor")
	require.Equal(t, 1, exitCode, "doctor unexpectedly succeeded: stdout=%s stderr=%s", doctorOut, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))
	assertPublicJSONDataOmitsInternalContractFields(t, doctorOut)
	env := decodeContractEnvelope(t, doctorOut)
	assert.True(t, env.OK)
	var doctorData map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &doctorData), doctorOut)
	assert.Equal(t, false, doctorData["healthy"])
}

func TestHiddenLegacyCommandHelpUsesPublicGuidance(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "snapshot",
			args: []string{"snapshot", "--help"},
			want: []string{"jvs checkpoint", "jvs fork", "jvs restore latest"},
		},
		{
			name: "worktree remove",
			args: []string{"worktree", "remove", "--help"},
			want: []string{"jvs workspace remove", "current differs from latest"},
		},
		{
			name: "worktree fork",
			args: []string{"worktree", "fork", "--help"},
			want: []string{"jvs fork", "checkpoint"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, err := runPublicCLI(t, tc.args...)
			require.NoError(t, err, stdout)
			assertNoLegacyStateGuidance(t, stdout)
			assert.NotContains(t, stdout, "HEAD"+" state")
			for _, want := range tc.want {
				assert.Contains(t, stdout, want)
			}
		})
	}
}

func TestHiddenLegacyCommandErrorsUsePublicGuidance(t *testing.T) {
	repoPath, mainPath := setupPublicCLIRepo(t, "legacyerrors")

	require.NoError(t, os.WriteFile("file.txt", []byte("v1"), 0644))
	first := createCheckpointForPublicCLI(t, "first")
	require.NoError(t, os.WriteFile("file.txt", []byte("v2"), 0644))
	createCheckpointForPublicCLI(t, "second")

	stdout, err := runPublicCLI(t, "--json", "restore", first)
	require.NoError(t, err, stdout)

	stdout, stderr, code := runContractSubprocess(t, mainPath, "snapshot", "from legacy command")
	require.NotZero(t, code)
	combined := stdout + stderr
	assertNoLegacyStateGuidance(t, combined)
	assert.Contains(t, combined, "current differs from latest")
	assert.Contains(t, combined, "jvs checkpoint")
	assert.Contains(t, combined, "jvs fork")
	assert.Contains(t, combined, "jvs restore latest")

	stdout, err = runPublicCLI(t, "--json", "restore", "latest")
	require.NoError(t, err, stdout)
	stdout, err = runPublicCLI(t, "--json", "fork", "feature")
	require.NoError(t, err, stdout)
	stdout, err = runPublicCLI(t, "--json", "--workspace", "feature", "restore", first)
	require.NoError(t, err, stdout)

	stdout, stderr, code = runContractSubprocess(t, repoPath, "worktree", "remove", "feature")
	require.NotZero(t, code)
	combined = stdout + stderr
	assertNoLegacyStateGuidance(t, combined)
	assert.Contains(t, combined, "current differs from latest")
	assert.Contains(t, combined, "jvs workspace remove --force feature")
	assert.Contains(t, combined, "jvs fork")
	assert.Contains(t, combined, "jvs restore latest")
}

func assertNoLegacyStateGuidance(t *testing.T, output string) {
	t.Helper()

	assert.NotContains(t, output, strings.Join([]string{"restore", "HEAD"}, " "))
	assert.NotContains(t, output, "DET"+"ACHED")
	assert.NotContains(t, output, "detached"+" state")
}

func assertPublicJSONDataOmitsInternalContractFields(t *testing.T, stdout string) {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	data := string(env.Data)
	for _, forbidden := range []string{
		"snapshot_id",
		"worktree",
		"head_snapshot",
		"latest_snapshot",
		"keep_min_snapshots",
		"protected_by_pin",
		"protected_by_retention",
		"retention",
	} {
		assert.NotContains(t, data, forbidden)
	}
}

func makeAllDescriptorsOldForPublicCLI(t *testing.T, repoPath string) {
	t.Helper()

	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	entries, err := os.ReadDir(descriptorsDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(descriptorsDir, entry.Name())
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var desc model.Descriptor
		require.NoError(t, json.Unmarshal(data, &desc))
		desc.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		checksum, err := integrity.ComputeDescriptorChecksum(&desc)
		require.NoError(t, err)
		desc.DescriptorChecksum = checksum
		data, err = json.MarshalIndent(desc, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0644))
		updateReadyDescriptorChecksumForPublicCLI(t, repoPath, desc.SnapshotID, checksum)
	}
}

func updateReadyDescriptorChecksumForPublicCLI(t *testing.T, repoPath string, snapshotID model.SnapshotID, checksum model.HashValue) {
	t.Helper()

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	for _, name := range []string{".READY", ".READY.gz"} {
		path := filepath.Join(snapshotDir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		var marker map[string]any
		require.NoError(t, json.Unmarshal(data, &marker))
		marker["descriptor_checksum"] = string(checksum)
		data, err = json.MarshalIndent(marker, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0644))
	}
}
