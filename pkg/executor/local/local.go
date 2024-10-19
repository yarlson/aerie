package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/yarlson/ftl/pkg/console"
)

type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}
	return bytes.NewReader(output), nil
}

func (e *Executor) RunCommandWithProgress(ctx context.Context, initialMsg, completeMsg string, commands []string) error {
	operations := make([]func() error, len(commands))
	for i, cmdString := range commands {
		command, args := splitCommandAndArgs(cmdString)
		operations[i] = func() error {
			_, err := e.RunCommand(ctx, command, args...)
			return err
		}
	}
	return console.ProgressSpinner(ctx, initialMsg, completeMsg, operations)
}

func splitCommandAndArgs(cmdString string) (string, []string) {
	parts := strings.Fields(cmdString)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
