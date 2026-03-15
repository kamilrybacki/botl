# Config TUI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `botl config` TUI subcommand for persistent configuration of clone mode and port blocking.

**Architecture:** New `internal/config` package handles YAML config load/save/validation. New `cmd/config.go` implements the raw-terminal TUI menu. `cmd/run.go` loads config as flag defaults. `internal/container/run.go` and `entrypoint.sh` consume the new settings.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `golang.org/x/term` (existing), Cobra (existing), iptables (container-side)

---

## Chunk 1: Config Package

### Task 1: Create config struct, load, save, and validation

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests for config loading/saving/validation**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Clone.Mode != "shallow" {
		t.Errorf("default clone mode = %q, want %q", cfg.Clone.Mode, "shallow")
	}
	if len(cfg.Network.BlockedPorts) != 0 {
		t.Errorf("default blocked ports = %v, want empty", cfg.Network.BlockedPorts)
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("Load nonexistent should not error: %v", err)
	}
	if cfg.Clone.Mode != "shallow" {
		t.Errorf("missing file should return defaults, got clone mode %q", cfg.Clone.Mode)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("clone:\n  mode: deep\nnetwork:\n  blocked_ports: [8080, 3000]\n")
	os.WriteFile(path, data, 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load valid file: %v", err)
	}
	if cfg.Clone.Mode != "deep" {
		t.Errorf("clone mode = %q, want %q", cfg.Clone.Mode, "deep")
	}
	if len(cfg.Network.BlockedPorts) != 2 || cfg.Network.BlockedPorts[0] != 8080 {
		t.Errorf("blocked ports = %v, want [8080 3000]", cfg.Network.BlockedPorts)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(":::bad yaml:::"), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load invalid YAML should not error (returns defaults): %v", err)
	}
	if cfg.Clone.Mode != "shallow" {
		t.Errorf("invalid YAML should return defaults, got clone mode %q", cfg.Clone.Mode)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")
	cfg := Config{
		Clone:   CloneConfig{Mode: "deep"},
		Network: NetworkConfig{BlockedPorts: []int{5432}},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if loaded.Clone.Mode != "deep" {
		t.Errorf("round-trip clone mode = %q, want %q", loaded.Clone.Mode, "deep")
	}
	if len(loaded.Network.BlockedPorts) != 1 || loaded.Network.BlockedPorts[0] != 5432 {
		t.Errorf("round-trip blocked ports = %v, want [5432]", loaded.Network.BlockedPorts)
	}
}

func TestValidatePorts_Valid(t *testing.T) {
	if err := ValidatePorts([]int{80, 443, 8080, 65535}); err != nil {
		t.Errorf("valid ports should not error: %v", err)
	}
}

func TestValidatePorts_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		ports []int
	}{
		{"zero", []int{0}},
		{"negative", []int{-1}},
		{"too high", []int{65536}},
		{"mixed", []int{80, 99999}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePorts(tt.ports); err == nil {
				t.Errorf("ports %v should fail validation", tt.ports)
			}
		})
	}
}

func TestConfigPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	t.Setenv("HOME", "/home/test")
	path := Path()
	if path != "/custom/config/botl/config.yaml" {
		t.Errorf("path = %q, want %q", path, "/custom/config/botl/config.yaml")
	}
}

func TestConfigPath_DefaultXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/test")
	path := Path()
	if path != "/home/test/.config/botl/config.yaml" {
		t.Errorf("path = %q, want %q", path, "/home/test/.config/botl/config.yaml")
	}
}

