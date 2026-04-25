# JVS Quick Start: Game Development

**Version:** v7.0
**Last Updated:** 2026-02-23

---

## Overview

This guide helps game developers use JVS for versioning large game assets that Git cannot handle efficiently. JVS complements your existing version control workflow by providing O(1) checkpoints for binary assets.

---

## Why JVS for Game Development?

| Problem | Git + Git LFS | JVS |
|---------|---------------|-----|
| 5GB texture files | Slow clone, bandwidth costs | O(1) checkpoint, instant restore |
| Repository size | Blobs grow endlessly | Checkpoints are references |
| Asset history | LFS pointer complexity | Simple checkpoint/restore |
| Team collaboration | Merge conflicts on binaries | Fork workspaces instead |

**Key Benefit:** Checkpoint your entire `Assets/` folder in seconds, regardless of size.

---

## Prerequisites

1. **JuiceFS mounted** (recommended for O(1) performance)
   ```bash
   # Check if JuiceFS is mounted
   mount | grep juicefs
   ```

2. **JVS installed**
   ```bash
   jvs --version
   ```

3. **Game project** (Unity or Unreal)

---

## Quick Start (5 Minutes)

### Step 1: Initialize JVS Repository

```bash
# Navigate to your JuiceFS mount
cd /mnt/juicefs/game-projects

# Initialize JVS repository
jvs init mygame
cd mygame/main
```

**Structure created:**
```
/mnt/juicefs/game-projects/mygame/
├── .jvs/           # JVS metadata (never checkpoint this)
└── main/           # Your workspace (this is where you work)
```

### Step 2: Import Your Game Project

```bash
# Copy Unity project (Assets/ and ProjectSettings/ only)
cp -r ~/UnityProjects/MyGame/Assets/* .
cp -r ~/UnityProjects/MyGame/ProjectSettings/* .

# Create initial checkpoint
jvs checkpoint "Initial Unity project import" --tag unity --tag baseline
```

**What just happened:**
- JVS created a checkpoint of your entire workspace
- The checkpoint is a reference (O(1) operation), not a copy
- Tags help you find this checkpoint later

### Step 3: Create Your First Asset Version

```bash
# Before working on an asset, create a checkpoint
jvs checkpoint "Before character model work" --tag prework

# ... work in Unity/Unreal ...

# After finishing, checkpoint the new version
jvs checkpoint "Character model v2: added armor details" --tag character --tag v2
```

### Step 4: Restore if Something Goes Wrong

```bash
# Oops, made a mistake? Restore to previous state
jvs restore --latest-tag prework

# Or restore to a specific checkpoint
jvs restore abc123  # Use checkpoint ID from jvs checkpoint list
```

---

## Unity-Specific Workflow

### Unity Project Structure

**What to version:**
```
MyGame/
├── Assets/              # ✅ Version this
├── ProjectSettings/     # ✅ Version this
├── Library/             # ❌ Generated, exclude
├── Temp/                # ❌ Temporary files, exclude
└── UserSettings/        # ❌ User-specific, exclude
```

### Setting Up `.jvsignore` for Unity

```bash
# Create .jvsignore in repository root
cat > .jvsignore << 'EOF'
# Unity generated files
Library/
Temp/
obj/
*.userprefs
*.csproj
*.sln
*.suo

# IDE files
.vscode/
.idea/
*.swp
*.swo

# OS files
.DS_Store
Thumbs.db
EOF
```

### Daily Unity Workflow

```bash
# Morning: Start fresh
cd /mnt/juicefs/game-projects/mygame/main
jvs restore baseline

# Before major work
jvs checkpoint "Before animation work $(date +%Y-%m-%d)" --tag prework

# After work
jvs checkpoint "Animation: player run cycle v3" --tag animation --tag $(date +%Y-%m-%d)

# View today's work
jvs checkpoint list | grep $(date +%Y-%m-%d)
```

### Unity Build Integration

Add this to your build script (before creating the build):

```bash
#!/bin/bash
# build_with_snapshot.sh

# Create pre-build checkpoint
jvs checkpoint "Pre-build: $(date +%Y-%m-%d-%H%M)" --tag prebuild

# Run Unity build
/Applications/Unity/Hub/Editor/2022.3.0f1/Unity.app/Contents/MacOS/Unity \
  -quit -batchmode -nographics \
  -projectPath "$(pwd)" \
  -executeMethod BuildScript.BuildAll

# If build succeeds, create post-build checkpoint
if [ $? -eq 0 ]; then
    jvs checkpoint "Build success: v$(cat version.txt)" --tag build --tag success
else
    jvs checkpoint "Build failed: $(date +%Y-%m-%d-%H%M)" --tag build --tag failed
    exit 1
fi
```

---

## Unreal-Specific Workflow

### Unreal Project Structure

