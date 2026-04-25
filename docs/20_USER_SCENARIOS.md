# User Scenarios and Behavior Patterns

This document captures typical user scenarios and expected behaviors for JVS (Juicy Versioned Workspaces).

## Core Concepts

### Workspace States

| State | Description | Can Checkpoint? |
|-------|-------------|---------------|
| **EMPTY** | Newly created workspace, no checkpoints yet | No (nothing to checkpoint) |
| **latest** | At the latest checkpoint of the lineage | Yes |
| **historical** | At a historical checkpoint | No (must fork first) |

### State Transitions

```
EMPTY ──[checkpoint]──► latest ◄──[restore latest]──► historical
                          │                         │
                          │     [restore <id>]      │
                          └────────────────────────►│
                          │                         │
                          │       [jvs fork]         │
                          │◄────────────────────────┤
                          │                         │
                          │      [jvs fork]          │
                          └──────► NEW latest ◄───────┘
```

---

## Scenario 1: Basic Workspace Versioning

**User Goal**: Save checkpoints while working on a project.

```bash
# Initialize repository
$ cd /projects
$ jvs init myproject
$ cd myproject/main

# Work on project...
$ echo "version 1" > file.txt

# Create first checkpoint
$ jvs checkpoint "initial version"
Created checkpoint 1771589366482-abc12345

# Continue working...
$ echo "version 2" > file.txt

# Create another checkpoint
$ jvs checkpoint "updated content"
Created checkpoint 1771589366483-def78901

# View history
$ jvs checkpoint list
1771589  2026-02-21 10:30  updated content     [latest]
1771588  2026-02-21 10:25  initial version
◄── you are here (latest)
```

**Key Behaviors**:
- Each checkpoint automatically becomes the new latest
- User can always create new checkpoints (in latest state)
- Files in workspace are always "live" - what you see is what you have

---

## Scenario 2: Exploring History (Time Travel)

**User Goal**: Look at how the project looked at a previous point in time.

```bash
# Current state: at latest
$ jvs checkpoint list
1771589  2026-02-21 10:30  release v2     [latest]
1771588  2026-02-21 10:25  release v1
1771587  2026-02-21 10:20  initial
◄── you are here (latest)

# Restore to historical checkpoint
$ jvs restore 1771587
Restored to checkpoint 1771587-xyz78901
Workspace is now at a historical checkpoint.

$ cat file.txt
initial content  # Files now show historical state

# History shows we're at a historical point
$ jvs checkpoint list
1771589  2026-02-21 10:30  release v2     [latest]
1771588  2026-02-21 10:25  release v1
1771587  2026-02-21 10:20  initial
◄── you are here (historical)

# Just looking around, now want to go back to latest
$ jvs restore latest
Restored to latest checkpoint 1771589
Workspace is back at latest state.
```

**Key Behaviors**:
- `restore <id>` always does inplace restore (no separate "safe restore")
- After restore, the workspace differs from latest until you restore latest or fork
- `restore latest` brings back to the latest state
- No data loss - all checkpoints in the lineage are preserved

---

## Scenario 3: Creating a Branch from History

**User Goal**: Found a bug introduced after a certain checkpoint, want to create a fix branch from that point.

```bash
# Restore to the known-good point
$ jvs restore 1771587
Restored to checkpoint 1771587
Workspace is now at a historical checkpoint.

# Verify this is the right starting point
$ cat file.txt
known good content

# Try to create checkpoint - NOT ALLOWED in current differs from latest
$ jvs checkpoint "bugfix attempt"
Error: cannot create checkpoint in current differs from latest

You are currently at checkpoint '1771587' (historical).
To continue working from this point:

    jvs fork bugfix-branch

Or return to the latest state:

    jvs restore latest

# Create a new workspace from current position
$ jvs fork bugfix-branch
Created workspace 'bugfix-branch' from checkpoint 1771587
Workspace is at latest state - you can now create checkpoints.

# Switch to the new branch
$ cd "$(jvs workspace path bugfix-branch)"

# Now can make changes and checkpoint
$ echo "bugfix applied" > file.txt
$ jvs checkpoint "fixed the bug"
Created checkpoint 1771590-aaa11111
```

**Key Behaviors**:
- Cannot create checkpoints in current differs from latest (prevents history corruption)
- Must use `jvs fork` to create a new branch
- Fork from current position by omitting checkpoint ID

---

## Scenario 4: Fork from Any Checkpoint

**User Goal**: Create an experimental branch from any historical point.

```bash
# Fork from specific checkpoint (even while at latest)
$ jvs fork 1771588 experiment-v1
Created workspace 'experiment-v1' from checkpoint 1771588

# Or fork from current position
$ jvs restore 1771587
$ jvs fork experiment-v2
Created workspace 'experiment-v2' from checkpoint 1771587

# List all workspaces
$ jvs workspace list
main              /repo/main              latest at 1771589
experiment-v1     /path/from/workspace/path   latest at 1771588
experiment-v2     /path/from/workspace/path   latest at 1771587
```

