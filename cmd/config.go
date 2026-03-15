package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kamilrybacki/botl/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify botl configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all config values",
	Args:  cobra.NoArgs,
	RunE:  runConfigList,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}

var validConfigKeys = []string{"clone-mode", "blocked-ports"}

func validateConfigKey(key string) error {
	for _, k := range validConfigKeys {
		if k == key {
			return nil
		}
	}
	return fmt.Errorf("unknown config key %q; valid keys: %s", key, strings.Join(validConfigKeys, ", "))
}

func runConfigSet(_ *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	if err := validateConfigKey(key); err != nil {
		return err
	}

	cfgPath := config.Path()
	cfg, _ := config.Load(cfgPath)

	switch key {
	case "clone-mode":
		if err := config.ValidateCloneMode(value); err != nil {
			return fmt.Errorf("invalid value for clone-mode: must be \"shallow\" or \"deep\"")
		}
		cfg.Clone.Mode = value
	case "blocked-ports":
		ports, err := parsePorts(value)
		if err != nil {
			return fmt.Errorf("invalid value for blocked-ports: %w", err)
		}
		cfg.Network.BlockedPorts = ports
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "botl: %s = %s\n", key, value)
	return nil
}

func runConfigGet(_ *cobra.Command, args []string) error {
	key := args[0]
	if err := validateConfigKey(key); err != nil {
		return err
	}

	cfg, _ := config.Load(config.Path())

	switch key {
	case "clone-mode":
		fmt.Println(cfg.Clone.Mode)
	case "blocked-ports":
		if len(cfg.Network.BlockedPorts) == 0 {
			fmt.Println("(none)")
		} else {
			parts := make([]string, len(cfg.Network.BlockedPorts))
			for i, p := range cfg.Network.BlockedPorts {
				parts[i] = strconv.Itoa(p)
			}
			fmt.Println(strings.Join(parts, ", "))
		}
	}
	return nil
}

func runConfigList(_ *cobra.Command, _ []string) error {
	cfg, _ := config.Load(config.Path())

	portsLabel := "(none)"
	if len(cfg.Network.BlockedPorts) > 0 {
		parts := make([]string, len(cfg.Network.BlockedPorts))
		for i, p := range cfg.Network.BlockedPorts {
			parts[i] = strconv.Itoa(p)
		}
		portsLabel = strings.Join(parts, ", ")
	}

	fmt.Printf("%-16s%s\n", "clone-mode", cfg.Clone.Mode)
	fmt.Printf("%-16s%s\n", "blocked-ports", portsLabel)
	return nil
}

// parsePorts parses a comma-separated list of port numbers.
func parsePorts(input string) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []int{}, nil
	}

	parts := strings.Split(input, ",")
	ports := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: must be a number", part)
		}
		if err := config.ValidatePorts([]int{p}); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, nil
}
