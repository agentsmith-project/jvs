# JVS Quick Start: Data ETL Pipelines

**Version:** v0 public contract
**Last Updated:** 2026-02-23

---

## Overview

This guide helps data engineers use JVS for versioning large datasets in ETL pipelines. On supported JuiceFS mounts, JVS provides O(1) metadata-clone checkpoints for TB-scale data; fallback engines remain available for other filesystems.

---

## Why JVS for ETL Pipelines?

| Problem | Git + Git LFS | DVC | JVS |
|---------|---------------|-----|-----|
| TB-scale datasets | Doesn't scale | Complex cache management | O(1) metadata clone on supported JuiceFS |
| Pipeline integration | Manual | Requires DVC CLI | Simple CLI integration |
| Data lineage | Manual tracking | DVC metrics | Checkpoint notes + tags |
| Reproducibility | Difficult | Good | Excellent |

**Key Benefit:** Checkpoint entire datasets with engine behavior made explicit; supported JuiceFS mounts use metadata clone, while copy fallback scales with payload size.

---

## Prerequisites

1. **Supported JuiceFS mounted** (recommended for O(1) performance)
2. **JVS installed**
3. **ETL pipeline** (Python, SQL, or any tool)

---

## Quick Start (5 Minutes)

### Step 1: Initialize Data Workspace

```bash
# Navigate to your JuiceFS mount
cd /mnt/juicefs/data-lake

# Initialize JVS repository
jvs init etl-pipeline
cd etl-pipeline/main

# Create initial structure
mkdir -p raw/ processed/ features/ models/
```

### Step 2: Create Baseline Checkpoint

```bash
# Copy initial data (or start empty)
cp -r /source/data/* raw/

# Create baseline
jvs checkpoint "Initial raw data import" --tag baseline --tag raw
```

### Step 3: ETL Pipeline with Checkpoints

```bash
#!/bin/bash
# Simple ETL pipeline

# Stage 1: Ingest raw data
echo "Ingesting raw data..."
cp /source/new_data/* raw/
jvs checkpoint "Raw ingestion: 2024-02-23" --tag raw --tag $(date +%Y-%m-%d)

# Stage 2: Process data
echo "Processing data..."
python process.py --input raw/ --output processed/
jvs checkpoint "Processed data: cleaned, normalized" --tag processed --tag $(date +%Y-%m-%d)

# Stage 3: Feature engineering
echo "Building features..."
python build_features.py --input processed/ --output features/
jvs checkpoint "Features: v2.0 schema" --tag features --tag $(date +%Y-%m-%d)

# Stage 4: Train model
echo "Training model..."
python train.py --input features/ --output models/model.pkl
jvs checkpoint "Model trained on $(date +%Y-%m-%d)" --tag model --tag $(date +%Y-%m-%d)
```

---

## Common Patterns

### Pattern 1: Daily ETL Pipeline

```bash
#!/bin/bash
# daily_etl.sh

set -e

TODAY=$(date +%Y-%m-%d)
cd /mnt/juicefs/data-lake/etl-pipeline/main

# Reset to baseline
jvs restore baseline

# 1. Extract: Copy from source
echo "Extracting data for $TODAY..."
aws s3 sync s3://data-lake/raw/$TODAY/ raw/

# 2. Transform: Clean and process
echo "Transforming data..."
python transform.py --input raw/ --output processed/ --date $TODAY

# 3. Load: Load to warehouse
echo "Loading to warehouse..."
python load_to_warehouse.py --input processed/ --date $TODAY

# 4. Checkpoint the completed pipeline
jvs checkpoint "ETL pipeline complete: $TODAY" --tag etl --tag $TODAY

# Send notification
echo "ETL pipeline completed for $TODAY"
```

### Pattern 2: Incremental Processing

```bash
#!/bin/bash
# Incremental updates

LATEST_CHECKPOINT=$(jvs checkpoint list | grep processed | head -1 | awk '{print $1}')

# Restore to last processed state
jvs restore $LATEST_CHECKPOINT

# Add new data
cat /source/incremental_*.csv >> raw/new_data.csv

# Process only new data
python process_incremental.py --input raw/new_data.csv --output processed/incremental/

# Create a checkpoint after the incremental stage completes
jvs checkpoint "Incremental update: $(date +%Y-%m-%d)" --tag incremental
```