**Key Behaviors**:
- `jvs fork <id> <name>` - fork from specific checkpoint
- `jvs fork <name>` - fork from current position (convenient shorthand)
- New workspace is always at latest state (can checkpoint immediately)

---

## Scenario 5: Parallel Development

**User Goal**: Work on multiple features in parallel without interference.

```bash
# Create feature branches from main
$ jvs fork feature-auth
Created workspace 'feature-auth'

$ jvs fork feature-ui
Created workspace 'feature-ui'

# Work on auth feature
$ cd "$(jvs workspace path feature-auth)"
$ echo "auth implementation" > auth.py
$ jvs checkpoint "auth module complete"
Created checkpoint 1771590-aaa11111

# Work on UI feature (independent)
$ cd "$(jvs workspace path feature-ui)"
$ echo "ui implementation" > ui.py
$ jvs checkpoint "ui module complete"
Created checkpoint 1771591-bbb22222

# Both features have independent lineages
# main workspace unchanged
$ cd /repo/main
$ jvs checkpoint list
# Only shows main's history, not feature branches
```

**Key Behaviors**:
- Each workspace has its own independent checkpoint lineage
- No "merging" needed - workspaces are isolated
- JuiceFS handles storage efficiency (CoW)

---

## Scenario 6: Recovering from Mistakes

**User Goal**: Made a mistake, want to go back to a known-good state.

```bash
# Current state with unwanted changes
$ cat file.txt
terrible mistake

# View history to find good state
$ jvs checkpoint list
1771589  2026-02-21 10:30  bad changes       [latest]
1771588  2026-02-21 10:25  good state
◄── you are here (latest)

# Restore to good state
$ jvs restore 1771588
Restored to checkpoint 1771588
Workspace is now at a historical checkpoint.

$ cat file.txt
good content here  # Back to good state

# Option A: Discard the bad checkpoint, continue from here
$ jvs fork main-v2
# ... continue in new workspace ...

# Option B: Go back to latest and try again
$ jvs restore latest
# Back at bad state, but can fix and create new checkpoint
```

**Key Behaviors**:
- Restoring doesn't delete any checkpoints
- User can always explore and return to any state
- "Bad" checkpoints can be cleaned up later via GC

---

## Scenario 7: Using Tags for Releases

**User Goal**: Mark important checkpoints with tags for easy reference.

```bash
# Create checkpoint with tags
$ jvs checkpoint "release 1.0" --tag v1.0 --tag release --tag stable
Created checkpoint 1771589-abc12345

# Create more checkpoints
$ jvs checkpoint "release 1.1" --tag v1.1 --tag release
Created checkpoint 1771590-def78901

# Find by tag
$ jvs checkpoint list | grep release
1771590  2026-02-21 10:30  release 1.1  [v1.1, release]
1771589  2026-02-21 10:25  release 1.0  [v1.0, release, stable]

# Restore by tag (using fuzzy match)
$ jvs restore v1.0
Restored to checkpoint 1771589 (v1.0)
Workspace is now at a historical checkpoint.
```

**Key Behaviors**:
- Tags are metadata on checkpoints
- Multiple tags per checkpoint allowed
- Fuzzy match by tag or note prefix

---

## Command Reference Summary

| Command | Description | State Change |
|---------|-------------|--------------|
| `jvs checkpoint [note]` | Create checkpoint | latest → latest (new head) |
| `jvs restore <id>` | Restore to checkpoint | Any → historical |
| `jvs restore latest` | Restore to latest | historical → latest |
| `jvs fork [name]` | Fork from current | (creates new latest) |
| `jvs fork <id> [name]` | Fork from checkpoint | (creates new latest) |
| `jvs checkpoint list` | Show checkpoint history | (no change) |

---

## Error Messages and Guidance

### Checkpoint While Current Differs From Latest

```
$ jvs checkpoint "my changes"
Error: cannot create checkpoint in current differs from latest

You are currently at checkpoint '1771587' (historical).
To continue working from this point:

    jvs fork <name>        # Create new workspace from here
    jvs restore latest                # Return to latest state
```

### Restore Non-existent Checkpoint

```
$ jvs restore nonexistent
Error: checkpoint not found: nonexistent

Use 'jvs checkpoint list' to see available checkpoints.
```

### Fork with Existing Name

```
$ jvs fork existing-name
Error: workspace 'existing-name' already exists

Use 'jvs workspace list' to see existing workspaces.
```

---

## Design Principles

1. **One Command, One Action**: Each command does exactly one thing. No mode flags.

2. **Safe by Default**: `restore` doesn't destroy data - it just moves a pointer.

3. **Explicit Over Implicit**: User must explicitly `fork` to create branches.

4. **Clear State Indication**: `history` always shows current position.

5. **No Surprise Data Loss**: All checkpoints are preserved until explicit GC.
