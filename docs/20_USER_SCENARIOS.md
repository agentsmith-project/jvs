# User Scenarios and Behavior Patterns

This document captures typical user scenarios and expected behaviors for JVS (Juicy Versioned Workspaces).

## Core Concepts

### Workspace States

| State | Description | Can Checkpoint? |
|-------|-------------|---------------|
| **EMPTY** | Newly initialized main workspace, no checkpoints yet; payload may be empty | Yes (creates the first/latest checkpoint) |
| **latest** | At the latest checkpoint of the lineage | Yes |
| **historical** | At a historical checkpoint | No (must fork first) |

### State Transitions

```
EMPTY ──[checkpoint creates first/latest]──► latest ◄──[restore latest]──► historical
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

# Create first checkpoint, even before files exist
$ jvs checkpoint "empty baseline"
Created checkpoint 1771589366481-aaa11111

# Work on project...
$ echo "version 1" > file.txt

# Create a payload checkpoint
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
1771587  2026-02-21 10:20  empty baseline
◄── you are here (latest)
```

**Key Behaviors**:
- A newly initialized empty main workspace can create its first checkpoint; it becomes both current and latest even when the payload is empty
- Each checkpoint automatically becomes the new latest
- User can create new checkpoints from EMPTY or latest state
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
| `jvs checkpoint [note]` | Create checkpoint | EMPTY → latest (first head); latest → latest (new head) |
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

---

## Product-Level E2E Test Requirements (v0)

This section defines the formal product requirements for user-story-driven
end-to-end testing. The suite should validate what users and automation can
observe from the public CLI, real workspace files, and stable JSON output. It
should not depend on private implementation layout except where a test
intentionally creates corruption or validates documented portability
boundaries.

### Background

JVS promises local-first, filesystem-native, checkpoint-first workspace
versioning. Users work in ordinary directories, create full-workspace
checkpoints, restore known states, fork workspaces when they need a separate
line of work, and verify that history remains trustworthy.

Existing conformance tests protect the public CLI contract. The user-story E2E
suite should complement those checks by exercising complete workflows from the
user's point of view, including the JuiceFS-backed behavior that makes JVS
valuable for large filesystem payloads.

### Goals

- Validate that the primary v0 user stories work as complete CLI workflows on
  real filesystem paths.
- Prove that users can understand and trust `current`, `latest`, and `dirty`
  states while moving through checkpoint, restore, and fork workflows.
- Validate JuiceFS-backed workflows on a local JuiceFS mount using sqlite
  metadata and a local bucket, without external services or credentials.
- Confirm that engine behavior is user-visible: supported JuiceFS mounts
  report `juicefs-clone` where applicable, and degraded or fallback behavior is
  explicit.
- Exercise both human-readable CLI behavior and stable JSON output for the
  same product stories.
- Classify defects by user impact so release decisions reflect product risk,
  not only command coverage.

### Non-Goals

- Remote push, pull, hosting, or replication protocols in JVS.
- Merge, rebase, conflict resolution, staging, or Git-compatible commit graph
  behavior.
- Server-side permissions, authn/authz, credential management, or multi-user
  locking services.
- Performance benchmarking beyond confirming the user-visible engine and
  degradation class.
- Private storage-layout tests except for documented corruption, migration, or
  portability scenarios.
- New CLI features, flags, or JSON fields outside the v0 public contract.

### Test Environment

The E2E suite should run against the built `jvs` CLI exactly as a user or
automation would invoke it. Every test should create isolated temporary
directories and should leave no mounted filesystem or background process
behind.

Required profiles:

| Profile | Purpose | Release expectation |
|---------|---------|---------------------|
| `story-local` | Validate all core user stories on a normal local POSIX filesystem. | Required on every release gate. |
| `story-juicefs-local` | Validate the same user stories on a locally mounted JuiceFS filesystem backed by sqlite metadata and a local bucket. | Required when JuiceFS is available in CI or release qualification. |
| `story-json` | Validate automation-facing JSON for the same stories. | Required for all stories that document `--json`. |

The local profile should be the portable baseline. It proves that JVS remains
useful without JuiceFS, even when operations use a copy or reflink-capable
engine.

The JuiceFS profile should be treated as product-critical integration evidence,
not a mock. It should use a real JuiceFS mount so the user-visible engine,
capability, and degradation messages reflect behavior users can reproduce.

### User Story Coverage

