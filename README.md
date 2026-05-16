<p align="center">
  <h1 align="center">JVS</h1>
  <p align="center">
    <strong>Pre-GA control data for real folders</strong>
  </p>
  <p align="center">
    <a href="https://github.com/agentsmith-project/jvs/releases"><img src="https://img.shields.io/github/v/release/agentsmith-project/jvs?style=flat-square" alt="Release"></a>
    <a href="https://github.com/agentsmith-project/jvs/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/agentsmith-project/jvs/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/agentsmith-project/jvs"><img src="https://goreportcard.com/badge/github.com/agentsmith-project/jvs?style=flat-square" alt="Go Report Card"></a>
    <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License: MIT"></a>
  </p>
</p>

---

JVS (**Juicy Versioned Workspaces**) is in a pre-GA surface collapse. The
current release-facing user path is intentionally small: initialize a folder,
inspect status/health, and use project clone/metadata commands that remain part
of the release path. The old public save/restore workflow is not the active
user path.

```bash
mkdir myproject
cd myproject

jvs init
echo "hello" > notes.txt
jvs status
jvs doctor
jvs repo clone ../myproject-copy --dry-run
```

Trusted platform callers use the internal direct AFSCP contract documented in
`docs/contracts/jvs-afscp-direct-v1.md`. That direct contract is not public
user CLI guidance.

## Why JVS?

| Need | JVS approach |
| --- | --- |
| Prepare a folder | `jvs init [folder]` creates JVS control data without moving files |
| Inspect current state | `jvs status` and `jvs doctor` report folder/control-data health |
| Keep real directories | A workspace is a normal folder; your tools keep using normal filesystem paths |
| Clone project metadata safely | `jvs repo clone <target-folder> --dry-run` previews a clone |

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
| `jvs status` | Show the active folder, workspace, control-data pointer, and unsaved-change summary |
| `jvs doctor` | Check repository health |
| `jvs repo clone <target-folder> [--dry-run]` | Clone a local JVS project into a new folder |
| `jvs completion <shell>` | Generate shell completion |

## How It Works

After `jvs init`, your folder stays where it is:

```text
myproject/
├── .jvs/          # JVS control data
├── notes.txt      # your managed files
└── ...
```

JVS control data is not workspace content. The direct AFSCP path keeps JVS
metadata outside the managed HOME and uses a strict JuiceFS clone path with no
public slow fallback.

## Documentation

Start with the collapsed pre-GA docs:

| Document | Description |
| --- | --- |
| [Overview](docs/00_OVERVIEW.md) | Active surface and boundaries |
| [CLI Spec](docs/02_CLI_SPEC.md) | Release-facing command surface |
| [AFSCP Direct Contract](docs/contracts/jvs-afscp-direct-v1.md) | Internal trusted platform JSON contract |
| [Documentation Index](docs/README.md) | Maintainer and historical reference map |

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
