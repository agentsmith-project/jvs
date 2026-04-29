package doctor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	_, err := repo.Init(dir, "test")
	require.NoError(t, err)
	return dir
}

func createTestSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "test", nil)
	require.NoError(t, err)
	return desc.SnapshotID
}

func createRemovedWorktreeSnapshot(t *testing.T, repoPath string) model.SnapshotID {
	t.Helper()

	wtMgr := worktree.NewManager(repoPath)
	_, err := wtMgr.Create("temp", nil)
	require.NoError(t, err)

	tempPath, err := wtMgr.Path("temp")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tempPath, "file.txt"), []byte("content"), 0644))

	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("temp", "recoverable", nil)
	require.NoError(t, err)
	require.NoError(t, wtMgr.Remove("temp"))
	return desc.SnapshotID
}

func writeSnapshotIntent(t *testing.T, repoPath string, snapshotID model.SnapshotID, worktreeName string) string {
	t.Helper()

	intentPath, err := repo.IntentPath(repoPath, snapshotID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(intentPath), 0755))
	intent := model.IntentRecord{
		SnapshotID:   snapshotID,
		WorktreeName: worktreeName,
		StartedAt:    time.Now().UTC(),
		Engine:       model.EngineCopy,
	}
	data, err := json.Marshal(intent)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(intentPath, data, 0644))
	return intentPath
}

func writeDoctorTestWorkspaceLocator(t *testing.T, dir, repoRoot string) []byte {
	t.Helper()

	data, err := json.Marshal(map[string]any{
		"type":           "jvs-workspace",
		"format_version": repo.FormatVersion,
		"repo_root":      repoRoot,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, repo.JVSDirName), data, 0644))
	return data
}

func copyDoctorTestTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	require.NoError(t, err)
}

func writeLineageDescriptor(t *testing.T, repoPath string, snapshotID model.SnapshotID, parentID *model.SnapshotID) {
	t.Helper()

	desc := &model.Descriptor{
		SnapshotID:      snapshotID,
		ParentID:        parentID,
		WorktreeName:    "main",
		CreatedAt:       time.Now().UTC(),
		Engine:          model.EngineCopy,
		PayloadRootHash: "hash",
		IntegrityState:  model.IntegrityVerified,
	}
	checksum, err := integrity.ComputeDescriptorChecksum(desc)
	require.NoError(t, err)
	desc.DescriptorChecksum = checksum

	data, err := json.MarshalIndent(desc, "", "  ")
	require.NoError(t, err)
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
}

func snapshotIDsContain(ids []model.SnapshotID, want model.SnapshotID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func assertWorktreeInvalidSnapshotIDFinding(t *testing.T, result *doctor.Result, field string, id model.SnapshotID) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category == "worktree" && f.Severity == "error" {
			if strings.Contains(f.Description, field) {
				assert.Contains(t, f.Description, string(id))
				return
			}
		}
	}
	t.Fatalf("expected invalid %s worktree finding in %#v", field, result.Findings)
}

func setMainWorktreeSnapshots(t *testing.T, repoPath string, head, latest model.SnapshotID) {
	t.Helper()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = head
	cfg.LatestSnapshotID = latest
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))
}

func assertMainHeadSnapshot(t *testing.T, repoPath string, want model.SnapshotID) {
	t.Helper()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.Equal(t, want, cfg.HeadSnapshotID)
}

func assertAdvanceHeadSkippedUnsafe(t *testing.T, result doctor.RepairResult) {
	t.Helper()

	assert.Equal(t, "advance_head", result.Action)
	assert.False(t, result.Success)
	assert.Zero(t, result.Cleaned)
	assert.Contains(t, result.Message, "skipped")
}

func TestDoctor_Check_Healthy(t *testing.T) {
	repoPath := setupTestRepo(t)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.True(t, result.Healthy)
	assert.Empty(t, result.Findings)
}

func TestDoctor_Check_WithSnapshots(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.True(t, result.Healthy)
}

func TestDoctorCheckReportsMissingParentWithMachineReadableCode(t *testing.T) {
	repoPath := setupTestRepo(t)
	childID := model.SnapshotID("1708300800000-deadbeef")
	parentID := model.SnapshotID("1708300700000-feedface")
	writeLineageDescriptor(t, repoPath, childID, &parentID)

	result, err := doctor.NewDoctor(repoPath).Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "lineage", "E_LINEAGE_PARENT_MISSING")
}

func TestDoctorCheckReportsParentCycleWithMachineReadableCode(t *testing.T) {
	repoPath := setupTestRepo(t)
	firstID := model.SnapshotID("1708300800000-deadbeef")
	secondID := model.SnapshotID("1708300900000-feedface")
	writeLineageDescriptor(t, repoPath, firstID, &secondID)
	writeLineageDescriptor(t, repoPath, secondID, &firstID)

	result, err := doctor.NewDoctor(repoPath).Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "lineage", "E_LINEAGE_CYCLE")
}

func TestDoctorCheckReportsHeadLatestMismatchButAllowsHistoricalHead(t *testing.T) {
	repoPath := setupTestRepo(t)
	ancestorID := model.SnapshotID("1708300800000-deadbeef")
	latestID := model.SnapshotID("1708300900000-feedface")
	unrelatedID := model.SnapshotID("1708301000000-cafebabe")
	writeLineageDescriptor(t, repoPath, ancestorID, nil)
	writeLineageDescriptor(t, repoPath, latestID, &ancestorID)
	writeLineageDescriptor(t, repoPath, unrelatedID, nil)

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = ancestorID
	cfg.LatestSnapshotID = latestID
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	result, err := doctor.NewDoctor(repoPath).Check(false)
	require.NoError(t, err)
	assertNoFindingCode(t, result, "E_WORKTREE_HEAD_LATEST_MISMATCH")

	cfg.HeadSnapshotID = unrelatedID
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	result, err = doctor.NewDoctor(repoPath).Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "worktree", "E_WORKTREE_HEAD_LATEST_MISMATCH")
}

func TestDoctor_Check_Strict(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, result.Healthy)
}

func TestDoctor_Check_OrphanIntent(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan intent file
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	os.MkdirAll(intentsDir, 0755)
	os.WriteFile(filepath.Join(intentsDir, "orphan.json"), []byte("{}"), 0644)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	// Orphan intents are warnings, not critical, so repo stays healthy
	assert.True(t, result.Healthy)
	assert.Len(t, result.Findings, 1)
	assert.Equal(t, "intent", result.Findings[0].Category)
	assert.Equal(t, "warning", result.Findings[0].Severity)
}

func TestDoctor_Check_OrphanTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan tmp file
	os.WriteFile(filepath.Join(repoPath, ".jvs", ".jvs-tmp-orphan"), []byte("data"), 0644)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	// Orphan tmp is info level, doesn't make repo unhealthy
	assert.True(t, result.Healthy || len(result.Findings) > 0)
}

