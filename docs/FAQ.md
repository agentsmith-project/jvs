# JVS FAQ (Frequently Asked Questions)

**Version:** v7.0
**Last Updated:** 2026-02-23

---

## General Questions

### What is JVS?

**JVS** (Juicy Versioned Workspaces) is a **workspace versioning system** built on JuiceFS. It captures entire workspace states as checkpoints, providing O(1) version control for data-intensive workloads.

**Key characteristics:**
- Checkpoint-first (not diff-first like Git)
- Filesystem-native (no virtualization)
- Local-first (no remote protocol)
- O(1) checkpoints via JuiceFS Copy-on-Write

---

### How is JVS different from Git?

| Aspect | Git | JVS |
|--------|-----|-----|
| **Unit of versioning** | Files/diffs | Entire workspace |
| **Storage model** | Blob store + refs | Checkpoints + descriptors |
| **Performance** | Slower with large files | O(1) regardless of size |
| **Use case** | Source code | Workspaces with data |
| **Merge** | Text-based 3-way merge | No merge (fork instead) |
| **Remote** | Push/pull to remotes | JuiceFS handles transport |

**Think of it this way:** Git is for code, JVS is for complete workspace states.

---

### Why not just use Git?

Git excels at text-based version control, but struggles with:
- **Large datasets** (ML models, scientific data)
- **Binary files** (storing full copies)
- **Workspace reproducibility** (Git submodules are complex)
- **O(1) checkpoints** (Git requires significant I/O for large files)

JVS handles these use cases natively.

---

### When should I use JVS?

**Use JVS when:**
- You have large datasets (10GB+ workspaces)
- You need O(1) checkpoint/restore
- You work with ML experiments that need exact reproduction
- You use JuiceFS for storage
- You want simple workspace versioning without Git complexity

**Use Git when:**
- You're versioning source code
- You need text-based merge
- You have distributed contributors
- You want GitHub integration

---

### Can JVS replace Git?

**No.** JVS is designed for **workspace versioning**, not source code control. Many teams use both:
- **Git** for code repositories
- **JVS** for runtime environments, data, and models

---

## Installation & Setup

### What are the prerequisites?

**Required:**
- Go 1.25+ (for building from source)
- A filesystem (JuiceFS recommended, any POSIX FS works)

**Optional but recommended:**
- JuiceFS mount (for O(1) checkpoints)
- CoW-capable filesystem (btrfs, XFS) for reflink engine

---

### How do I install JVS?

```bash
# Build from source
git clone https://github.com/jvs-project/jvs.git
cd jvs
make build

# Or using Go
go install github.com/jvs-project/jvs@latest

# Verify
jvs --version
```

---

### Does JVS work without JuiceFS?

**Yes!** JVS works on any POSIX filesystem:
- **Without JuiceFS:** Uses copy engine (O(n) but functional)
- **With JuiceFS:** Uses juicefs-clone engine (O(1), recommended)

---

## Concepts

### What is a checkpoint?

A checkpoint is a **complete capture of your workspace state** at a point in time. It includes:
- All files in your workspace
- Metadata describing the checkpoint (note, tags, timestamps)
- Integrity information (checksums and hashes)

Checkpoints are **immutable** - once created, they never change.

---

### What is a workspace?

A workspace is a **real directory** containing your workspace files. JVS manages multiple workspaces, each pointing to different checkpoints.

- `repo/main/` - The primary workspace
- Use `jvs workspace path <name>` to find an additional workspace directory

**Note:** The repository root (`repo/`) is NOT a workspace - `main/` is.

---

### What is current differs from latest?

When you restore to a historical checkpoint (not the latest), your workspace enters **current differs from latest**. This means:
- You can view/use the checkpoint state
- You cannot create new checkpoints (would break lineage)
- Use `jvs fork` to create a new branch from this state

---

### What does O(1) checkpoint mean?

With JuiceFS Copy-on-Write, creating a checkpoint is **constant time** - it doesn't matter if your workspace is 1GB or 1TB. The checkpoint is a metadata reference, not a copy.

**Without JuiceFS:** Checkpoints take O(n) time proportional to workspace size.

---

## Usage

### How do I create a checkpoint?

```bash
cd myrepo/main
jvs checkpoint "Work in progress"
```

Add tags for organization:
```bash
jvs checkpoint "v1.0 release" --tag release --tag v1.0 --tag stable
```

---

### How do I restore a checkpoint?

```bash
# By checkpoint ID (full or prefix)
jvs restore abc123

# By tag
jvs restore --latest-tag stable

# Back to latest
jvs restore latest
```

---

### How do I see history?

```bash
jvs checkpoint list

# Filter by tag
jvs checkpoint list | grep stable

# Show all checkpoints across all workspaces
jvs checkpoint list --all
```

---

### How do I create a branch?

```bash
# Fork from current state
jvs fork feature-branch

# Fork from a specific checkpoint
jvs fork abc123 feature-branch
```

