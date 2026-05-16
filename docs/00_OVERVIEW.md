# Overview

**Document set:** pre-GA collapsed active surface

JVS records real-folder state as save points, but the pre-GA active user
surface is intentionally collapsed. The current release-facing path is setup,
status/health inspection, and project clone/metadata operations that are still
needed while the internal direct AFSCP contract is validated.

Primary public path:

```bash
jvs init
jvs status
jvs doctor
jvs repo clone <target-folder> --dry-run
```

Core guarantees:

1. Workspaces are real folders, not virtualized folders.
2. JVS control data is never workspace content.
3. Internal direct AFSCP save points are immutable once published.
4. Direct AFSCP hot paths do not perform managed HOME content sync, content
   digesting, compression, size preflight, or recursive copying as a backup
   path.
5. Internal storage names do not define product vocabulary, commands,
   selectors, examples, or user mental models.

Current active specs:

- `02_CLI_SPEC.md`
- `contracts/jvs-afscp-direct-v1.md`
- `PRODUCT_PLAN.md`
- `ARCHITECTURE.md`
- `13_OPERATION_RUNBOOK.md`

Supporting non-release-facing redesign/reference:

- `21_SAVE_POINT_WORKSPACE_SEMANTICS.md`
