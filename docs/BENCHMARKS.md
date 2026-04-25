# JVS Performance Benchmarks

This document contains actual benchmark results for JVS operations, providing performance baselines and helping identify regressions.

## Test Environment

- **CPU**: Intel(R) Core(TM) Ultra 9 285H
- **Date**: 2026-02-23
- **OS**: Linux (Manjaro)
- **Filesystem**: tmpfs (for consistent benchmark results)
- **Go Version**: `go version go1.23.6 linux/amd64`

## Running Benchmarks

Run all benchmarks:
```bash
go test -bench=. -benchmem ./...
```

Run specific package benchmarks:
```bash
go test -bench=. -benchmem ./internal/snapshot/
go test -bench=. -benchmem ./internal/restore/
go test -bench=. -benchmem ./internal/gc/
```

Run a specific benchmark:
```bash
go test -bench=BenchmarkSnapshotCreation_CopyEngine_Small -benchmem ./internal/snapshot/
```

Note: Benchmark names and package paths below are Go test identifiers from internal packages; they may retain internal snapshot/worktree vocabulary. Public CLI examples should use checkpoint, workspace, and fork.

## Benchmark Categories

### Snapshot Operations (`internal/snapshot/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkSnapshotCreation_CopyEngine_Small` | 5,661,982 | 177 | 2,229,427 | 52,282 | ✅ |
| `BenchmarkSnapshotCreation_CopyEngine_Medium` | 2,729,661 | 366 | 842,660 | 19,010 | ✅ |
| `BenchmarkSnapshotCreation_ReflinkEngine_Small` | 6,883,343 | 145 | 2,619,164 | 61,163 | ✅ |
| `BenchmarkSnapshotCreation_ReflinkEngine_Medium` | 2,312,636 | 432 | 628,079 | 13,744 | ✅ |
| `BenchmarkSnapshotCreation_MultiFile` | 3,708,592 | 270 | 3,934,073 | 9,860 | ✅ |
| `BenchmarkSnapshotCreation_MultiFile_Large` | 28,667,012 | 35 | 36,981,941 | 46,765 | ✅ |
| `BenchmarkDescriptorSerialization` | 876.5 | 1,140,915 | 496 | 2 | ✅ |
| `BenchmarkDescriptorDeserialization` | 2,087 | 479,157 | 760 | 19 | ✅ |
| `BenchmarkLoadDescriptor` | 4,882 | 204,832 | 1,624 | 19 | ✅ |
| `BenchmarkVerifySnapshot_ChecksumOnly` | 10,606 | 94,287 | 4,703 | 78 | ✅ |
| `BenchmarkVerifySnapshot_WithPayloadHash` | 60,286 | 16,590 | 40,375 | 119 | ✅ |
| `BenchmarkComputeDescriptorChecksum` | 6,047 | 165,375 | 4,158 | 79 | ✅ |
| `BenchmarkListAll_Empty` | 1,648 | 606,796 | 264 | 5 | ✅ |
| `BenchmarkListAll_Single` | 7,705 | 129,788 | 2,028 | 29 | ✅ |
| `BenchmarkListAll_Many` | 326,660 | 3,061 | 91,224 | 1,169 | ✅ |
| `BenchmarkFind_ByTag` | 32,206 | 31,049 | 9,626 | 144 | ✅ |
| `BenchmarkFind_ByWorktree` | 58,563 | 17,076 | 18,825 | 250 | ✅ |
| `BenchmarkFindByTag` | 8,335 | 119,970 | 2,076 | 33 | ✅ |

### Restore Operations (`internal/restore/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkRestore_CopyEngine_Small` | 7,570,720 | 132 | 3,006,635 | 53,857 | ✅ |
| `BenchmarkRestore_CopyEngine_Medium` | 2,480,206 | 403 | 870,531 | 15,481 | ✅ |
| `BenchmarkRestore_ReflinkEngine_Small` | 5,535,040 | 181 | 2,132,300 | 38,126 | ✅ |
| `BenchmarkRestore_ReflinkEngine_Medium` | 2,841,165 | 352 | 1,085,857 | 19,338 | ✅ |
| `BenchmarkRestore_MultiFile` | 2,172,883 | 460 | 508,972 | 8,467 | ✅ |
| `BenchmarkRestore_MultiFile_Large` | 13,491,429 | 74 | 1,431,251 | 19,545 | ✅ |
| `BenchmarkRestoreToLatest` | 5,416,713 | 185 | 2,200,924 | 39,365 | ✅ |
| Restore from historical checkpoint | 6,450,542 | 155 | 2,641,551 | 47,298 | ✅ |
| `BenchmarkRestore_IntegrityVerification` | 5,293,811 | 189 | 2,152,842 | 38,495 | ✅ |
| `BenchmarkRestore_SnapshotToSnapshot` | 5,474,538 | 183 | 2,238,155 | 40,036 | ✅ |
| `BenchmarkRestore_EmptyWorktree` | 6,457,379 | 155 | 2,601,160 | 46,557 | ✅ |

