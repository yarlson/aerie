package logfmt

import "github.com/fatih/color"

var (
	Info       = color.New(color.FgCyan).PrintlnFunc()
	Success    = color.New(color.FgGreen).PrintlnFunc()
	Warning    = color.New(color.FgYellow).PrintlnFunc()
	ErrPrintln = color.New(color.FgRed).PrintlnFunc()
)
