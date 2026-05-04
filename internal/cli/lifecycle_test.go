package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleInitSaveHistoryStatusCurrentCommands(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	require.NoError(t, os.Chdir(repoRoot))

	initOut, err := executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err, initOut)
	assert.Contains(t, initOut, "Folder: "+repoRoot)
	assert.Contains(t, initOut, "Workspace: main")
	assert.Contains(t, initOut, "JVS is ready for this folder.")
	assert.NotContains(t, initOut, "main/")
	require.DirExists(t, filepath.Join(repoRoot, ".jvs"))
	require.NoDirExists(t, filepath.Join(repoRoot, "main"))

	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))

	statusBefore := decodeContractDataMapFromCommand(t, "--json", "status")
	assert.Equal(t, repoRoot, statusBefore["folder"])
	assert.Equal(t, "main", statusBefore["workspace"])
	assert.Equal(t, true, statusBefore["unsaved_changes"])
	assert.Equal(t, "not_saved", statusBefore["files_state"])

	saveOut, err := executeCommand(createTestRootCmd(), "--json", "save", "-m", "baseline")
	require.NoError(t, err, saveOut)
	saveEnv := decodeContractEnvelope(t, saveOut)
	require.True(t, saveEnv.OK, saveOut)
	assert.Equal(t, "save", saveEnv.Command)
	var saveData map[string]any
	require.NoError(t, json.Unmarshal(saveEnv.Data, &saveData), saveOut)
	savePointID, ok := saveData["save_point_id"].(string)
	require.True(t, ok, "save should expose save_point_id: %#v", saveData)
	require.NotEmpty(t, savePointID)
	assert.Equal(t, "baseline", saveData["message"])
	assert.Equal(t, false, saveData["unsaved_changes"])
	assertNoRemovedContractFields(t, saveData)

	statusAfter := decodeContractDataMapFromCommand(t, "--json", "status")
	assert.Equal(t, false, statusAfter["unsaved_changes"])
	assert.Equal(t, "matches_save_point", statusAfter["files_state"])
	assert.Equal(t, savePointID, statusAfter["newest_save_point"])

	history := decodeContractDataMapFromCommand(t, "--json", "history")
	assert.Equal(t, "main", history["workspace"])
	assert.Equal(t, savePointID, history["newest_save_point"])
	savePoints, ok := history["save_points"].([]any)
	require.True(t, ok)
	require.Len(t, savePoints, 1)
}

func TestLifecycleNotInsideWorkspaceHintDoesNotSuggestLegacyMainDirectory(t *testing.T) {
	err := notInsideWorkspaceError()
	jvsErr, ok := err.(*errclass.JVSError)
	require.True(t, ok)

	assert.Contains(t, jvsErr.Hint, "workspace folder")
	assert.NotContains(t, jvsErr.Hint, "main/")
}

func TestLifecycleCleanupPreviewUsesCurrentContract(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	_ = savePointForContract(t, "baseline")

	stdout, err := executeCommand(createTestRootCmd(), "--json", "cleanup", "preview")
	require.NoError(t, err, stdout)

	env := decodeContractEnvelope(t, stdout)
	require.True(t, env.OK, stdout)
	assert.Equal(t, "cleanup preview", env.Command)
	require.NotNil(t, env.RepoRoot)
	assert.Equal(t, repoRoot, *env.RepoRoot)

	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	assert.NotEmpty(t, data["plan_id"])
	assert.Contains(t, data, "protected_save_points")
	assert.Contains(t, data, "reclaimable_save_points")
	assert.NotContains(t, data, "protected_checkpoints")
	assert.NotContains(t, strings.ToLower(string(env.Data)), "checkpoint")
	assert.NotContains(t, strings.ToLower(string(env.Data)), "snapshot")
}

func TestRemovedLifecycleCommandsAreUnknown(t *testing.T) {
	isolateContractCLIState(t)
	for _, tc := range []struct {
		name        string
		args        []string
		wantCommand string
		wantUnknown string
	}{
		{name: "import", args: []string{"import", "src", "dst"}},
		{name: "clone", args: []string{"clone", "src", "dst"}},
		{name: "capability", args: []string{"capability", "."}},
		{name: "workspace remove", args: []string{"workspace", "remove", "old"}, wantCommand: "workspace", wantUnknown: "remove"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json"}, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), args...)
			require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			wantCommand := tc.wantCommand
			if wantCommand == "" {
				wantCommand = tc.name
			}
			assert.Equal(t, wantCommand, env.Command)
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_USAGE", env.Error.Code)
			wantUnknown := tc.wantUnknown
			if wantUnknown == "" {
				wantUnknown = tc.name
			}
			assert.Contains(t, env.Error.Message, `unknown command "`+wantUnknown+`"`)
			assert.JSONEq(t, `null`, string(env.Data))
		})
	}
}

func decodeContractDataMapFromCommand(t *testing.T, args ...string) map[string]any {
	t.Helper()

	stdout, err := executeCommand(createTestRootCmd(), args...)
	require.NoError(t, err, stdout)
	data := decodeContractDataMap(t, stdout)
	assertNoRemovedContractFields(t, data)
	return data
}
