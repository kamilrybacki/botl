# Sessions, Profiles & Config CLI Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add session tracking, reusable profiles via labeling, and replace the config TUI with a CLI — enabling users to save and reuse run configurations without re-entering flags.

**Architecture:** Three new internal packages (`internal/runconfig`, `internal/session`, `internal/profile`) provide the data model and persistence. Four new Cobra commands (`label`, `profiles list/show/delete`) and one rewritten command (`config set/get/list`) wire them to the CLI. `cmd/run.go` gains session ID lifecycle and `--with-label` profile loading.

**Tech Stack:** Go 1.24, Cobra, `gopkg.in/yaml.v3` (already indirect), `crypto/rand` (stdlib)

**Spec:** `docs/superpowers/specs/2026-03-15-sessions-profiles-config-cli-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/runconfig/runconfig.go` | Shared `RunConfig` struct and `Default()` constructor |
| `internal/runconfig/runconfig_test.go` | YAML round-trip, default values tests |
| `internal/session/session.go` | Session ID gen, `Record` struct, write/read/update-status, XDG data path |
| `internal/session/session_test.go` | ID gen, write/read, status update, collision retry tests |
| `internal/profile/profile.go` | `Profile` struct, save/load/list/delete, name validation, XDG config path |
| `internal/profile/profile_test.go` | CRUD, name validation, list/delete tests |
| `cmd/label.go` | `botl label <session-id> <name>` command |
| `cmd/profiles.go` | `botl profiles list/show/delete` commands |

### Modified Files

| File | Change |
|------|--------|
| `cmd/root.go` | Set `SilenceErrors: true` on `rootCmd`; add `botl: error:` prefix in `Execute()`. |
| `main.go` | Remove duplicate error printing (Execute() now handles it). |
| `cmd/config.go` | Delete TUI, replace with `set`, `get`, `list` subcommands. Keep `parsePorts`. |
| `cmd/cmd_test.go` | Add tests for new commands; update `TestParsePorts` if `parsePorts` moves. |
| `cmd/run.go` | Add `--with-label` flag, session ID lifecycle, profile loading, env var resolution. |

### Unchanged Files

| File | Note |
|------|------|
| `internal/config/config.go` | Kept as-is. Used by `cmd/config.go` and `cmd/run.go` for global defaults. |
| `internal/config/config_test.go` | Kept as-is. |
| `internal/ansi/ansi.go` | Reused by new commands for colored output. |
| `go.mod` | `go mod tidy` will promote `gopkg.in/yaml.v3` from indirect to direct. `golang.org/x/term` stays (used by `cmd/botl-postrun/main.go`). |

---

## Chunk 1: Shared RunConfig Package

### Task 1: Create `internal/runconfig` with `RunConfig` struct

**Files:**
- Create: `internal/runconfig/runconfig.go`
- Create: `internal/runconfig/runconfig_test.go`

- [ ] **Step 1: Write failing test for RunConfig defaults and YAML round-trip**

Create `internal/runconfig/runconfig_test.go`:

```go
package runconfig

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	rc := Default()
	if rc.CloneMode != "shallow" {
		t.Errorf("CloneMode = %q, want %q", rc.CloneMode, "shallow")
	}
	if rc.Depth != 1 {
		t.Errorf("Depth = %d, want 1", rc.Depth)
	}
	if len(rc.BlockedPorts) != 0 {
		t.Errorf("BlockedPorts = %v, want empty", rc.BlockedPorts)
	}
	if rc.Timeout != 30*time.Minute {
		t.Errorf("Timeout = %v, want 30m", rc.Timeout)
	}
	if rc.Image != "botl:latest" {
		t.Errorf("Image = %q, want %q", rc.Image, "botl:latest")
	}
	if rc.OutputDir != "./botl-output" {
		t.Errorf("OutputDir = %q, want %q", rc.OutputDir, "./botl-output")
	}
	if rc.EnvVarKeys != nil {
		t.Errorf("EnvVarKeys = %v, want nil", rc.EnvVarKeys)
	}
}

func TestRunConfig_YAMLRoundTrip(t *testing.T) {
	original := RunConfig{
		CloneMode:    "deep",
		Depth:        0,
		BlockedPorts: []int{8080, 3000},
		Timeout:      45 * time.Minute,
		Image:        "botl:custom",
		OutputDir:    "/tmp/out",
		EnvVarKeys:   []string{"FOO", "BAR"},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded RunConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.CloneMode != original.CloneMode {
		t.Errorf("CloneMode = %q, want %q", loaded.CloneMode, original.CloneMode)
	}
	if loaded.Depth != original.Depth {
		t.Errorf("Depth = %d, want %d", loaded.Depth, original.Depth)
	}
	if len(loaded.BlockedPorts) != 2 || loaded.BlockedPorts[0] != 8080 {
		t.Errorf("BlockedPorts = %v, want %v", loaded.BlockedPorts, original.BlockedPorts)
	}
	if loaded.Timeout != original.Timeout {
		t.Errorf("Timeout = %v, want %v", loaded.Timeout, original.Timeout)
	}
	if loaded.Image != original.Image {
		t.Errorf("Image = %q, want %q", loaded.Image, original.Image)
	}
	if loaded.OutputDir != original.OutputDir {
		t.Errorf("OutputDir = %q, want %q", loaded.OutputDir, original.OutputDir)
	}
	if len(loaded.EnvVarKeys) != 2 || loaded.EnvVarKeys[0] != "FOO" {
		t.Errorf("EnvVarKeys = %v, want %v", loaded.EnvVarKeys, original.EnvVarKeys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/runconfig/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

Create `internal/runconfig/runconfig.go`:

```go
package runconfig

