package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/radimsem/remindb/internal/bench"
	"github.com/spf13/cobra"
)

var (
	benchDir     string
	benchBudget  int
	benchQueries []string
)

var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Measure token efficiency of remindb tools against raw-file baselines",
	RunE:  runBench,
}

func init() {
	benchCmd.Flags().StringVar(&benchDir, "dir", "", "Source directory to compile and benchmark (required)")
	benchCmd.Flags().IntVar(&benchBudget, "budget", 1000, "Token budget for search and fetch scenarios")
	benchCmd.Flags().StringArrayVar(&benchQueries, "query", nil, "Search query (repeatable); skips the search scenario if empty")
	rootCmd.AddCommand(benchCmd)
}

func runBench(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	if benchDir == "" {
		return errors.New("--dir is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return bench.Run(ctx, bench.Config{
		Dir:     benchDir,
		Budget:  benchBudget,
		Queries: benchQueries,
		Out:     os.Stdout,
		Stderr:  os.Stderr,
	})
}
