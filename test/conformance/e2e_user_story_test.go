//go:build conformance

package conformance

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoryLocal_StartRepoDiscoveryAndUnsafeSetup(t *testing.T) {
	base, repoRoot, mainPath := storyNewRepo(t, "story-repo")

	list := storyRun(t, repoRoot, "--no-color", "workspace", "list")
	storyRequireSuccess(t, list, "workspace list from repo root")
	storyRequireContains(t, list.stdout, "main", "workspace list")

	statusFromRoot := storyRun(t, repoRoot, "--no-color", "--workspace", "main", "status")
	storyRequireSuccess(t, statusFromRoot, "status from repo root with workspace")
	storyRequireContains(t, statusFromRoot.stdout, "Workspace: main", "status output")
	storyRequireContains(t, statusFromRoot.stdout, "Current: (none)", "empty status output")

	nestedPath := filepath.Join(mainPath, "src", "pkg")
	storyMkdir(t, mainPath, "src/pkg")
	statusFromNested := storyRun(t, nestedPath, "--no-color", "status")
	storyRequireSuccess(t, statusFromNested, "status from nested workspace path")
	storyRequireContains(t, statusFromNested.stdout, "Workspace: main", "nested status output")
	storyRequireContains(t, statusFromNested.stdout, repoRoot, "nested status repo")

	unsafeTarget := filepath.Join(base, "unsafe-target")
	storyMkdir(t, base, "unsafe-target")
	storyWriteText(t, unsafeTarget, "keep.txt", "user data")
	rejectedNonEmpty := storyRun(t, base, "--no-color", "init", unsafeTarget)
	storyRequireFailure(t, rejectedNonEmpty, "init non-empty target")
	storyRequireContains(t, storyCombinedOutput(rejectedNonEmpty), "empty or not exist", "unsafe init rejection")
	storyRequireText(t, unsafeTarget, "keep.txt", "user data")
	storyRequirePathMissing(t, filepath.Join(unsafeTarget, ".jvs"))

	nestedRepoTarget := filepath.Join(mainPath, "nested-repo")
	rejectedNested := storyRun(t, base, "--no-color", "init", nestedRepoTarget)
	storyRequireFailure(t, rejectedNested, "init nested repo")
	storyRequireContains(t, storyCombinedOutput(rejectedNested), "nested repository", "nested init rejection")
	storyRequirePathMissing(t, filepath.Join(nestedRepoTarget, ".jvs"))
}

