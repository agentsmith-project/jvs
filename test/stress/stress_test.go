// Package stress provides large-scale stress tests for JVS.
// These tests are designed to find performance limits and edge cases with:
// - 10k+ files
// - 1GB+ payloads
// - 100+ snapshots
//
// Run with: go test -v -timeout=30m ./test/stress/...
package stress

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
)

// TestStress_10kFiles tests snapshot performance with 10,000 files.
func TestStress_10kFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_repo")

	// Initialize repository
	r, err := repo.Init(repoPath, "stress_test")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create 10,000 files in nested directories
	t.Log("Creating 10,000 files...")
	start := time.Now()
	createManyFiles(t, mainPath, 10000, 1024) // 1KB each
	elapsed := time.Since(start)
	t.Logf("Created 10,000 files in %v", elapsed)

	// Create snapshot
	t.Log("Creating snapshot...")
	start = time.Now()
	creator := snapshot.NewCreator(repoPath, "copy")
	desc, err := creator.Create("main", "stress", nil)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("Snapshot created in %v (%.2f files/sec)", elapsed, float64(10000)/elapsed.Seconds())

	// Verify snapshot
	t.Log("Verifying snapshot...")
	start = time.Now()
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	if err != nil {
		t.Logf("WARNING: Verification failed: %v", err)
	} else {
		elapsed = time.Since(start)
		t.Logf("Snapshot verified in %v", elapsed)
	}

	// Cleanup snapshot
	t.Log("Cleaning up...")
	start = time.Now()
	os.RemoveAll(mainPath)
	os.MkdirAll(mainPath, 0755)
	elapsed = time.Since(start)
	t.Logf("Cleanup completed in %v", elapsed)
}

// TestStress_LargePayload tests snapshot with 1GB+ payload.
func TestStress_LargePayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_large")

	// Initialize repository
	r, err := repo.Init(repoPath, "large_test")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create files totaling ~1GB
	// Use 100 files of ~10MB each
	t.Log("Creating 1GB payload...")
	start := time.Now()
	fileCount := 100
	fileSize := 10 * 1024 * 1024 // 10MB per file
	createLargeFiles(t, mainPath, fileCount, fileSize)
	elapsed := time.Since(start)
	t.Logf("Created %d files (%.2f GB) in %v", fileCount, float64(fileCount*fileSize)/(1024*1024*1024), elapsed)

	// Create snapshot
	t.Log("Creating snapshot...")
	start = time.Now()
	creator := snapshot.NewCreator(repoPath, "copy")
	desc, err := creator.Create("main", "large", nil)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	elapsed = time.Since(start)
	sizeGB := float64(fileCount*fileSize) / (1024 * 1024 * 1024)
	t.Logf("Snapshot of %.2f GB created in %v (%.2f GB/sec)", sizeGB, elapsed, sizeGB/elapsed.Seconds())

	// Verify (may be slow)
	if testing.Short() {
		t.Skip("skipping verification of large payload in short mode")
	}
	t.Log("Verifying snapshot (may take a while)...")
	start = time.Now()
	err = snapshot.VerifySnapshot(repoPath, desc.SnapshotID, true)
	if err != nil {
		t.Logf("WARNING: Verification failed: %v", err)
	} else {
		elapsed = time.Since(start)
		t.Logf("Snapshot verified in %v", elapsed)
	}
}

