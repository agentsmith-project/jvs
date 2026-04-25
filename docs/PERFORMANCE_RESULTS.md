# JVS Performance Results

**Version:** v0 public contract
**Last Updated:** 2026-04-25

This document records release-facing performance boundaries for the GA docs.
Numbers are baselines for regression detection, not portable latency promises.
Actual behavior depends on the selected engine, filesystem, file count,
metadata service health, and payload size.

## Running Benchmarks

Run the benchmark packages used for current GA result checks:

```bash
go test -bench=. -benchmem ./internal/snapshot/
go test -bench=. -benchmem ./internal/restore/
go test -bench=. -benchmem ./internal/gc/
```

Run a focused benchmark when investigating a specific path:

```bash
go test -bench=BenchmarkSnapshotCreation_CopyEngine_Small -benchmem ./internal/snapshot/
```

Compare local before/after results with `benchstat`:

```bash
go test -bench=. -benchmem ./internal/snapshot/ > old.txt
# make changes
go test -bench=. -benchmem ./internal/snapshot/ > new.txt
benchstat old.txt new.txt
```

## GA Result Boundaries (v0)

| Engine | GA boundary | Current result coverage | Release-facing claim |
|--------|-------------|-------------------------|----------------------|
| `juicefs-clone` | Metadata clone when source and destination are on a supported JuiceFS mount and the `juicefs` CLI succeeds | Contract and capability coverage; environment-specific latency must be measured on a mounted JuiceFS deployment | O(1) metadata clone only in the supported JuiceFS case |
| `reflink-copy` | Linear tree walk with per-file CoW data sharing on supported filesystems | Internal snapshot and restore benchmarks exercise the reflink engine path on test filesystems | Faster than copy for some workloads, but not an O(1) whole-tree promise |
| `copy` | Portable recursive copy fallback | Internal snapshot and restore benchmarks exercise copy fallback | O(n) in payload bytes and file count |

Release-facing docs must scope constant-time or O(1) claims to the `juicefs-clone` engine on supported JuiceFS mounts; `reflink-copy` can avoid data duplication per file, but it still walks the file tree. `copy` is always portable and linear.

## Current Unit Benchmark Baseline

**Environment:**

| Component | Specification |
|-----------|---------------|
| CPU | Intel Core Ultra 9 285H |
| OS | Linux (Manjaro) |
| Filesystem | tmpfs for stable unit benchmark inputs |
| Go Version | go1.23.6 linux/amd64 |
| Date | 2026-02-23 |

### Checkpoint Creation Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkSnapshotCreation_CopyEngine_Small` | 5,661,982 | 2,229,427 | 52,282 |
| `BenchmarkSnapshotCreation_CopyEngine_Medium` | 2,729,661 | 842,660 | 19,010 |
| `BenchmarkSnapshotCreation_ReflinkEngine_Small` | 6,883,343 | 2,619,164 | 61,163 |
| `BenchmarkSnapshotCreation_ReflinkEngine_Medium` | 2,312,636 | 628,079 | 13,744 |
| `BenchmarkSnapshotCreation_MultiFile` | 3,708,592 | 3,934,073 | 9,860 |
| `BenchmarkSnapshotCreation_MultiFile_Large` | 28,667,012 | 36,981,941 | 46,765 |

### Restore Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkRestore_CopyEngine_Small` | 7,570,720 | 3,006,635 | 53,857 |
| `BenchmarkRestore_CopyEngine_Medium` | 2,480,206 | 870,531 | 15,481 |
| `BenchmarkRestore_ReflinkEngine_Small` | 5,535,040 | 2,132,300 | 38,126 |
| `BenchmarkRestore_ReflinkEngine_Medium` | 2,841,165 | 1,085,857 | 19,338 |
| `BenchmarkRestore_MultiFile` | 2,172,883 | 508,972 | 8,467 |
| `BenchmarkRestore_MultiFile_Large` | 13,491,429 | 1,431,251 | 19,545 |

### Metadata and Integrity Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkDescriptorSerialization` | 876.5 | 496 | 2 |
| `BenchmarkDescriptorDeserialization` | 2,087 | 760 | 19 |
| `BenchmarkLoadDescriptor` | 4,882 | 1,624 | 19 |
| `BenchmarkVerifySnapshot_ChecksumOnly` | 10,606 | 4,703 | 78 |
| `BenchmarkVerifySnapshot_WithPayloadHash` | 60,286 | 40,375 | 119 |

### GC Benchmarks

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| `BenchmarkGCPlan_Small` | 112,264 | 31,955 | 388 |
| `BenchmarkGCPlan_Medium` | 684,292 | 221,844 | 2,482 |
| `BenchmarkGCPlan_Large` | 7,165,428 | 2,190,340 | 23,220 |
| `BenchmarkGCRun_DeleteSingle` | 8,210,714 | 3,324,222 | 67,652 |
| `BenchmarkGCRun_DeleteMultiple` | 59,789,998 | 24,281,968 | 509,037 |

## Regression Use

Investigate changes that move a benchmark by more than 10% after repeated
runs on the same machine. For JuiceFS deployments, record local mount,
metadata-service, and network details next to any environment-specific result.

## Related Documentation

- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
- [BENCHMARKS.md](BENCHMARKS.md) - Internal benchmark inventory
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Performance troubleshooting