func TestValidateCloneMode(t *testing.T) {
	if err := ValidateCloneMode("shallow"); err != nil {
		t.Errorf("shallow should be valid: %v", err)
	}
	if err := ValidateCloneMode("deep"); err != nil {
		t.Errorf("deep should be valid: %v", err)
	}
	if err := ValidateCloneMode("invalid"); err == nil {
		t.Error("invalid mode should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/config/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type CloneConfig struct {
	Mode string `yaml:"mode"`
}

type NetworkConfig struct {
	BlockedPorts []int `yaml:"blocked_ports"`
}

type Config struct {
	Clone   CloneConfig   `yaml:"clone"`
	Network NetworkConfig `yaml:"network"`
}

func Default() Config {
	return Config{
		Clone:   CloneConfig{Mode: "shallow"},
		Network: NetworkConfig{BlockedPorts: []int{}},
	}
}

func Path() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "botl", "config.yaml")
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: invalid config at %s, using defaults\n", path)
		return Default(), nil
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func ValidatePorts(ports []int) error {
	for _, p := range ports {
		if p < 1 || p > 65535 {
			return fmt.Errorf("port %d out of range (1-65535)", p)
		}
	}
	return nil
}

func ValidateCloneMode(mode string) error {
	if mode != "shallow" && mode != "deep" {
		return fmt.Errorf("invalid clone mode %q: must be 'shallow' or 'deep'", mode)
	}
	return nil
}
```

- [ ] **Step 4: Add yaml.v3 dependency**

Run: `cd /home/kamil-rybacki/Code/botl && go get gopkg.in/yaml.v3`

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/config/ -v`
Expected: PASS (all tests)

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add config package with YAML load/save and validation"
```

---

## Chunk 2: Config TUI Command

### Task 2: Create `botl config` subcommand with interactive TUI

**Files:**
- Create: `cmd/config.go`

- [ ] **Step 1: Write `cmd/config.go` with TUI**

The TUI reuses the raw terminal patterns from `cmd/botl-postrun/main.go` — arrow keys, vim bindings, ANSI colors. Two menu items: Clone mode (toggle) and Blocked ports (inline editor).

```go
package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kamilrybacki/botl/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	cfgColorReset  = "\033[0m"
	cfgColorBold   = "\033[1m"
	cfgColorDim    = "\033[2m"
	cfgColorGreen  = "\033[32m"
	cfgColorYellow = "\033[33m"
	cfgColorCyan   = "\033[36m"
	cfgColorRed    = "\033[31m"
	cfgCursorHide  = "\033[?25l"
	cfgCursorShow  = "\033[?25h"
	cfgClearLine   = "\033[2K"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure botl defaults via interactive TUI",
	RunE:  runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfgPath := config.Path()
	cfg, _ := config.Load(cfgPath)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("cannot enter raw terminal mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	fmt.Print(cfgCursorHide)
	defer fmt.Print(cfgCursorShow)

	selected := 0
	editing := false
	editBuf := ""
	editErr := ""
	items := 2 // Clone mode, Blocked ports

	renderMenu := func() {
		cloneLabel := "shallow (sanitized, depth=1)"
		if cfg.Clone.Mode == "deep" {
			cloneLabel = "deep (full history)"
		}

		portsLabel := "none"
		if len(cfg.Network.BlockedPorts) > 0 {
			parts := make([]string, len(cfg.Network.BlockedPorts))
			for i, p := range cfg.Network.BlockedPorts {
				parts[i] = strconv.Itoa(p)
			}
			portsLabel = strings.Join(parts, ", ")
		}

		fmt.Print("\r" + cfgClearLine)
		fmt.Printf("  %sbotl configuration%s (%s)\r\n", cfgColorBold, cfgColorReset, cfgPath)
		fmt.Print("\r" + cfgClearLine)
		fmt.Printf("  %s─────────────────────────────────────────────────%s\r\n", cfgColorDim, cfgColorReset)
		fmt.Print("\r" + cfgClearLine + "\r\n")

		// Clone mode
		fmt.Print("\r" + cfgClearLine)
		if selected == 0 {
			fmt.Printf("  %s▸ Clone mode        %s%s%s\r\n", cfgColorGreen, cfgColorCyan, cloneLabel, cfgColorReset)
		} else {
			fmt.Printf("    %sClone mode        %s%s\r\n", cfgColorDim, cloneLabel, cfgColorReset)
		}

		// Blocked ports
		fmt.Print("\r" + cfgClearLine)
		if selected == 1 && editing {
			fmt.Printf("  %s▸ Blocked ports     %s%s_%s\r\n", cfgColorGreen, cfgColorYellow, editBuf, cfgColorReset)
		} else if selected == 1 {
			fmt.Printf("  %s▸ Blocked ports     %s%s%s\r\n", cfgColorGreen, cfgColorYellow, portsLabel, cfgColorReset)
		} else {
			fmt.Printf("    %sBlocked ports     %s%s\r\n", cfgColorDim, portsLabel, cfgColorReset)
		}

		fmt.Print("\r" + cfgClearLine + "\r\n")

		// Error or help line
		fmt.Print("\r" + cfgClearLine)
		if editErr != "" {
			fmt.Printf("  %s%s✗ %s%s\r\n", cfgColorBold, cfgColorRed, editErr, cfgColorReset)
		} else if editing {
			fmt.Printf("  %sType port numbers separated by commas · enter confirm · esc cancel%s\r\n", cfgColorDim, cfgColorReset)
		} else {
			fmt.Printf("  %s↑/↓ navigate · enter select · q save & quit · ctrl+c discard & quit%s\r\n", cfgColorDim, cfgColorReset)
		}
	}

	clearMenu := func() {
		lines := 7 // header(2) + blank + 2 items + blank + help
		for i := 0; i < lines; i++ {
			fmt.Printf("\033[A\r" + cfgClearLine)
		}
	}

	renderMenu()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if editing {
			if n == 1 {
				switch buf[0] {
				case 13: // Enter - confirm edit
					editErr = ""
					ports, parseErr := parsePorts(editBuf)
					if parseErr != nil {
						editErr = parseErr.Error()
						clearMenu()
						renderMenu()
						continue
					}
					cfg.Network.BlockedPorts = ports
					editing = false
					editBuf = ""
					clearMenu()
					renderMenu()
				case 27: // Esc - cancel edit
					editing = false
					editBuf = ""
					editErr = ""
					clearMenu()
					renderMenu()
				case 127, 8: // Backspace
					if len(editBuf) > 0 {
						editBuf = editBuf[:len(editBuf)-1]
					}
					editErr = ""
					clearMenu()
					renderMenu()
				default:
					if (buf[0] >= '0' && buf[0] <= '9') || buf[0] == ',' || buf[0] == ' ' {
						editBuf += string(buf[0])
						editErr = ""
						clearMenu()
						renderMenu()
					}
				}
			}
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 'q': // Save and quit
				clearMenu()
				if err := config.Save(cfgPath, cfg); err != nil {
					fmt.Printf("\r%s%s✗ Failed to save: %s%s\r\n", cfgColorBold, cfgColorRed, err, cfgColorReset)
					return err
				}
				fmt.Printf("\r%s✓ Config saved to %s%s\r\n", cfgColorGreen, cfgPath, cfgColorReset)
				return nil
			case 3: // Ctrl+C - discard and quit
				clearMenu()
				fmt.Printf("\r%sDiscarded changes.%s\r\n", cfgColorDim, cfgColorReset)
				return nil
			case 13: // Enter - select item
				if selected == 0 {
					// Toggle clone mode
					if cfg.Clone.Mode == "shallow" {
						cfg.Clone.Mode = "deep"
					} else {
						cfg.Clone.Mode = "shallow"
					}
					clearMenu()
					renderMenu()
				} else if selected == 1 {
					// Enter port editing mode
					editing = true
					parts := make([]string, len(cfg.Network.BlockedPorts))
					for i, p := range cfg.Network.BlockedPorts {
						parts[i] = strconv.Itoa(p)
					}
					editBuf = strings.Join(parts, ", ")
					clearMenu()
					renderMenu()
				}
			case 'k': // vim up
				if selected > 0 {
					selected--
					clearMenu()
					renderMenu()
				}
			case 'j': // vim down
				if selected < items-1 {
					selected++
					clearMenu()
					renderMenu()
				}
			}
		}

		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				if selected > 0 {
					selected--
					clearMenu()
					renderMenu()
				}
			case 66: // Down
				if selected < items-1 {
					selected++
					clearMenu()
					renderMenu()
				}
			}
		}
	}

	return nil
}

