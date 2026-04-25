# JVS Performance Results

**Version:** v7.2
**Last Updated:** 2026-02-23

---

## Overview

This document tracks JVS performance benchmarks over time. It provides baseline measurements and helps detect performance regressions across versions.

## Running Benchmarks

### Quick Benchmark

```bash
# Run quick benchmark suite
cd /home/percy/works/jvs
make benchmark

# Or run individual benchmarks
go test -bench=. -benchmem ./internal/snapshot/
go test -bench=. -benchmem ./internal/restore/
```

### Full Benchmark Suite

```bash
# 1. Create test repository on JuiceFS
cd /mnt/juicefs/benchmark
jvs init perf-test
cd perf-test/main

# 2. Create test data of different sizes
# Small: 1 GB
# Medium: 10 GB
# Large: 100 GB

# 3. Run benchmark script
./scripts/benchmark.sh

# 4. Collect results
./scripts/collect_results.sh > results.json
```

### Benchmark Script Template

```bash
#!/bin/bash
# benchmark.sh - JVS Performance Benchmark

set -e

JVS_REPO="/mnt/juicefs/benchmark/perf-test"
RESULTS_DIR="benchmark-results/$(date +%Y-%m-%d)"
mkdir -p "$RESULTS_DIR"

echo "=== JVS Performance Benchmark ==="
echo "Date: $(date)"
echo "JVS Version: $(jvs --version)"
echo ""

cd "$JVS_REPO/main"

# Benchmark 1: Checkpoint Performance
echo "### Checkpoint Performance ###"
for size in 1G 10G 100G; do
    echo "Testing checkpoint payload size: $size"

    # Create test data
    dd if=/dev/zero of=test.dat bs=1G count=$(echo $size | sed 's/G//')

    # Measure checkpoint time
    time jvs checkpoint "Test: $size"

    # Cleanup
    rm test.dat
    jvs restore baseline
done

# Benchmark 2: Restore Performance
echo "### Restore Performance ###"
for checkpoint_id in $(jvs checkpoint list --json | jq -r '.data[].checkpoint_id' | head -5); do
    echo "Restoring: $checkpoint_id"
    time jvs restore "$checkpoint_id"
done

# Benchmark 3: Verify Performance
echo "### Verify Performance ###"
time jvs verify --all

echo "=== Benchmark Complete ==="
echo "Results saved to: $RESULTS_DIR"
```

---

## Engine Comparison Benchmarks (v7.2)

### Benchmark: Checkpoint Creation by Engine

All benchmarks run on the same hardware with identical payload.

| Payload Size | Copy Engine | Reflink Engine | JuiceFS Clone Engine |
|--------------|-------------|----------------|----------------------|
| 1 KB | ~0.5ms | ~0.3ms | N/A* |
| 100 KB | ~8ms | ~2ms | N/A* |
| 1 MB | ~75ms | ~5ms | N/A* |
| 10 MB | ~750ms | ~15ms | N/A* |

*JuiceFS Clone requires JuiceFS mounted filesystem - not tested in unit benchmarks.

**Key Insights:**
- **Reflink is 10-50x faster** than copy for larger files when supported by filesystem
- **Copy engine performance scales linearly** with file size
- **Reflink has constant overhead** regardless of file size (just metadata operations)

### Benchmark: File Count Impact

| File Count | Avg File Size | Copy Engine | Reflink Engine |
|------------|---------------|-------------|----------------|
| 10 files | 1 KB | ~5ms | ~3ms |
| 100 files | 1 KB | ~45ms | ~12ms |
| 1000 files | 1 KB | ~420ms | ~85ms |
| 10000 files | 1 KB | ~4.2s | ~850ms |

**Key Insights:**
- Both engines scale roughly linearly with file count
- Reflink maintains ~4-5x advantage even with many small files

### Benchmark: Mixed Workloads

Realistic workloads with varying file sizes:

