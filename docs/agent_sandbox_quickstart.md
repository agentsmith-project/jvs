# JVS Quick Start: AI Agent Sandbox

**Version:** v0 public contract
**Last Updated:** 2026-02-23

---

## Overview

This guide helps AI/ML engineers use JVS for creating deterministic, reproducible agent sandbox environments. On supported JuiceFS mounts, JVS can use `juicefs-clone` for O(1) metadata-clone checkpoints and restores; other engines fall back to filesystem-appropriate behavior.

---

## Why JVS for Agent Sandboxes?

| Problem | Docker | VMs | JVS |
|---------|--------|-----|-----|
| Environment reset | Slow rebuild | Very slow | Engine-scoped restore |
| Deterministic state | Complex to set up | Complex to set up | Simple checkpoint/restore |
| Parallel experiments | Container overhead | VM overhead | Workspace isolation |
| State tracking | Volume management | Checkpoint management | Built-in history |

**Key Benefit:** Reset agent environments to exact baseline state; supported JuiceFS mounts can use metadata-clone restores, while copy fallback scales with payload size.

---

## Prerequisites

1. **Supported JuiceFS mounted** (recommended for O(1) performance)
2. **JVS installed**
3. **Agent code** (Python, or any language)

---

## Quick Start (5 Minutes)

### Step 1: Initialize Base Agent Environment

```bash
# Navigate to your JuiceFS mount
cd /mnt/juicefs/agent-sandbox

# Initialize JVS repository
jvs init agent-base
cd agent-base/main

# Copy your agent environment
cp -r ~/agent-environment/* .

# Create baseline checkpoint
jvs checkpoint "Agent baseline v1" --tag baseline --tag v1
```

### Step 2: Run Agent Experiment

```bash
# Restore to baseline
jvs restore baseline

# Run your agent
python agent.py --config config/experiment1.json --output results/run1.json

# Checkpoint result
jvs checkpoint "Run 1: success" --tag run1 --tag agent
```

### Step 3: Batch Experiments

```bash
#!/bin/bash
# Run 100 agent experiments

for RUN in {1..100}; do
    # Reset to baseline
    jvs restore baseline

    # Run agent with different seed
    python agent.py \
        --seed $RUN \
        --config config/experiment_$RUN.json \
        --output results/$RUN.json

    # Checkpoint result state
    RESULT=$(cat results/$RUN.json | jq -r '.outcome')
    jvs checkpoint "Run $RUN: $RESULT" --tag "run-$RUN" --tag agent
done
```

---

## Common Patterns

### Pattern 1: Sequential Experiments

```bash
# Reset environment between each run
jvs restore baseline

# Run experiment 1
python agent.py --task task1
jvs checkpoint "After task1" --tag task1

# Run experiment 2
jvs restore baseline  # Reset to clean state
python agent.py --task task2
jvs checkpoint "After task2" --tag task2
```

### Pattern 2: Parallel Experiments with Workspaces

```bash
# Create workspaces for parallel execution
jvs fork experiment-a
jvs fork experiment-b
jvs fork experiment-c

# Run experiments in parallel
cd "$(jvs workspace path experiment-a)"
jvs restore baseline
python agent.py --variant A &

cd ../../experiment-b/main
jvs restore baseline
python agent.py --variant B &

cd ../../experiment-c/main
jvs restore baseline
python agent.py --variant C &

wait  # Wait for all to complete
```

### Pattern 3: A/B Testing

```bash
# Create baseline with model A
cp models/model_a.pth checkpoint.pth
jvs checkpoint "Baseline: model A" --tag baseline --tag model-a

# Run experiments with model A
for RUN in {1..50}; do
    jvs restore baseline
    python agent.py --checkpoint checkpoint.pth --run $RUN
    jvs checkpoint "ModelA-run-$RUN" --tag model-a
done

# Create baseline with model B
cp models/model_b.pth checkpoint.pth
jvs checkpoint "Baseline: model B" --tag baseline --tag model-b

# Run experiments with model B
for RUN in {1..50}; do
    jvs restore baseline
    python agent.py --checkpoint checkpoint.pth --run $RUN
    jvs checkpoint "ModelB-run-$RUN" --tag model-b
done

# Compare results
jvs checkpoint list | grep model-a | wc -l  # Count A experiments
jvs checkpoint list | grep model-b | wc -l  # Count B experiments
```

