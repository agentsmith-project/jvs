# JVS User Guide

JVS helps you save the state of a real folder, look back at earlier saved
states, and safely bring files back when you need them. You do not need to
learn the implementation. The everyday ideas are:

- **Folder**: the normal directory your editor and tools already use.
- **Workspace**: JVS's name for a folder it is looking after.
- **Save point**: a named saved state of that workspace.
- **Project/repo**: the JVS-managed folder and its save point history.
- **Control data**: JVS's own state for history, workspaces, runtime checks,
  and recovery.

## Start Here

If you are new to JVS, read these in order:

| Read | Why |
| --- | --- |
| [Quickstart](quickstart.md) | A guided first run with commands, what to look for, and what to do next |
| [Best Practices](best-practices.md) | Daily habits for saving, viewing, restoring, using workspaces, and cleanup |
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
| Learn the everyday JVS routine | [Best Practices](best-practices.md) |
| Save your current work | [Quickstart: Save a baseline](quickstart.md#2-save-a-baseline) |
| Find an earlier save point | [Quickstart: Check history](quickstart.md#4-check-history) |
| Look at old files without changing anything | [Quickstart: View a save point](quickstart.md#5-view-a-save-point-without-changing-files) |
| Restore a folder after a bad edit | [Quickstart: Restore safely](quickstart.md#6-restore-safely) |
| Restore one file or folder | [Quickstart: Restore one path](quickstart.md#7-restore-one-path) |
| Start a second workspace from a save point | [Quickstart: Create another workspace](quickstart.md#8-create-another-workspace) |
| Delete a workspace | [Command Reference: workspace delete](commands.md#jvs-workspace) |
| Free old save point storage | [Command Reference: cleanup](commands.md#jvs-cleanup) |

## What Changes Files?

Many JVS commands only show information. The commands below are the ones to
treat as important moments.

| Command | What happens |
| --- | --- |
| `jvs init [folder]` | Adds JVS control data to the folder. Your existing files stay in place. |
| `jvs save -m "message"` | Creates a save point from the current workspace. Your files stay as they are. |
| `jvs workspace new <folder> --from <save>` | Creates another real workspace folder at the path you choose. The original workspace is unchanged. |
| `jvs workspace rename <old> <new>` | Changes the JVS workspace name only. The real folder is not moved. |
| `jvs workspace move <name> <new-folder>` | Preview only. The workspace folder is not moved. |
| `jvs workspace move --run <workspace-move-plan-id>` | Moves the selected workspace folder and updates its workspace entry. |
| `jvs workspace delete <name>` | Preview only. The workspace folder is not deleted. |
| `jvs workspace delete --run <workspace-delete-plan-id>` | Deletes that workspace folder and its workspace entry. Save point storage stays. |
| `jvs repo clone <target-folder>` | Creates a new local JVS project folder with a new repo identity. The source project is unchanged. |
| `jvs repo clone <target-folder> --dry-run` | Preview/check only. No target project folder is created. |
| `jvs repo move <new-folder>` | Preview only. The project folder is not moved. |
| `jvs repo move --run <repo-move-plan-id>` | Moves the project folder while keeping the same repo identity and save point history. |
| `jvs repo rename <new-folder-name>` | Preview only. The project folder is not renamed. |
| `jvs repo rename --run <repo-rename-plan-id>` | Renames the project folder within the same parent directory. |
| `jvs repo detach` | Preview only. JVS metadata is not archived and files are not moved. |
| `jvs repo detach --run <repo-detach-plan-id>` | Archives JVS metadata and stops treating the project folder as an active JVS repo. Working files stay in place. |
| `jvs restore <save>` | Preview only. No files change. |
| `jvs restore --run <restore-plan-id>` | Changes files in the workspace according to the previewed restore plan. |
| `jvs cleanup preview` | Preview only. No storage is removed. |
| `jvs cleanup run --plan-id <cleanup-plan-id>` | Removes save point storage that the reviewed cleanup plan selected. |
| `jvs recovery resume <plan>` | Continues an interrupted restore and may change files. |
| `jvs recovery rollback <plan>` | Returns the folder to the protected pre-restore state when possible. |
| `jvs doctor --repair-runtime` | Runs safe automatic repairs for runtime state in JVS control data. Workspace files and save point history stay unchanged. |

Commands such as `jvs status`, `jvs history`, `jvs view`, `jvs workspace list`,
and `jvs workspace path` are for looking around. They do not change your
workspace files.

The biggest real-folder risks are easy to remember:

- `workspace new` and `repo clone` create new folders.
- `workspace move`, `repo move`, and `repo rename` move or rename existing
  folders only when you run the reviewed plan.
- `workspace delete --run` deletes a workspace folder, but not save point
  storage.
- `repo detach --run` keeps working files but archives JVS metadata; after that,
  the folder is no longer an active JVS repo.

## Preview-First Rule

JVS uses reviewed plans for the higher-impact operations:

```text
restore preview -> restore run
workspace move preview -> workspace move run
workspace delete preview -> workspace delete run
repo move preview -> repo move run
repo rename preview -> repo rename run
repo detach preview -> repo detach run
cleanup preview -> cleanup run
```

When a command prints a `Run:` line, stop and read the folder path, workspace
name, save point, and impact summary. Run the printed command only if the plan
matches what you meant. Use the plan ID from that same preview.
