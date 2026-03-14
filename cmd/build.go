package cmd

import (
	"context"
	"fmt"

	"github.com/kamilrybacki/botl/internal/container"
	"github.com/spf13/cobra"
)

var buildOpts struct {
	image string
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the botl Docker image",
	RunE:  runBuild,
}

func init() {
	buildCmd.Flags().StringVar(&buildOpts.image, "image", "botl:latest", "Image tag to build")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("botl: building Docker image...")
	if err := container.BuildImage(context.Background(), buildOpts.image); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	fmt.Println("botl: image built successfully")
	return nil
}
