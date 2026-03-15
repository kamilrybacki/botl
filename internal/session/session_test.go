package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamilrybacki/botl/internal/runconfig"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"abcd1234", false},
		{"00ff00ff", false},
		{"ABCD1234", true},  // uppercase not allowed
		{"abcd123", true},   // too short
		{"abcd12345", true}, // too long
		{"../../etc", true}, // path traversal
		{"abcdXYZW", true},  // non-hex
	}
	for _, tt := range tests {
		err := ValidateID(tt.id)
		if tt.wantErr && err == nil {
			t.Errorf("ValidateID(%q) should error", tt.id)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("ValidateID(%q) unexpected error: %v", tt.id, err)
		}
	}
}

func TestGenerateID(t *testing.T) {
	id, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID: %v", err)
	}
	if len(id) != 8 {
		t.Errorf("ID length = %d, want 8", len(id))
	}
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
