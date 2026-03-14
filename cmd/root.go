package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "botl",
	Short: "Run Claude Code in an ephemeral Docker container",
	Long:  "botl launches Claude Code inside a temporary Docker container with read-only access to host packages and a shallow-cloned git repo as workspace.",
}

func Execute() error {
	return rootCmd.Execute()
}