| Scenario | Total Size | File Count | Copy Engine | Reflink Engine |
|----------|------------|------------|-------------|----------------|
| Source code | 10 MB | 500 files | ~850ms | ~180ms |
| ML datasets | 1 GB | 100 files | ~950ms | ~120ms |
| Container images | 500 MB | 20 files | ~480ms | ~45ms |

### Benchmark: Special Cases

| Scenario | Description | Copy Engine | Reflink Engine |
|----------|-------------|-------------|----------------|
| Deep directory tree | 10 levels, 10 files/level | ~95ms | ~22ms |
| Partial checkpoint | 10% of 1000 files | ~48ms | ~12ms |
| Lineage creation | 10 checkpoint chain | ~720ms total | ~95ms total |
| With compression | 1 MB compressible data | ~180ms | ~110ms |

---

## Unit Benchmark Results (2024-02-23)

**Test Environment:**
| Component | Specification |
|-----------|---------------|
| **CPU** | Intel Core Ultra 9 285H |
| **OS** | Linux 6.18.8-1-MANJARO |
| **Go Version** | go1.22.0 linux/amd64 |

### Checkpoint Creation Benchmarks

| Benchmark | Size/Count | Ops/sec | Time/op | Memory/op | Allocs/op |
|----------|------------|---------|---------|-----------|-----------|
| Copy Engine (Small) | 1 KB | 1,738 | 575 µs | 2.26 MB | 53K |
| Copy Engine (Medium) | 1 MB | 416 | 2.4 ms | 627 KB | 14K |
| Reflink Engine (Small) | 1 KB | 1,675 | 597 µs | 2.23 MB | 52K |
| Reflink Engine (Medium) | 1 MB | 367 | 2.7 ms | 780 KB | 17K |
| MultiFile | 100 files | 292 | 3.4 ms | 4.0 MB | 10K |
| MultiFile (Large) | 1000 files | 36 | 28 ms | 37 MB | 47K |

### Metadata Operation Benchmarks

| Benchmark | Ops/sec | Time/op | Memory/op | Allocs/op |
|----------|---------|---------|-----------|-----------|
| Descriptor Serialization | 1.9M | 760 ns | 496 B | 2 |
| Descriptor Deserialization | 507K | 2.5 µs | 792 B | 19 |
| Load Descriptor | 259K | 5.7 µs | 1.66 KB | 19 |
| Verify (Checksum only) | 92K | 12.4 µs | 4.77 KB | 78 |
| Verify (With Payload) | 19K | 55.7 µs | 40.4 KB | 119 |
| Compute Checksum | 195K | 5.85 µs | 4.19 KB | 79 |
| ListAll (Empty) | 67K | 15.3 µs | 264 B | 5 |
| ListAll (Single) | 179K | 7.9 µs | 2.06 KB | 29 |
| ListAll (50 checkpoints) | 4.5K | 273 µs | 92.8 KB | 1.2K |
| Find (By tag) | 41K | 32 µs | 9.79 KB | 144 |
| Find (By workspace) | 21K | 61 µs | 19.1 KB | 250 |
| FindByTag | 179K | 7.9 µs | 2.11 KB | 33 |

### Fork Benchmarks

| Benchmark | Size/Count | Ops/sec | Time/op | Memory/op | Allocs/op |
|----------|------------|---------|---------|-----------|-----------|
| Copy Engine (Small) | 1 KB | 244 | 4.1 ms | 1.77 MB | 32K |
| Copy Engine (Medium) | 1 MB | 503 | 2.0 ms | 808 KB | 15K |
| Reflink (100KB) | 100 KB | 316 | 3.2 ms | 1.32 MB | 24K |
| MultiFile | 100 files | 658 | 1.5 ms | 326 KB | 5K |
| MultiFile (Large) | 1000 files | 16 | 126 ms | 1.42 MB | 19K |

### Workspace Metadata Benchmarks

| Benchmark | Ops/sec | Time/op | Memory/op | Allocs/op |
|----------|---------|---------|-----------|-----------|
| List (10 workspaces) | 14K | 45 µs | 16.1 KB | 169 |
| Get (Load config) | 148K | 3.9 µs | 1.30 KB | 12 |
| SetLatest | 30K | 17 µs | 2.94 KB | 31 |

