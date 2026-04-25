package library_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/compression"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/pkg/jvs"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepoDir(t *testing.T) string {
	t.Helper()
	base := os.Getenv("JVS_TEST_JUICEFS_PATH")
	if base == "" {
		base = t.TempDir()
	}
	dir := filepath.Join(base, t.Name())
	require.NoError(t, os.MkdirAll(dir, 0755))
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func createLibraryWorktree(t *testing.T, client *jvs.Client, name string) string {
	t.Helper()

	payloadPath := filepath.Join(client.RepoRoot(), "worktrees", name)
	require.NoError(t, os.MkdirAll(payloadPath, 0755))

	configDir := filepath.Join(client.RepoRoot(), ".jvs", "worktrees", name)
	require.NoError(t, os.MkdirAll(configDir, 0755))

	cfg := &model.WorktreeConfig{
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), data, 0644))

	return payloadPath
}

func setSnapshotCreatedAt(t *testing.T, client *jvs.Client, snapshotID model.SnapshotID, createdAt time.Time) {
	t.Helper()

	descriptorPath := filepath.Join(client.RepoRoot(), ".jvs", "descriptors", string(snapshotID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)

	var desc model.Descriptor
	require.NoError(t, json.Unmarshal(data, &desc))
	desc.CreatedAt = createdAt.UTC()

	data, err = json.MarshalIndent(&desc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
}

func createOldOrphanSnapshot(t *testing.T, client *jvs.Client, age time.Duration) model.SnapshotID {
	t.Helper()

	tempPath := createLibraryWorktree(t, client, "temp")
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644))

	desc, err := client.Snapshot(context.Background(), jvs.SnapshotOptions{
		WorktreeName: "temp",
		Note:         "temporary worktree snapshot",
	})
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(client.RepoRoot(), ".jvs", "worktrees", "temp")))
	require.NoError(t, os.RemoveAll(tempPath))

	setSnapshotCreatedAt(t, client, desc.SnapshotID, time.Now().Add(-age))
	return desc.SnapshotID
}

func TestInit_CreatesNewRepo(t *testing.T) {
	dir := testRepoDir(t)

	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.DirExists(t, filepath.Join(dir, ".jvs"))
	assert.DirExists(t, filepath.Join(dir, "main"))
	assert.NotEmpty(t, client.RepoID())
	assert.Equal(t, dir, client.RepoRoot())
}

func TestOpen_OpensExistingRepo(t *testing.T) {
	dir := testRepoDir(t)

	original, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	opened, err := jvs.Open(dir)
	require.NoError(t, err)
	assert.Equal(t, original.RepoRoot(), opened.RepoRoot())
	assert.Equal(t, original.RepoID(), opened.RepoID())
}

func TestOpenOrInit_InitializesWhenMissing(t *testing.T) {
	dir := testRepoDir(t)

	client, err := jvs.OpenOrInit(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, ".jvs"))
	assert.NotEmpty(t, client.RepoID())
}

func TestOpenOrInit_OpensWhenExists(t *testing.T) {
	dir := testRepoDir(t)

	first, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	second, err := jvs.OpenOrInit(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)
	assert.Equal(t, first.RepoID(), second.RepoID())
}

func TestOpenOrInit_OpensParentRepoFromMainWorktree(t *testing.T) {
	dir := testRepoDir(t)

	first, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	second, err := jvs.OpenOrInit(filepath.Join(dir, "main"), jvs.InitOptions{Name: "nested-repo"})
	require.NoError(t, err)

	assert.Equal(t, first.RepoRoot(), second.RepoRoot())
	assert.Equal(t, first.RepoID(), second.RepoID())
	assert.NoDirExists(t, filepath.Join(dir, "main", ".jvs"))
}

func TestHasSnapshots_FalseOnEmptyRepo(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx := context.Background()
	has, err := client.HasSnapshots(ctx, "main")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestSnapshot_CreateAndVerify(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	// Write a file to the workspace
	mainDir := client.WorktreePayloadPath("main")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "hello.txt"), []byte("world"), 0644))

	ctx := context.Background()
	desc, err := client.Snapshot(ctx, jvs.SnapshotOptions{
		Note: "first snapshot",
		Tags: []string{"v1", "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, desc)

	assert.NotEmpty(t, desc.SnapshotID)
	assert.Equal(t, "first snapshot", desc.Note)
	assert.Equal(t, []string{"v1", "test"}, desc.Tags)
	assert.Equal(t, model.IntegrityVerified, desc.IntegrityState)

	has, err := client.HasSnapshots(ctx, "main")
	require.NoError(t, err)
	assert.True(t, has)

	// Verify integrity
	require.NoError(t, client.Verify(ctx, desc.SnapshotID))
}

func TestVerify_CompressedSnapshotUsesLogicalPayload(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("compressed data"), 0644))

	creator := snapshot.NewCreator(client.RepoRoot(), model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)
	desc, err := creator.Create("main", "compressed", nil)
	require.NoError(t, err)
	require.NotNil(t, desc.Compression)

	require.NoError(t, client.Verify(context.Background(), desc.SnapshotID))
}

