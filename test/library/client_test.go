package library_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/compression"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/jvs"
	"github.com/agentsmith-project/jvs/pkg/model"
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

func createLibraryWorkspace(t *testing.T, client *jvs.Client, name string) string {
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

func setSavePointCreatedAt(t *testing.T, client *jvs.Client, savePointID jvs.SavePointID, createdAt time.Time) {
	t.Helper()

	descriptorPath := filepath.Join(client.RepoRoot(), ".jvs", "descriptors", string(savePointID)+".json")
	data, err := os.ReadFile(descriptorPath)
	require.NoError(t, err)

	var desc model.Descriptor
	require.NoError(t, json.Unmarshal(data, &desc))
	desc.CreatedAt = createdAt.UTC()
	checksum, err := integrity.ComputeDescriptorChecksum(&desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err = json.MarshalIndent(&desc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
	syncLibraryReadyMarkerWithDescriptor(t, client.RepoRoot(), desc)
}

func syncLibraryReadyMarkerWithDescriptor(t *testing.T, repoRoot string, desc model.Descriptor) {
	t.Helper()
	snapshotDir := filepath.Join(repoRoot, ".jvs", "snapshots", string(desc.SnapshotID))
	for _, name := range []string{".READY", ".READY.gz"} {
		path := filepath.Join(snapshotDir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		var marker map[string]any
		require.NoError(t, json.Unmarshal(data, &marker))
		marker["snapshot_id"] = string(desc.SnapshotID)
		marker["payload_root_hash"] = string(desc.PayloadRootHash)
		marker["descriptor_checksum"] = string(desc.DescriptorChecksum)
		data, err = json.MarshalIndent(marker, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0644))
	}
}

func createOldOrphanSavePoint(t *testing.T, client *jvs.Client, age time.Duration) jvs.SavePointID {
	t.Helper()

	tempPath := createLibraryWorkspace(t, client, "temp")
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("temp"), 0644))

	desc, err := client.Save(context.Background(), jvs.SaveOptions{
		WorkspaceName: "temp",
		Message:       "temporary workspace save point",
	})
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(filepath.Join(client.RepoRoot(), ".jvs", "worktrees", "temp")))
	require.NoError(t, os.RemoveAll(tempPath))

	setSavePointCreatedAt(t, client, desc.SavePointID, time.Now().Add(-age))
	return desc.SavePointID
}

func uniqueSavePointIDPrefix(id jvs.SavePointID, others ...jvs.SavePointID) string {
	full := string(id)
	for i := 1; i <= len(full); i++ {
		prefix := full[:i]
		unique := true
		for _, other := range others {
			if strings.HasPrefix(string(other), prefix) {
				unique = false
				break
			}
		}
		if unique {
			return prefix
		}
	}
	return full
}

func TestPublicFacadeUsesSaveCleanupNames(t *testing.T) {
	clientType := reflect.TypeOf(&jvs.Client{})
	for _, name := range []string{
		"Save",
		"LatestSavePoint",
		"HasSavePoints",
		"PreviewCleanup",
		"RunCleanup",
		"WorkspacePath",
	} {
		_, ok := clientType.MethodByName(name)
		assert.Truef(t, ok, "expected public Client.%s", name)
	}

	var _ jvs.SaveOptions
	var _ jvs.CleanupOptions
	var _ jvs.CleanupPlan
}

func TestCleanupProtectionReasonIsStringLikePublicFacade(t *testing.T) {
	group := jvs.CleanupProtectionGroup{Reason: jvs.CleanupProtectionReasonHistory}

	var reason string = group.Reason
	byReason := map[string]bool{group.Reason: true}
	assert.True(t, byReason[reason])
	assert.Equal(t, "history", acceptCleanupProtectionReasonString(group.Reason))
	assert.True(t, group.Reason == jvs.CleanupProtectionReasonHistory)
}

func acceptCleanupProtectionReasonString(reason string) string {
	return reason
}

