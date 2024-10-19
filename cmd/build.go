package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/yarlson/ftl/pkg/build"
	"github.com/yarlson/ftl/pkg/console"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build your application Docker image",
	Long: `Build your application Docker image as defined in ftl.yaml.
This command handles the entire build process, including
building and pushing the Docker image to the registry.`,
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

	executor := &localExecutor{}
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

type localExecutor struct{}

func (e *localExecutor) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}
	return bytes.NewReader(output), nil
}
