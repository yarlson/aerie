package console

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

var (
	Info       = color.New(color.FgCyan).PrintlnFunc()
	Success    = color.New(color.FgGreen).PrintlnFunc()
	Warning    = color.New(color.FgYellow).PrintlnFunc()
	ErrPrintln = color.New(color.FgRed).PrintlnFunc()
	ErrPrintf  = color.New(color.FgRed).PrintfFunc()
	Input      = color.New(color.FgYellow).PrintFunc()
)

func ProgressSpinner(ctx context.Context, initialMsg, completeMsg string, operations []func() error) error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + initialMsg
	_ = s.Color("yellow")
	s.Start()

	defer func() {
		s.Stop()
		fmt.Print("\r")
		fmt.Print(strings.Repeat(" ", len(s.Suffix)+3))
		fmt.Print("\r")
	}()

	for _, operation := range operations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := operation()
			if err != nil {
				s.Stop()
				errorMessage := fmt.Sprintf(
					"\n%s Operation failed\n%s %v\n",
					color.New(color.FgRed).SprintFunc()("X"),
					color.New(color.FgRed).SprintFunc()("Error:"),
					err,
				)
				fmt.Println(errorMessage)
				return fmt.Errorf("operation failed: %w", err)
			}
		}
	}

	s.Stop()
	fmt.Print("\r")
	fmt.Print(strings.Repeat(" ", len(s.Suffix)+3))
	fmt.Print("\r")
	checkMark := color.New(color.FgGreen).SprintFunc()("√")
	fmt.Printf("%s %s\n", checkMark, completeMsg)

	return nil
}

func ReadLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func ReadPassword() (string, error) {
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(password), nil
}
