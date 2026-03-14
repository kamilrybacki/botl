package container

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
)

//go:embed all:dockerctx
var dockerCtx embed.FS

// BuildImage builds the botl Docker image from the embedded Dockerfile.
func BuildImage(ctx context.Context, tag string) error {
	if err := checkDocker(); err != nil {
		return err
	}

	// Write embedded docker context to a temp dir
	tmpDir, err := os.MkdirTemp("", "botl-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract embedded files to temp dir
	entries, err := dockerCtx.ReadDir("dockerctx")
	if err != nil {
		return fmt.Errorf("failed to read embedded docker context: %w", err)
	}
	for _, entry := range entries {
		data, err := dockerCtx.ReadFile("dockerctx/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", entry.Name(), err)
		}
		perm := os.FileMode(0644)
		if entry.Name() == "entrypoint.sh" {
			perm = 0755
		}
		if err := os.WriteFile(tmpDir+"/"+entry.Name(), data, perm); err != nil {
			return fmt.Errorf("failed to write %s: %w", entry.Name(), err)
		}
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
