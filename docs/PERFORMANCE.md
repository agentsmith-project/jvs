# JVS Performance Tuning Guide

**Version:** v0 public contract
**Last Updated:** 2026-04-25

---

## Overview

JVS performance depends on several factors: storage engine, filesystem choices, hardware, and configuration. This guide helps you optimize JVS for your workload.

---

## Performance Characteristics

### Checkpoint Performance by Engine

| Engine | Performance Class | Use Case | Bottleneck |
|--------|-------------------|----------|------------|
| **juicefs-clone** | Constant-time metadata clone when supported | JuiceFS mount | Metadata service and mount health |
| **reflink-copy** | Linear tree walk with CoW data sharing per file | CoW filesystems (btrfs, XFS) | File count and metadata I/O |
| **copy** | Linear data copy | Fallback | Disk and network I/O |

### Verify Performance

| Operation | Complexity | Bottleneck |
|-----------|------------|------------|
| `jvs verify` | O(n) | Disk I/O, SHA-256 computation |
| `jvs gc plan` | O(m) | Descriptor reads (m = checkpoints) |
| `jvs restore` | Engine-dependent | Constant-time metadata clone with supported `juicefs-clone`; linear fallback for copy |

---

## Engine Selection

### Recommendation: Use juicefs-clone

**Best for:** Production workloads with large datasets

```bash
# Verify juicefs-clone is available
jvs capability /path/to/repo-parent --write-probe --json

# Prefer an engine for future materialization
export JVS_SNAPSHOT_ENGINE=juicefs-clone
jvs init myrepo
```

**When juicefs-clone is not available:**
1. **Use reflink-copy** on btrfs/XFS for CoW data sharing without JuiceFS
2. **Use copy** as fallback for one-time operations

### reflink-copy Engine

**Best for:** Local SSDs without JuiceFS

```bash
# Check if filesystem supports reflink
jvs capability /path/to/repo-parent --write-probe --json
```

**Requirements:**
- btrfs with CoW enabled
- XFS with reflink enabled
- ZFS with clone support

### copy Engine

**Use for:**
- Testing
- Small workspaces (< 1GB)
- One-time migrations
- Fallback when other engines unavailable

---

## Filesystem Recommendations

### JuiceFS (Recommended)

**Optimization tips:**

1. **Use SSD cache** for JuiceFS metadata:
   ```bash
   juicefs mount redis://... /mnt/jfs \
     --cache-dir /var/lib/juicefs/cache \
     --free-space-ratio 0.1
   ```

2. **Adjust block size** for large files:
   ```bash
   juicefs format ... --block-size 4MiB
   ```

3. **Enable compression:**
   ```bash
   juicefs mount ... --compress lzo
   ```

### btrfs

**Optimization tips:**

1. **Enable CoW:** `chattr +C +T /path/to/dir`
2. **Disable cow** for databases: `chattr +C /var/lib/mysql`
3. **Schedule defrag:** `btrfs filesystem defrag -r /path`

### XFS

**Optimization tips:**

1. **Use reflink=1** mount option
2. **Large inode size:** `mkfs.xfs -i size=512`
3. **Noatime mount option** to reduce metadata

### ZFS

**Optimization tips:**

1. **Enable compression:** `compression=lz4`
2. **Atime:** `atime=off` or `relatime`
3. **Record size:** `recordsize=1M` for large files

---

## Hardware Recommendations

### Storage

**Minimum requirements:**
- **IOPS:** 1000+ for acceptable performance
- **Throughput:** 500 MB/s sequential
- **Latency:** < 10ms average

**Recommended:**
- **NVMe SSD** for best performance
- **SATA SSD** for good cost/performance
- **HDD** only for archival/backup

### Network (for JuiceFS)

**For local network:**
- **10 GbE** or better
- **Low latency** (< 1ms)

**For cloud:**
- **Same region** as Redis/object storage
- **VPC/peering** for connectivity

### RAM

**Minimum:** 4 GB
**Recommended:** 8-16 GB for concurrent operations

JVS itself uses minimal RAM, but Go runtime benefits from more memory.

---

## Configuration Tuning

### Engine Selection

**Auto-detection (default):**
```yaml
# .jvs/config.yaml
default_engine: auto  # Tries juicefs-clone, reflink-copy, copy
```

**Force specific engine:**
```yaml
default_engine: juicefs-clone  # Prefer JuiceFS; command JSON reports fallback
```

### Logging

**Reduce logging overhead:**
```yaml
logging:
  level: warn  # Only log warnings and errors
  format: text  # Faster than JSON
```

### Progress Reporting

**Disable for scripts:**
```bash
jvs --no-progress checkpoint "Automated checkpoint"
```

---

## JuiceFS-Specific Tuning

### Client Configuration

**Optimal for JVS:**
```bash
juicefs mount redis://... /mnt/jfs \
  --cache-size 100GiB \
  --max-cached-inodes 10000000 \
  --buffer-size 10MiB \
  --upload-limit 10MiB
```

### Backend Configuration

**Redis backend:**
```
maxmemory 16gb
maxmemory-policy allkeys-lru
```

**S3 backend:**
- Use same region as compute
- Enable S3 Transfer Acceleration

---

## Operating System Tuning

### Filesystem Cache

**Increase page cache (Linux):**
```bash
# Add to /etc/sysctl.conf
vm.vfs_cache_pressure=50
vm.dirty_ratio=15
vm.dirty_background_ratio=5
```

