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
