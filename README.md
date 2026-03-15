<div align="center">

```
 ██████╗  ██████╗ ████████╗██╗
 ██╔══██╗██╔═══██╗╚══██╔══╝██║
 ██████╔╝██║   ██║   ██║   ██║
 ██╔══██╗██║   ██║   ██║   ██║
 ██████╔╝╚██████╔╝   ██║   ███████╗
 ╚═════╝  ╚═════╝    ╚═╝   ╚══════╝
```

**Run Claude Code in ephemeral, sandboxed Docker containers.**

[![CI](https://github.com/kamilrybacki/botl/actions/workflows/ci.yml/badge.svg)](https://github.com/kamilrybacki/botl/actions/workflows/ci.yml)
[![E2E](https://github.com/kamilrybacki/botl/actions/workflows/e2e.yml/badge.svg)](https://github.com/kamilrybacki/botl/actions/workflows/e2e.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kamilrybacki/botl)](https://goreportcard.com/report/github.com/kamilrybacki/botl)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kamilrybacki/botl)](go.mod)

[Installation](#installation) | [Quick Start](#quick-start) | [Usage](#usage) | [How It Works](#how-it-works) | [Testing](#testing) | [Contributing](#contributing)

</div>

---

## Why

You want Claude Code to work on a repository **without being able to modify anything on your host machine**. `botl` clones the repo inside a throwaway Docker container, mounts your local package caches read-only so builds work, and destroys everything when you're done.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) (running daemon)
- A Claude Pro or Max subscription (authenticate by running `claude` on your host once)

## Installation

```bash
go install github.com/kamilrybacki/botl@latest
```

<details>
<summary><strong>Build from source</strong></summary>

```bash
git clone https://github.com/kamilrybacki/botl.git
cd botl
make build
```

</details>

## Quick Start

```bash
# 1. Build the Docker image (once)
botl build

# 2. Launch an interactive session
botl run https://github.com/user/repo
```

Claude Code starts inside the container with the repo cloned and ready. When you exit, a post-session menu lets you **push**, **export a patch**, or **save the workspace** before the container is destroyed.

## Usage

### `botl run <repo-url>`

Clone a repo into a container and launch Claude Code.

```bash
botl run https://github.com/user/repo                          # interactive
botl run https://github.com/user/repo -b feat/new-thing        # specific branch
botl run https://github.com/user/repo -p "fix lint errors"     # headless mode
botl run https://github.com/user/repo --timeout 1h             # custom timeout
botl run https://github.com/user/repo -m /data:/data           # extra mounts
botl run https://github.com/user/repo -e MY_VAR=value          # extra env vars
```

<details>
<summary><strong>All flags</strong></summary>

| Flag | Default | Description |
|------|---------|-------------|
| `-b, --branch` | repo default | Branch to clone |
| `--depth` | `1` | Git clone depth |
| `-p, --prompt` | _(none)_ | Prompt for headless mode |
| `--mount-packages` | `true` | Auto-detect and mount host packages (ro) |
| `-m, --mount` | _(none)_ | Extra read-only mount `host:container` (repeatable) |
| `--timeout` | `30m` | Max session duration |
| `--image` | `botl:latest` | Docker image to use |
| `-e, --env` | _(none)_ | Extra env vars `KEY=VALUE` (repeatable) |
| `-o, --output-dir` | `./botl-output` | Host directory for exports |

</details>

### `botl build`

Build (or rebuild) the Docker image.

```bash
botl build
botl build --image my-custom-botl:v2
```

## How It Works

```
┌─────────────────────────────────────────────────────┐
│  Host Machine                                       │
│                                                     │
│  botl run https://github.com/user/repo              │
│    │                                                │
│    ├─ Detects host packages (node, python, go, etc) │
│    ├─ Starts ephemeral Docker container             │
│    │                                                │
│    │  ┌─────────────────────────────────────────┐   │
│    │  │  Container (--rm)                       │   │
│    │  │                                         │   │
│    │  │  /workspace/repo/  ← shallow clone (rw) │   │
│    │  │  /root/.claude/    ← OAuth creds (ro)   │   │
│    │  │  /usr/lib/node_modules/ ← host (ro)     │   │
│    │  │  /usr/lib/python3/...  ← host (ro)      │   │
│    │  │                                         │   │
│    │  │  $ claude --dangerously-skip-permissions │   │
│    │  │                                         │   │
│    │  │  ┌────────────────────────────────────┐  │   │
│    │  │  │ What to do with changes?           │  │   │
│    │  │  │ > Push to a remote branch          │  │   │
│    │  │  │   Create a git diff patch          │  │   │
│    │  │  │   Save workspace to local path     │  │   │
│    │  │  │   Discard and exit                 │  │   │
│    │  │  └────────────────────────────────────┘  │   │
│    │  └─────────────────────────────────────────┘   │
│    │                                                │
│    └─ Container destroyed after result exported      │
└─────────────────────────────────────────────────────┘
```

1. **Image** — `node:22-slim` with Claude Code pre-installed and git
2. **Clone** — Target repo is shallow-cloned inside the container
3. **Mounts** — Host package dirs are auto-detected and bind-mounted read-only
4. **Session** — Claude Code runs interactively (TTY) or headless (`--prompt`)
5. **Post-session** — TUI menu for exporting results
6. **Cleanup** — Container removed automatically

<details>
<summary><strong>Post-session options</strong></summary>

| Option | What it does |
|--------|-------------|
| **Push to a remote branch** | Commits uncommitted changes, prompts for branch name (default: `botl/<timestamp>`), pushes |
| **Create a git diff patch** | Exports all changes as a `.patch` file to `--output-dir` |
| **Save workspace to local path** | Copies the workspace directory to `--output-dir` |
| **Discard and exit** | Throws away everything |

</details>

<details>
<summary><strong>Auto-detected packages</strong></summary>

| Ecosystem | What's mounted |
|-----------|---------------|
| Node.js | Global `node_modules` (via `npm root -g`) |
| Python | `site-packages` directories |
| Go | Module cache (`$GOPATH/pkg/mod`) |
| Rust | Cargo registry (`~/.cargo/registry`) |

Disable with `--mount-packages=false`, or add extras with `--mount`.

</details>

## Testing

All tests run inside a Docker container for reproducible results. The test container uses Docker-in-Docker so E2E tests can build and run botl images.

```bash
make test               # all suites
make test-unit          # internal package unit tests
make test-integration   # CLI command tests
make test-entrypoint    # shell-level entrypoint.sh checks
make test-e2e           # full Docker build + container behavior
make lint               # golangci-lint
```

## Security

| Concern | How it's handled |
|---------|-----------------|
| Host filesystem writes | All package mounts are read-only; only `--output-dir` is writable |
| Repository isolation | Cloned repo lives only inside the container, destroyed on exit |
| Credential safety | `~/.claude` is mounted read-only — OAuth tokens are reused, not modifiable |
| Container permissions | Claude Code runs with `--dangerously-skip-permissions` (safe: the container itself is the sandbox) |

## Contributing

Contributions are welcome. Please open an issue first to discuss what you'd like to change.

```bash
git clone https://github.com/kamilrybacki/botl.git
cd botl
make test       # run the full test suite
make lint       # check for lint issues
```

## License

[MIT](LICENSE)
