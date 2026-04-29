# JVS Target Users: Pain Points And Requirements

**Status:** Active product research, non-release-facing, and not part of the v0 public contract.

This document describes the primary audiences for JVS and the promises that
matter to them. It is supporting research, not a GA product promise. Vertical
sections and flows below are non-normative examples; they do not create
commitments to game, ML/agent, ETL, editor, scheduler, or data-platform
integrations.

Any future release-facing examples must match the public save point CLI:

```text
init -> save -> history -> view -> restore
```

JVS is strongest when users need reproducible filesystem state for large or
tool-generated folders. The public model is folder, workspace, save point,
history, view, restore, workspace new, recovery, and doctor.

## Game Asset Management

### Persona

| Attribute | Description |
| --- | --- |
| Company type | Indie to mid-size game studios |
| Team size | 5-50 developers and artists |
| Primary roles | Technical artists, game developers, asset pipeline engineers |
| Typical folder | 100 GB to 2 TB per project |

### Pain Points

- Git and Git LFS are awkward for large binary assets.
- Unity and Unreal generate metadata and build artifacts that need folder
  discipline.
- Binary assets cannot be merged like text.
- Teams need simple saved states around risky asset or build changes.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Large binary folder state | Save points capture managed files in the workspace. |
| Safe recovery | Restore is preview-first and history is kept. |
| Variant work | `jvs workspace new <name> --from <save>` creates another real folder. |
| Health checks | `jvs doctor` reports repository health. |

JVS does not provide file locking, binary merge, source hosting, or a
game-editor plugin. Teams that need exclusive asset coordination should use an
external process or a system designed for multi-user locking.

### Example Flow

```bash
mkdir mygame-assets
cd mygame-assets
jvs init

cp -r ~/UnityProjects/MyGame/Assets .
cp -r ~/UnityProjects/MyGame/ProjectSettings .
jvs save -m "initial Unity asset import"

jvs save -m "before modifying character"
# Work in Unity or Unreal.
jvs save -m "updated character model"

jvs history --grep "character"
jvs view <save> Assets/Characters/Hero
jvs restore <save> --discard-unsaved
jvs restore --run <plan-id>
```

## AI Agent Sandbox Environments

### Persona

| Attribute | Description |
| --- | --- |
| Company type | AI research labs and agent platform teams |
| Team size | 2-20 engineers or researchers |
| Primary roles | ML engineers, agent infrastructure engineers |
| Typical folder | 1 GB to 100 GB per agent environment |

### Pain Points

- Each agent run needs a clean and reproducible starting state.
- Containers or VMs may be too heavy for tight local experiment loops.
- Parallel runs need separate filesystem state.
- Engineers need to capture the exact files that produced a result.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Deterministic reset | Restore a chosen save point before a run. |
| Parallel experiments | Create one workspace folder per run. |
| Result capture | Save result folders with descriptive messages. |
| Automation | `--json` output gives stable machine-readable state. |

JVS operates at the filesystem layer. It complements container isolation; it is
not a container runtime, scheduler, or security sandbox.

### Example Flow

```bash
mkdir agent-sandbox
cd agent-sandbox
jvs init

cp -r /baseline/agent/* .
BASELINE=$(jvs save -m "agent baseline" --json | jq -r '.data.save_point_id')

jvs workspace new run-1 --from "$BASELINE"
cd <printed-folder>
python agent.py --seed 1 --output results.json
jvs save -m "run 1 result"
```

## Data ETL Pipelines

### Persona

| Attribute | Description |
| --- | --- |
| Company type | Analytics, ML, fintech, and SaaS data teams |
| Team size | 5-50 data engineers |
| Primary roles | Data engineers, ML engineers, platform engineers |
| Typical folder | 10 GB to 10 TB per dataset workspace |

### Pain Points

- Models and reports depend on exact data files.
- Pipeline failures should not publish invalid state.
- Teams need clear saved states between ETL stages.
- Auditors may ask which folder state produced a result.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Dataset folder versions | Save points capture stage outputs as filesystem state. |
| Pipeline integration | The CLI is scriptable and local-first. |
| Retry | Restore a known save point before retrying a stage. |
| Health checks | `jvs doctor --strict` performs deeper repository checks. |

JVS complements table formats such as Iceberg and Delta. Use those systems for
table-level time travel and query semantics; use JVS for folder-level state.

### Example Flow

```bash
TODAY=$(date +%Y-%m-%d)
mkdir etl-workspace
cd etl-workspace
jvs init

python ingest_raw.py --date "$TODAY" --output raw/
jvs save -m "raw ingestion $TODAY"

python transform.py --input raw/ --output processed/
jvs save -m "processed $TODAY"

jvs history --grep "$TODAY"
jvs restore <save> --discard-unsaved
jvs restore --run <plan-id>
```

## Cross-Cutting Patterns

### Large Folders

All target users operate on folders where Git-style file-by-file workflows can
be painful. JVS should keep setup simple and make file-changing operations
explicit.

### Reproducibility

All target users need to answer: "which exact folder state produced this
result?" Save point IDs, messages, `jvs history`, `jvs view`, and `jvs doctor`
are the user-facing answer.

### Collaboration

JVS is local-first and does not replace multi-user coordination. Release-facing
collaboration guidance should focus on external process, mounted filesystem
permissions, clear workspace ownership, and using separate workspace folders
for separate runs or variants.

## Product Boundaries

This research recommends keeping the GA public contract focused on:

- Folder adoption.
- Save point creation and history.
- Read-only views.
- Preview-first whole-folder and path restore.
- Workspace creation from a save point.
- Restore recovery.
- Doctor health checks.
- JSON envelopes for automation.

Do not present as a release-facing user workflow:

- Merge, rebase, push, pull, or remote hosting.
- File locking or distributed coordination.
- Built-in container, scheduler, editor, or database integrations.
- Commands outside the public help surface.

## Core Value Proposition

JVS provides filesystem save points for large, local-first folders where
Git-style versioning is the wrong abstraction.