### File Descriptors

**Increase limits (Linux):**
```bash
# Add to /etc/security/limits.conf
* soft nofile 65536
* hard nofile 65536
```

### I/O Scheduler

**For SSDs:**
```bash
# Use deadline or noop scheduler
echo deadline | sudo tee /sys/block/sdX/queue/scheduler
```

**For HDDs:**
```bash
# Use cfq or deadline
echo cfq | sudo tee /sys/block/sdX/queue/scheduler
```

---

## Common Bottlenecks

### Issue: Slow first checkpoint

**Cause:** Computing initial payload hashes for all files

**Solutions:**
- Use JuiceFS with metadata cache
- Pre-warm cache: `find . -type f -exec cat {} \; > /dev/null`
- Accept a slower first checkpoint for long-term benefit

### Issue: Slow verify

**Cause:** Hashing every file in workspace

**Solutions:**
- Verify specific checkpoints instead of `--all`
- Use `jvs verify <checkpoint-id>` for focused diagnosis when you do not need a
  full repository sweep
- Run during off-peak hours

### Issue: Slow GC plan

**Cause:** Reading many descriptor files

**Solutions:**
- Keep descriptor count manageable (GC regularly)
- Run `jvs gc plan` and `jvs gc run --plan-id <plan-id>` after old checkpoints
  are no longer referenced by active workspaces

---

## Benchmarking

### Measuring Performance

**Checkpoint performance:**
```bash
time jvs checkpoint "Benchmark test"
```

**Restore performance:**
```bash
time jvs restore <checkpoint-id>
```

**Verify performance:**
```bash
time jvs verify --all
```

**Identify bottlenecks:**
```bash
# Record command wall time and health signal
time jvs verify --all
jvs doctor --json | jq '.data.healthy'
```

### Regression Baseline Matrix

Use this matrix to record local baselines for your own storage stack. Treat
every number as environment-specific regression data, not a portable latency
promise.

| Workload axis | Engine scope | Baseline signal |
|---------------|--------------|-----------------|
| Small to large workspace payloads | `juicefs-clone` on supported JuiceFS | Checkpoint and restore should track metadata-service health more than payload bytes |
| Small to large workspace payloads | `reflink-copy` on CoW-capable filesystems | File-count and metadata traversal usually dominate, with file data shared by the filesystem |
| Small to large workspace payloads | `copy` fallback | Payload bytes, disk throughput, and network I/O dominate |
| Verification runs | Any engine | Payload hash cost scales with file count and bytes read |

Store measured values with the benchmark command, engine, filesystem, hardware,
cache state, and workload shape so future runs can detect regressions against
the same environment.

---

## Optimization Checklist

### Before Deploying to Production

- [ ] Use JuiceFS with juicefs-clone engine
- [ ] Run on SSD or fast network storage
- [ ] Schedule GC during off-peak hours
- [ ] Test with actual workload size
- [ ] Run `jvs doctor --strict` to verify health
- [ ] Verify IOPS and throughput meet requirements
- [ ] Enable JuiceFS client caching
- [ ] Configure appropriate logging level

### Monitoring

**Key metrics to monitor:**
- Checkpoint creation time
- Restore time
- Verify time
- Disk usage (`.jvs/` and workspace)
- CPU usage during hash computation
- Network bandwidth (for JuiceFS)

---

## Scaling Considerations

### Checkpoint Count

**Operational starting point:** Keep active checkpoint count below 10,000 per
repository until local validation shows comfortable planning, listing, and GC
margins for your storage stack.

**More checkpoints?**
- Use tags to mark important checkpoints
- Run two-phase GC cleanup more frequently
- Consider splitting into multiple repositories
- Record local `jvs gc plan`, list, and restore baselines before treating a
  larger count as routine.

### Concurrent Operations

**JVS v0:** Single writer model

**For concurrent access:**
- Use external coordination (locks, queues)
- Separate repositories per team/member
- Single entry point for operations

### Workspace Size

**Operational starting point:** Validate workspaces up to 10 TB with your
actual filesystem, engine, network, and verification cadence before relying on
that size in production. This is a starting point for local validation, not a
portable product limit or latency commitment.

**Larger workspaces:**
- Ensure sufficient IOPS
- Monitor hash computation time
- Split independent payloads into separate repositories when needed; v0 does
  not expose partial checkpoint contracts.

---

## Troubleshooting Performance

### Issue: Checkpoints are slow

**Diagnostics:**
```bash
# Check which engine is being used
jvs info --json | jq '.data.engine'

# Check I/O
iostat -x 1 5

# Check JuiceFS cache
juicefs stats /mnt/jfs
```

**Solutions:**
- Switch to juicefs-clone engine
- Enable JuiceFS caching
- Check disk health

### Issue: Restores are slow

**Diagnostics:**
```bash
# Check if restore is using copy engine
jvs info --json | jq '.data.engine'
```

**Solutions:**
- Ensure juicefs-clone is being used
- Check network bandwidth (for JuiceFS)

### Issue: Verify is slow

**This is expected** - verify reads and hashes every file. Optimization:

1. Run full verification during a maintenance window:
   ```bash
   jvs verify --all
   ```

2. Run during off-peak hours

---

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - System design
- [FAQ.md](FAQ.md) - Common questions
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Performance issues

---

*For specific performance issues, please open a GitHub Issue with diagnostic information.*
