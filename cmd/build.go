package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yarlson/ftl/pkg/build"
	"github.com/yarlson/ftl/pkg/console"
	"github.com/yarlson/ftl/pkg/executor/local"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build your application Docker images",
	Long: `Build your application Docker images as defined in ftl.yaml.
This command handles the entire build process, including
building and pushing the Docker images to the registry.`,
	Run: runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) {
	cfg, err := parseConfig("ftl.yaml")
	if err != nil {
		console.ErrPrintln("Failed to parse config file:", err)
		return
	}

	executor := local.NewExecutor()
	builder := build.NewBuild(executor)

	ctx := context.Background()

	for _, service := range cfg.Services {
		if err := builder.Build(ctx, service.Image, service.Path); err != nil {
			console.ErrPrintln(fmt.Sprintf("Failed to build image for service %s:", service.Name), err)
			continue
		}
	}

	console.Success("Build process completed successfully.")
}