func TestDoctor_Check_MissingWorktreePayload(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Remove main payload directory (simulating corruption)
	os.RemoveAll(filepath.Join(repoPath, "main"))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.NotEmpty(t, result.Findings)
	found := false
	for _, f := range result.Findings {
		if f.Category == "worktree" {
			found = true
			assert.Contains(t, f.Description, "payload directory missing")
			assert.Equal(t, "error", f.Severity)
		}
	}
	assert.True(t, found, "expected worktree finding for missing payload")
}

func TestDoctor_Check_WorktreeConfigRejectsInvalidSnapshotIDsWithoutTrustingOutsidePath(t *testing.T) {
	repoPath := setupTestRepo(t)
	invalid := model.SnapshotID("../../../outside")

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = invalid
	cfg.LatestSnapshotID = invalid
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	outsideDescriptor := filepath.Join(repoPath, ".jvs", "descriptors", string(invalid)+".json")
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideDescriptor), 0755))
	require.NoError(t, os.WriteFile(outsideDescriptor, []byte("{}"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertWorktreeInvalidSnapshotIDFinding(t, result, "head_snapshot_id", invalid)
	assertWorktreeInvalidSnapshotIDFinding(t, result, "latest_snapshot_id", invalid)
}

func TestDoctorRepairRuntimeRebindsCopiedAdoptedMainWorkspace(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("portable\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)

	copied := filepath.Join(base, "restored", "project")
	copyDoctorTestTree(t, source, copied)
	require.NoError(t, os.Rename(source, source+".offline"))

	doc := doctor.NewDoctor(copied)
	before, err := doc.Check(true)
	require.NoError(t, err)
	require.False(t, before.Healthy)
	assertFindingCode(t, before, "worktree", doctor.ErrorCodeWorktreePayloadInvalid)

	_, err = doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, after.Healthy, "findings after repair: %#v", after.Findings)

	cfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, cfg.RealPath)
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)

	payloadPath, err := repo.WorktreePayloadPath(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, payloadPath)
	assert.FileExists(t, filepath.Join(copied, "app.txt"))
}

func TestDoctorRepairRuntimeRebindsCopiedAdoptedMainWorkspaceWhenSourceStillExists(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("source remains online\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)

	copied := filepath.Join(base, "restored", "project")
	copyDoctorTestTree(t, source, copied)

	doc := doctor.NewDoctor(copied)
	_, err = doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, after.Healthy, "findings after repair: %#v", after.Findings)

	cfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, cfg.RealPath)
	assert.Equal(t, desc.SnapshotID, cfg.HeadSnapshotID)
	assert.Equal(t, desc.SnapshotID, cfg.LatestSnapshotID)

	payloadPath, err := repo.WorktreePayloadPath(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, payloadPath)
	assert.FileExists(t, filepath.Join(source, "app.txt"))
	assert.FileExists(t, filepath.Join(copied, "app.txt"))
}

func TestDoctorRepairRuntimeRemovesCopiedCleanupPlans(t *testing.T) {
	repoPath := setupTestRepo(t)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	tempResidue := filepath.Join(repoPath, ".jvs", "gc", ".jvs-tmp-cleanup-plan")
	require.NoError(t, os.WriteFile(tempResidue, []byte(`{"partial":true}`), 0644))
	childDir := filepath.Join(repoPath, ".jvs", "gc", ".jvs-tmp-child")
	require.NoError(t, os.MkdirAll(childDir, 0755))
	tombstonesDir := filepath.Join(repoPath, ".jvs", "gc", "tombstones")
	require.NoError(t, os.MkdirAll(tombstonesDir, 0755))
	tombstonePath := filepath.Join(tombstonesDir, "1708300800000-deadbeef.json")
	require.NoError(t, os.WriteFile(tombstonePath, []byte(`{"gc_state":"committed"}`), 0644))

	results, err := doctor.NewDoctor(repoPath).Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)

	result := requireRepairAction(t, results, "clean_runtime_cleanup_plans")
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Cleaned)
	assert.Contains(t, result.Message, "cleanup plan")
	assert.NotContains(t, result.Message, ".jvs")
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs", "gc", plan.PlanID+".json"))
	assert.NoFileExists(t, tempResidue)
	assert.DirExists(t, childDir)
	assert.FileExists(t, tombstonePath)

	err = gc.NewCollector(repoPath).Run(plan.PlanID)
	require.Error(t, err)
	assert.ErrorIs(t, err, errclass.ErrGCPlanMismatch)
}

func TestDoctorCheckFlagsCopiedAdoptedMainWorkspaceWhenSourceStillExists(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("source remains online\n"), 0644))

	_, err = snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)

	copied := filepath.Join(base, "restored", "project")
	copyDoctorTestTree(t, source, copied)

	result, err := doctor.NewDoctor(copied).Check(true)
	require.NoError(t, err)
	require.False(t, result.Healthy)
	assertFindingCode(t, result, "worktree", doctor.ErrorCodeWorktreePayloadInvalid)
}

func TestDoctorRepairRuntimeRebindsCopiedExternalWorkspaceWhenContentMatches(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)
	_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	})
	require.NoError(t, err)

	sourceFeature := filepath.Join(filepath.Dir(source), "feature")
	copied := filepath.Join(base, "restored", "project")
	copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
	copyDoctorTestTree(t, source, copied)
	copyDoctorTestTree(t, sourceFeature, copiedFeature)
	require.NoError(t, os.Rename(source, source+".offline"))
	require.NoError(t, os.Rename(sourceFeature, sourceFeature+".offline"))

	doc := doctor.NewDoctor(copied)
	before, err := doc.Check(true)
	require.NoError(t, err)
	require.False(t, before.Healthy)
	assertFindingCode(t, before, "worktree", doctor.ErrorCodeWorktreePayloadInvalid)

	_, err = doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, after.Healthy, "findings after repair: %#v", after.Findings)

	mainCfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, mainCfg.RealPath)
	featureCfg, err := repo.LoadWorktreeConfig(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, copiedFeature, featureCfg.RealPath)

	featurePayload, err := repo.WorktreePayloadPath(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, copiedFeature, featurePayload)
}