func TestStoryLocal_ImportExistingWorkPreservesPayloadPurity(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "existing-work")
	storyWriteText(t, source, "README.md", "# Existing\n")
	storyWriteText(t, source, "src/app.txt", "payload\n")
	binaryPayload := []byte{0, 1, 2, 200, 255}
	storyWriteBytes(t, source, "bin/payload.dat", binaryPayload)
	storyMkdir(t, source, "empty-dir")

	repoRoot := filepath.Join(base, "imported-repo")
	imported := storyRun(t, base, "--no-color", "import", source, repoRoot)
	storyRequireSuccess(t, imported, "import existing work")
	storyRequireContains(t, imported.stdout, "Imported directory", "import output")
	storyRequireContains(t, imported.stdout, "Initial checkpoint", "import output")

	mainPath := filepath.Join(repoRoot, "main")
	storyRequireText(t, mainPath, "README.md", "# Existing\n")
	storyRequireText(t, mainPath, "src/app.txt", "payload\n")
	storyRequireBytes(t, mainPath, "bin/payload.dat", binaryPayload)
	storyRequirePathExists(t, filepath.Join(mainPath, "empty-dir"))
	storyRequirePathMissing(t, filepath.Join(mainPath, ".jvs"))

	storyRequireText(t, source, "README.md", "# Existing\n")
	storyRequireBytes(t, source, "bin/payload.dat", binaryPayload)

	checkpoints := storyCheckpointList(t, mainPath)
	if len(checkpoints) != 1 {
		t.Fatalf("import should create exactly one initial checkpoint, got %d", len(checkpoints))
	}
	if checkpoints[0].Workspace != "main" || checkpoints[0].CheckpointID == "" {
		t.Fatalf("unexpected import checkpoint record: %#v", checkpoints[0])
	}
	storyFindCheckpointByTag(t, checkpoints, "import")

	workspacePath := storyRun(t, repoRoot, "--no-color", "workspace", "path", "main")
	storyRequireSuccess(t, workspacePath, "workspace path main")
	if strings.TrimSpace(workspacePath.stdout) != mainPath {
		t.Fatalf("workspace path = %q, want %q", strings.TrimSpace(workspacePath.stdout), mainPath)
	}

	sourceWithMetadata := filepath.Join(base, "source-with-metadata")
	storyWriteText(t, sourceWithMetadata, "keep.txt", "keep\n")
	storyMkdir(t, sourceWithMetadata, ".jvs")
	rejectedTarget := filepath.Join(base, "rejected-import")
	rejected := storyRun(t, base, "--no-color", "import", sourceWithMetadata, rejectedTarget)
	storyRequireFailure(t, rejected, "import source with metadata")
	storyRequireContains(t, storyCombinedOutput(rejected), "must not contain .jvs metadata", "metadata rejection")
	storyRequireText(t, sourceWithMetadata, "keep.txt", "keep\n")
	storyRequirePathMissing(t, filepath.Join(rejectedTarget, ".jvs"))
}

func TestStoryLocal_SaveProgressStateHistoryAndRefs(t *testing.T) {
	_, repoRoot, mainPath := storyNewRepo(t, "story-progress")

	storyWriteText(t, mainPath, "README.md", "version 1\n")
	storyWriteText(t, mainPath, "src/app.txt", "app v1\n")
	logo := []byte{0x89, 'J', 'V', 'S', 0, 42}
	storyWriteBytes(t, mainPath, "assets/logo.bin", logo)
	storyMkdir(t, mainPath, "cache/empty")
	first := storyCheckpointHuman(t, repoRoot, "initial payload", "base")

	status := storyStatus(t, mainPath)
	storyRequireStatus(t, status, first.CheckpointID, first.CheckpointID, false, true)

	storyWriteText(t, mainPath, "README.md", "version 2\n")
	storyRemove(t, mainPath, "src/app.txt")
	storyWriteText(t, mainPath, "docs/guide.md", "how to use v2\n")
	dirtyStatus := storyStatus(t, mainPath)
	storyRequireStatus(t, dirtyStatus, first.CheckpointID, first.CheckpointID, true, false)

	second := storyCheckpointHuman(t, repoRoot, "feature update", "tip")
	status = storyStatus(t, mainPath)
	storyRequireStatus(t, status, second.CheckpointID, second.CheckpointID, false, true)

	history := storyRun(t, mainPath, "--no-color", "checkpoint", "list")
	storyRequireSuccess(t, history, "checkpoint list")
	storyRequireContains(t, history.stdout, "initial payload", "checkpoint list")
	storyRequireContains(t, history.stdout, "feature update", "checkpoint list")
	storyRequireContains(t, history.stdout, "[current,latest]", "checkpoint list")

	diff := storyRun(t, mainPath, "--no-color", "diff", "base", "tip", "--stat")
	storyRequireSuccess(t, diff, "diff base tip")
	storyRequireContains(t, diff.stdout, "Added: 1", "diff stat")
	storyRequireContains(t, diff.stdout, "Removed: 1", "diff stat")
	storyRequireContains(t, diff.stdout, "Modified: 1", "diff stat")

	restoreBase := storyRun(t, mainPath, "--no-color", "restore", "base")
	storyRequireSuccess(t, restoreBase, "restore by exact tag")
	storyRequireText(t, mainPath, "README.md", "version 1\n")
	storyRequireText(t, mainPath, "src/app.txt", "app v1\n")
	storyRequirePathMissing(t, filepath.Join(mainPath, "docs", "guide.md"))
	storyRequireBytes(t, mainPath, "assets/logo.bin", logo)
	storyRequirePathExists(t, filepath.Join(mainPath, "cache", "empty"))
	status = storyStatus(t, mainPath)
	storyRequireStatus(t, status, first.CheckpointID, second.CheckpointID, false, false)

	blockedCheckpoint := storyRun(t, mainPath, "--no-color", "checkpoint", "should fail from history")
	storyRequireFailure(t, blockedCheckpoint, "checkpoint from historical state")
	storyRequireContains(t, storyCombinedOutput(blockedCheckpoint), "cannot checkpoint while current differs from latest", "historical checkpoint guard")
	storyRequireContains(t, storyCombinedOutput(blockedCheckpoint), "jvs fork", "historical checkpoint hint")

	restoreLatestByPrefix := storyRun(t, mainPath, "--no-color", "restore", storyUniquePrefix(second.CheckpointID))
	storyRequireSuccess(t, restoreLatestByPrefix, "restore by unique checkpoint prefix")
	storyRequireText(t, mainPath, "README.md", "version 2\n")
	storyRequirePathExists(t, filepath.Join(mainPath, "docs", "guide.md"))
	storyRequirePathMissing(t, filepath.Join(mainPath, "src", "app.txt"))
	status = storyStatus(t, mainPath)
	storyRequireStatus(t, status, second.CheckpointID, second.CheckpointID, false, true)

	reservedTag := storyRun(t, mainPath, "--no-color", "checkpoint", "reserved tag", "--tag", "latest")
	storyRequireFailure(t, reservedTag, "checkpoint with reserved tag")
	storyRequireContains(t, storyCombinedOutput(reservedTag), "reserved ref", "reserved tag rejection")
}