Use `jvs workspace path feature-branch` to print the directory you can `cd` into.

---

### Can I create partial checkpoints?

Not as part of the v0 stable public CLI. A checkpoint records the current
workspace tree. Keep generated caches or temporary files outside the workspace
when you do not want them in checkpoints.

---

### Does v0 expose checkpoint compression flags?

No. Compression is not a v0 stable public CLI contract. Use the filesystem or
storage layer for compression decisions, and rely on `jvs capability`,
`jvs info`, and `jvs doctor` for engine visibility.

---

### How do I configure JVS defaults?

JVS supports configuration via `.jvs/config.yaml` for repository-specific settings. This allows you to set defaults like:

- **Engine visibility** - Check the selected engine with `jvs info`
- **Default tags** - Automatically add tags to every checkpoint
- **Output format** - Always use JSON output if preferred
- **Progress bars** - Enable/disable progress bar display

**Show current configuration:**
```bash
jvs config show
```

**Set a configuration value:**
```bash
# Set default engine
jvs config set default_engine juicefs-clone

# Set default tags (YAML list format)
jvs config set default_tags '["dev", "experimental"]'

# Set JSON output by default
jvs config set output_format json

# Disable progress bars
jvs config set progress_enabled false
```

**Get a single value:**
```bash
jvs config get default_engine
jvs config get default_tags
```

**Example config.yaml:**
```yaml
# .jvs/config.yaml
default_engine: juicefs-clone
default_tags:
  - auto
  - v1.0
output_format: text
progress_enabled: true
```

**Important notes:**
- Config is per-repository (stored in `.jvs/config.yaml`)
- Command-line flags override config values
- Default tags are combined with tags specified via `--tag`
- If the config file doesn't exist, JVS uses sensible defaults

---

## Common Misconceptions

### Misconception: "JVS is a Git replacement"

**Reality:** JVS complements Git. It's for workspace versioning, not source code. Many teams use both together.

---

### Misconception: "JVS requires JuiceFS"

**Reality:** JVS works on any filesystem. JuiceFS is recommended for O(1) performance but not required.

---

### Misconception: "JVS does distributed version control"

**Reality:** JVS is local-first. It has no push/pull/remote protocol. JuiceFS handles data transport if you need it.

---

### Misconception: "JVS has merge conflicts"

**Reality:** JVS has no merge. You fork workspaces instead. Different model for different use case.

---

### Misconception: "JVS stores my data"

**Reality:** JVS stores only metadata (`.jvs/`). Your workspace data lives in your filesystem. JVS references it, it doesn't own it.

---

## Technical

### What happens if a checkpoint creation is interrupted?

JVS uses **intent records** to track in-progress checkpoints. If interrupted:
- Partial checkpoints are detectable (missing `.READY` file)
- Run `jvs doctor --strict` to find and clean up

---

### How does JVS verify integrity?

JVS uses **two-layer verification**:
1. **Descriptor checksum** - SHA-256 of the descriptor JSON
2. **Payload root hash** - SHA-256 of all files in the workspace

Both must pass for verification to succeed.

---

### Can JVS handle concurrent access?

JVS v7.0 is designed for **single-writer** scenarios. Concurrent access from multiple processes is not supported and may cause:
- Corrupted checkpoints
- Lost updates
- Audit inconsistencies

**For concurrent access:** Coordinate access externally (file locks, queue systems, or single-user workflows).

---

### What is the storage overhead?

JVS metadata is minimal:
- **Descriptors:** ~1-2KB per checkpoint
- **Checkpoints:** Small reference files (juicefs-clone references)
- **Audit log:** ~200 bytes per operation

Your actual workspace data is stored once - checkpoints are references, not copies.

---

## Troubleshooting

### JVS says "no suitable engine found"

**Solution:** Probe the target path with `jvs capability <path>` and move the
repo to a filesystem where JVS reports a supported engine. The portable copy
engine is selected automatically when it is the best available option.

---

### Restore says "workspace is in current differs from latest"

**Solution:** This is normal for historical checkpoints. Return to latest:

```bash
jvs restore latest
```

Or create a fork to continue work:

```bash
jvs fork new-branch
```

---

### Doctor reports "partial checkpoint detected"

**Solution:** A previous checkpoint was interrupted. Clean up:

```bash
jvs doctor --strict --repair-runtime
```

---

## Performance

### How can I speed up checkpoints?

1. **Use JuiceFS** - Enables O(1) juicefs-clone engine
2. **Use fast storage** - NVMe SSD, optimized storage
3. **Reduce metadata** - Fewer files = faster hashing
4. **Skip payload hash** - Use `--no-payload` for testing (not recommended for production)

---

### Why is my first checkpoint slow?

The first checkpoint needs to:
- Create initial metadata structures
- Compute payload hashes (I/O intensive)

Subsequent checkpoints are much faster (incremental hashing).