func TestDoctorRepairRuntimeRewritesCopiedExternalWorkspaceLocatorWhenSourceOffline(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)
	_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	})
	require.NoError(t, err)

	sourceFeature := filepath.Join(filepath.Dir(source), "feature")
	copied := filepath.Join(base, "restored", "project")
	copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
	copyDoctorTestTree(t, source, copied)
	copyDoctorTestTree(t, sourceFeature, copiedFeature)
	require.NoError(t, os.Rename(source, source+".offline"))
	require.NoError(t, os.Rename(sourceFeature, sourceFeature+".offline"))

	doc := doctor.NewDoctor(copied)
	results, err := doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)
	assertRepairActionSucceeded(t, results, doctor.RepairRebindWorkspacePaths)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, after.Healthy, "findings after repair: %#v", after.Findings)

	discovered, workspace, err := repo.DiscoverWorktree(copiedFeature)
	require.NoError(t, err)
	assert.Equal(t, copied, discovered.Root)
	assert.Equal(t, "feature", workspace)
}

func TestDoctorRepairRuntimeRejectsMalformedExternalWorkspaceLocatorEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		repoRoot string
	}{
		{name: "blank", repoRoot: ""},
		{name: "relative", repoRoot: "relative/repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			source := filepath.Join(base, "source", "project")
			require.NoError(t, os.MkdirAll(source, 0755))
			_, err := repo.InitAdoptedWorkspace(source)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

			desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
			require.NoError(t, err)
			_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
				_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
				return err
			})
			require.NoError(t, err)

			sourceFeature := filepath.Join(filepath.Dir(source), "feature")
			copied := filepath.Join(base, "restored", "project")
			copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
			copyDoctorTestTree(t, source, copied)
			copyDoctorTestTree(t, sourceFeature, copiedFeature)
			malformed := writeDoctorTestWorkspaceLocator(t, copiedFeature, tc.repoRoot)

			doc := doctor.NewDoctor(copied)
			results, err := doc.Repair(doctor.RuntimeRepairActionIDs())
			require.NoError(t, err)
			assertRepairActionFailed(t, results, doctor.RepairRebindWorkspacePaths)

			after, err := os.ReadFile(filepath.Join(copiedFeature, repo.JVSDirName))
			require.NoError(t, err)
			assert.JSONEq(t, string(malformed), string(after))

			check, err := doc.Check(true)
			require.NoError(t, err)
			assert.False(t, check.Healthy)
			assertFindingCode(t, check, "worktree", doctor.ErrorCodeWorktreePathBindingInvalid)
		})
	}
}

func TestDoctorRepairRuntimeRebindsCopiedExternalWorkspaceWhenSourceStillExistsAndContentMatches(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)
	_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	})
	require.NoError(t, err)

	sourceFeature := filepath.Join(filepath.Dir(source), "feature")
	copied := filepath.Join(base, "restored", "project")
	copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
	copyDoctorTestTree(t, source, copied)
	copyDoctorTestTree(t, sourceFeature, copiedFeature)

	doc := doctor.NewDoctor(copied)
	_, err = doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, after.Healthy, "findings after repair: %#v", after.Findings)

	mainCfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, mainCfg.RealPath)
	featureCfg, err := repo.LoadWorktreeConfig(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, copiedFeature, featureCfg.RealPath)

	featurePayload, err := repo.WorktreePayloadPath(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, copiedFeature, featurePayload)
	assert.FileExists(t, filepath.Join(sourceFeature, "app.txt"))
	assert.FileExists(t, filepath.Join(copiedFeature, "app.txt"))
}

func TestDoctorRepairRuntimeKeepsCopiedExternalWorkspaceUnhealthyWhenDestinationSiblingMissing(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)
	_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	})
	require.NoError(t, err)

	sourceFeature := filepath.Join(filepath.Dir(source), "feature")
	copied := filepath.Join(base, "restored", "project")
	copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
	copyDoctorTestTree(t, source, copied)
	require.NoDirExists(t, copiedFeature)

	doc := doctor.NewDoctor(copied)
	results, err := doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)
	assertRepairActionFailed(t, results, doctor.RepairRebindWorkspacePaths)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.False(t, after.Healthy, "findings after skipped repair: %#v", after.Findings)
	assertFindingCode(t, after, "worktree", doctor.ErrorCodeWorktreePathBindingInvalid)

	mainCfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, mainCfg.RealPath)
	featureCfg, err := repo.LoadWorktreeConfig(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, sourceFeature, featureCfg.RealPath)

	featurePayload, err := repo.WorktreePayloadPath(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, sourceFeature, featurePayload)
	assert.NoDirExists(t, copiedFeature)
}

func TestDoctorRepairRuntimeSkipsCopiedExternalWorkspaceWhenDestinationContentDiffers(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source", "project")
	require.NoError(t, os.MkdirAll(source, 0755))
	_, err := repo.InitAdoptedWorkspace(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(source, "app.txt"), []byte("baseline\n"), 0644))

	desc, err := snapshot.NewCreator(source, model.EngineCopy).CreateSavePoint("main", "baseline", nil)
	require.NoError(t, err)
	_, err = worktree.NewManager(source).CreateStartedFromSnapshot("feature", desc.SnapshotID, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	})
	require.NoError(t, err)

	sourceFeature := filepath.Join(filepath.Dir(source), "feature")
	copied := filepath.Join(base, "restored", "project")
	copiedFeature := filepath.Join(filepath.Dir(copied), "feature")
	copyDoctorTestTree(t, source, copied)
	copyDoctorTestTree(t, sourceFeature, copiedFeature)
	require.NoError(t, os.WriteFile(filepath.Join(copiedFeature, "app.txt"), []byte("different destination content\n"), 0644))

	doc := doctor.NewDoctor(copied)
	results, err := doc.Repair(doctor.RuntimeRepairActionIDs())
	require.NoError(t, err)
	assertRepairActionFailed(t, results, doctor.RepairRebindWorkspacePaths)

	after, err := doc.Check(true)
	require.NoError(t, err)
	assert.False(t, after.Healthy, "findings after skipped repair: %#v", after.Findings)
	assertFindingCode(t, after, "worktree", doctor.ErrorCodeWorktreePathBindingInvalid)

	mainCfg, err := repo.LoadWorktreeConfig(copied, "main")
	require.NoError(t, err)
	assert.Equal(t, copied, mainCfg.RealPath)
	featureCfg, err := repo.LoadWorktreeConfig(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, sourceFeature, featureCfg.RealPath)

	featurePayload, err := repo.WorktreePayloadPath(copied, "feature")
	require.NoError(t, err)
	assert.Equal(t, sourceFeature, featurePayload)
}

func TestDoctor_ListRepairActions(t *testing.T) {
	repoPath := setupTestRepo(t)
	doc := doctor.NewDoctor(repoPath)

	actions := doc.ListRepairActions()
	require.Len(t, actions, 5)
	assert.Equal(t, "clean_locks", actions[0].ID)
	assert.Equal(t, "rebind_workspace_paths", actions[1].ID)
	assert.Equal(t, "clean_runtime_tmp", actions[2].ID)
	assert.Equal(t, "clean_runtime_operations", actions[3].ID)
	assert.Equal(t, "clean_runtime_cleanup_plans", actions[4].ID)
	for _, action := range actions {
		assert.True(t, action.AutoSafe, action.ID)
		assert.NotEmpty(t, action.Description, action.ID)
	}
}