**What to version:**
```
MyGame/
├── Content/             # ✅ Version this
├── Config/              # ✅ Version this
├── Binaries/            # ❌ Compiled, exclude
├── Build/              # ❌ Build artifacts, exclude
├── Intermediate/       # ❌ Intermediate files, exclude
├── Saved/              # ❌ Auto-saved, exclude
└── DerivedDataCache/   # ❌ Cache, exclude
```

### Setting Up `.jvsignore` for Unreal

```bash
cat > .jvsignore << 'EOF'
# Unreal generated files
Binaries/
Build/
Intermediate/
Saved/
DerivedDataCache/
*.Openspeed
*.log

# IDE files
.vscode/
.idea/
.vs/

# OS files
.DS_Store
Thumbs.db
EOF
```

### Unreal Daily Workflow

```bash
# Before opening Unreal Editor
jvs checkpoint "Before editor session" --tag prework

# Work in Unreal Editor...

# After closing editor
jvs checkpoint "New level: main menu v2" --tag level --tag $(date +%Y-%m-%d)
```

---

## Multi-Project Workflow

If you work on multiple games/projects:

```bash
# Single parent repository
cd /mnt/juicefs/studio
jvs init studio-projects

# Create workspace for each game
cd studio-projects/main
jvs fork game1-mobile
jvs fork game1-pc
jvs fork shared-assets

# Work on different games independently
cd "$(jvs workspace path game1-mobile)"
jvs restore baseline
# ... work on mobile version ...

cd ../game1-pc/main
jvs restore baseline
# ... work on PC version ...
```

---

## Collaboration Strategy

JVS is single-writer. For team collaboration:

### Option 1: JVS for Local, Git for Code

```bash
# Use Git for C# scripts, shaders
git add Assets/Scripts/
git commit -m "Update player controller"

# Use JVS for binary assets
jvs checkpoint "Updated character model" --tag assets
```

### Option 2: Asset Handoff via JVS

```bash
# Artist A: Work on asset
cd /mnt/juicefs/game-projects/mygame/main
jvs checkpoint "Character model ready for review" --tag review --tag character

# Artist B: Review asset
jvs restore --latest-tag review
# Review in Unity...
jvs checkpoint "Character model approved" --tag approved --tag character
```

---

## Best Practices

### 1. Checkpoint Semantic Milestones

```bash
# Good: Descriptive
jvs checkpoint "MainMenu: Added background animation v2"

# Bad: Generic
jvs checkpoint "work"
jvs checkpoint "update"
```

### 2. Use Tags for Organization

```bash
# Tag by asset type
jvs checkpoint "New character" --tag character --tag models

# Tag by milestone
jvs checkpoint "Alpha build ready" --tag alpha --tag build

# Tag by date
jvs checkpoint "Daily checkpoint" --tag $(date +%Y-%m-%d)
```

### 3. Keep Build Outputs Outside the Workspace

```bash
# Keep build and cache directories outside the workspace when they should not
# be checkpointed.
jvs checkpoint "Assets update"
jvs checkpoint "New audio assets"
```

### 4. Regular Garbage Collection

```bash
# Preview what will be deleted
jvs gc plan

# Actually run GC
jvs gc run --plan-id <plan-id>
```

---

## Common Workflows

### Recovering Deleted Assets

```bash
# Oops, deleted important asset
# 1. Check history
jvs checkpoint list | grep "important asset"

# 2. Restore to checkpoint with the asset
jvs restore abc123

# 3. Copy asset to safe location
cp Assets/Important/asset.fbx ~/backup/

# 4. Return to latest state
jvs restore latest
```

### Comparing Asset Versions

```bash
# List checkpoints and filter in your shell
jvs checkpoint list | grep "character model"

# Use JSON output when automation needs checkpoint IDs and metadata
jvs checkpoint list --json
```

### Creating Asset Variants

```bash
# Create baseline
jvs checkpoint "Character base model" --tag character --tag baseline

# Fork for variant
jvs fork character-armored
cd "$(jvs workspace path character-armored)"

# Modify armor...
jvs checkpoint "Character: armored variant" --tag character --tag armored
```

---

## Troubleshooting

### Problem: Checkpoint is slow

**Solution:** Make sure you're using juicefs-clone engine
```bash
jvs doctor --json | grep engine
# Should show: "engine": "juicefs-clone"
```

### Problem: "File too large" errors

**Solution:** JVS handles any size file. If you see this, you might not be on JuiceFS.
```bash
# Verify JuiceFS mount
df -T | grep juicefs
```

### Problem: Can't find specific checkpoint

**Solution:** Use tags and grep
```bash
# Find by tag
jvs checkpoint list | grep character

# Find by content in note
jvs checkpoint list | grep "animation"
```

---

## Next Steps

- Read [AGENT_SANDBOX_QUICKSTART.md](agent_sandbox_quickstart.md) for AI workflows
- Read [ETL_PIPELINE_QUICKSTART.md](etl_pipeline_quickstart.md) for data workflows
- Read [EXAMPLES.md](EXAMPLES.md) for more examples
- Join the community: [GitHub Discussions](https://github.com/jvs-project/jvs/discussions)
