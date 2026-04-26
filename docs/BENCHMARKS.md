# JVS Performance Benchmarks

This document contains internal Go benchmark results for JVS operations. These
numbers are baselines for regression detection on the recorded environment, not
portable latency promises.

## Test Environment

- **CPU**: Intel(R) Core(TM) Ultra 9 285H
- **Date**: 2026-04-25T19:51:20-07:00
- **OS**: Linux 6.18.18-1-MANJARO x86_64
- **Filesystem**: btrfs on `/home` (`/dev/nvme0n1p2`, `compress=zstd:1`, `ssd`)
- **Go Version**: `go version go1.26.1-X:nodwarf5 linux/amd64`
- **go.mod directive**: `go 1.25.6`
- **Result selection**: median of 5 samples from `-count=5 -benchtime=1s`

## Running Benchmarks

Run the current GA evidence package set:

```bash
go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot ./internal/restore ./internal/gc ./internal/worktree
```

Packages covered: `./internal/snapshot/`, `./internal/restore/`,
`./internal/gc/`, and `./internal/worktree/`.

Run all package benchmarks when doing broader investigation:

```bash
go test -run '^$' -bench=. -benchmem ./...
```

Run a specific benchmark:

```bash
go test -run '^$' -bench=BenchmarkSnapshotCreation_CopyEngine_Small -benchmem ./internal/snapshot/
```

Note: benchmark names and package paths below are Go test identifiers from
internal packages; they may retain internal snapshot/worktree vocabulary.
Public CLI examples should use checkpoint, workspace, and fork.

## Benchmark Categories

### Snapshot Operations (`internal/snapshot/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkSnapshotCreation_CopyEngine_Small` | 9,433,194 | 106 | 2,087,336 | 42,980 | ✅ |
| `BenchmarkSnapshotCreation_CopyEngine_Medium` | 8,696,409 | 115 | 1,487,723 | 30,242 | ✅ |
| `BenchmarkSnapshotCreation_ReflinkEngine_Small` | 11,239,611 | 89 | 2,340,440 | 48,033 | ✅ |
| `BenchmarkSnapshotCreation_ReflinkEngine_Medium` | 8,193,584 | 122 | 1,595,740 | 32,298 | ✅ |
| `BenchmarkSnapshotCreation_MultiFile` | 10,671,211 | 94 | 4,287,155 | 15,816 | ✅ |
| `BenchmarkSnapshotCreation_MultiFile_Large` | 64,001,917 | 16 | 37,140,057 | 43,262 | ✅ |
| `BenchmarkDescriptorSerialization` | 2,017 | 495,786 | 496 | 2 | ✅ |
| `BenchmarkDescriptorDeserialization` | 5,889 | 169,808 | 872 | 19 | ✅ |
| `BenchmarkLoadDescriptor` | 23,708 | 42,180 | 3,746 | 43 | ✅ |
| `BenchmarkVerifySnapshot_ChecksumOnly` | 54,987 | 18,186 | 10,653 | 173 | ✅ |
| `BenchmarkVerifySnapshot_WithPayloadHash` | 351,276 | 2,847 | 57,005 | 348 | ✅ |
| `BenchmarkComputeDescriptorChecksum` | 16,077 | 62,201 | 4,271 | 79 | ✅ |
| `BenchmarkListAll_Empty` | 7,230 | 138,313 | 1,227 | 14 | ✅ |
| `BenchmarkListAll_Single` | 61,584 | 16,238 | 10,172 | 111 | ✅ |
| `BenchmarkListAll_Many` | 2,830,170 | 353 | 454,309 | 4,785 | ✅ |
| `BenchmarkFind_ByTag` | 290,821 | 3,439 | 46,731 | 512 | ✅ |
| `BenchmarkFind_ByWorktree` | 552,677 | 1,809 | 93,109 | 981 | ✅ |
| `BenchmarkFindByTag` | 58,954 | 16,962 | 10,191 | 114 | ✅ |

