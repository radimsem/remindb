package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/radimsem/remindb/pkg/config"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
	"github.com/radimsem/remindb/pkg/version"
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
	serveCmd.Flags().DurationVar(&rescanInterval, "rescan-interval", 0, "Rescan interval (e.g. 30s, 5m), requires --source; 0 uses default (falls back to REMINDB_RESCAN_INTERVAL)")
	serveCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Emit debug-level logs (default level is info)")
	serveCmd.Flags().StringVar(&transport, "transport", remindb.TransportStdio, "Transport for the MCP server (stdio|http); falls back to REMINDB_TRANSPORT")
	serveCmd.Flags().StringVar(&listen, "listen", remindb.DefaultListenAddr, "Listen address for HTTP transport, requires --transport=http; falls back to REMINDB_LISTEN")
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

	var workspaceCfg config.Config
	if sourceDir != "" {
		workspaceCfg, err = config.Load(sourceDir)
		if err != nil {
			return fmt.Errorf("failed to load: workspace config: %w", err)
		}
	}

	cfg := applyTemperatureOverrides(temperature.DefaultConfig(), workspaceCfg.Temperature)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid temperature config in %s: %w", config.Path, err)
	}

	tracker, err := temperature.NewTracker(st, cfg, logger)
	if err != nil {
		return err
	}

	srv, err := remindb.NewServer(st, tracker, cfg,
		remindb.WithSourceDir(sourceDir),
		remindb.WithLogger(logger),
		remindb.WithTransport(transport),
		remindb.WithListen(listen),
		remindb.WithWorkspaceConfig(workspaceCfg),
	)
	if err != nil {
		return fmt.Errorf("failed to build: server: %w", err)
	}

	logger.Info("serve: starting", startupAttrs(cfg.TickInterval)...)

	go checkLatestVersion(ctx, version.Get(), logger)

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

func applyTemperatureOverrides(base temperature.Config, o config.TemperatureConfig) temperature.Config {
	if o.DecayRate != nil {
		base.DecayRate = *o.DecayRate
	}
	if o.AccessBoost != nil {
		base.AccessBoost = *o.AccessBoost
	}
	if o.ColdThreshold != nil {
		base.ColdThreshold = *o.ColdThreshold
	}
	if o.NotifyThreshold != nil {
		base.NotifyThreshold = *o.NotifyThreshold
	}
	if o.SummarizeRebound != nil {
		base.SummarizeRebound = *o.SummarizeRebound
	}
	if o.TickInterval != nil {
		base.TickInterval = time.Duration(*o.TickInterval)
	}
	if o.ColdNotifyTTL != nil {
		base.ColdNotifyTTL = time.Duration(*o.ColdNotifyTTL)
	}
	if o.ColdNotifyLimit != nil {
		base.ColdNotifyLimit = *o.ColdNotifyLimit
	}
	return base
}

func newServeLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func startupAttrs(tickInterval time.Duration) []any {
	attrs := []any{
		"db", dbPath,
		"transport", transport,
		"tick_interval", tickInterval,
		"verbose", verbose,
		"version", version.Get(),
	}

	if sourceDir != "" {
		attrs = append(attrs, "source", sourceDir, "rescan_interval", rescanInterval)
	}
	if transport == remindb.TransportHttp {
		attrs = append(attrs, "listen", listen)
	}

	return attrs
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

	rescanFromEnv := false
	if !cmd.Flags().Changed("rescan-interval") {
		if v := os.Getenv("REMINDB_RESCAN_INTERVAL"); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("failed to parse: REMINDB_RESCAN_INTERVAL=%q: %w", v, err)
			}

			rescanInterval = d
			rescanFromEnv = true
		}
	}

	if sourceDir == "" && (cmd.Flags().Changed("rescan-interval") || rescanFromEnv) {
		return fmt.Errorf("rescan interval requires --source (or REMINDB_SOURCE)")
	}

	if !cmd.Flags().Changed("transport") {
		if v := os.Getenv("REMINDB_TRANSPORT"); v != "" {
			transport = v
		}
	}

	listenFromEnv := false
	if !cmd.Flags().Changed("listen") {
		if v := os.Getenv("REMINDB_LISTEN"); v != "" {
			listen = v
			listenFromEnv = true
		}
	}

	switch transport {
	case remindb.TransportStdio, remindb.TransportHttp:
	default:
		return fmt.Errorf("unsupported transport %q (want %q or %q)", transport, remindb.TransportStdio, remindb.TransportHttp)
	}

	if transport != remindb.TransportHttp && (cmd.Flags().Changed("listen") || listenFromEnv) {
		return fmt.Errorf("listen address requires --transport=http")
	}
	return nil
}