import "time"

// RunConfig holds all run-level configuration that is captured in session
// records and reusable profiles. Mounts are intentionally excluded because
// host-specific paths are not portable across machines.
type RunConfig struct {
	CloneMode    string        `yaml:"clone_mode"`
	Depth        int           `yaml:"depth"`
	BlockedPorts []int         `yaml:"blocked_ports"`
	Timeout      time.Duration `yaml:"timeout"`
	Image        string        `yaml:"image"`
	OutputDir    string        `yaml:"output_dir"`
	EnvVarKeys   []string      `yaml:"env_var_keys,omitempty"`
}

// Default returns a RunConfig with built-in defaults matching botl's
// zero-configuration behavior.
func Default() RunConfig {
	return RunConfig{
		CloneMode:    "shallow",
		Depth:        1,
		BlockedPorts: []int{},
		Timeout:      30 * time.Minute,
		Image:        "botl:latest",
		OutputDir:    "./botl-output",
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/runconfig/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/runconfig/
git commit -m "feat: add internal/runconfig package with shared RunConfig struct"
```

---

## Chunk 2: Session Package

### Task 2: Create `internal/session` with ID generation, write/read, status updates

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

- [ ] **Step 1: Write failing tests for session operations**

Create `internal/session/session_test.go`:

```go
package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamilrybacki/botl/internal/runconfig"
)

func TestGenerateID(t *testing.T) {
	id, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID: %v", err)
	}
	if len(id) != 8 {
		t.Errorf("ID length = %d, want 8", len(id))
	}
	// Should be hex characters only
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("ID contains non-hex character %q", string(c))
		}
	}
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateID()
		if err != nil {
			t.Fatalf("GenerateID: %v", err)
		}
		if seen[id] {
			t.Errorf("duplicate ID %q after %d iterations", id, i)
		}
		seen[id] = true
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	rec := Record{
		ID:        "abcd1234",
		CreatedAt: time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		RepoURL:   "https://github.com/user/repo",
		Branch:    "main",
		Status:    StatusPending,
		Run: runconfig.RunConfig{
			CloneMode:    "deep",
			Depth:        0,
			BlockedPorts: []int{8080},
			Timeout:      30 * time.Minute,
			Image:        "botl:latest",
			OutputDir:    "/tmp/out",
			EnvVarKeys:   []string{"FOO"},
		},
	}

	if err := Write(rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// File should exist
	path := filepath.Join(dir, "botl", "sessions", "abcd1234.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file not found: %v", err)
	}

	loaded, err := Read("abcd1234")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if loaded.ID != "abcd1234" {
		t.Errorf("ID = %q, want %q", loaded.ID, "abcd1234")
	}
	if loaded.Status != StatusPending {
		t.Errorf("Status = %q, want %q", loaded.Status, StatusPending)
	}
	if loaded.Run.CloneMode != "deep" {
		t.Errorf("Run.CloneMode = %q, want %q", loaded.Run.CloneMode, "deep")
	}
	if len(loaded.Run.EnvVarKeys) != 1 || loaded.Run.EnvVarKeys[0] != "FOO" {
		t.Errorf("Run.EnvVarKeys = %v, want [FOO]", loaded.Run.EnvVarKeys)
	}
}

func TestRead_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	_, err := Read("nonexistent")
	if err == nil {
		t.Error("Read nonexistent should error")
	}
}

func TestUpdateStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	rec := Record{
		ID:        "abcd1234",
		CreatedAt: time.Now().UTC(),
		RepoURL:   "https://github.com/user/repo",
		Status:    StatusPending,
		Run:       runconfig.Default(),
	}
	if err := Write(rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := UpdateStatus("abcd1234", StatusSuccess); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	loaded, err := Read("abcd1234")
	if err != nil {
		t.Fatalf("Read after update: %v", err)
	}
	if loaded.Status != StatusSuccess {
		t.Errorf("Status = %q, want %q", loaded.Status, StatusSuccess)
	}
}

func TestDataDir_XDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	path := Dir()
	if path != "/custom/data/botl/sessions" {
		t.Errorf("Dir = %q, want %q", path, "/custom/data/botl/sessions")
	}
}

func TestDataDir_DefaultXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/test")
	path := Dir()
	if path != "/home/test/.local/share/botl/sessions" {
		t.Errorf("Dir = %q, want %q", path, "/home/test/.local/share/botl/sessions")
	}
}

