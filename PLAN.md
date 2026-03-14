# botl — Implementation Plan

## Summary

Go CLI tool that launches Claude Code inside an ephemeral Docker container with:
- Host packages/libraries auto-detected and mounted read-only
- A shallow-cloned git repo as the workspace
- Interactive TTY (default) or headless mode with `--prompt`
- Results stay in-container; user interacts live or gets output on exit

## Architecture

```
botl (Go binary)
├── cmd/           # CLI entry point (cobra)
│   └── root.go    # `botl run` command
├── internal/
│   ├── container/ # Docker container lifecycle
│   │   ├── build.go    # Image building
│   │   ├── create.go   # Container creation with mounts
│   │   ├── attach.go   # TTY attach logic
│   │   └── cleanup.go  # Container removal
│   ├── detect/    # Host package auto-detection
│   │   └── packages.go # Detect node_modules, site-packages, etc.
│   └── config/    # CLI config and flag parsing
│       └── config.go
├── docker/
│   └── Dockerfile # Base image with Claude Code + Node.js
├── go.mod
├── go.sum
└── main.go
```

## Dockerfile (base image)

```dockerfile
FROM node:22-slim
RUN npm install -g @anthropic-ai/claude-code
RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
ENTRYPOINT ["claude"]
```

Minimal image. Claude Code is the entrypoint. The repo gets cloned into
`/workspace` before Claude starts.

## CLI Commands

### `botl run <repo-url> [flags]`

Primary command. Full flow:

1. **Validate** inputs (repo URL, API key present)
2. **Build/check** Docker image exists (`botl:latest`)
3. **Detect** host packages (if `--mount-packages` enabled, default: true)
4. **Create** container with:
   - `ANTHROPIC_API_KEY` env var passed through
   - Detected package dirs as `:ro` bind mounts
   - Any `--mount` extras as `:ro` bind mounts
   - Init script that: `git clone --depth=<N> --branch=<B> <url> /workspace`
5. **Start** container
6. **Attach** TTY (interactive) or pass `--prompt` (headless)
7. **On exit**: container auto-removed (`--rm`)

Flags:
```
--branch, -b        Branch to clone (default: default branch)
--depth             Clone depth (default: 1)
--prompt, -p        Prompt for headless mode (omit for interactive)
--mount-packages    Auto-detect host packages (default: true)
--mount, -m         Extra read-only mount "host:container" (repeatable)
--timeout           Max duration (default: 30m)
--image             Custom image (default: botl:latest)
--env, -e           Extra env vars (repeatable)
--api-key           API key (default: $ANTHROPIC_API_KEY)
```

### `botl build`

Builds/rebuilds the Docker image from the embedded Dockerfile.

## Package Auto-Detection

Scan host for these directories/files and mount corresponding package dirs:

| Ecosystem | Detection Signal | Mount Source | Mount Target |
|-----------|-----------------|--------------|--------------|
| Node.js | `which node` | Global node_modules (`npm root -g`) | Same path `:ro` |
| Python | `which python3` | `python3 -c "import site; print(site.getsitepackages())"` | Same path `:ro` |
| Go | `which go` | `$GOPATH/pkg/mod` or `~/go/pkg/mod` | Same path `:ro` |
| Rust | `which cargo` | `~/.cargo/registry` | Same path `:ro` |
| System libs | Always | `/usr/local/lib` | `/usr/local/lib/host:ro` |

Users can disable with `--mount-packages=false` and/or add extras with `--mount`.

## Interactive vs Headless

**Interactive (default):**
- Allocate PTY on container
- Attach stdin/stdout/stderr
- User interacts with Claude Code directly in their terminal
- Container removed on exit

**Headless (`--prompt "..."`):**
- Pass prompt as argument to `claude --prompt "..."`
- Stream stdout to terminal
- Container removed on completion
- Exit code forwarded

## Key Implementation Details

- Use `github.com/docker/docker/client` (Docker SDK for Go)
- Use `github.com/spf13/cobra` for CLI
- Container is always `--rm` (ephemeral)
- `ANTHROPIC_API_KEY` is required (error if missing)
- Git clone happens inside container via entrypoint script
- Signal handling: SIGINT/SIGTERM → graceful container stop
- Timeout implemented via container stop after duration

## Phases

1. **Phase 1**: Scaffold Go project, Dockerfile, basic `run` command (interactive only)
2. **Phase 2**: Package auto-detection and mounting
3. **Phase 3**: Headless mode with `--prompt`
4. **Phase 4**: `build` subcommand, timeout, signal handling
5. **Phase 5**: Polish, error messages, edge cases