func parsePorts(input string) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []int{}, nil
	}

	parts := strings.Split(input, ",")
	ports := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: must be a number", part)
		}
		if p < 1 || p > 65535 {
			return nil, fmt.Errorf("port %d out of range (1-65535)", p)
		}
		ports = append(ports, p)
	}
	return ports, nil
}
```

- [ ] **Step 2: Run tests and verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/config.go
git commit -m "feat: add botl config TUI subcommand"
```

---

## Chunk 3: Integrate Config with Run Command

### Task 3: Load config in `botl run` and add new CLI flags

**Files:**
- Modify: `cmd/run.go:21-31` (add fields to runOpts struct)
- Modify: `cmd/run.go:40-51` (add flags in init)
- Modify: `cmd/run.go:54-125` (load config in runRun, populate RunOpts)
- Modify: `internal/container/types.go:25-36` (add SanitizeGit, BlockedPorts to RunOpts)

- [ ] **Step 1: Write failing tests for new flags and config integration**

Add to `cmd/cmd_test.go`:

```go
func TestRunCommand_NewFlags(t *testing.T) {
	cmd := runCmd
	flagTests := []struct {
		name     string
		defValue string
	}{
		{"clone-mode", ""},
		{"blocked-ports", "[]"},
	}
	for _, tt := range flagTests {
		f := cmd.Flags().Lookup(tt.name)
		if f == nil {
			t.Errorf("run command missing --%s flag", tt.name)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestRunCommand_NewFlags -v`
