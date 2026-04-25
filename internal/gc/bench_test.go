// Package gc_test contains performance benchmarks for garbage collection operations.
//
// # Baseline Performance Expectations (Intel Core i7, Linux x64)
//
// | Benchmark               | Ops/sec  | Memory/op    | Allocs/op |
// |-------------------------|----------|--------------|-----------|
// | Plan_Small (10 snaps)   | ~8,000   | ~30 KB       | ~400      |
// | Plan_Medium (100 snaps) | ~1,500   | ~220 KB      | ~2,500    |
// | Plan_Large (1000 snaps) | ~150     | ~2.2 MB      | ~23,000   |
// | DeleteSingle            | ~100     | ~4 MB        | ~80,000   |
// | EmptyRepo               | ~40,000  | ~5 KB        | ~65       |
//
// These baselines help detect performance regressions. If performance degrades
// by more than 20% from baseline, investigate changes.
package gc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/gc"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/uuidutil"
)

// setupBenchmarkRepo creates a repository with specified number of snapshots.
func setupBenchmarkRepo(b *testing.B, snapshotCount int) string {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatalf("init repo: %v", err)
	}

	mainPath := filepath.Join(dir, "main")
	creator := snapshot.NewCreator(dir, model.EngineCopy)

	// Create chain of snapshots
	for i := 0; i < snapshotCount; i++ {
		// Add unique content to force new snapshots
		filename := filepath.Join(mainPath, "file.txt")
		if err := os.WriteFile(filename, []byte(string(rune(i))), 0644); err != nil {
			b.Fatalf("write file: %v", err)
		}
		if _, err := creator.Create("main", "bench", nil); err != nil {
			b.Fatalf("create snapshot: %v", err)
		}
	}

	return dir
}

// setupBenchmarkRepoWithDeletable creates a repository with protected and deletable snapshots.
func setupBenchmarkRepoWithDeletable(b *testing.B, totalSnapshots, deletableCount int) string {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatalf("init repo: %v", err)
	}

	creator := snapshot.NewCreator(dir, model.EngineCopy)
	wtMgr := worktree.NewManager(dir)

	// Create snapshots in main worktree (protected)
	mainPath := filepath.Join(dir, "main")
	for i := 0; i < totalSnapshots-deletableCount; i++ {
		filename := filepath.Join(mainPath, "file.txt")
		if err := os.WriteFile(filename, []byte(string(rune(i))), 0644); err != nil {
			b.Fatalf("write file: %v", err)
		}
		if _, err := creator.Create("main", "bench", nil); err != nil {
			b.Fatalf("create snapshot: %v", err)
		}
	}

	// Create temporary worktrees with snapshots that can be deleted
	for i := 0; i < deletableCount; i++ {
		wtName := "temp-" + uuidutil.NewV4()[:8]
		cfg, err := wtMgr.Create(wtName, nil)
		if err != nil {
			b.Fatalf("create worktree: %v", err)
		}

		wtPath, err := wtMgr.Path(wtName)
		if err != nil {
			b.Fatalf("worktree path: %v", err)
		}
		filename := filepath.Join(wtPath, "file.txt")
		if err := os.WriteFile(filename, []byte(string(rune(i))), 0644); err != nil {
			b.Fatalf("write file: %v", err)
		}
		if _, err := creator.Create(wtName, "temp", nil); err != nil {
			b.Fatalf("create snapshot: %v", err)
		}
		_ = cfg

		// Delete worktree to make snapshot eligible for GC
		if err := wtMgr.Remove(wtName); err != nil {
			b.Fatalf("remove worktree: %v", err)
		}
	}

	return dir
}

// BenchmarkGCPlan_Small benchmarks plan generation with 10 snapshots.
func BenchmarkGCPlan_Small(b *testing.B) {
	repoPath := setupBenchmarkRepo(b, 10)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCPlan_Medium benchmarks plan generation with 100 snapshots.
func BenchmarkGCPlan_Medium(b *testing.B) {
	repoPath := setupBenchmarkRepo(b, 100)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCPlan_Large benchmarks plan generation with 1000 snapshots.
func BenchmarkGCPlan_Large(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping large benchmark in short mode")
	}
	repoPath := setupBenchmarkRepo(b, 1000)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCPlan_WithDeletable benchmarks plan generation when there are candidates.
func BenchmarkGCPlan_WithDeletable(b *testing.B) {
	// 100 total snapshots, 50 deletable
	repoPath := setupBenchmarkRepoWithDeletable(b, 100, 50)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan, err := collector.Plan()
		if err != nil {
			b.Fatalf("Plan: %v", err)
		}
		// Verify we have candidates
		if plan.CandidateCount == 0 {
			b.Fatalf("expected candidates, got 0")
		}
	}
}

// BenchmarkGCRun_DeleteSingle benchmarks deleting a single snapshot.
func BenchmarkGCRun_DeleteSingle(b *testing.B) {
	repoPath := setupBenchmarkRepoWithDeletable(b, 10, 1)
	collector := gc.NewCollector(repoPath)

	// Verify we have a plan that works
	_, err := collector.Plan()
	if err != nil {
		b.Fatalf("Plan: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Re-create plan for each iteration since Run deletes it
		plan, err := collector.Plan()
		if err != nil {
			b.Fatalf("Plan: %v", err)
		}
		if err := collector.Run(plan.PlanID); err != nil {
			b.Fatalf("Run: %v", err)
		}
		// Re-create one deletable snapshot for next iteration
		recreateDeletableSnapshot(b, repoPath)
	}
}

// BenchmarkGCRun_DeleteMultiple benchmarks deleting multiple snapshots.
func BenchmarkGCRun_DeleteMultiple(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping multiple delete benchmark in short mode")
	}
	// 110 total snapshots, 10 deletable
	repoPath := setupBenchmarkRepoWithDeletable(b, 110, 10)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan, err := collector.Plan()
		if err != nil {
			b.Fatalf("Plan: %v", err)
		}
		if err := collector.Run(plan.PlanID); err != nil {
			b.Fatalf("Run: %v", err)
		}
		// Re-create deletable snapshots for next iteration
		recreateDeletableSnapshots(b, repoPath, 10)
	}
}