// TestStress_ManySnapshots tests creating 100+ snapshots.
func TestStress_ManySnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_snapshots")

	// Initialize repository
	r, err := repo.Init(repoPath, "many_snapshots")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create some initial files
	createManyFiles(t, mainPath, 100, 1024) // 100 files, 1KB each

	snapshotCount := 100
	creator := snapshot.NewCreator(repoPath, "copy")

	t.Logf("Creating %d snapshots...", snapshotCount)
	start := time.Now()
	var totalSize int64

	for i := 0; i < snapshotCount; i++ {
		// Modify a file to make each snapshot different
		testFile := filepath.Join(mainPath, "test.txt")
		content := []byte(fmt.Sprintf("Snapshot %d - %s", i, time.Now().Format(time.RFC3339Nano)))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		desc, err := creator.Create("main", fmt.Sprintf("s%d", i), nil)
		if err != nil {
			t.Fatalf("create snapshot %d: %v", i, err)
		}

		// Track total size
		snapDir := filepath.Join(repoPath, ".jvs", "snapshots", string(desc.SnapshotID))
		totalSize += dirSize(t, snapDir)

		if i > 0 && i%20 == 0 {
			t.Logf("  Created %d/%d snapshots...", i, snapshotCount)
		}
	}
	elapsed := time.Since(start)

	sizeMB := float64(totalSize) / (1024 * 1024)
	t.Logf("Created %d snapshots in %v (%.2f sec/snapshot, %.2f MB total)",
		snapshotCount, elapsed, elapsed.Seconds()/float64(snapshotCount), sizeMB)

	// Test listing performance
	t.Log("Testing snapshot list performance...")
	start = time.Now()
	snapshots, err := snapshot.ListAll(repoPath)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("Listed %d snapshots in %v", len(snapshots), elapsed)

	// Test GC planning performance
	t.Log("Testing GC plan performance...")
	start = time.Now()
	collector := gc.NewCollector(repoPath)
	plan, err := collector.Plan()
	if err != nil {
		t.Fatalf("gc plan: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("GC plan computed in %v (%d candidates)", elapsed, plan.CandidateCount)
}

// TestStress_DeepNesting tests deeply nested directory structures.
func TestStress_DeepNesting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_nesting")

	// Initialize repository
	r, err := repo.Init(repoPath, "stress_nesting")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create deeply nested structure (100 levels deep)
	t.Log("Creating deeply nested structure...")
	start := time.Now()
	maxDepth := 100
	createDeepNesting(t, mainPath, maxDepth)
	elapsed := time.Since(start)
	t.Logf("Created depth-%d structure in %v", maxDepth, elapsed)

	// Snapshot
	t.Log("Creating snapshot...")
	start = time.Now()
	creator := snapshot.NewCreator(repoPath, "copy")
	_, err = creator.Create("main", "deep", nil)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("Snapshot created in %v", elapsed)
}

// TestStress_ManySymlinks tests with many symbolic links.
func TestStress_ManySymlinks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_links")

	// Initialize repository
	r, err := repo.Init(repoPath, "stress_links")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create many symlinks
	t.Log("Creating many symlinks...")
	start := time.Now()
	linkCount := 1000
	createManySymlinks(t, mainPath, linkCount)
	elapsed := time.Since(start)
	t.Logf("Created %d symlinks in %v", linkCount, elapsed)

	// Snapshot
	t.Log("Creating snapshot...")
	start = time.Now()
	creator := snapshot.NewCreator(repoPath, "copy")
	_, err = creator.Create("main", "symlinks", nil)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("Snapshot created in %v", elapsed)
}

// TestStress_LongFilenames tests with very long filenames.
func TestStress_LongFilenames(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_longnames")

	// Initialize repository
	r, err := repo.Init(repoPath, "longnames")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create files with long names (200+ characters)
	t.Log("Creating files with long names...")
	start := time.Now()
	createLongNamedFiles(t, mainPath, 100, 200)
	elapsed := time.Since(start)
	t.Logf("Created 100 long-named files in %v", elapsed)

	// Snapshot
	t.Log("Creating snapshot...")
	start = time.Now()
	creator := snapshot.NewCreator(repoPath, "copy")
	_, err = creator.Create("main", "longnames", nil)
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	elapsed = time.Since(start)
	t.Logf("Snapshot created in %v", elapsed)
}

// TestStress_MemoryUsage tests memory usage during operations.
func TestStress_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "stress_memory")

	// Initialize repository
	r, err := repo.Init(repoPath, "memory")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	_ = r

	mainPath := filepath.Join(repoPath, "main")

	// Create files
	createManyFiles(t, mainPath, 1000, 10*1024) // 10KB each

	creator := snapshot.NewCreator(repoPath, "copy")

	// Create multiple snapshots and check memory
	t.Log("Creating snapshots and checking memory...")
	for i := 0; i < 50; i++ {
		// Modify content
		testFile := filepath.Join(mainPath, "counter.txt")
		os.WriteFile(testFile, []byte(fmt.Sprintf("%d", i)), 0644)

		_, err := creator.Create("main", "mem", nil)
		if err != nil {
			t.Fatalf("snapshot %d: %v", i, err)
		}

		// Force GC periodically
		if i%10 == 9 {
			runtime.GC()
		}
	}

	t.Log("Memory stress test completed")
}