func TestGenerateUniqueID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	id, err := GenerateUniqueID()
	if err != nil {
		t.Fatalf("GenerateUniqueID: %v", err)
	}
	if len(id) != 8 {
		t.Errorf("ID length = %d, want 8", len(id))
	}
}

func TestGenerateUniqueID_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	// Pre-create a session to verify GenerateUniqueID skips it.
	// Since IDs are random, we can't force a collision, but we can
	// verify the function works when the directory already has files.
	sessDir := filepath.Join(dir, "botl", "sessions")
	if err := os.MkdirAll(sessDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "existing1.yaml"), []byte("id: existing1"), 0600); err != nil {
		t.Fatal(err)
	}

	id, err := GenerateUniqueID()
	if err != nil {
		t.Fatalf("GenerateUniqueID with existing files: %v", err)
	}
	if id == "existing1" {
		t.Error("GenerateUniqueID should not return ID of existing session file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/session/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

Create `internal/session/session.go`:

```go
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamilrybacki/botl/internal/runconfig"
	"gopkg.in/yaml.v3"
)

const (
	StatusPending = "pending"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// Record is a session written to disk for every botl run.
type Record struct {
	ID        string           `yaml:"id"`
	CreatedAt time.Time        `yaml:"created_at"`
	RepoURL   string           `yaml:"repo_url"`
	Branch    string           `yaml:"branch,omitempty"`
	Status    string           `yaml:"status"`
	Run       runconfig.RunConfig `yaml:"run"`
}

// Dir returns the sessions directory path, respecting XDG_DATA_HOME.
func Dir() string {
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdg = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdg, "botl", "sessions")
}

// GenerateID returns an 8-character lowercase hex string from crypto/rand.
func GenerateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateUniqueID generates an ID that does not collide with existing
// session files. Retries up to 5 times.
func GenerateUniqueID() (string, error) {
	dir := Dir()
	for i := 0; i < 5; i++ {
		id, err := GenerateID()
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, id+".yaml")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not generate unique session ID after 5 attempts")
}

// Write persists a session record to disk.
func Write(rec Record) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating sessions directory: %w", err)
	}

	data, err := yaml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshaling session record: %w", err)
	}

	path := filepath.Join(dir, rec.ID+".yaml")
	return os.WriteFile(path, data, 0600)
}

// Read loads a session record from disk by ID.
func Read(id string) (Record, error) {
	path := filepath.Join(Dir(), id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("session %q not found (%s)", id, path)
	}

	var rec Record
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return Record{}, fmt.Errorf("parsing session %q: %w", id, err)
	}
	return rec, nil
}

// UpdateStatus reads a session record, updates its status, and writes it back.
func UpdateStatus(id string, status string) error {
	rec, err := Read(id)
	if err != nil {
		return err
	}
	rec.Status = status
	return Write(rec)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/session/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat: add internal/session package with ID generation and record persistence"
```

---

## Chunk 3: Profile Package

### Task 3: Create `internal/profile` with CRUD, name validation, and listing

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

- [ ] **Step 1: Write failing tests for profile operations**

Create `internal/profile/profile_test.go`:

```go
package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamilrybacki/botl/internal/runconfig"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-profile", false},
		{"valid underscores", "my_profile_2", false},
		{"valid single char", "a", false},
		{"valid max length", "a" + strings.Repeat("b", 62), false},
		{"empty", "", true},
		{"starts with hyphen", "-foo", true},
		{"starts with underscore", "_foo", true},
		{"contains space", "my profile", true},
		{"contains slash", "my/profile", true},
		{"contains dot", "my.profile", true},
		{"too long", "a" + strings.Repeat("b", 63), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateName(%q) should error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateName(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	p := Profile{
		Name:          "test-profile",
		CreatedAt:     time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		SourceSession: "abcd1234",
		Run: runconfig.RunConfig{
			CloneMode:    "deep",
			Depth:        0,
			BlockedPorts: []int{8080},
			Timeout:      30 * time.Minute,
			Image:        "botl:latest",
			OutputDir:    "/tmp/out",
			EnvVarKeys:   []string{"FOO"},
		},
	}

	if err := Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("test-profile")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "test-profile" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test-profile")
	}
	if loaded.SourceSession != "abcd1234" {
		t.Errorf("SourceSession = %q, want %q", loaded.SourceSession, "abcd1234")
	}
	if loaded.Run.CloneMode != "deep" {
		t.Errorf("Run.CloneMode = %q, want %q", loaded.Run.CloneMode, "deep")
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	_, err := Load("nonexistent")
	if err == nil {
		t.Error("Load nonexistent should error")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if Exists("nope") {
		t.Error("Exists should be false for missing profile")
	}

	p := Profile{
		Name:      "existing",
		CreatedAt: time.Now().UTC(),
		Run:       runconfig.Default(),
	}
	if err := Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !Exists("existing") {
		t.Error("Exists should be true after save")
	}
}

func TestList_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	profiles, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("List should return empty, got %d", len(profiles))
	}
}

func TestList_Multiple(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	for _, name := range []string{"alpha", "beta"} {
		p := Profile{Name: name, CreatedAt: time.Now().UTC(), Run: runconfig.Default()}
		if err := Save(p); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	profiles, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 2 {
		t.Errorf("List count = %d, want 2", len(profiles))
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	p := Profile{Name: "to-delete", CreatedAt: time.Now().UTC(), Run: runconfig.Default()}
	if err := Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete("to-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if Exists("to-delete") {
		t.Error("profile should not exist after delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	err := Delete("nonexistent")
	if err == nil {
		t.Error("Delete nonexistent should error")
	}
}

func TestDir_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path := Dir()
	if path != "/custom/config/botl/profiles" {
		t.Errorf("Dir = %q, want %q", path, "/custom/config/botl/profiles")
	}
}

func TestDir_DefaultXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/test")
	path := Dir()
	if path != "/home/test/.config/botl/profiles" {
		t.Errorf("Dir = %q, want %q", path, "/home/test/.config/botl/profiles")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/profile/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

Create `internal/profile/profile.go`:

```go
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kamilrybacki/botl/internal/runconfig"
	"gopkg.in/yaml.v3"
)

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

// Profile is a reusable run configuration created from a session via botl label.
type Profile struct {
	Name          string              `yaml:"name"`
	CreatedAt     time.Time           `yaml:"created_at"`
	SourceSession string              `yaml:"source_session"`
	Run           runconfig.RunConfig `yaml:"run"`
}

// Dir returns the profiles directory path, respecting XDG_CONFIG_HOME.
func Dir() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "botl", "profiles")
}