---

## Integration with Agent Frameworks

### LangChain Integration

```python
#!/usr/bin/env python3
# agent_runner.py

import subprocess
import sys
import json

def run_agent_experiment(prompt: str, config: dict):
    """Run agent experiment with JVS checkpoint"""

    # Reset to baseline
    subprocess.run(['jvs', 'restore', 'baseline'], check=True)

    # Run agent
    from langchain.agents import initialize_agent
    # ... your LangChain code ...

    result = agent.run(prompt)

    # Checkpoint result
    snapshot_note = f"LangChain: {prompt[:50]}... -> {result['status']}"
    subprocess.run([
        'jvs', 'checkpoint', snapshot_note,
        '--tag', 'langchain',
        '--tag', result['status']
    ], check=True)

    return result

if __name__ == '__main__':
    result = run_agent_experiment(sys.argv[1], {})
    print(json.dumps(result, indent=2))
```

### AutoGen Integration

```python
#!/usr/bin/env python3
# autogen_runner.py

import subprocess
import autogen

def run_autogen_experiment(task: str):
    """Run AutoGen experiment with JVS checkpoint"""

    # Reset to baseline
    subprocess.run(['jvs', 'restore', 'baseline'], check=True)

    # Define agents
    assistant = autogen.AssistantAgent(...)
    user_proxy = autogen.UserProxyAgent(...)

    # Run conversation
    result = user_proxy.initiate_chat(
        assistant,
        message=task
    )

    # Checkpoint result
    snapshot_note = f"AutoGen: {task[:50]}... -> {result['status']}"
    subprocess.run([
        'jvs', 'checkpoint', snapshot_note,
        '--tag', 'autogen',
        '--tag', result['status']
    ], check=True)

if __name__ == '__main__':
    run_autogen_experiment("Analyze this dataset and generate insights")
```

### OpenAI Agents Integration

```python
#!/usr/bin/env python3
# openai_runner.py

import subprocess
from openai import OpenAI

def run_openai_agent(task: str):
    """Run OpenAI agent with JVS checkpoint"""

    # Reset to baseline
    subprocess.run(['jvs', 'restore', 'baseline'], check=True)

    # Run agent
    client = OpenAI()
    response = client.chat.completions.create(
        model="gpt-4",
        messages=[{"role": "user", "content": task}]
    )

    # Save result
    with open('result.json', 'w') as f:
        json.dump(response.choices[0].message.content, f)

    # Checkpoint result
    subprocess.run([
        'jvs', 'checkpoint',
        f"OpenAI: {task[:50]}...",
        '--tag', 'openai',
        '--tag', 'completed'
    ], check=True)

if __name__ == '__main__':
    run_openai_agent("Write a Python function to sort a list")
```

---

## Advanced Workflows

### Workflow 1: Hyperparameter Sweep

```bash
#!/bin/bash
# hyperparam_sweep.sh

# Baseline model
jvs checkpoint "Hyperparam sweep baseline" --tag hps-baseline

# Sweep learning rates
for LR in 0.001 0.01 0.1; do
    for BATCH in 16 32 64; do
        RUN_ID="lr-${LR}-batch-${BATCH}"

        # Reset to baseline
        jvs restore hps-baseline

        # Run with hyperparameters
        python agent.py \
            --learning-rate $LR \
            --batch-size $BATCH \
            --output results/$RUN_ID.json

        # Checkpoint result
        jvs checkpoint \
            "HP sweep: LR=$LR, BATCH=$BATCH" \
            --tag hps \
            --tag "lr-$LR" \
            --tag "batch-$BATCH"
    done
done

# Analyze results
python analyze_hps.py
```

### Workflow 2: Progressive Refinement

```bash
#!/bin/bash
# Progressive refinement: each experiment builds on previous

# Start with baseline
jvs restore baseline

# Stage 1: Basic functionality
python agent.py --stage 1
jvs checkpoint "Stage 1 complete" --tag stage1

# Stage 2: Build on stage 1
python agent.py --stage 2
jvs checkpoint "Stage 2 complete" --tag stage2

# Stage 3: Build on stage 2
python agent.py --stage 3
jvs checkpoint "Stage 3 complete" --tag stage3

# If stage 3 fails, go back to stage 2
jvs restore stage2
# Try different approach...
```

### Workflow 3: Fault Injection Testing

