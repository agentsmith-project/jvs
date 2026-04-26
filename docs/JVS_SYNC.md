# JVS Remote Sync Helper

**Status:** Archived, non-release-facing

> Archived note: this historical helper design is not part of the v0 public contract.
> Current release-facing migration guidance lives in
> [Migration & Backup](18_MIGRATION_AND_BACKUP.md). The helper commands below
> are retained only as non-current design context.

The `jvs-sync.sh` helper script enables convenient backup, restore, and migration of JVS repositories between machines and storage systems.

## Overview

JVS repositories store metadata in `.jvs/` and payload in worktree directories. The sync helper properly handles:

- **Portable metadata**: `format_version`, `worktrees/`, `snapshots/`, `descriptors/`, `audit/`, committed `gc/tombstones/`
- **Payload directories**: `main/` and `worktrees/*/`
- **Excluded items**: active `locks/`, `intents/`, and root `gc/*.json` runtime state, plus `index.sqlite` (rebuildable cache)

## Installation

The script is located at `scripts/jvs-sync.sh`. To use it system-wide:

```bash
# Install to /usr/local/bin
sudo cp scripts/jvs-sync.sh /usr/local/bin/jvs-sync
sudo chmod +x /usr/local/bin/jvs-sync

# Or add to PATH via symlink
sudo ln -s $(pwd)/scripts/jvs-sync.sh /usr/local/bin/jvs-sync
```

## Quick Start

### Backup to Remote Server

```bash
# Backup to remote server via SSH
jvs-sync backup /path/to/repo user@server:/backup/jvs

# Backup with custom exclude pattern
jvs-sync backup /path/to/repo user@server:/backup/jvs --exclude "*.tmp"

# Dry run to see what would be synced
jvs-sync backup -n /path/to/repo user@server:/backup/jvs
```

### Backup to Cloud Storage (via JuiceFS sync)

```bash
# Backup to S3
jvs-sync backup /path/to/repo s3://mybucket/jvs-backup

# Backup to Google Cloud Storage
jvs-sync backup /path/to/repo gs://mybucket/jvs-backup

# Backup to Azure Blob Storage
jvs-sync backup /path/to/repo az://mycontainer/jvs-backup

# Use more threads for faster sync
jvs-sync backup -j 20 /path/to/repo s3://mybucket/jvs-backup
```

### Restore from Backup

```bash
# Restore from remote server
jvs-sync restore user@server:/backup/jvs /path/to/repo

# Restore from cloud storage
jvs-sync restore s3://mybucket/jvs-backup /path/to/repo
```

### Mirror Between Locations

```bash
# Bidirectional sync (changes flow both ways)
jvs-sync mirror /path/to/repo /path/to/mirror

# Useful for keeping two machines in sync
jvs-sync mirror ~/projects/myrepo user@laptop:~/projects/myrepo
```

### Migrate to New Location

```bash
# Migrate repository to new storage
# Deletes files in dest that don't exist in source
jvs-sync migrate /old/location /new/location
```

### Verify Backup Integrity

```bash
# Check if backup matches source
jvs-sync verify /path/to/repo user@server:/backup/jvs
```

## Commands

| Command | Description | Direction | Deletes |
|---------|-------------|-----------|---------|
| `backup` | Backup to remote | Source → Dest | Dest extras only |
| `restore` | Restore from backup | Source → Dest | None |
| `mirror` | Bidirectional sync | Both directions | None |
| `migrate` | One-way migration | Source → Dest | Dest extras |
| `verify` | Check integrity | N/A | N/A |

## Options

| Option | Description |
|--------|-------------|
| `-n, --dry-run` | Show what would be done without making changes |
| `-v, --verbose` | Show detailed output |
| `-j, --threads N` | Number of concurrent threads (default: 10) |
| `-e, --exclude PATTERN` | Exclude pattern (can be repeated) |
| `--rsync-only` | Force use of rsync even if juicefs available |

## What Gets Synced

### Included (Portable State)

- `.jvs/format_version` - Repository format version
- `.jvs/worktrees/` - Worktree metadata
- `.jvs/snapshots/` - Snapshot data
- `.jvs/descriptors/` - Snapshot descriptors
- `.jvs/audit/` - Audit log events
- `.jvs/gc/tombstones/` - Committed GC tombstones
- `main/` - Main worktree payload
- `worktrees/*/` - Other worktree payloads

### Excluded (Runtime/Rebuildable)

