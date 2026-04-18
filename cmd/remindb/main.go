package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "remindb",
	Short: "Token-efficient agentic memory database",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		abs, err := filepath.Abs(dbPath)
		if err != nil {
			return fmt.Errorf("failed to resolve: %s: %w", dbPath, err)
		}

		dbPath = abs
		return nil
	},
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