// BenchmarkGCLineageTraversal benchmarks lineage chain traversal.
func BenchmarkGCLineageTraversal(b *testing.B) {
	repoPath := setupBenchmarkRepo(b, 100)
	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each Plan call does lineage traversal
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCWithPins benchmarks plan generation with many pins.
func BenchmarkGCWithPins(b *testing.B) {
	repoPath := setupBenchmarkRepo(b, 50)

	// Create 20 pins
	pinsDir := filepath.Join(repoPath, ".jvs", "pins")
	if err := os.MkdirAll(pinsDir, 0755); err != nil {
		b.Fatalf("mkdir pins: %v", err)
	}

	snapshotsDir := filepath.Join(repoPath, ".jvs", "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		b.Fatalf("read snapshots dir: %v", err)
	}

	// Pin first 20 snapshots
	for i := 0; i < 20 && i < len(entries); i++ {
		snapshotID := entries[i].Name()
		pin := model.Pin{
			SnapshotID: model.SnapshotID(snapshotID),
			PinnedAt:   parseTime("2024-01-01T00:00:00Z"),
			Reason:     "benchmark pin",
		}
		data, err := json.MarshalIndent(pin, "", "  ")
		if err != nil {
			b.Fatalf("marshal pin: %v", err)
		}
		pinPath := filepath.Join(pinsDir, snapshotID+".json")
		if err := os.WriteFile(pinPath, data, 0644); err != nil {
			b.Fatalf("write pin: %v", err)
		}
	}

	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCEmptyRepo benchmarks plan generation on empty repository.
func BenchmarkGCEmptyRepo(b *testing.B) {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatalf("init repo: %v", err)
	}

	collector := gc.NewCollector(dir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// BenchmarkGCWithIntents benchmarks plan generation with intents.
func BenchmarkGCWithIntents(b *testing.B) {
	repoPath := setupBenchmarkRepo(b, 50)

	// Create 10 intents
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	if err := os.MkdirAll(intentsDir, 0755); err != nil {
		b.Fatalf("mkdir intents: %v", err)
	}

	for i := 0; i < 10; i++ {
		intentID := uuidutil.NewV4()
		intentPath := filepath.Join(intentsDir, intentID+".json")
		intent := map[string]any{
			"intent_id":  intentID,
			"created_at": "2024-01-01T00:00:00Z",
			"type":       "snapshot",
		}
		data, err := json.MarshalIndent(intent, "", "  ")
		if err != nil {
			b.Fatalf("marshal intent: %v", err)
		}
		if err := os.WriteFile(intentPath, data, 0644); err != nil {
			b.Fatalf("write intent: %v", err)
		}
	}

	collector := gc.NewCollector(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := collector.Plan(); err != nil {
			b.Fatalf("Plan: %v", err)
		}
	}
}

// Helper functions

func recreateDeletableSnapshot(b *testing.B, repoPath string) {
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	wtName := "temp-" + uuidutil.NewV4()[:8]
	cfg, err := wtMgr.Create(wtName, nil)
	if err != nil {
		b.Fatalf("create worktree: %v", err)
	}

	wtPath, err := wtMgr.Path(wtName)
	if err != nil {
		b.Fatalf("worktree path: %v", err)
	}
	filename := filepath.Join(wtPath, "file.txt")
	if err := os.WriteFile(filename, []byte("temp"), 0644); err != nil {
		b.Fatalf("write file: %v", err)
	}
	if _, err := creator.Create(wtName, "temp", nil); err != nil {
		b.Fatalf("create snapshot: %v", err)
	}
	_ = cfg

	if err := wtMgr.Remove(wtName); err != nil {
		b.Fatalf("remove worktree: %v", err)
	}
}

func recreateDeletableSnapshots(b *testing.B, repoPath string, count int) {
	for i := 0; i < count; i++ {
		recreateDeletableSnapshot(b, repoPath)
	}
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
