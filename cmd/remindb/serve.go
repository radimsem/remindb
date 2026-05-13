package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	sourceDir      string
	rescanInterval time.Duration
	verbose        bool
	transport      string
	listen         string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server with background rescan and temperature tracking",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&sourceDir, "source", "", "Source directory to watch for changes (falls back to REMINDB_SOURCE)")
	serveCmd.Flags().DurationVar(&rescanInterval, "rescan-interval", 0, "Rescan interval (e.g. 30s, 5m); 0 uses default (falls back to REMINDB_RESCAN_INTERVAL)")
	serveCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Emit debug-level logs (default level is info)")
	serveCmd.Flags().StringVar(&transport, "transport", remindb.TransportStdio, "Transport for the MCP server (stdio|http); falls back to REMINDB_TRANSPORT")
	serveCmd.Flags().StringVar(&listen, "listen", remindb.DefaultListenAddr, "Listen address for HTTP transport (ignored for stdio); falls back to REMINDB_LISTEN")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	if err := applyServeEnv(cmd); err != nil {
		return err
	}

	logger := newServeLogger(verbose)

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
	tracker, err := temperature.NewTracker(st, cfg, logger)
	if err != nil {
		return err
	}

	srv := remindb.NewServer(st, tracker, cfg,
		remindb.WithSourceDir(sourceDir),
		remindb.WithLogger(logger),
		remindb.WithTransport(transport),
		remindb.WithListen(listen),
	)

	logger.Info("serve: starting",
		"db", dbPath,
		"source", sourceDir,
		"rescan_interval", rescanInterval,
		"tick_interval", cfg.TickInterval,
		"transport", transport,
		"listen", listen,
		"verbose", verbose,
		"version", version,
	)

	go checkLatestVersion(ctx, version, logger)

	if sourceDir != "" {
		if err := remindb.MaybeInitialCompile(ctx, st, sourceDir, logger); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)
	defer cancel()

	g.Go(func() error {
		defer cancel()
		return srv.Run(ctx)
	})
	g.Go(func() error {
		tracker.Run(ctx, func(ctx context.Context, nodes []*store.Node) {
			logger.Info("cold nodes detected", "count", len(nodes))
			tracker.MarkNotified(srv.NotifyColdNodes(ctx, nodes))
		})
		return nil
	})

	if sourceDir != "" {
		rescan, err := remindb.NewRescanLoop(st, sourceDir, rescanInterval, logger)
		if err != nil {
			return err
		}
		g.Go(func() error {
			rescan.Run(ctx)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		logger.Error("serve: stopped with error", "err", err)
		return err
	}
	logger.Info("serve: stopped")
	return nil
}

func newServeLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func applyServeEnv(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("db") {
		if v := os.Getenv("REMINDB_DB"); v != "" {
			dbPath = v
			if err := absolutizeDBPath(); err != nil {
				return err
			}
		}
	}

	if sourceDir == "" {
		sourceDir = os.Getenv("REMINDB_SOURCE")
	}

	if !cmd.Flags().Changed("rescan-interval") {
		if v := os.Getenv("REMINDB_RESCAN_INTERVAL"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("failed to parse: REMINDB_RESCAN_INTERVAL=%q: %w", v, err)
			}
			rescanInterval = d
		}
	}

	if !cmd.Flags().Changed("transport") {
		if v := os.Getenv("REMINDB_TRANSPORT"); v != "" {
			transport = v
		}
	}
	if !cmd.Flags().Changed("listen") {
		if v := os.Getenv("REMINDB_LISTEN"); v != "" {
			listen = v
		}
	}

	switch transport {
	case remindb.TransportStdio, remindb.TransportHttp:
	default:
		return fmt.Errorf("unsupported transport %q (want %q or %q)", transport, remindb.TransportStdio, remindb.TransportHttp)
	}
	return nil
}