Expected: FAIL

- [ ] **Step 3: Update `internal/container/types.go` — add new fields to RunOpts**

Add `SanitizeGit bool` and `BlockedPorts []int` to the `RunOpts` struct after `ClaudeConfigDir`:

```go
type RunOpts struct {
	Image           string
	RepoURL         string
	Branch          string
	Depth           int
	Prompt          string
	Mounts          []Mount
	EnvVars         []string
	Timeout         time.Duration
	OutputDir       string
	ClaudeConfigDir string
	SanitizeGit     bool  // Strip commit messages and reflog from .git
	BlockedPorts    []int // TCP ports to block inbound connections
}
```

- [ ] **Step 4: Update `cmd/run.go` — add fields, flags, and config loading**

Add to `runOpts` struct:
```go
cloneMode    string
blockedPorts []int
```

Add flags in `init()`:
```go
runCmd.Flags().StringVar(&runOpts.cloneMode, "clone-mode", "", "Clone mode: shallow or deep (default: from config)")
runCmd.Flags().IntSliceVar(&runOpts.blockedPorts, "blocked-ports", nil, "TCP ports to block inbound (default: from config)")
```

Update `runRun()` to load config before building `RunOpts`:
```go
// Load persistent config
cfg, _ := config.Load(config.Path())

// Apply config defaults, CLI flags override
cloneMode := cfg.Clone.Mode
if runOpts.cloneMode != "" {
	cloneMode = runOpts.cloneMode
}
if err := config.ValidateCloneMode(cloneMode); err != nil {
	return err
}

depth := runOpts.depth
sanitizeGit := false
if !cmd.Flags().Changed("depth") {
	if cloneMode == "shallow" {
		depth = 1
		sanitizeGit = true
	} else {
		depth = 0 // signals full clone (entrypoint omits --depth)
	}
} else if cloneMode == "shallow" {
	sanitizeGit = true
}

blockedPorts := cfg.Network.BlockedPorts
if cmd.Flags().Changed("blocked-ports") {
	blockedPorts = runOpts.blockedPorts
}
if err := config.ValidatePorts(blockedPorts); err != nil {
	return err
}
```

Then add to the `RunOpts` construction:
```go
opts := container.RunOpts{
	// ... existing fields ...
	Depth:          depth,
	SanitizeGit:    sanitizeGit,
	BlockedPorts:   blockedPorts,
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -v && go test ./internal/container/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/run.go cmd/cmd_test.go internal/container/types.go
git commit -m "feat: integrate config with run command and add new CLI flags"
```

---

## Chunk 4: Container-Side Changes

### Task 4: Update Docker args, entrypoint, and Dockerfile

**Files:**
- Modify: `internal/container/run.go:15-62` (pass new env vars, add NET_ADMIN conditionally)
- Modify: `internal/container/run_test.go` (new tests for sanitize and blocked ports args)
- Modify: `internal/container/dockerctx/entrypoint.sh` (git sanitization, iptables)
- Modify: `internal/container/dockerctx/Dockerfile` (install iptables)

- [ ] **Step 1: Write failing tests for new docker args**

Add to `internal/container/run_test.go`:

