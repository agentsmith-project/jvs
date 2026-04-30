<p align="center">
  <h1 align="center">JVS</h1>
  <p align="center">
    <strong>Filesystem-native save points for real folders</strong>
  </p>
  <p align="center">
    <a href="https://github.com/agentsmith-project/jvs/releases"><img src="https://img.shields.io/github/v/release/agentsmith-project/jvs?style=flat-square" alt="Release"></a>
    <a href="https://github.com/agentsmith-project/jvs/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/agentsmith-project/jvs/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/agentsmith-project/jvs"><img src="https://goreportcard.com/badge/github.com/agentsmith-project/jvs?style=flat-square" alt="Go Report Card"></a>
    <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License: MIT"></a>
  </p>
</p>

---

JVS (**Juicy Versioned Workspaces**) saves real folders as save points. Start
in an ordinary directory, save it, browse its history, open read-only views,
and restore a known state when you need to recover.

```bash
mkdir myproject
cd myproject

jvs init
echo "hello" > notes.txt
jvs save -m "baseline"

echo "experiment" >> notes.txt
jvs status
jvs history

# Pick a save point ID from history.
jvs view <save> notes.txt
jvs restore <save> --discard-unsaved
jvs restore --run <restore-plan-id>
```

Restore is preview-first: `jvs restore <save>` prints a plan and the exact
`Run:` line. Use the restore plan ID from that preview in
`jvs restore --run <restore-plan-id>`. No folder files change until you run the
plan.

## Why JVS?

| Need | JVS approach |
| --- | --- |
| Save a whole working folder | `jvs save -m "message"` captures managed files as a save point |
| Recover from a bad edit | `jvs history`, `jvs view`, and preview-first `jvs restore` help you choose and apply a save point |
| Keep real directories | A workspace is a normal folder; your tools keep using normal filesystem paths |
| Try another line of work | `jvs workspace new ../experiment --from <save>` creates another real folder from a save point |
| Diagnose trouble | `jvs doctor` and `jvs recovery` report health and interrupted restore plans |

## Install

Download a binary from [GitHub Releases](https://github.com/agentsmith-project/jvs/releases)
or build from source:

```bash
git clone https://github.com/agentsmith-project/jvs.git
cd jvs
make build
```

With Go installed, use a published version tag:

```bash
go install github.com/agentsmith-project/jvs/cmd/jvs@<VERSION>
```

## Core Commands

| Command | What it does |
| --- | --- |
| `jvs init [folder]` | Adopt a folder and prepare JVS control data |
| `jvs status` | Show the active folder, workspace, current pointer, newest save point, and unsaved changes |
| `jvs save -m "message"` | Create a save point for managed files in the active workspace |
| `jvs history` | List recent save points for the active workspace |
| `jvs history to <save>` | Show history ending at a save point |
| `jvs history from [<save>]` | Show history starting from a save point, or from the active workspace when omitted |
| `jvs history --path <path>` | Find save points that contain a workspace-relative path |
| `jvs view <save> [path]` | Open a read-only view of a save point or a path inside it |
| `jvs restore <save> [--path <path>]` | Preview a restore plan |
| `jvs restore --run <restore-plan-id>` | Execute a restore plan after JVS rechecks the folder |
| `jvs workspace new <folder> --from <save>` | Create another workspace folder at a path you choose |
| `jvs recovery status` | Show active restore recovery plans |
| `jvs doctor [--strict]` | Check repository health |

## How It Works

After `jvs init`, your folder stays where it is:

```text
myproject/
├── .jvs/          # JVS control data
├── notes.txt      # your managed files
└── ...
```

JVS control data is not part of save points. `view` creates a read-only copy
for inspection. `restore` copies managed files from a save point back into the
workspace and leaves history intact.

## Documentation

Start with the [User Docs](docs/user/README.md):

| Document | Description |
| --- | --- |
| [Quickstart](docs/user/quickstart.md) | First save point and first restore |
| [Best Practices](docs/user/best-practices.md) | Daily habits for saving, previewing, restoring, workspaces, and cleanup |
| [Concepts](docs/user/concepts.md) | Folder, workspace, save point, history, view, restore, and recovery |
| [Command Reference](docs/user/commands.md) | Release-facing command surface |
| [Examples](docs/user/examples.md) | Practical workflows |
| [FAQ](docs/user/faq.md) | Common questions |
| [Troubleshooting](docs/user/troubleshooting.md) | Common errors and fixes |
| [Safety](docs/user/safety.md) | Restore preview, unsaved changes, and read-only views |
| [Recovery](docs/user/recovery.md) | Interrupted restore recovery |

Contributor, architecture, and release evidence documents live under
[docs/](docs/README.md).

## Development

```bash
make test
make contract-check
make conformance
make lint
make release-gate
```

## License

[MIT](LICENSE)