---

### How much disk space does JVS use?

JVS itself uses very little space (metadata only). Your workspace data storage depends on your filesystem (JuiceFS, NFS, local disk, etc.).

With JuiceFS CoW:
- Checkpoints: Minimal overhead (reference, not copy)
- Descriptors: ~1KB each

---

## Comparison

### JVS vs DVC

| Aspect | JVS | DVC |
|--------|-----|-----|
| **Storage backend** | Any filesystem | Multiple backends (S3, GCS, etc.) |
| **Architecture** | Filesystem-native | Cache + remote |
| **Model tracking** | No (use Git/MLEM) | Yes (built-in) |
| **Checkpoint speed** | O(1) with JuiceFS | O(n) |
| **Setup complexity** | Low | Medium |

---

### JVS vs Git LFS

| Aspect | JVS | Git LFS |
|--------|-----|---------|
| **Versioning unit** | Entire workspace | Files (large files stored separately) |
| **Workflow** | Checkpoint restore | Git checkout |
| **O(1) operations** | Yes (with JuiceFS) | No |
| **Learning curve** | Simple | Moderate |

---

## Adoption

### Is JVS production-ready?

**Yes.** JVS v7.0 is used in production for:
- ML experiment tracking
- CI/CD environment versioning
- Agent workflow sandboxes

Key production features:
- Strong integrity verification
- Tamper-evident audit trail
- Health checks (`jvs doctor`)
- Garbage collection with retention policies

---

### Who uses JVS?

Target users include:
- **Data science teams** - Experiment versioning with large datasets
- **ML engineers** - Model and environment tracking
- **DevOps engineers** - CI/CD environment management
- **Platform engineers** - Multi-environment coordination
- **AI/ML agents** - Deterministic sandbox states

---

### How do I get started?

See the [Quick Start Guide](QUICKSTART.md) for a 5-minute tutorial.

**Basic workflow:**
```bash
# Initialize
jvs init myproject
cd myproject/main

# Checkpoint
jvs checkpoint "Initial state"

# Make changes
vim file.txt

# Checkpoint again
jvs checkpoint "Added feature"

# Restore if needed
jvs restore <checkpoint-id>
```

---

## Security

### Is JVS secure?

JVS provides **integrity protection** (not access control):
- Two-layer verification detects tampering
- Audit trail provides tamper evidence
- Access control delegated to OS/filesystem permissions

**Security model:** See [SECURITY.md](../SECURITY.md) for details.

---

### Can JVS prevent data loss?

JVS provides several safeguards:
- **Verification:** Detects corruption via checksums
- **Audit trail:** Tamper-evident operation history
- **Garbage collection:** With plan-preview, review before deletion

**Backup strategy:** Use JuiceFS sync (excluding `.jvs/intents`) for backup.

---

## License

### What license does JVS use?

**MIT License** - See [LICENSE](../LICENSE) for details.

This means:
- ✅ Free to use in personal and commercial projects
- ✅ Free to modify and distribute
- ✅ No attribution requirement (though appreciated!)

---

## Contributing

### How can I contribute?

See [CONTRIBUTING.md](../CONTRIBUTING.md) for details.

**Quick start:**
1. Fork the repository
2. Create a branch
3. Make your changes
4. Run `make verify`
5. Submit a pull request

---

### What skills are needed?

- **Go programming** for core contributions
- **Technical writing** for documentation
- **Testing** for conformance tests
- **Design** for feature proposals

---

## Future

### What's the current roadmap?

See the [changelog](docs/99_CHANGELOG.md) for recent releases. Current focus areas:
- Integration with agentsmith platform
- Performance optimization for large workspaces

---

### Will JVS add merge support?

**Not planned.** Merge complexity doesn't align with JVS's checkpoint-first philosophy. Use `jvs fork` to create parallel work streams instead.

---

### Will JVS add remote protocol?

**Not planned.** JVS is local-first. Use your filesystem's tools (rsync, JuiceFS sync, NFS) for data transport.

---

## Getting Help

### Where can I learn more?

- **Documentation:** [docs/](../docs)
- **Quick Start:** [docs/QUICKSTART.md](QUICKSTART.md)
- **Examples:** [docs/EXAMPLES.md](EXAMPLES.md)
- **Architecture:** [docs/ARCHITECTURE.md](ARCHITECTURE.md)
- **Troubleshooting:** [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md)

### How do I report a bug?

See [CONTRIBUTING.md](../CONTRIBUTING.md) or [SECURITY.md](../SECURITY.md) for security issues.

### Where can I ask questions?

- **GitHub Issues:** [github.com/jvs-project/jvs/issues](https://github.com/jvs-project/jvs/issues)
- **GitHub Discussions:** [github.com/jvs-project/jvs/discussions](https://github.com/jvs-project/jvs/discussions)

---

*Have a question not covered here? Please open a GitHub Issue or Discussion!*