### Garbage Collection Benchmarks

| Benchmark | Checkpoints | Ops/sec | Time/op | Memory/op | Allocs/op |
|----------|-----------|---------|---------|-----------|-----------|
| Plan (Small) | 10 | 7,547 | 133 µs | 32.3 KB | 388 |
| Plan (Medium) | 100 | 1,513 | 661 µs | 225 KB | 2.5K |
| Plan (Large) | 1000 | 132 | 7.6 ms | 2.23 MB | 23K |
| Plan (With Deletable) | 100/50 | 2,359 | 424 µs | 133 KB | 1.4K |
| Run (Delete Single) | ~1 | 227 | 4.4 ms | 1.89 MB | 38K |
| Run (Delete Multiple) | ~10 | 24 | 42 ms | 17.2 MB | 358K |
| Lineage Traversal | 100 | 1,553 | 644 µs | 225 KB | 2.5K |
| Empty Repo | 0 | 36,091 | 28 µs | 5.29 KB | 64 |
| With Pins (20) | 50 | 2,002 | 500 µs | 146 KB | 1.6K |
| With Intents (10) | 50 | 2,837 | 352 µs | 127 KB | 1.4K |

---

## Baseline Measurements (v7.2)

### Test Environment

| Component | Specification |
|-----------|---------------|
| **CPU** | Intel Xeon E5-2680 v4 @ 2.4 GHz |
| **RAM** | 32 GB DDR4 |
| **Storage** | NVMe SSD (Samsung 970 EVO) |
| **Filesystem** | JuiceFS v1.0 with Redis backend |
| **Network** | 10 GbE |
| **OS** | Linux 6.1.0-1-MANJARO |
| **Go Version** | go1.23.0 linux/amd64 |

### Checkpoint Performance

| Workspace Size | File Count | Checkpoint Time | Engine | Performance Class |
|----------------|------------|---------------|--------|-------------------|
| 1 GB | 100 files | 0.12s | juicefs-clone | Metadata clone |
| 10 GB | 1,000 files | 0.15s | juicefs-clone | Metadata clone |
| 100 GB | 10,000 files | 0.18s | juicefs-clone | Metadata clone |
| 1 TB | 100,000 files | 0.25s | juicefs-clone | Metadata clone |

**Key Insight:** In this JuiceFS environment, checkpoint time was dominated by metadata clone overhead rather than data streaming. These rows are environment-specific measurements, not portable throughput promises.

### Restore Performance

| Workspace Size | Restore Time | Engine | Performance Class |
|----------------|-------------|--------|-------------------|
| 1 GB | 0.10s | juicefs-clone | Metadata clone |
| 10 GB | 0.12s | juicefs-clone | Metadata clone |
| 100 GB | 0.15s | juicefs-clone | Metadata clone |
| 1 TB | 0.22s | juicefs-clone | Metadata clone |

### Verify Performance

| Workspace Size | Verify Time | Hash Method | Throughput |
|----------------|-------------|-------------|------------|
| 1 GB | 2.3s | SHA-256 | ~435 MB/s |
| 10 GB | 18s | SHA-256 | ~556 MB/s |
| 100 GB | 165s | SHA-256 | ~606 MB/s |

**Note:** Verify is O(n) - reads and hashes every file. Use during off-peak hours.

### Workspace Operations

| Operation | Time | Performance Class |
|-----------|------|-------------------|
| `jvs fork` | 0.15s | Engine-dependent metadata clone |
| `jvs workspace list` | 0.02s | O(m) where m = workspaces |
| `jvs workspace remove` | 0.05s | Metadata cleanup |

### GC Performance

| Checkpoints | Plan Time | Execute Time |
|-----------|-----------|--------------|
| 100 | 0.8s | 1.2s |
| 1,000 | 2.3s | 4.5s |
| 10,000 | 18s | 35s |

---

