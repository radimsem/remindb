package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const httpShutdownTimeout = 5 * time.Second

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
	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopbackHost(host) {
		s.logger.Warn("serve: HTTP bound to non-loopback; configure authentication before exposing publicly", "listen", addr)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil)
	httpSrv := &http.Server{Handler: handler}

	s.logger.Info("serve: HTTP transport ready", "listen", addr)

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
