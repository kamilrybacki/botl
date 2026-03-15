//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Shared image tag built once by TestMain.
const testImageTag = "botl-test:e2e"

// TestMain builds the botl binary and Docker image once for all E2E tests.
func TestMain(m *testing.M) {
	if err := requireDocker(); err != nil {
		fmt.Fprintf(os.Stderr, "skipping E2E tests: %v\n", err)
		os.Exit(0)
	}

	root := mustFindRepoRoot()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Build botl binary into a temp location
	tmpDir, err := os.MkdirTemp("", "botl-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create temp dir: %v\n", err)
		os.Exit(1)
	}
	botlBin := tmpDir + "/botl"

	build := exec.CommandContext(ctx, "go", "build", "-o", botlBin, ".")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		cancel()
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "cannot build botl binary: %v\n", err)
		os.Exit(1)
	}

	// Build the Docker image once
	img := exec.CommandContext(ctx, botlBin, "build", "--image", testImageTag)
	img.Dir = root
	img.Stdout = os.Stdout
	img.Stderr = os.Stderr
	if err := img.Run(); err != nil {
		cancel()
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "cannot build docker image: %v\n", err)
		os.Exit(1)
	}
	cancel()

	// Run tests
	code := m.Run()

	// Cleanup
	os.RemoveAll(tmpDir)
	exec.Command("docker", "rmi", testImageTag).Run()

	os.Exit(code)
}

func TestDockerBuild_ImageExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", testImageTag).CombinedOutput()
	if err != nil {
		t.Fatalf("image %s not found after build: %v\n%s", testImageTag, err, out)
	}
}

func TestDockerBuild_ImageHasEntrypoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{json .Config.Entrypoint}}", testImageTag).CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect failed: %v\n%s", err, out)
	}
	result := strings.TrimSpace(string(out))
	if !strings.Contains(result, "entrypoint.sh") {
		t.Errorf("expected entrypoint.sh in image entrypoint, got: %s", result)
	}
}

func TestDockerBuild_ImageHasPostrun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify botl-postrun binary exists inside the image
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--entrypoint", "ls",
		testImageTag, "/usr/local/bin/botl-postrun").CombinedOutput()
	if err != nil {
		t.Fatalf("botl-postrun not found in image: %v\n%s", err, out)
	}
}

func TestDockerBuild_ImageHasClaude(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify claude is installed
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--entrypoint", "which",
		testImageTag, "claude").CombinedOutput()
	if err != nil {
		t.Fatalf("claude not found in image: %v\n%s", err, out)
	}
}

func TestEntrypoint_RequiresRepoURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run without BOTL_REPO_URL — should fail with a clear message
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm", testImageTag).CombinedOutput()
	if err == nil {
		t.Fatal("container should fail without BOTL_REPO_URL")
	}
	if !strings.Contains(string(out), "BOTL_REPO_URL") {
		t.Errorf("expected error about BOTL_REPO_URL, got: %s", out)
	}
}

func TestEntrypoint_ClonePublicRepo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Clone a small public repo and verify the workspace exists.
	// Use --entrypoint to override and just test the clone + verify step.
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-e", "BOTL_REPO_URL=https://github.com/octocat/Hello-World",
		"-e", "BOTL_DEPTH=1",
		"--entrypoint", "sh",
		testImageTag,
		"-c", `
			set -e
			REPO_URL="$BOTL_REPO_URL"
			DEPTH="${BOTL_DEPTH:-1}"
			git clone --depth "$DEPTH" "$REPO_URL" /workspace/repo
			cd /workspace/repo
			test -d .git
			echo "CLONE_OK"
		`,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("clone test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "CLONE_OK") {
		t.Errorf("expected CLONE_OK in output, got: %s", out)
	}
}

func TestEntrypoint_BranchClone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-e", "BOTL_REPO_URL=https://github.com/octocat/Hello-World",
		"-e", "BOTL_BRANCH=master",
		"-e", "BOTL_DEPTH=1",
		"--entrypoint", "sh",
		testImageTag,
		"-c", `
			set -e
			REPO_URL="$BOTL_REPO_URL"
			BRANCH="$BOTL_BRANCH"
			DEPTH="${BOTL_DEPTH:-1}"
			git clone --depth "$DEPTH" --branch "$BRANCH" "$REPO_URL" /workspace/repo
			cd /workspace/repo
			CURRENT=$(git rev-parse --abbrev-ref HEAD)
			if [ "$CURRENT" = "$BRANCH" ]; then
				echo "BRANCH_OK"
			else
				echo "BRANCH_MISMATCH: $CURRENT"
			fi
		`,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("branch clone test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "BRANCH_OK") {
		t.Errorf("expected BRANCH_OK, got: %s", out)
	}
}

func TestEntrypoint_GitUserConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-e", "BOTL_REPO_URL=https://github.com/octocat/Hello-World",
		"-e", "BOTL_DEPTH=1",
		"--entrypoint", "sh",
		testImageTag,
		"-c", `
			set -e
			git clone --depth 1 "$BOTL_REPO_URL" /workspace/repo
			cd /workspace/repo
			git config user.email "botl@container"
			git config user.name "botl"
			EMAIL=$(git config user.email)
			NAME=$(git config user.name)
			echo "EMAIL=$EMAIL"
			echo "NAME=$NAME"
		`,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("git config test failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "EMAIL=botl@container") {
		t.Errorf("expected EMAIL=botl@container, got: %s", output)
	}
	if !strings.Contains(output, "NAME=botl") {
		t.Errorf("expected NAME=botl, got: %s", output)
	}
}

func TestContainer_OutputDirWritable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmpOut := t.TempDir()

	out, err := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", tmpOut+":/output:rw",
		"--entrypoint", "sh",
		testImageTag,
		"-c", `echo "test-content" > /output/test.txt && echo "WRITE_OK"`,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("output dir write test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "WRITE_OK") {
		t.Errorf("expected WRITE_OK, got: %s", out)
	}

	// Verify the file exists on the host
	content, err := os.ReadFile(tmpOut + "/test.txt")
	if err != nil {
		t.Fatalf("cannot read output file on host: %v", err)
	}
	if strings.TrimSpace(string(content)) != "test-content" {
		t.Errorf("output file content = %q, want %q", string(content), "test-content")
	}
}

// --- helpers ---

func requireDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		return fmt.Errorf("docker daemon not running")
	}
	return nil
}

func mustFindRepoRoot() string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find go.mod: %v\n", err)
		os.Exit(1)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		fmt.Fprintln(os.Stderr, "not in a Go module")
		os.Exit(1)
	}
	for i := len(gomod) - 1; i >= 0; i-- {
		if gomod[i] == '/' {
			return gomod[:i]
		}
	}
	fmt.Fprintln(os.Stderr, "cannot determine repo root")
	os.Exit(1)
	return ""
}
