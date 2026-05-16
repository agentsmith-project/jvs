//go:build !legacy_public_cli

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusCommandOutput struct {
	Folder          string  `json:"folder"`
	Workspace       string  `json:"workspace"`
	NewestSavePoint *string `json:"newest_save_point"`
	HistoryHead     *string `json:"history_head"`
	ContentSource   *string `json:"content_source"`
	UnsavedChanges  bool    `json:"unsaved_changes"`
	FilesState      string  `json:"files_state"`
}

func setupAdoptedSaveFacadeRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWd)) })
	require.NoError(t, os.Chdir(repoRoot))
	_, err = executeCommand(createTestRootCmd(), "init")
	require.NoError(t, err)
	return repoRoot
}

func decodeFacadeDataMap(t *testing.T, stdout string) (contractEnvelope, map[string]any) {
	t.Helper()
	env := decodeContractEnvelope(t, stdout)
	require.NotNil(t, env.Data, stdout)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data), stdout)
	return env, data
}

func savePointIDFromCLI(t *testing.T, message string) string {
	t.Helper()
	ctx, err := resolveWorkspaceScoped()
	require.NoError(t, err)
	return createSavePointForTest(t, ctx.Repo.Root, ctx.Workspace, message)
}

func createSavePointForTest(t *testing.T, repoRoot, workspace, message string) string {
	t.Helper()
	desc, err := snapshot.NewCreator(repoRoot, model.EngineCopy).CreateSavePoint(workspace, message, nil)
	require.NoError(t, err)
	id := desc.SnapshotID.String()
	require.NotEmpty(t, id)
	return id
}

func createTwoSavePoints(t *testing.T, repoRoot string) (string, string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v1"), 0644))
	firstID := savePointIDFromCLI(t, "first")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "app.txt"), []byte("v2"), 0644))
	secondID := savePointIDFromCLI(t, "second")
	require.NotEqual(t, model.SnapshotID(firstID), model.SnapshotID(secondID))
	return firstID, secondID
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, string(content))
}

func assertSavePointStorageExists(t *testing.T, repoPath, savePointID string) {
	t.Helper()
	assert.DirExists(t, filepath.Join(repoPath, ".jvs", "snapshots", savePointID))
	assert.FileExists(t, filepath.Join(repoPath, ".jvs", "descriptors", savePointID+".json"))
}

func requireSeparatedTransferByID(t *testing.T, data map[string]any, id string) map[string]any {
	t.Helper()

	transfers, ok := data["transfers"].([]any)
	require.True(t, ok, "transfers should be an array: %#v", data["transfers"])
	for _, item := range transfers {
		record, ok := item.(map[string]any)
		require.True(t, ok, "transfer should be an object: %#v", item)
		if record["transfer_id"] == id {
			return record
		}
	}
	t.Fatalf("missing transfer %q in %#v", id, transfers)
	return nil
}

func installCapacityGateHooks(gate capacitygate.Gate) func() {
	oldViewGate := viewCapacityGate
	restorePlanGate := restoreplan.SetCapacityGateForTest(gate)
	viewCapacityGate = gate
	return func() {
		viewCapacityGate = oldViewGate
		restorePlanGate()
	}
}

func savePointCatalogCount(t *testing.T, repoRoot string) int {
	t.Helper()
	savePoints, err := snapshot.ListAll(repoRoot)
	require.NoError(t, err)
	return len(savePoints)
}

func descriptorFileCount(t *testing.T, repoRoot string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".jvs", "descriptors"))
	require.NoError(t, err)
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func historySavePointIDsForSeparatedOpsTest(t *testing.T, data map[string]any, key string) []string {
	t.Helper()
	raw, ok := data[key].([]any)
	require.True(t, ok, "%s should be an array: %#v", key, data[key])
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		record, ok := item.(map[string]any)
		require.True(t, ok, "save point record should be an object: %#v", item)
		id, _ := record["save_point_id"].(string)
		require.NotEmpty(t, id, "save point record missing id: %#v", record)
		ids = append(ids, id)
	}
	return ids
}

func seedSeparatedControlMetadataSentinels(t *testing.T, controlRoot string) {
	t.Helper()
	for _, name := range []string{"audit", "locks", "restore-plans", "runtime"} {
		require.NoError(t, os.MkdirAll(filepath.Join(controlRoot, ".jvs", name), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "audit", "platform.log"), []byte("audit sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "locks", "platform.lock"), []byte("lock sentinel\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "restore-plans", "platform-state.tmp"), []byte("{}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", "runtime", "platform.tmp"), []byte("runtime sentinel\n"), 0644))
}

func staleCLISameHostOwner(t *testing.T, operation string) map[string]any {
	t.Helper()

	hostname, err := os.Hostname()
	require.NoError(t, err)
	return map[string]any{
		"operation":  operation,
		"pid":        exitedCLIChildPID(t),
		"hostname":   hostname,
		"created_at": time.Now().UTC().Add(-2 * time.Hour),
	}
}

func writeCLIRepoLockOwner(t *testing.T, repoPath string, owner any) {
	t.Helper()

	lockDir := filepath.Join(repoPath, ".jvs", "locks", "repo.lock")
	require.NoError(t, os.MkdirAll(lockDir, 0700))
	data, err := json.MarshalIndent(owner, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "owner.json"), data, 0600))
}

func exitedCLIChildPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait())
	return pid
}

func requireJSONNonNegativeNumber(t *testing.T, data map[string]any, key string) {
	t.Helper()
	value, ok := data[key].(float64)
	require.True(t, ok, "%s should be a JSON number: %#v", key, data[key])
	assert.GreaterOrEqual(t, value, float64(0), "%s should be non-negative", key)
}