Each story should include the happy path, the expected guardrail, and the
observable recovery path. Tests should assert workspace files first, then CLI
and JSON state.

| Story | User intent | Required coverage |
|-------|-------------|-------------------|
| Start a repo | "I want a normal directory where my tools can create files and JVS can start tracking checkpoints." | `init`, default `main` workspace, empty `main` can create the first/latest checkpoint, repo/workspace discovery from root and nested paths, clear rejection of unsafe or ambiguous setup paths. |
| Import existing work | "I already have files and want the first checkpoint to preserve them." | `import` creates a new repo, captures payload only, tags the initial checkpoint, rejects control-plane metadata in the source, and leaves source data untouched. |
| Save progress | "I want to checkpoint the full workspace without staging individual files." | Empty payload first checkpoint, file additions, edits, deletes, nested directories, binary files, empty directories where supported, repeated checkpoint creation from `latest`, notes, tags, and list output. |
| Understand state | "I need to know whether my workspace is safe, current, historical, or dirty." | `status` and `checkpoint list` show `current`, `latest`, `at_latest`, and `dirty` consistently after checkpoint, edit, restore, and fork. |
| Inspect history | "I want to compare or inspect earlier states without losing later checkpoints." | `diff`, `restore <checkpoint>`, `restore latest`, stable refs, unique ID prefixes, exact tags, and rejection of reserved tag names. |
| Protect dirty work | "I do not want restore or removal commands to overwrite uncheckpointed work by accident." | Dirty restore refusal, `--include-working`, `--discard-dirty`, mutual exclusion of safety flags, and workspace removal guardrails. |
| Fork from history | "I found a useful historical state and need a separate workspace from there." | `fork <name>`, `fork <ref> <name>`, `fork --from <ref> <name>`, checkpointing on the new workspace, and isolation from the original workspace. |
| Work in parallel | "I need multiple real directories for separate experiments or agents." | Workspace list/path/rename/remove, independent `current` and `latest`, independent dirty state, and no accidental mutation across workspaces. |
| Verify trust | "I need confidence that checkpoints have not been corrupted." | `verify`, `verify --all`, `doctor`, `doctor --strict`, clear failure on descriptor or payload tampering, and actionable recovery hints. |
| Clone locally | "I need a local copy of a repo or current workspace, not a remote protocol." | `clone --scope current`, `clone --scope full`, new repo identity where documented, copied user-visible workspaces/checkpoints, and excluded active runtime state. |
| Clean safely | "I need cleanup to be planned before anything is deleted." | `gc plan`, protected active lineages, stable plan ID, stale or mismatched plan rejection, and `gc run --plan-id` behavior. |
| Probe filesystem capability | "I need to know whether this path will use JuiceFS clone, reflink, or copy." | `capability`, `--write-probe`, engine summary in setup commands, warnings, degraded reasons, and stable JSON fields. |

### Test Design Principles

- Start from the user's mental model: real workspaces, visible files,
  checkpoints, restore, fork, and verification.
- Prefer complete workflows over isolated command smoke tests. A story should
  leave the workspace in a state that the next user action can observe.
- Treat the filesystem as the source of truth. Assertions should read files and
  directories the way user tools would.
- Use public CLI output, exit codes, and stable JSON fields as the contract.
  Avoid asserting private filenames, storage rows, or package internals.
- Keep state transitions explicit. Every story should state when the workspace
  is expected to be `latest`, historical, or `dirty`.
- Exercise refusal paths as first-class UX. A safe failure with a clear hint is
  a product success.
- Keep v0 vocabulary consistent: repo, workspace, checkpoint, current, latest,
  and dirty.
- Make engine expectations explicit. Tests should distinguish supported
  JuiceFS metadata clone, reflink/copy fallback, and environment skip.
- Keep fixtures deterministic and local. Avoid time, network, user-global
  config, shared mounts, or production paths as hidden dependencies.
- Add new E2E coverage as failing story expectations before changing product
  behavior.

### JuiceFS Local Sqlite + Local Bucket Requirements

The `story-juicefs-local` profile must use a real local JuiceFS deployment with
no external service dependency:

- Metadata store: a per-run sqlite database file in an isolated temporary
  directory.
- Object store: a per-run local bucket directory in an isolated temporary
  directory.
- Mount point: a per-run local mount directory where JVS repos and workspaces
  are created.
- Volume identity: unique per run to avoid collisions with developer or CI
  state.
- Lifecycle: format, mount, validate, unmount, and clean up within the test
  profile.
