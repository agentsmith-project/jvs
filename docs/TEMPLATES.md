# JVS Templates: `.jvsignore` Patterns

**Version:** v7.0
**Last Updated:** 2026-02-23
**Status:** Archived, non-release-facing

> Archived note: this historical template proposal is not part of the v0 public contract.
> v0 does not publish an ignore-file or partial checkpoint contract, so commands
> and examples below are retained only as non-current design context.

---

## Overview

This document provides `.jvsignore` templates for common scenarios. The `.jvsignore` file works like `.gitignore` - it specifies which paths should be excluded from snapshots.

**Location:** Place `.jvsignore` in your repository root (next to `.jvs/` directory).

---

## How `.jvsignore` Works

- **Patterns** follow `.gitignore` syntax
- **Lines starting with `#`** are comments
- **Blank lines** are ignored
- **`*`** matches any characters
- **`**`** matches any directories

**Example:**
```
# Exclude all .log files
*.log

# Exclude temp directory
temp/

# But include important.log even though *.log is excluded
!important.log
```

---

## Unity Templates

### Unity Complete Template

```gitignore
# Unity generated files
Library/
Temp/
obj/
*.userprefs
*.csproj
*.sln
*.suo

# Build outputs
Build/
[Oo]ut/[Pp]lay/
[Bb]uilds/
[Ll]ogs/

# Cache
*.cache
*.unityproj

# IDE files
.vscode/
.idea/
*.swp
*.swo
.vs/

# OS files
.DS_Store
Thumbs.db
desktop.ini
```

### Unity Minimal Template (Assets Only)

```gitignore
# Only version Assets/ and ProjectSettings/
Library/
Temp/
obj/
*.userprefs

# Everything else gets snapshotted
```

**Usage:** Use this if you want to snapshot everything except Unity's generated files.

---

## Unreal Templates

### Unreal Complete Template

```gitignore
# Unreal generated files
Binaries/
Build/
Intermediate/
Saved/
DerivedDataCache/
*.Openspeed
*.log

# Build artifacts
*.dll
*.exe
*.app

# IDE files
.vscode/
.idea/
.vs/
*.suo
*.user
*.sln
*.vcxproj

# OS files
.DS_Store
Thumbs.db
```

### Unreal Content-Only Template

```gitignore
# Only version Content/ and Config/
Binaries/
Build/
Intermediate/
Saved/
DerivedDataCache/

# Exclude compiled code but keep scripts
Scripts/*.dll
Scripts/*.so
```

---

## Python / ML Templates

### Python Project Template

```gitignore
# Byte-compiled / optimized / DLL files
__pycache__/
*.py[cod]
*$py.class

# C extensions
*.so

# Distribution / packaging
dist/
*.egg-info/
.eggs/
*.egg

# PyInstaller
*.manifest
*.spec

# Unit test / coverage
htmlcov/
.tox/
.coverage
.coverage.*
.cache
nosetests.xml
coverage.xml
*.cover
.hypothesis/
.pytest_cache/

# Virtual environments
venv/
ENV/
env/
.venv/

# IDEs
.vscode/
.idea/
*.swp
*.swo
*.sublime-project
*.sublime-workspace

# Jupyter Notebook
.ipynb_checkpoints
*.ipynb

# OS
.DS_Store
Thumbs.db
```

### ML Project Template (with data)

```gitignore
# Python (include all Python patterns above)
__pycache__/
*.py[cod]
*.so
dist/
*.egg-info/
venv/
.env/

# ML specific
models/checkpoints/
*.pth
*.pkl
*.h5
*.pb

# Data (optional - exclude raw data if too large)
# raw/
# processed/

# Jupyter
.ipynb_checkpoints/

# Experiment outputs
runs/
wandb/
mlruns/

# OS
.DS_Store
```

**Note:** Comment out `raw/` and `processed/` if you want to snapshot your data with JVS.

---

## Agent Sandbox Templates

### Agent Environment Template

```gitignore
# Python virtual environments
venv/
ENV/
env/
.venv/

# Agent outputs
runs/
outputs/
logs/
*.log

# Checkpoints (snapshots handle versioning)
checkpoints/
*.pth
*.pkl

# Temporary files
tmp/
temp/
*.tmp

# IDE
.vscode/
.idea/

# OS
.DS_Store
```

### Multi-Agent Template

```gitignore
# Per-agent temp directories
agents/*/tmp/
agents/*/logs/
agents/*/checkpoints/

# Shared cache
cache/
.cache/

# Communication
ipc/
sockets/
*.sock
```

---

## ETL Pipeline Templates

### Data Pipeline Template

```gitignore
# Processing temp
tmp/
temp/
*.tmp

# Logs
logs/
*.log
*.log.*

# Checkpoints
checkpoints/
*.checkpoint

# Query results (optional)
# results/

# Warehouse connections
connections/*.secret

# OS
.DS_Store
Thumbs.db
```

