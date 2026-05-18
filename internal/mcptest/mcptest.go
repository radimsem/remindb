package mcptest

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/logbuf"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Env struct {
	Session *mcp.ClientSession
	Store   *store.Store

	// RescanDir is the source dir a NewEnvWithRescan loop watches; "" otherwise.
	RescanDir string

	// WorkspaceDir is the source dir whose .remindb/ holds the session ledger.
	WorkspaceDir string

	srv *remindb.Server
}

// FlushSessions forces a session-ledger flush so tests observe it deterministically.
func (e *Env) FlushSessions() { e.srv.FlushSessions() }

func NewEnv(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()
	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	srv, err := remindb.NewServer(st, tracker, cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err = srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return &Env{Session: session, Store: st}
}

func NewEnvWithLog(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()

	buf := logbuf.NewBuffer(64)
	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(logbuf.NewHandler(base, buf))

	tracker, err := temperature.NewTracker(st, "", cfg, logger)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	srv, err := remindb.NewServer(st, tracker, cfg,
		remindb.WithLogger(logger),
		remindb.WithLogBuffer(buf),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err = srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return &Env{Session: session, Store: st}
}

func NewHttpEnv(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()

	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv, err := remindb.NewServer(st, tracker, cfg,
		remindb.WithTransport(remindb.TransportHttp),
		remindb.WithListener(ln),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.1.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: "http://" + ln.Addr().String()}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		cancel()
		<-runErr
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		if err := <-runErr; err != nil {
			t.Logf("server run returned: %v", err)
		}
	})

	return &Env{Session: session, Store: st}
}

func NewEnvWithRescan(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()
	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir .remindb: %v", err)
	}
	cfgJSON := `{"rescan":{"interval":"1s","settle":"1ms"}}`
	if err := os.WriteFile(filepath.Join(cfgDir, config.FileName), []byte(cfgJSON), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	status := rescanstat.New()
	srv, err := remindb.NewServer(st, tracker, cfg, remindb.WithRescanStatus(status))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	loop, err := remindb.NewRescanLoop(st, dir, time.Second, config.CompileConfig{}, nil, status)
	if err != nil {
		t.Fatalf("NewRescanLoop: %v", err)
	}

	loopCtx, cancelLoop := context.WithCancel(context.Background())
	t.Cleanup(cancelLoop)
	go loop.Run(loopCtx)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err = srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return &Env{Session: session, Store: st, RescanDir: dir}
}

func NewEnvWithSessionLedger(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()

	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	dir := t.TempDir()
	srv, err := remindb.NewServer(st, tracker, cfg, remindb.WithSourceDir(dir))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := srv.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return &Env{Session: session, Store: st, WorkspaceDir: dir, srv: srv}
}

func (e *Env) CallTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	t.Logf("→ %s %v", name, args)

	result, err := e.Session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}

	if result.IsError {
		msg := "unknown error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		t.Fatalf("CallTool %s returned error: %s", name, msg)
	}

	text := e.textPreview(result)
	t.Logf("← %s: %s", name, text)

	return result
}

func (e *Env) ReadResource(t *testing.T, uri string) string {
	t.Helper()

	res, err := e.Session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource %s: %v", uri, err)
	}

	if len(res.Contents) != 1 {
		t.Fatalf("ReadResource %s: contents=%d, want 1", uri, len(res.Contents))
	}
	return res.Contents[0].Text
}

func (e *Env) TextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty content in result")
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	return tc.Text
}

const maxPreview = 120

func (e *Env) textPreview(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return "(empty)"
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return "(non-text)"
	}

	s := tc.Text
	if len(s) <= maxPreview {
		return s
	}

	return s[:maxPreview] + "..."
}