```bash
#!/bin/bash
# Test agent behavior under various failure conditions

FAULTS=(
    "network_timeout"
    "api_error"
    "missing_file"
    "invalid_input"
    "resource_limit"
)

for FAULT in "${FAULTS[@]}"; do
    # Reset to baseline
    jvs restore baseline

    # Inject fault
    python agent.py --inject-fault $FAULT --output results/$FAULT.json

    # Checkpoint result
    jvs checkpoint "Fault test: $FAULT" --tag fault-test --tag "$FAULT"
done
```

---

## Best Practices

### 1. Always Restore Before Each Experiment

```bash
# Good: Clean state each time
for RUN in {1..100}; do
    jvs restore baseline
    python agent.py --run $RUN
    jvs checkpoint "Run $RUN"
done

# Bad: State bleeds between runs
for RUN in {1..100}; do
    python agent.py --run $RUN  # Previous state affects this run!
    jvs checkpoint "Run $RUN"
done
```

### 2. Use Descriptive Checkpoint Notes

```bash
# Good: Includes parameters and results
jvs checkpoint \
    "GPT-4 temp=0.7 max_tokens=1000 -> success:95% confidence" \
    --tag gpt4 --tag temp-0.7

# Bad: Generic
jvs checkpoint "experiment 123"
```

### 3. Tag by Experiment Type

```bash
# Tag by agent type
jvs checkpoint "..." --tag langchain --tag research
jvs checkpoint "..." --tag autogen --tag coding
jvs checkpoint "..." --tag openai --tag chat

# Find all experiments for a type
jvs checkpoint list | grep langchain
```

### 4. Regular Verification

```bash
# Verify baseline integrity
jvs verify baseline

# Verify all agent checkpoints
jvs verify --all
```

---

## Performance Tips

### Use Supported JuiceFS for O(1) Metadata-Clone Checkpoints

```bash
# Check which engine you're using
jvs info --json
jvs capability /mnt/juicefs
```

### Keep Large Generated Data Out of the Workspace

```bash
# Put cache/output directories outside the JVS workspace when they should not
# be part of checkpoints.
export AGENT_CACHE=/mnt/juicefs/agent-cache
jvs checkpoint "Code and config update"
```

### Parallel Workspaces for Concurrent Experiments

```bash
# Create 10 workspaces for parallel execution
for i in {1..10}; do
    jvs fork exp-$i
done

# Run experiments in parallel
for i in {1..10}; do
    (
        cd "$(jvs workspace path exp-$i)"
        jvs restore baseline
        python agent.py --run $i
    ) &
done
wait
```

---

## Troubleshooting

### Problem: Experiments have different results

**Solution:** Always restore to baseline before each run
```bash
jvs restore baseline
```

### Problem: Checkpoints are slow

**Solution:** Verify juicefs-clone engine
```bash
jvs doctor --json | jq '.engine'
# Should be: "juicefs-clone"
```

### Problem: Can't find specific experiment

**Solution:** Use tags and grep
```bash
jvs checkpoint list | grep autogen
jvs checkpoint list | grep "network_timeout"
```

---

## Integration Examples

### Airflow DAG for Agent Experiments

```python
# airflow_dags/agent_experiments.py

from airflow import DAG
from airflow.operators.bash import BashOperator
from datetime import datetime

with DAG('agent_experiments', start_date=datetime(2024, 1, 1)) as dag:
    # Restore baseline
    restore = BashOperator(
        task_id='restore_baseline',
        bash_command='cd /mnt/juicefs/agent-sandbox/main && jvs restore baseline'
    )

    # Run agent
    run_agent = BashOperator(
        task_id='run_agent',
        bash_command='cd /mnt/juicefs/agent-sandbox/main && python agent.py'
    )

    # Create checkpoint
    checkpoint = BashOperator(
        task_id='create_snapshot',
        bash_command='cd /mnt/juicefs/agent-sandbox/main && jvs checkpoint "Airflow run {{ ds_nodash }}" --tag airflow'
    )

    restore >> run_agent >> checkpoint
```

---

## Next Steps

- Read [GAME_DEV_QUICKSTART.md](game_dev_quickstart.md) for game workflows
- Read [ETL_PIPELINE_QUICKSTART.md](etl_pipeline_quickstart.md) for data workflows
- Join the community: [GitHub Discussions](https://github.com/jvs-project/jvs/discussions)
