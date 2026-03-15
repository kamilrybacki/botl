# Config TUI Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

Add a `botl config` subcommand that launches an interactive TUI for editing persistent configuration. The config file lives at `$XDG_CONFIG_HOME/botl/config.yaml` (defaulting to `~/.config/botl/config.yaml`) and provides defaults that CLI flags can override.

## Config Surface

Two settings:

1. **Clone mode** — `shallow` (depth=1, commit messages stripped, reflog cleared) or `deep` (full history preserved)
2. **Blocked ports** — list of TCP ports to block inbound connections on inside the container

## Config File Format

`~/.config/botl/config.yaml`:

```yaml
clone:
  mode: shallow    # "shallow" or "deep"
network:
  blocked_ports: [] # e.g. [8080, 3000, 5432]
```

Defaults (when file is missing): `shallow` clone, no blocked ports — matches current behavior.

**Error handling:** If the file exists but contains invalid YAML or unexpected types, `botl` prints a warning to stderr and falls back to built-in defaults.

## Config TUI

Raw terminal menu (no new TUI dependencies), consistent with the existing postrun TUI style:

```
  botl configuration (~/.config/botl/config.yaml)
  ─────────────────────────────────────────────────

  ▸ Clone mode        shallow (sanitized, depth=1)
    Blocked ports     none

  ↑/↓ navigate · enter select · q save & quit · ctrl+c discard & quit
```

- **Clone mode**: toggles between `shallow` and `deep` on Enter.
- **Blocked ports**: enters inline editor for comma-separated port numbers. Validates each port is a numeric integer in range 1-65535. Rejects invalid input with inline error. Enter confirms, Esc cancels.
- **q**: saves config to `~/.config/botl/config.yaml` and exits.
- **Ctrl+C**: discards unsaved changes and exits.

Creates config directory if it doesn't exist. Respects `$XDG_CONFIG_HOME`.

## CLI Flags

New flags on `botl run`:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--clone-mode` | string | from config or `shallow` | Clone mode: `shallow` or `deep` |
| `--blocked-ports` | []int | from config or `[]` | TCP ports to block inbound |

These override config file values. Existing `--depth` flag is still respected — if explicitly set, it overrides the clone mode's depth.

## Integration with `botl run`

**Config loading priority:** CLI flags > config file > built-in defaults.

1. `cmd/run.go` loads config file if it exists (warn and use defaults on parse error).
2. Config values applied as flag defaults.
3. CLI flags override.

### Clone Mode

- **Shallow**: passes `BOTL_DEPTH=1` and `BOTL_SANITIZE_GIT=true` to the container.
- **Deep**: omits `--depth` flag from `git clone` (full clone), no sanitization.

Note: `git clone --depth 0` is invalid — for deep mode, the `--depth` flag is omitted entirely from the clone command in `entrypoint.sh`.

In `entrypoint.sh`, after clone and **after** recording `BOTL_INITIAL_HEAD`:

```bash
if [ "$BOTL_SANITIZE_GIT" = "true" ]; then
    git commit --amend --allow-empty -m "initial"
    git reflog expire --expire=now --all
    git gc --prune=now
    # Re-record HEAD after sanitization so postrun diff detection works
    export BOTL_INITIAL_HEAD=$(git rev-parse HEAD)
fi
```

The `BOTL_INITIAL_HEAD` must be set **after** sanitization because `git commit --amend` changes the commit hash. The postrun TUI relies on this value for `git log` and `git format-patch` comparisons.

This strips commit messages and reflog while preserving author name/email, working tree, remote origin, and branch tracking. All postrun export options (push, patch, save, discard) remain functional.

### Port Blocking

Blocked ports passed as `BOTL_BLOCKED_PORTS=8080,3000` env var.

**Approach: Docker-level network control.** Instead of using `iptables` inside the container (which requires `NET_ADMIN` capability and root privileges), port blocking is implemented at the Docker level using `--network` options:

1. Create a custom Docker network with `--internal` flag (no external ingress) per-run.
2. Use `docker run --network=<custom>` to attach the container.
3. Alternatively, since botl never publishes ports (`--publish`), inbound connections from outside Docker are already blocked. The blocked-ports config adds explicit `iptables` rules on the **host** side via Docker's built-in network isolation — but since the container has no published ports, the primary use case is blocking connections from **other containers** on the same Docker network.

**Simplified approach chosen:** Since the container never publishes ports, external inbound access is already blocked by default. The `blocked_ports` config adds explicit `--publish` denials and intra-Docker-network blocking by running the container on an isolated network:

```go
// In buildDockerArgs, when blocked ports are configured:
// Create container with no inter-container networking on those ports
args = append(args, "--network=botl-isolated")
```

Before running the container, create the isolated network if it doesn't exist:

```go
// docker network create --internal botl-isolated 2>/dev/null || true
```

The `--internal` flag prevents any external routing to the network, and since no ports are published, inbound connections are fully blocked. This avoids granting `NET_ADMIN` capability entirely.

**Fallback for fine-grained port blocking:** If per-port granularity is needed (rather than full network isolation), the entrypoint uses `iptables` with the following safeguards:

- Container entrypoint runs initial setup as root, then drops to `botl` user via `exec su -s /bin/sh botl -c "..."` (the Dockerfile's ENTRYPOINT runs as root, then uses `gosu` or `su` to drop privileges).
- Each port value is validated as numeric in-range before passing to `iptables`:

```bash
if [ -n "$BOTL_BLOCKED_PORTS" ]; then
    for port in $(echo "$BOTL_BLOCKED_PORTS" | tr ',' ' '); do
        if echo "$port" | grep -qE '^[0-9]+$' && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
            iptables -A INPUT -p tcp --dport "$port" -j REJECT 2>&1 || echo "warning: failed to block port $port" >&2
        else
            echo "warning: invalid port '$port', skipping" >&2
        fi
    done
fi
```

Key changes from original approach:
- `REJECT` instead of `DROP` (sends ICMP unreachable for debuggability)
- Numeric validation before passing to `iptables`
- Warnings on failure instead of silent `|| true`
- `NET_ADMIN` capability added only when needed, documented as expanding the container's attack surface

## Code Changes

### New Files

| File | Purpose |
|------|---------|
| `internal/config/config.go` | Config struct, load/save YAML, defaults, validation |
| `internal/config/config_test.go` | Unit tests for config loading/saving/validation |
| `cmd/config.go` | `botl config` subcommand with TUI |

### Modified Files

| File | Change |
|------|--------|
| `cmd/run.go` | Load config, apply as flag defaults, add `--clone-mode` and `--blocked-ports` flags |
| `internal/container/types.go` | Add `SanitizeGit bool`, `BlockedPorts []int` to `RunOpts` |
| `internal/container/run.go` | Pass new env vars, create isolated network or add `--cap-add NET_ADMIN` |
| `internal/container/dockerctx/entrypoint.sh` | Git sanitization + port blocking with validation |
| `internal/container/dockerctx/Dockerfile` | Install `iptables` package |
| `go.mod` | Add `gopkg.in/yaml.v3` |

### Dependencies

- `gopkg.in/yaml.v3` — YAML parsing (only new dependency)
- TUI reuses raw terminal patterns from existing postrun binary (`golang.org/x/term`)
