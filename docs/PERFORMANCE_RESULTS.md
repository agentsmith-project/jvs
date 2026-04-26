# JVS Performance Results

**Version:** v0 public contract
**Last Updated:** 2026-04-25

This document records release-facing performance boundaries for the GA docs.
Numbers are same-machine regression baselines, not portable latency promises.
Actual behavior depends on the selected engine, filesystem, file count,
metadata service health, cache state, and payload size.

## Running Benchmarks

Run the benchmark packages used for current GA evidence checks:

```bash
go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot ./internal/restore ./internal/gc ./internal/worktree
```

Run quick package-level checks when a full evidence run is unnecessary:

```bash
go test -bench=. -benchmem ./internal/snapshot/
go test -bench=. -benchmem ./internal/restore/
go test -bench=. -benchmem ./internal/gc/
```

Run a focused benchmark when investigating a specific path:

```bash
go test -run '^$' -bench=BenchmarkSnapshotCreation_CopyEngine_Small -benchmem ./internal/snapshot/
```

Compare local before/after results with `benchstat`:

```bash
go test -run '^$' -bench=. -benchmem ./internal/snapshot/ > old.txt
# make changes
go test -run '^$' -bench=. -benchmem ./internal/snapshot/ > new.txt
benchstat old.txt new.txt
```

## GA Result Boundaries (v0)

| Engine | GA boundary | Current result coverage | Release-facing claim |
|--------|-------------|-------------------------|----------------------|
| `juicefs-clone` | Metadata clone when source and destination are on a supported JuiceFS mount and the `juicefs` CLI succeeds | Contract and capability coverage; environment-specific latency must be measured on a mounted JuiceFS deployment | O(1) metadata clone only in the supported JuiceFS case |
| `reflink-copy` | Linear tree walk with per-file CoW data sharing on supported filesystems | Internal snapshot, restore, and fork benchmarks exercise the reflink engine path on the local test filesystem | Faster than copy for some workloads, but not an O(1) whole-tree promise |
| `copy` | Portable recursive copy fallback | Internal snapshot, restore, GC, and fork benchmarks exercise copy fallback | O(n) in payload bytes and file count |

Release-facing docs must scope constant-time or O(1) claims to the `juicefs-clone` engine on supported JuiceFS mounts.
The 2026-04-25 evidence below was collected on local btrfs, not on a JuiceFS
mount; it must not be used as supported JuiceFS O(1) latency evidence.
`reflink-copy` can avoid data duplication per file, but it still walks the file
tree. `copy` is always portable and linear.

## Current Unit Benchmark Baseline

Results below are the median of five samples from one benchmark run.

**Environment:**

| Component | Specification |
|-----------|---------------|
| CPU | Intel(R) Core(TM) Ultra 9 285H |
| OS | Linux 6.18.18-1-MANJARO x86_64 |
| Filesystem | btrfs on `/home` (`/dev/nvme0n1p2`, `compress=zstd:1`, `ssd`) |
| Go Version | `go version go1.26.1-X:nodwarf5 linux/amd64` |
| go.mod directive | `go 1.25.6` |
| Date | 2026-04-25T19:51:20-07:00 |
| Benchmark command | `go test -run '^$' -bench=. -benchmem -count=5 -benchtime=1s ./internal/snapshot ./internal/restore ./internal/gc ./internal/worktree` |

### Checkpoint Creation Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkSnapshotCreation_CopyEngine_Small` | 9,433,194 | 2,087,336 | 42,980 |
| `BenchmarkSnapshotCreation_CopyEngine_Medium` | 8,696,409 | 1,487,723 | 30,242 |
| `BenchmarkSnapshotCreation_ReflinkEngine_Small` | 11,239,611 | 2,340,440 | 48,033 |
| `BenchmarkSnapshotCreation_ReflinkEngine_Medium` | 8,193,584 | 1,595,740 | 32,298 |
| `BenchmarkSnapshotCreation_MultiFile` | 10,671,211 | 4,287,155 | 15,816 |
| `BenchmarkSnapshotCreation_MultiFile_Large` | 64,001,917 | 37,140,057 | 43,262 |

### Restore Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkRestore_CopyEngine_Small` | 9,164,260 | 1,865,579 | 37,483 |
| `BenchmarkRestore_CopyEngine_Medium` | 7,914,740 | 1,304,387 | 25,775 |
| `BenchmarkRestore_ReflinkEngine_Small` | 8,532,841 | 1,742,190 | 34,901 |
| `BenchmarkRestore_ReflinkEngine_Medium` | 7,996,935 | 1,269,296 | 25,041 |
| `BenchmarkRestore_MultiFile` | 13,572,075 | 4,185,060 | 13,966 |
| `BenchmarkRestore_MultiFile_Large` | 81,431,212 | 37,463,088 | 54,334 |

### Metadata and Integrity Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkDescriptorSerialization` | 2,017 | 496 | 2 |
| `BenchmarkDescriptorDeserialization` | 5,889 | 872 | 19 |
| `BenchmarkLoadDescriptor` | 23,708 | 3,746 | 43 |
| `BenchmarkVerifySnapshot_ChecksumOnly` | 54,987 | 10,653 | 173 |
| `BenchmarkVerifySnapshot_WithPayloadHash` | 351,276 | 57,005 | 348 |

### GC Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkGCPlan_Small` | 8,963,736 | 1,792,154 | 27,003 |
| `BenchmarkGCPlan_Medium` | 40,793,554 | 8,129,481 | 78,854 |
| `BenchmarkGCPlan_Large` | 422,892,746 | 79,516,437 | 756,231 |
| `BenchmarkGCRun_DeleteSingle` | 48,034,198 | 9,836,908 | 176,763 |
| `BenchmarkGCRun_DeleteMultiple` | 263,769,716 | 54,614,361 | 907,926 |

### Fork Benchmarks (`./internal/worktree/`)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkWorktreeFork_Small` | 8,509,178 | 1,815,891 | 35,133 |
| `BenchmarkWorktreeFork_Medium` | 7,481,623 | 1,325,255 | 25,288 |
| `BenchmarkWorktreeFork_Reflink` | 8,263,617 | 1,570,902 | 30,206 |
| `BenchmarkWorktreeFork_MultiFile` | 13,949,247 | 4,282,570 | 14,820 |
| `BenchmarkWorktreeFork_MultiFile_Large` | 90,616,774 | 38,234,009 | 61,595 |

## Regression Use

Investigate changes that move a benchmark by more than 10% after repeated
runs on the same machine. For JuiceFS deployments, record local mount,
metadata-service, and network details next to any environment-specific result.

## Related Documentation

- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
- [BENCHMARKS.md](BENCHMARKS.md) - Internal benchmark inventory
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Performance troubleshooting