### Pattern 3: Data Quality Checks with Rollback

```bash
#!/bin/bash
# ETL with quality checks and automatic rollback

cd /mnt/juicefs/data-lake/etl-pipeline/main

# Restore baseline
jvs restore baseline

# Run ETL
python etl_pipeline.py

# Run quality checks
echo "Running quality checks..."
python quality_check.py --input processed/

if [ $? -eq 0 ]; then
    # Quality check passed - checkpoint
    jvs checkpoint "ETL + QC passed: $(date +%Y-%m-%d)" --tag etl --tag passed
    echo "ETL pipeline completed successfully"
else
    # Quality check failed - restore and alert
    jvs restore baseline
    echo "ETL pipeline failed quality check - restored to baseline"
    # Send alert...
    exit 1
fi
```

---

## Airflow Integration

### Simple Airflow DAG

```python
# airflow_dags/jvs_etl.py

from airflow import DAG
from airflow.operators.bash import BashOperator
from datetime import datetime, timedelta

default_args = {
    'owner': 'data-team',
    'depends_on_past': False,
    'start_date': datetime(2024, 1, 1),
    'email': ['data-team@example.com'],
    'email_on_failure': True,
}

JVS_PATH = '/mnt/juicefs/data-lake/etl-pipeline/main'

with DAG('jvs_etl_pipeline', default_args=default_args, schedule_interval='@daily') as dag:

    # Restore to baseline
    restore_baseline = BashOperator(
        task_id='restore_baseline',
        bash_command=f'cd {JVS_PATH} && jvs restore baseline'
    )

    # Extract data
    extract = BashOperator(
        task_id='extract',
        bash_command=f'cd {JVS_PATH} && python extract.py --date {{ ds_nodash }}'
    )

    # Checkpoint after extract
    snapshot_extract = BashOperator(
        task_id='snapshot_extract',
        bash_command=f'cd {JVS_PATH} && jvs checkpoint "Extract complete: {{{{ ds_nodash }}}}" --tag extract --tag {{{{ ds_nodash }}}}'
    )

    # Transform data
    transform = BashOperator(
        task_id='transform',
        bash_command=f'cd {JVS_PATH} && python transform.py --date {{ ds_nodash }}'
    )

    # Checkpoint after transform
    snapshot_transform = BashOperator(
        task_id='snapshot_transform',
        bash_command=f'cd {JVS_PATH} && jvs checkpoint "Transform complete: {{{{ ds_nodash }}}}" --tag transform --tag {{{{ ds_nodash }}}}'
    )

    # Load data
    load = BashOperator(
        task_id='load',
        bash_command=f'cd {JVS_PATH} && python load.py --date {{ ds_nodash }}'
    )

    # Final checkpoint
    snapshot_final = BashOperator(
        task_id='snapshot_final',
        bash_command=f'cd {JVS_PATH} && jvs checkpoint "ETL complete: {{{{ ds_nodash }}}}" --tag etl --tag {{{{ ds_nodash }}}}'
    )

    # Define dependencies
    restore_baseline >> extract >> snapshot_extract >> transform >> snapshot_transform >> load >> snapshot_final
```

### Custom Airflow Operator

```python
# airflow_plugins/operators/jvs_operator.py

from airflow.models.baseoperator import BaseOperator
from airflow.utils.decorators import apply_defaults
import subprocess
import json

class JVSSnapshotOperator(BaseOperator):
    """Airflow operator to create JVS checkpoint"""

    @apply_defaults
    def __init__(self, jvs_path, note, tags=None, **kwargs):
        super().__init__(**kwargs)
        self.jvs_path = jvs_path
        self.note = note
        self.tags = tags or []

    def execute(self, context):
        # Build JVS command
        cmd = ['jvs', '--json', 'checkpoint', self.note]
        for tag in self.tags:
            cmd.extend(['--tag', tag])

        # Execute JVS checkpoint
        result = subprocess.run(
            cmd,
            cwd=self.jvs_path,
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            raise Exception(f"JVS checkpoint failed: {result.stderr}")

        # Return checkpoint ID for XCom
        envelope = json.loads(result.stdout)
        return envelope['data']['checkpoint_id']

# Usage in DAG
from airflow_plugins.operators.jvs_operator import JVSSnapshotOperator

checkpoint = JVSSnapshotOperator(
    task_id='create_snapshot',
    jvs_path='/mnt/juicefs/data-lake/etl-pipeline/main',
    note='ETL complete: {{ ds_nodash }}',
    tags=['etl', '{{ ds_nodash }}']
)
```