```go
func TestBuildDockerArgs_SanitizeGit(t *testing.T) {
	opts := RunOpts{
		Image:       "botl:latest",
		RepoURL:     "https://github.com/user/repo",
		Depth:       1,
		SanitizeGit: true,
	}
	args := buildDockerArgs(opts)
	assertEnvVar(t, args, "BOTL_SANITIZE_GIT=true")
}

func TestBuildDockerArgs_NoSanitizeGit(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}
	args := buildDockerArgs(opts)
	for _, arg := range args {
		if strings.HasPrefix(arg, "BOTL_SANITIZE_GIT=") {
			t.Error("should not set BOTL_SANITIZE_GIT when false")
		}
	}
}

func TestBuildDockerArgs_BlockedPorts(t *testing.T) {
	opts := RunOpts{
		Image:        "botl:latest",
		RepoURL:      "https://github.com/user/repo",
		Depth:        1,
		BlockedPorts: []int{8080, 3000},
	}
	args := buildDockerArgs(opts)
	assertEnvVar(t, args, "BOTL_BLOCKED_PORTS=8080,3000")
	assertContains(t, args, "--cap-add")
	assertContains(t, args, "NET_ADMIN")
}

func TestBuildDockerArgs_NoBlockedPorts(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}
	args := buildDockerArgs(opts)
	for _, arg := range args {
		if arg == "--cap-add" || arg == "NET_ADMIN" {
			t.Error("should not add NET_ADMIN when no blocked ports")
		}
		if strings.HasPrefix(arg, "BOTL_BLOCKED_PORTS=") {
			t.Error("should not set BOTL_BLOCKED_PORTS when empty")
		}
	}
}

func TestBuildDockerArgs_DeepCloneDepthZero(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   0,
	}
	args := buildDockerArgs(opts)
	// Depth=0 means "no --depth flag" so BOTL_DEPTH should be "0"
	// entrypoint will interpret 0 as "omit --depth"
	assertEnvVar(t, args, "BOTL_DEPTH=0")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/container/ -v`
Expected: FAIL (new assertions not satisfied)

- [ ] **Step 3: Update `internal/container/run.go` — `buildDockerArgs`**

Add after the existing env var section (after line 34), before the mounts section:

```go
if opts.SanitizeGit {
	args = append(args, "-e", "BOTL_SANITIZE_GIT=true")
}
if len(opts.BlockedPorts) > 0 {
	portStrs := make([]string, len(opts.BlockedPorts))
	for i, p := range opts.BlockedPorts {
		portStrs[i] = strconv.Itoa(p)
	}
	args = append(args, "-e", "BOTL_BLOCKED_PORTS="+strings.Join(portStrs, ","))
}
```

Add after `--cap-drop ALL` (after line 52), before `--security-opt`:

```go
// Add NET_ADMIN capability only when port blocking is configured
if len(opts.BlockedPorts) > 0 {
	args = append(args, "--cap-add", "NET_ADMIN")
}
```

- [ ] **Step 4: Run container tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/container/ -v`
Expected: PASS

- [ ] **Step 5: Update `internal/container/dockerctx/Dockerfile`**

Add `iptables` to the apt-get install line. Change the `USER botl` / `ENTRYPOINT` setup to allow the entrypoint to run initial setup as root then drop privileges:

```dockerfile
FROM node:22-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends git ca-certificates iptables gosu && \
    rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code@latest

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

COPY botl-postrun /usr/local/bin/botl-postrun
RUN chmod +x /usr/local/bin/botl-postrun

# Create non-root user for workspace operations
RUN useradd -m -s /bin/sh botl && \
    mkdir -p /workspace /output && \
    chown botl:botl /workspace /output

# Entrypoint runs as root for iptables, then drops to botl user
WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
```

Note: `USER botl` is removed — the entrypoint handles privilege dropping via `gosu`.

- [ ] **Step 6: Update `internal/container/dockerctx/entrypoint.sh`**

Replace the full entrypoint with:

```bash
#!/bin/sh
set -e

REPO_URL="${BOTL_REPO_URL}"
BRANCH="${BOTL_BRANCH}"
DEPTH="${BOTL_DEPTH:-1}"
PROMPT="${BOTL_PROMPT}"
SANITIZE_GIT="${BOTL_SANITIZE_GIT}"
BLOCKED_PORTS="${BOTL_BLOCKED_PORTS}"

if [ -z "$REPO_URL" ]; then
    echo "botl: error: BOTL_REPO_URL is not set" >&2
    exit 1
fi

# Validate REPO_URL looks like a git URL (https, ssh, or git protocol)
case "$REPO_URL" in
    https://*|git://*|git@*|ssh://*)
        ;;
    *)
        echo "botl: error: BOTL_REPO_URL must be an https://, git://, git@, or ssh:// URL" >&2
        exit 1
        ;;
esac