// ValidateName checks that a profile name matches the allowed pattern.
func ValidateName(name string) error {
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,62}", name)
	}
	return nil
}

// Save writes a profile to disk, creating the profiles directory if needed.
func Save(p Profile) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating profiles directory: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	path := filepath.Join(dir, p.Name+".yaml")
	return os.WriteFile(path, data, 0600)
}

// Load reads a profile from disk by name.
func Load(name string) (Profile, error) {
	path := filepath.Join(Dir(), name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("profile %q not found (%s)", name, path)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parsing profile %q: %w", name, err)
	}
	return p, nil
}

// Exists returns true if a profile with the given name exists on disk.
func Exists(name string) bool {
	path := filepath.Join(Dir(), name+".yaml")
	_, err := os.Stat(path)
	return err == nil
}

// List returns all profiles found in the profiles directory.
func List() ([]Profile, error) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading profiles directory: %w", err)
	}

	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		p, err := Load(name)
		if err != nil {
			continue // skip corrupt files
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// Delete removes a profile file from disk.
func Delete(name string) error {
	path := filepath.Join(Dir(), name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}
	return os.Remove(path)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./internal/profile/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat: add internal/profile package with CRUD and name validation"
```

---

## Chunk 4: Config CLI (Replace TUI) and Error Prefix

### Task 4a: Update `cmd/root.go` for `botl: error:` prefix

The spec consistently uses `botl: error: <message>` but Cobra defaults to `Error: <message>`. We configure `SilenceErrors` and handle the prefix in `Execute()`.

**Files:**
- Modify: `cmd/root.go`

- [ ] **Step 1: Update `cmd/root.go`**

Replace the content of `cmd/root.go`:

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "botl",
	Short:         "Run Claude Code in an ephemeral Docker container",
	Long:          "botl launches Claude Code inside a temporary Docker container with read-only access to host packages and a shallow-cloned git repo as workspace.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "botl: error: %s\n", err)
		return err
	}
	return nil
}
```

- [ ] **Step 2: Update `main.go` to avoid double-printing errors**

Replace `main.go`:

```go
package main

import (
	"os"

	"github.com/kamilrybacki/botl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Run tests to verify nothing breaks**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/root.go main.go
git commit -m "feat: use botl: error: prefix for all CLI error messages"
```

### Task 4b: Rewrite `cmd/config.go` as `set`, `get`, `list` subcommands

**Files:**
- Modify: `cmd/config.go` (full rewrite, keep `parsePorts`)
- Modify: `cmd/cmd_test.go`

- [ ] **Step 1: Write failing tests for config subcommands**

Add to `cmd/cmd_test.go`:

```go
func TestConfigCommand_BareShowsHelp(t *testing.T) {
	rootCmd.SetArgs([]string{"config"})
	// Bare config should succeed (prints help)
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("bare config should succeed: %v", err)
	}
}

func TestConfigSetCommand_UnknownKey(t *testing.T) {
	rootCmd.SetArgs([]string{"config", "set", "unknown-key", "value"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("config set with unknown key should fail")
	}
}

func TestConfigSetCommand_ValidCloneMode(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	rootCmd.SetArgs([]string{"config", "set", "clone-mode", "deep"})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("config set clone-mode deep should succeed: %v", err)
	}
}

func TestConfigSetCommand_InvalidCloneMode(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	rootCmd.SetArgs([]string{"config", "set", "clone-mode", "invalid"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("config set clone-mode invalid should fail")
	}
}

func TestConfigGetCommand_Default(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	rootCmd.SetArgs([]string{"config", "get", "clone-mode"})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("config get clone-mode should succeed: %v", err)
	}
}

func TestConfigGetCommand_UnknownKey(t *testing.T) {
	rootCmd.SetArgs([]string{"config", "get", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("config get unknown key should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestConfig -v`
Expected: FAIL — `config` has `RunE` set (TUI), subcommands don't exist

- [ ] **Step 3: Rewrite `cmd/config.go`**

Replace the entire file content with:

```go
package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kamilrybacki/botl/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify botl configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all config values",
	Args:  cobra.NoArgs,
	RunE:  runConfigList,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}

var validConfigKeys = []string{"clone-mode", "blocked-ports"}

func validateConfigKey(key string) error {
	for _, k := range validConfigKeys {
		if k == key {
			return nil
		}
	}
	return fmt.Errorf("unknown config key %q; valid keys: %s", key, strings.Join(validConfigKeys, ", "))
}

func runConfigSet(_ *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	if err := validateConfigKey(key); err != nil {
		return err
	}

	cfgPath := config.Path()
	cfg, _ := config.Load(cfgPath)

	switch key {
	case "clone-mode":
		if err := config.ValidateCloneMode(value); err != nil {
			return fmt.Errorf("invalid value for clone-mode: must be \"shallow\" or \"deep\"")
		}
		cfg.Clone.Mode = value
	case "blocked-ports":
		ports, err := parsePorts(value)
		if err != nil {
			return fmt.Errorf("invalid value for blocked-ports: %w", err)
		}
		cfg.Network.BlockedPorts = ports
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "botl: %s = %s\n", key, value)
	return nil
}

func runConfigGet(_ *cobra.Command, args []string) error {
	key := args[0]
	if err := validateConfigKey(key); err != nil {
		return err
	}

	cfg, _ := config.Load(config.Path())

	switch key {
	case "clone-mode":
		fmt.Println(cfg.Clone.Mode)
	case "blocked-ports":
		if len(cfg.Network.BlockedPorts) == 0 {
			fmt.Println("(none)")
		} else {
			parts := make([]string, len(cfg.Network.BlockedPorts))
			for i, p := range cfg.Network.BlockedPorts {
				parts[i] = strconv.Itoa(p)
			}
			fmt.Println(strings.Join(parts, ", "))
		}
	}
	return nil
}

func runConfigList(_ *cobra.Command, _ []string) error {
	cfg, _ := config.Load(config.Path())

	portsLabel := "(none)"
	if len(cfg.Network.BlockedPorts) > 0 {
		parts := make([]string, len(cfg.Network.BlockedPorts))
		for i, p := range cfg.Network.BlockedPorts {
			parts[i] = strconv.Itoa(p)
		}
		portsLabel = strings.Join(parts, ", ")
	}

	fmt.Printf("%-16s%s\n", "clone-mode", cfg.Clone.Mode)
	fmt.Printf("%-16s%s\n", "blocked-ports", portsLabel)
	return nil
}

// parsePorts parses a comma-separated list of port numbers.
// Retained from the original config.go; used by config set and run.go.
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
		if err := config.ValidatePorts([]int{p}); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -v`
Expected: PASS (all existing tests + new config tests)

- [ ] **Step 5: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS — `golang.org/x/term` and `internal/ansi` imports removed from `cmd/config.go` (both still used elsewhere)

- [ ] **Step 6: Commit**

```bash
git add cmd/config.go cmd/cmd_test.go
git commit -m "feat: replace config TUI with set/get/list CLI subcommands"
```

---

## Chunk 5: Label and Profiles Commands

### Task 5: Create `cmd/label.go` — `botl label <session-id> <name>`

**Files:**
- Create: `cmd/label.go`
- Modify: `cmd/cmd_test.go`

- [ ] **Step 1: Write failing tests for `botl label`**

Add to `cmd/cmd_test.go`:

```go
func TestLabelCommand_MissingArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"label"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("label without args should fail")
	}
}

func TestLabelCommand_SessionNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	rootCmd.SetArgs([]string{"label", "nonexistent", "my-profile"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("label with nonexistent session should fail")
	}
}

func TestLabelCommand_InvalidName(t *testing.T) {
	rootCmd.SetArgs([]string{"label", "abcd1234", "invalid name"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("label with invalid name should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestLabel -v`
Expected: FAIL — label command does not exist

- [ ] **Step 3: Write `cmd/label.go`**

```go
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/kamilrybacki/botl/internal/session"
	"github.com/spf13/cobra"
)

var labelForce bool

var labelCmd = &cobra.Command{
	Use:   "label <session-id> <name>",
	Short: "Save a session's run config as a reusable profile",
	Args:  cobra.ExactArgs(2),
	RunE:  runLabel,
}

func init() {
	labelCmd.Flags().BoolVar(&labelForce, "force", false, "Overwrite existing profile")
	rootCmd.AddCommand(labelCmd)
}

func runLabel(_ *cobra.Command, args []string) error {
	sessionID, name := args[0], args[1]

	if err := profile.ValidateName(name); err != nil {
		return err
	}

	rec, err := session.Read(sessionID)
	if err != nil {
		return fmt.Errorf("session %q not found (%s/%s.yaml)", sessionID, session.Dir(), sessionID)
	}

	if rec.Status != session.StatusSuccess {
		return fmt.Errorf("session %q did not complete successfully (status: %s)", sessionID, rec.Status)
	}

	if profile.Exists(name) && !labelForce {
		return fmt.Errorf("profile %q already exists (use --force to overwrite)", name)
	}

	p := profile.Profile{
		Name:          name,
		CreatedAt:     time.Now().UTC(),
		SourceSession: sessionID,
		Run:           rec.Run,
	}

	if err := profile.Save(p); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "botl: profile %q saved (%s/%s.yaml)\n", name, profile.Dir(), name)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestLabel -v`
Expected: PASS

- [ ] **Step 5: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/label.go cmd/cmd_test.go
git commit -m "feat: add botl label command to promote sessions to profiles"
```

### Task 6: Create `cmd/profiles.go` — `botl profiles list/show/delete`

**Files:**
- Create: `cmd/profiles.go`
- Modify: `cmd/cmd_test.go`

- [ ] **Step 1: Write failing tests for `botl profiles` subcommands**

Add to `cmd/cmd_test.go`:

```go
func TestProfilesListCommand_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rootCmd.SetArgs([]string{"profiles", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("profiles list should succeed even when empty: %v", err)
	}
}

func TestProfilesShowCommand_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rootCmd.SetArgs([]string{"profiles", "show", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("profiles show nonexistent should fail")
	}
}

func TestProfilesDeleteCommand_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rootCmd.SetArgs([]string{"profiles", "delete", "--yes", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("profiles delete nonexistent should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestProfiles -v`
Expected: FAIL — profiles command does not exist

- [ ] **Step 3: Write `cmd/profiles.go`**

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Manage saved profiles",
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	Args:  cobra.NoArgs,
	RunE:  runProfilesList,
}

var profilesShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesShow,
}

var profilesDeleteYes bool

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesDelete,
}

func init() {
	profilesDeleteCmd.Flags().BoolVarP(&profilesDeleteYes, "yes", "y", false, "Skip confirmation prompt")
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesShowCmd)
	profilesCmd.AddCommand(profilesDeleteCmd)
	rootCmd.AddCommand(profilesCmd)
}

func runProfilesList(_ *cobra.Command, _ []string) error {
	profiles, err := profile.List()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Fprintln(os.Stderr, "botl: no profiles found — use 'botl label <session-id> <name>' to create one")
		return nil
	}

	fmt.Printf("%-20s %-12s %s\n", "NAME", "CREATED", "SESSION")
	for _, p := range profiles {
		fmt.Printf("%-20s %-12s %s\n", p.Name, p.CreatedAt.Format("2006-01-02"), p.SourceSession)
	}
	return nil
}

