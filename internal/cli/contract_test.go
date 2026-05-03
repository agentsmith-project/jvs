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
	Code                   string `json:"code"`
	Message                string `json:"message"`
	Hint                   string `json:"hint"`
	RecommendedNextCommand string `json:"recommended_next_command,omitempty"`
}

func TestCLIJSONEnvelope_CurrentCommandsAreSingleObjects(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	saveID := savePointForContract(t, "baseline")

	cases := []struct {
		name    string
		command string
		args    []string
		assert  func(t *testing.T, data map[string]any)
	}{
		{
			name:    "status",
			command: "status",
			args:    []string{"status"},
			assert: func(t *testing.T, data map[string]any) {
				assert.Equal(t, repoRoot, data["folder"])
				assert.Equal(t, "main", data["workspace"])
				assert.Equal(t, false, data["unsaved_changes"])
				assert.Equal(t, saveID, data["newest_save_point"])
			},
		},
		{
			name:    "history",
			command: "history",
			args:    []string{"history"},
			assert: func(t *testing.T, data map[string]any) {
				assert.Equal(t, "main", data["workspace"])
				assert.Equal(t, saveID, data["newest_save_point"])
				savePoints, ok := data["save_points"].([]any)
				require.True(t, ok, "save_points should be an array: %#v", data["save_points"])
				require.Len(t, savePoints, 1)
			},
		},
		{
			name:    "doctor",
			command: "doctor",
			args:    []string{"doctor"},
			assert: func(t *testing.T, data map[string]any) {
				assert.Equal(t, true, data["healthy"])
				_, ok := data["findings"].([]any)
				assert.True(t, ok, "findings should be an array: %#v", data["findings"])
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, err := executeCommand(createTestRootCmd(), append([]string{"--json"}, tc.args...)...)
			require.NoError(t, err, stdout)

			env := decodeContractEnvelope(t, stdout)
			assert.Equal(t, 1, env.SchemaVersion)
			assert.Equal(t, tc.command, env.Command)
			assert.True(t, env.OK)
			require.NotNil(t, env.RepoRoot)
			assert.Equal(t, repoRoot, *env.RepoRoot)
			assert.Nil(t, env.Error)

			var data map[string]any
			require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
			tc.assert(t, data)
			assertNoRemovedContractFields(t, data)
		})
	}
}

