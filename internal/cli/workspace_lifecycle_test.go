package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWorkspaceLifecycleExternalRepo(t *testing.T, repoName, workspaceName string) (repoPath, mainPath, savePointID, workspacePath string) {
	t.Helper()

	repoPath, mainPath = setupCoverageRepo(t, repoName)
	require.NoError(t, os.WriteFile("lifecycle-base.txt", []byte("base"), 0644))
	savePointID = createRootTestSavePoint(t, "lifecycle base")
	workspacePath = filepath.Join(filepath.Dir(repoPath), workspaceName+"-folder")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", workspacePath, "--from", savePointID, "--name", workspaceName)
	require.NoError(t, err, stdout)
	return repoPath, mainPath, savePointID, workspacePath
}

func TestCLIWorkspaceBasenameDifferentFromNameIsHealthy(t *testing.T) {
	repoRoot := setupAdoptedSaveFacadeRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("baseline"), 0644))
	savePointID := savePointIDFromCLI(t, "baseline")

	workspacePath := filepath.Join(filepath.Dir(repoRoot), "review-folder")
	stdout, err := executeCommand(createTestRootCmd(), "workspace", "new", workspacePath, "--from", savePointID, "--name", "experiment")
	require.NoError(t, err, stdout)
	require.NoError(t, os.Chdir(workspacePath))

	statusOut, err := executeCommand(createTestRootCmd(), "--json", "status")
	require.NoError(t, err, statusOut)
	_, statusData := decodeFacadeDataMap(t, statusOut)
	assert.Equal(t, "experiment", statusData["workspace"])
	assert.Equal(t, workspacePath, statusData["folder"])

	listOut, err := executeCommand(createTestRootCmd(), "--json", "workspace", "list")
	require.NoError(t, err, listOut)
	env := decodeContractEnvelope(t, listOut)
	require.True(t, env.OK, listOut)
	var records []publicWorkspaceListRecord
	require.NoError(t, json.Unmarshal(env.Data, &records), listOut)
	require.Len(t, records, 2)
	var found bool
	for _, record := range records {
		if record.Workspace == "experiment" {
			found = true
			assert.Equal(t, workspacePath, record.Folder)
		}
	}
	require.True(t, found, "workspace list should include experiment: %#v", records)

	doctorOut, err := executeCommand(createTestRootCmd(), "--json", "doctor", "--strict")
	require.NoError(t, err, doctorOut)
	doctorData := decodeContractDataMap(t, doctorOut)
	assert.Equal(t, true, doctorData["healthy"])
}
