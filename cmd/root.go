package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "botl",
	Short:         "Run Claude Code in an ephemeral Docker container",
	Long:          "botl launches Claude Code inside a temporary Docker container with read-only access to host packages and a shallow-cloned git repo as workspace.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "botl: error: %s\n", err)
		return err
	}
	return nil
}
