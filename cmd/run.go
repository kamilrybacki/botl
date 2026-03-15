package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kamilrybacki/botl/internal/container"
	"github.com/kamilrybacki/botl/internal/detect"
	"github.com/spf13/cobra"
)

var runOpts struct {
	branch        string
	depth         int
	prompt        string
	mountPackages bool
	mounts        []string
	timeout       time.Duration
	image         string
	envVars       []string
	outputDir     string
}

var runCmd = &cobra.Command{
	Use:   "run <repo-url>",
	Short: "Clone a repo and launch Claude Code in a container",
	Args:  cobra.ExactArgs(1),
	RunE:  runRun,
}

func init() {
	runCmd.Flags().StringVarP(&runOpts.branch, "branch", "b", "", "Branch to clone (default: repo default)")
	runCmd.Flags().IntVar(&runOpts.depth, "depth", 1, "Git clone depth")
	runCmd.Flags().StringVarP(&runOpts.prompt, "prompt", "p", "", "Prompt for headless mode (omit for interactive)")
	runCmd.Flags().BoolVar(&runOpts.mountPackages, "mount-packages", true, "Auto-detect and mount host packages read-only")
	runCmd.Flags().StringSliceVarP(&runOpts.mounts, "mount", "m", nil, "Extra read-only mount host:container (repeatable)")
	runCmd.Flags().DurationVar(&runOpts.timeout, "timeout", 30*time.Minute, "Max session duration")
	runCmd.Flags().StringVar(&runOpts.image, "image", "botl:latest", "Docker image to use")
	runCmd.Flags().StringSliceVarP(&runOpts.envVars, "env", "e", nil, "Extra env vars KEY=VALUE (repeatable)")
	runCmd.Flags().StringVarP(&runOpts.outputDir, "output-dir", "o", "./botl-output", "Host directory for patches and saved workspaces")

	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Resolve ~/.claude for OAuth credentials
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	claudeConfigDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeConfigDir); os.IsNotExist(err) {
		return fmt.Errorf("~/.claude not found — run 'claude' once on your host to authenticate first")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nbotl: received interrupt, stopping container...")
		cancel()
	}()

	// Auto-detect host packages
	var mounts []container.Mount
	if runOpts.mountPackages {
		detected := detect.HostPackages()
		for _, d := range detected {
			fmt.Fprintf(os.Stderr, "botl: auto-mounting %s → %s (ro)\n", d.Source, d.Target)
			mounts = append(mounts, d)
		}
	}

	// Add explicit mounts
	for _, m := range runOpts.mounts {
		parsed, err := container.ParseMount(m)
		if err != nil {
			return fmt.Errorf("invalid mount %q: %w", m, err)
		}
		mounts = append(mounts, parsed)
	}

	// Resolve and create output directory
	outputDir, err := filepath.Abs(runOpts.outputDir)
	if err != nil {
		return fmt.Errorf("invalid output dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output dir %s: %w", outputDir, err)
	}

	opts := container.RunOpts{
		Image:          runOpts.image,
		RepoURL:        repoURL,
		Branch:         runOpts.branch,
		Depth:          runOpts.depth,
		Prompt:         runOpts.prompt,
		Mounts:         mounts,
		EnvVars:        runOpts.envVars,
		Timeout:        runOpts.timeout,
		OutputDir:      outputDir,
		ClaudeConfigDir: claudeConfigDir,
	}

	return container.Run(ctx, opts)
}
