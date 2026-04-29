# JVS Quickstart: AI Agent Sandboxes

**Status:** Active non-release-facing non-normative example, not part of the v0 public contract.

Use this page as an illustrative domain example. It does not add GA product
commitments, sandbox guarantees, schedulers, or agent-specific save semantics.
The stable user workflow is in [docs/user/examples.md](user/examples.md).

## Why Agents Use JVS

Agent runs often need a clean folder before execution and an exact saved folder
after execution. JVS keeps that loop small:

```text
save baseline -> run agent -> save result -> restore baseline for the next run
```

JVS is not a container runtime or scheduler. Use your normal isolation,
secrets, queue, and network controls around the folder.

## Setup

```bash
mkdir agent-sandbox
cd agent-sandbox
jvs init

cp -r /path/to/agent-env/* .
jvs save -m "agent baseline"
jvs history
```

Copy the baseline save point ID from `jvs history`.

## One Run

```bash
BASELINE=<save>

python agent.py --seed 1 --output results/run-1.json
jvs save -m "agent run 1"
```

Before another run, preview and run a restore back to the baseline:

```bash
jvs restore "$BASELINE" --discard-unsaved
jvs restore --run <plan-id>
```

The first command prints what managed files would change. The second command
uses the printed plan ID and changes files only after JVS rechecks the folder.

## Batch Runs

For repeatable automation, parse JSON output and keep each restore as a
preview/run pair:

```bash
BASELINE=$(jvs save -m "batch baseline" --json | jq -r '.data.save_point_id')

for RUN in 1 2 3; do
    PLAN=$(jvs restore "$BASELINE" --discard-unsaved --json | jq -r '.data.plan_id')
    jvs restore --run "$PLAN"

    python agent.py --seed "$RUN" --output "results/$RUN.json"
    RESULT=$(jq -r '.outcome' "results/$RUN.json")
    jvs save -m "run $RUN: $RESULT"
done
```

## Parallel Runs

Create a separate real folder from the baseline for each worker:

```bash
jvs workspace new run-a --from "$BASELINE"
jvs workspace new run-b --from "$BASELINE"
```

The command prints each folder path. Start one agent process per folder and
save results inside that folder:

```bash
cd <printed-folder>
python agent.py --variant A --output results.json
jvs save -m "variant A result"
```

## Inspect And Recover

Find and inspect saved states:

```bash
jvs history --grep "run"
jvs view <save> results/run-1.json
jvs view close <view-id>
```

If a restore is interrupted:

```bash
jvs recovery status
jvs recovery resume <recovery-plan>
```

or:

```bash
jvs recovery rollback <recovery-plan>
```

For health checks:

```bash
jvs doctor
jvs doctor --strict
```