func TestSnapshot_FailsInDetachedState(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("first"), 0644))
	first, err := client.Snapshot(ctx, jvs.SnapshotOptions{Note: "first"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("second"), 0644))
	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "second"})
	require.NoError(t, err)

	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{Target: string(first.SnapshotID)}))

	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "detached snapshot"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached")
}

func TestSnapshot_RestoreLatest(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	// Write file and snapshot
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("original"), 0644))
	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "original state"})
	require.NoError(t, err)

	// Modify the file
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("modified"), 0644))

	// Verify file is modified
	data, err := os.ReadFile(filepath.Join(mainDir, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "modified", string(data))

	// Restore latest
	require.NoError(t, client.RestoreLatest(ctx, "main"))

	// Verify file is back to original
	data, err = os.ReadFile(filepath.Join(mainDir, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
}

func TestRestoreLatest_NoopOnEmptyRepo(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx := context.Background()
	err = client.RestoreLatest(ctx, "main")
	require.NoError(t, err) // should be a no-op, not an error
}

func TestHistory_OrderAndLimit(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	// Create 3 snapshots
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(mainDir, "counter.txt"),
			[]byte{byte('0' + i)},
			0644,
		))
		_, err := client.Snapshot(ctx, jvs.SnapshotOptions{
			Note: "snapshot " + string(rune('A'+i)),
			Tags: []string{"test"},
		})
		require.NoError(t, err)
	}

	// Get all history
	history, err := client.History(ctx, "main", 0)
	require.NoError(t, err)
	assert.Len(t, history, 3)
	// Newest first
	assert.Equal(t, "snapshot C", history[0].Note)
	assert.Equal(t, "snapshot A", history[2].Note)

	// Get limited history
	limited, err := client.History(ctx, "main", 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}

func TestLatestSnapshot(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	// No snapshots yet
	latest, err := client.LatestSnapshot(ctx, "main")
	require.NoError(t, err)
	assert.Nil(t, latest)

	// Create a snapshot
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("data"), 0644))
	desc, err := client.Snapshot(ctx, jvs.SnapshotOptions{Note: "first"})
	require.NoError(t, err)

	latest, err = client.LatestSnapshot(ctx, "main")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, desc.SnapshotID, latest.SnapshotID)
}

func TestRestore_ByTarget(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	// Create two snapshots with different content
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("v1"), 0644))
	desc1, err := client.Snapshot(ctx, jvs.SnapshotOptions{Note: "version-1", Tags: []string{"v1"}})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("v2"), 0644))
	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "version-2", Tags: []string{"v2"}})
	require.NoError(t, err)

	// Restore by snapshot ID prefix
	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{
		Target: string(desc1.SnapshotID),
	}))
	data, err := os.ReadFile(filepath.Join(mainDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(data))

	// Restore by tag
	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{Target: "v2"}))
	data, err = os.ReadFile(filepath.Join(mainDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(data))

	// Restore HEAD
	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{Target: "HEAD"}))
	data, err = os.ReadFile(filepath.Join(mainDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(data))
}

func TestGC_DryRun(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")
	ctx := context.Background()

	// Create a snapshot
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("data"), 0644))
	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "keep me"})
	require.NoError(t, err)

	plan, err := client.GC(ctx, jvs.GCOptions{DryRun: true})
	require.NoError(t, err)
	require.NotNil(t, plan)
	// HEAD snapshot is protected
	assert.Contains(t, plan.ProtectedSet, plan.ProtectedSet[0])
}