func TestStoryLocal_DirtyWorkSafetyMatrix(t *testing.T) {
	_, repoRoot, mainPath := storyNewRepo(t, "story-safety")

	storyWriteText(t, mainPath, "data.txt", "v1\n")
	first := storyCheckpointHuman(t, repoRoot, "safe base", "base")
	storyWriteText(t, mainPath, "data.txt", "v2\n")
	second := storyCheckpointHuman(t, repoRoot, "safe tip", "tip")

	storyWriteText(t, mainPath, "data.txt", "dirty\n")
	storyWriteText(t, mainPath, "scratch.txt", "do not overwrite\n")
	rejectedRestore := storyRun(t, mainPath, "--no-color", "restore", "base")
	storyRequireFailure(t, rejectedRestore, "dirty restore default")
	storyRequireContains(t, storyCombinedOutput(rejectedRestore), "dirty changes", "dirty restore rejection")
	storyRequireText(t, mainPath, "data.txt", "dirty\n")
	storyRequireText(t, mainPath, "scratch.txt", "do not overwrite\n")

	discardedRestore := storyRun(t, mainPath, "--no-color", "restore", "base", "--discard-dirty")
	storyRequireSuccess(t, discardedRestore, "restore discard dirty")
	storyRequireText(t, mainPath, "data.txt", "v1\n")
	storyRequirePathMissing(t, filepath.Join(mainPath, "scratch.txt"))
	status := storyStatus(t, mainPath)
	storyRequireStatus(t, status, first.CheckpointID, second.CheckpointID, false, false)

	restoreLatest := storyRun(t, mainPath, "--no-color", "restore", "latest")
	storyRequireSuccess(t, restoreLatest, "restore latest")
	storyRequireText(t, mainPath, "data.txt", "v2\n")

	storyWriteText(t, mainPath, "data.txt", "included dirty\n")
	includedRestore := storyRun(t, mainPath, "--no-color", "restore", "base", "--include-working")
	storyRequireSuccess(t, includedRestore, "restore include working")
	storyRequireText(t, mainPath, "data.txt", "v1\n")
	status = storyStatus(t, mainPath)
	if status.Current != first.CheckpointID || status.Latest == second.CheckpointID || status.Dirty || status.AtLatest {
		t.Fatalf("include-working restore status did not preserve current/base and new latest: %#v", status)
	}
	if got := len(storyCheckpointList(t, mainPath)); got != 3 {
		t.Fatalf("include-working restore should create a third checkpoint, got %d", got)
	}

	storyWriteText(t, mainPath, "data.txt", "mutual exclusion remains\n")
	mutual := storyRun(t, mainPath, "--no-color", "restore", "latest", "--include-working", "--discard-dirty")
	storyRequireFailure(t, mutual, "restore mutually exclusive safety flags")
	storyRequireContains(t, storyCombinedOutput(mutual), "cannot be used together", "mutual exclusion")
	storyRequireText(t, mainPath, "data.txt", "mutual exclusion remains\n")

	cleanLatest := storyRun(t, mainPath, "--no-color", "restore", "latest", "--discard-dirty")
	storyRequireSuccess(t, cleanLatest, "restore latest after mutual exclusion")

	createDirtyWorkspace := storyRun(t, mainPath, "--no-color", "fork", "dirty-feature")
	storyRequireSuccess(t, createDirtyWorkspace, "fork dirty-feature")
	dirtyFeaturePath := filepath.Join(repoRoot, "worktrees", "dirty-feature")
	storyWriteText(t, dirtyFeaturePath, "scratch.txt", "feature dirty\n")
	rejectedRemoveDirty := storyRun(t, repoRoot, "--no-color", "workspace", "remove", "dirty-feature")
	storyRequireFailure(t, rejectedRemoveDirty, "workspace remove dirty")
	storyRequireContains(t, storyCombinedOutput(rejectedRemoveDirty), "dirty changes", "dirty workspace remove rejection")
	storyRequireText(t, dirtyFeaturePath, "scratch.txt", "feature dirty\n")
	forcedRemoveDirty := storyRun(t, repoRoot, "--no-color", "workspace", "remove", "--force", "dirty-feature")
	storyRequireSuccess(t, forcedRemoveDirty, "workspace remove dirty force")
	storyRequirePathMissing(t, dirtyFeaturePath)

	createHistoricalWorkspace := storyRun(t, mainPath, "--no-color", "fork", "historical-clean")
	storyRequireSuccess(t, createHistoricalWorkspace, "fork historical-clean")
	historicalPath := filepath.Join(repoRoot, "worktrees", "historical-clean")
	restoreHistorical := storyRun(t, historicalPath, "--no-color", "restore", "base")
	storyRequireSuccess(t, restoreHistorical, "restore workspace to historical")
	rejectedRemoveHistorical := storyRun(t, repoRoot, "--no-color", "workspace", "remove", "historical-clean")
	storyRequireFailure(t, rejectedRemoveHistorical, "workspace remove current differs from latest")
	storyRequireContains(t, storyCombinedOutput(rejectedRemoveHistorical), "current differs from latest", "historical workspace remove rejection")
	forcedRemoveHistorical := storyRun(t, repoRoot, "--no-color", "workspace", "remove", "--force", "historical-clean")
	storyRequireSuccess(t, forcedRemoveHistorical, "workspace remove historical force")
	storyRequirePathMissing(t, historicalPath)
}