### Restore Operations (`internal/restore/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkRestore_CopyEngine_Small` | 9,164,260 | 109 | 1,865,579 | 37,483 | ✅ |
| `BenchmarkRestore_CopyEngine_Medium` | 7,914,740 | 126 | 1,304,387 | 25,775 | ✅ |
| `BenchmarkRestore_ReflinkEngine_Small` | 8,532,841 | 117 | 1,742,190 | 34,901 | ✅ |
| `BenchmarkRestore_ReflinkEngine_Medium` | 7,996,935 | 125 | 1,269,296 | 25,041 | ✅ |
| `BenchmarkRestore_MultiFile` | 13,572,075 | 74 | 4,185,060 | 13,966 | ✅ |
| `BenchmarkRestore_MultiFile_Large` | 81,431,212 | 12 | 37,463,088 | 54,334 | ✅ |
| `BenchmarkRestoreToLatest` | 8,983,408 | 111 | 1,778,932 | 35,636 | ✅ |
| `BenchmarkRestore_DetachedState` | 9,333,455 | 107 | 1,897,743 | 37,697 | ✅ |
| `BenchmarkRestore_IntegrityVerification` | 8,520,262 | 117 | 1,637,119 | 32,708 | ✅ |
| `BenchmarkRestore_SnapshotToSnapshot` | 8,365,827 | 120 | 1,670,030 | 33,182 | ✅ |
| `BenchmarkRestore_EmptyWorktree` | 9,537,246 | 105 | 1,834,540 | 36,852 | ✅ |

### GC Operations (`internal/gc/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkGCPlan_Small` | 8,963,736 | 112 | 1,792,154 | 27,003 | ✅ |
| `BenchmarkGCPlan_Medium` | 40,793,554 | 25 | 8,129,481 | 78,854 | ✅ |
| `BenchmarkGCPlan_Large` | 422,892,746 | 2 | 79,516,437 | 756,231 | ✅ |
| `BenchmarkGCPlan_WithDeletable` | 43,192,931 | 23 | 8,900,398 | 90,737 | ✅ |
| `BenchmarkGCRun_DeleteSingle` | 48,034,198 | 21 | 9,836,908 | 176,763 | ✅ |
| `BenchmarkGCRun_DeleteMultiple` | 263,769,716 | 4 | 54,614,361 | 907,926 | ✅ |
| `BenchmarkGCLineageTraversal` | 42,355,587 | 24 | 8,163,619 | 78,965 | ✅ |
| `BenchmarkGCWithPins` | 22,490,092 | 44 | 4,387,583 | 45,671 | ✅ |
| `BenchmarkGCEmptyRepo` | 10,633,443 | 94 | 2,171,339 | 42,706 | ✅ |
| `BenchmarkGCWithIntents` | 23,035,006 | 43 | 4,577,045 | 49,298 | ✅ |

### Fork Operations (`internal/worktree/bench_test.go`)

| Benchmark | ns/op | ops/sec | B/op | allocs/op | Status |
|-----------|--------|---------|------|-----------|--------|
| `BenchmarkWorktreeFork_Small` | 8,509,178 | 118 | 1,815,891 | 35,133 | ✅ |
| `BenchmarkWorktreeFork_Medium` | 7,481,623 | 134 | 1,325,255 | 25,288 | ✅ |
| `BenchmarkWorktreeFork_Reflink` | 8,263,617 | 121 | 1,570,902 | 30,206 | ✅ |
| `BenchmarkWorktreeFork_MultiFile` | 13,949,247 | 72 | 4,282,570 | 14,820 | ✅ |
| `BenchmarkWorktreeFork_MultiFile_Large` | 90,616,774 | 11 | 38,234,009 | 61,595 | ✅ |
| `BenchmarkWorktreeList` | 182,285 | 5,486 | 33,842 | 318 | ✅ |
| `BenchmarkWorktreeGet` | 16,126 | 62,012 | 2,826 | 25 | ✅ |
| `BenchmarkWorktreeSetLatest` | 48,063 | 20,806 | 6,118 | 57 | ✅ |

## Performance Expectations

