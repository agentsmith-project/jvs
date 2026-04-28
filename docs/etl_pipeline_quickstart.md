# JVS Quickstart: ETL Pipeline Folders

**Status:** Release-facing domain entry

Use this page as a short domain guide. The full user workflow is in
[docs/user/examples.md](user/examples.md).

## Why ETL Teams Use JVS

ETL runs produce folder state: raw files, processed files, feature outputs,
reports, and model artifacts. JVS lets you save those stage boundaries and
restore a known folder state before retrying.

JVS is not a scheduler, table catalog, SQL engine, or remote transport. Keep
using Airflow, dbt, Spark, Iceberg, Delta, or your storage platform for those
jobs.

## Setup

```bash
mkdir etl-workspace
cd etl-workspace
jvs init

mkdir -p raw processed features models reports
cp -r /source/initial-data/* raw/
jvs save -m "initial raw data"
jvs history
```

Copy the save point ID you want to use as the retry baseline.

## Stage Saves

```bash
TODAY=$(date +%Y-%m-%d)

python ingest.py --date "$TODAY" --output raw/
jvs save -m "raw ingestion $TODAY"

python transform.py --input raw/ --output processed/
jvs save -m "processed data $TODAY"

python build_features.py --input processed/ --output features/
jvs save -m "features $TODAY"

python train.py --input features/ --output models/model.pkl
jvs save -m "model trained $TODAY"
```

Use messages that include the stage name, dataset date, and any pipeline run
ID your orchestrator already has.

## Retry From A Baseline

Preview the restore first:

```bash
jvs restore <baseline-save> --discard-unsaved
```

If the plan is right, run the printed command:

```bash
jvs restore --run <plan-id>
```

Then rerun the failed stage and save the completed state:

```bash
python transform.py --input raw/ --output processed/
jvs save -m "processed data retry $TODAY"
```

## Restore One Output Path

If only one artifact needs recovery:

```bash
jvs restore --path reports/summary.parquet
jvs restore <save> --path reports/summary.parquet
jvs restore --run <plan-id>
```

Path restore keeps the impact smaller than replacing the whole folder.

## Automation Pattern

```bash
set -euo pipefail

BASELINE=$(jvs save -m "run baseline ${RUN_ID}" --json | jq -r '.data.save_point_id')

if ! python pipeline.py --run-id "$RUN_ID"; then
    PLAN=$(jvs restore "$BASELINE" --discard-unsaved --json | jq -r '.data.plan_id')
    jvs restore --run "$PLAN"
    exit 1
fi

jvs save -m "pipeline complete ${RUN_ID}"
```

## Inspect And Recover

```bash
jvs history --grep "$TODAY"
jvs view <save> reports/summary.parquet
jvs view close <view-id>
jvs doctor --strict
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