func TestPublicFacadeKeepsLegacyStorageNamesOutOfPublicAPI(t *testing.T) {
	clientType := reflect.TypeOf(&jvs.Client{})
	// These legacy names may still appear in internal storage packages, but the
	// public library facade should stay in the folder/workspace/save point vocabulary.
	for _, name := range []string{
		"Snapshot",
		"LatestSnapshot",
		"HasSnapshots",
		"GC",
		"RunGC",
		"WorktreePayloadPath",
	} {
		_, ok := clientType.MethodByName(name)
		assert.Falsef(t, ok, "old public Client.%s must not remain", name)
	}

	forbiddenTypes := map[string]struct{}{
		"SnapshotOptions": {},
		"GCOptions":       {},
		"GCPlan":          {},
	}
	forbiddenMethods := map[string]struct{}{
		"Snapshot":            {},
		"LatestSnapshot":      {},
		"HasSnapshots":        {},
		"GC":                  {},
		"RunGC":               {},
		"WorktreePayloadPath": {},
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "..", "pkg", "jvs"), nil, 0)
	require.NoError(t, err)
	pkg, ok := pkgs["jvs"]
	require.True(t, ok, "pkg/jvs should parse as package jvs")

	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, forbidden := forbiddenTypes[typeSpec.Name.Name]; forbidden {
						t.Fatalf("old public type %s must not remain in pkg/jvs", typeSpec.Name.Name)
					}
				}
			case *ast.FuncDecl:
				if decl.Recv == nil {
					continue
				}
				if _, forbidden := forbiddenMethods[decl.Name.Name]; forbidden {
					t.Fatalf("old public Client.%s must not remain in pkg/jvs", decl.Name.Name)
				}
			}
		}
	}
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

func TestHasSavePoints_FalseOnEmptyRepo(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx := context.Background()
	has, err := client.HasSavePoints(ctx, "main")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestSave_CreateAndVerify(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	// Write a file to the workspace
	mainDir := client.WorkspacePath("main")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "hello.txt"), []byte("world"), 0644))

	ctx := context.Background()
	desc, err := client.Save(ctx, jvs.SaveOptions{
		Message: "first save point",
		Tags:    []string{"v1", "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, desc)

	assert.NotEmpty(t, desc.SavePointID)
	assert.Equal(t, "first save point", desc.Message)
	assert.Equal(t, []string{"v1", "test"}, desc.Tags)
	assert.Equal(t, model.IntegrityVerified, desc.IntegrityState)

	has, err := client.HasSavePoints(ctx, "main")
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, client.Verify(ctx, desc.SavePointID))
}

func TestVerify_CompressedInternalSavePointUsesLogicalPayload(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("compressed data"), 0644))

	creator := snapshot.NewCreator(client.RepoRoot(), model.EngineCopy)
	creator.SetCompression(compression.LevelDefault)
	desc, err := creator.Create("main", "compressed", nil)
	require.NoError(t, err)
	require.NotNil(t, desc.Compression)

	require.NoError(t, client.Verify(context.Background(), jvs.SavePointID(desc.SnapshotID)))
}

func TestSave_FailsInDetachedState(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("first"), 0644))
	first, err := client.Save(ctx, jvs.SaveOptions{Message: "first"})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("second"), 0644))
	_, err = client.Save(ctx, jvs.SaveOptions{Message: "second"})
	require.NoError(t, err)

	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{Target: string(first.SavePointID)}))

	_, err = client.Save(ctx, jvs.SaveOptions{Message: "detached save point"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached")
}

func TestSave_RestoreLatest(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	// Write file and save
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data.txt"), []byte("original"), 0644))
	_, err = client.Save(ctx, jvs.SaveOptions{Message: "original state"})
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

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	// Create 3 save points
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(mainDir, "counter.txt"),
			[]byte{byte('0' + i)},
			0644,
		))
		_, err := client.Save(ctx, jvs.SaveOptions{
			Message: "save point " + string(rune('A'+i)),
			Tags:    []string{"test"},
		})
		require.NoError(t, err)
	}

	// Get all history
	history, err := client.History(ctx, "main", 0)
	require.NoError(t, err)
	assert.Len(t, history, 3)
	// Newest first
	assert.Equal(t, "save point C", history[0].Message)
	assert.Equal(t, "save point A", history[2].Message)

	// Get limited history
	limited, err := client.History(ctx, "main", 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}

