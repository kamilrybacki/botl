package profile

import (
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
