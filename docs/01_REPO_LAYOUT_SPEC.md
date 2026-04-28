# Repository Layout Spec

**Status:** release-facing save point contract with internal runtime boundary

Public model:

- A workspace is a real folder managed by JVS.
- A save point is a saved copy of managed files plus immutable creation facts.
- JVS control data is never managed workspace payload.

## Public Layout Rules

- The adopted folder remains the user workspace.
- Workspace payload folders contain user files only.
- JVS control data must be excluded from save, restore, view, and cleanup
  payload operations.
- Runtime state is not portable across backup or migration.
- Public docs and JSON facades use save point and workspace terms, not storage
  directory names or storage-shaped metadata fields.

## Internal Storage Boundary

The implementation owns its control data layout. Directory names and metadata
fields are implementation facts and do not define user commands, selectors,
examples, restore targets, or cleanup policy.

## Portability Classes

Portable durable state:

- repository identity and format records
- save point descriptors and payload storage
- workspace metadata
- audit records
- durable cleanup tombstones when present

Non-portable runtime state:

- `.jvs/locks/`
- `.jvs/intents/`
- active `.jvs/gc/*.json` internal cleanup runtime plans
- temporary files created by interrupted operations

After a physical copy or storage migration, rebuild runtime state with:

```bash
jvs doctor --strict --repair-runtime
```

## Descriptor And Metadata Notes

Descriptors and workspace metadata are implementation-owned creation and
provenance records. Public facades must expose save point and workspace terms.

Tags, labels, messages, and annotations are metadata for display and search.
They are not public restore/view targets and do not provide cleanup protection
unless a separate public keep contract is promoted.
