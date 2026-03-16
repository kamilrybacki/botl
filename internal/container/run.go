package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// buildDockerArgs constructs the docker run argument list from RunOpts.
// Extracted for testability.
func buildDockerArgs(opts RunOpts) []string {
	args := []string{"run", "--rm"}

	isInteractive := opts.Prompt == ""
	if isInteractive {
		args = append(args, "-it")
	}

	// Environment variables
	for _, env := range opts.EnvVars {
		args = append(args, "-e", env)
	}
	args = append(args, "-e", "BOTL_REPO_URL="+opts.RepoURL)
	args = append(args, "-e", "BOTL_DEPTH="+strconv.Itoa(opts.Depth))
	if opts.Branch != "" {
		args = append(args, "-e", "BOTL_BRANCH="+opts.Branch)
	}
	if opts.Prompt != "" {
		args = append(args, "-e", "BOTL_PROMPT="+opts.Prompt)
	}
	if opts.SanitizeGit {
		args = append(args, "-e", "BOTL_SANITIZE_GIT=true")
	}
	if len(opts.BlockedPorts) > 0 {
		portStrs := make([]string, len(opts.BlockedPorts))
		for i, p := range opts.BlockedPorts {
			portStrs[i] = strconv.Itoa(p)
		}
		args = append(args, "-e", "BOTL_BLOCKED_PORTS="+strings.Join(portStrs, ","))
	}

	// Read-only bind mounts
	for _, m := range opts.Mounts {
		args = append(args, "-v", m.Source+":"+m.Target+":ro")
	}

	// Output directory mount (read-write) for patches and workspace exports
	if opts.OutputDir != "" {
		args = append(args, "-v", opts.OutputDir+":/output:rw")
	}

	// Mount ~/.claude dir and ~/.claude.json for OAuth session credentials (read-only)
	if opts.ClaudeConfigDir != "" {
		args = append(args, "-v", opts.ClaudeConfigDir+":/home/botl/.claude:ro")
		// Claude Code also needs ~/.claude.json (auth tokens live here, separate from the dir)
		claudeJSON := filepath.Dir(opts.ClaudeConfigDir) + "/.claude.json"
		if _, err := os.Stat(claudeJSON); err == nil {
			args = append(args, "-v", claudeJSON+":/home/botl/.claude.json:ro")
		}
	}

	// Container security hardening: drop all capabilities, then add back
	// only what's needed:
	// - SETUID/SETGID: gosu switches from root to botl user
	// - DAC_READ_SEARCH: entrypoint copies host-mounted 0600 credential files
	//   (root can't read files owned by other UIDs without this)
	// - CHOWN/FOWNER: entrypoint needs to chown copied credentials to botl
	args = append(args, "--cap-drop", "ALL")
	args = append(args, "--cap-add", "SETUID")
	args = append(args, "--cap-add", "SETGID")
	args = append(args, "--cap-add", "DAC_READ_SEARCH")
	args = append(args, "--cap-add", "CHOWN")
	args = append(args, "--cap-add", "FOWNER")
	if len(opts.BlockedPorts) > 0 {
		args = append(args, "--cap-add", "NET_ADMIN")
	}
	args = append(args, "--init")

	// Stop timeout label (used by our signal handler)
	args = append(args, "--stop-timeout", "10")

	args = append(args, opts.Image)

	return args
}

// Run creates and starts an ephemeral container, attaches to it, and cleans up.
func Run(ctx context.Context, opts RunOpts) error {
	if err := checkDocker(); err != nil {
		return err
	}

	// Check if image exists
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", opts.Image).Run(); err != nil {
		return fmt.Errorf("image %q not found — run 'botl build' first", opts.Image)
	}

	args := buildDockerArgs(opts)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Handle timeout
	if opts.Timeout > 0 {
		timer := time.AfterFunc(opts.Timeout, func() {
			fmt.Fprintf(os.Stderr, "\nbotl: timeout (%s) reached, stopping...\n", opts.Timeout)
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt) //nolint:errcheck
			}
		})
		defer timer.Stop()
	}

	fmt.Fprintf(os.Stderr, "botl: launching container (%s)...\n", opts.Image)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("interrupted")
		}
		// Check if it's just a non-zero exit
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("container exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to run container: %w", err)
	}

	return nil
}

func checkDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH — please install Docker")
	}
	// Quick check that docker daemon is running
	out, err := exec.Command("docker", "info").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if idx := strings.Index(msg, "\n"); idx >= 0 {
			msg = msg[:idx]
		}
		return fmt.Errorf("docker daemon not available: %s", msg)
	}
	return nil
}
