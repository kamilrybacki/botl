package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/kamilrybacki/botl/internal/profile"
	"github.com/kamilrybacki/botl/internal/session"
	"github.com/spf13/cobra"
)

var labelForce bool

var labelCmd = &cobra.Command{
	Use:   "label <session-id> <name>",
	Short: "Save a session's run config as a reusable profile",
	Args:  cobra.ExactArgs(2),
	RunE:  runLabel,
}

func init() {
	labelCmd.Flags().BoolVar(&labelForce, "force", false, "Overwrite existing profile")
	rootCmd.AddCommand(labelCmd)
}

func runLabel(_ *cobra.Command, args []string) error {
	sessionID, name := args[0], args[1]

	if err := profile.ValidateName(name); err != nil {
		return err
	}

	rec, err := session.Read(sessionID)
	if err != nil {
		return err
	}

	if rec.Status != session.StatusSuccess {
		return fmt.Errorf("session %q did not complete successfully (status: %s)", sessionID, rec.Status)
	}

	if profile.Exists(name) && !labelForce {
		return fmt.Errorf("profile %q already exists (use --force to overwrite)", name)
	}

	p := profile.Profile{
		Name:          name,
		CreatedAt:     time.Now().UTC(),
		SourceSession: sessionID,
		Run:           rec.Run,
	}

	if err := profile.Save(p); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "botl: profile %q saved (%s/%s.yaml)\n", name, profile.Dir(), name)
	return nil
}
