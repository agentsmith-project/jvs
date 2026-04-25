package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	OK            bool            `json:"ok"`
	RepoRoot      *string         `json:"repo_root"`
	Workspace     *string         `json:"workspace"`
	Data          json.RawMessage `json:"data"`
	Error         *contractError  `json:"error"`
}

type contractError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func TestCLIJSONEnvelope_InfoIsSingleObject(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "testrepo")
	require.NoError(t, err)

	repoRoot := filepath.Join(dir, "testrepo")
	require.NoError(t, os.Chdir(filepath.Join(repoRoot, "main")))

	cmd = createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "info")
	require.NoError(t, err)

	env := decodeContractEnvelope(t, stdout)
	assert.Equal(t, 1, env.SchemaVersion)
	assert.Equal(t, "info", env.Command)
	assert.True(t, env.OK)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assert.Nil(t, env.Error)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data))
	assert.Equal(t, repoRoot, data["repo_root"])
	assert.Contains(t, data, "format_version")
}

func TestCLITargetingRepoFlag_InfoFromOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "testrepo")
	require.NoError(t, err)

	repoRoot := filepath.Join(dir, "testrepo")
	outside := filepath.Join(dir, "outside")
	require.NoError(t, os.Mkdir(outside, 0755))
	require.NoError(t, os.Chdir(outside))

	cmd = createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--repo", repoRoot, "info")
	require.NoError(t, err)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "info", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	assert.Nil(t, env.Workspace)
}

func TestCLITargetingWorkspaceFlag_HistoryFromRepoRoot(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "testrepo")
	require.NoError(t, err)

	repoRoot := filepath.Join(dir, "testrepo")
	require.NoError(t, os.Chdir(repoRoot))

	cmd = createTestRootCmd()
	stdout, err := executeCommand(cmd, "--json", "--repo", repoRoot, "--workspace", "main", "history")
	require.NoError(t, err)

	env := decodeContractEnvelope(t, stdout)
	assert.True(t, env.OK)
	assert.Equal(t, "history", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "main", *env.Workspace)
	assert.JSONEq(t, `[]`, string(env.Data))
}

func TestCLIJSONErrorEnvelope_NotRepo(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "info")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "info", env.Command)
	assert.Nil(t, env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_REPO", env.Error.Code)
	assert.Contains(t, env.Error.Message, "not a JVS repository")
	assert.Contains(t, env.Error.Hint, "jvs init")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIJSONErrorEnvelope_WorkspaceScopedRequiresWorkspace(t *testing.T) {
	dir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	require.NoError(t, os.Chdir(dir))
	cmd := createTestRootCmd()
	_, err := executeCommand(cmd, "init", "testrepo")
	require.NoError(t, err)

	repoRoot := filepath.Join(dir, "testrepo")
	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, "--json", "history")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "history", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	assert.Nil(t, env.Workspace)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_NOT_WORKSPACE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "not inside a workspace")
}

func TestCLIJSONErrorEnvelope_ArgValidationReportsCommand(t *testing.T) {
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "diff")

	require.Equal(t, 1, exitCode)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.False(t, env.OK)
	assert.Equal(t, "diff", env.Command)
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, "diff requires two checkpoint refs")
	assert.JSONEq(t, `null`, string(env.Data))
}

func TestCLIContractSubprocess(t *testing.T) {
	if os.Getenv("JVS_CONTRACT_SUBPROCESS") != "1" {
		t.Skip("contract subprocess helper")
	}

	cwd := os.Getenv("JVS_CONTRACT_CWD")
	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	var args []string
	if err := json.Unmarshal([]byte(os.Getenv("JVS_CONTRACT_ARGS")), &args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cmd := createTestRootCmd()
	cmd.SetArgs(args)
	executedCmd, err := cmd.ExecuteC()
	if err != nil {
		reportCommandErrorForCommand(executedCmd, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func decodeContractEnvelope(t *testing.T, stdout string) contractEnvelope {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(stdout))
	var env contractEnvelope
	require.NoError(t, dec.Decode(&env), "stdout was not a JSON envelope: %q", stdout)

	var extra any
	err := dec.Decode(&extra)
	require.True(t, errors.Is(err, io.EOF), "stdout must contain exactly one JSON value: %q", stdout)

	return env
}

func runContractSubprocess(t *testing.T, cwd string, args ...string) (string, string, int) {
	t.Helper()

	argData, err := json.Marshal(args)
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=TestCLIContractSubprocess")
	cmd.Env = append(os.Environ(),
		"JVS_CONTRACT_SUBPROCESS=1",
		"JVS_CONTRACT_CWD="+cwd,
		"JVS_CONTRACT_ARGS="+string(argData),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}

	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "unexpected subprocess error: %v", err)
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}