---

## ML Pipeline Integration

### MLflow + JVS

```python
#!/usr/bin/env python3
# ml_pipeline.py

import subprocess
import mlflow
import mlflow.sklearn
from sklearn.ensemble import RandomForestClassifier

def run_ml_pipeline_with_jvs():
    """Run ML pipeline with JVS checkpoints"""

    # Reset to baseline
    subprocess.run(['jvs', 'restore', 'baseline'], check=True)

    # Start MLflow run
    with mlflow.start_run():
        # Load data
        X_train, y_train = load_data('processed/train.csv')
        X_test, y_test = load_data('processed/test.csv')

        # Train model
        model = RandomForestClassifier(n_estimators=100)
        model.fit(X_train, y_train)

        # Log parameters and metrics
        mlflow.log_params({'n_estimators': 100})
        mlflow.log_metrics({'accuracy': model.score(X_test, y_test)})

        # Save model
        model_path = 'models/model.pkl'
        mlflow.sklearn.save_model(model, model_path)

        # Create JVS checkpoint with MLflow run info
        run_id = mlflow.active_run().info.run_id
        subprocess.run([
            'jvs', 'checkpoint',
            f'Model trained: MLflow run {run_id[:8]}, accuracy={model.score(X_test, y_test):.3f}',
            '--tag', 'mlflow',
            '--tag', 'model',
            '--tag', f'run-{run_id[:8]}'
        ], check=True)

if __name__ == '__main__':
    run_ml_pipeline_with_jvs()
```

---

## Best Practices

### 1. Checkpoint After Each Pipeline Stage

```bash
# Good: Checkpoint after each stage
extract_data && jvs checkpoint "Extract done" --tag extract
transform_data && jvs checkpoint "Transform done" --tag transform
load_data && jvs checkpoint "Load done" --tag load

# Bad: Only checkpoint at the end (hard to debug failures)
extract_data && transform_data && load_data && jvs checkpoint "ETL done"
```

### 2. Tag by Date and Pipeline Stage

```bash
# Tag with date
jvs checkpoint "..." --tag $(date +%Y-%m-%d)

# Tag with stage
jvs checkpoint "..." --tag extract --tag processed

# Find all checkpoints for a date
jvs checkpoint list | grep 2024-02-23
```

### 3. Use Meaningful Checkpoint Notes

```bash
# Good: Includes context
jvs checkpoint "Customer data: added 50k new rows, cleaned null emails, normalized phone numbers"

# Bad: Generic
jvs checkpoint "Data updated"
```

### 4. Regular Verification

```bash
# Verify data integrity
jvs verify --all

# Verify specific checkpoint
jvs verify abc123
```

---

## Advanced Workflows

### Workflow 1: A/B Test Different Pipelines

```bash
# Pipeline A: Current approach
jvs fork pipeline-a
cd "$(jvs workspace path pipeline-a)"
jvs restore baseline
python pipeline_a.py
jvs checkpoint "Pipeline A: accuracy=0.85" --tag pipeline-a --tag baseline

# Pipeline B: Experimental approach
cd ../../main
jvs fork pipeline-b
cd "$(jvs workspace path pipeline-b)"
jvs restore baseline
python pipeline_b.py
jvs checkpoint "Pipeline B: accuracy=0.87" --tag pipeline-b --tag experimental
```

### Workflow 2: Schema Migration Tracking

