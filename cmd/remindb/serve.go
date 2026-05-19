package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/logbuf"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/mcp/rescan"
	"github.com/radimsem/remindb/pkg/mcp/rescanlog"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
	"github.com/radimsem/remindb/pkg/mcp/sessionlog"
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

const (
	defaultLogBufferSize         = 1000
	defaultSessionLogMaxFileSize = 10 << 20 // 10 MiB
	defaultRescanLogMaxFileSize  = 10 << 20 // 10 MiB
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

	if err := resolveServerConfig(cmd, workspaceCfg.Server); err != nil {
		return err
	}

	logger, logFile, logBuf, err := newServeLogger(verbose, workspaceCfg.Server.Logging)
	if err != nil {
		return err
	}
	if logFile != nil {
		defer func() { _ = logFile.Close() }()
	}

	if sourceDir != "" {
		logger, err = withSessionLogs(logger, sourceDir, workspaceCfg.Server.Logging.SessionFiles)
		if err != nil {
			return err
		}
	}

	startCfg := temperature.DefaultConfig().WithOverrides(workspaceCfg.Temperature)
	if err := startCfg.Validate(); err != nil {
		return fmt.Errorf("invalid temperature config in %s: %w", config.Path, err)
	}

	redCfg, err := applyRedactionOverrides(redaction.DefaultConfig(), workspaceCfg.Redaction)
	if err != nil {
		return fmt.Errorf("invalid redaction config in %s: %w", config.Path, err)
	}

	red, err := redaction.New(redCfg)
	if err != nil {
		return fmt.Errorf("failed to build: redactor: %w", err)
	}

	tracker, err := temperature.NewTracker(st, sourceDir, temperature.DefaultConfig(), logger)
	if err != nil {
		return err
	}

	rescanStatus := rescanstat.New()

	srv, err := remindb.NewServer(st, tracker, startCfg,
		remindb.WithSourceDir(sourceDir),
		remindb.WithLogger(logger),
		remindb.WithTransport(transport),
		remindb.WithListen(listen),
		remindb.WithWorkspaceConfig(workspaceCfg),
		remindb.WithRedactor(red),
		remindb.WithLogBuffer(logBuf),
		remindb.WithRescanStatus(rescanStatus),
	)
	if err != nil {
		return fmt.Errorf("failed to build: server: %w", err)
	}

	logBuf.SetObserver(srv.NotifyLogRecord)
	tracker.SetTickObserver(srv.NotifyTemperatureTick)

	logger.Info("serve: starting", startupAttrs(startCfg.TickInterval)...)

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
	g.Go(func() error {
		srv.RunSessionLedger(ctx)
		return nil
	})

	if sourceDir != "" {
		rescanLog, err := newRescanLog(sourceDir, workspaceCfg.Server.RescanFiles)
		if err != nil {
			return err
		}

		rescanLoop, err := rescan.New(st, sourceDir, rescanInterval,
			rescan.WithCompileConfig(workspaceCfg.Compile),
			rescan.WithLogger(logger),
			rescan.WithStatus(rescanStatus),
			rescan.WithRescanLog(rescanLog),
		)
		if err != nil {
			return err
		}
		rescanLoop.SetChangeObserver(srv.NotifyRescan)

		g.Go(func() error {
			rescanLoop.Run(ctx)
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

func applyRedactionOverrides(base redaction.Config, o config.RedactionConfig) (redaction.Config, error) {
	if len(o.DisableBuiltinKinds) > 0 {
		valid := make(map[string]bool, len(base.BuiltinKinds))
		for _, k := range base.BuiltinKinds {
			valid[k] = true
		}

		disabled := make(map[string]bool, len(o.DisableBuiltinKinds))
		for _, k := range o.DisableBuiltinKinds {
			if !valid[k] {
				return base, fmt.Errorf("unknown built-in redaction kind %q", k)
			}
			disabled[k] = true
		}

		kept := make([]string, 0, len(base.BuiltinKinds))
		for _, k := range base.BuiltinKinds {
			if !disabled[k] {
				kept = append(kept, k)
			}
		}
		base.BuiltinKinds = kept
	}

	for _, p := range o.Custom {
		base.Custom = append(base.Custom, redaction.CustomPattern{Kind: p.Kind, Pattern: p.Pattern})
	}
	return base, nil
}

// Build the serve logger from config.
func newServeLogger(verbose bool, lg config.LoggingConfig) (*slog.Logger, *os.File, *logbuf.Buffer, error) {
	level := slog.LevelInfo
	if lg.Level != nil {
		level = parseLogLevel(*lg.Level)
	}
	if verbose {
		level = slog.LevelDebug
	}

	out := os.Stderr
	var file *os.File
	if lg.OutputPath != nil {
		f, err := os.OpenFile(*lg.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to open: log output %s: %w", *lg.OutputPath, err)
		}

		out, file = f, f
	}

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(out, opts)

	if lg.Format != nil && *lg.Format == "json" {
		h = slog.NewJSONHandler(out, opts)
	}

	size := defaultLogBufferSize
	if lg.BufferSize != nil {
		size = *lg.BufferSize
	}
	buf := logbuf.NewBuffer(size)

	return slog.New(logbuf.NewHandler(h, buf)), file, buf, nil
}

// withSessionLogs wraps logger with the outermost per-session file handler when enabled; otherwise returns it unchanged.
func withSessionLogs(logger *slog.Logger, workspace string, sl config.SessionFilesConfig) (*slog.Logger, error) {
	if sl.Enabled == nil || !*sl.Enabled {
		return logger, nil
	}

	maxFileSize := int64(defaultSessionLogMaxFileSize)
	if sl.MaxFileSize != nil {
		maxFileSize = int64(*sl.MaxFileSize)
	}

	sink, err := sessionlog.New(workspace, maxFileSize)
	if err != nil {
		return nil, err
	}

	return slog.New(sessionlog.NewHandler(logger.Handler(), sink)), nil
}

// newRescanLog builds the durable rescan-tick sink when enabled; nil otherwise.
func newRescanLog(workspace string, rf config.RescanFilesConfig) (*rescanlog.Sink, error) {
	if rf.Enabled == nil || !*rf.Enabled {
		return nil, nil
	}

	maxFileSize := int64(defaultRescanLogMaxFileSize)
	if rf.MaxFileSize != nil {
		maxFileSize = int64(*rf.MaxFileSize)
	}

	return rescanlog.New(workspace, maxFileSize)
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
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

	return nil
}

func resolveServerConfig(cmd *cobra.Command, sc config.ServerConfig) error {
	transport = config.Resolve(
		cmd.Flags().Changed("transport"), transport,
		sc.Transport, envPtr("REMINDB_TRANSPORT"),
		remindb.TransportStdio,
	)
	listen = config.Resolve(
		cmd.Flags().Changed("listen"), listen,
		sc.Listen, envPtr("REMINDB_LISTEN"),
		remindb.DefaultListenAddr,
	)

	switch transport {
	case remindb.TransportStdio, remindb.TransportHttp:
	default:
		return fmt.Errorf("unsupported transport %q (want %q or %q)", transport, remindb.TransportStdio, remindb.TransportHttp)
	}

	listenSet := cmd.Flags().Changed("listen") || sc.Listen != nil || envPtr("REMINDB_LISTEN") != nil
	if transport != remindb.TransportHttp && listenSet {
		return fmt.Errorf("listen address requires --transport=http")
	}
	return nil
}

func envPtr(key string) *string {
	if v := os.Getenv(key); v != "" {
		return &v
	}
	return nil
}
