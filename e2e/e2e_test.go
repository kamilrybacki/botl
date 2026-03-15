//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These tests require Docker and are gated behind the "e2e" build tag.
// Run with: go test -tags e2e ./e2e/

func checkDockerAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH, skipping E2E tests")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not running, skipping E2E tests")
	}
}

func TestDockerBuild(t *testing.T) {
	checkDockerAvailable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build the botl binary first
	buildBotl := exec.CommandContext(ctx, "go", "build", "-o", t.TempDir()+"/botl", ".")
	buildBotl.Dir = findRepoRoot(t)
	if out, err := buildBotl.CombinedOutput(); err != nil {
		t.Fatalf("failed to build botl: %v\n%s", err, out)
	}

	// Build the Docker image with a test tag
	tag := "botl-test:e2e"
	cmd := exec.CommandContext(ctx, t.TempDir()+"/botl", "build", "--image", tag)
	cmd.Dir = findRepoRoot(t)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// botl build may fail if we can't compile postrun or docker build fails
		// that's still a valid test result
		t.Fatalf("botl build failed: %v", err)
	}

	// Verify image exists
	inspect := exec.CommandContext(ctx, "docker", "image", "inspect", tag)
	if err := inspect.Run(); err != nil {
		t.Fatalf("image %s not found after build", tag)
	}

	// Clean up test image
	t.Cleanup(func() {
		exec.Command("docker", "rmi", tag).Run()
	})
}

func TestEntrypointRequiresRepoURL(t *testing.T) {
	checkDockerAvailable(t)

	// Use a minimal image to test entrypoint behavior.
	// We need the botl image to exist for this test.
	tag := "botl-test:entrypoint"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build image
	buildBotl := exec.CommandContext(ctx, "go", "build", "-o", t.TempDir()+"/botl", ".")
	buildBotl.Dir = findRepoRoot(t)
	if out, err := buildBotl.CombinedOutput(); err != nil {
		t.Skipf("cannot build botl binary: %v\n%s", err, out)
	}

	buildImg := exec.CommandContext(ctx, t.TempDir()+"/botl", "build", "--image", tag)
	buildImg.Dir = findRepoRoot(t)
	if out, err := buildImg.CombinedOutput(); err != nil {
		t.Skipf("cannot build docker image: %v\n%s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rmi", tag).Run() })

	// Run without BOTL_REPO_URL — should fail
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", tag)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("container should fail without BOTL_REPO_URL")
	}
	if !strings.Contains(string(out), "BOTL_REPO_URL") {
		t.Errorf("expected error about BOTL_REPO_URL, got: %s", out)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("cannot find go.mod: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Fatal("not in a Go module")
	}
	// Return directory containing go.mod
	for i := len(gomod) - 1; i >= 0; i-- {
		if gomod[i] == '/' {
			return gomod[:i]
		}
	}
	t.Fatal("cannot determine repo root from go.mod path")
	return ""
}