```bash
#!/bin/bash
# Track schema changes with JVS

# Schema v1
jvs checkpoint "Schema v1: customer_id, name, email" --tag schema --tag v1

# Apply migration
python migrate_v1_to_v2.py

# Schema v2
jvs checkpoint "Schema v2: added phone, address fields" --tag schema --tag v2

# Apply another migration
python migrate_v2_to_v3.py

# Schema v3
jvs checkpoint "Schema v3: normalized phone format, added created_at" --tag schema --tag v3

# To rollback to v2:
jvs restore v2
```

### Workflow 3: Multi-Region Data Sync

```bash
#!/bin/bash
# Sync data checkpoints across regions

# Create checkpoint in primary region
cd /mnt/juicefs-primary/data/main
jvs checkpoint "Daily data sync: $(date +%Y-%m-%d)" --tag sync --tag $(date +%Y-%m-%d)
CHECKPOINT_ID=$(jvs checkpoint list --json | jq -r '.data[0].checkpoint_id')

# Sync JVS repository data to secondary region without runtime state
rsync -avz \
  --exclude '.jvs/locks/**' \
  --exclude '.jvs/intents/**' \
  --exclude '.jvs/gc/*.json' \
  /mnt/juicefs-primary/data/ /mnt/juicefs-secondary/data/

# In secondary region, verify and use checkpoint
cd /mnt/juicefs-secondary/data/main
jvs verify $CHECKPOINT_ID
jvs restore $CHECKPOINT_ID
```

---

## Performance Tips

### Keep Large Generated Data Out of the Workspace

```bash
# Keep caches outside the workspace when they should not be checkpointed.
export ETL_CACHE=/mnt/juicefs/etl-cache
jvs checkpoint "Raw data update"
jvs checkpoint "Features update"
```

### Schedule GC During Off-Peak Hours

```bash
# Run GC cron job at 3 AM daily
0 3 * * * cd /mnt/juicefs/data/main && jvs gc plan && jvs gc run --plan-id <plan-id>
```

### Use Tags for Efficient Queries

```bash
# Find all checkpoints for a specific date
jvs checkpoint list | grep 2024-02-23

# Find all failed pipeline runs
jvs checkpoint list | grep "failed"

# Find all model training checkpoints
jvs checkpoint list | grep model
```

---

## Troubleshooting

### Problem: Out of space

**Solution:** Run garbage collection
```bash
jvs gc plan
jvs gc run --plan-id <plan-id>
```

### Problem: Can't find data for specific date

**Solution:** Use date tags
```bash
jvs checkpoint list | grep 2024-02-23
```

### Problem: Verify fails

**Solution:** Data may have been modified outside JVS
```bash
# Check what changed
find . -newer .jvs/snapshots/abc123

# Restore from checkpoint
jvs restore abc123
```

---

## Integration Examples

### dbt + JVS

```bash
#!/bin/bash
# dbt pipeline with JVS checkpoints

# Restore baseline
jvs restore baseline

# Run dbt
dbt run

# Checkpoint dbt artifacts
jvs checkpoint "dbt run complete: $(date +%Y-%m-%d)" --tag dbt --tag $(date +%Y-%m-%d)
```

### Spark + JVS

```python
#!/usr/bin/env python3
# spark_pipeline.py

import subprocess
from pyspark.sql import SparkSession

def run_spark_with_jvs():
    """Run Spark job with JVS checkpoint"""

    # Restore baseline
    subprocess.run(['jvs', 'restore', 'baseline'], check=True)

    # Run Spark
    spark = SparkSession.builder.appName("ETL").getOrCreate()

    # Read data
    df = spark.read.parquet("raw/data")

    # Transform
    df_transformed = df.groupBy("column").count()

    # Write output
    df_transformed.write.parquet("processed/output")

    # Checkpoint results
    subprocess.run([
        'jvs', 'checkpoint',
        f'Spark job complete: {df_transformed.count()} rows processed',
        '--tag', 'spark',
        '--tag', 'etl'
    ], check=True)

if __name__ == '__main__':
    run_spark_with_jvs()
```

---

## Next Steps

- Read [GAME_DEV_QUICKSTART.md](game_dev_quickstart.md) for game workflows
- Read [AGENT_SANDBOX_QUICKSTART.md](agent_sandbox_quickstart.md) for agent workflows
- Join the community: [GitHub Discussions](https://github.com/agentsmith-project/jvs/discussions)
