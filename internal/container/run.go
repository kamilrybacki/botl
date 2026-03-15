package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Run creates and starts an ephemeral container, attaches to it, and cleans up.
func Run(ctx context.Context, opts RunOpts) error {
	if err := checkDocker(); err != nil {
		return err
	}

	// Check if image exists
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", opts.Image).Run(); err != nil {
		return fmt.Errorf("image %q not found — run 'botl build' first", opts.Image)
	}

	isInteractive := opts.Prompt == ""

	// Build docker run arguments
	args := []string{"run", "--rm"}

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

	// Read-only bind mounts
	for _, m := range opts.Mounts {
		args = append(args, "-v", m.Source+":"+m.Target+":ro")
	}

	// Output directory mount (read-write) for patches and workspace exports
	if opts.OutputDir != "" {
		args = append(args, "-v", opts.OutputDir+":/output:rw")
	}

	// Stop timeout label (used by our signal handler)
	args = append(args, "--stop-timeout", "10")

	args = append(args, opts.Image)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Handle timeout
	if opts.Timeout > 0 {
		timer := time.AfterFunc(opts.Timeout, func() {
			fmt.Fprintf(os.Stderr, "\nbotl: timeout (%s) reached, stopping...\n", opts.Timeout)
			if cmd.Process != nil {
				_ = cmd.Process.Signal(os.Interrupt)
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
		return fmt.Errorf("docker daemon not available: %s", msg)
	}
	return nil
}
