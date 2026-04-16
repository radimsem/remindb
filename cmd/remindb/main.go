package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "remindb",
	Short: "Token-efficient agentic memory database",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "brain.db", "Path to the SQLite database")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
