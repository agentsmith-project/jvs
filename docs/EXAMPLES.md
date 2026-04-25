# JVS Example Workflows & Use Cases

**Version:** v7.0
**Last Updated:** 2026-02-23

---

## Overview

This document provides practical examples of using JVS in real-world scenarios. Each workflow includes step-by-step commands and explanations.

---

## Example 1: Machine Learning Experiment Workflow

**Use Case:** Data scientist tracking ML experiments with large datasets

### Scenario

You're training models on a 50GB dataset. You want to:
- Checkpoint before each experiment run
- Compare results across experiments
- Roll back when experiments fail
- Keep "good" experiments tagged

### Setup

```bash
# Initialize JVS repository on JuiceFS mount
cd /mnt/juicefs
jvs init ml-experiments
cd ml-experiments/main

# Initial setup
cp -r /data/dataset .
python setup_environment.py
jvs checkpoint "Initial setup with dataset" --tag baseline

# Verify
jvs checkpoint list
```

### Daily Experiment Loop

```bash
# Experiment 1: Try new model architecture
vim train.py          # Edit model architecture
jvs checkpoint "Exp1: ResNet50 architecture" --tag exp1 --tag model

# Run experiment
python train.py --epochs 100

# Experiment failed? Roll back instantly
jvs restore exp1
vim train.py          # Fix the issue
jvs checkpoint "Exp1: ResNet50 with fix" --tag exp1 --tag model

# Experiment 2: Try different hyperparameters
vim train.py          # Change learning rate
jvs checkpoint "Exp2: LR 0.001" --tag exp2 --tag hyperparam
python train.py --lr 0.001

# Compare experiments
jvs checkpoint list | grep exp
```

### Recovering Failed Runs

```bash
# Something went wrong during training
# Training crashed after 10 hours, dataset is corrupted

# Option 1: Roll back to last good checkpoint
jvs restore exp1      # O(1) instant recovery!

# Option 2: Create a branch to investigate
jvs fork investigation
cd "$(jvs workspace path investigation)"
# Debug in isolation without affecting main
```

### Tagging Strategy

```bash
# Tag release-ready experiments
jvs checkpoint "v1.0 candidate" --tag stable --tag v1.0

# Find all stable experiments
jvs checkpoint list | grep stable

# Restore to latest stable
jvs restore --latest-tag stable
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Instant rollback | O(1) restore regardless of dataset size |
| Experiment tracking | Tags + notes organize experiments |
| Safe experimentation | Fork branches without affecting main |
| Reproducibility | Exact workspace state captured |

---

## Example 2: Development Environment Versioning

**Use Case:** Developer managing multiple service versions

### Scenario

You're developing a microservice with:
- Multiple developers working on different features
- Need to test production bugs locally
- Want to switch between feature branches instantly

### Setup

```bash
# Initialize repo
jvs init myservice
cd myservice/main

# Import codebase
git clone https://github.com/company/myservice.git .
jvs checkpoint "Initial import" --tag v1.0

# Verify
jvs doctor --strict
jvs verify --all
```

### Feature Branch Workflow

```bash
# Developer A: Start feature A
vim src/handler.go    # Make changes
jvs checkpoint "Feature A: Add authentication" --tag feature-a --tag wip

# Developer B: Start feature B (different workspace)
jvs fork feature-b
cd "$(jvs workspace path feature-b)"
vim src/handler.go    # Make different changes
jvs checkpoint "Feature B: Add caching" --tag feature-b --tag wip

# Developer A: Continue work
cd ../../main
jvs restore feature-a
vim src/handler.go    # More changes
jvs checkpoint "Feature A: Add auth tests" --tag feature-a

# Both developers work independently with `cd` between directories
```

### Production Bug Investigation

```bash
# Production bug report came in for v1.0

# Create bugfix branch from production checkpoint
jvs restore v1.0
jvs fork bugfix-1234
cd "$(jvs workspace path bugfix-1234)"

