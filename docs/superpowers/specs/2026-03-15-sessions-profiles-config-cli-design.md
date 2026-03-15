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

Both directories are created on demand. `XDG_CONFIG_HOME` defaults to `~/.config`; `XDG_DATA_HOME` defaults to `~/.local/share`. Both XDG env vars are respected.

## Shared Run Configuration Struct

Both session records and profiles share the same run configuration shape. A single `RunConfig` struct lives in `internal/runconfig` to avoid duplication:

```go
type RunConfig struct {
    CloneMode    string        `yaml:"clone_mode"`
    Depth        int           `yaml:"depth"`
    BlockedPorts []int         `yaml:"blocked_ports"`
    Timeout      time.Duration `yaml:"timeout"`
    Image        string        `yaml:"image"`
    OutputDir    string        `yaml:"output_dir"`
    EnvVarKeys   []string      `yaml:"env_var_keys"` // keys only, never values
    // Mounts intentionally excluded: host-specific paths are not portable across machines
}
// Note: time.Duration marshals as nanosecond int64 in YAML (e.g. 1800000000000 for 30m).
// The "30m0s" shown in YAML examples is illustrative; actual on-disk format is an integer.
// botl profiles show output key ordering is implementation-defined (yaml.v3 marshals alphabetically).
```

Mounts are not stored in profiles or session records. They are host-specific and not reusable.

## Data Model

### Session Record (`~/.local/share/botl/sessions/<id>.yaml`)

Written **after** all flag merging is complete (CLI flags > profile > config > defaults) and **after** env vars have been resolved — capturing exactly what was passed to the container. Values from `--env KEY=VALUE` flags are recorded as keys only (values stripped before write).

```yaml
id: a3f2c1d8
created_at: 2026-03-15T14:32:00Z   # always UTC
repo_url: https://github.com/user/repo   # reference only, not copied to profile
branch: main
status: success                          # "pending" | "success" | "failed"

run:
  clone_mode: deep
  depth: 0
  blocked_ports: [8080, 3000]
  timeout: 30m0s
  image: botl:latest
  output_dir: /home/user/botl-output   # resolved absolute path
  env_var_keys: ["FOO", "BAR"]         # keys only, never values
```

The session record is written twice:
- At step 8 (before container run) with `status: pending`
- Updated to `status: success` or `status: failed` after the container exits

`botl label` rejects sessions with `status: pending` or `status: failed` with: `botl: error: session "a3f2c1d8" did not complete successfully (status: failed)`

### Profile (`~/.config/botl/profiles/<name>.yaml`)

Created by `botl label`. Contains the `run` section copied from the session record, plus metadata. Mounts are not included (host-specific).

```yaml
name: my-secure-env
created_at: 2026-03-15T14:35:00Z   # always UTC
source_session: a3f2c1d8

run:
  clone_mode: deep
  depth: 0
  blocked_ports: [8080, 3000]
  timeout: 30m0s
  image: botl:latest
  output_dir: /home/user/botl-output
  env_var_keys: ["FOO", "BAR"]
```

## Profile Name Rules

Profile names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`: starts with alphanumeric, followed by alphanumeric, hyphens, or underscores, maximum 63 characters. This prevents path traversal and shell quoting issues. Profile names are used directly as filenames (`<name>.yaml`).

## Session ID Generation

8-character lowercase hex string generated via `crypto/rand` (stdlib, no external dependency). If a session file with that ID already exists, retry up to 5 times before returning an error: `botl: error: could not generate unique session ID after 5 attempts`. Probability of collision is ~1 in 4 billion per attempt, so retries will never be needed in practice.

## Flag Resolution Order

```
CLI flags > --with-label profile > config file > built-in defaults
```

Each layer fills in only what the layer above left unset. The session record is written after this merge is complete, capturing the effective values.

**`botl run --with-label` execution flow:**

```
1.  Generate session ID
2.  Load config file → base defaults
3.  If --with-label: load profile → override config defaults
4.  Apply explicit CLI flags → override everything
5.  Validate all opts (clone mode, ports, branch, repo URL)
6.  Resolve env var keys from profile (see below)
7.  Resolve absolute path for output_dir, create if needed
8.  Write session record to disk
9.  Print session ID: "botl: session id: a3f2c1d8"
10. Execute container run
11. Print session ID again: "botl: session complete · id: a3f2c1d8"
```

## Env Var Resolution at Runtime

When a profile has `env_var_keys`, the following runs at step 6 above:

1. Explicit `--env KEY=VALUE` CLI flag → used as-is, no prompting.
2. Key already set in caller's shell environment → picked up silently.
3. Key not set and stdin is a TTY → prompt interactively: `botl: env var FOO not set · enter value: `
4. Key not set and stdin is **not** a TTY (CI, piped input) → hard error immediately: `botl: error: env var FOO not set and stdin is not a tty; set it in the environment before running`
5. Interactive prompt receives empty input → hard error: `botl: error: env var FOO not provided, aborting`

## Commands

### `botl run`

```
botl run [--with-label=<name>] [flags] <repo-url>
```

New flag:

| Flag | Type | Description |
|------|------|-------------|
| `--with-label` | string | Load named profile as defaults |

Error cases for `--with-label`:
- Profile not found → `botl: error: profile "foo" not found (~/.config/botl/profiles/foo.yaml)`
- Invalid profile name → `botl: error: invalid profile name "foo bar": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,62}`

### `botl label`

Intentionally top-level (not under `botl profiles`) to reflect the verb-centric UX: labeling is an action you take on a session.

```
botl label <session-id> <name> [--force]
```

Reads session record for `<session-id>`, copies its `run` section (env var keys only, no values) into a new profile. `--force` unconditionally replaces an existing profile file with the new one (complete replacement, no merging).

**Output on success:**
```
botl: profile "my-secure-env" saved (~/.config/botl/profiles/my-secure-env.yaml)
```

**Error cases:**
- Session not found: `botl: error: session "a3f2c1d8" not found (~/.local/share/botl/sessions/a3f2c1d8.yaml)`
- Session not successfully completed: `botl: error: session "a3f2c1d8" did not complete successfully (status: failed)`
- Profile name already taken: `botl: error: profile "my-secure-env" already exists (use --force to overwrite)`
- Invalid profile name: `botl: error: invalid profile name "my env": must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,62}`

### `botl profiles list`

```
$ botl profiles list
NAME            CREATED       SESSION
my-secure-env   2026-03-15    a3f2c1d8
dev-strict      2026-03-14    b2e4f9a1
```

If no profiles exist: `botl: no profiles found — use 'botl label <session-id> <name>' to create one`

### `botl profiles show <name>`

Prints the full profile YAML to stdout. If the profile has `env_var_keys`, a note is appended to stderr: `# note: this profile requires env vars: FOO, BAR`

