package mcp

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 60 * time.Second
	httpIdleTimeout       = 120 * time.Second
	httpMaxHeaderBytes    = 1 << 20 // 1 MiB
	httpShutdownTimeout   = 5 * time.Second
	bearerPrefix          = "Bearer "
	bearerRealm           = `Bearer realm="remindb"`
)

func (s *Server) runHttp(ctx context.Context) error {
	ln := s.listener
	if ln == nil {
		var err error

		ln, err = net.Listen("tcp", s.listen)
		if err != nil {
			return fmt.Errorf("failed to listen: %s: %w", s.listen, err)
		}
	}

	addr := ln.Addr().String()
	nonLoopback := false
	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopbackHost(host) {
		nonLoopback = true
	}

	if nonLoopback && s.authToken == "" && !s.insecurePublic {
		_ = ln.Close()
		return fmt.Errorf("refusing to bind HTTP transport to non-loopback %s without authentication: set REMINDB_AUTH_TOKEN to enable bearer-token auth, or pass --insecure-public (REMINDB_INSECURE_PUBLIC=1) to bypass", addr)
	}
	if nonLoopback && s.authToken == "" && s.insecurePublic {
		s.logger.Warn("serve: HTTP bound to non-loopback with --insecure-public; no authentication", "listen", addr)
	}

	var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil)
	if s.authToken != "" {
		handler = bearerAuthMiddleware(s.authToken, handler)
	}
	// WriteTimeout is intentionally left at the zero value: streamable HTTP
	// keeps responses open for long-running POSTs and SSE GETs, and a
	// server-wide cap would truncate them. Slow-read defense is left to the
	// streaming handler's own per-write deadlines.
	httpSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaxHeaderBytes,
	}

	s.logger.Info("serve: HTTP transport ready", "listen", addr, "auth", s.authToken != "")

	errCh := make(chan error, 1)
	go func() {
		err := httpSrv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()

		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shut down: HTTP server: %w", err)
		}
		if err := <-errCh; err != nil {
			return err
		}
		return nil
	}
}

func bearerAuthMiddleware(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, bearerPrefix) {
			w.Header().Set("WWW-Authenticate", bearerRealm)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		got := []byte(strings.TrimPrefix(h, bearerPrefix))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", bearerRealm)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(host string) bool {
	if host == "" || host == "0.0.0.0" || host == "::" {
		return false
	}
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
