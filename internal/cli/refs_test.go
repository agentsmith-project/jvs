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

func TestRefsSavePointIDPrefixWorksWithViewAndRestore(t *testing.T) {
	isolateContractCLIState(t)
	repoRoot := setupCurrentContractRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointForContract(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointForContract(t, "second")
	firstPrefix := uniqueIDPrefixForRefsTest(t, firstID, secondID)

	viewOut, err := executeCommand(createTestRootCmd(), "--json", "view", firstPrefix, "app.txt")
	require.NoError(t, err, viewOut)
	viewEnv := decodeContractEnvelope(t, viewOut)
	require.True(t, viewEnv.OK, viewOut)
	assert.Equal(t, "view", viewEnv.Command)
	var viewData map[string]any
	require.NoError(t, json.Unmarshal(viewEnv.Data, &viewData), viewOut)
	assert.Equal(t, firstID, viewData["save_point"])
	assert.Equal(t, "app.txt", viewData["path_inside_save_point"])
	viewPath, ok := viewData["view_path"].(string)
	require.True(t, ok, "view should expose view_path: %#v", viewData)
	viewContent, err := os.ReadFile(viewPath)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(viewContent))
	viewID, ok := viewData["view_id"].(string)
	require.True(t, ok, "view should expose view_id: %#v", viewData)
	closeOut, err := executeCommand(createTestRootCmd(), "--json", "view", "close", viewID)
	require.NoError(t, err, closeOut)

	restoreOut, err := executeCommand(createTestRootCmd(), "--json", "restore", firstPrefix)
	require.NoError(t, err, restoreOut)
	restoreEnv := decodeContractEnvelope(t, restoreOut)
	require.True(t, restoreEnv.OK, restoreOut)
	assert.Equal(t, "restore", restoreEnv.Command)
	var restorePreview map[string]any
	require.NoError(t, json.Unmarshal(restoreEnv.Data, &restorePreview), restoreOut)
	planID, ok := restorePreview["plan_id"].(string)
	require.True(t, ok, "restore preview should expose plan_id: %#v", restorePreview)
	require.NotEmpty(t, planID)

	runOut, err := executeCommand(createTestRootCmd(), "--json", "restore", "--run", planID)
	require.NoError(t, err, runOut)
	runEnv := decodeContractEnvelope(t, runOut)
	require.True(t, runEnv.OK, runOut)
	assert.Equal(t, "restore", runEnv.Command)
	content, err := os.ReadFile(filepath.Join(repoRoot, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))
}

func TestRemovedLegacyPublicCommandsAreUnknownJSON(t *testing.T) {
	isolateContractCLIState(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "checkpoint", args: []string{"checkpoint", "list"}},
		{name: "snapshot", args: []string{"snapshot", "list"}},
		{name: "fork", args: []string{"fork", "feature"}},
		{name: "worktree", args: []string{"worktree", "list"}},
		{name: "gc", args: []string{"gc", "plan"}},
		{name: "verify", args: []string{"verify", "--all"}},
		{name: "capability", args: []string{"capability", "."}},
		{name: "info", args: []string{"info"}},
		{name: "diff", args: []string{"diff", "one", "two"}},
		{name: "import", args: []string{"import", "src", "dst"}},
		{name: "clone", args: []string{"clone", "src", "dst"}},
		{name: "config", args: []string{"config", "get"}},
		{name: "conformance", args: []string{"conformance"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json"}, tc.args...)
			stdout, stderr, exitCode := runContractSubprocess(t, t.TempDir(), args...)
			require.Equal(t, 1, exitCode, "stdout=%s stderr=%s", stdout, stderr)
			assert.Empty(t, strings.TrimSpace(stderr))

			env := decodeContractEnvelope(t, stdout)
			assert.False(t, env.OK)
			assert.Equal(t, tc.name, env.Command)
			assert.Nil(t, env.RepoRoot)
			assert.Nil(t, env.Workspace)
			assert.JSONEq(t, `null`, string(env.Data))
			require.NotNil(t, env.Error)
			assert.Equal(t, "E_USAGE", env.Error.Code)
			assert.Contains(t, env.Error.Message, `unknown command "`+tc.name+`"`)
		})
	}
}

func uniqueIDPrefixForRefsTest(t *testing.T, id string, otherIDs ...string) string {
	t.Helper()

	for length := 1; length < len(id); length++ {
		prefix := id[:length]
		unique := true
		for _, otherID := range otherIDs {
			if strings.HasPrefix(otherID, prefix) {
				unique = false
				break
			}
		}
		if unique {
			return prefix
		}
	}
	return id
}
