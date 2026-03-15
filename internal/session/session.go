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

type Record struct {
	ID        string              `yaml:"id"`
	CreatedAt time.Time           `yaml:"created_at"`
	RepoURL   string              `yaml:"repo_url"`
	Branch    string              `yaml:"branch,omitempty"`
	Status    string              `yaml:"status"`
	Run       runconfig.RunConfig `yaml:"run"`
}

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

func GenerateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

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

func UpdateStatus(id string, status string) error {
	rec, err := Read(id)
	if err != nil {
		return err
	}
	rec.Status = status
	return Write(rec)
}
