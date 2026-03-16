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

// validEnvKeyRe matches valid environment variable key names.
var validEnvKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

// resolvedConfig holds the merged configuration from config file, profile, and CLI flags.
type resolvedConfig struct {
	cloneMode    string
	blockedPorts []int
	depth        int
	timeout      time.Duration
	image        string
	outputDir    string
	envVars      []string
	sanitizeGit  bool
}

func resolveClaudeConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	claudeConfigDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeConfigDir); os.IsNotExist(err) {
		return "", fmt.Errorf("~/.claude not found — run 'claude' once on your host to authenticate first")
	}
	return claudeConfigDir, nil
}

func loadRunConfig(cmd *cobra.Command) (resolvedConfig, error) {
	cfg, loadErr := config.Load(config.Path())
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not load config: %v\n", loadErr)
	}

	rc := resolvedConfig{
		cloneMode:    cfg.Clone.Mode,
		blockedPorts: cfg.Network.BlockedPorts,
		depth:        runOpts.depth,
		timeout:      runOpts.timeout,
		image:        runOpts.image,
		outputDir:    runOpts.outputDir,
		envVars:      runOpts.envVars,
	}

	var profileEnvKeys []string
	if runOpts.withLabel != "" {
		if err := profile.ValidateName(runOpts.withLabel); err != nil {
			return rc, err
		}
		p, err := profile.Load(runOpts.withLabel)
		if err != nil {
			return rc, fmt.Errorf("profile %q not found (%s/%s.yaml)", runOpts.withLabel, profile.Dir(), runOpts.withLabel)
		}
		fmt.Fprintf(os.Stderr, "botl: loading profile %q\n", runOpts.withLabel)
		applyProfile(cmd, &rc, p)
		profileEnvKeys = p.Run.EnvVarKeys
	}

	if cmd.Flags().Changed("clone-mode") {
		rc.cloneMode = runOpts.cloneMode
	}
	if cmd.Flags().Changed("blocked-ports") {
		rc.blockedPorts = runOpts.blockedPorts
	}

	if err := config.ValidateCloneMode(rc.cloneMode); err != nil {
		return rc, err
	}
	if err := config.ValidatePorts(rc.blockedPorts); err != nil {
		return rc, err
	}

	rc.sanitizeGit = resolveDepthAndSanitize(cmd, &rc)

	if len(profileEnvKeys) > 0 {
		resolved, err := resolveEnvVarKeys(profileEnvKeys, rc.envVars)
		if err != nil {
			return rc, err
		}
		rc.envVars = append(rc.envVars, resolved...)
	}

	return rc, nil
}

func applyProfile(cmd *cobra.Command, rc *resolvedConfig, p profile.Profile) {
	if !cmd.Flags().Changed("clone-mode") {
		rc.cloneMode = p.Run.CloneMode
	}
	if !cmd.Flags().Changed("blocked-ports") {
		rc.blockedPorts = p.Run.BlockedPorts
	}
	if !cmd.Flags().Changed("depth") {
		rc.depth = p.Run.Depth
	}
	if !cmd.Flags().Changed("timeout") {
		rc.timeout = p.Run.Timeout
	}
	if !cmd.Flags().Changed("image") {
		rc.image = p.Run.Image
	}
	if !cmd.Flags().Changed("output-dir") {
		rc.outputDir = p.Run.OutputDir
	}
}

func resolveDepthAndSanitize(cmd *cobra.Command, rc *resolvedConfig) bool {
	if !cmd.Flags().Changed("depth") && runOpts.withLabel == "" {
		if rc.cloneMode == "shallow" {
			rc.depth = 1
			return true
		}
		rc.depth = 0
		return false
	}
	return rc.cloneMode == "shallow"
}

func validateInputs(repoURL string, envVars []string) error {
	if runOpts.branch != "" && !validBranchRe.MatchString(runOpts.branch) {
		return fmt.Errorf("invalid branch name %q: must match [a-zA-Z0-9._/~^:@{}[]-]", runOpts.branch)
	}
	if !validRepoURLRe.MatchString(repoURL) {
		return fmt.Errorf("invalid repo URL %q: must be https://, git@, or ssh:// URL", repoURL)
	}
	for _, env := range envVars {
		key := env
		if idx := strings.Index(env, "="); idx >= 0 {
			key = env[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(key), "BOTL_") {
			return fmt.Errorf("environment variable %q uses reserved BOTL_ namespace", key)
		}
	}
	return nil
}

func prepareMounts() ([]container.Mount, error) {
	var mounts []container.Mount
	if runOpts.mountPackages {
		for _, d := range detect.HostPackages() {
			fmt.Fprintf(os.Stderr, "botl: auto-mounting %s → %s (ro)\n", d.Source, d.Target)
			mounts = append(mounts, d)
		}
	}
	for _, m := range runOpts.mounts {
		parsed, err := container.ParseMount(m)
		if err != nil {
			return nil, fmt.Errorf("invalid mount %q: %w", m, err)
		}
		mounts = append(mounts, parsed)
	}
	return mounts, nil
}

func runRun(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	sessionID, err := session.GenerateUniqueID()
	if err != nil {
		return err
	}

	claudeConfigDir, err := resolveClaudeConfig()
	if err != nil {
		return err
	}

	rc, err := loadRunConfig(cmd)
	if err != nil {
		return err
	}

	if err := validateInputs(repoURL, rc.envVars); err != nil {
		return err
	}

	absOutputDir, err := filepath.Abs(rc.outputDir)
	if err != nil {
		return fmt.Errorf("invalid output dir: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0700); err != nil {
		return fmt.Errorf("cannot create output dir %s: %w", absOutputDir, err)
	}

	rec := session.Record{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		RepoURL:   repoURL,
		Branch:    runOpts.branch,
		Status:    session.StatusPending,
		Run: runconfig.RunConfig{
			CloneMode:    rc.cloneMode,
			Depth:        rc.depth,
			BlockedPorts: rc.blockedPorts,
			Timeout:      rc.timeout,
			Image:        rc.image,
			OutputDir:    absOutputDir,
			EnvVarKeys:   extractEnvKeys(rc.envVars),
		},
	}
	if writeErr := session.Write(rec); writeErr != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not write session record: %v\n", writeErr)
	}

	fmt.Fprintf(os.Stderr, "botl: session id: %s\n", sessionID)

	mounts, err := prepareMounts()
	if err != nil {
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

	runErr := container.Run(ctx, container.RunOpts{
		Image:           rc.image,
		RepoURL:         repoURL,
		Branch:          runOpts.branch,
		Depth:           rc.depth,
		Prompt:          runOpts.prompt,
		Mounts:          mounts,
		EnvVars:         rc.envVars,
		Timeout:         rc.timeout,
		OutputDir:       absOutputDir,
		ClaudeConfigDir: claudeConfigDir,
		SanitizeGit:     rc.sanitizeGit,
		BlockedPorts:    rc.blockedPorts,
	})

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
		// Validate key format and namespace
		if !validEnvKeyRe.MatchString(key) {
			return nil, fmt.Errorf("invalid env var key %q in profile", key)
		}
		if strings.HasPrefix(strings.ToUpper(key), "BOTL_") {
			return nil, fmt.Errorf("env var key %q in profile uses reserved BOTL_ namespace", key)
		}

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
		if strings.ContainsAny(val, "\n\r\x00") {
			return nil, fmt.Errorf("env var %s value contains invalid characters", key)
		}
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
