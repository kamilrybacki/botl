package cmd

import (
	"bufio"
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
	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/kamilrybacki/botl/internal/runconfig"
	"github.com/kamilrybacki/botl/internal/session"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	withLabel     string
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
	runCmd.Flags().StringVar(&runOpts.withLabel, "with-label", "", "Load named profile as defaults")

	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Step 1: Generate session ID
	sessionID, err := session.GenerateUniqueID()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	// Resolve ~/.claude for OAuth credentials
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	claudeConfigDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeConfigDir); os.IsNotExist(err) {
		return fmt.Errorf("~/.claude not found — run 'claude' once on your host to authenticate first")
	}

	// Step 2: Load persistent config → base defaults
	cfg, loadErr := config.Load(config.Path())
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not load config: %v\n", loadErr)
	}

	// Start with config-level defaults
	cloneMode := cfg.Clone.Mode
	blockedPorts := cfg.Network.BlockedPorts
	depth := runOpts.depth
	timeout := runOpts.timeout
	image := runOpts.image
	outputDir := runOpts.outputDir
	envVars := runOpts.envVars
	var profileEnvKeys []string

	// Step 3: If --with-label, load profile and override config defaults
	if runOpts.withLabel != "" {
		if err := profile.ValidateName(runOpts.withLabel); err != nil {
			return err
		}
		p, err := profile.Load(runOpts.withLabel)
		if err != nil {
			return fmt.Errorf("profile %q not found (%s/%s.yaml)", runOpts.withLabel, profile.Dir(), runOpts.withLabel)
		}
		fmt.Fprintf(os.Stderr, "botl: loading profile %q\n", runOpts.withLabel)

		// Profile overrides config defaults (only when CLI flag not explicitly set)
		if !cmd.Flags().Changed("clone-mode") {
			cloneMode = p.Run.CloneMode
		}
		if !cmd.Flags().Changed("blocked-ports") {
			blockedPorts = p.Run.BlockedPorts
		}
		if !cmd.Flags().Changed("depth") {
			depth = p.Run.Depth
		}
		if !cmd.Flags().Changed("timeout") {
			timeout = p.Run.Timeout
		}
		if !cmd.Flags().Changed("image") {
			image = p.Run.Image
		}
		if !cmd.Flags().Changed("output-dir") {
			outputDir = p.Run.OutputDir
		}
		profileEnvKeys = p.Run.EnvVarKeys
	}

	// Step 4: Apply explicit CLI flags
	if cmd.Flags().Changed("clone-mode") {
		cloneMode = runOpts.cloneMode
	}
	if cmd.Flags().Changed("blocked-ports") {
		blockedPorts = runOpts.blockedPorts
	}

	// Step 5: Validate all opts
	if err := config.ValidateCloneMode(cloneMode); err != nil {
		return err
	}
	if err := config.ValidatePorts(blockedPorts); err != nil {
		return err
	}

	// Determine depth and sanitization from clone mode
	sanitizeGit := false
	if !cmd.Flags().Changed("depth") && runOpts.withLabel == "" {
		if cloneMode == "shallow" {
			depth = 1
			sanitizeGit = true
		} else {
			depth = 0
		}
	} else if cloneMode == "shallow" {
		sanitizeGit = true
	}

	if runOpts.branch != "" && !validBranchRe.MatchString(runOpts.branch) {
		return fmt.Errorf("invalid branch name %q: must match [a-zA-Z0-9._/~^:@{}[]-]", runOpts.branch)
	}
	if !validRepoURLRe.MatchString(repoURL) {
		return fmt.Errorf("invalid repo URL %q: must be https://, git@, or ssh:// URL", repoURL)
	}

	// Validate user-supplied env vars don't override internal namespace
	for _, env := range envVars {
		key := env
		if idx := strings.Index(env, "="); idx >= 0 {
			key = env[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(key), "BOTL_") {
			return fmt.Errorf("environment variable %q uses reserved BOTL_ namespace", key)
		}
	}

	// Step 6: Resolve env var keys from profile
	if len(profileEnvKeys) > 0 {
		resolvedEnvVars, resolveErr := resolveEnvVarKeys(profileEnvKeys, envVars)
		if resolveErr != nil {
			return resolveErr
		}
		envVars = append(envVars, resolvedEnvVars...)
	}

	// Step 7: Resolve and create output directory
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output dir: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0700); err != nil {
		return fmt.Errorf("cannot create output dir %s: %w", absOutputDir, err)
	}

	// Extract env var keys for session record (strip values)
	allEnvKeys := extractEnvKeys(envVars)

	// Step 8: Write session record as pending
	rec := session.Record{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		RepoURL:   repoURL,
		Branch:    runOpts.branch,
		Status:    session.StatusPending,
		Run: runconfig.RunConfig{
			CloneMode:    cloneMode,
			Depth:        depth,
			BlockedPorts: blockedPorts,
			Timeout:      timeout,
			Image:        image,
			OutputDir:    absOutputDir,
			EnvVarKeys:   allEnvKeys,
		},
	}
	if writeErr := session.Write(rec); writeErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not write session record: %v\n", writeErr)
	}

	// Step 9: Print session ID
	fmt.Fprintf(os.Stderr, "botl: session id: %s\n", sessionID)

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
		parsed, parseErr := container.ParseMount(m)
		if parseErr != nil {
			return fmt.Errorf("invalid mount %q: %w", m, parseErr)
		}
		mounts = append(mounts, parsed)
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

	opts := container.RunOpts{
		Image:           image,
		RepoURL:         repoURL,
		Branch:          runOpts.branch,
		Depth:           depth,
		Prompt:          runOpts.prompt,
		Mounts:          mounts,
		EnvVars:         envVars,
		Timeout:         timeout,
		OutputDir:       absOutputDir,
		ClaudeConfigDir: claudeConfigDir,
		SanitizeGit:     sanitizeGit,
		BlockedPorts:    blockedPorts,
	}

	// Step 10: Execute container run
	runErr := container.Run(ctx, opts)

	// Step 11: Update session status and print ID
	status := session.StatusSuccess
	if runErr != nil {
		status = session.StatusFailed
	}
	if updateErr := session.UpdateStatus(sessionID, status); updateErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not update session status: %v\n", updateErr)
	}

	fmt.Fprintf(os.Stderr, "botl: session complete · id: %s\n", sessionID)
	return runErr
}

