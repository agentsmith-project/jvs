package snapshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// setupBenchRepo creates a repository with test content for benchmarking.
func setupBenchRepo(b *testing.B, contentSize int) string {
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

// setupBenchRepoWithFiles creates a repository with multiple files for benchmarking.
func setupBenchRepoWithFiles(b *testing.B, fileCount int) string {
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

		data := []byte("test content for benchmarking")
		fileName := "file" + strconv.Itoa(i) + ".txt"
		filePath := filepath.Join(subDir, fileName)
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			b.Fatal(err)
		}
	}

	return dir
}

// BenchmarkSnapshotCreation_CopyEngine_Small benchmarks snapshot creation with small payload using copy engine.
func BenchmarkSnapshotCreation_CopyEngine_Small(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_CopyEngine_Medium benchmarks snapshot creation with medium payload using copy engine.
func BenchmarkSnapshotCreation_CopyEngine_Medium(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024*1024) // 1MB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_CopyEngine_Large benchmarks snapshot creation with large payload using copy engine.
func BenchmarkSnapshotCreation_CopyEngine_Large(b *testing.B) {
	b.Skip("Skipping large file benchmark in normal test runs")
	repoPath := setupBenchRepo(b, 10*1024*1024) // 10MB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_ReflinkEngine_Small benchmarks snapshot creation with small payload using reflink engine.
func BenchmarkSnapshotCreation_ReflinkEngine_Small(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024) // 1KB
	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_ReflinkEngine_Medium benchmarks snapshot creation with medium payload using reflink engine.
func BenchmarkSnapshotCreation_ReflinkEngine_Medium(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024*1024) // 1MB
	creator := snapshot.NewCreator(repoPath, model.EngineReflinkCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_MultiFile benchmarks snapshot creation with multiple files.
func BenchmarkSnapshotCreation_MultiFile(b *testing.B) {
	repoPath := setupBenchRepoWithFiles(b, 100) // 100 files
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSnapshotCreation_MultiFile_Large benchmarks snapshot creation with many files.
func BenchmarkSnapshotCreation_MultiFile_Large(b *testing.B) {
	repoPath := setupBenchRepoWithFiles(b, 1000) // 1000 files
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDescriptorSerialization benchmarks descriptor serialization.
func BenchmarkDescriptorSerialization(b *testing.B) {
	desc := &model.Descriptor{
		SnapshotID:         "0000000000000-abc12345",
		ParentID:           nil,
		WorktreeName:       "main",
		CreatedAt:          time.Now(),
		Note:               "benchmark snapshot descriptor with some content",
		Tags:               []string{"v1.0", "release", "stable"},
		Engine:             model.EngineCopy,
		PayloadRootHash:    "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		DescriptorChecksum: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IntegrityState:     model.IntegrityVerified,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(desc)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDescriptorDeserialization benchmarks descriptor deserialization.
func BenchmarkDescriptorDeserialization(b *testing.B) {
	desc := &model.Descriptor{
		SnapshotID:         "0000000000000-abc12345",
		ParentID:           nil,
		WorktreeName:       "main",
		CreatedAt:          time.Now(),
		Note:               "benchmark snapshot descriptor with some content",
		Tags:               []string{"v1.0", "release", "stable"},
		Engine:             model.EngineCopy,
		PayloadRootHash:    "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		DescriptorChecksum: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IntegrityState:     model.IntegrityVerified,
	}
	data, err := json.Marshal(desc)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result model.Descriptor
		if err := json.Unmarshal(data, &result); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoadDescriptor benchmarks loading a descriptor from disk.
func BenchmarkLoadDescriptor(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.LoadDescriptor(repoPath, desc.SnapshotID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifySnapshot_ChecksumOnly benchmarks snapshot verification without payload hash.
func BenchmarkVerifySnapshot_ChecksumOnly(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := snapshot.VerifySnapshot(repoPath, desc.SnapshotID, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifySnapshot_WithPayloadHash benchmarks snapshot verification with payload hash.
func BenchmarkVerifySnapshot_WithPayloadHash(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024*100) // 100KB
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	desc, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComputeDescriptorChecksum benchmarks descriptor checksum computation.
func BenchmarkComputeDescriptorChecksum(b *testing.B) {
	desc := &model.Descriptor{
		SnapshotID:      "0000000000000-abc12345",
		ParentID:        nil,
		WorktreeName:    "main",
		CreatedAt:       time.Now(),
		Note:            "benchmark snapshot descriptor with some content",
		Tags:            []string{"v1.0", "release", "stable"},
		Engine:          model.EngineCopy,
		PayloadRootHash: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		IntegrityState:  model.IntegrityVerified,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := integrity.ComputeDescriptorChecksum(desc)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListAll_Empty benchmarks listing snapshots in an empty repository.
func BenchmarkListAll_Empty(b *testing.B) {
	repoPath := setupBenchRepo(b, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.ListAll(repoPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListAll_Single benchmarks listing snapshots with one snapshot.
func BenchmarkListAll_Single(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)
	_, err := creator.Create("main", "bench snapshot", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.ListAll(repoPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListAll_Many benchmarks listing snapshots with many snapshots.
func BenchmarkListAll_Many(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create 50 snapshots
	for i := 0; i < 50; i++ {
		if _, err := creator.Create("main", "bench snapshot", nil); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.ListAll(repoPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFind_ByTag benchmarks finding snapshots by tag.
func BenchmarkFind_ByTag(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create snapshots with various tags
	tags := []string{"v1.0", "v1.1", "v2.0", "release", "wip"}
	for _, tag := range tags {
		_, err := creator.Create("main", "bench snapshot", []string{tag})
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := snapshot.FilterOptions{HasTag: "release"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.Find(repoPath, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFind_ByWorktree benchmarks finding snapshots by worktree name.
func BenchmarkFind_ByWorktree(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	// Create multiple snapshots
	for i := 0; i < 10; i++ {
		_, err := creator.Create("main", "bench snapshot", nil)
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := snapshot.FilterOptions{WorktreeName: "main"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.Find(repoPath, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFindByTag benchmarks finding a snapshot by tag.
func BenchmarkFindByTag(b *testing.B) {
	repoPath := setupBenchRepo(b, 1024)
	creator := snapshot.NewCreator(repoPath, model.EngineCopy)

	_, err := creator.Create("main", "release snapshot", []string{"v1.0"})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := snapshot.FindByTag(repoPath, "v1.0")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Engine Performance Comparison Benchmarks
// These benchmarks compare the three snapshot engines across different scenarios.
// ============================================================================

// BenchmarkEngineComparison_1KB compares all engines with 1KB payload.
func BenchmarkEngineComparison_1KB(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepo(b, 1024) // 1KB
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_100KB compares all engines with 100KB payload.
func BenchmarkEngineComparison_100KB(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepo(b, 100*1024) // 100KB
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_1MB compares all engines with 1MB payload.
func BenchmarkEngineComparison_1MB(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepo(b, 1024*1024) // 1MB
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_10MB compares all engines with 10MB payload.
func BenchmarkEngineComparison_10MB(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepo(b, 10*1024*1024) // 10MB
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_10Files compares all engines with 10 small files.
func BenchmarkEngineComparison_10Files(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepoWithFiles(b, 10)
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_100Files compares all engines with 100 small files.
func BenchmarkEngineComparison_100Files(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepoWithFiles(b, 100)
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_1000Files compares all engines with 1000 small files.
func BenchmarkEngineComparison_1000Files(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepoWithFiles(b, 1000)
			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngineComparison_Mixed compares all engines with mixed file sizes.
func BenchmarkEngineComparison_Mixed(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := b.TempDir()
			_, err := repo.Init(repoPath, "bench")
			if err != nil {
				b.Fatal(err)
			}

			mainPath := filepath.Join(repoPath, "main")

			// Create mixed file sizes: 1KB, 10KB, 100KB, 1MB
			sizes := []int{1024, 10 * 1024, 100 * 1024, 1024 * 1024}
			for i, size := range sizes {
				data := make([]byte, size)
				for j := range data {
					data[j] = byte((i + j) % 256)
				}
				fileName := filepath.Join(mainPath, "file"+strconv.Itoa(i)+".bin")
				if err := os.WriteFile(fileName, data, 0644); err != nil {
					b.Fatal(err)
				}
			}

			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "bench snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngine_LineageCreation benchmarks creating a chain of snapshots (lineage).
func BenchmarkEngine_LineageCreation(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	chainLength := 10

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				repoPath := setupBenchRepo(b, 100*1024) // 100KB
				creator := snapshot.NewCreator(repoPath, bm.engine)

				// Create a chain of snapshots
				for j := 0; j < chainLength; j++ {
					b.StartTimer()
					_, err := creator.Create("main", "chain snapshot", nil)
					b.StopTimer()
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkEngine_PartialSnapshot benchmarks partial snapshot creation.
func BenchmarkEngine_PartialSnapshot(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := b.TempDir()
			_, err := repo.Init(repoPath, "bench")
			if err != nil {
				b.Fatal(err)
			}

			mainPath := filepath.Join(repoPath, "main")

			// Create 100 files in subdirectories
			for i := 0; i < 100; i++ {
				subDir := filepath.Join(mainPath, "data", strconv.Itoa(i%10))
				if err := os.MkdirAll(subDir, 0755); err != nil {
					b.Fatal(err)
				}
				data := []byte("test content for file " + strconv.Itoa(i))
				filePath := filepath.Join(subDir, "file"+strconv.Itoa(i)+".txt")
				if err := os.WriteFile(filePath, data, 0644); err != nil {
					b.Fatal(err)
				}
			}

			creator := snapshot.NewCreator(repoPath, bm.engine)

			// Snapshot only the "data/0" subdirectory
			partialPaths := []string{"data/0"}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.CreatePartial("main", "partial snapshot", nil, partialPaths)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngine_SnapshotWithCompression benchmarks snapshot creation with compression.
func BenchmarkEngine_SnapshotWithCompression(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := setupBenchRepo(b, 1024*1024) // 1MB of compressible data

			// Use compressible data
			mainPath := filepath.Join(repoPath, "main")
			compressibleData := make([]byte, 1024*1024)
			for i := range compressibleData {
				compressibleData[i] = byte(i % 64) // Highly repetitive
			}
			if err := os.WriteFile(filepath.Join(mainPath, "compressible.bin"), compressibleData, 0644); err != nil {
				b.Fatal(err)
			}

			creator := snapshot.NewCreatorWithCompression(repoPath, bm.engine, nil)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "compressed snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEngine_DeepDirectoryTree benchmarks engines with deeply nested directories.
func BenchmarkEngine_DeepDirectoryTree(b *testing.B) {
	benchmarks := []struct {
		name   string
		engine model.EngineType
	}{
		{"Copy", model.EngineCopy},
		{"Reflink", model.EngineReflinkCopy},
		{"JuiceFS", model.EngineJuiceFSClone},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			repoPath := b.TempDir()
			_, err := repo.Init(repoPath, "bench")
			if err != nil {
				b.Fatal(err)
			}

			mainPath := filepath.Join(repoPath, "main")

			// Create a deep directory tree (10 levels deep, 10 files per level)
			for level := 0; level < 10; level++ {
				levelPath := mainPath
				for i := 0; i <= level; i++ {
					levelPath = filepath.Join(levelPath, "level"+strconv.Itoa(i))
				}
				if err := os.MkdirAll(levelPath, 0755); err != nil {
					b.Fatal(err)
				}
				// Add a file at each level
				data := []byte("content at level " + strconv.Itoa(level))
				filePath := filepath.Join(levelPath, "file.txt")
				if err := os.WriteFile(filePath, data, 0644); err != nil {
					b.Fatal(err)
				}
			}

			creator := snapshot.NewCreator(repoPath, bm.engine)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := creator.Create("main", "deep tree snapshot", nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