# Fix the bug
vim src/handler.go
go test ./...
jvs checkpoint "Fix: Handle null pointer in handler" --tag bugfix --tag v1.0.1

# Verify fix works
go test -run TestNullPointer
```

### Hotfix to Production

```bash
# Emergency hotfix needed

# 1. Rollback production environment to last known good
ssh production-server
cd /app
jvs restore --latest-tag stable
systemctl restart myservice

# 2. Create hotfix branch
jvs fork hotfix-critical
cd "$(jvs workspace path hotfix-critical)"

# 3. Apply fix
vim src/handler.go
jvs checkpoint "Hotfix: Critical memory leak" --tag hotfix --tag stable

# 4. Deploy hotfix
# ... deployment process ...
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Parallel development | Fork workspaces, no conflicts |
| Instant context switch | `cd` + `jvs restore` |
| Bug investigation | Reproduction environment preserved |
| Production rollback | O(1) restore minimizes downtime |

---

## Example 3: Backup and Recovery Scenarios

**Use Case:** System administrator protecting critical workspace

### Scenario

You have a critical workspace that must be:
- Backed up regularly
- Recoverable in case of corruption
- Migratable to new storage

### Regular Backup Strategy

```bash
# Initialize on production server
cd /production/workspace
jvs init critical-app
cd critical-app/main

# Automated daily checkpoints (via cron)
# 0 2 * * * cd /production/workspace/critical-app/main && jvs checkpoint "Daily backup $(date +%Y-%m-%d)" --tag daily

# Weekly tagged checkpoints
# 0 2 * * 0 cd /production/workspace/critical-app/main && jvs checkpoint "Weekly backup $(date +%Y-%m-%d)" --tag weekly
```

### Checkpoint Retention with GC

```bash
# Review plan
jvs gc plan
# Output shows what will be deleted

# Execute when ready
PLAN_ID=$(jvs gc plan | grep "Plan ID:" | cut -d: -f2)
jvs gc run --plan-id "$PLAN_ID"
```

### Cross-Machine Backup

```bash
# Backup .jvs/ directory (metadata only, payload handled by JuiceFS)
juicefs sync /production/workspace/critical-app/.jvs/ \
    /backup/location/critical-app/.jvs/ \
    --exclude '.jvs/intents/**' \
    --exclude '.jvs/index.sqlite'

# Restore on different machine
# 1. Mount JuiceFS at new location
# 2. Copy .jvs/ metadata
juicefs sync /backup/location/critical-app/.jvs/ \
    /new/location/critical-app/.jvs/

# 3. Rebuild index and repair
cd /new/location/critical-app
jvs doctor --strict --repair-runtime
jvs verify --all
```

### Disaster Recovery

```bash
# Scenario: Main workspace corrupted, but .jvs/ is intact

# 1. Verify repository health
jvs doctor --strict

# 2. Identify last good checkpoint
jvs checkpoint list
# Let's say last good checkpoint is abc123...

# 3. Restore main workspace
cd main
jvs restore abc123

# 4. Verify integrity
jvs verify abc123

# Scenario: Entire .jvs/ directory lost (but backup exists)

# 1. Restore metadata from backup
juicefs sync /backup/.jvs/ /workspace/.jvs/

# 2. Rebuild runtime state
cd /workspace
jvs doctor --strict --repair-runtime
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Incremental backup | JuiceFS handles data, JVS handles metadata |
| Point-in-time recovery | Restore any checkpoint in O(1) |
| Space-efficient | GC with retention policies |
| Disaster recovery | Separated metadata/payload |
| Verification integrity | Two-layer verification detects corruption |

---

## Example 4: CI/CD Pipeline Integration

**Use Case:** DevOps engineer integrating JVS into CI/CD

### GitHub Actions Example

```yaml
name: Test and Checkpoint

on: [push]