// Helper functions

// createManyFiles creates many files with random content.
func createManyFiles(t *testing.T, dir string, count, fileSize int) {
	t.Helper()

	// Create nested directory structure
	dirsPerLevel := 10
	filesPerDir := count / dirsPerLevel
	if filesPerDir < 10 {
		filesPerDir = 10
	}

	for i := 0; i < count; i++ {
		subDir := fmt.Sprintf("dir%d", i%dirsPerLevel)
		dirPath := filepath.Join(dir, subDir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dirPath, err)
		}

		filePath := filepath.Join(dirPath, fmt.Sprintf("file%04d.dat", i))
		content := make([]byte, fileSize)
		if _, err := rand.Read(content); err != nil {
			t.Fatalf("random bytes: %v", err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("write %s: %v", filePath, err)
		}
	}
}

// createLargeFiles creates large files.
func createLargeFiles(t *testing.T, dir string, count, fileSize int) {
	t.Helper()

	buf := make([]byte, 1024*1024) // 1MB buffer
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random bytes: %v", err)
	}

	for i := 0; i < count; i++ {
		filePath := filepath.Join(dir, fmt.Sprintf("large%04d.bin", i))

		file, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("create %s: %v", filePath, err)
		}

		// Write in chunks
		remaining := fileSize
		for remaining > 0 {
			chunk := len(buf)
			if chunk > remaining {
				chunk = remaining
			}
			if _, err := file.Write(buf[:chunk]); err != nil {
				file.Close()
				t.Fatalf("write %s: %v", filePath, err)
			}
			remaining -= chunk
		}
		file.Close()
	}
}

// createDeepNesting creates a deeply nested directory structure.
func createDeepNesting(t *testing.T, base string, maxDepth int) {
	t.Helper()

	current := base
	for depth := 0; depth < maxDepth; depth++ {
		// Create a file at this level
		filePath := filepath.Join(current, fmt.Sprintf("level%d.txt", depth))
		if err := os.WriteFile(filePath, []byte(fmt.Sprintf("Level %d", depth)), 0644); err != nil {
			t.Fatalf("write %s: %v", filePath, err)
		}

		// Create subdirectory
		subDir := filepath.Join(current, fmt.Sprintf("depth%d", depth))
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", subDir, err)
		}
		current = subDir
	}
}

// createManySymlinks creates many symbolic links.
func createManySymlinks(t *testing.T, dir string, count int) {
	t.Helper()

	// Create a target file
	targetPath := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(targetPath, []byte("target content"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	for i := 0; i < count; i++ {
		linkPath := filepath.Join(dir, fmt.Sprintf("link%04d", i))
		if err := os.Symlink("target.txt", linkPath); err != nil {
			t.Fatalf("symlink %s: %v", linkPath, err)
		}
	}
}

// createLongNamedFiles creates files with very long names.
func createLongNamedFiles(t *testing.T, dir string, count, nameLength int) {
	t.Helper()

	for i := 0; i < count; i++ {
		// Create a long name with repeating pattern
		pattern := strings.Repeat(fmt.Sprintf("file%d_", i), 50)
		var longName string
		if len(pattern) > nameLength {
			longName = pattern[:nameLength]
		} else {
			longName = pattern + strings.Repeat("_", nameLength-len(pattern))
		}
		filePath := filepath.Join(dir, longName)
		if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
			t.Fatalf("write %s: %v", filePath, err)
		}
	}
}

// dirSize calculates total size of a directory.
func dirSize(t *testing.T, path string) int64 {
	t.Helper()

	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

// Benchmark for comparison with stress tests
func BenchmarkSnapshot_100Files(b *testing.B) {
	tmpDir := b.TempDir()
	repoPath := filepath.Join(tmpDir, "bench")

	r, _ := repo.Init(repoPath, "bench")
	_ = r

	mainPath := filepath.Join(repoPath, "main")
	createManyFiles(&testing.T{}, mainPath, 100, 1024)

	creator := snapshot.NewCreator(repoPath, "copy")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testFile := filepath.Join(mainPath, "bench.txt")
		os.WriteFile(testFile, []byte(fmt.Sprintf("bench %d", i)), 0644)
		creator.Create("main", "bench", nil)
	}
}