### Minimal ETL Template (Snapshot Data)

```gitignore
# Only exclude processing artifacts
logs/
tmp/
*.log

# Everything else (raw/, processed/, features/) gets snapshotted
```

---

## Go Templates

### Go Project Template

```gitignore
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
bin/
dist/

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool, specifically when used with LiteIDE
*.out

# Dependency directories
vendor/

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db
```

---

## Rust Templates

### Rust Project Template

```gitignore
# Rust
target/
**/*.rs.bk
*.pdb
Cargo.lock

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
```

---

## Web Development Templates

### Node.js Template

```gitignore
# Dependencies
node_modules/
jspm_packages/

# Build output
dist/
build/
*.tgz

# Logs
logs/
*.log
npm-debug.log*
yarn-debug.log*
yarn-error.log*

# Runtime data
pids/
*.pid
*.seed
*.pid.lock

# Coverage
lib-cov/
coverage/
*.lcov
.nyc_output/

# Environment
.env
.env.local
.env.*.local

# IDE
.vscode/
.idea/
*.swp
*.swo
.DS_Store
```

---

## General Development Templates

### All-Purpose Template

```gitignore
# Compiled files
*.o
*.a
*.so
*.dylib
*.dll
*.exe

# Logs
*.log
logs/

# Temp files
tmp/
temp/
*.tmp
*.temp
*.swp
*.swo
*~

# OS files
.DS_Store
.DS_Store?
._*
.Spotlight-V100
.Trashes
ehthumbs.db
Thumbs.db
desktop.ini

# IDE
.vscode/
.idea/
*.sublime-project
*.sublime-workspace
*.code-workspace

# Build artifacts
build/
dist/
out/
target/
bin/
```

---

## Best Practices

### 1. Keep `.jvsignore` in Version Control

Just like `.gitignore`, `.jvsignore` should be tracked:

```bash
# Add .jvsignore to your repo
git add .jvsignore
git commit -m "Add .jvsignore template"
```

### 2. Document Your Exclusions

Add comments to explain why things are excluded:

```gitignore
# Exclude large downloaded datasets (use separate data repo)
data/large/

# Exclude model checkpoints (use JVS snapshots instead)
checkpoints/

# Exclude IDE settings (team-specific)
.vscode/
```

### 3. Use Team Conventions

Establish `.jvsignore` conventions for your team:

```bash
# Create team template
cat > .jvsignore << 'EOF'
# Company ETL Standard Template v1.0
# Last updated: 2024-02-23

# Standard exclusions
logs/
tmp/
*.log

# Add project-specific below
EOF
```

### 4. Verify Your `.jvsignore`

Test that your exclusions work:

```bash
# See what would be snapshotted
jvs snapshot --dry-run "Test"

# Or use inspect to see snapshot contents
jvs inspect <snapshot-id>
```

---

## Common Patterns

### Exclude by Extension

```gitignore
# Exclude all .log files
*.log

# Exclude all compiled binaries
*.exe
*.dll
*.so
```

### Exclude Directories

```gitignore
# Exclude entire directory trees
__pycache__/
node_modules/
target/
venv/
```

### Negation (Include Exception)

```gitignore
# Exclude all .log files
*.log

# But keep important.log
!important.log
```

### Wildcard Patterns

```gitignore
# Exclude all temp files anywhere
*~
*.tmp
*.bak

# Exclude all .DS_Store files in any directory
.DS_Store
```

---

## Example: Complete Project Setup

```bash
# Initialize JVS repo
cd /mnt/juicefs/myproject
jvs init myproject
cd myproject/main

# Copy .jvsignore template
cp ../templates/python-ml.jvsignore .jvsignore

# Edit for project specifics
vim .jvsignore

# Create initial snapshot
jvs snapshot "Initial project with .jvsignore" --tag baseline
```

---

## Troubleshooting

### Problem: File still being snapshotted

**Solution:** Check pattern syntax
```bash
# Debug .jvsignore
cat .jvsignore

# Test pattern
echo "test.log" | git check-ignore -v .jvsignore
```

### Problem: Want to snapshot ignored file

**Solution:** Use negation or force flag
```bash
# Method 1: Negation in .jvsignore
!important.log

# Method 2: Snapshot specific path
jvs snapshot "Update" --paths important.log
```

---

## Next Steps

- Read [GAME_DEV_QUICKSTART.md](game_dev_quickstart.md) for game workflows
- Read [AGENT_SANDBOX_QUICKSTART.md](agent_sandbox_quickstart.md) for agent workflows
- Read [ETL_PIPELINE_QUICKSTART.md](etl_pipeline_quickstart.md) for data workflows
- Contribute your template: Open a PR on GitHub

---

*Contributions welcome! If you have a `.jvsignore` template for a scenario not covered here, please submit a PR.*