Error: profile not found → `botl: error: profile "<name>" not found`

### `botl profiles delete <name>`

```
$ botl profiles delete my-secure-env
Delete profile "my-secure-env"? [y/N] y
botl: profile "my-secure-env" deleted
```

Supports `--yes` / `-y` to skip confirmation (for scripting):
```
$ botl profiles delete my-secure-env --yes
botl: profile "my-secure-env" deleted
```

Error: profile not found → `botl: error: profile "my-secure-env" not found`

### `botl config set <key> <value>`

Sets a config value and writes the config file.

```
$ botl config set clone-mode deep
botl: clone-mode = deep
```

**Error cases:**
- Unknown key: `botl: error: unknown config key "foo"; valid keys: clone-mode, blocked-ports`
- Invalid value: `botl: error: invalid value for clone-mode: must be "shallow" or "deep"`

### `botl config get <key>`

Prints the current effective value (from config file, or built-in default if unset).

```
$ botl config get clone-mode
shallow
$ botl config get blocked-ports
(none)
```

**Error cases:**
- Unknown key: same error as `set`

### `botl config list`

Prints all config keys and their effective values.

```
$ botl config list
clone-mode      shallow
blocked-ports   (none)
```

Valid config keys and values:

| Key | Valid values | Default |
|-----|-------------|---------|
| `clone-mode` | `shallow`, `deep` | `shallow` |
| `blocked-ports` | comma-separated integers 1–65535 (e.g. `8080,3000`), or empty string to clear | `(none)` |

## Code Changes

### New Packages

| Package | Responsibility |
|---------|---------------|
| `internal/runconfig` | Shared `RunConfig` struct used by both session and profile packages |
| `internal/session` | Session ID generation (`crypto/rand`), write/read session records, XDG data path |
| `internal/profile` | Load, save, list, delete profiles; name validation; XDG config path |

### New/Modified Files

| File | Change |
|------|--------|
| `internal/runconfig/runconfig.go` | New: shared `RunConfig` struct |
| `internal/runconfig/runconfig_test.go` | New: unit tests |
| `internal/session/session.go` | New: ID gen, record struct, write/read |
| `internal/session/session_test.go` | New: unit tests |
| `internal/profile/profile.go` | New: profile struct, CRUD, name validation |
| `internal/profile/profile_test.go` | New: unit tests |
| `cmd/label.go` | New: `botl label` command |
| `cmd/profiles.go` | New: `botl profiles` subcommands (list, show, delete) |
| `cmd/config.go` | Replace TUI with `set`, `get`, `list` subcommands. `configCmd` becomes a group command (no `RunE`); bare `botl config` prints help and exits 0. The existing `parsePorts` function must be retained or moved to `internal/profile` (it is referenced by `cmd_test.go`). Remove `golang.org/x/term` import if no longer used elsewhere in `cmd`. Run `go mod tidy` after implementation — `gopkg.in/yaml.v3` will be promoted from indirect to direct dependency. |
| `cmd/run.go` | Add `--with-label` flag, session ID, profile loading, env var resolution |
| `internal/config/config.go` | Keep as-is (struct, load, save, validate) |

### Superseded Spec

`2026-03-15-config-tui-design.md` is superseded. The `internal/config` package implementation from that spec's plan remains valid; only the TUI portion of `cmd/config.go` is replaced.

## Dependencies

No new external dependencies. `crypto/rand` is stdlib. `gopkg.in/yaml.v3` is already required by the config package.

## Implementation Notes

- The `BOTL_` namespace check in `cmd/run.go` applies only to explicitly passed `--env KEY=VALUE` flags. It is not re-applied to env var keys resolved from a profile's `env_var_keys` (those were already validated clean at label time).
- Profile `output_dir` is an absolute path captured from the session. When loading a profile on a different machine or as a different user, the path may not exist. In that case, the value from the profile is used as-is; if the directory cannot be created, `botl run` will error at step 7 of the execution flow. This is a known portability limitation of profiles with absolute paths.

## Known Limitations / Future Work

- Session files accumulate indefinitely. A future `botl sessions list` and `botl sessions prune` command will handle housekeeping.
- Profiles are currently global (not project-scoped).
- Profiles with absolute `output_dir` paths are not portable across users or machines. Per-project profile overrides are not in scope.