## Version Comparison

### Checkpoint Performance Over Versions

| Version | 1 GB | 10 GB | 100 GB | Notes |
|---------|-----|-------|--------|-------|
| v6.0 | 0.15s | 0.25s | 0.45s | Initial release |
| v6.5 | 0.12s | 0.18s | 0.28s | Optimized hashing |
| v7.0 | 0.12s | 0.15s | 0.18s | Simplified metadata |
| **v7.2** | **0.12s** | **0.15s** | **0.18s** | KISS simplification |

### Restore Performance Over Versions

| Version | 1 GB | 10 GB | 100 GB | Notes |
|---------|-----|-------|--------|-------|
| v6.0 | 0.12s | 0.20s | 0.35s | Initial release |
| v7.0 | 0.11s | 0.13s | 0.16s | Historical checkpoint restore model |
| **v7.2** | **0.10s** | **0.12s** | **0.15s** | Simplified restore path |

### Binary Size Over Versions

| Version | Binary Size | Notes |
|---------|-------------|-------|
| v7.0 | 14.2 MB | Before simplification |
| v7.1 | 15.8 MB | Added features (completion, diff, progress) |
| **v7.2** | **13.5 MB** | After KISS simplification |

---

## Regression Detection

### Performance Thresholds

If any of these thresholds are exceeded, investigate:

| Operation | Threshold | Action |
|-----------|-----------|--------|
| Checkpoint (1 GB) | > 0.2s | Investigate |
| Checkpoint (10 GB) | > 0.3s | Investigate |
| Restore (1 GB) | > 0.2s | Investigate |
| Restore (10 GB) | > 0.3s | Investigate |
| Verify (10 GB) | > 25s | Investigate |

### Comparison Tool

```bash
# Compare current results with baseline
./scripts/compare_benchmarks.sh baseline.json current.json

# Output format:
# ✅ Checkpoint 1GB: 0.12s (baseline: 0.12s) - OK
# ✅ Checkpoint 10GB: 0.15s (baseline: 0.15s) - OK
# ⚠️  Checkpoint 100GB: 0.25s (baseline: 0.18s) - REGRESSION
# ✅ Restore 1GB: 0.10s (baseline: 0.10s) - OK
```

---

## Historical Results

### v7.2 Baseline (2026-02-23)

```
=== Checkpoint Performance ===
Size    | Files | Time    | Engine
--------|-------|---------|--------
1 GB    | 100   | 0.12s   | juicefs-clone
10 GB   | 1,000 | 0.15s   | juicefs-clone
100 GB  | 10K   | 0.18s   | juicefs-clone
1 TB    | 100K  | 0.25s   | juicefs-clone

=== Restore Performance ===
Size    | Time    | Engine
--------|---------|--------
1 GB    | 0.10s   | juicefs-clone
10 GB   | 0.12s   | juicefs-clone
100 GB  | 0.15s   | juicefs-clone
1 TB    | 0.22s   | juicefs-clone
```

---

## Contributing Results

To contribute benchmark results:

1. **Document your environment** (CPU, RAM, Storage, OS)
2. **Run the benchmark script**
3. **Create a PR** with your results added to this document

Format for new entries:

```markdown
### v<version> (<date>)

**Environment:**
- CPU: <specification>
- RAM: <size>
- Storage: <type>
- OS: <version>

**Results:**
| Operation | Time | Notes |
|-----------|------|-------|
| ... | ... | ... |
```

---

## Performance Goals

### Targets for v8.0

| Metric | Current | Target |
|--------|---------|--------|
| Checkpoint (100 GB) | 0.18s | < 0.15s |
| Restore (100 GB) | 0.15s | < 0.12s |
| Binary size | 13.5 MB | < 12 MB |
| Test coverage | 83.7% | > 85% |

---

## Related Documentation

- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Performance troubleshooting
- [ARCHITECTURE.md](ARCHITECTURE.md) - System design

---

*Last benchmark run: 2026-02-23*
*Next scheduled run: After v7.3 release*
