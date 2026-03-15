package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/kamilrybacki/botl/internal/config"
	"github.com/kamilrybacki/botl/internal/container"
	"github.com/kamilrybacki/botl/internal/detect"
	"github.com/spf13/cobra"
)

// validBranchRe matches safe git ref names.
var validBranchRe = regexp.MustCompile(`^[a-zA-Z0-9._/~^:@{}\[\]-]{1,255}$`)

// validRepoURLRe matches allowed repo URL schemes (https, git@, ssh://).
var validRepoURLRe = regexp.MustCompile(`^(https://[^\s]+|git@[^\s]+|ssh://[^\s]+)$`)

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
	cloneMode     string
	blockedPorts  []int
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
	runCmd.Flags().StringVar(&runOpts.cloneMode, "clone-mode", "", "Clone mode: shallow or deep (default: from config)")
	runCmd.Flags().IntSliceVar(&runOpts.blockedPorts, "blocked-ports", nil, "TCP ports to block inbound (default: from config)")

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

	// Load persistent config
	cfg, loadErr := config.Load(config.Path())
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not load config: %v\n", loadErr)
	}

	// Determine clone mode (CLI flag > config > default)
	cloneMode := cfg.Clone.Mode
	if runOpts.cloneMode != "" {
		cloneMode = runOpts.cloneMode
	}
	if err := config.ValidateCloneMode(cloneMode); err != nil {
		return err
	}

	// Determine depth and sanitization from clone mode
	depth := runOpts.depth
	sanitizeGit := false
	if !cmd.Flags().Changed("depth") {
		if cloneMode == "shallow" {
			depth = 1
			sanitizeGit = true
		} else {
			depth = 0
		}
	} else if cloneMode == "shallow" {
		sanitizeGit = true
	}

	// Determine blocked ports (CLI flag > config > default)
	blockedPorts := cfg.Network.BlockedPorts
	if cmd.Flags().Changed("blocked-ports") {
		blockedPorts = runOpts.blockedPorts
	}
	if err := config.ValidatePorts(blockedPorts); err != nil {
		return err
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

	// Validate user-supplied env vars don't override internal namespace
	for _, env := range runOpts.envVars {
		key := env
		if idx := strings.Index(env, "="); idx >= 0 {
			key = env[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(key), "BOTL_") {
			return fmt.Errorf("environment variable %q uses reserved BOTL_ namespace", key)
		}
	}

	// Validate branch name if provided
	if runOpts.branch != "" && !validBranchRe.MatchString(runOpts.branch) {
		return fmt.Errorf("invalid branch name %q: must match [a-zA-Z0-9._/~^:@{}[]-]", runOpts.branch)
	}

	// Validate repo URL scheme
	if !validRepoURLRe.MatchString(repoURL) {
		return fmt.Errorf("invalid repo URL %q: must be https://, git@, or ssh:// URL", repoURL)
	}

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
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return fmt.Errorf("cannot create output dir %s: %w", outputDir, err)
	}

	opts := container.RunOpts{
		Image:           runOpts.image,
		RepoURL:         repoURL,
		Branch:          runOpts.branch,
		Depth:           depth,
		Prompt:          runOpts.prompt,
		Mounts:          mounts,
		EnvVars:         runOpts.envVars,
		Timeout:         runOpts.timeout,
		OutputDir:       outputDir,
		ClaudeConfigDir: claudeConfigDir,
		SanitizeGit:     sanitizeGit,
		BlockedPorts:    blockedPorts,
	}

	return container.Run(ctx, opts)
}
