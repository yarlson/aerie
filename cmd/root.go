package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ftl",
	Short: "FTL - Faster Than Light deployment tool",
	Long: `FTL (Faster Than Light) is a powerful deployment tool that simplifies 
the process of setting up servers and deploying applications. It's designed 
to make deployment easy and reliable, even for developers who aren't experts 
in server management or advanced deployment techniques.

Use 'ftl [command] --help' for more information about a command.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}