- Isolation: no network object store, remote metadata service, shared
  production bucket, user credentials, or pre-existing JuiceFS volume.
- Evidence: record JuiceFS CLI version, mount path, metadata path, bucket path,
  JVS version, OS, and whether `juicefs-clone` was selected or degraded.
- Capability gate: run `jvs capability --write-probe` against the mount before
  story execution and classify the result as supported, degraded, or skipped.

When the local JuiceFS environment reports supported clone behavior, the suite
must confirm that checkpoint, restore, import/clone setup, and capability
responses expose `juicefs-clone` or equivalent documented engine messaging.
When the environment is valid but clone support is unavailable, the suite must
not silently pass as if JuiceFS metadata-clone behavior was proven; it should
report degraded coverage with the fallback engine and reason.

### Defect Classification

Defects should be triaged by user impact:

| Class | Meaning | Release impact |
|-------|---------|----------------|
| P0 Data loss or corruption | A default workflow loses user data, corrupts checkpoint history, or reports corrupted data as valid. | Blocks release. |
| P0 Safety bypass | A destructive command overwrites dirty work without explicit user intent. | Blocks release. |
| P1 Story failure | A primary v0 story cannot be completed on the required local profile. | Blocks release unless explicitly waived. |
| P1 JuiceFS false claim | Tests or product output imply supported JuiceFS metadata-clone behavior without real supported evidence. | Blocks release claims and JuiceFS profile signoff. |
| P1 Automation contract break | JSON output for a story is malformed, mixed with diagnostics, or missing stable required fields. | Blocks release. |
| P2 UX ambiguity | Errors, hints, or state labels leave a reasonable user unsure what happened or what to do next. | Must be fixed or documented before release. |
| P2 Portability gap | Behavior differs across local and JuiceFS profiles without an explicit engine or filesystem explanation. | Requires triage before release. |
| P3 Coverage debt | A story path lacks regression coverage but no current product failure is observed. | Track in the implementation plan. |

### Deliverables

- A runnable E2E story suite with separate local, JuiceFS-local, and JSON
  selection profiles.
- A traceability table mapping each test file or scenario to the user stories
  in this document and the contract areas in `docs/11_CONFORMANCE_TEST_PLAN.md`.
- Local JuiceFS environment setup and cleanup automation suitable for CI and
  developer machines.
- Release evidence that records environment details, engine selection,
  pass/fail summary, skipped/degraded JuiceFS coverage, and defect
  classification.
- Product-facing documentation updates when tests reveal confusing terminology,
  unsafe examples, or unsupported claims.

### Acceptance Standards

- All `story-local` tests pass before a v0 release.
- All `story-json` tests pass for commands that document JSON output.
- `story-juicefs-local` either passes on a real local sqlite-backed JuiceFS
  mount with local bucket storage or reports an explicit environment skip that
  prevents unsupported JuiceFS performance claims from being made.
- Any P0 defect blocks release.
- Any unwaived P1 defect blocks release.
- Public docs, CLI help, examples, JSON fields, and test names use the same v0
  vocabulary.
- The suite proves that default destructive operations protect dirty user
  files, and that explicit safety flags behave exactly as described.
- The suite proves that JVS remains local-first: no test requires a remote
  server, remote repository, push/pull command, merge/rebase operation, or
  service-side permission model.

### Phased Implementation Guidance

1. **Story Baseline**: Build the `story-local` profile for repo setup,
   checkpoint, status, restore, fork, workspace isolation, and dirty guards.
   This phase should make the user state model visible and stable before
   expanding into specialized environments.
2. **Automation Parity**: Add `story-json` coverage for the same workflows.
   Human-readable and JSON assertions should describe the same product
   outcomes, with JSON tests focused on stable fields and machine-readable
   errors.
3. **Integrity And Cleanup**: Add verification, doctor, corruption detection,
   clone, migration boundary, and GC planning/running stories. Keep corruption
   setup scoped to documented integrity and portability promises.
4. **Local JuiceFS Integration**: Add the sqlite metadata plus local bucket
   JuiceFS profile. Start with capability probing and one checkpoint/restore
   story, then expand to import, clone, fork, dirty guard, and verification
   stories once environment setup is reliable.
5. **Release Evidence And Gates**: Connect story profiles to release
   qualification. Persist environment evidence, engine results,
   skipped/degraded reasons, and defect classification so release notes can
   make accurate product claims.
