package console

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

var (
	Info       = color.New(color.FgCyan).PrintlnFunc()
	Success    = color.New(color.FgGreen).PrintlnFunc()
	Warning    = color.New(color.FgYellow).PrintlnFunc()
	ErrPrintln = color.New(color.FgRed).PrintlnFunc()
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