jobs:
  test:
    runs-on: juicefs-mounted-runner
    steps:
      - uses: actions/checkout@v3

      - name: Setup JVS
        run: |
          go install github.com/jvs-project/jvs@latest

      - name: Create Test Checkpoint
        run: |
          cd /workspace
          jvs init ci-workspace || true
          cd ci-workspace/main
          jvs checkpoint "Pre-test checkpoint"

      - name: Run Tests
        run: |
          cd /workspace/ci-workspace/main
          go test ./... -cover

      - name: Checkpoint on Success
        if: success()
        run: |
          cd /workspace/ci-workspace/main
          jvs checkpoint "Tests passed - ${{ github.sha }}" --tag ci --tag passed

      - name: Checkpoint on Failure
        if: failure()
        run: |
          cd /workspace/ci-workspace/main
          jvs checkpoint "Tests failed - ${{ github.sha }}" --tag ci --tag failed
```

### Jenkins Pipeline Example

```groovy
pipeline {
    agent { label 'juicefs' }

    stages {
        stage('Setup') {
            steps {
                sh '''
                    cd /workspace
                    jvs init jenkins-build || true
                    cd jenkins-build/main
                    jvs checkpoint "Pre-build: ${env.BUILD_NUMBER}"
                '''
            }
        }

        stage('Build') {
            steps {
                sh 'cd /workspace/jenkins-build/main && make build'
            }
        }

        stage('Test') {
            steps {
                sh 'cd /workspace/jenkins-build/main && make test'
            }
        }

        stage('Package') {
            steps {
                sh 'cd /workspace/jenkins-build/main && make package'
            }
        }
    }

    post {
        success {
            sh '''
                cd /workspace/jenkins-build/main
                jvs checkpoint "Build ${BUILD_NUMBER} passed" --tag jenkins --tag passed
            '''
        }
        failure {
            sh '''
                cd /workspace/jenkins-build/main
                jvs checkpoint "Build ${BUILD_NUMBER} failed" --tag jenkins --tag failed
            '''
        }
    }
}
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Reproducible builds | Exact workspace state captured |
| Debug failed builds | Restore to checkpoint to investigate |
| Build artifacts | Workspace state preserved |
| Audit trail | Every build tagged and tracked |

---

## Example 5: Agent Workflow Sandboxing

**Use Case:** AI agent requiring deterministic, reproducible workspace states

### Scenario

AI agent that:
- Runs experiments that modify files
- Needs clean state between runs
- Tracks which checkpoints produced which results

### Agent Workflow

```bash
# Initialize agent workspace
jvs init agent-sandbox
cd agent-sandbox/main

# Set up initial environment
cp -r /initial/code/* .
python install_dependencies.py
jvs checkpoint "Initial agent environment" --tag agent-env

# Agent execution loop
for RUN in {1..100}; do
    # Restore to clean state
    jvs restore agent-env

    # Agent makes modifications
    python agent.py --experiment $RUN

    # Capture result
    RESULT=$(cat output/result.txt)

    # Checkpoint result state
    jvs checkpoint "Agent run $RUN: $RESULT" --tag "run-$RUN" --tag agent

    # Collect results
    jvs checkpoint list --json | grep "run-$RUN"
done
```

### Deterministic Experiments

```bash
# Ensure exact same starting state for each experiment
jvs restore baseline

# Run experiment with fixed seed
python experiment.py --seed 42 --output results.txt

# Checkpoint immediately after (no other changes)
jvs checkpoint "Experiment with seed 42" --tag deterministic

# Later: exact reproduction
jvs restore "Experiment with seed 42"
python experiment.py --seed 42
# Results will be identical
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Deterministic state | Exact workspace restoration |
| Experiment tracking | Tag each run separately |
| Clean isolation | Restore to baseline between runs |
| Result reproducibility | Same checkpoint = same results |

---

## Example 6: Multi-Environment Management

**Use Case:** Platform engineer managing dev/staging/production environments

### Setup

```bash
# Single repo, multiple environments
jvs init platform-envs