# --- Port blocking (requires root, done before dropping privileges) ---
if [ -n "$BLOCKED_PORTS" ]; then
    blocked_count=0
    for port in $(echo "$BLOCKED_PORTS" | tr ',' ' '); do
        if echo "$port" | grep -qE '^[0-9]+$' && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
            if iptables -A INPUT -p tcp --dport "$port" -j REJECT 2>&1; then
                blocked_count=$((blocked_count + 1))
            else
                echo "botl: warning: failed to block port $port" >&2
            fi
        else
            echo "botl: warning: invalid port '$port', skipping" >&2
        fi
    done
    if [ "$blocked_count" -gt 0 ]; then
        echo "botl: blocked $blocked_count inbound port(s)" >&2
    fi
fi

# --- Clone repository (as botl user) ---
echo "botl: cloning ${REPO_URL} (depth=${DEPTH}, branch=${BRANCH:-default})..." >&2
if [ "$DEPTH" -eq 0 ] 2>/dev/null; then
    # Deep clone: omit --depth flag
    if [ -n "$BRANCH" ]; then
        gosu botl git clone --branch "${BRANCH}" "${REPO_URL}" /workspace/repo
    else
        gosu botl git clone "${REPO_URL}" /workspace/repo
    fi
else
    if [ -n "$BRANCH" ]; then
        gosu botl git clone --depth "${DEPTH}" --branch "${BRANCH}" "${REPO_URL}" /workspace/repo
    else
        gosu botl git clone --depth "${DEPTH}" "${REPO_URL}" /workspace/repo
    fi
fi
cd /workspace/repo

# Configure git for commits inside container
gosu botl git config user.email "botl@container"
gosu botl git config user.name "botl"

# Record the initial HEAD so postrun can detect new commits
INITIAL_HEAD="$(gosu botl git rev-parse HEAD 2>/dev/null || echo '')"

# --- Git sanitization (shallow mode) ---
if [ "$SANITIZE_GIT" = "true" ]; then
    echo "botl: sanitizing git history..." >&2
    gosu botl git commit --amend --allow-empty -m "initial" 2>/dev/null || true
    gosu botl git reflog expire --expire=now --all 2>/dev/null || true
    gosu botl git gc --prune=now 2>/dev/null || true
    # Re-record HEAD after sanitization
    INITIAL_HEAD="$(gosu botl git rev-parse HEAD 2>/dev/null || echo '')"
fi

export BOTL_INITIAL_HEAD="$INITIAL_HEAD"

# --- Launch Claude Code (as botl user) ---
if [ -n "$PROMPT" ]; then
    gosu botl claude --dangerously-skip-permissions -p "$PROMPT"
else
    gosu botl claude --dangerously-skip-permissions
fi

# Run the post-session TUI to handle results
exec gosu botl botl-postrun
```

- [ ] **Step 7: Run all tests**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/container/run.go internal/container/run_test.go internal/container/dockerctx/Dockerfile internal/container/dockerctx/entrypoint.sh
git commit -m "feat: add git sanitization and port blocking to container runtime"
```

---

## Chunk 5: Update Entrypoint Tests

### Task 5: Update shell-level entrypoint tests

**Files:**
- Modify: `internal/container/dockerctx/entrypoint_test.sh`

- [ ] **Step 1: Read current entrypoint_test.sh to understand test patterns**

- [ ] **Step 2: Add tests for new entrypoint behavior**

Add test cases for:
- `BOTL_DEPTH=0` omits `--depth` flag (deep clone)
- `BOTL_SANITIZE_GIT=true` triggers git sanitization
- `BOTL_BLOCKED_PORTS` validates and applies port rules
- Invalid port values produce warnings

- [ ] **Step 3: Run entrypoint tests**

Run: `cd /home/kamil-rybacki/Code/botl && make test-entrypoint` (or equivalent)
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/container/dockerctx/entrypoint_test.sh
git commit -m "test: add entrypoint tests for git sanitization and port blocking"
```

---

## Chunk 6: Final Integration and Verification

### Task 6: Run full test suite and verify

- [ ] **Step 1: Run all Go tests**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./... -v -count=1`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd /home/kamil-rybacki/Code/botl && golangci-lint run ./...`
Expected: PASS (or only pre-existing warnings)

- [ ] **Step 3: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 4: Final commit if any adjustments were needed**
