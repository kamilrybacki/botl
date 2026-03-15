# botl

Run [Claude Code](https://docs.anthropic.com/en/docs/claude-code) in ephemeral Docker containers with read-only access to your host packages and an isolated git repo workspace.

## Why

You want Claude Code to work on a repository without being able to modify anything on your host machine. `botl` clones the repo inside a throwaway Docker container, mounts your local package caches read-only so builds work, and destroys everything when you're done.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/) (to build)
- [Docker](https://docs.docker.com/get-docker/) (running daemon)
- A Claude Pro or Max subscription (authenticate by running `claude` on your host once)

## Install

```bash
go install github.com/kamilrybacki/botl@latest
```

Or build from source:

```bash
git clone https://github.com/kamilrybacki/botl.git
cd botl
go build -o botl .
```

## Quick Start

```bash
# 1. Build the Docker image (once)
botl build

# 2. Authenticate Claude Code (if you haven't already)
claude

# 3. Launch an interactive session
botl run https://github.com/user/repo
```

Claude Code starts inside the container with the repo cloned and ready. When you exit Claude, a post-session menu lets you push, export a patch, or save the workspace before the container is destroyed.

## Usage

### `botl run <repo-url>`

Clone a repo into a container and launch Claude Code.

```bash
# Interactive session (default)
botl run https://github.com/user/repo

# Specific branch
botl run https://github.com/user/repo -b feat/new-thing

# Headless mode — pass a prompt, get output
botl run https://github.com/user/repo -p "fix all lint errors and commit"

# Custom timeout
botl run https://github.com/user/repo --timeout 1h

# Extra read-only mounts
botl run https://github.com/user/repo -m /path/to/data:/data

# Pass extra env vars
botl run https://github.com/user/repo -e MY_VAR=value
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-b, --branch` | repo default | Branch to clone |
| `--depth` | `1` | Git clone depth |
| `-p, --prompt` | _(none)_ | Prompt for headless mode (omit for interactive) |
| `--mount-packages` | `true` | Auto-detect and mount host packages read-only |
| `-m, --mount` | _(none)_ | Extra read-only mount `host:container` (repeatable) |
| `--timeout` | `30m` | Max session duration |
| `--image` | `botl:latest` | Docker image to use |
| `-e, --env` | _(none)_ | Extra env vars `KEY=VALUE` (repeatable) |
| `-o, --output-dir` | `./botl-output` | Host directory for patches and saved workspaces |

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
│    │  │  │ ▸ Push to a remote branch          │  │   │
│    │  │  │   Create a git diff patch          │  │   │
│    │  │  │   Save workspace to local path     │  │   │
│    │  │  │   Discard and exit                 │  │   │
│    │  │  └────────────────────────────────────┘  │   │
│    │  │                                         │   │
│    │  └─────────────────────────────────────────┘   │
│    │                                                │
│    └─ Container destroyed after result exported      │
│                                                     │
└─────────────────────────────────────────────────────┘
```

1. **Image** — Based on `node:22-slim` with Claude Code pre-installed and git
2. **Clone** — The target repo is shallow-cloned inside the container at `/workspace/repo`
3. **Mounts** — Host package directories are auto-detected and bind-mounted read-only
4. **Session** — Claude Code runs interactively (TTY) or headless (with `--prompt`)
5. **Post-session** — A TUI menu lets you choose how to export results
6. **Cleanup** — Container is removed automatically after export

## Post-Session Results

When Claude Code exits, an interactive menu appears with four options:

| Option | What it does |
|--------|-------------|
| **Push to a remote branch** | Commits any uncommitted changes, prompts for branch name (default: `botl/<timestamp>`), and pushes |
| **Create a git diff patch** | Exports all changes (commits + uncommitted) as a `.patch` file to `--output-dir` |
| **Save workspace to local path** | Copies the entire workspace directory to `--output-dir` |
| **Discard and exit** | Throws away everything |

The menu supports arrow keys, vim keys (j/k), and falls back to numbered input if no TTY is available.

For patch and workspace export, files are written to `/output` inside the container which maps to `--output-dir` on the host (default: `./botl-output/`).

## Auto-Detected Packages

When `--mount-packages` is enabled (default), botl scans for:

| Ecosystem | What's mounted |
|-----------|---------------|
| Node.js | Global `node_modules` (via `npm root -g`) |
| Python | `site-packages` directories (via `site.getsitepackages()`) |
| Go | Module cache (`$GOPATH/pkg/mod`) |
| Rust | Cargo registry (`~/.cargo/registry`) |

Use `--mount-packages=false` to disable, or add extras with `--mount`.

## Headless Mode

Pass `--prompt` / `-p` to run Claude Code non-interactively:

```bash
botl run https://github.com/user/repo \
  -p "refactor the database layer to use connection pooling, then commit"
```

Claude Code runs with the prompt, streams output to your terminal, and the container exits when done. Useful for scripting and CI pipelines.

## Testing

All tests run inside a Docker container for reproducible results. The test container uses Docker-in-Docker so E2E tests can build and run botl images.

```bash
# Run all test suites (unit + integration + entrypoint + E2E)
make test

# Run individual suites
make test-unit          # Pure Go unit tests (container, detect packages)
make test-integration   # CLI command tests
make test-entrypoint    # Shell-level entrypoint.sh validation
make test-e2e           # Full Docker build + container behavior tests
```

Requires Docker with `--privileged` support (for Docker-in-Docker).

## Security Notes

- The container **cannot write to the host filesystem** — all package mounts are read-only (only `--output-dir` is writable for exporting results)
- The cloned repo lives only inside the container and is destroyed on exit
- `~/.claude` is mounted read-only — OAuth credentials are reused but cannot be modified by the container
- Claude Code runs with `--dangerously-skip-permissions` inside the container (safe because the container itself is the sandbox)
