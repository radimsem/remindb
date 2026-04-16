package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var sourceDir string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server with background rescan and temperature tracking",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&sourceDir, "source", "", "Source directory to watch for changes")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	cfg := temperature.DefaultConfig()
	tracker := temperature.NewTracker(st, cfg)

	srv := remindb.NewServer(st, tracker)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return srv.Run(ctx)
	})

	g.Go(func() error {
		tracker.Run(ctx, func(ctx context.Context, nodes []*store.Node) {
			log.Printf("cold nodes detected: %d", len(nodes))
		})
		return nil
	})

	if sourceDir != "" {
		rescan := remindb.NewRescanLoop(st, sourceDir, 0)
		g.Go(func() error {
			rescan.Run(ctx)
			return nil
		})
	}

	return g.Wait()
}
