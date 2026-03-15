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

type Profile struct {
	Name          string              `yaml:"name"`
	CreatedAt     time.Time           `yaml:"created_at"`
	SourceSession string              `yaml:"source_session"`
	Run           runconfig.RunConfig `yaml:"run"`
}

func Dir() string {
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
	return filepath.Join(xdg, "botl", "profiles")
}

func ValidateName(name string) error {
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]{0,62}", name)
	}
	return nil
}

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

func Exists(name string) bool {
	path := filepath.Join(Dir(), name+".yaml")
	_, err := os.Stat(path)
	return err == nil
}

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
			fmt.Fprintf(os.Stderr, "botl: warning: skipping profile %q: %v\n", name, err)
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func Delete(name string) error {
	path := filepath.Join(Dir(), name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}
	return os.Remove(path)
}
