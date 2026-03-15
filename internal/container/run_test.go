package container

import (
	"strings"
	"testing"
	"time"
)

func TestBuildDockerArgs_Interactive(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}

	args := buildDockerArgs(opts)

	assertContains(t, args, "run")
	assertContains(t, args, "--rm")
	assertContains(t, args, "-it")
	assertContains(t, args, "botl:latest")
	assertEnvVar(t, args, "BOTL_REPO_URL=https://github.com/user/repo")
	assertEnvVar(t, args, "BOTL_DEPTH=1")

	// Should NOT have BOTL_BRANCH or BOTL_PROMPT
	for _, arg := range args {
		if strings.HasPrefix(arg, "BOTL_BRANCH=") {
			t.Error("interactive mode should not set BOTL_BRANCH")
		}
		if strings.HasPrefix(arg, "BOTL_PROMPT=") {
			t.Error("interactive mode should not set BOTL_PROMPT")
		}
	}
}

func TestBuildDockerArgs_Headless(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
		Prompt:  "fix all lint errors",
	}

	args := buildDockerArgs(opts)

	// Headless mode should NOT have -it
	for _, arg := range args {
		if arg == "-it" {
			t.Error("headless mode should not include -it")
		}
	}

	assertEnvVar(t, args, "BOTL_PROMPT=fix all lint errors")
}

func TestBuildDockerArgs_Branch(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   3,
		Branch:  "feat/new",
	}

	args := buildDockerArgs(opts)

	assertEnvVar(t, args, "BOTL_BRANCH=feat/new")
	assertEnvVar(t, args, "BOTL_DEPTH=3")
}

func TestBuildDockerArgs_Mounts(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
		Mounts: []Mount{
			{Source: "/usr/lib/node_modules", Target: "/usr/lib/node_modules"},
			{Source: "/usr/lib/python3", Target: "/usr/lib/python3"},
		},
		OutputDir:       "/home/user/output",
		ClaudeConfigDir: "/home/user/.claude",
	}

	args := buildDockerArgs(opts)

	assertContains(t, args, "/usr/lib/node_modules:/usr/lib/node_modules:ro")
	assertContains(t, args, "/usr/lib/python3:/usr/lib/python3:ro")
	assertContains(t, args, "/home/user/output:/output:rw")
	assertContains(t, args, "/home/user/.claude:/home/botl/.claude:ro")
}

func TestBuildDockerArgs_EnvVars(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
		EnvVars: []string{"MY_VAR=hello", "OTHER=world"},
	}

	args := buildDockerArgs(opts)

	assertEnvVar(t, args, "MY_VAR=hello")
	assertEnvVar(t, args, "OTHER=world")
}

func TestBuildDockerArgs_NoOptionalFields(t *testing.T) {
	opts := RunOpts{
		Image:   "myimage:v1",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}

	args := buildDockerArgs(opts)

	// No volume mounts for output or claude config
	for _, arg := range args {
		if strings.Contains(arg, "/output:rw") {
			t.Error("should not have output mount when OutputDir is empty")
		}
		if strings.Contains(arg, "/home/botl/.claude:ro") {
			t.Error("should not have claude config mount when ClaudeConfigDir is empty")
		}
	}
}

func TestBuildDockerArgs_StopTimeout(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}

	args := buildDockerArgs(opts)

	found := false
	for i, arg := range args {
		if arg == "--stop-timeout" && i+1 < len(args) && args[i+1] == "10" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --stop-timeout 10 in args")
	}
}

func TestBuildDockerArgs_SecurityFlags(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}

	args := buildDockerArgs(opts)

	assertContains(t, args, "--cap-drop")
	assertContains(t, args, "ALL")
	assertContains(t, args, "--cap-add")
	assertContains(t, args, "SETUID")
	assertContains(t, args, "SETGID")
	assertContains(t, args, "--init")
}

func TestBuildDockerArgs_ImageIsLast(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:custom",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
		Timeout: 5 * time.Minute,
	}

	args := buildDockerArgs(opts)

	last := args[len(args)-1]
	if last != "botl:custom" {
		t.Errorf("last arg = %q, want %q", last, "botl:custom")
	}
}

func TestBuildDockerArgs_SanitizeGit(t *testing.T) {
	opts := RunOpts{
		Image:       "botl:latest",
		RepoURL:     "https://github.com/user/repo",
		Depth:       1,
		SanitizeGit: true,
	}
	args := buildDockerArgs(opts)
	assertEnvVar(t, args, "BOTL_SANITIZE_GIT=true")
}

func TestBuildDockerArgs_NoSanitizeGit(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}
	args := buildDockerArgs(opts)
	for _, arg := range args {
		if strings.HasPrefix(arg, "BOTL_SANITIZE_GIT=") {
			t.Error("should not set BOTL_SANITIZE_GIT when false")
		}
	}
}

func TestBuildDockerArgs_BlockedPorts(t *testing.T) {
	opts := RunOpts{
		Image:        "botl:latest",
		RepoURL:      "https://github.com/user/repo",
		Depth:        1,
		BlockedPorts: []int{8080, 3000},
	}
	args := buildDockerArgs(opts)
	assertEnvVar(t, args, "BOTL_BLOCKED_PORTS=8080,3000")
	assertContains(t, args, "--cap-add")
	assertContains(t, args, "NET_ADMIN")
}

func TestBuildDockerArgs_NoBlockedPorts(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   1,
	}
	args := buildDockerArgs(opts)
	for _, arg := range args {
		if arg == "NET_ADMIN" {
			t.Error("should not add NET_ADMIN when no blocked ports")
		}
		if strings.HasPrefix(arg, "BOTL_BLOCKED_PORTS=") {
			t.Error("should not set BOTL_BLOCKED_PORTS when empty")
		}
	}
	// SETUID and SETGID should still be present (needed by gosu)
	assertContains(t, args, "SETUID")
	assertContains(t, args, "SETGID")
}

func TestBuildDockerArgs_DeepCloneDepthZero(t *testing.T) {
	opts := RunOpts{
		Image:   "botl:latest",
		RepoURL: "https://github.com/user/repo",
		Depth:   0,
	}
	args := buildDockerArgs(opts)
	assertEnvVar(t, args, "BOTL_DEPTH=0")
}

// --- helpers ---

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, want)
}

func assertEnvVar(t *testing.T, args []string, envVar string) {
	t.Helper()
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) && args[i+1] == envVar {
			return
		}
	}
	t.Errorf("args does not contain -e %q", envVar)
}
