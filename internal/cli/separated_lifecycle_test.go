package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeparatedLifecycleUnsupportedCommandsFailClosedJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(base, controlRoot, payloadRoot string) []string
	}{
		{
			name: "repo move preview",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "move", filepath.Join(base, "control-moved"))
			},
		},
		{
			name: "repo move run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "move", "--run", "missing-separated-plan")
			},
		},
		{
			name: "repo rename preview",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "rename", "control-renamed")
			},
		},
		{
			name: "repo rename run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "rename", "--run", "missing-separated-plan")
			},
		},
		{
			name: "repo detach preview",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "detach")
			},
		},
		{
			name: "repo detach run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "repo", "detach", "--run", "missing-separated-plan")
			},
		},
		{
			name: "workspace move preview",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "move", "main", filepath.Join(base, "payload-moved"))
			},
		},
		{
			name: "workspace move run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "move", "--run", "missing-separated-plan")
			},
		},
		{
			name: "workspace rename",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "rename", "main", "main-renamed")
			},
		},
		{
			name: "workspace rename dry run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "rename", "--dry-run", "main", "main-renamed")
			},
		},
		{
			name: "workspace delete preview",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "delete", "main")
			},
		},
		{
			name: "workspace delete run",
			args: func(base, controlRoot, payloadRoot string) []string {
				return separatedLifecycleArgs(controlRoot, "workspace", "delete", "--run", "missing-separated-plan")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, controlRoot, payloadRoot := setupSeparatedLifecycleRepo(t)
			before := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)

			stdout, stderr, exitCode := runContractSubprocess(t, base, tc.args(base, controlRoot, payloadRoot)...)
			require.Equal(t, 1, exitCode, "%s unexpectedly succeeded: stdout=%s stderr=%s", tc.name, stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK, stdout)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_SEPARATED_LIFECYCLE_UNSUPPORTED", env.Error.Code)
			assert.Contains(t, env.Error.Message, "No files changed")
			assert.JSONEq(t, `null`, string(env.Data))
			assertSeparatedLifecycleRootsUnchanged(t, before, controlRoot, payloadRoot)
		})
	}
}

func TestSeparatedLifecycleUnsupportedHumanOutputSaysNoFilesChanged(t *testing.T) {
	base, controlRoot, payloadRoot := setupSeparatedLifecycleRepo(t)
	before := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		"--control-root", controlRoot,
		"--workspace", "main",
		"workspace", "move", "main", filepath.Join(base, "payload-moved"),
	)
	require.Equal(t, 1, exitCode, "workspace move unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stdout))
	assert.Contains(t, stderr, "No files changed")
	assert.NotContains(t, stderr, "Run:")
	assertSeparatedLifecycleRootsUnchanged(t, before, controlRoot, payloadRoot)
}

func TestSeparatedLifecycleWorkspaceNewFailsClosedJSON(t *testing.T) {
	base, controlRoot, payloadRoot := setupSeparatedLifecycleRepo(t)
	saveOut, err := executeCommand(createTestRootCmd(), separatedLifecycleArgs(controlRoot, "save", "-m", "source")...)
	require.NoError(t, err, saveOut)
	_, saveData := decodeSeparatedControlDataMap(t, saveOut)
	sourceID, _ := saveData["save_point_id"].(string)
	require.NotEmpty(t, sourceID)
	targetFolder := filepath.Join(base, "feature-payload")
	before := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)

	stdout, stderr, exitCode := runContractSubprocess(t, base,
		separatedLifecycleArgs(controlRoot, "workspace", "new", targetFolder, "--from", sourceID, "--name", "feature")...,
	)
	require.Equal(t, 1, exitCode, "workspace new unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK, stdout)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_SEPARATED_LIFECYCLE_UNSUPPORTED", env.Error.Code)
	assert.Contains(t, env.Error.Message, "No files changed")
	assert.JSONEq(t, `null`, string(env.Data))
	assert.NoDirExists(t, targetFolder)
	assertSeparatedLifecycleRootsUnchanged(t, before, controlRoot, payloadRoot)
}

