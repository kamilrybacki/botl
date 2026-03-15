package container

import (
	"fmt"
	"strings"
	"time"
)

// Mount represents a read-only bind mount.
type Mount struct {
	Source string
	Target string
}

// ParseMount parses a "host:container" mount string.
func ParseMount(s string) (Mount, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Mount{}, fmt.Errorf("expected format host_path:container_path")
	}
	return Mount{Source: parts[0], Target: parts[1]}, nil
}

// RunOpts holds all options for running a container.
type RunOpts struct {
	Image     string
	RepoURL   string
	Branch    string
	Depth     int
	Prompt    string
	Mounts    []Mount
	EnvVars   []string
	Timeout   time.Duration
	OutputDir string // Host path mounted rw at /output inside container
}
