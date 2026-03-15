package container

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

//go:embed all:dockerctx
var dockerCtx embed.FS

// BuildImage cross-compiles the postrun TUI binary, then builds the Docker image.
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

	// Cross-compile the postrun TUI binary for linux/amd64
	fmt.Println("botl: cross-compiling postrun binary...")
	postrunBin := tmpDir + "/botl-postrun"
	if err := crossCompilePostrun(ctx, postrunBin); err != nil {
		return fmt.Errorf("failed to compile postrun binary: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// crossCompilePostrun builds the postrun binary targeting linux/amd64.
func crossCompilePostrun(ctx context.Context, outputPath string) error {
	// Find the postrun source relative to the module root.
	// We need to find go.mod to determine the module root.
	modRoot, err := findModuleRoot()
	if err != nil {
		return fmt.Errorf("cannot find module root: %w", err)
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", outputPath, "./cmd/botl-postrun")
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	// Keep the current toolchain
	cmd.Env = append(cmd.Env, "GOTOOLCHAIN=local")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// findModuleRoot walks up from the executable's directory (or cwd) to find go.mod.
func findModuleRoot() (string, error) {
	// Try using go env GOMOD
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err == nil {
		gomod := strings.TrimSpace(string(out))
		if gomod != "" && gomod != "/dev/null" && gomod != os.DevNull {
			// Return directory containing go.mod
			for i := len(gomod) - 1; i >= 0; i-- {
				if gomod[i] == '/' {
					return gomod[:i], nil
				}
			}
		}
	}

	// Fallback: cwd
	return os.Getwd()
}