func TestGCOptions_RetentionAffectsPlans(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx := context.Background()
	orphanID := createOldOrphanSnapshot(t, client, 48*time.Hour)

	basePlan, err := client.GC(ctx, jvs.GCOptions{DryRun: true})
	require.NoError(t, err)
	assert.Contains(t, basePlan.ToDelete, orphanID)

	countPlan, err := client.GC(ctx, jvs.GCOptions{
		DryRun:           true,
		KeepMinSnapshots: 1,
	})
	require.NoError(t, err)
	assert.NotContains(t, countPlan.ToDelete, orphanID)
	assert.Contains(t, countPlan.ProtectedSet, orphanID)
	assert.Equal(t, 1, countPlan.RetentionPolicy.KeepMinSnapshots)
	assert.Equal(t, 1, countPlan.ProtectedByRetention)

	agePlan, err := client.GC(ctx, jvs.GCOptions{
		DryRun:     true,
		KeepMinAge: 72 * time.Hour,
	})
	require.NoError(t, err)
	assert.NotContains(t, agePlan.ToDelete, orphanID)
	assert.Contains(t, agePlan.ProtectedSet, orphanID)
	assert.Equal(t, 72*time.Hour, agePlan.RetentionPolicy.KeepMinAge)
	assert.Equal(t, 1, agePlan.ProtectedByRetention)
}

func TestClientOperations_ReturnContextErrorWhenCanceled(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Snapshot(ctx, jvs.SnapshotOptions{Note: "canceled"})
	require.ErrorIs(t, err, context.Canceled)

	err = client.Restore(ctx, jvs.RestoreOptions{Target: "HEAD"})
	require.ErrorIs(t, err, context.Canceled)

	err = client.RestoreLatest(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.History(ctx, "main", 0)
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.LatestSnapshot(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.HasSnapshots(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	err = client.Verify(ctx, model.SnapshotID("missing"))
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.GC(ctx, jvs.GCOptions{DryRun: true})
	require.ErrorIs(t, err, context.Canceled)

	err = client.RunGC(ctx, "missing-plan")
	require.ErrorIs(t, err, context.Canceled)
}

func TestWorktreePayloadPath(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainPath := client.WorktreePayloadPath("main")
	assert.Equal(t, filepath.Join(dir, "main"), mainPath)

	// Empty defaults to main
	defaultPath := client.WorktreePayloadPath("")
	assert.Equal(t, mainPath, defaultPath)

	outside := t.TempDir()
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "worktrees")))
	if err := os.Symlink(outside, filepath.Join(dir, "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	assert.Empty(t, client.WorktreePayloadPath("unsafe"))
}

func TestDetectEngine(t *testing.T) {
	dir := t.TempDir()
	engine := jvs.DetectEngine(dir)
	// On a normal filesystem without JuiceFS/reflink, should get copy
	assert.Contains(t, []model.EngineType{
		model.EngineCopy,
		model.EngineReflinkCopy,
		model.EngineJuiceFSClone,
	}, engine)
}

func TestValidateEngine_CopyAlwaysValid(t *testing.T) {
	dir := t.TempDir()
	err := jvs.ValidateEngine(dir, model.EngineCopy)
	assert.NoError(t, err)
}

func TestValidateEngine_InvalidPath(t *testing.T) {
	err := jvs.ValidateEngine("/nonexistent/path/12345", model.EngineCopy)
	assert.Error(t, err)
}

func TestFullLifecycle_CreateSnapshotRestoreCleanup(t *testing.T) {
	dir := testRepoDir(t)
	ctx := context.Background()

	// 1. Initialize
	client, err := jvs.OpenOrInit(dir, jvs.InitOptions{Name: "agent-workspace"})
	require.NoError(t, err)

	mainDir := client.WorktreePayloadPath("main")

	// 2. Simulate agent writing files
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "config.json"), []byte(`{"model":"gpt-4"}`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "data"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data", "results.csv"), []byte("a,b,c\n1,2,3\n"), 0644))

	// 3. Snapshot (pod shutdown)
	desc, err := client.Snapshot(ctx, jvs.SnapshotOptions{
		Note: "auto: pod shutdown",
		Tags: []string{"auto", "shutdown"},
	})
	require.NoError(t, err)

	// 4. Simulate workspace corruption (pod deleted, files gone)
	require.NoError(t, os.RemoveAll(filepath.Join(mainDir, "config.json")))
	require.NoError(t, os.RemoveAll(filepath.Join(mainDir, "data")))

	// 5. Restore (pod startup)
	has, err := client.HasSnapshots(ctx, "main")
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, client.RestoreLatest(ctx, "main"))

	// 6. Verify all files restored
	data, err := os.ReadFile(filepath.Join(mainDir, "config.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"model":"gpt-4"}`, string(data))

	data, err = os.ReadFile(filepath.Join(mainDir, "data", "results.csv"))
	require.NoError(t, err)
	assert.Equal(t, "a,b,c\n1,2,3\n", string(data))

	// 7. Verify snapshot integrity
	require.NoError(t, client.Verify(ctx, desc.SnapshotID))

	// 8. GC (dry run)
	plan, err := client.GC(ctx, jvs.GCOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 0, plan.CandidateCount) // only 1 snapshot, protected as HEAD
}