func runProfilesShow(_ *cobra.Command, args []string) error {
	name := args[0]
	p, err := profile.Load(name)
	if err != nil {
		return fmt.Errorf("profile %q not found", name)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	fmt.Print(string(data))

	if len(p.Run.EnvVarKeys) > 0 {
		fmt.Fprintf(os.Stderr, "# note: this profile requires env vars: %s\n", strings.Join(p.Run.EnvVarKeys, ", "))
	}
	return nil
}

func runProfilesDelete(_ *cobra.Command, args []string) error {
	name := args[0]

	if !profile.Exists(name) {
		return fmt.Errorf("profile %q not found", name)
	}

	if !profilesDeleteYes {
		fmt.Fprintf(os.Stderr, "Delete profile %q? [y/N] ", name)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "botl: cancelled")
			return nil
		}
	}

	if err := profile.Delete(name); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "botl: profile %q deleted\n", name)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestProfiles -v`
Expected: PASS

- [ ] **Step 5: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/profiles.go cmd/cmd_test.go
git commit -m "feat: add botl profiles list/show/delete commands"
```

---

## Chunk 6: Integrate Sessions and Profiles into `botl run`

### Task 7: Add session lifecycle, `--with-label`, and env var resolution to `cmd/run.go`

**Files:**
- Modify: `cmd/run.go`
- Modify: `cmd/cmd_test.go`