func TestLatestSavePoint(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	// No save points yet
	latest, err := client.LatestSavePoint(ctx, "main")
	require.NoError(t, err)
	assert.Nil(t, latest)

	// Create a save point
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("data"), 0644))
	desc, err := client.Save(ctx, jvs.SaveOptions{Message: "first"})
	require.NoError(t, err)

	latest, err = client.LatestSavePoint(ctx, "main")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, desc.SavePointID, latest.SavePointID)
}

func TestRestore_RequiresSavePointIDPrefixTarget(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	// Create two save points with different content
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("v1"), 0644))
	desc1, err := client.Save(ctx, jvs.SaveOptions{Message: "version-1", Tags: []string{"release-target"}})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("v2"), 0644))
	desc2, err := client.Save(ctx, jvs.SaveOptions{Message: "version-2", Tags: []string{"latest-target"}})
	require.NoError(t, err)

	// Restore by save point ID prefix
	require.NoError(t, client.Restore(ctx, jvs.RestoreOptions{
		Target: uniqueSavePointIDPrefix(desc1.SavePointID, desc2.SavePointID),
	}))
	data, err := os.ReadFile(filepath.Join(mainDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(data))

	for _, target := range []string{"latest-target", "version-2"} {
		err = client.Restore(ctx, jvs.RestoreOptions{Target: target})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "save point ID")

		data, err = os.ReadFile(filepath.Join(mainDir, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "v1", string(data))
	}
}

func TestPreviewCleanup(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")
	ctx := context.Background()

	// Create a save point
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("data"), 0644))
	_, err = client.Save(ctx, jvs.SaveOptions{Message: "keep me"})
	require.NoError(t, err)

	plan, err := client.PreviewCleanup(ctx, jvs.CleanupOptions{})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotEmpty(t, plan.ProtectedSavePoints)
	assert.Contains(t, plan.ProtectedSavePoints, plan.ProtectedSavePoints[0])
}

func TestPreviewCleanupPublicJSONUsesSavePointFields(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx := context.Background()
	orphanID := createOldOrphanSavePoint(t, client, 48*time.Hour)

	plan, err := client.PreviewCleanup(ctx, jvs.CleanupOptions{})
	require.NoError(t, err)
	assert.Contains(t, plan.ReclaimableSavePoints, orphanID)

	data, err := json.Marshal(plan)
	require.NoError(t, err)
	encoded := string(data)
	assert.Contains(t, encoded, "protected_save_points")
	assert.Contains(t, encoded, "protected_by_history")
	assert.Contains(t, encoded, "reclaimable_save_points")
	assert.Contains(t, encoded, "reclaimable_bytes_estimate")
	assert.NotContains(t, encoded, "protected_set")
	assert.NotContains(t, encoded, "protected_by_lineage")
	assert.NotContains(t, encoded, "protected_by_pin")
	assert.NotContains(t, encoded, "protected_by_retention")
	assert.NotContains(t, encoded, "retention_policy")
	assert.NotContains(t, encoded, "to_delete")
	assert.NotContains(t, encoded, "deletable_bytes_estimate")
	assert.NotContains(t, encoded, "checkpoint")
	assert.NotContains(t, encoded, "snapshot")
	assert.NotContains(t, encoded, "gc")
	assert.NotContains(t, encoded, "keep_min_")
}

func TestPreviewCleanupActiveOperationScanErrorUsesPublicLanguage(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo", EngineType: model.EngineCopy})
	require.NoError(t, err)
	blockLibraryIntentDirectory(t, client.RepoRoot())

	_, err = client.PreviewCleanup(context.Background(), jvs.CleanupOptions{})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "active operations")
	assert.Contains(t, err.Error(), "doctor --strict")
	assertLibraryCleanupErrorOmitsInternalVocabulary(t, err.Error())
}

func TestPreviewCleanupDamagedReadyErrorUsesPublicLanguage(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo", EngineType: model.EngineCopy})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("baseline"), 0644))
	savePoint, err := client.Save(context.Background(), jvs.SaveOptions{Message: "baseline"})
	require.NoError(t, err)
	corruptLibraryReadyMarker(t, client.RepoRoot(), savePoint.SavePointID)

	_, err = client.PreviewCleanup(context.Background(), jvs.CleanupOptions{})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "save point storage")
	assert.Contains(t, err.Error(), "doctor --strict")
	assertLibraryCleanupErrorOmitsInternalVocabulary(t, err.Error())
}

