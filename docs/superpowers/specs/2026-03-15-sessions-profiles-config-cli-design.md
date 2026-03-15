# Sessions, Profiles & Config CLI Design Spec

**Date:** 2026-03-15
**Status:** Approved
**Supersedes:** `2026-03-15-config-tui-design.md`

## Overview

Three related features implemented together:

1. **Session IDs** — every `botl run` gets a unique ID, printed at start and end, and written to a session record on disk.
2. **Labels / Profiles** — `botl label <session-id> <name>` promotes a session's run configuration into a named, reusable profile. `botl run --with-label=<name>` loads a profile as defaults.
3. **Config CLI** — replaces the previously specced TUI with plain subcommands: `botl config set/get/list`.

## Storage Layout

```
~/.config/botl/
  config.yaml                   # global defaults (clone mode, blocked ports)
  profiles/
    <name>.yaml                 # one file per named profile

~/.local/share/botl/
  sessions/
    <id>.yaml                   # one file per run; accumulates over time
```

Both directories are created on demand. XDG env vars (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`) are respected.

## Data Model

### Session Record (`~/.local/share/botl/sessions/<id>.yaml`)

Written at the start of `botl run` with all resolved run options (after config loading, before profile override — see loading priority). Env var values are never persisted; only the key names are recorded.

```yaml
id: a3f2c1d8
created_at: 2026-03-15T14:32:00Z
repo_url: https://github.com/user/repo   # reference only, not copied to profile
branch: main

run:
  clone_mode: deep
  depth: 0
  blocked_ports: [8080, 3000]
  timeout: 30m
  image: botl:latest
  output_dir: /tmp/out
  env_var_keys: ["FOO", "BAR"]           # keys only — values never stored
  mounts: ["~/.npmrc:/home/botl/.npmrc"]
```

### Profile (`~/.config/botl/profiles/<name>.yaml`)

Identical to the `run` section of a session record, plus metadata. Created by `botl label`; never written directly by `botl run`.

```yaml
name: my-secure-env
created_at: 2026-03-15T14:35:00Z
source_session: a3f2c1d8

run:
  clone_mode: deep
  depth: 0
  blocked_ports: [8080, 3000]
  timeout: 30m
  image: botl:latest
  output_dir: /tmp/out
  env_var_keys: ["FOO", "BAR"]
  mounts: []
```

## Session ID Generation

8-character lowercase hex string generated via `crypto/rand`. Probability of collision across 10,000 sessions is negligible (~0.00002%). IDs are not globally unique identifiers — they are human-friendly references for local use only.

## Flag Resolution Order

```
CLI flags > --with-label profile > config file > built-in defaults
```

Each layer fills in only what the layer above left unset.

## Env Var Resolution at Runtime

When a profile has `env_var_keys` set, the following resolution runs before the container starts:

1. If the key is already set in the caller's shell environment → use it silently.
2. If not set → print `botl: env var <KEY> not set · enter value:` and read from stdin interactively.
3. If the user provides an empty value (blank input or EOF) → hard error, run aborted.

Explicit `--env KEY=VALUE` CLI flags bypass this flow and take precedence.

## Commands

### `botl run`

Existing command, extended:

```
$ botl run [--with-label=<name>] [flags] <repo-url>
```

New behavior:
- Generates and prints session ID at start: `botl: session id: a3f2c1d8`
- Writes session record to `~/.local/share/botl/sessions/a3f2c1d8.yaml`
- If `--with-label` given: loads profile, applies over config defaults
- Resolves env var keys (shell → interactive prompt)
- Prints session ID again on exit: `botl: session complete · id: a3f2c1d8`

New flag:

| Flag | Type | Description |
|------|------|-------------|
| `--with-label` | string | Load named profile as defaults |

### `botl label`

```
$ botl label <session-id> <name>
botl: profile "my-secure-env" saved (~/.config/botl/profiles/my-secure-env.yaml)
```

- Reads session record for `<session-id>` from `~/.local/share/botl/sessions/`.
- Copies `run` section into a new profile file.
- Errors: session not found; name already taken (use `--force` to overwrite).

### `botl profiles list`

```
$ botl profiles list
NAME            CREATED       SESSION
my-secure-env   2026-03-15    a3f2c1d8
dev-strict      2026-03-14    b2e4f9a1
```

### `botl profiles show <name>`

Prints the full profile YAML to stdout.

### `botl profiles delete <name>`

Prompts for confirmation, then removes the profile file.

```
$ botl profiles delete my-secure-env
Delete profile "my-secure-env"? [y/N] y
botl: profile "my-secure-env" deleted
```

### `botl config set/get/list`

Replaces the TUI specced in `2026-03-15-config-tui-design.md`.

```
$ botl config set clone-mode deep
$ botl config get clone-mode
deep
$ botl config list
clone-mode      deep
blocked-ports   8080, 3000
```

Valid keys and values:

| Key | Valid values |
|-----|-------------|
| `clone-mode` | `shallow`, `deep` |
| `blocked-ports` | comma-separated integers 1–65535 (e.g. `8080,3000`), or empty string to clear |

Validation runs on `set`. Invalid values print an error and leave the config unchanged.

## Code Changes

### New Packages

| Package | Responsibility |
|---------|---------------|
| `internal/session` | Session ID generation (`crypto/rand`), write/read session records, XDG data path |
| `internal/profile` | Load, save, list, delete profiles; XDG config path |

### New/Modified Files

| File | Change |
|------|--------|
| `internal/session/session.go` | New: ID gen, record struct, write/read |
| `internal/session/session_test.go` | New: unit tests |
| `internal/profile/profile.go` | New: profile struct, CRUD, path resolution |
| `internal/profile/profile_test.go` | New: unit tests |
| `cmd/label.go` | New: `botl label` command |
| `cmd/profiles.go` | New: `botl profiles` subcommands (list, show, delete) |
| `cmd/config.go` | Replace TUI with `set`, `get`, `list` subcommands |
| `cmd/run.go` | Add session ID, `--with-label` flag, profile loading, env var resolution |
| `internal/config/config.go` | Keep as-is (struct, load, save, validate) — TUI code was never implemented |

### Superseded Spec

`2026-03-15-config-tui-design.md` is superseded by this spec. The `internal/config` package implementation from that spec's plan is still valid and required; only the TUI portion (`cmd/config.go` raw terminal code) is replaced.

## Dependencies

No new external dependencies. `crypto/rand` and `encoding/yaml` (via `gopkg.in/yaml.v3`) cover all needs.