func TestStoryLocal_ForkAndParallelWorkspaceLifecycle(t *testing.T) {
	_, repoRoot, mainPath := storyNewRepo(t, "story-parallel")

	storyWriteText(t, mainPath, "app.txt", "base\n")
	baseCheckpoint := storyCheckpointHuman(t, repoRoot, "parallel base", "base")
	storyWriteText(t, mainPath, "app.txt", "tip\n")
	tipCheckpoint := storyCheckpointHuman(t, repoRoot, "parallel tip", "tip")

	restoreBase := storyRun(t, mainPath, "--no-color", "restore", "base")
	storyRequireSuccess(t, restoreBase, "restore main to base")
	forkCurrent := storyRun(t, mainPath, "--no-color", "fork", "from-current")
	storyRequireSuccess(t, forkCurrent, "fork current shorthand")
	fromCurrentPath := filepath.Join(repoRoot, "worktrees", "from-current")
	storyRequireText(t, fromCurrentPath, "app.txt", "base\n")

	restoreLatest := storyRun(t, mainPath, "--no-color", "restore", "latest")
	storyRequireSuccess(t, restoreLatest, "restore main latest")
	forkByRef := storyRun(t, mainPath, "--no-color", "fork", "base", "from-ref")
	storyRequireSuccess(t, forkByRef, "fork ref name")
	fromRefPath := filepath.Join(repoRoot, "worktrees", "from-ref")
	storyRequireText(t, fromRefPath, "app.txt", "base\n")
	forkByFlag := storyRun(t, mainPath, "--no-color", "fork", "--from", "tip", "from-flag")
	storyRequireSuccess(t, forkByFlag, "fork --from ref")
	fromFlagPath := filepath.Join(repoRoot, "worktrees", "from-flag")
	storyRequireText(t, fromFlagPath, "app.txt", "tip\n")

	storyWriteText(t, fromCurrentPath, "feature.txt", "feature current\n")
	currentBranchCheckpoint := storyCheckpointJSONAt(t, fromCurrentPath, "current branch work", "current-branch")
	fromCurrentStatus := storyStatus(t, fromCurrentPath)
	storyRequireStatus(t, fromCurrentStatus, currentBranchCheckpoint.CheckpointID, currentBranchCheckpoint.CheckpointID, false, true)
	storyRequireText(t, mainPath, "app.txt", "tip\n")
	storyRequirePathMissing(t, filepath.Join(mainPath, "feature.txt"))

	storyWriteText(t, fromRefPath, "experiment.txt", "dirty experiment\n")
	fromRefStatus := storyStatus(t, fromRefPath)
	storyRequireStatus(t, fromRefStatus, baseCheckpoint.CheckpointID, baseCheckpoint.CheckpointID, true, false)
	mainStatus := storyStatus(t, mainPath)
	storyRequireStatus(t, mainStatus, tipCheckpoint.CheckpointID, tipCheckpoint.CheckpointID, false, true)

	listResult := storyRunJSON(t, repoRoot, "workspace", "list")
	storyRequireJSONSuccess(t, listResult, "workspace list")
	workspaces := storyJSONData[[]storyWorkspaceRecord](t, listResult)
	for _, name := range []string{"main", "from-current", "from-ref", "from-flag"} {
		if !storyHasWorkspace(workspaces, name) {
			t.Fatalf("workspace list missing %q: %#v", name, workspaces)
		}
	}

	pathResult := storyRunJSON(t, repoRoot, "workspace", "path", "from-current")
	storyRequireJSONSuccess(t, pathResult, "workspace path")
	pathData := storyJSONData[storyWorkspacePath](t, pathResult)
	if pathData.Path != fromCurrentPath {
		t.Fatalf("workspace path from-current = %q, want %q", pathData.Path, fromCurrentPath)
	}

	rename := storyRun(t, repoRoot, "--no-color", "workspace", "rename", "from-flag", "renamed-tip")
	storyRequireSuccess(t, rename, "workspace rename")
	renamedPath := filepath.Join(repoRoot, "worktrees", "renamed-tip")
	storyRequirePathMissing(t, fromFlagPath)
	storyRequireText(t, renamedPath, "app.txt", "tip\n")

	removeRenamed := storyRun(t, repoRoot, "--no-color", "workspace", "remove", "renamed-tip")
	storyRequireSuccess(t, removeRenamed, "workspace remove clean renamed workspace")
	storyRequirePathMissing(t, renamedPath)
}