func TestSeparatedLifecycleReadonlyWorkspaceCommandsStillWork(t *testing.T) {
	_, controlRoot, payloadRoot := setupSeparatedLifecycleRepo(t)
	before := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)

	listOut, err := executeCommand(createTestRootCmd(), separatedLifecycleArgs(controlRoot, "workspace", "list")...)
	require.NoError(t, err, listOut)
	listEnv := decodeContractEnvelope(t, listOut)
	assert.True(t, listEnv.OK, listOut)
	var listData map[string]any
	require.NoError(t, json.Unmarshal(listEnv.Data, &listData), listOut)
	assertSeparatedControlAuthoritativeData(t, listData, controlRoot, payloadRoot, "main")
	assert.Equal(t, "not_run", listData["doctor_strict"])
	workspaces, ok := listData["workspaces"].([]any)
	require.True(t, ok, "workspace list should expose workspaces array: %#v", listData)
	require.Len(t, workspaces, 1)

	pathOut, err := executeCommand(createTestRootCmd(), separatedLifecycleArgs(controlRoot, "workspace", "path", "main")...)
	require.NoError(t, err, pathOut)
	pathEnv := decodeContractEnvelope(t, pathOut)
	require.True(t, pathEnv.OK, pathOut)
	var pathData map[string]any
	require.NoError(t, json.Unmarshal(pathEnv.Data, &pathData), pathOut)
	assertSeparatedControlAuthoritativeData(t, pathData, controlRoot, payloadRoot, "main")
	assert.Equal(t, "not_run", pathData["doctor_strict"])
	assert.Equal(t, payloadRoot, pathData["path"])
	assertSeparatedLifecycleRootsUnchanged(t, before, controlRoot, payloadRoot)
}

func setupSeparatedLifecycleRepo(t *testing.T) (base, controlRoot, payloadRoot string) {
	t.Helper()

	base = setupSeparatedControlCLICWD(t)
	controlRoot = filepath.Join(base, "control")
	payloadRoot = filepath.Join(base, "payload")
	initSeparatedControlForCLITest(t, controlRoot, payloadRoot, "main")
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, "app.txt"), []byte("payload sentinel\n"), 0644))
	return base, controlRoot, payloadRoot
}

func separatedLifecycleArgs(controlRoot string, command ...string) []string {
	args := []string{"--json", "--control-root", controlRoot, "--workspace", "main"}
	return append(args, command...)
}

type separatedLifecycleRootSnapshot map[string]separatedLifecycleNode

type separatedLifecycleNode struct {
	Mode    os.FileMode
	Content string
}

func captureSeparatedLifecycleRoots(t *testing.T, controlRoot, payloadRoot string) map[string]separatedLifecycleRootSnapshot {
	t.Helper()

	return map[string]separatedLifecycleRootSnapshot{
		"control": captureSeparatedLifecycleRoot(t, controlRoot),
		"payload": captureSeparatedLifecycleRoot(t, payloadRoot),
	}
}

func captureSeparatedLifecycleRoot(t *testing.T, root string) separatedLifecycleRootSnapshot {
	t.Helper()

	snapshot := separatedLifecycleRootSnapshot{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		info, err := entry.Info()
		require.NoError(t, err)
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		rel = filepath.ToSlash(rel)
		node := separatedLifecycleNode{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			node.Content = string(content)
		}
		snapshot[rel] = node
		return nil
	}))
	return snapshot
}

func assertSeparatedLifecycleRootsUnchanged(t *testing.T, before map[string]separatedLifecycleRootSnapshot, controlRoot, payloadRoot string) {
	t.Helper()

	after := captureSeparatedLifecycleRoots(t, controlRoot, payloadRoot)
	assert.Equal(t, before, after)
}