### GC Operations (`internal/gc/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkGCPlan_Small` | 112,264 | 8,907 | 31,955 | 388 | ✅ |
| `BenchmarkGCPlan_Medium` | 684,292 | 1,461 | 221,844 | 2,482 | ✅ |
| `BenchmarkGCPlan_Large` | 7,165,428 | 140 | 2,190,340 | 23,220 | ✅ |
| `BenchmarkGCPlan_WithDeletable` | 392,169 | 2,550 | 131,268 | 1,431 | ✅ |
| `BenchmarkGCRun_DeleteSingle` | 8,210,714 | 122 | 3,324,222 | 67,652 | ✅ |
| `BenchmarkGCRun_DeleteMultiple` | 59,789,998 | 17 | 24,281,968 | 509,037 | ✅ |
| `BenchmarkGCLineageTraversal` | 767,341 | 1,303 | 221,665 | 2,479 | ✅ |
| `BenchmarkGCWithPins` | 477,367 | 2,095 | 144,954 | 1,632 | ✅ |
| `BenchmarkGCEmptyRepo` | 27,127 | 36,862 | 5,305 | 64 | ✅ |
| `BenchmarkGCWithIntents` | 429,359 | 2,329 | 125,554 | 1,350 | ✅ |

## Performance Expectations

### Snapshot Creation
- **Small files (<100KB)**: Completes in ~5-7ms (includes setup overhead)
- **Medium files (~1MB)**: Completes in ~2.3-2.7ms
- **Multi-file (100+ files)**: Scales linearly with file count (~3.7ms for 100 files)
- **Multi-file large (1000 files)**: ~29ms with ~37MB allocations
- **Reflink vs Copy**: Performance varies by workload; reflink is faster for medium files, copy is slightly faster for small files on tmpfs

### Restore Operations
- **Small files (<100KB)**: Completes in ~5.5-7.6ms
- **Medium files (~1MB)**: Completes in ~2.5-2.8ms
- **Multi-file large (1000 files)**: ~13.5ms with ~1.4MB allocations
- **Historical checkpoint restore**: ~6.5ms (includes integrity verification)
- **Empty workspace restore**: ~6.5ms (similar to content replace due to verification overhead)

### Catalog Operations
- **ListAll_Empty**: ~1.6μs (fast path)
- **ListAll_Single**: ~7.7μs
- **ListAll_Many**: ~327μs for 50 checkpoints (~6.5μs per checkpoint)
- **Find by tag**: ~8.3μs (optimized index lookup)
- **Find by workspace**: ~59μs (requires filtering)

### Integrity Verification
- **Checksum only**: ~10.6μs (SHA-256 of descriptor)
- **With payload hash**: ~60μs for small payloads (SHA-256 tree hash)
- **ComputeDescriptorChecksum**: ~6μs

## Memory Allocations

Key allocations to track:

- **Large checkpoints**: 1000 files ~47K allocations (~37MB)
- **Large restores**: 1000 files ~20K allocations (~1.4MB)
- **ListAll(50)**: ~1.2K allocations (~91KB)
- **GC delete operations**: Scale heavily with checkpoint count (509K allocations for 100 checkpoints)

## Performance Regression Detection

When making changes to critical paths:

1. Run benchmarks before and after
2. Compare using `benchstat`:
   ```bash
   go test -bench=. -benchmem ./internal/snapshot/ > old.txt
   # make changes
   go test -bench=. -benchmem ./internal/snapshot/ > new.txt
   benchstat old.txt new.txt
   ```
3. Flag any regressions >10% for review

## Known Performance Characteristics

1. **Engine selection impact** (on tmpfs):
   - For small files: Copy is slightly faster than Reflink (~5.7ms vs ~6.9ms for snapshot)
   - For medium files: Reflink is faster than Copy (~2.3ms vs ~2.7ms for snapshot)
   - For large operations: Copy is generally faster on tmpfs (no CoW overhead)
   - Real filesystems (ext4/xfs) will favor reflink due to block-level cloning
   - Copy engine is always available as fallback

2. **Memory allocation patterns**:
   - Snapshot creation: ~50K-53K allocations for small files (setup overhead dominates)
   - Restore operations: ~40K-54K allocations per operation
   - Large checkpoints (1000 files): ~47K allocations, ~37MB
   - Large restores (1000 files): ~20K allocations, ~1.4MB
   - GC delete operations: Scale heavily with checkpoint count (509K allocations for 100 checkpoints)

3. **Filesystem matters**: Benchmarks run on tmpfs; results will vary on:
   - ext4/xfs: Reflink performance improves significantly
   - JuiceFS: Network latency dominates; engine selection less critical
   - btrfs/zfs: CoW performance similar to reflink results

4. **Concurrency**: Current implementation is single-threaded; parallel snapshot/restore is a future optimization

5. **GC Performance**:
   - Planning scales linearly with checkpoint count (~112μs for 10 checkpoints)
   - Deleting checkpoints is the most expensive GC operation (~8.2ms for single, ~60ms for 100)
   - Lineage traversal is efficient (~767μs for complex histories)
   - Empty repo operations are very fast (~27μs)

## Additional Benchmark Opportunities

Future benchmark areas to consider:
- [ ] Workspace forking performance
- [ ] Lock acquisition overhead
- [ ] Concurrent operations
- [ ] Large-scale scenarios (10K+ checkpoints, 100K+ files)
- [ ] Cross-engine performance comparison under various filesystems
