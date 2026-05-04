# Overview

**Document set:** active save point contract

JVS saves real folders as save points. A user starts from a normal filesystem
folder, saves managed files, reviews history, opens read-only views, and
restores selected content when needed.

Primary public path:

```bash
jvs init
jvs save -m "baseline"
jvs history
jvs view <save> [path]
jvs restore <save>
```

Core guarantees:

1. Workspaces are real folders, not virtualized folders.
2. JVS control data is never saved as workspace content.
3. A save point is immutable once published.
4. Restore is preview-first and does not rewrite save point history.
5. Interrupted restore is closed by recovery status/resume/rollback.
6. Cleanup is review-first and must protect workspace history, open views,
   active recovery plans, active operations, and imported clone history.
7. Internal storage names do not define product vocabulary, commands,
   selectors, examples, or user mental models.

Current active specs:

- `02_CLI_SPEC.md`
- `06_RESTORE_SPEC.md`
- `PRODUCT_PLAN.md`
- `ARCHITECTURE.md`
- `13_OPERATION_RUNBOOK.md`

Supporting non-release-facing redesign/reference:

- `21_SAVE_POINT_WORKSPACE_SEMANTICS.md`