- [ ] **Step 1: Write failing tests for new run functionality**

Add to `cmd/cmd_test.go`:

```go
func TestRunCommand_WithLabelFlag(t *testing.T) {
	f := runCmd.Flags().Lookup("with-label")
	if f == nil {
		t.Error("run command missing --with-label flag")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -run TestRunCommand_WithLabelFlag -v`
Expected: FAIL — `--with-label` flag does not exist

- [ ] **Step 3: Update `cmd/run.go`**

Add imports at the top of `cmd/run.go`:

```go
import (
	// ... existing imports ...
	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/kamilrybacki/botl/internal/runconfig"
	"github.com/kamilrybacki/botl/internal/session"
)
```

Add field to `runOpts` struct:

```go
var runOpts struct {
	// ... existing fields ...
	withLabel string
}
```

Add flag registration in `init()`:

```go
runCmd.Flags().StringVar(&runOpts.withLabel, "with-label", "", "Load named profile as defaults")
```

Replace the `runRun` function body with the updated flow. The key changes are:

1. Generate session ID at the top
2. After loading config, load profile if `--with-label` is set and merge
3. Resolve env var keys from profile
4. Write session record as pending before container run
5. Update session status after container run
6. Print session ID at start and end

```go
func runRun(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Step 1: Generate session ID
	sessionID, err := session.GenerateUniqueID()
	if err != nil {
		return fmt.Errorf("botl: error: %w", err)
	}

	// Resolve ~/.claude for OAuth credentials
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	claudeConfigDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeConfigDir); os.IsNotExist(err) {
		return fmt.Errorf("~/.claude not found — run 'claude' once on your host to authenticate first")
	}

	// Step 2: Load persistent config → base defaults
	cfg, loadErr := config.Load(config.Path())
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not load config: %v\n", loadErr)
	}

	// Start with config-level defaults
	cloneMode := cfg.Clone.Mode
	blockedPorts := cfg.Network.BlockedPorts
	depth := runOpts.depth
	timeout := runOpts.timeout
	image := runOpts.image
	outputDir := runOpts.outputDir
	envVars := runOpts.envVars
	var profileEnvKeys []string

	// Step 3: If --with-label, load profile and override config defaults
	if runOpts.withLabel != "" {
		if err := profile.ValidateName(runOpts.withLabel); err != nil {
			return err
		}
		p, err := profile.Load(runOpts.withLabel)
		if err != nil {
			return fmt.Errorf("profile %q not found (%s/%s.yaml)", runOpts.withLabel, profile.Dir(), runOpts.withLabel)
		}
		fmt.Fprintf(os.Stderr, "botl: loading profile %q\n", runOpts.withLabel)

		// Profile overrides config defaults (only when CLI flag not explicitly set)
		if !cmd.Flags().Changed("clone-mode") {
			cloneMode = p.Run.CloneMode
		}
		if !cmd.Flags().Changed("blocked-ports") {
			blockedPorts = p.Run.BlockedPorts
		}
		if !cmd.Flags().Changed("depth") {
			depth = p.Run.Depth
		}
		if !cmd.Flags().Changed("timeout") {
			timeout = p.Run.Timeout
		}
		if !cmd.Flags().Changed("image") {
			image = p.Run.Image
		}
		if !cmd.Flags().Changed("output-dir") {
			outputDir = p.Run.OutputDir
		}
		profileEnvKeys = p.Run.EnvVarKeys
	}

	// Step 4: Apply explicit CLI flags (already in variables above via Changed checks)
	if cmd.Flags().Changed("clone-mode") {
		cloneMode = runOpts.cloneMode
	}
	if cmd.Flags().Changed("blocked-ports") {
		blockedPorts = runOpts.blockedPorts
	}

	// Step 5: Validate all opts
	if err := config.ValidateCloneMode(cloneMode); err != nil {
		return err
	}
	if err := config.ValidatePorts(blockedPorts); err != nil {
		return err
	}

	// Determine depth and sanitization from clone mode
	sanitizeGit := false
	if !cmd.Flags().Changed("depth") && runOpts.withLabel == "" {
		if cloneMode == "shallow" {
			depth = 1
			sanitizeGit = true
		} else {
			depth = 0
		}
	} else if cloneMode == "shallow" {
		sanitizeGit = true
	}

	if runOpts.branch != "" && !validBranchRe.MatchString(runOpts.branch) {
		return fmt.Errorf("invalid branch name %q: must match [a-zA-Z0-9._/~^:@{}[]-]", runOpts.branch)
	}
	if !validRepoURLRe.MatchString(repoURL) {
		return fmt.Errorf("invalid repo URL %q: must be https://, git@, or ssh:// URL", repoURL)
	}

	// Validate user-supplied env vars don't override internal namespace
	for _, env := range envVars {
		key := env
		if idx := strings.Index(env, "="); idx >= 0 {
			key = env[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(key), "BOTL_") {
			return fmt.Errorf("environment variable %q uses reserved BOTL_ namespace", key)
		}
	}

	// Step 6: Resolve env var keys from profile
	if len(profileEnvKeys) > 0 {
		resolvedEnvVars, err := resolveEnvVarKeys(profileEnvKeys, envVars)
		if err != nil {
			return err
		}
		envVars = append(envVars, resolvedEnvVars...)
	}

	// Step 7: Resolve and create output directory
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output dir: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0700); err != nil {
		return fmt.Errorf("cannot create output dir %s: %w", absOutputDir, err)
	}

	// Extract env var keys for session record (strip values)
	allEnvKeys := extractEnvKeys(envVars)

	// Step 8: Write session record as pending
	rec := session.Record{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		RepoURL:   repoURL,
		Branch:    runOpts.branch,
		Status:    session.StatusPending,
		Run: runconfig.RunConfig{
			CloneMode:    cloneMode,
			Depth:        depth,
			BlockedPorts: blockedPorts,
			Timeout:      timeout,
			Image:        image,
			OutputDir:    absOutputDir,
			EnvVarKeys:   allEnvKeys,
		},
	}
	if err := session.Write(rec); err != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not write session record: %v\n", err)
	}

	// Step 9: Print session ID
	fmt.Fprintf(os.Stderr, "botl: session id: %s\n", sessionID)

	// Auto-detect host packages
	var mounts []container.Mount
	if runOpts.mountPackages {
		detected := detect.HostPackages()
		for _, d := range detected {
			fmt.Fprintf(os.Stderr, "botl: auto-mounting %s → %s (ro)\n", d.Source, d.Target)
			mounts = append(mounts, d)
		}
	}

	// Add explicit mounts
	for _, m := range runOpts.mounts {
		parsed, parseErr := container.ParseMount(m)
		if parseErr != nil {
			return fmt.Errorf("invalid mount %q: %w", m, parseErr)
		}
		mounts = append(mounts, parsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nbotl: received interrupt, stopping container...")
		cancel()
	}()

	opts := container.RunOpts{
		Image:           image,
		RepoURL:         repoURL,
		Branch:          runOpts.branch,
		Depth:           depth,
		Prompt:          runOpts.prompt,
		Mounts:          mounts,
		EnvVars:         envVars,
		Timeout:         timeout,
		OutputDir:       absOutputDir,
		ClaudeConfigDir: claudeConfigDir,
		SanitizeGit:     sanitizeGit,
		BlockedPorts:    blockedPorts,
	}

	// Step 10: Execute container run
	runErr := container.Run(ctx, opts)

	// Step 11: Update session status and print ID
	status := session.StatusSuccess
	if runErr != nil {
		status = session.StatusFailed
	}
	if updateErr := session.UpdateStatus(sessionID, status); updateErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not update session status: %v\n", updateErr)
	}

	fmt.Fprintf(os.Stderr, "botl: session complete · id: %s\n", sessionID)
	return runErr
}

// resolveEnvVarKeys checks each required env var key from the profile.
// Keys already provided via --env are skipped. Keys found in the shell
// environment are picked up silently. Missing keys are prompted for
// interactively (or produce a hard error when stdin is not a TTY).
func resolveEnvVarKeys(keys []string, existingEnvVars []string) ([]string, error) {
	// Build set of keys already provided via --env
	provided := make(map[string]bool)
	for _, env := range existingEnvVars {
		if idx := strings.Index(env, "="); idx >= 0 {
			provided[env[:idx]] = true
		}
	}

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	var resolved []string

	for _, key := range keys {
		if provided[key] {
			continue // already set via --env
		}

		if val, ok := os.LookupEnv(key); ok {
			resolved = append(resolved, key+"="+val)
			continue
		}

		if !isTTY {
			return nil, fmt.Errorf("env var %s not set and stdin is not a tty; set it in the environment before running", key)
		}

		fmt.Fprintf(os.Stderr, "botl: env var %s not set · enter value: ", key)
		reader := bufio.NewReader(os.Stdin)
		val, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("env var %s not provided, aborting", key)
		}
		val = strings.TrimRight(val, "\r\n")
		if val == "" {
			return nil, fmt.Errorf("env var %s not provided, aborting", key)
		}
		resolved = append(resolved, key+"="+val)
	}

	return resolved, nil
}

// extractEnvKeys returns just the key names from KEY=VALUE env var strings.
func extractEnvKeys(envVars []string) []string {
	if len(envVars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envVars))
	for _, env := range envVars {
		if idx := strings.Index(env, "="); idx >= 0 {
			keys = append(keys, env[:idx])
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}
```

