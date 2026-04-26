package restore_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// setupBenchRepo creates a repository with test content for benchmarking.
func setupRestoreBenchRepo(b *testing.B, contentSize int) string {
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

	return dir
}

// setupRestoreBenchRepoWithFiles creates a repository with multiple files for benchmarking.
func setupRestoreBenchRepoWithFiles(b *testing.B, fileCount int) string {
	dir := b.TempDir()
	_, err := repo.Init(dir, "bench")
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(dir, "main")

	// Create multiple files and directories
	for i := 0; i < fileCount; i++ {
		subDir := filepath.Join(mainPath, "dir", strconv.Itoa(i%10))
		if err := os.MkdirAll(subDir, 0755); err != nil {
			b.Fatal(err)
		}

		data := []byte("test content for restore benchmarking")
		filePath := filepath.Join(subDir, "file"+strconv.Itoa(i)+".txt")
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatal(err)
		}
	}

	return dir
}

// BenchmarkRestore_CopyEngine_Small benchmarks restore with small payload using copy engine.
func BenchmarkRestore_CopyEngine_Small(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Modify content to ensure restore changes it
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify before restore
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_CopyEngine_Medium benchmarks restore with medium payload using copy engine.
func BenchmarkRestore_CopyEngine_Medium(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024*1024) // 1MB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_CopyEngine_Large benchmarks restore with large payload using copy engine.
func BenchmarkRestore_CopyEngine_Large(b *testing.B) {
	b.Skip("Skipping large file benchmark in normal test runs")
	repoPath := setupRestoreBenchRepo(b, 10*1024*1024) // 10MB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_ReflinkEngine_Small benchmarks restore with small payload using reflink engine.
func BenchmarkRestore_ReflinkEngine_Small(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineReflinkCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_ReflinkEngine_Medium benchmarks restore with medium payload using reflink engine.
func BenchmarkRestore_ReflinkEngine_Medium(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024*1024) // 1MB
	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineReflinkCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_MultiFile benchmarks restore with multiple files.
func BenchmarkRestore_MultiFile(b *testing.B) {
	repoPath := setupRestoreBenchRepoWithFiles(b, 100) // 100 files
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Modify a file to ensure restore changes it
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "dir", "0", "file0.txt"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "dir", "0", "file0.txt"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_MultiFile_Large benchmarks restore with many files.
func BenchmarkRestore_MultiFile_Large(b *testing.B) {
	repoPath := setupRestoreBenchRepoWithFiles(b, 1000) // 1000 files
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "dir", "0", "file0.txt"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "dir", "0", "file0.txt"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestoreToLatest benchmarks restoring to the latest snapshot.
func BenchmarkRestoreToLatest(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "first snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create second snapshot (latest)
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("second content"), 0644)
	_, err = creator.Create("main", "second snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Modify content to ensure restore changes it
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.RestoreToLatest("main")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_DetachedState benchmarks restore that puts worktree into detached state.
func BenchmarkRestore_DetachedState(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create second snapshot (latest)
	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("second content"), 0644)
	_, err = creator.Create("main", "second snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify to latest
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("second content"), 0644)
		// Restore to first (enters detached state)
		err := restorer.Restore("main", desc1.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_IntegrityVerification benchmarks restore with integrity verification.
func BenchmarkRestore_IntegrityVerification(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024*100) // 100KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("modified"), 0644)
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_SnapshotToSnapshot benchmarks restore between different snapshots.
func BenchmarkRestore_SnapshotToSnapshot(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc1, err := creator.Create("main", "first snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte("second content"), 0644)
	desc2, err := creator.Create("main", "second snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate between snapshots
		if i%2 == 0 {
			err = restorer.Restore("main", desc1.SnapshotID)
		} else {
			err = restorer.Restore("main", desc2.SnapshotID)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRestore_EmptyWorktree benchmarks restore to an empty worktree.
func BenchmarkRestore_EmptyWorktree(b *testing.B) {
	repoPath := setupRestoreBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	mainPath := filepath.Join(repoPath, "main")
	restorer := restore.NewRestorer(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Remove all content
		os.RemoveAll(mainPath)
		os.MkdirAll(mainPath, 0755)
		// Restore
		err := restorer.Restore("main", desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
