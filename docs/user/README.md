# JVS User Guide

JVS helps you save the state of a real folder, look back at earlier saved
states, and safely bring files back when you need them. You do not need to
learn the implementation. The everyday ideas are:

- **Folder**: the normal directory your editor and tools already use.
- **Workspace**: JVS's name for a folder it is looking after.
- **Save point**: a named saved state of that workspace.

## Start Here

If you are new to JVS, read these in order:

| Read | Why |
| --- | --- |
| [Quickstart](quickstart.md) | A guided first run with commands, what to look for, and what to do next |
| [Concepts](concepts.md) | Plain-English meanings of folder, workspace, save point, history, view, restore, and cleanup |
| [Safety](safety.md) | What changes files, what is preview-only, and how JVS avoids surprising you |
| [Command Reference](commands.md) | A compact lookup once you know what you want to do |

Then keep these nearby:

| Read | When |
| --- | --- |
| [Examples](examples.md) | You want recipes for real workflows |
| [Tutorials](tutorials.md) | You want longer step-by-step project stories |
| [Troubleshooting](troubleshooting.md) | A command refuses to run or says a plan is stale |
| [Recovery](recovery.md) | A restore was interrupted and JVS asks you to resume or roll back |
| [FAQ](faq.md) | You want short answers before going deeper |

## Common Tasks

| Goal | Go to |
| --- | --- |
| Start using JVS in a folder | [Quickstart: Prepare a folder](quickstart.md#1-prepare-a-folder) |
| Save your current work | [Quickstart: Save a baseline](quickstart.md#2-save-a-baseline) |
| Find an earlier save point | [Quickstart: Check history](quickstart.md#4-check-history) |
| Look at old files without changing anything | [Quickstart: View a save point](quickstart.md#5-view-a-save-point-without-changing-files) |
| Restore a folder after a bad edit | [Quickstart: Restore safely](quickstart.md#6-restore-safely) |
| Restore one file or folder | [Quickstart: Restore one path](quickstart.md#7-restore-one-path) |
| Start a second workspace from a save point | [Quickstart: Create another workspace](quickstart.md#8-create-another-workspace) |
| Remove a workspace | [Command Reference: workspace remove](commands.md#jvs-workspace) |
| Free old save point storage | [Command Reference: cleanup](commands.md#jvs-cleanup) |

## What Changes Files?

Many JVS commands only show information. The commands below are the ones to
treat as important moments.

| Command | What happens |
| --- | --- |
| `jvs init` | Adds JVS control data to the folder. Your existing files stay in place. |
| `jvs save -m "message"` | Creates a save point. Your files stay as they are. |
| `jvs restore <save>` | Preview only. No files change. |
| `jvs restore --run <restore-plan-id>` | Changes files in the workspace according to the previewed restore plan. |
| `jvs workspace new <name> --from <save>` | Creates another real folder from a save point. |
| `jvs workspace remove <name>` | Preview only. The workspace folder is not removed. |
| `jvs workspace remove --run <remove-plan-id>` | Removes that workspace folder and its workspace entry. Save point storage stays. |
| `jvs cleanup preview` | Preview only. No storage is removed. |
| `jvs cleanup run --plan-id <cleanup-plan-id>` | Removes save point storage that the reviewed cleanup plan selected. |
| `jvs recovery resume <plan>` | Continues an interrupted restore and may change files. |
| `jvs recovery rollback <plan>` | Returns the folder to the protected pre-restore state when possible. |

Commands such as `jvs status`, `jvs history`, `jvs view`, `jvs workspace list`,
and `jvs workspace path` are for looking around. They do not change your
workspace files.

## Preview-First Rule

JVS uses reviewed plans for the higher-impact operations:

```text
restore preview -> restore run
workspace remove preview -> workspace remove run
cleanup preview -> cleanup run
```

When a command prints a `Run:` line, stop and read the folder path, workspace
name, save point, and impact summary. Run the printed command only if the plan
matches what you meant. Use the plan ID from that same preview.