func TestStoryJSON_AutomationContractForCoreStories(t *testing.T) {
	base := t.TempDir()
	repoRoot := filepath.Join(base, "json-repo")

	initResult := storyRunJSON(t, base, "init", repoRoot)
	storyRequireJSONSuccess(t, initResult, "init")
	initData := storyJSONData[map[string]any](t, initResult)
	if initData["repo_root"] != repoRoot || initData["main_workspace"] != filepath.Join(repoRoot, "main") {
		t.Fatalf("init JSON paths mismatch: %#v", initData)
	}
	if _, ok := initData["format_version"].(float64); !ok {
		t.Fatalf("init JSON missing numeric format_version: %#v", initData)
	}
	if _, ok := initData["repo_id"].(string); !ok {
		t.Fatalf("init JSON missing repo_id: %#v", initData)
	}
	if _, ok := initData["capabilities"].(map[string]any); !ok {
		t.Fatalf("init JSON missing capabilities object: %#v", initData)
	}
	if _, ok := initData["effective_engine"].(string); !ok {
		t.Fatalf("init JSON missing effective_engine: %#v", initData)
	}

	mainPath := filepath.Join(repoRoot, "main")
	storyWriteText(t, mainPath, "app.txt", "json base\n")
	baseRecord := storyCheckpointJSONAt(t, mainPath, "json base", "base")
	if baseRecord.Workspace != "main" || baseRecord.Note != "json base" || baseRecord.Engine == "" || baseRecord.IntegrityState == "" {
		t.Fatalf("checkpoint JSON missing machine-readable fields: %#v", baseRecord)
	}

	storyWriteText(t, mainPath, "app.txt", "json dirty\n")
	dirtyStatus := storyStatus(t, mainPath)
	storyRequireStatus(t, dirtyStatus, baseRecord.CheckpointID, baseRecord.CheckpointID, true, false)
	if len(dirtyStatus.RecoveryHints) == 0 {
		t.Fatalf("dirty status JSON should include recovery hints: %#v", dirtyStatus)
	}

	tipRecord := storyCheckpointJSONAt(t, mainPath, "json tip", "tip")
	if tipRecord.ParentCheckpointID != baseRecord.CheckpointID {
		t.Fatalf("tip parent = %q, want %q", tipRecord.ParentCheckpointID, baseRecord.CheckpointID)
	}

	listResult := storyRunJSON(t, mainPath, "checkpoint", "list")
	storyRequireJSONSuccess(t, listResult, "checkpoint list")
	checkpoints := storyJSONData[[]storyCheckpointRecord](t, listResult)
	if len(checkpoints) != 2 || checkpoints[0].CheckpointID != tipRecord.CheckpointID {
		t.Fatalf("checkpoint list JSON order/content mismatch: %#v", checkpoints)
	}
	storyFindCheckpointByTag(t, checkpoints, "base")
	storyFindCheckpointByTag(t, checkpoints, "tip")

	diffResult := storyRunJSON(t, mainPath, "diff", "base", "tip", "--stat")
	storyRequireJSONSuccess(t, diffResult, "diff")
	diffData := storyJSONData[storyDiffResult](t, diffResult)
	if diffData.FromCheckpoint != baseRecord.CheckpointID || diffData.ToCheckpoint != tipRecord.CheckpointID || diffData.TotalModified != 1 {
		t.Fatalf("diff JSON mismatch: %#v", diffData)
	}

	restoreResult := storyRunJSON(t, mainPath, "restore", "base")
	storyRequireJSONSuccess(t, restoreResult, "restore")
	restoreStatus := storyJSONData[storyWorkspaceStatus](t, restoreResult)
	storyRequireStatus(t, restoreStatus, baseRecord.CheckpointID, tipRecord.CheckpointID, false, false)
	storyRequireText(t, mainPath, "app.txt", "json base\n")

	forkResult := storyRunJSON(t, mainPath, "fork", "json-branch")
	storyRequireJSONSuccess(t, forkResult, "fork")
	forkData := storyJSONData[storyWorkspaceRecord](t, forkResult)
	if forkData.Workspace != "json-branch" || forkData.Current != baseRecord.CheckpointID || forkData.Latest != baseRecord.CheckpointID {
		t.Fatalf("fork JSON mismatch: %#v", forkData)
	}
	branchPath := filepath.Join(repoRoot, "worktrees", "json-branch")
	storyRequireText(t, branchPath, "app.txt", "json base\n")

	workspaceList := storyRunJSON(t, repoRoot, "workspace", "list")
	storyRequireJSONSuccess(t, workspaceList, "workspace list")
	workspaces := storyJSONData[[]storyWorkspaceRecord](t, workspaceList)
	if !storyHasWorkspace(workspaces, "main") || !storyHasWorkspace(workspaces, "json-branch") {
		t.Fatalf("workspace list JSON missing expected workspaces: %#v", workspaces)
	}

	pathResult := storyRunJSON(t, repoRoot, "workspace", "path", "json-branch")
	storyRequireJSONSuccess(t, pathResult, "workspace path")
	pathData := storyJSONData[storyWorkspacePath](t, pathResult)
	if pathData.Path != branchPath {
		t.Fatalf("workspace path JSON = %q, want %q", pathData.Path, branchPath)
	}

	renameResult := storyRunJSON(t, repoRoot, "workspace", "rename", "json-branch", "json-renamed")
	storyRequireJSONSuccess(t, renameResult, "workspace rename")
	renameData := storyJSONData[map[string]string](t, renameResult)
	if renameData["old_workspace"] != "json-branch" || renameData["workspace"] != "json-renamed" || renameData["status"] != "renamed" {
		t.Fatalf("workspace rename JSON mismatch: %#v", renameData)
	}

	removeResult := storyRunJSON(t, repoRoot, "workspace", "remove", "json-renamed")
	storyRequireJSONSuccess(t, removeResult, "workspace remove")
	removeData := storyJSONData[map[string]string](t, removeResult)
	if removeData["workspace"] != "json-renamed" || removeData["status"] != "removed" {
		t.Fatalf("workspace remove JSON mismatch: %#v", removeData)
	}
}