func TestDoctorRepairListOnlyListsExecutableActions(t *testing.T) {
	repoPath := setupTestRepo(t)

	actions := doctor.NewDoctor(repoPath).ListRepairActions()
	var ids []string
	for _, action := range actions {
		ids = append(ids, action.ID)
	}

	assert.Equal(t, []string{"clean_locks", "rebind_workspace_paths", "clean_runtime_tmp", "clean_runtime_operations", "clean_runtime_cleanup_plans"}, ids)
	assert.NotContains(t, ids, "rebuild_index")
	assert.NotContains(t, ids, "audit_repair")
	assert.NotContains(t, ids, "advance_head")
}

func TestDoctor_Repair_CleanTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan tmp files
	os.WriteFile(filepath.Join(repoPath, ".jvs-tmp-orphan1"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(repoPath, ".jvs-tmp-orphan2"), []byte("data"), 0644)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_tmp"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "clean_runtime_tmp", results[0].Action)
	assert.True(t, results[0].Success)
	assert.Equal(t, 2, results[0].Cleaned)

	// Verify files are gone
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs-tmp-orphan1"))
	assert.NoFileExists(t, filepath.Join(repoPath, ".jvs-tmp-orphan2"))
}

func TestDoctorRepairReturnsRepoBusyWhenMutationLockHeld(t *testing.T) {
	repoPath := setupTestRepo(t)

	held, err := repo.AcquireMutationLock(repoPath, "held-by-test")
	require.NoError(t, err)
	defer held.Release()

	_, err = doctor.NewDoctor(repoPath).Repair([]string{"clean_runtime_tmp"})
	require.ErrorIs(t, err, errclass.ErrRepoBusy)
}

func TestDoctorRepairCleanTmpDoesNotDeleteUserPayloadByName(t *testing.T) {
	repoPath := setupTestRepo(t)
	userPayloadTmp := filepath.Join(repoPath, "main", ".jvs-tmp-user-data")
	require.NoError(t, os.WriteFile(userPayloadTmp, []byte("payload"), 0644))

	results, err := doctor.NewDoctor(repoPath).Repair([]string{"clean_runtime_tmp"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.FileExists(t, userPayloadTmp, "doctor repair must not delete user payload by filename pattern")
}

func TestDoctor_Repair_CleanIntents(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan intent files
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	os.MkdirAll(intentsDir, 0755)
	os.WriteFile(filepath.Join(intentsDir, "orphan1.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(intentsDir, "orphan2.json"), []byte("{}"), 0644)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "clean_runtime_operations", results[0].Action)
	assert.True(t, results[0].Success)
	assert.Equal(t, 2, results[0].Cleaned)

	// Verify files are gone
	assert.NoFileExists(t, filepath.Join(intentsDir, "orphan1.json"))
	assert.NoFileExists(t, filepath.Join(intentsDir, "orphan2.json"))
}

func TestDoctor_Repair_CleanIntentsRemovesStaleSnapshotIntentWithoutEvidence(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()
	intentPath := writeSnapshotIntent(t, repoPath, snapshotID, "main")

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Equal(t, 1, results[0].Cleaned)
	assert.NoFileExists(t, intentPath)
}

func TestDoctor_Repair_CleanIntentsRetainsRecoverablePublishIntent(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createRemovedWorktreeSnapshot(t, repoPath)
	intentPath := writeSnapshotIntent(t, repoPath, snapshotID, "temp")

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	staleIntent := filepath.Join(intentsDir, "orphan.json")
	require.NoError(t, os.WriteFile(staleIntent, []byte("{}"), 0644))

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Equal(t, 1, results[0].Cleaned)
	assert.Contains(t, results[0].Message, "retained 1")
	assert.FileExists(t, intentPath, "descriptor/payload evidence from an uncertain publish must keep the intent")
	assert.NoFileExists(t, staleIntent)
}

func TestDoctor_Repair_CleanIntentsRetainsHeadUpdateIntentReferencedByMetadata(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.HeadSnapshotID = snapshotID
	cfg.LatestSnapshotID = snapshotID
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	intentPath := writeSnapshotIntent(t, repoPath, snapshotID, "main")

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Equal(t, 0, results[0].Cleaned)
	assert.Contains(t, results[0].Message, "retained 1")
	assert.FileExists(t, intentPath, "metadata evidence from an uncertain head update must keep the intent")
}

func TestDoctor_Repair_CleanIntentsRetainsMalformedIntentReferencingPublishedSnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createRemovedWorktreeSnapshot(t, repoPath)

	intentPath, err := repo.IntentPath(repoPath, snapshotID)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(intentPath, []byte(`{"snapshot_id":`), 0644))

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Equal(t, 0, results[0].Cleaned)
	assert.Contains(t, results[0].Message, "retained 1")
	assert.FileExists(t, intentPath, "malformed intent must fail closed while published snapshot evidence exists")
}

func TestDoctor_Repair_CleanIntentsRetainedIntentStillProtectsSnapshotFromGC(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createRemovedWorktreeSnapshot(t, repoPath)
	intentPath := writeSnapshotIntent(t, repoPath, snapshotID, "temp")

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Success)
	require.FileExists(t, intentPath)

	collector := gc.NewCollector(repoPath)
	plan, err := collector.PlanWithPolicy(model.RetentionPolicy{})
	require.NoError(t, err)
	assert.True(t, snapshotIDsContain(plan.ProtectedSet, snapshotID), "retained intent must stay in GC protected set")
	assert.False(t, snapshotIDsContain(plan.ToDelete, snapshotID), "retained intent must keep the snapshot out of GC candidates")
}

func TestDoctor_Repair_AdvanceHead(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	// Create second snapshot
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "second", nil)
	require.NoError(t, err)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"advance_head"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "advance_head", results[0].Action)
	assert.True(t, results[0].Success)
}

func TestDoctor_Repair_AdvanceHeadRejectsInvalidLatestSnapshotIDEvenWithOutsideReady(t *testing.T) {
	repoPath := setupTestRepo(t)
	invalid := model.SnapshotID("../../../outside")

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.LatestSnapshotID = invalid
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	outsideReady := filepath.Join(repoPath, ".jvs", "snapshots", string(invalid), ".READY")
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideReady), 0755))
	require.NoError(t, os.WriteFile(outsideReady, []byte("{}"), 0644))

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"advance_head"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "advance_head", results[0].Action)
	assert.Zero(t, results[0].Cleaned)
	assert.Contains(t, results[0].Message, "invalid")

	after, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	assert.NotEqual(t, invalid, after.HeadSnapshotID)
}

