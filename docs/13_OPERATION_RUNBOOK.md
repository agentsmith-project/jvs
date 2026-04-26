# Operation Runbook (v0)

## Daily checks
1. run `jvs doctor --strict`
2. run `jvs verify --all`

## Incident: verification failure
1. freeze writes for affected repo
2. run `jvs verify --all --json`
3. classify failure: checksum, payload hash
4. escalate tamper events and preserve evidence

## Incident: partial checkpoint artifacts
1. run `jvs doctor --strict --json`
2. run `jvs doctor --repair-list` and confirm only public runtime repairs are available
3. run `jvs doctor --strict --repair-runtime`, which may execute:
   - `clean_locks`: removes stale repository mutation locks
   - `clean_runtime_tmp`: removes stale JVS runtime temporary files
   - `clean_runtime_operations`: removes abandoned operation records
4. rerun `jvs doctor --strict` and `jvs verify --all`

## Incident: audit chain broken
1. run `jvs doctor --strict --json`, look for `E_AUDIT_CHAIN_BROKEN`
2. freeze writers and preserve `.jvs/audit/audit.jsonl` as evidence
3. investigate cause (truncation, manual edit, migration error)
4. restore from trusted backup or escalate for manual forensic remediation

## Migration runbook
1. freeze writers
2. doctor + verify pass on source
3. sync repository data while excluding active `.jvs/locks/`, `.jvs/intents/`,
   and `.jvs/gc/*.json` runtime state
4. run `jvs doctor --strict --repair-runtime` on destination, which:
   - `clean_locks`: removes stale repository mutation locks
   - `clean_runtime_tmp`: removes stale JVS runtime temporary files
   - `clean_runtime_operations`: removes abandoned operation records
5. run `jvs verify --all` and recovery drill

## GC runbook
1. run `jvs gc plan` and review `plan_id`
2. execute `jvs gc run --plan-id <id>`
3. if failure, inspect failed tombstones and retry safely
4. verify lineage/current integrity after gc batch