- `.jvs/locks/` - Host-local mutation locks
- `.jvs/intents/` - In-flight snapshot operations
- `.jvs/gc/*.json` - Active GC plans
- `.jvs/index.sqlite` - Search index (can be rebuilt)

## Sync Methods

The script automatically selects the best sync method:

### JuiceFS Sync (Preferred for Cloud)

- **Used when**: Source or destination is an object storage URL (s3://, gs://, etc.)
- **Requirements**: `juicefs` command installed
- **Benefits**: Native cloud integration, optimized for large files

```bash
# S3 backup
jvs-sync backup /repo s3://bucket/path

# With custom endpoint
jvs-sync backup /repo s3://bucket.s3.amazonaws.com/path
```

### Rsync (Fallback)

- **Used when**: Local paths or SSH destinations
- **Requirements**: `rsync` installed
- **Benefits**: Universal availability, SSH integration

```bash
# Local backup
jvs-sync backup /repo /backup

# SSH backup
jvs-sync backup /repo user@server:/backup
```

## Examples

### Daily Backup Script

```bash
#!/bin/bash
# daily-backup.sh

REPO="/home/user/projects/myrepo"
BACKUP="user@backup-server:/backups/jvs"

echo "Starting daily backup..."
jvs-sync backup -v "$REPO" "$BACKUP"

if jvs-sync verify "$REPO" "$BACKUP"; then
    echo "Backup verified successfully"
else
    echo "Backup verification failed!" >&2
    exit 1
fi
```

### Disaster Recovery Setup

```bash
# Primary to backup (daily)
jvs-sync backup -j 20 /primary s3://disaster-recovery/jvs-primary

# Backup to DR site (hourly)
jvs-sync backup s3://disaster-recovery/jvs-primary user@dr-site:/primary
```

### Multi-Machine Workflow

```bash
# Work on desktop
cd ~/projects/myrepo

# Sync to laptop before leaving
jvs-sync mirror ~/projects/myrepo user@laptop:~/projects/myrepo

# On laptop, pull latest changes
jvs-sync mirror user@desktop:~/projects/myrepo ~/projects/myrepo
```

## Performance Tuning

### Thread Count

For high-bandwidth connections, increase threads:

```bash
# 1 Gbps network
jvs-sync backup -j 50 /repo s3://bucket/path

# 100 Mbps network (default)
jvs-sync backup -j 10 /repo s3://bucket/path
```

### Bandwidth Limiting

With rsync, use `--bwlimit` (edit script or use rsync directly):

```bash
# Limit to 100 Mbps
rsync -a --bwlimit=100 /repo/ user@server:/backup/
```

## Security Considerations

### SSH Keys

For SSH-based sync, use key-based authentication:

```bash
# Generate SSH key if needed
ssh-keygen -t ed25519

# Copy to remote server
ssh-copy-id user@server

# Now sync works without password
jvs-sync backup /repo user@server:/backup
```

### Cloud Credentials

For object storage, configure credentials:

```bash
# AWS S3
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"

# Google Cloud Storage
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/creds.json"

# Then sync
jvs-sync backup /repo s3://bucket/path
```

## Troubleshooting

### "Not a JVS repository" Error

```bash
# Ensure you're pointing to the repository root
jvs-sync backup /path/to/repo user@server:/backup
#             ^^^^^^^^^^^^^^^ should contain .jvs/ directory
```

### Permission Denied

```bash
# Ensure you have read access to source
ls -la /path/to/repo/.jvs

# And write access to destination
ssh user@server "ls -la /backup"
```

### Slow Sync Performance

```bash
# Increase thread count for cloud storage
jvs-sync backup -j 50 /repo s3://bucket/path

# Use rsync-only for local networks
jvs-sync backup --rsync-only /repo /local/backup
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Backup JVS Repository

on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM

jobs:
  backup:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install JuiceFS
        run: |
          curl -sSL https://d.juicefs.com/install | sh -

      - name: Backup to S3
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
        run: |
          ./scripts/jvs-sync.sh backup -v \
            . s3://${{ secrets.BACKUP_BUCKET }}/jvs-backup
```

## See Also

- [Repository Layout Spec](01_REPO_LAYOUT_SPEC.md) - Details on `.jvs/` structure
- [JuiceFS Sync Documentation](https://juicefs.com/docs/community/administration/sync)
- [Rsync Manual](https://linux.die.net/man/1/rsync)
