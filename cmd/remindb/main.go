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
		return absolutizeDBPath()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "memory.db", "Path to the SQLite database (compile/bench derive ./<dirname>.db when given a directory and --db is unset)")
}

func absolutizeDBPath() error {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("failed to resolve: %s: %w", dbPath, err)
	}

	dbPath = abs
	return nil
}

func deriveDefaultDBPath(cmd *cobra.Command, dir string) error {
	if cmd.Flags().Changed("db") || dir == "" {
		return nil
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve: %s: %w", dir, err)
	}

	fi, err := os.Stat(absDir)
	if err != nil || !fi.IsDir() {
		return nil
	}

	dbPath = filepath.Base(absDir) + ".db"
	return absolutizeDBPath()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
