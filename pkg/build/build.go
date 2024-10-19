package build

import (
	"context"
	"fmt"
	"io"

	"github.com/yarlson/ftl/pkg/console"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
	RunCommandWithProgress(ctx context.Context, initialMsg, completeMsg string, commands []string) error
}

type Build struct {
	executor Executor
}

func NewBuild(executor Executor) *Build {
	return &Build{executor: executor}
}

func (b *Build) Build(ctx context.Context, image string, path string) error {
	if err := console.ProgressSpinner(
		context.Background(),
		fmt.Sprintf("Building image %s", image), fmt.Sprintf("Image %s built", image),
		[]func() error{
			func() error { return b.buildImage(ctx, image, path) },
		}); err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return nil
}

func (b *Build) buildImage(ctx context.Context, image, path string) error {
	_, err := b.executor.RunCommand(ctx, "docker", "build", "-t", image, "--platform", "linux/amd64", path)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	return nil
}

func (b *Build) Push(ctx context.Context, image string) error {
	if err := console.ProgressSpinner(
		context.Background(),
		fmt.Sprintf("Pushing image %s", image),
		fmt.Sprintf("Image %s pushed", image),
		[]func() error{
			func() error { return b.pushImage(ctx, image) },
		}); err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}

func (b *Build) pushImage(ctx context.Context, image string) error {
	_, err := b.executor.RunCommand(ctx, "docker", "push", image)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}