func TestDoctor_Repair_AdvanceHeadRejectsSymlinkedReadyMarker(t *testing.T) {
	repoPath := setupTestRepo(t)
	headID := createTestSnapshot(t, repoPath)
	latestID := createTestSnapshot(t, repoPath)
	setMainWorktreeSnapshots(t, repoPath, headID, latestID)

	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(latestID), ".READY")
	require.NoError(t, os.Remove(readyPath))
	outsideReady := filepath.Join(t.TempDir(), "READY")
	require.NoError(t, os.WriteFile(outsideReady, []byte("{}"), 0644))
	if err := os.Symlink(outsideReady, readyPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"advance_head"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assertAdvanceHeadSkippedUnsafe(t, results[0])
	assertMainHeadSnapshot(t, repoPath, headID)
}

func TestDoctor_Repair_AdvanceHeadRejectsUnsafeDescriptors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, descriptorPath string)
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, descriptorPath string) {
				t.Helper()
				require.NoError(t, os.Remove(descriptorPath))
			},
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, descriptorPath string) {
				t.Helper()
				require.NoError(t, os.Remove(descriptorPath))
				outsideDescriptor := filepath.Join(t.TempDir(), "descriptor.json")
				require.NoError(t, os.WriteFile(outsideDescriptor, []byte("{}"), 0644))
				if err := os.Symlink(outsideDescriptor, descriptorPath); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
		},
		{
			name: "directory",
			mutate: func(t *testing.T, descriptorPath string) {
				t.Helper()
				require.NoError(t, os.Remove(descriptorPath))
				require.NoError(t, os.Mkdir(descriptorPath, 0755))
			},
		},
		{
			name: "corrupt_json",
			mutate: func(t *testing.T, descriptorPath string) {
				t.Helper()
				require.NoError(t, os.WriteFile(descriptorPath, []byte("{invalid json"), 0644))
			},
		},
		{
			name: "checksum_mismatch",
			mutate: func(t *testing.T, descriptorPath string) {
				t.Helper()
				data, err := os.ReadFile(descriptorPath)
				require.NoError(t, err)
				var desc model.Descriptor
				require.NoError(t, json.Unmarshal(data, &desc))
				desc.Note = "tampered after checksum"
				data, err = json.Marshal(desc)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(descriptorPath, data, 0644))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			headID := createTestSnapshot(t, repoPath)
			latestID := createTestSnapshot(t, repoPath)
			setMainWorktreeSnapshots(t, repoPath, headID, latestID)

			descriptorPath, err := repo.SnapshotDescriptorPath(repoPath, latestID)
			require.NoError(t, err)
			tt.mutate(t, descriptorPath)

			doc := doctor.NewDoctor(repoPath)
			results, err := doc.Repair([]string{"advance_head"})
			require.NoError(t, err)
			require.Len(t, results, 1)
			assertAdvanceHeadSkippedUnsafe(t, results[0])
			assertMainHeadSnapshot(t, repoPath, headID)
		})
	}
}

func TestDoctor_Repair_AdvanceHeadPreservesReadyGzHistoricalHead(t *testing.T) {
	repoPath := setupTestRepo(t)
	headID := createTestSnapshot(t, repoPath)
	latestID := createTestSnapshot(t, repoPath)
	setMainWorktreeSnapshots(t, repoPath, headID, latestID)

	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(latestID))
	require.NoError(t, os.Rename(filepath.Join(snapshotDir, ".READY"), filepath.Join(snapshotDir, ".READY.gz")))

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"advance_head"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "advance_head", results[0].Action)
	assert.True(t, results[0].Success)
	assert.Zero(t, results[0].Cleaned)
	assertMainHeadSnapshot(t, repoPath, headID)
}

func TestDoctorRepairAdvanceHeadDoesNotAdvanceLegalHistoricalHead(t *testing.T) {
	repoPath := setupTestRepo(t)
	headID := createTestSnapshot(t, repoPath)
	latestID := createTestSnapshot(t, repoPath)
	setMainWorktreeSnapshots(t, repoPath, headID, latestID)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"advance_head"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "advance_head", results[0].Action)
	assert.True(t, results[0].Success)
	assert.Zero(t, results[0].Cleaned)
	assertMainHeadSnapshot(t, repoPath, headID)
}

func TestDoctor_Repair_UnknownAction(t *testing.T) {
	repoPath := setupTestRepo(t)
	doc := doctor.NewDoctor(repoPath)

	results, err := doc.Repair([]string{"unknown_action"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "unknown_action", results[0].Action)
	assert.False(t, results[0].Success)
	assert.Contains(t, results[0].Message, "unknown repair action")
}

func TestDoctor_Repair_MultipleActions(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan tmp and intent files
	os.WriteFile(filepath.Join(repoPath, ".jvs-tmp-orphan"), []byte("data"), 0644)
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	os.MkdirAll(intentsDir, 0755)
	os.WriteFile(filepath.Join(intentsDir, "orphan.json"), []byte("{}"), 0644)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_tmp", "clean_runtime_operations"})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)
}

func TestDoctor_Check_FormatVersionMismatch(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Set format version to a higher value
	versionPath := filepath.Join(repoPath, ".jvs", "format_version")
	os.WriteFile(versionPath, []byte("9999"), 0644)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.NotEmpty(t, result.Findings)
	assert.Equal(t, "format", result.Findings[0].Category)
	assert.Equal(t, "critical", result.Findings[0].Severity)
}

func TestDoctor_Check_MissingFormatVersion(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Remove format_version file
	os.Remove(filepath.Join(repoPath, ".jvs", "format_version"))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.NotEmpty(t, result.Findings)
	assert.Equal(t, "format", result.Findings[0].Category)
}

func TestDoctor_Check_OrphanSnapshotTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan snapshot tmp directory
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	os.MkdirAll(snapshotsDir, 0755)
	os.MkdirAll(filepath.Join(snapshotsDir, "something.tmp"), 0755)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	// Should find the tmp directory
	found := false
	for _, f := range result.Findings {
		if f.Category == "tmp" && f.Severity == "warning" {
			found = true
		}
	}
	assert.True(t, found, "expected tmp finding for orphan snapshot tmp directory")
}

func TestDoctor_Check_OrphanTmpRejectsSymlinkedSnapshotsDirWithoutFollowing(t *testing.T) {
	repoPath := setupTestRepo(t)
	outside := t.TempDir()
	outsideTmp := filepath.Join(outside, "outside.tmp")
	require.NoError(t, os.MkdirAll(outsideTmp, 0755))

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.RemoveAll(snapshotsDir))
	if err := os.Symlink(outside, snapshotsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertTmpFinding(t, result, "error", "snapshots", "symlink")
	assertNoTmpFinding(t, result, "warning", "outside.tmp")
}

