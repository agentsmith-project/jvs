# JVS Target Users: Pain Points And Requirements

**Version:** v0 public contract
**Last Updated:** 2026-04-25
**Status:** Active release-facing product research

---

## Overview

This document describes the primary audiences for JVS and the product promises
that matter to them. It is release-facing: command examples and terminology
must match the stable v0 public contract.

JVS is strongest when users need reproducible filesystem state for large or
tool-generated workspaces. The public model is repo, workspace, checkpoint,
current, latest, and dirty.

---

## Target User 1: Game Asset Management

### Persona

| Attribute | Description |
| --- | --- |
| Company type | Indie to mid-size game studios |
| Team size | 5-50 developers and artists |
| Primary roles | Technical artists, game developers, asset pipeline engineers |
| Typical workspace | 100 GB to 2 TB per project |

### Pain Points

- Git and Git LFS are awkward for large binary assets.
- Unity and Unreal generate metadata and build artifacts that need careful
  workspace discipline.
- Binary assets cannot be merged like text.
- Teams often need simple rollback points around risky asset or build changes.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Large binary workspace state | Checkpoints capture the full workspace tree. |
| Simple rollback | `jvs restore current` and `jvs restore latest` make state explicit. |
| Experiments without merge semantics | `jvs fork` creates another real workspace. |
| Integrity checks | `jvs verify` and `jvs doctor --strict` detect corruption. |

JVS does not provide file locking, binary merge, or a game-editor plugin in the
v0 contract. Teams that need exclusive asset coordination should use an
external process or a system designed for multi-user locking.

### Recommended Workflow

```bash
cd /mnt/juicefs/game-projects
jvs init mygame
cd mygame/main

cp -r ~/UnityProjects/MyGame/* .
jvs checkpoint "initial Unity project import" --tag unity --tag baseline

jvs checkpoint "before modifying character" --tag prework
# work in Unity or Unreal
jvs checkpoint "updated character model v2" --tag character --tag assets

jvs restore prework
jvs fork experiment-character
```

### Product Guidance

- Keep checkpoint creation and restore boring and reliable.
- Keep engine fallback visible; constant-time behavior depends on filesystem
  support.
- Document integration scripts rather than building editor-specific product
  surfaces.
- Do not add merge, rebase, file locking, or server orchestration to v0.

---

## Target User 2: AI Agent Sandbox Environments

### Persona

| Attribute | Description |
| --- | --- |
| Company type | AI research labs and agent platform teams |
| Team size | 2-20 engineers or researchers |
| Primary roles | ML engineers, agent infrastructure engineers |
| Typical workspace | 1 GB to 100 GB per agent environment |

### Pain Points

- Each agent run needs a clean and reproducible starting state.
- Containers or VMs may be too heavy for tight local experiment loops.
- Parallel runs need separate filesystem state.
- Engineers need to capture the exact files that produced a result.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Deterministic reset | Restore a named checkpoint before a run. |
| Parallel experiments | Fork one workspace per run. |
| Result capture | Create a checkpoint with run tags and notes. |
| Automation | `--json` output gives stable machine-readable state. |

JVS operates at the filesystem layer. It complements container isolation; it is
not a container runtime, scheduler, or security sandbox.

### Recommended Workflow

```bash
cd /mnt/juicefs/agent-sandbox
jvs init agent-base
cd agent-base/main

cp -r /baseline/agent/* .
jvs checkpoint "agent baseline v1" --tag baseline --tag v1

for RUN in 1 2 3; do
    jvs fork baseline run-$RUN
    cd "$(jvs workspace path run-$RUN)"

    python agent.py --seed "$RUN" --output "results/$RUN.json"
    RESULT=$(jq -r '.outcome' "results/$RUN.json")
    jvs checkpoint "run $RUN: $RESULT" --tag "run-$RUN" --tag agent

    cd "$(jvs workspace path main)"
done
```

### Product Guidance

- Keep workspace targeting deterministic and script-friendly.
- Keep JSON envelopes stable for automation.
- Avoid building an agent framework, orchestrator, or queue into JVS.

---

## Target User 3: Data ETL Pipelines

### Persona

| Attribute | Description |
| --- | --- |
| Company type | Analytics, ML, fintech, and SaaS data teams |
| Team size | 5-50 data engineers |
| Primary roles | Data engineers, ML engineers, platform engineers |
| Typical workspace | 10 GB to 10 TB per dataset workspace |

### Pain Points

- Models and reports depend on exact data files.
- Pipeline failures should not publish invalid state.
- Teams need clear rollback points between ETL stages.
- Auditors may ask which workspace state produced a result.

### JVS Fit

| Need | JVS fit |
| --- | --- |
| Dataset workspace versions | Checkpoints capture stage outputs as filesystem state. |
| Pipeline integration | The CLI is scriptable and local-first. |
| Rollback | Restore known refs before retrying a stage. |
| Integrity | Verification checks descriptor and payload hashes. |

JVS complements table formats such as Iceberg and Delta. Use those systems for
table-level time travel and query semantics; use JVS for workspace-level state.

### Recommended Workflow

```bash
TODAY=$(date +%Y-%m-%d)
cd /mnt/juicefs/etl-pipeline/main

jvs restore latest
python ingest_raw.py --date "$TODAY"
jvs checkpoint "raw ingestion $TODAY" --tag raw --tag "$TODAY"

python transform.py --input raw/ --output processed/
jvs checkpoint "transformed $TODAY" --tag processed --tag "$TODAY"

python build_features.py --input processed/ --output features/
jvs checkpoint "features $TODAY" --tag features --tag "$TODAY"

python train.py --input features/ --output model.pkl
jvs checkpoint "model trained on $TODAY" --tag model --tag "$TODAY"
```

### Product Guidance

- Keep setup and checkpoint operations path-safe for automation.
- Keep failure modes explicit and machine-readable.
- Do not add a scheduler, catalog, SQL engine, or remote protocol to v0.

---

## Cross-Cutting Patterns

### Large Workspaces

All target users operate on workspaces where Git-style file-by-file workflows
are painful. JVS should keep engine selection transparent so users can see when
the filesystem supports efficient materialization and when it falls back to
recursive copy.

### Reproducibility

All target users need to answer: "which exact workspace state produced this
result?" Checkpoint IDs, tags, notes, `jvs verify`, and `jvs doctor --strict`
are the core answer.

### Collaboration

JVS is local-first and does not replace multi-user coordination. For v0,
collaboration guidance should focus on external process, mounted filesystem
permissions, and clear workspace ownership.

---

## v0 Product Boundaries

Keep in the stable public contract:

- Repo and workspace setup.
- Checkpoint creation, listing, diffing, and restore.
- Fork, list, locate, rename, and remove workspaces.
- Dirty-state protection.
- Verification and strict doctor checks.
- Two-phase storage cleanup with plan IDs.
- JSON envelopes and stable error classes.

Do not add to v0:

- Merge, rebase, push, pull, or remote hosting.
- File locking or distributed coordination.
- Partial checkpoint contracts.
- Ignore-file contracts.
- Compression contracts.
- Complex retention policy flags.
- Built-in container, scheduler, editor, or database integrations.

---

## Core Value Proposition

JVS provides verifiable filesystem workspace checkpoints for large, local-first
workflows where Git-style versioning is the wrong abstraction.