// resolveEnvVarKeys checks each required env var key from the profile.
func resolveEnvVarKeys(keys []string, existingEnvVars []string) ([]string, error) {
	provided := make(map[string]bool)
	for _, env := range existingEnvVars {
		if idx := strings.Index(env, "="); idx >= 0 {
			provided[env[:idx]] = true
		}
	}

	isTTY := term.IsTerminal(int(os.Stdin.Fd()))
	var resolved []string

	for _, key := range keys {
		if provided[key] {
			continue
		}

		if val, ok := os.LookupEnv(key); ok {
			resolved = append(resolved, key+"="+val)
			continue
		}

		if !isTTY {
			return nil, fmt.Errorf("env var %s not set and stdin is not a tty; set it in the environment before running", key)
		}

		fmt.Fprintf(os.Stderr, "botl: env var %s not set · enter value: ", key)
		reader := bufio.NewReader(os.Stdin)
		val, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("env var %s not provided, aborting", key)
		}
		val = strings.TrimRight(val, "\r\n")
		if val == "" {
			return nil, fmt.Errorf("env var %s not provided, aborting", key)
		}
		resolved = append(resolved, key+"="+val)
	}

	return resolved, nil
}

// extractEnvKeys returns just the key names from KEY=VALUE env var strings.
func extractEnvKeys(envVars []string) []string {
	if len(envVars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(envVars))
	for _, env := range envVars {
		if idx := strings.Index(env, "="); idx >= 0 {
			keys = append(keys, env[:idx])
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}