func TestDoctor_Check_OrphanTmpRejectsSymlinkedSnapshotTmpEntryWithoutFollowing(t *testing.T) {
	repoPath := setupTestRepo(t)
	outsideTmp := filepath.Join(t.TempDir(), "outside.tmp")
	require.NoError(t, os.MkdirAll(outsideTmp, 0755))

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	tmpEntry := filepath.Join(snapshotsDir, "unsafe.tmp")
	if err := os.Symlink(outsideTmp, tmpEntry); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertTmpFinding(t, result, "error", "unsafe.tmp", "symlink")
	assertNoTmpFinding(t, result, "warning", "unsafe.tmp")
}

func TestDoctor_Repair_CleanTmp_SnapshotTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create orphan snapshot tmp directory
	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	os.MkdirAll(snapshotsDir, 0755)
	os.MkdirAll(filepath.Join(snapshotsDir, "something.tmp"), 0755)

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_tmp"})
	require.NoError(t, err)
	assert.True(t, results[0].Success)
	assert.GreaterOrEqual(t, results[0].Cleaned, 1)

	// Verify tmp directory is gone
	assert.NoDirExists(t, filepath.Join(snapshotsDir, "something.tmp"))
}

func TestDoctor_Repair_CleanTmpRejectsSymlinkedSnapshotsDirWithoutDeletingOutside(t *testing.T) {
	repoPath := setupTestRepo(t)
	outside := t.TempDir()
	outsideTmp := filepath.Join(outside, "outside.tmp")
	require.NoError(t, os.MkdirAll(outsideTmp, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideTmp, "keep.txt"), []byte("keep"), 0644))

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.RemoveAll(snapshotsDir))
	if err := os.Symlink(outside, snapshotsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_tmp"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.DirExists(t, outsideTmp)
	assert.FileExists(t, filepath.Join(outsideTmp, "keep.txt"))
}

func TestDoctor_Repair_CleanIntentsRejectsSymlinkedIntentsDirWithoutDeletingOutside(t *testing.T) {
	repoPath := setupTestRepo(t)
	outside := t.TempDir()
	outsideIntent := filepath.Join(outside, "orphan.json")
	require.NoError(t, os.WriteFile(outsideIntent, []byte("{}"), 0644))

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	require.NoError(t, os.RemoveAll(intentsDir))
	if err := os.Symlink(outside, intentsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_operations"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.FileExists(t, outsideIntent)
}

func TestDoctor_Check_AuditChain_WithBrokenChain(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create an audit log with broken hash chain
	auditDir := filepath.Join(repoPath, ".jvs", "audit")
	require.NoError(t, os.MkdirAll(auditDir, 0755))
	auditPath := filepath.Join(auditDir, "audit.jsonl")

	// Write audit records with mismatched hashes
	record1 := `{"prev_hash":"","record_hash":"hash1","timestamp":"2024-01-01T00:00:00Z","event_type":"test"}`
	record2 := `{"prev_hash":"wrong_hash","record_hash":"hash2","timestamp":"2024-01-01T01:00:00Z","event_type":"test"}`
	auditContent := record1 + "\n" + record2 + "\n"
	require.NoError(t, os.WriteFile(auditPath, []byte(auditContent), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)

	// Should detect broken audit chain
	assert.False(t, result.Healthy)
	found := false
	for _, f := range result.Findings {
		if f.Category == "audit" && f.ErrorCode == "E_AUDIT_CHAIN_BROKEN" {
			found = true
			assert.Equal(t, "critical", f.Severity)
		}
	}
	assert.True(t, found, "expected audit chain broken finding")
}

func TestDoctor_Check_AuditChain_WithMalformedRecord(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create an audit log with malformed record
	auditDir := filepath.Join(repoPath, ".jvs", "audit")
	require.NoError(t, os.MkdirAll(auditDir, 0755))
	auditPath := filepath.Join(auditDir, "audit.jsonl")

	record1 := `{"prev_hash":"","record_hash":"hash1","timestamp":"2024-01-01T00:00:00Z","event_type":"test"}`
	record2 := `{invalid json}`
	auditContent := record1 + "\n" + record2 + "\n"
	require.NoError(t, os.WriteFile(auditPath, []byte(auditContent), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)

	// Should fail closed on malformed audit records.
	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "audit", "E_AUDIT_RECORD_MALFORMED")
	for _, f := range result.Findings {
		if f.Category == "audit" && f.ErrorCode == "E_AUDIT_RECORD_MALFORMED" {
			assert.Equal(t, "critical", f.Severity)
			assert.Contains(t, f.Description, "malformed record")
			return
		}
	}
	t.Fatalf("expected malformed audit finding in %#v", result.Findings)
}

func TestDoctorStrictAuditVerificationCannotRunIsUnhealthy(t *testing.T) {
	repoPath := setupTestRepo(t)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	require.NoError(t, os.RemoveAll(auditPath))
	require.NoError(t, os.Mkdir(auditPath, 0755))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "audit", "E_AUDIT_SCAN_FAILED")
	for _, f := range result.Findings {
		if f.Category == "audit" && f.ErrorCode == "E_AUDIT_SCAN_FAILED" {
			assert.Equal(t, "error", f.Severity)
			return
		}
	}
	t.Fatalf("expected audit scan failure finding in %#v", result.Findings)
}

func TestDoctorStrictUnhealthyFindingsHaveStableErrorCodes(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)
	require.NoError(t, os.Remove(filepath.Join(repoPath, ".jvs", "format_version")))
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, "main")))
	require.NoError(t, os.Remove(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)
	require.False(t, result.Healthy)

	for _, f := range result.Findings {
		if f.Severity != "error" && f.Severity != "critical" {
			continue
		}
		require.NotEmpty(t, f.ErrorCode, "finding missing error_code: %#v", f)
	}
}

func TestDoctorStrictCatchesUnsafeDescriptorEntries(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		populate func(t *testing.T, descriptorsDir string)
	}{
		{
			name: "invalid_filename",
			code: "E_DESCRIPTOR_FILENAME_INVALID",
			populate: func(t *testing.T, descriptorsDir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(descriptorsDir, "not-a-snapshot.json"), []byte("{}"), 0644))
			},
		},
		{
			name: "symlink",
			code: "E_DESCRIPTOR_CONTROL_INVALID",
			populate: func(t *testing.T, descriptorsDir string) {
				t.Helper()
				outsideDescriptor := filepath.Join(t.TempDir(), "descriptor.json")
				require.NoError(t, os.WriteFile(outsideDescriptor, []byte("{}"), 0644))
				linkPath := filepath.Join(descriptorsDir, string(model.NewSnapshotID())+".json")
				if err := os.Symlink(outsideDescriptor, linkPath); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
		},
		{
			name: "directory",
			code: "E_DESCRIPTOR_CONTROL_INVALID",
			populate: func(t *testing.T, descriptorsDir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(descriptorsDir, string(model.NewSnapshotID())+".json"), 0755))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := setupTestRepo(t)
			descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
			tt.populate(t, descriptorsDir)

			result, err := doctor.NewDoctor(repoPath).Check(true)
			require.NoError(t, err)
			assert.False(t, result.Healthy)
			assertFindingCode(t, result, "descriptor", tt.code)
		})
	}
}

