package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/yarlson/ftl/pkg/build"
	"github.com/yarlson/ftl/pkg/console"
	"github.com/yarlson/ftl/pkg/executor/local"
)

var (
	noPush bool

	buildCmd = &cobra.Command{
		Use:   "build",
		Short: "Build your application Docker images",
		Long: `Build your application Docker images as defined in ftl.yaml.
This command handles the entire build process, including
building and pushing the Docker images to the registry.`,
		Run: runBuild,
	}
)

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().BoolVar(&noPush, "no-push", false, "Build images without pushing to registry")
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
			console.ErrPrintf("Failed to build image for service %s: %v\n", service.Name, err)
			continue
		}
		if !noPush {
			if err := builder.Push(ctx, service.Image); err != nil {
				console.ErrPrintf("Failed to push image for service %s: %v\n", service.Name, err)
				continue
			}
		}
	}

	message := "Build process completed successfully."
	if noPush {
		message += " Images were not pushed due to --no-push flag."
	}
	console.Success(message)
}