Note: Add these imports to `cmd/run.go`:
```go
"bufio"
"github.com/kamilrybacki/botl/internal/profile"
"github.com/kamilrybacki/botl/internal/runconfig"
"github.com/kamilrybacki/botl/internal/session"
"golang.org/x/term"
```

The `time` import is already present (from `time.Duration`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./cmd/ -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./... -v`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `cd /home/kamil-rybacki/Code/botl && golangci-lint run ./...`
Expected: PASS (or pre-existing warnings only)

- [ ] **Step 7: Run `go mod tidy`**

Run: `cd /home/kamil-rybacki/Code/botl && go mod tidy`
Expected: `gopkg.in/yaml.v3` promoted from indirect to direct in `go.mod`

- [ ] **Step 8: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add cmd/run.go cmd/cmd_test.go go.mod go.sum
git commit -m "feat: add session lifecycle, --with-label profile loading, and env var resolution to botl run"
```

---

## Chunk 7: Final Verification

### Task 8: Full test suite, lint, and build verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /home/kamil-rybacki/Code/botl && go test ./... -v -count=1`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd /home/kamil-rybacki/Code/botl && golangci-lint run ./...`
Expected: PASS

- [ ] **Step 3: Verify build**

Run: `cd /home/kamil-rybacki/Code/botl && go build ./...`
Expected: PASS

- [ ] **Step 4: Verify `botl config list` works**

Run: `cd /home/kamil-rybacki/Code/botl && go run . config list`
Expected: Prints config key/value pairs

- [ ] **Step 5: Verify `botl profiles list` works**

Run: `cd /home/kamil-rybacki/Code/botl && go run . profiles list`
Expected: Prints "no profiles found" message

- [ ] **Step 6: Final commit if any adjustments were needed**