func TestDoctorStrictRejectsSymlinkedIntentsDir(t *testing.T) {
	repoPath := setupTestRepo(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "operation.json"), []byte("{}"), 0644))

	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	require.NoError(t, os.RemoveAll(intentsDir))
	if err := os.Symlink(outside, intentsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "intent", "E_INTENT_CONTROL_INVALID")
}

func TestDoctor_Check_AuditChain_NoAuditLog(t *testing.T) {
	repoPath := setupTestRepo(t)

	// No audit log exists - should be OK
	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)
	assert.True(t, result.Healthy)
}

func TestDoctor_Check_WithOrphanTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs-tmp-snapshot-abc123"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".jvs-tmp-snapshot-def456"), 0755))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	tmpFindings := 0
	for _, f := range result.Findings {
		if f.Category == "tmp" {
			tmpFindings++
		}
	}
	assert.GreaterOrEqual(t, tmpFindings, 2, "should find at least 2 orphan tmp directories")
}

func TestDoctor_Repair_WithOrphanTmp(t *testing.T) {
	repoPath := setupTestRepo(t)

	dir1 := filepath.Join(repoPath, ".jvs-tmp-snapshot-abc123")
	dir2 := filepath.Join(repoPath, ".jvs-tmp-snapshot-def456")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "partial.dat"), []byte("data"), 0644))

	doc := doctor.NewDoctor(repoPath)
	results, err := doc.Repair([]string{"clean_runtime_tmp"})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.GreaterOrEqual(t, results[0].Cleaned, 2)

	assert.NoDirExists(t, dir1)
	assert.NoDirExists(t, dir2)
}

func TestDoctor_Check_CorruptedFormatVersion(t *testing.T) {
	repoPath := setupTestRepo(t)

	versionPath := filepath.Join(repoPath, ".jvs", "format_version")
	require.NoError(t, os.WriteFile(versionPath, []byte("not-a-number"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	require.NotEmpty(t, result.Findings)

	found := false
	for _, f := range result.Findings {
		if f.Category == "format" && f.Severity == "critical" {
			found = true
			assert.Contains(t, f.Description, "invalid content")
		}
	}
	assert.True(t, found, "expected critical format finding for corrupted format_version")
}

func TestDoctor_Check_SnapshotIntegrity_VerifyError(t *testing.T) {
	repoPath := setupTestRepo(t)

	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "snapshots")))
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots"), []byte("not a directory"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	found := false
	for _, f := range result.Findings {
		if f.Category == "integrity" && f.Severity == "error" {
			found = true
			assert.Contains(t, f.Description, "verification failed")
		}
	}
	assert.True(t, found, "expected strict verification execution error finding")
}

func TestDoctor_Check_SnapshotIntegrity_CorruptedDescriptorUnhealthy(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{invalid json"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	found := false
	for _, f := range result.Findings {
		if f.Category == "integrity" && f.Severity == "critical" {
			found = true
			assert.Contains(t, f.Description, string(snapshotID))
		}
	}
	assert.True(t, found, "expected corrupted descriptor to be a critical integrity finding")
}

func TestDoctorStrictReportsDescriptorWithoutPayload(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)
	require.NoError(t, os.RemoveAll(filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "integrity", "E_PAYLOAD_MISSING")
}

func TestStrictDoctorFailsDanglingLatestCheckpoint(t *testing.T) {
	repoPath := setupTestRepo(t)
	danglingID := model.NewSnapshotID()

	cfg, err := repo.LoadWorktreeConfig(repoPath, "main")
	require.NoError(t, err)
	cfg.LatestSnapshotID = danglingID
	require.NoError(t, repo.WriteWorktreeConfig(repoPath, "main", cfg))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "integrity", "E_DESCRIPTOR_MISSING")
}

func TestStrictDoctorPayloadMismatchHasStableErrorCode(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), "tampered.txt"),
		[]byte("tampered"),
		0644,
	))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "integrity", "E_PAYLOAD_HASH_MISMATCH")
}

func TestStrictDoctorAuditDetectsRecordHashTamper(t *testing.T) {
	repoPath := setupTestRepo(t)
	createTestSnapshot(t, repoPath)

	auditPath := filepath.Join(repoPath, ".jvs", "audit", "audit.jsonl")
	lines := readAuditJSONLines(t, auditPath)
	require.NotEmpty(t, lines)

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	details, ok := record["details"].(map[string]any)
	require.True(t, ok)
	details["note"] = "tampered after hash"
	tampered, err := json.Marshal(record)
	require.NoError(t, err)
	lines[0] = string(tampered)
	require.NoError(t, os.WriteFile(auditPath, []byte(strings.Join(lines, "\n")+"\n"), 0644))

	result, err := doctor.NewDoctor(repoPath).Check(true)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertFindingCode(t, result, "audit", "E_AUDIT_RECORD_HASH_MISMATCH")
}

func TestDoctor_Check_MissingReadyMarkerUnhealthy(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := createTestSnapshot(t, repoPath)

	readyPath := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID), ".READY")
	require.NoError(t, os.Remove(readyPath))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	found := false
	for _, f := range result.Findings {
		if f.Category == "snapshot" && f.Severity == "error" {
			found = true
			assert.Contains(t, f.Description, "READY marker missing")
			assert.Equal(t, "E_READY_MISSING", f.ErrorCode)
		}
	}
	assert.True(t, found, "expected missing READY marker finding")
}

func TestDoctor_Check_ReadyWithoutDescriptorFailsClosed(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), []byte("{}"), 0644))
	expectedDescriptorPath, err := repo.SnapshotDescriptorPath(repoPath, snapshotID)
	require.NoError(t, err)

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assertReadyWithoutDescriptorFinding(t, result, snapshotID, "error", expectedDescriptorPath)

	result, err = doc.Check(true)
	require.NoError(t, err)
	assert.False(t, result.Healthy)
	assertReadyWithoutDescriptorFinding(t, result, snapshotID, "critical", expectedDescriptorPath)
}

