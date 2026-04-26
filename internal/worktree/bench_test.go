package worktree_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// wrapCloneFunc wraps engine.Clone to match the Fork signature.
func wrapCloneFunc(eng engine.Engine) func(src, dst string) error {
	return func(src, dst string) error {
		_, err := eng.Clone(src, dst)
		return err
	}
}

// setupForkBenchRepo creates a repository with a snapshot for fork benchmarking.
func setupForkBenchRepo(b *testing.B, contentSize int) string {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(dir, "main")

	// Create content of specified size
	if contentSize > 0 {
		data := make([]byte, contentSize)
		for i := range data {
			data[i] = byte(i % 256)
		}
		if err := os.WriteFile(filepath.Join(mainPath, "data.bin"), data, 0644); err != nil {
			b.Fatal(err)
		}
	}

	// Create a snapshot to fork from
	creator := snapshot.NewCreator(dir, model.EngineCopy)
	if _, err := creator.Create("main", "base snapshot", nil); err != nil {
		b.Fatal(err)
	}

	return dir
}

// setupForkBenchRepoWithFiles creates a repository with snapshot having multiple files.
func setupForkBenchRepoWithFiles(b *testing.B, fileCount int) string {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(dir, "main")

	// Create multiple files
	for i := 0; i < fileCount; i++ {
		subDir := filepath.Join(mainPath, fmt.Sprintf("dir%d", i%3))
		if err := os.MkdirAll(subDir, 0755); err != nil {
			b.Fatal(err)
		}
		data := make([]byte, 1024) // 1KB per file
		for j := range data {
			data[j] = byte((i + j) % 256)
		}
		filePath := filepath.Join(subDir, fmt.Sprintf("file%d.dat", i))
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatal(err)
		}
	}

	// Create snapshot to fork from
	creator := snapshot.NewCreator(dir, model.EngineCopy)
	if _, err := creator.Create("main", "base snapshot", nil); err != nil {
		b.Fatal(err)
	}

	return dir
}

// BenchmarkWorktreeFork_Small benchmarks forking a worktree with small payload.
func BenchmarkWorktreeFork_Small(b *testing.B) {
	repoPath := setupForkBenchRepo(b, 1024) // 1KB base
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "base", nil)
	if err != nil {
		b.Fatal(err)
	}

	eng := engine.NewEngine(model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wtName := fmt.Sprintf("fork%d", i%10)
		_, err := wtMgr.Fork(desc.SnapshotID, wtName, wrapCloneFunc(eng))
		if err != nil {
			b.Fatal(err)
		}
		// Cleanup for next iteration
		wtMgr.Remove(wtName)
	}
}

// BenchmarkWorktreeFork_Medium benchmarks forking a worktree with medium payload.
func BenchmarkWorktreeFork_Medium(b *testing.B) {
	repoPath := setupForkBenchRepo(b, 1024*1024) // 1MB base
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "base", nil)
	if err != nil {
		b.Fatal(err)
	}

	eng := engine.NewEngine(model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wtName := fmt.Sprintf("fork%d", i%10)
		_, err := wtMgr.Fork(desc.SnapshotID, wtName, wrapCloneFunc(eng))
		if err != nil {
			b.Fatal(err)
		}
		wtMgr.Remove(wtName)
	}
}

// BenchmarkWorktreeFork_Reflink benchmarks forking with reflink engine.
func BenchmarkWorktreeFork_Reflink(b *testing.B) {
	repoPath := setupForkBenchRepo(b, 1024*100) // 100KB base
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)
	desc, err := creator.Create("main", "base", nil)
	if err != nil {
		b.Fatal(err)
	}

	eng := engine.NewEngine(model.EngineReflinkCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wtName := fmt.Sprintf("fork%d", i%10)
		_, err := wtMgr.Fork(desc.SnapshotID, wtName, wrapCloneFunc(eng))
		if err != nil {
			b.Fatal(err)
		}
		wtMgr.Remove(wtName)
	}
}

// BenchmarkWorktreeFork_MultiFile benchmarks forking with many files.
func BenchmarkWorktreeFork_MultiFile(b *testing.B) {
	repoPath := setupForkBenchRepoWithFiles(b, 100) // 100 files
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "base", nil)
	if err != nil {
		b.Fatal(err)
	}

	eng := engine.NewEngine(model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wtName := fmt.Sprintf("fork%d", i%10)
		_, err := wtMgr.Fork(desc.SnapshotID, wtName, wrapCloneFunc(eng))
		if err != nil {
			b.Fatal(err)
		}
		wtMgr.Remove(wtName)
	}
}

// BenchmarkWorktreeFork_MultiFile_Large benchmarks forking with many files.
func BenchmarkWorktreeFork_MultiFile_Large(b *testing.B) {
	repoPath := setupForkBenchRepoWithFiles(b, 1000) // 1000 files
	wtMgr := worktree.NewManager(repoPath)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "base", nil)
	if err != nil {
		b.Fatal(err)
	}

	eng := engine.NewEngine(model.EngineCopy)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wtName := fmt.Sprintf("fork%d", i%10)
		_, err := wtMgr.Fork(desc.SnapshotID, wtName, wrapCloneFunc(eng))
		if err != nil {
			b.Fatal(err)
		}
		wtMgr.Remove(wtName)
	}
}

// BenchmarkWorktreeList benchmarks listing worktrees.
func BenchmarkWorktreeList(b *testing.B) {
	repoPath := b.TempDir()
	_, err := repo.Init(repoPath, "bench")
	if err != nil {
		b.Fatal(err)
	}

	wtMgr := worktree.NewManager(repoPath)

	// Create 10 worktrees
	for i := 0; i < 10; i++ {
		wtName := fmt.Sprintf("wt%d", i)
		if _, err := wtMgr.Create(wtName, nil); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wtMgr.List()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWorktreeGet benchmarks getting a specific worktree config.
func BenchmarkWorktreeGet(b *testing.B) {
	repoPath := b.TempDir()
	_, err := repo.Init(repoPath, "bench")
	if err != nil {
		b.Fatal(err)
	}

	wtMgr := worktree.NewManager(repoPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wtMgr.Get("main")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWorktreeSetLatest benchmarks updating latest snapshot.
func BenchmarkWorktreeSetLatest(b *testing.B) {
	repoPath := b.TempDir()
	_, err := repo.Init(repoPath, "bench")
	if err != nil {
		b.Fatal(err)
	}

	wtMgr := worktree.NewManager(repoPath)
	snapshotID := model.SnapshotID("test-snapshot-id")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := wtMgr.SetLatest("main", snapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
