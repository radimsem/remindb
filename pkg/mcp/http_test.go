package mcp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/temperature"
)

func newHttpTestServer(t *testing.T, ln net.Listener, opts ...Option) *Server {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()
	cfg.TickInterval = time.Minute

	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	full := append([]Option{
		WithTransport(TransportHttp),
		WithListener(ln),
	}, opts...)

	srv, err := NewServer(st, tracker, cfg, full...)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestBearerAuthMiddleware_NoHeader_Returns401(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := bearerAuthMiddleware("secret", next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != bearerRealm {
		t.Errorf("WWW-Authenticate = %q, want %q", got, bearerRealm)
	}

	if called {
		t.Error("next handler called on missing auth")
	}
}

func TestBearerAuthMiddleware_WrongToken_Returns401(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := bearerAuthMiddleware("secret", next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if called {
		t.Error("next handler called on wrong token")
	}
}

func TestBearerAuthMiddleware_CorrectToken_Passes(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	h := bearerAuthMiddleware("secret", next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler not called with valid token")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRunHttp_RefusesNonLoopbackWithoutAuth(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := newHttpTestServer(t, ln)

	err = srv.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil, want refusal error")
	}

	if !strings.Contains(err.Error(), "refusing to bind") {
		t.Errorf("err = %v, want contains 'refusing to bind'", err)
	}
	if !strings.Contains(err.Error(), "REMINDB_AUTH_TOKEN") {
		t.Errorf("err = %v, want contains 'REMINDB_AUTH_TOKEN'", err)
	}
}

func TestRunHttp_LoopbackUnauth_StartsClean(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := newHttpTestServer(t, ln, WithLogger(logger))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "HTTP transport ready") {
		t.Errorf("expected HTTP transport ready log, got: %s", logs)
	}
	if strings.Contains(logs, "insecure-public") {
		t.Errorf("unexpected insecure-public warn for loopback bind: %s", logs)
	}
}

func TestRunHttp_WithAuthToken_EndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := newHttpTestServer(t, ln, WithAuthToken("secret"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	time.Sleep(50 * time.Millisecond)

	noAuth, err := http.Post("http://"+addr, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post no-auth: %v", err)
	}

	_ = noAuth.Body.Close()
	if noAuth.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-auth status = %d, want %d", noAuth.StatusCode, http.StatusUnauthorized)
	}
	if got := noAuth.Header.Get("WWW-Authenticate"); got != bearerRealm {
		t.Errorf("no-auth WWW-Authenticate = %q, want %q", got, bearerRealm)
	}

	req, err := http.NewRequest("POST", "http://"+addr, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")

	withAuth, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post auth: %v", err)
	}

	_ = withAuth.Body.Close()
	if withAuth.StatusCode == http.StatusUnauthorized {
		t.Errorf("authenticated status = 401, want non-401 (reached MCP layer)")
	}
}

func TestRunHttp_ReadHeaderTimeout_ClosesSlowClient(t *testing.T) {
	if testing.Short() {
		t.Skipf("skipping Slowloris regression in -short mode (waits ~%s)", httpReadHeaderTimeout)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := newHttpTestServer(t, ln)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	time.Sleep(50 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\n")); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	// Client deadline is intentionally much larger than the server's
	// ReadHeaderTimeout. If the client deadline fires first, the server
	// did not enforce its timeout and the test must fail — not pass on
	// the client-side timeout it accidentally observed.
	clientDeadline := httpReadHeaderTimeout + 10*time.Second
	if err := conn.SetReadDeadline(time.Now().Add(clientDeadline)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	buf := make([]byte, 256)
	start := time.Now()
	_, readErr := conn.Read(buf)
	elapsed := time.Since(start)

	var netErr net.Error
	if errors.As(readErr, &netErr) && netErr.Timeout() {
		t.Fatalf("client deadline fired after %v before server closed — server did not enforce ReadHeaderTimeout", elapsed)
	}
	if readErr == nil {
		t.Fatalf("read returned nil err after %v — server did not enforce ReadHeaderTimeout", elapsed)
	}
	if elapsed > httpReadHeaderTimeout+3*time.Second {
		t.Errorf("server closed after %v, want within %v", elapsed, httpReadHeaderTimeout+3*time.Second)
	}
}

func TestRunHttp_NonLoopbackInsecurePublic_LogsWarn(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := newHttpTestServer(t, ln, WithLogger(logger), WithInsecurePublic(true))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "insecure-public") {
		t.Errorf("expected insecure-public warn in logs, got: %s", logs)
	}
}