func TestDoctor_Check_ReadyRejectsSymlinkedDescriptorsDirWithoutTrustingExternalDescriptor(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), []byte("{}"), 0644))

	outsideDescriptors := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDescriptors, string(snapshotID)+".json"), []byte("{}"), 0644))

	descriptorsDir := filepath.Join(repoPath, ".jvs", "descriptors")
	require.NoError(t, os.RemoveAll(descriptorsDir))
	if err := os.Symlink(outsideDescriptors, descriptorsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertReadyDescriptorInvalidFinding(t, result, "descriptor", "symlink")
}

func TestDoctor_Check_ReadyRejectsSymlinkedSnapshotsDirWithoutTrustingExternalSnapshot(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()

	outsideSnapshots := t.TempDir()
	outsideSnapshotDir := filepath.Join(outsideSnapshots, string(snapshotID))
	require.NoError(t, os.MkdirAll(outsideSnapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideSnapshotDir, ".READY"), []byte("{}"), 0644))

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{}"), 0644))

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	require.NoError(t, os.RemoveAll(snapshotsDir))
	if err := os.Symlink(outsideSnapshots, snapshotsDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertSnapshotFinding(t, result, "error", "snapshots", "symlink")
}

func TestDoctor_Check_ReadyRejectsSymlinkedSnapshotEntryWithoutFollowing(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()

	outsideSnapshot := filepath.Join(t.TempDir(), "snapshot")
	require.NoError(t, os.MkdirAll(outsideSnapshot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(outsideSnapshot, ".READY"), []byte("{}"), 0644))

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{}"), 0644))

	snapshotEntry := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	if err := os.Symlink(outsideSnapshot, snapshotEntry); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertSnapshotFinding(t, result, "error", string(snapshotID), "symlink")
}

func TestDoctor_Check_ReadyRejectsSymlinkedReadyMarkerWithoutFollowing(t *testing.T) {
	repoPath := setupTestRepo(t)
	snapshotID := model.NewSnapshotID()
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", string(snapshotID))
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))

	outsideReady := filepath.Join(t.TempDir(), "READY")
	require.NoError(t, os.WriteFile(outsideReady, []byte("{}"), 0644))
	if err := os.Symlink(outsideReady, filepath.Join(snapshotDir, ".READY")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", string(snapshotID)+".json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{}"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertSnapshotFinding(t, result, "error", string(snapshotID), "READY marker", "symlink")
}

func TestDoctor_Check_ReadyRejectsInvalidSnapshotIDBeforeDescriptorLookup(t *testing.T) {
	repoPath := setupTestRepo(t)
	invalidName := "not-a-snapshot"
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", invalidName)
	require.NoError(t, os.MkdirAll(snapshotDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, ".READY"), []byte("{}"), 0644))

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", invalidName+".json")
	require.NoError(t, os.WriteFile(descriptorPath, []byte("{}"), 0644))

	doc := doctor.NewDoctor(repoPath)
	result, err := doc.Check(false)
	require.NoError(t, err)

	assert.False(t, result.Healthy)
	assertSnapshotFinding(t, result, "error", invalidName, "invalid snapshot ID")
}

func assertReadyWithoutDescriptorFinding(t *testing.T, result *doctor.Result, snapshotID model.SnapshotID, severity, expectedPath string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category == "snapshot" && f.ErrorCode == "E_READY_DESCRIPTOR_MISSING" && f.Severity == severity {
			assert.Contains(t, f.Description, string(snapshotID))
			assert.Equal(t, expectedPath, f.Path)
			return
		}
	}
	t.Fatalf("expected READY-without-descriptor finding with severity %s in %#v", severity, result.Findings)
}

func assertReadyDescriptorInvalidFinding(t *testing.T, result *doctor.Result, substrings ...string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category != "snapshot" || f.ErrorCode != "E_READY_DESCRIPTOR_INVALID" || f.Severity != "error" {
			continue
		}
		description := f.Description + " " + f.Path
		matches := true
		for _, substring := range substrings {
			if !strings.Contains(description, substring) {
				matches = false
				break
			}
		}
		if matches {
			assert.Empty(t, f.Path)
			return
		}
	}
	t.Fatalf("expected READY descriptor invalid finding containing %q in %#v", substrings, result.Findings)
}

func assertSnapshotFinding(t *testing.T, result *doctor.Result, severity string, substrings ...string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category != "snapshot" || f.Severity != severity {
			continue
		}
		description := f.Description + " " + f.Path
		matches := true
		for _, substring := range substrings {
			if !strings.Contains(description, substring) {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("expected snapshot finding severity %s containing %q in %#v", severity, substrings, result.Findings)
}

func assertTmpFinding(t *testing.T, result *doctor.Result, severity string, substrings ...string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category != "tmp" || f.Severity != severity {
			continue
		}
		description := f.Description + " " + f.Path
		matches := true
		for _, substring := range substrings {
			if !strings.Contains(description, substring) {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("expected tmp finding severity %s containing %q in %#v", severity, substrings, result.Findings)
}

func assertNoTmpFinding(t *testing.T, result *doctor.Result, severity string, substrings ...string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category != "tmp" || f.Severity != severity {
			continue
		}
		description := f.Description + " " + f.Path
		matches := true
		for _, substring := range substrings {
			if !strings.Contains(description, substring) {
				matches = false
				break
			}
		}
		if matches {
			t.Fatalf("unexpected tmp finding severity %s containing %q in %#v", severity, substrings, result.Findings)
		}
	}
}

func readAuditJSONLines(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func assertFindingCode(t *testing.T, result *doctor.Result, category, code string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.Category == category && f.ErrorCode == code {
			return
		}
	}
	t.Fatalf("expected finding %s/%s in %#v", category, code, result.Findings)
}

func assertNoFindingCode(t *testing.T, result *doctor.Result, code string) {
	t.Helper()

	for _, f := range result.Findings {
		if f.ErrorCode == code {
			t.Fatalf("unexpected finding with code %s in %#v", code, result.Findings)
		}
	}
}

func assertRepairActionFailed(t *testing.T, results []doctor.RepairResult, action string) {
	t.Helper()

	for _, result := range results {
		if result.Action == action {
			assert.False(t, result.Success, "repair %s should fail/skip: %#v", action, result)
			return
		}
	}
	t.Fatalf("expected repair result for %s in %#v", action, results)
}

func assertRepairActionSucceeded(t *testing.T, results []doctor.RepairResult, action string) {
	t.Helper()

	for _, result := range results {
		if result.Action == action {
			assert.True(t, result.Success, "repair %s should succeed: %#v", action, result)
			return
		}
	}
	t.Fatalf("expected repair result for %s in %#v", action, results)
}

func requireRepairAction(t *testing.T, results []doctor.RepairResult, action string) doctor.RepairResult {
	t.Helper()

	for _, result := range results {
		if result.Action == action {
			return result
		}
	}
	t.Fatalf("expected repair result for %s in %#v", action, results)
	return doctor.RepairResult{}
}
