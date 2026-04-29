# JVS Constitution

**Status:** active save point constitution

## Mission

JVS saves real folders as save points. It gives humans and agents a local,
filesystem-native way to save, inspect, view, restore, recover, and clean up
workspace state without a server or Git-style mental model.

## Principles

1. Real folders come first.
2. A workspace is a managed real folder.
3. A save point is immutable once published.
4. JVS control data is never workspace payload.
5. Restore copies content into a workspace; it does not rewrite history.
6. Destructive restore is preview-first and recovery-backed.
7. Cleanup is review-first and must protect workspace history, open views,
   active recovery plans, and active operations.
8. Labels/messages/tags are discovery metadata, not restore targets or cleanup
   protection.
9. Implementation names must not become user mental models.

## Public Vocabulary

| Term | Meaning |
| --- | --- |
| `folder` | The real filesystem directory where work happens. |
| `workspace` | JVS-managed real folder. |
| `save point` | Saved managed-file content plus immutable creation facts. |
| `save` | Create a save point from a workspace. |
| `history` | List and search save points. |
| `view` | Read-only materialization of a save point. |
| `restore` | Copy save point content into a workspace. |
| `unsaved changes` | Managed files differ from known source state or cannot be proven safe. |
| `cleanup` | Review-first deletion of unprotected save point storage. |
| `recovery plan` | Durable status/resume/rollback plan for interrupted restore. |

## Public Command Shape

Primary path:

```bash
jvs init
jvs save -m "baseline"
jvs history
jvs view <save> [path]
jvs restore <save>
```

Restore and recovery:

```bash
jvs restore <save>
jvs restore <save> --path <path>
jvs restore --run <plan-id>
jvs recovery status [plan]
jvs recovery resume <plan>
jvs recovery rollback <plan>
```

Workspace creation:

```bash
jvs workspace new <name> --from <save>
```

Health:

```bash
jvs status
jvs doctor --strict
```

Commands outside this visible surface are not the public user journey.

## Save Point Scope

A save point captures managed files from exactly one workspace. It excludes:

- JVS control data
- other workspaces
- runtime operation state
- restore/recovery/cleanup plans

## Restore Safety

- Preview changes no files.
- Run binds to a preview plan.
- Run revalidates expected target state before writing.
- Whole-workspace restore and path restore leave save point history unchanged.
- Unsaved changes are refused by default.
- `--save-first` and `--discard-unsaved` are explicit, mutually exclusive
  choices.
- Interrupted restore must produce a recovery plan or prove no files changed.

## Workspace Creation

`workspace new --from <save>` copies source content into a new real workspace.
It does not inherit the source history. The first save in the new workspace
records `started_from_save_point`.

## Cleanup

Cleanup must be two-stage:

```text
cleanup preview -> cleanup run
```

Cleanup protects workspace history, open views, active recovery plans, and
active operations.

## Implementation Boundary

Implementation storage paths, package names, and metadata fields are code
facts. Public docs, help, and release notes must use save point, workspace,
doctor, recovery, and cleanup vocabulary and must not use implementation names
as commands, selectors, examples, or user workflow concepts.

## Non-Goals

- Git-style commit graph, staging area, merge, rebase, or conflict
  resolution.
- Built-in remote hosting, push/pull, or credential management.
- Signing commands or trust policy as public CLI.
- Labels/messages/tags as direct restore targets.
- Public partial-save or compression contracts.
- Complex retention policy flags.
- Server-side authorization or multi-user locking.

## Motto

Real folders. Save points. Preview before restore. Recovery before panic.