### Snapshot Creation
- **Small files (<100KB)**: Local btrfs baseline is around 9-11ms in this run.
- **Medium files (~1MB)**: Local btrfs baseline is around 8-9ms in this run.
- **Multi-file (100+ files)**: File-count and metadata traversal dominate.
- **Multi-file large (1000 files)**: Around 64ms with about 37MB allocated in this run.
- **Reflink vs Copy**: Performance varies by workload and filesystem; local btrfs results are not portable.

### Restore Operations
- **Small files (<100KB)**: Local btrfs baseline is around 8.5-9.2ms in this run.
- **Medium files (~1MB)**: Local btrfs baseline is around 7.9-8.0ms in this run.
- **Multi-file large (1000 files)**: Around 81ms with about 37MB allocated in this run.
- **Latest checkpoint restore**: `BenchmarkRestoreToLatest` is the current Go benchmark identifier.
- **Empty workspace restore**: Around 9.5ms in this run.

### Catalog Operations
- **ListAll_Empty**: Around 7.2us.
- **ListAll_Single**: Around 62us.
- **ListAll_Many**: Around 2.8ms for 50 checkpoints.
- **Find by tag**: `BenchmarkFindByTag` is around 59us for direct tag lookup.
- **Find by workspace**: `BenchmarkFind_ByWorktree` is around 553us in this run.

### Integrity Verification
- **Checksum only**: Around 55us.
- **With payload hash**: Around 351us for the benchmark payload.
- **ComputeDescriptorChecksum**: Around 16us.

### Fork Operations
- **Small payload**: Around 8.5ms with copy fallback.
- **Medium payload**: Around 7.5ms with copy fallback.
- **Reflink path**: Around 8.3ms on this btrfs run.
- **Multi-file large (1000 files)**: Around 91ms with about 38MB allocated.

## Memory Allocations

Key allocations to track on this baseline:

- **Large checkpoints**: 1000 files: about 43K allocations and 37MB.
- **Large restores**: 1000 files: about 54K allocations and 37MB.
- **Large fork materialization**: 1000 files: about 62K allocations and 38MB.
- **ListAll(50)**: about 4.8K allocations and 454KB.
- **GC delete operations**: delete-multiple path is the highest allocation row in this run.

## Performance Regression Detection

When making changes to critical paths:

1. Run benchmarks before and after.
2. Compare using `benchstat`:
   ```bash
   go test -run '^$' -bench=. -benchmem ./internal/snapshot/ > old.txt
   # make changes
   go test -run '^$' -bench=. -benchmem ./internal/snapshot/ > new.txt
   benchstat old.txt new.txt
   ```
3. Flag any regressions over 10% for review after repeated runs on the same machine.

## Known Performance Characteristics

1. **Engine selection impact**:
   - Copy and reflink rows are internal implementation baselines on local btrfs.
   - The `BenchmarkEngineComparison_*/JuiceFS` sub-benchmarks in raw output are not supported JuiceFS mount evidence.
   - O(1) release claims require `juicefs-clone` on supported JuiceFS.
   - Copy engine is always available as fallback.

2. **Memory allocation patterns**:
   - Small checkpoint and restore rows are dominated by repository setup and descriptor I/O.
   - Large checkpoint, restore, and fork rows scale with file count and materialization shape.
   - GC delete operations scale heavily with candidate count.

3. **Filesystem matters**: This baseline ran on btrfs. Results will vary on:
   - ext4/xfs with reflink settings
   - supported JuiceFS mounts with real metadata-service and network behavior
   - zfs or other CoW-capable storage

4. **Concurrency**: Current implementation is single-threaded; parallel snapshot/restore remains a possible future optimization.

5. **GC Performance**:
   - Planning scales with checkpoint count and descriptor reads.
   - Deleting checkpoints is the most expensive GC operation in this baseline.
   - Empty repo operations still include repository setup and metadata checks in the benchmark harness.

## Additional Benchmark Opportunities

Future benchmark areas to consider:
- [ ] Lock acquisition overhead
- [ ] Concurrent operations
- [ ] Large-scale scenarios with locally validated checkpoint and file counts
- [ ] Cross-engine performance comparison under supported filesystems
