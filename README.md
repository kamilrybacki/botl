# botl

Run [Claude Code](https://docs.anthropic.com/en/docs/claude-code) in ephemeral Docker containers with read-only access to your host packages and an isolated git repo workspace.

## Why

You want Claude Code to work on a repository without being able to modify anything on your host machine. `botl` clones the repo inside a throwaway Docker container, mounts your local package caches read-only so builds work, and destroys everything when you're done.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/) (to build)
- [Docker](https://docs.docker.com/get-docker/) (running daemon)
- An [Anthropic API key](https://console.anthropic.com/)

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

# 2. Export your API key
export ANTHROPIC_API_KEY=sk-ant-...

# 3. Launch an interactive session
botl run https://github.com/user/repo
```

Claude Code starts inside the container with the repo cloned and ready. When you exit, the container is destroyed.

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
| `--api-key` | `$ANTHROPIC_API_KEY` | Anthropic API key |

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
│    │  │  /usr/lib/node_modules/ ← host (ro)     │   │
│    │  │  /usr/lib/python3/...  ← host (ro)      │   │
│    │  │                                         │   │
│    │  │  $ claude --dangerously-skip-permissions │   │
│    │  │                                         │   │
│    │  └─────────────────────────────────────────┘   │
│    │                                                │
│    └─ Container destroyed on exit                   │
│                                                     │
└─────────────────────────────────────────────────────┘
```

1. **Image** — Based on `node:22-slim` with Claude Code pre-installed and git
2. **Clone** — The target repo is shallow-cloned inside the container at `/workspace/repo`
3. **Mounts** — Host package directories are auto-detected and bind-mounted read-only
4. **Session** — Claude Code runs interactively (TTY) or headless (with `--prompt`)
5. **Cleanup** — Container is removed automatically when the session ends

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

## Security Notes

- The container **cannot write to the host filesystem** — all mounts are read-only
- The cloned repo lives only inside the container and is destroyed on exit
- `ANTHROPIC_API_KEY` is passed as an env var, never persisted in the image
- Claude Code runs with `--dangerously-skip-permissions` inside the container (safe because the container itself is the sandbox)
