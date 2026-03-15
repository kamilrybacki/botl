package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Manage saved profiles",
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	Args:  cobra.NoArgs,
	RunE:  runProfilesList,
}

var profilesShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's configuration",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesShow,
}

var profilesDeleteYes bool

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfilesDelete,
}

func init() {
	profilesDeleteCmd.Flags().BoolVarP(&profilesDeleteYes, "yes", "y", false, "Skip confirmation prompt")
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesShowCmd)
	profilesCmd.AddCommand(profilesDeleteCmd)
	rootCmd.AddCommand(profilesCmd)
}

func runProfilesList(_ *cobra.Command, _ []string) error {
	profiles, err := profile.List()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Fprintln(os.Stderr, "botl: no profiles found — use 'botl label <session-id> <name>' to create one")
		return nil
	}

	fmt.Printf("%-20s %-12s %s\n", "NAME", "CREATED", "SESSION")
	for _, p := range profiles {
		fmt.Printf("%-20s %-12s %s\n", p.Name, p.CreatedAt.Format("2006-01-02"), p.SourceSession)
	}
	return nil
}

func runProfilesShow(_ *cobra.Command, args []string) error {
	name := args[0]
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	p, err := profile.Load(name)
	if err != nil {
		return fmt.Errorf("profile %q not found", name)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	fmt.Print(string(data))

	if len(p.Run.EnvVarKeys) > 0 {
		fmt.Fprintf(os.Stderr, "# note: this profile requires env vars: %s\n", strings.Join(p.Run.EnvVarKeys, ", "))
	}
	return nil
}

func runProfilesDelete(_ *cobra.Command, args []string) error {
	name := args[0]

	if err := profile.ValidateName(name); err != nil {
		return err
	}

	if !profile.Exists(name) {
		return fmt.Errorf("profile %q not found", name)
	}

	if !profilesDeleteYes {
		fmt.Fprintf(os.Stderr, "Delete profile %q? [y/N] ", name)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "botl: cancelled")
			return nil
		}
	}

	if err := profile.Delete(name); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "botl: profile %q deleted\n", name)
	return nil
}