func TestStoryJSON_ImportAndSafetyErrorContracts(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "json-source")
	storyWriteText(t, source, "README.md", "json import\n")
	storyWriteBytes(t, source, "payload.bin", []byte{1, 2, 3, 4})

	importTarget := filepath.Join(base, "json-imported")
	importResult := storyRunJSON(t, base, "import", source, importTarget)
	storyRequireJSONSuccess(t, importResult, "import")
	importData := storyJSONData[map[string]any](t, importResult)
	if importData["scope"] != "import" || importData["requested_scope"] != "import" || importData["repo_root"] != importTarget {
		t.Fatalf("import JSON identity mismatch: %#v", importData)
	}
	if importData["main_workspace"] != filepath.Join(importTarget, "main") || importData["provenance"] != source {
		t.Fatalf("import JSON path fields mismatch: %#v", importData)
	}
	if _, ok := importData["initial_checkpoint"].(string); !ok {
		t.Fatalf("import JSON missing initial_checkpoint: %#v", importData)
	}
	if _, ok := importData["optimized_transfer"].(bool); !ok {
		t.Fatalf("import JSON missing optimized_transfer bool: %#v", importData)
	}
	if _, ok := importData["capabilities"].(map[string]any); !ok {
		t.Fatalf("import JSON missing capabilities object: %#v", importData)
	}
	storyRequireText(t, filepath.Join(importTarget, "main"), "README.md", "json import\n")
	storyRequireText(t, source, "README.md", "json import\n")

	sourceWithMetadata := filepath.Join(base, "json-source-with-metadata")
	storyWriteText(t, sourceWithMetadata, "keep.txt", "keep\n")
	storyMkdir(t, sourceWithMetadata, ".jvs")
	rejectedImport := storyRunJSON(t, base, "import", sourceWithMetadata, filepath.Join(base, "json-rejected-import"))
	storyRequireJSONFailure(t, rejectedImport, "import", "E_USAGE")
	storyRequireContains(t, rejectedImport.env.Error.Message, ".jvs metadata", "import metadata error")

	_, repoRoot, mainPath := storyNewRepo(t, "json-safety")
	storyWriteText(t, mainPath, "data.txt", "v1\n")
	baseRecord := storyCheckpointJSONAt(t, mainPath, "json safety base", "base")
	storyWriteText(t, mainPath, "data.txt", "v2\n")
	tipRecord := storyCheckpointJSONAt(t, mainPath, "json safety tip", "tip")

	storyWriteText(t, mainPath, "data.txt", "dirty\n")
	storyWriteText(t, mainPath, "scratch.txt", "scratch\n")
	dirtyRestore := storyRunJSON(t, mainPath, "restore", "base")
	storyRequireJSONFailure(t, dirtyRestore, "restore", "E_USAGE")
	storyRequireContains(t, dirtyRestore.env.Error.Message, "dirty changes", "dirty restore JSON error")
	storyRequireText(t, mainPath, "data.txt", "dirty\n")
	storyRequireText(t, mainPath, "scratch.txt", "scratch\n")

	discardRestore := storyRunJSON(t, mainPath, "restore", "base", "--discard-dirty")
	storyRequireJSONSuccess(t, discardRestore, "restore")
	discardStatus := storyJSONData[storyWorkspaceStatus](t, discardRestore)
	storyRequireStatus(t, discardStatus, baseRecord.CheckpointID, tipRecord.CheckpointID, false, false)
	storyRequirePathMissing(t, filepath.Join(mainPath, "scratch.txt"))

	storyWriteText(t, mainPath, "data.txt", "flag conflict\n")
	conflict := storyRunJSON(t, mainPath, "restore", "latest", "--include-working", "--discard-dirty")
	storyRequireJSONFailure(t, conflict, "restore", "E_USAGE")
	storyRequireContains(t, conflict.env.Error.Message, "cannot be used together", "restore flag conflict JSON error")
	storyRequireText(t, mainPath, "data.txt", "flag conflict\n")

	restoreLatest := storyRunJSON(t, mainPath, "restore", "latest", "--discard-dirty")
	storyRequireJSONSuccess(t, restoreLatest, "restore")

	reservedTag := storyRunJSON(t, mainPath, "checkpoint", "bad reserved tag", "--tag", "latest")
	storyRequireJSONFailure(t, reservedTag, "checkpoint", "E_REF_RESERVED")
	storyRequireContains(t, reservedTag.env.Error.Message, "reserved ref", "reserved tag JSON error")

	forkDirty := storyRunJSON(t, mainPath, "fork", "dirty-json-branch")
	storyRequireJSONSuccess(t, forkDirty, "fork")
	dirtyBranchPath := filepath.Join(repoRoot, "worktrees", "dirty-json-branch")
	storyWriteText(t, dirtyBranchPath, "dirty.txt", "dirty branch\n")
	removeDirty := storyRunJSON(t, repoRoot, "workspace", "remove", "dirty-json-branch")
	storyRequireJSONFailure(t, removeDirty, "workspace remove", "E_USAGE")
	storyRequireContains(t, removeDirty.env.Error.Message, "dirty changes", "workspace remove dirty JSON error")
	storyRequireText(t, dirtyBranchPath, "dirty.txt", "dirty branch\n")

	removeForce := storyRunJSON(t, repoRoot, "workspace", "remove", "--force", "dirty-json-branch")
	storyRequireJSONSuccess(t, removeForce, "workspace remove")
}