func TestClientOperations_ReturnContextErrorWhenCanceled(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Save(ctx, jvs.SaveOptions{Message: "canceled"})
	require.ErrorIs(t, err, context.Canceled)

	err = client.Restore(ctx, jvs.RestoreOptions{Target: "missing-save-point"})
	require.ErrorIs(t, err, context.Canceled)

	err = client.RestoreLatest(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.History(ctx, "main", 0)
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.LatestSavePoint(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.HasSavePoints(ctx, "main")
	require.ErrorIs(t, err, context.Canceled)

	err = client.Verify(ctx, jvs.SavePointID("missing"))
	require.ErrorIs(t, err, context.Canceled)

	_, err = client.PreviewCleanup(ctx, jvs.CleanupOptions{})
	require.ErrorIs(t, err, context.Canceled)

	err = client.RunCleanup(ctx, "missing-plan")
	require.ErrorIs(t, err, context.Canceled)
}

func blockLibraryIntentDirectory(t *testing.T, repoRoot string) {
	t.Helper()

	intentsDir := filepath.Join(repoRoot, repo.JVSDirName, "intents")
	require.NoError(t, os.RemoveAll(intentsDir))
	require.NoError(t, os.WriteFile(intentsDir, []byte("not a directory"), 0644))
}

func corruptLibraryReadyMarker(t *testing.T, repoRoot string, savePointID jvs.SavePointID) {
	t.Helper()

	readyPath := filepath.Join(repoRoot, repo.JVSDirName, "snapshots", string(savePointID), ".READY")
	require.NoError(t, os.WriteFile(readyPath, []byte("{not json"), 0644))
}

func assertLibraryCleanupErrorOmitsInternalVocabulary(t *testing.T, value string) {
	t.Helper()

	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"checkpoint",
		"publish state",
		"ready marker",
		"intents",
		"intent",
		".jvs",
		"control path",
		"control directory",
		"stat ",
		"gc",
	} {
		assert.NotContains(t, lower, forbidden)
	}
}

func TestWorkspacePath(t *testing.T) {
	dir := testRepoDir(t)
	client, err := jvs.Init(dir, jvs.InitOptions{Name: "test-repo"})
	require.NoError(t, err)

	mainPath := client.WorkspacePath("main")
	assert.Equal(t, filepath.Join(dir, "main"), mainPath)

	// Empty defaults to main
	defaultPath := client.WorkspacePath("")
	assert.Equal(t, mainPath, defaultPath)

	outside := t.TempDir()
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "worktrees")))
	if err := os.Symlink(outside, filepath.Join(dir, "worktrees")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	assert.Empty(t, client.WorkspacePath("unsafe"))
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

func TestFullLifecycle_SaveRestoreCleanup(t *testing.T) {
	dir := testRepoDir(t)
	ctx := context.Background()

	// 1. Initialize
	client, err := jvs.OpenOrInit(dir, jvs.InitOptions{Name: "agent-workspace"})
	require.NoError(t, err)

	mainDir := client.WorkspacePath("main")

	// 2. Simulate agent writing files
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "config.json"), []byte(`{"model":"gpt-4"}`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(mainDir, "data"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "data", "results.csv"), []byte("a,b,c\n1,2,3\n"), 0644))

	// 3. Save (pod shutdown)
	desc, err := client.Save(ctx, jvs.SaveOptions{
		Message: "auto: pod shutdown",
		Tags:    []string{"auto", "shutdown"},
	})
	require.NoError(t, err)

	// 4. Simulate workspace corruption (pod deleted, files gone)
	require.NoError(t, os.RemoveAll(filepath.Join(mainDir, "config.json")))
	require.NoError(t, os.RemoveAll(filepath.Join(mainDir, "data")))

	// 5. Restore (pod startup)
	has, err := client.HasSavePoints(ctx, "main")
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

	// 7. Verify save point integrity
	require.NoError(t, client.Verify(ctx, desc.SavePointID))

	// 8. Preview cleanup
	plan, err := client.PreviewCleanup(ctx, jvs.CleanupOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, plan.CandidateCount) // only 1 save point, currently protected
}
