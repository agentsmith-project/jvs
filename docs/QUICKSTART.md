# JVS Quick Start Guide

Get from an empty directory to a verified workspace checkpoint in a few
minutes.

## Prerequisites

- Go 1.25+ if you build from source.
- Optional: JuiceFS or a reflink-capable filesystem for faster copy-on-write
  materialization. JVS also works on regular POSIX filesystems with the copy
  engine.

## Install

```bash
git clone https://github.com/jvs-project/jvs.git
cd jvs
make build
sudo cp bin/jvs /usr/local/bin/
```

You can inspect the filesystem JVS will use before creating a repo:

```bash
jvs capability /path/to/parent
```

## 5-Minute Tutorial

### 1. Initialize a Repo

```bash
cd /path/to/parent
jvs init myproject
```

JVS creates a repository root with a default `main` workspace:

```text
myproject/
├── .jvs/          # control-plane metadata
└── main/          # workspace payload
```

The repo root is not the workspace payload. Work on files inside `main/`:

```bash
cd myproject/main
```

### 2. Create the First Checkpoint

```bash
echo "Hello JVS" > README.md
echo "print('hello')" > script.py

jvs status
jvs checkpoint "initial setup"
```

`jvs status` reports the targeted workspace, the `current` checkpoint, the
`latest` checkpoint, and whether the workspace is `dirty`.

### 3. Make and Save More Changes

```bash
echo "Updated content" >> README.md
echo "print('world')" >> script.py

jvs status
jvs checkpoint "added more content" --tag stable
```

List checkpoints:

```bash
jvs checkpoint list
```

### 4. Restore

Restore accepts `current`, `latest`, a full checkpoint ID, a unique ID prefix,
or an exact tag:

```bash
jvs restore stable
jvs restore latest
```

If the workspace has uncheckpointed changes, restore refuses to overwrite them.
Choose one explicit outcome:

```bash
jvs restore latest --include-working
jvs restore latest --discard-dirty
```

### 5. Fork Another Workspace

```bash
jvs fork experiment
cd "$(jvs workspace path experiment)"
```

The new workspace is a normal directory with its own `current`, `latest`, and
`dirty` state. To create a workspace from a specific checkpoint:

```bash
jvs fork stable release-test
# or
jvs fork --from stable release-test-2
```

### 6. Verify and Diagnose

```bash
jvs verify --all
jvs doctor --strict
```

`verify` checks descriptor checksums and payload hashes. `doctor --strict`
checks repository health and integrity more broadly.

## Import and Clone

Import an existing directory into a new JVS repo:

```bash
jvs import ./existing-files ./myproject
cd myproject/main
```

Clone an existing local repo:

```bash
jvs clone ./myproject ./myproject-copy
jvs clone ./myproject ./current-only-copy --scope current
```

`clone` is local filesystem copy, not a push/pull remote protocol.

## Command Reference

| Command | Description | Example |
| --- | --- | --- |
| `jvs init <repo-path>` | Create a repo | `jvs init myproject` |
| `jvs import <dir> <repo-path>` | Import an existing directory | `jvs import ./src ./repo` |
| `jvs clone <src> <dest>` | Clone a local repo | `jvs clone ./repo ./copy` |
| `jvs capability <path>` | Probe filesystem support | `jvs capability .` |
| `jvs info` | Show repo metadata | `jvs info --json` |
| `jvs status` | Show workspace state | `jvs status` |
| `jvs checkpoint [note]` | Save current workspace | `jvs checkpoint "fixed bug"` |
| `jvs checkpoint list` | List checkpoints | `jvs checkpoint list` |
| `jvs diff <from> <to>` | Compare checkpoints | `jvs diff current latest --stat` |
| `jvs restore <ref>` | Restore a checkpoint | `jvs restore latest` |
| `jvs fork [ref] <name>` | Create another workspace | `jvs fork stable test` |
| `jvs workspace list` | List workspaces | `jvs workspace list` |
| `jvs workspace path [name]` | Print a workspace path | `jvs workspace path main` |
| `jvs verify [--all]` | Verify integrity | `jvs verify --all` |
| `jvs doctor [--strict]` | Diagnose repository health | `jvs doctor --strict` |
| `jvs gc plan` | Preview retention cleanup | `jvs gc plan` |

## Tips

- Work inside a workspace payload such as `main/`, not in the repo root.
- Run `jvs status` before destructive operations.
- Use tags for important checkpoints, and keep external automation on full
  checkpoint IDs when possible.
- Use `jvs restore latest` to return a workspace from an older `current`
  checkpoint to the normal latest checkpoint.
- Use `jvs fork <name>` when you want to continue from the current checkpoint
  without moving the original workspace forward.

## What Is Not in v0

The v0 public CLI does not include remote push/pull, signing commands, partial
checkpoint contracts, compression contracts, merge/rebase, or complex retention
policy flags.
