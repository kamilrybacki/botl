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
	if xdg != "" && !filepath.IsAbs(xdg) {
		xdg = "" // fall back to default
	}
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
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

	return os.WriteFile(path, data, 0600)
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