func TestCLITargetingWorkspaceFlag_StatusHistorySave(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("main baseline"), 0644))
	baseID := savePointForContract(t, "main baseline")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "workspace", "new", "../feature", "--from", baseID)
	require.NoError(t, err, stdout)
	featureData := decodeContractDataMap(t, stdout)
	featurePath, ok := featureData["folder"].(string)
	require.True(t, ok, "workspace new should expose folder: %#v", featureData)
	require.NotEmpty(t, featurePath)
	require.NoError(t, os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature work"), 0644))

	statusData := runTargetedContractCommand(t, repoRoot, "status")
	assert.Equal(t, "feature", statusData["workspace"])
	assert.Equal(t, featurePath, statusData["folder"])
	assert.Equal(t, true, statusData["unsaved_changes"])

	historyData := runTargetedContractCommand(t, repoRoot, "history")
	assert.Equal(t, "feature", historyData["workspace"])
	assert.Empty(t, historyData["newest_save_point"])
	assert.Equal(t, baseID, historyData["current_pointer"])
	assert.Equal(t, baseID, historyData["started_from_save_point"])
	savePoints, ok := historyData["save_points"].([]any)
	require.True(t, ok, "save_points should be an array: %#v", historyData["save_points"])
	require.Len(t, savePoints, 1)
	sourcePoint, ok := savePoints[0].(map[string]any)
	require.True(t, ok, "save point should be an object: %#v", savePoints[0])
	assert.Equal(t, baseID, sourcePoint["save_point_id"])

	saveData := runTargetedContractCommand(t, repoRoot, "save", "-m", "feature baseline")
	assert.Equal(t, "feature", saveData["workspace"])
	assert.Equal(t, "feature baseline", saveData["message"])
	featureSavePoint, ok := saveData["save_point_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, featureSavePoint)

	historyData = runTargetedContractCommand(t, repoRoot, "history")
	assert.Equal(t, "feature", historyData["workspace"])
	assert.Equal(t, featureSavePoint, historyData["newest_save_point"])
	savePoints, ok = historyData["save_points"].([]any)
	require.True(t, ok)
	require.Len(t, savePoints, 1)
	first, ok := savePoints[0].(map[string]any)
	require.True(t, ok, "save point should be an object: %#v", savePoints[0])
	assert.Equal(t, featureSavePoint, first["save_point_id"])
	assert.Equal(t, "feature baseline", first["message"])
}

func TestCLIJSONErrorEnvelope_UnknownCommandIsSingleObject(t *testing.T) {
	isolateContractCLIState(t)
	stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), "--json", "histroy")

	require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	assert.Equal(t, 1, env.SchemaVersion)
	assert.Equal(t, "histroy", env.Command)
	assert.False(t, env.OK)
	assert.Nil(t, env.RepoRoot)
	assert.Nil(t, env.Workspace)
	assert.JSONEq(t, `null`, string(env.Data))
	require.NotNil(t, env.Error)
	assert.Equal(t, "E_USAGE", env.Error.Code)
	assert.Contains(t, env.Error.Message, `unknown command "histroy"`)
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
	primeOutputFlagsFromArgs(args)
	executedCmd, err := cmd.ExecuteC()
	if err != nil {
		reportCommandErrorForCommand(executedCmd, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runTargetedContractCommand(t *testing.T, repoRoot, command string, args ...string) map[string]any {
	t.Helper()

	cliArgs := []string{"--json", "--repo", repoRoot, "--workspace", "feature", command}
	cliArgs = append(cliArgs, args...)
	stdout, stderr, exitCode := runContractSubprocess(t, repoRoot, cliArgs...)
	require.Equal(t, 0, exitCode, "stdout=%s stderr=%s", stdout, stderr)
	assert.Empty(t, strings.TrimSpace(stderr))

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)
	require.NotNil(t, env.Workspace)
	assert.Equal(t, "feature", *env.Workspace)
	assert.Equal(t, command, env.Command)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assertNoRemovedContractFields(t, data)
	return data
}

func isolateContractCLIState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { _ = createTestRootCmd() })
}

func setupCurrentContractRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })

	require.NoError(t, os.Chdir(repoRoot))
	stdout, err := executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err, stdout)
	require.DirExists(t, filepath.Join(repoRoot, ".jvs"))
	require.NoDirExists(t, filepath.Join(repoRoot, "main"))
	return repoRoot
}

func savePointForContract(t *testing.T, message string) string {
	t.Helper()

	stdout, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", message)
	require.NoError(t, err, stdout)
	data := decodeContractDataMap(t, stdout)
	savePointID, ok := data["save_point_id"].(string)
	require.True(t, ok, "save should expose save_point_id: %#v", data)
	require.NotEmpty(t, savePointID)
	return savePointID
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

func decodeContractDataMap(t *testing.T, stdout string) map[string]any {
	t.Helper()

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return data
}

func assertNoRemovedContractFields(t *testing.T, data map[string]any) {
	t.Helper()

	for _, key := range []string{
		"checkpoint_id",
		"snapshot_id",
		"parent_checkpoint_id",
		"head_snapshot",
		"latest_snapshot",
		"source_worktree",
		"protected_checkpoints",
	} {
		assert.NotContains(t, data, key)
	}
}

func assertPublicErrorOmitsLegacyVocabulary(t *testing.T, errData *contractError) {
	t.Helper()
	require.NotNil(t, errData)
	for _, value := range []string{errData.Code, errData.Message, errData.Hint} {
		lower := strings.ToLower(value)
		assert.NotContains(t, lower, "worktree")
		assert.NotContains(t, lower, "snapshot")
		assert.NotContains(t, lower, "checkpoint")
	}
}

func assertPublicErrorOmitsLegacyVocabularyExcept(t *testing.T, errData *contractError, allowedValues ...string) {
	t.Helper()
	require.NotNil(t, errData)
	for _, value := range []string{errData.Code, errData.Message, errData.Hint} {
		for _, allowed := range allowedValues {
			value = strings.ReplaceAll(value, allowed, "")
		}
		lower := strings.ToLower(value)
		assert.NotContains(t, lower, "worktree")
		assert.NotContains(t, lower, "snapshot")
		assert.NotContains(t, lower, "checkpoint")
	}
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
