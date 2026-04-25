<p align="center">
  <h1 align="center">JVS</h1>
  <p align="center">
    <strong>Filesystem-native workspace checkpoints</strong>
  </p>
  <p align="center">
    <a href="https://github.com/jvs-project/jvs/releases/latest"><img src="https://img.shields.io/github/v/release/jvs-project/jvs?style=flat-square" alt="Release"></a>
    <a href="https://github.com/jvs-project/jvs/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/jvs-project/jvs/ci.yml?branch=main&style=flat-square&label=CI" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/jvs-project/jvs"><img src="https://goreportcard.com/badge/github.com/jvs-project/jvs?style=flat-square" alt="Go Report Card"></a>
    <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License: MIT"></a>
  </p>
</p>

---

JVS (**Juicy Versioned Workspaces**) creates verifiable checkpoints of entire
workspace directories. It is local-first, works with normal filesystem paths,
and uses the best available copy engine for the target filesystem: JuiceFS
clone where available, reflink where supported, and recursive copy everywhere
else.

```bash
jvs init myproject
cd myproject/main

echo "hello" > data.txt
jvs status
jvs checkpoint "baseline"

echo "experiment" >> data.txt
jvs checkpoint "experiment-1" --tag exp

jvs restore latest
jvs fork experiment
jvs verify --all
```

## Why JVS?

| Problem | JVS approach |
| --- | --- |
| Git is awkward for large binary workspace state | Checkpoint the whole directory tree without staging files one by one |
| Automation needs repeatable rollback points | `current`, `latest`, and `dirty` make workspace state explicit |
| Teams want local-first workflows | Repos are ordinary directories; no JVS server or remote protocol is required |
| Corruption should be detectable | Descriptor checksums and payload hashes can be verified with `jvs verify` |

## Install

**Download a binary** from the [latest release](https://github.com/jvs-project/jvs/releases/latest)
(Linux, macOS, Windows):

```bash
# Linux (amd64)
curl -L https://github.com/jvs-project/jvs/releases/latest/download/jvs-linux-amd64 -o jvs
chmod +x jvs && sudo mv jvs /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/jvs-project/jvs/releases/latest/download/jvs-darwin-arm64 -o jvs
chmod +x jvs && sudo mv jvs /usr/local/bin/
```

**Or build from source** (requires Go 1.25+):

```bash
git clone https://github.com/jvs-project/jvs.git
cd jvs
make build
```

## Quick Start

```bash
jvs capability .

jvs init myproject
cd myproject/main

echo "hello" > data.txt
jvs status
jvs checkpoint "first version"

echo "world" >> data.txt
jvs checkpoint "second version" --tag release

jvs checkpoint list
jvs restore latest

jvs fork experiment
cd "$(jvs workspace path experiment)"
```

The repository root (`myproject/`) is the control plane plus workspace
container. `myproject/main/` is the default workspace payload where your files
live. Use `jvs status` to see whether the workspace is dirty, which checkpoint
is `current`, and which checkpoint is `latest`.

## Commands

| Command | What it does |
| --- | --- |
| `jvs init <repo-path>` | Create a new JVS repo with a `main` workspace |
| `jvs import <existing-dir> <repo-path>` | Import files into a new repo and create an initial checkpoint |
| `jvs clone <source-repo> <dest-repo> [--scope full\|current]` | Copy a local JVS repo or its current workspace into a new repo |
| `jvs capability <target-path> [--write-probe]` | Probe JuiceFS, reflink, and copy support |
| `jvs info` | Show repo metadata and engine summary |
| `jvs status` | Show `current`, `latest`, `dirty`, and recovery hints |
| `jvs checkpoint [note] [--tag T]` | Create a checkpoint of the current workspace |
| `jvs checkpoint list` | List checkpoints |
| `jvs diff <from> <to> [--stat]` | Compare two checkpoints |
| `jvs restore <ref\|current\|latest>` | Replace the workspace with a checkpoint |
| `jvs fork [<ref> <name>\|<name>]` | Create another workspace from a checkpoint |
| `jvs workspace list\|path\|rename\|remove` | Inspect and manage workspaces |
| `jvs verify [<checkpoint-id>\|--all]` | Verify descriptor and payload integrity |
| `jvs doctor [--strict]` | Check repository health |
| `jvs gc plan` / `jvs gc run --plan-id ID` | Plan and run two-phase storage cleanup |

Commands that overwrite or remove workspace files refuse dirty state by
default. Use `--include-working` to checkpoint dirty work before `restore` or
`fork`, or `--discard-dirty` when you intentionally want to throw it away.

## How It Works

```text
myproject/
├── .jvs/                 # control plane: metadata, descriptors, audit, gc
├── main/                 # default workspace payload
└── ...                   # additional workspace payloads managed by JVS
```

JVS publishes checkpoints atomically: incomplete checkpoint attempts remain
invisible, and `jvs doctor` can report or repair safe runtime leftovers. The
active engine is selected per filesystem and reported through `jvs info`,
`jvs capability`, and checkpoint metadata.

## Integrity

`jvs verify` checks checkpoint descriptors and payload hashes. `jvs doctor`
checks repository layout, publish state, lineage, and safe repair candidates.
The audit log is hash-chained so repository history can be made
tamper-evident; signing and remote trust policy are future directions, not v0
stable commands.

## Development

```bash
make test
make contract-check
make conformance
make lint
make release-gate
```

## Documentation

| Document | Description |
| --- | --- |
| [Quick Start](docs/QUICKSTART.md) | 5-minute tutorial |
| [Product Plan](docs/PRODUCT_PLAN.md) | Current product and CLI direction |
| [CLI Spec](docs/02_CLI_SPEC.md) | Stable command reference |
| [Architecture](docs/ARCHITECTURE.md) | System design and internals |
| [Changelog](docs/99_CHANGELOG.md) | Release history |

## License

[MIT](LICENSE)