# Create development environment
cd platform-envs/main
cp -r /envs/dev/* .
jvs checkpoint "Development environment v1" --tag dev --tag v1.0

# Create staging workspace
jvs fork staging
cd "$(jvs workspace path staging)"
cp -r /envs/staging/* .
jvs checkpoint "Staging environment v1" --tag staging --tag v1.0

# Create production workspace
jvs fork production
cd "$(jvs workspace path production)"
cp -r /envs/production/* .
jvs checkpoint "Production environment v1" --tag production --tag v1.0 --tag stable
```

### Environment Promotion

```bash
# Promote staging to production
cd "$(jvs workspace path staging)"
jvs restore staging      # Ensure clean staging state

# Apply changes
vim config/database.yaml
jvs checkpoint "Staging v1.1: Database update" --tag staging

# Test in staging
./run_tests.sh
# ... tests pass ...

# Promote to production
cd "$(jvs workspace path production)"
jvs restore stable
cp -r ../staging/* .
jvs checkpoint "Production v1.1: Database update" --tag production --tag stable
```

### Rollback Strategy

```bash
# Production issue detected
cd "$(jvs workspace path production)"

# Rollback to previous stable immediately
jvs restore --latest-tag stable
# O(1) rollback, minimal downtime

# Investigate in separate workspace
jvs fork investigation
cd "$(jvs workspace path investigation)"
jvs restore production  # Current problematic state
# Debug...

# Fix verified, promote fix
cd ../production
cp -r ../investigation/* .
jvs checkpoint "Production v1.1.1: Hotfix" --tag production --tag stable
```

### What You Achieve

| Benefit | How JVS Helps |
|---------|--------------|
| Environment isolation | Separate workspaces, no conflicts |
| Safe promotion | Test in staging, promote to production |
| Instant rollback | O(1) restore minimizes downtime |
| Version tracking | Tag each environment version |

---

## Common Patterns

### Pattern 1: Pre-Experiment Checkpoint

```bash
# Always checkpoint before making changes
jvs checkpoint "Before: $(date +%H:%M)"
# ... make changes ...
jvs checkpoint "After: $(date +%H:%M)"
```

### Pattern 2: Tag Naming Convention

```bash
# Use hierarchical tags
jvs checkpoint "Feature complete" --tag feature --tag auth --tag v2.0

# Find all auth-related work
jvs checkpoint list | grep auth

# Find all v2.0 work
jvs checkpoint list | grep v2.0
```

### Pattern 3: Verification First

```bash
# Always verify before important operations
jvs verify --all
jvs doctor --strict
# Only proceed if both pass
```

### Pattern 4: Fork Before Risky Changes

```bash
# Always fork workspace for experimental changes
jvs fork experiment
cd "$(jvs workspace path experiment)"
# ... risky changes ...
# Main workspace remains safe
```

### Pattern 5: Tagged Checkpoints for Milestones

```bash
# Tag releases, milestones, verified states
jvs checkpoint "v1.0.0 release" --tag release --tag v1.0 --tag stable
jvs checkpoint "Model accuracy 95%" --tag milestone --tag 95-percent
```

---

## Tips and Best Practices

### DO ✅

- **Checkpoint before risky changes** - Easy rollback
- **Use meaningful notes** - `jvs checkpoint list` will thank you
- **Tag important checkpoints** - `stable`, `release`, `baseline`
- **Run `jvs doctor` periodically** - Catch issues early
- **Fork for experiments** - Keep main clean

### DON'T ❌

- **Don't** manually edit `.jvs/` - Let JVS manage it
- **Don't** ignore current differs from latest warnings - Understand what it means
- **Don't** checkpoint too frequently - Think semantic units
- **Don't** forget to tag important checkpoints - Makes recovery easier
- **Don't** skip verification before critical operations

---

## Related Documentation

- [QUICKSTART.md](QUICKSTART.md) - Getting started guide
- [ARCHITECTURE.md](ARCHITECTURE.md) - System design
- [13_OPERATION_RUNBOOK.md](13_OPERATION_RUNBOOK.md) - Operations guide
- [02_CLI_SPEC.md](02_CLI_SPEC.md) - Command reference

---

*These examples are based on real-world use cases. Have your own workflow? We'd love to hear it!*
